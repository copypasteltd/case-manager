package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

// Real native PostgreSQL, authenticated public HTTP and the actual MCP bridge.
// Only native-confinement proof is controlled; no model or external service runs.
func TestAgentRunMutations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []string{f.alice, f.bob} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, u)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	p := f.createProject("Exact L1 mutation project", []string{f.bob})
	foreign := f.createProject("Another project is outside this conversation", nil)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-update','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated exact update fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact project update conversation", MemberIDs: []string{}}, nil)
	expect(t, rec, 201, res)
	c := data[agent.Conversation](t, res)
	service := &agent.Service{DB: f.api.DB}
	bridge := &agentworker.Bridge{Service: service, Verifier: &controlledNativeVerifier{}, Model: "controlled-test", WorkingDirectory: "controlled-empty-scope"}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	credentials := map[string]string{}
	post := func(l agent.ExecutionLease, path string, body any, status int) map[string]json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+credentials[l.RunID])
		response, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var out map[string]json.RawMessage
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != status {
			t.Fatalf("owned update bridge returned %d, want %d", response.StatusCode, status)
		}
		return out
	}
	input := func(tool string, args map[string]any) agent.RunCreate {
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Only execute the exact selected update; never widen fields or replace human review."}
		if tool != "" {
			raw, _ := json.Marshal(args)
			in.Mutation = &agent.MutationInput{Tool: tool, Arguments: raw}
		}
		return in
	}
	start := func(in agent.RunCreate) (agent.Run, agent.ExecutionLease) {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 202, res)
		r := data[agent.Run](t, res)
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "mutation.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil {
			t.Fatal(e)
		}
		var prepared struct {
			Mutation *agent.MutationView `json:"authorized_mutation"`
		}
		if json.Unmarshal([]byte(x.PreparedContext), &prepared) != nil || (in.Mutation != nil && (prepared.Mutation == nil || prepared.Mutation.RunID != r.ID || prepared.Mutation.Tool != in.Mutation.Tool)) {
			t.Fatal("exact user mutation missing from prepared context")
		}
		l, e := service.LeaseForCredential(ctx, x.Credential)
		if e != nil {
			t.Fatal(e)
		}
		l.Job = *j
		credentials[l.RunID] = x.Credential
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation"}}
		post(l, agentworker.ControlPath+"confinement", native, 409)
		native.ExpectedTools = append(native.ExpectedTools, "company/create_task", "company/create_risk", "company/update_risk", "company/create_issue", "company/update_issue", "company/create_deliverable", "company/update_deliverable", "company/create_milestone", "company/update_project", "company/update_task", "company/transition_task", "company/transition_project", "company/update_milestone", "company/transition_milestone")
		post(l, agentworker.ControlPath+"confinement", native, 200)
		if e = service.StartExecution(ctx, l); e != nil {
			t.Fatal(e)
		}
		return r, l
	}
	tool := func(l agent.ExecutionLease, key, name string, in any) (json.RawMessage, error) {
		response := post(l, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": key, "method": "tools/call", "params": map[string]any{"name": name, "arguments": in}}, 200)
		if raw := response["error"]; len(raw) > 0 {
			var rpcError struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(raw, &rpcError) != nil {
				t.Fatal("invalid MCP error")
			}
			return nil, core.Fail(409, rpcError.Message, "Controlled MCP authority denial")
		}
		var rpc struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Output json.RawMessage `json:"structuredContent"`
		}
		if e := json.Unmarshal(response["result"], &rpc); e != nil {
			t.Fatalf("MCP %s returned no result: %s", name, string(response["error"]))
		}
		if rpc.IsError {
			return nil, core.Fail(409, rpc.Content[0].Text, "Controlled MCP denial")
		}
		return rpc.Output, nil
	}
	finish := func(l agent.ExecutionLease) {
		t.Helper()
		if e := service.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED"); e != nil {
			t.Fatal(e)
		}
	}
	complete := func(l agent.ExecutionLease, raw json.RawMessage) {
		t.Helper()
		var receipt agent.ExecutionReceipt
		if e := json.Unmarshal(raw, &receipt); e != nil {
			t.Fatal(e)
		}
		if receipt.OperationID == nil || !receipt.Committed || receipt.Status != "succeeded" {
			t.Fatal("write had no genuine business receipt")
		}
		result := agent.ProposedResult{Status: "completed", Summary: "Exact authorized update committed.", ResourceRefs: receipt.ResourceRefs, SourceRefs: receipt.SourceRefs, OperationIDs: []string{*receipt.OperationID}, MissingFields: []string{}}
		if e := service.CompleteExecution(ctx, l, result, agent.ExecutionUsage{}); e != nil {
			t.Fatal(e)
		}
	}
	denied := func(l agent.ExecutionLease, key, name string, args any, code string) {
		t.Helper()
		if _, e := tool(l, key, name, args); core.PublicError(e).Code != code {
			t.Fatalf("expected %s, got %v", code, e)
		}
	}
	task := f.createTask(p, "Task selected for update", false)
	taskArgs := map[string]any{"id": task.ID, "expected_version": task.Version, "reason": "Explicit L1 task edit", "title": "Precisely updated task", "body": "Selected task content only"}
	firstInput := input("update_task", taskArgs)
	firstRun, l := start(firstInput)
	listing := post(l, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": "list", "method": "tools/list"}, 200)
	var listed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(listing["result"], &listed)
	if len(listed.Tools) != 16 {
		t.Fatal("project-update did not expose its exact fourteen write and two read tools")
	}
	// The pre-write read cites immutable intent rather than invalidating its own turn.
	raw, e := tool(l, "read-base", "read_resource", map[string]any{"type": "task", "id": task.ID, "max_chars": 100})
	if e != nil {
		t.Fatal(e)
	}
	var read agent.ExecutionReceipt
	_ = json.Unmarshal(raw, &read)
	if len(read.SourceRefs) != 1 || read.SourceRefs[0].Type != "agent_mutation" || read.SourceRefs[0].ID != firstRun.ID {
		t.Fatal("target read was not bound to exact immutable intent")
	}
	denied(l, "wrong-target", "update_task", map[string]any{"id": core.NewID(), "expected_version": task.Version, "reason": "Explicit L1 task edit", "title": "Other"}, "MUTATION_INTENT_MISMATCH")
	denied(l, "wider-fields", "update_task", map[string]any{"id": task.ID, "expected_version": task.Version, "reason": "Explicit L1 task edit", "title": "Precisely updated task", "body": "Model substituted content"}, "MUTATION_INTENT_MISMATCH")
	raw, e = tool(l, "actual-edit", "update_task", taskArgs)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := tool(l, "another-key-same-intent", "update_task", taskArgs)
	if e != nil || !bytes.Equal(raw, replay) {
		t.Fatal("same run created a second operation", e)
	}
	if _, e = tool(l, "read-result", "read_resource", map[string]any{"type": "task", "id": task.ID}); e != nil {
		t.Fatal(e)
	}
	complete(l, raw)
	var version, commits int64
	if e = f.owner.QueryRow(ctx, "SELECT version FROM tasks WHERE id=$1", task.ID).Scan(&version); e != nil || version != task.Version+1 {
		t.Fatal("task was not changed exactly once", e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", firstRun.ID).Scan(&commits); e != nil || commits != 1 {
		t.Fatal("missing exact commit", e)
	}
	// Client-message recovery after the version has changed returns the original run.
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), firstInput, nil)
	expect(t, rec, 202, res)
	if data[agent.Run](t, res).ID != firstRun.ID {
		t.Fatal("lost-ack recovery created a second run")
	}
	rec, res = f.request("GET", "/v1/runs/"+firstRun.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	view := data[agent.Run](t, res).Mutation
	if view == nil || view.Status != "committed" || view.OperationID == nil || *view.ResultVersion != version {
		t.Fatal("public run omitted the real update receipt")
	}

	// Explicit null is a selected clear, while omitted dates are preserved.
	rec, res = f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "start_date": "2026-10-01", "due_date": "2026-12-01"}, nil)
	expect(t, rec, 200, res)
	p = data[project.Project](t, res)
	projectArgs := map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Only change the name", "name": "Exact L1 project rename"}
	_, l = start(input("update_project", projectArgs))
	raw, e = tool(l, "project-rename", "update_project", projectArgs)
	if e != nil {
		t.Fatal(e)
	}
	complete(l, raw)
	rec, res = f.request("GET", "/v1/projects/"+p.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	p = data[project.Project](t, res)
	if p.StartDate == nil || p.DueDate == nil {
		t.Fatal("omitted date fields were cleared")
	}
	projectArgs = map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Clear only the due date", "due_date": nil}
	_, l = start(input("update_project", projectArgs))
	raw, e = tool(l, "clear-date", "update_project", projectArgs)
	if e != nil {
		t.Fatal(e)
	}
	complete(l, raw)
	rec, res = f.request("GET", "/v1/projects/"+p.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	p = data[project.Project](t, res)
	if p.DueDate != nil || p.StartDate == nil {
		t.Fatal("explicit null clear widened the patch")
	}

	// Ordinary task start is a genuine transition, without a second approval.
	transitionArgs := map[string]any{"id": task.ID, "expected_version": version, "reason": "Explicitly start task", "action": "start"}
	_, l = start(input("transition_task", transitionArgs))
	raw, e = tool(l, "start-task", "transition_task", transitionArgs)
	if e != nil {
		t.Fatal(e)
	}
	complete(l, raw)

	// Plain conversation and a citation from an older run never authorize writes.
	_, l = start(input("", nil))
	denied(l, "no-grant", "update_task", taskArgs, "MUTATION_INTENT_REQUIRED")
	result := agent.ProposedResult{Status: "completed", Summary: "Cannot invent a completed mutation.", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}
	if e = service.CompleteExecution(ctx, l, result, agent.ExecutionUsage{}); core.PublicError(e).Code != "RESULT_COMPLETION_UNVERIFIED" {
		t.Fatal("read-only turn claimed an update", e)
	}
	finish(l)
	for _, bad := range []agent.RunCreate{
		input("update_project", map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Not permission authority", "owner_id": f.bob}),
		input("update_task", map[string]any{"id": task.ID, "expected_version": version + 1, "reason": "Cannot lower acceptance", "review_required": false}),
		input("update_project", map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Not policy authority", "classification": "internal"}),
	} {
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
		expect(t, rec, 422, res)
	}
	bad := input("update_project", map[string]any{"id": foreign.ID, "expected_version": foreign.Version, "reason": "Wrong project", "name": "Must not change"})
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
	expect(t, rec, 404, res)
	bad = input("", nil)
	bad.ContextRefs = []agent.SourceReference{{Type: "agent_mutation", ID: firstRun.ID}}
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
	expect(t, rec, 422, res)
	review := f.createTask(p, "Human acceptance stays human", true)
	bad = input("transition_task", map[string]any{"id": review.ID, "expected_version": review.Version, "reason": "Do not proxy human acceptance", "action": "complete"})
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
	expect(t, rec, 422, res)
	if res.Error.Code != "TASK_HUMAN_REVIEW_REQUIRED" {
		t.Fatal("missing human acceptance boundary")
	}

	// Existing dependency guards run inside the exact same business transaction.
	predecessor := f.createTask(p, "Unfinished predecessor", false)
	successor := f.createTask(p, "Blocked successor", false)
	rec, res = f.request("POST", "/v1/tasks/"+successor.ID+"/dependencies", f.aliceToken, core.NewID(), project.Dependency{ExpectedVersion: successor.Version, PredecessorID: predecessor.ID, Action: "add"}, nil)
	expect(t, rec, 200, res)
	rec, res = f.request("GET", "/v1/tasks/"+successor.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	successor = data[project.Task](t, res)
	blockedArgs := map[string]any{"id": successor.ID, "expected_version": successor.Version, "reason": "Preserve dependencies", "action": "start"}
	blockedRun, bl := start(input("transition_task", blockedArgs))
	denied(bl, "dependency-blocked", "transition_task", blockedArgs, "DEPENDENCY_BLOCKED")
	finish(bl)
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", blockedRun.ID).Scan(&commits); e != nil || commits != 0 {
		t.Fatal("denied mutation left a commit")
	}
	var operations int
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", "agent-mutation:"+blockedRun.ID).Scan(&operations); e != nil || operations != 0 {
		t.Fatal("denied transaction left a business operation")
	}

	// An external concurrent edit invalidates even a previously read target.
	concurrent := f.createTask(p, "Concurrent source", false)
	concurrentArgs := map[string]any{"id": concurrent.ID, "expected_version": concurrent.Version, "reason": "Exact old version", "title": "Old intended change"}
	_, l = start(input("update_task", concurrentArgs))
	if _, e = tool(l, "before-external-edit", "read_resource", map[string]any{"type": "task", "id": concurrent.ID}); e != nil {
		t.Fatal(e)
	}
	rec, res = f.request("PATCH", "/v1/tasks/"+concurrent.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": concurrent.Version, "title": "Another human changed the task"}, nil)
	expect(t, rec, 200, res)
	denied(l, "stale-after-read", "update_task", concurrentArgs, "RUNTIME_SOURCE_CHANGED")
	finish(l)

	// Current project scope/date gates remain the original approved-change flow.
	rec, res = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: "activate", Reason: "Start before scope guard verification"}, nil)
	expect(t, rec, 200, res)
	p = data[project.Project](t, res)
	scopeArgs := map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Preserve approved change process", "scope": "Cannot silently replace active scope"}
	scopeRun, sl := start(input("update_project", scopeArgs))
	denied(sl, "active-scope-guard", "update_project", scopeArgs, "PROJECT_SCOPE_REQUIRES_APPROVED_CHANGE")
	finish(sl)
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", scopeRun.ID).Scan(&commits); e != nil || commits != 0 {
		t.Fatal("scope gate left a mutation commit")
	}

	// Original child-task completion guard is not bypassed by L1 authority.
	parent := f.createTask(p, "Parent requires child completion", false)
	_ = f.createTask(p, "Separate unfinished task", false)
	rec, res = f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "Unfinished child", Priority: "normal", ParentID: &parent.ID}, nil)
	expect(t, rec, 201, res)
	rec, res = f.transition(parent, "start", f.aliceToken)
	expect(t, rec, 200, res)
	parent = data[project.Task](t, res)
	parentArgs := map[string]any{"id": parent.ID, "expected_version": parent.Version, "reason": "Do not omit child work", "action": "complete"}
	_, l = start(input("transition_task", parentArgs))
	denied(l, "children-still-open", "transition_task", parentArgs, "CHILD_TASKS_OPEN")
	finish(l)

	// Cancellation before submission leaves no business mutation.
	cancelTask := f.createTask(p, "Cancel before mutation", false)
	cancelArgs := map[string]any{"id": cancelTask.ID, "expected_version": cancelTask.Version, "reason": "Explicit request later cancelled", "title": "Never apply cancelled edit"}
	cancelRun, cl := start(input("update_task", cancelArgs))
	rec, res = f.request("GET", "/v1/runs/"+cancelRun.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	cancelRun = data[agent.Run](t, res)
	rec, res = f.request("POST", "/v1/runs/"+cancelRun.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": cancelRun.Version, "action": "cancel", "reason": "Stop before business commit"}, nil)
	expect(t, rec, 200, res)
	denied(cl, "cancel-fences-write", "update_task", cancelArgs, "RUNTIME_CANCEL_REQUESTED")
	finish(cl)
	if e = f.owner.QueryRow(ctx, "SELECT version FROM tasks WHERE id=$1", cancelTask.ID).Scan(&version); e != nil || version != cancelTask.Version {
		t.Fatal("cancelled task changed", e)
	}

	// Cancellation after a real commit reports that fact instead of claiming rollback.
	committedTask := f.createTask(p, "Cancel after mutation", false)
	committedArgs := map[string]any{"id": committedTask.ID, "expected_version": committedTask.Version, "reason": "Commit remains historical fact", "title": "Applied before cancellation"}
	committedRun, cl := start(input("update_task", committedArgs))
	raw, e = tool(cl, "commit-before-cancel", "update_task", committedArgs)
	if e != nil {
		t.Fatal(e)
	}
	rec, res = f.request("GET", "/v1/runs/"+committedRun.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	committedRun = data[agent.Run](t, res)
	rec, res = f.request("POST", "/v1/runs/"+committedRun.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": committedRun.Version, "action": "cancel", "reason": "Stop remaining model work"}, nil)
	expect(t, rec, 200, res)
	finish(cl)
	rec, res = f.request("GET", "/v1/runs/"+committedRun.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	committedRun = data[agent.Run](t, res)
	if committedRun.State != "cancelled" || committedRun.Mutation == nil || committedRun.Mutation.Status != "committed" {
		t.Fatal("cancellation erased the actual business commit")
	}
	if e = f.owner.QueryRow(ctx, "SELECT version FROM tasks WHERE id=$1", committedTask.ID).Scan(&version); e != nil || version != committedTask.Version+1 {
		t.Fatal("post-commit cancellation pretended to undo business state", e)
	}

	// Immutable records and the successful business operation cannot be rewritten.
	rollbackTask := f.createTask(p, "Rollback if receipt persistence fails", false)
	rollbackArgs := map[string]any{"id": rollbackTask.ID, "expected_version": rollbackTask.Version, "reason": "Atomic business and evidence", "title": "Must roll back with failed receipt"}
	rollbackRun, rl := start(input("update_task", rollbackArgs))
	exec(`CREATE FUNCTION fixture_reject_mutation_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.run_id='` + rollbackRun.ID + `'::uuid AND NEW.tool='update_task' THEN RAISE EXCEPTION 'controlled persistence failure'; END IF; RETURN NEW; END; $$`)
	exec("CREATE TRIGGER fixture_mutation_receipt_failure BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fixture_reject_mutation_receipt()")
	denied(rl, "forced-receipt-failure", "update_task", rollbackArgs, "INTERNAL_ERROR")
	exec("DROP TRIGGER fixture_mutation_receipt_failure ON agent_tool_receipts")
	exec("DROP FUNCTION fixture_reject_mutation_receipt()")
	if e = f.owner.QueryRow(ctx, "SELECT version FROM tasks WHERE id=$1", rollbackTask.ID).Scan(&version); e != nil || version != rollbackTask.Version {
		t.Fatal("business state escaped rolled back receipt", e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", "agent-mutation:"+rollbackRun.ID).Scan(&operations); e != nil || operations != 0 {
		t.Fatal("business operation escaped receipt rollback", e)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", rollbackRun.ID).Scan(&commits); e != nil || commits != 0 {
		t.Fatal("commit escaped receipt rollback", e)
	}
	raw, e = tool(rl, "retry-after-confirmed-rollback", "update_task", rollbackArgs)
	if e != nil {
		t.Fatal(e)
	}
	complete(rl, raw)

	// Shared-conversation participants can read only the common project scope.
	privateConversation := c
	rec, res = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Common project update scope", MemberIDs: []string{f.bob}}, nil)
	expect(t, rec, 201, res)
	c = data[agent.Conversation](t, res)
	sharedTask := f.createTask(p, "Shared exact update", false)
	sharedArgs := map[string]any{"id": sharedTask.ID, "expected_version": sharedTask.Version, "reason": "Commonly visible update", "title": "Updated in common project scope"}
	sharedRun, sharedLease := start(input("update_task", sharedArgs))
	raw, e = tool(sharedLease, "shared-mutation", "update_task", sharedArgs)
	if e != nil {
		t.Fatal(e)
	}
	complete(sharedLease, raw)
	rec, res = f.request("GET", "/v1/runs/"+sharedRun.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 200, res)
	if data[agent.Run](t, res).Mutation == nil {
		t.Fatal("common audience lost exact update evidence")
	}
	rec, res = f.request("GET", "/v1/runs/"+sharedRun.ID, f.memberToken, "", nil, nil)
	expect(t, rec, 404, res)
	// A new participant can read later common business documents, but not the
	// earlier private instruction through either HTTP history or a model tool.
	exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer')", f.org, p.ID, f.member)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.member)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/members", f.aliceToken, core.NewID(), agent.MemberChange{ExpectedVersion: c.Version, UserID: f.member, Action: "add", Reason: "Explicit later shared audience"}, nil)
	expect(t, rec, 200, res)
	rec, res = f.request("GET", "/v1/conversations/"+c.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	c = data[agent.Conversation](t, res)
	rec, res = f.request("GET", "/v1/runs/"+sharedRun.ID, f.memberToken, "", nil, nil)
	expect(t, rec, 404, res)
	_, laterLease := start(input("", nil))
	denied(laterLease, "do-not-redisclose-old-instruction", "read_resource", map[string]any{"type": "agent_mutation", "id": sharedRun.ID}, "RESOURCE_NOT_FOUND")
	finish(laterLease)
	c = privateConversation

	for _, sql := range []string{"UPDATE agent_run_mutations SET tool='update_project' WHERE run_id=$1", "DELETE FROM agent_run_mutations WHERE run_id=$1", "UPDATE agent_mutation_commits SET result_version=result_version+1 WHERE run_id=$1", "DELETE FROM agent_mutation_commits WHERE run_id=$1"} {
		if _, e = f.owner.Exec(ctx, sql, firstRun.ID); e == nil {
			t.Fatal("immutable mutation evidence was changed")
		}
	}
	// Keep a real committed record and an available profile for opt-in browser QA.
	if os.Getenv("CP_BROWSER_AGENT_MUTATION_FIXTURE") == "1" && !t.Failed() {
		// The ordinary L1 roles intentionally do not require an additional MFA
		// challenge. Only this browser fixture installs an actionless proof role
		// so its existing shared browser-login helper has an enrolled account.
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'mutation_browser_proof','Synthetic browser MFA proof without business actions',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"mutation_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "task_id": task.ID, "mutation_profile_id": profile, "committed_run_id": firstRun.ID})
	}

	// Revocation stops further writes and withdraws dependent historical content.
	pending := f.createTask(p, "Permission withdrawal", false)
	pendingArgs := map[string]any{"id": pending.ID, "expected_version": pending.Version, "reason": "Stop after withdrawal", "title": "Must not apply"}
	_, l = start(input("update_task", pendingArgs))
	exec("UPDATE role_bindings SET revoked_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	denied(l, "withdrawn-authority", "update_task", pendingArgs, "RUNTIME_AUTHORITY_CHANGED")
	finish(l)
	rec, res = f.request("GET", "/v1/runs/"+firstRun.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 404, res)
	rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=100", f.aliceToken, "", nil, nil)
	expect(t, rec, 404, res)
}
