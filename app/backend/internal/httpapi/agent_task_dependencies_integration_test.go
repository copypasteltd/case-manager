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
func TestAgentTaskDependencies(t *testing.T) {
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
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "准确调整前置依赖", MemberIDs: []string{}}, nil)
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

	argsFor := func(task, predecessor project.Task, action string) map[string]any {
		return map[string]any{"id": task.ID, "expected_version": task.Version, "predecessor_id": predecessor.ID, "expected_predecessor_version": predecessor.Version, "action": action, "reason": "员工核对两个任务后调整这一条依赖"}
	}
	pair := func() (project.Task, project.Task) {
		return f.createTask(p, "交付发布：后续任务", false), f.createTask(p, "资料核对：前置任务", false)
	}
	present := func(task, predecessor string) bool {
		var yes bool
		if e := f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM task_dependencies WHERE org_id=$1 AND successor_id=$2 AND predecessor_id=$3)", f.org, task, predecessor).Scan(&yes); e != nil {
			t.Fatal(e)
		}
		return yes
	}
	requireNoChange := func(task project.Task, predecessor string, edge bool) {
		if get(task.ID).Version != task.Version || present(task.ID, predecessor) != edge {
			t.Fatal("denied dependency changed the task or edge")
		}
	}
	rejectAdmission := func(args map[string]any) {
		t.Helper()
		h, _ := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("update_task_dependency", args), nil)
		if h.Code == 202 {
			t.Fatal("invalid dependency was admitted")
		}
	}
	runTest("all_six_fields_are_required_and_no_extra_or_null_authority_is_accepted", func() {
		task, pred := pair()
		for _, key := range []string{"id", "expected_version", "predecessor_id", "expected_predecessor_version", "action", "reason"} {
			args := argsFor(task, pred, "add")
			delete(args, key)
			rejectAdmission(args)
			args = argsFor(task, pred, "add")
			args[key] = nil
			rejectAdmission(args)
		}
		for key, value := range map[string]any{"action": "replace", "dependency_ids": []string{pred.ID}, "state": "done", "expected_predecessor_version": 0, "reason": "  "} {
			args := argsFor(task, pred, "add")
			args[key] = value
			rejectAdmission(args)
		}
		requireNoChange(task, pred.ID, false)
	})
	runTest("self_foreign_stale_versions_and_absent_removal_are_rejected", func() {
		task, pred := pair()
		foreignTask := f.createTask(foreign, "不可借用其他项目任务", false)
		for _, other := range []project.Task{task, foreignTask} {
			rejectAdmission(argsFor(task, other, "add"))
		}
		for _, key := range []string{"expected_version", "expected_predecessor_version"} {
			args := argsFor(task, pred, "add")
			args[key] = int64(2)
			rejectAdmission(args)
		}
		rejectAdmission(argsFor(task, pred, "remove"))
		requireNoChange(task, pred.ID, false)
	})
	var completedRun, removedRun agent.Run
	var demoTask, demoPredecessor project.Task
	runTest("add_is_exact_atomic_and_replay_keeps_one_operation_and_task_increment", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		in := input("update_task_dependency", args)
		r, l := start(in)
		raw, e := tool(l, "add-exact-dependency", "update_task_dependency", args)
		if e != nil {
			t.Fatal(e)
		}
		var receipt agent.ExecutionReceipt
		_ = json.Unmarshal(raw, &receipt)
		for _, key := range []string{"add-exact-dependency", "new-tool-key-same-human-intent"} {
			again, e := tool(l, key, "update_task_dependency", args)
			if e != nil {
				t.Fatal(e)
			}
			var v agent.ExecutionReceipt
			_ = json.Unmarshal(again, &v)
			if v.OperationID == nil || receipt.OperationID == nil || *v.OperationID != *receipt.OperationID {
				t.Fatal("replay issued another operation")
			}
		}
		if get(task.ID).Version != task.Version+1 || get(pred.ID).Version != pred.Version || !present(task.ID, pred.ID) {
			t.Fatal("exact dependency was not applied once")
		}
		complete(l, raw)
		got := getRun(r.ID)
		var actual project.DependencyReceipt
		_ = json.Unmarshal(got.Mutation.Result, &actual)
		if actual.ID != task.ID || actual.PredecessorID != pred.ID || actual.PredecessorVersion != pred.Version || actual.DependencyAction != "add" || !actual.DependencyPresent || actual.Version != task.Version+1 {
			t.Fatal("public receipt omitted actual relationship")
		}
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, h, 202, v)
		if data[agent.Run](t, v).ID != r.ID {
			t.Fatal("lost acknowledgment created another run")
		}
		rejectAdmission(argsFor(get(task.ID), pred, "add"))
		completedRun, demoTask, demoPredecessor = got, get(task.ID), pred
	})
	runTest("original_task_start_rule_observes_the_new_dependency", func() {
		task := demoTask
		h, v := f.request("POST", "/v1/tasks/"+task.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: task.Version, Action: "start", Reason: "尚未完成前置任务"}, nil)
		if h.Code != 422 || v.Error.Code != "DEPENDENCY_BLOCKED" {
			t.Fatalf("expected dependency blocker, got %d: %s", h.Code, v)
		}
		requireNoChange(task, demoPredecessor.ID, true)
	})
	runTest("remove_returns_the_actual_absent_edge_without_changing_predecessor", func() {
		task, pred := pair()
		h, v := f.request("POST", "/v1/tasks/"+task.ID+"/dependencies", f.aliceToken, core.NewID(), project.Dependency{ExpectedVersion: task.Version, PredecessorID: pred.ID, Action: "add"}, nil)
		expect(t, h, 200, v)
		task = data[project.Task](t, v)
		args := argsFor(task, pred, "remove")
		r, l := start(input("update_task_dependency", args))
		raw, e := tool(l, "remove-one-edge", "update_task_dependency", args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		var actual project.DependencyReceipt
		_ = json.Unmarshal(getRun(r.ID).Mutation.Result, &actual)
		if actual.DependencyPresent || actual.DependencyAction != "remove" || present(task.ID, pred.ID) || get(pred.ID).Version != pred.Version {
			t.Fatal("removal receipt or predecessor changed")
		}
		removedRun = getRun(r.ID)
		rejectAdmission(argsFor(get(task.ID), pred, "remove"))
	})
	runTest("plain_chat_grants_no_dependency_authority_and_arguments_cannot_be_widened", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		_, l := start(input("", nil))
		denied(l, "chat-is-not-authority", "update_task_dependency", args, "MUTATION_INTENT_REQUIRED")
		finish(l)
		_, l = start(input("update_task_dependency", args))
		for key, value := range map[string]any{"action": "remove", "predecessor_id": demoPredecessor.ID, "expected_predecessor_version": int64(2), "reason": "模型自行改写原因"} {
			changed := argsFor(task, pred, "add")
			changed[key] = value
			denied(l, core.NewID(), "update_task_dependency", changed, "MUTATION_INTENT_MISMATCH")
		}
		finish(l)
		requireNoChange(task, pred.ID, false)
	})
	runTest("cycle_and_in_progress_blockers_use_the_original_dependency_command", func() {
		task, pred := pair()
		h, v := f.request("POST", "/v1/tasks/"+pred.ID+"/dependencies", f.aliceToken, core.NewID(), project.Dependency{ExpectedVersion: pred.Version, PredecessorID: task.ID, Action: "add"}, nil)
		expect(t, h, 200, v)
		pred = data[project.Task](t, v)
		args := argsFor(task, pred, "add")
		_, l := start(input("update_task_dependency", args))
		denied(l, "no-cycle", "update_task_dependency", args, "DEPENDENCY_CYCLE")
		finish(l)
		requireNoChange(task, pred.ID, false)
		task, pred = pair()
		h, v = f.request("POST", "/v1/tasks/"+task.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: task.Version, Action: "start", Reason: "开始后续任务"}, nil)
		expect(t, h, 200, v)
		task = data[project.Task](t, v)
		args = argsFor(task, pred, "add")
		_, l = start(input("update_task_dependency", args))
		denied(l, "no-new-unfinished-predecessor", "update_task_dependency", args, "DEPENDENCY_BLOCKED")
		finish(l)
		requireNoChange(task, pred.ID, false)
	})
	for _, which := range []string{"successor", "predecessor"} {
		runTest("changed_"+which+"_after_queue_requires_new_human_intent", func() {
			task, pred := pair()
			args := argsFor(task, pred, "add")
			_, l := start(input("update_task_dependency", args))
			changed := pred
			if which == "successor" {
				changed = task
			}
			h, v := f.request("PATCH", "/v1/tasks/"+changed.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": changed.Version, "title": "员工已另行修改任务"}, nil)
			expect(t, h, 200, v)
			denied(l, "source-version-changed", "update_task_dependency", args, "RUNTIME_SOURCE_CHANGED")
			finish(l)
			if present(task.ID, pred.ID) {
				t.Fatal("stale intent added an edge")
			}
		})
	}
	runTest("historical_intent_stays_readable_after_later_predecessor_edits", func() {
		h, v := f.request("PATCH", "/v1/tasks/"+demoPredecessor.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": demoPredecessor.Version, "body": "历史完成后员工更新说明"}, nil)
		expect(t, h, 200, v)
		current := getRun(completedRun.ID)
		if current.Mutation == nil || current.Mutation.OperationID == nil || current.Mutation.Before == nil {
			t.Fatal("historical intent was lost")
		}
		demoPredecessor = get(demoPredecessor.ID)
	})
	runTest("relationship_change_between_queue_and_execution_is_not_rebased", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		_, l := start(input("update_task_dependency", args))
		h, v := f.request("POST", "/v1/tasks/"+task.ID+"/dependencies", f.aliceToken, core.NewID(), project.Dependency{ExpectedVersion: task.Version, PredecessorID: pred.ID, Action: "add"}, nil)
		expect(t, h, 200, v)
		denied(l, "human-already-changed-edge", "update_task_dependency", args, "RUNTIME_SOURCE_CHANGED")
		finish(l)
		if get(task.ID).Version != task.Version+1 {
			t.Fatal("stale delegation changed another version")
		}
	})
	runTest("business_receipt_failure_rolls_back_edge_and_version_then_retries_once", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		r, l := start(input("update_task_dependency", args))
		exec(`CREATE FUNCTION fail_dependency_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled dependency receipt failure'; END; $$; CREATE TRIGGER fail_dependency_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fail_dependency_receipt()`)
		if _, e := tool(l, "controlled-receipt-failure", "update_task_dependency", args); e == nil {
			t.Fatal("receipt failure was hidden")
		}
		requireNoChange(task, pred.ID, false)
		var n int
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", r.ID).Scan(&n)
		if n != 0 {
			t.Fatal("partial dependency commit survived")
		}
		exec("DROP TRIGGER fail_dependency_receipt ON agent_tool_receipts; DROP FUNCTION fail_dependency_receipt()")
		raw, e := tool(l, "retry-after-rollback", "update_task_dependency", args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
	})
	for _, field := range []string{"dependency_present", "predecessor_id", "predecessor_version", "dependency_action", "body"} {
		runTest("database_rejects_forged_"+field+"_receipt", func() {
			task, pred := pair()
			args := argsFor(task, pred, "add")
			_, l := start(input("update_task_dependency", args))
			patch := map[string]any{"dependency_present": false, "predecessor_id": core.NewID(), "predecessor_version": int64(999), "dependency_action": "remove", "body": "未经员工授权的内容"}
			raw, _ := json.Marshal(map[string]any{field: patch[field]})
			exec(`CREATE FUNCTION forge_dependency_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.result_data:=NEW.result_data||'` + string(raw) + `'::jsonb; RETURN NEW; END; $$; CREATE TRIGGER aaa_forge_dependency_receipt BEFORE INSERT ON agent_mutation_commits FOR EACH ROW EXECUTE FUNCTION forge_dependency_receipt()`)
			if _, e := tool(l, "forged-database-receipt", "update_task_dependency", args); e == nil {
				t.Fatal("forged actual relationship receipt was accepted")
			}
			requireNoChange(task, pred.ID, false)
			exec("DROP TRIGGER aaa_forge_dependency_receipt ON agent_mutation_commits; DROP FUNCTION forge_dependency_receipt()")
			result, e := tool(l, "retry-with-actual-receipt", "update_task_dependency", args)
			if e != nil {
				t.Fatal(e)
			}
			complete(l, result)
		})
	}
	runTest("cancellation_blocks_writes_and_fixed_dependency_intent_is_append_only", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		r, l := start(input("update_task_dependency", args))
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "取消本次依赖更新"}, nil)
		expect(t, h, 200, v)
		denied(l, "cancelled-dependency", "update_task_dependency", args, "RUNTIME_CANCEL_REQUESTED")
		finish(l)
		requireNoChange(task, pred.ID, false)
		if _, e := f.owner.Exec(ctx, "UPDATE agent_run_mutations SET arguments=arguments||'{\"action\":\"remove\"}'::jsonb WHERE run_id=$1", completedRun.ID); e == nil {
			t.Fatal("dependency instruction was mutable")
		}
	})

	runTest("completed_cancelled_tasks_and_readonly_projects_reject_new_intent", func() {
		for _, state := range []string{"done", "cancelled"} {
			task, pred := pair()
			exec("UPDATE tasks SET state=$2,completed_at=CASE WHEN $2='done' THEN now() ELSE NULL END,version=version+1 WHERE id=$1", task.ID, state)
			rejectAdmission(argsFor(get(task.ID), pred, "add"))
		}
		task, pred := pair()
		exec("UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", p.ID)
		rejectAdmission(argsFor(task, pred, "add"))
		exec("UPDATE projects SET state='draft',version=version+1 WHERE id=$1", p.ID)
	})
	runTest("current_task_update_permission_is_required_at_execution", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		_, l := start(input("update_task_dependency", args))
		exec("CREATE TABLE saved_dependency_actor_bindings AS SELECT * FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'dependency_no_update','Controlled reader without task update')", role, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) SELECT DISTINCT $1::uuid,$2::uuid,a.action FROM role_actions a JOIN saved_dependency_actor_bindings b ON b.org_id=a.org_id AND b.role_id=a.role_id WHERE a.action<>'task.update'", f.org, role)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.alice, role)
		if _, e := tool(l, "write-permission-revoked", "update_task_dependency", args); e == nil {
			t.Fatal("revoked task.update still wrote")
		}
		finish(l)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		exec("INSERT INTO role_bindings SELECT * FROM saved_dependency_actor_bindings")
		exec("DROP TABLE saved_dependency_actor_bindings")
		requireNoChange(task, pred.ID, false)
	})
	runTest("every_shared_recipient_must_keep_predecessor_read_permission", func() {
		task, pred := pair()
		saved := c
		h, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "全体成员共同可见的依赖委托", MemberIDs: []string{f.bob}}, nil)
		expect(t, h, 201, v)
		c = data[agent.Conversation](t, v)
		args := argsFor(task, pred, "add")
		_, l := start(input("update_task_dependency", args))
		exec("CREATE TABLE saved_dependency_audience_bindings AS SELECT * FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'dependency_audience_reader','Only selected successor and project')", role, f.org)
		for _, action := range []string{"project.read", "task.read"} {
			exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action)
		}
		for _, id := range []string{p.ID, task.ID} {
			exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) VALUES($1,$2,$3,$4,'resource',$5,'project_restricted')", core.NewID(), f.org, f.bob, role, id)
		}
		if _, e := tool(l, "audience-predecessor-no-longer-readable", "update_task_dependency", args); e == nil {
			t.Fatal("shared recipient's lost predecessor permission was ignored")
		}
		finish(l)
		rejectAdmission(args)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		exec("INSERT INTO role_bindings SELECT * FROM saved_dependency_audience_bindings")
		exec("DROP TABLE saved_dependency_audience_bindings")
		c = saved
		requireNoChange(task, pred.ID, false)
	})
	if os.Getenv("CP_BROWSER_TASK_DEPENDENCIES_FIXTURE") == "1" && !t.Failed() {
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'dependency_browser_mfa','Actionless MFA for owned dependency fixture',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.alice, proof)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		reviewTask, reviewPred := pair()
		exec("UPDATE tasks SET title=CASE WHEN id=$1 THEN '发布首轮交付' ELSE '核对交付资料' END WHERE id IN ($1,$2)", reviewTask.ID, reviewPred.ID)
		holdBrowserBusinessFixture(t, f, map[string]any{"task_dependencies_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "task_id": reviewTask.ID, "predecessor_id": reviewPred.ID, "profile_id": profile, "completed_run_id": completedRun.ID, "removed_run_id": removedRun.ID})
		return
	}
	runTest("revoked_original_login_prevents_dependency_writes", func() {
		task, pred := pair()
		args := argsFor(task, pred, "add")
		_, l := start(input("update_task_dependency", args))
		exec("UPDATE sessions SET revoked_at=now() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		if _, e := tool(l, "revoked-login-dependency", "update_task_dependency", args); e == nil {
			t.Fatal("revoked human session granted a write")
		}
		finish(l)
		// Direct DB check, since the original HTTP login was intentionally revoked.
		var version int64
		_ = f.owner.QueryRow(ctx, "SELECT version FROM tasks WHERE id=$1", task.ID).Scan(&version)
		if version != task.Version || present(task.ID, pred.ID) {
			t.Fatal("revoked delegation changed relationship")
		}
	})
}
