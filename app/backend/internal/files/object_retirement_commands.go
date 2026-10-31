package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func (s *ObjectRetirementService) Candidates(ctx context.Context, a core.Actor, fileID string, filter Filter) (core.Page[RetirementCandidate], error) {
	out := core.Page[RetirementCandidate]{Items: []RetirementCandidate{}}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	f, e := s.actor(ctx, tx, a, fileID, false)
	if e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "object-retirement-candidates/"+fileID, filter)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT kind,id,created_at FROM (
  SELECT 'history' kind,id,created_at FROM file_versions WHERE org_id=$1 AND file_id=$2
  UNION ALL SELECT 'committed_draft',u.id,u.created_at FROM upload_sessions u JOIN file_versions v ON v.org_id=u.org_id AND v.id=u.committed_version_id
    WHERE u.org_id=$1 AND v.file_id=$2 AND u.state='committed'
 ) q WHERE ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, fileID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	type target struct {
		kind, id string
		at       time.Time
	}
	targets := []target{}
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.kind, &t.id, &t.at); e != nil {
			break
		}
		targets = append(targets, t)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, t := range targets {
		v, _, e := s.assess(ctx, tx, a.OrgID, f, t.kind, t.id, "")
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > limit {
		last := targets[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.id, last.at)
	}
	return out, nil
}
func (s *ObjectRetirementService) Plans(ctx context.Context, a core.Actor, fileID string, filter Filter) (core.Page[RetirementPlan], error) {
	out := core.Page[RetirementPlan]{Items: []RetirementPlan{}}
	if _, e := s.actor(ctx, s.Files.DB, a, fileID, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "object-retirement-plans/"+fileID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, `SELECT id FROM object_retirement_plans WHERE org_id=$1 AND file_id=$2
  AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, fileID, at, id, limit+1)
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
		v, e := s.readPlan(ctx, s.Files.DB, a, id, false, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if len(out.Items) > limit {
		v := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, v.ID, v.CreatedAt)
	}
	return out, nil
}
func (s *ObjectRetirementService) Create(ctx context.Context, c core.Command, fileID string, in RetirementCreate) (core.Result, error) {
	if !core.ValidID(fileID) || !core.ValidID(in.TargetID) || in.ExpectedFileVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("target_id", "请明确选择固定来源、文件修订并说明原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.actor(ctx, tx, c.Actor, fileID, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		f, e := file(ctx, tx, c.Actor.OrgID, fileID, true)
		if e != nil {
			return core.Result{}, e
		}
		if f.Version != in.ExpectedFileVersion {
			return core.Result{}, core.Conflict()
		}
		snapshot, objects, e := s.assess(ctx, tx, c.Actor.OrgID, f, in.Kind, in.TargetID, "")
		if e != nil {
			return core.Result{}, e
		}
		if !snapshot.Eligible {
			return core.Result{}, core.Fail(409, "RETIREMENT_PROTECTED", "所选原件仍受保护或来源证据不完整，请刷新盘点")
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		wait := snapshot.Policy.ObservationSeconds
		if snapshot.TrashPolicy != nil {
			wait = max(wait, snapshot.TrashPolicy.ObservationSeconds)
		}
		ready := now.Add(time.Duration(wait) * time.Second)
		raw, _ := json.Marshal(snapshot)
		raw, _ = core.CanonicalJSON(raw)
		objectRaw, _ := json.Marshal(objects)
		id := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO object_retirement_plans(id,org_id,file_id,kind,target_id,snapshot,snapshot_sha256,objects,reason,created_at,ready_after,expires_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, c.Actor.OrgID, fileID, in.Kind, in.TargetID, raw, core.Digest(raw), objectRaw, in.Reason, now, ready, ready.Add(7*24*time.Hour))
		if e != nil {
			return core.Result{}, e
		}
		for i, obj := range objects {
			if _, e = tx.Exec(ctx, "INSERT INTO object_retirement_items(id,org_id,plan_id,ordinal,provider) VALUES($1,$2,$3,$4,$5)", core.NewID(), c.Actor.OrgID, id, i, obj.Provider); e != nil {
				return core.Result{}, e
			}
		}
		p, e := retirementPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 201, Data: p, Event: retirementEvent("file.original.retirement.created", p, in.Reason)}, e
	})
}
func (s *ObjectRetirementService) Approve(ctx context.Context, c core.Command, id string, in CleanupDecision) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请核对清单修订和摘要并填写批准依据")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, id, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, id, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.State != "observing" || p.Version != in.ExpectedVersion || p.SnapshotSHA256 != in.SnapshotSHA256 {
			return core.Result{}, core.Conflict()
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		if now.Before(p.ReadyAfter) {
			return core.Result{}, core.Fail(409, "CLEANUP_OBSERVATION_ACTIVE", "观察期尚未结束")
		}
		if !now.Before(p.ExpiresAt) {
			return core.Result{}, core.Fail(410, "CLEANUP_PLAN_EXPIRED", "批准期限已过，请重新盘点")
		}
		f, e := file(ctx, tx, c.Actor.OrgID, p.Snapshot.FileID, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.Snapshot.Kind == "history" {
			var locked string
			if e = tx.QueryRow(ctx, "SELECT id FROM file_versions WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, p.Snapshot.TargetID).Scan(&locked); e != nil {
				return core.Result{}, e
			}
		}
		snapshot, objects, e := s.assess(ctx, tx, c.Actor.OrgID, f, p.Snapshot.Kind, p.Snapshot.TargetID, "")
		if e != nil {
			return core.Result{}, e
		}
		if !snapshot.Eligible || digest(snapshot) != digest(p.Snapshot) || digest(objects) != digest(p.Objects) {
			return core.Result{}, core.Fail(409, "CLEANUP_SELECTION_CHANGED", "来源、制度、文件修订或保护状态已变化")
		}
		for _, obj := range objects {
			if _, e = tx.Exec(ctx, `INSERT INTO object_retirement_barriers(id,org_id,plan_id,kind,target_id,provider,bucket,object_key)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, core.NewID(), c.Actor.OrgID, id, p.Snapshot.Kind, p.Snapshot.TargetID, obj.Provider, obj.Bucket, obj.Source.Key); e != nil {
				return core.Result{}, e
			}
		}
		for _, item := range p.Items {
			if _, e = tx.Exec(ctx, "UPDATE object_retirement_items SET state='queued',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, item.ID); e != nil {
				return core.Result{}, e
			}
			if e = s.enqueue(ctx, tx, c.Actor.OrgID, id, item.ID, item.Generation); e != nil {
				return core.Result{}, e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE object_retirement_plans SET state='approved',version=version+1,approved_at=$3,approval_reason=$4,
   approved_by=$5,approved_session_id=$6,approved_channel=$7,approved_auth_version=$8,approved_org_version=$9 WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, now, in.Reason, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion)
		if e != nil {
			return core.Result{}, e
		}
		if p.Snapshot.Kind == "history" {
			if _, e = tx.Exec(ctx, "UPDATE file_versions SET availability='retiring' WHERE org_id=$1 AND id=$2", c.Actor.OrgID, p.Snapshot.TargetID); e != nil {
				return core.Result{}, e
			}
		}
		p, e = retirementPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 202, Data: p, Event: retirementEvent("file.original.retirement.approved", p, in.Reason)}, e
	})
}
func (s *ObjectRetirementService) Cancel(ctx context.Context, c core.Command, id string, in CleanupDecision) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请核对修订和摘要并说明停止原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, id, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, id, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if (p.State != "observing" && p.State != "approved") || p.Version != in.ExpectedVersion || p.SnapshotSHA256 != in.SnapshotSHA256 {
			return core.Result{}, core.Conflict()
		}
		if _, e = tx.Exec(ctx, "UPDATE object_retirement_plans SET state='cancelled',version=version+1,cancelled_at=clock_timestamp(),cancellation_reason=$3 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, in.Reason); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE object_retirement_items SET state='blocked',last_error='CLEANUP_PLAN_CANCELLED',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND plan_id=$2 AND state IN ('observing','queued','running','retry_wait')", c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		p, e = retirementPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Data: p, Event: retirementEvent("file.original.retirement.cancelled", p, in.Reason)}, e
	})
}
func (s *ObjectRetirementService) Retry(ctx context.Context, c core.Command, itemID string, in UploadExpected) (core.Result, error) {
	if !core.ValidID(itemID) || in.ExpectedVersion < 1 {
		return core.Result{}, core.Missing()
	}
	var planID string
	if e := s.Files.DB.QueryRow(ctx, "SELECT plan_id FROM object_retirement_items WHERE org_id=$1 AND id=$2", c.Actor.OrgID, itemID).Scan(&planID); e != nil {
		return core.Result{}, core.Missing()
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, planID, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, planID, true, true)
		if e != nil {
			return core.Result{}, e
		}
		item, e := retirementItem(tx.QueryRow(ctx, "SELECT "+retirementItemColumns+" FROM object_retirement_items WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, itemID))
		if e != nil {
			return core.Result{}, e
		}
		if p.State != "approved" || (item.State != "failed" && item.State != "blocked") || item.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if e = s.executionAllowed(ctx, tx, p); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE object_retirement_items SET state='queued',version=version+1,generation=generation+1,last_error=NULL,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, itemID); e != nil {
			return core.Result{}, e
		}
		if e = s.enqueue(ctx, tx, c.Actor.OrgID, p.ID, item.ID, item.Generation+1); e != nil {
			return core.Result{}, e
		}
		p, e = retirementPlan(ctx, tx, c.Actor.OrgID, p.ID, false)
		return core.Result{Status: 202, Data: p, Event: retirementEvent("file.original.retirement.retry.requested", p, "Explicit retry of "+item.ID)}, e
	})
}
