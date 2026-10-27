package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
	"time"
)

// A single repeatable-read snapshot binds totals and unconfirmed counts. Links
// never enter the aggregate join; each canonical entry contributes once.
func (s *Service) Report(ctx context.Context, a core.Actor, kind string, f Filter) (Report, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	out, err := incurredReportInTx(ctx, tx, a, kind, f)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

const incurredReadPredicate = ` WHERE c.org_id=$1 AND can_read_cost(c.org_id,$2,c.id,r.id,$3,$4) AND can_access(c.org_id,$2,'report.read','cost',c.id,c.project_id,c.creator_id,c.classification,$3,$4) `

const incurredReportPredicate = incurredReadPredicate + `
        AND r.occurred_on>=$5::date AND r.occurred_on<$6::date AND ($7='' OR r.currency=$7) AND ($8='' OR r.category=$8)
        AND (($9::uuid IS NULL AND NOT $10) OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id IS NOT DISTINCT FROM $9::uuid))
        AND ($11::uuid IS NULL OR r.counterparty_id=$11) AND ($12::uuid IS NULL OR c.creator_id=$12)
        AND ($13='' OR position(lower($13) in lower(r.title))>0 OR c.source_key=$13) `

func incurredReportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, kind string, f Filter) (Report, error) {
	return incurredFilteredReportInTx(ctx, tx, a, kind, f, nil, nil)
}

func incurredFilteredReportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, kind string, f Filter, supplier, owner *string) (Report, error) {
	out := Report{Kind: "incurred", Timezone: "Asia/Hong_Kong", FXCoverage: "unconfigured", Totals: []Total{}}
	if kind != "" && kind != "incurred" {
		return out, core.Fail(409, "REPORT_KIND_NOT_AVAILABLE", "此接口提供发生支出；现金请使用现金报表，预测及摊销仍待接入")
	}
	if err := validateFilter(f); err != nil {
		return out, err
	}
	var err error
	if err = authz.Require(ctx, tx, a, "report.read", authz.Resource{Type: "report", ProjectID: f.ProjectID, OwnerID: &a.UserID, Classification: "internal"}); err != nil {
		return out, err
	}
	var current bool
	if err = tx.QueryRow(ctx, "SELECT o.authorization_version=$3 AND u.auth_version=$4 AND u.status='active' FROM organizations o JOIN users u ON u.org_id=o.id AND u.id=$2 WHERE o.id=$1", a.OrgID, a.UserID, a.OrgAuthVersion, a.AuthVersion).Scan(&current); err != nil {
		return out, err
	}
	if !current {
		return out, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	if f.From == "" && f.To == "" {
		if err = tx.QueryRow(ctx, "SELECT to_char(date_trunc('month',clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong'),'YYYY-MM-DD'),to_char(date_trunc('month',clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')+interval '1 month','YYYY-MM-DD')").Scan(&f.From, &f.To); err != nil {
			return out, err
		}
	}
	from, e := time.Parse(time.DateOnly, f.From)
	if e != nil {
		return out, core.Invalid("from", "请明确起始日期")
	}
	to, e := time.Parse(time.DateOnly, f.To)
	if e != nil || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, core.Invalid("to", "结束日不包含在期间内，查询期间须为1至366天")
	}
	out.From = f.From
	out.To = f.To
	predicate := incurredReportPredicate
	args := []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", f.From, f.To, f.Currency, f.Category, f.ProjectID, f.CompanyOnly, supplier, owner, f.Query}
	rows, err := tx.Query(ctx, `SELECT currency,COALESCE(sum(share) FILTER(WHERE entry_kind='expense'),0)::text,COALESCE(sum(share) FILTER(WHERE entry_kind='reversal'),0)::text,
        sum(CASE entry_kind WHEN 'reversal' THEN -share ELSE share END)::text,count(*) FROM (
        SELECT c.entry_kind,r.currency,CASE WHEN $9::uuid IS NULL AND NOT $10 THEN r.amount ELSE (SELECT sum(a.amount) FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id IS NOT DISTINCT FROM $9::uuid) END share `+currentJoin+predicate+` AND c.state IN ('confirmed','reversed')) entries GROUP BY currency ORDER BY currency`, args...)
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
	if err = tx.QueryRow(ctx, "SELECT count(*)"+currentJoin+predicate+" AND c.state='draft'", args...).Scan(&out.UnconfirmedCount); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	return out, nil
}
