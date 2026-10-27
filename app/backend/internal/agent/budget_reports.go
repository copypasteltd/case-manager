package agent

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type BudgetFilter struct{ Month, Scope, ID string }
type BudgetSummary struct {
	Policy          *BudgetPolicy `json:"policy"`
	Month           string        `json:"month"`
	Scope           string        `json:"scope"`
	ScopeID         *string       `json:"scope_id"`
	LimitTokens     *int64        `json:"limit_tokens"`
	ReservedTokens  int64         `json:"reserved_tokens"`
	ChargedTokens   int64         `json:"charged_tokens"`
	PendingTokens   int64         `json:"pending_tokens"`
	AvailableTokens *int64        `json:"available_tokens"`
	UnbudgetedRuns  int64         `json:"unbudgeted_runs"`
}
type BudgetEntry struct {
	ID                   string     `json:"id"`
	PolicyID             string     `json:"policy_id"`
	ActorID              string     `json:"actor_id"`
	ProjectID            *string    `json:"project_id"`
	SkillID              string     `json:"skill_id"`
	RunVersion           int64      `json:"run_version"`
	RunState             string     `json:"run_state"`
	Status               string     `json:"status"`
	EstimatedTokens      int64      `json:"estimated_tokens"`
	ReservedTokens       int64      `json:"reserved_tokens"`
	ChargedTokens        int64      `json:"charged_tokens"`
	PendingTokens        int64      `json:"pending_tokens"`
	InputTokens          *int64     `json:"input_tokens"`
	OutputTokens         *int64     `json:"output_tokens"`
	EvidenceReference    *string    `json:"evidence_reference"`
	ReconciliationReason *string    `json:"reconciliation_reason"`
	ReconciledBy         *string    `json:"reconciled_by"`
	ReconciledAt         *time.Time `json:"reconciled_at"`
	CreatedAt            time.Time  `json:"created_at"`
}

const budgetEntryColumns = `run_id,policy_id,actor_id,project_id,skill_id,run_version,run_state,status,estimated_tokens,reserved_tokens,charged_tokens,pending_tokens,input_tokens,output_tokens,evidence_reference,reconciliation_reason,reconciled_by,reconciled_at,created_at`

func scanBudgetEntry(row pgx.Row) (BudgetEntry, error) {
	var e BudgetEntry
	err := row.Scan(&e.ID, &e.PolicyID, &e.ActorID, &e.ProjectID, &e.SkillID, &e.RunVersion, &e.RunState, &e.Status, &e.EstimatedTokens, &e.ReservedTokens, &e.ChargedTokens, &e.PendingTokens, &e.InputTokens, &e.OutputTokens, &e.EvidenceReference, &e.ReconciliationReason, &e.ReconciledBy, &e.ReconciledAt, &e.CreatedAt)
	return e, platform.DatabaseError(err)
}

const budgetFilterSQL = `org_id=$1 AND month_start=$2 AND ($3='organization' OR ($3='user' AND actor_id=$4::uuid) OR ($3='project' AND project_id=$4::uuid) OR ($3='skill' AND skill_id=$5))`

