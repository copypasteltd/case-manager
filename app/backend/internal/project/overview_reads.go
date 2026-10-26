package project

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
)

type overviewQuery struct {
	kind, action, resource, selectSQL string
}

// These are fixed SQL fragments. No identifier or SQL is supplied by a client.
// Each predicate follows the corresponding module's ordinary read, rather than
// closeout's stricter all-or-nothing settlement authorization.
func overviewQueries() []overviewQuery {
	queries := []overviewQuery{}
	for _, pair := range [][2]string{{"tasks", "task"}, {"milestones", "milestone"}, {"risks", "risk"}, {"issues", "issue"}} {
		table, kind := pair[0], pair[1]
		queries = append(queries, overviewQuery{table, kind + ".read", kind,
			`SELECT t.id,t.version,NULL::uuid revision_id,t.title,t.state,t.updated_at FROM ` + table + ` t WHERE t.org_id=$1 AND t.project_id=$2
			AND can_access(t.org_id,$3,'` + kind + `.read','` + kind + `',t.id,t.project_id,t.creator_id,$7,$4,$5)`})
	}
	queries = append(queries,
		overviewQuery{"deliverables", "deliverable.read", "deliverable", `SELECT d.id,d.version,d.current_revision_id revision_id,d.name title,d.state,d.updated_at FROM deliverables d WHERE d.org_id=$1 AND d.project_id=$2
			AND can_access(d.org_id,$3,'deliverable.read','deliverable',d.id,d.project_id,d.creator_id,$7,$4,$5)`},
		overviewQuery{"changes", "change.read", "change", `SELECT d.id,d.version,d.current_revision_id revision_id,r.payload->>'title' title,d.state,d.updated_at FROM change_requests d
			JOIN project_change_revisions r ON r.org_id=d.org_id AND r.change_id=d.id AND r.id=d.current_revision_id WHERE d.org_id=$1 AND d.project_id=$2 AND d.protocol_version=1
			AND can_access(d.org_id,$3,'change.read','change',d.id,d.project_id,d.creator_id,d.classification,$4,$5)
			AND can_access($1,$3,'budget.read','project',$2,$2,$6,$7,$4,$5)`},
		overviewQuery{"budgets", "budget.read", "project", `SELECT b.id,b.revision version,NULL::uuid revision_id,b.currency title,b.state,b.created_at updated_at FROM project_budgets b
			JOIN change_requests d ON d.org_id=b.org_id AND d.id=b.change_id WHERE b.org_id=$1 AND b.project_id=$2 AND b.protocol_version=1
			AND can_access($1,$3,'budget.read','project',$2,$2,$6,$7,$4,$5)
			AND (b.state<>'draft' OR can_access(d.org_id,$3,'change.read','change',d.id,d.project_id,d.creator_id,d.classification,$4,$5))`},
		overviewQuery{"files", "file.read", "file", `SELECT f.id,f.version,f.current_version_id revision_id,f.name title,f.state,f.updated_at FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
			WHERE f.org_id=$1 AND d.project_id=$2 AND f.state='active' AND can_access(f.org_id,$3,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$4,$5)
			AND (WITH RECURSIVE chain AS(SELECT id,parent_id,state,ARRAY[id] path FROM folders WHERE org_id=f.org_id AND id=f.folder_id
				UNION ALL SELECT p.id,p.parent_id,p.state,c.path||p.id FROM folders p JOIN chain c ON p.id=c.parent_id WHERE p.org_id=f.org_id AND NOT p.id=ANY(c.path))
				SELECT COALESCE(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM chain)`},
		overviewQuery{"posts", "post.read", "post", `SELECT p.id,p.version,r.id revision_id,r.title,p.state,p.updated_at FROM posts p JOIN post_revisions r ON r.org_id=p.org_id AND r.post_id=p.id
			AND r.id=CASE WHEN p.author_id=$3 OR can_access(p.org_id,$3,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$4,$5)
			THEN p.current_revision_id ELSE COALESCE(p.published_revision_id,p.current_revision_id) END
			WHERE p.org_id=$1 AND p.project_id=$2 AND p.state<>'archived' AND can_access(p.org_id,$3,'post.read','post',p.id,p.project_id,p.author_id,p.classification,$4,$5)
			AND (p.state<>'draft' OR p.author_id=$3 OR can_access(p.org_id,$3,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$4,$5))`},
		overviewQuery{"contracts", "contract.read", "contract", `SELECT d.id,d.version,d.current_revision_id revision_id,r.title,d.lifecycle state,d.updated_at FROM contracts d
			JOIN contract_revisions r ON r.org_id=d.org_id AND r.contract_id=d.id AND r.id=d.current_revision_id WHERE d.org_id=$1 AND d.project_id=$2 AND d.deleted_at IS NULL
			AND can_access(d.org_id,$3,'contract.read','contract',d.id,d.project_id,d.creator_id,d.classification,$4,$5)`},
	)
	for _, pair := range [][2]string{{"purchases", "purchase"}, {"claims", "claim"}} {
		queries = append(queries, overviewQuery{pair[0], pair[1] + ".read", pair[1], `SELECT d.id,d.version,d.current_revision_id revision_id,r.title,d.lifecycle state,d.updated_at FROM spend_documents d
			JOIN spend_revisions r ON r.org_id=d.org_id AND r.document_id=d.id AND r.id=d.current_revision_id WHERE d.org_id=$1 AND d.project_id=$2 AND d.kind='` + pair[1] + `' AND d.deleted_at IS NULL
			AND can_access(d.org_id,$3,d.kind||'.read',d.kind,d.id,d.project_id,d.creator_id,d.classification,$4,$5)`})
	}
	return append(queries,
		overviewQuery{"costs", "cost.read", "cost", `SELECT c.id,c.version,r.id revision_id,r.title,c.state,c.updated_at FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.cost_id=c.id AND r.id=c.current_revision_id
			WHERE c.org_id=$1 AND can_read_cost(c.org_id,$3,c.id,r.id,$4,$5)
			AND EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id=$2)`},
		overviewQuery{"payments", "payment.read", "payment", `SELECT p.id,p.version,r.id revision_id,r.title,p.state,p.updated_at FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.payment_id=p.id AND r.id=p.current_revision_id
			WHERE p.org_id=$1 AND can_read_payment(p.org_id,$3,p.id,$4,$5)
			AND (EXISTS(SELECT 1 FROM payment_allocations a JOIN payment_allocation_shares sh ON sh.org_id=a.org_id AND sh.allocation_id=a.id WHERE a.org_id=p.org_id AND a.payment_id=p.id AND sh.project_id=$2)
			OR contract_cash_return_share(p.org_id,COALESCE(p.reverses_id,p.id),$2)>0)`},
		overviewQuery{"subscriptions", "subscription.read", "subscription", `SELECT s.id,s.version,r.id revision_id,r.product title,s.state,s.updated_at FROM subscriptions s
			JOIN subscription_revisions r ON r.org_id=s.org_id AND r.subscription_id=s.id AND r.id=s.current_revision_id WHERE s.org_id=$1 AND s.project_id=$2 AND can_read_subscription(s.org_id,$3,s.id,$4,$5)`},
		overviewQuery{"services", "service.read", "service", `SELECT s.id,s.version,NULL::uuid revision_id,s.name title,s.lifecycle state,s.updated_at FROM services s WHERE s.org_id=$1 AND s.project_id=$2
			AND can_access(s.org_id,$3,'service.read','service',s.id,s.project_id,s.creator_id,s.classification,$4,$5)`},
	)
}

