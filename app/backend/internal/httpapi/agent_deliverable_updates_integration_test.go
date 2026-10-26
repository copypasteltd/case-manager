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
func TestAgentDeliverableUpdates(t *testing.T) {
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
		native.ExpectedTools = append(native.ExpectedTools, "company/create_risk", "company/update_risk", "company/create_issue", "company/update_issue", "company/create_deliverable", "company/update_deliverable", "company/create_milestone", "company/update_project", "company/update_task", "company/transition_task", "company/transition_project", "company/update_milestone", "company/transition_milestone")
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
	argsFor := func(d project.Deliverable) map[string]any {
		return map[string]any{"id": d.ID, "expected_version": d.Version, "name": d.Name, "acceptance_criteria": d.AcceptanceCriteria, "owner_id": d.OwnerID, "milestone_id": d.MilestoneID, "reason": "Use exactly the reviewed deliverable metadata"}
	}
	perform := func(args map[string]any) agent.Run {
		r, l := start(input("update_deliverable", args))
		raw, e := tool(l, "actual-deliverable-command", "update_deliverable", args)
		if e != nil {
			t.Fatal(e)
		}
		complete(l, raw)
		return getRun(r.ID)
	}
	d := create(p, "Draft for secretary metadata updates")
	other := create(p, "Other draft is not authority")
	linked := createMilestone(p, "Same-project delivery checkpoint", false)
	var committed agent.Run
	runTest("requires_complete_exact_fields_and_explicit_association", func() {
		for _, field := range []string{"name", "acceptance_criteria", "owner_id", "milestone_id"} {
			args := argsFor(d)
			delete(args, field)
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("update_deliverable", args), nil)
			expect(t, h, 422, v)
		}
		for _, patch := range []map[string]any{{"acceptance_criteria": " "}, {"owner_id": nil}, {"owner_id": "not-a-user"}, {"milestone_id": "not-a-milestone"}, {"state": "accepted"}, {"revision_id": core.NewID()}, {"expected_version": int64(999)}} {
			args := argsFor(d)
			for k, v := range patch {
				args[k] = v
			}
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("update_deliverable", args), nil)
			if h.Code < 400 || h.Code >= 500 {
				t.Fatalf("invalid update returned %d: %v", h.Code, v)
			}
		}
		_, l := start(input("", nil))
		denied(l, "chat-is-not-deliverable-authority", "update_deliverable", argsFor(d), "MUTATION_INTENT_REQUIRED")
		denied(l, "no-free-deliverable-read", "read_resource", map[string]any{"type": "deliverable", "id": d.ID}, "RESOURCE_NOT_FOUND")
		finish(l)
		if get(d.ID).Version != d.Version {
			t.Fatal("invalid input changed draft")
		}
	})
	runTest("exact_command_commits_real_metadata_and_atomic_receipt_once", func() {
		args := argsFor(d)
		args["name"] = "\u3000Reviewed delivery\u00a0"
		args["acceptance_criteria"] = "\n\tCompare all fixed evidence against the agreed scope.\r\n"
		args["owner_id"] = f.bob
		args["milestone_id"] = linked.ID
		r, l := start(input("update_deliverable", args))
		denied(l, "other-draft-is-not-authority", "read_resource", map[string]any{"type": "deliverable", "id": other.ID}, "RESOURCE_NOT_FOUND")
		read, e := tool(l, "read-exact-delivery", "read_resource", map[string]any{"type": "deliverable", "id": d.ID})
		if e != nil {
			t.Fatal(e)
		}
		var source agent.ExecutionReceipt
		_ = json.Unmarshal(read, &source)
		if len(source.SourceRefs) != 1 || source.SourceRefs[0].Type != "agent_mutation" || source.SourceRefs[0].ID != r.ID {
			t.Fatal("delivery read lost fixed authorization")
		}
		changed := argsFor(d)
		denied(l, "model-cannot-replace-metadata", "update_deliverable", changed, "MUTATION_INTENT_MISMATCH")
		raw, e := tool(l, "exact-delivery-update", "update_deliverable", args)
		if e != nil {
			t.Fatal(e)
		}
		for _, key := range []string{"exact-delivery-update", "same-authority-another-invocation"} {
			again, e := tool(l, key, "update_deliverable", args)
			if e != nil {
				t.Fatal(e)
			}
			a, _ := core.CanonicalJSON(raw)
			b, _ := core.CanonicalJSON(again)
			if !bytes.Equal(a, b) {
				t.Fatal("retry changed original operation")
			}
		}
		complete(l, raw)
		committed = getRun(r.ID)
		current := get(d.ID)
		if current.Version != d.Version+1 || current.Name != "Reviewed delivery" || current.AcceptanceCriteria != "Compare all fixed evidence against the agreed scope." || current.OwnerID != f.bob || current.MilestoneID == nil || *current.MilestoneID != linked.ID || current.State != "draft" || current.CurrentRevisionID != nil {
			t.Fatal("actual business differs from exact metadata")
		}
		var receipts, events, audit int
		if e = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_mutation_commits WHERE run_id=$1),(SELECT count(*) FROM outbox_events WHERE aggregate_type='deliverable' AND aggregate_id=$2 AND event_type='deliverable.updated'),(SELECT count(*) FROM audit_events WHERE resource_type='deliverable' AND resource_id=$2 AND run_id=$1)`, r.ID, d.ID).Scan(&receipts, &events, &audit); e != nil {
			t.Fatal(e)
		}
		if receipts != 1 || events != 1 || audit != 1 {
			t.Fatal("write did not preserve atomic business evidence")
		}
	})
	runTest("explicit_unlink_preserves_other_reviewed_fields", func() {
		before := get(d.ID)
		args := argsFor(before)
		args["milestone_id"] = nil
		perform(args)
		after := get(d.ID)
		if after.MilestoneID != nil || after.Version != before.Version+1 || after.Name != before.Name || after.OwnerID != before.OwnerID || after.AcceptanceCriteria != before.AcceptanceCriteria || after.State != before.State {
			t.Fatal("explicit unlink changed unrelated business")
		}
	})
	runTest("rejects_foreign_closed_milestone_and_nonmember_without_writes", func() {
		foreignM := createMilestone(foreign, "Outside the project", false)
		closed := createMilestone(p, "Cancelled delivery checkpoint", false)
		exec("UPDATE milestones SET state='cancelled',version=version+1 WHERE id=$1", closed.ID)
		for _, patch := range []map[string]any{{"milestone_id": foreignM.ID}, {"milestone_id": closed.ID}, {"owner_id": f.admin}} {
			before := get(d.ID)
			args := argsFor(before)
			for k, v := range patch {
				args[k] = v
			}
			_, l := start(input("update_deliverable", args))
			if _, e := tool(l, "invalid-association-or-owner", "update_deliverable", args); e == nil {
				t.Fatal("invalid delivery association accepted")
			}
			finish(l)
			if get(d.ID).Version != before.Version {
				t.Fatal("rejected command changed draft")
			}
		}
		foreignD := create(foreign, "Wrong project delivery")
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("update_deliverable", argsFor(foreignD)), nil)
		expect(t, h, 404, v)
	})
	runTest("stale_basis_and_read_only_project_stop_original_command", func() {
		before := get(d.ID)
		args := argsFor(before)
		_, l := start(input("update_deliverable", args))
		exec("UPDATE deliverables SET name=name||' later',version=version+1 WHERE id=$1", d.ID)
		if _, e := tool(l, "stale-delivery", "update_deliverable", args); e == nil {
			t.Fatal("stale delivery accepted")
		}
		finish(l)
		before = get(d.ID)
		args = argsFor(before)
		exec("UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", p.ID)
		_, l = start(input("update_deliverable", args))
		denied(l, "read-only-project", "update_deliverable", args, "PROJECT_READ_ONLY")
		finish(l)
		if get(d.ID).Version != before.Version {
			t.Fatal("read-only project changed draft")
		}
		exec("UPDATE projects SET state='draft',version=version+1 WHERE id=$1", p.ID)
	})
	runTest("legacy_protocol_remains_blocked", func() {
		// Recreate a pre-protocol row only inside this isolated owner transaction.
		// The runtime never receives this owner identity or trigger bypass.
		legacyID := core.NewID()
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO deliverables(id,org_id,project_id,name,state,acceptance_criteria,owner_id,creator_id,protocol_version) VALUES($1,$2,$3,'Synthetic legacy delivery','draft','Legacy criteria',$4,$4,0)`, legacyID, f.org, p.ID, f.alice); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		legacy := get(legacyID)
		_, l := start(input("update_deliverable", argsFor(legacy)))
		denied(l, "legacy-update", "update_deliverable", argsFor(legacy), "DELIVERABLE_LEGACY_EVIDENCE_REQUIRED")
		finish(l)
		if get(legacy.ID).Version != legacy.Version {
			t.Fatal("legacy evidence overwritten")
		}
	})
	runTest("submitted_delivery_stays_read_only_and_returned_revision_is_immutable", func() {
		proof := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'delivery_test_mfa','Actionless MFA proof for isolated tests',true)", proof, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proof)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key='deliverable_reviewer'`, f.org, f.bob, p.ID)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		f.bobToken = memberMFAToken(f, "bob", "android")
		submitted := create(p, "Human fixed submission")
		h, v := f.request("POST", "/v1/deliverables/"+submitted.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: submitted.Version, FileVersionIDs: []string{oldFile.Version.ID}, Description: "Actual fixed human result", Reason: "Human submission remains separate"}, nil)
		expect(t, h, 200, v)
		submitted = get(submitted.ID)
		_, l := start(input("update_deliverable", argsFor(submitted)))
		denied(l, "submitted-read-only", "update_deliverable", argsFor(submitted), "DELIVERABLE_READ_ONLY")
		finish(l)
		h, v = f.request("GET", "/v1/deliverables/"+submitted.ID+"/revisions/"+*submitted.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		revision := data[project.DeliverableRevision](t, v)
		h, v = f.request("POST", "/v1/deliverables/"+submitted.ID+"/decisions", f.bobToken, core.NewID(), project.DeliverableDecisionInput{ExpectedVersion: submitted.Version, RevisionID: revision.ID, ContentHash: revision.ContentHash, Decision: "request_changes", FilesVerified: true, CriteriaVerified: true, Reason: "Independent human requests the exact revision be revised"}, nil)
		expect(t, h, 200, v)
		returned := get(submitted.ID)
		args := argsFor(returned)
		args["name"] = "Reviewed returned delivery"
		perform(args)
		after := get(submitted.ID)
		if after.State != "changes_requested" || after.CurrentRevisionID == nil || *after.CurrentRevisionID != revision.ID {
			t.Fatal("metadata edit replaced human submission or decision")
		}
		h, v = f.request("GET", "/v1/deliverables/"+submitted.ID+"/revisions/"+revision.ID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		original := data[project.DeliverableRevision](t, v)
		if original.ContentHash != revision.ContentHash || original.Name != revision.Name || original.Decision == nil {
			t.Fatal("metadata edit rewrote fixed revision")
		}
		h, v = f.request("POST", "/v1/deliverables/"+after.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: after.Version, ExpectedRevisionID: after.CurrentRevisionID, FileVersionIDs: []string{oldFile.Version.ID}, Description: "Revised result explicitly submitted by a person", Reason: "Separate human resubmission"}, nil)
		expect(t, h, 200, v)
		after = get(after.ID)
		h, v = f.request("GET", "/v1/deliverables/"+after.ID+"/revisions/"+*after.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		revised := data[project.DeliverableRevision](t, v)
		h, v = f.request("POST", "/v1/deliverables/"+after.ID+"/decisions", f.bobToken, core.NewID(), project.DeliverableDecisionInput{ExpectedVersion: after.Version, RevisionID: revised.ID, ContentHash: revised.ContentHash, Decision: "accept", FilesVerified: true, CriteriaVerified: true, Reason: "Independent actual human acceptance"}, nil)
		expect(t, h, 200, v)
		accepted := get(after.ID)
		_, l = start(input("update_deliverable", argsFor(accepted)))
		denied(l, "accepted-read-only", "update_deliverable", argsFor(accepted), "DELIVERABLE_READ_ONLY")
		finish(l)
		if get(after.ID).Version != accepted.Version {
			t.Fatal("secretary changed an accepted delivery")
		}
	})
	runTest("cancelled_run_and_append_only_history", func() {
		before := get(d.ID)
		args := argsFor(before)
		r, l := start(input("update_deliverable", args))
		h, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: getRun(r.ID).Version, Action: "cancel", Reason: "Stop exact edit"}, nil)
		expect(t, h, 200, v)
		if _, e := tool(l, "cancelled-edit", "update_deliverable", args); e == nil {
			t.Fatal("cancelled edit executed")
		}
		finish(l)
		if get(d.ID).Version != before.Version {
			t.Fatal("cancel changed business")
		}
		for _, sql := range []string{"UPDATE agent_run_mutations SET arguments='{}' WHERE run_id=$1", "DELETE FROM agent_mutation_commits WHERE run_id=$1"} {
			if _, e := f.owner.Exec(ctx, sql, committed.ID); e == nil {
				t.Fatal("fixed delivery evidence mutable")
			}
		}
	})
	if os.Getenv("CP_BROWSER_DELIVERABLE_UPDATE_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"deliverable_update_fixture": true, "conversation_id": c.ID, "project_id": p.ID, "deliverable_id": d.ID, "milestone_id": linked.ID, "profile_id": profile, "completed_run_id": committed.ID})
		return
	}
	runTest("revoked_originating_session_blocks_later_write", func() {
		before := get(d.ID)
		args := argsFor(before)
		r, l := start(input("update_deliverable", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_run_mutations WHERE run_id=$1)", r.ID)
		if _, e := tool(l, "revoked-delivery-origin", "update_deliverable", args); e == nil {
			t.Fatal("revoked original login executed edit")
		}
		finish(l)
		var currentVersion int64
		if e := f.owner.QueryRow(ctx, "SELECT version FROM deliverables WHERE id=$1", d.ID).Scan(&currentVersion); e != nil {
			t.Fatal(e)
		}
		if currentVersion != before.Version {
			t.Fatal("revoked login changed business")
		}
	})
}
