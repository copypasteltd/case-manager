package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/party"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/uploadworker"
)

func TestAgentContractBodies(t *testing.T) {
	currentT := t
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f := u.f
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		currentT.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			currentT.Fatal(e)
		}
	}
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	for _, role := range []string{"contract_manager", "party_manager"} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4", core.NewID(), f.org, f.alice, role)
	}
	exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'contract_body_fixture_mfa','Synthetic human review',true)", core.NewID(), f.org)
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='contract_body_fixture_mfa'", core.NewID(), f.org, f.alice)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	before := []byte("\ufeff香港🙂\r\n旧计划\r\n结尾\r\n")
	base, cloud := commitContentFixture(t, u, "原有资料.md", "text/markdown", before, nil)
	reader := &downloadCloud{readFn: cloud.Read}
	hooks := &creationHookWriter{AgentDraftWriter: cloud}
	observedWriter := &observedAgentDraftWriter{AgentDraftWriter: hooks}
	writer := &storage.AgentDraftBackend{Provider: "oss", Bucket: u.service.Backend.Bucket, Writer: observedWriter}
	f.api.AgentFileReaders = map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: writer.Bucket, Reader: reader}}
	f.api.AgentDraftWriter = writer
	f.handler = f.api.Handler()
	service := &agent.Service{DB: f.api.DB, FileText: &storage.AgentTextService{Backends: f.api.AgentFileReaders}, FileDrafts: writer}
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'file-update','controlled-test',$3,$5,$4,'active',20000,8000,20,900,$6,'Isolated actual file update fixture only')`, profile, f.org, strings.Repeat("a", 64), strings.Repeat("b", 64), agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: base.File.ProjectID, Title: "完整合同正文起草与人工采用", MemberIDs: []string{}}, nil)
	expect(currentT, rec, 201, res)
	c := data[agent.Conversation](t, res)
	bridge := &agentworker.Bridge{Service: service, Verifier: &controlledNativeVerifier{}, Model: "controlled-model", WorkingDirectory: t.TempDir()}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	rpc := func(token, path string, payload any) map[string]json.RawMessage {
		currentT.Helper()
		body, _ := json.Marshal(payload)
		req, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(body))
		if e != nil {
			currentT.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		r, e := server.Client().Do(req)
		if e != nil {
			currentT.Fatal(e)
		}
		defer r.Body.Close()
		var out map[string]json.RawMessage
		if json.NewDecoder(io.LimitReader(r.Body, 120001)).Decode(&out) != nil || r.StatusCode != 200 {
			currentT.Fatal("file update bridge response invalid")
		}
		return out
	}
	credential := ""
	tool := func(key, name string, args any) (map[string]json.RawMessage, string) {
		currentT.Helper()
		response := rpc(credential, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": key, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		if len(response["error"]) > 0 {
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(response["error"], &e)
			return nil, e.Message
		}
		var result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Output map[string]json.RawMessage `json:"structuredContent"`
		}
		if json.Unmarshal(response["result"], &result) != nil {
			currentT.Fatal("invalid MCP result")
		}
		if result.IsError {
			return nil, result.Content[0].Text
		}
		return result.Output, ""
	}
	rec, res = f.request("GET", "/v1/folders/"+base.File.FolderID, f.aliceToken, "", nil, nil)
	expect(currentT, rec, 200, res)
	folder := data[storage.Folder](t, res)
	var selectedContract *agent.ContractBodyInput
	start := func(name string) (agent.Run, agent.ExecutionLease) {
		currentT.Helper()
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "起草完整合同正文；沿用准确合同事实，未明确的条款标注待确认，不作商业决定。", ContractBody: selectedContract, FileCreate: &agent.FileCreateInput{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: name, MediaType: "text/markdown"}}
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(currentT, rec, 202, res)
		run := data[agent.Run](t, res)
		if run.FileCreate == nil || run.FileUpdate != nil || run.FileCreate.Target.Name != name {
			currentT.Fatal("new file scope was not fixed")
		}
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "file-create.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			currentT.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil {
			currentT.Fatal(e)
		}
		if !strings.Contains(x.PreparedContext, "authorized_file_create") || strings.Contains(x.PreparedContext, "authorized_file_update") {
			currentT.Fatal("create/update scope confused")
		}
		credential = x.Credential
		lease, e := service.LeaseForCredential(ctx, credential)
		if e != nil {
			currentT.Fatal(e)
		}
		lease.Job = *j
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/recover_file_draft", "company/create_file_draft", "company/propose_file_update", "company/commit_file_update", "company/stage_file_create", "company/propose_file_create", "company/commit_file_create"}}
		if proof := rpc(credential, agentworker.ControlPath+"confinement", native); len(proof["error"]) > 0 {
			currentT.Fatal("creation tool confinement rejected")
		}
		if e = service.StartExecution(ctx, lease); e != nil {
			currentT.Fatal(e)
		}
		return run, lease
	}
	stop := func(lease agent.ExecutionLease) {
		currentT.Helper()
		if e := service.FinishStoppedExecution(ctx, lease.Job, "RUNTIME_CANCEL_REQUESTED"); e != nil {
			currentT.Fatal(e)
		}
	}
	count := func(sql string, args ...any) int {
		currentT.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, sql, args...).Scan(&n); e != nil {
			currentT.Fatal(e)
		}
		return n
	}
	scan := func() {
		currentT.Helper()
		for _, mode := range []string{"control", "verify-small", "promote-small"} {
			runner := &uploadworker.Runner{Service: u.service, Store: cloud, TargetStore: cloud, Owner: "agent-create." + mode, Mode: mode, Scanner: cleanIntegrityScanner(), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
			if worked, e := runner.RunOne(ctx); e != nil || !worked {
				currentT.Fatal("new file existing pipeline", mode, worked, e)
			}
		}
	}
	stage := func(name, content string) agent.FileCreateView {
		currentT.Helper()
		result, code := tool(core.NewID(), "stage_file_create", map[string]any{"folder_id": folder.ID, "name": name, "content": content})
		if code != "" {
			currentT.Fatal("stage creation", code)
		}
		var v agent.FileCreateView
		if json.Unmarshal(result["content"], &v) != nil || len(v.Drafts) != 1 || v.OperationID != nil {
			currentT.Fatal("creation draft receipt invalid")
		}
		return v
	}
	propose := func(name string, v agent.FileCreateView) map[string]any {
		currentT.Helper()
		args := map[string]any{"folder_id": folder.ID, "name": name, "draft_id": v.Drafts[0].ID}
		result, code := tool(core.NewID(), "propose_file_create", args)
		if code != "" || json.Unmarshal(result["content"], &v) != nil || v.Drafts[0].PlanHash == nil {
			currentT.Fatal("creation proposal invalid", code)
		}
		args["plan_hash"] = *v.Drafts[0].PlanHash
		return args
	}
	ptr := func(s string) *string { return &s }
	rec, res = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic contract supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: base.File.ProjectID, Classification: "project_restricted", Reason: "Isolated contract body facts"}, nil)
	expect(t, rec, 201, res)
	supplier := data[party.Party](t, res)
	createContract := func() contracts.Contract {
		currentT.Helper()
		input := contracts.Create{Content: contracts.Content{Title: "Website delivery contract " + core.NewID(), Direction: ptr("payable"), Type: ptr("service"), Category: "body.fixture", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("12000"), StartDate: ptr("2026-11-01"), EndDate: ptr("2027-10-31"), RenewalTerms: "Only on explicit human agreement", TerminationTerms: "Subject to reviewed contract text", Notes: "Synthetic facts only", DraftFileVersionID: &base.Version.ID}, ProjectID: base.File.ProjectID, Classification: "project_restricted", Reason: "Create isolated exact contract draft"}
		r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), input, nil)
		expect(currentT, r, 201, v)
		return data[contracts.Contract](currentT, v)
	}
	selectContract := func(b contracts.Contract) {
		selectedContract = &agent.ContractBodyInput{ContractID: b.ID, ExpectedVersion: b.Version, BaseRevisionID: b.CurrentRevisionID, ContentHash: b.ContentHash, Reason: "Draft a complete body using these exact commercial facts"}
	}
	getRun := func(id string) agent.Run {
		currentT.Helper()
		r, v := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(currentT, r, 200, v)
		return data[agent.Run](currentT, v)
	}
	adopt := func(run agent.Run, key string, input agent.ContractBodyAdopt, status int) agent.ContractBodyView {
		currentT.Helper()
		r, v := f.request("POST", "/v1/runs/"+run.ID+"/contract-body-adoption", f.aliceToken, key, input, nil)
		expect(currentT, r, status, v)
		if status != 200 {
			return agent.ContractBodyView{}
		}
		return data[agent.ContractBodyView](currentT, v)
	}
	adoption := func(run agent.Run) agent.ContractBodyAdopt {
		v := run.ContractBody
		return agent.ContractBodyAdopt{IntentSHA256: v.IntentSHA256, VersionID: v.File.Evidence.VersionID, ContentSHA256: v.File.Evidence.SHA256, Reviewed: true, Reason: "Human reviewed the complete exact contract body"}
	}
	complete := func(b contracts.Contract, name, body string) agent.Run {
		currentT.Helper()
		selectContract(b)
		run, lease := start(name)
		if run.ContractBody == nil || run.ContractBody.Before.ID != b.ID || run.ContractBody.File != nil || run.ContractBody.FileIntentSHA256 != run.FileCreate.IntentSHA256 {
			currentT.Fatal("contract basis not bound to exact file creation")
		}
		rec, response := f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(currentT, rec, 200, response)
		pending := data[agent.EventPage](currentT, response).Run
		if pending.ContractBody == nil || pending.ContractBody.Status != "authorized" || pending.ContractBody.IntentSHA256 != run.ContractBody.IntentSHA256 || pending.ContractBody.File != nil {
			currentT.Fatal("event-log omitted the exact pending contract body")
		}
		var read struct {
			Text  string `json:"text"`
			Next  *int   `json:"next_offset"`
			Total int    `json:"total_chars"`
		}
		var full string
		for offset := 0; ; {
			out, code := tool(core.NewID(), "read_resource", map[string]any{"type": "agent_contract_body", "id": run.ID, "offset": offset, "max_chars": 997})
			if code != "" || json.Unmarshal(out["content"], &read) != nil {
				currentT.Fatal("bounded contract basis read failed", code)
			}
			full += read.Text
			if read.Next == nil {
				break
			}
			offset = *read.Next
		}
		if !strings.Contains(full, b.CurrentRevisionID) || strings.Contains(full, "session_id") {
			currentT.Fatal("contract basis read omitted exact facts or leaked session")
		}
		view := stage(name, body)
		args := propose(name, view)
		scan()
		out, code := tool(core.NewID(), "commit_file_create", args)
		if code != "" || json.Unmarshal(out["content"], &view) != nil || view.OperationID == nil {
			currentT.Fatal("contract body formal file commit failed", code)
		}
		var file storage.CommitResult
		if json.Unmarshal(view.Result, &file) != nil {
			currentT.Fatal("invalid body file receipt")
		}
		if e := service.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "完整正文文件已创建，合同采用仍须本人核对。", ResourceRefs: []agent.ResultReference{{Type: "file", ID: file.File.ID, VersionID: &file.Version.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_file_create", ID: run.ID}, {Type: "agent_contract_body", ID: run.ID}}, OperationIDs: []string{*view.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			currentT.Fatal(e)
		}
		completed := getRun(run.ID)
		rec, response = f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(currentT, rec, 200, response)
		fromEvents := data[agent.EventPage](currentT, response).Run
		one, _ := json.Marshal(completed.ContractBody)
		two, _ := json.Marshal(fromEvents.ContractBody)
		if string(one) != string(two) || fromEvents.ContractBody == nil || fromEvents.ContractBody.File.Content != body {
			currentT.Fatal("event-log did not preserve the complete exact contract body")
		}
		return completed
	}
	const body = "# Website delivery contract — review draft\n\n1. Contract facts: HKD 12,000; 2026-11-01 to 2027-10-31.\n2. Deliverables: implementation and handover; acceptance details require human confirmation.\n3. Payment: no payment is scheduled by this draft; the authorized contract payment schedule applies.\n4. Renewal: only on explicit human agreement.\n5. Confidentiality: both parties protect project material.\n6. Termination: subject to reviewed contract text.\n7. Signatures: unsigned review draft; names and signing authority require human confirmation.\n\nEND OF COMPLETE BODY\n"
	var browserRun agent.Run
	t.Run("complete_body_exact_file_human_adoption_and_idempotent_receipt", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		run := complete(b, "完整合同正文.md", body+strings.Repeat("Additional reviewed draft clause.\n", 50))
		if run.ContractBody == nil || run.ContractBody.Status != "file_committed" || run.ContractBody.File.Content != body+strings.Repeat("Additional reviewed draft clause.\n", 50) || run.ContractBody.File.Evidence.SHA256 != contentSHA([]byte(run.ContractBody.File.Content)) {
			t.Fatal("complete body lost bytes or falsely adopted")
		}
		input := adoption(run)
		key := core.NewID()
		adopted := adopt(run, key, input, 200)
		if adopted.Status != "adopted" || adopted.Result == nil || adopted.Result.Version != b.Version+1 || adopted.OperationID == nil || adopted.Result.Payload.ContractContent().DraftFileVersionID == nil || *adopted.Result.Payload.ContractContent().DraftFileVersionID != input.VersionID {
			t.Fatal("body was not adopted through a real contract revision")
		}
		beforeContent := b.Content
		afterContent := adopted.Result.Payload.ContractContent()
		beforeContent.DraftFileVersionID = &input.VersionID
		one, _ := json.Marshal(beforeContent)
		two, _ := json.Marshal(afterContent)
		if string(one) != string(two) {
			t.Fatal("adoption changed commercial facts", string(one), string(two))
		}
		replayed := adopt(run, key, input, 200)
		if replayed.OperationID == nil || *replayed.OperationID != *adopted.OperationID || count("SELECT count(*) FROM contract_revisions WHERE contract_id=$1", b.ID) != 2 {
			t.Fatal("adoption replay changed history")
		}
		adopt(run, core.NewID(), input, 409)
		if getRun(run.ID).ContractBody.Status != "adopted" {
			t.Fatal("history became unreadable after adoption")
		}
		rec, response := f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, response)
		history := data[agent.EventPage](t, response).Run.ContractBody
		if history == nil || history.Status != "adopted" || history.OperationID == nil || *history.OperationID != *adopted.OperationID || history.Result.CurrentRevisionID != adopted.Result.CurrentRevisionID {
			t.Fatal("event-log omitted immutable human adoption history")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE agent_contract_body_intents SET input_data='{}' WHERE run_id=$1", run.ID); e == nil {
			t.Fatal("body intent mutable")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM agent_contract_body_adoptions WHERE run_id=$1", run.ID); e == nil {
			t.Fatal("body adoption mutable")
		}
	})
	t.Run("adoption_requires_exact_review_mfa_and_atomic_normal_command", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		run := complete(b, "人工核对合同正文.md", body)
		input := adoption(run)
		wrong := input
		wrong.Reviewed = false
		adopt(run, core.NewID(), wrong, 422)
		wrong = input
		wrong.VersionID = base.Version.ID
		adopt(run, core.NewID(), wrong, 409)
		wrong = input
		wrong.ContentSHA256 = strings.Repeat("e", 64)
		adopt(run, core.NewID(), wrong, 409)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		adopt(run, core.NewID(), input, 403)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		exec(`CREATE FUNCTION owned_contract_body_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'owned receipt rollback'; END; $$; CREATE TRIGGER owned_contract_body_failure BEFORE INSERT ON agent_contract_body_adoptions FOR EACH ROW EXECUTE FUNCTION owned_contract_body_failure()`)
		adopt(run, core.NewID(), input, 500)
		if count("SELECT count(*) FROM contract_revisions WHERE contract_id=$1", b.ID) != 1 || count("SELECT count(*) FROM agent_contract_body_adoptions WHERE run_id=$1", run.ID) != 0 {
			t.Fatal("failed body receipt left a business revision")
		}
		exec("DROP TRIGGER owned_contract_body_failure ON agent_contract_body_adoptions; DROP FUNCTION owned_contract_body_failure()")
		adopt(run, core.NewID(), input, 200)
	})
	t.Run("stale_contract_blocks_adoption_without_discarding_formal_file", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		run := complete(b, "过期依据合同正文.md", body)
		content := b.Content
		content.Notes = "Human changed contract facts after drafting"
		r, v := f.request("PATCH", "/v1/contracts/"+b.ID, f.aliceToken, core.NewID(), contracts.Patch{Content: content, ExpectedVersion: b.Version, BaseRevisionID: b.CurrentRevisionID, Reason: "Actual concurrent human revision"}, nil)
		expect(t, r, 200, v)
		adopt(run, core.NewID(), adoption(run), 409)
		if count("SELECT count(*) FROM file_versions WHERE id=$1", run.ContractBody.File.Evidence.VersionID) != 1 || getRun(run.ID).ContractBody.Status != "file_committed" {
			t.Fatal("stale adoption erased file history")
		}
	})
	t.Run("stale_basis_stops_runtime_before_file_write", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		selectContract(b)
		run, lease := start("执行前变更合同.md")
		content := b.Content
		content.Notes = "Changed while the run is active"
		r, v := f.request("PATCH", "/v1/contracts/"+b.ID, f.aliceToken, core.NewID(), contracts.Patch{Content: content, ExpectedVersion: b.Version, BaseRevisionID: b.CurrentRevisionID, Reason: "Human revision during execution"}, nil)
		expect(t, r, 200, v)
		if _, code := tool(core.NewID(), "stage_file_create", map[string]any{"folder_id": folder.ID, "name": "执行前变更合同.md", "content": body}); code != "RUNTIME_SOURCE_CHANGED" {
			t.Fatal("runtime accepted stale commercial facts", code)
		}
		if count("SELECT count(*) FROM agent_file_drafts WHERE run_id=$1", run.ID) != 0 {
			t.Fatal("stale contract staged a file")
		}
		stop(lease)
	})
	t.Run("unbound_history_and_other_intents_do_not_grant_contract_authority", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		selectContract(b)
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Invalid body scope", ContractBody: selectedContract}
		r, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		in.FileCreate = &agent.FileCreateInput{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: "范围验证.md", MediaType: "text/markdown"}
		in.ContractBody.ContentHash = strings.Repeat("f", 64)
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in.ContractBody = nil
		in.ContextRefs = []agent.SourceReference{{Type: "agent_contract_body", ID: core.NewID()}}
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
	})
	// A fresh, unadopted completed file is the actual browser review target.
	t.Run("current_contract_read_authority_hides_full_body_and_blocks_adoption", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		run := complete(b, "当前权限合同正文.md", body)
		exec(`UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='contract_manager')`, f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		r, v := f.request("GET", "/v1/runs/"+run.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 404, v)
		adopt(run, core.NewID(), adoption(run), 404)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='contract_manager'`, core.NewID(), f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		if getRun(run.ID).ContractBody.Status != "file_committed" {
			t.Fatal("restored reader cannot inspect preserved history")
		}
	})
	t.Run("stopped_run_cannot_adopt_even_when_file_is_already_committed", func(t *testing.T) {
		previous := currentT
		currentT = t
		f.t = t
		defer func() { currentT = previous; f.t = previous }()
		b := createContract()
		selectContract(b)
		run, lease := start("停止运行合同正文.md")
		view := stage("停止运行合同正文.md", body)
		args := propose("停止运行合同正文.md", view)
		scan()
		if _, code := tool(core.NewID(), "commit_file_create", args); code != "" {
			t.Fatal(code)
		}
		stop(lease)
		run = getRun(run.ID)
		adopt(run, core.NewID(), adoption(run), 404)
		if count("SELECT count(*) FROM contract_revisions WHERE contract_id=$1", b.ID) != 1 {
			t.Fatal("stopped run adopted its file")
		}
	})
	if os.Getenv("CP_BROWSER_CONTRACT_BODY_FIXTURE") == "1" {
		b := createContract()
		browserRun = complete(b, "待本人采用的完整合同.md", body+strings.Repeat("8. Annex: additional terms require explicit human confirmation.\n", 20)+"END OF COMPLETE BODY\n")
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "contract-body-drafting", "contract_body_fixture": true, "conversation_id": c.ID, "project_id": *base.File.ProjectID, "folder_id": folder.ID, "profile_id": profile, "completed_run_id": browserRun.ID, "contract_id": b.ID})
	}
}
