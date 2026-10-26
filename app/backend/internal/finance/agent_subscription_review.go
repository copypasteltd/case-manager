package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// ReviewScope is supplied by the leased conversation, never by model arguments.
type ReviewScope struct {
	ProjectID                      *string
	Classification, ConversationID string
	Audience                       []string
}
type SubscriptionDiscovery struct {
	Query  string `json:"query"`
	State  string `json:"state"`
	Cursor string `json:"cursor"`
}
type SubscriptionReviewInput struct {
	SubscriptionID string `json:"subscription_id"`
	From           string `json:"from"`
	To             string `json:"to"`
}
type FinanceRecordInput struct {
	SubscriptionID string `json:"subscription_id"`
	PeriodID       string `json:"period_id"`
	Kind           string `json:"kind"`
	ID             string `json:"id"`
}

func reviewID(id string) bool { return core.ValidID(id) && id == strings.ToLower(id) }
func reviewBounded(n, max int) error {
	if n > max {
		return core.Invalid("scope", "逐笔审阅范围超过受控上限，请缩小日期范围；未返回不完整前缀")
	}
	return nil
}
func reviewEnvelope(ctx context.Context, tx pgx.Tx, scope ReviewScope, view string, report any, refs []AgentSummaryEvidence) (map[string]any, error) {
	var captured time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&captured); err != nil {
		return nil, err
	}
	return map[string]any{"schema_version": 2, "view": view, "scope": "subscription_records", "project_id": scope.ProjectID, "captured_at": captured, "historical_snapshot": true, "untrusted_data": true, "input_count": len(refs), "report": report,
		"coverage_notice": "仅为当前会话公司订阅或单一项目订阅及其实际关联原记录。每名成员须有权读取全部来源；列表须继续分页。原费用与原付款可能服务多个项目，原额不等于本项目份额；分摊及分配分别核对，预测、发生支出、现金不合并。空账期、未归属退款、内部取消及候选异常不证明零义务、供应商停费、已退款或重复扣费。"}, nil
}

