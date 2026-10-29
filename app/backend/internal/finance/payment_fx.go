package finance

import (
	"context"
	"math/big"
	"regexp"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// Quotes describe an actual reviewed settlement, in payment units per cost
// unit. They never select or alter an organization's reporting FX book.
type PaymentFXInput struct {
	CostAmount   string `json:"cost_amount"`
	Rate         string `json:"rate"`
	Source       string `json:"source"`
	Acknowledged bool   `json:"acknowledged"`
}

type PaymentFXQuote struct {
	Rate     string `json:"rate"`
	Source   string `json:"source"`
	QuotedOn string `json:"quoted_on"`
}

var settlementRatePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$`)

func reviewedSettlementQuote(ctx context.Context, db platform.DBTX, p Payment, c Cost, in PaymentAllocate) (core.Decimal, *PaymentFXQuote, error) {
	if value(p.Currency) == value(c.Currency) {
		if in.FX != nil {
			return core.Decimal{}, nil, core.Invalid("fx", "同币种结算不接受汇率依据")
		}
		d, e := paymentAmount(ctx, db, value(c.Currency), in.Amount)
		return d, nil, e
	}
	if in.FX == nil || !in.FX.Acknowledged || !settlementRatePattern.MatchString(in.FX.Rate) || core.Name(in.FX.Source) != nil {
		return core.Decimal{}, nil, core.Invalid("fx", "跨币种核销须明确核对费用原币金额、实际结算汇率及凭证来源")
	}
	rate, ok := new(big.Rat).SetString(in.FX.Rate)
	if !ok || rate.Sign() <= 0 {
		return core.Decimal{}, nil, core.Invalid("fx.rate", "实际结算汇率必须为正数")
	}
	costAmount, e := paymentAmount(ctx, db, value(c.Currency), in.FX.CostAmount)
	if e != nil {
		return core.Decimal{}, nil, e
	}
	var exponent int
	if e = db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", p.Currency).Scan(&exponent); e != nil {
		return core.Decimal{}, nil, e
	}
	converted, e := convertFXAmount(costAmount.String(), in.FX.Rate, exponent)
	if e != nil {
		return core.Decimal{}, nil, e
	}
	actual, e := paymentAmount(ctx, db, value(p.Currency), in.Amount)
	if e != nil {
		return core.Decimal{}, nil, e
	}
	expected, e := core.ParseDecimal(converted)
	if e != nil || expected.Cmp(actual) != 0 {
		return core.Decimal{}, nil, core.Invalid("fx.rate", "费用原币金额乘实际汇率并按付款币精度舍入后，必须等于本次付款本金；手续费须另有真实费用及资金依据")
	}
	return costAmount, &PaymentFXQuote{Rate: rate.FloatString(10), Source: strings.TrimSpace(in.FX.Source), QuotedOn: value(p.PaidOn)}, nil
}

// Release/refund amounts use the remaining dual-unit root, rounding once in
// the cost currency. A final release consumes the exact remaining cost amount.
// Zero cost units on a small partial cash release are valid and stay explicit.
func inverseCostAmount(source PaymentAllocation, amount string) (string, error) {
	paid, e := core.ParseDecimal(amount)
	if e != nil || !paid.Positive() {
		return "", core.Invalid("amount", "本次付款币金额无效")
	}
	remaining, e := core.ParseDecimal(source.RemainingAmount)
	if e != nil || !remaining.Positive() || paid.Cmp(remaining) > 0 {
		return "", core.Fail(409, "SETTLEMENT_BALANCE_EXCEEDED", "金额超过原分配当前剩余付款币余额")
	}
	if source.CostCurrency == source.PaymentCurrency {
		return paid.String(), nil
	}
	cost, ok := new(big.Rat).SetString(source.RemainingCostAmount)
	denom, denominatorOK := new(big.Rat).SetString(source.RemainingAmount)
	if !ok || !denominatorOK || cost.Sign() < 0 || denom.Sign() <= 0 {
		return "", core.Fail(409, "SETTLEMENT_BASIS_INVALID", "原分配缺少准确双币余额，请重新读取")
	}
	cost.Quo(cost, denom)
	return convertFXRational(paid.String(), cost, source.CostExponent)
}

func convertFXRational(amount string, ratio *big.Rat, exponent int) (string, error) {
	if exponent < 0 || exponent > 6 || ratio == nil || ratio.Sign() < 0 {
		return "", core.Invalid("amount", "双币舍入依据无效")
	}
	a, ok := new(big.Rat).SetString(amount)
	if !ok || a.Sign() < 0 {
		return "", core.Invalid("amount", "双币金额无效")
	}
	a.Mul(a, ratio)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	numerator := new(big.Int).Mul(a.Num(), scale)
	units, rest := new(big.Int), new(big.Int)
	units.QuoRem(numerator, a.Denom(), rest)
	if new(big.Int).Lsh(rest, 1).Cmp(a.Denom()) >= 0 {
		units.Add(units, big.NewInt(1))
	}
	return new(big.Rat).SetFrac(units, scale).FloatString(6), nil
}

func requireInverseCostAmount(source PaymentAllocation, amount string, expected *string) (string, error) {
	result, e := inverseCostAmount(source, amount)
	if e != nil {
		return "", e
	}
	if source.CostCurrency != source.PaymentCurrency && expected == nil {
		return "", core.Invalid("cost_amount", "请核对本次反向核销的费用原币金额")
	}
	if expected != nil {
		actual, parse := core.ParseDecimal(*expected)
		wanted, _ := core.ParseDecimal(result)
		if parse != nil || actual.Cmp(wanted) != 0 {
			return "", core.Fail(409, "SETTLEMENT_BASIS_CHANGED", "双币余额或反向核销金额已变化，请重新读取并核对")
		}
	}
	return result, nil
}
