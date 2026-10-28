package notification

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/workflow"
	"errors"
	"github.com/jackc/pgx/v5"
	"slices"
)

func snapshot(ctx context.Context, tx pgx.Tx, a core.Actor) error {
	if a.RunID != "" || !slices.Contains([]string{"web", "wechat", "douyin", "android"}, a.Channel) {
		return core.Forbidden()
	}
	var org, user int64
	var state string
	if err := tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&org); err != nil {
		return platform.DatabaseError(err)
	}
	if err := tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&user, &state); err != nil {
		return platform.DatabaseError(err)
	}
	if org != a.OrgAuthVersion || user != a.AuthVersion || state != "active" {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	return nil
}

// There is deliberately no stored business title/body. The current domain read
// rule runs in this same authorization snapshot before a label or link exists.
func view(ctx context.Context, tx pgx.Tx, a core.Actor, n thread) (*Notification, error) {
	if a.OrgID != n.OrgID || a.UserID != n.RecipientID {
		return nil, core.Missing()
	}
	v := Notification{ID: n.ID, Category: n.Category, Title: categories[n.Category], Read: n.ReadSeq == n.DeliveredSeq,
		DeliverySeq: n.DeliveredSeq, ReadSeq: n.ReadSeq, BusinessEventID: n.EventID, TemplateVersion: 1,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt, ReadAt: n.ReadAt, Target: Target{Type: n.ResourceType, ID: n.ResourceID}}
	var err error
	switch n.ResourceType {
	case "task":
		var t project.Task
		t, err = project.NotificationTask(ctx, tx, a, n.ResourceID)
		if err == nil {
			if n.Category == "mention" {
				if n.ContextID == nil {
					return nil, core.Missing()
				}
				var leaf string
				leaf, err = knowledge.NotificationTaskMention(ctx, tx, a, *n.ContextID, t.ID)
				if err != nil {
					break
				}
				v.Target.CommentID = &leaf
			} else if t.AssigneeID == nil || *t.AssigneeID != a.UserID {
				return nil, nil
			}
			v.ResourceLabel = t.Title
			v.CurrentState = t.State
			v.AttentionRequired = !slices.Contains([]string{"done", "cancelled"}, t.State)
			v.Target.WebPath = "/projects/" + t.ProjectID + "?task=" + t.ID
			v.Target.ContainerID = &t.ProjectID
			if v.Target.CommentID != nil {
				v.Target.WebPath += "&comment=" + *v.Target.CommentID
				v.AttentionRequired = !v.Read
			}
		}
	case "request":
		var r workflow.Request
		r, err = workflow.NotificationRequest(ctx, tx, a, n.ResourceID)
		if err == nil {
			v.ResourceLabel = r.Title
			v.CurrentState = r.State
			v.Target.WebPath = "/requests/" + r.ID
			if n.Category == "approval" {
				found := false
				for _, step := range r.Steps {
					if n.ContextID != nil && step.ID == *n.ContextID && step.AssignedUserID == a.UserID {
						found = true
						v.AttentionRequired = step.CanDecide
					}
				}
				if !found {
					return nil, nil
				}
			} else {
				if r.ApplicantID != a.UserID {
					return nil, nil
				}
				v.AttentionRequired = r.State == "returned"
			}
		}
	case "contract":
		var c contracts.Contract
		c, err = contracts.NotificationContract(ctx, tx, a, n.ResourceID)
		if err == nil {
			v.ResourceLabel = c.Title
			v.CurrentState = c.Lifecycle
			v.Target.WebPath = "/contracts/" + c.ID
			v.AttentionRequired = c.EndDate != nil && c.Lifecycle == "effective" && ((c.OwnerID != nil && *c.OwnerID == a.UserID) || (c.OwnerID == nil && c.CreatorID == a.UserID))
			if v.AttentionRequired {
				var current bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_due_events n WHERE n.org_id=$1 AND n.event_id=$2
     AND n.contract_id=$3 AND n.revision_id=$4::uuid AND n.due_on=$5::date)`, a.OrgID, n.EventID, c.ID, c.ActiveRevisionID, c.EndDate).Scan(&current)
				v.AttentionRequired = err == nil && current
			}
		}
	case "subscription":
		var sub finance.Subscription
		sub, err = finance.NotificationSubscription(ctx, tx, a, n.ResourceID)
		if err == nil {
			v.ResourceLabel = sub.Product
			v.CurrentState = sub.State
			if n.ContextID == nil {
				return nil, core.Missing()
			}
			v.Target = Target{Type: "subscription_period", ID: *n.ContextID, ContainerID: &sub.ID, WebPath: "/subscription-periods/" + *n.ContextID}
			var state string
			var owner *string
			var eligible bool
			err = tx.QueryRow(ctx, `SELECT p.state,r.owner_id,subscription_charge_eligible(p.org_id,p.subscription_id,p.start_date)
    FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
    WHERE p.org_id=$1 AND p.subscription_id=$2 AND p.id=$3`, a.OrgID, sub.ID, *n.ContextID).Scan(&state, &owner, &eligible)
			if err == nil {
				if owner == nil || *owner != a.UserID {
					return nil, nil
				}
				v.CurrentState = state
				v.AttentionRequired = state == "forecast" && eligible
			}
		}
	case "file":
		var f files.File
		f, err = files.NotificationFile(ctx, tx, a, n.ResourceID)
		if err == nil {
			v.ResourceLabel = f.Name
			v.CurrentState = f.State
			v.Target.WebPath = "/files/" + f.ID
			if n.ContextID == nil {
				return nil, core.Missing()
			}
			var state string
			err = tx.QueryRow(ctx, `SELECT r.state FROM file_replicas r JOIN file_versions fv ON fv.org_id=r.org_id AND fv.id=r.version_id
    WHERE r.org_id=$1 AND r.id=$2 AND fv.file_id=$3`, a.OrgID, *n.ContextID, f.ID).Scan(&state)
			v.AttentionRequired = err == nil && state == "failed"
			v.CurrentState = state
		}
	case "incident":
		var incident servicestatus.Incident
		incident, err = servicestatus.NotificationIncident(ctx, tx, a, n.ResourceID)
		if err == nil {
			v.ResourceLabel = incident.Title
			v.CurrentState = incident.State
			v.Target.WebPath = "/incidents/" + incident.ID
			var responsible bool
			err = tx.QueryRow(ctx, "SELECT $2::uuid=$3::uuid OR EXISTS(SELECT 1 FROM services WHERE org_id=$1 AND id=ANY($4::uuid[]) AND owner_id=$2)", a.OrgID, a.UserID, incident.OwnerID, incident.ServiceIDs).Scan(&responsible)
			v.AttentionRequired = responsible && incident.State != "resolved"
		}
	case "agent_run":
		var run agent.Run
		run, err = agent.NotificationRun(ctx, tx, a, n.ResourceID)
		if err == nil {
			if run.ActorID != a.UserID {
				return nil, nil
			}
			v.ResourceLabel = "公司秘书任务"
			v.CurrentState = run.State
			v.Target.WebPath = "/conversations/" + run.ConversationID + "?run=" + run.ID
			v.Target.ContainerID = &run.ConversationID
			v.AttentionRequired = slices.Contains([]string{"failed", "interrupted"}, run.State) || run.Outcome != nil && slices.Contains([]string{"awaiting_input", "conflict", "denied", "failed"}, *run.Outcome)
		}
	default:
		return nil, core.Missing()
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		e := core.PublicError(platform.DatabaseError(err))
		if e.Status == 403 || e.Status == 404 || e.Status == 409 {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}
