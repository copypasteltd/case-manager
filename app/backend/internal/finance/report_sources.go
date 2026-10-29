package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const reportCostShare = `CASE WHEN $9::uuid IS NULL AND NOT $10 THEN r.amount ELSE (SELECT sum(a.amount) FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id IS NOT DISTINCT FROM $9::uuid) END`

func reportArguments(a core.Actor, f ReportQuery) []any {
	return []any{a.OrgID, a.UserID, a.Channel, a.RunID != "", f.From, f.To, f.Currency, f.Category, f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, f.Query}
}

func reportSourceStatement(f ReportQuery) (string, string, error) {
	var query, alias string
	switch f.Kind {
	case "amortization":
		alias = "f"
		query = amortizationSource
	case "incurred":
		alias = "c"
		query = `SELECT 'cost',c.id,c.id,r.id,c.version,r.title,r.occurred_on::text,NULL::text,c.state,c.entry_kind,r.currency,r.amount::text,
            CASE WHEN c.state='draft' THEN NULL ELSE ((` + reportCostShare + `)*(CASE c.entry_kind WHEN 'reversal' THEN -1 ELSE 1 END))::text END,
            r.category,r.counterparty_id,c.creator_id,c.project_id,c.created_at,'Asia/Hong_Kong' ` + currentJoin + incurredReportPredicate
	case "cash":
		alias = "p"
		query = `SELECT 'payment',p.id,p.id,r.id,p.version,r.title,r.paid_on::text,NULL::text,p.state,p.record_kind,r.currency,r.amount::text,
            CASE WHEN p.state='draft' THEN NULL ELSE round((` + reportCashContribution + `),6)::text END,
            r.category,r.counterparty_id,p.creator_id,p.project_id,p.created_at,'Asia/Hong_Kong' ` + paymentJoin + cashReportPredicate
	case "forecast":
		alias = "p"
		query = `SELECT 'subscription_period',p.id,s.id,r.id,p.version,cfg.product,p.start_date::text,p.end_date::text,p.state,'forecast',r.currency,r.amount::text,r.amount::text,
            r.category,r.supplier_id,r.owner_id,s.project_id,p.created_at,r.timezone ` + subscriptionPeriodJoin + ` JOIN subscription_revisions cfg ON cfg.org_id=s.org_id AND cfg.id=s.current_revision_id ` + reportForecastPredicate
	default:
		return "", "", core.Invalid("kind", "报表类型无效")
	}
	return query, alias, nil
}

func reportSources(ctx context.Context, tx pgx.Tx, a core.Actor, f ReportQuery, at *time.Time, before *string, limit int) ([]ReportSource, error) {
	query, alias, err := reportSourceStatement(f)
	if err != nil {
		return nil, err
	}
	query += fmt.Sprintf(` AND ($14::timestamptz IS NULL OR (%s.created_at,%s.id)<($14,$15::uuid)) ORDER BY %s.created_at DESC,%s.id DESC LIMIT $16`, alias, alias, alias, alias)
	args := append(reportArguments(a, f), at, before, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := []ReportSource{}
	for rows.Next() {
		var v ReportSource
		if err = rows.Scan(&v.SourceType, &v.ID, &v.ResourceID, &v.RevisionID, &v.Version, &v.Title, &v.From, &v.To, &v.State, &v.EntryKind, &v.Currency, &v.OriginalAmount, &v.ContributionAmount, &v.Category, &v.SupplierID, &v.OwnerID, &v.ProjectID, &v.CreatedAt, &v.Timezone); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = reportAmortizationEvidence(ctx, tx, a.OrgID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) FinancialReport(ctx context.Context, a core.Actor, f ReportQuery, cursor string, limit int) (ReportPage, error) {
	out := ReportPage{Filters: f, Scope: "authorized_records_only", Items: []ReportSource{}}
	if err := f.Validate(); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100条")
	}
	raw, _ := json.Marshal(f)
	bound := core.CursorBinding(a, "financial-report", core.Digest(raw))
	var at *time.Time
	var before *string
	if cursor != "" {
		position, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at, before = &position.UpdatedAt, &position.ID
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = reportAuthority(ctx, tx, a, f, false); err != nil {
		return out, err
	}
	if out.Summary, err = reportSummary(ctx, tx, a, f); err != nil {
		return out, err
	}
	if out.Items, err = reportSources(ctx, tx, a, f, at, before, limit+1); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		value := s.Pager.Encode(platform.Cursor{UpdatedAt: last.CreatedAt, ID: last.ID, Binding: bound})
		out.NextCursor = &value
	}
	if err = applyReportValuation(ctx, tx, a, &out); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

const reportForecastPredicate = ` WHERE p.org_id=$1 AND can_read_subscription(p.org_id,$2,s.id,$3,$4) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,$4)
        AND p.start_date>=$5::date AND p.start_date<$6::date AND ($7='' OR r.currency=$7) AND ($8='' OR r.category=$8) AND ($9::uuid IS NULL OR s.project_id=$9) AND (NOT $10 OR s.project_id IS NULL)
        AND ($11::uuid IS NULL OR r.supplier_id=$11) AND ($12::uuid IS NULL OR r.owner_id=$12) AND ($13='' OR position(lower($13) in lower(cfg.product))>0 OR s.source_key=$13)
        AND p.state NOT IN ('waived','void') AND subscription_charge_eligible(s.org_id,s.id,p.start_date) `

const reportCashContribution = `CASE WHEN $9::uuid IS NULL AND NOT $10 THEN r.amount*(CASE r.direction WHEN 'refund' THEN -1 ELSE 1 END)*(CASE p.record_kind WHEN 'reversal' THEN -1 ELSE 1 END)
        ELSE COALESCE((SELECT sum(a.effect*sh.amount) FROM payment_allocations a JOIN payment_allocation_shares sh ON sh.org_id=a.org_id AND sh.allocation_id=a.id WHERE a.org_id=p.org_id AND a.payment_id=p.id AND sh.project_id IS NOT DISTINCT FROM $9::uuid),0)+contract_cash_return_share(p.org_id,COALESCE(p.reverses_id,p.id),$9::uuid)*(CASE p.record_kind WHEN 'reversal' THEN -1 ELSE 1 END) END`
