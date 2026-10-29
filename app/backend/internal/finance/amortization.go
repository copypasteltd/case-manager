package finance

import (
	"context"
	"encoding/json"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const amortizationColumns = "id,cost_id,version,state,content_sha256,period_count,actor_id,reason,created_at"

func scanAmortization(row pgx.Row, full bool) (AmortizationVersion, error) {
	var out AmortizationVersion
	var raw []byte
	fields := []any{&out.ID, &out.CostID, &out.Version, &out.State, &out.ContentSHA256, &out.PeriodCount, &out.ActorID, &out.Reason, &out.CreatedAt}
	if full {
		fields = append(fields, &raw)
	}
	if err := row.Scan(fields...); err != nil {
		return out, platform.DatabaseError(err)
	}
	if full {
		if core.Digest(raw) != out.ContentSHA256 || json.Unmarshal(raw, &out.Content) != nil {
			return out, core.Fail(500, "AMORTIZATION_INTEGRITY_FAILED", "摊销版本内容校验失败")
		}
	}
	return out, nil
}
func amortizationVersion(ctx context.Context, tx pgx.Tx, org, cost, id string, full bool) (AmortizationVersion, error) {
	query := "SELECT " + amortizationColumns
	if full {
		query += ",content"
	}
	return scanAmortization(tx.QueryRow(ctx, query+" FROM cost_amortization_versions WHERE org_id=$1 AND cost_id=$2 AND id=$3", org, cost, id), full)
}
func amortizationCost(ctx context.Context, tx pgx.Tx, a core.Actor, id string, manage, mfa bool) (Cost, error) {
	c, err := metadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return c, err
	}
	if err = readable(ctx, tx, a, c, c.CurrentRevisionID); err != nil {
		return c, err
	}
	if manage {
		if err = identity.RequireInteractiveSession(ctx, tx, a, mfa); err != nil {
			return c, err
		}
		if err = authz.Require(ctx, tx, a, "cost.amortize", resource(c)); err != nil {
			return c, err
		}
	}
	return c, nil
}
func currentAmortization(ctx context.Context, tx pgx.Tx, org, cost string) (int64, *AmortizationVersion, error) {
	var id string
	err := tx.QueryRow(ctx, "SELECT id FROM cost_amortization_versions WHERE org_id=$1 AND cost_id=$2 ORDER BY version DESC LIMIT 1", org, cost).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	v, err := amortizationVersion(ctx, tx, org, cost, id, true)
	return v.Version, &v, err
}
func (s *Service) Amortization(ctx context.Context, a core.Actor, id string) (AmortizationOverview, error) {
	out := AmortizationOverview{CostID: id}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = reportIdentity(ctx, tx, a); err != nil {
		return out, err
	}
	c, err := amortizationCost(ctx, tx, a, id, false, false)
	if err != nil {
		return out, err
	}
	out.CanManage = c.EntryKind == "expense" && c.State != "draft" && writable(ctx, tx, a, c) && identity.RequireInteractiveSession(ctx, tx, a, false) == nil && authz.Require(ctx, tx, a, "cost.amortize", resource(c)) == nil
	out.Version, out.Current, err = currentAmortization(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) AmortizationVersions(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[AmortizationVersion], error) {
	out := core.Page[AmortizationVersion]{Items: []AmortizationVersion{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100个版本")
	}
	binding := core.CursorBinding(a, "cost-amortization", id)
	var at any
	var before any
	if cursor != "" {
		p, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		at, before = p.UpdatedAt, p.ID
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = reportIdentity(ctx, tx, a); err != nil {
		return out, err
	}
	if _, err = amortizationCost(ctx, tx, a, id, false, false); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT "+amortizationColumns+" FROM cost_amortization_versions WHERE org_id=$1 AND cost_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5", a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanAmortization(rows, false)
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
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		next := s.Pager.Encode(platform.Cursor{UpdatedAt: last.CreatedAt, ID: last.ID, Binding: binding})
		out.NextCursor = &next
	}
	return out, tx.Commit(ctx)
}
func (s *Service) AmortizationVersion(ctx context.Context, a core.Actor, cost, id string) (AmortizationVersion, error) {
	if !core.ValidID(id) {
		return AmortizationVersion{}, core.Missing()
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return AmortizationVersion{}, err
	}
	defer tx.Rollback(ctx)
	if err = reportIdentity(ctx, tx, a); err != nil {
		return AmortizationVersion{}, err
	}
	if _, err = amortizationCost(ctx, tx, a, cost, false, false); err != nil {
		return AmortizationVersion{}, err
	}
	out, err := amortizationVersion(ctx, tx, a.OrgID, cost, id, true)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func prepareAmortization(ctx context.Context, tx pgx.Tx, c Cost, in AmortizationInput) (AmortizationPreview, error) {
	var out AmortizationPreview
	if c.State != "confirmed" || c.EntryKind != "expense" {
		return out, core.Fail(409, "COST_NOT_AMORTIZABLE", "仅已确认、未冲销的原费用可发布摊销计划")
	}
	if c.Version != in.ExpectedVersion || c.CurrentRevisionID != strings.ToLower(in.RevisionID) || c.ContentHash != in.ContentHash {
		return out, core.Conflict()
	}
	var exponent int
	if err := tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", value(c.Currency)).Scan(&exponent); err != nil {
		return out, err
	}
	content, err := amortizationSchedule(c, exponent, in)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return out, err
	}
	raw, err = core.CanonicalJSON(raw)
	if err != nil {
		return out, err
	}
	out = AmortizationPreview{PlanVersion: in.ExpectedPlanVersion + 1, ContentSHA256: core.Digest(raw), Content: content}
	return out, nil
}

// All writers use the same project-then-cost lock order as ordinary cost
// commands. Replay still checks current identity, source visibility and MFA.
func (s *Service) amortizationWrite(ctx context.Context, cmd core.Command, id string, base Base, mfa bool, fn func(pgx.Tx, Cost) (core.Result, error)) (core.Result, error) {
	if err := validBase(base); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, err := amortizationCost(ctx, tx, cmd.Actor, id, true, mfa); return err }, func(tx pgx.Tx) (core.Result, error) {
		c, err := amortizationCost(ctx, tx, cmd.Actor, id, true, mfa)
		if err != nil {
			return core.Result{}, err
		}
		if err = projects(ctx, tx, cmd.Actor, c, Content{}, true); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		c, err = amortizationCost(ctx, tx, cmd.Actor, id, true, mfa)
		if err != nil {
			return core.Result{}, err
		}
		if c.Version != base.ExpectedVersion || c.CurrentRevisionID != strings.ToLower(base.RevisionID) || c.ContentHash != base.ContentHash {
			return core.Result{}, core.Conflict()
		}
		return fn(tx, c)
	})
}
func (s *Service) PreviewAmortization(ctx context.Context, cmd core.Command, id string, in AmortizationInput) (core.Result, error) {
	return s.amortizationWrite(ctx, cmd, id, in.Base, false, func(tx pgx.Tx, c Cost) (core.Result, error) {
		version, _, err := currentAmortization(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if version != in.ExpectedPlanVersion || version < 0 {
			return core.Result{}, core.Fail(409, "AMORTIZATION_CHANGED", "摊销版本已变化，请刷新后重新预览")
		}
		out, err := prepareAmortization(ctx, tx, c, in)
		return core.Result{Status: 200, Data: out, Event: core.Event{Type: "cost.amortization.previewed", ResourceType: "cost", ResourceID: id, Version: c.Version, Payload: map[string]any{"reason": in.Reason, "content_sha256": out.ContentSHA256}}}, err
	})
}
func (s *Service) PublishAmortization(ctx context.Context, cmd core.Command, id string, in AmortizationPublish) (core.Result, error) {
	if !in.Acknowledged || len(in.PreviewSHA256) != 64 || strings.Trim(in.PreviewSHA256, "0123456789abcdef") != "" {
		return core.Result{}, core.Invalid("acknowledged", "请先预览并明确确认替换当前计划口径")
	}
	return s.amortizationWrite(ctx, cmd, id, in.Base, true, func(tx pgx.Tx, c Cost) (core.Result, error) {
		version, _, err := currentAmortization(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if version != in.ExpectedPlanVersion {
			return core.Result{}, core.Fail(409, "AMORTIZATION_CHANGED", "摊销版本已变化，请刷新并重新预览")
		}
		preview, err := prepareAmortization(ctx, tx, c, in.AmortizationInput)
		if err != nil {
			return core.Result{}, err
		}
		if preview.ContentSHA256 != in.PreviewSHA256 {
			return core.Result{}, core.Fail(409, "AMORTIZATION_PREVIEW_CHANGED", "预览与当前准确费用或分摊不一致，请重新核对")
		}
		return insertAmortization(ctx, tx, cmd.Actor, id, version+1, "active", preview.Content, in.Reason)
	})
}
func (s *Service) StopAmortization(ctx context.Context, cmd core.Command, id string, in AmortizationStop) (core.Result, error) {
	if !in.Acknowledged {
		return core.Result{}, core.Invalid("acknowledged", "请明确确认停用当前计划口径，历史版本及导出保留")
	}
	return s.amortizationWrite(ctx, cmd, id, in.Base, true, func(tx pgx.Tx, c Cost) (core.Result, error) {
		version, current, err := currentAmortization(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if current == nil || current.State != "active" || version != in.ExpectedPlanVersion {
			return core.Result{}, core.Fail(409, "AMORTIZATION_CHANGED", "当前没有相同版本的有效计划，请刷新")
		}
		return insertAmortization(ctx, tx, cmd.Actor, id, version+1, "stopped", *current.Content, in.Reason)
	})
}
func insertAmortization(ctx context.Context, tx pgx.Tx, a core.Actor, cost string, version int64, state string, content AmortizationContent, reason string) (core.Result, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return core.Result{}, err
	}
	raw, err = core.CanonicalJSON(raw)
	if err != nil {
		return core.Result{}, err
	}
	id := core.NewID()
	tag, err := tx.Exec(ctx, `INSERT INTO cost_amortization_versions(id,org_id,cost_id,version,state,content,content_sha256,period_count,actor_id,session_id,mfa_verified_at,reason)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,s.id,s.mfa_verified_at,$10 FROM sessions s WHERE s.org_id=$2 AND s.id=$11 AND s.user_id=$9`,
		id, a.OrgID, cost, version, state, raw, core.Digest(raw), len(content.Periods), a.UserID, reason, a.SessionID)
	if err != nil {
		return core.Result{}, err
	}
	if tag.RowsAffected() != 1 {
		return core.Result{}, core.Forbidden()
	}
	if state == "active" {
		for _, p := range content.Periods {
			entry := core.NewID()
			if _, err = tx.Exec(ctx, `INSERT INTO cost_amortization_entries(id,reversal_id,org_id,plan_id,seq,service_from,service_to,recognized_on,amount) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, entry, core.NewID(), a.OrgID, id, p.Sequence, p.ServiceFrom, p.ServiceTo, p.RecognizedOn, p.Amount); err != nil {
				return core.Result{}, err
			}
			for i, share := range p.Shares {
				if _, err = tx.Exec(ctx, "INSERT INTO cost_amortization_shares(org_id,entry_id,position,project_id,amount) VALUES($1,$2,$3,$4,$5)", a.OrgID, entry, i, share.ProjectID, share.Amount); err != nil {
					return core.Result{}, err
				}
			}
		}
	}
	out, err := amortizationVersion(ctx, tx, a.OrgID, cost, id, true)
	return core.Result{Status: 201, Data: out, Event: core.Event{Type: "cost.amortization." + state, ResourceType: "cost", ResourceID: cost, Version: version, Payload: map[string]any{"reason": reason, "plan_id": id, "content_sha256": core.Digest(raw)}}}, err
}
