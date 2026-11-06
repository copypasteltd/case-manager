package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"encoding/json"
	"testing"
)

func TestRound161FixedScopeTransfer(t *testing.T) {
	h := newOrganizationFixture(t)
	f := h.f
	round161Diagnostics(t, f)
	ctx := context.Background()
	h.exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='system_admin'", core.NewID(), f.org, f.alice)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	targetProject := f.createProject("Exact cross-project target", []string{f.member})
	var target string
	var targetVersion int64
	if e := f.owner.QueryRow(ctx, "SELECT id,version FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, targetProject.ID).Scan(&target, &targetVersion); e != nil {
		t.Fatal(e)
	}
	r, v := f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), files.ScopeTransferCreate{Kind: "file", ResourceID: h.file.ID, ExpectedVersion: h.file.Version, TargetFolderID: target, ExpectedTargetVersion: targetVersion, Reason: "Exact all-item visibility review"}, nil)
	expect(t, r, 201, v)
	p := data[files.ScopeTransfer](t, v)
	profile := core.NewID()
	h.exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'information-organize','round161-fixture',repeat('a',64),$3,repeat('b',64),'active',20000,2000,30,900,$4,'controlled exact transfer only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	r, v = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Human fixed cross-scope instruction", MemberIDs: []string{}}, nil)
	expect(t, r, 201, v)
	c := data[agent.Conversation](t, v)
	input := agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Execute only this fully reviewed ordinary transfer", ScopeTransfer: &agent.ScopeTransferInput{PlanID: p.ID, ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reviewed: true, Authorized: true, Reason: "Approved exact before/after visibility"}}
	bad := input
	wrong := *input.ScopeTransfer
	wrong.PlanHash = core.Digest([]byte("another plan"))
	bad.ScopeTransfer = &wrong
	r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), bad, nil)
	expect(t, r, 409, v)
	r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input, nil)
	expect(t, r, 202, v)
	run := data[agent.Run](t, v)
	if run.FixedDelivery == nil || run.FixedDelivery.Kind != "scope_transfer" {
		t.Fatal("fixed receipt missing")
	}
	service, lease := round161Start(t, f, run.ID)
	args, _ := json.Marshal(map[string]any{"run_id": run.ID, "intent_sha256": run.FixedDelivery.IntentSHA256})
	before := h.count("SELECT authorization_version::int FROM organizations WHERE id=$1", f.org)
	if _, e := service.ExecutionTool(ctx, lease, "preview", "propose_scope_transfer", args); e != nil {
		t.Fatal(e)
	}
	if h.count("SELECT authorization_version::int FROM organizations WHERE id=$1", f.org) != before {
		t.Fatal("proposal moved authority")
	}
	if _, e := service.ExecutionTool(ctx, lease, "commit", "commit_scope_transfer", args); e != nil {
		t.Fatal(e)
	}
	if h.count("SELECT authorization_version::int FROM organizations WHERE id=$1", f.org) != before+1 {
		t.Fatal("ordinary invalidation missing")
	}
	var state string
	var oldStamp int
	if e := f.owner.QueryRow(ctx, "SELECT state,organization_auth_version::int FROM agent_runs WHERE org_id=$1 AND id=$2", f.org, run.ID).Scan(&state, &oldStamp); e != nil || state != "succeeded" || oldStamp != before {
		t.Fatal(state, oldStamp, e)
	}
	if e := service.CompleteExecution(ctx, lease, agent.ProposedResult{}, agent.ExecutionUsage{}); e != nil {
		t.Fatal("exact terminal reconciliation failed", e)
	}
	if _, e := service.ExecutionTool(ctx, lease, "late-read", "read_resource", json.RawMessage(`{}`)); e == nil {
		t.Fatal("old run authority was renewed")
	}
	r, v = f.request("GET", "/v1/runs/"+run.ID, f.aliceToken, "", nil, nil)
	expect(t, r, 200, v)
	current := data[agent.Run](t, v)
	if current.FixedDelivery == nil || string(current.FixedDelivery.Preview) != "null" {
		t.Fatal("historical visibility inventory disclosed after transfer")
	}
	if _, e := f.api.DB.Exec(ctx, "UPDATE agent_scope_transfer_terminals SET new_organization_version=new_organization_version+1 WHERE org_id=$1", f.org); e == nil {
		t.Fatal("terminal receipt mutable")
	}
}
