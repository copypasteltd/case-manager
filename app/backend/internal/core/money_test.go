package core

import "testing"

func TestDecimalPreservesLargeValuesAndReversals(t *testing.T) {
	a, err := ParseDecimal("99999999999999.999999")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseDecimal("0.000001")
	if got := a.Add(b).String(); got != "100000000000000.000000" {
		t.Fatalf("precision lost: %s", got)
	}
	if got := b.Sub(a).String(); got != "-99999999999999.999998" {
		t.Fatalf("incorrect reversal: %s", got)
	}
	if a.String() != "99999999999999.999999" {
		t.Fatal("arithmetic mutated source amount")
	}
	for _, invalid := range []string{"-1", "1e2", "01", "0.0000001", "NaN", "100000000000000", " 1", "1."} {
		if _, err = ParseDecimal(invalid); err == nil {
			t.Errorf("accepted invalid money %q", invalid)
		}
	}
}
func TestCurrencyRejectsUnsupportedPrecision(t *testing.T) {
	for _, test := range []struct {
		currency, amount string
		valid            bool
	}{
		{"JPY", "12", true}, {"JPY", "12.000", true}, {"JPY", "12.01", false},
		{"HKD", "0.10", true}, {"HKD", "0.001", false}, {"BTC", "1", false},
	} {
		_, err := CheckCurrencyAmount(test.currency, test.amount, map[string]int{"JPY": 0, "HKD": 2})
		if (err == nil) != test.valid {
			t.Errorf("unexpected currency precision result for %s %s", test.currency, test.amount)
		}
	}
}
