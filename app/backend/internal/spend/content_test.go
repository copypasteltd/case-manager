package spend

import "testing"

func TestCommercialAmountsKeepTaxRoundingAndCashSeparate(t *testing.T) {
	p := func(v string) *string { return &v }
	purchase := Content{Currency: p("HKD"), PricingBasis: "net_plus_tax", RoundingMode: "half_up", Lines: []Line{{ID: "line", Quantity: p("3"), UnitPrice: p("0.335"), TaxAmount: p("0.02"), DiscountAmount: p("0.01")}}}
	_, amount, employee, err := calculate("purchase", purchase, 2)
	if err != nil || amount == nil || *amount != "1.020000" || employee != nil {
		t.Fatal(amount, employee, err)
	}
	purchase.RoundingMode = "half_even"
	_, amount, _, err = calculate("purchase", purchase, 2)
	if err != nil || *amount != "1.010000" {
		t.Fatal(amount, err)
	}
	purchase.PricingBasis = "gross_includes_tax"
	_, amount, _, err = calculate("purchase", purchase, 2)
	if err != nil || *amount != "0.990000" {
		t.Fatal(amount, err)
	}
	claim := Content{Currency: p("HKD"), PricingBasis: "gross_includes_tax", RoundingMode: "half_up", Lines: []Line{{ID: "employee", DeclaredAmount: p("12.34"), TaxAmount: p("0.34"), Funding: "employee_advance"}, {ID: "company", DeclaredAmount: p("20"), Funding: "company_paid"}}}
	_, amount, employee, err = calculate("claim", claim, 2)
	if err != nil || *amount != "32.340000" || *employee != "12.340000" {
		t.Fatal(amount, employee, err)
	}
	claim.Lines[1].Funding = ""
	_, amount, employee, err = calculate("claim", claim, 2)
	if err != nil || amount == nil || employee != nil {
		t.Fatal(amount, employee, err)
	}
	claim.Lines[0].DeclaredAmount = nil
	_, amount, employee, err = calculate("claim", claim, 2)
	if err != nil || amount != nil || employee != nil {
		t.Fatal(amount, employee, err)
	}
}
func TestCommercialTotalsRejectOverDiscountAndOverflow(t *testing.T) {
	p := func(v string) *string { return &v }
	for _, lines := range [][]Line{
		{{Quantity: p("1"), UnitPrice: p("1"), DiscountAmount: p("2")}},
		{{Quantity: p("1"), UnitPrice: p("99999999999999")}, {Quantity: p("1"), UnitPrice: p("1")}},
		{{Quantity: p("1"), UnitPrice: p("1"), TaxAmount: p("2")}},
	} {
		if _, _, _, err := calculate("purchase", Content{Currency: p("HKD"), PricingBasis: "gross_includes_tax", RoundingMode: "half_up", Lines: lines}, 2); err == nil {
			t.Fatal("accepted invalid commercial amount")
		}
	}
}
