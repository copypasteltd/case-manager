package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"os"
	"testing"
)

func TestRound161BrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_BATCH161_FIXTURE") != "1" {
		t.Skip("explicit isolated browser fixture only")
	}
	u, original, current, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	p := f.createProject("轮次161协作与完整列表", []string{f.bob})
	task := f.createTask(p, "轮次161准确协作任务", false)
	for range 25 {
		f.createTask(p, "后续可选任务", false)
	}
	for _, key := range []string{"agent_user", "agent_plan_operator"} {
		if _, e := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", core.NewID(), f.org, f.alice, key); e != nil {
			t.Fatal(e)
		}
	}
	profile := core.NewID()
	if _, e := f.owner.Exec(ctx, `INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'information-organize','round161-browser',repeat('a',64),$3,repeat('b',64),'active',20000,2000,30,600,$4,'synthetic view only')`, profile, f.org, agentworker.RuntimeVersion, f.admin); e != nil {
		t.Fatal(e)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	r, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "轮次161固定授权入口", MemberIDs: []string{}}, nil)
	expect(t, r, 201, v)
	c := data[agent.Conversation](t, v)
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch161-functional-wave", "project_id": p.ID, "task_id": task.ID, "file_id": current.File.ID, "source_version_id": original.Version.ID, "conversation_id": c.ID, "profile_id": profile})
}