// Keep the historical relationship as part of authorization, not just the ID.
// Removing/reassigning an association makes the old detailed snapshot unreadable.
func validateReviewRelation(ctx context.Context, tx pgx.Tx, org string, project *string, ref AgentSummaryEvidence) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscriptions s WHERE s.org_id=$1 AND s.id=$2
 AND s.project_id IS NOT DISTINCT FROM $3::uuid AND (
 ($4='subscription' AND s.id=$5::uuid) OR
 ($4='subscription_period' AND EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=s.org_id AND p.subscription_id=s.id AND p.id=$5::uuid)) OR
 ($4 IN ('cost','payment') AND EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=s.org_id AND p.subscription_id=s.id AND p.id=$6::uuid AND (
 ($4='cost' AND EXISTS(SELECT 1 FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND (c.id=l.cost_id OR c.reverses_id=l.cost_id) WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id AND c.id=$5::uuid)) OR
 ($4='payment' AND EXISTS(SELECT 1 FROM cost_links l JOIN payment_allocations x ON x.org_id=l.org_id AND x.cost_id=l.cost_id JOIN payment_records q ON q.org_id=x.org_id JOIN payment_revisions r ON r.org_id=q.org_id AND r.id=q.current_revision_id WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id AND q.id=$5::uuid AND (q.id=x.payment_id OR q.reverses_id=x.payment_id OR r.refund_of_id=x.payment_id)))
 )))))`, org, ref.SubscriptionID, project, ref.Type, ref.ID, optional(ref.PeriodID)).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return core.Missing()
	}
	return nil
}

// Public projections can contain permission-dependent links and settlement data.
// Compare every participant's actual projection; never reuse the actor's richer
// projection for a participant with a restricted settlement or hidden attachment.
func commonReviewProjection(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, kind, id string) (any, error) {
	var result any
	var first []byte
	for _, user := range scope.Audience {
		target := a
		target.UserID = user
		var value any
		var err error
		switch kind {
		case "subscription":
			var v Subscription
			v, err = subscriptionMetadata(ctx, tx, a.OrgID, id)
			if err == nil {
				value, err = presentSubscription(ctx, tx, target, v)
				if err == nil {
					v = value.(Subscription)
					v.CanEdit, v.CanManage = false, false
					value = v
				}
			}
		case "subscription_period":
			var v SubscriptionPeriod
			v, err = subscriptionPeriodMetadata(ctx, tx, a.OrgID, id)
			if err == nil {
				value, err = presentSubscriptionPeriod(ctx, tx, target, v)
				if err == nil {
					v = value.(SubscriptionPeriod)
					v.CanManage = false
					value = v
				}
			}
		case "cost":
			var v Cost
			v, err = metadata(ctx, tx, a.OrgID, id)
			if err == nil {
				value, err = present(ctx, tx, target, v)
				if err == nil {
					v = value.(Cost)
					v.CanEdit, v.CanConfirm, v.CanReverse, v.CanLink = false, false, false, false
					value = v
				}
			}
		case "payment":
			var v Payment
			v, err = paymentMetadata(ctx, tx, a.OrgID, id)
			if err == nil {
				value, err = presentPayment(ctx, tx, target, v)
				if err == nil {
					v = value.(Payment)
					v.CanEdit, v.CanRecord, v.CanAllocate, v.CanReverse = false, false, false, false
					value = v
				}
			}
		default:
			return nil, core.Missing()
		}
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if first == nil {
			first = raw
			result = value
		} else if !bytes.Equal(first, raw) {
			return nil, core.Missing()
		}
	}
	if result == nil {
		return nil, core.Missing()
	}
	return result, nil
}

func AgentSubscriptions(ctx context.Context, tx pgx.Tx, pager platform.Pager, a core.Actor, scope ReviewScope, in SubscriptionDiscovery) (map[string]any, []AgentSummaryEvidence, error) {
	if len([]rune(in.Query)) > 200 || !slices.Contains([]string{"", "draft", "active", "paused", "cancelled", "archived"}, in.State) {
		return nil, nil, core.Invalid("filter", "订阅查询条件无效")
	}
	if err := ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, nil); err != nil {
		return nil, nil, err
	}
	audience := append([]string{}, scope.Audience...)
	slices.Sort(audience)
	binding := core.CursorBinding(a, "agent-subscriptions", scope.ConversationID, scope.Classification, strings.Join(audience, ","), value(scope.ProjectID), in.Query, in.State)
	var before *string
	var at *time.Time
	if in.Cursor != "" {
		c, err := pager.Decode(in.Cursor, binding)
		if err != nil {
			return nil, nil, err
		}
		before = &c.ID
		at = &c.UpdatedAt
	}
	rows, err := tx.Query(ctx, `SELECT s.id::text,s.current_revision_id::text,s.version,s.state,r.product,s.updated_at`+subscriptionJoin+`WHERE s.org_id=$1 AND can_read_subscription(s.org_id,$2,s.id,$3,true) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,true)
 AND s.project_id IS NOT DISTINCT FROM $4::uuid AND ($5='' OR strpos(lower(r.product),lower($5))>0 OR s.source_key=$5) AND ($6='' OR s.state=$6)
 AND ($7::timestamptz IS NULL OR (s.updated_at,s.id)<($7,$8::uuid)) ORDER BY s.updated_at DESC,s.id DESC LIMIT 21`, a.OrgID, a.UserID, a.Channel, scope.ProjectID, in.Query, in.State, at, before)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		ID       string    `json:"id"`
		Revision string    `json:"current_revision_id"`
		Version  int64     `json:"version"`
		State    string    `json:"state"`
		Product  string    `json:"product"`
		Updated  time.Time `json:"updated_at"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Revision, &v.Version, &v.State, &v.Product, &v.Updated); err != nil {
			break
		}
		items = append(items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	var next *string
	if len(items) > 20 {
		items = items[:20]
		last := items[19]
		v := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.Updated, Binding: binding})
		next = &v
	}
	refs := []AgentSummaryEvidence{}
	for _, v := range items {
		refs = append(refs, AgentSummaryEvidence{Type: "subscription", ID: v.ID, RevisionID: v.Revision, SubscriptionID: v.ID})
	}
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, refs); err != nil {
		return nil, nil, err
	}
	out, err := reviewEnvelope(ctx, tx, scope, "subscription_list", map[string]any{"items": items, "next_cursor": next, "page_size": 20, "page_exhausted": next == nil, "list_complete_on_this_page": in.Cursor == "" && next == nil, "query": in.Query, "state": in.State}, refs)
	return out, refs, err
}

func reviewSubscription(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, id string) (Subscription, []AgentSummaryEvidence, error) {
	if !reviewID(id) {
		return Subscription{}, nil, core.Missing()
	}
	s, err := subscriptionMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return s, nil, err
	}
	refs := []AgentSummaryEvidence{{Type: "subscription", ID: s.ID, RevisionID: s.CurrentRevisionID, SubscriptionID: s.ID}}
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, refs); err != nil {
		return s, nil, err
	}
	projected, err := commonReviewProjection(ctx, tx, a, scope, "subscription", id)
	if err != nil {
		return s, nil, err
	}
	return projected.(Subscription), refs, nil
}