func budgetSelection(ctx context.Context, tx pgx.Tx, a core.Actor, f BudgetFilter) (BudgetFilter, time.Time, error) {
	if f.Scope == "" || f.Scope == "mine" {
		f.Scope = "user"
		f.ID = a.UserID
	}
	if f.Scope != "user" && f.Scope != "organization" && f.Scope != "project" && f.Scope != "skill" {
		return f, time.Time{}, core.Invalid("scope", "请选择本人、组织、员工、项目或Skill范围")
	}
	manage := f.Scope != "user" || f.ID != a.UserID
	// Reading aggregate accounts does not require a fresh step-up; edits do.
	if err := budgetAuthority(ctx, tx, a, false); err != nil {
		return f, time.Time{}, err
	}
	if manage {
		if err := budgetReadOrganization(ctx, tx, a); err != nil {
			return f, time.Time{}, err
		}
	}
	if (f.Scope == "user" || f.Scope == "project") && !core.ValidID(f.ID) {
		return f, time.Time{}, core.Invalid("scope_id", "请选择准确员工或项目")
	}
	if f.Scope == "skill" && (len(f.ID) < 1 || len(f.ID) > 64 || strings.ContainsAny(f.ID, " \r\n\t")) {
		return f, time.Time{}, core.Invalid("scope_id", "请填写准确Skill标识")
	}
	if f.Scope == "organization" {
		f.ID = ""
	}
	var month time.Time
	var err error
	if f.Month == "" {
		err = tx.QueryRow(ctx, "SELECT date_trunc('month',clock_timestamp() AT TIME ZONE 'UTC')::date").Scan(&month)
	} else {
		month, err = time.Parse("2006-01", f.Month)
		if err != nil {
			return f, month, core.Invalid("month", "月份格式为YYYY-MM，按UTC月归属")
		}
	}
	f.Month = month.Format("2006-01")
	return f, month, err
}
func budgetReadOrganization(ctx context.Context, tx pgx.Tx, a core.Actor) error {
	return authz.RequireOrganization(ctx, tx, a, "policy.manage")
}
func budgetSQLID(f BudgetFilter) *string {
	if f.Scope == "user" || f.Scope == "project" {
		return &f.ID
	}
	return nil
}
func (s *Service) BudgetSummary(ctx context.Context, a core.Actor, f BudgetFilter) (BudgetSummary, error) {
	out := BudgetSummary{}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	f, month, err := budgetSelection(ctx, tx, a, f)
	if err != nil {
		return out, err
	}
	out.Month = f.Month
	out.Scope = f.Scope
	if f.ID != "" {
		out.ScopeID = &f.ID
	}
	out.Policy, err = readBudgetPolicy(ctx, tx, a.OrgID)
	if err != nil {
		return out, err
	}
	if out.Policy != nil {
		limit := out.Policy.OrganizationTokens
		switch f.Scope {
		case "user":
			limit = out.Policy.UserTokens
		case "project":
			limit = out.Policy.ProjectTokens
		case "skill":
			limit = out.Policy.SkillTokens
		}
		out.LimitTokens = &limit
	}
	err = tx.QueryRow(ctx, `SELECT coalesce(sum(reserved_tokens),0),coalesce(sum(charged_tokens),0),coalesce(sum(pending_tokens),0) FROM agent_budget_entries WHERE `+budgetFilterSQL, a.OrgID, month, f.Scope, budgetSQLID(f), f.ID).Scan(&out.ReservedTokens, &out.ChargedTokens, &out.PendingTokens)
	if err != nil {
		return out, err
	}
	if out.LimitTokens != nil {
		available := *out.LimitTokens - out.ReservedTokens - out.ChargedTokens
		out.AvailableTokens = &available
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id JOIN agent_runtime_profiles p ON p.id=r.profile_id
 WHERE r.org_id=$1 AND r.created_at>=($2::date::timestamp AT TIME ZONE 'UTC') AND r.created_at< (($2::date+interval '1 month') AT TIME ZONE 'UTC')
 AND ($3='organization' OR ($3='user' AND r.actor_id=$4::uuid) OR ($3='project' AND c.project_id=$4::uuid) OR ($3='skill' AND p.skill_id=$5))
 AND NOT EXISTS(SELECT 1 FROM agent_budget_reservations b WHERE b.run_id=r.id)`, a.OrgID, month, f.Scope, budgetSQLID(f), f.ID).Scan(&out.UnbudgetedRuns)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) BudgetEntries(ctx context.Context, a core.Actor, f BudgetFilter, cursor string, limit int) (core.Page[BudgetEntry], error) {
	out := core.Page[BudgetEntry]{Items: []BudgetEntry{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页1至100条")
	}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	f, month, err := budgetSelection(ctx, tx, a, f)
	if err != nil {
		return out, err
	}
	bound := core.CursorBinding(a, "agent-budget", f.Month, f.Scope, f.ID)
	var at *time.Time
	var id *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := tx.Query(ctx, "SELECT "+budgetEntryColumns+" FROM agent_budget_entries WHERE "+budgetFilterSQL+` AND ($6::timestamptz IS NULL OR (created_at,run_id)<($6,$7::uuid)) ORDER BY created_at DESC,run_id DESC LIMIT $8`, a.OrgID, month, f.Scope, budgetSQLID(f), f.ID, at, id, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanBudgetEntry(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &next
		out.Items = out.Items[:limit]
	}
	return out, tx.Commit(ctx)
}

type BudgetReconcileInput struct {
	ExpectedVersion   int64  `json:"expected_version"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	EvidenceReference string `json:"evidence_reference"`
	Reason            string `json:"reason"`
}

func (s *Service) ReconcileBudget(ctx context.Context, c core.Command, id string, in BudgetReconcileInput) (core.Result, error) {
	if !core.ValidID(id) || core.RequireVersion(in.ExpectedVersion) != nil || in.InputTokens < 0 || in.InputTokens > maxBudgetTokens || in.OutputTokens < 0 || in.OutputTokens > maxBudgetTokens || core.Name(in.EvidenceReference) != nil || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reconciliation", "请填写准确运行版本、Token数量、外部核对凭据编号与原因")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return budgetAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		r, err := runRecord(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		entry, err := scanBudgetEntry(tx.QueryRow(ctx, "SELECT "+budgetEntryColumns+" FROM agent_budget_entries WHERE org_id=$1 AND run_id=$2", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if entry.Status != "pending" {
			return core.Result{}, core.Fail(409, "BUDGET_NOT_PENDING", "仅可核对已终止但用量未知的运行")
		}
		_, err = tx.Exec(ctx, `INSERT INTO agent_budget_reconciliations(run_id,org_id,run_version,input_tokens,output_tokens,evidence_reference,reason,reconciled_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, c.Actor.OrgID, in.ExpectedVersion, in.InputTokens, in.OutputTokens, in.EvidenceReference, in.Reason, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		entry, err = scanBudgetEntry(tx.QueryRow(ctx, "SELECT "+budgetEntryColumns+" FROM agent_budget_entries WHERE org_id=$1 AND run_id=$2", c.Actor.OrgID, id))
		return core.Result{Status: 200, Data: entry, Event: core.Event{Type: "agent.budget.reconciled", ResourceType: "agent_run", ResourceID: id, Version: r.Version, Payload: map[string]any{"run_id": id, "input_tokens": in.InputTokens, "output_tokens": in.OutputTokens, "evidence_reference": in.EvidenceReference, "reason": in.Reason}}}, err
	})
}
