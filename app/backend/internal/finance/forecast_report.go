package finance

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// The report sums stored forecasts. Calendar coverage is a separate check;
// neither a missing period nor an unknown price is silently treated as zero.
func (s *Service) ForecastReport(ctx context.Context, a core.Actor, f SubscriptionFilter) (ForecastReport, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ForecastReport{}, err
	}
	defer tx.Rollback(ctx)
	out, err := forecastReportInTx(ctx, tx, a, f)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

const forecastReportScope = ` WHERE s.org_id=$1 AND can_read_subscription(s.org_id,$2,s.id,$3,$4) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,$4)
        AND ($5='' OR r.category=$5) AND ($6::uuid IS NULL OR s.project_id=$6) AND (NOT $7 OR s.project_id IS NULL) AND ($8::uuid IS NULL OR r.supplier_id=$8) AND ($9::uuid IS NULL OR r.owner_id=$9)
        AND ($10='' OR position(lower($10) in lower(r.product))>0 OR s.source_key=$10) `

func forecastReportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, f SubscriptionFilter) (ForecastReport, error) {
	out := ForecastReport{Kind: "forecast", CalendarBasis: "subscription_local_date", Timezones: []string{}, FXCoverage: "unconfigured", Totals: []ForecastTotal{}, CoverageComplete: true}
	if err := validateSubscriptionFilter(f); err != nil {
		return out, err
	}
	if f.State != "" || f.Cursor != "" {
		return out, core.Invalid("filter", "预测汇总不接受账期状态或分页游标，请通过账期列表钻取")
	}
	var err error
	if err = authz.Require(ctx, tx, a, "report.read", authz.Resource{Type: "report", ProjectID: f.ProjectID, OwnerID: &a.UserID, Classification: "internal"}); err != nil {
		return out, err
	}
	var current bool
	if err = tx.QueryRow(ctx, `SELECT o.authorization_version=$3 AND u.auth_version=$4 AND u.status='active' FROM organizations o JOIN users u ON u.org_id=o.id AND u.id=$2 WHERE o.id=$1`, a.OrgID, a.UserID, a.OrgAuthVersion, a.AuthVersion).Scan(&current); err != nil {
		return out, err
	}
	if !current {
		return out, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	if f.From == "" && f.To == "" {
		zone, _ := time.LoadLocation("Asia/Hong_Kong")
		now := out.AsOf.In(zone)
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		f.From = start.Format(time.DateOnly)
		f.To = start.AddDate(0, 1, 0).Format(time.DateOnly)
	}
	from, e := civilDate(f.From)
	if e != nil {
		return out, core.Invalid("from", "请明确预测起始日期")
	}
	to, e := civilDate(f.To)
	if e != nil || !to.After(from) || (to.Unix()-from.Unix())/86400 > 366 {
		return out, core.Invalid("to", "预测期间为1至366个自然日，结束日不含在内")
	}
	out.From = f.From
	out.To = f.To
	args := []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", f.From, f.To, f.Currency, f.Category, f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, f.Query}
	predicate := ` WHERE p.org_id=$1 AND can_read_subscription(p.org_id,$2,s.id,$3,$4) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,$4)
        AND p.start_date>=$5::date AND p.start_date<$6::date AND ($7='' OR r.currency=$7) AND ($8='' OR r.category=$8) AND ($9::uuid IS NULL OR s.project_id=$9) AND (NOT $10 OR s.project_id IS NULL)
        AND ($11::uuid IS NULL OR r.supplier_id=$11) AND ($12::uuid IS NULL OR r.owner_id=$12) AND ($13='' OR position(lower($13) in lower(cfg.product))>0 OR s.source_key=$13)
        AND p.state NOT IN ('waived','void') AND subscription_charge_eligible(s.org_id,s.id,p.start_date) `
	rows, err := tx.Query(ctx, `WITH facts AS(SELECT r.currency,r.amount,p.state,p.start_date,r.timezone,
        p.state='confirmed' AND p.review_fingerprint=subscription_actual_fingerprint(p.org_id,p.id) review_current,
        p.invoice_missing OR (p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(p.org_id,p.id)) OR actual.reversed>0 OR (actual.n>0 AND actual.amount<>r.amount) anomaly,
        actual.n>0 has_actual `+subscriptionPeriodJoin+` JOIN subscription_revisions cfg ON cfg.org_id=s.org_id AND cfg.id=s.current_revision_id
        LEFT JOIN LATERAL(SELECT count(*) n,COALESCE(sum(v.amount) FILTER(WHERE c.state='confirmed'),0) amount,count(*) FILTER(WHERE c.state='reversed') reversed FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions v ON v.org_id=c.org_id AND v.id=c.current_revision_id WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id) actual ON true `+predicate+`)
        SELECT currency,round(sum(amount),6)::text,count(*),count(*) FILTER(WHERE (state='forecast' AND (start_date<=(clock_timestamp() AT TIME ZONE timezone)::date OR has_actual)) OR (state='confirmed' AND NOT review_current)),count(*) FILTER(WHERE review_current),count(*) FILTER(WHERE anomaly) FROM facts GROUP BY currency ORDER BY currency`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var total ForecastTotal
		if err = rows.Scan(&total.Currency, &total.ExpectedAmount, &total.PeriodCount, &total.AwaitingCount, &total.ReviewedCount, &total.AnomalyCount); err != nil {
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
	scope := forecastReportScope
	scopeArgs := []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", f.Category, f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, f.Query}
	if err = tx.QueryRow(ctx, "SELECT count(*)"+subscriptionJoin+scope+" AND s.state='draft'", scopeArgs...).Scan(&out.DraftCount); err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT "+subscriptionColumns+subscriptionJoin+scope+` AND s.state<>'draft' AND EXISTS(SELECT 1 FROM subscription_events e WHERE e.org_id=s.org_id AND e.subscription_id=s.id AND e.kind='activated') ORDER BY s.id LIMIT 1001`, scopeArgs...)
	if err != nil {
		return out, err
	}
	subs := []Subscription{}
	for rows.Next() {
		v, e := scanSubscription(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		subs = append(subs, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(subs) > 1000 {
		out.CoverageLimitReached = true
		subs = subs[:1000]
	}
	for _, sub := range subs {
		calendar, e := subscriptionCalendar(sub.SubscriptionContent)
		if e != nil {
			out.IncompleteSubscriptionCount++
			continue
		}
		windows, e := calendar.Windows(f.From, f.To)
		if e != nil {
			return out, e
		}
		out.Timezones = append(out.Timezones, sub.Timezone)
		for _, w := range windows {
			var eligible, exists bool
			if err = tx.QueryRow(ctx, `SELECT subscription_charge_eligible($1,$2,$3::date),EXISTS(SELECT 1 FROM subscription_periods WHERE org_id=$1 AND subscription_id=$2 AND start_date=$3::date AND end_date=$4::date AND charge_type='recurring')`, a.OrgID, sub.ID, w.StartDate, w.EndDate).Scan(&eligible, &exists); err != nil {
				return out, err
			}
			if !eligible || exists {
				continue
			}
			price := subscriptionPriceAt(sub.Prices, w.StartDate)
			if price != nil && f.Currency != "" && price.Currency != f.Currency {
				continue
			}
			out.MissingPeriodCount++
			if price == nil {
				out.MissingPriceCount++
			}
		}
	}
	slices.Sort(out.Timezones)
	out.Timezones = slices.Compact(out.Timezones)
	out.CoverageComplete = !out.CoverageLimitReached && out.IncompleteSubscriptionCount == 0 && out.MissingPeriodCount == 0 && out.DraftCount == 0
	return out, nil
}
