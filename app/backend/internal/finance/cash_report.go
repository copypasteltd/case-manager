package finance

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CashReport(ctx context.Context, a core.Actor, f PaymentFilter) (CashReport, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CashReport{}, err
	}
	defer tx.Rollback(ctx)
	out, err := cashReportInTx(ctx, tx, a, f)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

const cashReportPredicate = ` WHERE p.org_id=$1 AND can_read_payment(p.org_id,$2,p.id,$3,$4) AND can_access(p.org_id,$2,'report.read','payment',p.id,p.project_id,p.creator_id,p.classification,$3,$4)
        AND r.paid_on>=$5::date AND r.paid_on<$6::date AND ($7='' OR r.currency=$7) AND ($8='' OR r.category=$8) AND r.direction IN ('outgoing','refund')
        AND (($9::uuid IS NULL AND NOT $10) OR EXISTS(SELECT 1 FROM payment_allocations a JOIN payment_allocation_shares sh ON sh.org_id=a.org_id AND sh.allocation_id=a.id WHERE a.org_id=p.org_id AND a.payment_id=p.id AND sh.project_id IS NOT DISTINCT FROM $9::uuid) OR contract_cash_return_share(p.org_id,COALESCE(p.reverses_id,p.id),$9::uuid)>0)
        AND ($11::uuid IS NULL OR r.counterparty_id=$11) AND ($12::uuid IS NULL OR p.creator_id=$12)
        AND ($13='' OR position(lower($13) in lower(r.title))>0 OR p.source_key=$13) `

func cashReportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, f PaymentFilter) (CashReport, error) {
	return cashFilteredReportInTx(ctx, tx, a, f, nil, nil)
}

func cashFilteredReportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, f PaymentFilter, supplier, owner *string) (CashReport, error) {
	out := CashReport{Kind: "cash", Timezone: "Asia/Hong_Kong", FXCoverage: "unconfigured", Totals: []CashTotal{}}
	if err := validatePaymentFilter(f); err != nil {
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
		return out, core.Invalid("from", "请明确现金报表起始日期")
	}
	to, e := time.Parse(time.DateOnly, f.To)
	if e != nil || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, core.Invalid("to", "现金期间为1至366天，结束日不包含在期间内")
	}
	out.From = f.From
	out.To = f.To
	predicate := cashReportPredicate
	args := []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", f.From, f.To, f.Currency, f.Category, f.ProjectID, f.CompanyOnly, supplier, owner, f.Query}
	rows, err := tx.Query(ctx, `WITH facts AS (SELECT r.currency,r.direction,p.record_kind,
        `+reportCashContribution+` contribution,
        COALESCE((SELECT sum(a.effect*COALESCE(a.refund_cash_amount,a.amount)) FROM payment_allocations a WHERE a.org_id=p.org_id AND a.payment_id=p.id),0)+contract_cash_return_share(p.org_id,COALESCE(p.reverses_id,p.id),p.project_id)*(CASE p.record_kind WHEN 'reversal' THEN -1 ELSE 1 END) assigned `+paymentJoin+predicate+` AND p.state IN ('recorded','reversed'))
        SELECT currency,round(COALESCE(sum(contribution) FILTER(WHERE direction='outgoing' AND record_kind='payment'),0),6)::text,
        round(-COALESCE(sum(contribution) FILTER(WHERE direction='refund' AND record_kind='payment'),0),6)::text,
        round(COALESCE(sum(contribution) FILTER(WHERE record_kind='reversal'),0),6)::text,round(sum(contribution),6)::text,
        round(CASE WHEN $9::uuid IS NULL AND NOT $10 THEN sum(contribution-assigned) ELSE 0 END,6)::text,count(*) FROM facts GROUP BY currency ORDER BY currency`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v CashTotal
		if err = rows.Scan(&v.Currency, &v.OutgoingAmount, &v.RefundAmount, &v.CorrectionAmount, &v.NetAmount, &v.UnallocatedAmount, &v.EntryCount); err != nil {
			rows.Close()
			return out, err
		}
		out.Totals = append(out.Totals, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT count(*)"+paymentJoin+predicate+" AND p.state='draft'", args...).Scan(&out.UnconfirmedCount); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	return out, nil
}
