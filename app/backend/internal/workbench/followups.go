package workbench

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
	"github.com/jackc/pgx/v5"
)

type FollowupItem struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	RevisionID *string   `json:"revision_id"`
	ProjectID  *string   `json:"project_id"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FollowupGroup struct {
	Kind    string         `json:"kind"`
	Count   int64          `json:"count"`
	Items   []FollowupItem `json:"items"`
	HasMore bool           `json:"has_more"`
}

type FollowupSummary struct {
	AsOf   time.Time       `json:"as_of"`
	Scope  string          `json:"scope"`
	Groups []FollowupGroup `json:"groups"`
}

// These are attention entries, not a claim that submission/acceptance can
// succeed. Ordinary detail commands still check exact revisions, files, MFA,
// milestone state and independent human acceptance at the time of the action.
const deliverableFollowupBase = ` FROM deliverables d JOIN projects p ON p.org_id=d.org_id AND p.id=d.project_id
 LEFT JOIN deliverable_revisions r ON r.org_id=d.org_id AND r.deliverable_id=d.id AND r.id=d.current_revision_id AND r.protocol_version=1
 WHERE d.org_id=$1 AND d.protocol_version=1 AND p.state NOT IN ('archived','cancelled','closing')
 AND (d.milestone_id IS NULL OR EXISTS(SELECT 1 FROM milestones m WHERE m.org_id=d.org_id AND m.id=d.milestone_id
 AND m.project_id=d.project_id AND m.state NOT IN ('achieved','cancelled')))
 AND can_access(p.org_id,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,$4)
 AND can_access(d.org_id,$2,'deliverable.read','deliverable',d.id,p.id,d.creator_id,p.classification,$3,$4)`

func followupQueries() []query {
	return []query{
		{"requests_to_revise", workflow.PersonalFollowupsSQL},
		{"deliverables_to_submit", `SELECT d.id,d.version,d.current_revision_id revision_id,d.project_id,d.name title,d.state,d.updated_at` + deliverableFollowupBase + `
 AND d.owner_id=$2 AND d.state IN ('draft','changes_requested')
 AND can_access(d.org_id,$2,'deliverable.submit','deliverable',d.id,p.id,d.creator_id,p.classification,$3,$4)`},
		{"deliverables_to_review", `SELECT d.id,d.version,r.id revision_id,d.project_id,d.name title,d.state,d.updated_at` + deliverableFollowupBase + `
 AND d.state='submitted' AND r.submitted_by<>$2
 AND can_access(d.org_id,$2,'deliverable.decide','deliverable',d.id,p.id,d.creator_id,p.classification,$3,$4)`},
	}
}

func (s *Service) Followups(ctx context.Context, a core.Actor) (FollowupSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := FollowupSummary{Scope: "my_readable_followups", Groups: []FollowupGroup{}}
	if a.RunID != "" || (a.Channel != "web" && a.Channel != "wechat" && a.Channel != "douyin" && a.Channel != "android") {
		return out, core.Forbidden()
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var orgVersion, userVersion int64
	var status string
	err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if err == nil {
		err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion || status != "active" {
		return out, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变化，请重新获取授权")
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	for _, q := range followupQueries() {
		g := FollowupGroup{Kind: q.kind, Items: []FollowupItem{}}
		var raw []byte
		err = tx.QueryRow(ctx, `WITH visible AS MATERIALIZED (`+q.sql+`)
		 SELECT count(*),COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY updated_at DESC,id DESC)
		 FROM (SELECT * FROM visible ORDER BY updated_at DESC,id DESC LIMIT 5) v),'[]'::jsonb) FROM visible`,
			a.OrgID, a.UserID, a.Channel, false).Scan(&g.Count, &raw)
		if err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &g.Items); err != nil {
			return out, err
		}
		g.HasMore = g.Count > int64(len(g.Items))
		out.Groups = append(out.Groups, g)
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return FollowupSummary{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return FollowupSummary{}, platform.DatabaseError(err)
	}
	return out, nil
}
