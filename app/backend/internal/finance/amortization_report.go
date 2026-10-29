package finance

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Current plans are a separate versioned allocation view. If the underlying
// expense is reversed, retain earlier recognized months and negate each on the
// actual reversal date. Future, unrecognized months never become a fact.
const amortizationSource = `SELECT 'cost_amortization',f.id,c.id,r.id,v.version,r.title,f.business_date::text,e.service_to::text,c.state,f.kind,r.currency,e.amount::text,
 (f.effect*(CASE WHEN $9::uuid IS NULL AND NOT $10 THEN e.amount ELSE coalesce((SELECT sum(sh.amount) FROM cost_amortization_shares sh WHERE sh.org_id=e.org_id AND sh.entry_id=e.id AND sh.project_id IS NOT DISTINCT FROM $9::uuid),0) END))::text,
 r.category,r.counterparty_id,c.creator_id,c.project_id,v.created_at,'Asia/Hong_Kong'
 FROM cost_amortization_versions v JOIN cost_amortization_entries e ON e.org_id=v.org_id AND e.plan_id=v.id
 JOIN actual_cost_entries c ON c.org_id=v.org_id AND c.id=v.cost_id
 JOIN cost_revisions r ON r.org_id=c.org_id AND r.cost_id=c.id AND r.id=(v.content::jsonb->>'cost_revision_id')::uuid
 LEFT JOIN actual_cost_entries rc ON rc.org_id=c.org_id AND rc.reverses_id=c.id
 LEFT JOIN cost_revisions rr ON rr.org_id=rc.org_id AND rr.id=rc.current_revision_id
 CROSS JOIN LATERAL (
  SELECT e.id,e.recognized_on business_date,1 effect,'amortization'::text kind,v.created_at
   WHERE rr.occurred_on IS NULL OR e.recognized_on<rr.occurred_on
  UNION ALL SELECT e.reversal_id,rr.occurred_on,-1,'reversal'::text,v.created_at
   WHERE rr.occurred_on IS NOT NULL AND e.recognized_on<rr.occurred_on
 ) f
 WHERE v.org_id=$1 AND v.state='active' AND c.entry_kind='expense' AND c.state IN ('confirmed','reversed')
 AND v.version=(SELECT max(latest.version) FROM cost_amortization_versions latest WHERE latest.org_id=v.org_id AND latest.cost_id=v.cost_id)
 AND can_read_cost(c.org_id,$2,c.id,r.id,$3,$4) AND can_access(c.org_id,$2,'report.read','cost',c.id,c.project_id,c.creator_id,c.classification,$3,$4)
 AND f.business_date>=$5::date AND f.business_date<$6::date AND ($7='' OR r.currency=$7) AND ($8='' OR r.category=$8)
 AND ($9::uuid IS NULL AND NOT $10 OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id IS NOT DISTINCT FROM $9::uuid))
 AND ($11::uuid IS NULL OR r.counterparty_id=$11) AND ($12::uuid IS NULL OR c.creator_id=$12)
 AND ($13='' OR position(lower($13) in lower(r.title))>0 OR c.source_key=$13)`

type AmortizationReport struct {
	Report
	Basis string `json:"basis"`
}

func amortizationReport(ctx context.Context, tx pgx.Tx, a core.Actor, f ReportQuery) (AmortizationReport, error) {
	out := AmortizationReport{Report: Report{Kind: "amortization", From: f.From, To: f.To, Timezone: "Asia/Hong_Kong", FXCoverage: "unconfigured", Totals: []Total{}}, Basis: "latest_active_schedules"}
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `WITH `+valuationFacts+` AS (`+amortizationSource+`)
 SELECT currency,coalesce(sum(contribution_amount::numeric) FILTER(WHERE entry_kind='amortization'),0)::text,
 coalesce(-sum(contribution_amount::numeric) FILTER(WHERE entry_kind='reversal'),0)::text,
 coalesce(sum(contribution_amount::numeric),0)::text,count(*) FROM facts GROUP BY currency ORDER BY currency`, reportArguments(a, f)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Total
		if err = rows.Scan(&v.Currency, &v.ExpenseAmount, &v.ReversalAmount, &v.NetAmount, &v.EntryCount); err != nil {
			return out, err
		}
		out.Totals = append(out.Totals, v)
	}
	return out, rows.Err()
}
func reportAmortizationEvidence(ctx context.Context, tx pgx.Tx, org string, items []ReportSource) error {
	ids := []string{}
	for _, v := range items {
		if v.SourceType == "cost_amortization" {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.reversal_id,v.id,v.version,v.content_sha256,e.seq,v.content::jsonb->>'method',e.service_from::text,e.service_to::text,e.recognized_on::text
 FROM cost_amortization_entries e JOIN cost_amortization_versions v ON v.org_id=e.org_id AND v.id=e.plan_id
 WHERE e.org_id=$1 AND (e.id=ANY($2::uuid[]) OR e.reversal_id=ANY($2::uuid[]))`, org, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[string]AmortizationEvidence{}
	for rows.Next() {
		var id, reversal string
		var v AmortizationEvidence
		if err = rows.Scan(&id, &reversal, &v.PlanID, &v.PlanVersion, &v.PlanSHA256, &v.Sequence, &v.Method, &v.ServiceFrom, &v.ServiceTo, &v.OriginalDate); err != nil {
			return err
		}
		found[id], found[reversal] = v, v
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if items[i].SourceType != "cost_amortization" {
			continue
		}
		v, ok := found[items[i].ID]
		if !ok {
			return core.Fail(500, "AMORTIZATION_SOURCE_MISSING", "摊销来源版本缺失")
		}
		items[i].Amortization = &v
	}
	return nil
}
