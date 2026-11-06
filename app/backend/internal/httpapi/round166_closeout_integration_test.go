package httpapi

import (
	"context"
	"os"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
)

func TestRound166CloseoutCalendar(t *testing.T) { round166CloseoutCalendar(t, false) }
func TestRound166CloseoutBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_ROUND166") != "1" {
		t.Skip("explicit owned browser fixture only")
	}
	round166CloseoutCalendar(t, true)
}
func round166CloseoutCalendar(t *testing.T, browser bool) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(s string) *string { return &s }
	for _, role := range []string{"subscription_manager", "party_manager", "project_closeout_reader"} {
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, role); err != nil {
			t.Fatal(err)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	call := func(method, path string, body any, status int) response {
		t.Helper()
		r, v := f.with(t).request(method, path, f.aliceToken, core.NewID(), body, nil)
		expect(t, r, status, v)
		return v
	}
	supplier := data[party.Party](t, call("POST", "/v1/parties", party.Create{Content: party.Content{Name: "Synthetic calendar supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated calendar basis"}, 201))
	checked := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	one, cycle := 1, "custom"
	ids := []string{}
	different := false
	for _, zone := range []string{"Etc/GMT+12", "Pacific/Kiritimati"} {
		var today, tomorrow, hk string
		if err := f.owner.QueryRow(ctx, `SELECT (statement_timestamp() AT TIME ZONE $1)::date::text,((statement_timestamp() AT TIME ZONE $1)::date+1)::text,(statement_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text`, zone).Scan(&today, &tomorrow, &hk); err != nil {
			t.Fatal(err)
		}
		different = different || today != hk
		for _, future := range []bool{false, true} {
			date, label := today, "合成订阅 · 当地今日已生效"
			if future {
				date, label = tomorrow, "合成订阅 · 当地明日待生效"
			}
			content := finance.SubscriptionContent{Product: label, Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: "round166", StartDate: &today, FirstChargeDate: &today, Cycle: &cycle, CustomIntervalDays: &one, Timezone: zone, RenewalMode: "manual", VendorState: "unknown", ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: today, Currency: "HKD", Amount: "1", Plan: "Synthetic calendar"}}}
			sub := data[finance.Subscription](t, call("POST", "/v1/subscriptions", finance.SubscriptionCreate{SubscriptionContent: content, SourceKey: "round166:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "ordinary synthetic draft"}, 201))
			base := func(s finance.Subscription) finance.Base {
				return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "exact ordinary cancellation"}
			}
			sub = data[finance.Subscription](t, call("POST", "/v1/subscriptions/"+sub.ID+"/transition", finance.SubscriptionTransition{Base: base(sub), Action: "cancel", EffectiveOn: ptr(date), VendorEvidenceVersionIDs: []string{}}, 200))
			sub = data[finance.Subscription](t, call("POST", "/v1/subscriptions/"+sub.ID+"/transition", finance.SubscriptionTransition{Base: base(sub), Action: "verify_vendor", VendorState: "cancelled", VendorCheckedAt: &checked, VendorEvidenceVersionIDs: []string{file.Version.ID}}, 200))
			assertProjectCloseoutItem(t, f, *file.File.ProjectID, "subscriptions", sub.ID, future)
			preview := data[project.CloseoutPreview](t, call("GET", "/v1/projects/"+*file.File.ProjectID+"/closeout", nil, 200))
			found := false
			for _, g := range preview.Groups {
				for _, item := range g.Items {
					if item.ID == sub.ID {
						found = true
						c := item.SubscriptionCalendar
						if c == nil || c.Timezone != zone || c.BusinessDate != today || c.CancelEffectiveOn == nil || *c.CancelEffectiveOn != date || c.CancellationEffective == future || item.Blocking != future {
							t.Fatal("HTTP exact calendar basis missing", item)
						}
					}
				}
			}
			if !found {
				t.Fatal("ordinary authorized checklist omitted subscription")
			}
			if future {
				call("POST", "/v1/subscriptions/"+sub.ID+"/transition", finance.SubscriptionTransition{Base: base(sub), Action: "archive", VendorEvidenceVersionIDs: []string{}}, 409)
			} else {
				call("POST", "/v1/subscriptions/"+sub.ID+"/transition", finance.SubscriptionTransition{Base: base(sub), Action: "archive", VendorEvidenceVersionIDs: []string{}}, 200)
			}
			ids = append(ids, sub.ID)
		}
	}
	if !different {
		t.Fatal("opposite calendar zones unexpectedly equal Hong Kong simultaneously")
	}
	if browser {
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch166-closeout-calendar", "project_id": *file.File.ProjectID, "subscription_ids": ids})
	}
}
