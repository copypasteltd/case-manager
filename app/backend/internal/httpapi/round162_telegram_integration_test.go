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
	projects "copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/telegram"
)

// All bot, profile and runtime identities are isolated fixtures. Only the real
// native database, HTTP authentication, task queue and controller are exercised.
func TestRound162TelegramProjectGrants(t *testing.T) {
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
	makeGrant := func(tool string, args map[string]any) agent.TelegramWriteGrant {
		current.Helper()
		rr, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &project.ID, Title: "Round162 fixed project instruction", MemberIDs: []string{}}, nil)
		expect(current, rr, 201, v)
		conversation := data[agent.Conversation](current, v)
		raw, _ := json.Marshal(args)
		input := agent.TelegramWriteGrantInput{ChannelID: channel.ID, ChannelVersion: channel.Version, BindingID: binding.ID, BindingVersion: binding.Version, Reason: "Only the exact reviewed project mutation", Run: agent.RunCreate{ExpectedVersion: conversation.Version, ContextSecurityVersion: conversation.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Perform only the stored exact project instruction", Mutation: &agent.MutationInput{Tool: tool, Arguments: raw}}}
		rr, v = f.request("POST", "/v1/conversations/"+conversation.ID+"/telegram-write-grants", f.aliceToken, core.NewID(), input, nil)
		expect(current, rr, 201, v)
		grant := data[agent.TelegramWriteGrant](current, v)
		if grant.Tool != tool || grant.ClientMessageID != input.Run.ClientMessageID || grant.RunVersion != 1 || grant.RunState != "queued" || grant.State != "pending_trigger" {
			current.Fatal("grant receipt differs from exact instruction", grant)
		}
		return grant
	}
	projectHead := func() projects.Project {
		rr, v := f.request("GET", "/v1/projects/"+project.ID, f.aliceToken, "", nil, nil)
		expect(current, rr, 200, v)
		return data[projects.Project](current, v)
	}
	createMilestone := func() projects.Milestone {
		rr, v := f.request("POST", "/v1/projects/"+project.ID+"/milestones", f.aliceToken, core.NewID(), map[string]any{"title": "Current milestone", "owner_id": f.alice, "due_at": time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)}, nil)
		expect(current, rr, 201, v)
		return data[projects.Milestone](current, v)
	}
	createDeliverable := func() projects.Deliverable {
		rr, v := f.request("POST", "/v1/projects/"+project.ID+"/deliverables", f.aliceToken, core.NewID(), projects.DeliverableCreate{Name: "Exact delivery", AcceptanceCriteria: "Independent actual acceptance", OwnerID: f.alice, Reason: "Fixture only"}, nil)
		expect(current, rr, 201, v)
		return data[projects.Deliverable](current, v)
	}
	createConcern := func(kind string) projects.Concern {
		rr, v := f.request("POST", "/v1/projects/"+project.ID+"/"+kind+"s", f.aliceToken, core.NewID(), map[string]any{"title": "Current concern", "description": "Synthetic only", "owner_id": f.alice, "severity": "medium"}, nil)
		expect(current, rr, 201, v)
		return data[projects.Concern](current, v)
	}
	build := func(tool string) map[string]any {
		current.Helper()
		head := projectHead()
		args := map[string]any{"id": head.ID, "expected_version": head.Version, "reason": "Explicit fixed mutation only"}
		switch tool {
		case "create_task":
			for k, v := range map[string]any{"title": "Fixed TG task", "body": "Human specified", "priority": "normal", "assignee_id": f.alice, "parent_id": nil, "milestone_id": nil, "start_at": nil, "due_at": nil, "review_required": true, "participant_ids": []string{f.bob}} {
				args[k] = v
			}
		case "update_task", "transition_task", "update_task_dependency":
			task := f.with(current).createTask(head, "Original task", false)
			args["id"], args["expected_version"] = task.ID, task.Version
			if tool == "update_task" {
				args["title"] = "Fixed reviewed title"
				args["participant_ids"] = []string{f.bob}
			} else if tool == "transition_task" {
				args["action"] = "start"
			} else {
				pre := f.with(current).createTask(head, "Fixed predecessor", false)
				args["predecessor_id"], args["expected_predecessor_version"], args["action"] = pre.ID, pre.Version, "add"
			}
		case "create_milestone":
			args["title"], args["owner_id"], args["due_at"] = "Fixed created milestone", f.alice, time.Now().Add(72*time.Hour).UTC().Truncate(time.Second)
		case "update_milestone", "transition_milestone":
			m := createMilestone()
			args["id"], args["expected_version"] = m.ID, m.Version
			if tool == "update_milestone" {
				args["title"] = "Fixed milestone title"
			} else {
				args["action"] = "cancel"
			}
		case "create_deliverable", "update_deliverable":
			if tool == "update_deliverable" {
				d := createDeliverable()
				args["id"], args["expected_version"] = d.ID, d.Version
			}
			args["name"], args["acceptance_criteria"], args["owner_id"], args["milestone_id"] = "Fixed TG delivery", "Independent reviewer still required", f.alice, nil
		case "submit_deliverable":
			d := createDeliverable()
			args["id"], args["expected_version"], args["expected_revision_id"] = d.ID, d.Version, d.CurrentRevisionID
			args["file_version_ids"], args["description"] = []string{original.Version.ID}, "Only this fixed formal file version"
		case "create_risk", "update_risk", "create_issue", "update_issue":
			kind := "issue"
			if strings.HasSuffix(tool, "risk") {
				kind = "risk"
			}
			if strings.HasPrefix(tool, "update_") {
				c := createConcern(kind)
				args["id"], args["expected_version"] = c.ID, c.Version
			}
			args["title"], args["description"], args["owner_id"], args["severity"], args["due_at"] = "Fixed TG concern", "Exact human facts", f.alice, "medium", nil
			if kind == "risk" {
				args["probability"], args["mitigation"] = "low", "Keep reviewed mitigation"
			} else {
				args["resolution"] = ""
			}
		case "update_project":
			args["name"] = "Fixed TG project name"
		case "transition_project":
			args["action"] = "activate"
		}
		return args
	}
	if os.Getenv("CP_BROWSER_ROUND162") == "1" {
		pending := makeGrant("create_task", build("create_task"))
		stopped := makeGrant("update_task", build("update_task"))
		rr, v := f.request("POST", "/v1/runs/"+stopped.RunID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: stopped.RunVersion, Action: "cancel", Reason: "Synthetic earlier cancelled grant"}, nil)
		expect(t, rr, 200, v)
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch162-functional-wave", "pending_grant_id": pending.ID, "pending_run_id": pending.RunID, "stopped_grant_id": stopped.ID, "conversation_id": pending.ConversationID, "project_id": project.ID})
		var jobs int
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_run_jobs WHERE org_id=$1 AND run_id=ANY($2::uuid[])", f.org, []string{pending.RunID, stopped.RunID}).Scan(&jobs); e != nil || jobs != 0 {
			t.Fatal("browser reads/cancellation enqueued a grant", jobs, e)
		}
		if e := f.owner.QueryRow(ctx, "SELECT state FROM agent_runs WHERE org_id=$1 AND id=$2", f.org, pending.RunID).Scan(&state); e != nil || state != "cancelled" {
			t.Fatal("browser ordinary cancellation not persisted", state, e)
		}
		return
	}
	tools := []string{"update_task", "create_task", "transition_task", "update_task_dependency", "create_milestone", "update_milestone", "transition_milestone", "create_deliverable", "update_deliverable", "submit_deliverable", "create_risk", "update_risk", "create_issue", "update_issue", "update_project", "transition_project"}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			current = t
			args := build(tool)
			grant := makeGrant(tool, args)
			var jobs int
			if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_run_jobs WHERE org_id=$1 AND run_id=$2", f.org, grant.RunID).Scan(&jobs); e != nil || jobs != 0 {
				t.Fatal("read/creation enqueued a grant", e, jobs)
			}
			rr, v := f.request("GET", "/v1/telegram-write-grants/"+grant.ID, f.aliceToken, "", nil, nil)
			expect(t, rr, 200, v)
			if data[agent.TelegramWriteGrant](t, v).State != "pending_trigger" {
				t.Fatal("fresh exact basis was not eligible")
			}
			request(987654321, grant.Trigger)
			if ok, e := service.DispatchTelegramOnce(ctx, f.org, bot.ID); e != nil || !ok {
				t.Fatal("fixed trigger dispatch", e)
			}
			worker, lease := round161Start(t, f, grant.RunID)
			raw, _ := json.Marshal(args)
			changed := map[string]any{}
			for k, v := range args {
				changed[k] = v
			}
			changed["reason"] = "model widened a fixed input"
			other, _ := json.Marshal(changed)
			if _, e := worker.ExecutionTool(ctx, lease, "widen", tool, other); e == nil {
				t.Fatal("changed complete intent accepted")
			}
			output, e := worker.ExecutionTool(ctx, lease, "exact", tool, raw)
			if e != nil {
				t.Fatal("exact TG business command", e)
			}
			replay, e := worker.ExecutionTool(ctx, lease, "exact", tool, raw)
			originalCanonical, firstError := core.CanonicalJSON(output)
			replayCanonical, replayError := core.CanonicalJSON(replay)
			if e != nil || firstError != nil || replayError != nil || !bytes.Equal(originalCanonical, replayCanonical) {
				t.Fatal("receipt replay changed", e)
			}
			var receipt struct {
				OperationID  string                  `json:"operation_id"`
				ResourceRefs []agent.ResultReference `json:"resource_refs"`
			}
			if json.Unmarshal(output, &receipt) != nil || receipt.OperationID == "" || len(receipt.ResourceRefs) == 0 {
				t.Fatal("missing immutable ordinary result")
			}
			if e = worker.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Exact fixed project mutation completed; acceptance is separate", ResourceRefs: receipt.ResourceRefs, SourceRefs: []agent.ResultReference{{Type: "agent_mutation", ID: grant.RunID}}, OperationIDs: []string{receipt.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
				t.Fatal(e)
			}
			rr, v = f.request("GET", "/v1/telegram-write-grants/"+grant.ID, f.aliceToken, "", nil, nil)
			expect(t, rr, 200, v)
			got := data[agent.TelegramWriteGrant](t, v)
			if got.State != "consumed" || got.RunState != "succeeded" {
				t.Fatal("consumption confused with run outcome", got.State, got.RunState)
			}
			var actual string
			if e = f.owner.QueryRow(ctx, "SELECT channel FROM agent_runs WHERE org_id=$1 AND id=$2", f.org, grant.RunID).Scan(&actual); e != nil || actual != "telegram" {
				t.Fatal("business channel rewritten", e)
			}
		})
	}
	current = t
	t.Run("owner_inventory_current_basis_and_ordinary_cancellation", func(t *testing.T) {
		current = t
		grant := makeGrant("update_task", build("update_task"))
		var all []agent.TelegramWriteGrant
		cursor := ""
		seen := map[string]bool{}
		for page := 0; page < 30; page++ {
			path := "/v1/me/telegram/write-grants?limit=2"
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			rr, v := f.request("GET", path, f.aliceToken, "", nil, nil)
			expect(t, rr, 200, v)
			p := data[core.Page[agent.TelegramWriteGrant]](t, v)
			for _, g := range p.Items {
				if seen[g.ID] {
					t.Fatal("duplicate inventory item")
				}
				seen[g.ID] = true
				all = append(all, g)
			}
			if p.NextCursor == nil {
				break
			}
			cursor = *p.NextCursor
		}
		if len(all) != 17 {
			t.Fatal("complete owner inventory", len(all))
		}
		rr, v := f.request("GET", "/v1/me/telegram/write-grants?limit=20", f.bobToken, "", nil, nil)
		expect(t, rr, 200, v)
		if len(data[core.Page[agent.TelegramWriteGrant]](t, v).Items) != 0 {
			t.Fatal("another owner metadata disclosed")
		}
		rr, v = f.request("GET", "/v1/me/telegram/write-grants?state=consumed&limit=2&cursor="+cursor, f.aliceToken, "", nil, nil)
		if rr.Code < 400 {
			t.Fatal("cursor reused across filters")
		}
		// A changed exact task basis invalidates the pending trigger, without changing grant evidence.
		var taskID string
		var version int64
		if e := f.owner.QueryRow(ctx, "SELECT resource_id,base_version FROM agent_run_mutations WHERE org_id=$1 AND run_id=$2", f.org, grant.RunID).Scan(&taskID, &version); e != nil {
			t.Fatal(e)
		}
		rr, v = f.request("PATCH", "/v1/tasks/"+taskID, f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "title": "Human changed exact basis"}, nil)
		expect(t, rr, 200, v)
		rr, v = f.request("GET", "/v1/telegram-write-grants/"+grant.ID, f.aliceToken, "", nil, nil)
		expect(t, rr, 200, v)
		grant = data[agent.TelegramWriteGrant](t, v)
		if grant.State != "invalidated" {
			t.Fatal("stale basis still triggerable", grant.State)
		}
		rr, v = f.request("POST", "/v1/runs/"+grant.RunID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: grant.RunVersion, Action: "cancel", Reason: "Revoke obsolete fixed grant"}, nil)
		expect(t, rr, 200, v)
		rr, v = f.request("GET", "/v1/telegram-write-grants/"+grant.ID, f.aliceToken, "", nil, nil)
		expect(t, rr, 200, v)
		if data[agent.TelegramWriteGrant](t, v).State != "stopped" {
			t.Fatal("ordinary cancellation missing")
		}
		request(987654321, grant.Trigger)
		_, _ = service.DispatchTelegramOnce(ctx, f.org, bot.ID)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_write_activations WHERE org_id=$1 AND grant_id=$2", f.org, grant.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal("stopped grant activated", e)
		}
	})
}
