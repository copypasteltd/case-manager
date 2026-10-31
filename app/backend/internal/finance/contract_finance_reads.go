package finance

import (
	"context"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type contractFinanceMeta struct {
	ID, CreatorID, Classification, Lifecycle             string
	ProjectID, ActiveRevisionID, ScheduleMode, Direction *string
	Version                                              int64
}

func contractFinanceMetadata(ctx context.Context, db platform.DBTX, org, id string) (contractFinanceMeta, error) {
	c := contractFinanceMeta{ID: id}
	if !core.ValidID(id) {
		return c, core.Missing()
	}
	err := db.QueryRow(ctx, `SELECT c.creator_id,c.classification,c.lifecycle,c.project_id,c.active_revision_id,c.version,r.payment_schedule->>'mode',r.direction
        FROM contracts c LEFT JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id WHERE c.org_id=$1 AND c.id=$2 AND c.deleted_at IS NULL`, org, id).
		Scan(&c.CreatorID, &c.Classification, &c.Lifecycle, &c.ProjectID, &c.ActiveRevisionID, &c.Version, &c.ScheduleMode, &c.Direction)
	return c, platform.DatabaseError(err)
}
func contractFinanceResource(c contractFinanceMeta) authz.Resource {
	return authz.Resource{Type: "contract", ID: c.ID, ProjectID: c.ProjectID, OwnerID: &c.CreatorID, Classification: c.Classification}
}
func contractFinanceAxisReadable(ctx context.Context, db platform.DBTX, a core.Actor, c contractFinanceMeta, axis string) (bool, error) {
	if axis == "cost" && value(c.Direction) == "receivable" {
		return true, nil
	}
	res := contractFinanceResource(c)
	res.Type = axis
	if authz.Require(ctx, db, a, axis+".read", res) != nil {
		return false, nil
	}
	var visible bool
	if axis == "cost" {
		err := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM contract_finance_events e JOIN actual_cost_entries x ON x.org_id=e.org_id AND x.id=e.cost_id
            WHERE e.org_id=$1 AND e.contract_id=$2 AND NOT can_read_cost(x.org_id,$3,x.id,x.current_revision_id,$4,$5))`, a.OrgID, c.ID, a.UserID, a.Channel, a.RunID != "").Scan(&visible)
		return visible, err
	}
	err := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=$1 AND e.contract_id=$2 AND e.payment_id IS NOT NULL
        AND (NOT can_read_payment(e.org_id,$3,e.payment_id,$4,$5) OR EXISTS(SELECT 1 FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
            WHERE p.org_id=e.org_id AND r.refund_of_id=e.payment_id AND p.state IN ('recorded','reversed') AND NOT can_read_payment(p.org_id,$3,p.id,$4,$5))))`, a.OrgID, c.ID, a.UserID, a.Channel, a.RunID != "").Scan(&visible)
	return visible, err
}
func financialContractStage(stage string) bool {
	return stage == "effective" || stage == "completed" || stage == "terminated"
}
func contractFinanceWriteAllowed(ctx context.Context, db platform.DBTX, a core.Actor, c contractFinanceMeta) bool {
	return a.RunID == "" && financialContractStage(c.Lifecycle) && authz.Require(ctx, db, a, "contract.reconcile", contractFinanceResource(c)) == nil &&
		writable(ctx, db, a, Cost{ProjectID: c.ProjectID, Classification: c.Classification})
}
func (s *Service) contractFinanceSnapshotTx(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	var orgVersion, userVersion int64
	var status string
	err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if err == nil {
		err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, platform.DatabaseError(err)
	}
	if status != "active" || orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion {
		tx.Rollback(ctx)
		return nil, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	return tx, nil
}
func contractFinanceView(ctx context.Context, db platform.DBTX, a core.Actor, c contractFinanceMeta) (ContractFinance, error) {
	out := ContractFinance{ContractID: c.ID, ContractVersion: c.Version, ActiveRevisionID: c.ActiveRevisionID, ScheduleMode: c.ScheduleMode, Nodes: []ContractFinanceNode{}, HistoryVisibility: "restricted"}
	if err := authz.Require(ctx, db, a, "contract.read", contractFinanceResource(c)); err != nil {
		return out, err
	}
	if err := db.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM contract_finance_heads WHERE org_id=$1 AND contract_id=$2),0)", a.OrgID, c.ID).Scan(&out.Version); err != nil {
		return out, err
	}
	costOK, err := contractFinanceAxisReadable(ctx, db, a, c, "cost")
	if err != nil {
		return out, err
	}
	cashOK, err := contractFinanceAxisReadable(ctx, db, a, c, "payment")
	if err != nil {
		return out, err
	}
	if costOK && cashOK {
		out.HistoryVisibility = "complete"
	}
	out.CanReconcile = contractFinanceWriteAllowed(ctx, db, a, c)
	rows, err := db.Query(ctx, `SELECT m.id,m.position,m.title,m.direction,m.currency,m.amount::text,m.due_date::text,m.conditions,
        COALESCE((SELECT sum(contract_finance_effective(m.org_id,e.id)) FROM contract_finance_events e WHERE e.org_id=m.org_id AND e.milestone_id=m.id AND e.kind='cost'),0)::text,
        COALESCE((SELECT sum(contract_finance_effective(m.org_id,e.id)) FROM contract_finance_events e WHERE e.org_id=m.org_id AND e.milestone_id=m.id AND e.kind IN ('cash','refund')),0)::text,
        EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=m.org_id AND e.milestone_id=m.id AND e.kind='cash' AND contract_unattributed_refunds(m.org_id,e.payment_id)>0),
        md5(contract_finance_snapshot(m.org_id,m.id)::text),review.id,review.review_fingerprint,review.conditions_satisfied
        FROM contract_milestones m LEFT JOIN LATERAL(SELECT id,review_fingerprint,conditions_satisfied FROM contract_finance_events e WHERE e.org_id=m.org_id AND e.milestone_id=m.id AND e.kind='review' ORDER BY e.seq DESC LIMIT 1) review ON true
        WHERE m.org_id=$1 AND m.contract_id=$2 ORDER BY m.position`, a.OrgID, c.ID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		n := ContractFinanceNode{CostVisibility: "restricted", CashVisibility: "restricted", ReviewState: "restricted"}
		var cost, cash, fingerprint string
		var pending bool
		var reviewID, reviewFingerprint *string
		var conditions *bool
		if err = rows.Scan(&n.ID, &n.Position, &n.Title, &n.Direction, &n.Currency, &n.Amount, &n.DueDate, &n.Conditions, &cost, &cash, &pending, &fingerprint, &reviewID, &reviewFingerprint, &conditions); err != nil {
			return out, err
		}
		if amount, e := core.ParseDecimal(cost); e == nil {
			cost = amount.String()
		} else {
			return out, e
		}
		if amount, e := core.ParseDecimal(cash); e == nil {
			cash = amount.String()
		} else {
			return out, e
		}
		if n.Direction == "receivable" {
			n.CostVisibility = "not_applicable"
		} else if costOK {
			n.CostVisibility = "complete"
			n.CostAmount = &cost
		}
		if cashOK {
			n.CashVisibility = "complete"
			n.CashAmount = &cash
			n.HasUnattributedRefunds = &pending
			actual, _ := core.ParseDecimal(cash)
			planned, _ := core.ParseDecimal(n.Amount)
			zero := core.Decimal{}
			outstanding, excess := planned.Sub(actual), actual.Sub(planned)
			if outstanding.Cmp(zero) < 0 {
				outstanding = zero
			}
			if excess.Cmp(zero) < 0 {
				excess = zero
			}
			left, over := outstanding.String(), excess.String()
			n.OutstandingAmount = &left
			n.ExcessAmount = &over
		}
		full := cashOK && (costOK || n.Direction == "receivable")
		if full {
			n.Fingerprint = &fingerprint
			n.ReviewID = reviewID
			n.ConditionsSatisfied = conditions
			n.ReviewState = "unreviewed"
			if reviewID != nil {
				n.ReviewState = "stale"
				if reviewFingerprint != nil && *reviewFingerprint == fingerprint {
					n.ReviewState = "current"
				}
			}
		}
		n.CanReview = out.CanReconcile && full
		out.Nodes = append(out.Nodes, n)
	}
	return out, rows.Err()
}
func (s *Service) ContractFinance(ctx context.Context, a core.Actor, id string) (ContractFinance, error) {
	tx, err := s.contractFinanceSnapshotTx(ctx, a)
	if err != nil {
		return ContractFinance{}, err
	}
	defer tx.Rollback(ctx)
	c, err := contractFinanceMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return ContractFinance{}, err
	}
	out, err := contractFinanceView(ctx, tx, a, c)
	if err == nil {
		err = platform.DatabaseError(tx.Commit(ctx))
	}
	return out, err
}

