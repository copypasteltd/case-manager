package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const subscriptionPeriodJoin = ` FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.period_id=p.id AND r.id=p.current_revision_id JOIN subscriptions s ON s.org_id=p.org_id AND s.id=p.subscription_id `
const subscriptionPeriodColumns = `p.id,p.subscription_id,p.version,p.current_revision_id,r.content_hash,p.start_date::text,p.end_date::text,p.charge_type,r.configuration_revision_id,r.price_id,r.currency,r.amount::text,r.timezone,r.category,r.owner_id,r.supplier_id,r.supplier_revision_id,p.state,p.review_reason,p.created_at,p.updated_at`

func scanSubscriptionPeriod(row pgx.Row) (SubscriptionPeriod, error) {
	var p SubscriptionPeriod
	err := row.Scan(&p.ID, &p.SubscriptionID, &p.Version, &p.CurrentRevisionID, &p.ContentHash, &p.StartDate, &p.EndDate, &p.ChargeType, &p.ConfigurationRevisionID, &p.PriceID, &p.Currency, &p.Amount, &p.Timezone, &p.Category, &p.OwnerID, &p.Supplier.PartyID, &p.Supplier.RevisionID, &p.RecordedState, &p.ReviewReason, &p.CreatedAt, &p.UpdatedAt)
	p.State = p.RecordedState
	return p, platform.DatabaseError(err)
}
func subscriptionPeriodMetadata(ctx context.Context, db platform.DBTX, org, id string) (SubscriptionPeriod, error) {
	if !core.ValidID(id) {
		return SubscriptionPeriod{}, core.Missing()
	}
	return scanSubscriptionPeriod(db.QueryRow(ctx, "SELECT "+subscriptionPeriodColumns+subscriptionPeriodJoin+"WHERE p.org_id=$1 AND p.id=$2", org, id))
}
func presentSubscriptionPeriod(ctx context.Context, db platform.DBTX, a core.Actor, p SubscriptionPeriod) (SubscriptionPeriod, error) {
	sub, err := subscriptionMetadata(ctx, db, a.OrgID, p.SubscriptionID)
	if err != nil {
		return p, err
	}
	if err = subscriptionReadable(ctx, db, a, sub); err != nil {
		return p, err
	}
	p.Product = sub.Product
	var actual string
	var count, active, reversed, comparison int
	var invoiceMissing, eligible bool
	var fingerprint *string
	var currentFingerprint string
	err = db.QueryRow(ctx, `SELECT COALESCE(round(sum(v.amount) FILTER(WHERE c.state='confirmed'),6),0)::text,count(*)::int,count(*) FILTER(WHERE c.state='confirmed')::int,count(*) FILTER(WHERE c.state='reversed')::int,
        CASE WHEN COALESCE(sum(v.amount) FILTER(WHERE c.state='confirmed'),0)<$3::numeric THEN -1 WHEN COALESCE(sum(v.amount) FILTER(WHERE c.state='confirmed'),0)>$3::numeric THEN 1 ELSE 0 END
        FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions v ON v.org_id=c.org_id AND v.id=c.current_revision_id WHERE l.org_id=$1 AND l.source_type='subscription_period' AND l.source_id=$2`, a.OrgID, p.ID, p.Amount).Scan(&actual, &count, &active, &reversed, &comparison)
	if err != nil {
		return p, err
	}
	if err = db.QueryRow(ctx, `SELECT review_fingerprint,invoice_missing,subscription_actual_fingerprint(org_id,id),subscription_charge_eligible(org_id,subscription_id,start_date) FROM subscription_periods WHERE org_id=$1 AND id=$2`, a.OrgID, p.ID).Scan(&fingerprint, &invoiceMissing, &currentFingerprint, &eligible); err != nil {
		return p, err
	}
	p.CostIDs = []string{}
	p.Anomalies = []string{}
	rows, err := db.Query(ctx, "SELECT cost_id::text FROM cost_links WHERE org_id=$1 AND source_type='subscription_period' AND source_id=$2 ORDER BY cost_id", a.OrgID, p.ID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return p, err
		}
		p.CostIDs = append(p.CostIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	p.ActualAmount = nil
	if count > 0 {
		p.ActualAmount = &actual
	}
	p.ReviewCurrent = p.RecordedState == "confirmed" && fingerprint != nil && *fingerprint == currentFingerprint && active > 0
	p.ActualFingerprint = currentFingerprint
	if count > 0 && comparison < 0 {
		p.Anomalies = append(p.Anomalies, "partial_charge")
	}
	if count > 0 && comparison > 0 {
		p.Anomalies = append(p.Anomalies, "overcharge")
	}
	if reversed > 0 {
		p.Anomalies = append(p.Anomalies, "cost_reversed")
	}
	if invoiceMissing {
		p.Anomalies = append(p.Anomalies, "invoice_missing")
	}
	if count > 0 && !eligible {
		if sub.CancelEffectiveOn != nil && p.StartDate >= *sub.CancelEffectiveOn {
			p.Anomalies = append(p.Anomalies, "charge_after_cancellation")
		} else {
			p.Anomalies = append(p.Anomalies, "charge_in_paused_period")
		}
	}
	if p.RecordedState == "confirmed" && !p.ReviewCurrent {
		p.Anomalies = append(p.Anomalies, "review_stale")
	}
	today, err := subscriptionDate(ctx, db, p.Timezone)
	if err != nil {
		return p, err
	}
	if (p.RecordedState == "forecast" && (p.StartDate <= today || count > 0)) || (p.RecordedState == "confirmed" && !p.ReviewCurrent) {
		p.State = "awaiting_confirmation"
	}
	p.CanManage = a.RunID == "" && sub.State != "archived" && authz.Require(ctx, db, a, "subscription.manage", subscriptionResource(sub)) == nil && subscriptionProjectLocks(ctx, db, a, sub, nil, false, false) == nil
	return p, nil
}
func (s *Service) SubscriptionPeriod(ctx context.Context, a core.Actor, id string) (SubscriptionPeriod, error) {
	p, err := subscriptionPeriodMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return p, err
	}
	return presentSubscriptionPeriod(ctx, s.DB, a, p)
}
func (s *Service) SubscriptionPeriods(ctx context.Context, a core.Actor, subID string, f SubscriptionFilter) (core.Page[SubscriptionPeriod], error) {
	out := core.Page[SubscriptionPeriod]{Items: []SubscriptionPeriod{}}
	if err := validateSubscriptionFilter(f); err != nil {
		return out, err
	}
	if !slices.Contains([]string{"", "forecast", "awaiting_confirmation", "confirmed", "waived", "void"}, f.State) {
		return out, core.Invalid("state", "账期状态无效")
	}
	if subID != "" {
		if _, err := s.Subscription(ctx, a, subID); err != nil {
			return out, err
		}
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	bound := core.CursorBinding(a, "subscription-periods", subID, f.Query, f.State, f.Currency, f.Category, f.From, f.To, value(f.ProjectID), value(f.SupplierID), value(f.OwnerID), fmt.Sprint(f.CompanyOnly), fmt.Sprint(f.ForecastOnly))
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		v, e := s.Pager.Decode(f.Cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	// Projection is identical to presentSubscriptionPeriod; stale reviews and
	// due forecasts enter awaiting_confirmation without rewriting the journal.
	rows, err := s.DB.Query(ctx, "SELECT "+subscriptionPeriodColumns+subscriptionPeriodJoin+`WHERE p.org_id=$1 AND can_read_subscription(p.org_id,$2,s.id,$3,$4)
        AND ($5::uuid IS NULL OR s.id=$5) AND ($6='' OR r.currency=$6) AND ($7='' OR r.category=$7) AND ($8::date IS NULL OR p.start_date>=$8) AND ($9::date IS NULL OR p.start_date<$9)
        AND ($10::uuid IS NULL OR s.project_id=$10) AND (NOT $11 OR s.project_id IS NULL) AND ($12::uuid IS NULL OR r.supplier_id=$12) AND ($13::uuid IS NULL OR r.owner_id=$13)
        AND ($14='' OR $14=CASE WHEN (p.state='forecast' AND (p.start_date<=(clock_timestamp() AT TIME ZONE r.timezone)::date OR EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id))) OR (p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(p.org_id,p.id)) THEN 'awaiting_confirmation' ELSE p.state END)
        AND ($18='' OR EXISTS(SELECT 1 FROM subscription_revisions cfg WHERE cfg.org_id=s.org_id AND cfg.id=s.current_revision_id AND strpos(lower(cfg.payload->>'product'),lower($18))>0))
        AND (NOT $19 OR (p.state NOT IN ('waived','void') AND subscription_charge_eligible(s.org_id,s.id,p.start_date)))
        AND ($15::timestamptz IS NULL OR (p.updated_at,p.id)<($15,$16::uuid)) ORDER BY p.updated_at DESC,p.id DESC LIMIT $17`, a.OrgID, a.UserID, a.Channel, a.RunID != "", optional(subID), f.Currency, f.Category, optional(f.From), optional(f.To), f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, f.State, at, before, f.Limit+1, f.Query, f.ForecastOnly)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanSubscriptionPeriod(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i, p := range out.Items {
		out.Items[i], err = presentSubscriptionPeriod(ctx, s.DB, a, p)
		if err != nil {
			return core.Page[SubscriptionPeriod]{Items: []SubscriptionPeriod{}}, err
		}
	}
	return out, nil
}
func (s *Service) SubscriptionPeriodHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[SubscriptionPeriodRevision], error) {
	out := core.Page[SubscriptionPeriodRevision]{Items: []SubscriptionPeriodRevision{}}
	if _, err := s.SubscriptionPeriod(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "subscription-period-revisions", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,seq,base_revision_id,content_hash,configuration_revision_id,price_id,currency,amount::text,timezone,category,owner_id,supplier_id,supplier_revision_id,reason,created_at FROM subscription_period_revisions WHERE org_id=$1 AND period_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SubscriptionPeriodRevision
		if err = rows.Scan(&v.ID, &v.Seq, &v.BaseRevisionID, &v.ContentHash, &v.ConfigurationRevisionID, &v.PriceID, &v.Currency, &v.Amount, &v.Timezone, &v.Category, &v.OwnerID, &v.Supplier.PartyID, &v.Supplier.RevisionID, &v.Reason, &v.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}
func (s *Service) SubscriptionPeriodEvents(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[SubscriptionPeriodEvent], error) {
	out := core.Page[SubscriptionPeriodEvent]{Items: []SubscriptionPeriodEvent{}}
	p, err := s.SubscriptionPeriod(ctx, a, id)
	if err != nil {
		return out, err
	}
	head, err := subscriptionMetadata(ctx, s.DB, a.OrgID, p.SubscriptionID)
	if err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "subscription-period-events", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,period_version,revision_id,kind,state,cost_link_id,review_fingerprint,acknowledge_difference,invoice_missing,actor_id,job_id,reason,created_at,ARRAY(SELECT version_id::text FROM subscription_period_event_files f WHERE f.org_id=e.org_id AND f.event_id=e.id ORDER BY version_id),cost_snapshot FROM subscription_period_events e WHERE org_id=$1 AND period_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v SubscriptionPeriodEvent
		var snapshot []byte
		if err = rows.Scan(&v.ID, &v.Version, &v.RevisionID, &v.Kind, &v.State, &v.CostLinkID, &v.ReviewFingerprint, &v.AcknowledgeDifference, &v.InvoiceMissing, &v.ActorID, &v.JobID, &v.Reason, &v.CreatedAt, &v.EvidenceVersionIDs, &snapshot); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(snapshot, &v.CostSnapshot); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i, v := range out.Items {
		_, out.Items[i].Attachments, err = references(ctx, s.DB, a, Cost{ProjectID: head.ProjectID, Classification: head.Classification}, Content{EvidenceVersionIDs: v.EvidenceVersionIDs}, false, false)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
