package agent

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

type TelegramDisclosurePreview struct {
	RequestID    string `json:"request_id"`
	RunID        string `json:"run_id"`
	RunVersion   int64  `json:"run_version"`
	Body         string `json:"body"`
	BodySHA256   string `json:"body_sha256"`
	TotalChars   int    `json:"total_chars"`
	ChannelLabel string `json:"channel_label"`
	ChatID       string `json:"chat_id"`
	TopicID      int64  `json:"topic_id"`
}
type TelegramDisclosureInput struct {
	ExpectedVersion  int64  `json:"expected_version"`
	BodySHA256       string `json:"body_sha256"`
	StartChar        int    `json:"start_char"`
	MaxChars         int    `json:"max_chars"`
	Reviewed         bool   `json:"reviewed"`
	DisclosedToGroup bool   `json:"disclosed_to_group"`
}
type TelegramDisclosureReceipt struct {
	ID         string `json:"id"`
	DeliveryID string `json:"delivery_id"`
	RunID      string `json:"run_id"`
	BodySHA256 string `json:"body_sha256"`
	StartChar  int    `json:"start_char"`
	MaxChars   int    `json:"max_chars"`
}

// Reused by the outbound worker immediately before freezing a send attempt.
// It checks the current human session and every observed result dependency.
func TelegramHandoffResult(ctx context.Context, tx pgx.Tx, a core.Actor, requestID string) (TelegramDisclosurePreview, error) {
	var out TelegramDisclosurePreview
	if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	if !core.ValidID(requestID) {
		return out, core.Missing()
	}
	err := tx.QueryRow(ctx, `SELECT h.run_id,ch.label,i.chat_id,i.topic_id FROM telegram_human_handoffs h JOIN telegram_inbox i ON i.org_id=h.org_id AND i.id=h.inbox_id JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id WHERE h.org_id=$1 AND h.inbox_id=$2 AND h.actor_id=$3`, a.OrgID, requestID, a.UserID).Scan(&out.RunID, &out.ChannelLabel, &out.ChatID, &out.TopicID)
	if err != nil {
		return out, core.Missing()
	}
	r, err := NotificationRun(ctx, tx, a, out.RunID)
	if err != nil {
		return out, err
	}
	if r.ActorID != a.UserID || r.State != "succeeded" || r.Outcome == nil || *r.Outcome != "completed" {
		return out, core.Fail(409, "TELEGRAM_RESULT_NOT_COMPLETED", "本次人工授权任务尚未有已完成的准确结果")
	}
	c, err := metadata(ctx, tx, a.OrgID, r.ConversationID, false)
	if err != nil {
		return out, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT body,source_snapshots FROM agent_run_results WHERE org_id=$1 AND run_id=$2", a.OrgID, r.ID).Scan(&out.Body, &raw); err != nil {
		return out, err
	}
	var snapshots []SourceSnapshot
	if json.Unmarshal(raw, &snapshots) != nil {
		return out, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	allowed, err := snapshotsReadable(ctx, tx, a, c, snapshots)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, core.Missing()
	}
	out.RequestID = requestID
	out.RunVersion = r.Version
	out.BodySHA256 = core.Digest([]byte(out.Body))
	out.TotalChars = len([]rune(out.Body))
	return out, nil
}

func (s *Service) TelegramDisclosurePreview(ctx context.Context, a core.Actor, id string) (TelegramDisclosurePreview, error) {
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return TelegramDisclosurePreview{}, err
	}
	defer tx.Rollback(ctx)
	return TelegramHandoffResult(ctx, tx, a, id)
}

func (s *Service) ApproveTelegramDisclosure(ctx context.Context, c core.Command, id string, in TelegramDisclosureInput) (core.Result, error) {
	if !core.ValidID(id) || core.RequireVersion(in.ExpectedVersion) != nil || !oaDigestPattern.MatchString(in.BodySHA256) || !in.Reviewed || !in.DisclosedToGroup || in.StartChar < 0 || in.MaxChars < 1 || in.MaxChars > 2000 {
		return core.Result{}, core.Invalid("disclosure", "请核对准确结果、限定片段及向原群公开的影响")
	}
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "telegram.handoff.disclose")
	}, func(tx pgx.Tx) (core.Result, error) {
		v, err := TelegramHandoffResult(ctx, tx, c.Actor, id)
		if err != nil {
			return core.Result{}, err
		}
		if v.RunVersion != in.ExpectedVersion || v.BodySHA256 != in.BodySHA256 || in.StartChar > v.TotalChars || in.MaxChars > v.TotalChars-in.StartChar {
			return core.Result{}, core.Conflict()
		}
		out := TelegramDisclosureReceipt{ID: core.NewID(), DeliveryID: core.NewID(), RunID: v.RunID, BodySHA256: v.BodySHA256, StartChar: in.StartChar, MaxChars: in.MaxChars}
		_, err = tx.Exec(ctx, `INSERT INTO telegram_handoff_reviews(id,org_id,handoff_id,run_id,run_version,body_sha256,start_char,max_chars,actor_id,session_id,actor_auth_version,organization_auth_version,reviewed,disclosed_to_group)
 SELECT $1,$2,h.id,h.run_id,$4,$5,$6,$7,$8,$9,$10,$11,true,true FROM telegram_human_handoffs h WHERE h.org_id=$2 AND h.inbox_id=$3`, out.ID, c.Actor.OrgID, id, v.RunVersion, v.BodySHA256, in.StartChar, in.MaxChars, c.Actor.UserID, c.Actor.SessionID, c.Actor.AuthVersion, c.Actor.OrgAuthVersion)
		if err != nil {
			return core.Result{}, err
		}
		var allowed bool
		if err = tx.QueryRow(ctx, "SELECT telegram_reviewed_delivery_authorized($1,$2)", c.Actor.OrgID, id).Scan(&allowed); err != nil {
			return core.Result{}, err
		}
		if !allowed {
			return core.Result{}, core.Fail(409, "TELEGRAM_DISCLOSURE_SCOPE_CHANGED", "原群、身份、发送开关或时限已变化，请重新核对")
		}
		_, err = tx.Exec(ctx, `INSERT INTO telegram_deliveries(id,org_id,inbox_id,channel_id,bot_id,run_id,phase,state,reason_code) SELECT $1,org_id,id,channel_id,bot_id,$4,'reviewed_result','pending','human_reviewed_result' FROM telegram_inbox WHERE org_id=$2 AND id=$3`, out.DeliveryID, c.Actor.OrgID, id, v.RunID)
		if err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO telegram_delivery_events(id,org_id,delivery_id,version,state,attempts,reason_code,payload_sha256,message_id,actor_id,reason)
 SELECT $1,org_id,id,version,state,attempts,reason_code,payload_sha256,message_id,$4,'明确人工审核后公开准确片段' FROM telegram_deliveries WHERE org_id=$2 AND id=$3`, core.NewID(), c.Actor.OrgID, out.DeliveryID, c.Actor.UserID)
		return core.Result{Data: out, Event: core.Event{Type: "telegram.result_disclosure_approved", ResourceType: "telegram_request", ResourceID: id, Version: v.RunVersion, Payload: map[string]any{"review_id": out.ID, "delivery_id": out.DeliveryID, "body_sha256": v.BodySHA256, "start_char": in.StartChar, "max_chars": in.MaxChars, "disclosed_to_group": true}}}, err
	})
}
