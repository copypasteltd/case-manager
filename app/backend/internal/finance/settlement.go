package finance

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func settlementState(paid, total core.Decimal) string {
	if !total.Positive() {
		return "settled"
	}
	if !paid.Positive() {
		return "unsettled"
	}
	if paid.Cmp(total) < 0 {
		return "partial"
	}
	return "settled"
}
func costSettlement(ctx context.Context, db platform.DBTX, a core.Actor, c *Cost) error {
	c.PaidAmount = nil
	c.OutstandingAmount = nil
	c.SettlementState = nil
	c.SettlementVisibility = "restricted"
	if c.State == "draft" {
		c.SettlementVisibility = "not_confirmed"
		return nil
	}
	if authz.Require(ctx, db, a, "payment.read", authz.Resource{Type: "payment", ID: c.ID, ProjectID: c.ProjectID, OwnerID: &c.CreatorID, Classification: c.Classification}) != nil {
		return nil
	}
	var visible bool
	var net string
	if err := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM payment_allocations x WHERE x.org_id=$1 AND x.cost_id=$2 AND NOT can_read_payment(x.org_id,$3,x.payment_id,$4,$5)),COALESCE(sum(effect*cost_amount),0)::text FROM payment_allocations WHERE org_id=$1 AND cost_id=$2`, a.OrgID, c.ID, a.UserID, a.Channel, a.RunID != "").Scan(&visible, &net); err != nil {
		return err
	}
	if !visible {
		return nil
	}
	paid, _ := core.ParseDecimal(net)
	amount, _ := core.ParseDecimal(value(c.Amount))
	if c.EntryKind != "expense" || c.State == "reversed" {
		amount = core.Decimal{}
	}
	paidText := paid.String()
	outstanding := amount.Sub(paid).String()
	state := settlementState(paid, amount)
	c.PaidAmount = &paidText
	c.OutstandingAmount = &outstanding
	c.SettlementState = &state
	c.SettlementVisibility = "complete"
	return nil
}

func (s *Service) ClaimSettlement(ctx context.Context, a core.Actor, id string) (ClaimSettlement, error) {
	out := ClaimSettlement{ClaimID: id, Visibility: "not_approved"}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var project, revision *string
	var creator, class string
	if err = tx.QueryRow(ctx, "SELECT creator_id,project_id,classification,approved_revision_id FROM spend_documents WHERE org_id=$1 AND id=$2 AND kind='claim'", a.OrgID, id).Scan(&creator, &project, &class, &revision); err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = authz.Require(ctx, tx, a, "claim.read", authz.Resource{Type: "claim", ID: id, ProjectID: project, OwnerID: &creator, Classification: class}); err != nil {
		return out, err
	}
	if revision == nil {
		return out, tx.Commit(ctx)
	}
	out.RevisionID = revision
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM spend_revisions WHERE org_id=$1 AND id=$2", a.OrgID, revision).Scan(&raw); err != nil {
		return out, err
	}
	var content struct {
		Currency *string `json:"currency"`
		Lines    []struct {
			Funding string  `json:"funding"`
			Amount  *string `json:"declared_amount"`
		} `json:"lines"`
	}
	if err = json.Unmarshal(raw, &content); err != nil {
		return out, err
	}
	out.Currency = content.Currency
	total := core.Decimal{}
	for _, line := range content.Lines {
		if line.Funding == "employee_advance" && line.Amount != nil {
			v, e := core.ParseDecimal(*line.Amount)
			if e != nil {
				return out, e
			}
			total = total.Add(v)
		}
	}
	eligible := total.String()
	out.EligibleAmount = &eligible
	out.Visibility = "restricted"
	if authz.Require(ctx, tx, a, "payment.read", authz.Resource{Type: "payment", ID: id, ProjectID: project, OwnerID: &creator, Classification: class}) != nil {
		return out, tx.Commit(ctx)
	}
	var visible bool
	var net string
	if err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM payment_allocations x WHERE x.org_id=$1 AND x.claim_id=$2 AND x.claim_revision_id=$3 AND NOT can_read_payment(x.org_id,$4,x.payment_id,$5,$6)),COALESCE(sum(effect*cost_amount),0)::text FROM payment_allocations WHERE org_id=$1 AND claim_id=$2 AND claim_revision_id=$3`, a.OrgID, id, revision, a.UserID, a.Channel, a.RunID != "").Scan(&visible, &net); err != nil {
		return out, err
	}
	if visible {
		paid, e := core.ParseDecimal(net)
		if e != nil {
			return out, e
		}
		paidText := paid.String()
		outstanding := total.Sub(paid).String()
		state := settlementState(paid, total)
		out.PaidAmount = &paidText
		out.OutstandingAmount = &outstanding
		out.State = &state
		out.Visibility = "complete"
	}
	return out, tx.Commit(ctx)
}
