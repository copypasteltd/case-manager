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

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/telegram"
)

// Native durable PG and actual authenticated HTTP; provider results and group
// identities are synthetic. No Bot API, model or cloud storage is contacted.
func TestTelegramInboxOperations(t *testing.T) {
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
	p := f.createProject("TG inbound queue operations", []string{f.bob, f.admin})
	bot := telegram.BotConfig{ID: "123456789", OrgID: f.org, Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("t", 40)}
	f.api.Telegram = &telegram.Service{DB: f.api.DB, Config: f.api.Config, Bots: []telegram.BotConfig{bot}}
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='member'", core.NewID(), f.org, f.admin)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_inbox_reader'", core.NewID(), f.org, f.member)
	managerBinding := core.NewID()
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_inbox_manager'", managerBinding, f.org, f.bob)
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

	service := &agent.Service{DB: f.api.DB}
	pending := func() string {
		current.Helper()
		seq++
		accept(map[string]any{"update_id": seq, "message": map[string]any{"message_id": seq, "date": time.Now().Unix(), "message_thread_id": 42, "from": map[string]any{"id": 987654321, "is_bot": false}, "chat": map[string]any{"id": int64(-1001122334455), "type": "supergroup"}, "text": "/brief PRIVATE_REQUEST_BODY 不可进入组织入站台账", "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}})
		var id string
		if err := f.owner.QueryRow(ctx, "SELECT id FROM telegram_inbox WHERE bot_id=$1 AND update_id=$2 AND kind='request'", bot.ID, seq).Scan(&id); err != nil {
			current.Fatal(err)
		}
		return id
	}
	get := func(id string) telegram.InboxOperationDetails {
		current.Helper()
		r, v := f.request("GET", "/v1/telegram/inbox-operations/"+id, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		for _, forbidden := range []string{"PRIVATE_", `"text"`, `"reason"`, `"telegram_user_id"`, `"binding_id"`, `"conversation_id"`} {
			if strings.Contains(r.Body.String(), forbidden) {
				current.Fatal("private data escaped request metadata allowlist")
			}
		}
		return data[telegram.InboxOperationDetails](current, v)
	}
	body := func(v telegram.InboxOperation) telegram.InboxWithdraw {
		return telegram.InboxWithdraw{ExpectedVersion: v.Version, ReviewedRequest: true, Reason: "PRIVATE_OPERATOR_NOTE 已核对尚未启动的准确请求"}
	}
	post := func(id string, in telegram.InboxWithdraw, token string, headers map[string]string, status int, key string) telegram.InboxOperation {
		current.Helper()
		r, v := f.request("POST", "/v1/telegram/inbox-operations/"+id+"/withdraw", token, key, in, headers)
		expect(current, r, status, v)
		if status == 200 {
			return data[telegram.InboxOperation](current, v)
		}
		return telegram.InboxOperation{}
	}
	closePending := func(id string) { post(id, body(get(id).Item), f.bobToken, nil, 200, core.NewID()) }
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
		run(device+" metadata withdrawal and replay", func() {
			id := pending()
			token, headers := login(device)
			r, v := f.request("GET", "/v1/telegram/inbox-operations?state=awaiting_executor&channel_id="+channel.ID+"&skill_id="+skill, token, "", nil, headers)
			expect(current, r, 200, v)
			list := data[core.Page[telegram.InboxOperation]](current, v)
			if len(list.Items) != 1 || list.Items[0].ID != id || list.Items[0].RequesterID != f.alice || strings.Contains(r.Body.String(), "PRIVATE_") {
				current.Fatal("wrong queue source or prose leak")
			}
			before := get(id)
			if before.Item.Version != 1 || len(before.History.Items) != 1 || before.History.Items[0].Source != "accepted" || !before.Item.ScopeCurrent || before.Item.ProfileConfigured {
				current.Fatal("incorrect actual admission metadata")
			}
			input := body(before.Item)
			key := core.NewID()
			result := post(id, input, token, headers, 200, key)
			post(id, input, token, headers, 200, key)
			after := get(id)
			if result.State != "withdrawn" || result.Withdrawable || result.Version != 2 || len(after.History.Items) != 2 || after.History.Items[0].ActorID == nil || *after.History.Items[0].ActorID != f.bob {
				current.Fatal("withdrawal lacks exact immutable operator event")
			}
			var audits, receipts, jobs int
			err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM audit_events WHERE resource_id=$1 AND action='telegram.inbox_withdrawn'),(SELECT count(*) FROM telegram_request_withdrawals WHERE inbox_id=$1),(SELECT count(*) FROM telegram_run_links WHERE inbox_id=$1)", id).Scan(&audits, &receipts, &jobs)
			if err != nil || audits != 1 || receipts != 1 || jobs != 0 {
				current.Fatal("replay duplicated facts or created a run")
			}
			if handled, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); err != nil || handled {
				current.Fatal("withdrawn request was picked by dispatcher", err)
			}
		})
	}
	run("independent read and withdraw roles preserve private owner prose", func() {
		id := pending()
		in := body(get(id).Item)
		r, v := f.request("GET", "/v1/telegram/inbox-operations", f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/telegram/inbox-operations", f.memberToken, "", nil, nil)
		expect(current, r, 200, v)
		post(id, in, f.memberToken, nil, 403, core.NewID())
		post(id, in, f.aliceToken, nil, 404, core.NewID())
		r, v = f.request("GET", "/v1/me/telegram/requests/"+id, f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/me/telegram/requests/"+id, f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		if !strings.Contains(r.Body.String(), "PRIVATE_REQUEST_BODY") {
			current.Fatal("private owner source replaced")
		}
		var privateID string
		if err := f.owner.QueryRow(ctx, "SELECT id FROM telegram_inbox WHERE kind='binding_proof' LIMIT 1").Scan(&privateID); err != nil {
			current.Fatal(err)
		}
		r, v = f.request("GET", "/v1/telegram/inbox-operations/"+privateID, f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
		closePending(id)
	})
	run("filter and actor bound pagination strict query and full history", func() {
		first, second := pending(), pending()
		r, v := f.request("GET", "/v1/telegram/inbox-operations?limit=1&state=awaiting_executor", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		page := data[core.Page[telegram.InboxOperation]](current, v)
		if page.NextCursor == nil {
			current.Fatal("missing request cursor")
		}
		r, v = f.request("GET", "/v1/telegram/inbox-operations?limit=1&state=awaiting_executor&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		if data[core.Page[telegram.InboxOperation]](current, v).Items[0].ID == page.Items[0].ID {
			current.Fatal("repeated queue head")
		}
		r, v = f.request("GET", "/v1/telegram/inbox-operations?state=withdrawn&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 400, v)
		r, v = f.request("GET", "/v1/telegram/inbox-operations?state=awaiting_executor&cursor="+*page.NextCursor, f.memberToken, "", nil, nil)
		expect(current, r, 400, v)
		for _, query := range []string{"limit=0", "limit=101", "state=ignored", "skill_id=file-update", "channel_id=invalid", "state=withdrawn&state=awaiting_executor", "text=private"} {
			r, v = f.request("GET", "/v1/telegram/inbox-operations?"+query, f.bobToken, "", nil, nil)
			expect(current, r, 422, v)
		}
		// Repeated equal-state wait fixture writes exercise complete history paging.
		// The next scenario separately invokes the actual dispatcher.
		for index := 0; index < 22; index++ {
			exec("UPDATE telegram_inbox SET dispatch_after=dispatch_after+interval '1 microsecond',reason_code='RUNTIME_UNAVAILABLE' WHERE id=$1", first)
		}
		details := get(first)
		if details.Item.Version != 23 || len(details.History.Items) != 20 || details.History.NextCursor == nil {
			current.Fatal("history silently truncated")
		}
		r, v = f.request("GET", "/v1/telegram/inbox-operations/"+first+"/history?limit=20&cursor="+*details.History.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		tail := data[core.Page[telegram.InboxOperationEvent]](current, v)
		if len(tail.Items) != 3 || tail.NextCursor != nil || tail.Items[2].Version != 1 {
			current.Fatal("history omitted original acceptance")
		}
		r, v = f.request("GET", "/v1/telegram/inbox-operations/"+second+"/history?cursor="+*details.History.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 400, v)
		closePending(first)
		closePending(second)
	})
	run("actual dispatcher wait invalidates stale withdrawal", func() {
		id := pending()
		before := get(id)
		if handled, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); err != nil || !handled {
			current.Fatal("actual dispatcher did not process request", err)
		}
		after := get(id)
		if after.Item.Version != 2 || after.Item.ReasonCode != "RUNTIME_UNAVAILABLE" || len(after.History.Items) != 2 || after.History.Items[0].Source != "transition" {
			current.Fatal("waiting transition missing")
		}
		post(id, body(before.Item), f.bobToken, nil, 409, core.NewID())
		closePending(id)
	})
	run("explicit review recent MFA and exact version", func() {
		id := pending()
		item := get(id).Item
		in := body(item)
		in.ReviewedRequest = false
		post(id, in, f.bobToken, nil, 422, core.NewID())
		in = body(item)
		in.ExpectedVersion++
		post(id, in, f.bobToken, nil, 409, core.NewID())
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		post(id, body(item), f.bobToken, nil, 403, core.NewID())
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		closePending(id)
	})
	run("audit failure rolls back request journal and receipt", func() {
		id := pending()
		before := get(id)
		exec("CREATE FUNCTION reject_tg_inbox_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='telegram.inbox_withdrawn' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $$")
		exec("CREATE TRIGGER reject_tg_inbox_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_tg_inbox_audit()")
		post(id, body(before.Item), f.bobToken, nil, 500, core.NewID())
		if after := get(id); after.Item.Version != 1 || after.Item.State != "awaiting_executor" || len(after.History.Items) != 1 {
			current.Fatal("failed audit left a partial withdrawal")
		}
		var receipts int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_request_withdrawals WHERE inbox_id=$1", id).Scan(&receipts); err != nil || receipts != 0 {
			current.Fatal("failed audit left a receipt")
		}
		exec("DROP TRIGGER reject_tg_inbox_audit ON audit_events")
		exec("DROP FUNCTION reject_tg_inbox_audit()")
		closePending(id)
	})
	run("database journal cannot be forged mutated or bypassed", func() {
		id := pending()
		if _, err := f.api.DB.Exec(ctx, "INSERT INTO telegram_request_events(org_id,inbox_id,version,state,reason_code,dispatch_after,source) SELECT org_id,id,2,state,reason_code,dispatch_after,'transition' FROM telegram_inbox WHERE id=$1", id); err == nil {
			current.Fatal("application forged an observed journal")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE telegram_request_events SET reason_code='invented' WHERE inbox_id=$1", id); err == nil {
			current.Fatal("journal rewritten")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE telegram_inbox SET state='withdrawn',reason_code='manual_queue_withdrawn' WHERE id=$1", id); err == nil {
			current.Fatal("manual withdrawal bypassed actual receipt")
		}
		if got := get(id); got.Item.Version != 1 || got.Item.State != "awaiting_executor" {
			current.Fatal("failed bypass changed head")
		}
		closePending(id)
	})
	run("dispatcher withdrawal race produces one coherent outcome", func() {
		profile := core.NewID()
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,telegram_configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','controlled-test',repeat('a',64),$3,repeat('b',64),repeat('d',64),'active',10000,1000,8,600,$4,'Isolated TG inbound race fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
		id := pending()
		item := get(id).Item
		start := make(chan struct{})
		worker := make(chan error, 1)
		human := make(chan int, 1)
		go func() { <-start; _, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); worker <- err }()
		go func() {
			<-start
			r, _ := f.request("POST", "/v1/telegram/inbox-operations/"+id+"/withdraw", f.bobToken, core.NewID(), body(item), nil)
			human <- r.Code
		}()
		close(start)
		workerError, humanStatus := <-worker, <-human
		if workerError != nil {
			current.Fatal("racing worker failed", workerError)
		}
		after := get(id)
		var links int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_run_links WHERE inbox_id=$1", id).Scan(&links); err != nil {
			current.Fatal(err)
		}
		if !((humanStatus == 200 && after.Item.State == "withdrawn" && links == 0) || (humanStatus == 409 && after.Item.State == "dispatched" && links == 1)) {
			current.Fatalf("incoherent race: status %d state %s links %d", humanStatus, after.Item.State, links)
		}
		if after.Item.State == "withdrawn" {
			id = pending()
			if handled, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); err != nil || !handled {
				current.Fatal(err)
			}
			after = get(id)
		}
		if after.Item.State != "dispatched" || after.Item.RunID == nil || after.Item.Withdrawable || !after.Item.ProfileConfigured {
			current.Fatal("actual dispatch metadata missing")
		}
		post(id, body(after.Item), f.bobToken, nil, 409, core.NewID())
	})
	run("automatic channel withdrawal is journaled without invented human decision", func() {
		id := pending()
		r, v := f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, ProjectID: channel.ProjectID, AllowedSkillIDs: channel.AllowedSkillIDs, DefaultSkillID: channel.DefaultSkillID, DeliveryEnabled: true, State: "active", Reason: "更新入口触发实际自动撤回"}, nil)
		expect(current, r, 200, v)
		channel = data[telegram.Channel](current, v)
		after := get(id)
		if after.Item.State != "withdrawn" || after.Item.ScopeCurrent || len(after.History.Items) != 2 || after.History.Items[0].ActorID != nil || after.History.Items[0].ReasonCode != "channel_changed" {
			current.Fatal("automatic scope withdrawal incorrectly journaled")
		}
	})
	run("current role is checked before successful replay", func() {
		id := pending()
		in := body(get(id).Item)
		key := core.NewID()
		post(id, in, f.bobToken, nil, 200, key)
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp(),version=version+1 WHERE id=$1", managerBinding)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		post(id, in, f.bobToken, nil, 404, key)
	})
	run("browser_workbench", func() {
		if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" {
			return
		}
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_inbox_manager'", core.NewID(), f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		id := pending()
		holdBrowserBusinessFixture(current, f, map[string]any{"fixture_kind": "telegram-inbox", "request_id": id, "channel_id": channel.ID})
		var state string
		var version, journal, receipts, audits, links int
		err := f.owner.QueryRow(ctx, `SELECT state,(SELECT max(version) FROM telegram_request_events WHERE inbox_id=i.id),(SELECT count(*) FROM telegram_request_events WHERE inbox_id=i.id),(SELECT count(*) FROM telegram_request_withdrawals WHERE inbox_id=i.id),(SELECT count(*) FROM audit_events WHERE resource_id=i.id AND action='telegram.inbox_withdrawn'),(SELECT count(*) FROM telegram_run_links WHERE inbox_id=i.id) FROM telegram_inbox i WHERE id=$1`, id).Scan(&state, &version, &journal, &receipts, &audits, &links)
		if err != nil || state != "withdrawn" || version != 2 || journal != 2 || receipts != 1 || audits != 1 || links != 0 {
			current.Fatal("browser did not persist exactly one unstarted withdrawal")
		}
	})
}
