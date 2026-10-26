package files

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

type UploadCapabilities struct {
	ControlReady bool    `json:"control_ready"`
	ReasonCode   *string `json:"reason_code"`
	MaxBytes     string  `json:"max_bytes"`
	SessionHours int     `json:"session_hours"`
}

// Readiness here only describes the control plane's explicit configuration.
// It is not proof that a worker, scanner or either cloud is currently healthy.
func (s *UploadService) UploadCapabilities(ctx context.Context, a core.Actor) (UploadCapabilities, error) {
	out := UploadCapabilities{MaxBytes: "120000000000", SessionHours: 48}
	if err := uploadChannel(a); err != nil {
		return out, err
	}
	if err := currentUploadActor(ctx, s.Files.DB, a); err != nil {
		return out, err
	}
	fail := func(code string) (UploadCapabilities, error) { out.ReasonCode = &code; return out, nil }
	if err := s.backend(nil); err != nil {
		return fail("STORAGE_NOT_CONFIGURED")
	}
	if s.Backend.Lister == nil {
		return fail("STORAGE_RECONCILIATION_UNAVAILABLE")
	}
	_, provider, err := currentEpoch(ctx, s.Files.DB, a.OrgID)
	if err != nil {
		if core.PublicError(err).Code == "STORAGE_NOT_CONFIGURED" {
			return fail("STORAGE_NOT_CONFIGURED")
		}
		return out, err
	}
	if provider != s.Backend.Provider {
		return fail("STORAGE_CONFIGURATION_CHANGED")
	}
	var quota bool
	if err = s.Files.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_quotas WHERE org_id=$1 AND logical_limit_bytes>0)", a.OrgID).Scan(&quota); err != nil {
		return out, err
	}
	if !quota {
		return fail("STORAGE_QUOTA_NOT_CONFIGURED")
	}
	out.ControlReady = true
	return out, nil
}

// Queue metadata is restricted to the owner and to targets they can still
// access. The same predicates run before LIMIT; cursors cannot skip ACL checks.
func (s *UploadService) List(ctx context.Context, a core.Actor, scope, cursor string, limit int) (core.Page[Upload], error) {
	out := core.Page[Upload]{Items: []Upload{}}
	if err := uploadChannel(a); err != nil {
		return out, err
	}
	if err := currentUploadActor(ctx, s.Files.DB, a); err != nil {
		return out, err
	}
	if scope == "" {
		scope = "active"
	}
	if scope != "active" && scope != "all" {
		return out, core.Invalid("state", "请选择进行中或全部上传")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/uploads/" + scope))
	var at *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := s.Files.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		at, beforeID = &c.UpdatedAt, &c.ID
	}
	rows, err := s.Files.DB.Query(ctx, `SELECT u.id,u.updated_at FROM upload_sessions u
		JOIN folders f ON f.org_id=u.org_id AND f.id=u.target_folder_id
		LEFT JOIN files fl ON fl.org_id=u.org_id AND fl.id=u.file_id
		LEFT JOIN file_versions cv ON cv.org_id=u.org_id AND cv.id=u.committed_version_id
		LEFT JOIN files cf ON cf.org_id=cv.org_id AND cf.id=cv.file_id
		LEFT JOIN folders cd ON cd.org_id=cf.org_id AND cd.id=cf.folder_id
		WHERE u.org_id=$1 AND u.owner_id=$2
		AND u.source_channel IS DISTINCT FROM 'agent'
		AND u.classification=f.classification
		AND ($3='all' OR (u.state IN ('created','uploading','finalizing','verifying','blocked','ready','failed') AND u.expires_at>now()))
		AND can_access($1,$2,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,$6,false)
		AND can_access($1,$2,CASE WHEN u.file_id IS NULL THEN 'file.upload' ELSE 'file.update' END,
			CASE WHEN u.file_id IS NULL THEN 'folder' ELSE 'file' END,coalesce(fl.id,f.id),
			f.project_id,CASE WHEN u.file_id IS NULL THEN f.creator_id ELSE fl.creator_id END,coalesce(fl.classification,f.classification),$6,false)
		AND (u.file_id IS NULL OR (fl.folder_id=f.id AND fl.state='active'))
		AND (u.committed_version_id IS NULL OR (
			can_access($1,$2,'file.read','file',cf.id,cd.project_id,cf.creator_id,cf.classification,$6,false)
			AND (WITH RECURSIVE committed_chain AS (
				SELECT id,parent_id,state,ARRAY[id] path FROM folders WHERE org_id=$1 AND id=cf.folder_id
				UNION ALL SELECT p.id,p.parent_id,p.state,c.path||p.id FROM folders p JOIN committed_chain c ON p.id=c.parent_id
				WHERE p.org_id=$1 AND NOT p.id=ANY(c.path)
			) SELECT coalesce(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM committed_chain)
		))
		AND (WITH RECURSIVE chain AS (
			SELECT id,parent_id,state,ARRAY[id] path FROM folders WHERE org_id=$1 AND id=f.id
			UNION ALL SELECT p.id,p.parent_id,p.state,c.path||p.id FROM folders p JOIN chain c ON p.id=c.parent_id
			WHERE p.org_id=$1 AND NOT p.id=ANY(c.path)
		) SELECT coalesce(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM chain)
		AND ($4::timestamptz IS NULL OR (u.updated_at,u.id)<($4,$5::uuid))
		ORDER BY u.updated_at DESC,u.id DESC LIMIT $7`, a.OrgID, a.UserID, scope, at, beforeID, a.Channel, limit+1)
	if err != nil {
		return out, err
	}
	type item struct {
		id string
		at time.Time
	}
	candidates := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.at); err != nil {
			rows.Close()
			return out, err
		}
		candidates = append(candidates, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
		last := candidates[len(candidates)-1]
		next := s.Files.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.at, Binding: binding})
		out.NextCursor = &next
	}
	for _, v := range candidates {
		u, err := s.Get(ctx, a, v.id)
		if err != nil {
			if core.PublicError(err).Status == 404 {
				continue
			}
			return out, err
		}
		out.Items = append(out.Items, u)
	}
	return out, nil
}
