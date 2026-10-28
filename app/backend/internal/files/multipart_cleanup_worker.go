package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const CleanupJobKind = "upload.multipart.cleanup.v1"
const CleanupJobLane = "storage-cleanup"

type cleanupJob struct {
	PlanID     string `json:"plan_id"`
	ItemID     string `json:"item_id"`
	Generation int    `json:"generation"`
}

func (s *MultipartCleanupService) enqueue(ctx context.Context, tx pgx.Tx, org, plan, item string, generation int) error {
	raw, _ := json.Marshal(cleanupJob{plan, item, generation})
	_, _, e := s.queue().Enqueue(ctx, tx, jobqueue.Input{OrgID: org, Kind: CleanupJobKind, Lane: CleanupJobLane,
		DedupKey: fmt.Sprintf("%s/%s/%d", CleanupJobKind, item, generation), PayloadVersion: 1, Payload: raw, MaxAttempts: 8})
	return e
}
func decodeCleanupJob(j jobqueue.Job) (cleanupJob, error) {
	var v cleanupJob
	d := json.NewDecoder(bytes.NewReader(j.Payload))
	d.DisallowUnknownFields()
	e := d.Decode(&v)
	if e != nil || d.Decode(new(any)) != io.EOF || j.Kind != CleanupJobKind || j.Lane != CleanupJobLane || j.PayloadVersion != 1 || !core.ValidID(v.PlanID) || !core.ValidID(v.ItemID) || v.Generation < 1 {
		return v, core.Fail(409, "INVALID_JOB_INPUT", "清理任务输入无效")
	}
	return v, nil
}
func (s *MultipartCleanupService) jobTx(ctx context.Context, j jobqueue.Job, fn func(pgx.Tx, CleanupPlan, CleanupItem) error) error {
	in, e := decodeCleanupJob(j)
	if e != nil {
		return e
	}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var org string
	if e = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&org); e != nil {
		return e
	}
	p, e := cleanupPlan(ctx, tx, j.OrgID, in.PlanID, true)
	if e != nil {
		return e
	}
	f, e := folder(ctx, tx, j.OrgID, p.Snapshot.FolderID, false)
	if e != nil {
		return e
	}
	if e = cleanupNamespace(ctx, tx, j.OrgID, f); e != nil {
		return e
	}
	index := slices.IndexFunc(p.Items, func(v CleanupItem) bool { return v.ID == in.ItemID })
	if index < 0 {
		return core.Missing()
	}
	item, e := scanCleanupItem(tx.QueryRow(ctx, "SELECT "+cleanupItemColumns+" FROM upload_cleanup_items WHERE org_id=$1 AND id=$2 FOR UPDATE", j.OrgID, in.ItemID))
	if e != nil {
		return e
	}
	if _, e = upload(ctx, tx, j.OrgID, item.UploadID, true); e != nil {
		return e
	}
	if e = s.queue().CheckInTx(ctx, tx, j); e != nil {
		return e
	}
	if item.Generation != in.Generation || item.State == "succeeded" {
		if e = s.queue().CompleteInTx(ctx, tx, j); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if e = fn(tx, p, item); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *MultipartCleanupService) executionAllowed(ctx context.Context, tx pgx.Tx, p CleanupPlan, item CleanupItem) error {
	if p.State != "approved" || p.ApprovedBy == nil || p.ApprovedSession == nil || p.ApprovedChannel == nil || p.ApprovedAuthVersion == nil || p.ApprovedOrgVersion == nil {
		return core.Fail(409, "CLEANUP_PLAN_CANCELLED", "清理计划已停止")
	}
	var org string
	if e := tx.QueryRow(ctx, "SELECT org_id FROM upload_cleanup_plans WHERE id=$1", p.ID).Scan(&org); e != nil {
		return e
	}
	a := core.Actor{OrgID: org, UserID: *p.ApprovedBy, SessionID: *p.ApprovedSession, Channel: *p.ApprovedChannel, AuthVersion: *p.ApprovedAuthVersion, OrgAuthVersion: *p.ApprovedOrgVersion}
	// Hold identity/session locks through the bounded provider operation. The
	// approval cannot outlive a logout, revocation or changed authorization.
	var user string
	if e := tx.QueryRow(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", org, a.UserID).Scan(&user); e != nil {
		return e
	}
	if e := tx.QueryRow(ctx, "SELECT id FROM sessions WHERE org_id=$1 AND id=$2 FOR SHARE", org, a.SessionID).Scan(&user); e != nil {
		return e
	}
	f, e := s.actor(ctx, tx, a, p.Snapshot.FolderID, false)
	if e != nil {
		return e
	}
	if f.Version != p.Snapshot.FolderVersion {
		return core.Fail(409, "CLEANUP_SELECTION_CHANGED", "清单目录已变化")
	}
	u, e := upload(ctx, tx, org, item.UploadID, false)
	if e != nil {
		return e
	}
	if e = cleanupUploadAccess(ctx, tx, a, f, u); e != nil {
		return e
	}
	if u.FileID != nil {
		if _, e = file(ctx, tx, org, *u.FileID, true); e != nil {
			return e
		}
	}
	if item.ApprovedUploadVersion == nil || u.Version != *item.ApprovedUploadVersion || u.State != "expired" {
		return core.Fail(409, "CLEANUP_SELECTION_CHANGED", "已批准的上传修订已变化")
	}
	current, e := s.assess(ctx, tx, org, u, item.ID)
	if e != nil {
		return e
	}
	index := slices.IndexFunc(p.Snapshot.Items, func(v CleanupCandidate) bool { return v.UploadID == u.ID })
	if index < 0 {
		return core.Conflict()
	}
	pinned := p.Snapshot.Items[index]
	if !current.Eligible || current.SourceSHA256 != pinned.SourceSHA256 || current.BackendSHA256 != pinned.BackendSHA256 || digest(current.Policy) != digest(pinned.Policy) {
		return core.Fail(409, "CLEANUP_SELECTION_CHANGED", "清单来源、制度、保护或存储目标已变化")
	}
	return nil
}
func (s *MultipartCleanupService) receipt(ctx context.Context, tx pgx.Tx, j jobqueue.Job, p CleanupPlan, item CleanupItem, outcome, code string) error {
	_, e := tx.Exec(ctx, `INSERT INTO upload_cleanup_receipts(id,org_id,item_id,job_id,generation,attempt,outcome,error_code)
  VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''))`, core.NewID(), j.OrgID, item.ID, j.ID, item.Generation, j.Attempt, outcome, code)
	if e != nil {
		return e
	}
	payload, _ := json.Marshal(map[string]any{"plan_id": p.ID, "item_id": item.ID, "upload_id": item.UploadID, "outcome": outcome, "error_code": code, "generation": item.Generation, "attempt": j.Attempt})
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,job_id)
  VALUES($1,$2,$3,NULL,'worker',$4,$5,'upload_cleanup_item',$6,$7,$8,'succeeded',$9)`, core.NewID(), j.OrgID, p.ApprovedBy, core.NewID(), "upload.cleanup."+outcome, item.ID, item.Version, core.Digest(payload), j.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
  VALUES($1,$2,$3,'upload_cleanup_item',$4,$5,$6,$7)`, core.NewID(), j.OrgID, "upload.cleanup."+outcome, item.ID, item.Version, payload, core.NewID())
	return e
}
func (s *MultipartCleanupService) finish(ctx context.Context, tx pgx.Tx, j jobqueue.Job, p CleanupPlan, item CleanupItem, state, code string, retry bool) error {
	outcome := state
	if state == "succeeded" {
		outcome = "confirmed_absent"
	}
	if state == "retry_wait" {
		outcome = "retry_scheduled"
	}
	e := tx.QueryRow(ctx, `UPDATE upload_cleanup_items SET state=$3,version=version+1,last_error=NULLIF($4,''),attempts=attempts+1,
  confirmed_at=CASE WHEN $3='succeeded' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp()
  WHERE org_id=$1 AND id=$2 RETURNING version`, j.OrgID, item.ID, state, code).Scan(&item.Version)
	if e != nil {
		return e
	}
	if e = s.receipt(ctx, tx, j, p, item, outcome, code); e != nil {
		return e
	}
	if state == "succeeded" || state == "blocked" {
		return s.queue().CompleteInTx(ctx, tx, j)
	}
	return s.queue().FailInTx(ctx, tx, j, code, !retry, time.Duration(1<<min(j.Attempt, 6))*time.Second)
}
func cleanupProviderFailure(cause error) (string, bool) {
	var cloud *objectstore.Error
	if errors.As(cause, &cloud) {
		switch cloud.Code {
		case "OBJECT_NOT_FOUND", "PROVIDER_TIMEOUT", "PROVIDER_RETRYABLE", "PROVIDER_ACCESS_DENIED", "SOURCE_CHANGED", "INVALID_STORAGE_KEY", "INVALID_MULTIPART_ID", "PROVIDER_FAILURE":
			return cloud.Code, cloud.Retryable || cloud.Uncertain
		}
	}
	return "CLEANUP_PROVIDER_UNCERTAIN", true
}
func (s *MultipartCleanupService) Process(ctx context.Context, j jobqueue.Job) error {
	stopped := false
	// Persist start separately. A crash or lost receipt leaves an explicit open
	// attempt; the next fenced owner reconciles the exact multipart ID again.
	e := s.jobTx(ctx, j, func(tx pgx.Tx, p CleanupPlan, item CleanupItem) error {
		if e := s.executionAllowed(ctx, tx, p, item); e != nil {
			stopped = true
			return s.finish(ctx, tx, j, p, item, "blocked", core.PublicError(e).Code, false)
		}
		if item.State == "blocked" || item.State == "failed" {
			stopped = true
			return s.queue().CompleteInTx(ctx, tx, j)
		}
		if e := tx.QueryRow(ctx, "UPDATE upload_cleanup_items SET state='running',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING version", j.OrgID, item.ID).Scan(&item.Version); e != nil {
			return e
		}
		return s.receipt(ctx, tx, j, p, item, "started", "")
	})
	if e != nil || stopped {
		return e
	}
	return s.jobTx(ctx, j, func(tx pgx.Tx, p CleanupPlan, item CleanupItem) error {
		if e := s.executionAllowed(ctx, tx, p, item); e != nil {
			return s.finish(ctx, tx, j, p, item, "blocked", core.PublicError(e).Code, false)
		}
		u, e := upload(ctx, tx, j.OrgID, item.UploadID, false)
		if e != nil {
			return e
		}
		b := s.Backends[*u.Provider]
		if b == nil || b.Cleaner == nil {
			return s.finish(ctx, tx, j, p, item, "blocked", "CLEANUP_IDENTITY_UNAVAILABLE", false)
		}
		// A successful Abort alone is insufficient. Re-query the exact upload ID;
		// only provider not-found confirms removal, never an empty parts page.
		absent, e := b.Cleaner.MultipartAbsent(ctx, u.multipart())
		if e == nil && !absent {
			e = b.Cleaner.Abort(ctx, u.multipart())
			if e == nil {
				absent, e = b.Cleaner.MultipartAbsent(ctx, u.multipart())
			}
		}
		if e == nil && !absent {
			e = &objectstore.Error{Code: "PROVIDER_RETRYABLE", Retryable: true}
		}
		if e != nil {
			code, retry := cleanupProviderFailure(e)
			state := "failed"
			if retry && j.Attempt < j.MaxAttempts {
				state = "retry_wait"
			}
			return s.finish(ctx, tx, j, p, item, state, code, retry)
		}
		return s.finish(ctx, tx, j, p, item, "succeeded", "", false)
	})
}
func (s *MultipartCleanupService) Fail(ctx context.Context, j jobqueue.Job, cause error) error {
	return s.jobTx(ctx, j, func(tx pgx.Tx, p CleanupPlan, item CleanupItem) error {
		code, retry := cleanupProviderFailure(cause)
		state := "failed"
		if retry && j.Attempt < j.MaxAttempts {
			state = "retry_wait"
		}
		return s.finish(ctx, tx, j, p, item, state, code, retry)
	})
}
func (s *MultipartCleanupService) Receipts(ctx context.Context, a core.Actor, itemID string, filter Filter) (core.Page[CleanupReceipt], error) {
	out := core.Page[CleanupReceipt]{Items: []CleanupReceipt{}}
	if !core.ValidID(itemID) {
		return out, core.Missing()
	}
	var planID string
	if e := s.Files.DB.QueryRow(ctx, "SELECT plan_id FROM upload_cleanup_items WHERE org_id=$1 AND id=$2", a.OrgID, itemID).Scan(&planID); e != nil {
		return out, platform.DatabaseError(e)
	}
	if _, e := s.readPlan(ctx, s.Files.DB, a, planID, false, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "upload-cleanup-receipts/"+itemID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, `SELECT id,generation,attempt,outcome,error_code,created_at FROM upload_cleanup_receipts
  WHERE org_id=$1 AND item_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, itemID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v CleanupReceipt
		if e = rows.Scan(&v.ID, &v.Generation, &v.Attempt, &v.Outcome, &v.ErrorCode, &v.CreatedAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.ID, last.CreatedAt)
	}
	return out, nil
}
func (s *MultipartCleanupService) RunOne(ctx context.Context, owner string) (bool, error) {
	// Reclaims include expired running jobs; every attempt carries a new fence.
	q := s.queue()
	if _, e := q.SweepExhausted(ctx, []string{CleanupJobKind}, CleanupJobLane); e != nil {
		return false, e
	}
	if e := s.sweepDead(ctx); e != nil {
		return false, e
	}
	j, e := q.Claim(ctx, []string{CleanupJobKind}, CleanupJobLane, owner, 2*time.Minute)
	if e != nil || j == nil {
		return false, e
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	e = s.Process(work, *j)
	cancel()
	if e != nil && ctx.Err() == nil {
		recovery, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		if fail := s.Fail(recovery, *j, e); fail != nil {
			return true, fail
		}
	}
	return true, e
}
