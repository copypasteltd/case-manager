package identity

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type Role struct {
	ID          string    `json:"id"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	RequiresMFA bool      `json:"requires_mfa"`
	Actions     []string  `json:"actions"`
}
type RoleBinding struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	RoleID         string     `json:"role_id"`
	ScopeType      string     `json:"scope_type"`
	ScopeID        *string    `json:"scope_id"`
	Classification string     `json:"classification_ceiling"`
	ValidFrom      time.Time  `json:"valid_from"`
	ValidUntil     *time.Time `json:"valid_until"`
	RevokedAt      *time.Time `json:"revoked_at"`
	Version        int64      `json:"version"`
}
type BindingCreate struct {
	UserID         string     `json:"user_id"`
	RoleID         string     `json:"role_id"`
	ScopeType      string     `json:"scope_type"`
	ScopeID        *string    `json:"scope_id"`
	Classification string     `json:"classification_ceiling"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	Reason         string     `json:"reason"`
}
type RevokeInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

const bindingColumns = `rb.id,rb.user_id,rb.role_id,rb.scope_type,rb.scope_id,rb.classification_ceiling,rb.valid_from,rb.valid_until,rb.revoked_at,rb.version`

func scanBinding(row pgx.Row) (RoleBinding, error) {
	var b RoleBinding
	err := row.Scan(&b.ID, &b.UserID, &b.RoleID, &b.ScopeType, &b.ScopeID, &b.Classification, &b.ValidFrom, &b.ValidUntil, &b.RevokedAt, &b.Version)
	return b, platform.DatabaseError(err)
}
func (s *Service) Roles(ctx context.Context, a core.Actor) (core.Page[Role], error) {
	out := core.Page[Role]{Items: []Role{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "role.read"); err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, `SELECT r.id,r.version,r.created_at,r.key,r.name,r.requires_mfa,
        ARRAY(SELECT action FROM role_actions ra WHERE ra.org_id=r.org_id AND ra.role_id=r.id ORDER BY action)
        FROM roles r WHERE r.org_id=$1 ORDER BY r.key`, a.OrgID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Role
		if err = rows.Scan(&r.ID, &r.Version, &r.CreatedAt, &r.Key, &r.Name, &r.RequiresMFA, &r.Actions); err != nil {
			return out, err
		}
		out.Items = append(out.Items, r)
	}
	return out, rows.Err()
}
func (s *Service) Bindings(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[RoleBinding], error) {
	out := core.Page[RoleBinding]{Items: []RoleBinding{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "role.read"); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(a.OrgID + a.UserID + fmt.Sprint(a.OrgAuthVersion) + "/role-bindings"))
	pager := platform.Pager{Key: s.Config.CursorKey}
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before, beforeID = &c.UpdatedAt, &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+bindingColumns+` FROM role_bindings rb WHERE org_id=$1
        AND ($2::timestamptz IS NULL OR (rb.valid_from,rb.id)<($2,$3::uuid)) ORDER BY rb.valid_from DESC,rb.id DESC LIMIT $4`, a.OrgID, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, b)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.ValidFrom, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}
