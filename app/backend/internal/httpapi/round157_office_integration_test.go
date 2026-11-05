package httpapi

import (
	"bytes"
	"context"
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

func TestRound157OfficeUpdateAndAbandonment(t *testing.T) {
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
	before, _, _, buildErr := storage.AgentCreateOffice(ctx, storage.OfficeDocument{Kind: "docx", Title: "准确Office更新", Paragraphs: []string{"原有段落", "原件其他资料"}}, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	base, cloud := commitContentFixture(t, u, "原有资料.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", before, nil)
	reader := &downloadCloud{readFn: cloud.Read}
	hooks := &creationHookWriter{AgentDraftWriter: cloud}
	observedWriter := &observedAgentDraftWriter{AgentDraftWriter: hooks}
	writer := &storage.AgentDraftBackend{Provider: "oss", Bucket: u.service.Backend.Bucket, Writer: observedWriter}
	f.api.AgentFileReaders = map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: writer.Bucket, Reader: reader}}
	f.api.AgentDraftWriter = writer
	f.api.Downloads = map[string]*storage.DownloadBackend{"oss": {Provider: "oss", Bucket: writer.Bucket, Reader: reader, Signer: reader}}
	f.handler = f.api.Handler()
	service := &agent.Service{DB: f.api.DB, FileText: &storage.AgentTextService{Backends: f.api.AgentFileReaders}, FileDrafts: writer}
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'file-update','controlled-test',$3,$5,$4,'active',20000,8000,20,900,$6,'Isolated actual file update fixture only')`, profile, f.org, strings.Repeat("a", 64), strings.Repeat("b", 64), agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: base.File.ProjectID, Title: "准确文件新建", MemberIDs: []string{}}, nil)
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
		decodeErr := json.NewDecoder(io.LimitReader(r.Body, 120001)).Decode(&out)
		if decodeErr != nil || r.StatusCode != 200 {
			currentT.Fatalf("file update bridge response invalid: status=%d decode=%v public_error=%s", r.StatusCode, decodeErr, out["error"])
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
			if result.Content[0].Text == "INTERNAL_ERROR" {
				lease, e := service.LeaseForCredential(ctx, credential)
				if e == nil {
					raw, _ := json.Marshal(args)
					_, e = service.ExecutionTool(ctx, lease, "mcp:\""+key+"\"", name, raw)
				}
				currentT.Logf("isolated Office tool diagnostic: %v", e)
			}
			return nil, result.Content[0].Text
		}
		return result.Output, ""
	}
	begin := func(run agent.Run) agent.ExecutionLease {
		currentT.Helper()
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "file-create.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			currentT.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil || x.RunID != run.ID {
			currentT.Fatal(e)
		}
		if !strings.Contains(x.PreparedContext, "authorized_file_update") || strings.Contains(x.PreparedContext, "authorized_file_create") {
			currentT.Fatal("create/update scope confused")
		}
		credential = x.Credential
		lease, e := service.LeaseForCredential(ctx, credential)
		if e != nil {
			currentT.Fatal(e)
		}
		lease.Job = *j
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_office_workspace", "company/read_resource", "company/inspect_operation", "company/recover_file_draft", "company/create_file_draft", "company/propose_file_update", "company/commit_file_update", "company/stage_file_create", "company/propose_file_create", "company/commit_file_create"}}
		if proof := rpc(credential, agentworker.ControlPath+"confinement", native); len(proof["error"]) > 0 {
			currentT.Fatal("creation tool confinement rejected")
		}
		if e = service.StartExecution(ctx, lease); e != nil {
			currentT.Fatal(e)
		}
		return lease
	}
	stop := func(lease agent.ExecutionLease) {
		currentT.Helper()
		var version int64
		var cancelled bool
		if e := f.owner.QueryRow(ctx, "SELECT version,cancel_requested_at IS NOT NULL FROM agent_runs WHERE id=$1", lease.RunID).Scan(&version, &cancelled); e != nil {
			currentT.Fatal(e)
		}
		if !cancelled {
			rr, rs := f.request("POST", "/v1/runs/"+lease.RunID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "action": "cancel", "reason": "保留准确草稿，在新运行中继续处理"}, nil)
			expect(currentT, rr, 200, rs)
		}
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
	scan := func(id string) {
		currentT.Helper()
		for attempt := 0; attempt < 50; attempt++ {
			if count("SELECT count(*) FROM upload_sessions WHERE id=$1 AND state='ready' AND scan_state='clean'", id) == 1 {
				return
			}
			for _, mode := range []string{"control", "verify-small", "promote-small"} {
				runner := &uploadworker.Runner{Service: u.service, Store: cloud, TargetStore: cloud, Owner: "file-recovery." + mode, Mode: mode, Scanner: cleanIntegrityScanner(), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
				if _, e := runner.RunOne(ctx); e != nil {
					currentT.Fatal("fresh recovery pipeline", mode, e)
				}
			}
		}
		currentT.Fatal("new exact draft never passed its own hash/scan/promotion pipeline")
	}

	create := func(file storage.File, version string) agent.Run {
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "只更新选定Office原件的准确文字单位", FileUpdate: &agent.FileUpdateInput{FileID: file.ID, BaseVersionID: version, ExpectedFileVersion: file.Version}}
		rr, rs := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, rr, 202, rs)
		return data[agent.Run](t, rs)
	}
	draft := func(run agent.Run) agent.FileUpdateView {
		result, code := tool(core.NewID(), "read_office_workspace", map[string]any{"file_id": run.FileUpdate.FileID, "base_version_id": run.FileUpdate.BaseVersionID, "offset": 0, "limit": 20})
		if code != "" {
			t.Fatal("read exact Office workspace", code)
		}
		var workspace struct {
			WorkspaceSHA256 string               `json:"workspace_sha256"`
			Units           []storage.OfficeUnit `json:"units"`
			Complete        bool                 `json:"complete"`
		}
		if json.Unmarshal(result["content"], &workspace) != nil || !workspace.Complete || len(workspace.Units) == 0 {
			t.Fatal("complete bounded workspace required")
		}
		var selected storage.OfficeUnit
		for _, unit := range workspace.Units {
			if !unit.ReadOnly {
				selected = unit
				break
			}
		}
		result, code = tool(core.NewID(), "create_file_draft", map[string]any{"file_id": run.FileUpdate.FileID, "base_version_id": run.FileUpdate.BaseVersionID, "office": storage.AgentOfficeEditInput{WorkspaceSHA256: workspace.WorkspaceSHA256, Changes: []storage.OfficeChange{{ID: selected.ID, Kind: selected.Kind, Value: selected.Value + "本轮准确修改🙂"}}}})
		var view agent.FileUpdateView
		if code != "" || json.Unmarshal(result["content"], &view) != nil || len(view.Drafts) != 1 || view.Drafts[0].Diff.Office == nil {
			t.Fatal("actual Office draft", code)
		}
		return view
	}
	run := create(base.File, base.Version.ID)
	lease := begin(run)
	view := draft(run)
	scan(view.Drafts[0].ID)
	args := map[string]any{"file_id": base.File.ID, "base_version_id": base.Version.ID, "draft_id": view.Drafts[0].ID}
	result, code := tool(core.NewID(), "propose_file_update", args)
	if code != "" || json.Unmarshal(result["content"], &view) != nil || view.Drafts[0].PlanHash == nil {
		t.Fatal("actual proposal", code)
	}
	args["plan_hash"] = *view.Drafts[0].PlanHash
	result, code = tool(core.NewID(), "commit_file_update", args)
	if code != "" || json.Unmarshal(result["content"], &view) != nil || view.OperationID == nil {
		t.Fatal("actual formal update", code)
	}
	var committed storage.CommitResult
	if json.Unmarshal(view.Result, &committed) != nil || committed.Version.Seq != 2 || !committed.Committed {
		t.Fatal("formal second version required")
	}
	if err := service.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "已提交准确Office第二版", ResourceRefs: []agent.ResultReference{{Type: "file", ID: committed.File.ID, VersionID: &committed.Version.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_file_update", ID: run.ID}}, OperationIDs: []string{*view.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); err != nil {
		t.Fatal(err)
	}
	second := create(committed.File, committed.Version.ID)
	secondLease := begin(second)
	saved := draft(second)
	stop(secondLease)
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	rr, rs := f.request("GET", "/v1/runs/"+second.ID, f.aliceToken, "", nil, nil)
	expect(t, rr, 200, rs)
	stopped := data[agent.Run](t, rs)
	in := agent.FileAbandonmentInput{ExpectedVersion: stopped.Version, ContentSHA256: saved.Drafts[0].ContentSHA256, Reviewed: true, Reason: "明确永久放弃此准确Office草稿的后续恢复"}
	key := core.NewID()
	path := "/v1/runs/" + second.ID + "/file-drafts/" + saved.Drafts[0].ID + "/abandon-recovery"
	rr, rs = f.request("POST", path, f.aliceToken, key, in, nil)
	expect(t, rr, 200, rs)
	rr, rs = f.request("POST", path, f.aliceToken, key, in, nil)
	expect(t, rr, 200, rs)
	rr, rs = f.request("GET", "/v1/runs/"+second.ID, f.aliceToken, "", nil, nil)
	expect(t, rr, 200, rs)
	if !data[agent.Run](t, rs).FileUpdate.Drafts[0].RecoveryAbandoned {
		t.Fatal("permanent recovery barrier missing from actual receipt")
	}
	recovery := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "尝试恢复已放弃来源应被拒绝", FileRecovery: &agent.FileRecoveryInput{SourceRunID: second.ID, DraftID: saved.Drafts[0].ID, ContentSHA256: saved.Drafts[0].ContentSHA256, Reviewed: true}}
	rr, rs = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), recovery, nil)
	expect(t, rr, 409, rs)
	if os.Getenv("CP_BROWSER_BATCH157_FIXTURE") == "office" {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT gen_random_uuid(),$1,$2,id,'org' FROM roles WHERE org_id=$1 AND key IN('agent_runtime_reader','agent_skill_manager')`, f.org, f.alice)
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch-round-157-office", "controlled_model": true, "conversation_id": c.ID, "project_id": base.File.ProjectID, "profile_id": profile, "file_id": committed.File.ID, "version_id": committed.Version.ID, "stopped_run_id": second.ID})
	}

}
