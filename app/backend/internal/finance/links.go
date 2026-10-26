package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"slices"
)

type source struct {
	Title, Class, Owner, Currency string
	Project                       *string
	Amount                        *string
	Valid                         bool
}

func sourceInfo(ctx context.Context, db platform.DBTX, a core.Actor, kind, id, revision string, lock bool) (source, error) {
	var v source
	var query string
	if kind == "subscription_period" {
		p, err := subscriptionPeriodMetadata(ctx, db, a.OrgID, id)
		if err != nil {
			return v, err
		}
		s, err := subscriptionMetadata(ctx, db, a.OrgID, p.SubscriptionID)
		if err != nil {
			return v, err
		}
		if err = subscriptionReadable(ctx, db, a, s); err != nil {
			return v, err
		}
		var title, currency, amount string
		err = db.QueryRow(ctx, `SELECT cfg.product||' · '||p.start_date::text,r.currency,r.amount::text FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.period_id=p.id AND r.id=$3 JOIN subscription_revisions cfg ON cfg.org_id=r.org_id AND cfg.id=r.configuration_revision_id WHERE p.org_id=$1 AND p.id=$2`, a.OrgID, id, revision).Scan(&title, &currency, &amount)
		if err != nil {
			return v, platform.DatabaseError(err)
		}
		return source{Title: title, Class: s.Classification, Owner: s.CreatorID, Project: s.ProjectID, Currency: currency, Amount: &amount, Valid: p.CurrentRevisionID == revision && !slices.Contains([]string{"waived", "void"}, p.RecordedState)}, nil
	} else if kind == "contract" {
		query = `SELECT r.title,c.classification,c.creator_id,c.project_id,COALESCE(r.currency,''),r.amount::text,
        c.lifecycle IN ('effective','completed','terminated') AND c.approved_revision_id=r.id AND c.active_revision_id=r.id AND r.direction='payable' AND c.deleted_at IS NULL
        FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.contract_id=c.id AND r.id=$3 WHERE c.org_id=$1 AND c.id=$2`
	} else if kind == "purchase" || kind == "claim" {
		query = `SELECT r.title,c.classification,c.creator_id,c.project_id,COALESCE(r.currency,''),CASE WHEN c.kind='purchase' THEN (SELECT amount::text FROM purchase_effective_plan(c.org_id,c.id)) ELSE r.amount::text END,
        c.approval_state='approved' AND c.approved_revision_id=r.id AND c.deleted_at IS NULL
        FROM spend_documents c JOIN spend_revisions r ON r.org_id=c.org_id AND r.document_id=c.id AND r.id=$3 WHERE c.org_id=$1 AND c.id=$2 AND c.kind='` + kind + `'`
	} else {
		return v, core.Invalid("source_type", "当前可关联已批准采购、报销或生效应付合同")
	}
	if lock {
		query += " FOR UPDATE OF c"
	}
	if err := db.QueryRow(ctx, query, a.OrgID, id, revision).Scan(&v.Title, &v.Class, &v.Owner, &v.Project, &v.Currency, &v.Amount, &v.Valid); err != nil {
		return v, platform.DatabaseError(err)
	}
	return v, authz.Require(ctx, db, a, kind+".read", authz.Resource{Type: kind, ID: id, ProjectID: v.Project, OwnerID: &v.Owner, Classification: v.Class})
}
func links(ctx context.Context, db platform.DBTX, a core.Actor, c Cost) ([]Link, error) {
	out := []Link{}
	rows, err := db.Query(ctx, `SELECT l.id,l.source_type,l.source_id,l.revision_id,l.relation,l.reason,l.actor_id,l.created_at FROM cost_links l JOIN cost_events e ON e.org_id=l.org_id AND e.link_id=l.id AND e.cost_id=l.cost_id WHERE l.org_id=$1 AND l.cost_id=$2 AND e.cost_version<=$3 ORDER BY l.created_at,l.id LIMIT 100`, a.OrgID, c.ID, c.Version)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var l Link
		if err = rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourceRevisionID, &l.Relation, &l.Reason, &l.ActorID, &l.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out = append(out, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range out {
		l := &out[i]
		v, e := sourceInfo(ctx, db, a, l.SourceType, l.SourceID, l.SourceRevisionID, false)
		if e != nil {
			if !slices.Contains([]int{403, 404, 409, 422}, core.PublicError(e).Status) {
				return out, e
			}
			continue
		}
		l.Title = &v.Title
	}
	return out, nil
}
func (s *Service) Link(ctx context.Context, cmd core.Command, id string, in LinkInput) (core.Result, error) {
	relations := map[string]string{"purchase": "incurred_for", "claim": "reimbursed_via", "contract": "contractual_basis"}
	if relations[in.SourceType] == "" || relations[in.SourceType] != in.Relation || !core.ValidID(in.SourceID) || !core.ValidID(in.SourceRevisionID) {
		return core.Result{}, core.Invalid("source", "请指定准确业务修订及对应关联关系")
	}
	var src source
	pre := func(tx pgx.Tx, c Cost) error {
		v, err := sourceInfo(ctx, tx, cmd.Actor, in.SourceType, in.SourceID, in.SourceRevisionID, true)
		src = v
		return err
	}
	return s.mutate(ctx, cmd, id, "cost.record", in.Base, Content{}, pre, func(tx pgx.Tx, c Cost) (core.Result, error) {
		if c.State != "confirmed" || c.EntryKind != "expense" {
			return core.Result{}, core.Fail(409, "COST_LINK_REQUIRES_CONFIRMATION", "仅已确认且未冲销的原费用可新增业务关联")
		}
		if !src.Valid || src.Amount == nil || c.Currency == nil || src.Currency != *c.Currency {
			return core.Result{}, core.Fail(409, "COST_SOURCE_NOT_APPROVED", "请核对已批准采购/报销或生效应付合同的准确币种与金额")
		}
		var adequate bool
		if err := tx.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", src.Class, c.Classification).Scan(&adequate); err != nil {
			return core.Result{}, err
		}
		if !adequate || (src.Project != nil && !slices.Contains(projectIDs(c, c.Content), *src.Project)) {
			return core.Result{}, core.Invalid("source", "关联业务须在费用管理或分摊范围内，分类不能高于费用")
		}
		var used string
		var count int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(r.amount),0)::text FROM cost_links l JOIN actual_cost_entries d ON d.org_id=l.org_id AND d.id=l.cost_id JOIN cost_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id WHERE l.org_id=$1 AND l.source_type=$2 AND l.source_id=$3 AND d.state='confirmed' AND d.entry_kind='expense'`, cmd.Actor.OrgID, in.SourceType, in.SourceID).Scan(&used); err != nil {
			return core.Result{}, err
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM cost_links WHERE org_id=$1 AND cost_id=$2", cmd.Actor.OrgID, c.ID).Scan(&count); err != nil {
			return core.Result{}, err
		}
		amount, _ := core.ParseDecimal(*c.Amount)
		already, _ := core.ParseDecimal(used)
		cap, _ := core.ParseDecimal(*src.Amount)
		if already.Add(amount).Cmp(cap) > 0 {
			return core.Result{}, core.Fail(409, "COST_SOURCE_AMOUNT_EXCEEDED", "该业务关联的有效费用合计超过准确批准金额，请先核对差异或重复费用")
		}
		if count >= 100 {
			return core.Result{}, core.Invalid("links", "一笔费用最多关联100个业务对象")
		}
		link := core.NewID()
		if _, err := tx.Exec(ctx, `INSERT INTO cost_links(id,org_id,cost_id,source_type,source_id,revision_id,relation,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, link, cmd.Actor.OrgID, c.ID, in.SourceType, in.SourceID, in.SourceRevisionID, in.Relation, cmd.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		return s.finish(ctx, tx, cmd.Actor, c, "link_added", in.Reason, &link)
	})
}
