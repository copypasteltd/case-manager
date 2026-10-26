package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Actual authenticated HTTP, native PostgreSQL and MCP; only native confinement
// proof is controlled. No model, external object store or Telegram is invoked.
func TestAgentOARequests(t *testing.T) {
	u, evidence, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	defer func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	}()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, key := range []string{"contract_manager", "purchase_manager", "claim_submitter", "request_manager", "party_manager", "agent_user"} {
		bind(f.alice, key)
	}
	for _, key := range []string{"contract_reader", "purchase_reader", "claim_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	admin := f.administratorToken()
	ptr := func(v string) *string { return &v }
	rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic OA supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: evidence.File.ProjectID, Classification: "project_restricted", Reason: "isolated OA fixture"}, nil)
	expect(t, rec, 201, res)
	supplier := data[party.Party](t, res)
	for _, kind := range []string{"contract", "purchase", "claim"} {
		input := workflow.DefinitionInput{Key: "oa.fixture." + kind, Name: "Real OA " + kind + " reviewer", Kind: kind, Category: "oa.fixture", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Reason: "isolated explicit OA approval policy", Steps: []workflow.StepInput{{Title: "Actual employee review", AssignedUserID: f.bob}}}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), input, nil)
		expect(t, rec, 201, res)
	}
	createBusiness := func(kind string) map[string]any {
		if kind == "contract" {
			in := contracts.Create{Content: contracts.Content{Title: "OA contract " + core.NewID(), Direction: ptr("payable"), Type: ptr("service"), Category: "oa.fixture", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("12.34"), StartDate: ptr("2026-11-01"), EndDate: ptr("2027-10-31"), DraftFileVersionID: &evidence.Version.ID}, ProjectID: evidence.File.ProjectID, Classification: "project_restricted", Reason: "isolated saved business revision"}
			r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
			expect(t, r, 201, v)
			d := data[contracts.Contract](t, v)
			return map[string]any{"kind": kind, "business_id": d.ID, "expected_business_version": d.Version, "business_revision_id": d.CurrentRevisionID, "business_content_hash": d.ContentHash, "reason": "Only prepare the exact saved business revision"}
		}
		content := spend.Content{Title: "OA " + kind + " " + core.NewID(), Category: "oa.fixture", Purpose: "Project tools", OwnerID: &f.alice, Currency: ptr("HKD"), PricingBasis: "net_plus_tax", RoundingMode: "half_up", ExpectedDeliveryDate: ptr("2026-11-20"), SupplierCandidates: []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}, QuoteVersionIDs: []string{evidence.Version.ID}, Lines: []spend.Line{{ID: core.NewID(), Description: "Project tools", Quantity: ptr("1"), Unit: "licence", UnitPrice: ptr("12.34")}}}
		path := "/v1/purchases"
		if kind == "claim" {
			path = "/v1/claims"
			content.PricingBasis = "gross_includes_tax"
			content.ExpectedDeliveryDate = nil
			content.SupplierCandidates = nil
			content.QuoteVersionIDs = nil
			content.PeriodStart = ptr("2026-10-01")
			content.PeriodEnd = ptr("2026-10-31")
			content.Lines = []spend.Line{{ID: core.NewID(), Description: "Employee expense", DeclaredAmount: ptr("12.34"), ExpenseDate: ptr("2026-10-15"), Category: "tools", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{evidence.Version.ID}, Funding: "employee_advance", DuplicateReceiptReason: "Reviewed synthetic shared evidence"}}
		}
		r, v := f.request("POST", path, f.aliceToken, core.NewID(), spend.Create{Content: content, ProjectID: evidence.File.ProjectID, Classification: "project_restricted", Reason: "isolated saved commercial revision"}, nil)
		expect(t, r, 201, v)
		d := data[spend.Record](t, v)
		return map[string]any{"kind": kind, "business_id": d.ID, "expected_business_version": d.Version, "business_revision_id": d.CurrentRevisionID, "business_content_hash": d.ContentHash, "reason": "Only prepare the exact saved business revision"}
	}
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'oa-assist','controlled-test',repeat('a',64),$3,repeat('b',64),'active',20000,2000,20,600,$4,'Isolated OA fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: evidence.File.ProjectID, Title: "Exact OA authorization conversation", MemberIDs: []string{}}, nil)
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

	input := func(name string, args any) agent.RunCreate {
		raw, _ := json.Marshal(args)
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Only execute the selected OA action. Approval, signing and payment remain separate human actions.", OARequest: &agent.OARequestInput{Tool: name, Arguments: raw}}
	}
	start := func(in agent.RunCreate) (agent.Run, agent.ExecutionLease) {
		r, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 202, v)
		run := data[agent.Run](t, v)
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "oa.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil {
			t.Fatal(e)
		}
		var prepared struct {
			OA *agent.OARequestView `json:"authorized_oa_request"`
		}
		if json.Unmarshal([]byte(x.PreparedContext), &prepared) != nil || prepared.OA == nil || prepared.OA.RunID != run.ID || strings.Contains(x.PreparedContext, "session_id") {
			t.Fatal("public exact OA preparation missing or leaked session")
		}
		l, e := service.LeaseForCredential(ctx, x.Credential)
		if e != nil {
			t.Fatal(e)
		}
		l.Job = *j
		credentials[l.RunID] = x.Credential
		post(l, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/draft_request", "company/submit_request"}}, 200)
		if e = service.StartExecution(ctx, l); e != nil {
			t.Fatal(e)
		}
		return run, l
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
		if e := service.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED"); e != nil {
			t.Fatal(e)
		}
	}
	getRun := func(id string) agent.Run {
		r, v := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[agent.Run](t, v)
	}
	submitArguments := func(r workflow.Request) agent.OASubmitArguments {
		h, v := f.request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		p := data[workflow.SubmissionPreview](t, v)
		return agent.OASubmitArguments{RequestID: r.ID, SubmitInput: workflow.SubmitInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, WorkflowDefinitionID: p.Workflow.ID, PreviewHash: p.PreviewHash, Reason: "Human reviewed the actual policy and assigned employee"}}
	}
	runTest := func(name string, fn func(*testing.T)) {
		t.Run(name, func(sub *testing.T) {
			previous := t
			t = sub
			f.t = sub
			defer func() { t = previous; f.t = previous }()
			fn(sub)
		})
	}
	var completedRun agent.Run
	for _, kind := range []string{"contract", "purchase", "claim"} {
		runTest(kind+"_draft_revise_and_exact_submission", func(sub *testing.T) {
			args := createBusiness(kind)
			run, l := start(input("draft_request", args))
			raw, e := tool(l, "create", "draft_request", args)
			if e != nil {
				sub.Fatal(e)
			}
			var receipt struct {
				OperationID string              `json:"operation_id"`
				Committed   bool                `json:"committed"`
				OA          agent.OARequestView `json:"content"`
			}
			if json.Unmarshal(raw, &receipt) != nil || !receipt.Committed || receipt.OA.Result == nil || receipt.OA.Result.State != "draft" {
				sub.Fatal("no real draft receipt")
			}
			first := *receipt.OA.Result
			if _, e = tool(l, "replay", "draft_request", args); e != nil {
				sub.Fatal(e)
			}
			var count int
			if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_oa_commits WHERE run_id=$1", run.ID).Scan(&count); e != nil || count != 1 {
				sub.Fatal("OA replay duplicated commit", e)
			}
			changed := map[string]any{}
			for k, v := range args {
				changed[k] = v
			}
			changed["reason"] = "Unauthorized alternate instruction"
			if _, e = tool(l, "different-args", "draft_request", changed); e == nil {
				sub.Fatal("model widened explicit authorization")
			}
			if _, e = tool(l, "read-evidence", "read_resource", map[string]any{"type": "agent_oa_request", "id": run.ID, "max_chars": 10000}); e != nil {
				sub.Fatal(e)
			}
			finish(l)
			// Refresh the actual business version after request linkage; never infer it.
			route := map[string]string{"contract": "contracts", "purchase": "purchases", "claim": "claims"}[kind]
			h, v := f.request("GET", "/v1/"+route+"/"+args["business_id"].(string), f.aliceToken, "", nil, nil)
			expect(sub, h, 200, v)
			var live struct {
				Version  int64  `json:"version"`
				Revision string `json:"current_revision_id"`
				Hash     string `json:"content_hash"`
			}
			b, _ := json.Marshal(v.Data)
			_ = json.Unmarshal(b, &live)
			args["expected_business_version"] = live.Version
			args["business_revision_id"] = live.Revision
			args["business_content_hash"] = live.Hash
			args["request_id"] = first.ID
			args["expected_version"] = first.Version
			_, rl := start(input("draft_request", args))
			raw, e = tool(rl, "revise", "draft_request", args)
			if e != nil {
				sub.Fatal(e)
			}
			_ = json.Unmarshal(raw, &receipt)
			revised := *receipt.OA.Result
			if revised.ID != first.ID || revised.Version != first.Version+1 {
				sub.Fatal("revision did not update the original request")
			}
			finish(rl)
			submit := submitArguments(revised)
			sr, sl := start(input("submit_request", submit))
			raw, e = tool(sl, "submit", "submit_request", submit)
			if e != nil {
				sub.Fatal(e)
			}
			_ = json.Unmarshal(raw, &receipt)
			actual := receipt.OA.Result
			if actual == nil || actual.State != "in_review" || actual.ActiveInstanceID == nil || len(actual.Steps) != 1 || actual.Steps[0].AssignedUserID != f.bob || actual.Workflow == nil || actual.Workflow.ID != submit.WorkflowDefinitionID {
				sub.Fatal("submission did not retain actual policy/reviewer")
			}
			if _, e = tool(sl, "submit-again", "submit_request", submit); e != nil {
				sub.Fatal(e)
			}
			var paid int
			if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM approval_decisions WHERE request_id=$1", actual.ID).Scan(&paid); e != nil || paid != 0 {
				sub.Fatal("secretary approved the request", e)
			}
			var proof struct {
				OperationID  *string                 `json:"operation_id"`
				ResourceRefs []agent.ResultReference `json:"resource_refs"`
				SourceRefs   []agent.ResultReference `json:"source_refs"`
			}
			_ = json.Unmarshal(raw, &proof)
			proposed := agent.ProposedResult{Status: "completed", Summary: "Sent for real employee review; no decision made.", ResourceRefs: proof.ResourceRefs, SourceRefs: proof.SourceRefs, OperationIDs: []string{*proof.OperationID}, MissingFields: []string{}}
			if e = service.CompleteExecution(ctx, sl, proposed, agent.ExecutionUsage{}); e != nil {
				sub.Fatal(e)
			}
			completedRun = getRun(sr.ID)
			if completedRun.OARequest == nil || completedRun.OARequest.Status != "committed" {
				sub.Fatal("readable run lost the immutable receipt")
			}
		})
	}
	if os.Getenv("CP_BROWSER_OA_REQUEST_FIXTURE") == "1" {
		// A fresh business draft is the human browser's selectable target.
		target := createBusiness("purchase")
		submitTarget := createBusiness("claim")
		body, _ := json.Marshal(submitTarget)
		var draft workflow.RequestCreate
		_ = json.Unmarshal(body, &draft)
		h, v := f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), draft, nil)
		expect(t, h, 201, v)
		ready := data[workflow.Request](t, v)
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'oa_browser_proof','Actionless local browser proof',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"agent_oa_request_fixture": true, "conversation_id": c.ID, "project_id": *evidence.File.ProjectID, "business_id": target["business_id"], "submit_business_id": submitTarget["business_id"], "submit_request_id": ready.ID, "profile_id": profile, "completed_run_id": completedRun.ID})
		return
	}
	runTest("stale_version_and_unbound_scope_denied", func(sub *testing.T) {
		args := createBusiness("purchase")
		args["expected_business_version"] = int64(99)
		r, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("draft_request", args), nil)
		if r.Code < 400 {
			sub.Fatal("stale business authorized", v)
		}
		args = createBusiness("claim")
		bad := input("draft_request", args)
		bad.OARequest.Arguments = []byte(`{"kind":"claim","business_id":"bad"}`)
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
		if r.Code < 400 {
			sub.Fatal("invalid scope accepted", v)
		}
	})
	runTest("receipt_failure_rolls_back_and_exact_retry_succeeds", func(sub *testing.T) {
		args := createBusiness("purchase")
		run, l := start(input("draft_request", args))
		exec("ALTER TABLE agent_tool_receipts ADD CONSTRAINT controlled_oa_receipt_failure CHECK(tool<>'draft_request') NOT VALID")
		_, err := tool(l, "receipt-fails", "draft_request", args)
		exec("ALTER TABLE agent_tool_receipts DROP CONSTRAINT controlled_oa_receipt_failure")
		if err == nil {
			sub.Fatal("receipt failure did not stop business commit")
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM requests WHERE business_id=$1)+(SELECT count(*) FROM agent_oa_commits WHERE run_id=$2)", args["business_id"], run.ID).Scan(&n); e != nil || n != 0 {
			sub.Fatal("receipt failure left an application or OA commit", e)
		}
		if _, e := tool(l, "exact-retry", "draft_request", args); e != nil {
			sub.Fatal(e)
		}
		finish(l)
	})
	runTest("cancel_before_write_leaves_no_application", func(sub *testing.T) {
		args := createBusiness("claim")
		run, l := start(input("draft_request", args))
		run = getRun(run.ID)
		h, v := f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": run.Version, "action": "cancel", "reason": "Stop before preparing any application"}, nil)
		expect(sub, h, 200, v)
		if _, e := tool(l, "cancelled", "draft_request", args); e == nil {
			sub.Fatal("cancelled task wrote")
		}
		finish(l)
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM requests WHERE business_id=$1", args["business_id"]).Scan(&n); e != nil || n != 0 {
			sub.Fatal("cancelled task left an application", e)
		}
	})
	runTest("uncommitted_completion_and_other_tool_denied", func(sub *testing.T) {
		args := createBusiness("contract")
		_, l := start(input("draft_request", args))
		result := agent.ProposedResult{Status: "completed", Summary: "Invented completion", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}
		if e := service.CompleteExecution(ctx, l, result, agent.ExecutionUsage{}); e == nil {
			sub.Fatal("invented completion accepted")
		}
		if _, e := tool(l, "approve", "approve_request", map[string]any{}); e == nil {
			sub.Fatal("approval tool exposed to OA assistant")
		}
		finish(l)
	})
	runTest("immutable_intent_and_commit_cannot_be_rewritten", func(sub *testing.T) {
		if _, e := f.api.DB.Exec(ctx, "UPDATE agent_oa_intents SET tool='draft_request' WHERE run_id=$1", completedRun.ID); e == nil {
			sub.Fatal("OA intent mutable")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM agent_oa_commits WHERE run_id=$1", completedRun.ID); e == nil {
			sub.Fatal("OA receipt deletable")
		}
		h, v := f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=20", f.aliceToken, "", nil, nil)
		expect(sub, h, 200, v)
		h, v = f.request("GET", "/v1/runs/"+completedRun.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(sub, h, 200, v)
		events := data[agent.EventPage](sub, v)
		if events.Run.OARequest == nil || events.Run.OARequest.OperationID == nil {
			sub.Fatal("event stream lost actual OA operation")
		}
	})
	runTest("business_changed_after_authorization_cannot_silently_rebase", func(sub *testing.T) {
		args := createBusiness("purchase")
		run, l := start(input("draft_request", args))
		h, v := f.request("GET", "/v1/purchases/"+args["business_id"].(string), f.aliceToken, "", nil, nil)
		expect(sub, h, 200, v)
		d := data[spend.Record](sub, v)
		d.Content.Title = "Human edited after secretary authorization"
		h, v = f.request("PATCH", "/v1/purchases/"+d.ID, f.aliceToken, core.NewID(), spend.Patch{Content: d.Content, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, Reason: "New human business revision"}, nil)
		expect(sub, h, 200, v)
		if _, e := tool(l, "stale-execution", "draft_request", args); e == nil {
			sub.Fatal("secretary silently rebased to new business revision")
		}
		finish(l)
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_oa_commits WHERE run_id=$1", run.ID).Scan(&n); e != nil || n != 0 {
			sub.Fatal("stale source left commit", e)
		}
	})
	runTest("AI_policy_change_fences_reads_and_writes", func(sub *testing.T) {
		args := createBusiness("claim")
		run, l := start(input("draft_request", args))
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		if _, e := tool(l, "policy-read", "read_resource", map[string]any{"type": "agent_oa_request", "id": run.ID}); e == nil {
			sub.Fatal("AI read survived policy revocation")
		}
		if _, e := tool(l, "policy-write", "draft_request", args); e == nil {
			sub.Fatal("AI write survived policy revocation")
		}
		finish(l)
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	})
	runTest("cancel_after_commit_retains_real_application", func(sub *testing.T) {
		args := createBusiness("contract")
		run, l := start(input("draft_request", args))
		if _, e := tool(l, "write-before-cancel", "draft_request", args); e != nil {
			sub.Fatal(e)
		}
		run = getRun(run.ID)
		h, v := f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": run.Version, "action": "cancel", "reason": "Stop remaining work; preserve actual draft"}, nil)
		expect(sub, h, 200, v)
		finish(l)
		run = getRun(run.ID)
		if run.State != "cancelled" || run.OARequest == nil || run.OARequest.OperationID == nil || run.OARequest.Result == nil {
			sub.Fatal("cancellation erased confirmed draft")
		}
	})
	runTest("foreign_conversation_and_submission_preview_rejected", func(sub *testing.T) {
		args := createBusiness("claim")
		h, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Company private scope", MemberIDs: []string{}}, nil)
		expect(sub, h, 201, v)
		other := data[agent.Conversation](sub, v)
		in := input("draft_request", args)
		in.ExpectedVersion = other.Version
		in.ContextSecurityVersion = other.ContextSecurityVersion
		h, v = f.request("POST", "/v1/conversations/"+other.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		if h.Code < 400 {
			sub.Fatal("project business accepted in company-only conversation")
		}
		raw, _ := json.Marshal(args)
		var draft workflow.RequestCreate
		_ = json.Unmarshal(raw, &draft)
		h, v = f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), draft, nil)
		expect(sub, h, 201, v)
		r := data[workflow.Request](sub, v)
		submit := submitArguments(r)
		submit.PreviewHash = strings.Repeat("0", 64)
		h, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("submit_request", submit), nil)
		if h.Code < 400 {
			sub.Fatal("invented preview authorized")
		}
	})
	runTest("revoked_original_session_fences_write_and_receipt", func(sub *testing.T) {
		args := createBusiness("purchase")
		run, l := start(input("draft_request", args))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_oa_intents WHERE run_id=$1)", run.ID)
		if _, e := tool(l, "revoked", "draft_request", args); e == nil {
			sub.Fatal("revoked original session still wrote")
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_oa_commits WHERE run_id=$1", run.ID).Scan(&n); e != nil || n != 0 {
			sub.Fatal("revoked session left a commit", e)
		}
		finish(l)
	})
}
