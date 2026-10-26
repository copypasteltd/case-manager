package core

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var amountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Decimal stores six fractional digits. Never convert business amounts to float64.
type Decimal struct{ units big.Int }

func ParseDecimal(s string) (Decimal, error) {
	var d Decimal
	if !amountPattern.MatchString(s) {
		return d, Invalid("amount", "金额需为非负十进制字符串，最多6位小数")
	}
	parts := strings.SplitN(s, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	d.units.SetString(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10)
	return d, nil
}
func (d Decimal) Add(other Decimal) Decimal {
	var out Decimal
	out.units.Add(&d.units, &other.units)
	return out
}
func (d Decimal) Sub(other Decimal) Decimal {
	var out Decimal
	out.units.Sub(&d.units, &other.units)
	return out
}
func (d Decimal) Cmp(other Decimal) int { return d.units.Cmp(&other.units) }
func (d Decimal) Positive() bool        { return d.units.Sign() > 0 }
func (d Decimal) String() string {
	var absolute big.Int
	absolute.Abs(&d.units)
	s := absolute.String()
	if len(s) < 7 {
		s = strings.Repeat("0", 7-len(s)) + s
	}
	sign := ""
	if d.units.Sign() < 0 {
		sign = "-"
	}
	return sign + s[:len(s)-6] + "." + s[len(s)-6:]
}
func CheckCurrencyAmount(currency, amount string, exponents map[string]int) (Decimal, error) {
	d, err := ParseDecimal(amount)
	if err != nil {
		return d, err
	}
	exponent, ok := exponents[currency]
	if !currencyPattern.MatchString(currency) || !ok {
		return d, Invalid("currency", "币种未配置")
	}
	parts := strings.SplitN(amount, ".", 2)
	if len(parts) == 2 && len(strings.TrimRight(parts[1], "0")) > exponent {
		return d, Invalid("amount", fmt.Sprintf("该币种最多允许%d位小数", exponent))
	}
	return d, nil
}
