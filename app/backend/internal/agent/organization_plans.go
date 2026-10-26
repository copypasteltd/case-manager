package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type OrganizationPlanCreate struct {
	ExpectedVersion        int64                      `json:"expected_version"`
	ContextSecurityVersion int64                      `json:"context_security_version"`
	Actions                []files.OrganizationAction `json:"actions"`
	Reason                 string                     `json:"reason"`
}
type OrganizationOutcome struct {
	OperationID string          `json:"operation_id"`
	Data        json.RawMessage `json:"data"`
}
type OrganizationPlan struct {
	ID                      string                      `json:"id"`
	Version                 int64                       `json:"version"`
	ConversationID          string                      `json:"conversation_id"`
	ConversationRevisionID  string                      `json:"conversation_revision_id"`
	ContextSecurityVersion  int64                       `json:"context_security_version"`
	ActorID                 string                      `json:"actor_id"`
	ActorAuthVersion        int64                       `json:"actor_auth_version"`
	OrganizationAuthVersion int64                       `json:"organization_auth_version"`
	OriginRunID             *string                     `json:"origin_run_id"`
	ProjectID               *string                     `json:"project_id"`
	PlanHash                string                      `json:"plan_hash"`
	Preview                 []files.OrganizationPreview `json:"preview"`
	Reason                  string                      `json:"reason"`
	State                   string                      `json:"state"`
	OperationID             *string                     `json:"operation_id"`
	Results                 []OrganizationOutcome       `json:"results"`
	CreatedAt               time.Time                   `json:"created_at"`
	ExpiresAt               time.Time                   `json:"expires_at"`
	BlockedReason           *string                     `json:"blocked_reason"`
	CanApprove              bool                        `json:"can_approve"`
	CanCommit               bool                        `json:"can_commit"`
	CanRevoke               bool                        `json:"can_revoke"`
}
type OrganizationPlanList struct {
	Items   []OrganizationPlan `json:"items"`
	HasMore bool               `json:"has_more"`
}

const organizationColumns = `id,version,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,origin_run_id,project_id,plan_hash,preview,reason,state,operation_id,result_data,created_at,expires_at`

