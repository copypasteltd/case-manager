package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/project"
)

// Real native PostgreSQL, authenticated public HTTP and the actual MCP bridge.
// Only native-confinement proof is controlled; no model or external service runs.
func TestAgentDeliverableSubmission(t *testing.T) {
	u, oldFile, _, _, _ := restorationFixture(t, 25)
	f := u.f
	ctx := context.Background()
	headers := map[string]string{}
	request := func(method, path, token, key string, body any, extra map[string]string) (*httptest.ResponseRecorder, response) {
		merged := map[string]string{}
		for k, v := range headers {
			merged[k] = v
		}
		for k, v := range extra {
			merged[k] = v
		}
		return f.request(method, path, token, key, body, merged)
	}

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
	h, v := request("GET", "/v1/projects/"+*oldFile.File.ProjectID, f.aliceToken, "", nil, nil)
	expect(t, h, 200, v)
	p := data[project.Project](t, v)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-update','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated exact update fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact project stage conversation", MemberIDs: []string{}}, nil)
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
		rec, res := request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
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
		h, v := request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[agent.Run](t, v)
	}

	get := func(id string) project.Deliverable {
		t.Helper()
		h, v := request("GET", "/v1/deliverables/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Deliverable](t, v)
	}
	create := func() project.Deliverable {
		t.Helper()
		h, v := request("POST", "/v1/projects/"+p.ID+"/deliverables", f.aliceToken, core.NewID(), project.DeliverableCreate{Name: "Reviewed secretary submission", AcceptanceCriteria: "Independent reviewer checks exact fixed versions", OwnerID: f.alice, Reason: "Synthetic fixed delivery"}, nil)
		expect(t, h, 201, v)
		return get(data[project.DeliverableReceipt](t, v).ID)
	}
	argsFor := func(d project.Deliverable) map[string]any {
		return map[string]any{"id": d.ID, "expected_version": d.Version, "expected_revision_id": d.CurrentRevisionID, "file_version_ids": []string{oldFile.Version.ID}, "description": "\u3000Reviewed fixed delivery\u00a0", "reason": "Only submit this exact revision"}
	}
	revision := func(d project.Deliverable) project.DeliverableRevision {
		t.Helper()
		h, v := request("GET", "/v1/deliverables/"+d.ID+"/revisions/"+*d.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.DeliverableRevision](t, v)
	}
	runTest("exact_fields_and_original_revision_required", func() {
		d := create()
		for _, field := range []string{"expected_revision_id", "file_version_ids", "description", "reason"} {
			args := argsFor(d)
			delete(args, field)
			h, v := request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("submit_deliverable", args), nil)
			expect(t, h, 422, v)
		}
		for _, patch := range []map[string]any{{"file_version_ids": []string{}}, {"file_version_ids": []string{oldFile.Version.ID, oldFile.Version.ID}}, {"state": "accepted"}, {"expected_revision_id": core.NewID()}, {"description": " "}} {
			args := argsFor(d)
			for k, v := range patch {
				args[k] = v
			}
			h, v := request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("submit_deliverable", args), nil)
			if h.Code < 400 || h.Code >= 500 {
				t.Fatalf("invalid submission %d: %v", h.Code, v)
			}
		}
		_, l := start(input("", nil))
		denied(l, "chat-not-authority", "submit_deliverable", argsFor(d), "MUTATION_INTENT_REQUIRED")
		finish(l)
		if get(d.ID).Version != d.Version {
			t.Fatal("invalid input changed business")
		}
	})
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		runTest(channel+"_commits_one_fixed_revision_and_replays_once", func() {
			headers = map[string]string{}
			h, v := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "alice", Password: "member-test-password-long", Device: channel}, map[string]string{"Origin": "http://localhost:5178"})
			expect(t, h, 200, v)
			login := data[identity.LoginResult](t, v)
			f.aliceToken = login.AccessToken
			if channel == "web" {
				cookies := []string{}
				csrf := ""
				for _, cookie := range h.Result().Cookies() {
					cookies = append(cookies, cookie.Name+"="+cookie.Value)
					if cookie.Name == "cp_csrf" {
						csrf = cookie.Value
					}
				}
				headers = map[string]string{"Cookie": strings.Join(cookies, "; "), "X-CSRF-Token": csrf, "Origin": "http://localhost:5178"}
			}
			d := create()
			args := argsFor(d)
			r, l := start(input("submit_deliverable", args))
			changed := argsFor(d)
			changed["description"] = "Model replacement"
			denied(l, "no-description-rewrite", "submit_deliverable", changed, "MUTATION_INTENT_MISMATCH")
			raw, e := tool(l, "exact-fixed-submit", "submit_deliverable", args)
			if e != nil {
				t.Fatal(e)
			}
			replay, e := tool(l, "second-invocation", "submit_deliverable", args)
			if e != nil {
				t.Fatal(e)
			}
			a, _ := core.CanonicalJSON(raw)
			b, _ := core.CanonicalJSON(replay)
			if !bytes.Equal(a, b) {
				t.Fatal("retry changed receipt")
			}
			complete(l, raw)
			actual := get(d.ID)
			rev := revision(actual)
			if actual.State != "submitted" || actual.Version != d.Version+1 || actual.CurrentRevisionID == nil || rev.Description != "Reviewed fixed delivery" {
				t.Fatal("actual fixed submission differs")
			}
			var commits, revisions, decisions, audit int
			if e = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1),(SELECT count(*) FROM deliverable_revisions WHERE deliverable_id=$2),(SELECT count(*) FROM acceptance_decisions WHERE revision_id=$3),(SELECT count(*) FROM audit_events WHERE resource_type='deliverable' AND resource_id=$2 AND run_id=$1)`, r.ID, d.ID, *actual.CurrentRevisionID).Scan(&commits, &revisions, &decisions, &audit); e != nil {
				t.Fatal(e)
			}
			if commits != 1 || revisions != 1 || decisions != 0 || audit != 1 {
				t.Fatal("submission lost atomic evidence or invented acceptance")
			}
			if getRun(r.ID).State != "succeeded" {
				t.Fatal("execution did not finish")
			}
		})
	}
	runTest("stale_head_and_revoked_original_session_never_write", func() {
		d := create()
		args := argsFor(d)
		_, l := start(input("submit_deliverable", args))
		h, v := request("PATCH", "/v1/deliverables/"+d.ID, f.aliceToken, core.NewID(), project.DeliverablePatch{ExpectedVersion: d.Version, DeliverableCreate: project.DeliverableCreate{Name: "Human changed the reviewed base", AcceptanceCriteria: d.AcceptanceCriteria, OwnerID: f.alice, Reason: "Changed before delegated submit"}}, nil)
		expect(t, h, 200, v)
		if _, e := tool(l, "stale-head", "submit_deliverable", args); e == nil {
			t.Fatal("stale delivery accepted")
		}
		finish(l)
		d = create()
		args = argsFor(d)
		_, l = start(input("submit_deliverable", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL", f.org, f.alice)
		if _, e := tool(l, "revoked-session", "submit_deliverable", args); e == nil {
			t.Fatal("revoked submission accepted")
		}
		finish(l)
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM deliverable_revisions WHERE deliverable_id=$1", d.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("revoked intent wrote revision")
		}
	})
}
