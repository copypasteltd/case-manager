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
	"copypaste.hk/platform/internal/telegram"
)

type controlledTelegramDelivery func(context.Context, telegram.DeliveryBot, telegram.OutboundMessage) telegram.DeliveryOutcome

func (f controlledTelegramDelivery) Send(ctx context.Context, b telegram.DeliveryBot, m telegram.OutboundMessage) telegram.DeliveryOutcome {
	return f(ctx, b, m)
}

// All bot, profile and runtime identities are isolated fixtures. Only the real
// native database, HTTP authentication, task queue and controller are exercised.
func TestTelegramDelivery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	current := t
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			current.Fatal(err)
		}
	}
	run := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { current = st; f.t = st; defer func() { current = t; f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	project := f.createProject("Telegram actual project scope", []string{f.bob, f.admin})
	bot := telegram.BotConfig{ID: "123456789", OrgID: f.org, Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("t", 40)}
	f.api.Telegram = &telegram.Service{DB: f.api.DB, Config: f.api.Config, Bots: []telegram.BotConfig{bot}}
	service := &agent.Service{DB: f.api.DB}
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	for _, user := range []string{f.alice, f.bob} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, user)
	}
	for _, level := range []string{"internal", "project_restricted"} {
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,$3,true,ARRAY['web','wechat','douyin','android','telegram'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3", core.NewID(), f.org, level)
	}
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='member'", core.NewID(), f.org, f.admin)
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='status_operator'`, core.NewID(), f.org, f.alice)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	admin := f.administratorToken()
	sequence := int64(100)
	accept := func(body any) {
		current.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/v1/integrations/telegram/"+bot.ID+"/webhook", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", bot.WebhookSecret)
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, r)
		if out.Code != 200 {
			current.Fatalf("isolated webhook status %d", out.Code)
		}
	}
	bind := func(token string, id int64) telegram.Binding {
		current.Helper()
		rec, res := f.request("POST", "/v1/me/telegram/challenges", token, core.NewID(), telegram.ChallengeCreate{BotID: bot.ID, Reason: "绑定渠道测试的本人身份"}, nil)
		expect(current, rec, 201, res)
		c := data[telegram.Challenge](current, res)
		sequence++
		accept(map[string]any{"update_id": sequence, "message": map[string]any{"message_id": sequence, "from": map[string]any{"id": id, "is_bot": false}, "chat": map[string]any{"id": id, "type": "private"}, "text": "/start " + c.StartToken, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": 6}}}})
		rec, res = f.request("GET", "/v1/me/telegram", token, "", nil, nil)
		expect(current, rec, 200, res)
		c = data[telegram.Status](current, res).Challenges[0]
		rec, res = f.request("POST", "/v1/me/telegram/challenges/"+c.ID+"/confirm", token, core.NewID(), telegram.ChallengeConfirm{ExpectedVersion: c.Version, TelegramUserID: *c.TelegramUserID, Reason: "原登录确认渠道数字身份"}, nil)
		expect(current, rec, 201, res)
		return data[telegram.Binding](current, res)
	}
	binding := bind(f.aliceToken, 987654321)
	bind(f.bobToken, 987654322)
	defaultSkill := "project-briefing"
	rec, res := f.request("POST", "/v1/telegram/channels", admin, core.NewID(), telegram.ChannelCreate{BotID: bot.ID, ChatID: "-1001122334455", TopicID: 42, Label: "股东群项目话题", ProjectID: &project.ID, AllowedSkillIDs: []string{"project-briefing", "incident-summary"}, DefaultSkillID: &defaultSkill, Reason: "核对指定项目和任务"}, nil)
	expect(t, rec, 201, res)
	channel := data[telegram.Channel](t, res)
	updateChannel := func(state string, enabled bool) {
		current.Helper()
		rec, res := f.request("GET", "/v1/telegram/channels", admin, "", nil, nil)
		expect(current, rec, 200, res)
		for _, c := range data[core.Page[telegram.Channel]](current, res).Items {
			if c.ID == channel.ID {
				channel = c
			}
		}
		rec, res = f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, ProjectID: channel.ProjectID, AllowedSkillIDs: channel.AllowedSkillIDs, DefaultSkillID: channel.DefaultSkillID, DeliveryEnabled: enabled, State: state, Reason: "明确核对当前入口版本"}, nil)
		expect(current, rec, 200, res)
		channel = data[telegram.Channel](current, res)
	}
	updateChannel("active", false)
	request := func(sender int64, text string) string {
		current.Helper()
		sequence++
		command := strings.Fields(text)[0]
		accept(map[string]any{"update_id": sequence, "message": map[string]any{"message_id": sequence, "date": time.Now().Unix(), "message_thread_id": 42, "from": map[string]any{"id": sender, "is_bot": false}, "chat": map[string]any{"id": int64(-1001122334455), "type": "supergroup"}, "text": text, "entities": []any{map[string]any{"type": "bot_command", "offset": 0, "length": len(command)}}}})
		var id string
		if err := f.owner.QueryRow(ctx, "SELECT id FROM telegram_inbox WHERE bot_id=$1 AND update_id=$2 AND kind='request'", bot.ID, sequence).Scan(&id); err != nil {
			current.Fatal(err)
		}
		return id
	}

	outbound := telegram.DeliveryBot{BotID: bot.ID, OrgID: f.org, Token: bot.ID + ":" + strings.Repeat("d", 35), PortalOrigin: "https://portal.example.test"}
	delivery := f.api.Telegram
	prepare := func() *telegram.PreparedDelivery {
		current.Helper()
		p, _, err := delivery.PrepareDelivery(ctx, outbound)
		if err != nil {
			current.Fatal(err)
		}
		return p
	}
	finish := func(p *telegram.PreparedDelivery, state, reason string, message *int64, delay time.Duration) {
		current.Helper()
		if p == nil {
			current.Fatal("missing prepared send")
		}
		if err := delivery.FinishDelivery(ctx, outbound, *p, telegram.DeliveryOutcome{State: state, Reason: reason, MessageID: message, RetryAfter: delay}); err != nil {
			current.Fatal(err)
		}
	}
	ready := func() {
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1", bot.ID)
	}
	details := func(id string) telegram.DeliveryDetails {
		current.Helper()
		rec, res := f.request("GET", "/v1/me/telegram/requests/"+id+"/deliveries", f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		return data[telegram.DeliveryDetails](current, res)
	}
	reconcile := func(id string, d telegram.Delivery, action string, message *int64) telegram.Delivery {
		current.Helper()
		body := telegram.DeliveryReconcile{ExpectedVersion: d.Version, Action: action, MessageID: message, Reason: "已核对指定原群消息，明确记录本人判断"}
		key := core.NewID()
		rec, res := f.request("POST", "/v1/me/telegram/requests/"+id+"/deliveries/"+d.ID+"/reconcile", f.aliceToken, key, body, nil)
		expect(current, rec, 200, res)
		value := data[telegram.Delivery](current, res)
		rec, res = f.request("POST", "/v1/me/telegram/requests/"+id+"/deliveries/"+d.ID+"/reconcile", f.aliceToken, key, body, nil)
		expect(current, rec, 200, res)
		if data[telegram.Delivery](current, res).Version != value.Version {
			current.Fatal("replayed reconciliation changed version")
		}
		return value
	}
	msg := int64(700)
	run("notifications default to disabled", func() {
		id := request(987654321, "/brief 未开启通知不能发出")
		if prepare() != nil || len(details(id).Deliveries) != 0 {
			current.Fatal("disabled channel prepared an outbound message")
		}
	})
	updateChannel("active", true)
	var accepted, unknown string
	run("prepared send is durable, private, deduplicated and paced", func() {
		accepted = request(987654321, "/brief PRIVATE_BODY 合同成本不得进入群通知")
		p := prepare()
		if p == nil || p.Delivery.Phase != "accepted" || p.Delivery.Attempts != 1 || p.Message.ChatID != channel.ChatID || p.Message.TopicID != 42 || p.Message.Reply.MessageID != sequence || p.Message.Reply.AllowMissing || !p.Message.Preview.Disabled || strings.Contains(p.Message.Text, "PRIVATE_BODY") || !strings.Contains(p.Message.Text, "/telegram?request="+accepted) {
			current.Fatal("prepared disclosure, location or identity changed")
		}
		if got := details(accepted); len(got.Events) != 2 || got.Deliveries[0].State != "sending" {
			current.Fatal("network was prepared without durable journal")
		}
		unknown = request(987654321, "/brief 下一任务仍受同一Bot限流")
		if prepare() != nil {
			current.Fatal("shared bot budget bypassed")
		}
		finish(p, "sent", "provider_confirmed", &msg, 0)
		finish(p, "delivery_unknown", "late_outcome", nil, 0)
		if d := details(accepted).Deliveries[0]; d.State != "sent" || d.MessageID == nil || *d.MessageID != msg {
			current.Fatal("stale completion changed a confirmed fact")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE telegram_delivery_events SET state='sent' WHERE delivery_id=$1", p.Delivery.ID); err == nil {
			current.Fatal("journal was writable")
		}
		if _, err := f.owner.Exec(ctx, "DELETE FROM telegram_deliveries WHERE id=$1", p.Delivery.ID); err == nil {
			current.Fatal("delivery audit fact deleted")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE telegram_deliveries SET version=version+1 WHERE id=$1", p.Delivery.ID); err == nil {
			current.Fatal("unrecorded or terminal transition accepted")
		}
		rec, res := f.request("GET", "/v1/me/telegram/requests/"+accepted, f.bobToken, "", nil, nil)
		expect(current, rec, 404, res)
		rec, res = f.request("GET", "/v1/me/telegram/requests/"+accepted+"/deliveries", f.bobToken, "", nil, nil)
		expect(current, rec, 404, res)
	})
	run("worker availability reports recent process presence", func() {
		rec, res := f.request("GET", "/v1/me/telegram", f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		if !data[telegram.Status](current, res).DeliveryAvailable {
			current.Fatal("live worker and enabled channel not reported")
		}
		exec("UPDATE telegram_delivery_budgets SET last_seen_at=clock_timestamp()-interval '91 seconds' WHERE bot_id=$1", bot.ID)
		rec, res = f.request("GET", "/v1/me/telegram", f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		if data[telegram.Status](current, res).DeliveryAvailable {
			current.Fatal("stale worker reported available")
		}
	})
	run("unknown never retries without current owner reconciliation", func() {
		ready()
		p := prepare()
		finish(p, "delivery_unknown", "provider_outcome_unknown", nil, 0)
		ready()
		if prepare() != nil {
			current.Fatal("uncertain send was retried automatically")
		}
		d := details(unknown).Deliveries[0]
		exec("UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		recMFA, resMFA := f.request("POST", "/v1/me/telegram/requests/"+unknown+"/deliveries/"+d.ID+"/reconcile", f.aliceToken, core.NewID(), telegram.DeliveryReconcile{ExpectedVersion: d.Version, Action: "abandon", Reason: "过期两步验证不得对账"}, nil)
		expect(current, recMFA, 403, resMFA)
		exec("UPDATE sessions SET mfa_verified_at=now() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		rec, res := f.request("POST", "/v1/me/telegram/requests/"+unknown+"/deliveries/"+d.ID+"/reconcile", f.bobToken, core.NewID(), telegram.DeliveryReconcile{ExpectedVersion: d.Version, Action: "abandon", Reason: "不能对账他人的请求"}, nil)
		expect(current, rec, 404, res)
		wrong := d
		wrong.Version++
		rec, res = f.request("POST", "/v1/me/telegram/requests/"+unknown+"/deliveries/"+d.ID+"/reconcile", f.aliceToken, core.NewID(), telegram.DeliveryReconcile{ExpectedVersion: wrong.Version, Action: "abandon", Reason: "验证版本栅栏"}, nil)
		expect(current, rec, 409, res)
		value := reconcile(unknown, d, "confirm_sent", &msg)
		if value.State != "confirmed_sent" {
			current.Fatal("manual claim mislabeled as Bot confirmation")
		}
	})
	run("explicitly attested retry retains old attempt and increments fence", func() {
		id := request(987654321, "/brief 手工核对后允许再次发送")
		ready()
		p := prepare()
		finish(p, "delivery_unknown", "provider_outcome_unknown", nil, 0)
		d := reconcile(id, details(id).Deliveries[0], "retry_confirmed_not_sent", nil)
		if d.State != "retry_wait" {
			current.Fatal("explicit retry not queued")
		}
		ready()
		next := prepare()
		if next == nil || next.Delivery.Attempts != 2 || next.Delivery.Version <= p.Delivery.Version {
			current.Fatal("retry did not create a new attempt fence")
		}
		finish(next, "sent", "provider_confirmed", &msg, 0)
		got := details(id)
		if len(got.Events) != 6 || got.Events[2].State != "delivery_unknown" || got.Events[3].ActorID == nil {
			current.Fatal("manual reconciliation erased uncertainty history")
		}
	})
	run("provider retry_after is a durable shared cooldown", func() {
		id := request(987654321, "/brief 服务端明确返回限流")
		ready()
		p := prepare()
		finish(p, "retry_wait", "provider_rate_limited", nil, time.Minute)
		var delayed bool
		if err := f.owner.QueryRow(ctx, "SELECT next_allowed_at>clock_timestamp()+interval '55 seconds' FROM telegram_delivery_budgets WHERE bot_id=$1", bot.ID).Scan(&delayed); err != nil || !delayed {
			current.Fatal("provider cooldown lost")
		}
		if prepare() != nil || details(id).Deliveries[0].State != "retry_wait" {
			current.Fatal("rate-limited delivery sent early")
		}
		updateChannel("active", true)
		ready()
		if prepare() != nil {
			current.Fatal("channel revision failed to suppress queued retry")
		}
	})
	run("retry cap still preserves a provider-wide cooldown", func() {
		id := request(987654321, "/brief 达到发送次数上限仍须遵守全局限流")
		for attempt := 1; attempt <= telegram.DeliveryMaxAttempts; attempt++ {
			ready()
			p := prepare()
			if p == nil || p.Delivery.Attempts != attempt {
				current.Fatal("attempt counter or retry fence changed")
			}
			if attempt < telegram.DeliveryMaxAttempts {
				finish(p, "delivery_unknown", "provider_outcome_unknown", nil, 0)
				reconcile(id, details(id).Deliveries[0], "retry_confirmed_not_sent", nil)
			} else {
				finish(p, "retry_wait", "provider_rate_limited", nil, time.Minute)
			}
		}
		d := details(id).Deliveries[0]
		if d.State != "failed" || d.ReasonCode != "retry_limit_reached" || d.Attempts != 5 {
			current.Fatal("retry limit was not enforced")
		}
		var delayed bool
		if err := f.owner.QueryRow(ctx, "SELECT next_allowed_at>clock_timestamp()+interval '55 seconds' FROM telegram_delivery_budgets WHERE bot_id=$1", bot.ID).Scan(&delayed); err != nil || !delayed {
			current.Fatal("retry limit bypassed the provider-wide cooldown")
		}
	})
	updateChannel("active", true)
	run("progress is coalesced into terminal result before network I/O", func() {
		profile := core.NewID()
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,telegram_configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','controlled-test',repeat('a',64),$3,repeat('b',64),repeat('d',64),'active',10000,1000,8,600,$4,'Isolated outbound lifecycle fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
		id := request(987654321, "/brief 核对真实秘书任务的阶段通知")
		if handled, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); err != nil || !handled {
			current.Fatal("dispatch", err)
		}
		rec, res := f.request("GET", "/v1/me/telegram/requests/"+id, f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		r := data[telegram.Request](current, res)
		if r.RunID == nil {
			current.Fatal("real run not dispatched")
		}
		ready()
		p := prepare()
		finish(p, "sent", "provider_confirmed", &msg, 0)
		j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "delivery.test", agent.ExecutionLeaseDuration)
		if err != nil || j == nil {
			current.Fatal("claim", err)
		}
		e, err := service.IssueExecution(ctx, *j)
		if err != nil {
			current.Fatal(err)
		}
		lease, err := service.LeaseForCredential(ctx, e.Credential)
		if err != nil {
			current.Fatal(err)
		}
		if err = service.RecordVerifiedConfinement(ctx, lease, e.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); err != nil {
			current.Fatal(err)
		}
		if err = service.StartExecution(ctx, lease); err != nil {
			current.Fatal(err)
		}
		if prepare() != nil {
			current.Fatal("progress bypassed pacing")
		}
		rec, res = f.request("GET", "/v1/runs/"+*r.RunID, f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		actual := data[agent.Run](current, res)
		rec, res = f.request("POST", "/v1/runs/"+actual.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": actual.Version, "action": "cancel", "reason": "结束受控投递生命周期验证"}, nil)
		expect(current, rec, 200, res)
		if err = service.FinishStoppedExecution(ctx, *j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
			current.Fatal(err)
		}
		ready()
		p = prepare()
		if p == nil || p.Delivery.Phase != "result" {
			current.Fatal("terminal result was not prepared")
		}
		finish(p, "sent", "provider_confirmed", &msg, 0)
		states := map[string]string{}
		for _, d := range details(id).Deliveries {
			states[d.Phase] = d.State
		}
		if states["accepted"] != "sent" || states["progress"] != "superseded" || states["result"] != "sent" {
			current.Fatal("phase coalescing rewrote prior delivered facts")
		}
	})
	run("abandoned prepared attempt becomes unknown after actual lease expiry", func() {
		id := request(987654321, "/brief 发送进程中断后不得猜测结果")
		ready()
		p := prepare()
		if p == nil {
			current.Fatal("missing crash fixture")
		}
		time.Sleep(telegram.DeliveryLease + 500*time.Millisecond)
		ready()
		if prepare() != nil {
			current.Fatal("expired send automatically repeated")
		}
		d := details(id).Deliveries[0]
		if d.State != "delivery_unknown" || d.ReasonCode != "sending_lease_expired" {
			current.Fatal("expired sending lease not reconciled as unknown")
		}
		finish(p, "sent", "late_provider_confirmed", &msg, 0)
		if details(id).Deliveries[0].State != "delivery_unknown" {
			current.Fatal("late process overwrote recovery fence")
		}
		reconcile(id, d, "abandon", nil)
	})
	run("network starts after durable preparation and releases database locks", func() {
		id := request(987654321, "/brief 测试发送中的配置变更边界")
		ready()
		sender := controlledTelegramDelivery(func(_ context.Context, _ telegram.DeliveryBot, m telegram.OutboundMessage) telegram.DeliveryOutcome {
			got := details(id)
			if len(got.Deliveries) != 1 || got.Deliveries[0].State != "sending" || len(got.Events) != 2 || !strings.Contains(m.Text, id) {
				current.Fatal("network started before durable preparation")
			}
			updateChannel("paused", false)
			return telegram.DeliveryOutcome{State: "sent", Reason: "provider_confirmed", MessageID: &msg}
		})
		if worked, err := delivery.DeliverOnce(ctx, outbound, sender); err != nil || !worked {
			current.Fatal("outbound orchestration failed", err)
		}
		if details(id).Deliveries[0].State != "sent" {
			current.Fatal("in-flight actual receipt was discarded after configuration change")
		}
		ready()
		if prepare() != nil {
			current.Fatal("paused channel produced another outbound attempt")
		}
		updateChannel("active", true)
	})
	run("multiple workers share one persisted bot budget", func() {
		request(987654321, "/brief 并发发送检查甲")
		request(987654321, "/brief 并发发送检查乙")
		ready()
		type outcome struct {
			p   *telegram.PreparedDelivery
			err error
		}
		results := make(chan outcome, 2)
		for i := 0; i < 2; i++ {
			go func() { p, _, err := delivery.PrepareDelivery(ctx, outbound); results <- outcome{p, err} }()
		}
		var prepared []*telegram.PreparedDelivery
		for i := 0; i < 2; i++ {
			r := <-results
			if r.err != nil {
				current.Fatal(r.err)
			}
			if r.p != nil {
				prepared = append(prepared, r.p)
			}
		}
		if len(prepared) != 1 {
			current.Fatal("parallel workers bypassed the global send budget")
		}
		finish(prepared[0], "sent", "provider_confirmed", &msg, 0)
		ready()
		p := prepare()
		if p == nil || p.Delivery.ID == prepared[0].Delivery.ID {
			current.Fatal("second request was lost or duplicated")
		}
		finish(p, "sent", "provider_confirmed", &msg, 0)
	})
	run("provider chat migration pauses the original channel without retargeting", func() {
		id := request(987654321, "/brief 群迁移必须人工复核")
		ready()
		p := prepare()
		finish(p, "failed", "chat_migration_requires_review", nil, 0)
		var state string
		var enabled bool
		if err := f.owner.QueryRow(ctx, "SELECT state,delivery_enabled FROM telegram_channels WHERE id=$1", channel.ID).Scan(&state, &enabled); err != nil || state != "paused" || enabled {
			current.Fatal("migrated channel did not pause")
		}
		if details(id).Deliveries[0].State != "failed" {
			current.Fatal("migration failure not retained")
		}
		updateChannel("active", true)
	})
	run("revoked backlog beyond one cleanup batch never blocks or sends stale requests", func() {
		for i := 0; i < 101; i++ {
			request(987654321, "/brief 验证超过单轮清理数量的撤销积压")
		}
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()+interval '1 hour' WHERE bot_id=$1", bot.ID)
		if prepare() != nil || prepare() != nil {
			current.Fatal("deferred backlog started a network attempt")
		}
		var pending int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_deliveries WHERE org_id=$1 AND state='pending'", f.org).Scan(&pending); err != nil || pending != 101 {
			current.Fatal("large backlog fixture was not actually queued")
		}
		updateChannel("active", true)
		ready()
		if prepare() != nil || prepare() != nil {
			current.Fatal("revoked backlog prepared a send")
		}
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_deliveries WHERE org_id=$1 AND state IN ('pending','retry_wait','sending')", f.org).Scan(&pending); err != nil || pending != 0 {
			current.Fatal("cleanup was rolled back or left a blocked backlog")
		}
	})
	run("binding revocation blocks manual retry and subsequent preparation", func() {
		id := request(987654321, "/brief 撤销绑定立即停止待发通知")
		ready()
		p := prepare()
		finish(p, "delivery_unknown", "provider_outcome_unknown", nil, 0)
		d := details(id).Deliveries[0]
		rec, res := f.request("POST", "/v1/me/telegram/bindings/"+binding.ID+"/revoke", f.aliceToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: binding.Version, Reason: "终止原数字身份的接收和通知"}, nil)
		expect(current, rec, 200, res)
		rec, res = f.request("POST", "/v1/me/telegram/requests/"+id+"/deliveries/"+d.ID+"/reconcile", f.aliceToken, core.NewID(), telegram.DeliveryReconcile{ExpectedVersion: d.Version, Action: "retry_confirmed_not_sent", Reason: "失效范围不得恢复通知"}, nil)
		expect(current, rec, 409, res)
		ready()
		if prepare() != nil {
			current.Fatal("revoked binding prepared another send")
		}
	})
	if os.Getenv("CP_BROWSER_TELEGRAM_DELIVERY_FIXTURE") == "1" {
		bind(f.aliceToken, 987654321)
		id := request(987654321, "/brief 股东群通知浏览器验收资料")
		ready()
		p := prepare()
		finish(p, "delivery_unknown", "provider_outcome_unknown", nil, 0)
		holdBrowserBusinessFixture(t, f, map[string]any{"telegram_delivery_fixture": true, "request_id": id, "delivery_id": p.Delivery.ID, "channel_id": channel.ID, "bot_id": bot.ID})
	}
}
