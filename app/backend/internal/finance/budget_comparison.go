package finance

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// All amounts are project allocation shares, in one original currency. An
// inaccessible cost contributes neither an amount nor an existence/count signal.
type BudgetMetrics struct {
	ExpenseAmount    string `json:"expense_amount"`
	ReversalAmount   string `json:"reversal_amount"`
	NetAmount        string `json:"net_amount"`
	EntryCount       int64  `json:"entry_count"`
	UnconfirmedCount int64  `json:"unconfirmed_count"`
}

type BudgetComparedLine struct {
	Index        int    `json:"index"`
	Category     string `json:"category"`
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	BudgetAmount string `json:"budget_amount"`
	Variance     string `json:"variance"`
	OverBudget   bool   `json:"over_budget"`
	BudgetMetrics
}

type BudgetAnalysis struct {
	Lines    []BudgetComparedLine `json:"lines"`
	Matched  BudgetMetrics        `json:"matched"`
	Outside  BudgetMetrics        `json:"outside"`
	Lifetime BudgetMetrics        `json:"lifetime"`
}

type BudgetCostEntry struct {
	ID           string    `json:"id"`
	RevisionID   string    `json:"revision_id"`
	Version      int64     `json:"version"`
	Title        string    `json:"title"`
	Category     string    `json:"category"`
	OccurredOn   *string   `json:"occurred_on"`
	Currency     string    `json:"currency"`
	EntryKind    string    `json:"entry_kind"`
	State        string    `json:"state"`
	ShareAmount  string    `json:"share_amount"`
	SignedAmount string    `json:"signed_amount"`
	LineIndex    *int      `json:"line_index"`
	CreatedAt    time.Time `json:"created_at"`
}

// Same canonical read predicate as incurred reports. No cost_links join: one
// economic fact contributes once, regardless of its purchase/claim/subscription
// links. Original reversed expenses remain positive on their occurrence date;
// reversal entries subtract on their own occurrence date. Drafts are counted
// separately, never treated as incurred expenditure.
const budgetFacts = `WITH windows AS (
 SELECT (ordinality-1)::int line_index,value FROM jsonb_array_elements($7::jsonb) WITH ORDINALITY
), facts AS (
 SELECT c.id,c.current_revision_id,c.version,r.payload->>'title' title,r.category,r.occurred_on,r.currency,
 c.entry_kind,c.state,c.created_at,
 (SELECT sum(a.amount) FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id=$5::uuid) share,
 (SELECT w.line_index FROM windows w WHERE w.value->>'category'=r.category
  AND r.occurred_on>=(w.value->>'period_start')::date AND r.occurred_on<=(w.value->>'period_end')::date) line_index
 ` + currentJoin + incurredReadPredicate + ` AND r.currency=$6
 AND EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id=$5::uuid)
 AND c.state IN ('draft','confirmed','reversed')
), valued AS (
 SELECT *,CASE WHEN state='draft' THEN 0 WHEN entry_kind='reversal' THEN -share ELSE share END signed_amount FROM facts
) `

const budgetAggregate = `jsonb_build_object(
 'expense_amount',COALESCE(sum(share) FILTER(WHERE state<>'draft' AND entry_kind='expense'),0)::text,
 'reversal_amount',COALESCE(sum(share) FILTER(WHERE state<>'draft' AND entry_kind='reversal'),0)::text,
 'net_amount',COALESCE(sum(signed_amount),0)::text,
 'entry_count',count(*) FILTER(WHERE state<>'draft'),
 'unconfirmed_count',count(*) FILTER(WHERE state='draft'))`

// The caller owns a repeatable-read, authorization-version-checked transaction
// and supplies only the current approved, non-overlapping budget line snapshot.
func ProjectBudgetAnalysisInTx(ctx context.Context, db platform.DBTX, a core.Actor, pid, currency string, windows []byte) (BudgetAnalysis, error) {
	var out BudgetAnalysis
	var raw []byte
	err := db.QueryRow(ctx, budgetFacts+`SELECT jsonb_build_object(
 'lines',COALESCE((SELECT jsonb_agg(w.value || jsonb_build_object(
  'index',w.line_index,'budget_amount',w.value->>'amount',
  'variance',((w.value->>'amount')::numeric-(m.metrics->>'net_amount')::numeric)::text,
  'over_budget',(m.metrics->>'net_amount')::numeric>(w.value->>'amount')::numeric
 ) || m.metrics ORDER BY w.line_index) FROM windows w
 CROSS JOIN LATERAL (SELECT `+budgetAggregate+` metrics FROM valued WHERE line_index=w.line_index) m),'[]'::jsonb),
 'matched',(SELECT `+budgetAggregate+` FROM valued WHERE line_index IS NOT NULL),
 'outside',(SELECT `+budgetAggregate+` FROM valued WHERE line_index IS NULL),
 'lifetime',(SELECT `+budgetAggregate+` FROM valued))`, a.OrgID, a.UserID, a.Channel, a.RunID != "", pid, currency, string(windows)).Scan(&raw)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func ProjectBudgetEntriesInTx(ctx context.Context, db platform.DBTX, a core.Actor, pid, currency string, windows []byte, line string, index *int, state string, before *time.Time, beforeID *string, limit int) ([]BudgetCostEntry, error) {
	out := []BudgetCostEntry{}
	rows, err := db.Query(ctx, budgetFacts+`SELECT id,current_revision_id,version,title,category,occurred_on::text,currency,entry_kind,state,share::text,signed_amount::text,line_index,created_at FROM valued
 WHERE ($8='' OR ($8='outside' AND line_index IS NULL) OR ($8='line' AND line_index=$9::int))
 AND (($10='draft' AND state='draft') OR ($10='incurred' AND state<>'draft'))
 AND ($11::timestamptz IS NULL OR (created_at,id)<($11,$12::uuid)) ORDER BY created_at DESC,id DESC LIMIT $13`,
		a.OrgID, a.UserID, a.Channel, a.RunID != "", pid, currency, string(windows), line, index, state, before, beforeID, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry BudgetCostEntry
		if err = rows.Scan(&entry.ID, &entry.RevisionID, &entry.Version, &entry.Title, &entry.Category, &entry.OccurredOn, &entry.Currency, &entry.EntryKind, &entry.State, &entry.ShareAmount, &entry.SignedAmount, &entry.LineIndex, &entry.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}
