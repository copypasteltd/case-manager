package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Actual authenticated HTTP, native PostgreSQL and MCP; only native confinement
// proof is controlled. No model, external object store or Telegram is invoked.
func TestRound158AgentBusinessDrafts(t *testing.T) {
	u, evidence, _, _, _ := restorationFixture(t, 15)
	f := u.f
	var oaHeaders map[string]string
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
	for _, key := range []string{"contract_manager", "purchase_manager", "claim_submitter", "request_manager", "party_manager", "agent_user", "subscription_manager"} {
		bind(f.alice, key)
	}
	for _, key := range []string{"contract_reader", "purchase_reader", "claim_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,'project_restricted',true,ARRAY['web','wechat','douyin','android'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
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
			r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, oaHeaders)
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
		r, v := f.request("POST", path, f.aliceToken, core.NewID(), spend.Create{Content: content, ProjectID: evidence.File.ProjectID, Classification: "project_restricted", Reason: "isolated saved commercial revision"}, oaHeaders)
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
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Only execute the selected OA action. Approval, signing and payment remain separate human actions.", BusinessDraft: &agent.BusinessDraftInput{Kind: name, Arguments: raw}}
	}
	start := func(in agent.RunCreate) (agent.Run, agent.ExecutionLease) {
		r, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, oaHeaders)
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
			OA *agent.BusinessDraftView `json:"authorized_business_draft"`
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
		post(l, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/draft_request", "company/submit_request", "company/draft_business_text", "company/create_business_draft"}}, 200)
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
			// Synthetic diagnostics only: a failed atomic tool transaction rolls
			// back. Repeating its exact key exposes the native error to this test,
			// never to a model or public API. Successful calls stay ordinary MCP.
			raw, _ := json.Marshal(in)
			invocation, _ := json.Marshal(key)
			_, detail := service.ExecutionTool(ctx, l, "mcp:"+string(invocation), name, raw)
			if detail != nil {
				return nil, fmt.Errorf("MCP %s; exact failed-command diagnostic: %w", rpc.Content[0].Text, detail)
			}
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
		r, v := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, oaHeaders)
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

	_ = exec
	_ = bind
	_ = ptr
	_ = createBusiness
	_ = post
	_ = input
	_ = start
	_ = tool
	_ = finish
	_ = getRun
	_ = submitArguments
	_ = runTest

	for _, kind := range []string{"contract", "purchase", "claim", "subscription"} {
		kind := kind
		runTest(kind+" creates an ordinary fixed draft only", func(t *testing.T) {
			var args any
			switch kind {
			case "contract":
				args = contracts.Create{Content: contracts.Content{Title: "Batch158 exact contract draft", Notes: "Human selected original prose"}, ProjectID: c.ProjectID, Classification: c.Classification, Reason: "fixed ordinary contract draft"}
			case "purchase", "claim":
				args = spend.Create{Content: spend.Content{Title: "Batch158 exact " + kind + " draft", Notes: "Human selected original prose"}, ProjectID: c.ProjectID, Classification: c.Classification, Reason: "fixed ordinary business draft"}
			case "subscription":
				args = finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Batch158 exact subscription draft", Notes: "Human selected original prose", VendorState: "unknown", ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{}}, SourceKey: "batch158:" + core.NewID(), ProjectID: c.ProjectID, Classification: c.Classification, Reason: "fixed ordinary subscription draft"}
			}
			run, l := start(input(kind, args))
			completed := false
			defer func() {
				if !completed {
					finish(l)
				}
			}()
			selected := getRun(run.ID)
			if selected.BusinessDraft == nil || selected.BusinessDraft.Input.Kind != kind || selected.OARequest != nil {
				t.Fatal("exact draft intent missing")
			}
			request := map[string]any{"run_id": run.ID, "intent_sha256": selected.BusinessDraft.IntentSHA256}
			forged := map[string]any{"run_id": run.ID, "intent_sha256": selected.BusinessDraft.IntentSHA256, "arguments": map[string]any{"title": "unselected model replacement"}}
			if _, e := tool(l, "deny-model-replacement", "create_business_draft", forged); e == nil {
				t.Fatal("model changed the fixed human draft")
			}
			out, e := tool(l, "fixed-create", "create_business_draft", request)
			if e != nil {
				t.Fatal(e)
			}
			var receipt struct {
				Committed   bool                    `json:"committed"`
				OperationID string                  `json:"operation_id"`
				Content     agent.BusinessDraftView `json:"content"`
			}
			if json.Unmarshal(out, &receipt) != nil || !receipt.Committed || receipt.Content.Result == nil || receipt.Content.Result.Kind != kind || receipt.Content.Result.State != "draft" || receipt.Content.Result.Version != 1 {
				t.Fatal("ordinary exact draft receipt missing", string(out))
			}
			again, e := tool(l, "replay-create", "create_business_draft", request)
			if e != nil {
				t.Fatal(e)
			}
			var replay struct {
				OperationID string `json:"operation_id"`
			}
			_ = json.Unmarshal(again, &replay)
			if replay.OperationID != receipt.OperationID {
				t.Fatal("retry duplicated ordinary draft")
			}
			created := receipt.Content.Result
			if e = service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "普通业务草稿已保存，送审及履行由本人另行处理", ResourceRefs: []agent.ResultReference{{Type: kind, ID: created.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_business_draft", ID: run.ID}}, OperationIDs: []string{receipt.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
				t.Fatal(e)
			}
			completed = true
			saved := getRun(run.ID)
			if saved.BusinessDraft == nil || saved.BusinessDraft.Status != "committed" || saved.BusinessDraft.Result.ID != created.ID {
				t.Fatal("formal run lost exact ordinary draft")
			}
			paths := map[string]string{"contract": "contracts", "purchase": "purchases", "claim": "claims", "subscription": "subscriptions"}
			r, v := f.request("GET", "/v1/"+paths[kind]+"/"+created.ID, f.aliceToken, "", nil, oaHeaders)
			expect(t, r, 200, v)
			if kind == "subscription" {
				record := data[finance.Subscription](t, v)
				if record.State != "draft" {
					t.Fatal("draft activated subscription")
				}
			} else {
				var record struct {
					Lifecycle     string `json:"lifecycle"`
					ApprovalState string `json:"approval_state"`
				}
				raw, _ := json.Marshal(v.Data)
				_ = json.Unmarshal(raw, &record)
				if record.Lifecycle != "draft" || record.ApprovalState != "not_submitted" {
					t.Fatal("draft submitted or approved ordinary business")
				}
			}
			if os.Getenv("CP_BROWSER_BATCH158_FIXTURE") != "" && kind == "subscription" {
				holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch158-business-drafts", "conversation_id": c.ID, "project_id": *c.ProjectID, "profile_id": profile, "completed_run_id": run.ID, "subscription_id": created.ID})
			}
		})
	}
}
