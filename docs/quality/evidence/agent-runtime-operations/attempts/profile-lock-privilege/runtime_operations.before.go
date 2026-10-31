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

type ProfileOperation struct {
	RuntimeProfile
	Version                     int64     `json:"version"`
	State                       string    `json:"state"`
	TelegramConfigurationSHA256 *string   `json:"telegram_configuration_sha256"`
	InstalledBy                 string    `json:"installed_by"`
	CreatedAt                   time.Time `json:"created_at"`
}
type ProfileEvent struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	State     string    `json:"state"`
	Source    string    `json:"source"`
	ActorID   *string   `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type ProfileImpact struct {
	Queued            int    `json:"queued"`
	Running           int    `json:"running"`
	AwaitingApproval  int    `json:"awaiting_approval"`
	Committing        int    `json:"committing"`
	ActiveBasisSHA256 string `json:"active_basis_sha256"`
}
type ProfileDetails struct {
	Item    ProfileOperation `json:"item"`
	Impact  ProfileImpact    `json:"impact"`
	History []ProfileEvent   `json:"history"`
}
type ProfileRetire struct {
	ExpectedVersion   int64  `json:"expected_version"`
	ActiveBasisSHA256 string `json:"active_basis_sha256"`
	ReviewedImpact    bool   `json:"reviewed_impact"`
	Reason            string `json:"reason"`
}

func profileAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, write); err != nil {
		return err
	}
	if err := authz.RequireOrganization(ctx, tx, a, "agent.runtime.read"); err != nil {
		return err
	}
	if write {
		return authz.RequireOrganization(ctx, tx, a, "agent.runtime.retire")
	}
	return nil
}

const profileOperationSelect = `SELECT p.id,p.skill_id,p.skill_version,p.skill_sha256,p.runtime_version,p.configuration_sha256,
 p.max_input_tokens,p.max_output_tokens,p.max_tool_calls,p.timeout_seconds,e.version,p.state,p.telegram_configuration_sha256,p.installed_by,p.created_at
 FROM agent_runtime_profiles p JOIN LATERAL(SELECT version FROM agent_profile_events WHERE org_id=p.org_id AND profile_id=p.id ORDER BY version DESC LIMIT 1)e ON true`

func scanProfileOperation(row pgx.Row) (ProfileOperation, error) {
	var v ProfileOperation
	err := row.Scan(&v.ID, &v.SkillID, &v.SkillVersion, &v.SkillSHA256, &v.RuntimeVersion, &v.ConfigurationSHA256, &v.MaxInputTokens, &v.MaxOutputTokens, &v.MaxToolCalls, &v.TimeoutSeconds, &v.Version, &v.State, &v.TelegramConfigurationSHA256, &v.InstalledBy, &v.CreatedAt)
	return v, platform.DatabaseError(err)
}
func (s *Service) ProfileOperations(ctx context.Context, a core.Actor, state, skill, cursor string, limit int) (core.Page[ProfileOperation], error) {
	out := core.Page[ProfileOperation]{Items: []ProfileOperation{}}
	if (state != "" && !slices.Contains([]string{"active", "retired"}, state)) || (skill != "" && !slices.Contains([]string{"project-create", "project-update", "file-update", "information-organize", "oa-assist", "subscription-review", "project-briefing", "incident-summary"}, skill)) || limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("filters", "请核对配置状态、任务与分页条件")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = profileAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "agent-runtime", state, skill)
	var before *time.Time
	var id *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, id = &v.UpdatedAt, &v.ID
	}
	rows, err := tx.Query(ctx, profileOperationSelect+` WHERE p.org_id=$1 AND ($2='' OR p.state=$2) AND ($3='' OR p.skill_id=$3)
 AND ($4::timestamptz IS NULL OR (p.created_at,p.id)<($4,$5::uuid)) ORDER BY p.created_at DESC,p.id DESC LIMIT $6`, a.OrgID, state, skill, before, id, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanProfileOperation(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}
func profileDetails(ctx context.Context, tx pgx.Tx, org, id string) (ProfileDetails, error) {
	out := ProfileDetails{History: []ProfileEvent{}}
	var err error
	out.Item, err = scanProfileOperation(tx.QueryRow(ctx, profileOperationSelect+` WHERE p.org_id=$1 AND p.id=$2`, org, id))
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='queued'),count(*) FILTER(WHERE state='running'),count(*) FILTER(WHERE state='awaiting_approval'),count(*) FILTER(WHERE state='committing'),agent_profile_active_basis($1,$2)
 FROM agent_runs WHERE org_id=$1 AND profile_id=$2`, org, id).Scan(&out.Impact.Queued, &out.Impact.Running, &out.Impact.AwaitingApproval, &out.Impact.Committing, &out.Impact.ActiveBasisSHA256)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.version,e.state,e.source,r.actor_id,e.created_at FROM agent_profile_events e LEFT JOIN agent_profile_retirements r ON r.org_id=e.org_id AND r.profile_id=e.profile_id AND r.event_version=e.version
 WHERE e.org_id=$1 AND e.profile_id=$2 ORDER BY e.version DESC`, org, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e ProfileEvent
		if err = rows.Scan(&e.ID, &e.Version, &e.State, &e.Source, &e.ActorID, &e.CreatedAt); err != nil {
			return out, err
		}
		out.History = append(out.History, e)
	}
	return out, rows.Err()
}
func (s *Service) ProfileOperation(ctx context.Context, a core.Actor, id string) (ProfileDetails, error) {
	if !core.ValidID(id) {
		return ProfileDetails{}, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return ProfileDetails{}, err
	}
	defer tx.Rollback(ctx)
	if err = profileAuthority(ctx, tx, a, false); err != nil {
		return ProfileDetails{}, err
	}
	// Org share lock from BeginInteractiveRead stabilizes ordinary run writers;
	// profile lock also serializes a deployment-owned retirement.
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2 FOR SHARE`, a.OrgID, id).Scan(&found); err != nil {
		return ProfileDetails{}, platform.DatabaseError(err)
	}
	return profileDetails(ctx, tx, a.OrgID, id)
}
func (s *Service) RetireProfile(ctx context.Context, c core.Command, id string, in ProfileRetire) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || !in.ReviewedImpact || core.Name(in.Reason) != nil || len(in.ActiveBasisSHA256) != 64 || strings.Trim(in.ActiveBasisSHA256, "0123456789abcdef") != "" {
		return core.Result{}, core.Invalid("retirement", "请核对准确版本、当前任务影响和停用原因")
	}
	c.ExclusiveOrganizationLock = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return profileAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		var found string
		if err := tx.QueryRow(ctx, `SELECT id FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2 FOR UPDATE`, c.Actor.OrgID, id).Scan(&found); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		v, err := profileDetails(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if v.Item.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if v.Item.State != "active" {
			return core.Result{}, core.Fail(409, "PROFILE_ALREADY_RETIRED", "该运行配置已停用，请读取实际记录")
		}
		if v.Impact.ActiveBasisSHA256 != in.ActiveBasisSHA256 {
			return core.Result{}, core.Fail(409, "PROFILE_IMPACT_CHANGED", "活动任务已变化，请重新核对影响")
		}
		reason := strings.TrimSpace(in.Reason)
		_, err = tx.Exec(ctx, `INSERT INTO agent_profile_retirements(id,org_id,profile_id,expected_version,event_version,active_basis_sha256,reviewed_impact,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,true,$7,$8,$9)`, core.NewID(), c.Actor.OrgID, id, v.Item.Version, v.Item.Version+1, in.ActiveBasisSHA256, c.Actor.UserID, c.Actor.SessionID, reason)
		if err != nil {
			return core.Result{}, err
		}
		out, err := profileDetails(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: out, Event: core.Event{Type: "agent.profile_retired", ResourceType: "agent_profile", ResourceID: id, Version: out.Item.Version, Payload: map[string]any{"reason": reason, "reviewed_impact": true, "active_basis_sha256": in.ActiveBasisSHA256, "skill_id": out.Item.SkillID}}}, nil
	})
}