func (s *Service) Overview(ctx context.Context, a core.Actor, pid string) (Overview, error) {
	out := Overview{Scope: "readable_records", Groups: []OverviewGroup{}, Budgets: []OverviewBudgetTotal{}}
	tx, err := s.deliverableSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	p, err := getProject(ctx, tx, a.OrgID, pid, false)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, tx, a, "project.read", resource(p)); err != nil {
		return out, err
	}
	out.ProjectID, out.ProjectVersion = p.ID, p.Version
	args := []any{a.OrgID, p.ID, a.UserID, a.Channel, a.RunID != "", p.CreatorID, p.Classification}
	for _, q := range overviewQueries() {
		g := OverviewGroup{Kind: q.kind, States: []OverviewState{}, Items: []OverviewItem{}}
		var raw []byte
		err = tx.QueryRow(ctx, `WITH visible AS MATERIALIZED (`+q.selectSQL+`), summary AS(SELECT count(*) n FROM visible)
			SELECT can_access($1,$3,'`+q.action+`','`+q.resource+`',$2,$2,$6,$7,$4,$5) OR n>0,
			jsonb_build_object('count',n,'states',COALESCE((SELECT jsonb_agg(jsonb_build_object('state',state,'count',n) ORDER BY state) FROM(SELECT state,count(*) n FROM visible GROUP BY state) s),'[]'::jsonb),
			'items',COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY updated_at DESC,id DESC) FROM(SELECT * FROM visible ORDER BY updated_at DESC,id DESC LIMIT 5) v),'[]'::jsonb)) FROM summary`, args...).Scan(&g.Available, &raw)
		if err != nil {
			return out, err
		}
		if g.Available {
			if err = json.Unmarshal(raw, &g); err != nil {
				return out, err
			}
			g.HasMore = *g.Count > int64(len(g.Items))
		}
		out.Groups = append(out.Groups, g)
	}
	out.Incurred, err = finance.ProjectIncurredInTx(ctx, tx, a, pid)
	if err != nil {
		return out, err
	}
	// Approved budgets are independent baselines. No expense/budget subtraction:
	// periods and readable scopes need not match, even when currencies do.
	rows, err := tx.Query(ctx, `SELECT b.id,b.revision,b.currency,COALESCE((SELECT sum(l.amount) FROM budget_lines l WHERE l.org_id=b.org_id AND l.budget_id=b.id),0)::text
		FROM project_budgets b JOIN change_requests d ON d.org_id=b.org_id AND d.id=b.change_id WHERE b.org_id=$1 AND b.project_id=$2 AND b.protocol_version=1 AND b.state='approved'
		AND can_access($1,$3,'budget.read','project',$2,$2,$6,$7,$4,$5) ORDER BY b.currency,b.id`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var b OverviewBudgetTotal
		if err = rows.Scan(&b.BudgetID, &b.Revision, &b.Currency, &b.Amount); err != nil {
			rows.Close()
			return out, err
		}
		out.Budgets = append(out.Budgets, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
