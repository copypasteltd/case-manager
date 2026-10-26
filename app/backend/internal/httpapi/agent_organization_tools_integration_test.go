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
	"copypaste.hk/platform/internal/files"
)

// Actual native PostgreSQL + authenticated MCP/control HTTP. The confinement
// verifier is controlled; no model/native launcher or cloud provider is used.
func TestAgentOrganizationTools(t *testing.T) {
	h := newOrganizationFixture(t)
	f := h.f
	ctx := context.Background()
	target := h.folder("秘书整理目标")
	profile := core.NewID()
	h.exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'information-organize','controlled-test',repeat('a',64),$3,repeat('b',64),'active',20000,8000,30,900,$4,'Isolated organization tool fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	service := &agent.Service{DB: f.api.DB}
	bridge := &agentworker.Bridge{Service: service, Verifier: &controlledNativeVerifier{}, Model: "controlled-model", WorkingDirectory: t.TempDir()}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	var credential string
	rpc := func(path string, body any) map[string]json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		request, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		request.Header.Set("Authorization", "Bearer "+credential)
		request.Header.Set("Content-Type", "application/json")
		response, e := server.Client().Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var out map[string]json.RawMessage
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != 200 {
			var public struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(out["error"], &public)
			t.Fatalf("owned bridge returned %d (%s)", response.StatusCode, public.Code)
		}
		return out
	}
	start := func(refs []agent.SourceReference) (agent.Run, agent.ExecutionLease) {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations/"+h.c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: h.c.Version, ContextSecurityVersion: h.c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "请核对并整理准确资料，等待本人批准。", ContextRefs: refs}, nil)
		expect(t, rec, 202, res)
		run := data[agent.Run](t, res)
		job, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "organization.fixture", agent.ExecutionLeaseDuration)
		if e != nil || job == nil {
			t.Fatal("execution not claimed", e)
		}
		execution, e := service.IssueExecution(ctx, *job)
		if e != nil {
			t.Fatal(e)
		}
		credential = execution.Credential
		lease, e := service.LeaseForCredential(ctx, credential)
		if e != nil {
			t.Fatal(e)
		}
		lease.Job = *job
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/inspect_operation", "company/list_directory", "company/propose_organization", "company/commit_organization"}}
		proof := rpc(agentworker.ControlPath+"confinement", native)
		if len(proof["error"]) > 0 {
			t.Fatal("organization confinement tool set rejected")
		}
		if e = service.StartExecution(ctx, lease); e != nil {
			t.Fatal(e)
		}
		return run, lease
	}
	tool := func(key, name string, in any) (json.RawMessage, string) {
		t.Helper()
		response := rpc(agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": key, "method": "tools/call", "params": map[string]any{"name": name, "arguments": in}})
		var result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Output json.RawMessage `json:"structuredContent"`
		}
		if e := json.Unmarshal(response["result"], &result); e != nil {
			t.Fatal(e)
		}
		if result.IsError {
			if len(result.Content) != 1 {
				t.Fatal("missing safe MCP error")
			}
			return nil, result.Content[0].Text
		}
		return result.Output, ""
	}
	run, lease := start([]agent.SourceReference{})
	t.Run("confinement exposes only the five organization tools", func(t *testing.T) {
		response := rpc(agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": "catalog", "method": "tools/list"})
		var list struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if json.Unmarshal(response["result"], &list) != nil || len(list.Tools) != 5 {
			t.Fatal("unexpected tool catalog")
		}
		expected := map[string]bool{"read_resource": true, "inspect_operation": true, "list_directory": true, "propose_organization": true, "commit_organization": true}
		for _, v := range list.Tools {
			if !expected[v.Name] {
				t.Fatal("unapproved tool exposed")
			}
		}
		if _, code := tool("no-approval-tool", "approve_organization", map[string]any{}); code != "SKILL_TOOL_DENIED" {
			t.Fatal("model approved own plan", code)
		}
		if e := service.ObserveExecution(ctx, lease, "tool.started", "list_directory", ""); e != nil {
			t.Fatal(e)
		}
	})
	var observation agent.SourceReference
	t.Run("bounded listing is immutable evidence and replayable with current ACL", func(t *testing.T) {
		out, code := tool("list-root", "list_directory", map[string]any{"folder_id": h.root})
		if code != "" {
			t.Fatal(code)
		}
		var view struct {
			Content struct {
				Directory files.DirectoryListing `json:"directory"`
				Untrusted bool                   `json:"untrusted_data"`
			} `json:"content"`
			Sources []agent.ResultReference `json:"source_refs"`
		}
		if json.Unmarshal(out, &view) != nil || !view.Content.Untrusted || view.Content.Directory.Folder.ID != h.root || len(view.Content.Directory.Items) == 0 || len(view.Sources) != 1 || view.Sources[0].Type != "agent_directory" {
			t.Fatal("listing lacks exact bounded observation")
		}
		observation = agent.SourceReference{Type: view.Sources[0].Type, ID: view.Sources[0].ID}
		if _, e := f.api.DB.Exec(ctx, "UPDATE agent_directory_snapshots SET payload='{}' WHERE org_id=$1", f.org); e == nil {
			t.Fatal("observation was mutable")
		}
		again, code := tool("list-root", "list_directory", map[string]any{"folder_id": h.root})
		firstJSON, _ := core.CanonicalJSON(out)
		replayJSON, _ := core.CanonicalJSON(again)
		if code != "" || !bytes.Equal(firstJSON, replayJSON) {
			t.Fatal("listing replay changed its historical observation", code)
		}
	})
	result := agent.ProposedResult{Status: "completed", Summary: "This completion must require real business evidence.", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: ""}
	if e := service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{}); core.PublicError(e).Code != "RESULT_COMPLETION_UNVERIFIED" {
		t.Fatal("fabricated completion accepted", e)
	}
	request := map[string]any{"actions": []files.OrganizationAction{{Kind: "file.move", ID: h.file.ID, ExpectedVersion: h.file.Version, TargetFolderID: &target.ID, ExpectedTargetVersion: &target.Version, Name: ptr("秘书已整理.md")}}, "reason": "精确文件整理测试"}
	output, code := tool("propose-exact", "propose_organization", request)
	if code != "" {
		t.Fatal(code)
	}
	var proposed struct {
		Content   agent.OrganizationPlan `json:"content"`
		Status    string                 `json:"status"`
		Committed bool                   `json:"committed"`
	}
	if json.Unmarshal(output, &proposed) != nil {
		t.Fatal("invalid plan projection")
	}
	p := proposed.Content
	if p.OriginRunID == nil || *p.OriginRunID != run.ID || proposed.Status != "pending" || proposed.Committed || p.CanApprove {
		t.Fatal("model proposal gained human approval or completion authority")
	}
	commit := func(p agent.OrganizationPlan) map[string]any {
		return map[string]any{"plan_id": p.ID, "expected_version": p.Version, "plan_hash": p.PlanHash, "reason": "consume only the exact human-approved batch"}
	}
	t.Run("approval is human only and tools cannot rewrite fixed intent", func(t *testing.T) {
		if _, code := tool("before-approval", "commit_organization", commit(p)); code != "PLAN_APPROVAL_REQUIRED" {
			t.Fatal("unapproved batch executed", code)
		}
		if _, code := tool("propose-exact", "propose_organization", map[string]any{"actions": request["actions"], "reason": "changed same invocation"}); code != "TOOL_INVOCATION_CONFLICT" {
			t.Fatal("same invocation accepted changed input", code)
		}
		if _, code := tool("injected-identity", "propose_organization", map[string]any{"actions": request["actions"], "reason": "reject injection", "actor_id": f.bob}); code != "REQUIRED_BUSINESS_FIELD" {
			t.Fatal("caller identity accepted", code)
		}
	})
	p = h.act(p, "approve", 200)
	output, code = tool("commit-exact", "commit_organization", commit(p))
	if code != "" {
		t.Fatal(code)
	}
	var completed struct {
		Content     agent.OrganizationPlan  `json:"content"`
		OperationID *string                 `json:"operation_id"`
		Committed   bool                    `json:"committed"`
		Refs        []agent.ResultReference `json:"resource_refs"`
		Sources     []agent.ResultReference `json:"source_refs"`
	}
	if json.Unmarshal(output, &completed) != nil || !completed.Committed || completed.OperationID == nil || len(completed.Content.Results) != 1 {
		t.Fatal("missing actual committed receipt")
	}
	if replay, code := tool("commit-exact", "commit_organization", commit(p)); code != "" || !sameOrganizationJSON(output, replay) {
		t.Fatal("repeat commit did not preserve the exact receipt", code)
	}
	if _, code := tool("read-approved-plan", "read_resource", map[string]any{"type": "agent_organization_plan", "id": p.ID, "max_chars": 64}); code != "" {
		t.Fatal("plan cannot be read after own mutation", code)
	}
	if _, code := tool("inspect-committed", "inspect_operation", map[string]any{"run_id": run.ID, "plan_id": p.ID}); code != "" {
		t.Fatal("cannot inspect actual organization receipt", code)
	}
	result.OperationIDs = []string{*completed.OperationID}
	result.ResourceRefs = completed.Refs
	result.SourceRefs = completed.Sources
	if e := service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{}); e != nil {
		t.Fatal("real business completion rejected", e)
	}
	_, follow := start([]agent.SourceReference{{Type: "agent_organization_plan", ID: p.ID}, observation})
	if _, code := tool("follow-receipt", "read_resource", map[string]any{"type": "agent_organization_plan", "id": p.ID}); code != "" {
		t.Fatal("follow-up lost exact historical plan source", code)
	}
	var rootVersion int64
	if e := f.owner.QueryRow(ctx, "SELECT version FROM folders WHERE id=$1", h.root).Scan(&rootVersion); e != nil {
		t.Fatal(e)
	}
	candidate, code := tool("stopped-origin-plan", "propose_organization", map[string]any{"actions": []files.OrganizationAction{{Kind: "folder.create", ID: h.root, ExpectedVersion: rootVersion, Name: ptr("中断后不能批准的目录")}}, "reason": "stopped origin authority"})
	if code != "" {
		t.Fatal(code)
	}
	var stopped struct {
		Content agent.OrganizationPlan `json:"content"`
	}
	if json.Unmarshal(candidate, &stopped) != nil {
		t.Fatal("invalid stopped origin fixture")
	}
	if e := service.FinishStoppedExecution(ctx, follow.Job, "RUNTIME_CANCEL_REQUESTED"); e != nil {
		t.Fatal(e)
	}
	h.act(stopped.Content, "approve", 409)
}

func sameOrganizationJSON(a, b []byte) bool {
	left, err := core.CanonicalJSON(a)
	if err != nil {
		return false
	}
	right, err := core.CanonicalJSON(b)
	return err == nil && bytes.Equal(left, right)
}
