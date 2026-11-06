package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/telegram"
)

// All bot, profile and runtime identities are isolated fixtures. Only the real
// native database, HTTP authentication, task queue and controller are exercised.
func TestRound161AuthorizedNotification(t *testing.T) {
	f := setup(t)
	round161Diagnostics(t, f)
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
	bind(f.aliceToken, 987654321)
	bind(f.bobToken, 987654322)
	defaultSkill := "project-briefing"
	rec, res := f.request("POST", "/v1/telegram/channels", admin, core.NewID(), telegram.ChannelCreate{BotID: bot.ID, ChatID: "-1001122334455", TopicID: 42, Label: "股东群项目话题", ProjectID: &project.ID, AllowedSkillIDs: []string{"project-briefing", "incident-summary", "project-update"}, DefaultSkillID: &defaultSkill, Reason: "核对指定项目和任务"}, nil)
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

	run("human handoff, exact write and reviewed group result", func() {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='telegram_handoff_discloser'", core.NewID(), f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		updateChannel("active", true)
		profile := core.NewID()
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
   VALUES($1,$2,'project-update','controlled-round161-notice',repeat('a',64),$3,repeat('b',64),'active',12000,3000,20,600,$4,'isolated human handoff fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
		requestID := request(987654321, "/task project-update 请更新我已明确指定的项目标题")
		service := &agent.Service{DB: f.api.DB}
		worked, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID)
		if err != nil || worked {
			current.Fatal("write intake was automatically dispatched", err)
		}
		rr, rs := f.request("GET", "/v1/me/telegram/requests/"+requestID+"/handoff", f.bobToken, "", nil, nil)
		expect(current, rr, 404, rs)
		rr, rs = f.request("GET", "/v1/me/telegram/requests/"+requestID+"/handoff", f.aliceToken, "", nil, nil)
		expect(current, rr, 200, rs)
		origin := data[agent.TelegramHandoffRequest](current, rs)
		if !origin.Eligible || origin.SkillID != "project-update" {
			current.Fatal("exact pending human request unavailable")
		}
		rr, rs = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &project.ID, Title: "人工授权TG请求", MemberIDs: []string{}}, nil)
		expect(current, rr, 201, rs)
		c := data[agent.Conversation](current, rs)
		arguments, _ := json.Marshal(map[string]any{"id": project.ID, "expected_version": project.Version, "name": "人工固定授权后的项目标题", "reason": "只处理本次准确范围"})
		handoff := &agent.TelegramHandoffInput{RequestID: requestID, ExpectedVersion: origin.Version, RequestSHA256: origin.RequestSHA256, Reviewed: true}
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: origin.Text, TelegramHandoff: handoff, Mutation: &agent.MutationInput{Tool: "update_project", Arguments: arguments}}
		rr, rs = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(current, rr, 202, rs)
		task := data[agent.Run](current, rs)
		if task.TelegramHandoff == nil || task.TelegramHandoff.RequestID != requestID {
			current.Fatal("handoff receipt missing")
		}
		job, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "round161-notice.handoff", agent.ExecutionLeaseDuration)
		if err != nil || job == nil {
			current.Fatal(err)
		}
		execution, err := service.IssueExecution(ctx, *job)
		if err != nil {
			current.Fatal(err)
		}
		lease, err := service.LeaseForCredential(ctx, execution.Credential)
		if err != nil {
			current.Fatal(err)
		}
		lease.Job = *job
		if err = service.RecordVerifiedConfinement(ctx, lease, execution.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); err != nil {
			current.Fatal(err)
		}
		if err = service.StartExecution(ctx, lease); err != nil {
			current.Fatal(err)
		}
		output, err := service.ExecutionTool(ctx, lease, core.NewID(), "update_project", arguments)
		if err != nil {
			current.Fatal(err)
		}
		var write struct {
			OperationID string `json:"operation_id"`
		}
		if json.Unmarshal(output, &write) != nil || write.OperationID == "" {
			current.Fatal("ordinary exact write receipt missing")
		}
		result := agent.ProposedResult{Status: "completed", Summary: "已按本人固定授权更新项目标题。", ResourceRefs: []agent.ResultReference{{Type: "project", ID: project.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_mutation", ID: task.ID}}, OperationIDs: []string{write.OperationID}, MissingFields: []string{}}
		if err = service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{}); err != nil {
			current.Fatal(err)
		}
		rr, rs = f.request("GET", "/v1/me/telegram/requests/"+requestID+"/outbound-preview", f.aliceToken, "", nil, nil)
		expect(current, rr, 200, rs)
		preview := data[agent.TelegramDisclosurePreview](current, rs)
		infoProfile := core.NewID()
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'information-organize','round161-fixture',repeat('a',64),$3,repeat('b',64),'active',20000,3000,20,600,$4,'isolated fixed notification')`, infoProfile, f.org, agentworker.RuntimeVersion, f.admin)
		notice := &agent.NotificationInput{RequestID: requestID, TelegramDisclosureInput: agent.TelegramDisclosureInput{ExpectedVersion: preview.RunVersion, BodySHA256: preview.BodySHA256, StartChar: 0, MaxChars: preview.TotalChars, Reviewed: true, DisclosedToGroup: true}}
		fixed := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: infoProfile, ClientMessageID: core.NewID(), Message: "Queue only the reviewed exact original-group result", Notification: notice}
		bad := fixed
		wrong := *notice
		wrong.BodySHA256 = strings.Repeat("f", 64)
		bad.Notification = &wrong
		rr, rs = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
		expect(current, rr, 409, rs)
		rr, rs = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), fixed, nil)
		expect(current, rr, 202, rs)
		followup := data[agent.Run](current, rs)
		if followup.FixedDelivery == nil || followup.FixedDelivery.Kind != "notification" {
			current.Fatal("exact notification receipt missing")
		}
		worker, l := round161Start(current, f, followup.ID)
		args, _ := json.Marshal(map[string]any{"run_id": followup.ID, "intent_sha256": followup.FixedDelivery.IntentSHA256})
		if _, e := worker.ExecutionTool(ctx, l, "review-fixed", "propose_notification", args); e != nil {
			current.Fatal(e)
		}
		out, e := worker.ExecutionTool(ctx, l, "queue-fixed", "send_authorized_notification", args)
		if e != nil {
			current.Fatal(e)
		}
		var received struct {
			OperationID string                          `json:"operation_id"`
			Content     agent.TelegramDisclosureReceipt `json:"content"`
		}
		if json.Unmarshal(out, &received) != nil || received.OperationID == "" || received.Content.DeliveryID == "" {
			current.Fatal("ordinary queue receipt missing")
		}
		var state, phase string
		var sent *int64
		if e = f.owner.QueryRow(ctx, "SELECT state,phase,message_id FROM telegram_deliveries WHERE org_id=$1 AND id=$2", f.org, received.Content.DeliveryID).Scan(&state, &phase, &sent); e != nil || state != "pending" || phase != "reviewed_result" || sent != nil {
			current.Fatal("queue was mislabeled delivered", state, phase, e)
		}
		replay, e := worker.ExecutionTool(ctx, l, "queue-fixed", "send_authorized_notification", args)
		canonicalOut, outErr := core.CanonicalJSON(out)
		canonicalReplay, replayErr := core.CanonicalJSON(replay)
		if e != nil || outErr != nil || replayErr != nil || !bytes.Equal(canonicalOut, canonicalReplay) {
			current.Fatal("fixed queue replay changed", e)
		}
		var n int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_handoff_reviews WHERE org_id=$1 AND id=$2", f.org, received.Content.ID).Scan(&n); e != nil || n != 1 {
			current.Fatal(n, e)
		}
		if e = worker.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "Fixed fragment queued; delivery remains pending", SourceRefs: []agent.ResultReference{{Type: "agent_fixed_delivery", ID: followup.ID}}, ResourceRefs: []agent.ResultReference{{Type: "telegram_delivery", ID: received.Content.DeliveryID}}, OperationIDs: []string{received.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			current.Fatal(e)
		}
		if _, e = f.api.DB.Exec(ctx, "DELETE FROM agent_fixed_delivery_commits WHERE org_id=$1", f.org); e == nil {
			current.Fatal("notification receipt mutable")
		}
	})
}
