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

// Legacy cost-control writes are retired. Authenticate the interactive manager
// before returning410, including when an old command key is replayed. Historical
// policies and ledger entries remain immutable and readable with scoped access.
func (s *Service) retiredBudgetCommand(ctx context.Context, a core.Actor) (core.Result, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return core.Result{}, err
	}
	defer tx.Rollback(ctx)
	if err = budgetAuthority(ctx, tx, a, true); err != nil {
		return core.Result{}, err
	}
	return core.Result{}, core.Fail(410, "AI_COST_CONTROL_RETIRED", "AI费用控制已停用；历史账本只读，不影响秘书接单与执行")
}

func (s *Service) PublishBudgetPolicy(ctx context.Context, c core.Command, _ BudgetPolicyInput) (core.Result, error) {
	return s.retiredBudgetCommand(ctx, c.Actor)
}
