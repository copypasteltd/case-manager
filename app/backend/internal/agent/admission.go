package agent

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// Portal and channel admission use the same limits and actual execution queue.
func checkRunCapacity(ctx context.Context, tx pgx.Tx, org, actor, conversation string) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,47))", org); err != nil {
		return err
	}
	var total, own int
	if err := tx.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE actor_id=$2) FROM agent_runs WHERE org_id=$1 AND state IN ('queued','running','awaiting_approval','committing')", org, actor).Scan(&total, &own); err != nil {
		return err
	}
	if total >= 20 || own >= 3 {
		return core.Fail(429, "RUN_CAPACITY_EXCEEDED", "当前待处理任务已达上限，请先核对或取消已有任务")
	}
	var active bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM agent_runs WHERE org_id=$1 AND conversation_id=$2 AND state IN ('queued','running','awaiting_approval','committing'))", org, conversation).Scan(&active); err != nil {
		return err
	}
	if active {
		return core.Fail(409, "CONVERSATION_RUN_ACTIVE", "此会话已有待处理任务，请先核对或取消")
	}
	return nil
}

func (s *Service) enqueueRun(ctx context.Context, tx pgx.Tx, org, run string) error {
	if err := reserveRunBudget(ctx, tx, org, run); err != nil {
		return err
	}
	if err := appendRunEvent(ctx, tx, org, run, 1, "run.queued", "queued", nil); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"run_id": run})
	jobID, _, err := s.Queue().Enqueue(ctx, tx, jobqueue.Input{OrgID: org, Kind: ExecutionJobKind, Lane: "agent", DedupKey: "agent.run:" + run, PayloadVersion: 1, Payload: payload, MaxAttempts: 1})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO agent_run_jobs(org_id,run_id,job_id) VALUES($1,$2,$3)", org, run, jobID)
	return err
}
