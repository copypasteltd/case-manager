package finance

import (
	"context"
	"math/big"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Principal is entered in the original payment currency. Cash is an exact
// remaining-ratio attribution of already recorded cash, never another FX rate
// or bank operation. The last principal consumes the exact remaining cash.
func foreignRefundCashAmount(ctx context.Context, tx pgx.Tx, org string, p Payment, principal string) (string, error) {
	var usedPrincipal, usedCash string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(amount),0)::text,COALESCE(sum(refund_cash_amount),0)::text
 FROM payment_allocations WHERE org_id=$1 AND payment_id=$2 AND kind='refund'`, org, p.ID).Scan(&usedPrincipal, &usedCash); err != nil {
		return "", err
	}
	total, _ := core.ParseDecimal(p.RefundFX.OriginalAmount)
	used, _ := core.ParseDecimal(usedPrincipal)
	wanted, err := paymentAmount(ctx, tx, p.RefundFX.OriginalCurrency, principal)
	if err != nil {
		return "", err
	}
	remaining := total.Sub(used)
	if !remaining.Positive() || wanted.Cmp(remaining) > 0 {
		return "", core.Fail(409, "REFUND_PRINCIPAL_EXCEEDED", "费用回转超过该退款尚未归属的原付款本金")
	}
	cash, _ := core.ParseDecimal(value(p.Amount))
	usedD, _ := core.ParseDecimal(usedCash)
	left := cash.Sub(usedD)
	var exponent int
	if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", p.Currency).Scan(&exponent); err != nil {
		return "", err
	}
	n, _ := new(big.Rat).SetString(left.String())
	d, _ := new(big.Rat).SetString(remaining.String())
	return convertFXRational(wanted.String(), new(big.Rat).Quo(n, d), exponent)
}

func (s *Service) inverseRefundAllocation(ctx context.Context, tx pgx.Tx, a core.Actor, p Payment, source PaymentAllocation, plan RefundAllocation, acknowledged bool, reason string) (string, error) {
	if p.RefundFX == nil {
		if acknowledged {
			return "", core.Invalid("refund_fx_acknowledged", "同币退款不接受异币回转声明")
		}
		return s.inverseAllocation(ctx, tx, a, p, source, plan.Amount, "refund", reason, plan.CostAmount)
	}
	if !acknowledged || plan.CostAmount == nil {
		return "", core.Invalid("refund_fx_acknowledged", "请明确核对实收现金、原付款本金与费用原币回转金额")
	}
	// Re-read under the ordinary project/claim/cost/payment locks.
	var err error
	source, err = allocationMetadata(ctx, tx, a.OrgID, source.ID)
	if err != nil {
		return "", err
	}
	if source.Kind != "allocation" || source.PaymentID != value(p.RefundOfID) || source.PaymentCurrency != p.RefundFX.OriginalCurrency {
		return "", core.Fail(409, "ALLOCATION_SOURCE_INVALID", "须选择该退款原付款的准确正向费用分配")
	}
	cash, err := foreignRefundCashAmount(ctx, tx, a.OrgID, p, plan.Amount)
	if err != nil {
		return "", err
	}
	// The existing inverse conserves original-payment and expense units, including
	// employee claims, project caps and all remaining-balance rounding.
	source.RefundCash = &PaymentAllocationRefundCash{Amount: cash, Currency: value(p.Currency), WholeAmount: value(p.Amount), OriginalAmount: p.RefundFX.OriginalAmount, OriginalCurrency: p.RefundFX.OriginalCurrency, Rate: p.RefundFX.Rate, Source: p.RefundFX.Source}
	return s.inverseAllocation(ctx, tx, a, p, source, plan.Amount, "refund", reason, plan.CostAmount)
}
