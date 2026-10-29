package finance

import (
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
)

func amortizationCostFixture(currency, amount string, shares []Allocation) Cost {
	return Cost{ID: core.NewID(), Version: 2, CurrentRevisionID: core.NewID(), ContentHash: strings.Repeat("a", 64),
		State: "confirmed", EntryKind: "expense", Content: Content{Currency: &currency, Amount: &amount, Allocations: shares}}
}
func TestAmortizationConservesEveryProjectAndCompanyMinorUnit(t *testing.T) {
	pid := core.NewID()
	for _, fixture := range []struct {
		currency, amount string
		exponent         int
	}{
		{"JPY", "99999999999999", 0}, {"HKD", "0.01", 2}, {"KWD", "0.005", 3}, {"USD", "99999999999999.99", 2},
	} {
		t.Run(fixture.currency+"/"+fixture.amount, func(t *testing.T) {
			c := amortizationCostFixture(fixture.currency, fixture.amount, []Allocation{{ProjectID: &pid, Amount: fixture.amount}})
			plan, err := amortizationSchedule(c, fixture.exponent, AmortizationInput{Method: "monthly_equal", ServiceFrom: "2020-01-01", ServiceTo: "2030-01-01"})
			if err != nil || len(plan.Periods) != 120 {
				t.Fatal(len(plan.Periods), err)
			}
			total := core.Decimal{}
			project := core.Decimal{}
			for i, p := range plan.Periods {
				money, e := core.CheckCurrencyAmount(fixture.currency, p.Amount, map[string]int{fixture.currency: fixture.exponent})
				if e != nil {
					t.Fatal(p, e)
				}
				total = total.Add(money)
				shared := core.Decimal{}
				for _, s := range p.Shares {
					if s.ProjectID == nil || *s.ProjectID != pid {
						t.Fatal("project attribution changed")
					}
					n, e := core.CheckCurrencyAmount(fixture.currency, s.Amount, map[string]int{fixture.currency: fixture.exponent})
					if e != nil {
						t.Fatal(s, e)
					}
					project = project.Add(n)
					shared = shared.Add(n)
				}
				if shared.Cmp(money) != 0 || p.Sequence != i+1 {
					t.Fatal("month conservation", p)
				}
			}
			want, _ := core.ParseDecimal(fixture.amount)
			if total.Cmp(want) != 0 || project.Cmp(want) != 0 {
				t.Fatal("full source not conserved", total.String(), project.String())
			}
		})
	}
	c := amortizationCostFixture("HKD", "0.03", []Allocation{{ProjectID: &pid, Amount: "0.02"}, {Amount: "0.01"}})
	plan, err := amortizationSchedule(c, 2, AmortizationInput{Method: "monthly_equal", ServiceFrom: "2025-01-01", ServiceTo: "2025-03-01"})
	if err != nil || plan.Periods[0].Amount != "0.020000" || plan.Periods[1].Amount != "0.010000" || len(plan.Periods[1].Shares) != 1 {
		t.Fatal(plan, err)
	}
}
func TestAmortizationCalendarUsesExactHalfOpenServiceDays(t *testing.T) {
	for _, v := range []struct {
		start, end, amount string
		expected           []string
	}{
		{"2024-02-01", "2024-04-01", "60", []string{"29.000000", "31.000000"}},
		{"2025-01-31", "2025-03-02", "30", []string{"1.000000", "28.000000", "1.000000"}},
		{"2025-01-01", "2025-01-02", "0.01", []string{"0.010000"}},
	} {
		c := amortizationCostFixture("HKD", v.amount, []Allocation{{Amount: v.amount}})
		out, err := amortizationSchedule(c, 2, AmortizationInput{Method: "daily_weighted", ServiceFrom: v.start, ServiceTo: v.end})
		if err != nil || len(out.Periods) != len(v.expected) {
			t.Fatal(out, err)
		}
		for i, p := range out.Periods {
			if p.Amount != v.expected[i] {
				t.Fatal(p, v.expected)
			}
			end, _ := time.Parse(time.DateOnly, p.ServiceTo)
			if p.RecognizedOn != end.AddDate(0, 0, -1).Format(time.DateOnly) || i > 0 && p.ServiceFrom != out.Periods[i-1].ServiceTo {
				t.Fatal("period gap", p)
			}
		}
	}
}
func TestAmortizationCumulativeRoundingDoesNotCreateNegativeTails(t *testing.T) {
	c := amortizationCostFixture("HKD", "0.01", []Allocation{{Amount: "0.01"}})
	out, err := amortizationSchedule(c, 2, AmortizationInput{Method: "monthly_equal", ServiceFrom: "2020-01-01", ServiceTo: "2030-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range out.Periods {
		want := "0.000000"
		if i == 59 {
			want = "0.010000"
		}
		if p.Amount != want {
			t.Fatal("cumulative threshold", i, p)
		}
	}
}
func TestAmortizationRefusesUnknownRulesInvalidPeriodsAndSourcePrecision(t *testing.T) {
	c := amortizationCostFixture("HKD", "1", []Allocation{{Amount: "1"}})
	for _, in := range []AmortizationInput{
		{Method: "", ServiceFrom: "2025-01-01", ServiceTo: "2025-02-01"},
		{Method: "daily_weighted", ServiceFrom: "2025-02-29", ServiceTo: "2025-03-01"},
		{Method: "monthly_equal", ServiceFrom: "2025-01-01", ServiceTo: "2025-01-01"},
		{Method: "monthly_equal", ServiceFrom: "2020-01-01", ServiceTo: "2030-02-01"},
	} {
		if _, err := amortizationSchedule(c, 2, in); err == nil {
			t.Fatal("unsafe schedule accepted", in)
		}
	}
	in := AmortizationInput{Method: "monthly_equal", ServiceFrom: "2025-01-01", ServiceTo: "2025-03-01"}
	c = amortizationCostFixture("JPY", "0.01", []Allocation{{Amount: "0.01"}})
	if _, err := amortizationSchedule(c, 0, in); err == nil {
		t.Fatal("fractional yen")
	}
	c = amortizationCostFixture("HKD", "1", []Allocation{{Amount: "0.99"}})
	if _, err := amortizationSchedule(c, 2, in); err == nil {
		t.Fatal("missing source share")
	}
}
