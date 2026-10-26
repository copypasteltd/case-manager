package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

func TestProjectCollaborationIntegration(t *testing.T) {
	f := setup(t)
	t.Run("membership changes revoke sessions and viewer roles restrict organization permissions", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Membership boundaries", nil)
		rec, result := f.request("GET", "/v1/projects/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		set := map[string]any{"expected_version": p.Version, "user_id": f.bob, "action": "set", "project_role": "viewer", "reason": "仅查阅本项目"}
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/members", f.aliceToken, core.NewID(), set, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		rec, result = f.request("GET", "/v1/me", f.bobToken, "", nil, nil)
		expect(t, rec, 401, result)
		bob := f.login("bob", "android")
		rec, result = f.request("GET", "/v1/projects/"+p.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/tasks", bob.AccessToken, core.NewID(), project.TaskCreate{Title: "Viewer must not write", Priority: "normal"}, nil)
		expect(t, rec, 404, result)
		set["expected_version"] = p.Version
		set["project_role"] = "member"
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/members", f.aliceToken, core.NewID(), set, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		task := f.createTask(p, "Transfer before removal", false)
		rec, result = f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "assignee_id": f.bob}, nil)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		remove := map[string]any{"expected_version": p.Version, "user_id": f.bob, "action": "remove", "reason": "移交完成后移出"}
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/members", f.aliceToken, core.NewID(), remove, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "assignee_id": nil}, nil)
		expect(t, rec, 200, result)
		_, err := f.owner.Exec(context.Background(), `INSERT INTO resource_grants(id,org_id,user_id,resource_type,resource_id,action,classification_ceiling,grantor_id,reason)
			VALUES($1,$2,$3,'task',$4,'task.read','project_restricted',$5,'Explicit exception fixture')`, core.NewID(), f.org, f.bob, task.ID, f.alice)
		if err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/members", f.aliceToken, core.NewID(), remove, nil)
		expect(t, rec, 200, result)
		bob = f.login("bob", "android")
		rec, result = f.request("GET", "/v1/tasks/"+task.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 404, result)
	})
	t.Run("milestone completion requires evidence and freezes linked tasks until explicit reopening", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Evidence-backed milestones", nil)
		rec, result := f.request("POST", "/v1/projects/"+p.ID+"/milestones", f.aliceToken, core.NewID(), map[string]any{"title": "首个交付节点", "owner_id": f.alice, "due_at": time.Now().Add(time.Hour).UTC()}, nil)
		expect(t, rec, 201, result)
		m := data[project.Milestone](t, result)
		action := map[string]any{"expected_version": m.Version, "action": "achieve", "reason": "核验关联完成证据"}
		rec, result = f.request("POST", "/v1/milestones/"+m.ID+"/transition", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "实际关联工作", Priority: "normal", MilestoneID: &m.ID}, nil)
		expect(t, rec, 201, result)
		task := data[project.Task](t, result)
		rec, result = f.request("POST", "/v1/milestones/"+m.ID+"/transition", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 422, result)
		rec, result = f.transition(task, "start", f.aliceToken)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		rec, result = f.transition(task, "complete", f.aliceToken)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		rec, result = f.request("POST", "/v1/milestones/"+m.ID+"/transition", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 200, result)
		m = data[project.Milestone](t, result)
		rec, result = f.transition(task, "reopen", f.aliceToken)
		expect(t, rec, 409, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "不能悄悄改变已达成证据", Priority: "normal", MilestoneID: &m.ID}, nil)
		expect(t, rec, 409, result)
		action["expected_version"] = m.Version
		action["action"] = "reopen"
		rec, result = f.request("POST", "/v1/milestones/"+m.ID+"/transition", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 200, result)
		rec, result = f.transition(task, "reopen", f.aliceToken)
		expect(t, rec, 200, result)
		foreign := f.createProject("Foreign milestone boundary", nil)
		rec, result = f.request("POST", "/v1/projects/"+foreign.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "不能跨项目关联", Priority: "normal", MilestoneID: &m.ID}, nil)
		expect(t, rec, 404, result)
	})
	t.Run("realized risk creates exactly one linked issue and closure requires its resolution", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Risk and issue lifecycle", nil)
		rec, result := f.request("POST", "/v1/projects/"+p.ID+"/risks", f.aliceToken, core.NewID(), map[string]any{"title": "交付依赖延迟", "description": "等待第三方确认", "owner_id": f.alice, "severity": "high", "probability": "medium"}, nil)
		expect(t, rec, 201, result)
		risk := data[project.Concern](t, result)
		rec, result = f.request("GET", "/v1/risks/"+risk.ID, f.memberToken, "", nil, nil)
		expect(t, rec, 404, result)
		key := core.NewID()
		patch := map[string]any{"expected_version": risk.Version, "state": "realized", "reason": "依赖已延误，转问题跟进"}
		rec, result = f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, key, patch, nil)
		expect(t, rec, 200, result)
		risk = data[project.Concern](t, result)
		if risk.IssueID == nil || risk.State != "realized" {
			t.Fatal("risk realization lost its linked issue")
		}
		rec, result = f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, key, patch, nil)
		expect(t, rec, 200, result)
		var count int
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM issues WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("realization duplicated the issue", count, err)
		}
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='issue.created_from_risk'", f.org, *risk.IssueID).Scan(&count); err != nil || count != 1 {
			t.Fatal("linked issue event was not committed exactly once", count, err)
		}
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "activate", "reason": "启动项目"}, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		close := map[string]any{"expected_version": p.Version, "action": "close", "reason": "结项检查"}
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), close, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("GET", "/v1/issues/"+*risk.IssueID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		issue := data[project.Concern](t, result)
		resolve := map[string]any{"expected_version": issue.Version, "state": "resolved", "reason": "解决问题"}
		rec, result = f.request("PATCH", "/v1/issues/"+issue.ID, f.aliceToken, core.NewID(), resolve, nil)
		expect(t, rec, 422, result)
		resolve["resolution"] = "供应商已补交并经核验"
		rec, result = f.request("PATCH", "/v1/issues/"+issue.ID, f.aliceToken, core.NewID(), resolve, nil)
		expect(t, rec, 200, result)
		issue = data[project.Concern](t, result)
		resolve["expected_version"] = issue.Version
		resolve["state"] = "closed"
		rec, result = f.request("PATCH", "/v1/issues/"+issue.ID, f.aliceToken, core.NewID(), resolve, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), close, nil)
		expect(t, rec, 200, result)
	})
	t.Run("nullable schedules clear dates and completed parents cannot acquire new children", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Schedule and parent constraints", nil)
		rec, result := f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "start_date": "2026-11-01", "due_date": "2026-11-02"}, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		rec, result = f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "start_date": nil, "due_date": nil}, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		if p.StartDate != nil || p.DueDate != nil {
			t.Fatal("explicit null did not clear project schedule")
		}
		task := f.createTask(p, "Clear task schedule", false)
		rec, result = f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "start_at": time.Now().UTC(), "due_at": time.Now().Add(time.Hour).UTC()}, nil)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		rec, result = f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": task.Version, "start_at": nil, "due_at": nil}, nil)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		if task.StartAt != nil || task.DueAt != nil {
			t.Fatal("explicit null did not clear task schedule")
		}
		rec, result = f.transition(task, "start", f.aliceToken)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		rec, result = f.transition(task, "complete", f.aliceToken)
		expect(t, rec, 200, result)
		task = data[project.Task](t, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "Cannot change completed parent evidence", Priority: "normal", ParentID: &task.ID}, nil)
		expect(t, rec, 409, result)
	})
	t.Run("risk realization requires issue authority and audit failure rolls back both aggregates", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Atomic and authorized realization", []string{f.bob})
		rec, result := f.request("POST", "/v1/projects/"+p.ID+"/risks", f.aliceToken, core.NewID(), map[string]any{"title": "Atomic realization", "description": "Must not be partially applied", "owner_id": f.alice, "severity": "high"}, nil)
		expect(t, rec, 201, result)
		risk := data[project.Concern](t, result)
		ctx := context.Background()
		role := core.NewID()
		if _, err := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob); err != nil {
			t.Fatal(err)
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'risk_only_editor','Risk update without issue authority')", role, f.org); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"project.read", "risk.read", "risk.update"} {
			if _, err := f.owner.Exec(ctx, "INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.bob, role); err != nil {
			t.Fatal(err)
		}
		bob := f.login("bob", "android")
		patch := map[string]any{"expected_version": risk.Version, "state": "realized", "reason": "Requires both resource authorities"}
		rec, result = f.request("PATCH", "/v1/risks/"+risk.ID, bob.AccessToken, core.NewID(), patch, nil)
		expect(t, rec, 404, result)
		if _, err := f.owner.Exec(ctx, "REVOKE INSERT ON audit_events FROM copypaste_app"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := f.owner.Exec(context.Background(), "GRANT INSERT ON audit_events TO copypaste_app"); err != nil {
				t.Error(err)
			}
		}()
		rec, result = f.request("PATCH", "/v1/risks/"+risk.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 500, result)
		var issues int
		var state string
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM issues WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&issues); err != nil || issues != 0 {
			t.Fatal("failed authorization/audit left a created issue", issues, err)
		}
		if err := f.owner.QueryRow(ctx, "SELECT state FROM risks WHERE org_id=$1 AND id=$2", f.org, risk.ID).Scan(&state); err != nil || state != "open" {
			t.Fatal("failed audit partially realized the risk", state, err)
		}
	})
}
