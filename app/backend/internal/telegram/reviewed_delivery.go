package telegram

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func reviewedResultMessage(ctx context.Context, tx pgx.Tx, bot DeliveryBot, d Delivery) (OutboundMessage, error) {
	a := core.Actor{OrgID: bot.OrgID}
	var sha string
	var start, count int
	err := tx.QueryRow(ctx, `SELECT rv.actor_id,rv.session_id,rv.actor_auth_version,rv.organization_auth_version,s.device_type,rv.body_sha256,rv.start_char,rv.max_chars
 FROM telegram_human_handoffs h JOIN telegram_handoff_reviews rv ON rv.org_id=h.org_id AND rv.handoff_id=h.id JOIN sessions s ON s.org_id=rv.org_id AND s.id=rv.session_id
 WHERE h.org_id=$1 AND h.inbox_id=$2`, bot.OrgID, d.InboxID).Scan(&a.UserID, &a.SessionID, &a.AuthVersion, &a.OrgAuthVersion, &a.Channel, &sha, &start, &count)
	if err != nil {
		return OutboundMessage{}, err
	}
	v, err := agent.TelegramHandoffResult(ctx, tx, a, d.InboxID)
	if err != nil {
		return OutboundMessage{}, err
	}
	chars := []rune(v.Body)
	if sha != v.BodySHA256 || start < 0 || count < 1 || count > 2000 || start > len(chars) || count > len(chars)-start {
		return OutboundMessage{}, core.Conflict()
	}
	m := OutboundMessage{ChatID: v.ChatID, TopicID: v.TopicID, Text: "经请求人审核的秘书结果片段：\n" + string(chars[start:start+count]) + "\n" + bot.PortalOrigin + "/telegram?request=" + d.InboxID}
	var reply int64
	if err = tx.QueryRow(ctx, "SELECT message_id FROM telegram_inbox WHERE org_id=$1 AND id=$2", bot.OrgID, d.InboxID).Scan(&reply); err != nil {
		return OutboundMessage{}, err
	}
	m.Reply.MessageID = reply
	m.Preview.Disabled = true
	return m, nil
}
