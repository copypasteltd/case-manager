package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
)

func TestSubscriptionBilling(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	ptr := func(s string) *string { return &s }
	anchor := 31
	exec := func(t *testing.T, q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	base := func(s finance.Subscription) finance.Base {
		return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "human fixes exact subscription basis"}
	}
	periodBase := func(p finance.SubscriptionPeriod) finance.SubscriptionPeriodBase {
		return finance.SubscriptionPeriodBase{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "human verifies exact billing period"}
	}
	get := func(t *testing.T, id string) finance.Subscription {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/subscriptions/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Subscription](t, v)
	}
	getPeriod := func(t *testing.T, id string) finance.SubscriptionPeriod {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/subscription-periods/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.SubscriptionPeriod](t, v)
	}
	create := func(t *testing.T, in finance.SubscriptionCreate) finance.Subscription {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[finance.Subscription](t, v)
	}
	transition := func(t *testing.T, s finance.Subscription, action string) finance.Subscription {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(s), Action: action, VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		return data[finance.Subscription](t, v)
	}
	generate := func(t *testing.T, s finance.Subscription, from, to string) finance.SubscriptionGeneration {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(s), From: from, To: to}, nil)
		expect(t, r, 200, v)
		return data[finance.SubscriptionGeneration](t, v)
	}
	draft := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic incomplete subscription", VendorState: "unknown", Prices: []finance.SubscriptionPriceInput{}, ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}}, SourceKey: "vendor:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated billing basis"}
	var supplier party.Party
	var manager string
	t.Run("unbound roles protect business data and incomplete drafts do not become charges", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), draft, nil)
		expect(t, r, 404, v)
		manager = bind(f.alice, "subscription_manager")
		for _, key := range []string{"forecast_reporter", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "cash_reporter", "party_manager"} {
			bind(f.alice, key)
		}
		f.aliceToken = memberMFAToken(f, "alice", "android")
		bind(f.bob, "subscription_reader")
		bind(f.bob, "forecast_reporter")
		r, v = f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic subscription vendor", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "fixed vendor billing proof"}, nil)
		expect(t, r, 201, v)
		supplier = data[party.Party](t, v)
		s := create(t, draft)
		if s.State != "draft" || s.NextChargeDate != nil {
			t.Fatal(s)
		}
		r, v = f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(s), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 422, v)
		r, v = f.with(t).request("GET", "/v1/subscriptions/"+s.ID, f.administratorToken(), "", nil, nil)
		expect(t, r, 404, v)
	})
	full := func(category string) finance.SubscriptionCreate {
		v := draft
		v.SourceKey = "vendor:" + core.NewID()
		v.SubscriptionContent = finance.SubscriptionContent{Product: "Synthetic exact recurring product", Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: category, StartDate: ptr("2025-01-01"), Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: ptr("2025-01-31"), AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0, 7}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "100", Plan: "Synthetic monthly plan"}}}
		return v
	}
	var sub finance.Subscription
	var periods []finance.SubscriptionPeriod
	t.Run("permanent source idempotence and exact revisions reject stale edits", func(t *testing.T) {
		in := full("billing-main")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		sub = data[finance.Subscription](t, v)
		r, v = f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[finance.Subscription](t, v).ID != sub.ID {
			t.Fatal("replay changed identity")
		}
		r, v = f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		before := sub
		in.Notes = "Retained exact billing revision"
		r, v = f.with(t).request("PATCH", "/v1/subscriptions/"+sub.ID, f.aliceToken, core.NewID(), finance.SubscriptionPatch{SubscriptionContent: in.SubscriptionContent, Base: base(sub)}, nil)
		expect(t, r, 200, v)
		sub = data[finance.Subscription](t, v)
		r, v = f.with(t).request("PATCH", "/v1/subscriptions/"+sub.ID, f.aliceToken, core.NewID(), finance.SubscriptionPatch{SubscriptionContent: in.SubscriptionContent, Base: base(before)}, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/subscriptions/"+sub.ID+"/revisions/"+before.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.SubscriptionRevision](t, v).Notes != "" {
			t.Fatal("history rewritten")
		}
		sub = transition(t, sub, "activate")
		if sub.State != "active" {
			t.Fatal(sub)
		}
		changed := sub.SubscriptionContent
		newAnchor := 30
		changed.AnchorDay = &newAnchor
		r, v = f.with(t).request("PATCH", "/v1/subscriptions/"+sub.ID, f.aliceToken, core.NewID(), finance.SubscriptionPatch{SubscriptionContent: changed, Base: base(sub)}, nil)
		if r.Code != 409 && r.Code != 422 {
			t.Fatal(r.Code, v)
		}
	})
	t.Run("SQL and application calendars preserve month end leap years and custom natural days", func(t *testing.T) {
		for _, tc := range []struct {
			first, cycle, start, end string
			anchor, days             int
		}{{"2024-01-31", "month", "2024-02-29", "2024-03-31", 31, 0}, {"2024-02-29", "year", "2025-02-28", "2026-02-28", 29, 0}, {"2024-02-29", "year", "2028-02-29", "2029-02-28", 29, 0}, {"2024-03-09", "custom", "2024-03-10", "2024-03-11", 0, 1}} {
			payload, _ := json.Marshal(map[string]any{"first_charge_date": tc.first, "cycle": tc.cycle, "anchor_day": tc.anchor, "custom_interval_days": tc.days})
			var end string
			if e := f.api.DB.QueryRow(ctx, "SELECT subscription_window_end($1::jsonb,$2::date)::text", payload, tc.start).Scan(&end); e != nil || end != tc.end {
				t.Fatal(tc, end, e)
			}
		}
	})
	t.Run("generation is unique and produces forecasts without actual cost or cash", func(t *testing.T) {
		result := generate(t, sub, "2025-01-01", "2025-04-01")
		if len(result.GeneratedIDs) != 3 || !result.CoverageComplete {
			t.Fatal(result)
		}
		for _, id := range result.GeneratedIDs {
			periods = append(periods, getPeriod(t, id))
		}
		if periods[1].StartDate != "2025-02-28" || periods[1].EndDate != "2025-03-31" {
			t.Fatal(periods)
		}
		sub = get(t, sub.ID)
		again := generate(t, sub, "2025-01-01", "2025-04-01")
		if len(again.GeneratedIDs) != 0 || again.ExistingCount != 3 {
			t.Fatal(again)
		}
		var costs, payments int
		if e := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM actual_cost_entries),(SELECT count(*) FROM payment_records)").Scan(&costs, &payments); e != nil || costs != 0 || payments != 0 {
			t.Fatal(costs, payments, e)
		}
		r, v := f.with(t).request("GET", "/v1/reports/forecast?from=2025-01-01&to=2025-04-01&category=billing-main", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report := data[finance.ForecastReport](t, v)
		if !report.CoverageComplete || len(report.Totals) != 1 || report.Totals[0].ExpectedAmount != "300.000000" {
			t.Fatal(report)
		}
	})
	t.Run("missing forecasts and price gaps are explicit and zero pricing is known", func(t *testing.T) {
		in := full("billing-gap")
		in.Prices[0].EffectiveTo = ptr("2025-02-01")
		s := transition(t, create(t, in), "activate")
		result := generate(t, s, "2025-01-01", "2025-04-01")
		if len(result.GeneratedIDs) != 1 || result.CoverageComplete || len(result.MissingPriceDates) != 2 {
			t.Fatal(result)
		}
		r, v := f.with(t).request("GET", "/v1/reports/forecast?from=2025-01-01&to=2025-04-01&category=billing-gap", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report := data[finance.ForecastReport](t, v)
		if report.CoverageComplete || report.MissingPriceCount != 2 || report.MissingPeriodCount != 2 {
			t.Fatal(report)
		}
		in = full("billing-zero")
		in.Prices[0].Amount = "0"
		s = transition(t, create(t, in), "activate")
		result = generate(t, s, "2025-01-01", "2025-02-01")
		p := getPeriod(t, result.GeneratedIDs[0])
		if p.Amount != "0.000000" || p.ActualAmount != nil {
			t.Fatal(p)
		}
	})
	t.Run("future repricing preserves stored forecasts and explicit reforecast keeps history", func(t *testing.T) {
		s := transition(t, create(t, full("billing-reprice")), "activate")
		zone, _ := time.LoadLocation("Asia/Hong_Kong")
		now := time.Now().In(zone)
		month := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		from := month.Format(time.DateOnly)
		to := month.AddDate(0, 3, 0).Format(time.DateOnly)
		generated := generate(t, s, from, to)
		p := getPeriod(t, generated.GeneratedIDs[0])
		old := p
		s = get(t, s.ID)
		effective := month.AddDate(0, 0, 14).Format(time.DateOnly)
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/prices", f.aliceToken, core.NewID(), finance.SubscriptionPriceChange{Base: base(s), SubscriptionPriceInput: finance.SubscriptionPriceInput{EffectiveFrom: effective, Currency: "HKD", Amount: "120", Plan: "New fixed plan"}}, nil)
		expect(t, r, 200, v)
		s = data[finance.Subscription](t, v)
		if p = getPeriod(t, p.ID); p.Amount != "100.000000" || p.CurrentRevisionID != old.CurrentRevisionID {
			t.Fatal(p)
		}
		r, v = f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/reforecast", f.aliceToken, core.NewID(), finance.SubscriptionPeriodReforecast{SubscriptionPeriodBase: periodBase(p), ConfigurationRevisionID: s.CurrentRevisionID, ConfigurationHash: s.ContentHash}, nil)
		expect(t, r, 200, v)
		p = data[finance.SubscriptionPeriod](t, v)
		if p.Amount != "120.000000" || p.CurrentRevisionID == old.CurrentRevisionID {
			t.Fatal(p)
		}
		r, v = f.with(t).request("GET", "/v1/subscription-periods/"+p.ID+"/revisions", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		history := data[core.Page[finance.SubscriptionPeriodRevision]](t, v)
		if len(history.Items) != 2 || history.Items[1].Amount != "100.000000" {
			t.Fatal(history)
		}
	})
	newCost := func(t *testing.T, amount string) finance.Cost {
		t.Helper()
		in := finance.Create{Content: finance.Content{Title: "Synthetic actual subscription charge", OccurredOn: ptr("2025-01-31"), Currency: ptr("HKD"), Amount: &amount, Category: "billing-main", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "charge:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "real economic cost fixture"}
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		c := data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "fixed actual supplier charge"}, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	linkInput := func(p finance.SubscriptionPeriod, c finance.Cost) finance.SubscriptionPeriodLink {
		return finance.SubscriptionPeriodLink{SubscriptionPeriodBase: periodBase(p), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash}
	}
	link := func(t *testing.T, p finance.SubscriptionPeriod, c finance.Cost) finance.SubscriptionPeriod {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/costs", f.aliceToken, core.NewID(), linkInput(p, c), nil)
		expect(t, r, 200, v)
		return data[finance.SubscriptionPeriod](t, v)
	}
	reviewInput := func(p finance.SubscriptionPeriod, ack bool) finance.SubscriptionPeriodDecision {
		return finance.SubscriptionPeriodDecision{SubscriptionPeriodBase: periodBase(p), ExpectedActualFingerprint: &p.ActualFingerprint, Action: "confirm", AcknowledgeDifference: ack, EvidenceVersionIDs: []string{file.Version.ID}}
	}
	var second finance.Cost
	var reviewed finance.SubscriptionPeriod
	t.Run("partial and excess actual charges retain forecast and need explicit fixed review", func(t *testing.T) {
		p := link(t, periods[0], newCost(t, "40"))
		if p.Amount != "100.000000" || p.ActualAmount == nil || *p.ActualAmount != "40.000000" || !slices.Contains(p.Anomalies, "partial_charge") {
			t.Fatal(p)
		}
		r, v := f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/review", f.aliceToken, core.NewID(), reviewInput(p, false), nil)
		expect(t, r, 422, v)
		r, v = f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/review", f.aliceToken, core.NewID(), reviewInput(p, true), nil)
		expect(t, r, 200, v)
		p = data[finance.SubscriptionPeriod](t, v)
		if !p.ReviewCurrent {
			t.Fatal(p)
		}
		second = newCost(t, "80")
		p = link(t, p, second)
		if p.ReviewCurrent || p.ActualAmount == nil || *p.ActualAmount != "120.000000" || !slices.Contains(p.Anomalies, "overcharge") {
			t.Fatal(p)
		}
		in := reviewInput(p, true)
		in.InvoiceMissing = true
		r, v = f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/review", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, v)
		reviewed = data[finance.SubscriptionPeriod](t, v)
		if reviewed.Amount != "100.000000" || !reviewed.ReviewCurrent || !slices.Contains(reviewed.Anomalies, "invoice_missing") {
			t.Fatal(reviewed)
		}
		r, v = f.with(t).request("GET", "/v1/reports/forecast?from=2025-01-01&to=2025-02-01&category=billing-main", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.ForecastReport](t, v).Totals[0].ExpectedAmount != "100.000000" {
			t.Fatal(v)
		}
		r, v = f.with(t).request("GET", "/v1/reports/expenses?from=2025-01-01&to=2025-02-01&category=billing-main", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.Report](t, v).Totals[0].NetAmount != "120.000000" {
			t.Fatal(v)
		}
	})
	t.Run("one canonical cost cannot be attributed twice and reverse invalidates exact review", func(t *testing.T) {
		r, v := f.with(t).request("GET", "/v1/costs/"+second.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		second = data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/subscription-periods/"+periods[1].ID+"/costs", f.aliceToken, core.NewID(), linkInput(periods[1], second), nil)
		expect(t, r, 409, v)
		oldReview := reviewInput(reviewed, true)
		r, v = f.with(t).request("POST", "/v1/costs/"+second.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: finance.Base{ExpectedVersion: second.Version, RevisionID: second.CurrentRevisionID, ContentHash: second.ContentHash, Reason: "correct actual reversed charge"}, EffectiveOn: "2025-02-01"}, nil)
		expect(t, r, 200, v)
		p := getPeriod(t, reviewed.ID)
		if p.Version != reviewed.Version || p.ReviewCurrent || p.State != "awaiting_confirmation" || p.ActualFingerprint == reviewed.ActualFingerprint || !slices.Contains(p.Anomalies, "review_stale") {
			t.Fatal(p)
		}
		r, v = f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/review", f.aliceToken, core.NewID(), oldReview, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/subscription-periods/"+p.ID+"/events?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		events := data[core.Page[finance.SubscriptionPeriodEvent]](t, v)
		if len(events.Items[0].CostSnapshot) != 2 || events.Items[0].CostSnapshot[0].State != "confirmed" || events.Items[0].CostSnapshot[1].State != "confirmed" {
			t.Fatal(events)
		}
	})
	t.Run("history lists and aggregates require every linked actual cost permission", func(t *testing.T) {
		for _, route := range []string{"/subscriptions/" + sub.ID, "/subscriptions/" + sub.ID + "/revisions", "/subscription-periods/" + reviewed.ID} {
			r, v := f.with(t).request("GET", "/v1"+route, f.bobToken, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v := f.with(t).request("GET", "/v1/reports/forecast?from=2025-01-01&to=2025-02-01&category=billing-main", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[finance.ForecastReport](t, v).Totals) != 0 {
			t.Fatal("hidden costs leaked forecast", v)
		}
		bind(f.bob, "cost_reader")
		r, v = f.with(t).request("GET", "/v1/subscription-periods/"+reviewed.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.SubscriptionPeriod](t, v).CanManage {
			t.Fatal("reader can write")
		}
	})
	t.Run("paused calendars do not backfill stopped anchors and cancellation has separate vendor proof", func(t *testing.T) {
		s := transition(t, create(t, full("billing-stop")), "activate")
		s = transition(t, s, "pause")
		if !slices.Contains(s.Signals, "vendor_stop_unverified") {
			t.Fatal(s)
		}
		today := time.Now().In(mustLocation(t, "Asia/Hong_Kong")).Format(time.DateOnly)
		calendar := finance.SubscriptionCalendar{Cycle: "month", Timezone: s.Timezone, FirstChargeDate: *s.FirstChargeDate, AnchorDay: 31}
		w, e := calendar.Next(today)
		if e != nil {
			t.Fatal(e)
		}
		var eligible bool
		if e = f.owner.QueryRow(ctx, "SELECT subscription_charge_eligible($1,$2,$3::date)", f.org, s.ID, w.StartDate).Scan(&eligible); e != nil || eligible {
			t.Fatal("paused future anchor became eligible", eligible, e)
		}
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/periods/unexpected", f.aliceToken, core.NewID(), finance.SubscriptionUnexpectedPeriod{Base: base(s), StartDate: "2025-01-31"}, nil)
		expect(t, r, 201, v)
		s = get(t, s.ID)
		s = transition(t, s, "resume")
		s = transition(t, s, "cancel")
		if s.VendorState != "unknown" || s.CancelEffectiveOn == nil {
			t.Fatal(s)
		}
		r, v = f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(s), Action: "archive", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 409, v)
		result := generate(t, s, w.StartDate, w.EndDate)
		if len(result.GeneratedIDs) != 0 {
			t.Fatal("cancelled charge regenerated", result)
		}
	})
	t.Run("forecast drilldown excludes waived and stopped predictions without hiding their historical periods", func(t *testing.T) {
		s := transition(t, create(t, full("billing-drill")), "activate")
		from := time.Now().In(mustLocation(t, "Asia/Hong_Kong")).Format(time.DateOnly)
		to := time.Now().In(mustLocation(t, "Asia/Hong_Kong")).AddDate(0, 3, 0).Format(time.DateOnly)
		generated := generate(t, s, from, to)
		if len(generated.GeneratedIDs) < 2 {
			t.Fatal(generated)
		}
		p := getPeriod(t, generated.GeneratedIDs[0])
		r, v := f.with(t).request("POST", "/v1/subscription-periods/"+p.ID+"/review", f.aliceToken, core.NewID(), finance.SubscriptionPeriodDecision{SubscriptionPeriodBase: periodBase(p), Action: "waive", EvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		query := "?from=" + from + "&to=" + to + "&category=billing-drill&forecast_only=true"
		r, v = f.with(t).request("GET", "/v1/subscription-periods"+query, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.SubscriptionPeriod]](t, v).Items) != len(generated.GeneratedIDs)-1 {
			t.Fatal("waived prediction remained in report drilldown", v)
		}
		r, v = f.with(t).request("GET", "/v1/subscription-periods"+query+"&q=nonexistent-product", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.SubscriptionPeriod]](t, v).Items) != 0 {
			t.Fatal("product search was ignored")
		}
		s = transition(t, get(t, s.ID), "pause")
		r, v = f.with(t).request("GET", "/v1/subscription-periods"+query, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.SubscriptionPeriod]](t, v).Items) != 0 {
			t.Fatal("paused future prediction remained in report drilldown")
		}
		r, v = f.with(t).request("GET", "/v1/subscriptions/"+s.ID+"/periods", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.SubscriptionPeriod]](t, v).Items) != len(generated.GeneratedIDs) {
			t.Fatal("normal history lost stopped predictions")
		}
	})
	t.Run("restoring an archive preserves its exact cancellation and only reopens historical reconciliation", func(t *testing.T) {
		s := transition(t, create(t, full("billing-restore")), "activate")
		s = transition(t, s, "cancel")
		checked := time.Now().UTC().Format(time.RFC3339)
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(s), Action: "verify_vendor", VendorState: "cancelled", VendorCheckedAt: &checked, VendorEvidenceVersionIDs: []string{file.Version.ID}}, nil)
		expect(t, r, 200, v)
		s = transition(t, data[finance.Subscription](t, v), "archive")
		archived := s
		s = transition(t, s, "restore")
		if s.State != "cancelled" || s.CurrentRevisionID != archived.CurrentRevisionID || s.CancelEffectiveOn == nil || *s.CancelEffectiveOn != *archived.CancelEffectiveOn || s.Version != archived.Version+1 {
			t.Fatal(s)
		}
		r, v = f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(s), Action: "resume", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 409, v)
	})
	t.Run("concurrent generation has one head successor and audit failure rolls everything back", func(t *testing.T) {
		s := transition(t, create(t, full("billing-race")), "activate")
		start := make(chan struct{})
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, _ := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(s), From: "2025-01-01", To: "2025-04-01"}, nil)
				codes <- r.Code
			}()
		}
		close(start)
		wg.Wait()
		close(codes)
		got := []int{}
		for code := range codes {
			got = append(got, code)
		}
		slices.Sort(got)
		if !slices.Equal(got, []int{200, 409}) {
			t.Fatal(got)
		}
		s = get(t, s.ID)
		exec(t, `CREATE FUNCTION fail_subscription_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='subscription.periods.generated' THEN RAISE EXCEPTION 'isolated subscription audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_subscription_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_subscription_audit()`)
		defer exec(t, "DROP TRIGGER IF EXISTS fail_subscription_audit ON audit_events; DROP FUNCTION IF EXISTS fail_subscription_audit()")
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(s), From: "2025-04-01", To: "2025-05-01"}, nil)
		expect(t, r, 500, v)
		if get(t, s.ID).Version != s.Version {
			t.Fatal("failed audit changed head")
		}
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM subscription_periods WHERE subscription_id=$1", s.ID).Scan(&count); e != nil || count != 3 {
			t.Fatal(count, e)
		}
	})
	t.Run("SQL and current human session guards reject history rewrites forged MFA and detached heads", func(t *testing.T) {
		for _, q := range []string{"UPDATE subscription_revisions SET reason='rewrite' WHERE id=$1", "DELETE FROM subscription_prices WHERE revision_id=$1", "DELETE FROM subscription_revision_files WHERE revision_id=$1"} {
			if _, e := f.api.DB.Exec(ctx, q, sub.CurrentRevisionID); e == nil {
				t.Fatal("mutable subscription fact", q)
			}
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE subscriptions SET version=version+1 WHERE id=$1", sub.ID); e == nil {
			t.Fatal("detached subscription head accepted")
		}
		a, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO subscription_events(id,org_id,subscription_id,subscription_version,revision_id,kind,state,actor_id,session_id,mfa_verified_at,reason) SELECT $1,$2,$3,$4,$5,'periods_generated','active',$6,id,mfa_verified_at+interval '1 second','forged MFA' FROM sessions WHERE id=$7`, core.NewID(), f.org, sub.ID, get(t, sub.ID).Version+1, sub.CurrentRevisionID, f.alice, a.SessionID)
		tx.Rollback(ctx)
		if e == nil || !strings.Contains(e.Error(), "actual human session") {
			t.Fatal(e)
		}
		oldMFA := a.MFAVerifiedAt
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE id=$1", a.SessionID)
		defer exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", a.SessionID, oldMFA)
		r, v := f.with(t).request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(get(t, sub.ID)), Action: "pause", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 403, v)
	})
	t.Run("revoked manager cannot replay a formerly successful creation", func(t *testing.T) {
		in := full("billing-revoked")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		exec(t, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", manager)
		r, v = f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, key, in, nil)
		expect(t, r, 404, v)
	})
}

func mustLocation(t *testing.T, zone string) *time.Location {
	t.Helper()
	v, e := time.LoadLocation(zone)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
