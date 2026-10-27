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
func TestAgentProjectStages(t *testing.T) {
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
	p := f.createProject("Exact milestone and project stages", []string{f.bob})
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
		native.ExpectedTools = append(native.ExpectedTools, "company/update_task_dependency", "company/create_task", "company/create_risk", "company/update_risk", "company/create_issue", "company/update_issue", "company/create_deliverable", "company/update_deliverable", "company/create_milestone", "company/update_project", "company/update_task", "company/transition_task", "company/transition_project", "company/update_milestone", "company/transition_milestone")
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
	selected := createMilestone(p, "Selected milestone for exact authoring", false)
	var committed agent.Run
	var closingConversation agent.Conversation
	var closingMilestone project.Milestone
	runTest("ordinary_chat_and_other_milestone_are_not_authority", func() {
		_, l := start(input("", nil))
		for _, name := range []string{"update_milestone", "transition_milestone", "transition_project"} {
			denied(l, "ordinary-"+name, name, arguments(selected.ID, selected.Version, "action", "cancel"), "MUTATION_INTENT_REQUIRED")
		}
		denied(l, "no-free-milestone-read", "read_resource", map[string]any{"type": "milestone", "id": selected.ID}, "RESOURCE_NOT_FOUND")
		finish(l)
	})
	runTest("exact_milestone_fields_retry_read_and_real_completion", func() {
		args := arguments(selected.ID, selected.Version, "title", "\u3000\nReviewed milestone title\t\u00a0")
		args["owner_id"] = f.bob
		args["due_at"] = "2027-01-03T18:00:00+08:00"
		r, l := start(input("update_milestone", args))
		foreignM := createMilestone(foreign, "Outside the authorized project", false)
		denied(l, "other-milestone", "read_resource", map[string]any{"type": "milestone", "id": foreignM.ID}, "RESOURCE_NOT_FOUND")
		raw, e := tool(l, "read-milestone-basis", "read_resource", map[string]any{"type": "milestone", "id": selected.ID})
		if e != nil {
			t.Fatal(e)
		}
		var read agent.ExecutionReceipt
		_ = json.Unmarshal(raw, &read)
		if len(read.SourceRefs) != 1 || read.SourceRefs[0].Type != "agent_mutation" || read.SourceRefs[0].ID != r.ID {
			t.Fatal("milestone read bypassed fixed intent")
		}
		changed := arguments(selected.ID, selected.Version, "title", "Model substituted content")
		denied(l, "widen-milestone", "update_milestone", changed, "MUTATION_INTENT_MISMATCH")
		raw, e = tool(l, "write-exact-milestone", "update_milestone", args)
		if e != nil {
			t.Fatal(e)
		}
		replay, e := tool(l, "retry-exact-milestone", "update_milestone", args)
		if e != nil || !bytes.Equal(raw, replay) {
			t.Fatal("milestone retry produced another operation", e)
		}
		if _, e = tool(l, "read-new-milestone", "read_resource", map[string]any{"type": "milestone", "id": selected.ID}); e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		committed = getRun(r.ID)
		m := getMilestone(selected.ID)
		if m.Version != selected.Version+1 || m.Title != "Reviewed milestone title" || m.OwnerID == nil || *m.OwnerID != f.bob || m.DueAt == nil || !m.DueAt.Equal(time.Date(2027, 1, 3, 10, 0, 0, 0, time.UTC)) || committed.Mutation == nil || committed.Mutation.ResourceType != "milestone" || committed.Mutation.Status != "committed" {
			t.Fatal("real milestone result differs from exact request")
		}
		selected = m
	})
	runTest("milestone_owner_must_remain_an_actual_project_member", func() {
		m := createMilestone(p, "Owner eligibility", false)
		args := arguments(m.ID, m.Version, "owner_id", f.member)
		_, l := start(input("update_milestone", args))
		if _, e := tool(l, "non-member-owner", "update_milestone", args); e == nil {
			t.Fatal("non-member became milestone owner")
		}
		finish(l)
		actual := getMilestone(m.ID)
		if actual.Version != m.Version || actual.OwnerID == nil || *actual.OwnerID != f.alice {
			t.Fatal("invalid owner left a business change")
		}
	})
	runTest("unsupported_fields_nulls_archive_and_foreign_targets_are_rejected", func() {
		for _, in := range []agent.RunCreate{
			input("update_milestone", arguments(selected.ID, selected.Version, "due_at", nil)),
			input("update_milestone", arguments(selected.ID, selected.Version, "owner_id", nil)),
			input("update_milestone", arguments(selected.ID, selected.Version, "state", "achieved")),
			input("update_milestone", arguments(selected.ID, selected.Version, "project_id", foreign.ID)),
			input("transition_project", arguments(p.ID, getProject(p.ID).Version, "action", "archive")),
			input("update_milestone", arguments(core.NewID(), 1, "title", "No target")),
		} {
			h, _ := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
			if h.Code == 202 {
				t.Fatal("invalid project-stage authority admitted")
			}
		}
	})
	runTest("achieve_requires_real_completed_associations", func() {
		m := createMilestone(p, "Evidence milestone", false)
		args := arguments(m.ID, m.Version, "action", "achieve")
		_, l := start(input("transition_milestone", args))
		denied(l, "no-evidence", "transition_milestone", args, "MILESTONE_EVIDENCE_INCOMPLETE")
		finish(l)
		task := f.createTask(p, "Actual related work", false)
		h, v := f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "milestone_id": m.ID}, nil)
		expect(t, h, 200, v)
		task = data[project.Task](t, v)
		_, l = start(input("transition_milestone", args))
		denied(l, "open-task", "transition_milestone", args, "MILESTONE_EVIDENCE_INCOMPLETE")
		finish(l)
		for _, action := range []string{"start", "complete"} {
			h, v = f.request("POST", "/v1/tasks/"+task.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: task.Version, Action: action, Reason: "Real task completion, not synthetic milestone evidence"}, nil)
			expect(t, h, 200, v)
			task = data[project.Task](t, v)
		}
		r := perform("transition_milestone", args)
		if r.Mutation == nil || getMilestone(m.ID).State != "achieved" {
			t.Fatal("actual related completion did not permit milestone achievement")
		}
	})
	runTest("miss_cancel_and_reopen_follow_the_actual_state_machine", func() {
		m := createMilestone(p, "Milestone lifecycle", false)
		args := arguments(m.ID, m.Version, "action", "miss")
		_, l := start(input("transition_milestone", args))
		if _, e := tool(l, "not-yet-due", "transition_milestone", args); e == nil {
			t.Fatal("future milestone marked missed")
		}
		finish(l)
		h, v := f.request("PATCH", "/v1/milestones/"+m.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": m.Version, "due_at": time.Now().UTC().Add(-time.Hour)}, nil)
		expect(t, h, 200, v)
		m = data[project.Milestone](t, v)
		for _, step := range [][2]string{{"miss", "missed"}, {"reopen", "open"}, {"cancel", "cancelled"}, {"reopen", "open"}} {
			perform("transition_milestone", arguments(m.ID, m.Version, "action", step[0]))
			m = getMilestone(m.ID)
			if m.State != step[1] {
				t.Fatal("milestone lifecycle did not match action", step)
			}
		}
	})
	runTest("receipt_failure_rolls_back_business_commit_then_exact_retry_succeeds", func() {
		m := createMilestone(p, "Rollback milestone", false)
		args := arguments(m.ID, m.Version, "title", "Title after a real atomic commit")
		r, l := start(input("update_milestone", args))
		exec(`CREATE FUNCTION fail_stage_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled same-transaction receipt failure'; END; $$; CREATE TRIGGER fail_stage_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fail_stage_receipt()`)
		if _, e := tool(l, "forced-receipt-failure", "update_milestone", args); e == nil {
			t.Fatal("receipt failure was hidden")
		}
		after := getMilestone(m.ID)
		var count int
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", r.ID).Scan(&count)
		if after.Version != m.Version || after.Title != m.Title || count != 0 {
			t.Fatal("failed tool left a partial milestone change")
		}
		exec("DROP TRIGGER fail_stage_receipt ON agent_tool_receipts; DROP FUNCTION fail_stage_receipt()")
		raw, e := tool(l, "retry-after-rollback", "update_milestone", args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		if getMilestone(m.ID).Version != m.Version+1 {
			t.Fatal("retry did not commit once")
		}
	})
	runTest("human_edit_invalidates_the_original_target_for_reads_and_writes", func() {
		m := createMilestone(p, "Concurrent milestone", false)
		args := arguments(m.ID, m.Version, "title", "Stale model update")
		_, l := start(input("update_milestone", args))
		h, v := f.request("PATCH", "/v1/milestones/"+m.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": m.Version, "title": "Human saved a new basis"}, nil)
		expect(t, h, 200, v)
		denied(l, "stale-milestone-read", "read_resource", map[string]any{"type": "milestone", "id": m.ID}, "RUNTIME_SOURCE_CHANGED")
		denied(l, "stale-milestone-write", "update_milestone", args, "RUNTIME_SOURCE_CHANGED")
		finish(l)
	})
	runTest("cancel_before_write_leaves_the_milestone_unchanged", func() {
		m := createMilestone(p, "Cancelled stage operation", false)
		args := arguments(m.ID, m.Version, "action", "cancel")
		r, l := start(input("transition_milestone", args))
		r = getRun(r.ID)
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": r.Version, "action": "cancel", "reason": "Stop before milestone command"}, nil)
		expect(t, h, 200, v)
		denied(l, "cancel-before-command", "transition_milestone", args, "RUNTIME_CANCEL_REQUESTED")
		finish(l)
		if getMilestone(m.ID).Version != m.Version {
			t.Fatal("cancelled run mutated milestone")
		}
	})
	runTest("project_stages_keep_closeout_blockers_and_separate_archive_review", func() {
		for _, step := range [][2]string{{"activate", "active"}, {"hold", "on_hold"}, {"resume", "active"}} {
			current := getProject(p.ID)
			perform("transition_project", arguments(p.ID, current.Version, "action", step[0]))
			if getProject(p.ID).State != step[1] {
				t.Fatal("project stage did not match exact action")
			}
		}
		current := getProject(p.ID)
		args := arguments(p.ID, current.Version, "action", "close")
		_, l := start(input("transition_project", args))
		denied(l, "actual-closeout-blocker", "transition_project", args, "CLOSE_BLOCKED")
		finish(l)
		for _, step := range [][2]string{{"cancel", "cancelled"}, {"reopen", "active"}} {
			current = getProject(p.ID)
			perform("transition_project", arguments(p.ID, current.Version, "action", step[0]))
			if getProject(p.ID).State != step[1] {
				t.Fatal("project cancel/reopen skipped state or permission")
			}
		}
		previous := c
		ready := f.createProject("Project ready to enter closing", nil)
		h, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &ready.ID, Title: "Ready closeout stage", MemberIDs: []string{}}, nil)
		expect(t, h, 201, v)
		c = data[agent.Conversation](t, v)
		closingConversation = c
		closingMilestone = createMilestone(ready, "Cancelled milestone on a closing project", false)
		perform("transition_milestone", arguments(closingMilestone.ID, closingMilestone.Version, "action", "cancel"))
		closingMilestone = getMilestone(closingMilestone.ID)
		perform("transition_project", arguments(ready.ID, ready.Version, "action", "activate"))
		current = getProject(ready.ID)
		perform("transition_project", arguments(ready.ID, current.Version, "action", "close"))
		if getProject(ready.ID).State != "closing" {
			t.Fatal("ready project failed to enter closing")
		}
		c = previous
	})
	runTest("closing_project_keeps_milestone_writes_read_only", func() {
		previous := c
		c = closingConversation
		defer func() { c = previous }()
		args := arguments(closingMilestone.ID, closingMilestone.Version, "title", "Must not edit during closeout")
		_, l := start(input("update_milestone", args))
		denied(l, "closing-project-mile-write", "update_milestone", args, "PROJECT_READ_ONLY")
		finish(l)
		if getMilestone(closingMilestone.ID).Version != closingMilestone.Version {
			t.Fatal("closeout milestone changed")
		}
	})
	runTest("fixed_history_and_model_intent_never_expose_the_login_identifier", func() {
		current := getMilestone(selected.ID)
		args := arguments(current.ID, current.Version, "title", "Secret session remains private")
		r, l := start(input("update_milestone", args))
		raw, e := tool(l, "public-fixed-instruction", "read_resource", map[string]any{"type": "agent_mutation", "id": r.ID, "max_chars": 10000})
		if e != nil {
			t.Fatal(e)
		}
		var session string
		if e = f.owner.QueryRow(ctx, "SELECT session_id::text FROM agent_run_mutations WHERE run_id=$1", r.ID).Scan(&session); e != nil {
			t.Fatal(e)
		}
		public, _ := json.Marshal(getRun(r.ID))
		if strings.Contains(string(raw), "session_id") || strings.Contains(string(raw), session) || strings.Contains(string(public), session) {
			t.Fatal("originating login leaked into public authoring context")
		}
		finish(l)
		for _, sql := range []string{"UPDATE agent_run_mutations SET arguments='{}' WHERE run_id=$1", "DELETE FROM agent_run_mutations WHERE run_id=$1", "UPDATE agent_mutation_commits SET result_version=result_version+1 WHERE run_id=$1"} {
			if _, e = f.owner.Exec(ctx, sql, committed.ID); e == nil {
				t.Fatal("immutable stage history was rewritten")
			}
		}
	})
	runTest("policy_withdrawal_stops_a_previously_issued_stage_command", func() {
		m := getMilestone(selected.ID)
		args := arguments(m.ID, m.Version, "title", "Must not write after policy withdrawal")
		_, l := start(input("update_milestone", args))
		defer func() {
			exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
			exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		}()
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		if _, e := tool(l, "withdrawn-stage-policy", "update_milestone", args); e == nil {
			t.Fatal("stage write bypassed policy withdrawal")
		}
		finish(l)
		var version int64
		_ = f.owner.QueryRow(ctx, "SELECT version FROM milestones WHERE id=$1", m.ID).Scan(&version)
		if version != m.Version {
			t.Fatal("withdrawn policy changed milestone")
		}
	})
	if os.Getenv("CP_BROWSER_PROJECT_STAGE_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'stage_browser_proof','Actionless browser MFA proof',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proof)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE id=$1", f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"project_stage_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "milestone_id": selected.ID, "profile_id": profile, "completed_run_id": committed.ID})
		return
	}
	runTest("revoked_original_login_stops_all_following_tools", func() {
		current := getMilestone(selected.ID)
		args := arguments(current.ID, current.Version, "title", "Must not write after logout")
		r, l := start(input("update_milestone", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, e := tool(l, "revoked-login-write", "update_milestone", args); e == nil {
			t.Fatal("write survived original login revocation")
		}
		if _, e := tool(l, "revoked-login-inspect", "inspect_operation", map[string]any{"run_id": r.ID}); e == nil {
			t.Fatal("read survived original login revocation")
		}
		finish(l)
		var title string
		_ = f.owner.QueryRow(ctx, "SELECT title FROM milestones WHERE id=$1", current.ID).Scan(&title)
		if title != current.Title {
			t.Fatal("revoked original login changed business data")
		}
	})
}
