package finance

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

type ProjectIncurred struct {
	Available        bool    `json:"available"`
	Totals           []Total `json:"totals"`
	UnconfirmedCount *int64  `json:"unconfirmed_count"`
}

// ProjectIncurredInTx reads lifetime incurred costs, without converting currencies
// or treating approvals, forecasts or payments as expenses. The caller must hold
// a consistent, authorization-version-checked transaction. It shares the exact
// per-entry authorization predicate with the incurred report.
func ProjectIncurredInTx(ctx context.Context, db platform.DBTX, a core.Actor, pid string) (ProjectIncurred, error) {
	out := ProjectIncurred{Totals: []Total{}}
	err := db.QueryRow(ctx, `SELECT can_access($1,$2,'report.read','report',NULL,$3,$2,'internal',$4,$5)`, a.OrgID, a.UserID, pid, a.Channel, a.RunID != "").Scan(&out.Available)
	if err != nil || !out.Available {
		return out, err
	}
	args := []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", pid}
	predicate := incurredReadPredicate + ` AND EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id=$5::uuid) `
	rows, err := db.Query(ctx, `SELECT currency,COALESCE(sum(share) FILTER(WHERE entry_kind='expense'),0)::text,
		COALESCE(sum(share) FILTER(WHERE entry_kind='reversal'),0)::text,
		sum(CASE entry_kind WHEN 'reversal' THEN -share ELSE share END)::text,count(*) FROM (
		SELECT c.entry_kind,r.currency,(SELECT sum(a.amount) FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id=$5::uuid) share `+
		currentJoin+predicate+` AND c.state IN ('confirmed','reversed')) entries GROUP BY currency ORDER BY currency`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var total Total
		if err = rows.Scan(&total.Currency, &total.ExpenseAmount, &total.ReversalAmount, &total.NetAmount, &total.EntryCount); err != nil {
			rows.Close()
			return out, err
		}
		out.Totals = append(out.Totals, total)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	var count int64
	if err = db.QueryRow(ctx, "SELECT count(*)"+currentJoin+predicate+" AND c.state='draft'", args...).Scan(&count); err != nil {
		return out, err
	}
	out.UnconfirmedCount = &count
	return out, nil
}
