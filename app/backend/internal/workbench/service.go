// Package workbench reads the current human's work across business modules.
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
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	RevisionID string    `json:"revision_id"`
	RelatedID  *string   `json:"related_id"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	DueOn      *string   `json:"due_on"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Group struct {
	Kind    string `json:"kind"`
	Count   int64  `json:"count"`
	Items   []Item `json:"items"`
	HasMore bool   `json:"has_more"`
}

type Summary struct {
	AsOf             time.Time `json:"as_of"`
	Scope            string    `json:"scope"`
	BusinessDate     string    `json:"business_date"`
	ContractTimezone string    `json:"contract_timezone"`
	ContractThrough  string    `json:"contract_through"`
	Groups           []Group   `json:"groups"`
}

type Service struct{ DB *pgxpool.Pool }

type query struct{ kind, sql string }

func queries() []query {
	return []query{
		{"approvals", `SELECT r.id,r.version,r.current_revision_id revision_id,q.id related_id,r.title,'active'::text state,NULL::text due_on,q.updated_at
		 FROM (` + workflow.PendingApprovalsSQL + `) q JOIN requests r ON r.org_id=$1 AND r.id=q.request_id`},
		// An effective contract's active revision can differ from its draft head.
		// The live contractual obligation, owner and date all come from that same
		// immutable active revision, never a newly edited draft or due-event cache.
		{"contracts", `SELECT c.id,c.version,r.id revision_id,NULL::uuid related_id,r.title,
		 CASE WHEN r.end_date < (transaction_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date THEN 'overdue' ELSE 'due_soon' END state,
		 r.end_date::text due_on,c.updated_at FROM contracts c
		 JOIN contract_revisions r ON r.org_id=c.org_id AND r.contract_id=c.id AND r.id=c.active_revision_id
		 WHERE c.org_id=$1 AND c.deleted_at IS NULL AND c.lifecycle='effective' AND COALESCE(r.owner_id,c.creator_id)=$2
		 AND r.end_date <= (transaction_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date+30
		 AND can_access(c.org_id,$2,'contract.read','contract',c.id,c.project_id,c.creator_id,c.classification,$3,$4)`},
		// Match the ordinary billing-period list's projected review state. A due
		// forecast or changed confirmed evidence needs review; it is not a payment
		// default. No charge generation, amounts, settlement or journal mutation.
		{"subscription_periods", `SELECT p.id,p.version,r.id revision_id,s.id related_id,cfg.product title,
		 'awaiting_confirmation'::text state,p.start_date::text due_on,p.updated_at
		 FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.period_id=p.id AND r.id=p.current_revision_id
		 JOIN subscriptions s ON s.org_id=p.org_id AND s.id=p.subscription_id
		 JOIN subscription_revisions cfg ON cfg.org_id=s.org_id AND cfg.subscription_id=s.id AND cfg.id=s.current_revision_id
		 WHERE p.org_id=$1 AND s.state<>'archived' AND COALESCE(r.owner_id,s.creator_id)=$2
		 AND can_read_subscription(p.org_id,$2,s.id,$3,$4)
		 AND (p.state='awaiting_confirmation'
		 OR (p.state='forecast' AND (p.start_date<=(transaction_timestamp() AT TIME ZONE r.timezone)::date
		 OR EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id)))
		 OR (p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(p.org_id,p.id)))`},
	}
}

func (s *Service) Read(ctx context.Context, a core.Actor) (Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := Summary{Scope: "my_readable_work", ContractTimezone: "Asia/Hong_Kong", Groups: []Group{}}
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
	err = tx.QueryRow(ctx, `SELECT transaction_timestamp(),(transaction_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text,
	 ((transaction_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date+30)::text`).Scan(&out.AsOf, &out.BusinessDate, &out.ContractThrough)
	if err != nil {
		return out, err
	}
	for _, q := range queries() {
		g := Group{Kind: q.kind, Items: []Item{}}
		var raw []byte
		err = tx.QueryRow(ctx, `WITH visible AS MATERIALIZED (`+q.sql+`)
		 SELECT count(*),COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY due_on ASC NULLS LAST,updated_at DESC,id DESC)
		 FROM (SELECT * FROM visible ORDER BY due_on ASC NULLS LAST,updated_at DESC,id DESC LIMIT 5) v),'[]'::jsonb) FROM visible`,
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
	// Recheck clock-based expiry before returning the complete snapshot. Row
	// locks above hold ordinary revocation/role changes until this read finishes.
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return Summary{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Summary{}, platform.DatabaseError(err)
	}
	return out, nil
}
