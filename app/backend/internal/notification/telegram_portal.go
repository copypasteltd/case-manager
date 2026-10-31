package notification

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type TelegramPolicy struct {
	ID             string    `json:"id"`
	BotID          string    `json:"bot_id"`
	Version        int64     `json:"version"`
	Enabled        bool      `json:"enabled"`
	Categories     []string  `json:"categories"`
	BindingID      *string   `json:"binding_id"`
	BindingVersion *int64    `json:"binding_version"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}
type TelegramSetting struct {
	BotID         string          `json:"bot_id"`
	BindingActive bool            `json:"binding_active"`
	Effective     bool            `json:"effective"`
	Current       *TelegramPolicy `json:"current"`
}
type TelegramPolicyInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	Enabled         bool     `json:"enabled"`
	Categories      []string `json:"categories"`
	Reason          string   `json:"reason"`
}
type TelegramReceipt struct {
	ID               string    `json:"id"`
	BotID            string    `json:"bot_id"`
	SourceDeliveryID string    `json:"source_delivery_id"`
	ThreadID         string    `json:"thread_id"`
	PolicyID         string    `json:"policy_id"`
	State            string    `json:"state"`
	Version          int64     `json:"version"`
	Attempts         int       `json:"attempts"`
	ReasonCode       string    `json:"reason_code"`
	NextAttemptAt    time.Time `json:"next_attempt_at"`
	PayloadSHA256    *string   `json:"payload_sha256"`
	MessageID        *int64    `json:"message_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type TelegramReceiptEvent struct {
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	State         string    `json:"state"`
	Attempts      int       `json:"attempts"`
	ReasonCode    string    `json:"reason_code"`
	PayloadSHA256 *string   `json:"payload_sha256"`
	MessageID     *int64    `json:"message_id"`
	ActorID       *string   `json:"actor_id"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}
type TelegramReceiptDetails struct {
	Receipt TelegramReceipt        `json:"receipt"`
	Events  []TelegramReceiptEvent `json:"events"`
}
type TelegramReconcile struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	MessageID       *int64 `json:"message_id"`
	Reason          string `json:"reason"`
}

const telegramPolicyColumns = `id,bot_id,version,enabled,categories,binding_id,binding_version,reason,created_at`
const telegramReceiptColumns = `id,bot_id,source_delivery_id,thread_id,policy_id,state,version,attempts,reason_code,next_attempt_at,payload_sha256,message_id,created_at,updated_at`

func scanTelegramPolicy(row pgx.Row) (p TelegramPolicy, err error) {
	err = row.Scan(&p.ID, &p.BotID, &p.Version, &p.Enabled, &p.Categories, &p.BindingID, &p.BindingVersion, &p.Reason, &p.CreatedAt)
	return
}
func scanTelegramReceipt(row pgx.Row) (d TelegramReceipt, err error) {
	err = row.Scan(&d.ID, &d.BotID, &d.SourceDeliveryID, &d.ThreadID, &d.PolicyID, &d.State, &d.Version, &d.Attempts, &d.ReasonCode, &d.NextAttemptAt, &d.PayloadSHA256, &d.MessageID, &d.CreatedAt, &d.UpdatedAt)
	return
}
func currentTelegramPolicy(ctx context.Context, tx platform.DBTX, org, user, bot string) (*TelegramPolicy, error) {
	p, err := scanTelegramPolicy(tx.QueryRow(ctx, `SELECT `+telegramPolicyColumns+` FROM notification_telegram_policies WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 ORDER BY version DESC LIMIT 1`, org, user, bot))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (s *Service) TelegramSettings(ctx context.Context, a core.Actor) ([]TelegramSetting, error) {
	out := []TelegramSetting{}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT bot_id FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 UNION SELECT bot_id FROM notification_telegram_policies WHERE org_id=$1 AND user_id=$2 ORDER BY bot_id LIMIT 101`, a.OrgID, a.UserID)
	if err != nil {
		return out, err
	}
	bots := []string{}
	for rows.Next() {
		var bot string
		if err = rows.Scan(&bot); err != nil {
			break
		}
		bots = append(bots, bot)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(bots) > 100 {
		return out, core.Fail(409, "TELEGRAM_SETTINGS_LIMIT", "请联系管理员整理历史Bot绑定")
	}
	for _, bot := range bots {
		p, e := currentTelegramPolicy(ctx, tx, a.OrgID, a.UserID, bot)
		if e != nil {
			return out, e
		}
		var active, effective bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND state='active'),EXISTS(SELECT 1 FROM telegram_bindings b JOIN notification_telegram_policies p ON p.org_id=b.org_id AND p.binding_id=b.id AND p.binding_version=b.version WHERE p.org_id=$1 AND p.user_id=$2 AND p.bot_id=$3 AND p.enabled AND b.state='active' AND p.id=$4)`, a.OrgID, a.UserID, bot, func() any {
			if p == nil {
				return nil
			}
			return p.ID
		}()).Scan(&active, &effective)
		if err != nil {
			return out, err
		}
		out = append(out, TelegramSetting{bot, active, effective, p})
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) PublishTelegramPolicy(ctx context.Context, c core.Command, bot string, in TelegramPolicyInput) (core.Result, error) {
	if len(bot) < 1 || len(bot) > 16 || bot[0] == '0' || strings.Trim(bot, "0123456789") != "" || in.ExpectedVersion < 0 || in.ExpectedVersion >= 9007199254740991 || core.Name(in.Reason) != nil || in.Categories == nil || len(in.Categories) > len(categories) || (in.Enabled && len(in.Categories) == 0) {
		return core.Result{}, core.Invalid("policy", "请核对Bot、当前版本、通知类型和1至200字原因")
	}
	in.Categories = slices.Clone(in.Categories)
	slices.Sort(in.Categories)
	for i, key := range in.Categories {
		if _, ok := categories[key]; !ok || (i > 0 && in.Categories[i-1] == key) {
			return core.Result{}, core.Invalid("categories", "通知类型无效或重复")
		}
	}
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return identity.RequireInteractiveSession(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		previous, err := currentTelegramPolicy(ctx, tx, c.Actor.OrgID, c.Actor.UserID, bot)
		if err != nil {
			return core.Result{}, err
		}
		version := int64(0)
		if previous != nil {
			version = previous.Version
		}
		if version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "提醒设置已更新，请重新读取")
		}
		var binding *string
		var bindingVersion *int64
		err = tx.QueryRow(ctx, `SELECT id,version FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND state='active' FOR SHARE`, c.Actor.OrgID, c.Actor.UserID, bot).Scan(&binding, &bindingVersion)
		if err != nil && err != pgx.ErrNoRows {
			return core.Result{}, err
		}
		if in.Enabled && binding == nil {
			return core.Result{}, core.Fail(409, "TELEGRAM_BINDING_REQUIRED", "请先在账户设置完成该Bot的双向身份绑定")
		}
		if previous == nil && binding == nil {
			return core.Result{}, core.Missing()
		}
		p, err := scanTelegramPolicy(tx.QueryRow(ctx, `INSERT INTO notification_telegram_policies(id,org_id,user_id,bot_id,version,enabled,categories,binding_id,binding_version,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+telegramPolicyColumns, core.NewID(), c.Actor.OrgID, c.Actor.UserID, bot, version+1, in.Enabled, in.Categories, binding, bindingVersion, strings.TrimSpace(in.Reason)))
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "notification.telegram_policy.publish", ResourceType: "notification_telegram_policy", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"enabled": p.Enabled, "categories": p.Categories, "reason": p.Reason}}}, nil
	})
}
func (s *Service) TelegramPolicyHistory(ctx context.Context, a core.Actor, bot, cursor string, limit int) (core.Page[TelegramPolicy], error) {
	out := core.Page[TelegramPolicy]{Items: []TelegramPolicy{}}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条记录")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	binding := core.CursorBinding(a, "notification-telegram-policies:"+bot)
	var at *time.Time
	var id *string
	if cursor != "" {
		before, err := s.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		at = &before.UpdatedAt
		id = &before.ID
	}
	rows, err := tx.Query(ctx, `SELECT `+telegramPolicyColumns+` FROM notification_telegram_policies WHERE org_id=$1 AND user_id=$2 AND bot_id=$3 AND ($4::timestamptz IS NULL OR (created_at,id)<($4,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6`, a.OrgID, a.UserID, bot, at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanTelegramPolicy(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		p := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) TelegramReceipts(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[TelegramReceipt], error) {
	out := core.Page[TelegramReceipt]{Items: []TelegramReceipt{}}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条记录")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	binding := core.CursorBinding(a, "notification-telegram-receipts")
	var at *time.Time
	var id *string
	if cursor != "" {
		before, err := s.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		at = &before.UpdatedAt
		id = &before.ID
	}
	rows, err := tx.Query(ctx, `SELECT `+telegramReceiptColumns+` FROM notification_telegram_deliveries WHERE org_id=$1 AND user_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, a.UserID, at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanTelegramReceipt(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		p := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func telegramJournal(ctx context.Context, tx pgx.Tx, org, id string, actor, reason *string) error {
	_, err := tx.Exec(ctx, `INSERT INTO notification_telegram_events(id,org_id,delivery_id,version,state,attempts,reason_code,payload_sha256,message_id,actor_id,reason) SELECT $1,org_id,id,version,state,attempts,reason_code,payload_sha256,message_id,$4,$5 FROM notification_telegram_deliveries WHERE org_id=$2 AND id=$3`, core.NewID(), org, id, actor, reason)
	return err
}
func (s *Service) TelegramReceipt(ctx context.Context, a core.Actor, id string) (TelegramReceiptDetails, error) {
	out := TelegramReceiptDetails{Events: []TelegramReceiptEvent{}}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.Receipt, err = scanTelegramReceipt(tx.QueryRow(ctx, `SELECT `+telegramReceiptColumns+` FROM notification_telegram_deliveries WHERE org_id=$1 AND user_id=$2 AND id=$3`, a.OrgID, a.UserID, id))
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	rows, err := tx.Query(ctx, `SELECT id,version,state,attempts,reason_code,payload_sha256,message_id,actor_id,reason,created_at FROM notification_telegram_events WHERE org_id=$1 AND delivery_id=$2 ORDER BY version`, a.OrgID, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var e TelegramReceiptEvent
		if err = rows.Scan(&e.ID, &e.Version, &e.State, &e.Attempts, &e.ReasonCode, &e.PayloadSHA256, &e.MessageID, &e.ActorID, &e.Reason, &e.CreatedAt); err != nil {
			break
		}
		out.Events = append(out.Events, e)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ReconcileTelegram(ctx context.Context, c core.Command, id string, in TelegramReconcile) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil || !slices.Contains([]string{"confirm_sent", "retry", "abandon"}, in.Action) || (in.Action == "confirm_sent" && (in.MessageID == nil || *in.MessageID < 1 || *in.MessageID > 9007199254740991)) || (in.Action != "confirm_sent" && in.MessageID != nil) {
		return core.Result{}, core.Invalid("reconciliation", "请核对版本、处置方式、消息号及原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return identity.RequireInteractiveSession(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		d, err := scanTelegramReceipt(tx.QueryRow(ctx, `SELECT `+telegramReceiptColumns+` FROM notification_telegram_deliveries WHERE org_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, c.Actor.OrgID, c.Actor.UserID, id))
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if d.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "投递回执已更新，请重新读取")
		}
		state := "abandoned"
		if in.Action == "abandon" {
			if !slices.Contains([]string{"pending", "retry_wait", "delivery_unknown", "failed"}, d.State) {
				return core.Result{}, core.Fail(409, "TELEGRAM_RECEIPT_FINAL", "此投递目前不可终止")
			}
		} else if in.Action == "confirm_sent" {
			if d.State != "delivery_unknown" {
				return core.Result{}, core.Fail(409, "TELEGRAM_RECEIPT_STATE", "仅可核对送达结果未知的投递")
			}
			state = "confirmed_sent"
		} else {
			if !slices.Contains([]string{"delivery_unknown", "failed"}, d.State) || d.Attempts >= 5 {
				return core.Result{}, core.Fail(409, "TELEGRAM_RETRY_UNAVAILABLE", "当前状态或尝试次数不允许重试")
			}
			ok, e := s.telegramEligible(ctx, tx, c.Actor.OrgID, d)
			if e != nil {
				return core.Result{}, e
			}
			if !ok {
				return core.Result{}, core.Fail(409, "TELEGRAM_NOTICE_STALE", "通知、绑定、偏好或业务权限已变更，不能重新外发")
			}
			state = "retry_wait"
		}
		d, err = scanTelegramReceipt(tx.QueryRow(ctx, `UPDATE notification_telegram_deliveries SET state=$4,reason_code=$5,message_id=$6,lease_until=NULL,next_attempt_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND id=$3 RETURNING `+telegramReceiptColumns, c.Actor.OrgID, c.Actor.UserID, id, state, "human_"+in.Action, in.MessageID))
		if err != nil {
			return core.Result{}, err
		}
		note := strings.TrimSpace(in.Reason)
		if err = telegramJournal(ctx, tx, c.Actor.OrgID, id, &c.Actor.UserID, &note); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 200, Data: d, Event: core.Event{Type: "notification.telegram_delivery.reconcile", ResourceType: "notification_telegram_delivery", ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"action": in.Action, "reason": note}}}, nil
	})
}
