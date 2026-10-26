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

type Concern struct {
	ID          string     `json:"id"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ProjectID   string     `json:"project_id"`
	CreatorID   string     `json:"creator_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	OwnerID     string     `json:"owner_id"`
	Severity    string     `json:"severity"`
	State       string     `json:"state"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	Probability *string    `json:"probability,omitempty"`
	Mitigation  string     `json:"mitigation,omitempty"`
	Resolution  string     `json:"resolution,omitempty"`
	IssueID     *string    `json:"issue_id,omitempty"`
}
type ConcernCreate struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	OwnerID     string     `json:"owner_id"`
	Severity    string     `json:"severity"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	Probability *string    `json:"probability,omitempty"`
	Mitigation  string     `json:"mitigation,omitempty"`
	Resolution  string     `json:"resolution,omitempty"`
}
type ConcernPatch struct {
	ExpectedVersion int64                    `json:"expected_version"`
	Title           *string                  `json:"title,omitempty"`
	Description     *string                  `json:"description,omitempty"`
	OwnerID         *string                  `json:"owner_id,omitempty"`
	Severity        *string                  `json:"severity,omitempty"`
	DueAt           core.Nullable[time.Time] `json:"due_at,omitempty"`
	Probability     core.Nullable[string]    `json:"probability,omitempty"`
	Mitigation      *string                  `json:"mitigation,omitempty"`
	Resolution      *string                  `json:"resolution,omitempty"`
	State           *string                  `json:"state,omitempty"`
	Reason          string                   `json:"reason,omitempty"`
}

const concernBase = `r.id,r.version,r.created_at,r.updated_at,r.project_id,r.creator_id,r.title,r.description,r.owner_id,r.severity,r.state,r.due_at`

