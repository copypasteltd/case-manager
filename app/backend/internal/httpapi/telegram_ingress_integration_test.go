package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/telegram"
)

func TestTelegramIngress(t *testing.T) {
	f := setup(t)
	f.api.Logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bot := telegram.BotConfig{ID: "123456789", OrgID: f.org, Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("t", 40)}
	rec, result := f.request("GET", "/v1/me/telegram", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	if data[telegram.Status](t, result).IngressConfigured {
		t.Fatal("ingress was implicitly enabled")
	}
	f.api.Telegram = &telegram.Service{DB: f.api.DB, Config: f.api.Config, Bots: []telegram.BotConfig{bot}}
	admin := f.administratorToken()
	role := core.NewID()
	exec(`INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'telegram_test_mfa','Telegram fixture MFA only',true)`, role, f.org)
	for _, id := range []string{f.alice, f.bob} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')`, core.NewID(), f.org, id, role)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	webhook := func(body any, secret string) *httptest.ResponseRecorder {
		var raw []byte
		switch v := body.(type) {
		case []byte:
			raw = v
		default:
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest("POST", "/v1/integrations/telegram/"+bot.ID+"/webhook", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if secret != "" {
			r.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, r)
		return out
	}
	status := func(token string) telegram.Status {
		t.Helper()
		rec, res := f.request("GET", "/v1/me/telegram", token, "", nil, nil)
		expect(t, rec, 200, res)
		return data[telegram.Status](t, res)
	}
	create := func(token, key string) telegram.Challenge {
		t.Helper()
		rec, res := f.request("POST", "/v1/me/telegram/challenges", token, key, telegram.ChallengeCreate{BotID: bot.ID, Reason: "本人发起身份绑定"}, nil)
		expect(t, rec, 201, res)
		return data[telegram.Challenge](t, res)
	}
	prove := func(id int64, tgID int64, code string) {
		t.Helper()
		out := webhook(map[string]any{"update_id": id, "message": map[string]any{"message_id": id, "from": map[string]any{"id": tgID, "is_bot": false}, "chat": map[string]any{"id": tgID, "type": "private"}, "text": "/start " + code, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}}, bot.WebhookSecret)
		if out.Code != 200 {
			t.Fatalf("proof returned %d: %s", out.Code, out.Body.String())
		}
	}
	confirm := func(token string, c telegram.Challenge, tgID string) telegram.Binding {
		t.Helper()
		rec, res := f.request("POST", "/v1/me/telegram/challenges/"+c.ID+"/confirm", token, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: c.Version, TelegramUserID: tgID, Reason: "已核对私聊数字身份"}, nil)
		expect(t, rec, 201, res)
		return data[telegram.Binding](t, res)
	}
	request := func(id int64, chat int64, topic int64, from int64, text string) map[string]any {
		return map[string]any{"update_id": id, "message": map[string]any{"message_id": id, "date": time.Now().Unix(), "message_thread_id": topic, "from": map[string]any{"id": from, "is_bot": false}, "chat": map[string]any{"id": chat, "type": "supergroup"}, "text": text, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 4}}}}
	}
	accepted := func(body any) {
		t.Helper()
		out := webhook(body, bot.WebhookSecret)
		if out.Code != 200 {
			t.Fatalf("update returned %d: %s", out.Code, out.Body.String())
		}
		if strings.TrimSpace(out.Body.String()) != `{"ok":true}` {
			t.Fatal("external acknowledgement leaked internal state")
		}
	}
	reason := func(id int64) (string, *string) {
		t.Helper()
		var code string
		var text *string
		if err := f.owner.QueryRow(ctx, "SELECT reason_code,request_text FROM telegram_inbox WHERE bot_id=$1 AND update_id=$2", bot.ID, id).Scan(&code, &text); err != nil {
			t.Fatal(err)
		}
		return code, text
	}
	run := func(name string, fn func()) {
		t.Run(name, func(sub *testing.T) {
			old := t
			t = sub
			oldFT := f.t
			f.t = sub
			defer func() { t = old; f.t = oldFT }()
			fn()
		})
	}

	run("webhook secret and size are checked before persistence", func() {
		for _, secret := range []string{"", "wrong"} {
			if out := webhook([]byte(`{"update_id":1}`), secret); out.Code != 404 {
				t.Fatal("unauthenticated update accepted")
			}
		}
		if out := webhook([]byte(strings.Repeat(" ", 65537)), bot.WebhookSecret); out.Code != 413 {
			t.Fatal("oversized payload accepted")
		}
		for _, raw := range []string{`{"update_id":null}`, `{"update_id":1,"update_id":2}`, `{"update_id":1} {}`} {
			if out := webhook([]byte(raw), bot.WebhookSecret); out.Code != 422 {
				t.Fatal("ambiguous input accepted")
			}
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_inbox").Scan(&count); err != nil || count != 0 {
			t.Fatal("invalid webhook was stored")
		}
	})
	var challenge telegram.Challenge
	var binding telegram.Binding
	run("durable inbox failure rolls private proof back and exact retry can recover", func() {
		pending := create(f.bobToken, core.NewID())
		exec(`CREATE FUNCTION telegram_test_inbox_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.update_id=12 THEN RAISE EXCEPTION 'controlled inbox persistence failure' USING ERRCODE='XX000'; END IF; RETURN NEW; END $$`)
		exec(`CREATE TRIGGER telegram_test_inbox_failure BEFORE INSERT ON telegram_inbox FOR EACH ROW EXECUTE FUNCTION telegram_test_inbox_failure()`)
		body := map[string]any{"update_id": 12, "message": map[string]any{"message_id": 12, "from": map[string]any{"id": int64(987654323), "is_bot": false}, "chat": map[string]any{"id": int64(987654323), "type": "private"}, "text": "/start " + pending.StartToken, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}}
		out := webhook(body, bot.WebhookSecret)
		exec(`DROP TRIGGER telegram_test_inbox_failure ON telegram_inbox`)
		exec(`DROP FUNCTION telegram_test_inbox_failure()`)
		if out.Code != 500 {
			t.Fatal("failed persistence was acknowledged")
		}
		current := status(f.bobToken).Challenges[0]
		if current.ClaimedAt != nil || current.Version != 1 {
			t.Fatal("failed inbox consumed private proof")
		}
		accepted(body)
		current = status(f.bobToken).Challenges[0]
		if current.ClaimedAt == nil || current.Version != 2 {
			t.Fatal("exact retry did not recover actual proof")
		}
	})
	run("proof is private single use and portal confirmation remains separate", func() {
		key := core.NewID()
		challenge = create(f.aliceToken, key)
		if len(challenge.StartToken) != 43 || !strings.Contains(challenge.StartURL, challenge.StartToken) {
			t.Fatal("missing bounded start capability")
		}
		replay := create(f.aliceToken, key)
		if replay.ID != challenge.ID || replay.StartToken != challenge.StartToken {
			t.Fatal("issue retry changed the binding capability")
		}
		plain := status(f.aliceToken)
		if len(plain.Challenges) != 1 || plain.Challenges[0].StartToken != "" {
			t.Fatal("GET exposed binding capability")
		}
		prove(10, 987654321, challenge.StartToken)
		current := status(f.aliceToken)
		if len(current.Bindings) != 0 || current.Challenges[0].TelegramUserID == nil || *current.Challenges[0].TelegramUserID != "987654321" {
			t.Fatal("private proof changed formal binding")
		}
		challenge = current.Challenges[0]
		prove(11, 987654322, replay.StartToken)
		current = status(f.aliceToken)
		if *current.Challenges[0].TelegramUserID != "987654321" {
			t.Fatal("single-use proof was reassigned")
		}
		rec, res := f.request("POST", "/v1/me/telegram/challenges/"+challenge.ID+"/confirm", f.bobToken, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: challenge.Version, TelegramUserID: "987654321", Reason: "不能确认其他员工"}, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("POST", "/v1/me/telegram/challenges/"+challenge.ID+"/confirm", f.aliceToken, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: challenge.Version, TelegramUserID: "987654322", Reason: "不匹配的数字身份"}, nil)
		expect(t, rec, 409, res)
		binding = confirm(f.aliceToken, challenge, "987654321")
		if binding.State != "active" {
			t.Fatal("binding not committed")
		}
		var leaked bool
		if err := f.owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM idempotency_records WHERE response_data::text LIKE '%'||$1||'%') OR EXISTS(SELECT 1 FROM audit_events a WHERE to_jsonb(a)::text LIKE '%'||$1||'%') OR EXISTS(SELECT 1 FROM telegram_inbox WHERE request_text LIKE '%'||$1||'%')`, replay.StartToken).Scan(&leaked); err != nil || leaked {
			t.Fatal("start token leaked into persisted output")
		}
	})
	var channel telegram.Channel
	run("only authorized MFA administrators configure paused exact topics", func() {
		in := telegram.ChannelCreate{BotID: bot.ID, ChatID: "-1001122334455", TopicID: 42, Label: "股东测试话题", AllowedSkillIDs: []string{"project-briefing"}, Reason: "配置准确群和话题"}
		rec, res := f.request("POST", "/v1/telegram/channels", f.aliceToken, core.NewID(), in, nil)
		if rec.Code < 400 || res.Error == nil {
			t.Fatal("member configured group")
		}
		rec, res = f.request("POST", "/v1/telegram/channels", admin, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		channel = data[telegram.Channel](t, res)
		if channel.State != "paused" || channel.DisclosureMode != "portal_only" {
			t.Fatal("group activated or expanded disclosure implicitly")
		}
		accepted(request(20, -1001122334455, 42, 987654321, "/ask 请整理项目"))
		if code, text := reason(20); code != "channel_paused" || text != nil {
			t.Fatal("paused group captured request")
		}
		patch := telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, AllowedSkillIDs: channel.AllowedSkillIDs, State: "active", Reason: "核对群范围后启用"}
		rec, res = f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), patch, nil)
		expect(t, rec, 200, res)
		channel = data[telegram.Channel](t, res)
	})
	run("unrelated chats topics senders and nontriggers retain no text", func() {
		samples := []struct {
			id   int64
			body map[string]any
			want string
		}{
			{21, request(21, -1001122334456, 42, 987654321, "/ask 不在白名单"), "channel_not_allowed"},
			{22, request(22, -1001122334455, 43, 987654321, "/ask 其他话题"), "channel_not_allowed"},
			{23, request(23, -1001122334455, 42, 987654322, "/ask 未绑定身份"), "identity_not_bound"},
			{24, request(24, -1001122334455, 42, 987654321, "只是包含CopyPasteTestBot名称"), "not_triggered"},
		}
		samples[3].body["message"].(map[string]any)["entities"] = []any{}
		anonymous := request(25, -1001122334455, 42, 987654321, "/ask 匿名管理员")
		anonymous["message"].(map[string]any)["sender_chat"] = map[string]any{"id": -1001122334455}
		samples = append(samples, struct {
			id   int64
			body map[string]any
			want string
		}{25, anonymous, "sender_not_supported"})
		for _, v := range samples {
			accepted(v.body)
			if code, text := reason(v.id); code != v.want || text != nil {
				t.Fatalf("privacy gate mismatch for update %d", v.id)
			}
		}
		old := request(26, -1001122334455, 42, 987654321, "/ask 旧入口下发出的消息")
		old["message"].(map[string]any)["date"] = time.Now().Add(-time.Hour).Unix()
		accepted(old)
		if code, text := reason(26); code != "channel_changed_after_message" || text != nil {
			t.Fatal("old message acquired a new channel scope")
		}
	})
	run("concurrent duplicate updates persist one real request and no model run", func() {
		body := request(30, -1001122334455, 42, 987654321, "/ask 明确请求，不视为授权修改")
		var wg sync.WaitGroup
		statuses := make(chan int, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); statuses <- webhook(body, bot.WebhookSecret).Code }()
		}
		wg.Wait()
		close(statuses)
		for status := range statuses {
			if status != 200 {
				t.Fatal("duplicate retry failed")
			}
		}
		if code, text := reason(30); code != "executor_not_connected" || text == nil {
			t.Fatal("accepted request not durable")
		}
		var count, runs int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_inbox WHERE bot_id=$1 AND update_id=30", bot.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("duplicate request inserted")
		}
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_runs").Scan(&runs); err != nil || runs != 0 {
			t.Fatal("ingress pretended to start a model")
		}
		altered := request(30, -1001122334455, 42, 987654321, "/ask 已改变内容")
		if out := webhook(altered, bot.WebhookSecret); out.Code != 409 {
			t.Fatal("update identity reused for different content")
		}
		rec, res := f.request("GET", "/v1/me/telegram/requests", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[telegram.Request]](t, res)
		if len(page.Items) != 1 || page.Items[0].State != "awaiting_executor" {
			t.Fatal("own durable request missing")
		}
		rec, res = f.request("GET", "/v1/me/telegram/requests", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[telegram.Request]](t, res).Items) != 0 {
			t.Fatal("foreign employee saw private request")
		}
	})
	run("membership change pauses all topics withdraws pending work and records actual history", func() {
		accepted(map[string]any{"update_id": 40, "chat_member": map[string]any{"chat": map[string]any{"id": int64(-1001122334455), "type": "supergroup"}}})
		rec, res := f.request("GET", "/v1/telegram/channels", admin, "", nil, nil)
		expect(t, rec, 200, res)
		channel = data[core.Page[telegram.Channel]](t, res).Items[0]
		if channel.State != "paused" || channel.Version != 3 {
			t.Fatal("membership did not pause exact group")
		}
		if code, _ := reason(30); code != "group_review_required" {
			t.Fatal("old pending request stayed executable")
		}
		rec, res = f.request("GET", "/v1/telegram/channels/"+channel.ID+"/history", admin, "", nil, nil)
		expect(t, rec, 200, res)
		history := data[core.Page[telegram.Revision]](t, res)
		if len(history.Items) != 3 {
			t.Fatal("channel history missing")
		}
		found := false
		for _, v := range history.Items {
			if v.Source == "membership" && v.Snapshot.State == "paused" && v.Version == channel.Version {
				found = true
			}
		}
		if !found {
			t.Fatal("history differs from actual paused revision")
		}
		rec, res = f.request("GET", "/v1/telegram/channels/"+channel.ID+"/history?limit=1", admin, "", nil, nil)
		expect(t, rec, 200, res)
		one := data[core.Page[telegram.Revision]](t, res)
		if len(one.Items) != 1 || one.Items[0].Version != channel.Version || one.NextCursor == nil {
			t.Fatal("history did not page from latest actual revision")
		}
		rec, res = f.request("GET", "/v1/telegram/channels/"+channel.ID+"/history?limit=1&cursor="+*one.NextCursor, admin, "", nil, nil)
		expect(t, rec, 200, res)
		if data[core.Page[telegram.Revision]](t, res).Items[0].Version != channel.Version-1 {
			t.Fatal("history skipped or repeated a revision")
		}
	})
	run("binding cancellation revocation and identity ownership cannot be bypassed", func() {
		pending := create(f.bobToken, core.NewID())
		rec, res := f.request("POST", "/v1/me/telegram/challenges/"+pending.ID+"/cancel", f.bobToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: pending.Version, Reason: "取消此次绑定"}, nil)
		expect(t, rec, 200, res)
		prove(50, 987654323, pending.StartToken)
		if code, _ := reason(50); code != "binding_proof_invalid" {
			t.Fatal("cancelled proof accepted")
		}
		rec, res = f.request("POST", "/v1/me/telegram/bindings/"+binding.ID+"/revoke", f.bobToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: binding.Version, Reason: "无权撤销他人"}, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("POST", "/v1/me/telegram/bindings/"+binding.ID+"/revoke", f.aliceToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: binding.Version, Reason: "本人撤销TG绑定"}, nil)
		expect(t, rec, 200, res)
		binding = data[telegram.Binding](t, res)
		if binding.State != "revoked" {
			t.Fatal("binding not revoked")
		}
		pending = create(f.bobToken, core.NewID())
		prove(51, 987654321, pending.StartToken)
		pending = status(f.bobToken).Challenges[0]
		rec, res = f.request("POST", "/v1/me/telegram/challenges/"+pending.ID+"/confirm", f.bobToken, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: pending.Version, TelegramUserID: "987654321", Reason: "禁止重分配身份"}, nil)
		expect(t, rec, 409, res)
	})
	run("original identity and receipt content are immutable at database boundary", func() {
		for _, sql := range []string{`UPDATE telegram_identity_owners SET telegram_user_id='777'`, `DELETE FROM telegram_channel_revisions`, `UPDATE telegram_inbox SET request_text='rewritten' WHERE kind='request'`, `UPDATE telegram_bindings SET telegram_user_id='777'`, `UPDATE telegram_channels SET chat_id='-100777',version=version+1`} {
			if _, err := f.api.DB.Exec(ctx, sql); err == nil {
				t.Fatal("database accepted identity or receipt rewrite")
			}
		}
	})
	run("issuing a binding requires actual recent MFA", func() {
		exec(`UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL`, f.org, f.bob)
		rec, res := f.request("POST", "/v1/me/telegram/challenges", f.bobToken, core.NewID(), telegram.ChallengeCreate{BotID: bot.ID, Reason: "过期验证不能发起绑定"}, nil)
		expect(t, rec, 403, res)
		if res.Error.Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("recent MFA was not enforced")
		}
	})
	if os.Getenv("CP_BROWSER_TELEGRAM_FIXTURE") == "1" {
		holdBrowserBusinessFixture(t, f, map[string]any{"telegram_ingress_fixture": true, "bot_id": bot.ID, "bot_username": bot.Username, "webhook_secret": bot.WebhookSecret, "channel_id": channel.ID})
	}
	run("original login revocation invalidates unused private proof", func() {
		pending := create(f.aliceToken, core.NewID())
		rec, res := f.request("GET", "/v1/me/sessions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		var current string
		for _, v := range data[core.Page[identitySession]](t, res).Items {
			if v.Current {
				current = v.ID
			}
		}
		if current == "" {
			t.Fatal("current login missing")
		}
		rec, res = f.request("POST", "/v1/me/sessions/"+current+"/revoke", f.aliceToken, core.NewID(), map[string]string{"reason": "撤销绑定来源登录"}, nil)
		expect(t, rec, 200, res)
		prove(60, 987654321, pending.StartToken)
		if code, _ := reason(60); code != "binding_proof_invalid" {
			t.Fatal("revoked original login proof accepted")
		}
	})
}

// The route has a larger device schema; this test deliberately reads only the
// two fields needed to revoke the genuine current login through its public API.
type identitySession struct {
	ID      string `json:"id"`
	Current bool   `json:"is_current"`
}
