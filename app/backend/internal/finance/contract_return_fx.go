package finance

import (
	"context"
	"math/big"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// A return quote relates this attribution's contract principal to a portion of
// an existing outgoing cash fact. It neither creates cash nor changes a FX book.
type ContractReturnFXInput struct {
	CashAmount   string `json:"cash_amount"`
	Rate         string `json:"rate"`
	Source       string `json:"source"`
	Acknowledged bool   `json:"acknowledged"`
}

type ContractReturnCash struct {
	Currency         string  `json:"currency"`
	Amount           string  `json:"amount"`
	WholeAmount      string  `json:"whole_amount"`
	PaidOn           string  `json:"paid_on"`
	OriginalAmount   string  `json:"original_amount"`
	OriginalCurrency string  `json:"original_currency"`
	Rate             string  `json:"rate"`
	Source           string  `json:"source"`
	RemainingAmount  *string `json:"remaining_amount"`
}

func reviewedContractReturnFX(ctx context.Context, db platform.DBTX, currency, principal string, in *ContractReturnFXInput) (string, string, string, error) {
	if in == nil || !in.Acknowledged || !settlementRatePattern.MatchString(in.Rate) || core.Name(strings.TrimSpace(in.Source)) != nil {
		return "", "", "", core.Invalid("return_fx", "异币向外退回须核对合同本金、实际付出金额、汇率与凭证来源")
	}
	rate, ok := new(big.Rat).SetString(in.Rate)
	if !ok || rate.Sign() <= 0 {
		return "", "", "", core.Invalid("return_fx.rate", "实际退回汇率须为正数")
	}
	cash, err := paymentAmount(ctx, db, currency, in.CashAmount)
	if err != nil {
		return "", "", "", err
	}
	var exponent int
	if err = db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", currency).Scan(&exponent); err != nil {
		return "", "", "", err
	}
	expected, err := convertFXRational(principal, rate, exponent)
	if err != nil {
		return "", "", "", err
	}
	wanted, _ := core.ParseDecimal(expected)
	if wanted.Cmp(cash) != 0 {
		return "", "", "", core.Invalid("return_fx.cash_amount", "合同本金乘汇率并按付出币精度舍入后，须等于本次实际付出归属金额")
	}
	return cash.String(), rate.FloatString(10), strings.TrimSpace(in.Source), nil
}

// Partial releases follow remaining cash / remaining principal, rounded once;
// the final release consumes the exact cash tail, including a possible zero.
func contractReturnReleaseCash(ctx context.Context, db platform.DBTX, original ContractFinanceEvent, principal string) (string, error) {
	left, ok := new(big.Rat).SetString(value(original.ReturnCash.RemainingAmount))
	denom, valid := new(big.Rat).SetString(value(original.RemainingAmount))
	if !ok || !valid || denom.Sign() <= 0 || left.Sign() < 0 {
		return "", core.Conflict()
	}
	var exponent int
	if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", original.ReturnCash.Currency).Scan(&exponent); err != nil {
		return "", err
	}
	return convertFXRational(principal, left.Quo(left, denom), exponent)
}
