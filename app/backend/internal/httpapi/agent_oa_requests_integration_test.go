package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
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
	for _, key := range []string{"contract_manager", "purchase_manager", "claim_submitter", "request_manager", "party_manager", "agent_user"} {
		bind(f.alice, key)
	}
	for _, key := range []string{"contract_reader", "purchase_reader", "claim_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,'project_restricted',true,ARRAY['web','wechat','douyin','android'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
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
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Only execute the selected OA action. Approval, signing and payment remain separate human actions.", OARequest: &agent.OARequestInput{Tool: name, Arguments: raw}}
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
		post(l, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/draft_request", "company/submit_request", "company/draft_business_text"}}, 200)
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

	// The model may save selected prose, but only the actual employee can adopt
	// it through the normal business update path after a verified draft result.
	textDraft := func(kind string) (agent.Run, agent.ExecutionLease, map[string]any) {
		args := createBusiness(kind)
		fields := []string{"notes"}
		if kind == "contract" {
			fields = append(fields, "renewal_terms", "termination_terms")
		} else {
			fields = append(fields, "purpose")
		}
		args["fields"] = fields
		run, l := start(input("draft_business_text", args))
		values := map[string]string{"notes": "Reviewed synthetic business notes\nNo payment instruction."}
		if kind == "contract" {
			values["renewal_terms"] = "Renew only after a separate written review."
			values["termination_terms"] = "Confirm outstanding deliveries before termination."
		} else {
			values["purpose"] = "Synthetic project tools for the current delivery."
		}
		proposal := map[string]any{"run_id": run.ID, "intent_sha256": run.OARequest.IntentSHA256, "fields": values}
		return run, l, proposal
	}
	completeText := func(l agent.ExecutionLease) {
		result := agent.ProposedResult{Status: "draft", Summary: "Selected business prose is a suggestion awaiting human adoption", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{{Type: "agent_oa_request", ID: l.RunID}}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: "Review the original and proposed prose before adoption"}
		wrong := result
		wrong.Status = "completed"
		if err := service.CompleteExecution(ctx, l, wrong, agent.ExecutionUsage{InputTokens: 8, OutputTokens: 2}); err == nil {
			t.Fatal("prose suggestion falsely completed a business write")
		}
		if err := service.CompleteExecution(ctx, l, result, agent.ExecutionUsage{InputTokens: 8, OutputTokens: 2}); err != nil {
			t.Fatal(err)
		}
	}
	adoptText := func(id, token, key string, body any, status int) agent.OARequestView {
		headers := map[string]string(nil)
		if token == f.aliceToken {
			headers = oaHeaders
		}
		h, v := f.request("POST", "/v1/runs/"+id+"/oa-text-adoption", token, key, body, headers)
		expect(t, h, status, v)
		if status != 200 {
			return agent.OARequestView{}
		}
		return data[agent.OARequestView](t, v)
	}
	lineArguments := func(kind string) map[string]any {
		args := createBusiness(kind)
		route := "/v1/" + map[string]string{"purchase": "purchases", "claim": "claims"}[kind] + "/" + args["business_id"].(string)
		h, v := f.request("GET", route, f.aliceToken, "", nil, oaHeaders)
		expect(t, h, 200, v)
		doc := data[spend.Record](t, v)
		// Same visible description, distinct immutable IDs: position/name is not authority.
		second := doc.Lines[0]
		second.ID = core.NewID()
		doc.Lines = append(doc.Lines, second)
		h, v = f.request("PATCH", route, f.aliceToken, core.NewID(), spend.Patch{Content: doc.Content, ExpectedVersion: doc.Version, BaseRevisionID: doc.CurrentRevisionID, Reason: "Two exact synthetic line identities"}, oaHeaders)
		expect(t, h, 200, v)
		doc = data[spend.Record](t, v)
		args["expected_business_version"], args["business_revision_id"], args["business_content_hash"] = doc.Version, doc.CurrentRevisionID, doc.ContentHash
		args["fields"] = []string{"notes", "line:" + doc.Lines[0].ID + ":description"}
		return args
	}
	lineDraft := func(kind string) (agent.Run, agent.ExecutionLease, map[string]any) {
		args := lineArguments(kind)
		run, l := start(input("draft_business_text", args))
		fields := args["fields"].([]string)
		return run, l, map[string]any{"run_id": run.ID, "intent_sha256": run.OARequest.IntentSHA256, "fields": map[string]string{"notes": "Reviewed line prose only.", fields[1]: "明细说明经人工核对；付款及凭证信息保持原业务记录。"}}
	}
	if os.Getenv("CP_BROWSER_OA_LINE_PROSE_FIXTURE") == "1" {
		runTest("browser_line_workbench", func(sub *testing.T) {
			role := core.NewID()
			exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'oa_line_browser_proof','Actionless isolated MFA proof',true)", role, f.org)
			exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
			exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
			exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
			f.aliceToken = memberMFAToken(f, "alice", "android")
			drafts := map[string]any{"fixture_kind": "oa-line-prose", "conversation_id": c.ID, "project_id": *evidence.File.ProjectID, "profile_id": profile}
			for _, kind := range []string{"purchase", "claim"} {
				run, l, proposal := lineDraft(kind)
				if _, err := tool(l, "browser-line-prose", "draft_business_text", proposal); err != nil {
					sub.Fatal(err)
				}
				completeText(l)
				drafts[kind+"_run_id"], drafts[kind+"_business_id"] = run.ID, run.OARequest.BusinessID
			}
			target := lineArguments("claim")
			drafts["target_claim_id"] = target["business_id"]
			holdBrowserBusinessFixture(sub, f, drafts)
		})
		return
	}
	runTest("line_selection_rejects_unknown_duplicate_wrong_kind_and_commercial_fields", func(sub *testing.T) {
		args := lineArguments("purchase")
		field := args["fields"].([]string)[1]
		for _, fields := range [][]string{{"line:" + core.NewID() + ":description"}, {field, field}, {"line:0:description"}, {"line:" + strings.Split(field, ":")[1] + ":unit_price"}, {"amount"}, {"renewal_terms"}} {
			args["fields"] = fields
			h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("draft_business_text", args), oaHeaders)
			expect(sub, h, 422, v)
		}
		contract := createBusiness("contract")
		contract["fields"] = []string{field}
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input("draft_business_text", contract), oaHeaders)
		expect(sub, h, 422, v)
	})
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		for _, kind := range []string{"purchase", "claim"} {
			runTest(channel+"_"+kind+"_selected_line_human_adoption_preserves_all_facts", func(sub *testing.T) {
				previous := f.aliceToken
				defer func() { f.aliceToken = previous; oaHeaders = nil }()
				headers := map[string]string{"Origin": "http://localhost:5178"}
				h, v := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "alice", Password: "member-test-password-long", Device: channel}, headers)
				expect(sub, h, 200, v)
				session := data[identity.LoginResult](sub, v)
				f.aliceToken = session.AccessToken
				if channel == "web" {
					headers["X-CSRF-Token"] = session.CSRFToken
					for _, cookie := range h.Result().Cookies() {
						if cookie.Name == "cp_session" {
							headers["Cookie"] = "cp_session=" + cookie.Value
						}
					}
					oaHeaders = headers
				}
				run, l, proposal := lineDraft(kind)
				values := proposal["fields"].(map[string]string)
				var selected string
				for k := range values {
					if k != "notes" {
						selected = k
					}
				}
				for _, forged := range []map[string]string{{"notes": "same count", "line:" + core.NewID() + ":description": "unselected"}, {"notes": "same count", selected: strings.Repeat("界", 1001)}, {"notes": "same count", selected: "\x00"}} {
					bad := map[string]any{"run_id": run.ID, "intent_sha256": run.OARequest.IntentSHA256, "fields": forged}
					if _, err := tool(l, "rejected-"+core.NewID(), "draft_business_text", bad); err == nil {
						sub.Fatal("invalid model field accepted")
					}
				}
				if channel == "android" && kind == "purchase" {
					var content map[string]any
					_ = json.Unmarshal(run.OARequest.Before.Business.Payload.Content, &content)
					var limit int
					if err := f.owner.QueryRow(ctx, "SELECT agent_oa_text_field_limit($1,$2,$3::jsonb)", kind, selected, run.OARequest.Before.Business.Payload.Content).Scan(&limit); err != nil || limit != 1000 {
						sub.Fatal("database identity guard", err)
					}
					bad, _ := json.Marshal(map[string]string{"notes": "Valid notes", selected: strings.Repeat("界", 1001)})
					sources, _ := json.Marshal([]agent.SourceSnapshot{{SourceReference: agent.SourceReference{Type: "agent_oa_request", ID: run.ID}, ResourceVersion: 1, SHA256: run.OARequest.IntentSHA256}})
					if _, err := f.api.DB.Exec(ctx, "INSERT INTO agent_oa_text_drafts(run_id,org_id,fields,draft_sha256,source_snapshots) VALUES($1,$2,$3,$4,$5)", run.ID, f.org, bad, core.Digest(bad), sources); err == nil {
						sub.Fatal("direct database overlong prose bypassed binding")
					}
				}
				if _, err := tool(l, "selected-line-proposal", "draft_business_text", proposal); err != nil {
					sub.Fatal(err)
				}
				current := getRun(run.ID)
				if current.OARequest.TextDraft == nil {
					sub.Fatal("missing line suggestion")
				}
				body := agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "Human verified both line identities and all unchanged commercial facts"}
				adoptText(run.ID, f.aliceToken, core.NewID(), body, 404)
				completeText(l)
				key := core.NewID()
				saved := adoptText(run.ID, f.aliceToken, key, body, 200)
				replay := adoptText(run.ID, f.aliceToken, key, body, 200)
				if saved.TextDraft == nil || saved.TextDraft.Result == nil || saved.TextDraft.OperationID == nil || replay.TextDraft.OperationID == nil || *saved.TextDraft.OperationID != *replay.TextDraft.OperationID {
					sub.Fatal("missing or duplicate adoption receipt")
				}
				var before, after spend.Content
				_ = json.Unmarshal(run.OARequest.Before.Business.Payload.Content, &before)
				_ = json.Unmarshal(saved.TextDraft.Result.Payload.Content, &after)
				if after.Lines[0].Description != values[selected] || after.Lines[1].Description != before.Lines[1].Description || after.Notes != values["notes"] {
					sub.Fatal("wrong line or unselected line modified")
				}
				after.Lines[0].Description = before.Lines[0].Description
				after.Notes = before.Notes
				a, _ := json.Marshal(before)
				b, _ := json.Marshal(after)
				if !bytes.Equal(a, b) || saved.TextDraft.Result.Version != run.OARequest.Before.Business.Version+1 {
					sub.Fatal("commercial facts or line identities/order changed")
				}
				var count int
				if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM requests WHERE org_id=$1 AND business_id=$2", f.org, run.OARequest.BusinessID).Scan(&count); err != nil || count != 0 {
					sub.Fatal("line adoption submitted an approval", err)
				}
				adoptText(run.ID, f.aliceToken, core.NewID(), body, 409)
			})
		}
	}

	for _, kind := range []string{"contract", "purchase", "claim"} {
		runTest(kind+"_prose_draft_review_and_human_adoption", func(sub *testing.T) {
			run, l, proposal := textDraft(kind)
			bad := map[string]any{"run_id": run.ID, "intent_sha256": run.OARequest.IntentSHA256, "fields": map[string]string{"amount": "99999"}}
			if _, err := tool(l, "commercial-forgery", "draft_business_text", bad); err == nil {
				sub.Fatal("model altered commercial fields")
			}
			raw, err := tool(l, "propose-prose", "draft_business_text", proposal)
			if err != nil {
				sub.Fatal(err)
			}
			var receipt struct {
				Committed   bool                `json:"committed"`
				OperationID *string             `json:"operation_id"`
				Content     agent.OARequestView `json:"content"`
			}
			if json.Unmarshal(raw, &receipt) != nil || receipt.Committed || receipt.OperationID != nil || receipt.Content.Status != "drafted" || receipt.Content.TextDraft == nil {
				sub.Fatal("prose was not a noncommitted suggestion")
			}
			if _, err = tool(l, "propose-prose-again", "draft_business_text", proposal); err != nil {
				sub.Fatal(err)
			}
			other := map[string]any{"run_id": run.ID, "intent_sha256": run.OARequest.IntentSHA256, "fields": map[string]string{"notes": "Replace an existing suggestion"}}
			if _, err = tool(l, "different-prose", "draft_business_text", other); err == nil {
				sub.Fatal("immutable suggestion overwritten")
			}
			var currentVersion int64
			table := "spend_documents"
			if kind == "contract" {
				table = "contracts"
			}
			if err = f.owner.QueryRow(ctx, "SELECT version FROM "+table+" WHERE id=$1", run.OARequest.BusinessID).Scan(&currentVersion); err != nil || currentVersion != run.OARequest.Before.Business.Version {
				sub.Fatal("model saved business content", err)
			}
			body := agent.OATextAdopt{DraftSHA256: receipt.Content.TextDraft.DraftSHA256, Reviewed: true, Reason: "Human reviewed every selected prose field"}
			adoptText(run.ID, f.aliceToken, core.NewID(), body, 404)
			completeText(l)
			adoptText(run.ID, f.bobToken, core.NewID(), body, 404)
			unreviewed := body
			unreviewed.Reviewed = false
			adoptText(run.ID, f.aliceToken, core.NewID(), unreviewed, 422)
			wrongHash := body
			wrongHash.DraftSHA256 = strings.Repeat("0", 64)
			adoptText(run.ID, f.aliceToken, core.NewID(), wrongHash, 409)
			key := core.NewID()
			saved := adoptText(run.ID, f.aliceToken, key, body, 200)
			replay := adoptText(run.ID, f.aliceToken, key, body, 200)
			if saved.Status != "adopted" || saved.TextDraft.Result == nil || saved.TextDraft.OperationID == nil || replay.TextDraft.OperationID == nil || *saved.TextDraft.OperationID != *replay.TextDraft.OperationID {
				sub.Fatal("adoption receipt missing or duplicated")
			}
			adoptText(run.ID, f.aliceToken, core.NewID(), body, 409)
			var before, after map[string]json.RawMessage
			_ = json.Unmarshal(run.OARequest.Before.Business.Payload.Content, &before)
			_ = json.Unmarshal(saved.TextDraft.Result.Payload.Content, &after)
			for field, wanted := range proposal["fields"].(map[string]string) {
				var actual string
				_ = json.Unmarshal(after[field], &actual)
				if actual != wanted {
					sub.Fatal("selected prose not saved", field)
				}
				delete(before, field)
				delete(after, field)
			}
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			a, _ = core.CanonicalJSON(a)
			b, _ = core.CanonicalJSON(b)
			if !bytes.Equal(a, b) || saved.TextDraft.Result.Version != run.OARequest.Before.Business.Version+1 {
				sub.Fatal("unselected commercial facts changed")
			}
			actual := getRun(run.ID)
			if actual.OARequest.Status != "adopted" || actual.Outcome == nil || *actual.Outcome != "draft" {
				sub.Fatal("business draft conflated with approval completion")
			}
			var n int
			if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM requests WHERE org_id=$1 AND business_id=$2", f.org, run.OARequest.BusinessID).Scan(&n); err != nil || n != 0 {
				sub.Fatal("adoption created or submitted an approval", err)
			}
			if _, err = f.api.DB.Exec(ctx, "UPDATE agent_oa_text_drafts SET fields=fields WHERE run_id=$1", run.ID); err == nil {
				sub.Fatal("application overwrote proposal")
			}
			if _, err = f.api.DB.Exec(ctx, "DELETE FROM agent_oa_text_adoptions WHERE run_id=$1", run.ID); err == nil {
				sub.Fatal("application removed adoption receipt")
			}
		})
	}
	runTest("prose_rejects_stale_business_after_completed_suggestion", func(sub *testing.T) {
		run, l, proposal := textDraft("contract")
		if _, err := tool(l, "propose", "draft_business_text", proposal); err != nil {
			sub.Fatal(err)
		}
		completeText(l)
		current := getRun(run.ID)
		var content contracts.Content
		_ = json.Unmarshal(run.OARequest.Before.Business.Payload.Content, &content)
		content.Notes = "Human changed this draft independently"
		h, v := f.request("PATCH", "/v1/contracts/"+run.OARequest.BusinessID, f.aliceToken, core.NewID(), contracts.Patch{Content: content, ExpectedVersion: run.OARequest.Before.Business.Version, BaseRevisionID: run.OARequest.Before.Business.CurrentRevisionID, Reason: "Independent human edit"}, nil)
		expect(sub, h, 200, v)
		adoptText(run.ID, f.aliceToken, core.NewID(), agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "Stale adoption must be refused"}, 409)
	})
	runTest("prose_proposal_cannot_be_adopted_after_cancellation", func(sub *testing.T) {
		run, l, proposal := textDraft("purchase")
		if _, err := tool(l, "propose", "draft_business_text", proposal); err != nil {
			sub.Fatal(err)
		}
		current := getRun(run.ID)
		h, v := f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: current.Version, Action: "cancel", Reason: "Stop before human adoption"}, nil)
		expect(sub, h, 200, v)
		finish(l)
		adoptText(run.ID, f.aliceToken, core.NewID(), agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "Cancelled adoption must be refused"}, 404)
	})

	runTest("prose_adoption_is_atomic_when_receipt_insert_fails", func(sub *testing.T) {
		run, l, proposal := lineDraft("claim")
		if _, err := tool(l, "propose", "draft_business_text", proposal); err != nil {
			sub.Fatal(err)
		}
		completeText(l)
		current := getRun(run.ID)
		body := agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "Human review with atomic receipt"}
		key := core.NewID()
		exec("ALTER TABLE agent_oa_text_adoptions ADD CONSTRAINT controlled_prose_receipt_failure CHECK(false) NOT VALID")
		defer exec("ALTER TABLE agent_oa_text_adoptions DROP CONSTRAINT IF EXISTS controlled_prose_receipt_failure")
		adoptText(run.ID, f.aliceToken, key, body, 422)
		exec("ALTER TABLE agent_oa_text_adoptions DROP CONSTRAINT controlled_prose_receipt_failure")
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM spend_revisions WHERE org_id=$1 AND document_id=$2", f.org, run.OARequest.BusinessID).Scan(&n); err != nil || n != int(run.OARequest.Before.Business.Version) {
			sub.Fatal("failed adoption leaked a new business revision", err)
		}
		adoptText(run.ID, f.aliceToken, key, body, 200)
	})
	runTest("prose_rechecks_current_update_permission", func(sub *testing.T) {
		run, l, proposal := textDraft("contract")
		if _, err := tool(l, "propose", "draft_business_text", proposal); err != nil {
			sub.Fatal(err)
		}
		completeText(l)
		current := getRun(run.ID)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='contract_manager')", f.org, f.alice)
		body := agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "A former writer cannot adopt"}
		h, v := f.request("POST", "/v1/runs/"+run.ID+"/oa-text-adoption", f.aliceToken, core.NewID(), body, nil)
		if h.Code != 403 && h.Code != 404 {
			sub.Fatal("revoked writer retained adoption", h.Code, v)
		}
		bind(f.alice, "contract_manager")
		adoptText(run.ID, f.aliceToken, core.NewID(), body, 200)
	})

	runTest("prose_withdraws_unselected_observed_source_when_it_changes", func(sub *testing.T) {
		run, l, proposal := textDraft("contract")
		if _, err := tool(l, "read-project", "read_resource", map[string]any{"type": "project", "id": *evidence.File.ProjectID}); err != nil {
			sub.Fatal(err)
		}
		if _, err := tool(l, "propose", "draft_business_text", proposal); err != nil {
			sub.Fatal(err)
		}
		completeText(l)
		current := getRun(run.ID)
		if len(current.OARequest.TextDraft.SourceRefs) < 2 {
			sub.Fatal("additional read dependency missing")
		}
		exec("UPDATE projects SET version=version+1 WHERE org_id=$1 AND id=$2", f.org, *evidence.File.ProjectID)
		h, v := f.request("GET", "/v1/runs/"+run.ID, f.aliceToken, "", nil, nil)
		expect(sub, h, 404, v)
		adoptText(run.ID, f.aliceToken, core.NewID(), agent.OATextAdopt{DraftSHA256: current.OARequest.TextDraft.DraftSHA256, Reviewed: true, Reason: "Changed observed source must be re-read"}, 404)
	})

	if os.Getenv("CP_BROWSER_OA_PROSE_FIXTURE") == "1" {
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'oa_prose_browser_proof','Actionless synthetic browser MFA proof',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		drafts := map[string]any{"agent_oa_prose_fixture": true, "conversation_id": c.ID, "project_id": *evidence.File.ProjectID, "profile_id": profile}
		for _, kind := range []string{"contract", "purchase"} {
			run, l, proposal := textDraft(kind)
			values := proposal["fields"].(map[string]string)
			values["notes"] = "本次资料已按交付范围整理。后续变更须记录原因，并重新核对相关业务修订。"
			if kind == "contract" {
				values["renewal_terms"] = "到期前由项目负责人核对续约范围、期限与费用，经双方书面确认后另行办理续约。"
				values["termination_terms"] = "终止前核对已交付成果、未完成事项及资料交接安排，形成双方确认的书面记录。"
			} else {
				values["purpose"] = "用于当前项目交付所需的工具订阅，凭证及费用归属由经办人核对后提交审批。"
			}
			if _, err := tool(l, "browser-prose", "draft_business_text", proposal); err != nil {
				t.Fatal(err)
			}
			completeText(l)
			drafts[kind+"_run_id"] = run.ID
			drafts[kind+"_business_id"] = run.OARequest.BusinessID
		}
		target := createBusiness("claim")
		drafts["claim_business_id"] = target["business_id"]
		holdBrowserBusinessFixture(t, f, drafts)
		return
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
