package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// All financial workflows acquire projects in ascending order before a
// subscription, period and cost. Halting a vendor obligation remains possible
// after project closure; closure itself cannot prove vendor cancellation.
func subscriptionProjectLocks(ctx context.Context, db platform.DBTX, a core.Actor, s Subscription, extraCosts []string, lock, allowClosed bool) error {
	ids := []string{}
	if s.ProjectID != nil {
		ids = append(ids, *s.ProjectID)
	}
	for _, id := range extraCosts {
		c, e := metadata(ctx, db, a.OrgID, id)
		if e != nil {
			return e
		}
		if e = readable(ctx, db, a, c, c.CurrentRevisionID); e != nil {
			return e
		}
		ids = append(ids, projectIDs(c, c.Content)...)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		query := "SELECT state,classification,creator_id FROM projects WHERE org_id=$1 AND id=$2"
		if lock {
			query += " FOR UPDATE"
		}
		var state, class, owner string
		if err := db.QueryRow(ctx, query, a.OrgID, id).Scan(&state, &class, &owner); err != nil {
			return platform.DatabaseError(err)
		}
		if err := authz.Require(ctx, db, a, "project.read", authz.Resource{Type: "project", ID: id, ProjectID: &id, OwnerID: &owner, Classification: class}); err != nil {
			return err
		}
		if s.ProjectID != nil && *s.ProjectID == id {
			var ok bool
			if err := db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", class, s.Classification).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return core.Invalid("classification", "订阅分类不能低于所属项目")
			}
		}
		if !allowClosed && slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
			return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许新增或调整财务安排")
		}
	}
	return nil
}
func insertSubscriptionRevision(ctx context.Context, tx pgx.Tx, a core.Actor, s Subscription, id string, seq int64, base *string, c SubscriptionContent, reason string) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subscription_revisions(id,org_id,subscription_id,seq,base_revision_id,payload,content_hash,author_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, a.OrgID, s.ID, seq, base, raw, core.Digest(raw), a.UserID, reason); err != nil {
		return err
	}
	for i, p := range c.Prices {
		if _, err = tx.Exec(ctx, `INSERT INTO subscription_prices(id,org_id,revision_id,position,effective_from,effective_to,currency,amount,plan) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, core.NewID(), a.OrgID, id, i+1, p.EffectiveFrom, p.EffectiveTo, p.Currency, p.Amount, p.Plan); err != nil {
			return err
		}
	}
	for _, file := range c.VendorEvidenceVersionIDs {
		if _, err = tx.Exec(ctx, "INSERT INTO subscription_revision_files(org_id,revision_id,version_id) VALUES($1,$2,$3)", a.OrgID, id, file); err != nil {
			return err
		}
	}
	return nil
}
func saveSubscriptionContent(ctx context.Context, tx pgx.Tx, a core.Actor, s *Subscription, c SubscriptionContent, reason string) error {
	var seq int64
	if err := tx.QueryRow(ctx, "SELECT seq FROM subscription_revisions WHERE org_id=$1 AND id=$2", a.OrgID, s.CurrentRevisionID).Scan(&seq); err != nil {
		return err
	}
	next := core.NewID()
	if err := insertSubscriptionRevision(ctx, tx, a, *s, next, seq+1, &s.CurrentRevisionID, c, reason); err != nil {
		return err
	}
	s.CurrentRevisionID = next
	s.SubscriptionContent = c
	return nil
}
func subscriptionEvent(ctx context.Context, tx pgx.Tx, a core.Actor, s Subscription, eventID, kind, reason string, period, effective *string) error {
	tag, err := tx.Exec(ctx, `INSERT INTO subscription_events(id,org_id,subscription_id,subscription_version,revision_id,kind,state,period_id,effective_on,actor_id,session_id,mfa_verified_at,reason)
        SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,id,mfa_verified_at,$11 FROM sessions WHERE org_id=$2 AND id=$12 AND user_id=$10`, eventID, a.OrgID, s.ID, s.Version, s.CurrentRevisionID, kind, s.State, period, effective, a.UserID, reason, a.SessionID)
	if err == nil && tag.RowsAffected() != 1 {
		return core.Forbidden()
	}
	return err
}
func subscriptionResult(s Subscription, kind, reason string, status int) core.Result {
	return core.Result{Status: status, Data: s, Event: core.Event{Type: "subscription." + strings.ReplaceAll(kind, "_", "."), ResourceType: "subscription", ResourceID: s.ID, Version: s.Version, Payload: map[string]any{"reason": reason, "revision_id": s.CurrentRevisionID}}}
}
func (s *Service) subscriptionReplay(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var header struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
	}
	if e = json.Unmarshal(raw, &header); e != nil {
		return core.Result{}, e
	}
	if header.SubscriptionID != "" {
		if header.ID != "" {
			var old SubscriptionPeriod
			if e = json.Unmarshal(raw, &old); e != nil {
				return core.Result{}, e
			}
			current, e := s.SubscriptionPeriod(ctx, a, header.ID)
			if e != nil {
				return core.Result{}, e
			}
			old.CanManage = current.CanManage && current.Version == old.Version
			out.Data = old
		}
		return out, nil
	} // Keep fixed monetary receipt snapshots; project current ACL and capabilities.
	var old Subscription
	if e = json.Unmarshal(raw, &old); e != nil {
		return core.Result{}, e
	}
	current, e := s.Subscription(ctx, a, old.ID)
	if e != nil {
		return core.Result{}, e
	}
	old, e = presentSubscription(ctx, s.DB, a, old)
	if e != nil {
		return core.Result{}, e
	}
	if old.Version != current.Version {
		old.CanEdit = false
		old.CanManage = false
		old.NextChargeDate = nil
	}
	out.Data = old
	return out, nil
}
func (s *Service) CreateSubscription(ctx context.Context, cmd core.Command, in SubscriptionCreate) (out core.Result, err error) {
	defer func() { out, err = s.subscriptionReplay(ctx, cmd.Actor, out, err) }()
	in.SourceKey = strings.TrimSpace(in.SourceKey)
	if !sourcePattern.MatchString(in.SourceKey) || !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) || core.Name(in.Reason) != nil {
		return out, core.Invalid("source_key", "请提供永久订阅来源、有效范围、分类及原因")
	}
	v := Subscription{ID: core.NewID(), Version: 1, SourceKey: in.SourceKey, ProjectID: in.ProjectID, CreatorID: cmd.Actor.UserID, Classification: in.Classification, State: "draft", CurrentRevisionID: core.NewID()}
	authorize := func(tx pgx.Tx) error {
		if e := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, false); e != nil {
			return e
		}
		for _, action := range []string{"subscription.read", "subscription.create"} {
			if e := authz.Require(ctx, tx, cmd.Actor, action, subscriptionResource(v)); e != nil {
				return e
			}
		}
		return nil
	}
	return s.Commands.Do(ctx, cmd, authorize, func(tx pgx.Tx) (core.Result, error) {
		c, e := normalizeSubscription(ctx, tx, in.SubscriptionContent, false)
		if e != nil {
			return core.Result{}, e
		}
		if e = subscriptionProjectLocks(ctx, tx, cmd.Actor, v, nil, true, false); e != nil {
			return core.Result{}, e
		}
		if _, _, _, _, e = subscriptionReferences(ctx, tx, cmd.Actor, v, c, true); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO subscriptions(id,org_id,source_key,project_id,creator_id,classification,state,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,'draft',$7)`, v.ID, cmd.Actor.OrgID, v.SourceKey, v.ProjectID, v.CreatorID, v.Classification, v.CurrentRevisionID); e != nil {
			return core.Result{}, e
		}
		if e = insertSubscriptionRevision(ctx, tx, cmd.Actor, v, v.CurrentRevisionID, 1, nil, c, in.Reason); e != nil {
			return core.Result{}, e
		}
		if e = subscriptionEvent(ctx, tx, cmd.Actor, v, core.NewID(), "draft_created", in.Reason, nil, nil); e != nil {
			return core.Result{}, e
		}
		v, e = subscriptionMetadata(ctx, tx, cmd.Actor.OrgID, v.ID)
		if e != nil {
			return core.Result{}, e
		}
		v, e = presentSubscription(ctx, tx, cmd.Actor, v)
		return subscriptionResult(v, "draft_created", in.Reason, 201), e
	})
}
func (s *Service) mutateSubscription(ctx context.Context, cmd core.Command, id, action string, base Base, mfa, allowClosed bool, extraCosts []string, fn func(pgx.Tx, Subscription) (core.Result, error)) (out core.Result, err error) {
	defer func() { out, err = s.subscriptionReplay(ctx, cmd.Actor, out, err) }()
	if e := validBase(base); e != nil {
		return out, core.Invalid("revision_id", "请固定准确订阅版本、修订和摘要，并说明原因")
	}
	check := func(tx pgx.Tx) (Subscription, error) {
		v, e := subscriptionMetadata(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return v, e
		}
		if e = subscriptionReadable(ctx, tx, cmd.Actor, v); e != nil {
			return v, e
		}
		if e = authz.Require(ctx, tx, cmd.Actor, action, subscriptionResource(v)); e != nil {
			return v, e
		}
		return v, identity.RequireInteractiveSession(ctx, tx, cmd.Actor, mfa || v.State != "draft")
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		v, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = subscriptionProjectLocks(ctx, tx, cmd.Actor, v, extraCosts, true, allowClosed); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM subscriptions WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		v, e = check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if v.Version != base.ExpectedVersion || v.CurrentRevisionID != strings.ToLower(base.RevisionID) || v.ContentHash != base.ContentHash {
			return core.Result{}, core.Conflict()
		}
		before, _ := json.Marshal(v)
		out, e := fn(tx, v)
		out.Event.BeforeDigest = core.Digest(before)
		return out, e
	})
}
func advanceSubscription(ctx context.Context, tx pgx.Tx, a core.Actor, v *Subscription, kind, reason string, period, effective *string) (string, error) {
	v.Version++
	if _, err := tx.Exec(ctx, `UPDATE subscriptions SET version=version+1,state=$3,current_revision_id=$4,cancel_effective_on=$5,paused_on=$6,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, a.OrgID, v.ID, v.State, v.CurrentRevisionID, v.CancelEffectiveOn, v.PausedOn); err != nil {
		return "", err
	}
	id := core.NewID()
	return id, subscriptionEvent(ctx, tx, a, *v, id, kind, reason, period, effective)
}
func (s *Service) finishSubscription(ctx context.Context, tx pgx.Tx, a core.Actor, v Subscription, kind, reason string, effective *string) (core.Result, error) {
	if _, err := advanceSubscription(ctx, tx, a, &v, kind, reason, nil, effective); err != nil {
		return core.Result{}, err
	}
	v, err := subscriptionMetadata(ctx, tx, a.OrgID, v.ID)
	if err != nil {
		return core.Result{}, err
	}
	v, err = presentSubscription(ctx, tx, a, v)
	return subscriptionResult(v, kind, reason, 200), err
}
func (s *Service) UpdateSubscription(ctx context.Context, cmd core.Command, id string, in SubscriptionPatch) (core.Result, error) {
	return s.mutateSubscription(ctx, cmd, id, "subscription.update", in.Base, false, false, nil, func(tx pgx.Tx, v Subscription) (core.Result, error) {
		if !slices.Contains([]string{"draft", "active", "paused"}, v.State) {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_IMMUTABLE", "取消或归档后不能改写订阅配置")
		}
		c, err := normalizeSubscription(ctx, tx, in.SubscriptionContent, v.State != "draft")
		if err != nil {
			return core.Result{}, err
		}
		if v.State != "draft" {
			old, _ := json.Marshal(v.Prices)
			next, _ := json.Marshal(c.Prices)
			if !bytes.Equal(old, next) || !bytes.Equal(subscriptionFixedCalendar(v.SubscriptionContent), subscriptionFixedCalendar(c)) {
				return core.Result{}, core.Fail(409, "BILLING_CALENDAR_IMMUTABLE", "已启用日历固定；调价请建立新的价格修订")
			}
		}
		if _, _, _, _, err = subscriptionReferences(ctx, tx, cmd.Actor, v, c, true); err != nil {
			return core.Result{}, err
		}
		if err = saveSubscriptionContent(ctx, tx, cmd.Actor, &v, c, in.Reason); err != nil {
			return core.Result{}, err
		}
		return s.finishSubscription(ctx, tx, cmd.Actor, v, "config_revised", in.Reason, nil)
	})
}
func (s *Service) ChangeSubscriptionPrice(ctx context.Context, cmd core.Command, id string, in SubscriptionPriceChange) (core.Result, error) {
	return s.mutateSubscription(ctx, cmd, id, "subscription.manage", in.Base, true, false, nil, func(tx pgx.Tx, v Subscription) (core.Result, error) {
		if !slices.Contains([]string{"active", "paused"}, v.State) {
			return core.Result{}, core.Fail(409, "SUBSCRIPTION_NOT_ACTIVE", "仅已启用或暂停订阅可以追加调价")
		}
		today, err := subscriptionDate(ctx, tx, v.Timezone)
		if err != nil {
			return core.Result{}, err
		}
		if in.EffectiveFrom < today {
			return core.Result{}, core.Invalid("effective_from", "调价不得回填到过去；历史差额以真实费用说明")
		}
		var exponent int
		if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", in.Currency).Scan(&exponent); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		currencies := map[string]int{in.Currency: exponent}
		for _, p := range v.Prices {
			if _, ok := currencies[p.Currency]; !ok {
				var n int
				if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", p.Currency).Scan(&n); err != nil {
					return core.Result{}, err
				}
				currencies[p.Currency] = n
			}
		}
		c := v.SubscriptionContent
		c.Prices, err = subscriptionAppendPrice(c.Prices, in.SubscriptionPriceInput, currencies)
		if err != nil {
			return core.Result{}, err
		}
		c, err = normalizeSubscription(ctx, tx, c, true)
		if err != nil {
			return core.Result{}, err
		}
		if _, _, _, _, err = subscriptionReferences(ctx, tx, cmd.Actor, v, c, true); err != nil {
			return core.Result{}, err
		}
		if err = saveSubscriptionContent(ctx, tx, cmd.Actor, &v, c, in.Reason); err != nil {
			return core.Result{}, err
		}
		return s.finishSubscription(ctx, tx, cmd.Actor, v, "price_changed", in.Reason, nil)
	})
}
func (s *Service) TransitionSubscription(ctx context.Context, cmd core.Command, id string, in SubscriptionTransition) (core.Result, error) {
	halt := in.Action == "pause" || in.Action == "cancel" || in.Action == "verify_vendor" || in.Action == "archive" || in.Action == "restore"
	return s.mutateSubscription(ctx, cmd, id, "subscription.manage", in.Base, true, halt, nil, func(tx pgx.Tx, v Subscription) (core.Result, error) {
		today, err := subscriptionDate(ctx, tx, v.Timezone)
		if err != nil {
			return core.Result{}, err
		}
		kind := ""
		var effective *string
		switch in.Action {
		case "activate":
			if v.State != "draft" {
				return core.Result{}, core.Conflict()
			}
			v.State = "active"
			kind = "activated"
		case "pause":
			if v.State != "active" {
				return core.Result{}, core.Conflict()
			}
			v.State = "paused"
			v.PausedOn = &today
			effective = &today
			kind = "paused"
		case "resume":
			if v.State != "paused" {
				return core.Result{}, core.Conflict()
			}
			v.State = "active"
			v.PausedOn = nil
			effective = &today
			kind = "resumed"
		case "cancel":
			if !slices.Contains([]string{"draft", "active", "paused"}, v.State) {
				return core.Result{}, core.Conflict()
			}
			effective = &today
			if in.EffectiveOn != nil {
				if _, e := civilDate(*in.EffectiveOn); e != nil {
					return core.Result{}, e
				}
				if *in.EffectiveOn < today {
					return core.Result{}, core.Invalid("effective_on", "取消生效日不能早于实际操作日")
				}
				effective = in.EffectiveOn
			}
			v.State = "cancelled"
			v.CancelEffectiveOn = effective
			kind = "cancelled"
		case "verify_vendor":
			if !slices.Contains([]string{"active", "paused", "cancelled"}, v.State) || in.VendorState == "" {
				return core.Result{}, core.Invalid("vendor_state", "请提供待核对的供应方实际状态")
			}
			kind = "config_revised"
		case "archive":
			if v.State != "cancelled" || v.CancelEffectiveOn == nil || *v.CancelEffectiveOn > today {
				return core.Result{}, core.Fail(409, "CANCELLATION_NOT_EFFECTIVE", "取消尚未生效，不能归档")
			}
			v.State = "archived"
			kind = "archived"
		case "restore":
			if v.State != "archived" || in.VendorState != "" || in.VendorCheckedAt != nil || len(in.VendorEvidenceVersionIDs) > 0 {
				return core.Result{}, core.Invalid("action", "归档恢复只重新开放历史核对，保留原取消日期和配置")
			}
			v.State = "cancelled"
			kind = "restored"
		default:
			return core.Result{}, core.Invalid("action", "订阅操作无效")
		}
		if in.Action != "cancel" && in.EffectiveOn != nil {
			return core.Result{}, core.Invalid("effective_on", "仅取消操作可指定生效日期，暂停与恢复按实际操作日记录")
		}
		c := v.SubscriptionContent
		if in.VendorState != "" {
			c.VendorState = in.VendorState
			c.VendorCheckedAt = in.VendorCheckedAt
			c.VendorEvidenceVersionIDs = in.VendorEvidenceVersionIDs
		} else if in.VendorCheckedAt != nil || len(in.VendorEvidenceVersionIDs) > 0 {
			return core.Result{}, core.Invalid("vendor_state", "提交供应方核对证据时须明确核对状态")
		}
		complete := slices.Contains([]string{"activate", "resume"}, in.Action)
		c, err = normalizeSubscription(ctx, tx, c, complete)
		if err != nil {
			return core.Result{}, err
		}
		if _, _, _, _, err = subscriptionReferences(ctx, tx, cmd.Actor, v, c, complete || in.VendorState != ""); err != nil {
			return core.Result{}, err
		}
		if in.Action == "archive" {
			if c.VendorState != "cancelled" {
				return core.Result{}, core.Fail(409, "VENDOR_STOP_UNVERIFIED", "归档前须用固定证据核对供应方确已取消")
			}
			var open bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=$1 AND p.subscription_id=$2 AND p.start_date<$3::date AND p.state NOT IN ('waived','void') AND (p.state<>'confirmed' OR p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(p.org_id,p.id)))`, cmd.Actor.OrgID, v.ID, v.CancelEffectiveOn).Scan(&open); err != nil {
				return core.Result{}, err
			}
			if open {
				return core.Result{}, core.Fail(409, "BILLING_REVIEW_REQUIRED", "归档前请核对取消前的全部账期及费用变更")
			}
		}
		old, _ := json.Marshal(v.SubscriptionContent)
		next, _ := json.Marshal(c)
		if !bytes.Equal(old, next) {
			if err = saveSubscriptionContent(ctx, tx, cmd.Actor, &v, c, in.Reason); err != nil {
				return core.Result{}, err
			}
		}
		return s.finishSubscription(ctx, tx, cmd.Actor, v, kind, in.Reason, effective)
	})
}
