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
func TestAgentProjectConcerns(t *testing.T) {
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
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact risk and issue conversation", MemberIDs: []string{}}, nil)
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

	get := func(kind, id string) project.Concern {
		t.Helper()
		h, v := f.request("GET", "/v1/"+kind+"s/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Concern](t, v)
	}
	count := func(kind string) int {
		t.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM "+kind+"s WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	argsFor := func(kind, title string) map[string]any {
		args := map[string]any{"id": p.ID, "expected_version": p.Version, "reason": "Only the complete reviewed concern is authorized.", "title": title, "description": "Original evidence 中文. <script>untrusted()</script>", "owner_id": f.alice, "severity": "high", "due_at": "2027-03-05T18:00:00.123456+08:00"}
		if kind == "risk" {
			args["probability"] = nil
			args["mitigation"] = "Review the actual supplier."
		} else {
			args["resolution"] = ""
		}
		return args
	}
	argsExisting := func(kind string, r project.Concern) map[string]any {
		args := argsFor(kind, r.Title)
		args["id"], args["expected_version"] = r.ID, r.Version
		args["description"], args["owner_id"], args["severity"], args["due_at"] = r.Description, r.OwnerID, r.Severity, r.DueAt
		if kind == "risk" {
			args["probability"] = r.Probability
			args["mitigation"] = r.Mitigation
		} else {
			args["resolution"] = r.Resolution
		}
		return args
	}
	committedConcern := func(raw json.RawMessage) (project.Concern, agent.ExecutionReceipt) {
		t.Helper()
		var receipt agent.ExecutionReceipt
		if e := json.Unmarshal(raw, &receipt); e != nil {
			t.Fatal(e)
		}
		var payload struct {
			Content agent.MutationView `json:"content"`
		}
		if e := json.Unmarshal(raw, &payload); e != nil {
			t.Fatal(e)
		}
		value := payload.Content
		var concern project.Concern
		if e := json.Unmarshal(value.Result, &concern); e != nil {
			t.Fatal(e)
		}
		if value.Status != "committed" || value.OperationID == nil || receipt.OperationID == nil || *value.OperationID != *receipt.OperationID {
			t.Fatal("unconfirmed concern commit")
		}
		return concern, receipt
	}
	apply := func(toolName string, args map[string]any) (project.Concern, agent.Run) {
		t.Helper()
		r, l := start(input(toolName, args))
		raw, e := tool(l, core.NewID(), toolName, args)
		if e != nil {
			t.Fatal(e)
		}
		item, _ := committedConcern(raw)
		complete(l, raw)
		return item, getRun(r.ID)
	}
	var risk, issue project.Concern
	var createdRun agent.Run
	runTest("complete_inputs_reject_omission_cross_kind_fields_state_null_and_unrepresentable_time", func() {
		for _, toolName := range []string{"create_risk", "update_risk", "create_issue", "update_issue"} {
			kind := "risk"
			if strings.HasSuffix(toolName, "issue") {
				kind = "issue"
			}
			required := []string{"title", "description", "owner_id", "severity", "due_at"}
			if kind == "risk" {
				required = append(required, "probability", "mitigation")
			} else {
				required = append(required, "resolution")
			}
			for _, field := range required {
				args := argsFor(kind, "Incomplete")
				delete(args, field)
				h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(toolName, args), nil)
				if h.Code != 400 && h.Code != 422 {
					t.Fatalf("missing %s accepted: %d %v", field, h.Code, v.Error)
				}
			}
		}
		for _, change := range []map[string]any{{"state": "closed"}, {"title": nil}, {"owner_id": foreign.OwnerID[:8] + "-INVALID"}, {"severity": "urgent"}, {"due_at": "2027-03-05T10:00:00.123456789Z"}, {"resolution": "Wrong resource field"}, {"issue_id": core.NewID()}} {
			args := argsFor("risk", "Invalid")
			for k, v := range change {
				args[k] = v
			}
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_risk", args), nil)
			if h.Code != 400 && h.Code != 422 {
				t.Fatalf("invalid concern accepted: %d %v", h.Code, v.Error)
			}
		}
	})
	runTest("risk_creation_has_real_parent_child_receipts_and_two_exact_retries", func() {
		before := count("risk")
		args := argsFor("risk", "\u0085 新项目风险\u3000")
		r, l := start(input("create_risk", args))
		raw, e := tool(l, "create-risk-once", "create_risk", args)
		if e != nil {
			t.Fatal(e)
		}
		var receipt agent.ExecutionReceipt
		risk, receipt = committedConcern(raw)
		if risk.ID == p.ID || risk.Version != 1 || risk.State != "open" || risk.Title != "新项目风险" || risk.CreatorID != f.alice || risk.Probability != nil || risk.DueAt == nil || risk.DueAt.Nanosecond() != 123456000 || len(receipt.ResourceRefs) != 1 || receipt.ResourceRefs[0].Type != "risk" || receipt.ResourceRefs[0].ID != risk.ID {
			t.Fatal("incorrect risk creation result")
		}
		for _, key := range []string{"create-risk-once", "create-risk-second-key"} {
			again, e := tool(l, key, "create_risk", args)
			if e != nil {
				t.Fatal(e)
			}
			copy, ack := committedConcern(again)
			if copy.ID != risk.ID || *ack.OperationID != *receipt.OperationID {
				t.Fatal("retry created a different concern")
			}
		}
		if count("risk") != before+1 {
			t.Fatal("risk creation duplicated")
		}
		var commits, audits, events, operations int
		for _, pair := range []struct {
			sql string
			out *int
		}{{"SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", &commits}, {"SELECT count(*) FROM audit_events WHERE resource_type='risk' AND resource_id=$1", &audits}, {"SELECT count(*) FROM outbox_events WHERE aggregate_id=$1", &events}, {"SELECT count(*) FROM idempotency_records WHERE idempotency_key='agent-mutation:'||$1::text", &operations}} {
			id := r.ID
			if pair.out == &events || pair.out == &audits {
				id = risk.ID
			}
			if e = f.owner.QueryRow(ctx, pair.sql, id).Scan(pair.out); e != nil {
				t.Fatal(e)
			}
		}
		if commits != 1 || audits < 1 || events != 1 || operations != 1 {
			t.Fatal("missing atomic business evidence", commits, audits, events, operations)
		}
		if _, e = tool(l, "read-created-risk", "read_resource", map[string]any{"type": "risk", "id": risk.ID}); e != nil {
			t.Fatal(e)
		}
		denied(l, "read-unrelated-risk", "read_resource", map[string]any{"type": "risk", "id": core.NewID()}, "RESOURCE_NOT_FOUND")
		complete(l, raw)
		createdRun = getRun(r.ID)
	})
	runTest("issue_creation_keeps_an_explicitly_empty_date_and_does_not_resolve_it", func() {
		args := argsFor("issue", "Actual project issue")
		args["due_at"] = nil
		args["resolution"] = "An initial note, not an approval to resolve."
		issue, _ = apply("create_issue", args)
		if issue.State != "open" || issue.Version != 1 || issue.DueAt != nil || issue.Resolution != args["resolution"] {
			t.Fatal("creation inferred an unapproved state")
		}
	})
	runTest("full_risk_update_clears_date_probability_and_keeps_omitted_state", func() {
		args := argsExisting("risk", risk)
		args["title"] = " Updated reviewed risk "
		args["due_at"], args["probability"] = nil, nil
		args["mitigation"] = "New mitigation"
		risk, _ = apply("update_risk", args)
		if risk.Version != 2 || risk.State != "open" || risk.DueAt != nil || risk.Probability != nil || risk.Title != "Updated reviewed risk" {
			t.Fatal("complete update lost clear/preserve semantics")
		}
	})
	runTest("issue_resolution_closure_and_reopening_follow_the_original_state_machine", func() {
		args := argsExisting("issue", issue)
		args["state"] = "resolved"
		args["resolution"] = ""
		r, l := start(input("update_issue", args))
		if _, e := tool(l, "empty-resolution", "update_issue", args); e == nil {
			t.Fatal("empty resolution was accepted")
		}
		finish(l)
		_ = r
		for _, state := range []string{"resolved", "closed", "open"} {
			args = argsExisting("issue", issue)
			args["state"], args["resolution"] = state, "Actual findings and resolution evidence."
			issue, _ = apply("update_issue", args)
			if issue.State != state {
				t.Fatal("wrong issue state")
			}
		}
	})
	runTest("normal_public_patches_distinguish_omission_from_explicit_null", func() {
		due := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
		prob := "medium"
		h, v := f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": risk.Version, "due_at": due, "probability": prob}, nil)
		expect(t, h, 200, v)
		risk = data[project.Concern](t, v)
		h, v = f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": risk.Version, "title": risk.Title}, nil)
		expect(t, h, 200, v)
		risk = data[project.Concern](t, v)
		if risk.DueAt == nil || !risk.DueAt.Equal(due) || risk.Probability == nil || *risk.Probability != prob {
			t.Fatal("omitted patch fields were cleared")
		}
		h, v = f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": risk.Version, "due_at": nil, "probability": nil}, nil)
		expect(t, h, 200, v)
		risk = data[project.Concern](t, v)
		if risk.DueAt != nil || risk.Probability != nil {
			t.Fatal("explicit clearing did not persist")
		}
	})
	runTest("foreign_project_and_changed_parent_are_not_substituted", func() {
		args := argsFor("risk", "Wrong parent")
		args["id"], args["expected_version"] = foreign.ID, foreign.Version
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_risk", args), nil)
		expect(t, h, 404, v)
		before := count("risk")
		args = argsFor("risk", "Stale parent")
		_, l := start(input("create_risk", args))
		exec("UPDATE projects SET version=version+1 WHERE id=$1", p.ID)
		if _, e := tool(l, "changed-parent", "create_risk", args); e == nil {
			t.Fatal("stale parent was silently refreshed")
		}
		finish(l)
		if count("risk") != before {
			t.Fatal("stale parent created a concern")
		}
		exec("UPDATE projects SET version=$2 WHERE id=$1", p.ID, p.Version)
	})
	runTest("exact_arguments_and_changed_concern_are_not_replaced", func() {
		args := argsExisting("issue", issue)
		_, l := start(input("update_issue", args))
		changed := argsExisting("issue", issue)
		changed["reason"] = "Unapproved replacement reason"
		denied(l, "different-args", "update_issue", changed, "MUTATION_INTENT_MISMATCH")
		h, v := f.request("PATCH", "/v1/issues/"+issue.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": issue.Version, "description": "An actual concurrent edit."}, nil)
		expect(t, h, 200, v)
		issue = data[project.Concern](t, v)
		if _, e := tool(l, "stale-concern", "update_issue", args); e == nil {
			t.Fatal("stale concern version was replaced")
		}
		finish(l)
	})
	runTest("receipt_failure_rolls_back_risk_and_new_issue_then_exact_retry_commits_once", func() {
		args := argsExisting("risk", risk)
		args["state"] = "realized"
		before := count("issue")
		r, l := start(input("update_risk", args))
		exec(`CREATE FUNCTION fail_concern_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled concern receipt failure'; END; $$; CREATE TRIGGER fail_concern_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fail_concern_receipt()`)
		if _, e := tool(l, "realization-rollback", "update_risk", args); e == nil {
			t.Fatal("missing receipt was reported as success")
		}
		exec("DROP TRIGGER fail_concern_receipt ON agent_tool_receipts; DROP FUNCTION fail_concern_receipt()")
		current := get("risk", risk.ID)
		if current.Version != risk.Version || current.State != risk.State || current.IssueID != nil || count("issue") != before {
			t.Fatal("partial realization escaped rollback")
		}
		var commits, operations int
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", r.ID).Scan(&commits)
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE idempotency_key='agent-mutation:'||$1::text", r.ID).Scan(&operations)
		if commits != 0 || operations != 0 {
			t.Fatal("rollback left operation evidence")
		}
		raw, e := tool(l, "realization-after-rollback", "update_risk", args)
		if e != nil {
			t.Fatal(e)
		}
		risk, _ = committedConcern(raw)
		if risk.IssueID == nil || count("issue") != before+1 {
			t.Fatal("risk did not create exactly one issue")
		}
		linked := get("issue", *risk.IssueID)
		if linked.State != "open" || linked.Version != 1 || linked.Title != risk.Title || linked.Description != risk.Description || linked.OwnerID != risk.OwnerID || linked.Severity != risk.Severity {
			t.Fatal("realized issue did not inherit exact authorized risk metadata")
		}
		var ack agent.ExecutionReceipt
		_ = json.Unmarshal(raw, &ack)
		if len(ack.ResourceRefs) != 2 || ack.ResourceRefs[1].Type != "issue" || ack.ResourceRefs[1].ID != linked.ID {
			t.Fatal("new issue result reference missing")
		}
		if _, e = tool(l, "read-linked-issue", "read_resource", map[string]any{"type": "issue", "id": linked.ID}); e != nil {
			t.Fatal(e)
		}
		if _, e = tool(l, "realization-replay", "update_risk", args); e != nil {
			t.Fatal(e)
		}
		if count("issue") != before+1 {
			t.Fatal("realization replay duplicated an issue")
		}
		complete(l, raw)
	})
	runTest("new_linked_issue_changes_stop_later_model_reads_without_rewriting_history", func() {
		args := argsFor("risk", "Risk with later human issue work")
		fresh, _ := apply("create_risk", args)
		args = argsExisting("risk", fresh)
		args["state"] = "realized"
		r, l := start(input("update_risk", args))
		raw, e := tool(l, "realize-then-edit", "update_risk", args)
		if e != nil {
			t.Fatal(e)
		}
		fresh, _ = committedConcern(raw)
		child := get("issue", *fresh.IssueID)
		h, v := f.request("PATCH", "/v1/issues/"+child.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": child.Version, "description": "Human follow-up changed the new issue."}, nil)
		expect(t, h, 200, v)
		if _, e = tool(l, "read-changed-new-issue", "read_resource", map[string]any{"type": "issue", "id": child.ID}); e == nil {
			t.Fatal("follow-up child read ignored its changed version")
		}
		finish(l)
		history := getRun(r.ID)
		if history.Mutation == nil || history.Mutation.Status != "committed" {
			t.Fatal("historical committed fact was lost")
		}
	})
	runTest("invalid_member_and_read_only_project_create_no_concerns", func() {
		before := count("risk")
		args := argsFor("risk", "Invalid member")
		args["owner_id"] = core.NewID()
		_, l := start(input("create_risk", args))
		if _, e := tool(l, "invalid-owner", "create_risk", args); e == nil {
			t.Fatal("invalid member accepted")
		}
		finish(l)
		args = argsFor("risk", "Cancelled parent")
		_, l = start(input("create_risk", args))
		exec("UPDATE projects SET state='cancelled' WHERE id=$1", p.ID)
		if _, e := tool(l, "readonly-parent", "create_risk", args); e == nil {
			t.Fatal("read-only project accepted")
		}
		finish(l)
		exec("UPDATE projects SET state=$2 WHERE id=$1", p.ID, p.State)
		if count("risk") != before {
			t.Fatal("invalid intent created a concern")
		}
	})
	runTest("risk_realization_requires_issue_creation_authority_at_preparation", func() {
		args := argsFor("risk", "Risk for a risk-only actor")
		fresh, _ := apply("create_risk", args)
		exec("CREATE TABLE saved_concern_bindings AS SELECT * FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'concern_no_issue_create','Controlled actor without issue creation')", role, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) SELECT DISTINCT $1::uuid,$2::uuid,a.action FROM role_actions a JOIN saved_concern_bindings b ON b.org_id=a.org_id AND b.role_id=a.role_id WHERE a.action<>'issue.create'", f.org, role)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.bob, role)
		defer func() {
			exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
			exec("INSERT INTO role_bindings SELECT * FROM saved_concern_bindings")
			exec("DROP TABLE saved_concern_bindings")
		}()
		h, v := f.request("POST", "/v1/conversations", f.bobToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Risk-only delegation", MemberIDs: []string{}}, nil)
		expect(t, h, 201, v)
		bc := data[agent.Conversation](t, v)
		args = argsExisting("risk", fresh)
		args["state"] = "realized"
		in := input("update_risk", args)
		in.ExpectedVersion, in.ContextSecurityVersion = bc.Version, bc.ContextSecurityVersion
		h, v = f.request("POST", "/v1/conversations/"+bc.ID+"/runs", f.bobToken, core.NewID(), in, nil)
		expect(t, h, 404, v)
	})
	runTest("cancellation_and_append_only_concern_history", func() {
		before := count("issue")
		args := argsFor("issue", "Cancelled creation")
		r, l := start(input("create_issue", args))
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "Cancel exact issue creation"}, nil)
		expect(t, h, 200, v)
		if _, e := tool(l, "cancelled-issue", "create_issue", args); e == nil {
			t.Fatal("cancelled run created an issue")
		}
		finish(l)
		if count("issue") != before {
			t.Fatal("cancellation changed issue count")
		}
		for _, sql := range []string{"UPDATE agent_run_mutations SET arguments='{}' WHERE run_id=$1", "DELETE FROM agent_mutation_commits WHERE run_id=$1"} {
			if _, e := f.owner.Exec(ctx, sql, createdRun.ID); e == nil {
				t.Fatal("concern delegation history was mutable")
			}
		}
	})
	if os.Getenv("CP_BROWSER_PROJECT_CONCERNS_FIXTURE") == "1" && !t.Failed() {
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'concern_browser_mfa','Actionless MFA requirement for owned concern browser fixture',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.alice, proof)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"project_concerns_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "risk_id": risk.ID, "issue_id": issue.ID, "profile_id": profile, "completed_run_id": createdRun.ID})
		return
	}
	runTest("revoked_origin_cannot_create_a_concern", func() {
		before := count("risk")
		args := argsFor("risk", "Revoked login concern")
		r, l := start(input("create_risk", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, e := tool(l, "revoked-origin-concern", "create_risk", args); e == nil {
			t.Fatal("revoked original login created a concern")
		}
		finish(l)
		if count("risk") != before {
			t.Fatal("revoked login created a concern")
		}
	})
}
