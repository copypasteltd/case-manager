package finance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func insertPaymentAllocation(ctx context.Context, tx pgx.Tx, a core.Actor, p Payment, v PaymentAllocation) error {
	if v.CostAmount == "" {
		v.CostAmount = v.Amount
	}
	if v.CostShares == nil {
		v.CostShares = v.Shares
	}
	if len(v.Shares) != len(v.CostShares) {
		return core.Invalid("shares", "双币项目份额未对齐")
	}
	var rate, source, day *string
	var refundCash *string
	if v.RefundCash != nil {
		refundCash = &v.RefundCash.Amount
	}
	if v.FX != nil {
		rate, source, day = &v.FX.Rate, &v.FX.Source, &v.FX.QuotedOn
	}
	_, err := tx.Exec(ctx, `INSERT INTO payment_allocations(id,org_id,payment_id,cost_id,cost_revision_id,kind,effect,amount,root_id,source_allocation_id,claim_id,claim_revision_id,claim_line_id,actor_id,reason,cost_amount,fx_rate,fx_source,fx_quoted_on,refund_cash_amount,refund_fx_acknowledged) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, v.ID, a.OrgID, p.ID, v.CostID, v.CostRevisionID, v.Kind, v.Effect, v.Amount, v.RootID, v.SourceAllocationID, v.ClaimID, v.ClaimRevisionID, v.ClaimLineID, a.UserID, v.Reason, v.CostAmount, rate, source, day, refundCash, v.RefundCash != nil)
	if err != nil {
		return err
	}
	for i, share := range v.Shares {
		if value(share.ProjectID) != value(v.CostShares[i].ProjectID) {
			return core.Invalid("shares", "双币项目份额的范围未对齐")
		}
		var cash *string
		if v.RefundCash != nil {
			if len(v.RefundCash.Shares) != len(v.Shares) || value(v.RefundCash.Shares[i].ProjectID) != value(share.ProjectID) {
				return core.Invalid("shares", "实收现金份额与原本金范围未对齐")
			}
			cash = &v.RefundCash.Shares[i].Amount
		}
		if _, err = tx.Exec(ctx, "INSERT INTO payment_allocation_shares(org_id,allocation_id,position,project_id,amount,cost_amount,refund_cash_amount) VALUES($1,$2,$3,$4,$5,$6,$7)", a.OrgID, v.ID, i+1, share.ProjectID, share.Amount, v.CostShares[i].Amount, cash); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AllocatePayment(ctx context.Context, cmd core.Command, id string, in PaymentAllocate) (core.Result, error) {
	if !core.ValidID(in.CostID) || core.RequireVersion(in.CostVersion) != nil || !core.ValidID(in.CostRevisionID) || len(in.CostContentHash) != 64 || strings.Trim(in.CostContentHash, "0123456789abcdef") != "" {
		return core.Result{}, core.Invalid("cost_id", "请固定准确费用、版本、修订和摘要")
	}
	claims := []string{}
	if in.ClaimID != nil {
		claims = append(claims, *in.ClaimID)
	}
	for _, id := range []*string{in.ClaimID, in.ClaimRevisionID, in.ClaimLineID} {
		if id != nil && !core.ValidID(*id) {
			return core.Result{}, core.Invalid("claim_id", "报销来源标识无效")
		}
	}
	if (in.ClaimID == nil) != (in.ClaimRevisionID == nil) || (in.ClaimID == nil) != (in.ClaimLineID == nil) {
		return core.Result{}, core.Invalid("claim_id", "员工结算须固定完整已批准报销明细")
	}
	return s.mutatePayment(ctx, cmd, id, "payment.record", in.Base, true, []string{in.CostID}, claims, func(tx pgx.Tx, p Payment) (core.Result, error) {
		if p.State != "recorded" || p.RecordKind != "payment" || value(p.Direction) != "outgoing" {
			return core.Result{}, core.Fail(409, "PAYMENT_NOT_ALLOCATABLE", "仅已登记未冲销的实际付款可以分配费用")
		}
		c, err := metadata(ctx, tx, cmd.Actor.OrgID, in.CostID)
		if err != nil {
			return core.Result{}, err
		}
		if err = readable(ctx, tx, cmd.Actor, c, c.CurrentRevisionID); err != nil {
			return core.Result{}, err
		}
		if c.Version != in.CostVersion || c.CurrentRevisionID != strings.ToLower(in.CostRevisionID) || c.ContentHash != in.CostContentHash {
			return core.Result{}, core.Conflict()
		}
		if c.State != "confirmed" || c.EntryKind != "expense" || (value(c.Currency) != value(p.Currency) && in.FX == nil) {
			return core.Result{}, core.Fail(409, "COST_NOT_SETTLEABLE", "须选择已确认且未冲销的原费用；跨币种须明确核对实际汇率及费用原币金额")
		}
		var adequate bool
		if err = tx.QueryRow(ctx, "SELECT classification_rank($1)>=classification_rank($2)", p.Classification, c.Classification).Scan(&adequate); err != nil {
			return core.Result{}, err
		}
		if !adequate {
			return core.Result{}, core.Invalid("classification", "资金分类不能低于费用")
		}
		amount, err := paymentAmount(ctx, tx, value(p.Currency), in.Amount)
		if err != nil {
			return core.Result{}, err
		}
		if err = paymentBalances(ctx, tx, cmd.Actor.OrgID, &p); err != nil {
			return core.Result{}, err
		}
		balance, _ := core.ParseDecimal(value(p.UnallocatedAmount))
		if amount.Cmp(balance) > 0 {
			return core.Result{}, core.Fail(409, "PAYMENT_BALANCE_EXCEEDED", "分配金额超过付款当前未分配余额")
		}
		costAmount, quote, err := reviewedSettlementQuote(ctx, tx, p, c, in)
		if err != nil {
			return core.Result{}, err
		}
		if p.EmployeeID == nil {
			if in.ClaimID != nil || p.Counterparty == nil || c.Counterparty == nil || p.Counterparty.PartyID != c.Counterparty.PartyID {
				return core.Result{}, core.Invalid("counterparty", "直接付款须匹配费用供应方，员工报销须使用员工收款付款")
			}
		} else if err = verifyClaimSettlement(ctx, tx, cmd.Actor, p, c, in, costAmount); err != nil {
			return core.Result{}, err
		}
		costShares, err := settlementShares(ctx, tx, cmd.Actor.OrgID, value(c.Currency), costAmount.String(), c.ID, false)
		if err != nil {
			return core.Result{}, err
		}
		shares := costShares
		if quote != nil {
			var exponent int
			if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", p.Currency).Scan(&exponent); err != nil {
				return core.Result{}, err
			}
			shares, err = splitSettlementShares(amount.String(), costShares, exponent, false)
			if err != nil {
				return core.Result{}, err
			}
			shares, costShares = pairedSettlementShares(costShares, shares, costShares)
		}
		allocation := core.NewID()
		v := PaymentAllocation{ID: allocation, CostID: c.ID, CostRevisionID: c.CurrentRevisionID, Kind: "allocation", Effect: 1, Amount: amount.String(), CostAmount: costAmount.String(), FX: quote, RootID: allocation, ClaimID: in.ClaimID, ClaimRevisionID: in.ClaimRevisionID, ClaimLineID: in.ClaimLineID, Shares: shares, CostShares: costShares, Reason: in.Reason}
		if err = insertPaymentAllocation(ctx, tx, cmd.Actor, p, v); err != nil {
			return core.Result{}, err
		}
		return s.finishPayment(ctx, tx, cmd.Actor, p, "allocation_added", in.Reason, &allocation)
	})
}

func verifyClaimSettlement(ctx context.Context, tx pgx.Tx, a core.Actor, p Payment, c Cost, in PaymentAllocate, amount core.Decimal) error {
	if in.ClaimID == nil {
		return core.Invalid("claim_id", "员工报销结算须选择已批准的垫付明细")
	}
	var creator, class string
	var project *string
	var approved *string
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT d.creator_id,d.classification,d.project_id,d.approved_revision_id,r.payload FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.document_id=d.id AND r.id=$3 WHERE d.org_id=$1 AND d.id=$2 AND d.kind='claim'`, a.OrgID, in.ClaimID, in.ClaimRevisionID).Scan(&creator, &class, &project, &approved, &payload)
	if err != nil {
		return core.Missing()
	}
	if err = authz.Require(ctx, tx, a, "claim.read", authz.Resource{Type: "claim", ID: *in.ClaimID, ProjectID: project, OwnerID: &creator, Classification: class}); err != nil {
		return err
	}
	if approved == nil || *approved != *in.ClaimRevisionID || creator != value(p.EmployeeID) {
		return core.Fail(409, "CLAIM_NOT_SETTLEABLE", "须使用该收款员工的已批准报销修订")
	}
	var linked bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cost_links WHERE org_id=$1 AND cost_id=$2 AND source_type='claim' AND source_id=$3 AND revision_id=$4)", a.OrgID, c.ID, in.ClaimID, in.ClaimRevisionID).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return core.Fail(409, "CLAIM_COST_NOT_LINKED", "费用须先关联该准确报销修订")
	}
	var content struct {
		Currency *string `json:"currency"`
		Lines    []struct {
			ID       string    `json:"id"`
			Funding  string    `json:"funding"`
			Amount   *string   `json:"declared_amount"`
			Supplier *PartyRef `json:"supplier"`
		} `json:"lines"`
	}
	if err = json.Unmarshal(payload, &content); err != nil {
		return err
	}
	for _, line := range content.Lines {
		if line.ID != *in.ClaimLineID {
			continue
		}
		if line.Funding != "employee_advance" || line.Amount == nil || line.Supplier == nil || c.Counterparty == nil || line.Supplier.PartyID != c.Counterparty.PartyID || value(content.Currency) != value(c.Currency) {
			return core.Fail(409, "CLAIM_LINE_NOT_SETTLEABLE", "须选择与费用相同币种及供应方的员工垫付明细")
		}
		var paid string
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(effect*cost_amount),0)::text FROM payment_allocations WHERE org_id=$1 AND claim_id=$2 AND claim_revision_id=$3 AND claim_line_id=$4", a.OrgID, in.ClaimID, in.ClaimRevisionID, in.ClaimLineID).Scan(&paid); err != nil {
			return err
		}
		used, _ := core.ParseDecimal(paid)
		eligible, _ := core.ParseDecimal(*line.Amount)
		if used.Add(amount).Cmp(eligible) > 0 {
			return core.Fail(409, "CLAIM_BALANCE_EXCEEDED", "结算超过该员工垫付明细的当前未付金额")
		}
		return nil
	}
	return core.Invalid("claim_line_id", "报销修订不包含所选明细")
}

