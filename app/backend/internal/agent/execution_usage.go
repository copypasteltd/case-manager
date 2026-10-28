package agent

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// A telemetry sanity bound, independent of any monetary or monthly allowance.
const maxExecutionUsageTokens int64 = 1000000000000

// agent_budget_usage is the retained storage name. Migration79 binds telemetry
// directly to the exact tenant/run; fee reservations are never required.
func validExecutionUsage(u ExecutionUsage) bool {
	for _, n := range []int64{u.InputTokens, u.OutputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.ReasoningOutputTokens} {
		if n < 0 || n > maxExecutionUsageTokens {
			return false
		}
	}
	return u.CachedInputTokens <= u.InputTokens
}
func recordExecutionUsage(ctx context.Context, tx pgx.Tx, org, run string, u ExecutionUsage) error {
	if !validExecutionUsage(u) {
		return runtimeFailure("RUNTIME_USAGE_INVALID")
	}
	var old ExecutionUsage
	err := tx.QueryRow(ctx, `SELECT input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens FROM agent_budget_usage WHERE org_id=$1 AND run_id=$2`, org, run).Scan(&old.InputTokens, &old.CachedInputTokens, &old.CacheWriteInputTokens, &old.OutputTokens, &old.ReasoningOutputTokens)
	if err == nil {
		if old != u {
			return runtimeFailure("RUNTIME_USAGE_CONFLICT")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_budget_usage(run_id,org_id,input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens) VALUES($1,$2,$3,$4,$5,$6,$7)`, run, org, u.InputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.OutputTokens, u.ReasoningOutputTokens)
	return err
}

// Accounting may still be accepted after cancellation/revocation, using a valid
// controller lease. It grants no business authority and never revives a run.
func (s *Service) RecordExecutionUsage(ctx context.Context, lease ExecutionLease, u ExecutionUsage) error {
	tx, st, err := s.executionTx(ctx, lease, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if st.Run.State == "queued" {
		return runtimeFailure("RUNTIME_NOT_STARTED")
	}
	if err = recordExecutionUsage(ctx, tx, lease.OrgID, lease.RunID, u); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
