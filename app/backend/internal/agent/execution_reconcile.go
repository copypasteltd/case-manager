package agent

import (
	"context"
	"time"
)

// Reconcile never restarts an old execution or invents a rollback of business writes.
func (s *Service) ReconcileExecutions(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `SELECT x.org_id,x.run_id,x.job_id FROM agent_run_jobs x JOIN agent_runs r ON r.id=x.run_id JOIN jobs j ON j.id=x.job_id
 WHERE (r.state IN ('queued','running','awaiting_approval','committing') AND (r.expires_at<=clock_timestamp() OR j.state IN ('dead','cancelled','succeeded') OR (j.state='running' AND j.lease_until<=clock_timestamp())))
 OR (r.state IN ('succeeded','failed','cancelled','interrupted') AND j.state IN ('queued','running','retry_wait'))
 ORDER BY r.created_at,r.id LIMIT 100`)
	if err != nil {
		return 0, err
	}
	type item struct{ org, run, job string }
	items := []item{}
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.org, &i.run, &i.job); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, i := range items {
		changed, e := s.reconcileExecution(ctx, i.org, i.run, i.job)
		if e != nil {
			return count, e
		}
		if changed {
			count++
		}
	}
	return count, nil
}
func (s *Service) reconcileExecution(ctx context.Context, org, runID, jobID string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var version int64
	if err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", org).Scan(&version); err != nil {
		return false, err
	}
	var st executionState
	st.Actor.OrgID = org
	if err = tx.QueryRow(ctx, "SELECT actor_id,channel FROM agent_runs WHERE org_id=$1 AND id=$2", org, runID).Scan(&st.Actor.UserID, &st.Actor.Channel); err != nil {
		return false, err
	}
	var actorID, conversationID string
	if err = tx.QueryRow(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", org, st.Actor.UserID).Scan(&actorID); err != nil {
		return false, err
	}
	if err = tx.QueryRow(ctx, "SELECT c.id FROM conversations c JOIN agent_runs r ON r.conversation_id=c.id AND r.org_id=c.org_id WHERE r.org_id=$1 AND r.id=$2 FOR SHARE OF c", org, runID).Scan(&conversationID); err != nil {
		return false, err
	}
	st.Run, err = runRecord(ctx, tx, org, runID, true)
	if err != nil {
		return false, err
	}
	var state string
	var until *time.Time
	if err = tx.QueryRow(ctx, "SELECT state,lease_until FROM jobs WHERE org_id=$1 AND id=$2 FOR UPDATE", org, jobID).Scan(&state, &until); err != nil {
		return false, err
	}
	var expired bool
	if err = tx.QueryRow(ctx, "SELECT $1::timestamptz<=clock_timestamp() OR ($2::timestamptz IS NOT NULL AND $2<=clock_timestamp())", st.Run.ExpiresAt, until).Scan(&expired); err != nil {
		return false, err
	}
	terminal := st.Run.State == "succeeded" || st.Run.State == "failed" || st.Run.State == "cancelled" || st.Run.State == "interrupted"
	if terminal {
		if state != "queued" && state != "running" && state != "retry_wait" {
			return false, tx.Commit(ctx)
		}
		final := "dead"
		if st.Run.State == "succeeded" {
			final = "succeeded"
		} else if st.Run.State == "cancelled" {
			final = "cancelled"
		}
		_, err = tx.Exec(ctx, "UPDATE jobs SET state=$2,lease_owner=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1", jobID, final)
		if err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if !expired && state != "dead" && state != "cancelled" && state != "succeeded" {
		return false, tx.Commit(ctx)
	}
	next, kind, code := "interrupted", "run.interrupted", "RUNTIME_LEASE_LOST"
	if st.Run.State == "queued" {
		next = "failed"
		kind = "run.failed"
	}
	if st.Run.ExpiresAt.Before(time.Now()) {
		code = "RUNTIME_DEADLINE_EXCEEDED"
	}
	if st.Run.State == "committing" {
		code = "COMMIT_RECONCILIATION_REQUIRED"
	}
	if err = executionTransition(ctx, tx, st, next, kind, &code, nil); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, "UPDATE jobs SET state='dead',lease_owner=NULL,lease_until=NULL,last_error=$2,first_failed_at=coalesce(first_failed_at,clock_timestamp()),updated_at=clock_timestamp() WHERE id=$1", jobID, code)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// A queued task can fail without a launched executor. Started tasks remain interrupted until business effects are reconciled.
func executionStopState(st executionState) (string, string) {
	if st.Run.CancelRequestedAt != nil && st.Run.State != "committing" {
		return "cancelled", "run.cancelled"
	}
	if st.Run.State == "queued" {
		return "failed", "run.failed"
	}
	return "interrupted", "run.interrupted"
}
