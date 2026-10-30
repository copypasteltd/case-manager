package finance

import (
	"context"
	"math/big"
	"strings"
	"unicode"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// The quote bridges actual refund cash to original principal only. It is never
// an expense allocation, a reporting valuation rate, or an FX gain/loss entry.
type PaymentRefundFX struct {
	OriginalCurrency string `json:"original_currency"`
	OriginalAmount   string `json:"original_amount"`
	Rate             string `json:"rate"`
	Source           string `json:"source"`
}

func normalizeRefundFX(ctx context.Context, db platform.DBTX, c PaymentContent) (*PaymentRefundFX, error) {
	if c.RefundFX == nil {
		return nil, nil
	}
	q := *c.RefundFX
	q.Source = strings.TrimSpace(q.Source)
	if value(c.Direction) != "refund" || c.RefundOfID == nil || c.Currency == nil || c.Amount == nil || q.OriginalCurrency == *c.Currency {
		return nil, core.Invalid("refund_fx", "异币退款依据仅适用于实际实收币种不同于原付款的退款")
	}
	principal, err := paymentAmount(ctx, db, q.OriginalCurrency, q.OriginalAmount)
	if err != nil {
		return nil, err
	}
	var exponent int
	if err = db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", value(c.Currency)).Scan(&exponent); err != nil {
		return nil, core.Invalid("currency", "实收币种未配置")
	}
	if err = checkRefundQuote(q, principal.String(), value(c.Amount), exponent); err != nil {
		return nil, err
	}
	q.OriginalAmount = principal.String()
	return &q, nil
}

func checkRefundQuote(q PaymentRefundFX, principal, cash string, exponent int) error {
	if exponent < 0 || len([]rune(q.Source)) < 1 || len([]rune(q.Source)) > 200 || strings.IndexFunc(q.Source, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 || !settlementRatePattern.MatchString(q.Rate) {
		return core.Invalid("refund_fx", "请填写准确原付款本金、正向实际汇率和有效凭证来源")
	}
	rate, ok := new(big.Rat).SetString(q.Rate)
	if !ok || rate.Sign() <= 0 {
		return core.Invalid("refund_fx", "实际实收汇率须为正数")
	}
	converted, err := convertFXAmount(principal, q.Rate, exponent)
	d, e := core.ParseDecimal(cash)
	if err != nil || e != nil || !d.Positive() || converted != d.String() {
		return core.Invalid("refund_fx", "原付款本金乘实际汇率按实收币种精度舍入后须精确等于实收金额")
	}
	return nil
}

func refundPrincipal(p Payment) string {
	if p.RefundFX != nil {
		return p.RefundFX.OriginalAmount
	}
	return value(p.Amount)
}
