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
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/uploadworker"
)

func TestAgentFileRecovery(t *testing.T) {
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
	want := []byte("\ufeff香港🙂\r\n新计划🙂\r\n结尾\r\n")
	base, cloud := commitContentFixture(t, u, "跨运行草稿.md", "text/markdown", before, nil)
	reader := &downloadCloud{readFn: cloud.Read}
	observedWriter := &observedAgentDraftWriter{AgentDraftWriter: cloud}
	writer := &storage.AgentDraftBackend{Provider: "oss", Bucket: u.service.Backend.Bucket, Writer: observedWriter}
	f.api.AgentFileReaders = map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: writer.Bucket, Reader: reader}}
	f.api.AgentDraftWriter = writer
	f.handler = f.api.Handler()
	service := &agent.Service{DB: f.api.DB, FileText: &storage.AgentTextService{Backends: f.api.AgentFileReaders}, FileDrafts: writer}
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'file-update','controlled-test',$3,$5,$4,'active',20000,8000,20,900,$6,'Isolated actual file update fixture only')`, profile, f.org, strings.Repeat("a", 64), strings.Repeat("b", 64), agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: base.File.ProjectID, Title: "准确草稿恢复", MemberIDs: []string{}}, nil)
	expect(t, rec, 201, res)
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
	request := func(in agent.RunCreate, status int) agent.Run {
		currentT.Helper()
		in.ExpectedVersion, in.ContextSecurityVersion, in.ProfileID, in.ClientMessageID = c.Version, c.ContextSecurityVersion, profile, core.NewID()
		if in.Message == "" {
			in.Message = "仅恢复选定草稿准确内容，重新核验与扫描后提交；不改变目标或基础版本。"
		}
		rr, rs := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(currentT, rr, status, rs)
		if status != 202 {
			return agent.Run{}
		}
		return data[agent.Run](currentT, rs)
	}
	begin := func(run agent.Run) agent.ExecutionLease {
		currentT.Helper()
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "file-recovery.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			currentT.Fatal("claim", e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil || x.RunID != run.ID {
			currentT.Fatal("issue", e)
		}
		credential = x.Credential
		lease, e := service.LeaseForCredential(ctx, credential)
		if e != nil {
			currentT.Fatal(e)
		}
		lease.Job = *j
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/recover_file_draft", "company/create_file_draft", "company/propose_file_update", "company/commit_file_update", "company/stage_file_create", "company/propose_file_create", "company/commit_file_create"}}
		if proof := rpc(credential, agentworker.ControlPath+"confinement", native); len(proof["error"]) > 0 {
			currentT.Fatal("confinement")
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
	count := func(query string, args ...any) int {
		currentT.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, query, args...).Scan(&n); e != nil {
			currentT.Fatal(e)
		}
		return n
	}
	readView := func(output map[string]json.RawMessage) agent.FileUpdateView {
		currentT.Helper()
		var v agent.FileUpdateView
		if json.Unmarshal(output["content"], &v) != nil || len(v.Drafts) != 1 {
			currentT.Fatal("exact draft view missing")
		}
		return v
	}
	update := func(current storage.CommitResult) agent.RunCreate {
		return agent.RunCreate{FileUpdate: &agent.FileUpdateInput{FileID: current.File.ID, BaseVersionID: current.Version.ID, ExpectedFileVersion: current.File.Version}}
	}
	edit := map[string]any{"file_id": base.File.ID, "base_version_id": base.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 9, Text: "新计划🙂"}}}
	get := func(id string) agent.Run {
		currentT.Helper()
		rr, rs := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(currentT, rr, 200, rs)
		return data[agent.Run](currentT, rs)
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
	scenario := func(name string, fn func()) {
		t.Run(name, func(st *testing.T) { currentT, f.t = st, st; defer func() { currentT, f.t = t, t }(); fn() })
		if t.Failed() {
			t.FailNow()
		}
	}
	var original agent.Run
	var originalLease agent.ExecutionLease
	var originalView agent.FileUpdateView
	var selected agent.FileRecoveryInput
	scenario("save_draft_and_refuse_a_still_running_source", func() {
		original = request(update(base), 202)
		originalLease = begin(original)
		result, code := tool("save-original", "create_file_draft", edit)
		if code != "" {
			currentT.Fatal(code)
		}
		originalView = readView(result)
		selected = agent.FileRecoveryInput{SourceRunID: original.ID, DraftID: originalView.Drafts[0].ID, ContentSHA256: originalView.Drafts[0].ContentSHA256, Reviewed: true}
		request(agent.RunCreate{FileRecovery: &selected}, 409)
		stop(originalLease)
		if get(original.ID).State != "cancelled" {
			currentT.Fatal("old source was not settled")
		}
	})
	scenario("explicit_review_exact_identity_digest_and_single_intent", func() {
		bad := selected
		bad.Reviewed = false
		request(agent.RunCreate{FileRecovery: &bad}, 422)
		bad = selected
		bad.ContentSHA256 = strings.Repeat("f", 64)
		request(agent.RunCreate{FileRecovery: &bad}, 409)
		bad = selected
		bad.DraftID = core.NewID()
		request(agent.RunCreate{FileRecovery: &bad}, 404)
		mixed := update(base)
		mixed.FileRecovery = &selected
		request(mixed, 422)
		rr, rs := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.bobToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "不能借用别人的草稿", FileRecovery: &selected}, nil)
		if rr.Code < 400 {
			currentT.Fatal("another human inherited draft authority", rs)
		}
		if count("SELECT count(*) FROM agent_file_recoveries WHERE org_id=$1", f.org) != 0 {
			currentT.Fatal("rejected selection left provenance")
		}
	})
	var recovery agent.Run
	var recoveryLease agent.ExecutionLease
	var recovered agent.FileUpdateView
	var firstBody string
	scenario("expired_upload_reconstructs_exact_bytes_in_a_fresh_run", func() {
		exec("UPDATE upload_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", selected.DraftID)
		recovery = request(agent.RunCreate{FileRecovery: &selected}, 202)
		if recovery.FileUpdate == nil || recovery.FileUpdate.Recovery == nil || recovery.FileUpdate.Recovery.SourceRunID != original.ID || recovery.FileUpdate.Recovery.Depth != 1 || len(recovery.FileUpdate.Drafts) != 0 {
			currentT.Fatal("new scope/provenance not fixed")
		}
		if !recovery.ExpiresAt.After(time.Now()) {
			currentT.Fatal("old timeout was reused")
		}
		recoveryLease = begin(recovery)
		if _, code := tool("cannot-edit-recovery", "create_file_draft", edit); code != "FILE_RECOVERY_EXACT_BYTES_REQUIRED" {
			currentT.Fatal("model rewrote recovered bytes", code)
		}
		if _, code := tool("wrong-recovery-hash", "recover_file_draft", map[string]any{"run_id": recovery.ID, "recovery_sha256": strings.Repeat("f", 64)}); code != "FILE_RECOVERY_SCOPE_REQUIRED" {
			currentT.Fatal(code)
		}
		if _, code := tool("injected-recovery-content", "recover_file_draft", map[string]any{"run_id": recovery.ID, "recovery_sha256": recovery.FileUpdate.Recovery.RecoverySHA256, "content": "injected"}); code != "REQUIRED_BUSINESS_FIELD" {
			currentT.Fatal("unexpected recovery field accepted", code)
		}
		args := map[string]any{"run_id": recovery.ID, "recovery_sha256": recovery.FileUpdate.Recovery.RecoverySHA256}
		result, code := tool("recover-selected", "recover_file_draft", args)
		if code != "" {
			currentT.Fatal(code)
		}
		recovered = readView(result)
		d := recovered.Drafts[0]
		if d.ID == selected.DraftID || d.ContentSHA256 != contentSHA(want) || d.Diff != originalView.Drafts[0].Diff || d.State != "finalizing" || d.ScanState != "pending" || recovered.OperationID != nil {
			currentT.Fatal("old storage/scan/commit receipt was reused")
		}
		writes := len(cloud.writes)
		if _, code = tool("recover-selected", "recover_file_draft", args); code != "" || len(cloud.writes) != writes {
			currentT.Fatal("replay uploaded duplicate bytes", code)
		}
		if err := f.owner.QueryRow(ctx, "SELECT recovery_body FROM agent_file_recoveries WHERE run_id=$1", recovery.ID).Scan(&firstBody); err != nil {
			currentT.Fatal(err)
		}
	})
	scenario("database_rejects_forged_content_and_mutable_provenance", func() {
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			currentT.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `INSERT INTO agent_file_drafts(id,org_id,run_id,invocation_key,input_sha256,edits,content_sha256,size_bytes,diff,source_fingerprint)
 SELECT $1,org_id,$2,'forged-recovery',input_sha256,'[{"start_char":0,"end_char":1,"text":"bad"}]',content_sha256,size_bytes,diff,source_fingerprint FROM agent_file_drafts WHERE id=$3`, core.NewID(), recovery.ID, selected.DraftID)
		if e == nil {
			currentT.Fatal("database accepted different saved edits")
		}
		if _, e = f.owner.Exec(ctx, "UPDATE agent_file_recoveries SET depth=2 WHERE run_id=$1", recovery.ID); e == nil {
			currentT.Fatal("immutable provenance changed")
		}
		if count("SELECT count(*) FROM information_schema.role_table_grants WHERE grantee='copypaste_app' AND table_name='agent_file_recoveries' AND privilege_type IN ('UPDATE','DELETE','TRUNCATE')") != 0 {
			currentT.Fatal("application may rewrite recovery history")
		}
	})
	scenario("old_draft_and_old_plan_cannot_submit_a_new_run", func() {
		if _, code := tool("cannot-reuse-old-draft", "propose_file_update", map[string]any{"file_id": base.File.ID, "base_version_id": base.Version.ID, "draft_id": selected.DraftID}); code == "" {
			currentT.Fatal("old draft promoted under a new run")
		}
		stop(recoveryLease)
	})
	var chain agent.Run
	var chainLease agent.ExecutionLease
	var chainView agent.FileUpdateView
	var proposal map[string]any
	scenario("second_recovery_preserves_lineage_and_requires_its_own_scan", func() {
		in := agent.FileRecoveryInput{SourceRunID: recovery.ID, DraftID: recovered.Drafts[0].ID, ContentSHA256: selected.ContentSHA256, Reviewed: true}
		chain = request(agent.RunCreate{FileRecovery: &in}, 202)
		chainLease = begin(chain)
		if chain.FileUpdate.Recovery.Depth != 2 {
			currentT.Fatal("lineage was reset")
		}
		result, code := tool("recover-chain", "recover_file_draft", map[string]any{"run_id": chain.ID, "recovery_sha256": chain.FileUpdate.Recovery.RecoverySHA256})
		if code != "" {
			currentT.Fatal(code)
		}
		chainView = readView(result)
		proposal = map[string]any{"file_id": base.File.ID, "base_version_id": base.Version.ID, "draft_id": chainView.Drafts[0].ID}
		result, code = tool("propose-chain", "propose_file_update", proposal)
		if code != "" {
			currentT.Fatal(code)
		}
		chainView = readView(result)
		proposal["plan_hash"] = *chainView.Drafts[0].PlanHash
		pending, code := tool("before-new-scan", "commit_file_update", proposal)
		if code != "" || string(pending["committed"]) != "false" || string(pending["status"]) != "\"pending\"" {
			currentT.Fatal("fresh pipeline was bypassed", code)
		}
		scan(chainView.Drafts[0].ID)
	})
	var final storage.CommitResult
	scenario("formal_commit_and_new_receipt_are_atomic_idempotent_and_exact", func() {
		exec(`CREATE FUNCTION owned_recovery_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.tool='commit_file_update' THEN RAISE EXCEPTION 'owned recovery rollback'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER owned_recovery_receipt_failure BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION owned_recovery_receipt_failure()`)
		if _, code := tool("failed-final-receipt", "commit_file_update", proposal); code == "" {
			currentT.Fatal("receipt failure committed bytes")
		}
		if count("SELECT count(*) FROM file_versions WHERE file_id=$1", base.File.ID) != 1 {
			currentT.Fatal("failed transaction advanced file history")
		}
		exec("DROP TRIGGER owned_recovery_receipt_failure ON agent_tool_receipts; DROP FUNCTION owned_recovery_receipt_failure()")
		result, code := tool("actual-final-commit", "commit_file_update", proposal)
		if code != "" || string(result["committed"]) != "true" {
			currentT.Fatal("recovered formal commit", code)
		}
		chainView = readView(result)
		if json.Unmarshal(chainView.Result, &final) != nil || final.Version.SHA256 != contentSHA(want) || final.Version.BaseVersionID == nil || *final.Version.BaseVersionID != base.Version.ID || final.Version.Source != "agent" || final.Version.Replicas[1].State != "queued" {
			currentT.Fatal("exact formal provenance lost")
		}
		if _, code = tool("repeat-final-commit", "commit_file_update", proposal); code != "" || count("SELECT count(*) FROM file_versions WHERE file_id=$1", base.File.ID) != 2 {
			currentT.Fatal("recovery retry duplicated version", code)
		}
		if e := service.CompleteExecution(ctx, chainLease, agent.ProposedResult{Status: "completed", Summary: "选定准确草稿已提交；第二副本待校验。", ResourceRefs: []agent.ResultReference{{Type: "file", ID: final.File.ID, VersionID: &final.Version.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_file_update", ID: chain.ID}}, OperationIDs: []string{*chainView.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			currentT.Fatal(e)
		}
		old := get(original.ID)
		if old.State != "cancelled" || old.FileUpdate.OperationID != nil || old.FileUpdate.Drafts[0].ID != selected.DraftID {
			currentT.Fatal("old run was continued or rewritten")
		}
		var after string
		if e := f.owner.QueryRow(ctx, "SELECT recovery_body FROM agent_file_recoveries WHERE run_id=$1", recovery.ID).Scan(&after); e != nil || after != firstBody {
			currentT.Fatal("lineage history changed", e)
		}
		request(agent.RunCreate{FileRecovery: &selected}, 409)
		committed := agent.FileRecoveryInput{SourceRunID: chain.ID, DraftID: chainView.Drafts[0].ID, ContentSHA256: selected.ContentSHA256, Reviewed: true}
		request(agent.RunCreate{FileRecovery: &committed}, 409)
	})
	var folder storage.Folder
	var createOriginal agent.Run
	var createSelected agent.FileRecoveryInput
	scenario("new_file_draft_restores_bom_and_crlf_without_overwrite", func() {
		rr, rs := f.request("GET", "/v1/folders/"+final.File.FolderID, f.aliceToken, "", nil, nil)
		expect(currentT, rr, 200, rs)
		folder = data[storage.Folder](currentT, rs)
		name, content := "恢复新建纪要.md", "\ufeff# 香港🙂\r\n恢复明确的新建纪要。\r\n"
		createOriginal = request(agent.RunCreate{FileCreate: &agent.FileCreateInput{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: name, MediaType: "text/markdown"}}, 202)
		lease := begin(createOriginal)
		result, code := tool("original-create", "stage_file_create", map[string]any{"folder_id": folder.ID, "name": name, "content": content})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		createSelected = agent.FileRecoveryInput{SourceRunID: createOriginal.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		newRun := request(agent.RunCreate{FileRecovery: &createSelected}, 202)
		newLease := begin(newRun)
		if newRun.FileCreate == nil || newRun.FileUpdate != nil || newRun.FileCreate.Recovery == nil {
			currentT.Fatal("create recovery became an update")
		}
		result, code = tool("recover-new-create", "recover_file_draft", map[string]any{"run_id": newRun.ID, "recovery_sha256": newRun.FileCreate.Recovery.RecoverySHA256})
		if code != "" {
			currentT.Fatal(code)
		}
		restored := readView(result)
		if restored.Drafts[0].ContentSHA256 != contentSHA([]byte(content)) || !restored.Drafts[0].Diff.UTF8BOM || restored.Drafts[0].Diff.LineEnding != "crlf" {
			currentT.Fatal("new body formatting lost")
		}
		scan(restored.Drafts[0].ID)
		args := map[string]any{"folder_id": folder.ID, "name": name, "draft_id": restored.Drafts[0].ID}
		result, code = tool("propose-new-create", "propose_file_create", args)
		if code != "" {
			currentT.Fatal(code)
		}
		restored = readView(result)
		args["plan_hash"] = *restored.Drafts[0].PlanHash
		result, code = tool("commit-new-create", "commit_file_create", args)
		if code != "" || string(result["committed"]) != "true" {
			currentT.Fatal("new file recovery commit", code)
		}
		var created storage.CommitResult
		committedView := readView(result)
		if json.Unmarshal(committedView.Result, &created) != nil {
			currentT.Fatal("create commit result")
		}
		if e := service.CompleteExecution(ctx, newLease, agent.ProposedResult{Status: "completed", Summary: "准确恢复的新文件已提交。", ResourceRefs: []agent.ResultReference{{Type: "file", ID: created.File.ID, VersionID: &created.Version.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_file_create", ID: newRun.ID}}, OperationIDs: []string{*committedView.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			currentT.Fatal(e)
		}
		if count("SELECT count(*) FROM files WHERE org_id=$1 AND folder_id=$2 AND name=$3", f.org, folder.ID, name) != 1 {
			currentT.Fatal("new file was duplicated")
		}
		request(agent.RunCreate{FileRecovery: &createSelected}, 409)
	})
	scenario("blocked_scan_cannot_be_laundered_by_a_new_request", func() {
		run := request(update(final), 202)
		lease := begin(run)
		result, code := tool("block-original", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "待"}}})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		exec("UPDATE upload_sessions SET scan_state='quarantined',state='blocked' WHERE id=$1", v.Drafts[0].ID)
		in := agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		request(agent.RunCreate{FileRecovery: &in}, 409)
	})
	scenario("cancellation_during_new_upload_never_stages_or_submits", func() {
		run := request(update(final), 202)
		lease := begin(run)
		result, code := tool("cancel-source", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "后"}}})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		in := agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		newRun := request(agent.RunCreate{FileRecovery: &in}, 202)
		newLease := begin(newRun)
		cloud.onWrite = func() {
			cloud.onWrite = nil
			var version int64
			if e := f.owner.QueryRow(ctx, "SELECT version FROM agent_runs WHERE id=$1", newRun.ID).Scan(&version); e != nil {
				currentT.Fatal(e)
			}
			rr, rs := f.request("POST", "/v1/runs/"+newRun.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "action": "cancel", "reason": "恢复上传期间明确取消"}, nil)
			expect(currentT, rr, 200, rs)
		}
		aborts := observedWriter.aborts
		if _, code = tool("cancel-new-stage", "recover_file_draft", map[string]any{"run_id": newRun.ID, "recovery_sha256": newRun.FileUpdate.Recovery.RecoverySHA256}); code != "RUNTIME_CANCEL_REQUESTED" {
			currentT.Fatal(code)
		}
		if count("SELECT count(*) FROM agent_file_stages WHERE run_id=$1", newRun.ID) != 0 || count("SELECT count(*) FROM agent_file_commits WHERE run_id=$1", newRun.ID) != 0 || observedWriter.aborts != aborts+1 {
			currentT.Fatal("cancelled new storage was retained as staged")
		}
		stop(newLease)
	})
	scenario("tool_read_dependency_change_hides_preview_and_prevents_recovery", func() {
		run := request(update(final), 202)
		lease := begin(run)
		if _, code := tool("extra-project-read", "read_resource", map[string]any{"type": "project", "id": *final.File.ProjectID}); code != "" {
			currentT.Fatal(code)
		}
		result, code := tool("dependent-draft", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "依赖"}}})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		_ = get(run.ID)
		rr, rs := f.request("GET", "/v1/projects/"+*final.File.ProjectID, f.aliceToken, "", nil, nil)
		expect(currentT, rr, 200, rs)
		project := data[map[string]any](currentT, rs)
		rr, rs = f.request("PATCH", "/v1/projects/"+*final.File.ProjectID, f.aliceToken, core.NewID(), map[string]any{"expected_version": project["version"], "scope": "原草稿读取后的项目范围已明确修订"}, nil)
		expect(currentT, rr, 200, rs)
		rr, rs = f.request("GET", "/v1/runs/"+run.ID, f.aliceToken, "", nil, nil)
		expect(currentT, rr, 404, rs)
		in := agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		request(agent.RunCreate{FileRecovery: &in}, 409)
	})
	scenario("a_later_quarantine_invalidates_already_queued_recovery", func() {
		run := request(update(final), 202)
		lease := begin(run)
		result, code := tool("late-block-source", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "稍后"}}})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		in := agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		newRun := request(agent.RunCreate{FileRecovery: &in}, 202)
		exec("UPDATE upload_sessions SET scan_state='quarantined',state='blocked' WHERE id=$1", in.DraftID)
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "late-block.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			currentT.Fatal(e)
		}
		if _, e = service.IssueExecution(ctx, *j); e == nil {
			currentT.Fatal("stale original scan state admitted")
		}
		if e = service.FinishStoppedExecution(ctx, *j, "RUNTIME_SOURCE_CHANGED"); e != nil {
			currentT.Fatal(e)
		}
		if count("SELECT count(*) FROM agent_file_drafts WHERE run_id=$1", newRun.ID) != 0 {
			currentT.Fatal("quarantined source was restaged")
		}
	})
	scenario("revoked_human_session_during_upload_reclaims_new_multipart", func() {
		run := request(update(final), 202)
		lease := begin(run)
		result, code := tool("session-source", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "授权"}}})
		if code != "" {
			currentT.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		in := agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: v.Drafts[0].ID, ContentSHA256: v.Drafts[0].ContentSHA256, Reviewed: true}
		newRun := request(agent.RunCreate{FileRecovery: &in}, 202)
		newLease := begin(newRun)
		cloud.onWrite = func() {
			cloud.onWrite = nil
			exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT (recovery_body::jsonb->>'session_id')::uuid FROM agent_file_recoveries WHERE run_id=$1)", newRun.ID)
		}
		aborts := observedWriter.aborts
		if _, code = tool("revoked-session-stage", "recover_file_draft", map[string]any{"run_id": newRun.ID, "recovery_sha256": newRun.FileUpdate.Recovery.RecoverySHA256}); code != "RUNTIME_SOURCE_CHANGED" {
			currentT.Fatal("revoked session retained execution authority", code)
		}
		if observedWriter.aborts != aborts+1 || count("SELECT count(*) FROM agent_file_stages WHERE run_id=$1", newRun.ID) != 0 {
			currentT.Fatal("revoked session completed a new stage")
		}
		if e := service.FinishStoppedExecution(ctx, newLease.Job, "RUNTIME_SOURCE_CHANGED"); e != nil {
			currentT.Fatal(e)
		}
		f.aliceToken = f.login("alice", "android").AccessToken
		if get(newRun.ID).FileUpdate.Recovery == nil {
			currentT.Fatal("new login lost readable historical provenance")
		}
	})
	if os.Getenv("CP_BROWSER_FILE_RECOVERY_FIXTURE") == "1" && !t.Failed() {
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'file_recovery_browser_proof','Actionless local browser proof',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		run := request(update(final), 202)
		lease := begin(run)
		result, code := tool("browser-original", "create_file_draft", map[string]any{"file_id": final.File.ID, "base_version_id": final.Version.ID, "edits": []storage.AgentTextEdit{{Start: 6, End: 7, Text: "确认"}}})
		if code != "" {
			t.Fatal(code)
		}
		v := readView(result)
		stop(lease)
		holdBrowserBusinessFixture(t, f, map[string]any{"file_recovery_fixture": true, "conversation_id": c.ID, "project_id": *final.File.ProjectID, "profile_id": profile, "original_run_id": run.ID, "original_draft_id": v.Drafts[0].ID, "content_sha256": v.Drafts[0].ContentSHA256, "completed_recovery_run_id": chain.ID})
	}
}
