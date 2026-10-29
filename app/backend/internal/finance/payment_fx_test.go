package finance

import (
	"math/big"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestCrossCurrencySettlementMath(t *testing.T) {
	t.Run("dual balances use their own currencies", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "780.000000", RemainingCostAmount: "100.000000", PaymentCurrency: "HKD", CostCurrency: "USD", CostExponent: 2}
		v, e := inverseCostAmount(source, "390")
		if e != nil || v != "50.000000" {
			t.Fatalf("cost units %s: %v", v, e)
		}
		v, e = inverseCostAmount(source, "780")
		if e != nil || v != "100.000000" {
			t.Fatalf("final units %s: %v", v, e)
		}
	})
	t.Run("sequential partial releases conserve both currencies", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "100.000000", RemainingCostAmount: "3.000000", PaymentCurrency: "USD", CostCurrency: "JPY", CostExponent: 0}
		for _, amount := range []string{"33.33", "33.33", "33.34"} {
			v, e := inverseCostAmount(source, amount)
			if e != nil || v != "1.000000" {
				t.Fatalf("release %s gives %s: %v", amount, v, e)
			}
			paid, _ := core.ParseDecimal(source.RemainingAmount)
			d, _ := core.ParseDecimal(amount)
			cost, _ := core.ParseDecimal(source.RemainingCostAmount)
			c, _ := core.ParseDecimal(v)
			source.RemainingAmount = paid.Sub(d).String()
			source.RemainingCostAmount = cost.Sub(c).String()
		}
		if source.RemainingAmount != "0.000000" || source.RemainingCostAmount != "0.000000" {
			t.Fatal(source)
		}
	})
	t.Run("zero rounded cost units stay explicit until the final cash release", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "100.000000", RemainingCostAmount: "1.000000", PaymentCurrency: "HKD", CostCurrency: "JPY", CostExponent: 0}
		v, e := inverseCostAmount(source, "1")
		if e != nil || v != "0.000000" {
			t.Fatalf("small release %s: %v", v, e)
		}
		source.RemainingAmount = "99.000000"
		v, e = inverseCostAmount(source, "99")
		if e != nil || v != "1.000000" {
			t.Fatalf("final release %s: %v", v, e)
		}
	})
	t.Run("rational rounding distinguishes either side of a distant half", func(t *testing.T) {
		for _, v := range []struct{ ratio, want string }{{"4999999999999999999/10000000000000000000", "0.000000"}, {"5000000000000000001/10000000000000000000", "1.000000"}} {
			r, _ := new(big.Rat).SetString(v.ratio)
			got, e := convertFXRational("1", r, 0)
			if e != nil || got != v.want {
				t.Fatalf("%s: %s %v", v.ratio, got, e)
			}
		}
	})
	t.Run("large exact balances do not pass through floating point", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "99999999999999.999999", RemainingCostAmount: "99999999999999.999999", PaymentCurrency: "USD", CostCurrency: "HKD", CostExponent: 6}
		v, e := inverseCostAmount(source, "99999999999999.999999")
		if e != nil || v != source.RemainingCostAmount {
			t.Fatalf("%s %v", v, e)
		}
	})
	t.Run("invalid and exhausted cash inputs fail", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "1.000000", RemainingCostAmount: "1.000000", PaymentCurrency: "HKD", CostCurrency: "USD", CostExponent: 2}
		for _, v := range []string{"0", "-1", "NaN", "1.000001"} {
			if _, e := inverseCostAmount(source, v); e == nil {
				t.Fatalf("accepted %s", v)
			}
		}
	})
	t.Run("cross currency inverse requires the explicitly reviewed cost amount", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "780.000000", RemainingCostAmount: "100.000000", PaymentCurrency: "HKD", CostCurrency: "USD", CostExponent: 2}
		wrong, right := "49.99", "50"
		if _, e := requireInverseCostAmount(source, "390", nil); e == nil {
			t.Fatal("missing review accepted")
		}
		if _, e := requireInverseCostAmount(source, "390", &wrong); e == nil {
			t.Fatal("stale amount accepted")
		}
		if v, e := requireInverseCostAmount(source, "390", &right); e != nil || v != "50.000000" {
			t.Fatalf("%s %v", v, e)
		}
	})
	t.Run("same currency keeps its original protocol and exact amount", func(t *testing.T) {
		source := PaymentAllocation{RemainingAmount: "100.000000", RemainingCostAmount: "100.000000", PaymentCurrency: "HKD", CostCurrency: "HKD", CostExponent: 2}
		if v, e := requireInverseCostAmount(source, "20.1", nil); e != nil || v != "20.100000" {
			t.Fatalf("%s %v", v, e)
		}
	})
	t.Run("cash shares follow cost weights without mixing their unit capacities", func(t *testing.T) {
		project := "project-a"
		weights := []Allocation{{ProjectID: &project, Amount: "4.000000"}, {Amount: "6.000000"}}
		v, e := splitSettlementShares("78", weights, 2, false)
		if e != nil || len(v) != 2 || v[0].Amount != "31.200000" || v[1].Amount != "46.800000" {
			t.Fatalf("%+v %v", v, e)
		}
	})
	t.Run("one cash minor unit preserves cost-only project rows", func(t *testing.T) {
		a, b := "project-a", "project-b"
		weights := []Allocation{{ProjectID: &a, Amount: "0.010000"}, {ProjectID: &b, Amount: "0.010000"}, {Amount: "0.010000"}}
		v, e := splitSettlementShares("1", weights, 0, false)
		if e != nil || len(v) != 1 || v[0].ProjectID != &a || v[0].Amount != "1.000000" {
			t.Fatalf("%+v %v", v, e)
		}
		cash, cost := pairedSettlementShares(weights, v, weights)
		if len(cash) != 3 || len(cost) != 3 || cash[1].Amount != "0.000000" || cash[2].Amount != "0.000000" || cost[2].Amount != "0.010000" {
			t.Fatalf("%+v %+v", cash, cost)
		}
	})
	t.Run("unavailable or invalid weight sets fail", func(t *testing.T) {
		for _, v := range [][]Allocation{nil, {{Amount: "0"}}, {{Amount: "-1"}}} {
			if _, e := splitSettlementShares("1", v, 2, false); e == nil {
				t.Fatalf("accepted %+v", v)
			}
		}
	})
}
