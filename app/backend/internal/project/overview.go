package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"time"
)

type Dashboard struct {
	AsOf           time.Time `json:"as_of"`
	ActiveProjects int       `json:"active_projects"`
	MyOpenTasks    int       `json:"my_open_tasks"`
	MyOverdueTasks int       `json:"my_overdue_tasks"`
	Tasks          []Task    `json:"tasks"`
}

func (s *Service) Dashboard(ctx context.Context, a core.Actor) (Dashboard, error) {
	out := Dashboard{AsOf: time.Now().UTC(), Tasks: []Task{}}
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM projects p WHERE p.org_id=$1 AND p.state='active'
		AND can_access(p.org_id,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,$4)`, a.OrgID, a.UserID, a.Channel, a.RunID != "").Scan(&out.ActiveProjects)
	if err != nil {
		return out, err
	}
	err = s.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE t.due_at<now()) FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
        WHERE t.org_id=$1 AND t.assignee_id=$2 AND t.state NOT IN ('done','cancelled') AND p.state NOT IN ('archived','cancelled')
		AND can_access(p.org_id,$2,'task.read','task',t.id,p.id,t.creator_id,p.classification,$3,$4)`, a.OrgID, a.UserID, a.Channel, a.RunID != "").Scan(&out.MyOpenTasks, &out.MyOverdueTasks)
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, "SELECT "+taskColumns+` FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
        WHERE t.org_id=$1 AND t.assignee_id=$2 AND t.state NOT IN ('done','cancelled') AND p.state NOT IN ('archived','cancelled')
		AND can_access(p.org_id,$2,'task.read','task',t.id,p.id,t.creator_id,p.classification,$3,$4)
        ORDER BY t.due_at ASC NULLS LAST,t.updated_at DESC,t.id LIMIT 8`, a.OrgID, a.UserID, a.Channel, a.RunID != "")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return out, err
		}
		out.Tasks = append(out.Tasks, task)
	}
	return out, rows.Err()
}
func (s *Service) Members(ctx context.Context, a core.Actor, id string) (map[string]any, error) {
	if err := authorizeProject(ctx, s.DB, a, id, "project.read"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT u.id,u.version,u.created_at,u.updated_at,u.display_name,u.status,pm.project_role,pm.authorization_version
        FROM project_members pm JOIN users u ON u.org_id=pm.org_id AND u.id=pm.user_id
        WHERE pm.org_id=$1 AND pm.project_id=$2 ORDER BY u.display_name,u.id`, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []map[string]any{}
	memberships := []map[string]any{}
	for rows.Next() {
		var id, name, status string
		var version int64
		var role string
		var authorizationVersion int64
		var created, updated time.Time
		if err = rows.Scan(&id, &version, &created, &updated, &name, &status, &role, &authorizationVersion); err != nil {
			return nil, err
		}
		users = append(users, map[string]any{"id": id, "version": version, "created_at": created, "updated_at": updated, "display_name": name, "status": status})
		memberships = append(memberships, map[string]any{"user_id": id, "project_role": role, "authorization_version": authorizationVersion})
	}
	return map[string]any{"items": users, "memberships": memberships}, rows.Err()
}

type Predecessor struct {
	ID    string `json:"predecessor_id"`
	Title string `json:"title"`
	State string `json:"state"`
}

func (s *Service) Dependencies(ctx context.Context, a core.Actor, id string) (map[string]any, error) {
	if err := authorizeTask(ctx, s.DB, a, id, "task.read"); err != nil {
		return nil, err
	}
	task, err := getTask(ctx, s.DB, a.OrgID, id, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT p.id,p.title,p.state FROM task_dependencies d JOIN tasks p ON p.org_id=d.org_id AND p.id=d.predecessor_id
        JOIN projects project ON project.org_id=p.org_id AND project.id=p.project_id
		WHERE d.org_id=$1 AND d.successor_id=$2 AND d.project_id=$3
        AND can_access(p.org_id,$4,'task.read','task',p.id,p.project_id,p.creator_id,project.classification,$5,$6)
        ORDER BY p.title,p.id`, a.OrgID, id, task.ProjectID, a.UserID, a.Channel, a.RunID != "")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Predecessor{}
	for rows.Next() {
		var edge Predecessor
		if err = rows.Scan(&edge.ID, &edge.Title, &edge.State); err != nil {
			return nil, err
		}
		items = append(items, edge)
	}
	return map[string]any{"items": items}, rows.Err()
}
