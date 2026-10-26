package finance

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func subscriptionPeriodHash(p SubscriptionPeriod) string {
	raw, _ := json.Marshal(struct {
		SubscriptionID, StartDate, EndDate, ChargeType, ConfigurationRevisionID, PriceID, Currency, Amount, Timezone, Category, OwnerID string
		Supplier                                                                                                                        PartyRef
	}{p.SubscriptionID, p.StartDate, p.EndDate, p.ChargeType, p.ConfigurationRevisionID, p.PriceID, p.Currency, p.Amount, p.Timezone, p.Category, p.OwnerID, p.Supplier})
	return core.Digest(raw)
}
func pinSubscriptionPeriod(ctx context.Context, tx pgx.Tx, org string, p *SubscriptionPeriod, s Subscription, seq int64, base *string, reason string) error {
	var price SubscriptionPriceInput
	if err := tx.QueryRow(ctx, `SELECT id,currency,amount::text FROM subscription_prices WHERE org_id=$1 AND revision_id=$2 AND effective_from<=$3::date AND (effective_to IS NULL OR effective_to>$3::date)`, org, s.CurrentRevisionID, p.StartDate).Scan(&p.PriceID, &price.Currency, &price.Amount); err != nil {
		return platform.DatabaseError(err)
	}
	p.ConfigurationRevisionID = s.CurrentRevisionID
	p.Currency = price.Currency
	p.Amount = price.Amount
	p.Timezone = s.Timezone
	p.Category = s.Category
	if s.OwnerID == nil || s.Supplier == nil {
		return core.Invalid("content", "账期须有固定供应方与负责人")
	}
	p.OwnerID = *s.OwnerID
	p.Supplier = *s.Supplier
	p.ContentHash = subscriptionPeriodHash(*p)
	_, err := tx.Exec(ctx, `INSERT INTO subscription_period_revisions(id,org_id,period_id,seq,base_revision_id,configuration_revision_id,price_id,content_hash,currency,amount,timezone,category,owner_id,supplier_id,supplier_revision_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, p.CurrentRevisionID, org, p.ID, seq, base, p.ConfigurationRevisionID, p.PriceID, p.ContentHash, p.Currency, p.Amount, p.Timezone, p.Category, p.OwnerID, p.Supplier.PartyID, p.Supplier.RevisionID, reason)
	return err
}
func newSubscriptionPeriod(ctx context.Context, tx pgx.Tx, org string, s Subscription, w SubscriptionWindow, reason string) (SubscriptionPeriod, error) {
	p := SubscriptionPeriod{ID: core.NewID(), SubscriptionID: s.ID, Version: 1, CurrentRevisionID: core.NewID(), StartDate: w.StartDate, EndDate: w.EndDate, ChargeType: "recurring", State: "forecast", RecordedState: "forecast"}
	if _, err := tx.Exec(ctx, `INSERT INTO subscription_periods(id,org_id,subscription_id,start_date,end_date,charge_type,current_revision_id,state) VALUES($1,$2,$3,$4,$5,'recurring',$6,'forecast')`, p.ID, org, s.ID, p.StartDate, p.EndDate, p.CurrentRevisionID); err != nil {
		return p, err
	}
	err := pinSubscriptionPeriod(ctx, tx, org, &p, s, 1, nil, reason)
	return p, err
}
func subscriptionPeriodEvent(ctx context.Context, tx pgx.Tx, a core.Actor, p SubscriptionPeriod, parent string, e SubscriptionPeriodEvent, job *jobqueue.Job) error {
	id := core.NewID()
	var err error
	if job != nil {
		_, err = tx.Exec(ctx, `INSERT INTO subscription_period_events(id,org_id,period_id,period_version,revision_id,subscription_event_id,kind,state,job_id,job_fence,job_owner,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, a.OrgID, p.ID, p.Version, p.CurrentRevisionID, parent, e.Kind, p.RecordedState, job.ID, job.FencingToken, job.LeaseOwner, e.Reason)
	} else {
		var tagErr error
		tag, execErr := tx.Exec(ctx, `INSERT INTO subscription_period_events(id,org_id,period_id,period_version,revision_id,subscription_event_id,kind,state,cost_link_id,review_fingerprint,acknowledge_difference,invoice_missing,actor_id,session_id,mfa_verified_at,reason,cost_snapshot)
			SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,id,mfa_verified_at,$14,CASE WHEN $7='confirmed' THEN subscription_actual_snapshot($2,$3) ELSE '[]'::jsonb END FROM sessions WHERE org_id=$2 AND id=$15 AND user_id=$13`, id, a.OrgID, p.ID, p.Version, p.CurrentRevisionID, parent, e.Kind, p.RecordedState, e.CostLinkID, e.ReviewFingerprint, e.AcknowledgeDifference, e.InvoiceMissing, a.UserID, e.Reason, a.SessionID)
		if execErr == nil && tag.RowsAffected() != 1 {
			tagErr = core.Forbidden()
		}
		err = execErr
		if err == nil {
			err = tagErr
		}
	}
	if err != nil {
		return err
	}
	for _, file := range e.EvidenceVersionIDs {
		if _, err = tx.Exec(ctx, "INSERT INTO subscription_period_event_files(org_id,event_id,version_id) VALUES($1,$2,$3)", a.OrgID, id, file); err != nil {
			return err
		}
	}
	return nil
}

