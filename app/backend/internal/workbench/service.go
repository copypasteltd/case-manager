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
		// Compute from the original current configuration and signed observation.
		// Inbox read markers, delivery queues and projection heads cannot resolve work.
		{"resource_attention", `SELECT s.id,s.version,r.id revision_id,c.observation_id related_id,s.name title,
		 CASE c.state WHEN 'critical' THEN 'resource_critical' WHEN 'degraded' THEN 'resource_warning' ELSE 'resource_unconfirmed' END state,
		 NULL::text due_on,GREATEST(s.updated_at,r.created_at,c.received_at,
		 CASE WHEN c.state_reason='NO_OBSERVATION' THEN c.ready_after
		 WHEN c.state_reason='OBSERVATION_STALE' THEN LEAST(c.observed_at,c.received_at)+make_interval(secs=>(r.config->>'stale_after_seconds')::int) END) updated_at
		 FROM services s JOIN service_resource_sources h ON h.org_id=s.org_id AND h.service_id=s.id
		 JOIN service_resource_revisions r ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id
		 CROSS JOIN LATERAL service_resource_current(h.org_id,h.id,transaction_timestamp()) c
		 WHERE s.org_id=$1 AND s.owner_id=$2 AND s.lifecycle='active' AND r.enabled
		 AND resource_needs_attention(c.state,c.state_reason,c.ready_after,transaction_timestamp())
		 AND can_access(s.org_id,$2,'service.read','service',s.id,s.project_id,s.creator_id,s.classification,$3,$4)`},
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
		// Match the current human owner and every affected service. The actual
		// immutable timeline entry is the displayed revision; no guessed UUID.
		{"incidents", `SELECT i.id,i.version,t.id revision_id,NULL::uuid related_id,i.title,
		 i.state,NULL::text due_on,i.updated_at FROM incidents i
		 JOIN incident_timeline t ON t.org_id=i.org_id AND t.incident_id=i.id AND t.seq=i.version
		 WHERE i.org_id=$1 AND i.owner_id=$2 AND i.state IN ('open','investigating','monitoring')
		 AND cardinality(i.service_ids) BETWEEN 1 AND 20
		 AND NOT EXISTS(SELECT 1 FROM unnest(i.service_ids) affected(id)
		 LEFT JOIN services s ON s.org_id=i.org_id AND s.id=affected.id
		 WHERE s.id IS NULL OR NOT can_access(s.org_id,$2,'service.read','service',s.id,s.project_id,s.creator_id,s.classification,$3,$4))`},
		// Files have a creator, not a reassigned incident-style owner. Match the
		// ordinary file list's ACL and complete active ancestor chain. Only the
		// current head contributes; old failures and provider coordinates stay out.
		{"file_attention", `SELECT f.id,f.version,v.id revision_id,NULL::uuid related_id,f.name title,
		 CASE WHEN v.availability<>'available' OR v.scan_state='quarantined' THEN 'file_unavailable'
		 WHEN r.attention OR r.providers=1 THEN 'replica_attention' ELSE 'replica_pending' END state,
		 NULL::text due_on,GREATEST(f.updated_at,r.updated_at) updated_at
		 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
		 JOIN file_versions v ON v.org_id=f.org_id AND v.file_id=f.id AND v.id=f.current_version_id
		 LEFT JOIN LATERAL (SELECT max(p.updated_at) updated_at,
		 bool_or(p.state IN ('failed','missing','retry_wait')
		 OR (p.state='verified' AND (p.expected_sha256<>v.sha256 OR p.actual_sha256 IS DISTINCT FROM v.sha256 OR p.verified_at IS NULL))
		 OR EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=p.org_id AND j.id=p.active_job_id AND j.state IN ('dead','cancelled'))) attention,
		 bool_or(p.state<>'verified') pending,count(DISTINCT p.provider) providers
		 FROM file_replicas p WHERE p.org_id=f.org_id AND p.version_id=v.id) r ON true
		 WHERE f.org_id=$1 AND f.creator_id=$2 AND f.state='active'
		 AND can_access(f.org_id,$2,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$3,$4)
		 AND (WITH RECURSIVE chain AS (
		 SELECT id,parent_id,state,ARRAY[id] path FROM folders WHERE org_id=f.org_id AND id=f.folder_id
		 UNION ALL SELECT p.id,p.parent_id,p.state,c.path||p.id FROM folders p JOIN chain c ON p.id=c.parent_id
		 WHERE p.org_id=f.org_id AND NOT p.id=ANY(c.path)
		 ) SELECT coalesce(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM chain)
		 AND (v.availability<>'available' OR v.scan_state='quarantined' OR r.attention OR r.pending OR r.providers=1)`},
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
