package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

type ProjectPlanCreate struct {
	ExpectedVersion        int64          `json:"expected_version"`
	ContextSecurityVersion int64          `json:"context_security_version"`
	Project                project.Create `json:"project"`
	Reason                 string         `json:"reason"`
}
type PlanAction struct {
	ExpectedVersion int64  `json:"expected_version"`
	PlanHash        string `json:"plan_hash"`
	Reason          string `json:"reason"`
}
type Plan struct {
	ID                      string          `json:"id"`
	Version                 int64           `json:"version"`
	ConversationID          string          `json:"conversation_id"`
	ConversationRevisionID  string          `json:"conversation_revision_id"`
	ContextSecurityVersion  int64           `json:"context_security_version"`
	ActorID                 string          `json:"actor_id"`
	ActorAuthVersion        int64           `json:"actor_auth_version"`
	OrganizationAuthVersion int64           `json:"organization_auth_version"`
	OriginRunID             *string         `json:"origin_run_id"`
	Kind                    string          `json:"kind"`
	PlanHash                string          `json:"plan_hash"`
	Project                 project.Create  `json:"project"`
	TemplateSnapshot        json.RawMessage `json:"template_snapshot"`
	Reason                  string          `json:"reason"`
	State                   string          `json:"state"`
	OperationID             *string         `json:"operation_id"`
	ProjectID               *string         `json:"project_id"`
	CreatedAt               time.Time       `json:"created_at"`
	ExpiresAt               time.Time       `json:"expires_at"`
	BlockedReason           *string         `json:"blocked_reason"`
	CanApprove              bool            `json:"can_approve"`
	CanCommit               bool            `json:"can_commit"`
	CanRevoke               bool            `json:"can_revoke"`
}
type PlanList struct {
	Items   []Plan `json:"items"`
	HasMore bool   `json:"has_more"`
}

const planColumns = `id,version,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,
 origin_run_id,kind,plan_hash,payload,template_snapshot,reason,state,operation_id,project_id,created_at,expires_at`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	var payload []byte
	err := row.Scan(&p.ID, &p.Version, &p.ConversationID, &p.ConversationRevisionID, &p.ContextSecurityVersion, &p.ActorID, &p.ActorAuthVersion, &p.OrganizationAuthVersion,
		&p.OriginRunID, &p.Kind, &p.PlanHash, &payload, &p.TemplateSnapshot, &p.Reason, &p.State, &p.OperationID, &p.ProjectID, &p.CreatedAt, &p.ExpiresAt)
	if err != nil {
		return p, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &p.Project)
	return p, err
}
func planRecord(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Plan, error) {
	if !core.ValidID(id) {
		return Plan{}, core.Missing()
	}
	q := "SELECT " + planColumns + " FROM agent_plans WHERE org_id=$1 AND id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	return scanPlan(db.QueryRow(ctx, q, org, id))
}
func planConversation(ctx context.Context, tx pgx.Tx, a core.Actor, id, action string) (Conversation, error) {
	c, err := metadata(ctx, tx, a.OrgID, id, true)
	if err != nil {
		return c, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return c, err
	}
	if c.Type != "private" || c.ProjectID != nil || c.CreatorID != a.UserID {
		return c, core.Fail(409, "PLAN_SCOPE_UNSUPPORTED", "项目创建计划请在本人的公司私密会话中拟定")
	}
	if err = authz.RequireOrganization(ctx, tx, a, action); err != nil {
		return c, err
	}
	return c, nil
}
func planTemplate(ctx context.Context, tx pgx.Tx, org string, in project.Create) (json.RawMessage, error) {
	if in.TemplateID == nil {
		return json.RawMessage(`{}`), nil
	}
	if !core.ValidID(*in.TemplateID) {
		return nil, core.Invalid("template_id", "项目模板标识无效")
	}
	var revision int64
	var state string
	var schema []byte
	if err := tx.QueryRow(ctx, "SELECT revision,task_and_folder_schema,state FROM project_templates WHERE org_id=$1 AND id=$2 FOR SHARE", org, *in.TemplateID).Scan(&revision, &schema, &state); err != nil {
		return nil, platform.DatabaseError(err)
	}
	if state != "published" {
		return nil, core.Fail(409, "PLAN_TEMPLATE_CHANGED", "固定模板已停用，请重新拟定")
	}
	canonical, err := core.CanonicalJSON(schema)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"id": *in.TemplateID, "published_revision": revision, "schema_sha256": core.Digest(canonical)})
}
func authorizePlannedProject(ctx context.Context, tx pgx.Tx, a core.Actor, in project.Create) error {
	if err := authz.RequireOrganization(ctx, tx, a, "project.create"); err != nil {
		return err
	}
	if in.OwnerID != a.UserID || len(in.MemberIDs) > 0 {
		if err := authz.RequireOrganization(ctx, tx, a, "project.manage_members"); err != nil {
			return err
		}
	}
	ids := append([]string{a.UserID, in.OwnerID}, in.MemberIDs...)
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		var active bool
		if err := tx.QueryRow(ctx, "SELECT status='active' FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, id).Scan(&active); err != nil {
			return platform.DatabaseError(err)
		}
		if !active {
			return core.Fail(409, "PLAN_MEMBER_UNAVAILABLE", "计划中的人员已停用，请重新拟定")
		}
	}
	return nil
}
func preparePlannedProject(in project.Create) (project.Create, error) {
	var err error
	in, err = project.PrepareCreate(in)
	if err != nil {
		return in, err
	}
	if !slices.Contains([]string{"internal", "project_restricted"}, in.Classification) || len(in.MemberIDs) > 30 {
		return in, core.Invalid("project", "秘书项目创建仅支持内部或项目受限资料，最多30名成员")
	}
	in.OwnerID = strings.ToLower(in.OwnerID)
	for i, id := range in.MemberIDs {
		if !core.ValidID(id) {
			return in, core.Invalid("member_ids", "项目成员标识无效")
		}
		in.MemberIDs[i] = strings.ToLower(id)
	}
	slices.Sort(in.MemberIDs)
	in.MemberIDs = slices.Compact(in.MemberIDs)
	if in.TemplateID != nil {
		v := strings.ToLower(*in.TemplateID)
		in.TemplateID = &v
	}
	return in, nil
}

