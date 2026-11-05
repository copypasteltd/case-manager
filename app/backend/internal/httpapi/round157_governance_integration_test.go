package httpapi

import (
	"context"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
)

func TestRound157SkillGovernance(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	profile := core.NewID()
	_, err := f.owner.Exec(ctx, `INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-briefing','round157-synthetic',repeat('a',64),$3,repeat('b',64),'active',10000,2000,10,600,$4,'isolated exact governance fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	admin := f.administratorToken()
	path := "/v1/agent/runtime-profiles/" + profile + "/skill-releases"
	r, v := f.request("GET", path, f.aliceToken, "", nil, nil)
	expect(t, r, 404, v)
	r, v = f.request("GET", path, admin, "", nil, nil)
	expect(t, r, 200, v)
	initial := data[agent.SkillReleaseDetails](t, v)
	if initial.Version != 0 || len(initial.History) != 0 {
		t.Fatal("unassessed installation was mislabeled")
	}
	in := agent.SkillReleaseInput{ExpectedVersion: 0, ProfileVersion: initial.Profile.Item.Version, SkillSHA256: strings.Repeat("a", 64), ActiveBasisSHA256: initial.Profile.Impact.ActiveBasisSHA256, Stage: "published", AssessmentRunIDs: []string{core.NewID()}, Reviewed: true, Reason: "核对固定代码与本人评估"}
	r, v = f.request("POST", path, admin, core.NewID(), in, nil)
	expect(t, r, 409, v)
	in.Stage = "pilot"
	in.AssessmentRunIDs = []string{}
	r, v = f.request("POST", path, admin, core.NewID(), in, nil)
	expect(t, r, 200, v)
	pilot := data[agent.SkillReleaseDetails](t, v)
	if pilot.Version != 1 || pilot.History[0].Stage != "pilot" {
		t.Fatal("pilot decision missing")
	}
	r, v = f.request("POST", path, admin, core.NewID(), in, nil)
	expect(t, r, 409, v)
	in.ExpectedVersion = 1
	in.Stage = "disabled"
	key := core.NewID()
	r, v = f.request("POST", path, admin, key, in, nil)
	expect(t, r, 200, v)
	r, v = f.request("POST", path, admin, key, in, nil)
	expect(t, r, 200, v)
	var enabled bool
	if err = f.api.DB.QueryRow(ctx, "SELECT agent_profile_release_enabled($1,$2)", f.org, profile).Scan(&enabled); err != nil || enabled {
		t.Fatal("explicit disable did not block execution", err)
	}
	if _, err = f.api.DB.Exec(ctx, "UPDATE agent_skill_releases SET reason='rewrite' WHERE org_id=$1 AND profile_id=$2", f.org, profile); err == nil {
		t.Fatal("immutable release history changed")
	}
	r, v = f.request("GET", path+"?before_version=2", admin, "", nil, nil)
	expect(t, r, 200, v)
	if len(data[agent.SkillReleaseDetails](t, v).History) != 1 {
		t.Fatal("history pagination changed current version")
	}
}
