package agent

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

const maxBudgetTokens int64 = 1000000000000

type BudgetPolicy struct {
	ID                 string    `json:"id"`
	Version            int64     `json:"version"`
	OrganizationTokens int64     `json:"organization_tokens"`
	UserTokens         int64     `json:"user_tokens"`
	ProjectTokens      int64     `json:"project_tokens"`
	SkillTokens        int64     `json:"skill_tokens"`
	PublishedBy        string    `json:"published_by"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}
type BudgetPolicyInput struct {
	ExpectedVersion    int64  `json:"expected_version"`
	OrganizationTokens int64  `json:"organization_tokens"`
	UserTokens         int64  `json:"user_tokens"`
	ProjectTokens      int64  `json:"project_tokens"`
	SkillTokens        int64  `json:"skill_tokens"`
	Reason             string `json:"reason"`
}

const budgetPolicyColumns = `id,version,organization_tokens,user_tokens,project_tokens,skill_tokens,published_by,reason,created_at`

func readBudgetPolicy(ctx context.Context, db platform.DBTX, org string) (*BudgetPolicy, error) {
	var p BudgetPolicy
	err := db.QueryRow(ctx, "SELECT "+budgetPolicyColumns+" FROM agent_budget_policies WHERE org_id=$1 ORDER BY version DESC LIMIT 1", org).Scan(&p.ID, &p.Version, &p.OrganizationTokens, &p.UserTokens, &p.ProjectTokens, &p.SkillTokens, &p.PublishedBy, &p.Reason, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}
func budgetAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, manage bool) error {
	if a.RunID != "" || a.Channel == "telegram" {
		return core.Forbidden()
	}
	if err := identity.RequireInteractiveSession(ctx, tx, a, manage); err != nil {
		return err
	}
	if manage {
		return authz.RequireOrganization(ctx, tx, a, "policy.manage")
	}
	return nil
}
func (s *Service) PublishBudgetPolicy(ctx context.Context, c core.Command, in BudgetPolicyInput) (core.Result, error) {
	if in.ExpectedVersion < 0 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("policy", "请填写版本、完整额度与发布原因")
	}
	for _, v := range []int64{in.OrganizationTokens, in.UserTokens, in.ProjectTokens, in.SkillTokens} {
		if v < 0 || v > maxBudgetTokens {
			return core.Result{}, core.Invalid("tokens", "额度必须是0到1000000000000的整数")
		}
	}
	c.AuthorizationMutation = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return budgetAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		old, err := readBudgetPolicy(ctx, tx, c.Actor.OrgID)
		if err != nil {
			return core.Result{}, err
		}
		version := int64(0)
		if old != nil {
			version = old.Version
		}
		if version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if old != nil && old.OrganizationTokens == in.OrganizationTokens && old.UserTokens == in.UserTokens && old.ProjectTokens == in.ProjectTokens && old.SkillTokens == in.SkillTokens {
			return core.Result{}, core.Fail(409, "BUDGET_POLICY_UNCHANGED", "额度未变化")
		}
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO agent_budget_policies(id,org_id,version,organization_tokens,user_tokens,project_tokens,skill_tokens,published_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.Actor.OrgID, version+1, in.OrganizationTokens, in.UserTokens, in.ProjectTokens, in.SkillTokens, c.Actor.UserID, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		p, err := readBudgetPolicy(ctx, tx, c.Actor.OrgID)
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "agent.budget.published", ResourceType: "agent_budget_policy", ResourceID: id, Version: version + 1, Payload: map[string]any{"policy_id": id, "reason": in.Reason}}}, err
	})
}

// Called in the same transaction as the new run, exact input and single-attempt job.
// Every channel, including Telegram, passes through this shared queue admission.
func reserveRunBudget(ctx context.Context, tx pgx.Tx, org, run string) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,47))", org); err != nil {
		return err
	}
	p, err := readBudgetPolicy(ctx, tx, org)
	if err != nil {
		return err
	}
	if p == nil {
		return core.Fail(409, "AGENT_BUDGET_UNCONFIGURED", "管理员尚未配置秘书月度Token额度")
	}
	var actor, skill string
	var project *string
	var estimate int64
	var month time.Time
	err = tx.QueryRow(ctx, `SELECT r.actor_id,c.project_id,p.skill_id,p.max_input_tokens::bigint+p.max_output_tokens,date_trunc('month',clock_timestamp() AT TIME ZONE 'UTC')::date
 FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id JOIN agent_runtime_profiles p ON p.id=r.profile_id WHERE r.org_id=$1 AND r.id=$2`, org, run).Scan(&actor, &project, &skill, &estimate, &month)
	if err != nil {
		return err
	}
	var total, user, proj, sk int64
	err = tx.QueryRow(ctx, `SELECT coalesce(sum(reserved_tokens+charged_tokens),0),coalesce(sum(reserved_tokens+charged_tokens) FILTER(WHERE actor_id=$3),0),
 coalesce(sum(reserved_tokens+charged_tokens) FILTER(WHERE project_id=$4),0),coalesce(sum(reserved_tokens+charged_tokens) FILTER(WHERE skill_id=$5),0)
 FROM agent_budget_entries WHERE org_id=$1 AND month_start=$2`, org, month, actor, project, skill).Scan(&total, &user, &proj, &sk)
	if err != nil {
		return err
	}
	if estimate > p.OrganizationTokens-total || estimate > p.UserTokens-user || estimate > p.SkillTokens-sk || (project != nil && estimate > p.ProjectTokens-proj) {
		return core.Fail(429, "AGENT_BUDGET_EXCEEDED", "月度Token额度不足；本次未创建运行、消息或队列任务")
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_budget_reservations(run_id,org_id,policy_id,actor_id,project_id,skill_id,month_start,estimated_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, run, org, p.ID, actor, project, skill, month, estimate)
	return err
}

func validBudgetUsage(u ExecutionUsage) bool {
	for _, n := range []int64{u.InputTokens, u.OutputTokens, u.CachedInputTokens, u.CacheWriteInputTokens, u.ReasoningOutputTokens} {
		if n < 0 || n > maxBudgetTokens {
			return false
		}
	}
	return u.CachedInputTokens <= u.InputTokens
}
func recordBudgetUsage(ctx context.Context, tx pgx.Tx, org, run string, u ExecutionUsage) error {
	if !validBudgetUsage(u) {
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
	if err = recordBudgetUsage(ctx, tx, lease.OrgID, lease.RunID, u); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
