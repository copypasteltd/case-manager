package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/telegram"
	"github.com/jackc/pgx/v5"
)

type PreparedTelegram struct {
	Receipt TelegramReceipt
	Message telegram.OutboundMessage
}

// telegramEligible checks current consent/identity and ordinary domain access;
// choosing a different client never grants access beyond that client's rules.
func (s *Service) telegramEligible(ctx context.Context, tx pgx.Tx, org string, d TelegramReceipt) (bool, error) {
	var structural bool
	if err := tx.QueryRow(ctx, `SELECT notification_telegram_authorized($1,$2)`, org, d.ID).Scan(&structural); err != nil || !structural {
		return false, err
	}
	n, err := scan(tx.QueryRow(ctx, "SELECT "+threadColumns+threadJoin+` WHERE n.org_id=$1 AND n.id=$2 AND n.latest_delivery_id=$3`, org, d.ThreadID, d.SourceDeliveryID))
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var orgVersion int64
	if err = tx.QueryRow(ctx, `SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE`, org).Scan(&orgVersion); err != nil {
		return false, err
	}
	return eligible(ctx, tx, orgVersion, n)
}
func changeTelegram(ctx context.Context, tx pgx.Tx, org, id, state, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE notification_telegram_deliveries SET state=$3,reason_code=$4,lease_until=NULL,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, id, state, reason)
	if err != nil {
		return err
	}
	return telegramJournal(ctx, tx, org, id, nil, nil)
}

