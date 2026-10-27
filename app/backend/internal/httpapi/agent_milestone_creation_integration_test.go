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
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

// Real native PostgreSQL, authenticated public HTTP and the actual MCP bridge.
// Only native-confinement proof is controlled; no model or external service runs.
func TestAgentMilestoneCreation(t *testing.T) {
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
	p := f.createProject("Secretary milestone creation and association", []string{f.bob})
	foreign := f.createProject("Another project is outside this conversation", nil)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-update','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated exact update fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact project stage conversation", MemberIDs: []string{}}, nil)
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

	runTest := func(name string, fn func()) {
		t.Run(name, func(sub *testing.T) {
			previous := t
			t = sub
			f.t = sub
			defer func() { t = previous; f.t = previous }()
			fn()
		})
	}
	getRun := func(id string) agent.Run {
		t.Helper()
		h, v := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[agent.Run](t, v)
	}
	getMilestone := func(id string) project.Milestone {
		t.Helper()
		h, v := f.request("GET", "/v1/milestones/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Milestone](t, v)
	}
	getProject := func(id string) project.Project {
		t.Helper()
		h, v := f.request("GET", "/v1/projects/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Project](t, v)
	}
	createMilestone := func(target project.Project, title string, past bool) project.Milestone {
		t.Helper()
		due := time.Now().UTC().Add(24 * time.Hour)
		if past {
			due = time.Now().UTC().Add(-24 * time.Hour)
		}
		h, v := f.request("POST", "/v1/projects/"+target.ID+"/milestones", f.aliceToken, core.NewID(), project.MilestoneCreate{Title: title, OwnerID: f.alice, DueAt: due}, nil)
		expect(t, h, 201, v)
		return data[project.Milestone](t, v)
	}
	arguments := func(id string, version int64, field string, value any) map[string]any {
		return map[string]any{"id": id, "expected_version": version, "reason": "Only the exact chosen milestone or project action", field: value}
	}
	perform := func(name string, args map[string]any) agent.Run {
		t.Helper()
		r, l := start(input(name, args))
		raw, e := tool(l, "actual-stage-command", name, args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		return getRun(r.ID)
	}

	creationArgs := func(title string) map[string]any {
		return map[string]any{"id": p.ID, "expected_version": getProject(p.ID).Version, "title": title, "owner_id": f.bob, "due_at": "2027-03-05T18:00:00+08:00", "reason": "Create exactly one reviewed milestone"}
	}
	milestoneCount := func() int {
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM milestones WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&count); e != nil {
			t.Fatal(e)
		}
		return count
	}
	var created project.Milestone
	var committed agent.Run
	runTest("creation_requires_complete_exact_human_instruction", func() {
		for _, field := range []string{"title", "owner_id", "due_at"} {
			args := creationArgs("Missing field")
			delete(args, field)
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_milestone", args), nil)
			expect(t, h, 422, v)
		}
		for _, change := range []map[string]any{{"due_at": nil}, {"owner_id": nil}, {"title": " "}, {"owner_id": "not-a-user"}, {"id": foreign.ID}, {"expected_version": int64(999)}, {"state": "achieved"}} {
			args := creationArgs("Exact fields only")
			for k, v := range change {
				args[k] = v
			}
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_milestone", args), nil)
			if h.Code < 400 || h.Code >= 500 {
				t.Fatalf("invalid creation accepted or internal failure: %d, %v", h.Code, v)
			}
		}
		_, l := start(input("", nil))
		denied(l, "chat-is-not-create-authority", "create_milestone", creationArgs("No authority"), "MUTATION_INTENT_REQUIRED")
		finish(l)
		if milestoneCount() != 0 {
			t.Fatal("invalid requests created a milestone")
		}
	})
	runTest("creates_once_with_original_command_and_exact_result", func() {
		args := creationArgs("\u3000\nSecretary created milestone\t\u00a0")
		r, l := start(input("create_milestone", args))
		changed := creationArgs("Different model title")
		denied(l, "model-replaced-fields", "create_milestone", changed, "MUTATION_INTENT_MISMATCH")
		if e := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "No receipt", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); e == nil {
			t.Fatal("creation completed without business receipt")
		}
		raw, e := tool(l, "create-exact-milestone", "create_milestone", args)
		if e != nil {
			t.Fatal(e)
		}
		var result struct {
			agent.ExecutionReceipt
			Content agent.MutationView `json:"content"`
		}
		if json.Unmarshal(raw, &result) != nil || json.Unmarshal(result.Content.Result, &created) != nil {
			t.Fatal("invalid creation result")
		}
		if created.ID == p.ID || created.ProjectID != p.ID || created.Version != 1 || created.Title != "Secretary created milestone" || created.OwnerID == nil || *created.OwnerID != f.bob || created.State != "open" || result.Content.BaseVersion != p.Version || *result.Content.ResultVersion != 1 {
			t.Fatal("created result is not the exact new milestone")
		}
		if len(result.ResourceRefs) != 1 || result.ResourceRefs[0].Type != "milestone" || result.ResourceRefs[0].ID != created.ID {
			t.Fatal("creation cited parent instead of actual child")
		}
		again, e := tool(l, "create-exact-milestone", "create_milestone", args)
		if e != nil {
			t.Fatal(e)
		}
		originalJSON, _ := core.CanonicalJSON(raw)
		replayedJSON, _ := core.CanonicalJSON(again)
		if !bytes.Equal(originalJSON, replayedJSON) {
			t.Fatal("transport retry changed creation receipt")
		}
		another, e := tool(l, "new-invocation-same-authority", "create_milestone", args)
		if e != nil {
			t.Fatal(e)
		}
		var other agent.ExecutionReceipt
		_ = json.Unmarshal(another, &other)
		if other.OperationID == nil || *other.OperationID != *result.OperationID || milestoneCount() != 1 {
			t.Fatal("same authority created multiple milestones")
		}
		if _, e = tool(l, "read-created-milestone", "read_resource", map[string]any{"type": "milestone", "id": created.ID}); e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		committed = getRun(r.ID)
		if committed.Mutation == nil || committed.Mutation.Status != "committed" || getMilestone(created.ID).ID != created.ID {
			t.Fatal("committed business result missing")
		}
		var receipts, events, audit int
		if e = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1),
   (SELECT count(*) FROM outbox_events WHERE aggregate_type='milestone' AND aggregate_id=$2),
   (SELECT count(*) FROM audit_events WHERE resource_type='milestone' AND resource_id=$2)`, r.ID, created.ID).Scan(&receipts, &events, &audit); e != nil {
			t.Fatal(e)
		}
		if receipts != 1 || events != 1 || audit != 1 {
			t.Fatal("creation did not atomically persist its business evidence")
		}
	})
	task := f.createTask(p, "Task to associate explicitly", false)
	getTask := func() project.Task {
		h, v := f.request("GET", "/v1/tasks/"+task.ID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Task](t, v)
	}
	runTest("task_association_and_explicit_unlink_are_separate_authority", func() {
		if getTask().MilestoneID != nil {
			t.Fatal("creation silently linked tasks")
		}
		current := getTask()
		perform("update_task", arguments(task.ID, current.Version, "milestone_id", created.ID))
		linked := getTask()
		if linked.MilestoneID == nil || *linked.MilestoneID != created.ID {
			t.Fatal("exact task association missing")
		}
		perform("update_task", arguments(task.ID, linked.Version, "milestone_id", nil))
		if getTask().MilestoneID != nil {
			t.Fatal("explicit null failed to unlink")
		}
	})
	runTest("task_association_rejects_foreign_and_closed_milestones", func() {
		foreignM := createMilestone(foreign, "Not in this project", false)
		closed := createMilestone(p, "Closed target", false)
		exec("UPDATE milestones SET state='cancelled',version=version+1 WHERE id=$1", closed.ID)
		for _, id := range []string{foreignM.ID, closed.ID} {
			current := getTask()
			args := arguments(task.ID, current.Version, "milestone_id", id)
			_, l := start(input("update_task", args))
			if _, e := tool(l, "invalid-link", "update_task", args); e == nil {
				t.Fatal("invalid milestone link accepted")
			}
			finish(l)
			if getTask().Version != current.Version {
				t.Fatal("failed link changed task")
			}
		}
	})
	runTest("owner_must_be_a_live_member_of_the_same_project", func() {
		count := milestoneCount()
		args := creationArgs("No external owner")
		args["owner_id"] = f.admin
		_, l := start(input("create_milestone", args))
		if _, e := tool(l, "invalid-owner", "create_milestone", args); e == nil {
			t.Fatal("non-member became milestone owner")
		}
		finish(l)
		if milestoneCount() != count {
			t.Fatal("invalid owner changed business state")
		}
	})
	runTest("changed_parent_version_conflicts_without_creation", func() {
		args := creationArgs("Stale parent must not create")
		_, l := start(input("create_milestone", args))
		count := milestoneCount()
		exec("UPDATE projects SET name=name||' changed',version=version+1 WHERE id=$1", p.ID)
		if _, e := tool(l, "stale-parent", "create_milestone", args); e == nil {
			t.Fatal("stale parent accepted")
		}
		finish(l)
		if milestoneCount() != count {
			t.Fatal("stale parent created milestone")
		}
	})
	runTest("read_only_parent_prevents_creation", func() {
		exec("UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", p.ID)
		args := creationArgs("Read-only parent")
		_, l := start(input("create_milestone", args))
		count := milestoneCount()
		denied(l, "readonly-create", "create_milestone", args, "PROJECT_READ_ONLY")
		finish(l)
		if milestoneCount() != count {
			t.Fatal("read-only project created milestone")
		}
		exec("UPDATE projects SET state='draft',version=version+1 WHERE id=$1", p.ID)
	})
	runTest("cancel_before_execution_keeps_business_unchanged", func() {
		args := creationArgs("Cancelled creation")
		r, l := start(input("create_milestone", args))
		count := milestoneCount()
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "Cancel before create"}, nil)
		expect(t, h, 200, v)
		if _, e := tool(l, "cancelled-create", "create_milestone", args); e == nil {
			t.Fatal("cancelled run created milestone")
		}
		finish(l)
		if milestoneCount() != count {
			t.Fatal("cancelled run changed milestone count")
		}
	})
	runTest("committed_creation_history_is_immutable", func() {
		for _, sql := range []string{"UPDATE agent_run_mutations SET arguments='{}' WHERE run_id=$1", "DELETE FROM agent_mutation_commits WHERE run_id=$1"} {
			if _, e := f.owner.Exec(ctx, sql, committed.ID); e == nil {
				t.Fatal("creation evidence was mutable")
			}
		}
	})
	if os.Getenv("CP_BROWSER_MILESTONE_CREATION_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'milestone_browser_proof','Actionless browser MFA proof',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proof)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE id=$1", f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"milestone_creation_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "milestone_id": created.ID, "task_id": task.ID, "profile_id": profile, "completed_run_id": committed.ID})
		return
	}
	runTest("revoked_original_login_cannot_create", func() {
		args := creationArgs("Revoked login")
		r, l := start(input("create_milestone", args))
		count := milestoneCount()
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, e := tool(l, "revoked-login-create", "create_milestone", args); e == nil {
			t.Fatal("revoked login created milestone")
		}
		finish(l)
		if milestoneCount() != count {
			t.Fatal("revoked login changed business state")
		}
	})
}
