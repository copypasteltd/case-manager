package telegram

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const ownRequestSQL = `SELECT i.id,i.bot_id,i.channel_id,i.channel_version,i.state,i.reason_code,i.request_text,i.received_at,i.requested_skill_id,l.conversation_id,l.run_id,r.state FROM telegram_inbox i LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id WHERE i.org_id=$1 AND i.requester_id=$2 AND i.kind='request' AND i.id=$3`

func ownRequest(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Request, error) {
	var v Request
	if !core.ValidID(id) {
		return v, core.Missing()
	}
	err := tx.QueryRow(ctx, ownRequestSQL, a.OrgID, a.UserID, id).Scan(&v.ID, &v.BotID, &v.ChannelID, &v.ChannelVersion, &v.State, &v.ReasonCode, &v.Text, &v.ReceivedAt, &v.RequestedSkillID, &v.ConversationID, &v.RunID, &v.RunState)
	return v, platform.DatabaseError(err)
}
func (s *Service) Request(ctx context.Context, a core.Actor, id string) (Request, error) {
	tx, err := s.snapshot(ctx, a, false)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	return ownRequest(ctx, tx, a, id)
}
func (s *Service) DeliveryDetails(ctx context.Context, a core.Actor, id string) (DeliveryDetails, error) {
	out := DeliveryDetails{Deliveries: []Delivery{}, Events: []DeliveryEvent{}}
	tx, err := s.snapshot(ctx, a, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = ownRequest(ctx, tx, a, id); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT "+deliveryColumns+" FROM telegram_deliveries WHERE org_id=$1 AND inbox_id=$2 ORDER BY created_at,id", a.OrgID, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Deliveries = append(out.Deliveries, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, `SELECT e.id,e.delivery_id,e.version,e.state,e.attempts,e.reason_code,e.message_id,e.actor_id,e.reason,e.created_at
 FROM telegram_delivery_events e JOIN telegram_deliveries d ON d.org_id=e.org_id AND d.id=e.delivery_id WHERE d.org_id=$1 AND d.inbox_id=$2 ORDER BY d.created_at,d.id,e.version`, a.OrgID, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e DeliveryEvent
		if err = rows.Scan(&e.ID, &e.DeliveryID, &e.Version, &e.State, &e.Attempts, &e.ReasonCode, &e.MessageID, &e.ActorID, &e.Reason, &e.CreatedAt); err != nil {
			return out, err
		}
		out.Events = append(out.Events, e)
	}
	return out, rows.Err()
}
func (s *Service) ReconcileDelivery(ctx context.Context, c core.Command, requestID, id string, in DeliveryReconcile) (core.Result, error) {
	if !core.ValidID(requestID) || !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil ||
		(in.Action != "confirm_sent" && in.Action != "retry_confirmed_not_sent" && in.Action != "abandon") ||
		(in.Action == "confirm_sent" && (in.MessageID == nil || *in.MessageID <= 0 || *in.MessageID > 9007199254740991)) || (in.Action != "confirm_sent" && in.MessageID != nil) {
		return core.Result{}, core.Invalid("reconciliation", "请核对操作、消息编号和对账原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, false, true) }, func(tx pgx.Tx) (core.Result, error) {
		if _, err := ownRequest(ctx, tx, c.Actor, requestID); err != nil {
			return core.Result{}, err
		}
		d, err := scanDelivery(tx.QueryRow(ctx, "SELECT "+deliveryColumns+" FROM telegram_deliveries WHERE org_id=$1 AND inbox_id=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, requestID, id))
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if d.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "投递记录已更新，请重新读取")
		}
		if d.State != "delivery_unknown" {
			return core.Result{}, core.Fail(409, "DELIVERY_NOT_UNKNOWN", "当前投递无需人工对账")
		}
		state, reason := "abandoned", "manual_abandoned"
		if in.Action == "confirm_sent" {
			state, reason = "confirmed_sent", "manual_confirmed_sent"
		}
		if in.Action == "retry_confirmed_not_sent" {
			if d.Attempts >= DeliveryMaxAttempts {
				return core.Result{}, core.Fail(409, "DELIVERY_RETRY_LIMIT", "已达到本通知的发送次数上限")
			}
			var allowed bool
			if err = tx.QueryRow(ctx, "SELECT telegram_delivery_authorized($1,$2)", c.Actor.OrgID, requestID).Scan(&allowed); err != nil {
				return core.Result{}, err
			}
			if !allowed {
				return core.Result{}, core.Fail(409, "TELEGRAM_SCOPE_CHANGED", "群入口或绑定范围已改变，不可重发")
			}
			state, reason = "retry_wait", "manual_retry"
		}
		d, err = scanDelivery(tx.QueryRow(ctx, `UPDATE telegram_deliveries SET state=$3,reason_code=$4,message_id=$5,next_attempt_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING `+deliveryColumns, c.Actor.OrgID, id, state, reason, in.MessageID))
		if err != nil {
			return core.Result{}, err
		}
		note := strings.TrimSpace(in.Reason)
		if err = deliveryEvent(ctx, tx, c.Actor.OrgID, id, &c.Actor.UserID, &note); err != nil {
			return core.Result{}, err
		}
		return eventResult(d, "telegram.delivery_reconciled", id, note, d.Version, false), nil
	})
}
