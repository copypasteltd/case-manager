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
func TestAgentTaskCreation(t *testing.T) {
	u, oldFile, _, _, _ := restorationFixture(t, 25)
	f := u.f
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
	h, v := f.request("GET", "/v1/projects/"+*oldFile.File.ProjectID, f.aliceToken, "", nil, nil)
	expect(t, h, 200, v)
	p := data[project.Project](t, v)
	foreign := f.createProject("Another project is outside this conversation", nil)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-update','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated exact update fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "准确创建任务与子任务", MemberIDs: []string{}}, nil)
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
		native.ExpectedTools = append(native.ExpectedTools, "company/submit_deliverable", "company/update_task_dependency", "company/create_task", "company/create_risk", "company/update_risk", "company/create_issue", "company/update_issue", "company/create_deliverable", "company/update_deliverable", "company/create_milestone", "company/update_project", "company/update_task", "company/transition_task", "company/transition_project", "company/update_milestone", "company/transition_milestone")
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
	get := func(id string) project.Task {
		h, v := f.request("GET", "/v1/tasks/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Task](t, v)
	}
	count := func() int {
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM tasks WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	checkpoint := func(target project.Project, title string) project.Milestone {
		h, v := f.request("POST", "/v1/projects/"+target.ID+"/milestones", f.aliceToken, core.NewID(), project.MilestoneCreate{Title: title, OwnerID: f.alice, DueAt: time.Now().UTC().Add(24 * time.Hour)}, nil)
		expect(t, h, 201, v)
		return data[project.Milestone](t, v)
	}
	parent := f.createTask(p, "上线准备：父任务", false)
	milestone := checkpoint(p, "首轮可验收交付")
	argsFor := func(title string) map[string]any {
		return map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "只创建本次完整核对的任务与关联资料", "title": title, "body": "核对真实资料、负责人和人工验收。<script>untrusted()</script>", "assignee_id": f.bob,
			"parent_id": parent.ID, "milestone_id": milestone.ID, "start_at": "2027-03-05T10:00:00.123456+08:00", "due_at": "2027-03-05T18:00:00.123457+08:00", "priority": "high", "review_required": true}
	}
	createdTask := func(raw json.RawMessage) project.Task {
		var envelope struct {
			Content agent.MutationView `json:"content"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Content.Status != "committed" || envelope.Content.OperationID == nil {
			t.Fatal("missing genuine creation receipt")
		}
		var result project.Task
		if json.Unmarshal(envelope.Content.Result, &result) != nil || !core.ValidID(result.ID) {
			t.Fatal("missing actual new task")
		}
		return result
	}
	var created project.Task
	var completedRun agent.Run
	runTest("complete_input_rejects_each_missing_field_invalid_types_hidden_fields_and_dates", func() {
		for _, field := range []string{"title", "body", "assignee_id", "parent_id", "milestone_id", "start_at", "due_at", "priority", "review_required"} {
			args := argsFor("Incomplete task")
			delete(args, field)
			h, _ := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_task", args), nil)
			if h.Code != 400 && h.Code != 422 {
				t.Fatalf("missing %s accepted: %d", field, h.Code)
			}
		}
		for _, patch := range []map[string]any{{"review_required": nil}, {"review_required": "false"}, {"state": "done"}, {"dependency_ids": []string{parent.ID}}, {"creator_id": f.bob}, {"task_id": core.NewID()}, {"title": nil}, {"body": strings.Repeat("中", 20001)}, {"assignee_id": "guessed"}, {"start_at": "2027-03-05T10:00:00.123456789+08:00"}, {"due_at": "2027-03-05T10:00:00.123455+08:00"}} {
			args := argsFor("Invalid task")
			for k, v := range patch {
				args[k] = v
			}
			h, _ := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_task", args), nil)
			if h.Code != 400 && h.Code != 422 {
				t.Fatalf("invalid complete input accepted: %d", h.Code)
			}
		}
	})
	runTest("creates_exact_subtask_with_review_and_microsecond_dates_replays_once", func() {
		before := count()
		args := argsFor("\u0085 整理首轮交付验收资料\u3000")
		r, l := start(input("create_task", args))
		raw, e := tool(l, "create-exact-task", "create_task", args)
		if e != nil {
			t.Fatal(e)
		}
		created = createdTask(raw)
		actual := get(created.ID)
		if actual.Title != "整理首轮交付验收资料" || actual.ProjectID != p.ID || actual.Version != 1 || actual.State != "todo" || actual.CreatorID != f.alice || actual.AssigneeID == nil || *actual.AssigneeID != f.bob || actual.ParentID == nil || *actual.ParentID != parent.ID || actual.MilestoneID == nil || *actual.MilestoneID != milestone.ID || !actual.ReviewRequired || actual.Priority != "high" || actual.Body != args["body"] {
			t.Fatal("created task differs from complete authorized metadata")
		}
		if actual.StartAt == nil || actual.DueAt == nil || actual.StartAt.Nanosecond() != 123456000 || actual.DueAt.Nanosecond() != 123457000 {
			t.Fatal("microsecond instants were lost")
		}
		for _, key := range []string{"create-exact-task", "replay-with-new-call-id"} {
			again, e := tool(l, key, "create_task", args)
			if e != nil {
				t.Fatal(e)
			}
			if createdTask(again).ID != actual.ID {
				t.Fatal("replay duplicated task")
			}
		}
		if _, e = tool(l, "read-actual-task", "read_resource", map[string]any{"type": "task", "id": actual.ID, "offset": 0, "max_chars": 1000}); e != nil {
			t.Fatal(e)
		}
		if _, e = tool(l, "cannot-enumerate-parent-task", "read_resource", map[string]any{"type": "task", "id": parent.ID, "offset": 0, "max_chars": 1000}); e == nil {
			t.Fatal("creation widened to another task")
		}
		complete(l, raw)
		completedRun = getRun(r.ID)
		var commits, audits, events, deps int
		e = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1), (SELECT count(*) FROM audit_events WHERE resource_type='task' AND resource_id=$2), (SELECT count(*) FROM outbox_events WHERE aggregate_type='task' AND aggregate_id=$2), (SELECT count(*) FROM task_dependencies WHERE predecessor_id=$2 OR successor_id=$2)`, r.ID, actual.ID).Scan(&commits, &audits, &events, &deps)
		if e != nil {
			t.Fatal(e)
		}
		if count() != before+1 || commits != 1 || audits != 1 || events != 1 || deps != 0 {
			t.Fatal("creation and evidence are not exactly one operation without inferred dependencies")
		}
	})
	runTest("explicit_nulls_and_false_create_independent_unassigned_task", func() {
		args := argsFor("独立任务，明确无需人工验收")
		for _, k := range []string{"assignee_id", "parent_id", "milestone_id", "start_at", "due_at"} {
			args[k] = nil
		}
		args["review_required"] = false
		args["body"] = ""
		_, l := start(input("create_task", args))
		raw, e := tool(l, "create-explicit-null-task", "create_task", args)
		if e != nil {
			t.Fatal(e)
		}
		x := createdTask(raw)
		if x.AssigneeID != nil || x.ParentID != nil || x.MilestoneID != nil || x.StartAt != nil || x.DueAt != nil || x.ReviewRequired || x.Body != "" || x.State != "todo" {
			t.Fatal("explicit nullable choices or false review were replaced")
		}
		complete(l, raw)
	})
	runTest("model_cannot_widen_any_reviewed_field_or_replace_tool", func() {
		before := count()
		args := argsFor("Employee decision")
		_, l := start(input("create_task", args))
		for _, patch := range []map[string]any{{"review_required": false}, {"assignee_id": f.alice}, {"parent_id": nil}, {"milestone_id": nil}, {"title": "Model replacement"}, {"start_at": nil}, {"reason": "Different reason"}} {
			changed := argsFor("Employee decision")
			for k, v := range patch {
				changed[k] = v
			}
			denied(l, core.NewID(), "create_task", changed, "MUTATION_INTENT_MISMATCH")
		}
		denied(l, "wrong-write-tool", "create_risk", map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "No authority", "title": "Other write", "description": "", "owner_id": f.alice, "severity": "low", "due_at": nil, "probability": nil, "mitigation": ""}, "MUTATION_INTENT_MISMATCH")
		finish(l)
		if count() != before {
			t.Fatal("changed intent created a task")
		}
	})
	runTest("ordinary_chat_and_foreign_project_do_not_authorize_creation", func() {
		before := count()
		args := argsFor("No complete authority")
		_, l := start(input("", nil))
		denied(l, "plain-chat-task", "create_task", args, "MUTATION_INTENT_REQUIRED")
		finish(l)
		args["id"], args["expected_version"] = foreign.ID, foreign.Version
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_task", args), nil)
		expect(t, h, 404, v)
		if count() != before {
			t.Fatal("ordinary chat or foreign parent created a task")
		}
	})
	runTest("changed_project_and_later_human_task_edits_stop_execution", func() {
		before := count()
		args := argsFor("Stale parent")
		_, l := start(input("create_task", args))
		exec("UPDATE projects SET version=version+1 WHERE id=$1", p.ID)
		denied(l, "changed-project-task", "create_task", args, "RUNTIME_SOURCE_CHANGED")
		finish(l)
		exec("UPDATE projects SET version=$2 WHERE id=$1", p.ID, p.Version)
		if count() != before {
			t.Fatal("changed project was silently refreshed")
		}
		args = argsFor("Committed before human edit")
		r, l := start(input("create_task", args))
		raw, e := tool(l, "commit-before-human-change", "create_task", args)
		if e != nil {
			t.Fatal(e)
		}
		x := createdTask(raw)
		h, v := f.request("PATCH", "/v1/tasks/"+x.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": x.Version, "title": "Human update must survive"}, nil)
		expect(t, h, 200, v)
		denied(l, "cannot-overwrite-human-task", "create_task", args, "RUNTIME_SOURCE_CHANGED")
		finish(l)
		if get(x.ID).Title != "Human update must survive" || getRun(r.ID).Mutation == nil {
			t.Fatal("historical task creation lost the actual human edit or receipt")
		}
	})
	runTest("invalid_members_foreign_parent_and_foreign_milestone_are_rejected", func() {
		before := count()
		args := argsFor("Invalid assignee")
		args["assignee_id"] = f.admin
		_, l := start(input("create_task", args))
		if _, e := tool(l, "invalid-task-assignee", "create_task", args); e == nil {
			t.Fatal("nonmember assignee was accepted")
		}
		finish(l)
		if count() != before {
			t.Fatal("nonmember assignee created task")
		}
		otherParent := f.createTask(foreign, "Foreign parent", false)
		otherMilestone := checkpoint(foreign, "Foreign milestone")
		for _, patch := range []map[string]any{{"parent_id": otherParent.ID}, {"milestone_id": otherMilestone.ID}} {
			before = count()
			args = argsFor("Foreign business link")
			for k, v := range patch {
				args[k] = v
			}
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_task", args), nil)
			expect(t, h, 404, v)
			if count() != before {
				t.Fatal("foreign reference created a task")
			}
		}
	})
	runTest("assigning_others_requires_assignment_permission_but_self_and_null_do_not", func() {
		exec("CREATE TABLE saved_task_creation_bindings AS SELECT * FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'task_creation_no_assignment','Controlled creator without assignment')", role, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) SELECT DISTINCT $1::uuid,$2::uuid,a.action FROM role_actions a JOIN saved_task_creation_bindings b ON b.org_id=a.org_id AND b.role_id=a.role_id WHERE a.action<>'task.assign'", f.org, role)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.alice, role)
		defer func() {
			exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
			exec("INSERT INTO role_bindings SELECT * FROM saved_task_creation_bindings")
			exec("DROP TABLE saved_task_creation_bindings")
		}()
		args := argsFor("Assigning another member is forbidden")
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_task", args), nil)
		expect(t, h, 404, v)
		for _, assignee := range []any{nil, f.alice} {
			args = argsFor("Self or unassigned task follows original permission")
			args["assignee_id"] = assignee
			_, l := start(input("create_task", args))
			raw, e := tool(l, core.NewID(), "create_task", args)
			if e != nil {
				t.Fatal(e)
			}
			complete(l, raw)
		}
	})
	runTest("closed_parent_and_milestone_at_commit_are_not_ignored", func() {
		lateParent := f.createTask(p, "Parent closes while secretary is queued", false)
		lateMilestone := checkpoint(p, "Milestone closes while queued")
		for _, field := range []string{"parent_id", "milestone_id"} {
			before := count()
			args := argsFor("Must recheck writable links")
			args["parent_id"], args["milestone_id"] = lateParent.ID, lateMilestone.ID
			_, l := start(input("create_task", args))
			if field == "parent_id" {
				exec("UPDATE tasks SET state='cancelled',version=version+1 WHERE id=$1", lateParent.ID)
			} else {
				exec("UPDATE milestones SET state='cancelled',version=version+1 WHERE id=$1", lateMilestone.ID)
			}
			code := "PARENT_TASK_READ_ONLY"
			if field == "milestone_id" {
				code = "MILESTONE_READ_ONLY"
			}
			denied(l, core.NewID(), "create_task", args, code)
			finish(l)
			if count() != before {
				t.Fatal("closed association created a task")
			}
			exec("UPDATE tasks SET state='todo',version=version+1 WHERE id=$1", lateParent.ID)
		}
	})
	runTest("receipt_failure_rolls_back_task_audit_outbox_and_exact_retry_succeeds_once", func() {
		before := count()
		args := argsFor("原子回滚后准确重试")
		r, l := start(input("create_task", args))
		exec(`CREATE FUNCTION fail_task_creation_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled task receipt failure'; END; $$; CREATE TRIGGER fail_task_creation_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fail_task_creation_receipt()`)
		if _, e := tool(l, "forced-task-receipt-failure", "create_task", args); e == nil {
			t.Fatal("receipt failure was hidden")
		}
		var commits int
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", r.ID).Scan(&commits)
		if count() != before || commits != 0 {
			t.Fatal("failed receipt left partial task or commit")
		}
		exec("DROP TRIGGER fail_task_creation_receipt ON agent_tool_receipts; DROP FUNCTION fail_task_creation_receipt()")
		raw, e := tool(l, "retry-task-after-rollback", "create_task", args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		if count() != before+1 {
			t.Fatal("retry did not create exactly once")
		}
	})
	runTest("cancellation_prevents_creation_and_instruction_is_append_only", func() {
		before := count()
		args := argsFor("Cancelled task")
		r, l := start(input("create_task", args))
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "Cancel exact task creation"}, nil)
		expect(t, h, 200, v)
		denied(l, "cancelled-task-create", "create_task", args, "RUNTIME_CANCEL_REQUESTED")
		finish(l)
		if count() != before {
			t.Fatal("cancelled run created a task")
		}
		if _, e := f.owner.Exec(ctx, "UPDATE agent_run_mutations SET arguments=arguments||'{\"review_required\":false}'::jsonb WHERE run_id=$1", completedRun.ID); e == nil {
			t.Fatal("complete task instruction was mutable")
		}
	})
	if os.Getenv("CP_BROWSER_TASK_CREATION_FIXTURE") == "1" && !t.Failed() {
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'task_creation_browser_mfa','Actionless MFA for owned task creation fixture',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.alice, proof)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"task_creation_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "task_id": created.ID, "parent_task_id": parent.ID, "milestone_id": milestone.ID, "profile_id": profile, "completed_run_id": completedRun.ID})
		return
	}
	runTest("revoked_original_login_prevents_task_creation", func() {
		before := count()
		args := argsFor("Revoked original session")
		r, l := start(input("create_task", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, e := tool(l, "revoked-session-task", "create_task", args); e == nil {
			t.Fatal("revoked original session created a task")
		}
		finish(l)
		if count() != before {
			t.Fatal("revoked session changed task count")
		}
	})
}