func organizationPlanRecord(ctx context.Context, tx pgx.Tx, org, id string, lock bool) (OrganizationPlan, error) {
	var p OrganizationPlan
	var preview, result []byte
	if !core.ValidID(id) {
		return p, core.Missing()
	}
	q := "SELECT " + organizationColumns + " FROM agent_organization_plans WHERE org_id=$1 AND id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, q, org, id).Scan(&p.ID, &p.Version, &p.ConversationID, &p.ConversationRevisionID, &p.ContextSecurityVersion, &p.ActorID, &p.ActorAuthVersion, &p.OrganizationAuthVersion, &p.OriginRunID, &p.ProjectID, &p.PlanHash, &preview, &p.Reason, &p.State, &p.OperationID, &result, &p.CreatedAt, &p.ExpiresAt)
	if err != nil {
		return p, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(preview, &p.Preview); err == nil {
		err = json.Unmarshal(result, &p.Results)
	}
	return p, err
}
func organizationConversation(ctx context.Context, tx pgx.Tx, a core.Actor, id, action string, lock bool) (Conversation, error) {
	c, err := metadata(ctx, tx, a.OrgID, id, lock)
	if err != nil {
		return c, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return c, err
	}
	if c.Type != "private" || c.CreatorID != a.UserID {
		return c, core.Fail(409, "PLAN_SCOPE_UNSUPPORTED", "请在本人的私密会话中拟定资料整理计划")
	}
	return c, authz.RequireOrganization(ctx, tx, a, "agent.plan."+action)
}
func organizationEntryScope(c Conversation, e files.OrganizationEntry) error {
	if !sameProject(c.ProjectID, e.ProjectID) || classRank(e.Classification) == 0 || classRank(e.Classification) > classRank(c.Classification) {
		return core.Missing()
	}
	return nil
}
func organizationReadable(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, p OrganizationPlan) error {
	if p.ActorID != a.UserID || p.ConversationID != c.ID || c.Type != "private" || c.CreatorID != a.UserID || !sameProject(c.ProjectID, p.ProjectID) {
		return core.Missing()
	}
	ai := a
	if ai.RunID == "" {
		ai.RunID = p.ID
	}
	seen := map[string]bool{}
	for _, v := range p.Preview {
		entries := append(append([]files.OrganizationEntry{v.Source}, v.Path...), v.TargetPath...)
		if v.Target != nil {
			entries = append(entries, *v.Target)
		}
		for _, old := range entries {
			if seen[old.Kind+old.ID] {
				continue
			}
			seen[old.Kind+old.ID] = true
			current, err := files.OrganizationMetadata(ctx, tx, ai, old.Kind, old.ID, old.Kind+".read")
			if err != nil {
				return err
			}
			if err = organizationEntryScope(c, current); err != nil {
				return err
			}
		}
	}
	for _, result := range p.Results {
		var e files.OrganizationEntry
		if err := json.Unmarshal(result.Data, &e); err != nil {
			return err
		}
		kind := "folder"
		var data map[string]json.RawMessage
		_ = json.Unmarshal(result.Data, &data)
		if _, ok := data["folder_id"]; ok {
			kind = "file"
		}
		current, err := files.OrganizationMetadata(ctx, tx, ai, kind, e.ID, kind+".read")
		if err != nil {
			return err
		}
		if err = organizationEntryScope(c, current); err != nil {
			return err
		}
	}
	return nil
}
func organizationAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, p OrganizationPlan, lock bool) error {
	if err := organizationReadable(ctx, tx, a, c, p); err != nil {
		return err
	}
	if p.ActorAuthVersion != a.AuthVersion || p.OrganizationAuthVersion != a.OrgAuthVersion {
		return core.Fail(412, "PLAN_AUTHORIZATION_CHANGED", "计划对应的权限已变化，请重新拟定")
	}
	if c.State != "active" || c.CurrentRevisionID != p.ConversationRevisionID || c.ContextSecurityVersion != p.ContextSecurityVersion {
		return core.Fail(409, "PLAN_CONTEXT_CHANGED", "会话已变化，请重新拟定")
	}
	var live bool
	if err := tx.QueryRow(ctx, "SELECT $1::timestamptz>clock_timestamp()", p.ExpiresAt).Scan(&live); err != nil {
		return err
	}
	if !live {
		return core.Fail(409, "PLAN_EXPIRED", "整理计划已过期，请重新拟定")
	}
	if p.OriginRunID != nil {
		if err := tx.QueryRow(ctx, `SELECT state='succeeded' OR (state IN ('queued','running','awaiting_approval','committing') AND cancel_requested_at IS NULL AND expires_at>clock_timestamp()) FROM agent_runs WHERE org_id=$1 AND id=$2 AND actor_id=$3 AND conversation_id=$4`, a.OrgID, *p.OriginRunID, a.UserID, c.ID).Scan(&live); err != nil {
			return platform.DatabaseError(err)
		}
		if !live {
			return core.Fail(409, "PLAN_ORIGIN_UNAVAILABLE", "拟定计划的任务已取消、中断或失败")
		}
	}
	actions := make([]files.OrganizationAction, len(p.Preview))
	for i, v := range p.Preview {
		actions[i] = v.Action
	}
	ai := a
	if ai.RunID == "" {
		ai.RunID = p.ID
	}
	current, err := files.PreviewOrganization(ctx, tx, ai, c.ProjectID, actions, lock)
	if err != nil {
		return err
	}
	left, _ := json.Marshal(current)
	right, _ := json.Marshal(p.Preview)
	cl, _ := core.CanonicalJSON(left)
	cr, _ := core.CanonicalJSON(right)
	if string(cl) != string(cr) {
		return core.Fail(409, "PLAN_SOURCE_CHANGED", "目录、名称或版本已变化，请重新拟定")
	}
	return nil
}
func presentOrganizationPlan(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, p OrganizationPlan) (OrganizationPlan, error) {
	if err := organizationReadable(ctx, tx, a, c, p); err != nil {
		return OrganizationPlan{}, err
	}
	p.CanRevoke = a.RunID == "" && slices.Contains([]string{"proposed", "approved"}, p.State) && authz.RequireOrganization(ctx, tx, a, "agent.plan.revoke") == nil
	if p.State == "committed" || p.State == "revoked" {
		return p, nil
	}
	if err := organizationAuthority(ctx, tx, a, c, p, false); err != nil {
		if core.PublicError(err).Status >= 500 {
			return OrganizationPlan{}, err
		}
		code := core.PublicError(err).Code
		p.BlockedReason = &code
		return p, nil
	}
	p.CanApprove = a.RunID == "" && p.State == "proposed" && authz.RequireOrganization(ctx, tx, a, "agent.plan.approve") == nil
	p.CanCommit = a.RunID == "" && p.State == "approved" && authz.RequireOrganization(ctx, tx, a, "agent.plan.commit") == nil
	return p, nil
}
func organizationEvent(p OrganizationPlan, reason string) core.Event {
	return core.Event{Type: "agent.organization." + p.State, ResourceType: "agent_organization_plan", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"plan_hash": p.PlanHash, "reason": reason, "operation_id": p.OperationID, "count": len(p.Preview)}}
}
func appendOrganizationEvent(ctx context.Context, tx pgx.Tx, a core.Actor, p OrganizationPlan, reason string) error {
	var mfa *time.Time
	if p.State == "approved" {
		if err := tx.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE org_id=$1 AND id=$2", a.OrgID, a.SessionID).Scan(&mfa); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_organization_events(id,org_id,plan_id,version,kind,plan_hash,actor_id,session_id,mfa_verified_at,operation_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10,$11)`, core.NewID(), a.OrgID, p.ID, p.Version, p.State, p.PlanHash, a.UserID, a.SessionID, mfa, p.OperationID, reason)
	return err
}
func proposeOrganization(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in OrganizationPlanCreate) (OrganizationPlan, error) {
	var p OrganizationPlan
	if c.State != "active" || c.Version != in.ExpectedVersion || c.ContextSecurityVersion != in.ContextSecurityVersion {
		return p, core.Fail(409, "PLAN_CONTEXT_CHANGED", "请重新核对当前会话")
	}
	if c.Type != "private" || c.CreatorID != a.UserID {
		return p, core.Fail(409, "PLAN_SCOPE_UNSUPPORTED", "整理计划仅限本人私密会话")
	}
	if err := core.Name(strings.TrimSpace(in.Reason)); err != nil {
		return p, err
	}
	ai := a
	if ai.RunID == "" {
		ai.RunID = core.NewID()
	}
	preview, err := files.PreviewOrganization(ctx, tx, ai, c.ProjectID, in.Actions, true)
	if err != nil {
		return p, err
	}
	p = OrganizationPlan{ID: core.NewID(), Version: 1, ConversationID: c.ID, ConversationRevisionID: c.CurrentRevisionID, ContextSecurityVersion: c.ContextSecurityVersion, ActorID: a.UserID, ActorAuthVersion: a.AuthVersion, OrganizationAuthVersion: a.OrgAuthVersion, ProjectID: c.ProjectID, Preview: preview, Reason: strings.TrimSpace(in.Reason), State: "proposed", Results: []OrganizationOutcome{}}
	if a.RunID != "" {
		id := a.RunID
		p.OriginRunID = &id
	}
	if err = organizationReadable(ctx, tx, ai, c, p); err != nil {
		return p, err
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&p.CreatedAt); err != nil {
		return p, err
	}
	p.ExpiresAt = p.CreatedAt.Add(30 * time.Minute)
	intent, _ := json.Marshal(map[string]any{"id": p.ID, "conversation_id": p.ConversationID, "conversation_revision_id": p.ConversationRevisionID, "context_security_version": p.ContextSecurityVersion, "actor_id": p.ActorID, "actor_auth_version": p.ActorAuthVersion, "organization_auth_version": p.OrganizationAuthVersion, "project_id": p.ProjectID, "origin_run_id": p.OriginRunID, "preview": p.Preview, "reason": p.Reason, "created_at": p.CreatedAt, "expires_at": p.ExpiresAt})
	canonical, err := core.CanonicalJSON(intent)
	if err != nil {
		return p, err
	}
	if len(canonical) > 100000 {
		return p, core.Invalid("actions", "目录路径信息过大，请拆分计划")
	}
	p.PlanHash = core.Digest(canonical)
	raw, _ := json.Marshal(preview)
	_, err = tx.Exec(ctx, `INSERT INTO agent_organization_plans(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,origin_run_id,project_id,intent,plan_hash,preview,reason,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, p.ID, a.OrgID, c.ID, c.CurrentRevisionID, c.ContextSecurityVersion, a.UserID, a.AuthVersion, a.OrgAuthVersion, p.OriginRunID, p.ProjectID, string(canonical), p.PlanHash, raw, p.Reason, p.CreatedAt, p.ExpiresAt)
	if err == nil {
		err = appendOrganizationEvent(ctx, tx, a, p, p.Reason)
	}
	return p, err
}
func (s *Service) ProposeOrganization(ctx context.Context, cmd core.Command, id string, in OrganizationPlanCreate) (core.Result, error) {
	if cmd.Actor.RunID != "" {
		return core.Result{}, core.Forbidden()
	}
	cmd.AuthorizationMutation = true
	var c Conversation
	r, err := s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		var e error
		c, e = organizationConversation(ctx, tx, cmd.Actor, id, "propose", true)
		if e == nil {
			e = require(ctx, tx, cmd.Actor, c, "agent.run")
		}
		return e
	}, func(tx pgx.Tx) (core.Result, error) {
		p, e := proposeOrganization(ctx, tx, cmd.Actor, c, in)
		return core.Result{Status: 201, Data: p, Event: organizationEvent(p, in.Reason)}, e
	})
	return s.currentOrganizationResult(ctx, cmd.Actor, r, err)
}
func (s *Service) GetOrganizationPlan(ctx context.Context, a core.Actor, id string) (OrganizationPlan, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return OrganizationPlan{}, err
	}
	defer tx.Rollback(ctx)
	p, err := organizationPlanRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return p, err
	}
	c, err := organizationConversation(ctx, tx, a, p.ConversationID, "read", false)
	if err != nil {
		return OrganizationPlan{}, err
	}
	p, err = presentOrganizationPlan(ctx, tx, a, c, p)
	if err != nil {
		return OrganizationPlan{}, err
	}
	return p, tx.Commit(ctx)
}
func (s *Service) ListOrganizationPlans(ctx context.Context, a core.Actor, id string) (OrganizationPlanList, error) {
	out := OrganizationPlanList{Items: []OrganizationPlan{}}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := organizationConversation(ctx, tx, a, id, "read", false)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM agent_organization_plans WHERE org_id=$1 AND conversation_id=$2 AND actor_id=$3 ORDER BY created_at DESC,id DESC LIMIT 51`, a.OrgID, id, a.UserID)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = len(ids) > 50
	if out.HasMore {
		ids = ids[:50]
	}
	for _, id := range ids {
		p, e := organizationPlanRecord(ctx, tx, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		p, e = presentOrganizationPlan(ctx, tx, a, c, p)
		if e != nil {
			if code := core.PublicError(e).Status; code == 403 || code == 404 {
				continue
			}
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, tx.Commit(ctx)
}
func (s *Service) currentOrganizationResult(ctx context.Context, a core.Actor, r core.Result, err error) (core.Result, error) {
	if err != nil {
		return r, err
	}
	raw, e := json.Marshal(r.Data)
	if e != nil {
		return r, e
	}
	var stored OrganizationPlan
	if e = json.Unmarshal(raw, &stored); e != nil {
		return r, e
	}
	current, e := s.GetOrganizationPlan(ctx, a, stored.ID)
	if e == nil {
		r.Data = current
	}
	return r, e
}
func (s *Service) commitOrganization(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, p OrganizationPlan, in PlanAction, request string) (OrganizationPlan, error) {
	if p.PlanHash != in.PlanHash {
		return p, core.Fail(409, "PLAN_HASH_MISMATCH", "请核对准确计划摘要")
	}
	if p.State == "committed" {
		if in.ExpectedVersion != 2 {
			return p, core.Conflict()
		}
		return p, organizationReadable(ctx, tx, a, c, p)
	}
	if p.Version != in.ExpectedVersion {
		return p, core.Conflict()
	}
	if p.State != "approved" {
		return p, core.Fail(409, "PLAN_APPROVAL_REQUIRED", "需要本人先通过两步验证批准准确整理计划")
	}
	if err := organizationAuthority(ctx, tx, a, c, p, true); err != nil {
		return p, err
	}
	var approved bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM agent_organization_events WHERE org_id=$1 AND plan_id=$2 AND version=2 AND kind='approved' AND plan_hash=$3 AND actor_id=$4)", a.OrgID, p.ID, p.PlanHash, a.UserID).Scan(&approved); err != nil {
		return p, err
	}
	if !approved {
		return p, core.Fail(409, "PLAN_APPROVAL_REQUIRED", "未找到本人批准回执")
	}
	business := files.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
	for i, v := range p.Preview {
		act := v.Action
		var payload any
		if act.Kind == "folder.create" {
			parent := act.ID
			payload = files.FolderCreate{ParentID: &parent, ProjectID: c.ProjectID, Name: *act.Name, Classification: v.Source.Classification}
		} else {
			payload = files.Move{ExpectedVersion: act.ExpectedVersion, TargetFolderID: act.TargetFolderID, NewName: act.Name, Reason: p.Reason}
		}
		raw, _ := json.Marshal(payload)
		cmd := core.Command{Actor: a, RequestID: request, Route: "/internal/agent-organization/" + act.Kind, Key: fmt.Sprintf("agent-organization:%s:%d", p.ID, i), Body: raw}
		var result core.Result
		var err error
		switch act.Kind {
		case "folder.create":
			result, err = business.CreateFolder(ctx, cmd, payload.(files.FolderCreate))
		case "folder.move":
			result, err = business.MoveFolder(ctx, cmd, act.ID, payload.(files.Move))
		case "file.move":
			result, err = business.MoveFile(ctx, cmd, act.ID, payload.(files.Move))
		}
		if err != nil {
			return p, err
		}
		data, err := json.Marshal(result.Data)
		if err != nil {
			return p, err
		}
		if !core.ValidID(result.OperationID) {
			return p, core.Fail(500, "PLAN_RECEIPT_INVALID", "业务操作回执不完整")
		}
		p.Results = append(p.Results, OrganizationOutcome{OperationID: result.OperationID, Data: data})
	}
	p.Version = 3
	p.State = "committed"
	op := p.Results[0].OperationID
	p.OperationID = &op
	raw, _ := json.Marshal(p.Results)
	_, err := tx.Exec(ctx, "UPDATE agent_organization_plans SET version=3,state='committed',operation_id=$3,result_data=$4 WHERE org_id=$1 AND id=$2", a.OrgID, p.ID, p.OperationID, raw)
	if err == nil {
		err = appendOrganizationEvent(ctx, tx, a, p, in.Reason)
	}
	return p, err
}
func (s *Service) ActOnOrganizationPlan(ctx context.Context, cmd core.Command, id, action string, in PlanAction) (core.Result, error) {
	if cmd.Actor.RunID != "" || !slices.Contains([]string{"approve", "revoke", "commit"}, action) {
		return core.Result{}, core.Forbidden()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || len(in.PlanHash) != 64 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("plan", "请核对计划摘要、版本和原因")
	}
	cmd.AuthorizationMutation = true
	var p OrganizationPlan
	var c Conversation
	r, err := s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		var e error
		p, e = organizationPlanRecord(ctx, tx, cmd.Actor.OrgID, id, true)
		if e != nil {
			return e
		}
		if p.ActorID != cmd.Actor.UserID {
			return core.Missing()
		}
		c, e = organizationConversation(ctx, tx, cmd.Actor, p.ConversationID, action, true)
		if e != nil {
			return e
		}
		if p.PlanHash != in.PlanHash {
			return core.Fail(409, "PLAN_HASH_MISMATCH", "计划摘要不匹配")
		}
		if e = identity.RequireInteractiveSession(ctx, tx, cmd.Actor, action != "revoke"); e != nil {
			return e
		}
		if e = organizationReadable(ctx, tx, cmd.Actor, c, p); e != nil {
			return e
		}
		if action == "revoke" || p.State == "committed" {
			return nil
		}
		return organizationAuthority(ctx, tx, cmd.Actor, c, p, true)
	}, func(tx pgx.Tx) (core.Result, error) {
		var e error
		alreadyCommitted := p.State == "committed"
		if action == "commit" {
			p, e = s.commitOrganization(ctx, tx, cmd.Actor, c, p, in, cmd.RequestID)
		} else {
			if p.Version != in.ExpectedVersion {
				return core.Result{}, core.Conflict()
			}
			if (action == "approve" && p.State != "proposed") || (action == "revoke" && !slices.Contains([]string{"proposed", "approved"}, p.State)) {
				return core.Result{}, core.Fail(409, "PLAN_STATE_CONFLICT", "当前计划状态不允许此操作")
			}
			p.Version++
			p.State = "revoked"
			if action == "approve" {
				p.State = "approved"
			}
			_, e = tx.Exec(ctx, "UPDATE agent_organization_plans SET version=$3,state=$4 WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, p.ID, p.Version, p.State)
			if e == nil {
				e = appendOrganizationEvent(ctx, tx, cmd.Actor, p, in.Reason)
			}
		}
		event := organizationEvent(p, in.Reason)
		if alreadyCommitted && action == "commit" {
			event.Type = "agent.organization.reconciled"
		}
		return core.Result{Data: p, Event: event}, e
	})
	return s.currentOrganizationResult(ctx, cmd.Actor, r, err)
}
