package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/telegram"
)

// All bot, profile and runtime identities are isolated fixtures. Only the real
// native database, HTTP authentication, task queue and controller are exercised.
func TestTelegramDispatch(t *testing.T) {
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
	updateChannel := func(state string) {
		current.Helper()
		rec, res := f.request("GET", "/v1/telegram/channels", admin, "", nil, nil)
		expect(current, rec, 200, res)
		for _, c := range data[core.Page[telegram.Channel]](current, res).Items {
			if c.ID == channel.ID {
				channel = c
			}
		}
		rec, res = f.request("PATCH", "/v1/telegram/channels/"+channel.ID, admin, core.NewID(), telegram.ChannelPatch{ExpectedVersion: channel.Version, Label: channel.Label, ProjectID: channel.ProjectID, AllowedSkillIDs: channel.AllowedSkillIDs, DefaultSkillID: channel.DefaultSkillID, State: state, Reason: "明确核对当前入口版本"}, nil)
		expect(current, rec, 200, res)
		channel = data[telegram.Channel](current, res)
	}
	updateChannel("active")
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
	requests := func(token string) []telegram.Request {
		current.Helper()
		rec, res := f.request("GET", "/v1/me/telegram/requests?limit=100", token, "", nil, nil)
		expect(current, rec, 200, res)
		return data[core.Page[telegram.Request]](current, res).Items
	}
	lookup := func(id string) telegram.Request {
		current.Helper()
		for _, r := range requests(f.aliceToken) {
			if r.ID == id {
				return r
			}
		}
		for _, r := range requests(f.bobToken) {
			if r.ID == id {
				return r
			}
		}
		current.Fatal("request missing")
		return telegram.Request{}
	}
	dispatch := func() {
		current.Helper()
		if handled, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); err != nil || !handled {
			current.Fatal("actual dispatch failed", err)
		}
	}
	install := func(skill string, tg ...bool) string {
		current.Helper()
		id := core.NewID()
		var digest *string
		if (skill == "project-briefing" || skill == "incident-summary") && (len(tg) == 0 || tg[0]) {
			value := strings.Repeat("d", 64)
			digest = &value
		}
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,telegram_configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,$3,'controlled-test',repeat('a',64),$4,repeat('b',64),$6,'active',10000,1000,8,600,$5,'Isolated TG controller fixture')`, id, f.org, skill, agentworker.RuntimeVersion, f.admin, digest)
		return id
	}
	claim := func(runID string) (jobqueue.Job, agent.Execution, agent.ExecutionLease) {
		current.Helper()
		for i := 0; i < 30; i++ {
			j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "telegram.test", agent.ExecutionLeaseDuration)
			if err != nil || j == nil {
				current.Fatal("claim", err)
			}
			var p struct {
				RunID string `json:"run_id"`
			}
			json.Unmarshal(j.Payload, &p)
			if p.RunID != runID {
				if err = service.FinishStoppedExecution(ctx, *j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
					current.Fatal(err)
				}
				continue
			}
			execution, err := service.IssueExecution(ctx, *j)
			if err != nil {
				current.Fatal(err)
			}
			lease, err := service.LeaseForCredential(ctx, execution.Credential)
			if err != nil {
				current.Fatal(err)
			}
			return *j, execution, lease
		}
		current.Fatal("desired job not found")
		return jobqueue.Job{}, agent.Execution{}, agent.ExecutionLease{}
	}
	start := func(e agent.Execution, l agent.ExecutionLease) {
		current.Helper()
		if err := service.RecordVerifiedConfinement(ctx, l, e.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); err != nil {
			current.Fatal(err)
		}
		if err := service.StartExecution(ctx, l); err != nil {
			current.Fatal(err)
		}
	}
	cancel := func(r telegram.Request) {
		current.Helper()
		rec, res := f.request("GET", "/v1/runs/"+*r.RunID, f.aliceToken, "", nil, nil)
		if rec.Code == 404 {
			rec, res = f.request("GET", "/v1/runs/"+*r.RunID, f.bobToken, "", nil, nil)
		}
		expect(current, rec, 200, res)
		state := data[agent.Run](current, res)
		token := f.aliceToken
		if state.ActorID == f.bob {
			token = f.bobToken
		}
		rec, res = f.request("POST", "/v1/runs/"+state.ID+"/cancel", token, core.NewID(), map[string]any{"expected_version": state.Version, "action": "cancel", "reason": "结束受控渠道验证"}, nil)
		expect(current, rec, 200, res)
	}
	var profile string
	var completed telegram.Request

	run("missing runtime profile retains the real request without orphan contexts or jobs", func() {
		id := request(987654321, "/brief 请根据指定项目作简报")
		dispatch()
		r := lookup(id)
		if r.State != "awaiting_executor" || r.ReasonCode != "RUNTIME_UNAVAILABLE" || r.RunID != nil {
			current.Fatal("missing profile was not deferred")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_conversation_scopes").Scan(&count); err != nil || count != 0 {
			current.Fatal("failed admission leaked a context")
		}
	})
	updateChannel("active")
	run("portal runtime configuration cannot authorize Telegram execution", func() {
		id := install("project-briefing", false)
		receipt := request(987654321, "/brief 没有独立TG配置时不能运行")
		dispatch()
		r := lookup(receipt)
		if r.RunID != nil || r.State != "awaiting_executor" || r.ReasonCode != "RUNTIME_UNAVAILABLE" {
			current.Fatal("portal profile authorized Telegram")
		}
		exec("UPDATE agent_runtime_profiles SET state='retired' WHERE id=$1", id)
	})
	updateChannel("active")
	profile = install("project-briefing")
	install("incident-summary")
	run("incident request dispatches through a restricted Telegram execution", func() {
		rec, res := f.request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{Content: status.Content{Name: "TG status fixture", OwnerID: f.alice}, ProjectID: &project.ID, Classification: "project_restricted", Reason: "Controlled real TG service read"}, nil)
		expect(current, rec, 201, res)
		monitored := data[status.Record](current, res)
		id := request(987654321, "/incident 请核对服务故障")
		dispatch()
		r := lookup(id)
		if r.State != "dispatched" || r.RunID == nil {
			current.Fatal("incident request was not dispatched", r.State, r.ReasonCode)
		}
		_, e, l := claim(*r.RunID)
		start(e, l)
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "search_resources", json.RawMessage(`{"query":"不存在的监控服务"}`)); err != nil {
			current.Fatal(err)
		}
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "draft_post", json.RawMessage(`{}`)); err == nil {
			current.Fatal("incident execution accepted write")
		}
		args, _ := json.Marshal(map[string]any{"service_id": monitored.ID})
		out, err := service.ExecutionTool(ctx, l, core.NewID(), "read_service_summary", args)
		if err != nil {
			current.Fatal(err)
		}
		var result struct {
			Sources []agent.ResultReference `json:"source_refs"`
		}
		if json.Unmarshal(out, &result) != nil || len(result.Sources) != 1 {
			current.Fatal("TG missed immutable service evidence")
		}
		if err := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "No collector has been configured; captured service state is unknown.", SourceRefs: result.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{InputTokens: 50, OutputTokens: 20}); err != nil {
			current.Fatal(err)
		}
	})
	updateChannel("active")
	run("concurrent dispatch creates one exact channel run and one durable job", func() {
		id := request(987654321, "/brief 读取明确项目并核对来源")
		var wg sync.WaitGroup
		failures := make(chan error, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID); failures <- err }()
		}
		wg.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				current.Fatal(err)
			}
		}
		completed = lookup(id)
		if completed.State != "dispatched" || completed.RunID == nil || completed.ConversationID == nil || completed.RunState == nil || *completed.RunState != "queued" {
			current.Fatal("not queued with actual references")
		}
		var count int
		f.owner.QueryRow(ctx, "SELECT count(*) FROM telegram_run_links WHERE inbox_id=$1", id).Scan(&count)
		if count != 1 {
			current.Fatal("duplicate run mapping")
		}
		f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_run_jobs WHERE run_id=$1", completed.RunID).Scan(&count)
		if count != 1 {
			current.Fatal("missing or duplicate actual job")
		}
	})
	run("busy conversation retains the next request without inserting another task", func() {
		id := request(987654321, "/brief 前一任务完成后再处理")
		dispatch()
		r := lookup(id)
		if r.RunID != nil || r.State != "awaiting_executor" || r.ReasonCode != "CONVERSATION_RUN_ACTIVE" {
			current.Fatal("busy context created competing task")
		}
	})
	run("controller executes a read under Telegram identity and stores a real portal result", func() {
		_, e, l := claim(*completed.RunID)
		if !strings.Contains(e.PreparedContext, `"channel":"telegram"`) || !strings.Contains(e.PreparedContext, `"delivery_mode":"portal_only"`) {
			current.Fatal("channel context not pinned")
		}
		if e.Channel != "telegram" || e.Profile.ConfigurationSHA256 != strings.Repeat("d", 64) {
			current.Fatal("wrong SDK scope configuration")
		}
		bridge := &agentworker.Bridge{Service: service, Verifier: &controlledNativeVerifier{}, Address: "127.0.0.1:19992", Model: "controlled-model", WorkingDirectory: current.TempDir()}
		invoke := func(path string, payload any) *httptest.ResponseRecorder {
			raw, _ := json.Marshal(payload)
			request := httptest.NewRequest("POST", "http://"+bridge.Address+path, bytes.NewReader(raw))
			request.RemoteAddr = "127.0.0.1:12345"
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+e.Credential)
			reply := httptest.NewRecorder()
			bridge.ServeHTTP(reply, request)
			return reply
		}
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: e.Profile.ID, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: e.Profile.ConfigurationSHA256, ExpectedTools: []string{"company/read_resource", "company/search_resources", "company/inspect_operation"}}
		wrong := native
		wrong.ExpectedTools = append(append([]string{}, native.ExpectedTools...), "company/draft_post", "company/publish_post")
		if r := invoke(agentworker.ControlPath+"confinement", wrong); r.Code != 409 {
			current.Fatal("TG accepted a write-capable confinement profile", r.Code)
		}
		if r := invoke(agentworker.ControlPath+"confinement", native); r.Code != http.StatusOK {
			current.Fatal("TG read-only confinement refused", r.Code)
		}
		listed := invoke(agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
		var tools struct {
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					Annotations struct {
						ReadOnly bool `json:"readOnlyHint"`
					} `json:"annotations"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal(listed.Body.Bytes(), &tools) != nil || len(tools.Result.Tools) != 3 {
			current.Fatal("TG tools not narrowed")
		}
		for _, item := range tools.Result.Tools {
			if !item.Annotations.ReadOnly || !strings.Contains(strings.Join(native.ExpectedTools, ","), "company/"+item.Name) {
				current.Fatal("TG advertised forbidden tools")
			}
		}
		if err := service.StartExecution(ctx, l); err != nil {
			current.Fatal(err)
		}
		args, _ := json.Marshal(map[string]any{"type": "project", "id": project.ID})
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "read_resource", args); err != nil {
			current.Fatal(err)
		}
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "draft_post", json.RawMessage(`{}`)); err == nil {
			current.Fatal("group request acquired a write tool")
		}
		if err := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "已核对指定项目的实际资料。", SourceRefs: []agent.ResultReference{{Type: "project", ID: project.ID}}, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{InputTokens: 100, OutputTokens: 20}); err != nil {
			current.Fatal(err)
		}
		rec, res := f.request("GET", "/v1/conversations?type=telegram", f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		list := data[core.Page[agent.Conversation]](current, res)
		found := false
		for _, item := range list.Items {
			if item.Type != "telegram" {
				current.Fatal("filter returned a portal context")
			}
			if item.ID == *completed.ConversationID {
				found = true
			}
		}
		if !found {
			current.Fatal("channel conversation filter did not show exact context")
		}
		completed = lookup(completed.ID)
		if *completed.RunState != "succeeded" {
			current.Fatal("actual run did not complete")
		}
	})
	run("each requester gets a separate context and portal cannot inject tasks or change members", func() {
		id := request(987654322, "/brief 请核对指定项目的已知进展")
		dispatch()
		other := lookup(id)
		if other.ConversationID == nil || *other.ConversationID == *completed.ConversationID {
			current.Fatal("two actors shared a context")
		}
		rec, res := f.request("GET", "/v1/conversations/"+*completed.ConversationID, f.bobToken, "", nil, nil)
		expect(current, rec, 404, res)
		rec, res = f.request("GET", "/v1/conversations/"+*completed.ConversationID, f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		c := data[agent.Conversation](current, res)
		if c.Type != "telegram" || c.CanEdit || c.CanManageMembers {
			current.Fatal("channel scope became an editable portal context")
		}
		rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/runtime-options", f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		if options := data[agent.RuntimeOptions](current, res); options.Available || options.Reason == nil || *options.Reason != "CHANNEL_MANAGED" {
			current.Fatal("portal task input was enabled")
		}
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "不能从门户注入TG会话"}, nil)
		expect(current, rec, 403, res)
		cancel(other)
	})
	run("channel revision withdraws queued work before execution", func() {
		id := request(987654321, "/brief 此请求应随入口修订撤回")
		dispatch()
		r := lookup(id)
		updateChannel("active")
		if lookup(id).State != "withdrawn" {
			current.Fatal("queued receipt not withdrawn")
		}
		cancel(r)
	})
	run("member changes pause the topic and revoke a running execution before further tools", func() {
		id := request(987654321, "/brief 检查成员变化时的执行停止")
		dispatch()
		r := lookup(id)
		j, e, l := claim(*r.RunID)
		start(e, l)
		sequence++
		accept(map[string]any{"update_id": sequence, "chat_member": map[string]any{"chat": map[string]any{"id": int64(-1001122334455), "type": "supergroup"}}})
		if lookup(id).State != "withdrawn" {
			current.Fatal("running receipt not withdrawn")
		}
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "read_resource", json.RawMessage(`{}`)); err == nil {
			current.Fatal("tool survived channel withdrawal")
		}
		if err := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "不能提交失效上下文", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); err == nil {
			current.Fatal("result survived withdrawal")
		}
		if err := service.FinishStoppedExecution(ctx, j, "TELEGRAM_SCOPE_CHANGED"); err != nil {
			current.Fatal(err)
		}
		updateChannel("active")
	})
	run("link failure rolls back task context message and queue then exact retry recovers", func() {
		id := request(987654321, "/brief 回滚后只能入队一次")
		exec(`CREATE FUNCTION telegram_test_link_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled link failure' USING ERRCODE='XX000'; END $$`)
		exec(`CREATE TRIGGER telegram_test_link_failure BEFORE INSERT ON telegram_run_links FOR EACH ROW EXECUTE FUNCTION telegram_test_link_failure()`)
		_, err := service.DispatchTelegramOnce(ctx, f.org, bot.ID)
		exec(`DROP TRIGGER telegram_test_link_failure ON telegram_run_links`)
		exec(`DROP FUNCTION telegram_test_link_failure()`)
		if err == nil || lookup(id).State != "awaiting_executor" || lookup(id).RunID != nil {
			current.Fatal("partial failed dispatch survived")
		}
		dispatch()
		r := lookup(id)
		if r.RunID == nil {
			current.Fatal("exact receipt retry failed")
		}
		cancel(r)
	})
	run("retired profile blocks an already queued Telegram run", func() {
		id := request(987654321, "/brief 旧运行配置不得继续执行")
		dispatch()
		r := lookup(id)
		exec("UPDATE agent_runtime_profiles SET state='retired' WHERE id=$1", profile)
		var desired *jobqueue.Job
		for i := 0; i < 20; i++ {
			j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "telegram.retired", agent.ExecutionLeaseDuration)
			if err != nil || j == nil {
				current.Fatal(err)
			}
			var p struct {
				RunID string `json:"run_id"`
			}
			json.Unmarshal(j.Payload, &p)
			if p.RunID == *r.RunID {
				desired = j
				break
			}
			if err = service.FinishStoppedExecution(ctx, *j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
				current.Fatal(err)
			}
		}
		if desired == nil {
			current.Fatal("queued job missing")
		}
		if _, err := service.IssueExecution(ctx, *desired); err == nil {
			current.Fatal("retired profile executed")
		}
		if err := service.FinishStoppedExecution(ctx, *desired, "RUNTIME_PROFILE_RETIRED"); err != nil {
			current.Fatal(err)
		}
		profile = install("project-briefing")
	})
	run("binding revocation fences a live execution and the original result remains historical", func() {
		id := request(987654321, "/brief 撤销身份即停止新的工具调用")
		dispatch()
		r := lookup(id)
		j, e, l := claim(*r.RunID)
		start(e, l)
		rec, res := f.request("POST", "/v1/me/telegram/bindings/"+binding.ID+"/revoke", f.aliceToken, core.NewID(), telegram.ReasonInput{ExpectedVersion: binding.Version, Reason: "本人撤销TG执行身份"}, nil)
		expect(current, rec, 200, res)
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "search_resources", json.RawMessage(`{}`)); err == nil {
			current.Fatal("revoked binding retained tools")
		}
		if err := service.FinishStoppedExecution(ctx, j, "TELEGRAM_SCOPE_CHANGED"); err != nil {
			current.Fatal(err)
		}
		if *lookup(completed.ID).RunState != "succeeded" {
			current.Fatal("revocation rewrote completed fact")
		}
		binding = bind(f.aliceToken, 987654321)
	})
	run("scope and task origins are append only and raw portal tasks cannot use Telegram context", func() {
		if _, err := f.owner.Exec(ctx, "UPDATE agent_runtime_profiles SET telegram_configuration_sha256=repeat('e',64) WHERE id=$1", profile); err == nil {
			current.Fatal("installed TG configuration was rewritten")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE telegram_run_links SET run_id=$1 WHERE inbox_id=$2", core.NewID(), completed.ID); err == nil {
			current.Fatal("app rewrote immutable link")
		}
		if _, err := f.owner.Exec(ctx, "DELETE FROM telegram_conversation_scopes WHERE conversation_id=$1", completed.ConversationID); err == nil {
			current.Fatal("owner rewrote immutable scope")
		}
	})

	// Prepare a fresh completed context for the real browser, then let the normal
	// dispatcher consume browser inbound updates without invoking any model.
	if os.Getenv("CP_BROWSER_TELEGRAM_DISPATCH_FIXTURE") == "1" {
		id := request(987654321, "/brief 浏览器可查阅的真实TG任务")
		dispatch()
		r := lookup(id)
		_, e, l := claim(*r.RunID)
		start(e, l)
		args, _ := json.Marshal(map[string]any{"type": "project", "id": project.ID})
		if _, err := service.ExecutionTool(ctx, l, core.NewID(), "read_resource", args); err != nil {
			t.Fatal(err)
		}
		if err := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "已核对浏览器指定项目的真实资料。", SourceRefs: []agent.ResultReference{{Type: "project", ID: project.ID}}, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{InputTokens: 100, OutputTokens: 20}); err != nil {
			t.Fatal(err)
		}
		owned, cancelLoop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-owned.Done():
					return
				case <-ticker.C:
					_, _ = service.DispatchTelegramOnce(owned, f.org, bot.ID)
				}
			}
		}()
		holdBrowserBusinessFixture(t, f, map[string]any{"telegram_dispatch_fixture": true, "bot_id": bot.ID, "bot_username": bot.Username, "webhook_secret": bot.WebhookSecret, "channel_id": channel.ID, "conversation_id": *r.ConversationID, "completed_run_id": *r.RunID, "profile_id": profile, "project_id": project.ID})
		cancelLoop()
		<-done
	}
}
