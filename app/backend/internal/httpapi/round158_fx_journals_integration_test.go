package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"encoding/json"
	"os"
	"testing"
)

func TestRound158FXJournals(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(s string) *string { return &s }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"payment_recorder", "payment_reverser", "cash_reporter", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "claim_submitter", "request_manager", "finance_fx_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, key)
	}
	for _, key := range []string{"approval_reviewer", "party_manager", "claim_reader"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.bob, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	call := func(t *testing.T, method, path string, body any, status int) response {
		t.Helper()
		r, v := f.with(t).request(method, path, f.aliceToken, core.NewID(), body, nil)
		expect(t, r, status, v)
		return v
	}
	supplier := data[party.Party](t, call(t, "POST", "/v1/parties", party.Create{Content: party.Content{Name: "Synthetic FX supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated actual settlement"}, 201))
	partyRef := &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}
	base := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "review actual dual currency settlement"}
	}
	cb := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "review exact incurred cost"}
	}
	get := func(t *testing.T, id string) finance.Payment {
		t.Helper()
		return data[finance.Payment](t, call(t, "GET", "/v1/payments/"+id, nil, 200))
	}
	getCost := func(t *testing.T, id string) finance.Cost {
		t.Helper()
		return data[finance.Cost](t, call(t, "GET", "/v1/costs/"+id, nil, 200))
	}
	rows := func(t *testing.T, id string) []finance.PaymentAllocation {
		t.Helper()
		return data[core.Page[finance.PaymentAllocation]](t, call(t, "GET", "/v1/payments/"+id+"/allocations?limit=100", nil, 200)).Items
	}
	root := func(t *testing.T, id string) finance.PaymentAllocation {
		t.Helper()
		for _, a := range rows(t, id) {
			if a.Kind == "allocation" {
				return a
			}
		}
		t.Fatal("missing original allocation")
		return finance.PaymentAllocation{}
	}
	newCost := func(t *testing.T, currency, amount string, shares []finance.Allocation) finance.Cost {
		t.Helper()
		if shares == nil {
			shares = []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}
		}
		c := data[finance.Cost](t, call(t, "POST", "/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic FX incurred service", OccurredOn: ptr("2025-01-15"), Currency: ptr(currency), Amount: ptr(amount), Category: "settlement116", Counterparty: partyRef, Allocations: shares, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "fx-cost:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated incurred fact"}, 201))
		return data[finance.Cost](t, call(t, "POST", "/v1/costs/"+c.ID+"/confirm", cb(c), 200))
	}
	draft := func(t *testing.T, currency, amount string, employee *string, original *finance.Payment) finance.Payment {
		t.Helper()
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic FX actual principal", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-02T12:00:00+08:00"), Currency: ptr(currency), Amount: ptr(amount), Category: "settlement116", Counterparty: partyRef, EmployeeID: employee, MethodLabel: "synthetic bank statement", EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "fx-bank:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated actual cash fact"}
		if employee != nil {
			in.Counterparty = nil
		}
		if original != nil {
			in.Direction = ptr("refund")
			in.RefundOfID = &original.ID
			in.Counterparty = original.Counterparty
			in.EmployeeID = original.EmployeeID
		}
		return data[finance.Payment](t, call(t, "POST", "/v1/payments", in, 201))
	}
	record := func(t *testing.T, p finance.Payment) finance.Payment {
		t.Helper()
		return data[finance.Payment](t, call(t, "POST", "/v1/payments/"+p.ID+"/record", finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, 200))
	}
	allocInput := func(p finance.Payment, c finance.Cost, cash, cost, rate string) finance.PaymentAllocate {
		in := finance.PaymentAllocate{Base: base(p), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: cash}
		if rate != "" {
			in.FX = &finance.PaymentFXInput{CostAmount: cost, Rate: rate, Source: "synthetic reviewed bank settlement", Acknowledged: true}
		}
		return in
	}
	allocate := func(t *testing.T, p finance.Payment, c finance.Cost, cash, cost, rate string) finance.Payment {
		t.Helper()
		return data[finance.Payment](t, call(t, "POST", "/v1/payments/"+p.ID+"/allocations", allocInput(p, c, cash, cost, rate), 200))
	}
	release := func(t *testing.T, p finance.Payment, a finance.PaymentAllocation, cash, cost string) finance.Payment {
		t.Helper()
		return data[finance.Payment](t, call(t, "POST", "/v1/payments/"+p.ID+"/allocations/"+a.ID+"/release", finance.PaymentRelease{Base: base(p), Amount: cash, CostAmount: &cost}, 200))
	}
	reverse := func(t *testing.T, p finance.Payment, status int) finance.Payment {
		t.Helper()
		v := call(t, "POST", "/v1/payments/"+p.ID+"/reverse", finance.PaymentReverseInput{Base: base(p), EffectiveAt: "2025-02-03T12:00:00+08:00"}, status)
		if status != 200 {
			return p
		}
		return data[finance.Payment](t, v)
	}
	assertCost := func(t *testing.T, c finance.Cost, paid, outstanding string) {
		t.Helper()
		v := getCost(t, c.ID)
		if valueForTest(v.PaidAmount) != paid || valueForTest(v.OutstandingAmount) != outstanding {
			t.Fatal(v)
		}
	}
	_ = ptr
	_ = exec
	_ = call
	_ = base
	_ = cb
	_ = get
	_ = getCost
	_ = rows
	_ = root
	_ = newCost
	_ = draft
	_ = record
	_ = allocInput
	_ = allocate
	_ = release
	_ = reverse
	_ = assertCost

	c := newCost(t, "USD", "100", nil)
	p := record(t, draft(t, "HKD", "790", nil, nil))
	p = allocate(t, p, c, "790", "100", "7.9")
	a := root(t, p.ID)
	bookInput := finance.FXBookCreate{ExpectedVersion: 0, Acknowledged: true, Reason: "synthetic reviewed independent FX basis", Content: finance.FXContent{Title: "Batch158 pinned quotations", BaseCurrency: "HKD", Method: "manual_interval", Rounding: "scope_line_half_up", Rates: []finance.FXRate{{Currency: "USD", ValidFrom: "2025-01-01", ValidTo: "2026-01-01", RateDate: "2024-12-31", Rate: "7.8", Source: "synthetic treasury quotation"}}}}
	r, v := f.request("POST", "/v1/finance/fx-books", f.aliceToken, core.NewID(), bookInput, nil)
	expect(t, r, 201, v)
	book := data[finance.FXBook](t, v)
	path := "/v1/payments/" + p.ID + "/fx-differences"
	preview := data[finance.FXJournalPreview](t, call(t, "GET", path+"/basis?allocation_id="+a.ID+"&fx_book_id="+book.ID, nil, 200))
	var b finance.FXJournalBasis
	if json.Unmarshal([]byte(preview.Basis), &b) != nil || b.Loss != "10.000000" || b.Gain != "0.000000" {
		t.Fatal("wrong realized difference", b)
	}
	in := finance.FXJournalCreate{Base: base(p), ExpectedJournalVersion: preview.JournalVersion, AllocationID: a.ID, FXBookID: book.ID, BasisSHA256: preview.BasisSHA256, RecognizedOn: "2025-02-02", Reviewed: true}
	beforeCost, beforePayment := getCost(t, c.ID), get(t, p.ID)
	key := core.NewID()
	r, v = f.request("POST", path, f.aliceToken, key, in, nil)
	expect(t, r, 201, v)
	journal := data[finance.FXJournal](t, v)
	r, v = f.request("POST", path, f.aliceToken, key, in, nil)
	expect(t, r, 201, v)
	if data[finance.FXJournal](t, v).ID != journal.ID {
		t.Fatal("FX replay duplicated journal")
	}
	if getCost(t, c.ID).Version != beforeCost.Version || get(t, p.ID).Version != beforePayment.Version {
		t.Fatal("FX journal changed original cost/cash")
	}
	ledger := data[finance.FXJournalLedger](t, call(t, "GET", path, nil, 200))
	if !ledger.Complete || ledger.JournalVersion != 1 || len(ledger.Items) != 1 || ledger.Items[0].State != "active" || len(ledger.Totals) != 1 || ledger.Totals[0].Loss != "10.000000" {
		t.Fatal("FX complete ledger wrong", ledger)
	}
	in.ExpectedJournalVersion = 1
	call(t, "POST", path, in, 409)
	p = release(t, p, a, "7.9", "1")
	ledger = data[finance.FXJournalLedger](t, call(t, "GET", path, nil, 200))
	if ledger.Items[0].State != "basis_changed" || len(ledger.Totals) != 0 {
		t.Fatal("stale FX basis stayed in active totals")
	}
	revoked := data[finance.FXJournal](t, call(t, "POST", path+"/"+journal.ID+"/revoke", finance.FXJournalRevoke{Base: base(p), ExpectedJournalVersion: 1, BasisSHA256: journal.BasisSHA256, Reviewed: true}, 200))
	if revoked.Revocation == nil || revoked.Revocation.Version != 2 || revoked.Basis != journal.Basis {
		t.Fatal("FX immutable revocation missing")
	}
	ledger = data[finance.FXJournalLedger](t, call(t, "GET", path, nil, 200))
	if ledger.Items[0].State != "revoked" || len(ledger.Totals) != 0 {
		t.Fatal("revocation did not remove active totals")
	}
	if _, e := f.api.DB.Exec(ctx, "UPDATE payment_fx_journals SET reason='forged' WHERE id=$1", journal.ID); e == nil {
		t.Fatal("application changed immutable FX evidence")
	}
	if os.Getenv("CP_BROWSER_BATCH158_FIXTURE") != "" {
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch158-fx-journals", "payment_id": p.ID, "project_id": *file.File.ProjectID, "journal_id": journal.ID})
	}
}
