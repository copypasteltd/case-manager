package finance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type CostPaymentSource struct {
	ID          string `json:"id"`
	Version     int64  `json:"version"`
	RevisionID  string `json:"revision_id"`
	ContentHash string `json:"content_hash"`
}

type CostPaymentClaim struct {
	CostPaymentSource
	LineID string `json:"line_id"`
}

type CostPaymentBasis struct {
	Cost Cost      `json:"cost"`
	AsOf time.Time `json:"as_of"`
}

// The actual cash amount and the amount assigned to this expense are distinct.
// An unassigned remainder stays on the ordinary payment, never on a new cost.
type CostPaymentRecord struct {
	PaymentContent
	Cost               CostPaymentSource `json:"cost"`
	Claim              *CostPaymentClaim `json:"claim"`
	AllocationAmount   string            `json:"allocation_amount"`
	FX                 *PaymentFXInput   `json:"fx,omitempty"`
	SourceKey          string            `json:"source_key"`
	Reason             string            `json:"reason"`
	ActualPaid         bool              `json:"actual_paid"`
	SettlementReviewed bool              `json:"settlement_reviewed"`
}

type CostPaymentReceipt struct {
	Payment    Payment           `json:"payment"`
	Allocation PaymentAllocation `json:"allocation"`
}

func validCostPaymentSource(v CostPaymentSource) bool {
	return core.ValidID(v.ID) && core.RequireVersion(v.Version) == nil && core.ValidID(v.RevisionID) && len(v.ContentHash) == 64 && strings.Trim(v.ContentHash, "0123456789abcdef") == ""
}

func costPaymentProspect(a core.Actor, c Cost) Payment {
	return Payment{ID: core.NewID(), CreatorID: a.UserID, ProjectID: c.ProjectID, Classification: c.Classification}
}

func costPaymentAuthority(ctx context.Context, db platform.DBTX, a core.Actor, c Cost, mfa bool) error {
	if e := identity.RequireInteractiveSession(ctx, db, a, mfa); e != nil {
		return e
	}
	if e := readable(ctx, db, a, c, c.CurrentRevisionID); e != nil {
		return e
	}
	for _, action := range []string{"payment.read", "payment.record"} {
		if e := authz.Require(ctx, db, a, action, paymentResource(costPaymentProspect(a, c))); e != nil {
			return e
		}
	}
	return nil
}

func costPaymentClaim(ctx context.Context, db platform.DBTX, a core.Actor, in CostPaymentClaim, lock, exact bool) error {
	query := `SELECT d.version,d.approval_state,d.creator_id,d.project_id,d.classification,d.approved_revision_id,COALESCE(r.content_hash,''),d.deleted_at IS NULL FROM spend_documents d LEFT JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.approved_revision_id WHERE d.org_id=$1 AND d.id=$2 AND d.kind='claim'`
	if lock {
		query += " FOR UPDATE OF d"
	}
	var version int64
	var state, creator, class, digest string
	var project, revision *string
	var active bool
	if e := db.QueryRow(ctx, query, a.OrgID, in.ID).Scan(&version, &state, &creator, &project, &class, &revision, &digest, &active); e != nil {
		return platform.DatabaseError(e)
	}
	if e := authz.Require(ctx, db, a, "claim.read", authz.Resource{Type: "claim", ID: in.ID, OwnerID: &creator, ProjectID: project, Classification: class}); e != nil {
		return e
	}
	if exact && (!active || version != in.Version || state != "approved" || value(revision) != in.RevisionID || digest != in.ContentHash) {
		return core.Fail(409, "PAYMENT_CLAIM_CHANGED", "报销批准依据已变更，请重新核对收款员工及垫付明细")
	}
	return nil
}

func (s *Service) CostPaymentBasis(ctx context.Context, a core.Actor, id string) (CostPaymentBasis, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return CostPaymentBasis{}, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var orgVersion, userVersion int64
	var status string
	if e = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion); e != nil {
		return CostPaymentBasis{}, platform.DatabaseError(e)
	}
	if e = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status); e != nil {
		return CostPaymentBasis{}, platform.DatabaseError(e)
	}
	if orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion || status != "active" {
		return CostPaymentBasis{}, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新查阅")
	}
	c, e := metadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return CostPaymentBasis{}, e
	}
	if e = costPaymentAuthority(ctx, tx, a, c, false); e != nil {
		return CostPaymentBasis{}, e
	}
	if c.State != "confirmed" || c.EntryKind != "expense" || !writable(ctx, tx, a, c) {
		return CostPaymentBasis{}, core.Fail(409, "COST_NOT_SETTLEABLE", "请选择可写项目内已确认且未冲销的原费用")
	}
	c, e = present(ctx, tx, a, c)
	if e != nil {
		return CostPaymentBasis{}, e
	}
	basis := CostPaymentBasis{Cost: c}
	if e = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&basis.AsOf); e != nil {
		return CostPaymentBasis{}, e
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return CostPaymentBasis{}, e
	}
	return basis, platform.DatabaseError(tx.Commit(ctx))
}

