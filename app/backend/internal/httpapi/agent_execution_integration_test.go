package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/project"
)

type controlledNativeVerifier struct{ denied bool }

func (v *controlledNativeVerifier) Verify(_ context.Context, l agent.ExecutionLease, _ agentworker.NativeProfile) (agentworker.ConfinementProof, error) {
	if v.denied {
		return agentworker.ConfinementProof{}, core.Fail(409, "RUNTIME_CONFINEMENT_UNVERIFIED", "Controlled denied fixture")
	}
	return agentworker.ConfinementProof{SHA256: strings.Repeat("c", 64), ExpiresAt: time.Now().Add(30 * time.Second)}, nil
}

type controlledAgentExecutor func(context.Context, agent.Execution) error

func (f controlledAgentExecutor) Execute(ctx context.Context, r agent.Execution) error {
	return f(ctx, r)
}

// All identities, profiles and confinement responses here are isolated test data;
// this suite never starts Codex or calls a provider, Telegram or object storage.
func TestAgentExecutionController(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	service := &agent.Service{DB: f.api.DB}
	current := t
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			current.Fatal(err)
		}
	}
	p := f.createProject("Bounded execution evidence", []string{f.bob, f.member})
	foreign := f.createProject("Private execution evidence", []string{f.member})
	for _, u := range []string{f.alice, f.bob} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, u)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	profileID := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-briefing','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,8,600,$4,'Isolated controller fixture only')`, profileID, f.org, agentworker.RuntimeVersion, f.admin)
	verifier := &controlledNativeVerifier{}
	bridge := &agentworker.Bridge{Service: service, Verifier: verifier, Model: "controlled-test", WorkingDirectory: "controlled-empty-scope"}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	post := func(token, path string, body any, status int) map[string]json.RawMessage {
		current.Helper()
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
		if err != nil {
			current.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := server.Client().Do(req)
		if err != nil {
			current.Fatal(err)
		}
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			current.Fatal(err)
		}
		if response.StatusCode != status {
			current.Fatalf("controller status %d expected %d: %s", response.StatusCode, status, content)
		}
		var out map[string]json.RawMessage
		if len(content) > 0 && json.Unmarshal(content, &out) != nil {
			current.Fatal("invalid controller JSON")
		}
		return out
	}
	newRun := func() agent.Run {
		current.Helper()
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Controller test", MemberIDs: []string{f.bob}}, nil)
		expect(current, rec, 201, res)
		c := data[agent.Conversation](current, res)
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profileID, ClientMessageID: core.NewID(), Message: "Read only this shared project; do not perform business writes."}
		key := core.NewID()
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, key, in, nil)
		expect(current, rec, 202, res)
		r := data[agent.Run](current, res)
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, key, in, nil)
		expect(current, rec, 202, res)
		if data[agent.Run](current, res).ID != r.ID {
			current.Fatal("duplicate run")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_run_jobs WHERE run_id=$1", r.ID).Scan(&n); err != nil || n != 1 {
			current.Fatal("run must map one durable job", err)
		}
		return r
	}
	claim := func(r agent.Run) (jobqueue.Job, agent.Execution, agent.ExecutionLease) {
		current.Helper()
		j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "controller.test", agent.ExecutionLeaseDuration)
		if err != nil || j == nil {
			current.Fatal("claim failed", err)
		}
		run, err := service.IssueExecution(ctx, *j)
		if err != nil {
			current.Fatal(err)
		}
		if run.RunID != r.ID {
			current.Fatal("wrong queued run claimed")
		}
		lease, err := service.LeaseForCredential(ctx, run.Credential)
		if err != nil {
			current.Fatal(err)
		}
		return *j, run, lease
	}
	state := func(r agent.Run) agent.Run {
		rec, res := f.request("GET", "/v1/runs/"+r.ID, f.aliceToken, "", nil, nil)
		expect(current, rec, 200, res)
		return data[agent.Run](current, res)
	}
	start := func(run agent.Execution) {
		post(run.Credential, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profileID, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/search_resources", "company/inspect_operation", "company/draft_post", "company/publish_post"}}, 200)
		post(run.Credential, agentworker.ControlPath+"events", map[string]any{"type": "run.started"}, 200)
	}
	tool := func(lease agent.ExecutionLease, key, name string, args any) json.RawMessage {
		raw, _ := json.Marshal(args)
		out, err := service.ExecutionTool(ctx, lease, key, name, raw)
		if err != nil {
			current.Fatal(err)
		}
		return out
	}
	result := func() agent.ProposedResult {
		return agent.ProposedResult{Status: "completed", Summary: "已依据当前共同权限读取项目。", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{{Type: "project", ID: p.ID}}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: ""}
	}
	usage := agent.ExecutionUsage{InputTokens: 100, OutputTokens: 20}
	finish := func(j jobqueue.Job) {
		current.Helper()
		settled, err := service.ExecutionSettled(ctx, j)
		if err != nil {
			current.Fatal(err)
		}
		if !settled {
			if err = service.FinishStoppedExecution(ctx, j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
				current.Fatal(err)
			}
		}
	}
	runCase := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { current = t; defer func() { current = t }(); fn(t) })
		current = t
	}

	runCase("one live claim and exact credentials cannot start without verified native confinement", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		other, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "other.test", agent.ExecutionLeaseDuration)
		if err != nil || other != nil {
			t.Fatal("second worker claimed same run", err)
		}
		post(strings.Repeat("f", 96), agentworker.ControlPath+"current", map[string]any{}, 401)
		post(run.Credential, agentworker.ControlPath+"current", map[string]any{"actor_id": f.admin}, 422)
		if err = service.StartExecution(ctx, lease); core.PublicError(err).Code != "RUNTIME_CONFINEMENT_UNVERIFIED" {
			t.Fatal("missing confinement admitted", err)
		}
		bad := lease
		bad.Job.FencingToken++
		if service.CheckExecution(ctx, bad) == nil {
			t.Fatal("old fence admitted")
		}
		verifier.denied = true
		post(run.Credential, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profileID, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/search_resources", "company/inspect_operation", "company/draft_post", "company/publish_post"}}, 409)
		verifier.denied = false
		start(run)
		if state(r).State != "running" {
			t.Fatal("verified execution did not start")
		}
	})
	runCase("real MCP reads have bounded schemas deduplicated receipts and reject invented references", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		start(run)
		rpc := func(id int, method string, params any) map[string]json.RawMessage {
			return post(run.Credential, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}, 200)
		}
		initialized := rpc(1, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "controlled-test", "version": "1"}})
		if initialized["error"] != nil {
			t.Fatal(string(initialized["error"]))
		}
		negotiated := rpc(10, "initialize", map[string]any{"protocolVersion": "2099-01-01", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "controlled-new-client", "version": "1"}})
		if !bytes.Contains(negotiated["result"], []byte(`"protocolVersion":"2025-06-18"`)) {
			t.Fatal("newer client protocol did not negotiate a supported version")
		}
		defs := rpc(2, "tools/list", map[string]any{})
		if !bytes.Contains(defs["result"], []byte("read_resource")) || bytes.Contains(defs["result"], []byte("commit_file")) {
			t.Fatal("wrong tool surface")
		}
		args := map[string]any{"type": "project", "id": p.ID, "max_chars": 1000}
		got := rpc(3, "tools/call", map[string]any{"name": "read_resource", "arguments": args})
		if !bytes.Contains(got["result"], []byte("Bounded execution evidence")) {
			t.Fatal("real project text absent")
		}
		rpc(3, "tools/call", map[string]any{"name": "read_resource", "arguments": args})
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_tool_receipts WHERE run_id=$1", r.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal("replay duplicated receipt", err)
		}
		args["actor_id"] = f.admin
		got = rpc(4, "tools/call", map[string]any{"name": "read_resource", "arguments": args})
		if !bytes.Contains(got["result"], []byte(`"isError":true`)) {
			t.Fatal("identity injection accepted")
		}
		forged := result()
		forged.SourceRefs[0].ID = core.NewID()
		if core.PublicError(service.CompleteExecution(ctx, lease, forged, usage)).Code != "RESULT_REFERENCE_UNVERIFIED" {
			t.Fatal("invented source completed")
		}
		forged = result()
		forged.OperationIDs = []string{core.NewID()}
		if core.PublicError(service.CompleteExecution(ctx, lease, forged, usage)).Code != "RESULT_OPERATION_UNVERIFIED" {
			t.Fatal("invented business commit admitted")
		}
		over := usage
		over.InputTokens = 10001
		if core.PublicError(service.CompleteExecution(ctx, lease, result(), over)).Code != "RUNTIME_USAGE_BUDGET_EXCEEDED" {
			t.Fatal("provider budget not enforced")
		}
		if err := service.CompleteExecution(ctx, lease, result(), usage); err != nil {
			t.Fatal(err)
		}
		gotRun := state(r)
		if gotRun.State != "succeeded" || gotRun.Outcome == nil || *gotRun.Outcome != "completed" {
			t.Fatal(gotRun)
		}
		drainNotifications(t, f)
		notice := requireBusinessNotification(t, f, f.aliceToken, r.ID, "secretary", false)
		if notice.Target.ContainerID == nil || *notice.Target.ContainerID != gotRun.ConversationID || notificationFor(t, f, f.bobToken, r.ID) != nil {
			t.Fatal("secretary result was not isolated to the initiating employee", notice)
		}
	})
	runCase("reply result event audit outbox and queue success roll back together", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		start(run)
		tool(lease, "read", "read_resource", map[string]any{"type": "project", "id": p.ID})
		exec(`CREATE FUNCTION test_agent_result_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='run.completed' THEN RAISE EXCEPTION 'controlled atomic result failure'; END IF;RETURN NEW; END; $$;CREATE TRIGGER test_agent_result_failure BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION test_agent_result_failure()`)
		if service.CompleteExecution(ctx, lease, result(), usage) == nil {
			t.Fatal("failed audit admitted result")
		}
		exec("DROP TRIGGER test_agent_result_failure ON audit_events;DROP FUNCTION test_agent_result_failure()")
		var messages, results int
		var jobState string
		if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM conversation_messages WHERE run_id=$1 AND role='assistant'),(SELECT count(*) FROM agent_run_results WHERE run_id=$1),j.state FROM jobs j WHERE j.id=$2`, r.ID, j.ID).Scan(&messages, &results, &jobState); err != nil {
			t.Fatal(err)
		}
		if messages != 0 || results != 0 || jobState != "running" || state(r).State != "running" {
			t.Fatal("partial result survived rollback", messages, results, jobState)
		}
		reply := result()
		reply.Status = "awaiting_input"
		reply.MissingFields = []string{"统计时间范围"}
		reply.NextAction = "请提供月份"
		if err := service.CompleteExecution(ctx, lease, reply, usage); err != nil {
			t.Fatal(err)
		}
		if service.CompleteExecution(ctx, lease, reply, usage) == nil {
			t.Fatal("terminal reply recreated")
		}
		rec, res := f.request("GET", "/v1/conversations/"+r.ConversationID+"/messages?limit=20", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[agent.Message]](t, res)
		found := false
		for _, m := range page.Items {
			if m.Role == "assistant" && m.Visibility == "available" && strings.Contains(m.Body, "待补充资料") && strings.Contains(m.Body, "统计时间范围") {
				found = true
			}
		}
		if len(page.Items) != 2 || !found {
			t.Fatal("business outcome omitted from real message")
		}
	})
	runCase("read source changes revoke later receipts and suppress a stale answer", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		start(run)
		tool(lease, "read", "read_resource", map[string]any{"type": "project", "id": p.ID})
		exec("UPDATE projects SET name='Changed execution evidence',version=version+1 WHERE id=$1", p.ID)
		if _, err := service.ExecutionReceipts(ctx, lease); core.PublicError(err).Code != "RUNTIME_SOURCE_CHANGED" {
			t.Fatal("changed evidence replayed", err)
		}
		if err := service.CompleteExecution(ctx, lease, result(), usage); core.PublicError(err).Code != "RUNTIME_SOURCE_CHANGED" {
			t.Fatal("stale answer published", err)
		}
	})
	runCase("final evidence check and response commit fence concurrent business source updates", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		start(run)
		tool(lease, "read", "read_resource", map[string]any{"type": "project", "id": p.ID})
		lock, err := f.owner.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()
		if _, err = lock.Exec(ctx, "SELECT pg_advisory_lock(7482048)"); err != nil {
			t.Fatal(err)
		}
		released := false
		defer func() {
			if !released {
				_, _ = lock.Exec(ctx, "SELECT pg_advisory_unlock(7482048)")
			}
		}()
		exec(`CREATE FUNCTION test_agent_result_pause() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='run.completed' THEN PERFORM pg_advisory_xact_lock(7482048); END IF;RETURN NEW; END; $$;CREATE TRIGGER test_agent_result_pause BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION test_agent_result_pause()`)
		completionCtx, stopCompletion := context.WithTimeout(ctx, 10*time.Second)
		defer func() {
			stopCompletion()
			if !released {
				_, _ = lock.Exec(ctx, "SELECT pg_advisory_unlock(7482048)")
				released = true
			}
			exec("DROP TRIGGER test_agent_result_pause ON audit_events;DROP FUNCTION test_agent_result_pause()")
		}()
		completed := make(chan error, 1)
		go func() { completed <- service.CompleteExecution(completionCtx, lease, result(), usage) }()
		waitUntil := time.Now().Add(3 * time.Second)
		blocked := false
		for time.Now().Before(waitUntil) {
			if err = f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=7482048 AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))").Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			if blocked {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !blocked {
			t.Fatal("result did not reach the controlled commit boundary")
		}
		var version int64
		if err = f.owner.QueryRow(ctx, "SELECT version FROM projects WHERE id=$1", p.ID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		written := make(chan int, 1)
		go func() {
			rec, _ := f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "name": "Source changed after atomic reply"}, nil)
			written <- rec.Code
		}()
		select {
		case <-written:
			t.Fatal("business source changed between final evidence verification and commit")
		case <-time.After(100 * time.Millisecond):
		}
		if _, err = lock.Exec(ctx, "SELECT pg_advisory_unlock(7482048)"); err != nil {
			t.Fatal(err)
		}
		released = true
		select {
		case err = <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("result did not commit")
		}
		select {
		case status := <-written:
			if status != 200 {
				t.Fatal("business command failed after reply", status)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("business update did not resume")
		}
		rec, res := f.request("GET", "/v1/conversations/"+r.ConversationID+"/messages?limit=20", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[agent.Message]](t, res)
		hidden := false
		for _, m := range page.Items {
			if m.Role == "assistant" && m.Visibility == "unavailable" && m.Body == "" {
				hidden = true
			}
		}
		if !hidden {
			t.Fatal("later business update did not withdraw the dependent answer")
		}
	})
	runCase("cancel reports do not confirm teardown until the trusted executor exits", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		start(run)
		live := state(r)
		rec, res := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: live.Version, Action: "cancel", Reason: "Controlled stop"}, nil)
		expect(t, rec, 200, res)
		post(run.Credential, agentworker.ControlPath+"events", map[string]any{"type": "run.interrupted", "code": "RUNTIME_CANCEL_REQUESTED", "reconciliation_required": true}, 200)
		if state(r).State != "running" {
			t.Fatal("runtime observation falsely confirmed process exit")
		}
		if err := service.CheckExecution(ctx, lease); core.PublicError(err).Code != "RUNTIME_CANCEL_REQUESTED" {
			t.Fatal("cancelled authority still current", err)
		}
		finish(j)
		if state(r).State != "cancelled" {
			t.Fatal("stopped controller did not settle cancellation")
		}
	})
	runCase("expired lease interrupts without retrying a model or inventing confirmed cancellation", func(t *testing.T) {
		r := newRun()
		j, run, _ := claim(r)
		start(run)
		live := state(r)
		rec, res := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: live.Version, Action: "cancel", Reason: "Lost worker must not invent stop acknowledgement"}, nil)
		expect(t, rec, 200, res)
		exec("UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID)
		if _, err := service.ReconcileExecutions(ctx); err != nil {
			t.Fatal(err)
		}
		if state(r).State != "interrupted" {
			t.Fatal("expired worker not interrupted")
		}
		next, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "replacement.test", agent.ExecutionLeaseDuration)
		if err != nil || next != nil {
			t.Fatal("model job was retried", err)
		}
	})
	runCase("bounded tool budget and search scope do not allow writes or other projects", func(t *testing.T) {
		for _, title := range []string{"Execution cursor A", "Execution cursor B"} {
			rec, res := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: title, Priority: "normal"}, nil)
			expect(t, rec, 201, res)
		}
		r := newRun()
		j, run, lease := claim(r)
		defer finish(j)
		start(run)
		out := tool(lease, "search", "search_resources", map[string]any{"query": "execution", "resource_types": []string{"task"}, "limit": 1})
		var page struct {
			Content struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
				Cursor *string `json:"next_cursor"`
			} `json:"content"`
		}
		if json.Unmarshal(out, &page) != nil || len(page.Content.Items) != 1 || page.Content.Cursor == nil {
			t.Fatal("bounded real search did not paginate")
		}
		cursor := *page.Content.Cursor
		first := page.Content.Items[0].ID
		next := tool(lease, "next-search", "search_resources", map[string]any{"query": "execution", "resource_types": []string{"task"}, "limit": 1, "cursor": cursor})
		if json.Unmarshal(next, &page) != nil || len(page.Content.Items) != 1 || page.Content.Items[0].ID == first || page.Content.Cursor != nil {
			t.Fatal("cursor repeated or skipped visible tasks")
		}
		for _, bad := range []map[string]any{{"query": "another query", "resource_types": []string{"task"}, "limit": 1, "cursor": cursor}, {"query": "execution", "resource_types": []string{"task"}, "limit": 1, "cursor": cursor + "forged"}} {
			raw, _ := json.Marshal(bad)
			if _, err := service.ExecutionTool(ctx, lease, core.NewID(), "search_resources", raw); err == nil {
				t.Fatal("cursor scope or signature forgery admitted")
			}
		}
		raw, _ := json.Marshal(map[string]any{"type": "project", "id": foreign.ID})
		if _, err := service.ExecutionTool(ctx, lease, "unrelated", "read_resource", raw); err == nil {
			t.Fatal("unrelated project exposed")
		}
		if _, err := service.ExecutionTool(ctx, lease, "write", "create_project", json.RawMessage(`{}`)); err == nil {
			t.Fatal("write tool admitted")
		}
		for i := 0; i < 6; i++ {
			tool(lease, core.NewID(), "inspect_operation", map[string]any{"run_id": r.ID})
		}
		if _, err := service.ExecutionTool(ctx, lease, "over-budget", "inspect_operation", json.RawMessage(`{"run_id":"`+r.ID+`"}`)); core.PublicError(err).Code != "RUNTIME_TOOL_BUDGET_EXCEEDED" {
			t.Fatal("budget not enforced", err)
		}
	})
	runCase("controller consumes queue and settles a verified reply without a provider", func(t *testing.T) {
		r := newRun()
		exited := false
		executor := controlledAgentExecutor(func(_ context.Context, run agent.Execution) error {
			defer func() { exited = true }()
			start(run)
			lease, err := service.LeaseForCredential(ctx, run.Credential)
			if err != nil {
				return err
			}
			tool(lease, "read", "read_resource", map[string]any{"type": "project", "id": p.ID})
			return service.CompleteExecution(ctx, lease, result(), usage)
		})
		runner := agentworker.Runner{Service: service, Executor: executor, Owner: "real.queue.fixture", Heartbeat: time.Second}
		handled, err := runner.Once(ctx)
		if err != nil || !handled || !exited || state(r).State != "succeeded" {
			t.Fatal("queue controller did not complete actual ledger", err)
		}
	})
	runCase("running controller observes cancel and waits for its executor to exit", func(t *testing.T) {
		r := newRun()
		started := make(chan struct{})
		exited := false
		runner := agentworker.Runner{Service: service, Owner: "cancel.controller.fixture", Heartbeat: 10 * time.Millisecond, Executor: controlledAgentExecutor(func(execution context.Context, run agent.Execution) error {
			defer func() { exited = true }()
			start(run)
			close(started)
			<-execution.Done()
			return execution.Err()
		})}
		done := make(chan error, 1)
		go func() { _, err := runner.Once(ctx); done <- err }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("controlled executor did not start")
		}
		live := state(r)
		rec, res := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: live.Version, Action: "cancel", Reason: "Stop actual owned controller fixture"}, nil)
		expect(t, rec, 200, res)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("controller failed to stop executor")
		}
		if !exited || state(r).State != "cancelled" {
			t.Fatal("cancel was confirmed before executor exit")
		}
	})
	runCase("current audience authorization changes stop execution even after native start", func(t *testing.T) {
		r := newRun()
		j, run, lease := claim(r)
		start(run)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='agent_user')", f.org, f.bob)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		if err := service.CheckExecution(ctx, lease); core.PublicError(err).Code != "RUNTIME_AUTHORITY_CHANGED" {
			t.Fatal("revoked audience admitted", err)
		}
		finish(j)
	})
}
