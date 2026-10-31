package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDeliverableSubmissionBrowser(t *testing.T) {
	if os.Getenv("CP_BROWSER_DELIVERABLE_SUBMISSION_FIXTURE") != "1" {
		return
	}
	u, oldFile, _, _, _ := restorationFixture(t, 25)
	f := u.f
	ctx := context.Background()
	headers := map[string]string{}
	request := func(method, path, token, key string, body any, extra map[string]string) (*httptest.ResponseRecorder, response) {
		merged := map[string]string{}
		for k, v := range headers {
			merged[k] = v
		}
		for k, v := range extra {
			merged[k] = v
		}
		return f.request(method, path, token, key, body, merged)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []string{f.alice, f.bob} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, u)
	}
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	h, v := request("GET", "/v1/projects/"+*oldFile.File.ProjectID, f.aliceToken, "", nil, nil)
	expect(t, h, 200, v)
	p := data[project.Project](t, v)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-update','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated exact update fixture only')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	rec, res := request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "Exact project stage conversation", MemberIDs: []string{}}, nil)
	expect(t, rec, 201, res)
	c := data[agent.Conversation](t, res)

	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='project_manager'", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	h, v = f.request("POST", "/v1/projects/"+p.ID+"/deliverables", f.aliceToken, core.NewID(), project.DeliverableCreate{Name: "发布包交付", AcceptanceCriteria: "独立人员核对固定附件及约定结果", OwnerID: f.alice, Reason: "Synthetic submission capture"}, nil)
	expect(t, h, 201, v)
	d := data[project.DeliverableReceipt](t, v)
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "deliverable-submission", "conversation_id": c.ID, "project_id": p.ID, "deliverable_id": d.ID, "profile_id": profile, "file_id": oldFile.File.ID, "file_version_id": oldFile.Version.ID, "file_name": oldFile.File.Name, "file_seq": oldFile.Version.Seq})
}
