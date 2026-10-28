package fileimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
)

type Runner struct {
	Client           *Client
	WaitLimit        time.Duration
	PollInterval     time.Duration
	ReviewedMappings bool
	SourcesReadOnly  bool
	OnlySourceKey    string
}

type ReportSummary struct {
	Type                    string `json:"type"`
	BatchID                 string `json:"batch_id"`
	Expected                int    `json:"expected"`
	Observed                int    `json:"observed"`
	ObservedBytes           string `json:"observed_bytes"`
	FullyVerified           int    `json:"fully_verified"`
	Pending                 int    `json:"pending"`
	Invalid                 int    `json:"invalid"`
	AllExpectedRowsObserved bool   `json:"all_expected_rows_observed"`
	ObservedContentVerified bool   `json:"observed_content_verified"`
	CutoverApproved         bool   `json:"cutover_approved"`
	Scope                   string `json:"scope"`
}

func (r *Runner) pause(ctx context.Context) error {
	d := r.PollInterval
	if d <= 0 {
		d = time.Second
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateBatch(b files.ImportBatch, h importspec.BatchInput, owner string) error {
	if !importspec.CanonicalID(b.ID) || b.BatchInput != h || b.OwnerID != owner || b.AdmittedCount < 0 || b.AdmittedCount > b.EntryCount || !importspec.Hash(b.AdmittedChain) {
		return fmt.Errorf("BATCH_RECEIPT_INCOMPLETE_OR_DIFFERENT")
	}
	return nil
}

func validateEntry(e files.ImportEntry, batch string, want importspec.Entry) error {
	if !importspec.CanonicalID(e.ID) || e.BatchID != batch || e.CreatedAt.IsZero() || importspec.EntryDigest(e.Entry) != importspec.EntryDigest(want) {
		return fmt.Errorf("ENTRY_RECEIPT_INCOMPLETE_OR_DIFFERENT")
	}
	return nil
}

func validateUpload(u files.Upload, e importspec.Entry, namespace string) error {
	n, _ := importspec.Bytes(e.SizeBytes)
	if !importspec.CanonicalID(u.ID) || u.Version < 1 || u.SizeBytes != e.SizeBytes || u.FolderID != e.FolderID || u.Name != e.Name || u.MediaType != e.MediaType || u.SourceFingerprint != core.Digest([]byte(namespace+"/"+e.SourceKey+"/"+e.SourceSHA256)) || u.PartBytes < 5<<20 || u.PartBytes > 512<<20 || u.PartCount != int((n+u.PartBytes-1)/u.PartBytes) || u.PartCount > 10000 || u.ExpiresAt.IsZero() {
		return fmt.Errorf("UPLOAD_RECEIPT_INCOMPLETE_OR_DIFFERENT")
	}
	if u.State != "committed" && (!sameOptional(u.FileID, e.FileID) || !sameOptional(u.BaseVersionID, e.BaseVersionID)) {
		return fmt.Errorf("UPLOAD_BASE_RECEIPT_DIFFERENT")
	}
	switch u.State {
	case "created", "uploading", "finalizing", "verifying", "ready", "committed", "failed", "blocked", "expired", "aborted":
	default:
		return fmt.Errorf("UPLOAD_RECEIPT_STATE_INVALID")
	}
	seen := map[int]bool{}
	for _, p := range u.ConfirmedParts {
		if p < 1 || p > u.PartCount || seen[p] {
			return fmt.Errorf("UPLOAD_PART_RECEIPT_INVALID")
		}
		seen[p] = true
	}
	return nil
}

func sameOptional(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func holdSource(ctx context.Context, root *os.Root, e importspec.Entry) (*os.File, os.FileInfo, error) {
	before, err := root.Lstat(e.SourcePath)
	if err != nil || !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("SOURCE_UNREADABLE_OR_NOT_REGULAR")
	}
	f, err := root.Open(e.SourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("SOURCE_UNREADABLE")
	}
	fail := func(code string) (*os.File, os.FileInfo, error) { f.Close(); return nil, nil, fmt.Errorf("%s", code) }
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) || strconv.FormatInt(info.Size(), 10) != e.SizeBytes || info.ModTime().UTC().Format(time.RFC3339Nano) != e.SourceModifiedAt {
		return fail("SOURCE_CHANGED")
	}
	h := sha256.New()
	n, err := copyContext(ctx, h, f)
	if err != nil {
		return fail("SOURCE_HASH_READ_FAILED")
	}
	if n != info.Size() || hex.EncodeToString(h.Sum(nil)) != e.SourceSHA256 {
		return fail("SOURCE_DIGEST_CHANGED")
	}
	if err = stableSource(root, f, info, e.SourcePath); err != nil {
		f.Close()
		return nil, nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return fail("SOURCE_SEEK_FAILED")
	}
	return f, info, nil
}

func stableSource(root *os.Root, f *os.File, original os.FileInfo, path string) error {
	held, err := f.Stat()
	if err != nil {
		return fmt.Errorf("SOURCE_CHANGED")
	}
	current, err := root.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(original, held) || !os.SameFile(original, current) || held.Size() != original.Size() || current.Size() != original.Size() || !held.ModTime().Equal(original.ModTime()) || !current.ModTime().Equal(original.ModTime()) {
		return fmt.Errorf("SOURCE_CHANGED")
	}
	return nil
}

func (r *Runner) getEntry(ctx context.Context, batch, namespace string, want importspec.Entry, id string) (files.ImportEntry, error) {
	var e files.ImportEntry
	err := r.Client.Request(ctx, "GET", "/file-import-entries/"+id, "", nil, &e)
	if err == nil {
		err = validateEntry(e, batch, want)
	}
	if err == nil && e.Upload != nil {
		err = validateUpload(*e.Upload, want, namespace)
	}
	return e, err
}

func (r *Runner) transfer(ctx context.Context, root *os.Root, b files.ImportBatch, e files.ImportEntry) error {
	f, info, err := holdSource(ctx, root, e.Entry)
	if err != nil {
		return err
	}
	defer f.Close()
	var reserved files.ImportEntry
	if err = r.Client.Request(ctx, "POST", "/file-import-entries/"+e.ID+"/uploads", "import-start/"+e.ID, map[string]any{}, &reserved); err != nil {
		return err
	}
	if err = validateEntry(reserved, b.ID, e.Entry); err != nil {
		return err
	}
	limit := r.WaitLimit
	if limit <= 0 {
		limit = 24 * time.Hour
	}
	work, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	reconciled := false
	for {
		current, err := r.getEntry(work, b.ID, b.SourceNamespace, e.Entry, e.ID)
		if err != nil {
			return err
		}
		if current.Upload == nil {
			return fmt.Errorf("UPLOAD_MAPPING_MISSING")
		}
		u := *current.Upload
		switch u.State {
		case "committed":
			if u.ResultVersionID == nil || u.ResultFileID == nil || current.FileVersion == nil || current.FileVersion.ID != *u.ResultVersionID || current.FileVersion.FileID != *u.ResultFileID || current.FileVersion.SHA256 != e.SourceSHA256 || current.FileVersion.SizeBytes != e.SizeBytes {
				return fmt.Errorf("COMMITTED_RECEIPT_INCOMPLETE")
			}
			return stableSource(root, f, info, e.SourcePath)
		case "created", "finalizing", "verifying":
			if err = r.pause(work); err != nil {
				return fmt.Errorf("UPLOAD_WORKER_WAIT_INCOMPLETE_RERUN_TO_RECONCILE")
			}
		case "uploading":
			if !reconciled {
				var out files.Upload
				if err = r.Client.Request(work, "POST", "/uploads/"+u.ID+"/resume", core.NewID(), files.ResumeUpload{ExpectedVersion: u.Version, SourceFingerprint: u.SourceFingerprint}, &out); err != nil {
					return err
				}
				if err = validateUpload(out, e.Entry, b.SourceNamespace); err != nil {
					return err
				}
				u = out
				reconciled = true
			}
			present := map[int]bool{}
			for _, n := range u.ConfirmedParts {
				present[n] = true
			}
			size, _ := importspec.Bytes(e.SizeBytes)
			for n := 1; n <= u.PartCount; n++ {
				if present[n] {
					continue
				}
				if err = stableSource(root, f, info, e.SourcePath); err != nil {
					return err
				}
				var signed files.SignedParts
				if err = r.Client.Request(work, "POST", "/uploads/"+u.ID+"/sign-parts", core.NewID(), files.SignParts{ExpectedVersion: u.Version, PartNumbers: []int{n}}, &signed); err != nil {
					return err
				}
				if len(signed.Items) != 1 || signed.Items[0].PartNumber != n {
					return fmt.Errorf("SIGNED_PART_RECEIPT_INVALID")
				}
				offset := int64(n-1) * u.PartBytes
				length := min(u.PartBytes, size-offset)
				etag, err := r.Client.Put(work, signed.Items[0], io.NewSectionReader(f, offset, length), length)
				if err != nil {
					return err
				}
				if err = stableSource(root, f, info, e.SourcePath); err != nil {
					return err
				}
				body := files.AckParts{ExpectedVersion: u.Version, Parts: []files.AckPart{{PartNumber: n, ETag: etag, SizeBytes: strconv.FormatInt(length, 10)}}}
				raw, _ := json.Marshal(body)
				if err = r.Client.Request(work, "POST", "/uploads/"+u.ID+"/ack-parts", "import-ack/"+core.Digest(raw), body, &u); err != nil {
					return err
				}
				if err = validateUpload(u, e.Entry, b.SourceNamespace); err != nil {
					return err
				}
			}
			if err = r.Client.Request(work, "POST", "/uploads/"+u.ID+"/finalize", "import-finalize/"+u.ID+"/"+strconv.FormatInt(u.Version, 10), files.UploadExpected{ExpectedVersion: u.Version}, &u); err != nil {
				return err
			}
		case "ready":
			if u.SHA256 == nil || *u.SHA256 != e.SourceSHA256 {
				return fmt.Errorf("VERIFIED_UPLOAD_DIGEST_DIFFERENT")
			}
			if err = stableSource(root, f, info, e.SourcePath); err != nil {
				return err
			}
			var receipt files.CommitResult
			in := files.UploadCommit{ExpectedVersion: u.Version, SHA256: e.SourceSHA256, BaseVersionID: e.BaseVersionID}
			if err = r.Client.Request(work, "POST", "/uploads/"+u.ID+"/commit", "import-commit/"+u.ID+"/"+strconv.FormatInt(u.Version, 10), in, &receipt); err != nil {
				return err
			}
			if !importspec.CanonicalID(receipt.File.ID) || receipt.Version.FileID != receipt.File.ID || !importspec.CanonicalID(receipt.Version.ID) || receipt.Version.SHA256 != e.SourceSHA256 || receipt.Version.SizeBytes != e.SizeBytes {
				return fmt.Errorf("COMMIT_RECEIPT_INCOMPLETE_REREAD_BEFORE_RETRY")
			}
		default:
			return fmt.Errorf("UPLOAD_REQUIRES_OPERATOR_REVIEW_%s", strings.ToUpper(u.State))
		}
	}
}

type entryIterator struct {
	client        *Client
	batch         string
	page          core.Page[files.ImportEntry]
	index         int
	started, done bool
}

func (i *entryIterator) next(ctx context.Context) (files.ImportEntry, error) {
	if i.index >= len(i.page.Items) {
		if i.done {
			return files.ImportEntry{}, io.EOF
		}
		path := "/file-imports/" + i.batch + "/entries?limit=100"
		if i.started {
			if i.page.NextCursor == nil {
				return files.ImportEntry{}, io.EOF
			}
			path += "&cursor=" + url.QueryEscape(*i.page.NextCursor)
		}
		var page core.Page[files.ImportEntry]
		if err := i.client.Request(ctx, "GET", path, "", nil, &page); err != nil {
			return files.ImportEntry{}, err
		}
		if len(page.Items) == 0 {
			if page.NextCursor != nil {
				return files.ImportEntry{}, fmt.Errorf("EMPTY_PAGE_WITH_CURSOR")
			}
			i.done = true
			return files.ImportEntry{}, io.EOF
		}
		i.page = page
		i.index = 0
		i.started = true
	}
	e := i.page.Items[i.index]
	i.index++
	return e, nil
}

func (r *Runner) Run(ctx context.Context, rootPath, planPath, approvedSHA string, to io.Writer) (string, error) {
	plan, err := os.Open(planPath)
	if err != nil {
		return "", fmt.Errorf("PLAN_UNREADABLE")
	}
	defer plan.Close()
	header, err := ReadPlan(plan, nil)
	if err != nil {
		return "", err
	}
	if approvedSHA != header.PlanSHA256 || !r.ReviewedMappings || !r.SourcesReadOnly {
		return "", fmt.Errorf("EXPLICIT_EXACT_PLAN_APPROVAL_REQUIRED")
	}
	if r.OnlySourceKey != "" && !importspec.Hash(r.OnlySourceKey) {
		return "", fmt.Errorf("INVALID_SELECTED_SOURCE_KEY")
	}
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err = r.Client.Request(ctx, "GET", "/me", "", nil, &me); err != nil {
		return "", err
	}
	if !importspec.CanonicalID(me.User.ID) {
		return "", fmt.Errorf("IDENTITY_RECEIPT_INCOMPLETE")
	}
	var batch files.ImportBatch
	if err = r.Client.Request(ctx, "POST", "/file-imports", "import-batch/"+header.SourceNamespace+"/"+header.PlanSHA256, header.BatchInput, &batch); err != nil {
		return "", err
	}
	if err = validateBatch(batch, header.BatchInput, me.User.ID); err != nil {
		return "", err
	}
	if err = r.Client.Request(ctx, "GET", "/file-imports/"+batch.ID, "", nil, &batch); err != nil {
		return batch.ID, err
	}
	if err = validateBatch(batch, header.BatchInput, me.User.ID); err != nil {
		return batch.ID, err
	}
	enc := json.NewEncoder(to)
	if err = enc.Encode(map[string]any{"type": "run", "protocol": 1, "batch_id": batch.ID, "plan_sha256": header.PlanSHA256, "started_at": time.Now().UTC(), "only_source_key": r.OnlySourceKey}); err != nil {
		return batch.ID, err
	}
	plan.Seek(0, io.SeekStart)
	_, err = ReadPlan(plan, func(in importspec.Entry) error {
		if in.Ordinal <= batch.AdmittedCount {
			return nil
		}
		var e files.ImportEntry
		if err := r.Client.Request(ctx, "POST", "/file-imports/"+batch.ID+"/entries", "import-entry/"+batch.ID+"/"+importspec.EntryDigest(in), in, &e); err != nil {
			return err
		}
		return validateEntry(e, batch.ID, in)
	})
	if err != nil {
		return batch.ID, err
	}
	if err = r.Client.Request(ctx, "GET", "/file-imports/"+batch.ID, "", nil, &batch); err != nil {
		return batch.ID, err
	}
	if err = validateBatch(batch, header.BatchInput, me.User.ID); err != nil {
		return batch.ID, err
	}
	if !batch.Approved {
		approval := importspec.ApprovalInput{PlanSHA256: header.PlanSHA256, ReviewedMappings: r.ReviewedMappings, SourcesReadOnly: r.SourcesReadOnly, Reason: header.Reason}
		if err = r.Client.Request(ctx, "POST", "/file-imports/"+batch.ID+"/approve", core.NewID(), approval, &batch); err != nil {
			return batch.ID, err
		}
		if !batch.Approved || batch.ApprovalExpiresAt == nil {
			return batch.ID, fmt.Errorf("APPROVAL_RECEIPT_INCOMPLETE")
		}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return batch.ID, fmt.Errorf("SOURCE_ROOT_UNREADABLE")
	}
	defer root.Close()
	iterator := entryIterator{client: r.Client, batch: batch.ID}
	failed := 0
	selected := 0
	plan.Seek(0, io.SeekStart)
	_, err = ReadPlan(plan, func(in importspec.Entry) error {
		e, err := iterator.next(ctx)
		if err != nil {
			return err
		}
		if err = validateEntry(e, batch.ID, in); err != nil {
			return err
		}
		if r.OnlySourceKey != "" && r.OnlySourceKey != in.SourceKey {
			return nil
		}
		selected++
		err = r.transfer(ctx, root, batch, e)
		code := ""
		if err != nil {
			failed++
			code = err.Error()
		}
		if writeErr := enc.Encode(map[string]any{"type": "entry_result", "ordinal": in.Ordinal, "source_key": in.SourceKey, "entry_id": e.ID, "succeeded": err == nil, "error_code": code}); writeErr != nil {
			return writeErr
		}
		var rejected *APIError
		if errors.As(err, &rejected) && (rejected.Status == 401 || rejected.Status == 403 || rejected.Status == 404 || rejected.Status == 412) {
			return err
		}
		if err != nil && (strings.Contains(code, "UNCERTAIN") || strings.Contains(code, "INCOMPLETE") || strings.Contains(code, "RECONCILE")) {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return batch.ID, err
	}
	if selected == 0 {
		return batch.ID, fmt.Errorf("SELECTED_SOURCE_NOT_IN_PLAN")
	}
	if failed > 0 {
		return batch.ID, fmt.Errorf("IMPORT_HAS_%d_ENTRY_FAILURES", failed)
	}
	return batch.ID, nil
}

func (r *Runner) Report(ctx context.Context, batchID string, to io.Writer) (ReportSummary, error) {
	summary := ReportSummary{Type: "summary", BatchID: batchID, ObservedBytes: "0", Scope: "Live per-row observations with current authority; not an atomic restore point, source ACL equivalence or cutover approval."}
	if !importspec.CanonicalID(batchID) {
		return summary, fmt.Errorf("CANONICAL_BATCH_ID_REQUIRED")
	}
	var b files.ImportBatch
	if err := r.Client.Request(ctx, "GET", "/file-imports/"+batchID, "", nil, &b); err != nil {
		return summary, err
	}
	if b.ID != batchID || !importspec.CanonicalID(b.OwnerID) || b.AdmittedCount < 0 || b.AdmittedCount > b.EntryCount {
		return summary, fmt.Errorf("REPORT_BATCH_RECEIPT_INVALID")
	}
	summary.Expected = b.EntryCount
	if err := importspec.ValidateBatch(b.BatchInput); err != nil {
		return summary, err
	}
	enc := json.NewEncoder(to)
	if err := enc.Encode(map[string]any{"type": "verification", "protocol": 1, "batch": b, "observed_at": time.Now().UTC()}); err != nil {
		return summary, err
	}
	iterator := entryIterator{client: r.Client, batch: batchID}
	chain := importspec.ZeroChain
	var total int64
	for {
		e, err := iterator.next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return summary, err
		}
		summary.Observed++
		if err = importspec.ValidateEntry(b.SourceNamespace, e.Entry); err != nil {
			return summary, err
		}
		if e.Ordinal != summary.Observed || !importspec.CanonicalID(e.ID) || e.BatchID != b.ID || summary.Observed > b.EntryCount {
			return summary, fmt.Errorf("REPORT_ENTRY_IDENTITY_OR_ORDER_INVALID")
		}
		n, _ := importspec.Bytes(e.SizeBytes)
		total += n
		chain = importspec.Chain(chain, importspec.EntryDigest(e.Entry))
		full := e.VerificationState == "replicas_verified" && e.MappingCurrent && e.MappedOwnerCanRead && e.FileVersion != nil && e.Upload != nil
		if full {
			u, v := e.Upload, e.FileVersion
			if err := validateUpload(*u, e.Entry, b.SourceNamespace); err != nil {
				return summary, err
			}
			full = importspec.CanonicalID(v.ID) && importspec.CanonicalID(v.FileID) && u.State == "committed" && u.ResultFileID != nil && u.ResultVersionID != nil && v.ID == *u.ResultVersionID && v.FileID == *u.ResultFileID && v.SHA256 == e.SourceSHA256 && v.SizeBytes == e.SizeBytes
			seen := map[string]bool{}
			for _, replica := range v.Replicas {
				if replica.State == "verified" && replica.VerifiedAt != nil && replica.ActualSHA256 != nil && *replica.ActualSHA256 == e.SourceSHA256 {
					seen[replica.Provider] = true
				}
			}
			full = full && seen["oss"] && seen["r2"]
		}
		if full {
			summary.FullyVerified++
		} else if e.VerificationState == "content_mismatch" || e.VerificationState == "mapping_changed" || e.VerificationState == "failed" || e.VerificationState == "aborted" || e.VerificationState == "expired" || e.VerificationState == "blocked" {
			summary.Invalid++
		} else {
			summary.Pending++
		}
		if e.VerificationState == "replicas_verified" && !full {
			return summary, fmt.Errorf("REPLICA_VERIFICATION_RECEIPT_INCOMPLETE")
		}
		if err = enc.Encode(map[string]any{"type": "entry", "observed_at": time.Now().UTC(), "entry": e}); err != nil {
			return summary, err
		}
	}
	summary.ObservedBytes = strconv.FormatInt(total, 10)
	summary.AllExpectedRowsObserved = summary.Observed == b.EntryCount && summary.ObservedBytes == b.TotalBytes && chain == b.PlanSHA256
	summary.ObservedContentVerified = summary.AllExpectedRowsObserved && summary.FullyVerified == summary.Expected && summary.Invalid == 0 && summary.Pending == 0
	if err := enc.Encode(summary); err != nil {
		return summary, err
	}
	if !summary.AllExpectedRowsObserved {
		return summary, fmt.Errorf("REPORT_MANIFEST_INCOMPLETE")
	}
	return summary, nil
}