// Parent/head locks and a current human session or checked worker lease are
// acquired by the caller. Existing immutable periods are never recomputed.
func generateSubscriptionPeriods(ctx context.Context, tx pgx.Tx, a core.Actor, s Subscription, from, to, parent, reason string, job *jobqueue.Job) (SubscriptionGeneration, error) {
	out := SubscriptionGeneration{SubscriptionID: s.ID, Version: s.Version, From: from, To: to, GeneratedIDs: []string{}, MissingPriceDates: []string{}, CoverageComplete: true}
	calendar, err := subscriptionCalendar(s.SubscriptionContent)
	if err != nil {
		return out, err
	}
	windows, err := calendar.Windows(from, to)
	if err != nil {
		return out, err
	}
	for _, w := range windows {
		var eligible, exists bool
		if err = tx.QueryRow(ctx, `SELECT subscription_charge_eligible($1,$2,$3::date),EXISTS(SELECT 1 FROM subscription_periods WHERE org_id=$1 AND subscription_id=$2 AND start_date=$3::date AND end_date=$4::date AND charge_type='recurring')`, a.OrgID, s.ID, w.StartDate, w.EndDate).Scan(&eligible, &exists); err != nil {
			return out, err
		}
		if !eligible {
			continue
		}
		if exists {
			out.ExistingCount++
			continue
		}
		if subscriptionPriceAt(s.Prices, w.StartDate) == nil {
			out.MissingPriceDates = append(out.MissingPriceDates, w.StartDate)
			out.CoverageComplete = false
			continue
		}
		p, e := newSubscriptionPeriod(ctx, tx, a.OrgID, s, w, reason)
		if e != nil {
			return out, e
		}
		if e = subscriptionPeriodEvent(ctx, tx, a, p, parent, SubscriptionPeriodEvent{Kind: "generated", Reason: reason}, job); e != nil {
			return out, e
		}
		out.GeneratedIDs = append(out.GeneratedIDs, p.ID)
	}
	return out, nil
}
func (s *Service) GenerateSubscription(ctx context.Context, cmd core.Command, id string, in SubscriptionGenerate) (core.Result, error) {
	return s.mutateSubscription(ctx, cmd, id, "subscription.manage", in.Base, true, false, nil, func(tx pgx.Tx, v Subscription) (core.Result, error) {
		if !slices.Contains([]string{"active", "cancelled"}, v.State) {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_NOT_ENABLED", "仅已启用或取消生效前的订阅可以生成正常预测")
		}
		var enabled bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM subscription_events WHERE org_id=$1 AND subscription_id=$2 AND kind='activated')", cmd.Actor.OrgID, v.ID).Scan(&enabled); err != nil {
			return core.Result{}, err
		}
		if !enabled {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_NOT_ENABLED", "该订阅未曾启用，不能自动推定扣费周期")
		}
		parent, err := advanceSubscription(ctx, tx, cmd.Actor, &v, "periods_generated", in.Reason, nil, nil)
		if err != nil {
			return core.Result{}, err
		}
		generated, err := generateSubscriptionPeriods(ctx, tx, cmd.Actor, v, in.From, in.To, parent, in.Reason, nil)
		if err != nil {
			return core.Result{}, err
		}
		out := subscriptionResult(v, "periods_generated", in.Reason, 200)
		out.Data = generated
		return out, nil
	})
}
func (s *Service) UnexpectedSubscriptionPeriod(ctx context.Context, cmd core.Command, id string, in SubscriptionUnexpectedPeriod) (core.Result, error) {
	return s.mutateSubscription(ctx, cmd, id, "subscription.manage", in.Base, true, false, nil, func(tx pgx.Tx, v Subscription) (core.Result, error) {
		if !slices.Contains([]string{"paused", "cancelled"}, v.State) {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_NOT_STOPPED", "异常停订扣款入口只用于已暂停或取消的订阅")
		}
		today, err := subscriptionDate(ctx, tx, v.Timezone)
		if err != nil {
			return core.Result{}, err
		}
		if in.StartDate > today {
			return core.Result{}, core.Invalid("start_date", "异常扣款须对应已经发生的账期")
		}
		calendar, err := subscriptionCalendar(v.SubscriptionContent)
		if err != nil {
			return core.Result{}, err
		}
		w, err := calendar.Next(in.StartDate)
		if err != nil {
			return core.Result{}, err
		}
		if w.StartDate != in.StartDate {
			return core.Result{}, core.Invalid("start_date", "请选择原始日历的准确账期起点")
		}
		if subscriptionPriceAt(v.Prices, w.StartDate) == nil {
			return core.Result{}, core.Invalid("start_date", "该原始账期价格未知，请保留真实费用并先核对价格依据")
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM subscription_periods WHERE org_id=$1 AND subscription_id=$2 AND start_date=$3::date)", cmd.Actor.OrgID, v.ID, w.StartDate).Scan(&exists); err != nil {
			return core.Result{}, err
		}
		if exists {
			return core.Result{}, core.Fail(409, "BILLING_PERIOD_EXISTS", "该账期已存在，请在原账期核对或重新开启，不能重复建立")
		}
		p, err := newSubscriptionPeriod(ctx, tx, cmd.Actor.OrgID, v, w, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		parent, err := advanceSubscription(ctx, tx, cmd.Actor, &v, "period_changed", in.Reason, &p.ID, nil)
		if err != nil {
			return core.Result{}, err
		}
		if err = subscriptionPeriodEvent(ctx, tx, cmd.Actor, p, parent, SubscriptionPeriodEvent{Kind: "unexpected", Reason: in.Reason}, nil); err != nil {
			return core.Result{}, err
		}
		p, err = subscriptionPeriodMetadata(ctx, tx, cmd.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		p, err = presentSubscriptionPeriod(ctx, tx, cmd.Actor, p)
		out := subscriptionResult(v, "period_changed", in.Reason, 201)
		out.Data = p
		return out, err
	})
}
func (s *Service) mutateSubscriptionPeriod(ctx context.Context, cmd core.Command, id string, base SubscriptionPeriodBase, costs []string, fn func(pgx.Tx, Subscription, SubscriptionPeriod) (core.Result, error)) (out core.Result, err error) {
	defer func() { out, err = s.subscriptionReplay(ctx, cmd.Actor, out, err) }()
	if e := validBase(Base{ExpectedVersion: base.ExpectedVersion, RevisionID: base.RevisionID, ContentHash: base.ContentHash, Reason: base.Reason}); e != nil {
		return out, core.Invalid("revision_id", "请固定准确账期版本、修订及摘要，并说明原因")
	}
	check := func(tx pgx.Tx) (Subscription, SubscriptionPeriod, error) {
		p, e := subscriptionPeriodMetadata(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return Subscription{}, p, e
		}
		v, e := subscriptionMetadata(ctx, tx, cmd.Actor.OrgID, p.SubscriptionID)
		if e != nil {
			return v, p, e
		}
		if e = subscriptionReadable(ctx, tx, cmd.Actor, v); e != nil {
			return v, p, e
		}
		if e = authz.Require(ctx, tx, cmd.Actor, "subscription.manage", subscriptionResource(v)); e != nil {
			return v, p, e
		}
		for _, costID := range costs {
			c, ce := metadata(ctx, tx, cmd.Actor.OrgID, costID)
			if ce != nil {
				return v, p, ce
			}
			if ce = readable(ctx, tx, cmd.Actor, c, c.CurrentRevisionID); ce != nil {
				return v, p, ce
			}
			if ce = authz.Require(ctx, tx, cmd.Actor, "cost.record", resource(c)); ce != nil {
				return v, p, ce
			}
		}
		return v, p, identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true)
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		v, _, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if v.State == "archived" {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_ARCHIVED", "已归档订阅不可新增账期操作")
		}
		allCosts := append([]string{}, costs...)
		rows, e := tx.Query(ctx, "SELECT cost_id::text FROM cost_links WHERE org_id=$1 AND source_type='subscription_period' AND source_id=$2 ORDER BY cost_id", cmd.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		for rows.Next() {
			var costID string
			if e = rows.Scan(&costID); e != nil {
				rows.Close()
				return core.Result{}, e
			}
			allCosts = append(allCosts, costID)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return core.Result{}, e
		}
		if e = subscriptionProjectLocks(ctx, tx, cmd.Actor, v, allCosts, true, false); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM subscriptions WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, v.ID); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM subscription_periods WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		sorted := allCosts
		slices.Sort(sorted)
		sorted = slices.Compact(sorted)
		for _, costID := range sorted {
			if _, e = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, costID); e != nil {
				return core.Result{}, e
			}
		}
		v, p, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if v.State == "archived" {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_ARCHIVED", "订阅已归档")
		}
		if p.Version != base.ExpectedVersion || p.CurrentRevisionID != strings.ToLower(base.RevisionID) || p.ContentHash != base.ContentHash {
			return core.Result{}, core.Conflict()
		}
		before, _ := json.Marshal(p)
		out, e := fn(tx, v, p)
		out.Event.BeforeDigest = core.Digest(before)
		return out, e
	})
}
func (s *Service) finishSubscriptionPeriod(ctx context.Context, tx pgx.Tx, a core.Actor, v Subscription, p SubscriptionPeriod, e SubscriptionPeriodEvent) (core.Result, error) {
	p.Version++
	if _, err := tx.Exec(ctx, `UPDATE subscription_periods SET version=version+1,current_revision_id=$3,state=$4,review_fingerprint=$5,review_reason=$6,invoice_missing=$7,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, a.OrgID, p.ID, p.CurrentRevisionID, p.RecordedState, e.ReviewFingerprint, p.ReviewReason, e.InvoiceMissing); err != nil {
		return core.Result{}, err
	}
	parent, err := advanceSubscription(ctx, tx, a, &v, "period_changed", e.Reason, &p.ID, nil)
	if err != nil {
		return core.Result{}, err
	}
	if err = subscriptionPeriodEvent(ctx, tx, a, p, parent, e, nil); err != nil {
		return core.Result{}, err
	}
	p, err = subscriptionPeriodMetadata(ctx, tx, a.OrgID, p.ID)
	if err != nil {
		return core.Result{}, err
	}
	p, err = presentSubscriptionPeriod(ctx, tx, a, p)
	out := subscriptionResult(v, "period_changed", e.Reason, 200)
	out.Data = p
	out.Events = []core.Event{{Type: "subscription.period." + strings.ReplaceAll(e.Kind, "_", "."), ResourceType: "subscription_period", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"reason": e.Reason, "revision_id": p.CurrentRevisionID, "subscription_id": p.SubscriptionID}}}
	return out, err
}
func (s *Service) ReforecastSubscriptionPeriod(ctx context.Context, cmd core.Command, id string, in SubscriptionPeriodReforecast) (core.Result, error) {
	if !core.ValidID(in.ConfigurationRevisionID) || len(in.ConfigurationHash) != 64 {
		return core.Result{}, core.Invalid("configuration_revision_id", "请固定本次重估的准确当前配置")
	}
	return s.mutateSubscriptionPeriod(ctx, cmd, id, in.SubscriptionPeriodBase, nil, func(tx pgx.Tx, v Subscription, p SubscriptionPeriod) (core.Result, error) {
		if v.CurrentRevisionID != strings.ToLower(in.ConfigurationRevisionID) || v.ContentHash != in.ConfigurationHash {
			return core.Result{}, core.Conflict()
		}
		var linked bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cost_links WHERE org_id=$1 AND source_type='subscription_period' AND source_id=$2)", cmd.Actor.OrgID, p.ID).Scan(&linked); err != nil {
			return core.Result{}, err
		}
		today, err := subscriptionDate(ctx, tx, p.Timezone)
		if err != nil {
			return core.Result{}, err
		}
		if p.RecordedState != "forecast" || linked || p.StartDate < today {
			return core.Result{}, core.Fail(409, "BILLING_REFORECAST_FORBIDDEN", "仅未来且从未关联实际费用的预测可以显式重估")
		}
		if _, _, _, _, err = subscriptionReferences(ctx, tx, cmd.Actor, v, v.SubscriptionContent, true); err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM subscription_period_revisions WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, p.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		old := p.CurrentRevisionID
		p.CurrentRevisionID = core.NewID()
		if err = pinSubscriptionPeriod(ctx, tx, cmd.Actor.OrgID, &p, v, seq+1, &old, in.Reason); err != nil {
			return core.Result{}, err
		}
		return s.finishSubscriptionPeriod(ctx, tx, cmd.Actor, v, p, SubscriptionPeriodEvent{Kind: "reforecast", Reason: in.Reason})
	})
}
func (s *Service) LinkSubscriptionCost(ctx context.Context, cmd core.Command, id string, in SubscriptionPeriodLink) (core.Result, error) {
	if !core.ValidID(in.CostID) || validBase(Base{ExpectedVersion: in.CostVersion, RevisionID: in.CostRevisionID, ContentHash: in.CostContentHash, Reason: in.Reason}) != nil {
		return core.Result{}, core.Invalid("cost_id", "请固定准确实际费用版本、修订与摘要")
	}
	return s.mutateSubscriptionPeriod(ctx, cmd, id, in.SubscriptionPeriodBase, []string{in.CostID}, func(tx pgx.Tx, v Subscription, p SubscriptionPeriod) (core.Result, error) {
		c, err := metadata(ctx, tx, cmd.Actor.OrgID, in.CostID)
		if err != nil {
			return core.Result{}, err
		}
		if c.Version != in.CostVersion || c.CurrentRevisionID != strings.ToLower(in.CostRevisionID) || c.ContentHash != in.CostContentHash {
			return core.Result{}, core.Conflict()
		}
		if !slices.Contains([]string{"forecast", "confirmed"}, p.RecordedState) || c.State != "confirmed" || c.EntryKind != "expense" {
			return core.Result{}, core.Fail(409, "BILLING_COST_NOT_CONFIRMED", "请先核对已确认且未冲销的真实费用，并重新开启已作废账期")
		}
		if c.Currency == nil || *c.Currency != p.Currency || c.Counterparty == nil || c.Counterparty.PartyID != p.Supplier.PartyID {
			return core.Result{}, core.Invalid("cost_id", "实际费用币种与供应方须和固定账期一致")
		}
		var adequate bool
		if err = tx.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", v.Classification, c.Classification).Scan(&adequate); err != nil {
			return core.Result{}, err
		}
		if !adequate || (v.ProjectID != nil && !slices.Contains(projectIDs(c, c.Content), *v.ProjectID)) {
			return core.Result{}, core.Invalid("cost_id", "实际费用须覆盖订阅项目，且分类不得降低")
		}
		var count int
		var linked bool
		if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM cost_links WHERE org_id=$1 AND source_type='subscription_period' AND source_id=$2),(SELECT EXISTS(SELECT 1 FROM cost_links WHERE org_id=$1 AND cost_id=$3 AND source_type='subscription_period'))`, cmd.Actor.OrgID, p.ID, c.ID).Scan(&count, &linked); err != nil {
			return core.Result{}, err
		}
		if linked {
			return core.Result{}, core.Fail(409, "COST_SUBSCRIPTION_ALREADY_LINKED", "该费用已经归属一个订阅账期，不可重复计入")
		}
		if count >= 100 {
			return core.Result{}, core.Invalid("cost_id", "单一账期最多100项规范费用")
		}
		linkID := core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO cost_links(id,org_id,cost_id,source_type,source_id,revision_id,relation,actor_id,reason) VALUES($1,$2,$3,'subscription_period',$4,$5,'subscription_charge',$6,$7)`, linkID, cmd.Actor.OrgID, c.ID, p.ID, p.CurrentRevisionID, cmd.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		c.Version++
		if _, err = tx.Exec(ctx, "UPDATE actual_cost_entries SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, c.ID); err != nil {
			return core.Result{}, err
		}
		if err = event(ctx, tx, cmd.Actor, c, "link_added", in.Reason, &linkID); err != nil {
			return core.Result{}, err
		}
		p.RecordedState = "forecast"
		p.ReviewReason = nil
		out, err := s.finishSubscriptionPeriod(ctx, tx, cmd.Actor, v, p, SubscriptionPeriodEvent{Kind: "cost_linked", CostLinkID: &linkID, Reason: in.Reason})
		out.Events = append(out.Events, result(c, "link.added", in.Reason, 200).Event)
		return out, err
	})
}
func (s *Service) DecideSubscriptionPeriod(ctx context.Context, cmd core.Command, id string, in SubscriptionPeriodDecision) (core.Result, error) {
	if len(in.EvidenceVersionIDs) > 20 {
		return core.Result{}, core.Invalid("evidence_version_ids", "单次核对最多20项固定凭证")
	}
	evidence := append([]string{}, in.EvidenceVersionIDs...)
	for i, id := range evidence {
		if !core.ValidID(id) {
			return core.Result{}, core.Invalid("evidence_version_ids", "核对凭证标识无效")
		}
		evidence[i] = strings.ToLower(id)
	}
	slices.Sort(evidence)
	if len(slices.Compact(append([]string{}, evidence...))) != len(evidence) {
		return core.Result{}, core.Invalid("evidence_version_ids", "核对凭证不得重复")
	}
	return s.mutateSubscriptionPeriod(ctx, cmd, id, in.SubscriptionPeriodBase, nil, func(tx pgx.Tx, v Subscription, p SubscriptionPeriod) (core.Result, error) {
		if _, _, err := references(ctx, tx, cmd.Actor, Cost{ProjectID: v.ProjectID, Classification: v.Classification}, Content{EvidenceVersionIDs: evidence}, true, true); err != nil {
			return core.Result{}, err
		}
		e := SubscriptionPeriodEvent{Reason: in.Reason, EvidenceVersionIDs: evidence, AcknowledgeDifference: in.AcknowledgeDifference, InvoiceMissing: in.InvoiceMissing}
		switch in.Action {
		case "confirm":
			if in.ExpectedActualFingerprint == nil || len(*in.ExpectedActualFingerprint) != 32 || strings.Trim(*in.ExpectedActualFingerprint, "0123456789abcdef") != "" {
				return core.Result{}, core.Invalid("expected_actual_fingerprint", "请固定本次看到的准确实际费用集合")
			}
			if !slices.Contains([]string{"forecast", "confirmed"}, p.RecordedState) || len(evidence) == 0 {
				return core.Result{}, core.Invalid("evidence_version_ids", "实际核对须保留固定凭证，已作废账期须先重新开启")
			}
			var fingerprint string
			var active int
			var equal bool
			if err := tx.QueryRow(ctx, `SELECT subscription_actual_fingerprint($1,$2),count(*) FILTER(WHERE c.state='confirmed')::int,COALESCE(sum(r.amount) FILTER(WHERE c.state='confirmed'),0)=$3::numeric FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE l.org_id=$1 AND l.source_type='subscription_period' AND l.source_id=$2`, cmd.Actor.OrgID, p.ID, p.Amount).Scan(&fingerprint, &active, &equal); err != nil {
				return core.Result{}, err
			}
			if active == 0 {
				return core.Result{}, core.Invalid("cost_ids", "核对不能把预测认作真实扣款，请先关联实际费用")
			}
			if *in.ExpectedActualFingerprint != fingerprint {
				return core.Result{}, core.Conflict()
			}
			if !equal && !in.AcknowledgeDifference {
				return core.Result{}, core.Invalid("acknowledge_difference", "实际扣款与预测不符，请明确确认差额并说明原因")
			}
			p.RecordedState = "confirmed"
			e.Kind = "confirmed"
			e.ReviewFingerprint = &fingerprint
			p.ReviewReason = &in.Reason
		case "waive", "void":
			var linked bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cost_links WHERE org_id=$1 AND source_type='subscription_period' AND source_id=$2)", cmd.Actor.OrgID, p.ID).Scan(&linked); err != nil {
				return core.Result{}, err
			}
			if p.RecordedState != "forecast" || linked {
				return core.Result{}, core.Fail(409, "BILLING_ACTUAL_EXISTS", "存在真实费用的账期不能免付或作废；请先按真实费用更正流程处理")
			}
			p.RecordedState = map[string]string{"waive": "waived", "void": "void"}[in.Action]
			e.Kind = p.RecordedState
			p.ReviewReason = &in.Reason
		case "reopen":
			if !slices.Contains([]string{"confirmed", "waived", "void"}, p.RecordedState) {
				return core.Result{}, core.Conflict()
			}
			p.RecordedState = "forecast"
			e.Kind = "reopened"
			p.ReviewReason = nil
		default:
			return core.Result{}, core.Invalid("action", "账期核对操作无效")
		}
		if in.Action != "confirm" && (in.AcknowledgeDifference || in.InvoiceMissing || in.ExpectedActualFingerprint != nil) {
			return core.Result{}, core.Invalid("action", "差额与缺发票标记只用于实际扣款核对")
		}
		return s.finishSubscriptionPeriod(ctx, tx, cmd.Actor, v, p, e)
	})
}
