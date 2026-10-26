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

const subscriptionJoin = ` FROM subscriptions s JOIN subscription_revisions r ON r.org_id=s.org_id AND r.subscription_id=s.id AND r.id=s.current_revision_id `
const subscriptionColumns = `s.id,s.version,s.source_key,s.project_id,s.creator_id,s.classification,s.state,s.current_revision_id,r.content_hash,r.payload,s.cancel_effective_on::text,s.paused_on::text,s.created_at,s.updated_at`

func subscriptionResource(s Subscription) authz.Resource {
	return authz.Resource{Type: "subscription", ID: s.ID, ProjectID: s.ProjectID, OwnerID: &s.CreatorID, Classification: s.Classification}
}
func scanSubscription(row pgx.Row) (Subscription, error) {
	var s Subscription
	var payload []byte
	err := row.Scan(&s.ID, &s.Version, &s.SourceKey, &s.ProjectID, &s.CreatorID, &s.Classification, &s.State, &s.CurrentRevisionID, &s.ContentHash, &payload, &s.CancelEffectiveOn, &s.PausedOn, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &s.SubscriptionContent)
	return s, err
}
func subscriptionMetadata(ctx context.Context, db platform.DBTX, org, id string) (Subscription, error) {
	if !core.ValidID(id) {
		return Subscription{}, core.Missing()
	}
	return scanSubscription(db.QueryRow(ctx, "SELECT "+subscriptionColumns+subscriptionJoin+"WHERE s.org_id=$1 AND s.id=$2", org, id))
}
func subscriptionReadable(ctx context.Context, db platform.DBTX, a core.Actor, s Subscription) error {
	var ok bool
	if err := db.QueryRow(ctx, "SELECT can_read_subscription($1,$2,$3,$4,$5)", a.OrgID, a.UserID, s.ID, a.Channel, a.RunID != "").Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return core.Missing()
	}
	return nil
}
func subscriptionDate(ctx context.Context, db platform.DBTX, zone string) (string, error) {
	var now time.Time
	if err := db.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return "", err
	}
	if zone == "" {
		zone = "Asia/Hong_Kong"
	}
	return SubscriptionToday(now, zone)
}
func presentSubscription(ctx context.Context, db platform.DBTX, a core.Actor, s Subscription) (Subscription, error) {
	if err := subscriptionReadable(ctx, db, a, s); err != nil {
		return s, err
	}
	var err error
	s.SupplierEvidence, s.Attachments, s.OwnerDisplayName, s.OwnerAvailable, err = subscriptionReferences(ctx, db, a, s, s.SubscriptionContent, false)
	if err != nil {
		return s, err
	}
	s.Signals = []string{}
	today, err := subscriptionDate(ctx, db, s.Timezone)
	if err != nil {
		return s, err
	}
	if s.State == "cancelled" && s.CancelEffectiveOn != nil && *s.CancelEffectiveOn > today {
		s.Signals = append(s.Signals, "cancellation_pending")
	}
	if slices.Contains([]string{"paused", "cancelled", "archived"}, s.State) && s.VendorState != "cancelled" {
		s.Signals = append(s.Signals, "vendor_stop_unverified")
	}
	if !s.OwnerAvailable && s.OwnerID != nil {
		s.Signals = append(s.Signals, "owner_unavailable")
	}
	if slices.Contains([]string{"active", "cancelled"}, s.State) {
		calendar, e := subscriptionCalendar(s.SubscriptionContent)
		if e != nil {
			if s.State == "active" {
				return s, e
			}
		} else {
			next, e := calendar.Next(today)
			if e != nil {
				return s, e
			}
			var eligible bool
			if e = db.QueryRow(ctx, "SELECT subscription_charge_eligible($1,$2,$3::date)", a.OrgID, s.ID, next.StartDate).Scan(&eligible); e != nil {
				return s, e
			}
			if eligible {
				s.NextChargeDate = &next.StartDate
				if subscriptionPriceAt(s.Prices, next.StartDate) == nil {
					s.Signals = append(s.Signals, "next_price_unknown")
				}
			}
		}
	}
	human := a.RunID == ""
	writable := human && subscriptionProjectLocks(ctx, db, a, s, nil, false, false) == nil
	s.CanEdit = writable && slices.Contains([]string{"draft", "active", "paused"}, s.State) && authz.Require(ctx, db, a, "subscription.update", subscriptionResource(s)) == nil
	s.CanManage = human && authz.Require(ctx, db, a, "subscription.manage", subscriptionResource(s)) == nil
	return s, nil
}
func (s *Service) Subscription(ctx context.Context, a core.Actor, id string) (Subscription, error) {
	v, err := subscriptionMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return v, err
	}
	return presentSubscription(ctx, s.DB, a, v)
}
func validateSubscriptionFilter(f SubscriptionFilter) error {
	if len([]rune(f.Query)) > 200 || len([]rune(f.Category)) > 50 || (f.Currency != "" && !currencyPatternForFilter(f.Currency)) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) || (f.SupplierID != nil && !core.ValidID(*f.SupplierID)) || (f.OwnerID != nil && !core.ValidID(*f.OwnerID)) || (f.CompanyOnly && f.ProjectID != nil) || core.DateRange(f.From, f.To) != nil {
		return core.Invalid("filter", "订阅筛选无效")
	}
	return nil
}
func currencyPatternForFilter(c string) bool {
	return len(c) == 3 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' && c[2] >= 'A' && c[2] <= 'Z'
}
func (s *Service) Subscriptions(ctx context.Context, a core.Actor, f SubscriptionFilter) (core.Page[Subscription], error) {
	out := core.Page[Subscription]{Items: []Subscription{}}
	if err := validateSubscriptionFilter(f); err != nil {
		return out, err
	}
	if !slices.Contains([]string{"", "draft", "active", "paused", "cancelled", "archived"}, f.State) {
		return out, core.Invalid("state", "订阅状态无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	bound := core.CursorBinding(a, "subscriptions", f.Query, f.State, f.Currency, f.Category, value(f.ProjectID), value(f.SupplierID), value(f.OwnerID), fmt.Sprint(f.CompanyOnly))
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
	rows, err := s.DB.Query(ctx, "SELECT "+subscriptionColumns+subscriptionJoin+`WHERE s.org_id=$1 AND can_read_subscription(s.org_id,$2,s.id,$3,$4)
        AND ($5='' OR position(lower($5) in lower(r.product))>0 OR s.source_key=$5) AND ($6='' OR s.state=$6) AND ($7='' OR EXISTS(SELECT 1 FROM subscription_prices p WHERE p.org_id=s.org_id AND p.revision_id=r.id AND p.currency=$7))
        AND ($8='' OR r.category=$8) AND ($9::uuid IS NULL OR s.project_id=$9) AND (NOT $10 OR s.project_id IS NULL) AND ($11::uuid IS NULL OR r.supplier_id=$11) AND ($12::uuid IS NULL OR r.owner_id=$12)
        AND ($13::timestamptz IS NULL OR (s.updated_at,s.id)<($13,$14::uuid)) ORDER BY s.updated_at DESC,s.id DESC LIMIT $15`, a.OrgID, a.UserID, a.Channel, a.RunID != "", f.Query, f.State, f.Currency, f.Category, f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, at, before, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanSubscription(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &cursor
	}
	for i, v := range out.Items {
		out.Items[i], err = presentSubscription(ctx, s.DB, a, v)
		if err != nil {
			return core.Page[Subscription]{Items: []Subscription{}}, err
		}
	}
	return out, nil
}
func (s *Service) SubscriptionRevision(ctx context.Context, a core.Actor, id, revision string) (SubscriptionRevision, error) {
	head, err := s.Subscription(ctx, a, id)
	if err != nil {
		return SubscriptionRevision{}, err
	}
	if !core.ValidID(revision) {
		return SubscriptionRevision{}, core.Missing()
	}
	var v SubscriptionRevision
	var payload []byte
	err = s.DB.QueryRow(ctx, `SELECT id,seq,base_revision_id,content_hash,payload,author_id,reason,created_at FROM subscription_revisions WHERE org_id=$1 AND subscription_id=$2 AND id=$3`, a.OrgID, id, revision).Scan(&v.ID, &v.Seq, &v.BaseRevisionID, &v.ContentHash, &payload, &v.AuthorID, &v.Reason, &v.CreatedAt)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &v.SubscriptionContent); err != nil {
		return v, err
	}
	v.SupplierEvidence, v.Attachments, v.OwnerDisplayName, v.OwnerAvailable, err = subscriptionReferences(ctx, s.DB, a, head, v.SubscriptionContent, false)
	return v, err
}
func (s *Service) SubscriptionHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[SubscriptionRevision], error) {
	out := core.Page[SubscriptionRevision]{Items: []SubscriptionRevision{}}
	if _, err := s.Subscription(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "subscription-revisions", id)
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
	rows, err := s.DB.Query(ctx, `SELECT id,seq,base_revision_id,content_hash,payload,author_id,reason,created_at FROM subscription_revisions WHERE org_id=$1 AND subscription_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v SubscriptionRevision
		var payload []byte
		if err = rows.Scan(&v.ID, &v.Seq, &v.BaseRevisionID, &v.ContentHash, &payload, &v.AuthorID, &v.Reason, &v.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(payload, &v.SubscriptionContent); err != nil {
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
		last := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i, v := range out.Items {
		out.Items[i], err = s.SubscriptionRevision(ctx, a, id, v.ID)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) SubscriptionEvents(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[SubscriptionEvent], error) {
	out := core.Page[SubscriptionEvent]{Items: []SubscriptionEvent{}}
	if _, err := s.Subscription(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "subscription-events", id)
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
	rows, err := s.DB.Query(ctx, `SELECT id,subscription_version,revision_id,kind,state,period_id,effective_on::text,actor_id,job_id,reason,created_at FROM subscription_events WHERE org_id=$1 AND subscription_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SubscriptionEvent
		if err = rows.Scan(&v.ID, &v.Version, &v.RevisionID, &v.Kind, &v.State, &v.PeriodID, &v.EffectiveOn, &v.ActorID, &v.JobID, &v.Reason, &v.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}

// This is a responsibility picker, not a member-administration or HR endpoint.
func (s *Service) SubscriptionOwners(ctx context.Context, a core.Actor, project *string, class string) (core.Page[SubscriptionOwner], error) {
	out := core.Page[SubscriptionOwner]{Items: []SubscriptionOwner{}}
	scope := Subscription{ID: core.NewID(), ProjectID: project, CreatorID: a.UserID, Classification: class}
	if !core.Classification(class) || (project != nil && !core.ValidID(*project)) || (class == "project_restricted" && project == nil) {
		return out, core.Invalid("scope", "请提供订阅的项目范围与分类")
	}
	if err := authz.Require(ctx, s.DB, a, "subscription.create", subscriptionResource(scope)); err != nil {
		return out, err
	}
	if err := subscriptionProjectLocks(ctx, s.DB, a, scope, nil, false, false); err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,display_name FROM users WHERE org_id=$1 AND status='active' ORDER BY display_name,id LIMIT 1000", a.OrgID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SubscriptionOwner
		if err = rows.Scan(&v.ID, &v.DisplayName); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
