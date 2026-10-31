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
	"copypaste.hk/platform/internal/telegram"
)

// Native durable PG and actual authenticated HTTP; provider results and group
// identities are synthetic. No Bot API, model or cloud storage is contacted.
func TestTelegramOperations(t *testing.T) {
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
	p := f.createProject("TG organization operations", []string{f.bob, f.admin})
	bot := telegram.BotConfig{ID: "123456789", OrgID: f.org, Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("t", 40)}
	f.api.Telegram = &telegram.Service{DB: f.api.DB, Config: f.api.Config, Bots: []telegram.BotConfig{bot}}
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='member'", core.NewID(), f.org, f.admin)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_operations_reader'", core.NewID(), f.org, f.member)
	managerBinding := core.NewID()
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_operations_manager'", managerBinding, f.org, f.bob)
	for _, level := range []string{"internal", "project_restricted"} {
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,$3,true,ARRAY['web','wechat','douyin','android','telegram'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3", core.NewID(), f.org, level)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	f.memberToken = f.login("member", "android").AccessToken
	admin := f.administratorToken()
	seq := int64(100)
	accept := func(body any) {
		current.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/v1/integrations/telegram/"+bot.ID+"/webhook", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", bot.WebhookSecret)
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, r)
		if out.Code != 200 {
			current.Fatalf("synthetic webhook status %d", out.Code)
		}
	}
	r, v := f.request("POST", "/v1/me/telegram/challenges", f.aliceToken, core.NewID(), telegram.ChallengeCreate{BotID: bot.ID, Reason: "绑定本人测试身份"}, nil)
	expect(t, r, 201, v)
	challenge := data[telegram.Challenge](t, v)
	seq++
	accept(map[string]any{"update_id": seq, "message": map[string]any{"message_id": seq, "from": map[string]any{"id": 987654321, "is_bot": false}, "chat": map[string]any{"id": 987654321, "type": "private"}, "text": "/start " + challenge.StartToken, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}})
	r, v = f.request("GET", "/v1/me/telegram", f.aliceToken, "", nil, nil)
	expect(t, r, 200, v)
	challenge = data[telegram.Status](t, v).Challenges[0]
	r, v = f.request("POST", "/v1/me/telegram/challenges/"+challenge.ID+"/confirm", f.aliceToken, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: challenge.Version, TelegramUserID: *challenge.TelegramUserID, Reason: "原登录确认测试数字身份"}, nil)
	expect(t, r, 201, v)
	skill := "project-briefing"
	r, v = f.request("POST", "/v1/telegram/channels", admin, core.NewID(), telegram.ChannelCreate{BotID: bot.ID, ChatID: "-1001122334455", TopicID: 42, Label: "股东群项目话题", ProjectID: &p.ID, AllowedSkillIDs: []string{skill}, DefaultSkillID: &skill, DeliveryEnabled: true, Reason: "启用合成通知旅程"}, nil)
	expect(t, r, 201, v)
	channel := data[telegram.Channel](t, v)
	r, v = f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, ProjectID: channel.ProjectID, AllowedSkillIDs: channel.AllowedSkillIDs, DefaultSkillID: channel.DefaultSkillID, DeliveryEnabled: true, State: "active", Reason: "明确启用通知入口"}, nil)
	expect(t, r, 200, v)
	channel = data[telegram.Channel](t, v)
	outbound := telegram.DeliveryBot{BotID: bot.ID, OrgID: f.org, Token: bot.ID + ":" + strings.Repeat("d", 35), PortalOrigin: "https://portal.example.test"}
	unknown := func() telegram.Delivery {
		current.Helper()
		seq++
		accept(map[string]any{"update_id": seq, "message": map[string]any{"message_id": seq, "date": time.Now().Unix(), "message_thread_id": 42, "from": map[string]any{"id": 987654321, "is_bot": false}, "chat": map[string]any{"id": int64(-1001122334455), "type": "supergroup"}, "text": "/brief PRIVATE_REQUEST_BODY 不可进入组织运维正文", "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}})
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1", bot.ID)
		prepared, _, err := f.api.Telegram.PrepareDelivery(ctx, outbound)
		if err != nil || prepared == nil {
			current.Fatalf("prepare synthetic group notification: %v", err)
		}
		if strings.Contains(prepared.Message.Text, "PRIVATE_REQUEST_BODY") {
			current.Fatal("request leaked into outbound")
		}
		if err = f.api.Telegram.FinishDelivery(ctx, outbound, *prepared, telegram.DeliveryOutcome{State: "delivery_unknown", Reason: "provider_outcome_unknown"}); err != nil {
			current.Fatal(err)
		}
		r, v := f.request("GET", "/v1/me/telegram/requests/"+prepared.Delivery.InboxID+"/deliveries", f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		return data[telegram.DeliveryDetails](current, v).Deliveries[0]
	}
	get := func(d telegram.Delivery) telegram.OperationDetails {
		current.Helper()
		r, v := f.request("GET", "/v1/telegram/operations/"+d.ID, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		if strings.Contains(r.Body.String(), "PRIVATE_") || strings.Contains(r.Body.String(), `"text"`) || strings.Contains(r.Body.String(), `"reason"`) {
			current.Fatal("organization metadata exposed private prose")
		}
		return data[telegram.OperationDetails](current, v)
	}
	body := func(d telegram.Delivery, action string) telegram.OperationReconcile {
		return telegram.OperationReconcile{DeliveryReconcile: telegram.DeliveryReconcile{ExpectedVersion: d.Version, Action: action, Reason: "PRIVATE_JOURNAL_NOTE 已核对原群和话题"}, ReviewedOriginalChannel: true, DuplicateRiskAccepted: action == "retry_confirmed_not_sent"}
	}
	post := func(d telegram.Delivery, in telegram.OperationReconcile, token string, headers map[string]string, status int, key string) telegram.Delivery {
		current.Helper()
		r, v := f.request("POST", "/v1/telegram/operations/"+d.ID+"/reconcile", token, key, in, headers)
		expect(current, r, status, v)
		if status == 200 {
			return data[telegram.Delivery](current, v)
		}
		return d
	}
	login := func(device string) (string, map[string]string) {
		var cipher []byte
		if e := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.bob).Scan(&cipher); e != nil {
			current.Fatal(e)
		}
		secret, e := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.bob)
		if e != nil {
			current.Fatal(e)
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.bob, time.Now().Unix()/30-1)
		c := f.login("bob", device)
		h := map[string]string{}
		if device == "web" {
			h["Origin"] = "http://localhost:5178"
		}
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": c.Challenge, "code": authenticatorCode(secret, time.Now())}, h)
		expect(current, r, 200, v)
		l := data[identity.LoginResult](current, v)
		if device == "web" {
			h["X-CSRF-Token"] = l.CSRFToken
			for _, cookie := range r.Result().Cookies() {
				if cookie.Name == "cp_session" {
					h["Cookie"] = "cp_session=" + cookie.Value
				}
			}
			return "", h
		}
		return l.AccessToken, h
	}
	for _, device := range []string{"web", "wechat", "douyin", "android"} {
		run(device+" metadata and manual reconciliation", func() {
			d := unknown()
			token, headers := login(device)
			r, v := f.request("GET", "/v1/telegram/operations?state=delivery_unknown&channel_id="+channel.ID, token, "", nil, headers)
			expect(current, r, 200, v)
			page := data[core.Page[telegram.Operation]](current, v)
			if len(page.Items) != 1 || page.Items[0].ID != d.ID || page.Items[0].RequesterID != f.alice || strings.Contains(r.Body.String(), "PRIVATE_") {
				current.Fatal("organization metadata source or allowlist changed")
			}
			details := get(d)
			if !details.Item.RetryAuthorized || len(details.Events) != 3 {
				current.Fatal("missing current scope or exact journal")
			}
			in := body(d, "confirm_sent")
			message := int64(888001)
			in.MessageID = &message
			key := core.NewID()
			result := post(d, in, token, headers, 200, key)
			post(d, in, token, headers, 200, key)
			if result.State != "confirmed_sent" || result.Version != d.Version+1 {
				current.Fatal("manual confirmation changed provider history")
			}
			if got := get(d); len(got.Events) != 4 || got.Events[3].ActorID == nil || *got.Events[3].ActorID != f.bob {
				current.Fatal("missing immutable real operator")
			}
			var audits, journals int
			err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM audit_events WHERE resource_id=$1 AND action='telegram.operation_reconciled'),(SELECT count(*) FROM telegram_delivery_events WHERE delivery_id=$1)", d.ID).Scan(&audits, &journals)
			if err != nil || audits != 1 || journals != 4 {
				current.Fatal("replay manufactured extra facts")
			}
		})
	}
	run("separate read authority and unchanged owner-only prose", func() {
		d := unknown()
		r, v := f.request("GET", "/v1/telegram/operations", f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/telegram/operations", f.memberToken, "", nil, nil)
		expect(current, r, 200, v)
		post(d, body(d, "abandon"), f.memberToken, nil, 403, core.NewID())
		post(d, body(d, "abandon"), f.aliceToken, nil, 404, core.NewID())
		r, v = f.request("GET", "/v1/me/telegram/requests/"+d.InboxID, f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/me/telegram/requests/"+d.InboxID, f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		if !strings.Contains(r.Body.String(), "PRIVATE_REQUEST_BODY") {
			current.Fatal("owner source was replaced")
		}
		post(d, body(d, "abandon"), f.bobToken, nil, 200, core.NewID())
	})
	run("fixed filter-bound pagination and strict input", func() {
		r, v := f.request("GET", "/v1/telegram/operations?limit=1", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		pg := data[core.Page[telegram.Operation]](current, v)
		if pg.NextCursor == nil {
			current.Fatal("expected page cursor")
		}
		r, v = f.request("GET", "/v1/telegram/operations?limit=1&cursor="+*pg.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		if data[core.Page[telegram.Operation]](current, v).Items[0].ID == pg.Items[0].ID {
			current.Fatal("pagination repeated head")
		}
		r, v = f.request("GET", "/v1/telegram/operations?state=sent&cursor="+*pg.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 400, v)
		for _, path := range []string{"?state=invalid", "?channel_id=invalid", "?state=sent&state=failed", "?q=PRIVATE", "?limit=101"} {
			r, v = f.request("GET", "/v1/telegram/operations"+path, f.bobToken, "", nil, nil)
			expect(current, r, 422, v)
		}
		r, v = f.request("GET", "/v1/telegram/operations/"+core.NewID(), f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	run("retry requires explicit review, risk and current version", func() {
		d := unknown()
		in := body(d, "retry_confirmed_not_sent")
		in.ReviewedOriginalChannel = false
		post(d, in, f.bobToken, nil, 422, core.NewID())
		in.ReviewedOriginalChannel = true
		in.DuplicateRiskAccepted = false
		post(d, in, f.bobToken, nil, 422, core.NewID())
		in.DuplicateRiskAccepted = true
		in.ExpectedVersion++
		post(d, in, f.bobToken, nil, 409, core.NewID())
		in.ExpectedVersion = d.Version
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE user_id=$1", f.bob)
		post(d, in, f.bobToken, nil, 403, core.NewID())
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE user_id=$1", f.bob)
		result := post(d, in, f.bobToken, nil, 200, core.NewID())
		if result.State != "retry_wait" || result.Attempts != d.Attempts {
			current.Fatal("queue manufactured a send")
		}
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1", bot.ID)
		prepared, _, err := f.api.Telegram.PrepareDelivery(ctx, outbound)
		if err != nil || prepared == nil || prepared.Delivery.ID != d.ID || prepared.Delivery.Attempts != 2 || strings.Contains(prepared.Message.Text, "PRIVATE_") {
			current.Fatalf("fixed retry not prepared: %v", err)
		}
		message := int64(888002)
		if err = f.api.Telegram.FinishDelivery(ctx, outbound, *prepared, telegram.DeliveryOutcome{State: "sent", Reason: "provider_confirmed", MessageID: &message}); err != nil {
			current.Fatal(err)
		}
		if got := get(d); got.Item.State != "sent" || len(got.Events) != 6 {
			current.Fatal("retry receipts missing")
		}
	})
	run("expired original request cannot be requeued", func() {
		d := unknown()
		// Only this isolated fixture advances the immutable received clock.
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			current.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "ALTER TABLE telegram_inbox DISABLE TRIGGER telegram_inbox_guard"); err != nil {
			current.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "UPDATE telegram_inbox SET received_at=clock_timestamp()-interval '25 hours' WHERE id=$1", d.InboxID); err != nil {
			current.Fatal(err)
		}
		for _, q := range []string{"SET CONSTRAINTS ALL IMMEDIATE", "ALTER TABLE telegram_inbox ENABLE TRIGGER telegram_inbox_guard"} {
			if _, err = tx.Exec(ctx, q); err != nil {
				current.Fatal(err)
			}
		}
		if err = tx.Commit(ctx); err != nil {
			current.Fatal(err)
		}
		if get(d).Item.RetryAuthorized {
			current.Fatal("expired request still claims retry")
		}
		post(d, body(d, "retry_confirmed_not_sent"), f.bobToken, nil, 409, core.NewID())
		post(d, body(d, "abandon"), f.bobToken, nil, 200, core.NewID())
	})
	run("operator retries retain the five-attempt cap", func() {
		d := unknown()
		for count := 1; count < telegram.DeliveryMaxAttempts; count++ {
			post(d, body(d, "retry_confirmed_not_sent"), f.bobToken, nil, 200, core.NewID())
			exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1", bot.ID)
			p, _, err := f.api.Telegram.PrepareDelivery(ctx, outbound)
			if err != nil || p == nil || p.Delivery.ID != d.ID {
				current.Fatalf("bounded retry %d: %v", count, err)
			}
			if err = f.api.Telegram.FinishDelivery(ctx, outbound, *p, telegram.DeliveryOutcome{State: "delivery_unknown", Reason: "provider_outcome_unknown"}); err != nil {
				current.Fatal(err)
			}
			d = get(d).Item.Delivery
		}
		if get(d).Item.RetryAuthorized {
			current.Fatal("exhausted delivery still claims retry")
		}
		post(d, body(d, "retry_confirmed_not_sent"), f.bobToken, nil, 409, core.NewID())
		post(d, body(d, "abandon"), f.bobToken, nil, 200, core.NewID())
	})
	run("audit failure rolls back both delivery and journal", func() {
		d := unknown()
		exec("CREATE FUNCTION reject_tg_ops_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='telegram.operation_reconciled' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $$")
		exec("CREATE TRIGGER reject_tg_ops_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_tg_ops_audit()")
		post(d, body(d, "abandon"), f.bobToken, nil, 500, core.NewID())
		if got := get(d); got.Item.Version != d.Version || len(got.Events) != 3 {
			current.Fatal("audit failure left partial facts")
		}
		exec("DROP TRIGGER reject_tg_ops_audit ON audit_events")
		exec("DROP FUNCTION reject_tg_ops_audit()")
		post(d, body(d, "abandon"), f.bobToken, nil, 200, core.NewID())
	})
	run("changed recipient blocks retry but preserves human closure", func() {
		d := unknown()
		r, v := f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, ProjectID: channel.ProjectID, AllowedSkillIDs: channel.AllowedSkillIDs, DefaultSkillID: channel.DefaultSkillID, DeliveryEnabled: true, State: "active", Reason: "更新入口使旧范围失效"}, nil)
		expect(current, r, 200, v)
		channel = data[telegram.Channel](current, v)
		if get(d).Item.RetryAuthorized {
			current.Fatal("changed route still claims retry")
		}
		post(d, body(d, "retry_confirmed_not_sent"), f.bobToken, nil, 409, core.NewID())
		post(d, body(d, "abandon"), f.bobToken, nil, 200, core.NewID())
	})
	run("revoked reconciliation authority is checked before replay", func() {
		d := unknown()
		in := body(d, "abandon")
		key := core.NewID()
		post(d, in, f.bobToken, nil, 200, key)
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp(),version=version+1 WHERE id=$1", managerBinding)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		post(d, in, f.bobToken, nil, 404, key)
	})
	run("browser_workbench", func() {
		if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" {
			return
		}
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_operations_manager'", core.NewID(), f.org, f.alice)
		// Current Alice token is invalid after the explicit fixture authorization
		// change above; obtain a new real session for the synthetic request source.
		var cipher []byte
		if err := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.alice).Scan(&cipher); err != nil {
			current.Fatal(err)
		}
		secret, err := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.alice)
		if err != nil {
			current.Fatal(err)
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.alice, time.Now().Unix()/30-1)
		c := f.login("alice", "android")
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": c.Challenge, "code": authenticatorCode(secret, time.Now())}, nil)
		expect(current, r, 200, v)
		f.aliceToken = data[identity.LoginResult](current, v).AccessToken
		d := unknown()
		holdBrowserBusinessFixture(current, f, map[string]any{"fixture_kind": "telegram-operations", "delivery_id": d.ID, "request_id": d.InboxID, "channel_id": channel.ID})
		var state string
		var journal, audits int
		if err = f.owner.QueryRow(ctx, "SELECT state,(SELECT count(*) FROM telegram_delivery_events WHERE delivery_id=d.id),(SELECT count(*) FROM audit_events WHERE resource_id=d.id AND action='telegram.operation_reconciled') FROM telegram_deliveries d WHERE id=$1", d.ID).Scan(&state, &journal, &audits); err != nil || state != "confirmed_sent" || journal != 4 || audits != 1 {
			current.Fatal("browser did not persist one exact human reconciliation")
		}
	})
}
