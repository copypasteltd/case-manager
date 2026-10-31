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
func TestAgentDeliverableCreation(t *testing.T) {
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
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact deliverable creation conversation", MemberIDs: []string{}}, nil)
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

	get := func(id string) project.Deliverable {
		t.Helper()
		h, v := f.request("GET", "/v1/deliverables/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Deliverable](t, v)
	}
	create := func(target project.Project, name string) project.Deliverable {
		t.Helper()
		h, v := f.request("POST", "/v1/projects/"+target.ID+"/deliverables", f.aliceToken, core.NewID(), project.DeliverableCreate{Name: name, AcceptanceCriteria: "Verify the complete actual result.", OwnerID: f.alice, Reason: "Isolated real draft"}, nil)
		expect(t, h, 201, v)
		return get(data[project.DeliverableReceipt](t, v).ID)
	}

	getProject := func() project.Project {
		h, v := f.request("GET", "/v1/projects/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[project.Project](t, v)
	}
	argsFor := func(name string) map[string]any {
		return map[string]any{"id": p.ID, "expected_version": getProject().Version, "name": name, "acceptance_criteria": "Check complete actual evidence.", "owner_id": f.bob, "milestone_id": nil, "reason": "Create exactly one reviewed delivery draft"}
	}
	count := func() int {
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM deliverables WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	perform := func(args map[string]any) agent.Run {
		r, l := start(input("create_deliverable", args))
		raw, err := tool(l, "create-exact-draft", "create_deliverable", args)
		if err != nil {
			t.Fatal(err)
		}
		complete(l, raw)
		return getRun(r.ID)
	}
	other := create(p, "Existing draft grants no authority")
	linked := createMilestone(p, "Same-project draft checkpoint", false)
	var created project.Deliverable
	var committed agent.Run
	runTest("requires_complete_parent_instruction_and_explicit_association", func() {
		before := count()
		for _, field := range []string{"name", "acceptance_criteria", "owner_id", "milestone_id"} {
			args := argsFor("Missing creation field")
			delete(args, field)
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_deliverable", args), nil)
			expect(t, h, 422, v)
		}
		for _, patch := range []map[string]any{{"acceptance_criteria": " "}, {"owner_id": nil}, {"owner_id": "bad-user"}, {"milestone_id": "bad-milestone"}, {"state": "submitted"}, {"file_version_ids": []string{oldFile.Version.ID}}, {"revision_id": core.NewID()}, {"id": foreign.ID}, {"expected_version": int64(999)}} {
			args := argsFor("Invalid creation")
			for key, value := range patch {
				args[key] = value
			}
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("create_deliverable", args), nil)
			if h.Code < 400 || h.Code >= 500 {
				t.Fatalf("invalid draft creation returned %d: %v", h.Code, v)
			}
		}
		_, l := start(input("", nil))
		denied(l, "chat-cannot-create", "create_deliverable", argsFor("No authority"), "MUTATION_INTENT_REQUIRED")
		denied(l, "no-free-draft-read", "read_resource", map[string]any{"type": "deliverable", "id": other.ID}, "RESOURCE_NOT_FOUND")
		finish(l)
		if count() != before {
			t.Fatal("invalid input created a draft")
		}
	})
	runTest("creates_one_real_draft_with_exact_receipt_and_atomic_evidence", func() {
		before := count()
		parent := getProject()
		args := argsFor("\u3000\nSecretary created delivery\t\u00a0")
		args["acceptance_criteria"] = "\n\tVerify every agreed fixed result.\r\n"
		args["milestone_id"] = linked.ID
		r, l := start(input("create_deliverable", args))
		changed := argsFor("Model substituted another draft")
		denied(l, "cannot-replace-draft", "create_deliverable", changed, "MUTATION_INTENT_MISMATCH")
		if err := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "No receipt", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); err == nil {
			t.Fatal("draft creation completed without business evidence")
		}
		raw, err := tool(l, "create-reviewed-draft", "create_deliverable", args)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			agent.ExecutionReceipt
			Content agent.MutationView `json:"content"`
		}
		var receipt project.DeliverableReceipt
		if json.Unmarshal(raw, &result) != nil || json.Unmarshal(result.Content.Result, &receipt) != nil {
			t.Fatal("invalid draft creation receipt")
		}
		created = get(receipt.ID)
		if receipt.ID == p.ID || receipt.Version != 1 || receipt.RevisionID != nil || created.Version != 1 || created.State != "draft" || created.ProtocolVersion != 1 || created.CurrentRevisionID != nil || created.ProjectID != p.ID || created.CreatorID == nil || *created.CreatorID != f.alice || created.Name != "Secretary created delivery" || created.AcceptanceCriteria != "Verify every agreed fixed result." || created.OwnerID != f.bob || created.MilestoneID == nil || *created.MilestoneID != linked.ID {
			t.Fatal("created resource is not the exact fresh business draft")
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(result.Content.Result, &fields)
		if len(fields) != 3 || result.Content.ResourceType != "project" || result.Content.ResourceID != p.ID || result.Content.BaseVersion != parent.Version || result.Content.ResultVersion == nil || *result.Content.ResultVersion != 1 {
			t.Fatal("parent and child versions or receipt shape were conflated")
		}
		if len(result.ResourceRefs) != 1 || result.ResourceRefs[0].Type != "deliverable" || result.ResourceRefs[0].ID != created.ID {
			t.Fatal("draft result cited the parent")
		}
		for _, key := range []string{"create-reviewed-draft", "same-authority-new-transport-key"} {
			again, err := tool(l, key, "create_deliverable", args)
			if err != nil {
				t.Fatal(err)
			}
			var repeated agent.ExecutionReceipt
			_ = json.Unmarshal(again, &repeated)
			if repeated.OperationID == nil || result.OperationID == nil || *repeated.OperationID != *result.OperationID || count() != before+1 {
				t.Fatal("retry created another draft or operation")
			}
		}
		read, err := tool(l, "read-only-created-draft", "read_resource", map[string]any{"type": "deliverable", "id": created.ID})
		if err != nil {
			t.Fatal(err)
		}
		var observation agent.ExecutionReceipt
		_ = json.Unmarshal(read, &observation)
		if len(observation.SourceRefs) != 1 || observation.SourceRefs[0].Type != "agent_mutation" || observation.SourceRefs[0].ID != r.ID {
			t.Fatal("new draft read widened the fixed source")
		}
		denied(l, "other-draft-still-denied", "read_resource", map[string]any{"type": "deliverable", "id": other.ID}, "RESOURCE_NOT_FOUND")
		complete(l, raw)
		committed = getRun(r.ID)
		if committed.Mutation == nil || committed.Mutation.Status != "committed" || getProject().Version != parent.Version {
			t.Fatal("draft creation changed the parent or lost its receipt")
		}
		var receipts, events, audit, revisions int
		err = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1),
 (SELECT count(*) FROM outbox_events WHERE aggregate_type='deliverable' AND aggregate_id=$2),
 (SELECT count(*) FROM audit_events WHERE resource_type='deliverable' AND resource_id=$2),
 (SELECT count(*) FROM deliverable_revisions WHERE deliverable_id=$2)`, r.ID, created.ID).Scan(&receipts, &events, &audit, &revisions)
		if err != nil {
			t.Fatal(err)
		}
		if receipts != 1 || events != 1 || audit != 1 || revisions != 0 {
			t.Fatal("creation evidence is not one atomic draft without human revisions")
		}
	})
	runTest("explicit_null_creates_an_unassociated_draft", func() {
		before := count()
		r := perform(argsFor("Explicitly unassociated delivery"))
		var receipt project.DeliverableReceipt
		_ = json.Unmarshal(r.Mutation.Result, &receipt)
		draft := get(receipt.ID)
		if count() != before+1 || draft.MilestoneID != nil || draft.State != "draft" || draft.CurrentRevisionID != nil {
			t.Fatal("null association or draft boundary was lost")
		}
	})
	runTest("invalid_members_and_foreign_or_closed_links_create_nothing", func() {
		closed := createMilestone(p, "Closed creation target", false)
		exec("UPDATE milestones SET state='cancelled',version=version+1 WHERE id=$1", closed.ID)
		foreignLink := createMilestone(foreign, "Foreign creation target", false)
		for _, patch := range []map[string]any{{"owner_id": f.admin}, {"milestone_id": closed.ID}, {"milestone_id": foreignLink.ID}} {
			before := count()
			args := argsFor("Invalid business references")
			for key, value := range patch {
				args[key] = value
			}
			_, l := start(input("create_deliverable", args))
			if _, err := tool(l, "invalid-draft-references", "create_deliverable", args); err == nil {
				t.Fatal("invalid references created a draft")
			}
			finish(l)
			if count() != before {
				t.Fatal("failed references left a partial draft")
			}
		}
	})
	runTest("changed_parent_version_stops_creation", func() {
		args := argsFor("Stale project must not create")
		_, l := start(input("create_deliverable", args))
		before := count()
		exec("UPDATE projects SET name=name||' revised',version=version+1 WHERE id=$1", p.ID)
		denied(l, "stale-parent-create", "create_deliverable", args, "RUNTIME_SOURCE_CHANGED")
		finish(l)
		if count() != before {
			t.Fatal("stale parent created a draft")
		}
	})
	runTest("read_only_project_prevents_creation", func() {
		for _, state := range []string{"cancelled"} {
			exec("UPDATE projects SET state=$2,version=version+1 WHERE id=$1", p.ID, state)
			args := argsFor("Read-only parent")
			_, l := start(input("create_deliverable", args))
			before := count()
			denied(l, "readonly-draft-create", "create_deliverable", args, "PROJECT_READ_ONLY")
			finish(l)
			if count() != before {
				t.Fatal("read-only project created a draft")
			}
		}
		exec("UPDATE projects SET state=$2,version=version+1 WHERE id=$1", p.ID, p.State)
	})
	runTest("receipt_failure_rolls_back_draft_and_exact_retry_creates_once", func() {
		before := count()
		args := argsFor("Draft after an atomic retry")
		r, l := start(input("create_deliverable", args))
		exec(`CREATE FUNCTION fail_delivery_creation_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled draft receipt failure'; END; $$; CREATE TRIGGER fail_delivery_creation_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION fail_delivery_creation_receipt()`)
		if _, err := tool(l, "forced-draft-receipt-failure", "create_deliverable", args); err == nil {
			t.Fatal("receipt failure was hidden")
		}
		var commits int
		_ = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1", r.ID).Scan(&commits)
		if count() != before || commits != 0 {
			t.Fatal("failed receipt left a partial draft")
		}
		exec("DROP TRIGGER fail_delivery_creation_receipt ON agent_tool_receipts; DROP FUNCTION fail_delivery_creation_receipt()")
		raw, err := tool(l, "retry-draft-after-rollback", "create_deliverable", args)
		if err != nil {
			t.Fatal(err)
		}
		complete(l, raw)
		if count() != before+1 {
			t.Fatal("retry did not create exactly one draft")
		}
	})
	runTest("created_draft_uses_the_original_human_submission_and_acceptance", func() {
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'creation_test_mfa','Actionless MFA proof for isolated creation tests',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proof)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key='deliverable_reviewer'`, f.org, f.bob, p.ID)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		f.bobToken = memberMFAToken(f, "bob", "android")
		h, v := f.request("POST", "/v1/deliverables/"+created.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: created.Version, FileVersionIDs: []string{oldFile.Version.ID}, Description: "Actual human submission of the secretary-created draft", Reason: "Separate human submission"}, nil)
		expect(t, h, 200, v)
		submitted := get(created.ID)
		if submitted.State != "submitted" || submitted.CurrentRevisionID == nil {
			t.Fatal("created draft could not enter the original human workflow")
		}
		h, v = f.request("GET", "/v1/deliverables/"+created.ID+"/revisions/"+*submitted.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		revision := data[project.DeliverableRevision](t, v)
		h, v = f.request("POST", "/v1/deliverables/"+created.ID+"/decisions", f.bobToken, core.NewID(), project.DeliverableDecisionInput{ExpectedVersion: submitted.Version, RevisionID: revision.ID, ContentHash: revision.ContentHash, Decision: "accept", FilesVerified: true, CriteriaVerified: true, Reason: "Independent human acceptance"}, nil)
		expect(t, h, 200, v)
		accepted := get(created.ID)
		if accepted.State != "accepted" || accepted.CurrentRevisionID == nil || *accepted.CurrentRevisionID != revision.ID {
			t.Fatal("original acceptance did not preserve its actual fixed revision")
		}
	})
	runTest("cancellation_and_append_only_creation_history", func() {
		before := count()
		args := argsFor("Cancelled draft")
		r, l := start(input("create_deliverable", args))
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "Cancel before draft creation"}, nil)
		expect(t, h, 200, v)
		if _, err := tool(l, "cancelled-draft", "create_deliverable", args); err == nil {
			t.Fatal("cancelled run created a draft")
		}
		finish(l)
		if count() != before {
			t.Fatal("cancellation changed draft count")
		}
		for _, sql := range []string{"UPDATE agent_run_mutations SET arguments='{}' WHERE run_id=$1", "DELETE FROM agent_mutation_commits WHERE run_id=$1"} {
			if _, err := f.owner.Exec(ctx, sql, committed.ID); err == nil {
				t.Fatal("creation history was mutable")
			}
		}
	})
	if os.Getenv("CP_BROWSER_DELIVERABLE_CREATION_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"deliverable_creation_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "deliverable_id": created.ID, "milestone_id": linked.ID, "profile_id": profile, "completed_run_id": committed.ID})
		return
	}
	runTest("revoked_original_login_cannot_create", func() {
		before := count()
		args := argsFor("Revoked login draft")
		r, l := start(input("create_deliverable", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, err := tool(l, "revoked-draft-origin", "create_deliverable", args); err == nil {
			t.Fatal("revoked original login created a draft")
		}
		finish(l)
		if count() != before {
			t.Fatal("revoked login changed draft count")
		}
	})
}