// A plan's hash binds every execution-relevant field, not just its human title.
func planIntent(p Plan) ([]byte, error) {
	return json.Marshal(map[string]any{"id": p.ID, "conversation_id": p.ConversationID, "conversation_revision_id": p.ConversationRevisionID, "context_security_version": p.ContextSecurityVersion,
		"actor_id": p.ActorID, "actor_auth_version": p.ActorAuthVersion, "organization_auth_version": p.OrganizationAuthVersion, "origin_run_id": p.OriginRunID,
		"kind": p.Kind, "project": p.Project, "template_snapshot": p.TemplateSnapshot, "reason": p.Reason, "created_at": p.CreatedAt, "expires_at": p.ExpiresAt})
}
func planEvent(p Plan, reason string) core.Event {
	return core.Event{Type: "agent.plan." + p.State, ResourceType: "agent_plan", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"plan_hash": p.PlanHash, "reason": reason, "operation_id": p.OperationID, "project_id": p.ProjectID}}
}
func appendPlanEvent(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan, reason string) error {
	var mfa *time.Time
	if p.State == "approved" {
		if err := tx.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE org_id=$1 AND id=$2", a.OrgID, a.SessionID).Scan(&mfa); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_plan_events(id,org_id,plan_id,version,kind,plan_hash,actor_id,session_id,mfa_verified_at,operation_id,project_id,reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10,$11,$12)`, core.NewID(), a.OrgID, p.ID, p.Version, p.State, p.PlanHash, a.UserID, a.SessionID, mfa, p.OperationID, p.ProjectID, reason)
	return err
}
func proposeProjectPlan(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ProjectPlanCreate) (Plan, error) {
	var p Plan
	if c.State != "active" || c.Version != in.ExpectedVersion || c.ContextSecurityVersion != in.ContextSecurityVersion {
		return p, core.Fail(409, "PLAN_CONTEXT_CHANGED", "会话已变化，请重新读取后拟定计划")
	}
	if c.Type != "private" || c.ProjectID != nil || c.CreatorID != a.UserID {
		return p, core.Fail(409, "PLAN_SCOPE_UNSUPPORTED", "项目创建计划仅限发起人的公司私密会话")
	}
	if err := core.Name(strings.TrimSpace(in.Reason)); err != nil {
		return p, err
	}
	input, err := preparePlannedProject(in.Project)
	if err != nil {
		return p, err
	}
	if err = authorizePlannedProject(ctx, tx, a, input); err != nil {
		return p, err
	}
	template, err := planTemplate(ctx, tx, a.OrgID, input)
	if err != nil {
		return p, err
	}
	p = Plan{ID: core.NewID(), Version: 1, ConversationID: c.ID, ConversationRevisionID: c.CurrentRevisionID, ContextSecurityVersion: c.ContextSecurityVersion,
		ActorID: a.UserID, ActorAuthVersion: a.AuthVersion, OrganizationAuthVersion: a.OrgAuthVersion, Kind: "project.create", Project: input, TemplateSnapshot: template, Reason: strings.TrimSpace(in.Reason), State: "proposed"}
	if a.RunID != "" {
		v := a.RunID
		p.OriginRunID = &v
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&p.CreatedAt); err != nil {
		return p, err
	}
	p.ExpiresAt = p.CreatedAt.Add(30 * time.Minute)
	intent, err := planIntent(p)
	if err != nil {
		return p, err
	}
	canonical, err := core.CanonicalJSON(intent)
	if err != nil {
		return p, err
	}
	p.PlanHash = core.Digest(canonical)
	payload, _ := json.Marshal(input)
	_, err = tx.Exec(ctx, `INSERT INTO agent_plans(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,origin_run_id,kind,intent,plan_hash,payload,template_snapshot,reason,created_at,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, p.ID, a.OrgID, p.ConversationID, p.ConversationRevisionID, p.ContextSecurityVersion, p.ActorID, p.ActorAuthVersion, p.OrganizationAuthVersion, p.OriginRunID, p.Kind, string(canonical), p.PlanHash, payload, p.TemplateSnapshot, p.Reason, p.CreatedAt, p.ExpiresAt)
	if err != nil {
		return p, err
	}
	err = appendPlanEvent(ctx, tx, a, p, p.Reason)
	return p, err
}
func (s *Service) ProposeProjectPlan(ctx context.Context, cmd core.Command, id string, in ProjectPlanCreate) (core.Result, error) {
	if cmd.Actor.RunID != "" {
		return core.Result{}, core.Forbidden()
	}
	cmd.AuthorizationMutation = true
	var c Conversation
	result, err := s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		var e error
		c, e = planConversation(ctx, tx, cmd.Actor, id, "agent.plan.propose")
		if e != nil {
			return e
		}
		return require(ctx, tx, cmd.Actor, c, "agent.run")
	}, func(tx pgx.Tx) (core.Result, error) {
		p, e := proposeProjectPlan(ctx, tx, cmd.Actor, c, in)
		return core.Result{Status: 201, Data: p, Event: planEvent(p, in.Reason)}, e
	})
	return s.currentPlanResult(ctx, cmd.Actor, result, err)
}
func planAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan, c Conversation) error {
	if p.ActorID != a.UserID {
		return core.Missing()
	}
	if p.ActorAuthVersion != a.AuthVersion || p.OrganizationAuthVersion != a.OrgAuthVersion {
		return core.Fail(412, "PLAN_AUTHORIZATION_CHANGED", "计划对应的权限已变化，请重新拟定")
	}
	if c.State != "active" || c.CurrentRevisionID != p.ConversationRevisionID || c.ContextSecurityVersion != p.ContextSecurityVersion {
		return core.Fail(409, "PLAN_CONTEXT_CHANGED", "计划对应的会话版本已变化，请重新拟定")
	}
	var live bool
	if err := tx.QueryRow(ctx, "SELECT $1::timestamptz>clock_timestamp()", p.ExpiresAt).Scan(&live); err != nil {
		return err
	}
	if !live {
		return core.Fail(409, "PLAN_EXPIRED", "计划已过期，请重新拟定")
	}
	if p.OriginRunID != nil {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT state='succeeded' OR (state IN ('queued','running','awaiting_approval','committing') AND cancel_requested_at IS NULL AND expires_at>clock_timestamp()) FROM agent_runs
 WHERE org_id=$1 AND id=$2 AND actor_id=$3 AND conversation_id=$4`, a.OrgID, *p.OriginRunID, a.UserID, p.ConversationID).Scan(&available); err != nil {
			return platform.DatabaseError(err)
		}
		if !available {
			return core.Fail(409, "PLAN_ORIGIN_UNAVAILABLE", "拟定计划的任务已取消、中断或失败，请重新拟定")
		}
	}
	if err := authorizePlannedProject(ctx, tx, a, p.Project); err != nil {
		return err
	}
	template, err := planTemplate(ctx, tx, a.OrgID, p.Project)
	if err != nil {
		return err
	}
	left, _ := core.CanonicalJSON(template)
	right, _ := core.CanonicalJSON(p.TemplateSnapshot)
	if string(left) != string(right) {
		return core.Fail(409, "PLAN_TEMPLATE_CHANGED", "项目模板已变化，请重新拟定")
	}
	return nil
}
func presentPlan(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan, c Conversation) (Plan, error) {
	if p.ActorID != a.UserID {
		return Plan{}, core.Missing()
	}
	if err := authz.RequireOrganization(ctx, tx, a, "agent.plan.read"); err != nil {
		return Plan{}, err
	}
	if p.ProjectID != nil {
		if err := plannedProjectReadable(ctx, tx, a, p); err != nil {
			return Plan{}, err
		}
	}
	p.CanRevoke = a.RunID == "" && slices.Contains([]string{"proposed", "approved"}, p.State) && authz.RequireOrganization(ctx, tx, a, "agent.plan.revoke") == nil
	if p.State == "committed" || p.State == "revoked" {
		return p, nil
	}
	if err := planAuthority(ctx, tx, a, p, c); err != nil {
		if core.PublicError(err).Status >= 500 {
			return Plan{}, err
		}
		v := core.PublicError(err).Code
		p.BlockedReason = &v
		return p, nil
	}
	p.CanApprove = a.RunID == "" && p.State == "proposed" && authz.RequireOrganization(ctx, tx, a, "agent.plan.approve") == nil
	p.CanCommit = a.RunID == "" && p.State == "approved" && authz.RequireOrganization(ctx, tx, a, "agent.plan.commit") == nil
	return p, nil
}
func plannedProjectReadable(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan) error {
	var creator, class string
	if p.ProjectID == nil {
		return nil
	}
	if err := tx.QueryRow(ctx, "SELECT creator_id,classification FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, *p.ProjectID).Scan(&creator, &class); err != nil {
		return platform.DatabaseError(err)
	}
	return authz.Require(ctx, tx, a, "project.read", authz.Resource{Type: "project", ID: *p.ProjectID, ProjectID: p.ProjectID, OwnerID: &creator, Classification: class})
}
func (s *Service) GetPlan(ctx context.Context, a core.Actor, id string) (Plan, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback(ctx)
	p, err := planRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return p, err
	}
	c, err := metadata(ctx, tx, a.OrgID, p.ConversationID, false)
	if err != nil {
		return p, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return Plan{}, err
	}
	p, err = presentPlan(ctx, tx, a, p, c)
	if err != nil {
		return Plan{}, err
	}
	return p, tx.Commit(ctx)
}
func (s *Service) ListPlans(ctx context.Context, a core.Actor, id string) (PlanList, error) {
	out := PlanList{Items: []Plan{}}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return out, err
	}
	if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.read"); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT "+planColumns+" FROM agent_plans WHERE org_id=$1 AND conversation_id=$2 AND actor_id=$3 ORDER BY created_at DESC,id DESC LIMIT 51", a.OrgID, id, a.UserID)
	if err != nil {
		return out, err
	}
	plans := []Plan{}
	for rows.Next() {
		p, e := scanPlan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		plans = append(plans, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = len(plans) > 50
	if out.HasMore {
		plans = plans[:50]
	}
	for _, p := range plans {
		p, e := presentPlan(ctx, tx, a, p, c)
		if e != nil {
			if core.PublicError(e).Status == 403 || core.PublicError(e).Status == 404 {
				continue
			}
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, tx.Commit(ctx)
}
func (s *Service) currentPlanResult(ctx context.Context, a core.Actor, r core.Result, err error) (core.Result, error) {
	if err != nil {
		return r, err
	}
	raw, e := json.Marshal(r.Data)
	if e != nil {
		return core.Result{}, e
	}
	var stored Plan
	if e = json.Unmarshal(raw, &stored); e != nil {
		return core.Result{}, e
	}
	p, e := s.GetPlan(ctx, a, stored.ID)
	if e != nil {
		return core.Result{}, e
	}
	r.Data = p
	return r, nil
}
