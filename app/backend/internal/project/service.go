package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}

const projectColumns = `p.id,p.version,p.created_at,p.updated_at,p.name,p.scope,p.type,p.owner_id,
	ARRAY(SELECT pm.user_id::text FROM project_members pm WHERE pm.org_id=p.org_id AND pm.project_id=p.id ORDER BY pm.user_id),
	p.start_date::text,p.due_date::text,p.classification,p.template_id,p.project_no,p.state,p.initialization_state,p.creator_id,p.template_version`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.Name, &p.Scope, &p.Type, &p.OwnerID, &p.MemberIDs,
		&p.StartDate, &p.DueDate, &p.Classification, &p.TemplateID, &p.ProjectNo, &p.State, &p.InitializationState, &p.CreatorID, &p.TemplateVersion)
	return p, platform.DatabaseError(err)
}
func resource(p Project) authz.Resource {
	return authz.Resource{Type: "project", ID: p.ID, ProjectID: &p.ID, OwnerID: &p.CreatorID, Classification: p.Classification}
}
func getProject(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Project, error) {
	if !core.ValidID(id) {
		return Project{}, core.Missing()
	}
	query := "SELECT " + projectColumns + " FROM projects p WHERE p.org_id=$1 AND p.id=$2"
	if lock {
		query += " FOR UPDATE OF p"
	}
	return scanProject(db.QueryRow(ctx, query, org, id))
}
func authorizeProject(ctx context.Context, tx platform.DBTX, a core.Actor, id, action string) error {
	p, err := getProject(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return err
	}
	return authz.Require(ctx, tx, a, action, resource(p))
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Project, error) {
	p, err := getProject(ctx, s.DB, a.OrgID, id, false)
	if err != nil {
		return p, err
	}
	err = authz.Require(ctx, s.DB, a, "project.read", resource(p))
	return p, err
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Project], error) {
	page := core.Page[Project]{Items: []Project{}}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "projects", f.State, f.Query)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return page, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+projectColumns+` FROM projects p WHERE p.org_id=$1
		AND can_access(p.org_id,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,$9)
		AND ($4='' OR p.state=$4) AND ($5='' OR position(lower($5) IN lower(p.name||' '||p.project_no))>0)
		AND ($6::timestamptz IS NULL OR (p.updated_at,p.id)<($6,$7::uuid))
		ORDER BY p.updated_at DESC,p.id DESC LIMIT $8`, a.OrgID, a.UserID, a.Channel, f.State, f.Query, before, beforeID, f.Limit+1, a.RunID != "")
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, p)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > f.Limit {
		page.Items = page.Items[:f.Limit]
		p := page.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.UpdatedAt, Binding: binding})
		page.NextCursor = &cursor
	}
	return page, nil
}
func validMembers(ctx context.Context, db platform.DBTX, org string, ids []string) error {
	if len(ids) > 100 {
		return core.Invalid("member_ids", "一次最多配置100位成员")
	}
	for _, id := range ids {
		if !core.ValidID(id) {
			return core.Invalid("member_ids", "成员标识无效")
		}
		var active bool
		err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND status='active')", org, id).Scan(&active)
		if err != nil {
			return err
		}
		if !active {
			return core.Invalid("member_ids", "成员不存在或已停用")
		}
	}
	return nil
}

