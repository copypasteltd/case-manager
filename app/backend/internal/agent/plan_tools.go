package agent

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

// Plan citations contain immutable user intent, never a general-purpose escape
// from conversation project scope. Created-project content is not returned by
// this projection; even its receipt requires current project/AI read authority.
func planSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string) (SourceSnapshot, error) {
	var out SourceSnapshot
	p, err := planRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if p.ConversationID != c.ID || p.ActorID != a.UserID || c.Type != "private" || c.ProjectID != nil {
		return out, core.Missing()
	}
	if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.read"); err != nil {
		return out, err
	}
	// A proposed project's scope is business data too. AI permission must apply
	// before the project exists, not only after a committed-project receipt.
	var classificationAllowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM classification_policies WHERE org_id=$1 AND level=$2
 AND policy_version=(SELECT max(policy_version) FROM classification_policies WHERE org_id=$1 AND level=$2)
 AND $3=ANY(allowed_channels) AND (NOT $4::boolean OR ai_allowed))`, a.OrgID, p.Project.Classification, a.Channel, a.RunID != "").Scan(&classificationAllowed); err != nil {
		return out, err
	}
	if !classificationAllowed {
		return out, core.Missing()
	}
	if err = plannedProjectReadable(ctx, tx, a, p); err != nil {
		return out, err
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_plan", ID: p.ID}, ResourceVersion: 1, SHA256: p.PlanHash}, nil
}

func executionPlan(ctx context.Context, tx pgx.Tx, st executionState, id string) (Plan, SourceSnapshot, error) {
	a := st.Actor
	a.RunID = st.Run.ID
	src, err := planSource(ctx, tx, a, st.Conversation, id)
	if err != nil {
		return Plan{}, SourceSnapshot{}, err
	}
	p, err := planRecord(ctx, tx, a.OrgID, id, false)
	if err == nil {
		p, err = presentPlan(ctx, tx, a, p, st.Conversation)
	}
	return p, src, err
}
func (s *Service) executionProjectPlan(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	a := st.Actor
	a.RunID = st.Run.ID
	var p Plan
	var err error
	if tool == "propose_project" {
		var intent struct {
			Project project.Create `json:"project"`
			Reason  string         `json:"reason"`
		}
		if err = strictArguments(args, &intent); err != nil {
			return nil, nil, err
		}
		// The execution lease already binds exact conversation authority. Models
		// neither invent security versions nor select an execution identity.
		if intent.Project.OwnerID == "" {
			intent.Project.OwnerID = a.UserID
		}
		in := ProjectPlanCreate{ExpectedVersion: st.Conversation.Version, ContextSecurityVersion: st.Conversation.ContextSecurityVersion, Project: intent.Project, Reason: intent.Reason}
		if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.propose"); err != nil {
			return nil, nil, err
		}
		if p, err = proposeProjectPlan(ctx, tx, a, st.Conversation, in); err != nil {
			return nil, nil, err
		}
		if err = executionAudit(ctx, tx, st, "agent.plan.proposed", map[string]any{"plan_id": p.ID, "plan_hash": p.PlanHash}); err != nil {
			return nil, nil, err
		}
	} else {
		var in struct {
			PlanID string `json:"plan_id"`
			PlanAction
		}
		if err = strictArguments(args, &in); err != nil {
			return nil, nil, err
		}
		if err = core.Name(in.Reason); err != nil {
			return nil, nil, err
		}
		p, err = planRecord(ctx, tx, a.OrgID, in.PlanID, true)
		if err != nil {
			return nil, nil, err
		}
		if err = authorizePlanTool(ctx, tx, a, p, st.Conversation, "commit"); err != nil {
			return nil, nil, err
		}
		if p, err = s.commitProjectPlan(ctx, tx, a, p, st.Conversation, in.PlanAction, core.NewID()); err != nil {
			return nil, nil, err
		}
		if err = executionAudit(ctx, tx, st, "agent.plan.committed", map[string]any{"plan_id": p.ID, "plan_hash": p.PlanHash, "operation_id": p.OperationID, "project_id": p.ProjectID}); err != nil {
			return nil, nil, err
		}
	}
	snapshot, err := planSource(ctx, tx, a, st.Conversation, p.ID)
	if err != nil {
		return nil, nil, err
	}
	refs := []ResultReference{{Type: "agent_plan", ID: p.ID}}
	status, summary, next := "pending", "项目计划已拟定，等待本人批准；项目尚未创建", "请在会话的项目计划中核对内容并完成两步验证后批准"
	committed := p.State == "committed"
	if committed {
		status, summary, next = "succeeded", "已按本人批准的准确计划创建项目并保存业务操作回执", ""
		refs = append(refs, ResultReference{Type: "project", ID: *p.ProjectID})
	}
	return map[string]any{"status": status, "resource_refs": refs, "source_refs": []ResultReference{reference(snapshot)}, "operation_id": p.OperationID, "committed": committed,
		"safe_summary": summary, "retryable": false, "next_action": next, "content": p}, []SourceSnapshot{snapshot}, nil
}
