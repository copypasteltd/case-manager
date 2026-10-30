package finance

import "testing"

func TestActualRefundQuote(t *testing.T) {
	q := PaymentRefundFX{OriginalCurrency: "HKD", OriginalAmount: "780", Rate: "0.1282051282", Source: "actual synthetic bank statement"}
	for _, v := range []struct {
		principal, cash, rate string
		exponent              int
	}{
		{"780", "100", q.Rate, 2}, {"1", "1", "0.5", 0}, {"0.01", "0.01", "0.5", 2},
		{"99999999999999.99", "99999999999999.99", "1", 2},
	} {
		q.Rate = v.rate
		if err := checkRefundQuote(q, v.principal, v.cash, v.exponent); err != nil {
			t.Fatal(v, err)
		}
	}
	for _, v := range []struct{ rate, source, cash string }{
		{"0", "bank", "100"}, {"-1", "bank", "100"}, {"1e-1", "bank", "100"},
		{"0.12", "bank", "100"}, {"0.12820512820", "bank", "100"},
		{"0.1282051282", "bank\u200b", "100"}, {"0.1282051282", "", "100"}, {"0.1282051282", "bank", "0"},
	} {
		q.Rate, q.Source = v.rate, v.source
		if checkRefundQuote(q, "780", v.cash, 2) == nil {
			t.Fatal("invalid quote accepted", v)
		}
	}
}
