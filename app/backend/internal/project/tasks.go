package project

import (
	"context"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const taskColumns = `t.id,t.version,t.created_at,t.updated_at,t.project_id,t.title,t.body,t.assignee_id,t.parent_id,t.milestone_id,
	t.start_at,t.due_at,t.priority,t.state,t.review_required,t.completed_at,t.creator_id`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.Version, &t.CreatedAt, &t.UpdatedAt, &t.ProjectID, &t.Title, &t.Body, &t.AssigneeID, &t.ParentID, &t.MilestoneID, &t.StartAt, &t.DueAt, &t.Priority, &t.State, &t.ReviewRequired, &t.CompletedAt, &t.CreatorID)
	return t, platform.DatabaseError(err)
}
func getTask(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Task, error) {
	if !core.ValidID(id) {
		return Task{}, core.Missing()
	}
	query := "SELECT " + taskColumns + " FROM tasks t WHERE t.org_id=$1 AND t.id=$2"
	if lock {
		query += " FOR UPDATE OF t"
	}
	return scanTask(db.QueryRow(ctx, query, org, id))
}
func authorizeTask(ctx context.Context, db platform.DBTX, a core.Actor, id, action string) error {
	t, err := getTask(ctx, db, a.OrgID, id, false)
	if err != nil {
		return err
	}
	p, err := getProject(ctx, db, a.OrgID, t.ProjectID, false)
	if err != nil {
		return err
	}
	return authz.Require(ctx, db, a, action, taskResource(p, t))
}
func taskResource(p Project, t Task) authz.Resource {
	r := resource(p)
	r.Type = "task"
	r.ID = t.ID
	r.OwnerID = &t.CreatorID
	return r
}
func projectWritable(p Project) error {
	if p.State == "archived" || p.State == "cancelled" || p.State == "closing" {
		return core.Fail(409, "PROJECT_READ_ONLY", "当前项目阶段不允许普通业务写入")
	}
	return nil
}
func (s *Service) GetTask(ctx context.Context, a core.Actor, id string) (Task, error) {
	t, err := getTask(ctx, s.DB, a.OrgID, id, false)
	if err != nil {
		return t, err
	}
	err = authorizeTask(ctx, s.DB, a, t.ID, "task.read")
	return t, err
}
func (s *Service) ListTasks(ctx context.Context, a core.Actor, projectID string, f Filter) (core.Page[Task], error) {
	out := core.Page[Task]{Items: []Task{}}
	if err := authorizeProject(ctx, s.DB, a, projectID, "project.read"); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "tasks", projectID, f.State, f.Query)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		cursor, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return out, err
		}
		before = &cursor.UpdatedAt
		beforeID = &cursor.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+taskColumns+` FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
		WHERE t.org_id=$1 AND t.project_id=$2 AND can_access(p.org_id,$3,'task.read','task',t.id,p.id,t.creator_id,p.classification,$4,$10)
		AND ($5='' OR t.state=$5) AND ($6='' OR position(lower($6) IN lower(t.title))>0)
		AND ($7::timestamptz IS NULL OR (t.updated_at,t.id)<($7,$8::uuid)) ORDER BY t.updated_at DESC,t.id DESC LIMIT $9`,
		a.OrgID, projectID, a.UserID, a.Channel, f.State, f.Query, before, beforeID, f.Limit+1, a.RunID != "")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, t)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		t := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: t.ID, UpdatedAt: t.UpdatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	return out, nil
}
func validateTaskFields(t TaskCreate) error {
	if err := core.Name(t.Title); err != nil {
		return core.Invalid("title", "标题需为1至200个字符")
	}
	if len([]rune(t.Body)) > 20000 {
		return core.Invalid("body", "说明过长")
	}
	if t.Priority != "low" && t.Priority != "normal" && t.Priority != "high" && t.Priority != "urgent" {
		return core.Invalid("priority", "优先级无效")
	}
	if t.StartAt != nil && t.DueAt != nil && t.DueAt.Before(*t.StartAt) {
		return core.Invalid("due_at", "截止时间不能早于开始时间")
	}
	for _, id := range []*string{t.AssigneeID, t.ParentID, t.MilestoneID} {
		if id != nil && !core.ValidID(*id) {
			return core.Invalid("reference", "关联标识无效")
		}
	}
	return nil
}
func validAssignee(ctx context.Context, db platform.DBTX, org, projectID string, assignee *string) error {
	if assignee == nil {
		return nil
	}
	var valid bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN users u ON u.org_id=pm.org_id AND u.id=pm.user_id
		WHERE pm.org_id=$1 AND pm.project_id=$2 AND pm.user_id=$3 AND u.status='active')`, org, projectID, *assignee).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return core.Invalid("assignee_id", "负责人需为本项目的有效成员")
	}
	return nil
}
func validParent(ctx context.Context, db platform.DBTX, org, projectID string, parent *string) error {
	if parent == nil {
		return nil
	}
	task, err := getTask(ctx, db, org, *parent, false)
	if err != nil {
		return err
	}
	if task.ProjectID != projectID {
		return core.Missing()
	}
	if task.State == "done" || task.State == "cancelled" {
		return core.Fail(409, "PARENT_TASK_READ_ONLY", "请先重开父任务，再变更其子任务")
	}
	return nil
}
func (s *Service) CreateTask(ctx context.Context, c core.Command, projectID string, input TaskCreate) (core.Result, error) {
	if err := validateTaskFields(input); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := authorizeProject(ctx, tx, c.Actor, projectID, "task.create"); err != nil {
			return err
		}
		if input.AssigneeID != nil && *input.AssigneeID != c.Actor.UserID {
			return authorizeProject(ctx, tx, c.Actor, projectID, "task.assign")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		p, err := getProject(ctx, tx, c.Actor.OrgID, projectID, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		if err = validAssignee(ctx, tx, c.Actor.OrgID, projectID, input.AssigneeID); err != nil {
			return core.Result{}, err
		}
		if err = validMilestone(ctx, tx, c.Actor.OrgID, projectID, input.MilestoneID); err != nil {
			return core.Result{}, err
		}
		if err = validParent(ctx, tx, c.Actor.OrgID, projectID, input.ParentID); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO tasks(id,org_id,project_id,title,body,assignee_id,parent_id,milestone_id,start_at,due_at,priority,state,review_required,creator_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'todo',$12,$13)`, id, c.Actor.OrgID, projectID, strings.TrimSpace(input.Title), input.Body, input.AssigneeID, input.ParentID, input.MilestoneID, input.StartAt, input.DueAt, input.Priority, input.ReviewRequired, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		t, err := getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return taskResult(t, "task.created"), nil
	})
}
func taskResult(t Task, event string) core.Result {
	status := 200
	if event == "task.created" {
		status = 201
	}
	return core.Result{Status: status, Data: t, Event: core.Event{Type: event, ResourceType: "task", ResourceID: t.ID, Version: t.Version, Payload: map[string]any{"task_id": t.ID, "project_id": t.ProjectID}}}
}
func (s *Service) UpdateTask(ctx context.Context, c core.Command, id string, input TaskPatch) (core.Result, error) {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := authorizeTask(ctx, tx, c.Actor, id, "task.update"); err != nil {
			return err
		}
		if input.AssigneeID.Set {
			return authorizeTask(ctx, tx, c.Actor, id, "task.assign")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		current, err := getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, current.ProjectID, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		t, err := getTask(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if t.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if t.State == "done" || t.State == "cancelled" {
			return core.Result{}, core.Fail(409, "TASK_READ_ONLY", "请重开任务后修改")
		}
		if input.Title != nil {
			t.Title = *input.Title
		}
		if input.Body != nil {
			t.Body = *input.Body
		}
		if input.Priority != nil {
			t.Priority = *input.Priority
		}
		if input.ReviewRequired != nil {
			t.ReviewRequired = *input.ReviewRequired
		}
		if input.AssigneeID.Set {
			t.AssigneeID = input.AssigneeID.Value
		}
		if input.ParentID.Set {
			t.ParentID = input.ParentID.Value
		}
		if input.MilestoneID.Set {
			if err = validMilestone(ctx, tx, c.Actor.OrgID, t.ProjectID, t.MilestoneID); err != nil {
				return core.Result{}, err
			}
			t.MilestoneID = input.MilestoneID.Value
		}
		if input.StartAt.Set {
			t.StartAt = input.StartAt.Value
		}
		if input.DueAt.Set {
			t.DueAt = input.DueAt.Value
		}
		fields := TaskCreate{Title: t.Title, Body: t.Body, AssigneeID: t.AssigneeID, ParentID: t.ParentID, MilestoneID: t.MilestoneID, StartAt: t.StartAt, DueAt: t.DueAt, Priority: t.Priority, ReviewRequired: t.ReviewRequired}
		if err = validateTaskFields(fields); err != nil {
			return core.Result{}, err
		}
		if err = validAssignee(ctx, tx, c.Actor.OrgID, t.ProjectID, t.AssigneeID); err != nil {
			return core.Result{}, err
		}
		if err = validMilestone(ctx, tx, c.Actor.OrgID, t.ProjectID, t.MilestoneID); err != nil {
			return core.Result{}, err
		}
		if err = validParent(ctx, tx, c.Actor.OrgID, t.ProjectID, t.ParentID); err != nil {
			return core.Result{}, err
		}
		if t.ParentID != nil {
			var cycle bool
			err = tx.QueryRow(ctx, `WITH RECURSIVE parents(id,parent_id) AS (
				SELECT id,parent_id FROM tasks WHERE org_id=$1 AND project_id=$2 AND id=$3
				UNION SELECT t.id,t.parent_id FROM tasks t JOIN parents p ON t.id=p.parent_id WHERE t.org_id=$1 AND t.project_id=$2
			) SELECT EXISTS(SELECT 1 FROM parents WHERE id=$4)`, c.Actor.OrgID, t.ProjectID, *t.ParentID, id).Scan(&cycle)
			if err != nil {
				return core.Result{}, err
			}
			if cycle {
				return core.Result{}, core.Fail(409, "TASK_PARENT_CYCLE", "父子任务不能形成循环")
			}
		}
		_, err = tx.Exec(ctx, `UPDATE tasks SET title=$3,body=$4,assignee_id=$5,parent_id=$6,milestone_id=$7,start_at=$8,due_at=$9,
			priority=$10,review_required=$11,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`,
			c.Actor.OrgID, id, strings.TrimSpace(t.Title), t.Body, t.AssigneeID, t.ParentID, t.MilestoneID, t.StartAt, t.DueAt, t.Priority, t.ReviewRequired)
		if err != nil {
			return core.Result{}, err
		}
		t, err = getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return taskResult(t, "task.updated"), nil
	})
}
func taskTarget(state, action, resume string, review bool) (string, bool) {
	if action == "block" {
		return "blocked", state != "done" && state != "cancelled" && state != "blocked"
	}
	if action == "cancel" {
		return "cancelled", state != "done" && state != "cancelled"
	}
	if state == "blocked" && action == "resume" {
		return resume, resume != ""
	}
	if state == "done" && action == "reopen" {
		return "in_progress", true
	}
	if state == "cancelled" && action == "reopen" {
		return "todo", true
	}
	switch {
	case state == "todo" && action == "start":
		return "in_progress", true
	case state == "in_progress" && action == "review":
		return "in_review", true
	case state == "in_review" && action == "return":
		return "in_progress", true
	case state == "in_review" && action == "complete":
		return "done", true
	case state == "in_progress" && action == "complete" && !review:
		return "done", true
	}
	return "", false
}
func (s *Service) TransitionTask(ctx context.Context, c core.Command, id string, input Transition) (core.Result, error) {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("reason", "请填写任务变更原因")
	}
	action := "task.transition"
	if input.Action == "reopen" {
		action = "task.reopen"
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeTask(ctx, tx, c.Actor, id, action) }, func(tx pgx.Tx) (core.Result, error) {
		current, err := getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, current.ProjectID, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		t, err := getTask(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if t.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		var resume *string
		if err = validMilestone(ctx, tx, c.Actor.OrgID, t.ProjectID, t.MilestoneID); err != nil {
			return core.Result{}, err
		}
		if err = validParent(ctx, tx, c.Actor.OrgID, t.ProjectID, t.ParentID); err != nil {
			return core.Result{}, err
		}
		if err = tx.QueryRow(ctx, "SELECT resume_state FROM tasks WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id).Scan(&resume); err != nil {
			return core.Result{}, err
		}
		previous := ""
		if resume != nil {
			previous = *resume
		}
		target, ok := taskTarget(t.State, input.Action, previous, t.ReviewRequired)
		if !ok {
			return core.Result{}, core.UnexpectedState(t.State, input.Action)
		}
		if target == "in_progress" || target == "in_review" || target == "done" {
			var blocked bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_dependencies d JOIN tasks predecessor ON predecessor.org_id=d.org_id AND predecessor.id=d.predecessor_id
				WHERE d.org_id=$1 AND d.successor_id=$2 AND predecessor.state<>'done')`, c.Actor.OrgID, id).Scan(&blocked)
			if err != nil {
				return core.Result{}, err
			}
			if blocked {
				return core.Result{}, core.Fail(422, "DEPENDENCY_BLOCKED", "前置任务尚未完成")
			}
		}
		if target == "done" {
			var openChildren bool
			err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM tasks WHERE org_id=$1 AND parent_id=$2 AND state NOT IN ('done','cancelled'))", c.Actor.OrgID, id).Scan(&openChildren)
			if err != nil {
				return core.Result{}, err
			}
			if openChildren {
				return core.Result{}, core.Fail(422, "CHILD_TASKS_OPEN", "请先完成所有未取消子任务")
			}
			if t.ReviewRequired {
				if err = authz.Require(ctx, tx, c.Actor, "task.review", taskResource(p, t)); err != nil {
					return core.Result{}, err
				}
			}
		}
		var completedAt *time.Time
		var blockedReason *string
		var nextResume *string
		if target == "done" {
			at := time.Now().UTC()
			completedAt = &at
		}
		if target == "blocked" {
			blockedReason = &input.Reason
			nextResume = &t.State
		}
		_, err = tx.Exec(ctx, "UPDATE tasks SET state=$3,resume_state=$4,blocked_reason=$5,completed_at=$6,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, target, nextResume, blockedReason, completedAt)
		if err != nil {
			return core.Result{}, err
		}
		t, err = getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		out := taskResult(t, "task.transitioned")
		out.Event.Payload["reason"] = input.Reason
		out.Event.Payload["state"] = target
		return out, nil
	})
}
func (s *Service) Dependency(ctx context.Context, c core.Command, id string, input Dependency) (core.Result, error) {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(input.PredecessorID) || input.PredecessorID == id {
		return core.Result{}, core.Invalid("predecessor_id", "前置任务标识无效")
	}
	if input.Action != "add" && input.Action != "remove" {
		return core.Result{}, core.Invalid("action", "依赖操作无效")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeTask(ctx, tx, c.Actor, id, "task.update") }, func(tx pgx.Tx) (core.Result, error) {
		current, err := getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, current.ProjectID, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		t, err := getTask(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if t.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if t.State == "done" || t.State == "cancelled" {
			return core.Result{}, core.Fail(409, "TASK_READ_ONLY", "请重开任务后修改依赖")
		}
		predecessor, err := getTask(ctx, tx, c.Actor.OrgID, input.PredecessorID, false)
		if err != nil {
			return core.Result{}, err
		}
		if predecessor.ProjectID != t.ProjectID {
			return core.Result{}, core.Invalid("predecessor_id", "前置任务必须属于同一项目")
		}
		if err = authz.Require(ctx, tx, c.Actor, "task.read", taskResource(p, predecessor)); err != nil {
			return core.Result{}, err
		}
		if input.Action == "add" {
			if t.State != "todo" && t.State != "blocked" && predecessor.State != "done" {
				return core.Result{}, core.Fail(409, "DEPENDENCY_BLOCKED", "进行中的任务不能新增未完成前置任务")
			}
			var cycle bool
			err = tx.QueryRow(ctx, `WITH RECURSIVE reachable(id) AS (
				SELECT successor_id FROM task_dependencies WHERE org_id=$1 AND project_id=$2 AND predecessor_id=$3
				UNION SELECT d.successor_id FROM task_dependencies d JOIN reachable r ON d.predecessor_id=r.id WHERE d.org_id=$1 AND d.project_id=$2
			) SELECT EXISTS(SELECT 1 FROM reachable WHERE id=$4)`, c.Actor.OrgID, t.ProjectID, id, input.PredecessorID).Scan(&cycle)
			if err != nil {
				return core.Result{}, err
			}
			if cycle {
				return core.Result{}, core.Fail(409, "DEPENDENCY_CYCLE", "任务依赖不能形成循环")
			}
			_, err = tx.Exec(ctx, "INSERT INTO task_dependencies(org_id,project_id,predecessor_id,successor_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", c.Actor.OrgID, t.ProjectID, input.PredecessorID, id)
		} else {
			_, err = tx.Exec(ctx, "DELETE FROM task_dependencies WHERE org_id=$1 AND predecessor_id=$2 AND successor_id=$3", c.Actor.OrgID, input.PredecessorID, id)
		}
		if err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, "UPDATE tasks SET version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		t, err = getTask(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return taskResult(t, "task.dependencies.changed"), nil
	})
}
