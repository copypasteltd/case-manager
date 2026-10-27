package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type BudgetComparisonBasis struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Total    string `json:"total"`
}

type BudgetComparison struct {
	ProjectID      string                 `json:"project_id"`
	ProjectVersion int64                  `json:"project_version"`
	Currency       string                 `json:"currency"`
	BasisHash      string                 `json:"basis_hash"`
	Budget         *BudgetComparisonBasis `json:"budget"`
	BudgetVariance *string                `json:"budget_variance"`
	Scope          string                 `json:"scope"`
	Timezone       string                 `json:"timezone"`
	AsOf           time.Time              `json:"as_of"`
	finance.BudgetAnalysis
}

type BudgetComparisonFilter struct {
	Currency, BasisHash, Line, State, Cursor string
	Limit                                    int
}

type BudgetComparisonEntries struct {
	BasisHash  string                    `json:"basis_hash"`
	AsOf       time.Time                 `json:"as_of"`
	Items      []finance.BudgetCostEntry `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

func comparisonBasis(ctx context.Context, tx pgx.Tx, a core.Actor, pid, currency string) (BudgetComparison, []byte, error) {
	out := BudgetComparison{ProjectID: pid, Currency: currency, Scope: "authorized_records_only", Timezone: "Asia/Hong_Kong"}
	p, err := getProject(ctx, tx, a.OrgID, pid, false)
	if err != nil {
		return out, nil, err
	}
	for _, action := range []string{"project.read", "budget.read", "report.read"} {
		if err = authz.Require(ctx, tx, a, action, resource(p)); err != nil {
			return out, nil, err
		}
	}
	var configured bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM currency_definitions WHERE code=$1)", currency).Scan(&configured); err != nil {
		return out, nil, err
	}
	if !configured {
		return out, nil, core.Invalid("currency", "请选择已配置币种；不同币种分别核对")
	}
	basis, err := currentChangeBasis(ctx, tx, a.OrgID, pid, currency)
	if err != nil {
		return out, nil, err
	}
	out.ProjectVersion = p.Version
	if basis.BudgetLines == nil {
		basis.BudgetLines = []BudgetLine{}
	}
	windows, err := json.Marshal(basis.BudgetLines)
	if err != nil {
		return out, nil, err
	}
	if basis.BudgetID != nil {
		out.Budget = &BudgetComparisonBasis{ID: *basis.BudgetID, Total: basis.BudgetTotal}
		if err = tx.QueryRow(ctx, `SELECT revision FROM project_budgets WHERE org_id=$1 AND project_id=$2 AND id=$3 AND state='approved' AND protocol_version=1`, a.OrgID, pid, *basis.BudgetID).Scan(&out.Budget.Revision); err != nil {
			return out, nil, platform.DatabaseError(err)
		}
	}
	// Pins the current implemented plan and project, not a historical expense
	// snapshot. Each detail page is a fresh authorized read with its own as_of.
	raw, _ := json.Marshal([]any{pid, p.Version, currency, basis.BudgetID, basis.BudgetLines})
	hash := sha256.Sum256(raw)
	out.BasisHash = hex.EncodeToString(hash[:])
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, nil, err
	}
	return out, windows, nil
}

func (s *Service) CompareBudget(ctx context.Context, a core.Actor, pid, currency string) (BudgetComparison, error) {
	tx, err := s.deliverableSnapshot(ctx, a)
	if err != nil {
		return BudgetComparison{}, err
	}
	defer tx.Rollback(ctx)
	out, windows, err := comparisonBasis(ctx, tx, a, pid, currency)
	if err != nil {
		return out, err
	}
	out.BudgetAnalysis, err = finance.ProjectBudgetAnalysisInTx(ctx, tx, a, pid, currency, windows)
	if err != nil {
		return out, err
	}
	if out.Budget != nil {
		var variance string
		if err = tx.QueryRow(ctx, "SELECT ($1::numeric-$2::numeric)::text", out.Budget.Total, out.Matched.NetAmount).Scan(&variance); err != nil {
			return out, err
		}
		out.BudgetVariance = &variance
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}

func (s *Service) BudgetComparisonEntries(ctx context.Context, a core.Actor, pid string, f BudgetComparisonFilter) (BudgetComparisonEntries, error) {
	out := BudgetComparisonEntries{Items: []finance.BudgetCostEntry{}}
	if f.State == "" {
		f.State = "incurred"
	}
	if f.State != "incurred" && f.State != "draft" {
		return out, core.Invalid("state", "请选择已发生费用或未确认费用")
	}
	if f.Limit < 1 || f.Limit > 100 {
		return out, core.Invalid("limit", "每页1至100项")
	}
	tx, err := s.deliverableSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	basis, windows, err := comparisonBasis(ctx, tx, a, pid, f.Currency)
	if err != nil {
		return out, err
	}
	if f.BasisHash != basis.BasisHash {
		return out, core.Fail(409, "BUDGET_COMPARISON_CHANGED", "项目或批准预算已变化，请重新查询对比依据")
	}
	mode := f.Line
	var index *int
	if f.Line != "" && f.Line != "outside" {
		i, e := strconv.Atoi(f.Line)
		var lines []BudgetLine
		if e != nil || json.Unmarshal(windows, &lines) != nil || i < 0 || i >= len(lines) || strconv.Itoa(i) != f.Line {
			return out, core.Invalid("line", "预算明细序号无效")
		}
		index, mode = &i, "line"
	}
	binding := core.CursorBinding(a, "budget-comparison", pid, f.Currency, f.BasisHash, f.Line, f.State)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		cursor, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before, beforeID = &cursor.UpdatedAt, &cursor.ID
	}
	out.Items, err = finance.ProjectBudgetEntriesInTx(ctx, tx, a, pid, f.Currency, windows, mode, index, f.State, before, beforeID, f.Limit+1)
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	out.BasisHash, out.AsOf = basis.BasisHash, basis.AsOf
	return out, platform.DatabaseError(tx.Commit(ctx))
}
