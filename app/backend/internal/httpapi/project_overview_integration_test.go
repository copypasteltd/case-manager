package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/spend"
)

func overviewGroup(t *testing.T, value project.Overview, kind string) project.OverviewGroup {
	t.Helper()
	for _, group := range value.Groups {
		if group.Kind == kind {
			return group
		}
	}
	t.Fatal("missing overview category", kind)
	return project.OverviewGroup{}
}

// Real HTTP, native PostgreSQL and the ordinary business commands exercise the
// overview's read scope. The only direct metadata fixtures are explicitly noted.
func TestProjectOverview(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	pid := *original.File.ProjectID
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(user, key string) {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, user, key)
	}
	for _, role := range []string{"cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "contract_manager", "purchase_manager", "claim_submitter", "subscription_manager", "status_operator", "payment_recorder"} {
		bind(f.alice, role)
	}
	// A reader with deliberately narrow actions can read the project but cannot
	// infer financial or other restricted rows, including their counts.
	exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer') ON CONFLICT DO NOTHING", f.org, pid, f.bob)
	exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
	role := core.NewID()
	exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'overview_reader','Narrow overview reader')", role, f.org)
	for _, action := range []string{"project.read", "task.read", "post.read"} {
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action)
	}
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.bob, role)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	bob := f.login("bob", "android").AccessToken
	read := func(t *testing.T, token, id string) project.Overview {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+id+"/overview", token, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Overview](t, v)
	}
	post := func(t *testing.T, path string, input any) response {
		t.Helper()
		r, v := f.with(t).request("POST", path, f.aliceToken, core.NewID(), input, nil)
		expect(t, r, 201, v)
		return v
	}
	var initialTasks int64
	t.Run("consistent sixteen categories separate readable zero from unavailable", func(t *testing.T) {
		full := read(t, f.aliceToken, pid)
		if len(full.Groups) != 16 || full.ProjectID != pid || full.ProjectVersion < 1 || full.AsOf.IsZero() || full.Scope != "readable_records" {
			t.Fatal("overview snapshot is incomplete")
		}
		initialTasks = *overviewGroup(t, full, "tasks").Count
		narrow := read(t, bob, pid)
		for _, kind := range []string{"contracts", "purchases", "claims", "costs", "payments", "subscriptions", "budgets", "changes", "services"} {
			g := overviewGroup(t, narrow, kind)
			if g.Available || g.Count != nil || len(g.Items) != 0 || len(g.States) != 0 || g.HasMore {
				t.Fatal("restricted category leaked facts or a false zero", kind)
			}
		}
		if narrow.Incurred.Available || narrow.Incurred.UnconfirmedCount != nil || len(narrow.Incurred.Totals) != 0 || len(narrow.Budgets) != 0 {
			t.Fatal("restricted financial aggregate leaked")
		}
	})
	t.Run("full readable counts precede recent five preview and exclude other projects", func(t *testing.T) {
		p := f.createProject("Unrelated overview project", nil)
		f.createTask(p, "Must stay outside the project overview", false)
		var selected project.Project
		r, v := f.request("GET", "/v1/projects/"+pid, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		selected = data[project.Project](t, v)
		for i := 0; i < 8; i++ {
			task := f.createTask(selected, fmt.Sprintf("Overview actual task %02d", i), false)
			if i%2 == 0 {
				r, v = f.transition(task, "start", f.aliceToken)
				expect(t, r, 200, v)
			}
		}
		g := overviewGroup(t, read(t, f.aliceToken, pid), "tasks")
		if g.Count == nil || *g.Count != initialTasks+8 || len(g.Items) != 5 || !g.HasMore {
			t.Fatal("preview was confused with full count", g)
		}
		var sum int64
		for _, state := range g.States {
			sum += state.Count
		}
		if sum != *g.Count {
			t.Fatal("state totals are not snapshot consistent")
		}
		for _, item := range g.Items {
			if item.Title == "Must stay outside the project overview" || item.ID == "" || item.Version < 1 || item.UpdatedAt.IsZero() {
				t.Fatal("foreign or unstable overview item")
			}
		}
	})
	var contract contracts.Contract
	t.Run("real contract purchase claim subscription and service heads retain stable identities", func(t *testing.T) {
		contract = data[contracts.Contract](t, post(t, "/v1/contracts", contracts.Create{Content: contracts.Content{Title: "Overview visible contract"}, ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic project contract"}))
		purchase := data[spend.Record](t, post(t, "/v1/purchases", spend.Create{Content: spend.Content{Title: "Overview visible purchase"}, ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic purchase"}))
		claim := data[spend.Record](t, post(t, "/v1/claims", spend.Create{Content: spend.Content{Title: "Overview visible claim"}, ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic claim"}))
		sub := data[finance.Subscription](t, post(t, "/v1/subscriptions", finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Overview subscription draft", VendorState: "unknown", Prices: []finance.SubscriptionPriceInput{}, ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic subscription"}))
		svc := data[status.Record](t, post(t, "/v1/services", status.Create{Content: status.Content{Name: "Overview service without monitoring", OwnerID: f.alice}, ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic configuration only"}))
		full := read(t, f.aliceToken, pid)
		for kind, id := range map[string]string{"contracts": contract.ID, "purchases": purchase.ID, "claims": claim.ID, "subscriptions": sub.ID, "services": svc.ID} {
			g := overviewGroup(t, full, kind)
			if !g.Available || g.Count == nil || *g.Count != 1 || len(g.Items) != 1 || g.Items[0].ID != id {
				t.Fatal("business head missing", kind, g)
			}
		}
		encoded, _ := json.Marshal(overviewGroup(t, full, "services"))
		if strings.Contains(string(encoded), "operational") || strings.Contains(string(encoded), "healthy") {
			t.Fatal("configuration was represented as monitored health")
		}
	})
	t.Run("an explicit record grant exposes only that record without granting category authority", func(t *testing.T) {
		resourceRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'overview_one_contract','Read one contract only')", resourceRole, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'contract.read')", f.org, resourceRole)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) VALUES($1,$2,$3,$4,'resource',$5,'project_restricted')", core.NewID(), f.org, f.bob, resourceRole, contract.ID)
		exec(`INSERT INTO resource_grants(id,org_id,user_id,resource_type,resource_id,action,classification_ceiling,grantor_id,reason) VALUES($1,$2,$3,'contract',$4,'contract.read','project_restricted',$5,'exact overview read')`, core.NewID(), f.org, f.bob, contract.ID, f.alice)
		post(t, "/v1/contracts", contracts.Create{Content: contracts.Content{Title: "Hidden other contract"}, ProjectID: &pid, Classification: "project_restricted", Reason: "no record grant"})
		bob = f.login("bob", "android").AccessToken
		g := overviewGroup(t, read(t, bob, pid), "contracts")
		if !g.Available || g.Count == nil || *g.Count != 1 || len(g.Items) != 1 || g.Items[0].ID != contract.ID || g.HasMore {
			t.Fatal("record grant widened the count or disappeared", g)
		}
	})
	t.Run("published log readers never see the unpublished head title or revision", func(t *testing.T) {
		p := data[knowledge.Post](t, post(t, "/v1/posts", map[string]any{"project_id": pid, "type": "project_log", "title": "Reviewed overview log", "body": "Synthetic published content", "tags": []string{}, "classification": "project_restricted"}))
		r, v := f.request("POST", "/v1/posts/"+p.ID+"/publish", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "base_revision_id": p.CurrentRevisionID, "action": "publish", "reason": "reviewed synthetic log"}, nil)
		expect(t, r, 200, v)
		p = data[knowledge.Post](t, v)
		published := p.CurrentRevisionID
		r, v = f.request("PATCH", "/v1/posts/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "base_revision_id": p.CurrentRevisionID, "title": "Unpublished overview secret", "body": "Unpublished material", "tags": []string{}}, nil)
		expect(t, r, 200, v)
		post(t, "/v1/posts", map[string]any{"project_id": pid, "type": "project_log", "title": "Unpublished private draft", "body": "Draft only", "tags": []string{}, "classification": "project_restricted"})
		g := overviewGroup(t, read(t, bob, pid), "posts")
		if g.Count == nil || *g.Count != 1 || len(g.Items) != 1 || g.Items[0].Title != "Reviewed overview log" || g.Items[0].RevisionID == nil || *g.Items[0].RevisionID != published {
			t.Fatal("unpublished log facts leaked", g)
		}
	})
	t.Run("files in inactive ancestor chains are excluded from preview and totals", func(t *testing.T) {
		folder := data[storage.Folder](t, post(t, "/v1/folders", map[string]any{"name": "Overview temporary folder", "parent_id": original.File.FolderID, "project_id": pid, "classification": "project_restricted"}))
		// Metadata-only file; no bytes or cloud availability are asserted.
		exec(`INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,'overview-metadata.txt','overview-metadata.txt','project_restricted','active',$4)`, core.NewID(), f.org, folder.ID, f.alice)
		before := overviewGroup(t, read(t, f.aliceToken, pid), "files")
		// Structural inactive-ancestor fixture; there is no folder-trash command.
		exec("UPDATE folders SET state='trashed' WHERE org_id=$1 AND id=$2", f.org, folder.ID)
		after := overviewGroup(t, read(t, f.aliceToken, pid), "files")
		if *after.Count != *before.Count-1 {
			t.Fatal("inactive ancestor remained in the file aggregate")
		}
	})
	t.Run("incurred costs use exact project shares separate currencies drafts and reversals", func(t *testing.T) {
		other := f.createProject("Other actual cost allocation", nil)
		supplier := data[party.Party](t, post(t, "/v1/parties", party.Create{Content: party.Content{Name: "Overview actual supplier", Kind: "organization", Relationships: []string{"supplier"}}, Classification: "internal", Reason: "synthetic company supplier"}))
		ptr := func(s string) *string { return &s }
		create := func(currency, amount string, allocations []finance.Allocation) finance.Cost {
			return data[finance.Cost](t, post(t, "/v1/costs", finance.Create{Content: finance.Content{Title: "Overview canonical " + currency, OccurredOn: ptr("2025-01-15"), Currency: ptr(currency), Amount: ptr(amount), Category: "overview", Allocations: allocations, Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, EvidenceVersionIDs: []string{original.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "exact synthetic cost"}))
		}
		base := func(c finance.Cost) finance.Base {
			return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "review exact synthetic revision"}
		}
		confirm := func(c finance.Cost) finance.Cost {
			r, v := f.request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
			expect(t, r, 200, v)
			return data[finance.Cost](t, v)
		}
		shared := confirm(create("HKD", "90071992547409.90", []finance.Allocation{{ProjectID: &pid, Amount: "45035996273704.95"}, {ProjectID: &other.ID, Amount: "45035996273704.95"}}))
		settled := confirm(create("HKD", "100.10", []finance.Allocation{{ProjectID: &pid, Amount: "100.10"}}))
		confirm(create("USD", "0.02", []finance.Allocation{{ProjectID: &pid, Amount: "0.02"}}))
		create("HKD", "999", []finance.Allocation{{ProjectID: &pid, Amount: "999"}})
		r, v := f.request("POST", "/v1/costs/"+shared.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: base(shared), EffectiveOn: "2025-02-10"}, nil)
		expect(t, r, 200, v)
		actual := read(t, f.aliceToken, pid).Incurred
		if !actual.Available || actual.UnconfirmedCount == nil || *actual.UnconfirmedCount != 1 || len(actual.Totals) != 2 {
			t.Fatal("draft/currency separation failed", actual)
		}
		for _, total := range actual.Totals {
			for _, amounts := range [][2]string{{total.ExpenseAmount, map[string]string{"HKD": "45035996273805.05", "USD": "0.02"}[total.Currency]}, {total.ReversalAmount, map[string]string{"HKD": "45035996273704.95", "USD": "0"}[total.Currency]}, {total.NetAmount, map[string]string{"HKD": "100.10", "USD": "0.02"}[total.Currency]}} {
				got, want := amounts[0], amounts[1]
				a, err := core.ParseDecimal(got)
				b, e := core.ParseDecimal(want)
				if err != nil || e != nil || a.String() != b.String() {
					t.Fatal("inexact or duplicated project amount", total, got, want)
				}
			}
		}
		t.Run("other allocation scopes cannot be inferred through the financial aggregate", func(t *testing.T) {
			bind(f.bob, "cost_reader")
			bind(f.bob, "finance_reporter")
			bob = f.login("bob", "android").AccessToken
			narrow := read(t, bob, pid).Incurred
			if !narrow.Available || len(narrow.Totals) != 2 {
				t.Fatal("authorized cost scope was lost")
			}
			for _, total := range narrow.Totals {
				if total.EntryCount != 1 || total.ReversalAmount != "0" {
					t.Fatal("other project's allocation or reversal was inferred", total)
				}
			}
		})
		t.Run("payments join through immutable shares without duplicating incurred expense", func(t *testing.T) {
			payment := data[finance.Payment](t, post(t, "/v1/payments", finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Overview actual payment", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-01T02:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr("100.10"), Category: "overview", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "Synthetic transfer", EvidenceVersionIDs: []string{original.Version.ID}}, SourceKey: "bank:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "synthetic real cash fact"}))
			paymentBase := func(p finance.Payment) finance.Base {
				return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "exact synthetic cash snapshot"}
			}
			before := overviewGroup(t, read(t, f.aliceToken, pid), "payments")
			if *before.Count != 0 {
				t.Fatal("unallocated payment draft invented an allocation")
			}
			r, v := f.with(t).request("POST", "/v1/payments/"+payment.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: paymentBase(payment), RefundAllocations: []finance.RefundAllocation{}}, nil)
			expect(t, r, 200, v)
			payment = data[finance.Payment](t, v)
			r, v = f.with(t).request("POST", "/v1/payments/"+payment.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: paymentBase(payment), CostID: settled.ID, CostVersion: settled.Version, CostRevisionID: settled.CurrentRevisionID, CostContentHash: settled.ContentHash, Amount: "100.10"}, nil)
			expect(t, r, 200, v)
			after := read(t, f.aliceToken, pid)
			g := overviewGroup(t, after, "payments")
			if *g.Count != 1 || len(g.Items) != 1 || g.Items[0].ID != payment.ID || g.Items[0].State != "recorded" {
				t.Fatal("actual payment allocation was not linked", g)
			}
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(after.Incurred)
			if string(a) != string(b) {
				t.Fatal("cash payment duplicated or changed occurred expense")
			}
		})
	})
	t.Run("unknown foreign unauthorized and revoked projects disclose no overview", func(t *testing.T) {
		for _, id := range []string{"not-a-uuid", core.NewID()} {
			r, v := f.request("GET", "/v1/projects/"+id+"/overview", f.aliceToken, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v := f.request("GET", "/v1/projects/"+pid+"/overview", f.memberToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.request("GET", "/v1/projects/"+pid, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		p := data[project.Project](t, v)
		r, v = f.request("POST", "/v1/projects/"+pid+"/members", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "user_id": f.bob, "action": "remove", "reason": "actual access revocation"}, nil)
		expect(t, r, 200, v)
		r, v = f.request("GET", "/v1/projects/"+pid+"/overview", bob, "", nil, nil)
		expect(t, r, 401, v)
		bob = f.login("bob", "android").AccessToken
		r, v = f.request("GET", "/v1/projects/"+pid+"/overview", bob, "", nil, nil)
		expect(t, r, 404, v)
	})
	if !t.Failed() && os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") == "1" {
		// Restore a deliberately narrow viewer for actual browser reads. These
		// fixture-only statements never run in the development/production schema.
		exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer')", f.org, pid, f.bob)
		exec("DELETE FROM role_bindings b USING roles r WHERE r.org_id=b.org_id AND r.id=b.role_id AND b.org_id=$1 AND b.user_id=$2 AND r.key IN('cost_reader','finance_reporter')", f.org, f.bob)
		holdBrowserBusinessFixture(t, f, map[string]any{"project_id": pid, "contract_id": contract.ID, "fixture_kind": "project-overview"})
	}
}
