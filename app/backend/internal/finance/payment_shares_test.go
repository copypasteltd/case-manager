package finance

import (
	"copypaste.hk/platform/internal/core"
	"testing"
)

func TestPaymentShareRounding(t *testing.T) {
	t.Run("successive cents consume each remaining project exactly", func(t *testing.T) {
		a, b := "a", "b"
		remaining := []Allocation{{ProjectID: &a, Amount: "0.01"}, {ProjectID: &b, Amount: "0.01"}, {ProjectID: nil, Amount: "0.01"}}
		got := map[string]int{}
		for i := 0; i < 3; i++ {
			out, err := splitPaymentShares("0.01", remaining, 2)
			if err != nil || len(out) != 1 {
				t.Fatal(out, err)
			}
			key := value(out[0].ProjectID)
			got[key]++
			for j := range remaining {
				if value(remaining[j].ProjectID) == key {
					remaining[j].Amount = "0"
				}
			}
		}
		if got["a"] != 1 || got["b"] != 1 || got[""] != 1 {
			t.Fatal(got)
		}
		if _, err := splitPaymentShares("0.01", remaining, 2); core.PublicError(err).Code != "SETTLEMENT_BALANCE_EXCEEDED" {
			t.Fatal(err)
		}
	})
	t.Run("large amounts preserve all six fractional digits without floating point", func(t *testing.T) {
		a, b := "a", "b"
		out, err := splitPaymentShares("99999999999999.999999", []Allocation{{ProjectID: &a, Amount: "50000000000000"}, {ProjectID: &b, Amount: "49999999999999.999999"}}, 6)
		if err != nil || len(out) != 2 || out[0].Amount != "50000000000000.000000" || out[1].Amount != "49999999999999.999999" {
			t.Fatal(out, err)
		}
	})
	t.Run("unconfigured precision and fractional minor units are rejected", func(t *testing.T) {
		for _, input := range []struct {
			amount string
			exp    int
		}{{"0.001", 2}, {"0.1", 0}, {"0", 2}} {
			if _, err := splitPaymentShares(input.amount, []Allocation{{Amount: "1"}}, input.exp); err == nil {
				t.Fatal(input)
			}
		}
	})
}