func concernTable(kind string) (string, error) {
	switch kind {
	case "risk":
		return "risks", nil
	case "issue":
		return "issues", nil
	}
	return "", core.Missing()
}
func concernColumns(kind string) string {
	if kind == "risk" {
		return concernBase + ",r.probability,r.mitigation,r.issue_id"
	}
	return concernBase + ",r.resolution"
}
func scanConcern(kind string) func(pgx.Row) (Concern, error) {
	return func(row pgx.Row) (Concern, error) {
		var r Concern
		fields := []any{&r.ID, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ProjectID, &r.CreatorID, &r.Title, &r.Description, &r.OwnerID, &r.Severity, &r.State, &r.DueAt}
		if kind == "risk" {
			fields = append(fields, &r.Probability, &r.Mitigation, &r.IssueID)
		} else {
			fields = append(fields, &r.Resolution)
		}
		err := row.Scan(fields...)
		return r, platform.DatabaseError(err)
	}
}
func getConcern(ctx context.Context, db platform.DBTX, org, id, kind string) (Concern, error) {
	if !core.ValidID(id) {
		return Concern{}, core.Missing()
	}
	table, err := concernTable(kind)
	if err != nil {
		return Concern{}, err
	}
	return scanConcern(kind)(db.QueryRow(ctx, "SELECT "+concernColumns(kind)+" FROM "+table+" r WHERE r.org_id=$1 AND r.id=$2", org, id))
}
func (s *Service) ListConcerns(ctx context.Context, a core.Actor, id, kind string, f Filter) (core.Page[Concern], error) {
	table, err := concernTable(kind)
	if err != nil {
		return core.Page[Concern]{}, err
	}
	return listProjectResources(ctx, s, a, id, table, kind, concernColumns(kind), f, scanConcern(kind), func(r Concern) (string, time.Time) { return r.ID, r.UpdatedAt })
}
func (s *Service) GetConcern(ctx context.Context, a core.Actor, id, kind string) (Concern, error) {
	r, err := getConcern(ctx, s.DB, a.OrgID, id, kind)
	if err != nil {
		return r, err
	}
	p, err := getProject(ctx, s.DB, a.OrgID, r.ProjectID, false)
	if err != nil {
		return r, err
	}
	return r, authz.Require(ctx, s.DB, a, kind+".read", collaborationResource(p, kind, id, r.CreatorID))
}
func validateConcern(input ConcernCreate) error {
	if err := core.Name(input.Title); err != nil {
		return err
	}
	if len([]rune(input.Description)) > 20000 || len([]rune(input.Mitigation)) > 20000 || len([]rune(input.Resolution)) > 20000 {
		return core.Invalid("description", "说明超过20000个字符")
	}
	if !core.ValidID(input.OwnerID) {
		return core.Invalid("owner_id", "请选择有效负责人")
	}
	switch input.Severity {
	case "low", "medium", "high", "critical":
	default:
		return core.Invalid("severity", "严重程度无效")
	}
	if input.Probability != nil && *input.Probability != "low" && *input.Probability != "medium" && *input.Probability != "high" {
		return core.Invalid("probability", "风险概率无效")
	}
	return nil
}
func concernResult(r Concern, kind, event string, created bool, reason string) core.Result {
	status := 200
	if created {
		status = 201
	}
	return core.Result{Status: status, Data: r, Event: core.Event{Type: event, ResourceType: kind, ResourceID: r.ID, Version: r.Version,
		Payload: map[string]any{"project_id": r.ProjectID, "state": r.State, "issue_id": r.IssueID, "reason": reason}}}
}
func (s *Service) CreateConcern(ctx context.Context, c core.Command, id, kind string, input ConcernCreate) (core.Result, error) {
	if _, err := concernTable(kind); err != nil {
		return core.Result{}, err
	}
	if err := validateConcern(input); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeProject(ctx, tx, c.Actor, id, kind+".create") }, func(tx pgx.Tx) (core.Result, error) {
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
		rID := core.NewID()
		if kind == "risk" {
			_, err = tx.Exec(ctx, `INSERT INTO risks(id,org_id,project_id,creator_id,title,description,owner_id,severity,due_at,probability,mitigation,state)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'open')`, rID, c.Actor.OrgID, id, c.Actor.UserID, strings.TrimSpace(input.Title), input.Description, input.OwnerID, input.Severity, input.DueAt, input.Probability, input.Mitigation)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO issues(id,org_id,project_id,creator_id,title,description,owner_id,severity,due_at,resolution,state)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'open')`, rID, c.Actor.OrgID, id, c.Actor.UserID, strings.TrimSpace(input.Title), input.Description, input.OwnerID, input.Severity, input.DueAt, input.Resolution)
		}
		if err != nil {
			return core.Result{}, err
		}
		r, err := getConcern(ctx, tx, c.Actor.OrgID, rID, kind)
		return concernResult(r, kind, kind+".created", true, ""), err
	})
}
func (s *Service) UpdateConcern(ctx context.Context, c core.Command, id, kind string, input ConcernPatch) (core.Result, error) {
	table, err := concernTable(kind)
	if err != nil {
		return core.Result{}, err
	}
	if err = core.RequireVersion(input.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if input.State != nil && strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("reason", "状态变更须填写原因")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		r, err := getConcern(ctx, tx, c.Actor.OrgID, id, kind)
		if err != nil {
			return err
		}
		p, err := getProject(ctx, tx, c.Actor.OrgID, r.ProjectID, false)
		if err != nil {
			return err
		}
		if err = authz.Require(ctx, tx, c.Actor, kind+".update", collaborationResource(p, kind, id, r.CreatorID)); err != nil {
			return err
		}
		if kind == "risk" && input.State != nil && *input.State == "realized" {
			return authorizeProject(ctx, tx, c.Actor, p.ID, "issue.create")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		p, err := collaborationProject(ctx, tx, c.Actor.OrgID, id, table, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(p); err != nil {
			return core.Result{}, err
		}
		r, err := getConcern(ctx, tx, c.Actor.OrgID, id, kind)
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		previous := r.State
		if previous == "realized" {
			return core.Result{}, core.Fail(409, "RISK_ALREADY_REALIZED", "该风险已转成问题，请在关联问题中跟进")
		}
		if previous == "closed" && (input.State == nil || *input.State != "open") {
			return core.Result{}, core.Fail(409, "CONCERN_READ_ONLY", "请先重新打开已关闭事项")
		}
		if input.Title != nil {
			r.Title = strings.TrimSpace(*input.Title)
		}
		if input.Description != nil {
			r.Description = *input.Description
		}
		if input.OwnerID != nil {
			r.OwnerID = *input.OwnerID
		}
		if input.Severity != nil {
			r.Severity = *input.Severity
		}
		if input.DueAt.Set {
			r.DueAt = input.DueAt.Value
		}
		if input.Probability.Set {
			r.Probability = input.Probability.Value
		}
		if input.Mitigation != nil {
			r.Mitigation = *input.Mitigation
		}
		if input.Resolution != nil {
			r.Resolution = *input.Resolution
		}
		if err = validateConcern(ConcernCreate{Title: r.Title, Description: r.Description, OwnerID: r.OwnerID, Severity: r.Severity, Probability: r.Probability, Mitigation: r.Mitigation, Resolution: r.Resolution}); err != nil {
			return core.Result{}, err
		}
		if err = validAssignee(ctx, tx, c.Actor.OrgID, p.ID, &r.OwnerID); err != nil {
			return core.Result{}, err
		}
		if input.State != nil {
			target := *input.State
			allowed := false
			if kind == "risk" {
				allowed = (previous == "open" && (target == "mitigating" || target == "closed" || target == "realized")) || (previous == "mitigating" && (target == "closed" || target == "realized")) || (previous == "closed" && target == "open")
			} else {
				allowed = (previous == "open" && (target == "investigating" || target == "resolved")) || (previous == "investigating" && target == "resolved") || (previous == "resolved" && (target == "closed" || target == "investigating")) || (previous == "closed" && target == "open")
			}
			if !allowed {
				return core.Result{}, core.UnexpectedState(previous, target)
			}
			if kind == "issue" && (target == "resolved" || target == "closed") && strings.TrimSpace(r.Resolution) == "" {
				return core.Result{}, core.Invalid("resolution", "解决或关闭问题须填写解决说明")
			}
			r.State = target
			if kind == "risk" && target == "realized" {
				issueID := core.NewID()
				_, err = tx.Exec(ctx, `INSERT INTO issues(id,org_id,project_id,creator_id,title,description,owner_id,severity,due_at,state)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'open')`, issueID, c.Actor.OrgID, p.ID, c.Actor.UserID, r.Title, r.Description, r.OwnerID, r.Severity, r.DueAt)
				if err != nil {
					return core.Result{}, err
				}
				r.IssueID = &issueID
			}
		}
		if kind == "risk" {
			_, err = tx.Exec(ctx, `UPDATE risks SET title=$3,description=$4,owner_id=$5,severity=$6,due_at=$7,probability=$8,mitigation=$9,state=$10,issue_id=$11,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`,
				c.Actor.OrgID, id, r.Title, r.Description, r.OwnerID, r.Severity, r.DueAt, r.Probability, r.Mitigation, r.State, r.IssueID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE issues SET title=$3,description=$4,owner_id=$5,severity=$6,due_at=$7,resolution=$8,state=$9,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`,
				c.Actor.OrgID, id, r.Title, r.Description, r.OwnerID, r.Severity, r.DueAt, r.Resolution, r.State)
		}
		if err != nil {
			return core.Result{}, err
		}
		r, err = getConcern(ctx, tx, c.Actor.OrgID, id, kind)
		result := concernResult(r, kind, kind+".updated", false, input.Reason)
		if kind == "risk" && r.IssueID != nil && previous != "realized" {
			result.Events = []core.Event{{Type: "issue.created_from_risk", ResourceType: "issue", ResourceID: *r.IssueID, Version: 1,
				Payload: map[string]any{"project_id": p.ID, "risk_id": id, "reason": input.Reason}}}
		}
		return result, err
	})
}
