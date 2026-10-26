package finance

import "testing"

func TestSubscriptionPriceTimeline(t *testing.T) {
	defs := map[string]int{"HKD": 2, "USD": 2, "JPY": 0}
	text := func(s string) *string { return &s }
	t.Run("mid month repricing leaves original snapshots intact and applies at the next anchor", func(t *testing.T) {
		original := []SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "100", Plan: "original"}}
		next, err := subscriptionAppendPrice(original, SubscriptionPriceInput{EffectiveFrom: "2025-02-15", Currency: "HKD", Amount: "120", Plan: "new"}, defs)
		if err != nil || original[0].EffectiveTo != nil || original[0].Amount != "100" || len(next) != 2 || *next[0].EffectiveTo != "2025-02-15" {
			t.Fatal(original, next, err)
		}
		calendar := SubscriptionCalendar{Cycle: "month", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2025-01-31", AnchorDay: 31}
		periods, err := calendar.Windows("2025-01-01", "2025-03-01")
		if err != nil || len(periods) != 2 || subscriptionPriceAt(next, periods[0].StartDate).Amount != "100.000000" || subscriptionPriceAt(next, periods[1].StartDate).Amount != "120.000000" {
			t.Fatal(periods, next, err)
		}
	})
	t.Run("half open price windows preserve explicit gaps instead of filling them", func(t *testing.T) {
		initial := []SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", EffectiveTo: text("2025-02-01"), Currency: "HKD", Amount: "100"}}
		next, err := subscriptionAppendPrice(initial, SubscriptionPriceInput{EffectiveFrom: "2025-03-01", Currency: "USD", Amount: "20"}, defs)
		if err != nil || subscriptionPriceAt(next, "2025-02-01") != nil || subscriptionPriceAt(next, "2025-02-28") != nil || subscriptionPriceAt(next, "2025-03-01").Currency != "USD" {
			t.Fatal(next, err)
		}
	})
	t.Run("overlap backdated append invalid dates and unconfigured precision are rejected", func(t *testing.T) {
		base := SubscriptionPriceInput{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "1"}
		for _, rows := range [][]SubscriptionPriceInput{
			{base, {EffectiveFrom: "2025-02-01", Currency: "HKD", Amount: "1"}},
			{{EffectiveFrom: "2025-01-01", EffectiveTo: text("2025-01-01"), Currency: "HKD", Amount: "1"}},
			{{EffectiveFrom: "2025-02-30", Currency: "HKD", Amount: "1"}},
			{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "0.001"}},
			{{EffectiveFrom: "2025-01-01", Currency: "JPY", Amount: "1.1"}},
			{{EffectiveFrom: "2025-01-01", Currency: "XXX", Amount: "1"}},
		} {
			if _, err := subscriptionPrices(rows, defs); err == nil {
				t.Fatal("accepted invalid prices", rows)
			}
		}
		if _, err := subscriptionAppendPrice([]SubscriptionPriceInput{base}, base, defs); err == nil {
			t.Fatal("accepted backdated append")
		}
	})
	t.Run("known zero price remains different from an unknown price", func(t *testing.T) {
		prices, err := subscriptionPrices([]SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "0"}}, defs)
		if err != nil || subscriptionPriceAt(prices, "2025-01-01").Amount != "0.000000" || subscriptionPriceAt(prices, "2024-12-31") != nil {
			t.Fatal(prices, err)
		}
	})
}
