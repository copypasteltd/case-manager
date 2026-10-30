package httpapi

import (
	"context"
	"os"
	"testing"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/spend"
)

// Ordinary HTTP commands create all three scopes. No browser route is mocked;
// native PG, normal authorization and ordinary immutable business revisions run.
func TestProjectBusinessJourneys(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	pid := *file.File.ProjectID
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, role := range []string{"cost_recorder", "payment_recorder", "contract_manager", "purchase_manager", "claim_submitter", "subscription_manager", "party_manager", "finance_reporter"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, role)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	other := f.createProject("Other business scope", nil)
	type head struct {
		ID             string  `json:"id"`
		ProjectID      *string `json:"project_id"`
		Version        int64   `json:"version"`
		Classification string  `json:"classification"`
	}
	selected := map[string]string{}
	post := func(path string, input any) response {
		t.Helper()
		r, v := f.request("POST", path, f.aliceToken, core.NewID(), input, nil)
		expect(t, r, 201, v)
		return v
	}
	create := func(scope *string, prefix string) map[string]string {
		level := "internal"
		if scope != nil {
			level = "project_restricted"
		}
		out := map[string]string{}
		for _, kind := range []string{"contracts", "purchases", "claims", "costs", "payments", "subscriptions"} {
			title := prefix + " " + kind
			var in any
			switch kind {
			case "contracts":
				in = contracts.Create{Content: contracts.Content{Title: title}, ProjectID: scope, Classification: level, Reason: "synthetic project journey"}
			case "purchases", "claims":
				in = spend.Create{Content: spend.Content{Title: title}, ProjectID: scope, Classification: level, Reason: "synthetic project journey"}
			case "costs":
				in = finance.Create{Content: finance.Content{Title: title, Allocations: []finance.Allocation{}, EvidenceVersionIDs: []string{}}, SourceKey: "manual:" + core.NewID(), ProjectID: scope, Classification: level, Reason: "synthetic project journey"}
			case "payments":
				in = finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: title, EvidenceVersionIDs: []string{}}, SourceKey: "manual:" + core.NewID(), ProjectID: scope, Classification: level, Reason: "synthetic project journey"}
			case "subscriptions":
				in = finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: title, Timezone: "Asia/Hong_Kong", VendorState: "unknown", Prices: []finance.SubscriptionPriceInput{}, ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}}, SourceKey: "manual:" + core.NewID(), ProjectID: scope, Classification: level, Reason: "synthetic project journey"}
			}
			h := data[head](t, post("/v1/"+kind, in))
			if h.Version != 1 || h.Classification != level || (scope != nil && (h.ProjectID == nil || *h.ProjectID != *scope)) || (scope == nil && h.ProjectID != nil) {
				t.Fatal("scope not retained", kind, h)
			}
			out[kind] = h.ID
		}
		return out
	}
	readContext := func(token, id string) project.BusinessContext {
		t.Helper()
		r, v := f.request("GET", "/v1/projects/"+id+"/business-context", token, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.BusinessContext](t, v)
	}
	t.Run("human current project capabilities agree with six ordinary create commands", func(t *testing.T) {
		c := readContext(f.aliceToken, pid)
		if c.Scope != "project_business" || c.Project.ID != pid || len(c.Capabilities) != 6 || c.AsOf.IsZero() {
			t.Fatal(c)
		}
		for _, cap := range c.Capabilities {
			if !cap.CanCreate || cap.BlockedReason != nil {
				t.Fatal("creator capability missing", cap)
			}
		}
		selected = create(&pid, "Project journey")
		create(&other.ID, "Other journey")
		create(nil, "Company journey")
	})
	list := func(kind, query string) struct {
		Items      []head  `json:"items"`
		NextCursor *string `json:"next_cursor"`
	} {
		t.Helper()
		r, v := f.request("GET", "/v1/"+kind+"?"+query, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[struct {
			Items      []head  `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}](t, v)
	}
	t.Run("project list navigation preserves drafts and excludes company and other projects", func(t *testing.T) {
		for kind, id := range selected {
			q := "limit=20&project_id=" + pid
			if kind == "costs" || kind == "payments" {
				q += "&project_records=true"
			}
			result := list(kind, q)
			if len(result.Items) != 1 || result.Items[0].ID != id {
				t.Fatal("project filter widened or lost draft", kind, result)
			}
			company := list(kind, "limit=20&company_only=true")
			if ((kind == "costs" || kind == "payments") && len(company.Items) != 0) || ((kind != "costs" && kind != "payments") && (len(company.Items) != 1 || company.Items[0].ProjectID != nil)) {
				t.Fatal("company filter widened", kind, company)
			}
			found := list(kind, q+"&q=Project%20journey")
			if len(found.Items) != 1 || found.Items[0].ID != id {
				t.Fatal("search lost project", kind)
			}
		}
	})
	t.Run("record navigation never creates project money facts or changes report scope", func(t *testing.T) {
		for _, kind := range []string{"costs", "payments"} {
			if len(list(kind, "limit=20&project_id="+pid).Items) != 0 {
				t.Fatal("unallocated draft became actual share", kind)
			}
		}
		r, v := f.request("GET", "/v1/projects/"+pid+"/overview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		o := data[project.Overview](t, v)
		for _, kind := range []string{"costs", "payments"} {
			g := overviewGroup(t, o, kind)
			if len(g.Items) != 1 || g.Items[0].ID != selected[kind] {
				t.Fatal("project draft absent from overview", kind)
			}
		}
		if len(o.Incurred.Totals) != 0 {
			t.Fatal("drafts polluted incurred money")
		}
		for _, path := range []string{"/v1/costs?project_records=true", "/v1/payments?project_id=" + pid + "&company_only=true&project_records=true", "/v1/reports/expenses?kind=incurred&project_id=" + pid + "&project_records=true", "/v1/reports/cash?project_id=" + pid + "&project_records=false"} {
			r, v = f.request("GET", path, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
	})
	t.Run("project record cursors cannot be replayed into share-only lists", func(t *testing.T) {
		create(&pid, "Project second")
		for _, kind := range []string{"costs", "payments"} {
			q := "limit=1&project_id=" + pid + "&project_records=true"
			a := list(kind, q)
			if a.NextCursor == nil {
				t.Fatal("pagination missing", kind)
			}
			b := list(kind, q+"&cursor="+*a.NextCursor)
			if len(b.Items) != 1 || b.Items[0].ID == a.Items[0].ID {
				t.Fatal("cursor repeated record", kind)
			}
			r, v := f.request("GET", "/v1/"+kind+"?limit=1&project_id="+pid+"&cursor="+*a.NextCursor, f.aliceToken, "", nil, nil)
			expect(t, r, 400, v)
		}
	})
	t.Run("closed project previews are read-only and old previews cannot authorize writes", func(t *testing.T) {
		closed := f.createProject("Closed business preview", nil)
		exec("UPDATE projects SET state='closing',version=version+1 WHERE org_id=$1 AND id=$2", f.org, closed.ID)
		c := readContext(f.aliceToken, closed.ID)
		for _, cap := range c.Capabilities {
			if cap.CanCreate || cap.BlockedReason == nil || *cap.BlockedReason != "project_read_only" {
				t.Fatal(cap)
			}
		}
		r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), contracts.Create{Content: contracts.Content{Title: "stale preview must not write"}, ProjectID: &closed.ID, Classification: "project_restricted", Reason: "old preview"}, nil)
		expect(t, r, 409, v)
	})
	t.Run("readers see no invented create rights and foreign projects remain hidden", func(t *testing.T) {
		exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer') ON CONFLICT DO NOTHING", f.org, pid, f.bob)
		token := f.login("bob", "android").AccessToken
		c := readContext(token, pid)
		for _, cap := range c.Capabilities {
			if cap.CanCreate {
				t.Fatal("reader received create", cap)
			}
		}
		for _, suffix := range []string{"?project_id=" + other.ID, "?q=ignored"} {
			r, v := f.request("GET", "/v1/projects/"+pid+"/business-context"+suffix, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		r, v := f.request("GET", "/v1/projects/"+core.NewID()+"/business-context", f.aliceToken, "", nil, nil)
		expect(t, r, 404, v)
	})
	if !t.Failed() && os.Getenv("CP_BROWSER_PROJECT_BUSINESS") == "1" {
		vendor := data[party.Party](t, post("/v1/parties", party.Create{Content: party.Content{Name: "订阅核对演示供应方", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &pid, Classification: "project_restricted", Reason: "isolated recurring supplier"}))
		date, cycle, first, amount, currency, anchor := "2026-01-01", "month", "2026-01-01", "10", "HKD", 1
		sub := data[finance.Subscription](t, post("/v1/subscriptions", finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "项目月开支 · 账期核对演示", Supplier: &finance.PartyRef{PartyID: vendor.ID, RevisionID: vendor.CurrentRevisionID}, OwnerID: &f.alice, Category: "software", StartDate: &date, Cycle: &cycle, Timezone: "Asia/Hong_Kong", FirstChargeDate: &first, AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: date, Currency: currency, Amount: amount, Plan: "月订阅"}}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic period journey"}))
		base := func(s finance.Subscription) finance.Base {
			return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "exact recurring basis"}
		}
		r, v := f.request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(sub), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		sub = data[finance.Subscription](t, v)
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(sub), From: "2026-10-01", To: "2026-11-01"}, nil)
		expect(t, r, 200, v)
		generated := data[finance.SubscriptionGeneration](t, v)
		if len(generated.GeneratedIDs) != 1 {
			t.Fatal(generated)
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"project_business_fixture": true, "fixture_kind": "human-project-business", "project_id": pid, "other_project_id": other.ID, "business_ids": selected, "subscription_id": sub.ID, "period_id": generated.GeneratedIDs[0]})
	}
}
