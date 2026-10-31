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

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

func TestAgentProjectTools(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, user := range []string{f.alice, f.bob} {
		for _, role := range []string{"agent_user", "agent_plan_operator"} {
			exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", core.NewID(), f.org, user, role)
		}
	}
	exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
	exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
	for _, level := range []string{"internal", "project_restricted"} {
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,$3,true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3", core.NewID(), f.org, level)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-create','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated project tool fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Synthetic project tool chain", MemberIDs: []string{}}, nil)
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
			t.Fatalf("owned bridge returned %d, want %d", response.StatusCode, status)
		}
		return out
	}
	start := func(refs ...agent.SourceReference) (agent.Run, agent.ExecutionLease) {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Create only the exact project intent after my interactive approval.", ContextRefs: refs}, nil)
		expect(t, rec, 202, res)
		r := data[agent.Run](t, res)
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "project.tools.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil {
			t.Fatal(e)
		}
		if len(refs) > 0 {
			var prepared struct {
				Sources []agent.SourceSnapshot `json:"source_refs"`
			}
			if json.Unmarshal([]byte(x.PreparedContext), &prepared) != nil || len(prepared.Sources) != len(refs) || prepared.Sources[0].ID != refs[0].ID || prepared.Sources[0].SHA256 == "" {
				t.Fatal("selected plan was not bound into prepared execution context")
			}
		}
		lease, e := service.LeaseForCredential(ctx, x.Credential)
		if e != nil {
			t.Fatal(e)
		}
		lease.Job = *j
		credentials[lease.RunID] = x.Credential
		native := agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/search_resources", "company/inspect_operation"}}
		post(lease, agentworker.ControlPath+"confinement", native, 409)
		native.ExpectedTools = append(native.ExpectedTools, "company/propose_project", "company/create_project", "company/list_project_catalog")
		post(lease, agentworker.ControlPath+"confinement", native, 200)
		if e = service.StartExecution(ctx, lease); e != nil {
			t.Fatal(e)
		}
		return r, lease
	}
	tool := func(l agent.ExecutionLease, key, name string, in any) (json.RawMessage, error) {
		response := post(l, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": key, "method": "tools/call", "params": map[string]any{"name": name, "arguments": in}}, 200)
		var rpc struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Output json.RawMessage `json:"structuredContent"`
		}
		if e := json.Unmarshal(response["result"], &rpc); e != nil {
			return nil, e
		}
		if rpc.IsError {
			if len(rpc.Content) != 1 {
				t.Fatal("missing safe MCP error")
			}
			return nil, core.Fail(409, rpc.Content[0].Text, "Controlled MCP denial")
		}
		return rpc.Output, nil
	}
	firstRun, lease := start()
	request := map[string]any{"project": map[string]any{"name": "Synthetic tool project", "scope": "Only bound input", "classification": "internal"}, "reason": "Synthetic project proposal"}
	output, e := tool(lease, "proposal-1", "propose_project", request)
	if e != nil {
		t.Fatal(e)
	}
	var proposed struct {
		Content   agent.Plan `json:"content"`
		Status    string     `json:"status"`
		Committed bool       `json:"committed"`
	}
	if e = json.Unmarshal(output, &proposed); e != nil {
		t.Fatal(e)
	}
	p := proposed.Content
	if p.Project.OwnerID != f.alice || p.ConversationRevisionID != c.CurrentRevisionID || p.ContextSecurityVersion != c.ContextSecurityVersion {
		t.Fatal("tool did not bind current actor and conversation")
	}
	if proposed.Status != "pending" || proposed.Committed || p.OriginRunID == nil || *p.OriginRunID != lease.RunID {
		t.Fatal("model proposal fabricated completion or lost origin")
	}
	repeated, e := tool(lease, "proposal-1", "propose_project", request)
	if e != nil {
		t.Fatal(e)
	}
	var repeatedValue any
	var originalValue any
	json.Unmarshal(repeated, &repeatedValue)
	json.Unmarshal(output, &originalValue)
	a, _ := json.Marshal(repeatedValue)
	b, _ := json.Marshal(originalValue)
	if string(a) != string(b) {
		t.Fatal("proposal replay changed")
	}
	wrong := map[string]any{"project": map[string]any{"name": "substituted", "scope": "Only bound input", "classification": "internal"}, "reason": "Synthetic project proposal"}
	if _, e = tool(lease, "proposal-1", "propose_project", wrong); core.PublicError(e).Code != "TOOL_INVOCATION_CONFLICT" {
		t.Fatal("reused tool key accepted changed intent", e)
	}
	if _, e = tool(lease, "injected-context", "propose_project", map[string]any{"project": request["project"], "reason": "Do not accept caller security metadata", "expected_version": c.Version}); core.PublicError(e).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("tool accepted caller-supplied context versions", e)
	}
	commit := func(p agent.Plan) map[string]any {
		return map[string]any{"plan_id": p.ID, "expected_version": p.Version, "plan_hash": p.PlanHash, "reason": "Consume exactly approved grant"}
	}
	if _, e = tool(lease, "unapproved", "create_project", commit(p)); core.PublicError(e).Code != "PLAN_APPROVAL_REQUIRED" {
		t.Fatal("unapproved tool created project", e)
	}
	if _, e = tool(lease, "fake-approval", "approve_plan", commit(p)); core.PublicError(e).Code != "SKILL_TOOL_DENIED" {
		t.Fatal("model acquired approval tool", e)
	}
	result := agent.ProposedResult{Status: "pending", Summary: "Fixed project intent is awaiting your explicit approval.", ResourceRefs: []agent.ResultReference{{Type: "agent_plan", ID: p.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_plan", ID: p.ID}}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: "Approve the exact plan in the conversation."}
	if e = service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{}); e != nil {
		t.Fatal(e)
	}
	// A later task can consume a grant approved between turns, using the same
	// actor/context. The earlier pending response remains an honest historical result.
	rec, res = f.request("POST", "/v1/plans/"+p.ID+"/approve", f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "Human interactive approval"}, nil)
	expect(t, rec, 200, res)
	p = data[agent.Plan](t, res)
	// Discovery starts from the new request's bound conversation, not a caller's
	// actor/conversation identifier or a hidden native conversation resume.
	manualPlan := func(token, actor, conversationID string, class string) agent.Plan {
		rec, res := f.request("GET", "/v1/conversations/"+conversationID, token, "", nil, nil)
		expect(t, rec, 200, res)
		current := data[agent.Conversation](t, res)
		rec, res = f.request("POST", "/v1/conversations/"+conversationID+"/plans", token, core.NewID(), agent.ProjectPlanCreate{ExpectedVersion: current.Version, ContextSecurityVersion: current.ContextSecurityVersion, Project: project.Create{Name: "Synthetic tool project option " + core.NewID(), Scope: "Explicit selected intent only", Type: "general", OwnerID: actor, Classification: class}, Reason: "Fixture plan lookup"}, nil)
		expect(t, rec, 201, res)
		return data[agent.Plan](t, res)
	}
	first := manualPlan(f.aliceToken, f.alice, c.ID, "internal")
	second := manualPlan(f.aliceToken, f.alice, c.ID, "internal")
	otherConversation := func(token string) agent.Conversation {
		rec, res := f.request("POST", "/v1/conversations", token, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Other discovery scope", MemberIDs: []string{}}, nil)
		expect(t, rec, 201, res)
		return data[agent.Conversation](t, res)
	}
	other := manualPlan(f.aliceToken, f.alice, otherConversation(f.aliceToken).ID, "internal")
	bob := manualPlan(f.bobToken, f.bob, otherConversation(f.bobToken).ID, "internal")
	secondRun, nextLease := start(agent.SourceReference{Type: "agent_plan", ID: p.ID})
	lease = nextLease
	type planResult struct {
		Content struct {
			Items []struct {
				ID      string `json:"id"`
				State   string `json:"state"`
				Version int64  `json:"plan_version"`
				Hash    string `json:"plan_hash"`
			} `json:"items"`
			Next       *string    `json:"next_cursor"`
			Text       string     `json:"text"`
			Offset     int        `json:"offset"`
			NextOffset *int       `json:"next_offset"`
			Plan       agent.Plan `json:"plan"`
		} `json:"content"`
		Committed bool    `json:"committed"`
		Operation *string `json:"operation_id"`
	}
	decode := func(raw json.RawMessage) planResult {
		var out planResult
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("invalid discovery output")
		}
		return out
	}
	search := func(key string, args map[string]any) planResult {
		out, err := tool(lease, key, "search_resources", args)
		if err != nil {
			t.Fatal(err)
		}
		return decode(out)
	}
	args := map[string]any{"query": "Synthetic tool project", "resource_types": []string{"agent_plan"}, "limit": 1}
	page := search("plan-page-1", args)
	if len(page.Content.Items) != 1 || page.Content.Next == nil || page.Committed || page.Operation != nil {
		t.Fatal("missing bounded uncommitted plan page")
	}
	seen := map[string]bool{page.Content.Items[0].ID: true}
	cursor := *page.Content.Next
	for i := 0; i < 2; i++ {
		next := map[string]any{"query": "Synthetic tool project", "resource_types": []string{"agent_plan"}, "limit": 1, "cursor": cursor}
		page = search("plan-next-"+string(rune('a'+i)), next)
		if len(page.Content.Items) != 1 || seen[page.Content.Items[0].ID] {
			t.Fatal("pagination skipped or duplicated visible plans")
		}
		seen[page.Content.Items[0].ID] = true
		if page.Content.Next != nil {
			cursor = *page.Content.Next
		}
	}
	if page.Content.Next != nil || !seen[p.ID] || !seen[first.ID] || !seen[second.ID] || seen[other.ID] || seen[bob.ID] {
		t.Fatal("plan discovery crossed actor or conversation scope")
	}
	if _, err := tool(lease, "cursor-filter-change", "search_resources", map[string]any{"query": "different", "resource_types": []string{"agent_plan"}, "limit": 1, "cursor": cursor}); core.PublicError(err).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("cursor accepted another filter", err)
	}
	if _, err := tool(lease, "cursor-tampered", "search_resources", map[string]any{"query": "Synthetic tool project", "resource_types": []string{"agent_plan"}, "limit": 1, "cursor": cursor + "x"}); core.PublicError(err).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("cursor tampering accepted", err)
	}
	all := search("plan-empty-query", map[string]any{"query": "", "resource_types": []string{"agent_plan"}, "plan_states": []string{"approved"}})
	if len(all.Content.Items) != 1 || all.Content.Items[0].ID != p.ID || all.Content.Items[0].Version != 2 || all.Content.Items[0].Hash != p.PlanHash {
		t.Fatal("approved discovery did not return current exact grant")
	}
	for _, outside := range []agent.Plan{other, bob} {
		if items := search("outside-"+outside.ID, map[string]any{"query": outside.ID, "resource_types": []string{"agent_plan"}}).Content.Items; len(items) != 0 {
			t.Fatal("outside plan leaked through exact-id search")
		}
		if _, err := tool(lease, "outside-read-"+outside.ID, "read_resource", map[string]any{"type": "agent_plan", "id": outside.ID}); core.PublicError(err).Code != "RESOURCE_NOT_FOUND" {
			t.Fatal("outside plan intent was read", err)
		}
	}
	raw, err := tool(lease, "inspect-discovered-plan", "inspect_operation", map[string]any{"run_id": lease.RunID, "plan_id": all.Content.Items[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	recovered := decode(raw).Content.Plan
	if current := recovered; current.State != "approved" || current.Version != 2 || current.BlockedReason != nil || current.CanApprove || current.CanCommit {
		t.Fatal("model inspection manufactured human approval controls")
	}
	raw, err = tool(lease, "read-plan-chunk", "read_resource", map[string]any{"type": "agent_plan", "id": p.ID, "max_chars": 80})
	if err != nil {
		t.Fatal(err)
	}
	chunk := decode(raw)
	if len([]rune(chunk.Content.Text)) != 80 || chunk.Content.NextOffset == nil || chunk.Content.Offset != 0 || chunk.Committed {
		t.Fatal("plan intent reader ignored bound")
	}
	raw, err = tool(lease, "read-plan-next", "read_resource", map[string]any{"type": "agent_plan", "id": p.ID, "offset": *chunk.Content.NextOffset, "max_chars": 80})
	if err != nil {
		t.Fatal(err)
	}
	if decode(raw).Content.Offset != 80 {
		t.Fatal("plan chunk continuation lost exact position")
	}
	if _, err = tool(lease, "reject-mixed-plan-search", "search_resources", map[string]any{"query": "project", "resource_types": []string{"agent_plan", "post"}}); core.PublicError(err).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("mixed plan search accepted", err)
	}
	if _, err = tool(lease, "reject-injected-plan-actor", "search_resources", map[string]any{"query": "", "resource_types": []string{"agent_plan"}, "actor_id": f.bob}); core.PublicError(err).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("caller selected plan identity", err)
	}
	output, e = tool(lease, "create-1", "create_project", commit(recovered))

	if e != nil {
		t.Fatal(e)
	}
	var committed struct {
		Content     agent.Plan `json:"content"`
		OperationID *string    `json:"operation_id"`
		Committed   bool       `json:"committed"`
	}
	if e = json.Unmarshal(output, &committed); e != nil {
		t.Fatal(e)
	}
	if !committed.Committed || committed.OperationID == nil || committed.Content.ProjectID == nil {
		t.Fatal("missing actual business operation")
	}
	receipts, e := service.ExecutionReceipts(ctx, lease)
	if e != nil || len(receipts) < 8 || !receipts[len(receipts)-1].Committed || receipts[len(receipts)-1].Tool != "create_project" || *receipts[len(receipts)-1].OperationID != *committed.OperationID {
		t.Fatal("receipt ledger disagrees", e)
	}
	result.Status = "completed"
	result.OperationIDs = []string{*committed.OperationID}
	result.ResourceRefs = append(result.ResourceRefs, agent.ResultReference{Type: "project", ID: *committed.Content.ProjectID})
	forged := result
	forged.OperationIDs = []string{core.NewID()}
	if e = service.CompleteExecution(ctx, lease, forged, agent.ExecutionUsage{}); core.PublicError(e).Code != "RESULT_OPERATION_UNVERIFIED" {
		t.Fatal("fabricated business completion accepted", e)
	}
	if e = service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{}); e != nil {
		t.Fatal(e)
	}
	rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=20", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	messages := data[core.Page[agent.Message]](t, res)
	if len(messages.Items) != 4 {
		t.Fatal("missing two real turns")
	}
	// Cancellation invalidates a proposal even if a client retained its hash.
	_, cancelLease := start()
	if _, err := tool(cancelLease, "old-task-cursor", "search_resources", map[string]any{"query": "Synthetic tool project", "resource_types": []string{"agent_plan"}, "limit": 1, "cursor": cursor}); core.PublicError(err).Code != "REQUIRED_BUSINESS_FIELD" {
		t.Fatal("plan cursor crossed tasks", err)
	}
	output, e = tool(cancelLease, "cancelled-proposal", "propose_project", request)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(output, &proposed); e != nil {
		t.Fatal(e)
	}
	cancelledPlan := proposed.Content
	rec, res = f.request("GET", "/v1/runs/"+cancelLease.RunID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	live := data[agent.Run](t, res)
	rec, res = f.request("POST", "/v1/runs/"+cancelLease.RunID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: live.Version, Action: "cancel", Reason: "Cancel the proposal origin"}, nil)
	expect(t, rec, 200, res)
	rec, res = f.request("POST", "/v1/plans/"+cancelledPlan.ID+"/approve", f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: cancelledPlan.Version, PlanHash: cancelledPlan.PlanHash, Reason: "Try a retained cancelled intent"}, nil)
	expect(t, rec, 409, res)
	if res.Error.Code != "PLAN_ORIGIN_UNAVAILABLE" {
		t.Fatal("cancelled origin remained usable")
	}
	if e = service.FinishStoppedExecution(ctx, cancelLease.Job, "RUNTIME_CANCEL_REQUESTED"); e != nil {
		t.Fatal(e)
	}
	// Discovery can show an honest blocked observation; it cannot revive a
	// cancelled origin or turn a read receipt into permission to write.
	_, blockedLease := start()
	output, e = tool(blockedLease, "find-cancelled-plan", "search_resources", map[string]any{"query": cancelledPlan.ID, "resource_types": []string{"agent_plan"}})
	if e != nil || len(decode(output).Content.Items) != 1 {
		t.Fatal("cancelled plan cannot be inspected", e)
	}
	output, e = tool(blockedLease, "inspect-cancelled-plan", "inspect_operation", map[string]any{"run_id": blockedLease.RunID, "plan_id": cancelledPlan.ID})
	if e != nil {
		t.Fatal(e)
	}
	blocked := decode(output).Content.Plan
	if blocked.BlockedReason == nil || *blocked.BlockedReason != "PLAN_ORIGIN_UNAVAILABLE" {
		t.Fatal("inspection hid the current cancelled-origin block")
	}
	if _, err := tool(blockedLease, "do-not-revive-cancelled-plan", "create_project", commit(blocked)); core.PublicError(err).Code != "PLAN_ORIGIN_UNAVAILABLE" {
		t.Fatal("lookup revived cancelled origin", err)
	}
	pendingResult := agent.ProposedResult{Status: "pending", Summary: "The observed plan is blocked because its source task was cancelled.", ResourceRefs: []agent.ResultReference{{Type: "agent_plan", ID: blocked.ID}}, SourceRefs: []agent.ResultReference{{Type: "agent_plan", ID: blocked.ID}}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: "Use a new explicitly checked intent if the project is still needed."}
	if e = service.CompleteExecution(ctx, blockedLease, pendingResult, agent.ExecutionUsage{}); e != nil {
		t.Fatal(e)
	}
	// A manual intent may exist in a class which is not admitted to AI. Its
	// private header, body, selected context and even an empty-list continuation
	// must not disclose that intent to an execution.
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	hidden := manualPlan(f.aliceToken, f.alice, c.ID, "project_restricted")
	rec, res = f.request("GET", "/v1/plans/"+hidden.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Explicit selected context still needs classification policy.", ContextRefs: []agent.SourceReference{{Type: "agent_plan", ID: hidden.ID}}}, nil)
	expect(t, rec, 404, res)
	otherProfile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 SELECT $1,org_id,'project-briefing',skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,'Isolated incompatible context fixture' FROM agent_runtime_profiles WHERE id=$2`, otherProfile, profile)
	rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: otherProfile, ClientMessageID: core.NewID(), Message: "Other skills cannot consume creation-plan context.", ContextRefs: []agent.SourceReference{{Type: "agent_plan", ID: p.ID}}}, nil)
	expect(t, rec, 422, res)
	_, hiddenLease := start()
	output, e = tool(hiddenLease, "classification-filters-list", "search_resources", map[string]any{"query": hidden.ID, "resource_types": []string{"agent_plan"}})
	if e != nil || len(decode(output).Content.Items) != 0 {
		t.Fatal("AI-disabled intent leaked through search", e)
	}
	for _, name := range []string{"read_resource", "inspect_operation"} {
		args := map[string]any{"type": "agent_plan", "id": hidden.ID}
		if name == "inspect_operation" {
			args = map[string]any{"run_id": hiddenLease.RunID, "plan_id": hidden.ID}
		}
		if _, err := tool(hiddenLease, "classification-denies-"+name, name, args); core.PublicError(err).Code != "RESOURCE_NOT_FOUND" {
			t.Fatal("AI-disabled intent leaked through direct inspection", err)
		}
	}
	var beforePlans, afterPlans int
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_plans WHERE org_id=$1", f.org).Scan(&beforePlans); e != nil {
		t.Fatal(e)
	}
	if _, err := tool(hiddenLease, "classification-rolls-back-proposal", "propose_project", map[string]any{"project": map[string]any{"name": "AI-disabled model proposal", "scope": "must not persist", "classification": "project_restricted"}, "reason": "Rejected classification"}); core.PublicError(err).Code != "RESOURCE_NOT_FOUND" {
		t.Fatal("AI-disabled model proposal escaped policy", err)
	}
	if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_plans WHERE org_id=$1", f.org).Scan(&afterPlans); e != nil || beforePlans != afterPlans {
		t.Fatal("denied AI proposal left an intent behind", e)
	}
	if e = service.CompleteExecution(ctx, hiddenLease, agent.ProposedResult{Status: "awaiting_input", Summary: "The requested intent cannot be processed under current AI data policy.", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{"AI-permitted project intent"}, NextAction: "Review data policy through an authorized manual workflow."}, agent.ExecutionUsage{}); e != nil {
		t.Fatal(e)
	}
	if os.Getenv("CP_BROWSER_AGENT_PLAN_DISCOVERY_FIXTURE") == "1" {
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"plan_fixture": true, "plan_discovery_fixture": true, "plan_conversation_id": c.ID, "plan_id": p.ID, "plan_hash": p.PlanHash, "project_create_profile_id": profile, "incompatible_profile_id": otherProfile})
	}
	exec("UPDATE role_bindings SET revoked_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=20", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	for _, message := range data[core.Page[agent.Message]](t, res).Items {
		boundToCreatedProject := message.RunID == firstRun.ID || message.RunID == secondRun.ID
		if message.Role == "assistant" && boundToCreatedProject && (message.Visibility == "available" || message.Body != "") {
			t.Fatal("withdrawn project authority left secretary content visible")
		}
	}
}
