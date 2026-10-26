package telegram

import (
	"context"
	"crypto/subtle"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type inboxRecord struct {
	kind, state, reason                           string
	requester, binding, telegramUser, chat, text  *string
	authVersion, topic, messageID, channelVersion *int64
	channel                                       *string
	skill                                         *string
}

func (s *Service) prove(ctx context.Context, tx pgx.Tx, b BotConfig, m message, token string) (bool, error) {
	id, ok := tokenID(token)
	if !ok {
		return false, nil
	}
	c, err := scanChallenge(tx.QueryRow(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND bot_id=$2 AND id=$3 FOR UPDATE", b.OrgID, b.ID, id))
	if err != nil {
		if core.PublicError(platform.DatabaseError(err)).Status == 404 {
			return false, nil
		}
		return false, err
	}
	var a core.Actor
	var usable bool
	a.OrgID = b.OrgID
	err = tx.QueryRow(ctx, `SELECT c.user_id,c.session_id,c.actor_auth_version,o.authorization_version,s.device_type,
 c.expires_at>clock_timestamp() AND c.claimed_at IS NULL AND c.confirmed_at IS NULL AND c.cancelled_at IS NULL
 FROM telegram_binding_challenges c JOIN organizations o ON o.id=c.org_id JOIN sessions s ON s.org_id=c.org_id AND s.id=c.session_id WHERE c.id=$1`, id).Scan(&a.UserID, &a.SessionID, &a.AuthVersion, &a.OrgAuthVersion, &a.Channel, &usable)
	if err != nil {
		return false, err
	}
	if !usable || subtle.ConstantTimeCompare([]byte(token), []byte(s.startToken(c, b.OrgID, a.UserID, a.SessionID))) != 1 {
		return false, nil
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		if core.PublicError(err).Status < 500 {
			return false, nil
		}
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE telegram_binding_challenges SET telegram_user_id=$2,claimed_at=clock_timestamp(),version=version+1 WHERE id=$1`, id, strconv.FormatInt(m.From.ID, 10))
	return err == nil, err
}
func pauseChat(ctx context.Context, tx pgx.Tx, b BotConfig, chat int64, source string) error {
	if chat == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id FROM telegram_channels WHERE org_id=$1 AND bot_id=$2 AND chat_id=$3 AND state='active' ORDER BY id FOR UPDATE`, b.OrgID, b.ID, strconv.FormatInt(chat, 10))
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, `UPDATE telegram_channels SET state='paused',version=version+1,updated_at=clock_timestamp() WHERE id=$1`, id); err != nil {
			return err
		}
		if err = withdrawChannel(ctx, tx, b.OrgID, id, "group_review_required"); err != nil {
			return err
		}
		if err = channelRevision(ctx, tx, b.OrgID, id, nil, source, "群成员或群标识变化，需要重新核对入口范围"); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) classify(ctx context.Context, tx pgx.Tx, b BotConfig, u update) (inboxRecord, error) {
	out := inboxRecord{kind: "ignored", state: "ignored", reason: "unsupported_update"}
	if u.Membership != nil {
		chat := u.Membership.Chat
		if !numericID(strconv.FormatInt(chat.ID, 10), true) || (chat.Type != "group" && chat.Type != "supergroup") {
			return out, nil
		}
		if err := pauseChat(ctx, tx, b, chat.ID, "membership"); err != nil {
			return out, err
		}
		value := strconv.FormatInt(chat.ID, 10)
		out.kind, out.state, out.reason, out.chat = "membership", "paused", "group_review_required", &value
		return out, nil
	}
	if u.Message == nil {
		return out, nil
	}
	m := *u.Message
	if m.Chat.Type == "group" || m.Chat.Type == "supergroup" {
		if !numericID(strconv.FormatInt(m.Chat.ID, 10), true) {
			return out, nil
		}
		if len(m.NewMembers) > 0 || m.LeftMember != nil || m.MigrateFrom != 0 || m.MigrateTo != 0 {
			source := "membership"
			if m.MigrateFrom != 0 || m.MigrateTo != 0 {
				source = "migration"
			}
			// Stable ordering also covers two overlapping migration updates.
			chats := []int64{m.Chat.ID}
			if m.MigrateFrom != 0 && m.MigrateFrom != m.Chat.ID {
				chats = append(chats, m.MigrateFrom)
			}
			if len(chats) == 2 && chats[0] > chats[1] {
				chats[0], chats[1] = chats[1], chats[0]
			}
			for _, chat := range chats {
				if err := pauseChat(ctx, tx, b, chat, source); err != nil {
					return out, err
				}
			}
			value := strconv.FormatInt(m.Chat.ID, 10)
			out.kind, out.state, out.reason, out.chat = "membership", "paused", "group_review_required", &value
			return out, nil
		}
	}
	if m.From == nil || m.From.IsBot || !numericID(strconv.FormatInt(m.From.ID, 10), false) || len(m.SenderChat) > 0 || m.ID <= 0 || m.ID > 2147483647 || m.Topic < 0 || m.Topic > 2147483647 {
		out.reason = "sender_not_supported"
		return out, nil
	}
	if m.Chat.Type == "private" {
		if m.Chat.ID != m.From.ID {
			out.reason = "private_identity_mismatch"
			return out, nil
		}
		code, ok := startCode(m, b)
		if !ok {
			out.reason = "private_proof_only"
			return out, nil
		}
		proved, err := s.prove(ctx, tx, b, m, code)
		if err != nil {
			return out, err
		}
		out.reason = "binding_proof_invalid"
		if proved {
			out.kind, out.state, out.reason = "binding_proof", "binding_proved", "portal_confirmation_required"
		}
		return out, nil
	}
	if m.Chat.Type != "group" && m.Chat.Type != "supergroup" {
		return out, nil
	}
	if !triggered(m, b) {
		out.reason = "not_triggered"
		return out, nil
	}
	text := strings.TrimSpace(m.Text)
	if text == "" || utf8.RuneCountInString(text) > 4000 {
		out.reason = "text_not_supported"
		return out, nil
	}
	channel, err := scanChannel(tx.QueryRow(ctx, "SELECT "+channelColumns+" FROM telegram_channels WHERE org_id=$1 AND bot_id=$2 AND chat_id=$3 AND topic_id=$4 FOR SHARE", b.OrgID, b.ID, strconv.FormatInt(m.Chat.ID, 10), m.Topic))
	if err != nil {
		if core.PublicError(platform.DatabaseError(err)).Status == 404 {
			out.reason = "channel_not_allowed"
			return out, nil
		}
		return out, err
	}
	if channel.State != "active" {
		out.reason = "channel_paused"
		return out, nil
	}
	skill, ok := requestSkill(m, b, channel)
	if !ok {
		out.reason = "skill_selection_required"
		return out, nil
	}
	if m.Date <= 0 || time.Unix(m.Date, 0).After(time.Now().Add(time.Minute)) || time.Unix(m.Date, 0).Before(channel.UpdatedAt.Truncate(time.Second)) {
		out.reason = "channel_changed_after_message"
		return out, nil
	}
	var userID string
	var authVersion int64
	// Lock the current user before its binding, matching portal command order.
	err = tx.QueryRow(ctx, `SELECT u.id,u.auth_version FROM users u JOIN telegram_bindings b ON b.org_id=u.org_id AND b.user_id=u.id
 WHERE b.org_id=$1 AND b.bot_id=$2 AND b.telegram_user_id=$3 AND b.state='active' AND u.status='active' FOR SHARE OF u`, b.OrgID, b.ID, strconv.FormatInt(m.From.ID, 10)).Scan(&userID, &authVersion)
	if err == pgx.ErrNoRows {
		out.reason = "identity_not_bound"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	binding, err := scanBinding(tx.QueryRow(ctx, "SELECT "+bindingColumns+" FROM telegram_bindings WHERE org_id=$1 AND bot_id=$2 AND user_id=$3 AND telegram_user_id=$4 AND state='active' FOR SHARE", b.OrgID, b.ID, userID, strconv.FormatInt(m.From.ID, 10)))
	if err != nil {
		if core.PublicError(platform.DatabaseError(err)).Status == 404 {
			out.reason = "identity_not_bound"
			return out, nil
		}
		return out, err
	}
	tgID, chatID := strconv.FormatInt(m.From.ID, 10), strconv.FormatInt(m.Chat.ID, 10)
	out = inboxRecord{kind: "request", state: "awaiting_executor", reason: "executor_not_connected", requester: &userID, binding: &binding.ID, authVersion: &authVersion, channel: &channel.ID, channelVersion: &channel.Version, telegramUser: &tgID, chat: &chatID, topic: &m.Topic, messageID: &m.ID, text: &text}
	out.skill = &skill
	return out, nil
}
func (s *Service) Accept(ctx context.Context, botID, secret string, raw []byte) error {
	bot, ok := s.WebhookBot(botID, secret)
	if !ok {
		return core.Missing()
	}
	u, canonical, err := parseUpdate(raw)
	if err != nil {
		return err
	}
	digest := core.Digest(canonical)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", bot.OrgID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "telegram-update/"+bot.ID+"/"+strconv.FormatInt(u.ID, 10)); err != nil {
		return err
	}
	var previous string
	err = tx.QueryRow(ctx, "SELECT payload_sha256 FROM telegram_inbox WHERE bot_id=$1 AND update_id=$2", bot.ID, u.ID).Scan(&previous)
	if err == nil {
		if previous != digest {
			return core.Fail(409, "TELEGRAM_UPDATE_CONFLICT", "同一更新标识不能携带不同内容")
		}
		return tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	value, err := s.classify(ctx, tx, bot, u)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO telegram_inbox(id,org_id,bot_id,update_id,payload_sha256,kind,state,reason_code,requester_id,binding_id,actor_auth_version,channel_id,channel_version,telegram_user_id,chat_id,topic_id,message_id,request_text,requested_skill_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, core.NewID(), bot.OrgID, bot.ID, u.ID, digest, value.kind, value.state, value.reason, value.requester, value.binding, value.authVersion, value.channel, value.channelVersion, value.telegramUser, value.chat, value.topic, value.messageID, value.text, value.skill)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
