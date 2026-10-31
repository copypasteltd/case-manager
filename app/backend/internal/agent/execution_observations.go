package agent

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"github.com/jackc/pgx/v5"
)

// Called only after the native controller verifies an externally signed, run-bound proof.
func (s *Service) RecordVerifiedConfinement(ctx context.Context, lease ExecutionLease, configuration, proof string, expires time.Time) error {
	if len(proof) != 64 {
		return runtimeFailure("RUNTIME_CONFINEMENT_UNVERIFIED")
	}
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if st.Run.State != "queued" || st.Profile.ConfigurationSHA256 != configuration {
		return runtimeFailure("RUNTIME_CONFINEMENT_UNVERIFIED")
	}
	var live bool
	if err = tx.QueryRow(ctx, "SELECT $1::timestamptz>clock_timestamp() AND $1::timestamptz<=clock_timestamp()+interval '60 seconds' AND $1::timestamptz<=$2", expires, st.Run.ExpiresAt).Scan(&live); err != nil {
		return err
	}
	if !live {
		return runtimeFailure("RUNTIME_CONFINEMENT_UNVERIFIED")
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_execution_verifications(run_id,org_id,authority_stamp,configuration_sha256,proof_sha256,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, lease.RunID, lease.OrgID, lease.AuthorityStamp, configuration, proof, expires)
	if err != nil {
		return err
	}
	if err = executionAudit(ctx, tx, st, "agent.native.verified", map[string]any{"configuration_sha256": configuration, "proof_sha256": proof}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Observations contain no prompt, reasoning, document body or credential. An executor's
// interruption report is evidence of its report, not evidence of process teardown.
func (s *Service) ObserveExecution(ctx context.Context, lease ExecutionLease, kind, tool, code string) error {
	if !slices.Contains([]string{"tool.started", "tool.finished", "run.interrupted"}, kind) {
		return runtimeFailure("RUNTIME_PROTOCOL_INVALID")
	}
	if kind == "run.interrupted" {
		if !slices.Contains(runtimeCodes, code) {
			code = "RUNTIME_UNAVAILABLE"
		}
		tool = ""
	} else if !slices.Contains([]string{"list_project_catalog", "list_subscriptions", "read_subscription_review", "read_finance_record", "read_finance_summary", "read_service_summary", "read_resource", "search_resources", "inspect_operation", "propose_project", "create_project", "update_task_dependency", "create_task", "create_risk", "update_risk", "create_issue", "update_issue", "create_deliverable", "update_deliverable", "create_milestone", "update_project", "update_task", "transition_task", "transition_project", "update_milestone", "transition_milestone", "recover_file_draft", "create_file_draft", "propose_file_update", "commit_file_update", "list_directory", "propose_organization", "commit_organization", "stage_file_create", "propose_file_create", "commit_file_create", "draft_request", "submit_request", "draft_business_text", "draft_post", "publish_post"}, tool) || code != "" {
		return runtimeFailure("SKILL_TOOL_DENIED")
	}
	tx, st, err := s.executionTx(ctx, lease, kind != "run.interrupted")
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if kind != "run.interrupted" && (st.Run.State != "running" || !allowedExecutionTool(st, tool)) {
		return runtimeFailure("SKILL_TOOL_DENIED")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM agent_execution_observations WHERE org_id=$1 AND run_id=$2", lease.OrgID, lease.RunID).Scan(&count); err != nil {
		return err
	}
	var limit int
	if err = tx.QueryRow(ctx, "SELECT max_tool_calls*2+4 FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2", lease.OrgID, st.Run.ProfileID).Scan(&limit); err != nil {
		return err
	}
	if count >= limit {
		return runtimeFailure("RUNTIME_TOOL_BUDGET_EXCEEDED")
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_execution_observations(id,org_id,run_id,authority_stamp,type,tool,safe_code) VALUES($1,$2,$3,$4,$5,nullif($6,''),nullif($7,''))`, core.NewID(), lease.OrgID, lease.RunID, lease.AuthorityStamp, kind, tool, code)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) ExecutionStopCode(ctx context.Context, j jobqueue.Job) string {
	var code string
	err := s.DB.QueryRow(ctx, `SELECT o.safe_code FROM agent_execution_observations o JOIN agent_run_jobs x ON x.run_id=o.run_id
 JOIN agent_execution_leases l ON l.run_id=x.run_id AND l.authority_stamp=o.authority_stamp WHERE x.org_id=$1 AND x.job_id=$2 AND l.fencing_token=$3 AND o.type='run.interrupted' ORDER BY o.created_at DESC,o.id DESC LIMIT 1`, j.OrgID, j.ID, j.FencingToken).Scan(&code)
	if err == pgx.ErrNoRows || err != nil || !slices.Contains(runtimeCodes, code) {
		return "RUNTIME_EXECUTOR_EXITED"
	}
	return code
}

func (s *Service) ExecutionProfile(ctx context.Context, lease ExecutionLease) (RuntimeProfile, error) {
	profile, _, err := s.ExecutionScope(ctx, lease)
	return profile, err
}

// Scope is derived from the authenticated lease, never from an MCP request.
func (s *Service) ExecutionScope(ctx context.Context, lease ExecutionLease) (RuntimeProfile, string, error) {
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return RuntimeProfile{}, "", err
	}
	defer tx.Rollback(ctx)
	return st.Profile, st.Actor.Channel, tx.Commit(ctx)
}
