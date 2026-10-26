package core

import "math/big"

// QuantityPrice rounds the exact product only once, at the currency's
// configured precision. Both inputs remain decimal strings throughout.
func QuantityPrice(quantity, price string, exponent int, rounding string) (Decimal, error) {
	q, err := ParseDecimal(quantity)
	if err != nil || !q.Positive() {
		return Decimal{}, Invalid("quantity", "数量须为正十进制数，最多6位小数")
	}
	p, err := ParseDecimal(price)
	if err != nil {
		return Decimal{}, err
	}
	if exponent < 0 || exponent > 6 || (rounding != "half_up" && rounding != "half_even") {
		return Decimal{}, Invalid("rounding_mode", "请明确币种精度及舍入方式")
	}
	var product, divisor, quotient, remainder, twice big.Int
	product.Mul(&q.units, &p.units)
	divisor.Exp(big.NewInt(10), big.NewInt(int64(12-exponent)), nil)
	quotient.QuoRem(&product, &divisor, &remainder)
	twice.Lsh(&remainder, 1)
	comparison := twice.Cmp(&divisor)
	if comparison > 0 || (comparison == 0 && (rounding == "half_up" || quotient.Bit(0) != 0)) {
		quotient.Add(&quotient, big.NewInt(1))
	}
	var scale big.Int
	scale.Exp(big.NewInt(10), big.NewInt(int64(6-exponent)), nil)
	var result Decimal
	result.units.Mul(&quotient, &scale)
	// numeric(20,6) must not silently truncate a large quantity-price product.
	if _, err := ParseDecimal(result.String()); err != nil {
		return Decimal{}, Invalid("amount", "数量与单价乘积超过金额范围")
	}
	return result, nil
}
