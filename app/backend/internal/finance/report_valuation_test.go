package finance

import "testing"

func TestExactFXArithmetic(t *testing.T) {
	for _, v := range []struct {
		amount, rate string
		exponent     int
		want         string
	}{
		{"0.005", "1", 2, "0.010000"}, {"-0.005", "1", 2, "-0.010000"}, {"0.004999", "1", 2, "0.000000"},
		{"99999999999999.999999", "9999999999.9999999999", 6, "999999999999999999980000.000000"},
		{"10", "7.8123456789", 2, "78.120000"}, {"-0.5", "1", 0, "-1.000000"},
	} {
		got, err := convertFXAmount(v.amount, v.rate, v.exponent)
		if err != nil || got != v.want {
			t.Errorf("%s * %s @ %d = %s, want %s (%v)", v.amount, v.rate, v.exponent, got, v.want, err)
		}
	}
	amount, currency := "-10.000000", "USD"
	book := FXContent{BaseCurrency: "HKD", Rates: []FXRate{{Currency: "USD", ValidFrom: "2025-01-01", ValidTo: "2025-02-01", RateDate: "2024-12-31", Rate: "7.8", Source: "human attested quote"}}}
	source := ReportSource{State: "confirmed", Currency: &currency, ContributionAmount: &amount}
	a, e := valueSource(source, book, 2, "2025-01-31")
	if e != nil || a.Status != "converted" || *a.BaseAmount != "-78.000000" || *a.RateDate != "2024-12-31" {
		t.Fatal(a, e)
	}
	a, e = valueSource(source, book, 2, "2025-02-01")
	if e != nil || a.Status != "missing_rate" || a.BaseAmount != nil {
		t.Fatal("exclusive interval silently extended", a, e)
	}
	source.State = "draft"
	a, e = valueSource(source, book, 2, "2025-01-31")
	if e != nil || a.Status != "unconfirmed" || a.BaseAmount != nil {
		t.Fatal("draft valued", a, e)
	}
}
