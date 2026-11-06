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
	projects "copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/telegram"
)

// All bot, profile and runtime identities are isolated fixtures. Only the real
// native database, HTTP authentication, task queue and controller are exercised.
func TestRound161TelegramWriteGrant(t *testing.T) {
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
	binding := bind(f.aliceToken, 987654321)
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

	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,telegram_write_configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-update','round161-fixture',repeat('a',64),$3,repeat('b',64),repeat('d',64),'active',12000,2000,20,600,$4,'isolated exact TG grant')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &project.ID, Title: "Private fixed TG execution", MemberIDs: []string{}}, nil)
	expect(t, rec, 201, res)
	c := data[agent.Conversation](t, res)
	task := f.createTask(project, "Original exact task", false)
	arguments, _ := json.Marshal(map[string]any{"id": task.ID, "expected_version": task.Version, "title": "Fixed TG task title", "reason": "Reviewed exact field"})
	input := agent.TelegramWriteGrantInput{ChannelID: channel.ID, ChannelVersion: channel.Version, BindingID: binding.ID, BindingVersion: binding.Version, Reason: "Authorize only this fixed task mutation", Run: agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Only the fixed reviewed task title", Mutation: &agent.MutationInput{Tool: "update_task", Arguments: arguments}}}
	bad := input
	bad.BindingVersion++
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/telegram-write-grants", f.aliceToken, core.NewID(), bad, nil)
	expect(t, rec, 409, res)
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/telegram-write-grants", f.aliceToken, core.NewID(), input, nil)
	expect(t, rec, 201, res)
	grant := data[agent.TelegramWriteGrant](t, res)
	var queued int
	if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_run_jobs WHERE org_id=$1 AND run_id=$2", f.org, grant.RunID).Scan(&queued); e != nil || queued != 0 {
		t.Fatal("grant started without TG trigger", queued, e)
	}
	service := &agent.Service{DB: f.api.DB}
	triggerInbox := request(987654321, grant.Trigger)
	if worked, e := service.DispatchTelegramOnce(ctx, f.org, bot.ID); e != nil || !worked {
		t.Fatal("exact trigger not dispatched", e)
	}
	var triggerState, triggerReason string
	if e := f.owner.QueryRow(ctx, "SELECT state,reason_code FROM telegram_inbox WHERE org_id=$1 AND id=$2", f.org, triggerInbox).Scan(&triggerState, &triggerReason); e != nil || triggerState != "dispatched" {
		t.Fatal("fixed trigger rejected", triggerState, triggerReason, e)
	}
	worker, lease := round161Start(t, f, grant.RunID)
	var actual string
	if e := f.owner.QueryRow(ctx, "SELECT channel FROM agent_runs WHERE org_id=$1 AND id=$2", f.org, grant.RunID).Scan(&actual); e != nil || actual != "telegram" {
		t.Fatal("TG channel was rewritten", actual, e)
	}
	output, e := worker.ExecutionTool(ctx, lease, "fixed-update", "update_task", arguments)
	if e != nil {
		t.Fatal(e)
	}
	var receipt struct {
		OperationID string `json:"operation_id"`
	}
	if json.Unmarshal(output, &receipt) != nil || receipt.OperationID == "" {
		t.Fatal("no ordinary update receipt")
	}
	if _, e = worker.ExecutionTool(ctx, lease, "forbidden", "transition_task", arguments); e == nil {
		t.Fatal("TG grant widened tool scope")
	}
	if e = worker.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Fixed title updated in private result", ResourceRefs: []agent.ResultReference{{Type: "task", ID: task.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_mutation", ID: grant.RunID}}, OperationIDs: []string{receipt.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
		t.Fatal(e)
	}
	request(987654321, grant.Trigger)
	_, _ = service.DispatchTelegramOnce(ctx, f.org, bot.ID)
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_write_activations WHERE org_id=$1 AND grant_id=$2", f.org, grant.ID).Scan(&queued); e != nil || queued != 1 {
		t.Fatal("one-use activation changed", queued, e)
	}
	rec, res = f.request("GET", "/v1/me/telegram/requests?limit=20", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	rec, res = f.request("GET", "/v1/tasks/"+task.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	changed := data[projects.Task](t, res)
	if changed.Title != "Fixed TG task title" || changed.Version != task.Version+1 {
		t.Fatal(changed)
	}
	if _, e = f.api.DB.Exec(ctx, "DELETE FROM telegram_write_grants WHERE org_id=$1", f.org); e == nil {
		t.Fatal("immutable grant deleted")
	}
}
