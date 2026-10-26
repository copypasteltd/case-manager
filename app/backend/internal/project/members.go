package project

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

type MemberChange struct {
	ExpectedVersion int64  `json:"expected_version"`
	UserID          string `json:"user_id"`
	Role            string `json:"project_role,omitempty"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}

// Candidate identities are disclosed only to a manager of this specific
// project. No login key, credentials, or unrelated role bindings are returned.
func (s *Service) MemberCandidates(ctx context.Context, a core.Actor, id string) (map[string]any, error) {
	if err := authorizeProject(ctx, s.DB, a, id, "project.manage_members"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT u.id,u.version,u.created_at,u.updated_at,u.display_name,u.status
		FROM users u WHERE u.org_id=$1 AND u.status='active' ORDER BY u.display_name,u.id LIMIT 101`, a.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []identity.User{}
	for rows.Next() {
		var u identity.User
		if err = rows.Scan(&u.ID, &u.Version, &u.CreatedAt, &u.UpdatedAt, &u.DisplayName, &u.Status); err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(items) > 100 {
		return nil, core.Fail(422, "MEMBER_DIRECTORY_TOO_LARGE", "成员目录已超过当前选择器容量，请使用成员账号精确查询")
	}
	return map[string]any{"items": items}, nil
}

func (s *Service) ChangeMember(ctx context.Context, c core.Command, id string, input MemberChange) (core.Result, error) {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(input.UserID) || strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("user_id", "请选择成员并填写变更原因")
	}
	if input.Action != "set" && input.Action != "remove" {
		return core.Result{}, core.Invalid("action", "成员变更操作无效")
	}
	// Membership controls eligibility; business actions continue to require an
	// independently granted role. Owner changes use the project owner command.
	if input.Action == "set" && input.Role != "member" && input.Role != "manager" && input.Role != "viewer" {
		return core.Result{}, core.Invalid("project_role", "项目角色无效")
	}
	c.AuthorizationMutation = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeProject(ctx, tx, c.Actor, id, "project.manage_members") }, func(tx pgx.Tx) (core.Result, error) {
		p, err := getProject(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if p.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		if input.UserID == p.OwnerID {
			return core.Result{}, core.Invalid("user_id", "项目负责人须通过负责人移交操作变更")
		}
		if input.UserID == c.Actor.UserID {
			return core.Result{}, core.Invalid("user_id", "请由其他有权限的项目负责人变更你的项目成员资格")
		}
		if input.Action == "remove" {
			var count int
			err = tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM tasks WHERE org_id=$1 AND project_id=$2 AND assignee_id=$3 AND state NOT IN ('done','cancelled'))+
				(SELECT count(*) FROM milestones WHERE org_id=$1 AND project_id=$2 AND owner_id=$3 AND state IN ('open','missed'))+
				(SELECT count(*) FROM risks WHERE org_id=$1 AND project_id=$2 AND owner_id=$3 AND state IN ('open','mitigating'))+
				(SELECT count(*) FROM issues WHERE org_id=$1 AND project_id=$2 AND owner_id=$3 AND state<>'closed')+
				(SELECT count(*) FROM deliverables WHERE org_id=$1 AND project_id=$2 AND owner_id=$3 AND state<>'accepted')`, c.Actor.OrgID, id, input.UserID).Scan(&count)
			if err != nil {
				return core.Result{}, err
			}
			if count > 0 {
				return core.Result{}, core.Fail(422, "MEMBER_HAS_OPEN_ASSIGNMENTS", "该成员仍负责未闭环事项，请先完成或移交")
			}
			tag, err := tx.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", c.Actor.OrgID, id, input.UserID)
			if err != nil {
				return core.Result{}, err
			}
			if tag.RowsAffected() == 0 {
				return core.Result{}, core.Missing()
			}
			// Explicit exceptions must not silently preserve access after removal.
			if _, err = tx.Exec(ctx, `DELETE FROM resource_grants rg WHERE rg.org_id=$1 AND rg.user_id=$3 AND (
				(rg.resource_type='project' AND rg.resource_id=$2)
				OR (rg.resource_type='task' AND EXISTS(SELECT 1 FROM tasks t WHERE t.org_id=rg.org_id AND t.project_id=$2 AND t.id=rg.resource_id))
				OR (rg.resource_type='milestone' AND EXISTS(SELECT 1 FROM milestones m WHERE m.org_id=rg.org_id AND m.project_id=$2 AND m.id=rg.resource_id))
				OR (rg.resource_type='risk' AND EXISTS(SELECT 1 FROM risks r WHERE r.org_id=rg.org_id AND r.project_id=$2 AND r.id=rg.resource_id))
				OR (rg.resource_type='issue' AND EXISTS(SELECT 1 FROM issues i WHERE i.org_id=rg.org_id AND i.project_id=$2 AND i.id=rg.resource_id))
				OR (rg.resource_type='deliverable' AND EXISTS(SELECT 1 FROM deliverables d WHERE d.org_id=rg.org_id AND d.project_id=$2 AND d.id=rg.resource_id))
				OR (rg.resource_type='post' AND EXISTS(SELECT 1 FROM posts p WHERE p.org_id=rg.org_id AND p.project_id=$2 AND p.id=rg.resource_id))
				OR (rg.resource_type='folder' AND EXISTS(SELECT 1 FROM folders f WHERE f.org_id=rg.org_id AND f.project_id=$2 AND f.id=rg.resource_id))
				OR (rg.resource_type='file' AND EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=rg.org_id AND d.project_id=$2 AND f.id=rg.resource_id))
			)`, c.Actor.OrgID, id, input.UserID); err != nil {
				return core.Result{}, err
			}
			if _, err = tx.Exec(ctx, `UPDATE role_bindings SET revoked_at=now(),version=version+1 WHERE org_id=$1 AND user_id=$3 AND revoked_at IS NULL
				AND ((scope_type='project' AND scope_id=$2) OR (scope_type='resource' AND (scope_id=$2 OR scope_id IN(SELECT id FROM tasks WHERE org_id=$1 AND project_id=$2))))`, c.Actor.OrgID, id, input.UserID); err != nil {
				return core.Result{}, err
			}
		} else {
			if err = validMembers(ctx, tx, c.Actor.OrgID, []string{input.UserID}); err != nil {
				return core.Result{}, err
			}
			_, err = tx.Exec(ctx, `INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,$4)
				ON CONFLICT(project_id,user_id) DO UPDATE SET project_role=excluded.project_role,authorization_version=project_members.authorization_version+1`, c.Actor.OrgID, id, input.UserID, input.Role)
			if err != nil {
				return core.Result{}, err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, input.UserID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE projects SET version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		p, err = getProject(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: p, Event: core.Event{Type: "project.member_changed", ResourceType: "project", ResourceID: id, Version: p.Version, Payload: map[string]any{"user_id": input.UserID, "action": input.Action, "project_role": input.Role, "reason": input.Reason}}}, nil
	})
}
