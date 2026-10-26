package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

// commitProjectPlan runs the existing project command in the caller's transaction.
// A successful receipt, project/folders/template initialization, plan head, audit
// and outbox records are therefore visible together, or none of them are visible.
func (s *Service) commitProjectPlan(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan, c Conversation, in PlanAction, requestID string) (Plan, error) {
	if p.PlanHash != in.PlanHash {
		return p, core.Fail(409, "PLAN_HASH_MISMATCH", "所批准的计划内容与当前计划不一致")
	}
	if p.State == "committed" {
		if in.ExpectedVersion != 2 {
			return p, core.Fail(409, "PLAN_VERSION_CONFLICT", "请读取最新的计划版本")
		}
		return p, plannedProjectReadable(ctx, tx, a, p)
	}
	if err := planAuthority(ctx, tx, a, p, c); err != nil {
		return p, err
	}
	if p.Version != in.ExpectedVersion {
		return p, core.Fail(409, "PLAN_VERSION_CONFLICT", "计划状态已变化，请读取最新版本")
	}
	if p.State != "approved" {
		return p, core.Fail(409, "PLAN_APPROVAL_REQUIRED", "需要本人先批准这份准确计划，才能创建项目")
	}
	var approved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_plan_events WHERE org_id=$1 AND plan_id=$2 AND version=2 AND kind='approved' AND plan_hash=$3 AND actor_id=$4)`, a.OrgID, p.ID, p.PlanHash, a.UserID).Scan(&approved); err != nil {
		return p, err
	}
	if !approved {
		return p, core.Fail(409, "PLAN_APPROVAL_REQUIRED", "未找到匹配的人工批准回执")
	}
	payload, err := json.Marshal(p.Project)
	if err != nil {
		return p, err
	}
	command := core.Command{Actor: a, RequestID: requestID, Route: "/internal/agent-plans/project.create", Key: "agent-plan:" + p.ID, Body: payload}
	business := project.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
	result, err := business.Create(ctx, command, p.Project)
	if err != nil {
		return p, err
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return p, err
	}
	var created project.Project
	if err = json.Unmarshal(raw, &created); err != nil {
		return p, err
	}
	if !core.ValidID(created.ID) || !core.ValidID(result.OperationID) {
		return p, core.Fail(500, "PLAN_RECEIPT_INVALID", "业务回执不完整")
	}
	p.Version++
	p.State = "committed"
	p.OperationID = &result.OperationID
	p.ProjectID = &created.ID
	_, err = tx.Exec(ctx, "UPDATE agent_plans SET version=$3,state=$4,operation_id=$5,project_id=$6 WHERE org_id=$1 AND id=$2", a.OrgID, p.ID, p.Version, p.State, p.OperationID, p.ProjectID)
	if err != nil {
		return p, err
	}
	err = appendPlanEvent(ctx, tx, a, p, in.Reason)
	return p, err
}
func (s *Service) ActOnPlan(ctx context.Context, cmd core.Command, id, action string, in PlanAction) (core.Result, error) {
	if cmd.Actor.RunID != "" || !slices.Contains([]string{"approve", "revoke", "commit"}, action) {
		return core.Result{}, core.Forbidden()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || len(in.PlanHash) != 64 {
		return core.Result{}, core.Invalid("plan", "请提供准确计划摘要与版本")
	}
	if err := core.Name(strings.TrimSpace(in.Reason)); err != nil {
		return core.Result{}, err
	}
	cmd.AuthorizationMutation = true
	var p Plan
	var c Conversation
	result, err := s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		var e error
		p, e = planRecord(ctx, tx, cmd.Actor.OrgID, id, true)
		if e != nil {
			return e
		}
		if p.ActorID != cmd.Actor.UserID {
			return core.Missing()
		}
		c, e = planConversation(ctx, tx, cmd.Actor, p.ConversationID, "agent.plan."+action)
		if e != nil {
			return e
		}
		if p.PlanHash != in.PlanHash {
			return core.Fail(409, "PLAN_HASH_MISMATCH", "计划摘要不匹配，请重新读取")
		}
		if e = identity.RequireInteractiveSession(ctx, tx, cmd.Actor, action == "approve" || action == "commit"); e != nil {
			return e
		}
		if p.State == "committed" {
			return plannedProjectReadable(ctx, tx, cmd.Actor, p)
		}
		if action == "revoke" {
			return nil
		}
		return planAuthority(ctx, tx, cmd.Actor, p, c)
	}, func(tx pgx.Tx) (core.Result, error) {
		var e error
		alreadyCommitted := p.State == "committed"
		if action == "commit" {
			p, e = s.commitProjectPlan(ctx, tx, cmd.Actor, p, c, in, cmd.RequestID)
		} else {
			if p.Version != in.ExpectedVersion {
				return core.Result{}, core.Fail(409, "PLAN_VERSION_CONFLICT", "计划已变化，请重新读取")
			}
			if (action == "approve" && p.State != "proposed") || (action == "revoke" && !slices.Contains([]string{"proposed", "approved"}, p.State)) {
				return core.Result{}, core.Fail(409, "PLAN_STATE_CONFLICT", "当前计划状态不允许此操作")
			}
			p.Version++
			if action == "approve" {
				p.State = "approved"
			} else {
				p.State = "revoked"
			}
			_, e = tx.Exec(ctx, "UPDATE agent_plans SET version=$3,state=$4 WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, p.ID, p.Version, p.State)
			if e == nil {
				e = appendPlanEvent(ctx, tx, cmd.Actor, p, in.Reason)
			}
		}
		event := planEvent(p, in.Reason)
		if alreadyCommitted && action == "commit" {
			event.Type = "agent.plan.reconciled"
		}
		return core.Result{Data: p, Event: event}, e
	})
	return s.currentPlanResult(ctx, cmd.Actor, result, err)
}

// Model tools can consume an already approved exact grant, but never manufacture
// an interactive approval or borrow another user's plan.
func authorizePlanTool(ctx context.Context, tx pgx.Tx, a core.Actor, p Plan, c Conversation, action string) error {
	if p.ActorID != a.UserID || p.ConversationID != c.ID || c.Type != "private" || c.ProjectID != nil {
		return core.Missing()
	}
	if err := authz.RequireOrganization(ctx, tx, a, "agent.plan."+action); err != nil {
		return err
	}
	if err := require(ctx, tx, a, c, "conversation.read"); err != nil {
		return err
	}
	return nil
}
