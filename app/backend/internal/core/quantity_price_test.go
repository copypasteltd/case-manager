package core

import "testing"

func TestQuantityPriceUsesExactProductAndExplicitRounding(t *testing.T) {
	for _, test := range []struct {
		quantity, price string
		exponent        int
		rounding, want  string
	}{
		{"3", "0.335", 2, "half_up", "1.010000"},
		{"3", "0.335", 2, "half_even", "1.000000"},
		{"1", "1.015", 2, "half_even", "1.020000"},
		{"0.000001", "99999999999999", 6, "half_up", "99999999.999999"},
		{"0.1", "0.2", 2, "half_even", "0.020000"},
		{"2.5", "1", 0, "half_even", "2.000000"},
		{"3.5", "1", 0, "half_even", "4.000000"},
		{"5", "0", 2, "half_up", "0.000000"},
	} {
		actual, err := QuantityPrice(test.quantity, test.price, test.exponent, test.rounding)
		if err != nil || actual.String() != test.want {
			t.Fatalf("%s * %s: got %s / %v, want %s", test.quantity, test.price, actual.String(), err, test.want)
		}
	}
}

func TestQuantityPriceRefusesAmbiguousOrUnrepresentableAmounts(t *testing.T) {
	for _, test := range []struct {
		quantity, price string
		exponent        int
		rounding        string
	}{
		{"0", "2", 2, "half_up"}, {"1e3", "2", 2, "half_up"}, {"1", "-1", 2, "half_up"},
		{"1", "1", 2, ""}, {"1", "1", 7, "half_up"}, {"1", "1", -1, "half_even"},
		{"99999999999999", "2", 2, "half_up"}, {"0.0000001", "1", 6, "half_up"},
	} {
		if _, err := QuantityPrice(test.quantity, test.price, test.exponent, test.rounding); err == nil {
			t.Fatalf("accepted unbounded or ambiguous product: %+v", test)
		}
	}
}
