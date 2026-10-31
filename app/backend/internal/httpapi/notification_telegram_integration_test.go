package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/telegram"
)

// Native PG + real portal login/binding/business/outbox, controlled transport.
// No messages are sent to Telegram or to real employee accounts.
func TestTelegramBusinessNotices(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	current := t
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			current.Fatal(e)
		}
	}
	run := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { current = st; f.t = st; defer func() { current = t; f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	p := f.createProject("Synthetic private notification project", []string{f.bob})
	exec(`UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='project_manager'`, f.org)
	f.bobToken = memberMFAToken(f, "bob", "android")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	bot := telegram.BotConfig{ID: "123456789", OrgID: f.org, Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("t", 40)}
	f.api.Telegram = &telegram.Service{DB: f.api.DB, Config: f.api.Config, Bots: []telegram.BotConfig{bot}}
	deliveryBot := telegram.DeliveryBot{BotID: bot.ID, OrgID: f.org, Token: bot.ID + ":" + strings.Repeat("T", 40), PortalOrigin: "https://portal.example.invalid"}
	service := notification.Service{DB: f.api.DB}
	var binding telegram.Binding
	run("real_two_way_binding_is_required_and_does_not_enable_delivery", func() {
		r, v := f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, f.bobToken, core.NewID(), notification.TelegramPolicyInput{Enabled: true, Categories: []string{"task"}, Reason: "Synthetic consent needs confirmed binding"}, nil)
		expect(current, r, 409, v)
		r, v = f.request("POST", "/v1/me/telegram/challenges", f.bobToken, core.NewID(), telegram.ChallengeCreate{BotID: bot.ID, Reason: "Synthetic personal identity binding"}, nil)
		expect(current, r, 201, v)
		challenge := data[telegram.Challenge](current, v)
		raw, _ := json.Marshal(map[string]any{"update_id": 801, "message": map[string]any{"message_id": 801, "from": map[string]any{"id": 987654322, "is_bot": false}, "chat": map[string]any{"id": 987654322, "type": "private"}, "text": "/start " + challenge.StartToken, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}})
		req := httptest.NewRequest("POST", "/v1/integrations/telegram/"+bot.ID+"/webhook", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", bot.WebhookSecret)
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, req)
		if out.Code != 200 {
			current.Fatal(out.Code)
		}
		r, v = f.request("GET", "/v1/me/telegram", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		challenge = data[telegram.Status](current, v).Challenges[0]
		r, v = f.request("POST", "/v1/me/telegram/challenges/"+challenge.ID+"/confirm", f.bobToken, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: challenge.Version, TelegramUserID: *challenge.TelegramUserID, Reason: "Synthetic original-session confirmation"}, nil)
		expect(current, r, 201, v)
		binding = data[telegram.Binding](current, v)
		r, v = f.request("GET", "/v1/notification-telegram-settings", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		s := data[struct {
			Items []notification.TelegramSetting `json:"items"`
		}](current, v)
		if len(s.Items) != 1 || s.Items[0].Current != nil || s.Items[0].Effective {
			current.Fatal("binding silently enabled notices")
		}
	})
	create := func(title string) project.Task {
		current.Helper()
		r, v := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": title, "priority": "normal", "assignee_id": f.bob, "review_required": false}, nil)
		expect(current, r, 201, v)
		task := data[project.Task](current, v)
		drainNotifications(current, f)
		return task
	}
	var policy notification.TelegramPolicy
	publish := func(enabled bool, categories []string) notification.TelegramPolicy {
		current.Helper()
		r, v := f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, f.bobToken, core.NewID(), notification.TelegramPolicyInput{ExpectedVersion: policy.Version, Enabled: enabled, Categories: categories, Reason: "Synthetic explicit employee preference"}, nil)
		expect(current, r, 201, v)
		return data[notification.TelegramPolicy](current, v)
	}
	ready := func() {
		exec(`UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1`, bot.ID)
	}
	prepare := func() *notification.PreparedTelegram {
		current.Helper()
		ready()
		d, _, err := service.PrepareTelegram(ctx, deliveryBot)
		if err != nil {
			current.Fatal(err)
		}
		return d
	}
	finish := func(d *notification.PreparedTelegram, state string) {
		current.Helper()
		var message *int64
		if state == "sent" {
			id := int64(9001)
			message = &id
		}
		if err := service.FinishTelegram(ctx, deliveryBot, *d, telegram.DeliveryOutcome{State: state, Reason: map[string]string{"sent": "provider_confirmed", "delivery_unknown": "provider_outcome_unknown"}[state], MessageID: message}); err != nil {
			current.Fatal(err)
		}
	}
	details := func(id string) notification.TelegramReceiptDetails {
		current.Helper()
		r, v := f.request("GET", "/v1/notification-telegram-deliveries/"+id, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		return data[notification.TelegramReceiptDetails](current, v)
	}
	run("explicit_consent_has_no_historical_backfill_and_is_idempotent", func() {
		create("Never disclose this historical task title")
		policy = publish(true, []string{"task"})
		if prepare() != nil {
			current.Fatal("old notice sent after opt-in")
		}
		input := notification.TelegramPolicyInput{ExpectedVersion: policy.Version, Enabled: true, Categories: []string{"approval", "task"}, Reason: "Synthetic fixed idempotent preference"}
		key := core.NewID()
		r, v := f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, f.bobToken, key, input, nil)
		expect(current, r, 201, v)
		policy = data[notification.TelegramPolicy](current, v)
		r, v = f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, f.bobToken, key, input, nil)
		expect(current, r, 201, v)
		if data[notification.TelegramPolicy](current, v).ID != policy.ID {
			current.Fatal("replay republished consent")
		}
		r, v = f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, f.bobToken, core.NewID(), input, nil)
		expect(current, r, 409, v)
	})
	var first *notification.PreparedTelegram
	run("real_business_event_prepares_only_fixed_private_prompt_and_shared_budget", func() {
		create("CONFIDENTIAL 123456 contract amount and file")
		first = prepare()
		if first == nil || first.Message.ChatID != "987654322" || first.Message.TopicID != 0 || first.Message.Text != "您有新的公司内部通知，请登录查看。\nhttps://portal.example.invalid/notifications" {
			current.Fatal("unexpected external content or destination")
		}
		raw, _ := json.Marshal(first.Message)
		if bytes.Contains(raw, []byte("reply_parameters")) || bytes.Contains(raw, []byte("CONFIDENTIAL")) || !first.Message.Preview.Disabled {
			current.Fatal("private transport contains reply/body/preview")
		}
		var readyNow bool
		if err := f.owner.QueryRow(ctx, `SELECT next_allowed_at>clock_timestamp() FROM telegram_delivery_budgets WHERE bot_id=$1`, bot.ID).Scan(&readyNow); err != nil || !readyNow {
			current.Fatal("shared Bot budget missing", err)
		}
		finish(first, "sent")
		d := details(first.Receipt.ID)
		if d.Receipt.State != "sent" || len(d.Events) != 3 || d.Receipt.MessageID == nil {
			current.Fatal("missing immutable provider receipt")
		}
		finish(first, "delivery_unknown")
		if details(first.Receipt.ID).Receipt.State != "sent" {
			current.Fatal("stale completion overwrote provider result")
		}
	})
	run("other_user_cannot_read_or_reconcile_receipt", func() {
		r, v := f.request("GET", "/v1/notification-telegram-deliveries/"+first.Receipt.ID, f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/notification-telegram-deliveries", f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		if len(data[core.Page[notification.TelegramReceipt]](current, v).Items) != 0 {
			current.Fatal("receipt leaked across employees")
		}
	})
	var unknown *notification.PreparedTelegram
	run("unknown_outcome_never_automatically_resends", func() {
		create("Unknown outcome synthetic task")
		unknown = prepare()
		if unknown == nil {
			current.Fatal("missing prepared notice")
		}
		finish(unknown, "delivery_unknown")
		if prepare() != nil {
			current.Fatal("uncertain notice resent automatically")
		}
	})
	if os.Getenv("CP_BROWSER_NOTIFICATION_TELEGRAM_FIXTURE") == "1" {
		holdTelegramNoticeBrowser(current, f, bot.ID)
		return
	}
	run("recent_MFA_version_and_owner_are_required_to_reconcile", func() {
		d := details(unknown.Receipt.ID).Receipt
		input := notification.TelegramReconcile{ExpectedVersion: d.Version, Action: "retry", Reason: "Synthetic explicit duplicate risk confirmation"}
		exec(`UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2`, f.org, f.bob)
		r, v := f.request("POST", "/v1/notification-telegram-deliveries/"+d.ID+"/reconcile", f.bobToken, core.NewID(), input, nil)
		expect(current, r, 403, v)
		exec(`UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2`, f.org, f.bob)
		input.ExpectedVersion++
		r, v = f.request("POST", "/v1/notification-telegram-deliveries/"+d.ID+"/reconcile", f.bobToken, core.NewID(), input, nil)
		expect(current, r, 409, v)
		input.ExpectedVersion--
		r, v = f.request("POST", "/v1/notification-telegram-deliveries/"+d.ID+"/reconcile", f.bobToken, core.NewID(), input, nil)
		expect(current, r, 200, v)
		retry := prepare()
		if retry == nil || retry.Receipt.ID != d.ID || retry.Receipt.Attempts != 2 {
			current.Fatal("manual retry did not reuse exact source")
		}
		finish(retry, "sent")
	})
	run("read_or_resolved_notice_is_suppressed_before_send", func() {
		task := create("Already read synthetic task")
		n := notificationFor(current, f, f.bobToken, task.ID)
		if n == nil {
			current.Fatal("missing in-app source")
		}
		r, v := f.request("POST", "/v1/notifications/"+n.ID+"/read", f.bobToken, core.NewID(), notification.ReadInput{DeliverySeq: n.DeliverySeq}, nil)
		expect(current, r, 200, v)
		if prepare() != nil {
			current.Fatal("already read notice sent")
		}
	})
	run("withdrawn_consent_suppresses_queued_attempt_and_keeps_history", func() {
		create("Consent withdrawal synthetic task")
		d := prepare()
		if d == nil {
			current.Fatal("missing prepared")
		}
		if err := service.FinishTelegram(ctx, deliveryBot, *d, telegram.DeliveryOutcome{State: "retry_wait", Reason: "provider_rate_limited", RetryAfter: time.Second}); err != nil {
			current.Fatal(err)
		}
		policy = publish(false, []string{"task"})
		if prepare() != nil {
			current.Fatal("disabled consent still sends")
		}
		if details(d.Receipt.ID).Receipt.State != "suppressed" {
			current.Fatal("old queued attempt not suppressed")
		}
		r, v := f.request("GET", "/v1/notification-telegram-policies/"+bot.ID+"?limit=1", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		page := data[core.Page[notification.TelegramPolicy]](current, v)
		if len(page.Items) != 1 || page.NextCursor == nil {
			current.Fatal("policy history missing cursor")
		}
		r, v = f.request("GET", "/v1/notification-telegram-policies/"+bot.ID+"?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(current, r, 400, v)
		policy = publish(true, []string{"task"})
	})
	run("worker_pipeline_confirms_provider_receipt_and_rate_limits_stop_after_five_attempts", func() {
		create("Synthetic complete worker pipeline")
		ready()
		calls := 0
		worked, err := service.DeliverTelegramOnce(ctx, deliveryBot, controlledTelegramDelivery(func(_ context.Context, _ telegram.DeliveryBot, m telegram.OutboundMessage) telegram.DeliveryOutcome {
			calls++
			if strings.Contains(m.Text, "Synthetic") {
				current.Fatal("business title escaped")
			}
			id := int64(9002)
			return telegram.DeliveryOutcome{State: "sent", Reason: "provider_confirmed", MessageID: &id}
		}))
		if err != nil || !worked || calls != 1 {
			current.Fatal("worker pipeline not exercised", err, calls)
		}
		create("Synthetic bounded rate-limit retries")
		var receipt string
		for i := 1; i <= 5; i++ {
			d := prepare()
			if d == nil || d.Receipt.Attempts != i {
				current.Fatal("missing bounded attempt", i)
			}
			receipt = d.Receipt.ID
			if err = service.FinishTelegram(ctx, deliveryBot, *d, telegram.DeliveryOutcome{State: "retry_wait", Reason: "provider_rate_limited", RetryAfter: time.Nanosecond}); err != nil {
				current.Fatal(err)
			}
		}
		if d := details(receipt); d.Receipt.State != "failed" || d.Receipt.ReasonCode != "retry_limit_reached" {
			current.Fatal("retry budget did not stop")
		}
		if prepare() != nil {
			current.Fatal("automatic sixth attempt")
		}
	})
	run("human_confirmed_delivery_does_not_read_or_complete_business", func() {
		create("Synthetic human delivery reconciliation")
		d := prepare()
		if d == nil {
			current.Fatal("missing attempt")
		}
		finish(d, "delivery_unknown")
		latest := details(d.Receipt.ID).Receipt
		msg := int64(9003)
		r, v := f.request("POST", "/v1/notification-telegram-deliveries/"+d.Receipt.ID+"/reconcile", f.bobToken, core.NewID(), notification.TelegramReconcile{ExpectedVersion: latest.Version, Action: "confirm_sent", MessageID: &msg, Reason: "Synthetic employee verified personal chat message"}, nil)
		expect(current, r, 200, v)
		if data[notification.TelegramReceipt](current, v).State != "confirmed_sent" {
			current.Fatal("human fact missing")
		}
		var read int64
		if err := f.owner.QueryRow(ctx, `SELECT read_seq FROM notification_threads WHERE org_id=$1 AND id=$2`, f.org, d.Receipt.ThreadID).Scan(&read); err != nil || read != 0 {
			current.Fatal("delivery marked business as read", err)
		}
	})
	run("current_business_authority_is_rechecked_before_send", func() {
		create("Authority withdrawal synthetic task")
		d := prepare()
		if d == nil {
			current.Fatal("missing prepared")
		}
		if err := service.FinishTelegram(ctx, deliveryBot, *d, telegram.DeliveryOutcome{State: "retry_wait", Reason: "provider_rate_limited", RetryAfter: time.Second}); err != nil {
			current.Fatal(err)
		}
		exec(`UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL`, f.org, f.bob)
		if prepare() != nil {
			current.Fatal("current domain authority was not rechecked")
		}
		if details(d.Receipt.ID).Receipt.State != "suppressed" {
			current.Fatal("revoked domain receipt not suppressed")
		}
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='project_manager'`, core.NewID(), f.org, f.bob)
	})
	run("authority_and_binding_are_rechecked_before_manual_retry", func() {
		create("Binding withdrawal synthetic task")
		d := prepare()
		if d == nil {
			current.Fatal("missing prepared")
		}
		finish(d, "delivery_unknown")
		r, v := f.request("POST", "/v1/me/telegram/bindings/"+binding.ID+"/revoke", f.bobToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: binding.Version, Reason: "Synthetic employee revokes confirmed identity"}, nil)
		expect(current, r, 200, v)
		latest := details(d.Receipt.ID).Receipt
		r, v = f.request("POST", "/v1/notification-telegram-deliveries/"+d.Receipt.ID+"/reconcile", f.bobToken, core.NewID(), notification.TelegramReconcile{ExpectedVersion: latest.Version, Action: "retry", Reason: "Synthetic revoked binding cannot retry"}, nil)
		expect(current, r, 409, v)
		if prepare() != nil {
			current.Fatal("revoked identity still sends")
		}
	})
	run("four_human_clients_read_own_settings_and_web_requires_CSRF", func() {
		for _, channel := range []string{"wechat", "douyin", "android"} {
			token := f.login("member", channel).AccessToken
			r, v := f.request("GET", "/v1/notification-telegram-settings", token, "", nil, nil)
			expect(current, r, 200, v)
		}
		r, v := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "member", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
		expect(current, r, 200, v)
		cookies := []string{}
		for _, c := range r.Result().Cookies() {
			cookies = append(cookies, c.Name+"="+c.Value)
		}
		r, v = f.request("GET", "/v1/notification-telegram-settings", "", "", nil, map[string]string{"Cookie": strings.Join(cookies, "; ")})
		expect(current, r, 200, v)
		r, v = f.request("POST", "/v1/notification-telegram-policies/"+bot.ID, "", core.NewID(), notification.TelegramPolicyInput{Enabled: false, Categories: []string{}, Reason: "Synthetic Web mutation must require CSRF"}, map[string]string{"Cookie": strings.Join(cookies, "; "), "Origin": "http://localhost:5178"})
		expect(current, r, 403, v)
	})
	run("receipt_and_policy_history_are_immutable_in_database", func() {
		for _, query := range []string{`UPDATE notification_telegram_policies SET reason='rewrite' WHERE org_id=$1`, `DELETE FROM notification_telegram_events WHERE org_id=$1`, `UPDATE notification_telegram_deliveries SET policy_id='00000000-0000-0000-0000-000000000000' WHERE org_id=$1`} {
			if _, err := f.owner.Exec(ctx, query, f.org); err == nil {
				current.Fatal("immutable history changed")
			}
		}
	})
}
