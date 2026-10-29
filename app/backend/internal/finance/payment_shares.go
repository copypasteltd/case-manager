package finance

import (
	"context"
	"math/big"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// Split only the remaining balances, using integer minor units and largest
// remainders. Repeated one-cent payments cannot overfill a project allocation.
func splitPaymentShares(amount string, remaining []Allocation, exponent int) ([]Allocation, error) {
	return splitSettlementShares(amount, remaining, exponent, true)
}

func splitSettlementShares(amount string, remaining []Allocation, exponent int, enforceBalance bool) ([]Allocation, error) {
	units := func(text string, precision int) (*big.Int, error) {
		d, err := core.CheckCurrencyAmount("XXX", text, map[string]int{"XXX": precision})
		if err != nil {
			return nil, err
		}
		v, _ := new(big.Int).SetString(strings.ReplaceAll(d.String(), ".", ""), 10)
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(6-precision)), nil)
		return v.Quo(v, factor), nil
	}
	wanted, err := units(amount, exponent)
	if err != nil {
		return nil, err
	}
	if wanted.Sign() <= 0 {
		return nil, core.Invalid("amount", "分配金额须大于零")
	}
	type share struct {
		value     *big.Int
		remainder *big.Int
		index     int
	}
	total := new(big.Int)
	parts := []share{}
	weightPrecision := exponent
	if !enforceBalance {
		// Weights may use a different currency; their common six-place scale
		// cancels in the ratio and is never treated as a cash-unit balance.
		weightPrecision = 6
	}
	for i, r := range remaining {
		v, e := units(r.Amount, weightPrecision)
		if e != nil {
			return nil, e
		}
		if v.Sign() > 0 {
			total.Add(total, v)
			parts = append(parts, share{value: v, index: i})
		}
	}
	if total.Sign() <= 0 || (enforceBalance && wanted.Cmp(total) > 0) {
		return nil, core.Fail(409, "SETTLEMENT_BALANCE_EXCEEDED", "金额超过当前可分配余额")
	}
	used := new(big.Int)
	for i := range parts {
		product := new(big.Int).Mul(wanted, parts[i].value)
		q, rem := new(big.Int), new(big.Int)
		q.QuoRem(product, total, rem)
		parts[i].value = q
		parts[i].remainder = rem
		used.Add(used, q)
	}
	slices.SortStableFunc(parts, func(a, b share) int { return b.remainder.Cmp(a.remainder) })
	extra := new(big.Int).Sub(wanted, used).Int64()
	for i := int64(0); i < extra; i++ {
		parts[i].value.Add(parts[i].value, big.NewInt(1))
	}
	slices.SortFunc(parts, func(a, b share) int { return a.index - b.index })
	out := []Allocation{}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(6-exponent)), nil)
	for _, s := range parts {
		if s.value.Sign() == 0 {
			continue
		}
		digits := new(big.Int).Mul(s.value, factor).String()
		if len(digits) < 7 {
			digits = strings.Repeat("0", 7-len(digits)) + digits
		}
		text := digits[:len(digits)-6] + "." + digits[len(digits)-6:]
		out = append(out, Allocation{ProjectID: remaining[s.index].ProjectID, Amount: text})
	}
	return out, nil
}

func remainingShares(ctx context.Context, db platform.DBTX, org, id string, root bool) ([]Allocation, error) {
	query := `SELECT c.project_id,(c.amount-COALESCE((SELECT sum(a.effect*s.cost_amount) FROM payment_allocations a JOIN payment_allocation_shares s ON s.org_id=a.org_id AND s.allocation_id=a.id WHERE a.org_id=c.org_id AND a.cost_id=$2 AND s.project_id IS NOT DISTINCT FROM c.project_id),0))::text
        FROM cost_allocations c JOIN actual_cost_entries h ON h.org_id=c.org_id AND h.current_revision_id=c.revision_id WHERE c.org_id=$1 AND h.id=$2 ORDER BY c.position`
	if root {
		query = `SELECT s.project_id,COALESCE((SELECT sum(a.effect*x.amount) FROM payment_allocations a JOIN payment_allocation_shares x ON x.org_id=a.org_id AND x.allocation_id=a.id WHERE a.org_id=s.org_id AND a.root_id=$2 AND x.project_id IS NOT DISTINCT FROM s.project_id),0)::text FROM payment_allocation_shares s WHERE s.org_id=$1 AND s.allocation_id=$2 ORDER BY s.position`
	}
	rows, err := db.Query(ctx, query, org, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Allocation{}
	for rows.Next() {
		var s Allocation
		if err = rows.Scan(&s.ProjectID, &s.Amount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func settlementShares(ctx context.Context, db platform.DBTX, org, currency, amount, id string, root bool) ([]Allocation, error) {
	remaining, err := remainingShares(ctx, db, org, id, root)
	if err != nil {
		return nil, err
	}
	var exponent int
	if err = db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", currency).Scan(&exponent); err != nil {
		return nil, err
	}
	return splitPaymentShares(amount, remaining, exponent)
}
