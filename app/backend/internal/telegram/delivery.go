package telegram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// A deliberately conservative shared bot budget also stays below group limits.
const DeliveryInterval = 3200 * time.Millisecond
const DeliveryLease = 30 * time.Second
const DeliveryMaxAttempts = 5

type Delivery struct {
	ID            string    `json:"id"`
	InboxID       string    `json:"request_id"`
	RunID         *string   `json:"run_id"`
	Phase         string    `json:"phase"`
	State         string    `json:"state"`
	Version       int64     `json:"version"`
	Attempts      int       `json:"attempts"`
	ReasonCode    string    `json:"reason_code"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	MessageID     *int64    `json:"message_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type DeliveryEvent struct {
	DeliveryID string    `json:"delivery_id"`
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	ReasonCode string    `json:"reason_code"`
	MessageID  *int64    `json:"message_id"`
	ActorID    *string   `json:"actor_id"`
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
type DeliveryDetails struct {
	Deliveries []Delivery      `json:"deliveries"`
	Events     []DeliveryEvent `json:"events"`
}
type DeliveryReconcile struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	MessageID       *int64 `json:"message_id"`
	Reason          string `json:"reason"`
}
type PreparedDelivery struct {
	Delivery Delivery
	Message  OutboundMessage
}

const deliveryColumns = `id,inbox_id,run_id,phase,state,version,attempts,reason_code,next_attempt_at,message_id,created_at,updated_at`

