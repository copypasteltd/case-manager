package telegram

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Organization operations disclose routing and execution metadata, never the
// request, conversation, model output, credentials or free-form journal notes.
type Operation struct {
	Delivery
	BotID                 string    `json:"bot_id"`
	ChannelID             string    `json:"channel_id"`
	ChannelLabel          string    `json:"channel_label"`
	ChatID                string    `json:"chat_id"`
	TopicID               int64     `json:"topic_id"`
	RequesterID           string    `json:"requester_id"`
	RequestState          string    `json:"request_state"`
	RequestChannelVersion int64     `json:"request_channel_version"`
	CurrentChannelVersion int64     `json:"current_channel_version"`
	ChannelState          string    `json:"channel_state"`
	DeliveryEnabled       bool      `json:"delivery_enabled"`
	ReceivedAt            time.Time `json:"received_at"`
	RetryAuthorized       bool      `json:"retry_authorized"`
}

type OperationEvent struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	ReasonCode string    `json:"reason_code"`
	MessageID  *int64    `json:"message_id"`
	ActorID    *string   `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type OperationDetails struct {
	Item   Operation        `json:"item"`
	Events []OperationEvent `json:"events"`
}

type OperationReconcile struct {
	DeliveryReconcile
	ReviewedOriginalChannel bool `json:"reviewed_original_channel"`
	DuplicateRiskAccepted   bool `json:"duplicate_risk_accepted"`
}

func operationsAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, write); err != nil {
		return err
	}
	if err := authz.RequireOrganization(ctx, tx, a, "telegram.operations.read"); err != nil {
		return err
	}
	if write {
		return authz.RequireOrganization(ctx, tx, a, "telegram.operations.reconcile")
	}
	return nil
}

const operationSelect = `SELECT d.id,d.inbox_id,d.run_id,d.phase,d.state,d.version,d.attempts,d.reason_code,d.next_attempt_at,d.message_id,d.created_at,d.updated_at,
 d.bot_id,d.channel_id,c.label,i.chat_id,i.topic_id,i.requester_id,i.state,i.channel_version,c.version,c.state,c.delivery_enabled,i.received_at,
 (d.state='delivery_unknown' AND d.attempts<5 AND i.received_at>clock_timestamp()-interval '24 hours' AND telegram_delivery_authorized(d.org_id,d.inbox_id))
 FROM telegram_deliveries d JOIN telegram_inbox i ON i.org_id=d.org_id AND i.id=d.inbox_id
 JOIN telegram_channels c ON c.org_id=d.org_id AND c.id=d.channel_id`

func scanOperation(row pgx.Row) (Operation, error) {
	var v Operation
	err := row.Scan(&v.ID, &v.InboxID, &v.RunID, &v.Phase, &v.State, &v.Version, &v.Attempts, &v.ReasonCode, &v.NextAttemptAt, &v.MessageID, &v.CreatedAt, &v.UpdatedAt,
		&v.BotID, &v.ChannelID, &v.ChannelLabel, &v.ChatID, &v.TopicID, &v.RequesterID, &v.RequestState, &v.RequestChannelVersion, &v.CurrentChannelVersion, &v.ChannelState, &v.DeliveryEnabled, &v.ReceivedAt, &v.RetryAuthorized)
	return v, platform.DatabaseError(err)
}

func (s *Service) Operations(ctx context.Context, a core.Actor, state, channel, cursor string, limit int) (core.Page[Operation], error) {
	out := core.Page[Operation]{Items: []Operation{}}
	if (state != "" && !slices.Contains([]string{"pending", "sending", "retry_wait", "sent", "delivery_unknown", "confirmed_sent", "failed", "superseded", "suppressed", "abandoned"}, state)) || (channel != "" && !core.ValidID(channel)) {
		return out, core.Invalid("filters", "请核对通知状态和群入口编号")
	}
	if limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("page", "请核对分页参数")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = operationsAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	// Creation ordering is immutable even while workers update a delivery.
	binding := core.CursorBinding(a, "telegram-operations", state, channel)
	pager := platform.Pager{Key: s.Config.CursorKey}
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		p, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, beforeID = &p.UpdatedAt, &p.ID
	}
	rows, err := tx.Query(ctx, operationSelect+` WHERE d.org_id=$1 AND ($2='' OR d.state=$2) AND ($3::uuid IS NULL OR d.channel_id=$3)
 AND ($4::timestamptz IS NULL OR (d.created_at,d.id)<($4,$5::uuid)) ORDER BY d.created_at DESC,d.id DESC LIMIT $6`, a.OrgID, state, emptyID(channel), before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanOperation(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		next := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}

func emptyID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func (s *Service) Operation(ctx context.Context, a core.Actor, id string) (OperationDetails, error) {
	out := OperationDetails{Events: []OperationEvent{}}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = operationsAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	// Hold the exact head while reading its append-only journal.
	out.Item, err = scanOperation(tx.QueryRow(ctx, operationSelect+` WHERE d.org_id=$1 AND d.id=$2 FOR SHARE OF d`, a.OrgID, id))
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT id,version,state,attempts,reason_code,message_id,actor_id,created_at FROM telegram_delivery_events WHERE org_id=$1 AND delivery_id=$2 ORDER BY version`, a.OrgID, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e OperationEvent
		if err = rows.Scan(&e.ID, &e.Version, &e.State, &e.Attempts, &e.ReasonCode, &e.MessageID, &e.ActorID, &e.CreatedAt); err != nil {
			return out, err
		}
		out.Events = append(out.Events, e)
	}
	return out, rows.Err()
}

