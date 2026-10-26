package project

import (
	"context"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type Milestone struct {
	ID        string     `json:"id"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ProjectID string     `json:"project_id"`
	CreatorID string     `json:"creator_id"`
	Title     string     `json:"title"`
	OwnerID   *string    `json:"owner_id"`
	DueAt     *time.Time `json:"due_at"`
	State     string     `json:"state"`
	Overdue   bool       `json:"overdue"`
}
type MilestoneCreate struct {
	Title   string    `json:"title"`
	OwnerID string    `json:"owner_id"`
	DueAt   time.Time `json:"due_at"`
}
type MilestonePatch struct {
	ExpectedVersion int64      `json:"expected_version"`
	Title           *string    `json:"title,omitempty"`
	OwnerID         *string    `json:"owner_id,omitempty"`
	DueAt           *time.Time `json:"due_at,omitempty"`
}

const milestoneColumns = `r.id,r.version,r.created_at,r.updated_at,r.project_id,r.creator_id,r.title,r.owner_id,r.due_at,r.state`

func scanMilestone(row pgx.Row) (Milestone, error) {
	var m Milestone
	err := row.Scan(&m.ID, &m.Version, &m.CreatedAt, &m.UpdatedAt, &m.ProjectID, &m.CreatorID, &m.Title, &m.OwnerID, &m.DueAt, &m.State)
	m.Overdue = (m.State == "open" || m.State == "missed") && m.DueAt != nil && m.DueAt.Before(time.Now())
	return m, platform.DatabaseError(err)
}
func getMilestone(ctx context.Context, db platform.DBTX, org, id string) (Milestone, error) {
	if !core.ValidID(id) {
		return Milestone{}, core.Missing()
	}
	return scanMilestone(db.QueryRow(ctx, "SELECT "+milestoneColumns+" FROM milestones r WHERE r.org_id=$1 AND r.id=$2", org, id))
}
func (s *Service) ListMilestones(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[Milestone], error) {
	return listProjectResources(ctx, s, a, id, "milestones", "milestone", milestoneColumns, f, scanMilestone, func(m Milestone) (string, time.Time) { return m.ID, m.UpdatedAt })
}
func (s *Service) GetMilestone(ctx context.Context, a core.Actor, id string) (Milestone, error) {
	m, err := getMilestone(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return m, err
	}
	p, err := getProject(ctx, s.DB, a.OrgID, m.ProjectID, false)
	if err != nil {
		return m, err
	}
	return m, authz.Require(ctx, s.DB, a, "milestone.read", collaborationResource(p, "milestone", id, m.CreatorID))
}
func milestoneResult(m Milestone, event string) core.Result {
	status := 200
	if event == "milestone.created" {
		status = 201
	}
	return core.Result{Status: status, Data: m, Event: core.Event{Type: event, ResourceType: "milestone", ResourceID: m.ID, Version: m.Version, Payload: map[string]any{"project_id": m.ProjectID, "state": m.State}}}
}
func (s *Service) CreateMilestone(ctx context.Context, c core.Command, id string, input MilestoneCreate) (core.Result, error) {
	if err := core.Name(input.Title); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeProject(ctx, tx, c.Actor, id, "milestone.create") }, func(tx pgx.Tx) (core.Result, error) {
		p, err := getProject(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		if err = validAssignee(ctx, tx, c.Actor.OrgID, id, &input.OwnerID); err != nil {
			return core.Result{}, err
		}
		mID := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO milestones(id,org_id,project_id,title,owner_id,due_at,state,creator_id) VALUES($1,$2,$3,$4,$5,$6,'open',$7)`, mID, c.Actor.OrgID, id, strings.TrimSpace(input.Title), input.OwnerID, input.DueAt, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		m, err := getMilestone(ctx, tx, c.Actor.OrgID, mID)
		return milestoneResult(m, "milestone.created"), err
	})
}
func (s *Service) UpdateMilestone(ctx context.Context, c core.Command, id string, input MilestonePatch) (core.Result, error) {
	return s.mutateMilestone(ctx, c, id, input.ExpectedVersion, func(tx pgx.Tx, p Project, m Milestone) (Milestone, error) {
		if m.State == "achieved" || m.State == "cancelled" {
			return m, core.Fail(409, "MILESTONE_READ_ONLY", "请先重新打开里程碑")
		}
		if input.Title != nil {
			if err := core.Name(*input.Title); err != nil {
				return m, err
			}
			m.Title = strings.TrimSpace(*input.Title)
		}
		if input.OwnerID != nil {
			if err := validAssignee(ctx, tx, c.Actor.OrgID, p.ID, input.OwnerID); err != nil {
				return m, err
			}
			m.OwnerID = input.OwnerID
		}
		if input.DueAt != nil {
			m.DueAt = input.DueAt
		}
		_, err := tx.Exec(ctx, "UPDATE milestones SET title=$3,owner_id=$4,due_at=$5,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, m.Title, m.OwnerID, m.DueAt)
		return m, err
	}, "milestone.updated", "")
}
func (s *Service) TransitionMilestone(ctx context.Context, c core.Command, id string, input Transition) (core.Result, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("reason", "请填写里程碑变更原因")
	}
	return s.mutateMilestone(ctx, c, id, input.ExpectedVersion, func(tx pgx.Tx, p Project, m Milestone) (Milestone, error) {
		target := ""
		switch input.Action {
		case "achieve":
			if m.State == "open" || m.State == "missed" {
				target = "achieved"
			}
			var total, open int
			err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM tasks WHERE org_id=$1 AND project_id=$2 AND milestone_id=$3 AND state<>'cancelled')+
				(SELECT count(*) FROM deliverables WHERE org_id=$1 AND project_id=$2 AND milestone_id=$3),
				(SELECT count(*) FROM tasks WHERE org_id=$1 AND project_id=$2 AND milestone_id=$3 AND state NOT IN ('done','cancelled'))+
				(SELECT count(*) FROM deliverables WHERE org_id=$1 AND project_id=$2 AND milestone_id=$3 AND state<>'accepted')`, c.Actor.OrgID, p.ID, id).Scan(&total, &open)
			if err != nil {
				return m, err
			}
			if total == 0 || open > 0 {
				return m, core.Fail(422, "MILESTONE_EVIDENCE_INCOMPLETE", "须有已完成的关联任务或已验收交付，且所有关联事项均已完成")
			}
		case "miss":
			if m.State == "open" && m.DueAt != nil && m.DueAt.Before(time.Now()) {
				target = "missed"
			}
		case "cancel":
			if m.State == "open" || m.State == "missed" {
				target = "cancelled"
			}
		case "reopen":
			if m.State == "achieved" || m.State == "cancelled" || m.State == "missed" {
				target = "open"
			}
		}
		if target == "" {
			return m, core.UnexpectedState(m.State, input.Action)
		}
		_, err := tx.Exec(ctx, "UPDATE milestones SET state=$3,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, target)
		return m, err
	}, "milestone.transitioned", input.Reason)
}
func (s *Service) mutateMilestone(ctx context.Context, c core.Command, id string, version int64, fn func(pgx.Tx, Project, Milestone) (Milestone, error), event, reason string) (core.Result, error) {
	if err := core.RequireVersion(version); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		m, err := getMilestone(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return err
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, m.ProjectID, false)
		if err != nil {
			return err
		}
		return authz.Require(ctx, tx, c.Actor, "milestone.update", collaborationResource(p, "milestone", id, m.CreatorID))
	}, func(tx pgx.Tx) (core.Result, error) {
		p, err := collaborationProject(ctx, tx, c.Actor.OrgID, id, "milestones", true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		m, err := getMilestone(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if m.Version != version {
			return core.Result{}, core.Conflict()
		}
		if _, err = fn(tx, p, m); err != nil {
			return core.Result{}, err
		}
		m, err = getMilestone(ctx, tx, c.Actor.OrgID, id)
		result := milestoneResult(m, event)
		result.Event.Payload["reason"] = reason
		return result, err
	})
}
func validMilestone(ctx context.Context, db platform.DBTX, org, projectID string, id *string) error {
	if id == nil {
		return nil
	}
	m, err := getMilestone(ctx, db, org, *id)
	if err != nil {
		return err
	}
	if m.ProjectID != projectID {
		return core.Missing()
	}
	if m.State == "achieved" || m.State == "cancelled" {
		return core.Fail(409, "MILESTONE_READ_ONLY", "请先重开关联里程碑再变更任务")
	}
	return nil
}