// PrepareCreate validates and normalizes the same payload used by manual and
// approved secretary commands. Database authorization remains in Create.
func PrepareCreate(input Create) (Create, error) {
	if err := core.Name(input.Name); err != nil {
		return input, err
	}
	if !core.ValidID(input.OwnerID) {
		return input, core.Invalid("owner_id", "请选择有效负责人")
	}
	if !core.Classification(input.Classification) {
		return input, core.Invalid("classification", "资料分类无效")
	}
	if len([]rune(input.Scope)) > 20000 {
		return input, core.Invalid("scope", "范围说明过长")
	}
	if err := core.DateRange(input.StartDate, input.DueDate); err != nil {
		return input, err
	}
	if input.Type == "" {
		input.Type = "general"
	}
	if len(input.Type) > 100 {
		return input, core.Invalid("type", "项目类型过长")
	}
	return input, nil
}
func (s *Service) Create(ctx context.Context, c core.Command, input Create) (core.Result, error) {
	var err error
	if input, err = PrepareCreate(input); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := authz.RequireOrganization(ctx, tx, c.Actor, "project.create"); err != nil {
			return err
		}
		if input.OwnerID != c.Actor.UserID || len(input.MemberIDs) > 0 {
			if err := authz.RequireOrganization(ctx, tx, c.Actor, "project.manage_members"); err != nil {
				return err
			}
		}
		if input.Classification == "confidential" {
			return authz.RequireOrganization(ctx, tx, c.Actor, "policy.manage")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		members := map[string]bool{input.OwnerID: true, c.Actor.UserID: true}
		for _, id := range input.MemberIDs {
			members[id] = true
		}
		ids := make([]string, 0, len(members))
		for id := range members {
			ids = append(ids, id)
		}
		if err := validMembers(ctx, tx, c.Actor.OrgID, ids); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		var number int64
		if err := tx.QueryRow(ctx, "SELECT nextval('project_number_seq')").Scan(&number); err != nil {
			return core.Result{}, err
		}
		var templateVersion *int64
		var selectedTemplate Template
		if input.TemplateID != nil {
			var err error
			selectedTemplate, err = getTemplate(ctx, tx, c.Actor.OrgID, *input.TemplateID, " FOR SHARE")
			if err != nil {
				return core.Result{}, err
			}
			if selectedTemplate.State != "published" {
				return core.Result{}, core.Fail(409, "TEMPLATE_UNAVAILABLE", "模板已停用或尚未发布，请重新选择")
			}
			if _, err = validTemplateSchema(selectedTemplate.Schema); err != nil {
				return core.Result{}, err
			}
			templateVersion = &selectedTemplate.Revision
		}
		_, err := tx.Exec(ctx, `INSERT INTO projects(id,org_id,project_no,name,type,scope,owner_id,start_date,due_date,state,classification,template_id,template_version,creator_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::date,NULLIF($9,'')::date,'draft',$10,$11,$12,$13)`,
			id, c.Actor.OrgID, fmt.Sprintf("PRJ-%06d", number), strings.TrimSpace(input.Name), input.Type, input.Scope, input.OwnerID, input.StartDate, input.DueDate, input.Classification, input.TemplateID, templateVersion, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		for _, member := range ids {
			role := "member"
			if member == input.OwnerID {
				role = "owner"
			}
			_, err = tx.Exec(ctx, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,$4)", c.Actor.OrgID, id, member, role)
			if err != nil {
				return core.Result{}, err
			}
		}
		rootID := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO folders(id,org_id,project_id,parent_id,root_scope,name,name_key,classification,state,creator_id)
            VALUES($1,$2,$3,NULL,$4,'项目资料','项目资料',$5,'active',$6)`, rootID, c.Actor.OrgID, id, "project:"+id, input.Classification, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		for _, name := range []string{"合同", "交付", "工作资料"} {
			_, err = tx.Exec(ctx, "INSERT INTO folders(id,org_id,project_id,parent_id,root_scope,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,$4,$5,$6,$6,$7,'active',$8)",
				core.NewID(), c.Actor.OrgID, id, rootID, "project:"+id, name, input.Classification, c.Actor.UserID)
			if err != nil {
				return core.Result{}, err
			}
		}
		if input.TemplateID != nil {
			if err = s.applyTemplate(ctx, tx, c.Actor, id, rootID, input.OwnerID, input.Classification, selectedTemplate.Schema); err != nil {
				return core.Result{}, err
			}
		}
		if input.Classification == "confidential" {
			for _, member := range ids {
				_, err = tx.Exec(ctx, `INSERT INTO resource_grants(id,org_id,user_id,resource_type,resource_id,action,classification_ceiling,grantor_id,reason)
					SELECT gen_random_uuid(),$1,$2,'project',$3,ra.action,'confidential',$4,'initial confidential project scope'
					FROM role_bindings rb JOIN role_actions ra ON ra.org_id=rb.org_id AND ra.role_id=rb.role_id
					WHERE rb.org_id=$1 AND rb.user_id=$2 AND rb.scope_type='org' AND rb.classification_ceiling='confidential'
					AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())
					AND ra.action LIKE 'project.%' GROUP BY ra.action`, c.Actor.OrgID, member, id, c.Actor.UserID)
				if err != nil {
					return core.Result{}, err
				}
			}
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "project.created", ResourceType: "project", ResourceID: id, Version: p.Version, Payload: map[string]any{"project_id": id, "folder_id": rootID}}}, nil
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, id string, input Patch) (core.Result, error) {
	c.AuthorizationMutation = input.OwnerID != nil
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := authorizeProject(ctx, tx, c.Actor, id, "project.update"); err != nil {
			return err
		}
		if input.OwnerID != nil {
			if err := authorizeProject(ctx, tx, c.Actor, id, "project.manage_members"); err != nil {
				return err
			}
		}
		if input.Classification != nil {
			return authz.RequireOrganization(ctx, tx, c.Actor, "policy.manage")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		p, err := getProject(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if p.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if (p.State == "active" || p.State == "on_hold") && ((input.Scope != nil && *input.Scope != p.Scope) || (input.StartDate.Set && !sameProjectDate(input.StartDate.Value, p.StartDate)) || (input.DueDate.Set && !sameProjectDate(input.DueDate.Value, p.DueDate))) {
			return core.Result{}, core.Fail(409, "PROJECT_SCOPE_REQUIRES_APPROVED_CHANGE", "进行中项目的范围和起止日期须通过项目变更独立审批并明确实施")
		}
		if p.State == "closing" || p.State == "archived" || p.State == "cancelled" {
			return core.Result{}, core.Fail(409, "PROJECT_READ_ONLY", "项目已归档或取消，请按规则重开后编辑")
		}
		if input.Name != nil {
			if err = core.Name(*input.Name); err != nil {
				return core.Result{}, err
			}
			p.Name = strings.TrimSpace(*input.Name)
		}
		if input.Scope != nil {
			if len([]rune(*input.Scope)) > 20000 {
				return core.Result{}, core.Invalid("scope", "范围说明过长")
			}
			p.Scope = *input.Scope
		}
		if input.Type != nil {
			if err = core.Name(*input.Type); err != nil {
				return core.Result{}, err
			}
			p.Type = *input.Type
		}
		if input.StartDate.Set {
			p.StartDate = input.StartDate.Value
		}
		if input.DueDate.Set {
			p.DueDate = input.DueDate.Value
		}
		start, end := "", ""
		if p.StartDate != nil {
			start = *p.StartDate
		}
		if p.DueDate != nil {
			end = *p.DueDate
		}
		if err = core.DateRange(start, end); err != nil {
			return core.Result{}, err
		}
		if input.Classification != nil && *input.Classification != p.Classification {
			return core.Result{}, core.Fail(422, "POLICY_CHANGE_REQUIRES_PLAN", "分类变更须提交独立范围影响计划，不能通过普通项目编辑直接修改")
		}
		if input.OwnerID != nil && *input.OwnerID != p.OwnerID {
			if err = validMembers(ctx, tx, c.Actor.OrgID, []string{*input.OwnerID}); err != nil {
				return core.Result{}, err
			}
			_, err = tx.Exec(ctx, "UPDATE project_members SET project_role='manager' WHERE org_id=$1 AND project_id=$2 AND project_role='owner'", c.Actor.OrgID, id)
			if err != nil {
				return core.Result{}, err
			}
			_, err = tx.Exec(ctx, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'owner') ON CONFLICT(project_id,user_id) DO UPDATE SET project_role='owner',authorization_version=project_members.authorization_version+1", c.Actor.OrgID, id, *input.OwnerID)
			if err != nil {
				return core.Result{}, err
			}
			p.OwnerID = *input.OwnerID
			_, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID)
			if err != nil {
				return core.Result{}, err
			}
		}
		_, err = tx.Exec(ctx, "UPDATE projects SET name=$3,scope=$4,type=$5,owner_id=$6,start_date=NULLIF($7,'')::date,due_date=NULLIF($8,'')::date,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2",
			c.Actor.OrgID, id, p.Name, p.Scope, p.Type, p.OwnerID, start, end)
		if err != nil {
			return core.Result{}, err
		}
		p, err = getProject(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: p, Event: core.Event{Type: "project.updated", ResourceType: "project", ResourceID: id, Version: p.Version}}, nil
	})
}
func (s *Service) Transition(ctx context.Context, c core.Command, id string, input Transition) (core.Result, error) {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("reason", "请说明变更原因")
	}
	action := "project.update"
	if input.Action == "archive" {
		return core.Result{}, core.Fail(422, "PROJECT_ARCHIVE_REVIEW_REQUIRED", "请通过结项核查填写复盘并确认准确依据后归档")
	}
	if input.Action == "reopen" {
		action = "project.reopen"
	}
	if input.Action == "archive" {
		action = "project.archive"
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeProject(ctx, tx, c.Actor, id, action) }, func(tx pgx.Tx) (core.Result, error) {
		p, err := getProject(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if p.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		target, ok := projectTarget(p.State, input.Action)
		if !ok {
			return core.Result{}, core.UnexpectedState(p.State, input.Action)
		}
		if target == "active" {
			if strings.TrimSpace(p.Scope) == "" {
				return core.Result{}, core.Invalid("scope", "请先完善项目范围")
			}
			if err = validMembers(ctx, tx, c.Actor.OrgID, []string{p.OwnerID}); err != nil {
				return core.Result{}, err
			}
			var count int
			if err = tx.QueryRow(ctx, "SELECT count(*) FROM project_members WHERE org_id=$1 AND project_id=$2", c.Actor.OrgID, id).Scan(&count); err != nil {
				return core.Result{}, err
			}
			if count == 0 {
				return core.Result{}, core.Invalid("members", "请先配置项目成员")
			}
		}
		if target == "closing" || target == "archived" {
			basis, _, _, err := closeoutSnapshot(ctx, tx, c.Actor.OrgID, id)
			if err != nil {
				return core.Result{}, err
			}
			if closeoutBlocked(basis.Items) {
				visible, _, err := closeoutVisibility(ctx, tx, c.Actor, id)
				if err != nil {
					return core.Result{}, err
				}
				return core.Result{}, closeoutBlockedError(basis.Items, visible)
			}
		}
		_, err = tx.Exec(ctx, "UPDATE projects SET state=$3,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, target)
		if err != nil {
			return core.Result{}, err
		}
		p, err = getProject(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: p, Event: core.Event{Type: "project.transitioned", ResourceType: "project", ResourceID: id, Version: p.Version, Payload: map[string]any{"state": target, "reason": input.Reason}}}, nil
	})
}
func projectTarget(state, action string) (string, bool) {
	switch action {
	case "activate":
		return "active", state == "draft"
	case "hold":
		return "on_hold", state == "active"
	case "resume":
		return "active", state == "on_hold" || state == "closing"
	case "close":
		return "closing", state == "active" || state == "on_hold"
	case "archive":
		return "archived", state == "closing"
	case "cancel":
		return "cancelled", state == "draft" || state == "active" || state == "on_hold"
	case "reopen":
		return "active", state == "archived" || state == "cancelled"
	}
	return "", false
}
func sameProjectDate(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
