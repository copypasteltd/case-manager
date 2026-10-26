package files

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const versionColumns = `id,file_id,seq,base_version_id,original_name,size_bytes::text,sha256,media_type,source,author_id,executor_id,created_at,
        CASE WHEN scan_state='quarantined' THEN 'blocked' ELSE scan_state END,scan_waiver_ref,availability,restored_from_version_id`

func scanVersion(row pgx.Row) (Version, error) {
	var v Version
	err := row.Scan(&v.ID, &v.FileID, &v.Seq, &v.BaseVersionID, &v.OriginalName, &v.SizeBytes, &v.SHA256, &v.MediaType, &v.Source, &v.AuthorID, &v.ExecutorID, &v.CreatedAt, &v.ScanState, &v.ScanWaiverRef, &v.Availability, &v.RestoredFromVersionID)
	return v, platform.DatabaseError(err)
}

// Version resolves the requested immutable version, never the file's current
// version or a page-relative position. Metadata does not grant object access.
func (s *Service) Version(ctx context.Context, a core.Actor, id, versionID string) (Version, error) {
	if _, err := authorizeFile(ctx, s.DB, a, id, "file.read"); err != nil {
		return Version{}, err
	}
	if !core.ValidID(versionID) {
		return Version{}, core.Missing()
	}
	v, err := scanVersion(s.DB.QueryRow(ctx, "SELECT "+versionColumns+" FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3", a.OrgID, id, versionID))
	if err != nil {
		return Version{}, err
	}
	if err = s.replicas(ctx, a, &v); err != nil {
		return Version{}, err
	}
	return v, nil
}

func (s *Service) Versions(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Version], error) {
	out := core.Page[Version]{Items: []Version{}}
	if _, err := authorizeFile(ctx, s.DB, a, id, "file.read"); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/file-versions/" + id))
	var beforeSeq *int64
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		var seq int64
		err = s.DB.QueryRow(ctx, "SELECT seq FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3 AND created_at=$4", a.OrgID, id, c.ID, c.UpdatedAt).Scan(&seq)
		if err != nil {
			return out, platform.DatabaseError(err)
		}
		beforeSeq = &seq
	}
	rows, err := s.DB.Query(ctx, `SELECT `+versionColumns+` FROM file_versions WHERE org_id=$1 AND file_id=$2 AND ($3::bigint IS NULL OR seq<$3)
        ORDER BY seq DESC LIMIT $4`, a.OrgID, id, beforeSeq, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: binding})
		out.NextCursor = &c
	}
	for i := range out.Items {
		if err = s.replicas(ctx, a, &out.Items[i]); err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) replicas(ctx context.Context, a core.Actor, v *Version) error {
	v.Replicas = []Replica{}
	rows, err := s.DB.Query(ctx, `SELECT id,version,provider,state,actual_sha256,verified_at,updated_at,expected_sha256
        FROM file_replicas WHERE org_id=$1 AND version_id=$2 ORDER BY provider,id`, a.OrgID, v.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	providers := map[string]bool{}
	allVerified := true
	rank := map[string]int{"verified": 0, "queued": 1, "copying": 2, "verifying": 3, "retry_wait": 4, "missing": 5, "failed": 6}
	overall := "verified"
	for rows.Next() {
		var r Replica
		var expected string
		if err = rows.Scan(&r.ID, &r.Version, &r.Provider, &r.State, &r.ActualSHA256, &r.VerifiedAt, &r.UpdatedAt, &expected); err != nil {
			return err
		}
		// Never report verified from a state label alone.
		if r.State == "verified" && (r.ActualSHA256 == nil || *r.ActualSHA256 != v.SHA256 || expected != v.SHA256 || r.VerifiedAt == nil) {
			r.State = "failed"
			code := "VERIFICATION_EVIDENCE_MISSING"
			r.ErrorCode = &code
		} else {
			code := ""
			switch r.State {
			case "failed":
				code = "REPLICA_FAILED"
			case "retry_wait":
				code = "RETRY_SCHEDULED"
			case "missing":
				code = "OBJECT_MISSING"
			}
			if code != "" {
				r.ErrorCode = &code
			}
		}
		providers[r.Provider] = true
		allVerified = allVerified && r.State == "verified"
		if rank[r.State] > rank[overall] {
			overall = r.State
		}
		v.Replicas = append(v.Replicas, r)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(v.Replicas) == 0 {
		return nil
	}
	if (!providers["oss"] || !providers["r2"]) && rank[overall] < rank["missing"] {
		overall = "missing"
	}
	if providers["oss"] && providers["r2"] && allVerified {
		overall = "verified"
	}
	v.ReplicaState = &overall
	return nil
}
