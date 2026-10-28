package files

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func cleanupNamespace(ctx context.Context, tx pgx.Tx, org string, f Folder) error {
	if f.ProjectID == nil {
		_, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", org+"/company-files")
		return e
	}
	var id string
	return tx.QueryRow(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", org, *f.ProjectID).Scan(&id)
}
func cleanupPage(s *Service, a core.Actor, key string, f Filter) (*time.Time, *string, int, string, error) {
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/" + key))
	if f.Cursor == "" {
		return nil, nil, f.Limit, binding, nil
	}
	c, e := s.Pager.Decode(f.Cursor, binding)
	return &c.UpdatedAt, &c.ID, f.Limit, binding, e
}
func cleanupNext[T any](s *Service, out *core.Page[T], limit int, binding, id string, at time.Time) {
	out.Items = out.Items[:limit]
	v := s.Pager.Encode(platform.Cursor{ID: id, UpdatedAt: at, Binding: binding})
	out.NextCursor = &v
}
func (s *MultipartCleanupService) Candidates(ctx context.Context, a core.Actor, folderID string, filter Filter) (core.Page[CleanupCandidate], error) {
	out := core.Page[CleanupCandidate]{Items: []CleanupCandidate{}}
	f, e := s.actor(ctx, s.Files.DB, a, folderID, false)
	if e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "upload-cleanup-candidates/"+folderID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, `SELECT u.id FROM upload_sessions u WHERE u.org_id=$1 AND u.target_folder_id=$2 AND u.state<>'committed'
 AND u.classification=$3 AND u.source_channel IN ('web','android')
 AND (u.file_id IS NULL OR EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
  WHERE f.org_id=u.org_id AND f.id=u.file_id AND f.folder_id=u.target_folder_id AND f.classification=u.classification
  AND can_access(f.org_id,$4,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$5,$6)
  AND can_access(f.org_id,$4,'file.retention.manage','file',f.id,d.project_id,f.creator_id,f.classification,$5,$6)))
 AND ($7::timestamptz IS NULL OR (u.created_at,u.id)<($7,$8::uuid)) ORDER BY u.created_at DESC,u.id DESC LIMIT $9`, a.OrgID, f.ID, f.Classification, a.UserID, a.Channel, a.RunID != "", at, id, limit+1)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			break
		}
		ids = append(ids, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		u, e := upload(ctx, s.Files.DB, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		v, e := s.assess(ctx, s.Files.DB, a.OrgID, u, "")
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.UploadID, last.CreatedAt)
	}
	return out, nil
}
func (s *MultipartCleanupService) Plans(ctx context.Context, a core.Actor, folderID string, filter Filter) (core.Page[CleanupPlan], error) {
	out := core.Page[CleanupPlan]{Items: []CleanupPlan{}}
	if _, e := s.actor(ctx, s.Files.DB, a, folderID, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "upload-cleanup-plans/"+folderID, filter)
	if e != nil {
		return out, e
	}
	// Check each selected file before pagination; an old plan cannot disclose an
	// upload after its file has moved or lost the caller's exact-object grant.
	rows, e := s.Files.DB.Query(ctx, `SELECT p.id FROM upload_cleanup_plans p WHERE p.org_id=$1 AND p.folder_id=$2
 AND NOT EXISTS(SELECT 1 FROM upload_cleanup_items i JOIN upload_sessions u ON u.org_id=i.org_id AND u.id=i.upload_id
  WHERE i.org_id=p.org_id AND i.plan_id=p.id AND (u.target_folder_id<>p.folder_id OR
   (u.file_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
    WHERE f.org_id=u.org_id AND f.id=u.file_id AND f.folder_id=p.folder_id AND f.classification=u.classification
    AND can_access(f.org_id,$3,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$4,$5)
    AND can_access(f.org_id,$3,'file.retention.manage','file',f.id,d.project_id,f.creator_id,f.classification,$4,$5)))))
 AND ($6::timestamptz IS NULL OR (p.created_at,p.id)<($6,$7::uuid)) ORDER BY p.created_at DESC,p.id DESC LIMIT $8`, a.OrgID, folderID, a.UserID, a.Channel, a.RunID != "", at, id, limit+1)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		p, e := s.readPlan(ctx, s.Files.DB, a, id, false, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.ID, last.CreatedAt)
	}
	return out, nil
}
func (s *MultipartCleanupService) Create(ctx context.Context, c core.Command, folderID string, in CleanupCreate) (core.Result, error) {
	if len(in.Items) < 1 || len(in.Items) > 50 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("items", "请选择1至50项准确上传记录并说明原因")
	}
	seen := map[string]bool{}
	for _, v := range in.Items {
		if !core.ValidID(v.UploadID) || v.ExpectedVersion < 1 || seen[v.UploadID] {
			return core.Result{}, core.Invalid("items", "上传记录或修订无效、重复")
		}
		seen[v.UploadID] = true
	}
	sort.Slice(in.Items, func(i, j int) bool { return in.Items[i].UploadID < in.Items[j].UploadID })
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.actor(ctx, tx, c.Actor, folderID, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		f, e := s.actor(ctx, tx, c.Actor, folderID, false)
		if e != nil {
			return core.Result{}, e
		}
		if e = cleanupNamespace(ctx, tx, c.Actor.OrgID, f); e != nil {
			return core.Result{}, e
		}
		snapshot := CleanupSnapshot{FolderID: f.ID, FolderVersion: f.Version, Items: []CleanupCandidate{}}
		var observation int64
		for _, v := range in.Items {
			u, e := upload(ctx, tx, c.Actor.OrgID, v.UploadID, true)
			if e != nil {
				return core.Result{}, e
			}
			if e = cleanupUploadAccess(ctx, tx, c.Actor, f, u); e != nil {
				return core.Result{}, e
			}
			if u.Version != v.ExpectedVersion {
				return core.Result{}, core.Conflict()
			}
			candidate, e := s.assess(ctx, tx, c.Actor.OrgID, u, "")
			if e != nil {
				return core.Result{}, e
			}
			if !candidate.Eligible {
				return core.Result{}, core.Fail(409, "CLEANUP_NOT_ELIGIBLE", "候选记录存在阻断，请刷新盘点后核对")
			}
			observation = max(observation, candidate.Policy.ObservationSeconds)
			snapshot.Items = append(snapshot.Items, candidate)
		}
		raw, e := json.Marshal(snapshot)
		if e != nil {
			return core.Result{}, e
		}
		raw, e = core.CanonicalJSON(raw)
		if e != nil {
			return core.Result{}, e
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		id := core.NewID()
		ready := now.Add(time.Duration(observation) * time.Second)
		_, e = tx.Exec(ctx, `INSERT INTO upload_cleanup_plans(id,org_id,folder_id,creator_id,reason,snapshot,snapshot_sha256,item_count,created_at,ready_after,expires_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, c.Actor.OrgID, f.ID, c.Actor.UserID, in.Reason, string(raw), core.Digest(raw), len(snapshot.Items), now, ready, ready.Add(7*24*time.Hour))
		if e != nil {
			return core.Result{}, e
		}
		for _, v := range snapshot.Items {
			if _, e = tx.Exec(ctx, "INSERT INTO upload_cleanup_items(id,org_id,plan_id,upload_id) VALUES($1,$2,$3,$4)", core.NewID(), c.Actor.OrgID, id, v.UploadID); e != nil {
				return core.Result{}, e
			}
		}
		p, e := cleanupPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 201, Data: p, Event: cleanupEvent("upload.cleanup.planned", p, in.Reason)}, e
	})
}
func sameCleanupCandidate(a, b CleanupCandidate) bool { return digest(a) == digest(b) }
func (s *MultipartCleanupService) Approve(ctx context.Context, c core.Command, id string, in CleanupDecision) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil || len(in.SnapshotSHA256) != 64 {
		return core.Result{}, core.Invalid("plan", "请核对清单摘要、修订及批准原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, id, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, id, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.Version != in.ExpectedVersion || p.SnapshotSHA256 != in.SnapshotSHA256 || p.State != "observing" {
			return core.Result{}, core.Conflict()
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		if now.Before(p.ReadyAfter) {
			return core.Result{}, core.Fail(409, "CLEANUP_OBSERVATION_ACTIVE", "清理观察期尚未结束")
		}
		if !now.Before(p.ExpiresAt) {
			return core.Result{}, core.Fail(410, "CLEANUP_PLAN_EXPIRED", "清单批准期限已过，请重新盘点")
		}
		f, e := cleanupFolder(ctx, tx, c.Actor, p.Snapshot.FolderID)
		if e != nil {
			return core.Result{}, e
		}
		if f.Version != p.Snapshot.FolderVersion {
			return core.Result{}, core.Conflict()
		}
		if e = cleanupNamespace(ctx, tx, c.Actor.OrgID, f); e != nil {
			return core.Result{}, e
		}
		for i, item := range p.Items {
			u, e := upload(ctx, tx, c.Actor.OrgID, item.UploadID, true)
			if e != nil {
				return core.Result{}, e
			}
			candidate, e := s.assess(ctx, tx, c.Actor.OrgID, u, "")
			if e != nil {
				return core.Result{}, e
			}
			if !sameCleanupCandidate(candidate, p.Snapshot.Items[i]) || !candidate.Eligible {
				return core.Result{}, core.Fail(409, "CLEANUP_SELECTION_CHANGED", "清单来源、制度或保护状态已变化，请重新盘点")
			}
			// Fence further upload progress before any cleaner can claim its job.
			if _, e = tx.Exec(ctx, "UPDATE upload_sessions SET state='expired',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, u.ID); e != nil {
				return core.Result{}, e
			}
			if _, e = tx.Exec(ctx, "UPDATE upload_cleanup_items SET state='queued',approved_upload_version=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, item.ID, u.Version+1); e != nil {
				return core.Result{}, e
			}
			if e = s.enqueue(ctx, tx, c.Actor.OrgID, p.ID, item.ID, item.Generation); e != nil {
				return core.Result{}, e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE upload_cleanup_plans SET state='approved',version=version+1,approved_by=$3,approved_session_id=$4,
   approved_channel=$5,approved_auth_version=$6,approved_org_version=$7,approved_at=$8,approval_reason=$9 WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, now, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		p, e = cleanupPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 202, Data: p, Event: cleanupEvent("upload.cleanup.approved", p, in.Reason)}, e
	})
}
func (s *MultipartCleanupService) Cancel(ctx context.Context, c core.Command, id string, in CleanupDecision) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请核对修订并填写撤销原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, id, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, id, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.Version != in.ExpectedVersion || p.SnapshotSHA256 != in.SnapshotSHA256 || p.State == "cancelled" {
			return core.Result{}, core.Conflict()
		}
		_, e = tx.Exec(ctx, "UPDATE upload_cleanup_plans SET state='cancelled',version=version+1,cancelled_at=clock_timestamp(),cancellation_reason=$3 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		_, e = tx.Exec(ctx, "UPDATE upload_cleanup_items SET state='blocked',last_error='CLEANUP_PLAN_CANCELLED',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND plan_id=$2 AND state IN ('observing','queued','running','retry_wait')", c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		p, e = cleanupPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Data: p, Event: cleanupEvent("upload.cleanup.cancelled", p, in.Reason)}, e
	})
}
func (s *MultipartCleanupService) Retry(ctx context.Context, c core.Command, itemID string, in UploadExpected) (core.Result, error) {
	if !core.ValidID(itemID) || in.ExpectedVersion < 1 {
		return core.Result{}, core.Missing()
	}
	var planID string
	if e := s.Files.DB.QueryRow(ctx, "SELECT plan_id FROM upload_cleanup_items WHERE org_id=$1 AND id=$2", c.Actor.OrgID, itemID).Scan(&planID); e != nil {
		return core.Result{}, platform.DatabaseError(e)
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, planID, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, planID, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.State != "approved" {
			return core.Result{}, core.Conflict()
		}
		index := slices.IndexFunc(p.Items, func(v CleanupItem) bool { return v.ID == itemID })
		if index < 0 {
			return core.Result{}, core.Missing()
		}
		item := p.Items[index]
		if item.Version != in.ExpectedVersion || item.State != "failed" {
			return core.Result{}, core.Conflict()
		}
		if e = s.executionAllowed(ctx, tx, p, item); e != nil {
			return core.Result{}, e
		}
		_, e = tx.Exec(ctx, "UPDATE upload_cleanup_items SET state='queued',generation=generation+1,version=version+1,last_error=NULL,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, item.ID)
		if e != nil {
			return core.Result{}, e
		}
		if e = s.enqueue(ctx, tx, c.Actor.OrgID, p.ID, item.ID, item.Generation+1); e != nil {
			return core.Result{}, e
		}
		p, e = cleanupPlan(ctx, tx, c.Actor.OrgID, p.ID, false)
		return core.Result{Status: 202, Data: p, Event: cleanupEvent("upload.cleanup.retry.requested", p, "explicit retry of "+item.ID)}, e
	})
}
