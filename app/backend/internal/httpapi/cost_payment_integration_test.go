package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/spend"
)

func costPaymentFixture(t *testing.T) (*fixture, storage.CommitResult, party.Party) {
	t.Helper()
	f, file, supplier := businessCostFixture(t)
	for _, role := range []string{"payment_recorder", "payment_reverser", "cash_reporter"} {
		if _, e := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, role); e != nil {
			t.Fatal(e)
		}
	}
	return f, file, supplier
}
func costPaymentCost(t *testing.T, f *fixture, file storage.CommitResult, supplier party.Party, kind, title, amount string) (finance.Cost, finance.BusinessCostSource) {
	t.Helper()
	source := businessCostSourceFixture(t, f, file, supplier, kind, amount, true)
	ptr := func(v string) *string { return &v }
	in := finance.BusinessCostRecord{Source: source, SourceKey: "manual:" + core.NewID(), Reason: "Human reviewed expense for later independent actual payment", ActualIncurred: true, SourceReviewed: true, Content: finance.Content{Title: title, OccurredOn: ptr("2025-02-10"), Currency: ptr("HKD"), Amount: &amount, Category: "cost-payment", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}}
	r, v := f.request("POST", "/v1/costs/from-business", f.aliceToken, core.NewID(), in, nil)
	expect(t, r, 201, v)
	return data[finance.Cost](t, v), source
}
func costPaymentInput(c finance.Cost, file storage.CommitResult, supplier party.Party, cash, allocation string) finance.CostPaymentRecord {
	ptr := func(v string) *string { return &v }
	return finance.CostPaymentRecord{PaymentContent: finance.PaymentContent{Title: "Actual reviewed payment", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-10T01:30:00Z"), Currency: ptr("HKD"), Amount: &cash, Category: "cost-payment", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "Bank transfer already completed", ExternalRef: "synthetic-human-reconciled", EvidenceVersionIDs: []string{file.Version.ID}, Notes: "Actual cash and expense settlement remain distinct"}, Cost: finance.CostPaymentSource{ID: c.ID, Version: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash}, AllocationAmount: allocation, SourceKey: "manual:" + core.NewID(), Reason: "Human verified cash and exact cost settlement", ActualPaid: true, SettlementReviewed: true}
}
func TestCostPaymentRecording(t *testing.T) {
	f, file, supplier := costPaymentFixture(t)
	ctx := context.Background()
	call := func(t *testing.T, method, path, token, key string, body any, status int) response {
		t.Helper()
		r, v := f.with(t).request(method, path, token, key, body, nil)
		expect(t, r, status, v)
		return v
	}
	post := func(t *testing.T, in finance.CostPaymentRecord, key string, status int) response {
		t.Helper()
		return call(t, "POST", "/v1/payments/from-cost", f.aliceToken, key, in, status)
	}
	count := func(t *testing.T, q string, args ...any) int {
		t.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, q, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	makeCost := func(t *testing.T, kind, amount string) (finance.Cost, finance.BusinessCostSource) {
		t.Helper()
		return costPaymentCost(t, f.with(t), file, supplier, kind, "Actual expense for payment", amount)
	}
	t.Run("preview fixes confirmed expense and never creates or reserves money", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "100")
		before := count(t, "SELECT count(*) FROM payment_records WHERE org_id=$1", f.org)
		b := data[finance.CostPaymentBasis](t, call(t, "GET", "/v1/costs/"+c.ID+"/payment-basis", f.aliceToken, "", nil, 200))
		if b.Cost.ID != c.ID || b.Cost.Version != c.Version || b.Cost.CurrentRevisionID != c.CurrentRevisionID || b.AsOf.IsZero() {
			t.Fatal(b)
		}
		if count(t, "SELECT count(*) FROM payment_records WHERE org_id=$1", f.org) != before {
			t.Fatal("preview created cash")
		}
		call(t, "GET", "/v1/costs/"+c.ID+"/payment-basis?project_id="+core.NewID(), f.aliceToken, "", nil, 422)
		call(t, "GET", "/v1/costs/"+c.ID+"/payment-basis", f.bobToken, "", nil, 404)
	})
	t.Run("partial supplier cash preserves unassigned money ordinary journals and cost facts", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "100")
		in := costPaymentInput(c, file, supplier, "60", "40")
		receipt := data[finance.CostPaymentReceipt](t, post(t, in, core.NewID(), 201))
		p, a := receipt.Payment, receipt.Allocation
		if p.Version != 3 || p.State != "recorded" || p.EffectiveAllocatedAmount != "40.000000" || *p.UnallocatedAmount != "20.000000" || a.CostID != c.ID || a.CostRevisionID != c.CurrentRevisionID || a.Amount != "40.000000" || a.CostAmount != "40.000000" || a.ClaimID != nil || p.ProjectID == nil || *p.ProjectID != *c.ProjectID {
			t.Fatal(receipt)
		}
		fresh := data[finance.Cost](t, call(t, "GET", "/v1/costs/"+c.ID, f.aliceToken, "", nil, 200))
		if fresh.CurrentRevisionID != c.CurrentRevisionID || fresh.Version != c.Version || fresh.Amount == nil || *fresh.Amount != "100.000000" || fresh.PaidAmount == nil || *fresh.PaidAmount != "40.000000" || *fresh.OutstandingAmount != "60.000000" {
			t.Fatal(fresh)
		}
		if count(t, "SELECT count(*) FROM payment_events WHERE payment_id=$1", p.ID) != 3 || count(t, "SELECT count(*) FROM payment_revisions WHERE payment_id=$1", p.ID) != 1 || count(t, "SELECT count(*) FROM audit_events WHERE resource_type='payment' AND resource_id=$1", p.ID) != 4 || count(t, "SELECT count(*) FROM outbox_events WHERE aggregate_type='payment' AND aggregate_id=$1", p.ID) != 4 || count(t, "SELECT count(*) FROM file_version_references WHERE owner_type='payment_revision' AND owner_id=$1", p.CurrentRevisionID) != 1 {
			t.Fatal("ordinary journals or immutable references missing")
		}
	})
	t.Run("foreign cash retains independent currencies and reviewed conversion", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "100")
		in := costPaymentInput(c, file, supplier, "13", "6.50")
		currency := "USD"
		in.Currency = &currency
		in.FX = &finance.PaymentFXInput{CostAmount: "50", Rate: "0.13", Source: "Human actual bank conversion", Acknowledged: true}
		receipt := data[finance.CostPaymentReceipt](t, post(t, in, core.NewID(), 201))
		if *receipt.Payment.UnallocatedAmount != "6.500000" || receipt.Allocation.CostAmount != "50.000000" || receipt.Allocation.CostCurrency != "HKD" || receipt.Allocation.PaymentCurrency != "USD" || receipt.Allocation.FX == nil || receipt.Allocation.CostShares[0].Amount != "50.000000" {
			t.Fatal(receipt)
		}
	})
	t.Run("employee advance requires exact approved linked claim line and common cap", func(t *testing.T) {
		c, source := makeCost(t, "claim", "100")
		claim := data[spend.Record](t, call(t, "GET", "/v1/claims/"+source.ID, f.aliceToken, "", nil, 200))
		in := costPaymentInput(c, file, supplier, "30", "25")
		in.Counterparty = nil
		in.EmployeeID = &f.alice
		in.Claim = &finance.CostPaymentClaim{CostPaymentSource: finance.CostPaymentSource{ID: claim.ID, Version: claim.Version, RevisionID: *claim.ApprovedRevisionID, ContentHash: claim.ContentHash}, LineID: claim.Lines[0].ID}
		receipt := data[finance.CostPaymentReceipt](t, post(t, in, core.NewID(), 201))
		if receipt.Payment.EmployeeID == nil || *receipt.Payment.EmployeeID != f.alice || receipt.Allocation.ClaimLineID == nil || *receipt.Allocation.ClaimLineID != claim.Lines[0].ID || *receipt.Payment.UnallocatedAmount != "5.000000" {
			t.Fatal(receipt)
		}
		changed := in
		changed.SourceKey = "manual:" + core.NewID()
		copyClaim := *in.Claim
		copyClaim.Version++
		changed.Claim = &copyClaim
		v := post(t, changed, core.NewID(), 409)
		if v.Error.Code != "PAYMENT_CLAIM_CHANGED" {
			t.Fatal(v)
		}
	})
	t.Run("late over allocation atomically rolls back cash files events outbox and command keys", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "10")
		in := costPaymentInput(c, file, supplier, "20", "11")
		key := core.NewID()
		tables := []string{"payment_records", "payment_revisions", "payment_events", "payment_allocations", "outbox_events", "idempotency_records", "file_version_references"}
		before := map[string]int{}
		for _, table := range tables {
			before[table] = count(t, "SELECT count(*) FROM "+table+" WHERE org_id=$1", f.org)
		}
		beforeAudit := count(t, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND result='succeeded'", f.org)
		post(t, in, key, 409)
		if count(t, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND result='succeeded'", f.org) != beforeAudit {
			t.Fatal("failed allocation leaked successful business audit")
		}
		for _, table := range tables {
			if count(t, "SELECT count(*) FROM "+table+" WHERE org_id=$1", f.org) != before[table] {
				t.Fatal("partial failed command leaked", table)
			}
		}
		in.AllocationAmount = "10"
		post(t, in, key, 201)
	})
	t.Run("concurrent actual payments serialize on shared expense balance", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "10")
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				in := costPaymentInput(c, file, supplier, "10", "10")
				r, _ := f.with(t).request("POST", "/v1/payments/from-cost", f.aliceToken, core.NewID(), in, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		ok := 0
		for code := range codes {
			if code == 201 {
				ok++
			} else if code != 409 {
				t.Fatal("unexpected concurrent status", code)
			}
		}
		if ok != 1 {
			t.Fatal("shared expense balance over allocated", ok)
		}
	})
	t.Run("fixed cost source and explicit facts reject stale heads scope injection and inferred cash", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "100")
		in := costPaymentInput(c, file, supplier, "10", "10")
		in.Cost.Version++
		post(t, in, core.NewID(), 409)
		in.Cost.Version--
		for _, which := range []string{"actual", "reviewed", "future", "no_file", "wrong_party", "unknown_scope"} {
			bad := in
			bad.SourceKey = "manual:" + core.NewID()
			status := 422
			switch which {
			case "actual":
				bad.ActualPaid = false
			case "reviewed":
				bad.SettlementReviewed = false
			case "future":
				at := "2099-01-01T00:00:00Z"
				bad.PaidAt = &at
			case "no_file":
				bad.EvidenceVersionIDs = []string{}
			case "wrong_party":
				bad.Counterparty = &finance.PartyRef{PartyID: core.NewID(), RevisionID: core.NewID()}
				status = 404
			case "unknown_scope":
				raw, _ := json.Marshal(bad)
				var body map[string]any
				json.Unmarshal(raw, &body)
				body["project_id"] = core.NewID()
				call(t, "POST", "/v1/payments/from-cost", f.aliceToken, core.NewID(), body, 422)
				continue
			}
			post(t, bad, core.NewID(), status)
		}
	})
	t.Run("cached replay preserves original receipt and still requires real current MFA and authority", func(t *testing.T) {
		c, _ := makeCost(t, "purchase", "100")
		in, key := costPaymentInput(c, file, supplier, "20", "10"), core.NewID()
		first := data[finance.CostPaymentReceipt](t, post(t, in, key, 201))
		second := data[finance.CostPaymentReceipt](t, post(t, in, key, 201))
		if first.Payment.ID != second.Payment.ID || first.Allocation.ID != second.Allocation.ID || *second.Payment.UnallocatedAmount != "10.000000" {
			t.Fatal("duplicate or changed replay", second)
		}
		changed := in
		changed.Notes = "different body"
		post(t, changed, key, 409)
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		actual := actor.MFAVerifiedAt
		if _, e = f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID); e != nil {
			t.Fatal(e)
		}
		post(t, in, key, 403)
		if _, e = f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual); e != nil {
			t.Fatal(e)
		}
		if _, e = f.owner.Exec(ctx, `DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN (SELECT id FROM roles WHERE org_id=$1 AND key='payment_recorder')`, f.org, f.alice); e != nil {
			t.Fatal(e)
		}
		post(t, in, key, 404)
	})
}

func TestCostPaymentBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_COST_PAYMENT") != "1" {
		t.Skip("explicit owned browser opt-in only")
	}
	f, file, supplier := costPaymentFixture(t)
	costs := []map[string]any{}
	for _, scenario := range []string{"supplier", "foreign", "employee"} {
		kind := "purchase"
		if scenario == "employee" {
			kind = "claim"
		}
		c, source := costPaymentCost(t, f, file, supplier, kind, "付款核销演示 · "+scenario, "100")
		costs = append(costs, map[string]any{"scenario": scenario, "id": c.ID, "source_id": source.ID, "title": c.Title})
	}
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "human-cost-payment", "cost_payment_fixture": true, "project_id": file.File.ProjectID, "file_id": file.File.ID, "file_version_id": file.Version.ID, "supplier_id": supplier.ID, "supplier_name": supplier.Name, "employee_id": f.alice, "payment_costs": costs, "created_at": time.Now().UTC()})
}
