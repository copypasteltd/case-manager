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
func TestRound166TelegramGrantDelivery(t *testing.T) {
	upload, original, _, _, _ := restorationFixture(t, 25)
	f := upload.f
	round161Diagnostics(t, f)
	ctx := context.Background()
	current := t
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			current.Fatal(err)
		}
	}
	project := func() projects.Project {
		rr, v := f.request("GET", "/v1/projects/"+*original.File.ProjectID, f.aliceToken, "", nil, nil)
		expect(t, rr, 200, v)
		return data[projects.Project](t, v)
	}()
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
	rr, memberResult := f.request("POST", "/v1/projects/"+project.ID+"/members", f.aliceToken, core.NewID(), projects.MemberChange{ExpectedVersion: project.Version, UserID: f.admin, Role: "member", Action: "set", Reason: "Fixture administrator needs actual project membership"}, nil)
	expect(t, rr, 200, memberResult)
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

	service := &agent.Service{DB: f.api.DB}
	var grantArguments json.RawMessage
	makeGrant := func(tool string, args map[string]any) agent.TelegramWriteGrant {
		current.Helper()
		rr, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &project.ID, Title: "Round166 fixed project instruction", MemberIDs: []string{}}, nil)
		expect(current, rr, 201, v)
		conversation := data[agent.Conversation](current, v)
		raw, _ := json.Marshal(args)
		grantArguments = append(json.RawMessage(nil), raw...)
		input := agent.TelegramWriteGrantInput{ChannelID: channel.ID, ChannelVersion: channel.Version, BindingID: binding.ID, BindingVersion: binding.Version, Reason: "Only the exact reviewed project mutation", Run: agent.RunCreate{ExpectedVersion: conversation.Version, ContextSecurityVersion: conversation.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Perform only the stored exact project instruction", Mutation: &agent.MutationInput{Tool: tool, Arguments: raw}}}
		rr, v = f.request("POST", "/v1/conversations/"+conversation.ID+"/telegram-write-grants", f.aliceToken, core.NewID(), input, nil)
		expect(current, rr, 201, v)
		grant := data[agent.TelegramWriteGrant](current, v)
		if grant.Tool != tool || grant.ClientMessageID != input.Run.ClientMessageID || grant.RunVersion != 1 || grant.RunState != "queued" || grant.State != "pending_trigger" {
			current.Fatal("grant receipt differs from exact instruction", grant)
		}
		return grant
	}

	updateChannel("active", true)
	outbound := telegram.DeliveryBot{BotID: bot.ID, OrgID: f.org, Token: bot.ID + ":" + strings.Repeat("d", 35), PortalOrigin: "https://portal.example.test"}
	prepare := func() *telegram.PreparedDelivery {
		p, _, e := f.api.Telegram.PrepareDelivery(ctx, outbound)
		if e != nil {
			current.Fatal(e)
		}
		return p
	}
	ready := func() {
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()-interval '1 second' WHERE bot_id=$1", bot.ID)
	}
	finish := func(p *telegram.PreparedDelivery) {
		message := int64(700)
		if e := f.api.Telegram.FinishDelivery(ctx, outbound, *p, telegram.DeliveryOutcome{State: "sent", Reason: "provider_confirmed", MessageID: &message}); e != nil {
			current.Fatal(e)
		}
	}
	newGrant := func() agent.TelegramWriteGrant {
		rr, v := f.request("GET", "/v1/projects/"+project.ID, f.aliceToken, "", nil, nil)
		expect(current, rr, 200, v)
		head := data[projects.Project](current, v)
		return makeGrant("update_project", map[string]any{"id": head.ID, "expected_version": head.Version, "name": "Synthetic reviewed project name", "reason": "Exact human intent"})
	}
	t.Run("accepted_progress_result_use_exact_effective_origin_without_prose", func(t *testing.T) {
		current = t
		g := newGrant()
		id := request(987654321, g.Trigger)
		if ok, e := service.DispatchTelegramOnce(ctx, f.org, bot.ID); e != nil || !ok {
			t.Fatal("grant activation", e)
		}
		check := func(phase string) *telegram.PreparedDelivery {
			ready()
			p := prepare()
			if p == nil || p.Delivery.Phase != phase || p.Delivery.RunID == nil || *p.Delivery.RunID != g.RunID || p.Delivery.InboxID != id || strings.Contains(p.Message.Text, "Synthetic reviewed") || strings.Contains(p.Message.Text, "Perform only") {
				t.Fatal("exact generic notice origin/privacy", phase, p)
			}
			finish(p)
			return p
		}
		first := check("accepted")
		worker, lease := round161Start(t, f, g.RunID)
		check("progress")
		args := grantArguments
		output, e := worker.ExecutionTool(ctx, lease, "exact", "update_project", args)
		if e != nil {
			t.Fatal(e)
		}
		var receipt struct {
			OperationID  string                  `json:"operation_id"`
			ResourceRefs []agent.ResultReference `json:"resource_refs"`
		}
		if json.Unmarshal(output, &receipt) != nil || receipt.OperationID == "" {
			t.Fatal("ordinary receipt absent")
		}
		if e = worker.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Private project completion", ResourceRefs: receipt.ResourceRefs, SourceRefs: []agent.ResultReference{{Type: "agent_mutation", ID: g.RunID}}, OperationIDs: []string{receipt.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			t.Fatal(e)
		}
		check("result")
		if _, e = f.owner.Exec(ctx, "UPDATE telegram_deliveries SET run_id=NULL WHERE id=$1", first.Delivery.ID); e == nil {
			t.Fatal("immutable origin changed")
		}
	})
	t.Run("accepted_before_activation_is_suppressed_after_organization_change", func(t *testing.T) {
		current = t
		g := newGrant()
		id := request(987654321, g.Trigger)
		exec("UPDATE telegram_delivery_budgets SET next_allowed_at=clock_timestamp()+interval '1 hour' WHERE bot_id=$1", bot.ID)
		if prepare() != nil {
			t.Fatal("shared budget bypassed")
		}
		var pending string
		var runID *string
		if e := f.owner.QueryRow(ctx, "SELECT id,run_id FROM telegram_deliveries WHERE inbox_id=$1 AND phase='accepted'", id).Scan(&pending, &runID); e != nil || runID != nil {
			t.Fatal("preactivation pending source", e)
		}
		if ok, e := service.DispatchTelegramOnce(ctx, f.org, bot.ID); e != nil || !ok {
			t.Fatal(e)
		}
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		var authorized bool
		if e := f.owner.QueryRow(ctx, "SELECT telegram_delivery_authorized($1,$2)", f.org, id).Scan(&authorized); e != nil || authorized {
			t.Fatal("stale run organization authorized", e)
		}
		ready()
		if prepare() != nil {
			t.Fatal("stale grant disclosed")
		}
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT state FROM telegram_deliveries WHERE id=$1", pending).Scan(&state); e != nil || state != "suppressed" {
			t.Fatal("stale pending not suppressed", state, e)
		}
	})
}
