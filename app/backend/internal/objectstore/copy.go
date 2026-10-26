package objectstore

import (
	"context"
	"errors"
	"sort"
)

// CopyHooks persist provider facts under the caller's live domain/job fences.
// Each callback is short; no provider request runs in its database transaction.
type CopyHooks struct {
	Check  func() error
	Part   func(Part) error
	Seal   func([]Part) error
	Frozen func(Source) error
	Hash   func(HashCheckpoint) error
}
type CopyState struct {
	Source         Source
	Target         Multipart
	Layout         Layout
	Manifest       []Part
	Frozen         *Source
	Hash           HashCheckpoint
	ExpectedSHA256 string
}

func ListExactParts(ctx context.Context, store Store, target Multipart, layout Layout) ([]Part, error) {
	parts := []Part{}
	seen := map[int]bool{}
	markers := map[string]bool{}
	marker := ""
	for {
		page, err := store.Parts(ctx, target, marker)
		if err != nil {
			return nil, err
		}
		for _, p := range page.Items {
			size, err := layout.PartSize(p.Number)
			if err != nil || p.Size != size || !validETag(p.ETag) || seen[p.Number] {
				return nil, invalid("PARTS_MISMATCH")
			}
			seen[p.Number] = true
			parts = append(parts, p)
		}
		if len(parts) > layout.Count {
			return nil, invalid("PARTS_MISMATCH")
		}
		if page.NextMarker == "" {
			break
		}
		if len(page.NextMarker) > 200 || page.NextMarker == marker || markers[page.NextMarker] || len(markers) >= MaxParts {
			return nil, invalid("PARTS_PAGINATION_INVALID")
		}
		markers[page.NextMarker] = true
		marker = page.NextMarker
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	return parts, nil
}

// CopyFrozen resumes only the same known multipart. No new upload is created
// on an existing key, so a superseded writer cannot overwrite a completed key.
// Every writer reads identical fixed source ranges. Target bytes are then read
// in full and compared with authoritative SHA-256 before being accepted.
func CopyFrozen(ctx context.Context, from, to Store, state CopyState, hooks CopyHooks) (Source, Digest, error) {
	if from == nil || to == nil || hooks.Check == nil || hooks.Part == nil || hooks.Seal == nil || hooks.Frozen == nil || hooks.Hash == nil ||
		state.Source.Size != state.Layout.Size || validateMultipart(state.Target) != nil || len(state.ExpectedSHA256) != 64 {
		return Source{}, Digest{}, invalid("INVALID_TRANSFER")
	}
	if err := hooks.Check(); err != nil {
		return Source{}, Digest{}, err
	}
	target, err := to.Head(ctx, state.Target.Key)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) || e.Code != "OBJECT_NOT_FOUND" {
			return Source{}, Digest{}, err
		}
		if state.Frozen != nil {
			return Source{}, Digest{}, invalid("SOURCE_EVIDENCE_MISMATCH")
		}
		manifest := state.Manifest
		if len(manifest) == 0 {
			parts, err := ListExactParts(ctx, to, state.Target, state.Layout)
			if err != nil {
				return Source{}, Digest{}, err
			}
			confirmed := map[int]Part{}
			for _, part := range parts {
				if err := hooks.Part(part); err != nil {
					return Source{}, Digest{}, err
				}
				confirmed[part.Number] = part
			}
			for n := 1; n <= state.Layout.Count; n++ {
				if _, ok := confirmed[n]; ok {
					continue
				}
				if err := hooks.Check(); err != nil {
					return Source{}, Digest{}, err
				}
				part, err := TransferPart(ctx, from, to, state.Source, state.Target, state.Layout, n)
				if err != nil {
					return Source{}, Digest{}, err
				}
				if err = hooks.Part(part); err != nil {
					return Source{}, Digest{}, err
				}
				confirmed[n] = part
			}
			manifest = make([]Part, 0, len(confirmed))
			for n := 1; n <= state.Layout.Count; n++ {
				manifest = append(manifest, confirmed[n])
			}
			if err = hooks.Seal(manifest); err != nil {
				return Source{}, Digest{}, err
			}
		}
		manifest, err = state.Layout.Reconcile(manifest)
		if err != nil {
			return Source{}, Digest{}, err
		}
		if err = hooks.Check(); err != nil {
			return Source{}, Digest{}, err
		}
		if _, err = to.Complete(ctx, state.Target, manifest); err != nil {
			var e *Error
			if errors.As(err, &e) && (e.Code == "OBJECT_NOT_FOUND" || e.Code == "MULTIPART_NOT_FOUND" || e.Code == "UPLOAD_NOT_FOUND") {
				return Source{}, Digest{}, &Error{Code: "TRANSFER_COMPLETION_UNKNOWN", Retryable: true, Uncertain: true}
			}
			return Source{}, Digest{}, err
		}
		target, err = to.Head(ctx, state.Target.Key)
		if err != nil {
			var e *Error
			if errors.As(err, &e) && e.Code == "OBJECT_NOT_FOUND" {
				return Source{}, Digest{}, &Error{Code: "TRANSFER_COMPLETION_UNKNOWN", Retryable: true, Uncertain: true}
			}
			return Source{}, Digest{}, err
		}
	}
	if target.Key != state.Target.Key || target.Size != state.Layout.Size || !validETag(target.ETag) || (state.Frozen != nil && target != *state.Frozen) {
		return Source{}, Digest{}, invalid("SOURCE_EVIDENCE_MISMATCH")
	}
	if err = hooks.Frozen(target); err != nil {
		return Source{}, Digest{}, err
	}
	digest, err := ResumeHashSource(ctx, to, target, state.Hash, hooks.Hash)
	if err != nil {
		return Source{}, Digest{}, err
	}
	if digest.SHA256 != state.ExpectedSHA256 || digest.Size != state.Layout.Size {
		return Source{}, Digest{}, invalid("CONTENT_DIGEST_MISMATCH")
	}
	return target, digest, nil
}