func (s *Service) inverseAllocation(ctx context.Context, tx pgx.Tx, a core.Actor, p Payment, source PaymentAllocation, amount, kind, reason string, expectedCost *string) (string, error) {
	// The caller has now acquired cost/payment locks; an earlier source read
	// can be stale after a separate refund changed the root balance.
	var err error
	refundCash := source.RefundCash
	source, err = allocationMetadata(ctx, tx, a.OrgID, source.ID)
	if err != nil {
		return "", err
	}
	if source.Kind != "allocation" || (kind == "release" && source.PaymentID != p.ID) || (kind == "refund" && source.PaymentID != value(p.RefundOfID)) {
		return "", core.Fail(409, "ALLOCATION_SOURCE_INVALID", "须引用相应原付款的正向费用分配")
	}
	d, err := paymentAmount(ctx, tx, source.PaymentCurrency, amount)
	if err != nil {
		return "", err
	}
	costAmount, err := requireInverseCostAmount(source, d.String(), expectedCost)
	if err != nil {
		return "", err
	}
	shares, err := settlementShares(ctx, tx, a.OrgID, source.PaymentCurrency, d.String(), source.RootID, true)
	if err != nil {
		return "", err
	}
	costShares := shares
	if source.FX != nil {
		remaining, e := rootCostShares(ctx, tx, a.OrgID, source.RootID)
		if e != nil {
			return "", e
		}
		costShares = []Allocation{}
		cost, _ := core.ParseDecimal(costAmount)
		if cost.Positive() {
			costShares, e = splitPaymentShares(costAmount, remaining, source.CostExponent)
			if e != nil {
				return "", e
			}
		}
		scopes, e := remainingShares(ctx, tx, a.OrgID, source.RootID, true)
		if e != nil {
			return "", e
		}
		shares, costShares = pairedSettlementShares(scopes, shares, costShares)
	}
	v := source
	v.ID = core.NewID()
	v.Kind = kind
	v.Effect = -1
	v.Amount = d.String()
	v.CostAmount = costAmount
	v.SourceAllocationID = &source.ID
	v.Shares = shares
	v.CostShares = costShares
	v.Reason = reason
	v.RefundCash = refundCash
	if refundCash != nil {
		var exponent int
		if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", p.Currency).Scan(&exponent); err != nil {
			return "", err
		}
		cash, _ := core.ParseDecimal(refundCash.Amount)
		cashShares := []Allocation{}
		if cash.Positive() {
			cashShares, err = splitSettlementShares(cash.String(), v.Shares, exponent, false)
			if err != nil {
				return "", err
			}
		}
		// Retain positions where the cash currency contributes zero minor units.
		v.RefundCash.Shares = make([]Allocation, len(v.Shares))
		for i, share := range v.Shares {
			v.RefundCash.Shares[i] = Allocation{ProjectID: share.ProjectID, Amount: "0.000000"}
			for _, part := range cashShares {
				if value(part.ProjectID) == value(share.ProjectID) {
					v.RefundCash.Shares[i].Amount = part.Amount
					break
				}
			}
		}
	}
	return v.ID, insertPaymentAllocation(ctx, tx, a, p, v)
}
func (s *Service) ReleasePaymentAllocation(ctx context.Context, cmd core.Command, id, allocation string, in PaymentRelease) (core.Result, error) {
	source, err := allocationMetadata(ctx, s.DB, cmd.Actor.OrgID, allocation)
	if err != nil {
		return core.Result{}, err
	}
	claims := []string{}
	if source.ClaimID != nil {
		claims = append(claims, *source.ClaimID)
	}
	return s.mutatePayment(ctx, cmd, id, "payment.record", in.Base, true, []string{source.CostID}, claims, func(tx pgx.Tx, p Payment) (core.Result, error) {
		if p.State != "recorded" || p.RecordKind != "payment" || value(p.Direction) != "outgoing" {
			return core.Result{}, core.Fail(409, "PAYMENT_NOT_ALLOCATABLE", "仅未冲销原付款可以释放费用分配")
		}
		key, err := s.inverseAllocation(ctx, tx, cmd.Actor, p, source, in.Amount, "release", in.Reason, in.CostAmount)
		if err != nil {
			return core.Result{}, err
		}
		return s.finishPayment(ctx, tx, cmd.Actor, p, "allocation_released", in.Reason, &key)
	})
}
func (s *Service) AllocateRefund(ctx context.Context, cmd core.Command, id string, in PaymentRefundAllocate) (core.Result, error) {
	source, err := allocationMetadata(ctx, s.DB, cmd.Actor.OrgID, in.SourceAllocationID)
	if err != nil {
		return core.Result{}, err
	}
	claims := []string{}
	if source.ClaimID != nil {
		claims = append(claims, *source.ClaimID)
	}
	return s.mutatePayment(ctx, cmd, id, "payment.record", in.Base, true, []string{source.CostID}, claims, func(tx pgx.Tx, p Payment) (core.Result, error) {
		if p.State != "recorded" || p.RecordKind != "payment" || value(p.Direction) != "refund" {
			return core.Result{}, core.Fail(409, "REFUND_NOT_ALLOCATABLE", "仅未冲销的实际退款可以关联原分配")
		}
		if p.RefundFX != nil {
			key, err := s.inverseRefundAllocation(ctx, tx, cmd.Actor, p, source, in.RefundAllocation, in.RefundFXAcknowledged, in.Reason)
			if err != nil {
				return core.Result{}, err
			}
			return s.finishPayment(ctx, tx, cmd.Actor, p, "refund_allocated", in.Reason, &key)
		}
		if in.RefundFXAcknowledged {
			return core.Result{}, core.Invalid("refund_fx_acknowledged", "同币退款不接受异币回转声明")
		}
		if err := paymentBalances(ctx, tx, cmd.Actor.OrgID, &p); err != nil {
			return core.Result{}, err
		}
		d, err := paymentAmount(ctx, tx, value(p.Currency), in.Amount)
		if err != nil {
			return core.Result{}, err
		}
		balance, _ := core.ParseDecimal(value(p.UnallocatedAmount))
		if d.Cmp(balance) > 0 {
			return core.Result{}, core.Fail(409, "REFUND_BALANCE_EXCEEDED", "超过该退款当前尚未分配金额")
		}
		key, err := s.inverseAllocation(ctx, tx, cmd.Actor, p, source, d.String(), "refund", in.Reason, in.CostAmount)
		if err != nil {
			return core.Result{}, err
		}
		return s.finishPayment(ctx, tx, cmd.Actor, p, "refund_allocated", in.Reason, &key)
	})
}

