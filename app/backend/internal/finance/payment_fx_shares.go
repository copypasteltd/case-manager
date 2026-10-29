package finance

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func rootCostShares(ctx context.Context, db platform.DBTX, org, id string) ([]Allocation, error) {
	rows, e := db.Query(ctx, `SELECT s.project_id,COALESCE((SELECT sum(a.effect*x.cost_amount)
 FROM payment_allocations a JOIN payment_allocation_shares x ON x.org_id=a.org_id AND x.allocation_id=a.id
 WHERE a.org_id=s.org_id AND a.root_id=$2 AND x.project_id IS NOT DISTINCT FROM s.project_id),0)::text
 FROM payment_allocation_shares s WHERE s.org_id=$1 AND s.allocation_id=$2 ORDER BY s.position`, org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Allocation{}
	for rows.Next() {
		var v Allocation
		if e = rows.Scan(&v.ProjectID, &v.Amount); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Store aligned rows even when only one currency contributes a minor unit.
// Reversals can then copy the exact original positions in both currencies.
func pairedSettlementShares(scopes, cash, cost []Allocation) ([]Allocation, []Allocation) {
	key := func(id *string) string {
		if id == nil {
			return "company"
		}
		return *id
	}
	cashBy, costBy := map[string]string{}, map[string]string{}
	for _, v := range cash {
		cashBy[key(v.ProjectID)] = v.Amount
	}
	for _, v := range cost {
		costBy[key(v.ProjectID)] = v.Amount
	}
	paid, incurred := []Allocation{}, []Allocation{}
	for _, scope := range scopes {
		id := key(scope.ProjectID)
		p, c := cashBy[id], costBy[id]
		if p == "" {
			p = "0.000000"
		}
		if c == "" {
			c = "0.000000"
		}
		pv, _ := core.ParseDecimal(p)
		cv, _ := core.ParseDecimal(c)
		if !pv.Positive() && !cv.Positive() {
			continue
		}
		paid = append(paid, Allocation{ProjectID: scope.ProjectID, Amount: p})
		incurred = append(incurred, Allocation{ProjectID: scope.ProjectID, Amount: c})
	}
	return paid, incurred
}
