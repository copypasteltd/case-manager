package project

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"slices"
	"time"
)

func (s *Service) GetChangeBasis(ctx context.Context, a core.Actor, pid, currency string) (ChangeBasis, error) {
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return ChangeBasis{}, e
	}
	defer tx.Rollback(ctx)
	p, e := getProject(ctx, tx, a.OrgID, pid, false)
	if e != nil {
		return ChangeBasis{}, e
	}
	for _, action := range []string{"project.read", "budget.read"} {
		if e = authz.Require(ctx, tx, a, action, resource(p)); e != nil {
			return ChangeBasis{}, e
		}
	}
	var exists bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM currency_definitions WHERE code=$1)", currency).Scan(&exists); e != nil {
		return ChangeBasis{}, e
	}
	if !exists {
		return ChangeBasis{}, core.Invalid("currency", "请选择已配置币种")
	}
	b, e := currentChangeBasis(ctx, tx, a.OrgID, pid, currency)
	if e != nil {
		return b, e
	}
	return b, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ListBudgets(ctx context.Context, a core.Actor, pid string, f Filter) (core.Page[ProjectBudget], error) {
	out := core.Page[ProjectBudget]{Items: []ProjectBudget{}}
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	p, e := getProject(ctx, tx, a.OrgID, pid, false)
	if e != nil {
		return out, e
	}
	for _, action := range []string{"project.read", "budget.read"} {
		if e = authz.Require(ctx, tx, a, action, resource(p)); e != nil {
			return out, e
		}
	}
	if f.State != "" && !slices.Contains([]string{"draft", "approved", "superseded"}, f.State) {
		return out, core.Invalid("state", "预算状态无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-budgets", pid, f.State, f.Query)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, e := tx.Query(ctx, `SELECT b.id,b.project_id,b.revision,b.state,b.currency,b.creator_id,b.change_id,b.change_revision_id,b.predecessor_id,project_budget_line_snapshot(b.org_id,b.id),COALESCE((SELECT sum(amount) FROM budget_lines WHERE org_id=b.org_id AND budget_id=b.id),0)::numeric(20,6)::text,b.created_at FROM project_budgets b JOIN change_requests d ON d.org_id=b.org_id AND d.id=b.change_id WHERE b.org_id=$1 AND b.project_id=$2 AND b.protocol_version=1 AND ($3='' OR b.state=$3) AND ($4='' OR b.currency=$4) AND (b.state<>'draft' OR can_access(d.org_id,$5,'change.read','change',d.id,d.project_id,d.creator_id,d.classification,$6,$10)) AND ($7::timestamptz IS NULL OR (b.created_at,b.id)<($7,$8::uuid)) ORDER BY b.created_at DESC,b.id DESC LIMIT $9`, a.OrgID, pid, f.State, f.Query, a.UserID, a.Channel, before, beforeID, f.Limit+1, a.RunID != "")
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var b ProjectBudget
		var raw []byte
		e = rows.Scan(&b.ID, &b.ProjectID, &b.Revision, &b.State, &b.Currency, &b.CreatorID, &b.ChangeID, &b.ChangeRevisionID, &b.PredecessorID, &raw, &b.Total, &b.CreatedAt)
		if e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &b.Lines); e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
