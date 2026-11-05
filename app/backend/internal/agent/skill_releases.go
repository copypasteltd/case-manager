package agent

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type SkillRelease struct {
	ID                string    `json:"id"`
	ProfileID         string    `json:"profile_id"`
	Version           int64     `json:"version"`
	Stage             string    `json:"stage"`
	ProfileVersion    int64     `json:"profile_version"`
	SkillSHA256       string    `json:"skill_sha256"`
	ActiveBasisSHA256 string    `json:"active_basis_sha256"`
	AssessmentRunIDs  []string  `json:"assessment_run_ids"`
	RollbackReleaseID *string   `json:"rollback_release_id"`
	Reason            string    `json:"reason"`
	ActorID           string    `json:"actor_id"`
	CreatedAt         time.Time `json:"created_at"`
}

type SkillReleaseDetails struct {
	Profile       ProfileDetails `json:"profile"`
	Version       int64          `json:"version"`
	History       []SkillRelease `json:"history"`
	BeforeVersion *int64         `json:"before_version"`
}

type SkillReleaseInput struct {
	ExpectedVersion   int64    `json:"expected_version"`
	ProfileVersion    int64    `json:"profile_version"`
	SkillSHA256       string   `json:"skill_sha256"`
	ActiveBasisSHA256 string   `json:"active_basis_sha256"`
	Stage             string   `json:"stage"`
	AssessmentRunIDs  []string `json:"assessment_run_ids"`
	RollbackReleaseID *string  `json:"rollback_release_id"`
	Reviewed          bool     `json:"reviewed"`
	Reason            string   `json:"reason"`
}

func skillReleaseAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, write); err != nil {
		return err
	}
	if err := authz.RequireOrganization(ctx, tx, a, "agent.skill.read"); err != nil {
		return err
	}
	if write {
		return authz.RequireOrganization(ctx, tx, a, "agent.skill.manage")
	}
	return nil
}

func skillReleaseDetails(ctx context.Context, tx pgx.Tx, org, id string, before int64) (SkillReleaseDetails, error) {
	out := SkillReleaseDetails{History: []SkillRelease{}}
	var err error
	out.Profile, err = profileDetails(ctx, tx, org, id)
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM agent_skill_releases WHERE org_id=$1 AND profile_id=$2", org, id).Scan(&out.Version); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT id,profile_id,version,stage,profile_version,skill_sha256,active_basis_sha256,assessment_run_ids,rollback_release_id,reason,actor_id,created_at
 FROM agent_skill_releases WHERE org_id=$1 AND profile_id=$2 AND ($3::bigint=0 OR version<$3) ORDER BY version DESC LIMIT 21`, org, id, before)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SkillRelease
		if err = rows.Scan(&v.ID, &v.ProfileID, &v.Version, &v.Stage, &v.ProfileVersion, &v.SkillSHA256, &v.ActiveBasisSHA256, &v.AssessmentRunIDs, &v.RollbackReleaseID, &v.Reason, &v.ActorID, &v.CreatedAt); err != nil {
			return out, err
		}
		out.History = append(out.History, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.History) > 20 {
		out.History = out.History[:20]
		v := out.History[19].Version
		out.BeforeVersion = &v
	}
	return out, nil
}

func (s *Service) SkillReleases(ctx context.Context, a core.Actor, id string, before int64) (SkillReleaseDetails, error) {
	if !core.ValidID(id) || before < 0 {
		return SkillReleaseDetails{}, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return SkillReleaseDetails{}, err
	}
	defer tx.Rollback(ctx)
	if err = skillReleaseAuthority(ctx, tx, a, false); err != nil {
		return SkillReleaseDetails{}, err
	}
	return skillReleaseDetails(ctx, tx, a.OrgID, id, before)
}

func (s *Service) RecordSkillRelease(ctx context.Context, c core.Command, id string, in SkillReleaseInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if in.ExpectedVersion < 0 || core.RequireVersion(in.ProfileVersion) != nil || !slices.Contains([]string{"pilot", "published", "disabled"}, in.Stage) || !in.Reviewed || core.Name(in.Reason) != nil || len(in.AssessmentRunIDs) > 20 || in.Stage == "published" && len(in.AssessmentRunIDs) == 0 || len(in.SkillSHA256) != 64 || strings.Trim(in.SkillSHA256, "0123456789abcdef") != "" || len(in.ActiveBasisSHA256) != 64 || strings.Trim(in.ActiveBasisSHA256, "0123456789abcdef") != "" {
		return core.Result{}, core.Invalid("release", "请核对准确配置、任务影响、评估运行与发布决定")
	}
	seen := map[string]bool{}
	for _, v := range in.AssessmentRunIDs {
		if !core.ValidID(v) || seen[v] {
			return core.Result{}, core.Invalid("assessment_run_ids", "评估运行必须有效且不重复")
		}
		seen[v] = true
	}
	if in.RollbackReleaseID != nil && !core.ValidID(*in.RollbackReleaseID) {
		return core.Result{}, core.Invalid("rollback_release_id", "请选择准确已发布记录")
	}
	if in.AssessmentRunIDs == nil {
		in.AssessmentRunIDs = []string{}
	}
	c.ExclusiveOrganizationLock = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return skillReleaseAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		current, err := skillReleaseDetails(ctx, tx, c.Actor.OrgID, id, 0)
		if err != nil {
			return core.Result{}, err
		}
		p := current.Profile
		if current.Version != in.ExpectedVersion || p.Item.Version != in.ProfileVersion || p.Item.SkillSHA256 != in.SkillSHA256 || p.Item.State != "active" || p.Impact.ActiveBasisSHA256 != in.ActiveBasisSHA256 {
			return core.Result{}, core.Conflict()
		}
		for _, runID := range in.AssessmentRunIDs {
			var ok bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE org_id=$1 AND id=$2 AND profile_id=$3 AND actor_id=$4 AND state='succeeded' AND outcome='completed')`, c.Actor.OrgID, runID, id, c.Actor.UserID).Scan(&ok); err != nil {
				return core.Result{}, err
			}
			if !ok {
				return core.Result{}, core.Fail(409, "SKILL_ASSESSMENT_NOT_ELIGIBLE", "评估须为本人此准确配置已完成的成功运行")
			}
		}
		receiptID := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO agent_skill_releases(id,org_id,profile_id,version,stage,profile_version,skill_sha256,active_basis_sha256,assessment_run_ids,rollback_release_id,reviewed,reason,actor_id,session_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$11,$12,$13)`, receiptID, c.Actor.OrgID, id, current.Version+1, in.Stage, in.ProfileVersion, in.SkillSHA256, in.ActiveBasisSHA256, in.AssessmentRunIDs, in.RollbackReleaseID, strings.TrimSpace(in.Reason), c.Actor.UserID, c.Actor.SessionID)
		if err != nil {
			return core.Result{}, err
		}
		out, err := skillReleaseDetails(ctx, tx, c.Actor.OrgID, id, 0)
		return core.Result{Data: out, Event: core.Event{Type: "agent.skill_release_recorded", ResourceType: "agent_profile", ResourceID: id, Version: out.Version, Payload: map[string]any{"receipt_id": receiptID, "stage": in.Stage, "skill_sha256": in.SkillSHA256, "reviewed": true}}}, err
	})
}
