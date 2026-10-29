package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
)

func event(ctx context.Context, tx pgx.Tx, a core.Actor, c Cost, kind, reason string, link *string) error {
	tag, err := tx.Exec(ctx, `INSERT INTO cost_events(id,org_id,cost_id,cost_version,revision_id,kind,state,link_id,actor_id,session_id,mfa_verified_at,reason)
        SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,s.id,s.mfa_verified_at,$10 FROM sessions s WHERE s.org_id=$2 AND s.id=$11 AND s.user_id=$9`, core.NewID(), a.OrgID, c.ID, c.Version, c.CurrentRevisionID, kind, c.State, link, a.UserID, reason, a.SessionID)
	if err == nil && tag.RowsAffected() != 1 {
		return core.Forbidden()
	}
	return err
}
func result(c Cost, kind, reason string, status int) core.Result {
	return core.Result{Status: status, Data: c, Event: core.Event{Type: "cost." + kind, ResourceType: "cost", ResourceID: c.ID, Version: c.Version, Payload: map[string]any{"reason": reason, "revision_id": c.CurrentRevisionID}}}
}
func (s *Service) replay(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var old Cost
	if e = json.Unmarshal(raw, &old); e != nil {
		return core.Result{}, e
	}
	current, e := s.Get(ctx, a, old.ID)
	if e != nil {
		return core.Result{}, e
	}
	old, e = present(ctx, s.DB, a, old)
	if e != nil {
		return core.Result{}, e
	}
	if current.Version != old.Version {
		old.CanEdit = false
		old.CanConfirm = false
		old.CanReverse = false
		old.CanLink = false
	}
	out.Data = old
	return out, nil
}
func (s *Service) Create(ctx context.Context, cmd core.Command, in Create) (out core.Result, err error) {
	defer func() { out, err = s.replay(ctx, cmd.Actor, out, err) }()
	in.SourceKey = strings.TrimSpace(in.SourceKey)
	if !sourcePattern.MatchString(in.SourceKey) || strings.HasPrefix(in.SourceKey, "reversal:") || !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) || core.Name(in.Reason) != nil {
		return out, core.Invalid("source_key", "请提供永久来源标识、有效范围、分类及登记原因")
	}
	c := Cost{ID: core.NewID(), Version: 1, SourceKey: in.SourceKey, ProjectID: in.ProjectID, CreatorID: cmd.Actor.UserID, Classification: in.Classification, EntryKind: "expense", State: "draft", CurrentRevisionID: core.NewID()}
	authorize := func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, false); err != nil {
			return err
		}
		for _, action := range []string{"cost.read", "cost.record"} {
			if err := authz.Require(ctx, tx, cmd.Actor, action, resource(c)); err != nil {
				return err
			}
		}
		return nil
	}
	return s.Commands.Do(ctx, cmd, authorize, func(tx pgx.Tx) (core.Result, error) {
		content, e := normalize(ctx, tx, in.Content, false)
		if e != nil {
			return core.Result{}, e
		}
		if e = projects(ctx, tx, cmd.Actor, c, content, true); e != nil {
			return core.Result{}, e
		}
		if _, _, e = references(ctx, tx, cmd.Actor, c, content, true, false); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO actual_cost_entries(id,org_id,source_key,project_id,creator_id,classification,entry_kind,state,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,'expense','draft',$7)`, c.ID, cmd.Actor.OrgID, c.SourceKey, c.ProjectID, c.CreatorID, c.Classification, c.CurrentRevisionID); e != nil {
			return core.Result{}, e
		}
		if e = insertRevision(ctx, tx, cmd.Actor, c, c.CurrentRevisionID, 1, nil, content, in.Reason); e != nil {
			return core.Result{}, e
		}
		if e = event(ctx, tx, cmd.Actor, c, "draft_created", in.Reason, nil); e != nil {
			return core.Result{}, e
		}
		c, e = metadata(ctx, tx, cmd.Actor.OrgID, c.ID)
		if e != nil {
			return core.Result{}, e
		}
		c, e = present(ctx, tx, cmd.Actor, c)
		return result(c, "draft.created", in.Reason, 201), e
	})
}
func (s *Service) mutate(ctx context.Context, cmd core.Command, id, action string, base Base, extra Content, pre func(pgx.Tx, Cost) error, fn func(pgx.Tx, Cost) (core.Result, error)) (out core.Result, err error) {
	defer func() { out, err = s.replay(ctx, cmd.Actor, out, err) }()
	if e := validBase(base); e != nil {
		return out, e
	}
	check := func(tx pgx.Tx) (Cost, error) {
		c, e := metadata(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return c, e
		}
		if e = readable(ctx, tx, cmd.Actor, c, c.CurrentRevisionID); e != nil {
			return c, e
		}
		if e = authz.Require(ctx, tx, cmd.Actor, action, resource(c)); e != nil {
			return c, e
		}
		return c, identity.RequireInteractiveSession(ctx, tx, cmd.Actor, action != "cost.record" || pre != nil)
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		c, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = projects(ctx, tx, cmd.Actor, c, extra, true); e != nil {
			return core.Result{}, e
		}
		if pre != nil {
			if e = pre(tx, c); e != nil {
				return core.Result{}, e
			}
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		c, e = check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if c.Version != base.ExpectedVersion || c.CurrentRevisionID != strings.ToLower(base.RevisionID) || c.ContentHash != base.ContentHash {
			return core.Result{}, core.Conflict()
		}
		before, _ := json.Marshal(c)
		out, e := fn(tx, c)
		out.Event.BeforeDigest = core.Digest(before)
		return out, e
	})
}
func (s *Service) finish(ctx context.Context, tx pgx.Tx, a core.Actor, c Cost, kind, reason string, link *string) (core.Result, error) {
	c.Version++
	if _, err := tx.Exec(ctx, "UPDATE actual_cost_entries SET state=$3,current_revision_id=$4,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", a.OrgID, c.ID, c.State, c.CurrentRevisionID); err != nil {
		return core.Result{}, err
	}
	if err := event(ctx, tx, a, c, kind, reason, link); err != nil {
		return core.Result{}, err
	}
	v, err := metadata(ctx, tx, a.OrgID, c.ID)
	if err != nil {
		return core.Result{}, err
	}
	v, err = present(ctx, tx, a, v)
	return result(v, strings.ReplaceAll(kind, "_", "."), reason, 200), err
}
func (s *Service) Update(ctx context.Context, cmd core.Command, id string, in Patch) (core.Result, error) {
	return s.mutate(ctx, cmd, id, "cost.record", in.Base, in.Content, nil, func(tx pgx.Tx, c Cost) (core.Result, error) {
		if c.State != "draft" {
			return core.Result{}, core.Fail(409, "COST_IMMUTABLE", "已确认费用须通过冲销更正")
		}
		v, err := normalize(ctx, tx, in.Content, false)
		if err != nil {
			return core.Result{}, err
		}
		if _, _, err = references(ctx, tx, cmd.Actor, c, v, true, false); err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM cost_revisions WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, c.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		next := core.NewID()
		if err = insertRevision(ctx, tx, cmd.Actor, c, next, seq+1, &c.CurrentRevisionID, v, in.Reason); err != nil {
			return core.Result{}, err
		}
		c.CurrentRevisionID = next
		return s.finish(ctx, tx, cmd.Actor, c, "draft_revised", in.Reason, nil)
	})
}
func (s *Service) Confirm(ctx context.Context, cmd core.Command, id string, in Base) (core.Result, error) {
	return s.mutate(ctx, cmd, id, "cost.confirm", in, Content{}, nil, func(tx pgx.Tx, c Cost) (core.Result, error) {
		if c.State != "draft" {
			return core.Result{}, core.Fail(409, "COST_NOT_DRAFT", "仅草稿费用可人工确认")
		}
		if _, err := normalize(ctx, tx, c.Content, true); err != nil {
			return core.Result{}, err
		}
		if _, _, err := references(ctx, tx, cmd.Actor, c, c.Content, true, true); err != nil {
			return core.Result{}, err
		}
		c.State = "confirmed"
		return s.finish(ctx, tx, cmd.Actor, c, "confirmed", in.Reason, nil)
	})
}
func (s *Service) Reverse(ctx context.Context, cmd core.Command, id string, in Reverse) (core.Result, error) {
	if in.EffectiveOn == "" || core.Date(in.EffectiveOn) != nil {
		return core.Result{}, core.Invalid("effective_on", "请填写实际冲销生效日")
	}
	return s.mutate(ctx, cmd, id, "cost.reverse", in.Base, Content{}, nil, func(tx pgx.Tx, c Cost) (core.Result, error) {
		if c.State != "confirmed" || c.EntryKind != "expense" {
			return core.Result{}, core.Fail(409, "COST_NOT_REVERSIBLE", "仅未冲销的已确认原费用可全额冲销")
		}
		var unpaid bool
		if err := tx.QueryRow(ctx, "SELECT NOT cost_has_effective_settlement($1,$2)", cmd.Actor.OrgID, c.ID).Scan(&unpaid); err != nil {
			return core.Result{}, err
		}
		if !unpaid {
			return core.Result{}, core.Fail(409, "COST_HAS_SETTLEMENT", "请先退款、释放或冲销付款分配，再冲销原费用")
		}
		var today string
		if err := tx.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
			return core.Result{}, err
		}
		if in.EffectiveOn < *c.OccurredOn || in.EffectiveOn > today {
			return core.Result{}, core.Invalid("effective_on", "冲销日期须在原发生日与香港当前日期之间")
		}
		reversal := c
		reversal.ID = core.NewID()
		reversal.Version = 1
		reversal.SourceKey = "reversal:" + c.ID
		reversal.EntryKind = "reversal"
		reversal.ReversesID = &c.ID
		reversal.ReversalID = nil
		reversal.CurrentRevisionID = core.NewID()
		reversal.OccurredOn = &in.EffectiveOn
		if _, err := tx.Exec(ctx, `INSERT INTO actual_cost_entries(id,org_id,source_key,project_id,creator_id,classification,entry_kind,state,reverses_id,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,'reversal','confirmed',$7,$8)`, reversal.ID, cmd.Actor.OrgID, reversal.SourceKey, reversal.ProjectID, reversal.CreatorID, reversal.Classification, c.ID, reversal.CurrentRevisionID); err != nil {
			return core.Result{}, err
		}
		if err := insertRevision(ctx, tx, cmd.Actor, reversal, reversal.CurrentRevisionID, 1, nil, reversal.Content, in.Reason); err != nil {
			return core.Result{}, err
		}
		if err := event(ctx, tx, cmd.Actor, reversal, "reversal_created", in.Reason, nil); err != nil {
			return core.Result{}, err
		}
		c.State = "reversed"
		out, err := s.finish(ctx, tx, cmd.Actor, c, "reversed", in.Reason, nil)
		if err == nil {
			out.Events = []core.Event{result(reversal, "reversal.created", in.Reason, 201).Event}
		}
		return out, err
	})
}