func AgentSubscriptionReview(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, in SubscriptionReviewInput) (map[string]any, []AgentSummaryEvidence, error) {
	from, e := civilDate(in.From)
	if e != nil {
		return nil, nil, e
	}
	to, e := civilDate(in.To)
	if e != nil || !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
		return nil, nil, core.Invalid("dates", "审阅日期须为明确有效的开始日和不含结束日，最多366个自然日")
	}
	sub, refs, err := reviewSubscription(ctx, tx, a, scope, in.SubscriptionID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id::text FROM subscription_periods p WHERE org_id=$1 AND subscription_id=$2 AND start_date>=$3::date AND start_date<$4::date ORDER BY start_date,p.id LIMIT 101`, a.OrgID, sub.ID, in.From, in.To)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if err = reviewBounded(len(ids), 100); err != nil {
		return nil, nil, err
	}
	deps, e := reviewDependencies(ctx, tx, a, scope, sub)
	if e != nil {
		return nil, nil, e
	}
	refs = append(refs, deps...)
	periods := []SubscriptionPeriod{}
	costs := []map[string]any{}
	payments := []map[string]any{}
	allocations := []PaymentAllocation{}
	seenCost, seenPayment := map[string]bool{}, map[string]bool{}
	for _, id := range ids {
		projected, e := commonReviewProjection(ctx, tx, a, scope, "subscription_period", id)
		if e != nil {
			return nil, nil, e
		}
		p := projected.(SubscriptionPeriod)
		periods = append(periods, p)
		refs = append(refs, AgentSummaryEvidence{Type: "subscription_period", ID: p.ID, RevisionID: p.CurrentRevisionID, SubscriptionID: sub.ID})
		for _, costID := range p.CostIDs {
			if seenCost[costID] {
				continue
			}
			seenCost[costID] = true
			if e := reviewBounded(len(seenCost), 200); e != nil {
				return nil, nil, e
			}
			projected, e = commonReviewProjection(ctx, tx, a, scope, "cost", costID)
			if e != nil {
				return nil, nil, e
			}
			c := projected.(Cost)
			deps, e := reviewDependencies(ctx, tx, a, scope, c)
			if e != nil {
				return nil, nil, e
			}
			refs = append(refs, deps...)
			refs = append(refs, AgentSummaryEvidence{Type: "cost", ID: c.ID, RevisionID: c.CurrentRevisionID, SubscriptionID: sub.ID, PeriodID: p.ID})
			if e := reviewBounded(len(refs), 500); e != nil {
				return nil, nil, e
			}
			costs = append(costs, map[string]any{"id": c.ID, "version": c.Version, "current_revision_id": c.CurrentRevisionID, "period_id": p.ID, "title": c.Title, "state": c.State, "entry_kind": c.EntryKind, "currency": c.Currency, "original_amount": c.Amount, "occurred_on": c.OccurredOn, "allocations": c.Allocations, "settlement_visibility": c.SettlementVisibility, "paid_amount": c.PaidAmount, "outstanding_amount": c.OutstandingAmount, "reversal_id": c.ReversalID, "evidence": c.Attachments})
		}
	}
	if err = reviewBounded(len(costs), 200); err != nil {
		return nil, nil, err
	}
	// Include only actual allocation links. A global payment/refund amount is not
	// allocated to this subscription merely because it belongs to the same family.
	rows, err = tx.Query(ctx, `SELECT `+allocationColumns+` FROM payment_allocations a WHERE a.org_id=$1 AND a.cost_id=ANY($2::uuid[]) ORDER BY a.created_at,a.id LIMIT 201`, a.OrgID, stringMapKeys(seenCost))
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		v, e := scanAllocation(rows)
		if e != nil {
			err = e
			break
		}
		allocations = append(allocations, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if err = reviewBounded(len(allocations), 200); err != nil {
		return nil, nil, err
	}
	for i := range allocations {
		v := &allocations[i]
		if err = allocationShares(ctx, tx, a.OrgID, v); err != nil {
			return nil, nil, err
		}
		costPeriod := ""
		for _, c := range costs {
			if c["id"] == v.CostID {
				costPeriod = c["period_id"].(string)
				break
			}
		}
		refs = append(refs, AgentSummaryEvidence{Type: "cost", ID: v.CostID, RevisionID: v.CostRevisionID, SubscriptionID: sub.ID, PeriodID: costPeriod})
		if seenPayment[v.PaymentID] {
			continue
		}
		seenPayment[v.PaymentID] = true
		projected, e := commonReviewProjection(ctx, tx, a, scope, "payment", v.PaymentID)
		if e != nil {
			return nil, nil, e
		}
		p := projected.(Payment)
		deps, e := reviewDependencies(ctx, tx, a, scope, p)
		if e != nil {
			return nil, nil, e
		}
		refs = append(refs, deps...)
		periodID := ""
		for _, c := range costs {
			if c["id"] == v.CostID {
				periodID = c["period_id"].(string)
				break
			}
		}
		refs = append(refs, AgentSummaryEvidence{Type: "payment", ID: p.ID, RevisionID: p.CurrentRevisionID, SubscriptionID: sub.ID, PeriodID: periodID})
		payments = append(payments, map[string]any{"id": p.ID, "version": p.Version, "current_revision_id": p.CurrentRevisionID, "title": p.Title, "state": p.State, "record_kind": p.RecordKind, "direction": p.Direction, "currency": p.Currency, "original_amount": p.Amount, "paid_at": p.PaidAt, "paid_on": p.PaidOn, "refund_of_id": p.RefundOfID, "reverses_id": p.ReversesID, "refunded_amount": p.RefundedAmount, "unallocated_amount": p.UnallocatedAmount, "evidence": p.Attachments})
	}
	if err = reviewBounded(len(refs), 500); err != nil {
		return nil, nil, err
	}
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, refs); err != nil {
		return nil, nil, err
	}
	missing := []SubscriptionWindow{}
	coverageKnown := sub.State != "draft"
	expected := 0
	calendar, e := subscriptionCalendar(sub.SubscriptionContent)
	if e != nil {
		coverageKnown = false
	} else {
		windows, e := calendar.Windows(in.From, in.To)
		if e != nil {
			return nil, nil, e
		}
		if e = reviewBounded(len(windows), 100); e != nil {
			return nil, nil, e
		}
		for _, w := range windows {
			var eligible bool
			if err = tx.QueryRow(ctx, "SELECT subscription_charge_eligible($1,$2,$3::date)", a.OrgID, sub.ID, w.StartDate).Scan(&eligible); err != nil {
				return nil, nil, err
			}
			if !eligible {
				continue
			}
			expected++
			found := false
			for _, p := range periods {
				if p.StartDate == w.StartDate && p.ChargeType == "recurring" {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, w)
			}
		}
	}
	out, err := reviewEnvelope(ctx, tx, scope, "subscription_review", map[string]any{"from": in.From, "to_exclusive": in.To, "date_basis": "subscription_local_start_date", "subscription": sub, "periods": periods, "costs": costs, "payments": payments, "allocations": allocations, "all_stored_periods_in_window": true, "coverage_known": coverageKnown, "coverage_complete": coverageKnown && len(missing) == 0, "expected_scheduled_period_count": expected, "missing_scheduled_periods": missing, "cash_notice": "原付款金额、退款总额及未分配余额是整条资金记录的事实；只有 allocations 中的本次关联流水构成订阅分配。费用原额、项目份额及现金分别判断，内部候选异常不等于已确认重复扣费。"}, refs)
	return out, refs, err
}

// Use a deterministic ID slice when querying the actual ledger.
func stringMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for id := range values {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	return keys
}

func AgentFinanceRecord(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, in FinanceRecordInput) (map[string]any, []AgentSummaryEvidence, error) {
	if !reviewID(in.SubscriptionID) || !reviewID(in.PeriodID) || !reviewID(in.ID) || !slices.Contains([]string{"cost", "payment"}, in.Kind) {
		return nil, nil, core.Invalid("record", "请选择准确订阅、账期、费用或资金记录")
	}
	_, refs, err := reviewSubscription(ctx, tx, a, scope, in.SubscriptionID)
	if err != nil {
		return nil, nil, err
	}
	relation := AgentSummaryEvidence{Type: in.Kind, ID: in.ID, SubscriptionID: in.SubscriptionID, PeriodID: in.PeriodID}
	if err = validateReviewRelation(ctx, tx, a.OrgID, scope.ProjectID, relation); err != nil {
		return nil, nil, err
	}
	projected, err := commonReviewProjection(ctx, tx, a, scope, in.Kind, in.ID)
	if err != nil {
		return nil, nil, err
	}
	if in.Kind == "cost" {
		relation.RevisionID = projected.(Cost).CurrentRevisionID
	} else {
		relation.RevisionID = projected.(Payment).CurrentRevisionID
	}
	refs = append(refs, relation)
	deps, e := reviewDependencies(ctx, tx, a, scope, projected)
	if e != nil {
		return nil, nil, e
	}
	refs = append(refs, deps...)
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, refs); err != nil {
		return nil, nil, err
	}
	out, err := reviewEnvelope(ctx, tx, scope, "finance_record", map[string]any{"subscription_id": in.SubscriptionID, "period_id": in.PeriodID, "kind": in.Kind, "record": projected, "record_amount_basis": "original_record_not_project_or_subscription_share"}, refs)
	return out, refs, err
}
