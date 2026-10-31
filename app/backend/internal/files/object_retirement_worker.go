package files

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

const RetirementJobKind = "file.original.retirement.v1"
const RetirementJobLane = "original-cleanup"

func (s *ObjectRetirementService) queue() jobqueue.Queue { return jobqueue.Queue{DB: s.Files.DB} }
func (s *ObjectRetirementService) enqueue(ctx context.Context, tx pgx.Tx, org, plan, item string, generation int) error {
	raw, _ := json.Marshal(cleanupJob{plan, item, generation})
	_, _, e := s.queue().Enqueue(ctx, tx, jobqueue.Input{OrgID: org, Kind: RetirementJobKind, Lane: RetirementJobLane, DedupKey: fmt.Sprintf("%s/%s/%d", RetirementJobKind, item, generation), PayloadVersion: 1, Payload: raw, MaxAttempts: 8})
	return e
}
func (s *ObjectRetirementService) jobTx(ctx context.Context, j jobqueue.Job, fn func(pgx.Tx, RetirementPlan, RetirementItem) error) error {
	var in cleanupJob
	d := json.NewDecoder(bytes.NewReader(j.Payload))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || j.Kind != RetirementJobKind || j.Lane != RetirementJobLane || j.PayloadVersion != 1 || !core.ValidID(in.PlanID) || !core.ValidID(in.ItemID) || in.Generation < 1 {
		return core.Fail(409, "INVALID_JOB_INPUT", "原件清理任务无效")
	}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SET LOCAL idle_in_transaction_session_timeout='60s'"); e != nil {
		return e
	}
	var id string
	if e = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&id); e != nil {
		return e
	}
	p, e := retirementPlan(ctx, tx, j.OrgID, in.PlanID, true)
	if e != nil {
		return e
	}
	if _, e = file(ctx, tx, j.OrgID, p.Snapshot.FileID, true); e != nil {
		return e
	}
	item, e := retirementItem(tx.QueryRow(ctx, "SELECT "+retirementItemColumns+" FROM object_retirement_items WHERE org_id=$1 AND plan_id=$2 AND id=$3 FOR UPDATE", j.OrgID, p.ID, in.ItemID))
	if e != nil {
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
func (s *ObjectRetirementService) executionAllowed(ctx context.Context, tx pgx.Tx, p RetirementPlan) error {
	if p.State != "approved" || p.ApprovedBy == nil || p.ApprovedSession == nil || p.ApprovedChannel == nil || p.ApprovedAuthVersion == nil || p.ApprovedOrgVersion == nil {
		return core.Fail(409, "CLEANUP_PLAN_CANCELLED", "清理计划已经停止")
	}
	var org string
	if e := tx.QueryRow(ctx, "SELECT org_id FROM object_retirement_plans WHERE id=$1", p.ID).Scan(&org); e != nil {
		return e
	}
	a := core.Actor{OrgID: org, UserID: *p.ApprovedBy, SessionID: *p.ApprovedSession, Channel: *p.ApprovedChannel, AuthVersion: *p.ApprovedAuthVersion, OrgAuthVersion: *p.ApprovedOrgVersion}
	var lock string
	if e := tx.QueryRow(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", org, a.UserID).Scan(&lock); e != nil {
		return e
	}
	if e := tx.QueryRow(ctx, "SELECT id FROM sessions WHERE org_id=$1 AND id=$2 FOR SHARE", org, a.SessionID).Scan(&lock); e != nil {
		return e
	}
	f, e := s.actor(ctx, tx, a, p.Snapshot.FileID, false)
	if e != nil {
		return e
	}
	current, objects, e := s.assess(ctx, tx, org, f, p.Snapshot.Kind, p.Snapshot.TargetID, p.ID)
	if e != nil {
		return e
	}
	if !current.Eligible || digest(current) != digest(p.Snapshot) || digest(objects) != digest(p.Objects) {
		return core.Fail(409, "CLEANUP_SELECTION_CHANGED", "原件来源、制度、授权或保护已变化")
	}
	var count int
	e = tx.QueryRow(ctx, "SELECT count(*) FROM object_retirement_barriers WHERE org_id=$1 AND plan_id=$2", org, p.ID).Scan(&count)
	if e != nil {
		return e
	}
	if count != len(p.Objects) {
		return core.Fail(409, "CLEANUP_BARRIER_REQUIRED", "原件保护屏障不完整")
	}
	return nil
}
func (s *ObjectRetirementService) receipt(ctx context.Context, tx pgx.Tx, j jobqueue.Job, p RetirementPlan, item RetirementItem, outcome, code string) error {
	_, e := tx.Exec(ctx, `INSERT INTO object_retirement_receipts(id,org_id,item_id,job_id,generation,attempt,outcome,error_code)
 VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')) ON CONFLICT(org_id,item_id,generation,attempt,outcome) DO NOTHING`, core.NewID(), j.OrgID, item.ID, j.ID, item.Generation, j.Attempt, outcome, code)
	if e != nil {
		return e
	}
	payload, _ := json.Marshal(map[string]any{"plan_id": p.ID, "file_id": p.Snapshot.FileID, "kind": p.Snapshot.Kind, "target_id": p.Snapshot.TargetID, "item_id": item.ID, "provider": item.Provider, "generation": item.Generation, "attempt": j.Attempt, "outcome": outcome, "error_code": code})
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,job_id)
 VALUES($1,$2,$3,NULL,'worker',$4,$5,'object_retirement_item',$6,$7,$8,'succeeded',$9)`, core.NewID(), j.OrgID, p.ApprovedBy, core.NewID(), "file.original.retirement."+outcome, item.ID, item.Version, core.Digest(payload), j.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
 VALUES($1,$2,$3,'object_retirement_item',$4,$5,$6,$7)`, core.NewID(), j.OrgID, "file.original.retirement."+outcome, item.ID, item.Version, payload, core.NewID())
	return e
}
func (s *ObjectRetirementService) finish(ctx context.Context, tx pgx.Tx, j jobqueue.Job, p RetirementPlan, item RetirementItem, state, code string, retry bool) error {
	outcome := state
	if state == "succeeded" {
		outcome = "confirmed_absent"
	}
	if state == "retry_wait" {
		outcome = "retry_scheduled"
	}
	if e := tx.QueryRow(ctx, `UPDATE object_retirement_items SET state=$3,version=version+1,attempts=attempts+1,last_error=NULLIF($4,''),
 confirmed_at=CASE WHEN $3='succeeded' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING version`, j.OrgID, item.ID, state, code).Scan(&item.Version); e != nil {
		return e
	}
	if e := s.receipt(ctx, tx, j, p, item, outcome, code); e != nil {
		return e
	}
	if state == "succeeded" && p.Snapshot.Kind == "history" {
		obj := p.Objects[item.Ordinal]
		if _, e := tx.Exec(ctx, "UPDATE file_replicas SET state='missing',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", j.OrgID, obj.ReplicaID); e != nil {
			return e
		}
	}
	if state == "succeeded" {
		var remaining int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM object_retirement_items WHERE org_id=$1 AND plan_id=$2 AND state<>'succeeded'", j.OrgID, p.ID).Scan(&remaining); e != nil {
			return e
		}
		if remaining == 0 {
			if p.Snapshot.Kind == "history" {
				if _, e := tx.Exec(ctx, "UPDATE file_versions SET availability='purged' WHERE org_id=$1 AND id=$2", j.OrgID, p.Snapshot.TargetID); e != nil {
					return e
				}
			}
			if _, e := tx.Exec(ctx, "UPDATE object_retirement_plans SET state='completed',version=version+1 WHERE org_id=$1 AND id=$2", j.OrgID, p.ID); e != nil {
				return e
			}
		}
	}
	if state == "succeeded" || state == "blocked" {
		return s.queue().CompleteInTx(ctx, tx, j)
	}
	return s.queue().FailInTx(ctx, tx, j, code, !retry, time.Duration(1<<min(j.Attempt, 6))*time.Second)
}
func (s *ObjectRetirementService) Process(ctx context.Context, j jobqueue.Job) error {
	ctx, stop := context.WithTimeout(ctx, 45*time.Second)
	defer stop()
	stopped := false
	e := s.jobTx(ctx, j, func(tx pgx.Tx, p RetirementPlan, item RetirementItem) error {
		if e := s.executionAllowed(ctx, tx, p); e != nil {
			stopped = true
			return s.finish(ctx, tx, j, p, item, "blocked", core.PublicError(e).Code, false)
		}
		if item.State == "blocked" || item.State == "failed" {
			stopped = true
			return s.queue().CompleteInTx(ctx, tx, j)
		}
		if e := tx.QueryRow(ctx, "UPDATE object_retirement_items SET state='running',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING version", j.OrgID, item.ID).Scan(&item.Version); e != nil {
			return e
		}
		return s.receipt(ctx, tx, j, p, item, "started", "")
	})
	if e != nil || stopped {
		return e
	}
	return s.jobTx(ctx, j, func(tx pgx.Tx, p RetirementPlan, item RetirementItem) error {
		if e := s.executionAllowed(ctx, tx, p); e != nil {
			return s.finish(ctx, tx, j, p, item, "blocked", core.PublicError(e).Code, false)
		}
		if item.Ordinal < 0 || item.Ordinal >= len(p.Objects) {
			return core.Conflict()
		}
		obj := p.Objects[item.Ordinal]
		b := s.Backends[obj.Provider]
		if b == nil || b.Cleaner == nil {
			return s.finish(ctx, tx, j, p, item, "blocked", "CLEANUP_IDENTITY_UNAVAILABLE", false)
		}
		absent, e := b.Cleaner.RemoveFrozen(ctx, obj.Source)
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
func (s *ObjectRetirementService) Fail(ctx context.Context, j jobqueue.Job, cause error) error {
	return s.jobTx(ctx, j, func(tx pgx.Tx, p RetirementPlan, item RetirementItem) error {
		// A human may stop the plan after an operation transaction rolls back and
		// before its recovery transaction acquires the lock. Never revive that work.
		if p.State != "approved" || item.State == "blocked" || item.State == "failed" {
			return s.queue().CompleteInTx(ctx, tx, j)
		}
		code, retry := cleanupProviderFailure(cause)
		state := "failed"
		if retry && j.Attempt < j.MaxAttempts {
			state = "retry_wait"
		}
		return s.finish(ctx, tx, j, p, item, state, code, retry)
	})
}
func (s *ObjectRetirementService) RunOne(ctx context.Context, owner string) (bool, error) {
	q := s.queue()
	if _, e := q.SweepExhausted(ctx, []string{RetirementJobKind}, RetirementJobLane); e != nil {
		return false, e
	}
	if e := s.sweepDead(ctx); e != nil {
		return false, e
	}
	j, e := q.Claim(ctx, []string{RetirementJobKind}, RetirementJobLane, owner, 2*time.Minute)
	if e != nil || j == nil {
		return false, e
	}
	work, stop := context.WithTimeout(ctx, 45*time.Second)
	e = s.Process(work, *j)
	stop()
	if e != nil && ctx.Err() == nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if failure := s.Fail(recovery, *j, e); failure != nil {
			return true, failure
		}
	}
	return true, e
}