func (s *Service) RecordCostPayment(ctx context.Context, cmd core.Command, in CostPaymentRecord) (out core.Result, err error) {
	defer func() {
		if err != nil || !out.Replayed {
			return
		}
		var receipt CostPaymentReceipt
		raw, e := json.Marshal(out.Data)
		if e == nil {
			e = json.Unmarshal(raw, &receipt)
		}
		if e != nil {
			out, err = core.Result{}, e
			return
		}
		projected, e := s.paymentReplay(ctx, cmd.Actor, core.Result{Data: receipt.Payment, Replayed: true}, nil)
		if e != nil {
			out, err = core.Result{}, e
			return
		}
		receipt.Payment = projected.Data.(Payment)
		out.Data = receipt
	}()
	if !validCostPaymentSource(in.Cost) || in.Claim != nil && (!validCostPaymentSource(in.Claim.CostPaymentSource) || !core.ValidID(in.Claim.LineID)) {
		return out, core.Invalid("cost", "请固定准确费用及可选批准报销的版本、修订与摘要")
	}
	if !in.ActualPaid || !in.SettlementReviewed || value(in.Direction) != "outgoing" || in.RefundOfID != nil || in.RefundFX != nil {
		return out, core.Invalid("actual_paid", "请核对已经发生的实际付款及本笔费用核销；此操作不发起银行转账")
	}
	check := func(tx pgx.Tx) (Cost, error) {
		c, e := metadata(ctx, tx, cmd.Actor.OrgID, in.Cost.ID)
		if e != nil {
			return c, e
		}
		if e = costPaymentAuthority(ctx, tx, cmd.Actor, c, true); e != nil {
			return c, e
		}
		if in.Claim != nil {
			e = costPaymentClaim(ctx, tx, cmd.Actor, *in.Claim, false, false)
		}
		return c, e
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		c, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = paymentProjectLocks(ctx, tx, cmd.Actor, costPaymentProspect(cmd.Actor, c), []string{c.ID}, true); e != nil {
			return core.Result{}, e
		}
		if in.Claim != nil {
			if e = costPaymentClaim(ctx, tx, cmd.Actor, *in.Claim, true, true); e != nil {
				return core.Result{}, e
			}
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", cmd.Actor.OrgID, c.ID); e != nil {
			return core.Result{}, e
		}
		c, e = metadata(ctx, tx, cmd.Actor.OrgID, c.ID)
		if e != nil {
			return core.Result{}, e
		}
		if c.Version != in.Cost.Version || c.CurrentRevisionID != in.Cost.RevisionID || c.ContentHash != in.Cost.ContentHash {
			return core.Result{}, core.Fail(409, "PAYMENT_COST_CHANGED", "费用依据已变更，请重新查阅并核对付款")
		}
		child := *s
		child.Commands.Tx = tx
		invoke := func(step string, body any) core.Command {
			next := cmd
			next.Route = cmd.Route + "/composed/" + step
			next.Key = "cost-payment:" + core.Digest([]byte(cmd.Route+"\n"+cmd.Key+"\n"+step))
			next.Body, _ = json.Marshal(body)
			return next
		}
		create := PaymentCreate{PaymentContent: in.PaymentContent, SourceKey: in.SourceKey, ProjectID: c.ProjectID, Classification: c.Classification, Reason: in.Reason}
		created, e := child.CreatePayment(ctx, invoke("create", create), create)
		if e != nil {
			return core.Result{}, e
		}
		p := created.Data.(Payment)
		base := func() Base {
			return Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: in.Reason}
		}
		record := PaymentRecordInput{Base: base(), RefundAllocations: []RefundAllocation{}}
		recorded, e := child.RecordPayment(ctx, invoke("record", record), p.ID, record)
		if e != nil {
			return core.Result{}, e
		}
		p = recorded.Data.(Payment)
		allocate := PaymentAllocate{Base: base(), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: in.AllocationAmount, FX: in.FX}
		if in.Claim != nil {
			allocate.ClaimID, allocate.ClaimRevisionID, allocate.ClaimLineID = &in.Claim.ID, &in.Claim.RevisionID, &in.Claim.LineID
		}
		allocated, e := child.AllocatePayment(ctx, invoke("allocate", allocate), p.ID, allocate)
		if e != nil {
			return core.Result{}, e
		}
		p = allocated.Data.(Payment)
		var allocationID string
		if e = tx.QueryRow(ctx, "SELECT id FROM payment_allocations WHERE org_id=$1 AND payment_id=$2 AND kind='allocation'", cmd.Actor.OrgID, p.ID).Scan(&allocationID); e != nil {
			return core.Result{}, e
		}
		allocation, e := allocationMetadata(ctx, tx, cmd.Actor.OrgID, allocationID)
		if e != nil {
			return core.Result{}, e
		}
		result := paymentResult(p, "cost.recorded", in.Reason, 201)
		result.Event.Payload["cost_id"] = c.ID
		result.Event.Payload["cost_revision_id"] = c.CurrentRevisionID
		result.Event.Payload["actual_paid"] = true
		result.Event.Payload["settlement_reviewed"] = true
		result.Data = CostPaymentReceipt{Payment: p, Allocation: allocation}
		return result, nil
	})
}
