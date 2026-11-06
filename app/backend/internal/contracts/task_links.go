package contracts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type MilestoneTaskInput struct {
	ContractVersion            int64  `json:"contract_version"`
	ContractRevisionID         string `json:"contract_revision_id"`
	TaskID                     string `json:"task_id"`
	TaskVersion                int64  `json:"task_version"`
	AcknowledgeScopeDifference bool   `json:"acknowledge_scope_difference"`
	Reason                     string `json:"reason"`
}

type MilestoneTaskCurrent struct {
	ID        string `json:"id"`
	Version   int64  `json:"version"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	State     string `json:"state"`
}

type MilestoneTaskEvent struct {
	ID                          string                `json:"id"`
	ContractID                  string                `json:"contract_id"`
	MilestoneID                 string                `json:"milestone_id"`
	ContractRevisionID          string                `json:"contract_revision_id"`
	ContractVersion             int64                 `json:"contract_version"`
	Seq                         int64                 `json:"seq"`
	Action                      string                `json:"action"`
	SourceLinkID                *string               `json:"source_link_id"`
	TaskID                      string                `json:"task_id"`
	TaskVersion                 int64                 `json:"task_version"`
	TaskProjectID               string                `json:"task_project_id"`
	TaskTitle                   string                `json:"task_title"`
	TaskState                   string                `json:"task_state"`
	CrossScope                  bool                  `json:"cross_scope"`
	ScopeDifferenceAcknowledged bool                  `json:"scope_difference_acknowledged"`
	ActorID                     string                `json:"actor_id"`
	CreatedAt                   time.Time             `json:"created_at"`
	Reason                      string                `json:"reason"`
	Revoked                     bool                  `json:"revoked"`
	CurrentTask                 *MilestoneTaskCurrent `json:"current_task"`
}

const taskEventColumns = `e.id,e.contract_id,e.milestone_id,e.contract_revision_id,e.contract_version,e.seq,e.action,e.source_link_id,
 e.task_id,e.task_version,e.task_project_id,e.task_title,e.task_state,e.cross_scope,e.scope_difference_acknowledged,e.actor_id,e.created_at,e.reason`

func scanTaskEvent(row pgx.Row) (MilestoneTaskEvent, error) {
	var e MilestoneTaskEvent
	err := row.Scan(&e.ID, &e.ContractID, &e.MilestoneID, &e.ContractRevisionID, &e.ContractVersion, &e.Seq, &e.Action, &e.SourceLinkID,
		&e.TaskID, &e.TaskVersion, &e.TaskProjectID, &e.TaskTitle, &e.TaskState, &e.CrossScope, &e.ScopeDifferenceAcknowledged, &e.ActorID, &e.CreatedAt, &e.Reason)
	return e, platform.DatabaseError(err)
}

func taskReference(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock bool) (MilestoneTaskCurrent, error) {
	var t MilestoneTaskCurrent
	if lock {
		// Match ordinary task commands: lock the parent project before its task.
		if _, err := db.Exec(ctx, `SELECT p.id FROM projects p JOIN tasks t ON t.org_id=p.org_id AND t.project_id=p.id
            WHERE t.org_id=$1 AND t.id=$2 FOR SHARE OF p`, a.OrgID, id); err != nil {
			return t, err
		}
	}
	var creator, classification string
	query := `SELECT t.id,t.version,t.project_id,t.title,t.state,t.creator_id,p.classification FROM tasks t
        JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id WHERE t.org_id=$1 AND t.id=$2`
	if lock {
		query += " FOR SHARE OF t"
	}
	err := db.QueryRow(ctx, query, a.OrgID, id).Scan(&t.ID, &t.Version, &t.ProjectID, &t.Title, &t.State, &creator, &classification)
	if err != nil {
		return t, platform.DatabaseError(err)
	}
	err = authz.Require(ctx, db, a, "task.read", authz.Resource{Type: "task", ID: t.ID, ProjectID: &t.ProjectID, OwnerID: &creator, Classification: classification})
	return t, err
}

func taskNode(ctx context.Context, db platform.DBTX, a core.Actor, d Contract, node string) (string, error) {
	var revision string
	err := db.QueryRow(ctx, `SELECT revision_id FROM contract_milestones WHERE org_id=$1 AND contract_id=$2 AND id=$3`, a.OrgID, d.ID, node).Scan(&revision)
	return revision, platform.DatabaseError(err)
}

// Current links and history remain metadata references. A task's later progress
// is shown separately from the exact version reviewed when a link was written.
func (s *Service) MilestoneTasks(ctx context.Context, a core.Actor, id, node, cursor string, limit int, history bool) (core.Page[MilestoneTaskEvent], error) {
	out := core.Page[MilestoneTaskEvent]{Items: []MilestoneTaskEvent{}}
	if !core.ValidID(id) || !core.ValidID(node) {
		return out, core.Missing()
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", a.OrgID); err != nil {
		return out, err
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	d, err := metadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, tx, a, "contract.read", resource(d)); err != nil {
		return out, err
	}
	if _, err = taskNode(ctx, tx, a, d, node); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "contract-milestone-tasks", id, node, fmt.Sprint(history))
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		value, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		before, beforeID = &value.UpdatedAt, &value.ID
	}
	rows, err := tx.Query(ctx, `SELECT `+taskEventColumns+`,
        EXISTS(SELECT 1 FROM contract_milestone_task_events r WHERE r.org_id=e.org_id AND r.action='revoke' AND r.source_link_id=e.id),
        t.version,t.title,t.state FROM contract_milestone_task_events e JOIN tasks t ON t.org_id=e.org_id AND t.id=e.task_id
        JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
        WHERE e.org_id=$1 AND e.contract_id=$2 AND e.milestone_id=$3
        AND can_access(p.org_id,$4,'task.read','task',t.id,p.id,t.creator_id,p.classification,$5,false)
        AND ($6 OR (e.action='link' AND NOT EXISTS(SELECT 1 FROM contract_milestone_task_events r WHERE r.org_id=e.org_id AND r.action='revoke' AND r.source_link_id=e.id)))
        AND ($7::timestamptz IS NULL OR (e.created_at,e.id)<($7,$8::uuid)) ORDER BY e.created_at DESC,e.id DESC LIMIT $9`,
		a.OrgID, id, node, a.UserID, a.Channel, history, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var e MilestoneTaskEvent
		t := &MilestoneTaskCurrent{}
		err = rows.Scan(&e.ID, &e.ContractID, &e.MilestoneID, &e.ContractRevisionID, &e.ContractVersion, &e.Seq, &e.Action, &e.SourceLinkID,
			&e.TaskID, &e.TaskVersion, &e.TaskProjectID, &e.TaskTitle, &e.TaskState, &e.CrossScope, &e.ScopeDifferenceAcknowledged, &e.ActorID, &e.CreatedAt, &e.Reason,
			&e.Revoked, &t.Version, &t.Title, &t.State)
		if err != nil {
			rows.Close()
			return out, err
		}
		t.ID, t.ProjectID, e.CurrentTask = e.TaskID, e.TaskProjectID, t
		out.Items = append(out.Items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		value := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &value
	}
	return out, tx.Commit(ctx)
}

func (s *Service) LinkMilestoneTask(ctx context.Context, c core.Command, id, node, root string, in MilestoneTaskInput) (core.Result, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !core.ValidID(id) || !core.ValidID(node) || !core.ValidID(in.TaskID) || !core.ValidID(in.ContractRevisionID) || (root != "" && !core.ValidID(root)) {
		return core.Result{}, core.Invalid("task_id", "请选择准确的合同节点、任务和生效修订")
	}
	if core.RequireVersion(in.ContractVersion) != nil || core.RequireVersion(in.TaskVersion) != nil || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请核对合同与任务版本并填写操作依据")
	}
	action := "link"
	if root != "" {
		action = "revoke"
	}
	check := func(tx pgx.Tx) (Contract, MilestoneTaskCurrent, error) {
		var task MilestoneTaskCurrent
		if err := identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return Contract{}, task, err
		}
		d, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return d, task, err
		}
		for _, permission := range []string{"contract.read", "contract.execute"} {
			if err = authz.Require(ctx, tx, c.Actor, permission, resource(d)); err != nil {
				return d, task, err
			}
		}
		if d.ProjectID != nil {
			if _, err = tx.Exec(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR SHARE", c.Actor.OrgID, *d.ProjectID); err != nil {
				return d, task, err
			}
		}
		task, err = taskReference(ctx, tx, c.Actor, in.TaskID, true)
		return d, task, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, _, err := check(tx); return err }, func(tx pgx.Tx) (core.Result, error) {
		if _, err := tx.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		d, task, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if d.Version != in.ContractVersion || task.Version != in.TaskVersion || d.ActiveRevisionID == nil || *d.ActiveRevisionID != in.ContractRevisionID {
			return core.Result{}, core.Conflict()
		}
		if d.DeletedAt != nil || (action == "link" && d.Lifecycle != "effective") || (action == "revoke" && d.Lifecycle != "effective" && d.Lifecycle != "completed" && d.Lifecycle != "terminated") {
			return core.Result{}, core.Fail(409, "CONTRACT_NOT_EFFECTIVE", "请核对合同当前阶段；新关联仅用于生效合同")
		}
		if err = projectWritable(ctx, tx, c.Actor.OrgID, d.ProjectID, d.Classification, true); err != nil {
			return core.Result{}, err
		}
		revision, err := taskNode(ctx, tx, c.Actor, d, node)
		if err != nil {
			return core.Result{}, err
		}
		if revision != in.ContractRevisionID {
			return core.Result{}, core.Conflict()
		}
		crossScope := d.ProjectID == nil || *d.ProjectID != task.ProjectID
		if crossScope && !in.AcknowledgeScopeDifference {
			return core.Result{}, core.Invalid("acknowledge_scope_difference", "合同与任务范围不同，请逐项核对两侧权限和用途后明确确认")
		}
		if root != "" {
			var valid bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contract_milestone_task_events e WHERE e.org_id=$1 AND e.id=$2 AND e.contract_id=$3 AND e.milestone_id=$4
                AND e.task_id=$5 AND e.action='link' AND NOT EXISTS(SELECT 1 FROM contract_milestone_task_events r WHERE r.org_id=e.org_id AND r.action='revoke' AND r.source_link_id=e.id))`, c.Actor.OrgID, root, id, node, task.ID).Scan(&valid)
			if err != nil {
				return core.Result{}, err
			}
			if !valid {
				return core.Result{}, core.Conflict()
			}
		} else {
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contract_milestone_task_events e WHERE e.org_id=$1 AND e.contract_id=$2 AND e.milestone_id=$3 AND e.task_id=$4
                AND e.action='link' AND NOT EXISTS(SELECT 1 FROM contract_milestone_task_events r WHERE r.org_id=e.org_id AND r.action='revoke' AND r.source_link_id=e.id))`, c.Actor.OrgID, id, node, task.ID).Scan(&exists)
			if err != nil {
				return core.Result{}, err
			}
			if exists {
				return core.Result{}, core.Fail(409, "CONTRACT_TASK_ALREADY_LINKED", "该准确节点已有此任务的有效关联，请刷新当前记录")
			}
		}
		var source *string
		if root != "" {
			source = &root
		}
		e, err := scanTaskEvent(tx.QueryRow(ctx, `INSERT INTO contract_milestone_task_events AS e
            (id,org_id,contract_id,milestone_id,contract_revision_id,contract_version,seq,action,source_link_id,
             task_id,task_version,task_project_id,task_title,task_state,cross_scope,scope_difference_acknowledged,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,$6,(SELECT coalesce(max(seq),0)+1 FROM contract_milestone_task_events WHERE org_id=$2 AND contract_id=$3),$7,$8,
             $9,$10,$11,$12,$13,$14,$15,$16,s.id,s.mfa_verified_at,$18 FROM sessions s WHERE s.org_id=$2 AND s.id=$17 AND s.user_id=$16 RETURNING `+taskEventColumns,
			core.NewID(), c.Actor.OrgID, id, node, revision, d.Version, action, source, task.ID, task.Version, task.ProjectID, task.Title, task.State, crossScope, in.AcknowledgeScopeDifference, c.Actor.UserID, c.Actor.SessionID, in.Reason))
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: e, Event: core.Event{Type: "contract.task." + action, ResourceType: "contract", ResourceID: id, Version: e.Seq, Payload: map[string]any{"milestone_id": node, "task_id": task.ID, "task_version": task.Version, "event_id": e.ID, "source_link_id": source, "reason": in.Reason}}}, nil
	})
}
