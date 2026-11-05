package agent

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type TelegramHandoffInput struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion int64  `json:"expected_version"`
	RequestSHA256   string `json:"request_sha256"`
	Reviewed        bool   `json:"reviewed"`
}
type TelegramHandoffRequest struct {
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	RequestSHA256 string    `json:"request_sha256"`
	Text          string    `json:"text"`
	SkillID       string    `json:"skill_id"`
	ProjectID     *string   `json:"project_id"`
	ChannelLabel  string    `json:"channel_label"`
	ChatID        string    `json:"chat_id"`
	TopicID       int64     `json:"topic_id"`
	Eligible      bool      `json:"eligible"`
	ExpiresAt     time.Time `json:"expires_at"`
	RunID         *string   `json:"run_id"`
}

func (s *Service) TelegramHandoffRequest(ctx context.Context, a core.Actor, id string) (TelegramHandoffRequest, error) {
	var out TelegramHandoffRequest
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT i.id,e.version,i.request_text,i.requested_skill_id,ch.project_id,ch.label,i.chat_id,i.topic_id,i.received_at+interval '30 minutes',h.run_id,
 (i.state='awaiting_executor' AND i.received_at>clock_timestamp()-interval '30 minutes' AND ch.state='active' AND ch.version=i.channel_version
 AND b.state='active' AND b.bot_id=i.bot_id AND b.user_id=i.requester_id AND b.telegram_user_id=i.telegram_user_id AND i.actor_auth_version=$4
 AND i.requested_skill_id=ANY(ch.allowed_skill_ids) AND h.run_id IS NULL AND NOT EXISTS(SELECT 1 FROM telegram_run_links WHERE org_id=i.org_id AND inbox_id=i.id))
 FROM telegram_inbox i JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id
 JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
 JOIN LATERAL(SELECT version FROM telegram_request_events WHERE org_id=i.org_id AND inbox_id=i.id ORDER BY version DESC LIMIT 1)e ON true
 LEFT JOIN telegram_human_handoffs h ON h.org_id=i.org_id AND h.inbox_id=i.id
 WHERE i.org_id=$1 AND i.requester_id=$2 AND i.id=$3 AND i.kind='request'`, a.OrgID, a.UserID, id, a.AuthVersion).Scan(&out.ID, &out.Version, &out.Text, &out.SkillID, &out.ProjectID, &out.ChannelLabel, &out.ChatID, &out.TopicID, &out.ExpiresAt, &out.RunID, &out.Eligible)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	out.RequestSHA256 = core.Digest([]byte(out.Text))
	return out, nil
}

func recordTelegramHandoff(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in *TelegramHandoffInput) error {
	if in == nil {
		return nil
	}
	if !core.ValidID(in.RequestID) || core.RequireVersion(in.ExpectedVersion) != nil || !oaDigestPattern.MatchString(in.RequestSHA256) || !in.Reviewed {
		return core.Invalid("telegram_handoff", "请核对准确TG请求、摘要及本次人工授权")
	}
	if err := identity.RequireInteractiveSession(ctx, tx, a, true); err != nil {
		return err
	}
	if c.Type != "private" || c.CreatorID != a.UserID {
		return core.Forbidden()
	}
	var channelVersion, bindingVersion int64
	var sha string
	if err := tx.QueryRow(ctx, `SELECT ch.version,b.version,i.request_text FROM telegram_inbox i JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id WHERE i.org_id=$1 AND i.id=$2 AND i.requester_id=$3 FOR UPDATE OF i`, a.OrgID, in.RequestID, a.UserID).Scan(&channelVersion, &bindingVersion, &sha); err != nil {
		return platform.DatabaseError(err)
	}
	if core.Digest([]byte(sha)) != in.RequestSHA256 {
		return core.Conflict()
	}
	_, err := tx.Exec(ctx, `INSERT INTO telegram_human_handoffs(id,org_id,inbox_id,run_id,request_version,request_sha256,channel_version,binding_version,actor_id,actor_auth_version,organization_auth_version,session_id,reviewed)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true)`, core.NewID(), a.OrgID, in.RequestID, runID, in.ExpectedVersion, in.RequestSHA256, channelVersion, bindingVersion, a.UserID, a.AuthVersion, a.OrgAuthVersion, a.SessionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE telegram_inbox SET state='withdrawn',reason_code='human_authorized_handoff' WHERE org_id=$1 AND id=$2", a.OrgID, in.RequestID)
	return err
}

func telegramHandoffAuthority(ctx context.Context, tx pgx.Tx, st executionState) error {
	var exists, valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_human_handoffs WHERE org_id=$1 AND run_id=$2),EXISTS(
 SELECT 1 FROM telegram_human_handoffs h JOIN telegram_inbox i ON i.org_id=h.org_id AND i.id=h.inbox_id
 JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
 JOIN sessions s ON s.org_id=h.org_id AND s.id=h.session_id JOIN users u ON u.org_id=h.org_id AND u.id=h.actor_id JOIN organizations o ON o.id=h.org_id
 WHERE h.org_id=$1 AND h.run_id=$2 AND h.actor_id=$3 AND h.actor_auth_version=u.auth_version AND u.status='active' AND h.organization_auth_version=o.authorization_version
 AND s.user_id=h.actor_id AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version
 AND ch.state='active' AND ch.version=h.channel_version AND ch.version=i.channel_version AND i.requested_skill_id=ANY(ch.allowed_skill_ids)
 AND b.state='active' AND b.version=h.binding_version AND b.user_id=h.actor_id AND b.bot_id=i.bot_id AND b.telegram_user_id=i.telegram_user_id
 AND i.state='withdrawn' AND i.reason_code='human_authorized_handoff')`, st.Actor.OrgID, st.Run.ID, st.Actor.UserID).Scan(&exists, &valid)
	if err != nil {
		return err
	}
	if exists && !valid {
		return runtimeFailure("TELEGRAM_SCOPE_CHANGED")
	}
	return nil
}