func (s *Service) ReversePayment(ctx context.Context, cmd core.Command, id string, in PaymentReverseInput) (core.Result, error) {
	at, err := time.Parse(time.RFC3339Nano, in.EffectiveAt)
	if err != nil {
		return core.Result{}, core.Invalid("effective_at", "冲销生效时间须包含时区")
	}
	text := at.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	return s.mutatePayment(ctx, cmd, id, "payment.reverse", in.Base, true, nil, nil, func(tx pgx.Tx, p Payment) (core.Result, error) {
		if p.State != "recorded" || p.RecordKind != "payment" {
			return core.Result{}, core.Fail(409, "PAYMENT_NOT_REVERSIBLE", "仅未冲销的已登记原资金可全额冲销")
		}
		if err := paymentBalances(ctx, tx, cmd.Actor.OrgID, &p); err != nil {
			return core.Result{}, err
		}
		refunds, _ := core.ParseDecimal(p.RefundedAmount)
		if refunds.Positive() {
			return core.Result{}, core.Fail(409, "PAYMENT_HAS_REFUNDS", "须先冲销已登记退款，再冲销原付款")
		}
		blocked, err := paymentHasContractReturns(ctx, tx, cmd.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		if blocked {
			return core.Result{}, core.Fail(409, "PAYMENT_HAS_CONTRACT_RETURNS", "须先冲销或解除已归属的退回资金，再冲销原收付款")
		}
		var now time.Time
		if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return core.Result{}, err
		}
		paid, _ := time.Parse(time.RFC3339Nano, value(p.PaidAt))
		if at.Before(paid) || at.After(now) {
			return core.Result{}, core.Invalid("effective_at", "冲销时间须介于原付款与当前时间之间")
		}
		reversal := p
		reversal.ID = core.NewID()
		reversal.Version = 1
		reversal.SourceKey = "reversal:" + p.ID
		reversal.RecordKind = "reversal"
		reversal.ReversesID = &p.ID
		reversal.ReversalID = nil
		reversal.CurrentRevisionID = core.NewID()
		reversal.PaidAt = &text
		if _, err := tx.Exec(ctx, `INSERT INTO payment_records(id,org_id,source_key,project_id,creator_id,classification,record_kind,state,reverses_id,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,'reversal','recorded',$7,$8)`, reversal.ID, cmd.Actor.OrgID, reversal.SourceKey, reversal.ProjectID, reversal.CreatorID, reversal.Classification, p.ID, reversal.CurrentRevisionID); err != nil {
			return core.Result{}, err
		}
		if err := insertPaymentRevision(ctx, tx, cmd.Actor, reversal, reversal.CurrentRevisionID, 1, nil, reversal.PaymentContent, in.Reason); err != nil {
			return core.Result{}, err
		}
		rows, err := tx.Query(ctx, "SELECT "+allocationColumns+" FROM payment_allocations a WHERE a.org_id=$1 AND a.payment_id=$2 ORDER BY a.created_at,a.id", cmd.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		legs := []PaymentAllocation{}
		for rows.Next() {
			v, e := scanAllocation(rows)
			if e != nil {
				rows.Close()
				return core.Result{}, e
			}
			legs = append(legs, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return core.Result{}, err
		}
		for _, v := range legs {
			if err = allocationShares(ctx, tx, cmd.Actor.OrgID, &v); err != nil {
				return core.Result{}, err
			}
			source := v.ID
			v.ID = core.NewID()
			v.SourceAllocationID = &source
			v.Kind = "reversal"
			v.Effect = -v.Effect
			v.Reason = in.Reason
			if err = insertPaymentAllocation(ctx, tx, cmd.Actor, reversal, v); err != nil {
				return core.Result{}, err
			}
		}
		// Reversing a refund restores its original cost attribution. A later
		// payment may have consumed that headroom; never silently overpay it.
		for _, v := range legs {
			var valid bool
			if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(a.effect*a.cost_amount),0) BETWEEN 0 AND r.amount FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id LEFT JOIN payment_allocations a ON a.org_id=c.org_id AND a.cost_id=c.id WHERE c.org_id=$1 AND c.id=$2 GROUP BY r.amount`, cmd.Actor.OrgID, v.CostID).Scan(&valid); err != nil {
				return core.Result{}, err
			}
			if !valid {
				return core.Result{}, core.Fail(409, "COST_BALANCE_EXCEEDED", "冲销将超过费用结算余额，请先纠正后续付款分配")
			}
		}
		if err = paymentEvent(ctx, tx, cmd.Actor, reversal, "reversal_created", in.Reason, nil); err != nil {
			return core.Result{}, err
		}
		p.State = "reversed"
		out, err := s.finishPayment(ctx, tx, cmd.Actor, p, "reversed", in.Reason, nil)
		if err == nil {
			out.Events = []core.Event{paymentResult(reversal, "reversal.created", in.Reason, 201).Event}
		}
		return out, err
	})
}