func scanDelivery(row pgx.Row) (Delivery, error) {
	var d Delivery
	err := row.Scan(&d.ID, &d.InboxID, &d.RunID, &d.Phase, &d.State, &d.Version, &d.Attempts, &d.ReasonCode, &d.NextAttemptAt, &d.MessageID, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
func deliveryEvent(ctx context.Context, tx pgx.Tx, org, id string, actor, reason *string) error {
	_, err := tx.Exec(ctx, `INSERT INTO telegram_delivery_events(id,org_id,delivery_id,version,state,attempts,reason_code,payload_sha256,message_id,actor_id,reason)
 SELECT $1,org_id,id,version,state,attempts,reason_code,payload_sha256,message_id,$4,$5 FROM telegram_deliveries WHERE org_id=$2 AND id=$3`, core.NewID(), org, id, actor, reason)
	return err
}
func deliveryPhase(inbox string, run *string) string {
	if inbox == "withdrawn" {
		return ""
	}
	if run == nil || *run == "queued" {
		return "accepted"
	}
	switch *run {
	case "running", "awaiting_approval", "committing":
		return "progress"
	default:
		return "result"
	}
}
func phaseText(phase string) string {
	switch phase {
	case "accepted":
		return "请求已受理，等待处理。"
	case "progress":
		return "秘书任务正在处理。"
	default:
		return "秘书任务状态已更新，请登录查看结果。"
	}
}
func portalMessage(bot DeliveryBot, inbox, phase, chat string, topic, reply int64) OutboundMessage {
	m := OutboundMessage{ChatID: chat, TopicID: topic, Text: phaseText(phase) + "\n" + bot.PortalOrigin + "/telegram?request=" + inbox}
	m.Reply.MessageID = reply
	m.Preview.Disabled = true
	return m
}
func changeDelivery(ctx context.Context, tx pgx.Tx, org, id, state, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE telegram_deliveries SET state=$3,reason_code=$4,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, id, state, reason)
	if err != nil {
		return err
	}
	return deliveryEvent(ctx, tx, org, id, nil, nil)
}

// PrepareDelivery commits the exact prepared attempt before any network I/O.
// Another process can recover an abandoned sending lease only as unknown.
func (s *Service) PrepareDelivery(ctx context.Context, bot DeliveryBot) (*PreparedDelivery, bool, error) {
	if !bot.valid() {
		return nil, false, core.Invalid("delivery", "发送配置无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", bot.OrgID); err != nil {
		return nil, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO telegram_delivery_budgets(bot_id,org_id) VALUES($1,$2)
 ON CONFLICT(bot_id) DO UPDATE SET last_seen_at=clock_timestamp() WHERE telegram_delivery_budgets.org_id=EXCLUDED.org_id`, bot.BotID, bot.OrgID)
	if err != nil {
		return nil, false, err
	}
	if tag.RowsAffected() != 1 {
		return nil, false, core.Fail(409, "TELEGRAM_BOT_OWNER_CONFLICT", "Bot归属不一致")
	}

	// Fold stale queued phases, revoke invalid scopes, and preserve uncertain sends.
	rows, err := tx.Query(ctx, `SELECT d.id,d.state,(d.lease_until<clock_timestamp()),telegram_delivery_authorized(d.org_id,d.inbox_id),i.received_at<clock_timestamp()-interval '24 hours',i.state,r.state,d.phase
 FROM telegram_deliveries d JOIN telegram_inbox i ON i.org_id=d.org_id AND i.id=d.inbox_id
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
 WHERE d.org_id=$1 AND d.bot_id=$2 AND d.state IN ('pending','retry_wait','sending') ORDER BY d.created_at,d.id LIMIT 100 FOR UPDATE OF d SKIP LOCKED`, bot.OrgID, bot.BotID)
	if err != nil {
		return nil, false, err
	}
	type adjustment struct{ id, state, reason string }
	changes := []adjustment{}
	for rows.Next() {
		var id, state, inbox, phase string
		var expired *bool
		var allowed, old bool
		var run *string
		if err = rows.Scan(&id, &state, &expired, &allowed, &old, &inbox, &run, &phase); err != nil {
			break
		}
		if state == "sending" {
			if expired != nil && *expired {
				changes = append(changes, adjustment{id, "delivery_unknown", "sending_lease_expired"})
			}
			continue
		}
		if !allowed || old {
			changes = append(changes, adjustment{id, "suppressed", "scope_or_window_changed"})
		} else if deliveryPhase(inbox, run) != phase {
			changes = append(changes, adjustment{id, "superseded", "phase_coalesced"})
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	for _, c := range changes {
		if err = changeDelivery(ctx, tx, bot.OrgID, c.id, c.state, c.reason); err != nil {
			return nil, false, err
		}
	}
	worked := len(changes) > 0
	rows, err = tx.Query(ctx, `SELECT i.id,i.channel_id,l.run_id,i.state,r.state FROM telegram_inbox i
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
 WHERE i.org_id=$1 AND i.bot_id=$2 AND i.received_at>clock_timestamp()-interval '24 hours' AND telegram_delivery_authorized(i.org_id,i.id)
 AND NOT EXISTS(SELECT 1 FROM telegram_deliveries d WHERE d.inbox_id=i.id AND d.phase=CASE WHEN r.state IS NULL OR r.state='queued' THEN 'accepted' WHEN r.state IN ('running','awaiting_approval','committing') THEN 'progress' ELSE 'result' END)
 ORDER BY i.received_at,i.id LIMIT 100`, bot.OrgID, bot.BotID)
	if err != nil {
		return nil, false, err
	}
	type item struct {
		id, channel, phase string
		run                *string
	}
	items := []item{}
	for rows.Next() {
		var v item
		var inbox string
		var state *string
		if err = rows.Scan(&v.id, &v.channel, &v.run, &inbox, &state); err != nil {
			break
		}
		v.phase = deliveryPhase(inbox, state)
		items = append(items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	for _, v := range items {
		id := core.NewID()
		var inserted string
		e := tx.QueryRow(ctx, `INSERT INTO telegram_deliveries(id,org_id,inbox_id,channel_id,bot_id,run_id,phase,state,reason_code) VALUES($1,$2,$3,$4,$5,$6,$7,'pending','phase_ready') ON CONFLICT(inbox_id,phase) DO NOTHING RETURNING id`, id, bot.OrgID, v.id, v.channel, bot.BotID, v.run, v.phase).Scan(&inserted)
		if e == pgx.ErrNoRows {
			continue
		}
		if e != nil {
			return nil, false, e
		}
		if err = deliveryEvent(ctx, tx, bot.OrgID, id, nil, nil); err != nil {
			return nil, false, err
		}
		worked = true
	}
	var due bool
	if err = tx.QueryRow(ctx, "SELECT next_allowed_at<=clock_timestamp() FROM telegram_delivery_budgets WHERE bot_id=$1", bot.BotID).Scan(&due); err != nil {
		return nil, false, err
	}
	if !due {
		return nil, worked, tx.Commit(ctx)
	}
	d, err := scanDelivery(tx.QueryRow(ctx, `SELECT d.id,d.inbox_id,d.run_id,d.phase,d.state,d.version,d.attempts,d.reason_code,d.next_attempt_at,d.message_id,d.created_at,d.updated_at
 FROM telegram_deliveries d JOIN telegram_inbox i ON i.org_id=d.org_id AND i.id=d.inbox_id
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
 WHERE d.org_id=$1 AND d.bot_id=$2 AND d.state IN ('pending','retry_wait') AND d.next_attempt_at<=clock_timestamp()
 AND i.received_at>clock_timestamp()-interval '24 hours' AND telegram_delivery_authorized(d.org_id,d.inbox_id)
 AND d.phase=CASE WHEN r.state IS NULL OR r.state='queued' THEN 'accepted' WHEN r.state IN ('running','awaiting_approval','committing') THEN 'progress' ELSE 'result' END
 ORDER BY d.next_attempt_at,d.created_at,d.id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`, bot.OrgID, bot.BotID))
	if err == pgx.ErrNoRows {
		return nil, worked, tx.Commit(ctx)
	}
	if err != nil {
		return nil, false, err
	}
	var chat string
	var topic, reply int64
	if err = tx.QueryRow(ctx, "SELECT chat_id,topic_id,message_id FROM telegram_inbox WHERE org_id=$1 AND id=$2", bot.OrgID, d.InboxID).Scan(&chat, &topic, &reply); err != nil {
		return nil, false, err
	}
	message := portalMessage(bot, d.InboxID, d.Phase, chat, topic, reply)
	raw, _ := json.Marshal(message)
	digest := sha256.Sum256(raw)
	d, err = scanDelivery(tx.QueryRow(ctx, `UPDATE telegram_deliveries SET state='sending',reason_code='attempt_prepared',attempts=attempts+1,version=version+1,lease_until=clock_timestamp()+interval '30 seconds',payload_sha256=$3,message_id=NULL,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING `+deliveryColumns, bot.OrgID, d.ID, hex.EncodeToString(digest[:])))
	if err != nil {
		return nil, false, err
	}
	if err = deliveryEvent(ctx, tx, bot.OrgID, d.ID, nil, nil); err != nil {
		return nil, false, err
	}
	if _, err = tx.Exec(ctx, "UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()+interval '3.2 seconds' WHERE bot_id=$1", bot.BotID); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &PreparedDelivery{Delivery: d, Message: message}, true, nil
}

func (s *Service) FinishDelivery(ctx context.Context, bot DeliveryBot, prepared PreparedDelivery, outcome DeliveryOutcome) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Use the same lock order as preparation and channel commands.
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", bot.OrgID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT bot_id FROM telegram_delivery_budgets WHERE bot_id=$1 AND org_id=$2 FOR UPDATE", bot.BotID, bot.OrgID); err != nil {
		return err
	}
	d, err := scanDelivery(tx.QueryRow(ctx, "SELECT "+deliveryColumns+" FROM telegram_deliveries WHERE org_id=$1 AND bot_id=$2 AND id=$3 FOR UPDATE", bot.OrgID, bot.BotID, prepared.Delivery.ID))
	if err != nil {
		return err
	}
	if d.State != "sending" || d.Version != prepared.Delivery.Version {
		return tx.Commit(ctx)
	}
	if outcome.State != "sent" && outcome.State != "retry_wait" && outcome.State != "failed" && outcome.State != "delivery_unknown" {
		return core.Invalid("outcome", "发送结果无效")
	}
	if outcome.State == "retry_wait" && (outcome.RetryAfter <= 0 || outcome.RetryAfter > 24*time.Hour) {
		return core.Invalid("outcome", "重试间隔无效")
	}
	rateLimited := outcome.State == "retry_wait"
	if rateLimited && d.Attempts >= DeliveryMaxAttempts {
		outcome.State = "failed"
		outcome.Reason = "retry_limit_reached"
	}
	if rateLimited {
		if _, err = tx.Exec(ctx, "UPDATE telegram_delivery_budgets SET next_allowed_at=GREATEST(next_allowed_at,clock_timestamp()+$2::double precision*interval '1 second') WHERE bot_id=$1", bot.BotID, outcome.RetryAfter.Seconds()); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE telegram_deliveries SET state=$3,reason_code=$4,message_id=$5,version=version+1,next_attempt_at=clock_timestamp()+$6::double precision*interval '1 second',updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, bot.OrgID, d.ID, outcome.State, outcome.Reason, outcome.MessageID, outcome.RetryAfter.Seconds())
	if err != nil {
		return err
	}
	if err = deliveryEvent(ctx, tx, bot.OrgID, d.ID, nil, nil); err != nil {
		return err
	}
	if outcome.Reason == "chat_migration_requires_review" {
		var channel string
		if err = tx.QueryRow(ctx, `UPDATE telegram_channels c SET state='paused',delivery_enabled=false,version=version+1,updated_at=clock_timestamp() FROM telegram_inbox i WHERE i.org_id=$1 AND i.id=$2 AND c.org_id=i.org_id AND c.id=i.channel_id AND c.version=i.channel_version RETURNING c.id`, bot.OrgID, d.InboxID).Scan(&channel); err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil {
			if err = withdrawChannel(ctx, tx, bot.OrgID, channel, "chat_migration_requires_review"); err != nil {
				return err
			}
			if err = channelRevision(ctx, tx, bot.OrgID, channel, nil, "migration", "Bot API reported a changed chat identifier; manual review required"); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
func (s *Service) DeliverOnce(ctx context.Context, bot DeliveryBot, transport DeliveryTransport) (bool, error) {
	prepared, worked, err := s.PrepareDelivery(ctx, bot)
	if err != nil || prepared == nil {
		return worked, err
	}
	outcome := transport.Send(ctx, bot, prepared.Message)
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return true, s.FinishDelivery(bounded, bot, *prepared, outcome)
}
