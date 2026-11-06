package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"strings"
	"testing"
)

func TestRound168ContractTaskLinks(t *testing.T) {
	f, file, supplier := businessCostFixture(t)
	ctx := context.Background()
	source := businessCostSourceFixture(t, f, file, supplier, "contract", "100", true)
	rec, res := f.request("GET", "/v1/contracts/"+source.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	contract := data[contracts.Contract](t, res)
	rec, res = f.request("GET", "/v1/contracts/"+contract.ID+"/milestones", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	nodes := data[core.Page[contracts.Milestone]](t, res)
	if len(nodes.Items) != 1 {
		t.Fatal("exact ordinary scheduled node missing")
	}
	node := nodes.Items[0]
	other := f.createProject("168 explicit independent task scope", nil)
	rec, res = f.request("POST", "/v1/projects/"+other.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": "168 delivery prerequisite", "priority": "high", "assignee_id": f.alice, "review_required": false, "due_at": "2026-11-20T10:00:00+08:00"}, nil)
	expect(t, rec, 201, res)
	task := data[project.Task](t, res)
	path := "/v1/contracts/" + contract.ID + "/milestones/" + node.ID + "/task-links"
	body := contracts.MilestoneTaskInput{ContractVersion: contract.Version, ContractRevisionID: *contract.ActiveRevisionID, TaskID: task.ID, TaskVersion: task.Version, Reason: "review two distinct scopes before creating exact link"}
	rec, res = f.request("POST", path, f.aliceToken, core.NewID(), body, nil)
	expect(t, rec, 422, res)
	body.AcknowledgeScopeDifference = true
	key := core.NewID()
	rec, res = f.request("POST", path, f.aliceToken, key, body, nil)
	expect(t, rec, 201, res)
	link := data[contracts.MilestoneTaskEvent](t, res)
	if !link.CrossScope || !link.ScopeDifferenceAcknowledged || link.TaskVersion != task.Version || link.ContractRevisionID != *contract.ActiveRevisionID {
		t.Fatal("cross-scope source receipt changed")
	}
	rec, res = f.request("POST", path, f.aliceToken, key, body, nil)
	expect(t, rec, 201, res)
	if data[contracts.MilestoneTaskEvent](t, res).ID != link.ID {
		t.Fatal("idempotent exact link duplicated")
	}
	rec, res = f.request("POST", path, f.aliceToken, core.NewID(), body, nil)
	expect(t, rec, 409, res)
	rec, res = f.request("GET", path+"?limit=1", f.bobToken, "", nil, nil)
	expect(t, rec, 200, res)
	if len(data[core.Page[contracts.MilestoneTaskEvent]](t, res).Items) != 0 {
		t.Fatal("contract visibility disclosed independently unreadable task")
	}
	rec, res = f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "title": "168 current delivery progress"}, nil)
	expect(t, rec, 200, res)
	current := data[project.Task](t, res)
	rec, res = f.request("GET", path, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	visible := data[core.Page[contracts.MilestoneTaskEvent]](t, res)
	if len(visible.Items) != 1 || visible.Items[0].TaskTitle != task.Title || visible.Items[0].TaskVersion != task.Version || visible.Items[0].CurrentTask == nil || visible.Items[0].CurrentTask.Version != current.Version {
		t.Fatal("history source was relabeled with current progress")
	}
	rec, res = f.request("POST", path+"/"+link.ID+"/revoke", f.aliceToken, core.NewID(), body, nil)
	expect(t, rec, 409, res)
	body.TaskVersion = current.Version
	body.Reason = "explicitly revoke original link against current task"
	rec, res = f.request("POST", path+"/"+link.ID+"/revoke", f.aliceToken, core.NewID(), body, nil)
	expect(t, rec, 201, res)
	revoked := data[contracts.MilestoneTaskEvent](t, res)
	if revoked.SourceLinkID == nil || *revoked.SourceLinkID != link.ID {
		t.Fatal("revocation did not preserve original identity")
	}
	rec, res = f.request("GET", path, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	if len(data[core.Page[contracts.MilestoneTaskEvent]](t, res).Items) != 0 {
		t.Fatal("revoked root remains active")
	}
	rec, res = f.request("GET", path+"?history=true&limit=1", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	history := data[core.Page[contracts.MilestoneTaskEvent]](t, res)
	if history.NextCursor == nil {
		t.Fatal("history pagination incomplete")
	}
	rec, res = f.request("GET", path+"?history=true&limit=1&cursor="+*history.NextCursor, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	if !data[core.Page[contracts.MilestoneTaskEvent]](t, res).Items[0].Revoked {
		t.Fatal("original history root did not show actual revocation")
	}
	body.Reason = "fresh independent human association"
	rec, res = f.request("POST", path, f.aliceToken, core.NewID(), body, nil)
	expect(t, rec, 201, res)
	var unchanged bool
	if err := f.owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contracts WHERE org_id=$1 AND id=$2 AND version=$3) AND EXISTS(SELECT 1 FROM tasks WHERE org_id=$1 AND id=$4 AND version=$5)`, f.org, contract.ID, contract.Version, task.ID, current.Version).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("association changed contract/task facts", err)
	}
	if _, err := f.api.DB.Exec(ctx, "UPDATE contract_milestone_task_events SET reason='rewrite' WHERE id=$1", link.ID); err == nil {
		t.Fatal("immutable association evidence changed")
	}
	profileID := core.NewID()
	if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_skill_manager'`, core.NewID(), f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','synthetic-168',repeat('a',64),'codex-sdk-0.164.0',repeat('b',64),'active',10000,1000,5,300,$3,'isolated metadata fixture, no real installation claim')`, profileID, f.org, f.admin); err != nil {
		t.Fatal(err)
	}
	rec, res = f.request("GET", "/v1/agent/runtime-profiles/"+profileID+"/skill-releases", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	release := data[agent.SkillReleaseDetails](t, res)
	evaluation := &agent.SkillEvaluation{SetVersion: "synthetic-set-168", SetSHA256: strings.Repeat("c", 64), ResultsSHA256: strings.Repeat("d", 64), Reference: "isolated assessment result reference", Passed: 0, Failed: 1, Skipped: 0}
	rec, res = f.request("POST", "/v1/agent/runtime-profiles/"+profileID+"/skill-releases", f.aliceToken, core.NewID(), agent.SkillReleaseInput{ExpectedVersion: 0, ProfileVersion: release.Profile.Item.Version, SkillSHA256: release.Profile.Item.SkillSHA256, ActiveBasisSHA256: release.Profile.Impact.ActiveBasisSHA256, Stage: "pilot", AssessmentRunIDs: []string{}, Evaluation: evaluation, Reviewed: true, Reason: "如实记录合成失败评定；未发布"}, nil)
	expect(t, rec, 200, res)
	if row := data[agent.SkillReleaseDetails](t, res).History[0]; row.Evaluation == nil || row.Evaluation.SetVersion != evaluation.SetVersion || row.Evaluation.Failed != 1 {
		t.Fatal("external assessment evidence not bound to exact release")
	}
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch168-contract-tasklinks", "profile_id": profileID, "increment": 168, "project_id": other.ID, "contract_id": contract.ID, "milestone_id": node.ID, "task_id": task.ID, "task_title": current.Title})
}