func (s *Service) CreateBinding(ctx context.Context, c core.Command, input BindingCreate) (core.Result, error) {
	if !core.ValidID(input.UserID) || !core.ValidID(input.RoleID) || !core.Classification(input.Classification) {
		return core.Result{}, core.Invalid("binding", "授权资料无效")
	}
	if err := core.Name(input.Reason); err != nil {
		return core.Result{}, core.Invalid("reason", "请填写授权原因")
	}
	if (input.ScopeType == "org" || input.ScopeType == "own") && input.ScopeID != nil {
		return core.Result{}, core.Invalid("scope_id", "本组织或本人范围不能附加对象标识")
	}
	if input.ScopeType != "org" && input.ScopeType != "own" && input.ScopeType != "project" && input.ScopeType != "resource" {
		return core.Result{}, core.Invalid("scope_type", "授权范围无效")
	}
	if (input.ScopeType == "project" || input.ScopeType == "resource") && (input.ScopeID == nil || !core.ValidID(*input.ScopeID)) {
		return core.Result{}, core.Invalid("scope_id", "请选择授权对象")
	}
	if input.ValidUntil != nil && !input.ValidUntil.After(time.Now()) {
		return core.Result{}, core.Invalid("valid_until", "授权失效时间必须晚于当前时间")
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := recentMFA(c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "role.assign")
	}, func(tx pgx.Tx) (core.Result, error) {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND status IN ('active','invited'))
            AND EXISTS(SELECT 1 FROM roles WHERE org_id=$1 AND id=$3)`, c.Actor.OrgID, input.UserID, input.RoleID).Scan(&valid); err != nil {
			return core.Result{}, err
		}
		if !valid {
			return core.Result{}, core.Invalid("binding", "用户或角色不存在于当前组织")
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM role_bindings rb JOIN role_actions ra ON ra.org_id=rb.org_id AND ra.role_id=rb.role_id
            WHERE rb.org_id=$1 AND rb.user_id=$2 AND ra.action='role.assign' AND rb.scope_type='org' AND rb.revoked_at IS NULL
            AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())
            AND classification_rank(rb.classification_ceiling)>=classification_rank($3))`, c.Actor.OrgID, c.Actor.UserID, input.Classification).Scan(&valid); err != nil {
			return core.Result{}, err
		}
		if !valid {
			return core.Result{}, core.Forbidden()
		}
		if input.ScopeType == "project" {
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE org_id=$1 AND id=$2)", c.Actor.OrgID, input.ScopeID).Scan(&valid); err != nil {
				return core.Result{}, err
			}
			if !valid {
				return core.Result{}, core.Invalid("scope_id", "项目不存在于当前组织")
			}
		}
		if input.ScopeType == "resource" {
			// Resource scope is limited to aggregates with an implemented and
			// checked resolver. More kinds must be added before exposing them.
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE org_id=$1 AND id=$2)
                OR EXISTS(SELECT 1 FROM tasks WHERE org_id=$1 AND id=$2)`, c.Actor.OrgID, input.ScopeID).Scan(&valid); err != nil {
				return core.Result{}, err
			}
			if !valid {
				return core.Result{}, core.Invalid("scope_id", "授权对象不存在或类型尚不支持")
			}
		}
		id := core.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling,valid_until,grantor_id,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, c.Actor.OrgID, input.UserID, input.RoleID, input.ScopeType, input.ScopeID, input.Classification, input.ValidUntil, c.Actor.UserID, input.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err := tx.Exec(ctx, "UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, input.UserID); err != nil {
			return core.Result{}, err
		}
		if _, err := tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		b, err := scanBinding(tx.QueryRow(ctx, "SELECT "+bindingColumns+" FROM role_bindings rb WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: b, Event: core.Event{Type: "role.assigned", ResourceType: "role_binding", ResourceID: id, Version: b.Version, Payload: map[string]any{"subject_id": input.UserID, "reason": input.Reason}}}, nil
	})
}
func (s *Service) RevokeBinding(ctx context.Context, c core.Command, id string, input RevokeInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := recentMFA(c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "role.assign")
	}, func(tx pgx.Tx) (core.Result, error) {
		b, err := scanBinding(tx.QueryRow(ctx, "SELECT "+bindingColumns+" FROM role_bindings rb WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if b.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if b.RevokedAt != nil {
			return core.Result{}, core.Fail(409, "BINDING_ALREADY_REVOKED", "该授权已撤销")
		}
		var lastAdmin bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE org_id=$1 AND id=$2 AND key='system_admin')
            AND NOT EXISTS(SELECT 1 FROM role_bindings rb JOIN roles r ON r.org_id=rb.org_id AND r.id=rb.role_id
            JOIN users u ON u.org_id=rb.org_id AND u.id=rb.user_id WHERE rb.org_id=$1 AND rb.id<>$3 AND r.key='system_admin'
            AND rb.revoked_at IS NULL AND rb.scope_type='org' AND u.status='active' AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now()))`, c.Actor.OrgID, b.RoleID, id).Scan(&lastAdmin)
		if err != nil {
			return core.Result{}, err
		}
		if lastAdmin {
			return core.Result{}, core.Fail(422, "LAST_ADMIN_REQUIRED", "至少需要保留一位有效的系统管理员")
		}
		if _, err = tx.Exec(ctx, "UPDATE role_bindings SET revoked_at=now(),version=version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, b.UserID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: map[string]bool{"revoked": true}, Event: core.Event{Type: "role.revoked", ResourceType: "role_binding", ResourceID: id, Version: b.Version + 1, Payload: map[string]any{"subject_id": b.UserID, "reason": input.Reason}}}, nil
	})
}