func (s *Service) ReconcileOperation(ctx context.Context, c core.Command, id string, in OperationReconcile) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil || !in.ReviewedOriginalChannel ||
		!slices.Contains([]string{"confirm_sent", "retry_confirmed_not_sent", "abandon"}, in.Action) ||
		(in.Action == "confirm_sent" && (in.MessageID == nil || *in.MessageID <= 0 || *in.MessageID > 9007199254740991)) ||
		(in.Action != "confirm_sent" && in.MessageID != nil) || in.DuplicateRiskAccepted != (in.Action == "retry_confirmed_not_sent") {
		return core.Result{}, core.Invalid("reconciliation", "请核对处理、原群核对确认、重复风险确认和原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return operationsAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		d, err := scanDelivery(tx.QueryRow(ctx, "SELECT "+deliveryColumns+" FROM telegram_deliveries WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if d.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "投递记录已更新，请重新核对")
		}
		if d.State != "delivery_unknown" {
			return core.Result{}, core.Fail(409, "DELIVERY_NOT_UNKNOWN", "仅结果未知的通知需要人工对账")
		}
		state, code := "abandoned", "manual_abandoned"
		if in.Action == "confirm_sent" {
			state, code = "confirmed_sent", "manual_confirmed_sent"
		}
		if in.Action == "retry_confirmed_not_sent" {
			if d.Attempts >= DeliveryMaxAttempts {
				return core.Result{}, core.Fail(409, "DELIVERY_RETRY_LIMIT", "已达到此通知的发送次数上限")
			}
			var allowed bool
			if err = tx.QueryRow(ctx, `SELECT telegram_delivery_authorized(org_id,id) AND received_at>clock_timestamp()-interval '24 hours' FROM telegram_inbox WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, d.InboxID).Scan(&allowed); err != nil {
				return core.Result{}, err
			}
			if !allowed {
				return core.Result{}, core.Fail(409, "TELEGRAM_SCOPE_CHANGED", "原群、本人绑定或有效期已变化，不可重发")
			}
			state, code = "retry_wait", "manual_retry"
		}
		d, err = scanDelivery(tx.QueryRow(ctx, `UPDATE telegram_deliveries SET state=$3,reason_code=$4,message_id=$5,next_attempt_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING `+deliveryColumns, c.Actor.OrgID, id, state, code, in.MessageID))
		if err != nil {
			return core.Result{}, err
		}
		note := strings.TrimSpace(in.Reason)
		if err = deliveryEvent(ctx, tx, c.Actor.OrgID, id, &c.Actor.UserID, &note); err != nil {
			return core.Result{}, err
		}
		result := eventResult(d, "telegram.operation_reconciled", id, note, d.Version, false)
		result.Event.Payload["reviewed_original_channel"] = true
		result.Event.Payload["duplicate_risk_accepted"] = in.DuplicateRiskAccepted
		return result, nil
	})
}