const contractFinanceEventColumns = `e.id,e.contract_id,e.milestone_id,e.contract_revision_id,e.seq,e.kind,e.cost_id,e.payment_id,e.source_version,e.source_revision_id,e.source_content_hash,e.source_event_id,
    e.amount::text,contract_finance_remaining(e.org_id,e.id)::text,contract_finance_effective(e.org_id,e.id)::text,COALESCE(e.review_snapshot,'null'::jsonb),e.review_fingerprint,
    e.conditions_satisfied,e.acknowledge_difference,e.acknowledge_unattributed_refunds,e.actor_id,u.display_name,e.reason,e.created_at,e.refund_fx_acknowledged,
    (SELECT jsonb_build_object('currency',pr.currency,'amount',pr.amount::text,'paid_on',pr.paid_on::text,'refund_fx',pr.payload->'refund_fx')
     FROM payment_revisions pr WHERE pr.org_id=e.org_id AND pr.id=COALESCE(e.source_revision_id,
       (SELECT src.source_revision_id FROM contract_finance_events src WHERE src.org_id=e.org_id AND src.id=e.source_event_id AND src.kind='refund'))
     AND jsonb_typeof(pr.payload->'refund_fx')='object')`

func scanContractFinanceEvent(row pgx.Row) (ContractFinanceEvent, error) {
	e := ContractFinanceEvent{Attachments: []fileref.Evidence{}}
	err := row.Scan(&e.ID, &e.ContractID, &e.MilestoneID, &e.ContractRevisionID, &e.Seq, &e.Kind, &e.CostID, &e.PaymentID, &e.SourceVersion, &e.SourceRevisionID, &e.SourceContentHash, &e.SourceEventID, &e.Amount, &e.RemainingAmount, &e.EffectiveAmount,
		&e.ReviewSnapshot, &e.ReviewFingerprint, &e.ConditionsSatisfied, &e.AcknowledgeDifference, &e.AcknowledgeUnattributedRefunds, &e.ActorID, &e.ActorName, &e.Reason, &e.CreatedAt, &e.RefundFXAcknowledged, &e.RefundCash)
	return e, platform.DatabaseError(err)
}
func (s *Service) ContractFinanceEvents(ctx context.Context, a core.Actor, id, mid, cursor string, limit int) (core.Page[ContractFinanceEvent], error) {
	out := core.Page[ContractFinanceEvent]{Items: []ContractFinanceEvent{}}
	if mid != "" && !core.ValidID(mid) {
		return out, core.Invalid("milestone_id", "履约节点无效")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.Digest([]byte(fmt.Sprint(core.CursorBinding(a, "contract-finance-events"), id, mid)))
	var before *time.Time
	var after *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		before = &c.UpdatedAt
		after = &c.ID
	}
	tx, err := s.contractFinanceSnapshotTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := contractFinanceMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, tx, a, "contract.read", contractFinanceResource(c)); err != nil {
		return out, err
	}
	for _, axis := range []string{"cost", "payment"} {
		ok, e := contractFinanceAxisReadable(ctx, tx, a, c, axis)
		if e != nil {
			return out, e
		}
		if !ok {
			return out, core.Missing()
		}
	}
	rows, err := tx.Query(ctx, `SELECT `+contractFinanceEventColumns+` FROM contract_finance_events e JOIN users u ON u.org_id=e.org_id AND u.id=e.actor_id
        WHERE e.org_id=$1 AND e.contract_id=$2 AND ($3='' OR e.milestone_id::text=$3) AND ($4::timestamptz IS NULL OR (e.created_at,e.id)<($4,$5::uuid)) ORDER BY e.created_at DESC,e.id DESC LIMIT $6`, a.OrgID, id, mid, before, after, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		e, x := scanContractFinanceEvent(rows)
		if x != nil {
			rows.Close()
			return out, x
		}
		out.Items = append(out.Items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	more := len(out.Items) > limit
	if more {
		out.Items = out.Items[:limit]
	}
	for i := range out.Items {
		e := &out.Items[i]
		fileRows, x := tx.Query(ctx, "SELECT version_id FROM contract_finance_event_files WHERE org_id=$1 AND event_id=$2 ORDER BY version_id", a.OrgID, e.ID)
		if x != nil {
			return out, x
		}
		ids := []string{}
		for fileRows.Next() {
			var file string
			if x = fileRows.Scan(&file); x != nil {
				fileRows.Close()
				return out, x
			}
			ids = append(ids, file)
		}
		x = fileRows.Err()
		fileRows.Close()
		if x != nil {
			return out, x
		}
		for _, file := range ids {
			proof, x := fileref.Resolve(ctx, tx, a, file, c.ProjectID, c.Classification)
			if x != nil {
				return out, x
			}
			e.Attachments = append(e.Attachments, proof)
		}
		e.CanRelease = contractFinanceWriteAllowed(ctx, tx, a, c) && e.RemainingAmount != nil && *e.RemainingAmount != "0.000000" && *e.RemainingAmount != "0"
	}
	if more {
		last := out.Items[len(out.Items)-1]
		value := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &value
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
