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
	"reflect"
	"strconv"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/uploadworker"
)

func TestAgentOfficeCreate(t *testing.T) {
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
	before := []byte("\ufeff香港🙂\r\n旧计划\r\n结尾\r\n")
	base, cloud := commitContentFixture(t, u, "原有资料.md", "text/markdown", before, nil)
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
			return nil, result.Content[0].Text
		}
		return result.Output, ""
	}
	rec, res = f.request("GET", "/v1/folders/"+base.File.FolderID, f.aliceToken, "", nil, nil)
	expect(currentT, rec, 200, res)
	folder := data[storage.Folder](t, res)
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
		if !strings.Contains(x.PreparedContext, "authorized_file_create") || strings.Contains(x.PreparedContext, "authorized_file_update") {
			currentT.Fatal("create/update scope confused")
		}
		credential = x.Credential
		lease, e := service.LeaseForCredential(ctx, credential)
		if e != nil {
			currentT.Fatal(e)
		}
		lease.Job = *j
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/recover_file_draft", "company/read_office_workspace", "company/create_file_draft", "company/propose_file_update", "company/commit_file_update", "company/stage_file_create", "company/propose_file_create", "company/commit_file_create"}}
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

	requestRun := func(in agent.RunCreate) agent.Run {
		currentT.Helper()
		in.ExpectedVersion = c.Version
		in.ContextSecurityVersion = c.ContextSecurityVersion
		in.ProfileID = profile
		in.ClientMessageID = core.NewID()
		in.Message = "按员工明确选定的目录、名称和格式生成真实Office文件。仅处理本次准确范围。"
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(currentT, rec, 202, res)
		run := data[agent.Run](currentT, res)
		if run.FileCreate == nil || run.FileUpdate != nil {
			currentT.Fatal("exact creation scope missing")
		}
		return run
	}
	start := func(name, media string) (agent.Run, agent.ExecutionLease) {
		run := requestRun(agent.RunCreate{FileCreate: &agent.FileCreateInput{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: name, MediaType: media}})
		return run, begin(run)
	}
	readView := func(result map[string]json.RawMessage) agent.FileCreateView {
		currentT.Helper()
		var view agent.FileCreateView
		if json.Unmarshal(result["content"], &view) != nil || len(view.Drafts) != 1 {
			currentT.Fatal("Office candidate view missing")
		}
		return view
	}
	stage := func(name string, document storage.OfficeDocument) agent.FileCreateView {
		result, code := tool(core.NewID(), "stage_file_create", map[string]any{"folder_id": folder.ID, "name": name, "document": document})
		if code != "" {
			currentT.Fatal("Office stage rejected", code)
		}
		return readView(result)
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
	finish := func(run agent.Run, lease agent.ExecutionLease, view agent.FileCreateView) storage.CommitResult {
		currentT.Helper()
		var result storage.CommitResult
		if json.Unmarshal(view.Result, &result) != nil || !result.Committed || view.OperationID == nil || result.Version.Source != "agent" || result.Version.Seq != 1 || result.Version.BaseVersionID != nil {
			currentT.Fatal("actual first-version receipt required")
		}
		if err := service.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "已创建真实Office文件；第二副本仍待校验，请核对完整文件与版式。", ResourceRefs: []agent.ResultReference{{Type: "file", ID: result.File.ID, VersionID: &result.Version.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_file_create", ID: run.ID}}, OperationIDs: []string{*view.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); err != nil {
			currentT.Fatal(err)
		}
		return result
	}
	scenario := func(name string, fn func()) {
		t.Run(name, func(st *testing.T) { currentT, f.t = st, st; defer func() { currentT, f.t = t, t }(); fn() })
		if t.Failed() {
			t.FailNow()
		}
	}
	media := map[string]string{
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
	documents := []storage.OfficeDocument{
		{Kind: "docx", Title: "项目会议纪要", Paragraphs: []string{"香港团队🙂", "已确认交付事项，等待员工核对完整文件。"}, Table: [][]string{{"事项", "责任"}, {"交付清单", "项目负责人"}}},
		{Kind: "xlsx", Sheets: []storage.OfficeSheet{{Name: "月开支", Rows: [][]storage.OfficeCell{{{Kind: "text", Value: "订阅"}, {Kind: "number", Value: "1200.50"}}, {{Kind: "text", Value: "合计"}, {Kind: "formula", Value: "=SUM(B1:B1)"}}}}, {Name: "说明", Rows: [][]storage.OfficeCell{{{Kind: "text", Value: "=literal"}}}}}},
		{Kind: "pptx", Slides: []storage.OfficeSlide{{Title: "项目周报", Lines: []string{"已生成完整交付清单", "需要核对版式"}}, {Title: "下一步", Lines: []string{"由员工独立验收"}}}},
	}
	completed := map[string]string{}
	for _, document := range documents {
		scenario(document.Kind+"_real_binary_scan_commit_and_exact_receipt", func() {
			name := "秘书生成项目资料." + document.Kind
			run, lease := start(name, media[document.Kind])
			view := stage(name, document)
			expected, projection, _, err := storage.AgentCreateOffice(ctx, document, media[document.Kind])
			if err != nil || view.Drafts[0].ContentSHA256 != core.Digest(expected) || view.Drafts[0].SizeBytes != strconv.Itoa(len(expected)) || !reflect.DeepEqual(view.Drafts[0].Diff, projection) {
				currentT.Fatal("generated binary evidence mismatch", err)
			}
			if count("SELECT count(*) FROM files WHERE org_id=$1 AND name=$2", f.org, name) != 0 {
				currentT.Fatal("draft became formal before scan")
			}
			args := propose(name, view)
			pending, code := tool("office-before-scan", "commit_file_create", args)
			if code != "" || string(pending["committed"]) != "false" || string(pending["status"]) != `"pending"` {
				currentT.Fatal("unscanned Office completed", code)
			}
			scan(view.Drafts[0].ID)
			output, code := tool("office-final-commit", "commit_file_create", args)
			if code != "" || string(output["committed"]) != "true" {
				currentT.Fatal("Office commit failed", code)
			}
			view = readView(output)
			var result storage.CommitResult
			if json.Unmarshal(view.Result, &result) != nil {
				currentT.Fatal("formal Office result missing")
			}
			if result.Version.MediaType != media[document.Kind] || result.Version.SHA256 != core.Digest(expected) || result.Version.Replicas[1].State != "queued" {
				currentT.Fatal("formal format/hash or replica truth lost")
			}
			var key string
			if err = f.owner.QueryRow(ctx, "SELECT draft_key FROM upload_sessions WHERE id=$1", view.Drafts[0].ID).Scan(&key); err != nil {
				currentT.Fatal(err)
			}
			if !bytes.Equal(cloud.stored[key][1], expected) {
				currentT.Fatal("actual isolated object bytes differ")
			}
			rec, res := f.request("GET", "/v1/files/"+result.File.ID+"/versions/"+result.Version.ID+"/office", f.aliceToken, "", nil, nil)
			expect(currentT, rec, 200, res)
			ordinary := data[storage.OfficeRead](currentT, res)
			if ordinary.SourceSHA256 != result.Version.SHA256 || ordinary.Workspace.SHA256 != projection.Office.WorkspaceSHA256 {
				currentT.Fatal("ordinary exact-version read did not read real generated bytes")
			}
			// Reconciliation is allowed after the first actual commit, within this run.
			if _, code = tool("office-final-commit", "commit_file_create", args); code != "" {
				currentT.Fatal("exact invocation replay failed", code)
			}
			if count("SELECT count(*) FROM files WHERE org_id=$1 AND name=$2", f.org, name) != 1 || count("SELECT count(*) FROM file_versions WHERE file_id=$1", result.File.ID) != 1 {
				currentT.Fatal("Office creation duplicated")
			}
			finish(run, lease, view)
			rec, res = f.request("GET", "/v1/runs/"+run.ID, f.aliceToken, "", nil, nil)
			expect(currentT, rec, 200, res)
			human := data[agent.Run](currentT, res)
			if human.State != "succeeded" || human.FileCreate == nil || human.FileCreate.Status != "committed" {
				currentT.Fatal("human Office completion receipt unavailable")
			}
			rec, res = f.request("GET", "/v1/runs/"+run.ID, f.bobToken, "", nil, nil)
			if rec.Code != 403 && rec.Code != 404 {
				currentT.Fatal("private completion receipt disclosed to another reader")
			}
			completed[document.Kind] = run.ID
		})
	}
	if os.Getenv("CP_BROWSER_AGENT_OFFICE_CREATE_FIXTURE") == "1" {
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'office_create_browser_proof','Actionless owned browser MFA proof',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		_ = memberMFAToken(f, "alice", "web")
		values := map[string]any{"fixture_kind": "agent-office-create", "controlled_model": true, "conversation_id": c.ID, "project_id": *base.File.ProjectID, "folder_id": folder.ID, "profile_id": profile, "completed_runs": completed}
		holdBrowserBusinessFixture(t, f, values, func(mux *http.ServeMux, nonce string) {
			mux.HandleFunc("POST /__fixture/office-create/complete", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+nonce {
					w.WriteHeader(403)
					return
				}
				var input struct {
					RunID string `json:"run_id"`
				}
				if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil || !core.ValidID(input.RunID) {
					w.WriteHeader(400)
					return
				}
				if count(`SELECT count(*) FROM agent_file_intents i JOIN agent_runs r ON r.org_id=i.org_id AND r.id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2 AND i.actor_id=$3 AND i.conversation_id=$4 AND i.mode='create' AND i.target_data->>'name'='Web生成会议纪要.docx' AND i.target_data->>'media_type'=$5 AND i.target_data->>'folder_id'=$6 AND r.state='queued'`, f.org, input.RunID, f.alice, c.ID, media["docx"], folder.ID) != 1 {
					w.WriteHeader(409)
					return
				}
				run := agent.Run{ID: input.RunID}
				lease := begin(run)
				view := stage("Web生成会议纪要.docx", documents[0])
				args := propose("Web生成会议纪要.docx", view)
				scan(view.Drafts[0].ID)
				output, code := tool("browser-office-create", "commit_file_create", args)
				if code != "" {
					t.Error("owned actual Office process", code)
					w.WriteHeader(500)
					return
				}
				result := finish(run, lease, readView(output))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"run_id": run.ID, "file_id": result.File.ID, "version_id": result.Version.ID, "sha256": result.Version.SHA256})
			})
		})
		return
	}
	scenario("human_selected_format_and_strict_content_shape_prevent_expansion", func() {
		_, lease := start("限定格式.docx", media["docx"])
		before := count("SELECT count(*) FROM agent_file_drafts WHERE org_id=$1", f.org)
		for _, args := range []map[string]any{
			{"folder_id": folder.ID, "name": "另一个.docx", "document": documents[0]},
			{"folder_id": folder.ID, "name": "限定格式.docx", "content": "ambiguous", "document": documents[0]},
			{"folder_id": folder.ID, "name": "限定格式.docx", "content": "not a binary Office file"},
			{"folder_id": folder.ID, "name": "限定格式.docx", "document": documents[1]},
			{"folder_id": folder.ID, "name": "限定格式.docx", "document": map[string]any{"kind": "docx", "paragraphs": []string{"a"}, "content_base64": "UEs="}},
		} {
			if _, code := tool(core.NewID(), "stage_file_create", args); code == "" {
				currentT.Fatal("expanded or ambiguous Office input accepted")
			}
		}
		if count("SELECT count(*) FROM agent_file_drafts WHERE org_id=$1", f.org) != before {
			currentT.Fatal("invalid structured input reserved a draft")
		}
		stop(lease)
	})
	scenario("dangerous_formula_is_rejected_before_any_isolated_upload", func() {
		_, lease := start("受控表格.xlsx", media["xlsx"])
		before := count("SELECT count(*) FROM agent_file_drafts WHERE org_id=$1", f.org)
		bad := storage.OfficeDocument{Kind: "xlsx", Sheets: []storage.OfficeSheet{{Name: "unsafe", Rows: [][]storage.OfficeCell{{{Kind: "formula", Value: "=WEBSERVICE(\"https://invalid.example\")"}}}}}}
		if _, code := tool(core.NewID(), "stage_file_create", map[string]any{"folder_id": folder.ID, "name": "受控表格.xlsx", "document": bad}); code == "" {
			currentT.Fatal("external formula accepted")
		}
		if count("SELECT count(*) FROM agent_file_drafts WHERE org_id=$1", f.org) != before {
			currentT.Fatal("unsafe formula left a draft")
		}
		stop(lease)
	})
	scenario("cancelled_office_draft_recovers_exact_recipe_bytes_with_fresh_scan_and_first_version", func() {
		name := "可恢复的新纪要.docx"
		original, oldLease := start(name, media["docx"])
		oldView := stage(name, documents[0])
		stop(oldLease)
		selected := agent.FileRecoveryInput{SourceRunID: original.ID, DraftID: oldView.Drafts[0].ID, ContentSHA256: oldView.Drafts[0].ContentSHA256, Reviewed: true}
		run := requestRun(agent.RunCreate{FileRecovery: &selected})
		lease := begin(run)
		if _, code := tool("no-rewrite", "stage_file_create", map[string]any{"folder_id": folder.ID, "name": name, "document": documents[0]}); code != "FILE_RECOVERY_EXACT_BYTES_REQUIRED" {
			currentT.Fatal("recovery rewrite accepted", code)
		}
		args := map[string]any{"run_id": run.ID, "recovery_sha256": run.FileCreate.Recovery.RecoverySHA256}
		output, code := tool("recover-office", "recover_file_draft", args)
		if code != "" {
			currentT.Fatal("Office recovery failed", code)
		}
		view := readView(output)
		if view.Drafts[0].ID == oldView.Drafts[0].ID || view.Drafts[0].ContentSHA256 != selected.ContentSHA256 || !reflect.DeepEqual(view.Drafts[0].Diff, oldView.Drafts[0].Diff) || view.Drafts[0].ScanState != "pending" {
			currentT.Fatal("recovery reused an old candidate or changed exact bytes")
		}
		scan(view.Drafts[0].ID)
		commitArgs := propose(name, view)
		output, code = tool("commit-recovered-office", "commit_file_create", commitArgs)
		if code != "" {
			currentT.Fatal(code)
		}
		result := finish(run, lease, readView(output))
		if result.Version.SHA256 != selected.ContentSHA256 || count("SELECT count(*) FROM files WHERE org_id=$1 AND name=$2", f.org, name) != 1 {
			currentT.Fatal("recovered Office did not create exactly one correct file")
		}
	})
	scenario("existing_office_update_has_independent_exact_original_authority", func() {
		var id, version string
		var revision int64
		if err := f.owner.QueryRow(ctx, "SELECT id,current_version_id,version FROM files WHERE org_id=$1 AND name=$2", f.org, "秘书生成项目资料.docx").Scan(&id, &version, &revision); err != nil {
			currentT.Fatal(err)
		}
		in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "明确授权修改此准确Office原件", FileUpdate: &agent.FileUpdateInput{FileID: id, BaseVersionID: version, ExpectedFileVersion: revision}}
		rr, rs := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(currentT, rr, 202, rs)
		accepted := data[agent.Run](currentT, rs)
		if accepted.FileUpdate == nil || accepted.FileCreate != nil || accepted.FileUpdate.FileID != id {
			currentT.Fatal("independent original update authorization missing")
		}
		rr, rs = f.request("POST", "/v1/runs/"+accepted.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: accepted.Version, Action: "cancel", Reason: "结束本场景的准确原件更新授权"}, nil)
		expect(currentT, rr, 200, rs)
	})
	scenario("format_extension_and_same_name_remain_human_scope_constraints", func() {
		for _, input := range []agent.FileCreateInput{
			{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: "错误扩展名.xlsx", MediaType: media["docx"]},
			{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: "秘书生成项目资料.docx", MediaType: media["docx"]},
		} {
			in := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "按准确范围新建", FileCreate: &input}
			rr, rs := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
			if rr.Code != 422 && rr.Code != 409 {
				currentT.Fatal("wrong format or implicit overwrite accepted", rr.Code, rs)
			}
		}
	})
	scenario("authorization_change_during_binary_io_aborts_without_formal_file", func() {
		_, _ = start("撤权中断.docx", media["docx"])
		aborts := observedWriter.aborts
		hooks.after = func() {
			exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		}
		if _, code := tool("revoke-during-office-io", "stage_file_create", map[string]any{"folder_id": folder.ID, "name": "撤权中断.docx", "document": documents[0]}); code == "" {
			currentT.Fatal("authority change during I/O accepted")
		}
		if observedWriter.aborts <= aborts || count("SELECT count(*) FROM files WHERE org_id=$1 AND name=$2", f.org, "撤权中断.docx") != 0 {
			currentT.Fatal("binary write was not aborted without formal effect")
		}
	})
}