// PrepareTelegram commits one exact attempt before network I/O. It consumes
// only notices produced after the current explicit consent, never old history.
func (s *Service) PrepareTelegram(ctx context.Context, bot telegram.DeliveryBot) (*PreparedTelegram, bool, error) {
	if !bot.Valid() {
		return nil, false, core.Invalid("delivery", "发送配置无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR SHARE`, bot.OrgID); err != nil {
		return nil, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO telegram_delivery_budgets(bot_id,org_id) VALUES($1,$2) ON CONFLICT(bot_id) DO UPDATE SET last_seen_at=clock_timestamp() WHERE telegram_delivery_budgets.org_id=EXCLUDED.org_id`, bot.BotID, bot.OrgID)
	if err != nil {
		return nil, false, err
	}
	if tag.RowsAffected() != 1 {
		return nil, false, core.Fail(409, "TELEGRAM_BOT_OWNER_CONFLICT", "Bot归属不一致")
	}
	var ready bool
	if err = tx.QueryRow(ctx, `SELECT next_allowed_at<=clock_timestamp() FROM telegram_delivery_budgets WHERE bot_id=$1 AND org_id=$2 FOR UPDATE`, bot.BotID, bot.OrgID).Scan(&ready); err != nil {
		return nil, false, err
	}
	// Unique source/bot keys preserve one cross-channel receipt per in-app event.
	rows, err := tx.Query(ctx, `INSERT INTO notification_telegram_deliveries(id,org_id,user_id,bot_id,source_delivery_id,thread_id,policy_id,state,reason_code)
 SELECT gen_random_uuid(),d.org_id,d.recipient_id,$2,d.id,d.thread_id,p.id,'pending','queued'
 FROM notification_deliveries d JOIN notification_threads n ON n.org_id=d.org_id AND n.id=d.thread_id AND n.latest_delivery_id=d.id
 JOIN notification_telegram_policies p ON p.org_id=d.org_id AND p.user_id=d.recipient_id AND p.bot_id=$2 AND p.enabled
 JOIN telegram_bindings b ON b.org_id=p.org_id AND b.id=p.binding_id AND b.version=p.binding_version AND b.user_id=p.user_id AND b.bot_id=p.bot_id AND b.state='active'
 JOIN users u ON u.org_id=d.org_id AND u.id=d.recipient_id AND u.status='active'
 WHERE d.org_id=$1 AND n.read_seq<n.delivered_seq AND n.category=ANY(p.categories) AND d.created_at>=p.created_at AND d.created_at>clock_timestamp()-interval '24 hours'
 AND NOT EXISTS(SELECT 1 FROM notification_telegram_policies newer WHERE newer.org_id=p.org_id AND newer.user_id=p.user_id AND newer.bot_id=p.bot_id AND newer.version>p.version)
 AND NOT EXISTS(SELECT 1 FROM notification_telegram_deliveries sent WHERE sent.org_id=d.org_id AND sent.source_delivery_id=d.id AND sent.bot_id=$2)
 ORDER BY d.created_at,d.id LIMIT 100 ON CONFLICT(org_id,source_delivery_id,bot_id) DO NOTHING RETURNING id`, bot.OrgID, bot.BotID)
	if err != nil {
		return nil, false, err
	}
	created := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		created = append(created, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	for _, id := range created {
		if err = telegramJournal(ctx, tx, bot.OrgID, id, nil, nil); err != nil {
			return nil, false, err
		}
	}
	rows, err = tx.Query(ctx, `SELECT `+telegramReceiptColumns+` FROM notification_telegram_deliveries WHERE org_id=$1 AND bot_id=$2 AND state IN ('pending','retry_wait','sending') ORDER BY next_attempt_at,created_at,id LIMIT 100 FOR UPDATE SKIP LOCKED`, bot.OrgID, bot.BotID)
	if err != nil {
		return nil, false, err
	}
	pending := []TelegramReceipt{}
	for rows.Next() {
		d, e := scanTelegramReceipt(rows)
		if e != nil {
			err = e
			break
		}
		pending = append(pending, d)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	worked := len(created) > 0
	var selected *TelegramReceipt
	for _, d := range pending {
		if d.State == "sending" {
			var expired bool
			if err = tx.QueryRow(ctx, `SELECT lease_until<clock_timestamp() FROM notification_telegram_deliveries WHERE org_id=$1 AND id=$2`, bot.OrgID, d.ID).Scan(&expired); err != nil {
				return nil, false, err
			}
			if expired {
				if err = changeTelegram(ctx, tx, bot.OrgID, d.ID, "delivery_unknown", "sending_lease_expired"); err != nil {
					return nil, false, err
				}
				worked = true
			}
			continue
		}
		ok, e := s.telegramEligible(ctx, tx, bot.OrgID, d)
		if e != nil {
			return nil, false, e
		}
		if !ok {
			if err = changeTelegram(ctx, tx, bot.OrgID, d.ID, "suppressed", "notice_or_authority_changed"); err != nil {
				return nil, false, err
			}
			worked = true
			continue
		}
		if ready && selected == nil {
			var due bool
			if err = tx.QueryRow(ctx, `SELECT next_attempt_at<=clock_timestamp() FROM notification_telegram_deliveries WHERE org_id=$1 AND id=$2`, bot.OrgID, d.ID).Scan(&due); err != nil {
				return nil, false, err
			}
			if due {
				copy := d
				selected = &copy
			}
		}
	}
	if selected == nil {
		return nil, worked, tx.Commit(ctx)
	}
	var chat string
	if err = tx.QueryRow(ctx, `SELECT b.telegram_user_id FROM notification_telegram_policies p JOIN telegram_bindings b ON b.org_id=p.org_id AND b.id=p.binding_id AND b.version=p.binding_version WHERE p.org_id=$1 AND p.id=$2`, bot.OrgID, selected.PolicyID).Scan(&chat); err != nil {
		return nil, false, err
	}
	// Direct messages only: the confirmed personal Telegram identity is the chat.
	message := telegram.OutboundMessage{ChatID: chat, Text: "您有新的公司内部通知，请登录查看。\n" + bot.PortalOrigin + "/notifications"}
	message.Preview.Disabled = true
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, false, err
	}
	digest := sha256.Sum256(raw)
	d, err := scanTelegramReceipt(tx.QueryRow(ctx, `UPDATE notification_telegram_deliveries SET state='sending',reason_code='attempt_prepared',attempts=attempts+1,version=version+1,lease_until=clock_timestamp()+interval '30 seconds',payload_sha256=$3,message_id=NULL,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING `+telegramReceiptColumns, bot.OrgID, selected.ID, hex.EncodeToString(digest[:])))
	if err != nil {
		return nil, false, err
	}
	if err = telegramJournal(ctx, tx, bot.OrgID, d.ID, nil, nil); err != nil {
		return nil, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()+interval '3.2 seconds' WHERE bot_id=$1`, bot.BotID); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &PreparedTelegram{d, message}, true, nil
}

var telegramSafeReason = regexp.MustCompile(`^[a-z0-9_]{1,80}$`)

func (s *Service) FinishTelegram(ctx context.Context, bot telegram.DeliveryBot, prepared PreparedTelegram, outcome telegram.DeliveryOutcome) error {
	if !bot.Valid() || !telegramSafeReason.MatchString(outcome.Reason) {
		return core.Invalid("outcome", "投递结果无效")
	}
	switch outcome.State {
	case "sent":
		if outcome.MessageID == nil || *outcome.MessageID < 1 || *outcome.MessageID > 9007199254740991 {
			return core.Invalid("outcome", "送达结果缺少消息号")
		}
	case "retry_wait":
		if outcome.RetryAfter <= 0 || outcome.RetryAfter > 24*time.Hour || outcome.MessageID != nil {
			return core.Invalid("outcome", "重试间隔无效")
		}
	case "failed", "delivery_unknown":
		if outcome.MessageID != nil {
			return core.Invalid("outcome", "未送达结果不应含消息号")
		}
	default:
		return core.Invalid("outcome", "投递状态无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR SHARE`, bot.OrgID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT bot_id FROM telegram_delivery_budgets WHERE org_id=$1 AND bot_id=$2 FOR UPDATE`, bot.OrgID, bot.BotID); err != nil {
		return err
	}
	d, err := scanTelegramReceipt(tx.QueryRow(ctx, `SELECT `+telegramReceiptColumns+` FROM notification_telegram_deliveries WHERE org_id=$1 AND bot_id=$2 AND id=$3 FOR UPDATE`, bot.OrgID, bot.BotID, prepared.Receipt.ID))
	if err != nil {
		return err
	}
	if d.State != "sending" || d.Version != prepared.Receipt.Version || d.PayloadSHA256 == nil || prepared.Receipt.PayloadSHA256 == nil || *d.PayloadSHA256 != *prepared.Receipt.PayloadSHA256 {
		return tx.Commit(ctx)
	}
	if outcome.State == "retry_wait" {
		if _, err = tx.Exec(ctx, `UPDATE telegram_delivery_budgets SET next_allowed_at=GREATEST(next_allowed_at,clock_timestamp()+$2::double precision*interval '1 second') WHERE bot_id=$1`, bot.BotID, outcome.RetryAfter.Seconds()); err != nil {
			return err
		}
		if d.Attempts >= 5 {
			outcome.State = "failed"
			outcome.Reason = "retry_limit_reached"
		}
	}
	_, err = tx.Exec(ctx, `UPDATE notification_telegram_deliveries SET state=$3,reason_code=$4,message_id=$5,lease_until=NULL,next_attempt_at=clock_timestamp()+$6::double precision*interval '1 second',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, bot.OrgID, d.ID, outcome.State, outcome.Reason, outcome.MessageID, outcome.RetryAfter.Seconds())
	if err != nil {
		return err
	}
	if err = telegramJournal(ctx, tx, bot.OrgID, d.ID, nil, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) DeliverTelegramOnce(ctx context.Context, bot telegram.DeliveryBot, transport telegram.DeliveryTransport) (bool, error) {
	prepared, worked, err := s.PrepareTelegram(ctx, bot)
	if err != nil || prepared == nil {
		return worked, err
	}
	// A network/timeout outcome stays unknown, including a process crash before
	// FinishTelegram. No unknown outcome is automatically sent a second time.
	outcome := transport.Send(ctx, bot, prepared.Message)
	return true, s.FinishTelegram(ctx, bot, *prepared, outcome)
}
