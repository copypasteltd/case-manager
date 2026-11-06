package httpapi

import (
	"context"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
)

func TestRound160ForeignCloseoutPrincipal(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(s string) *string { return &s }
	for _, role := range []string{"payment_recorder", "payment_reverser", "cash_reporter", "party_manager", "cost_recorder", "cost_confirmer"} {
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3 ON CONFLICT DO NOTHING`, f.org, f.alice, role); err != nil {
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
	supplier := data[party.Party](t, call("POST", "/v1/parties", party.Create{Content: party.Content{Name: "Synthetic three-currency supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "refund134 fixture"}, 201))
	base := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "review actual three-currency refund"}
	}
	get := func(id string) finance.Payment {
		return data[finance.Payment](t, call("GET", "/v1/payments/"+id, nil, 200))
	}
	rows := func(p finance.Payment) []finance.PaymentAllocation {
		return data[core.Page[finance.PaymentAllocation]](t, call("GET", "/v1/payments/"+p.ID+"/allocations?limit=100", nil, 200)).Items
	}
	draft := func(currency, amount string, original *finance.Payment, quote *finance.PaymentRefundFX) finance.Payment {
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic foreign expense refund", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-02T12:00:00+08:00"), Currency: ptr(currency), Amount: ptr(amount), Category: "expense-refund134", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "synthetic statement", EvidenceVersionIDs: []string{file.Version.ID}, RefundFX: quote}, SourceKey: "refund134:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "actual cash fixture"}
		if original != nil {
			in.Direction = ptr("refund")
			in.RefundOfID = &original.ID
		}
		return data[finance.Payment](t, call("POST", "/v1/payments", in, 201))
	}
	record := func(p finance.Payment) finance.Payment {
		return data[finance.Payment](t, call("POST", "/v1/payments/"+p.ID+"/record", finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, 200))
	}
	cost := data[finance.Cost](t, call("POST", "/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic EUR expense", OccurredOn: ptr("2025-01-15"), Currency: ptr("EUR"), Amount: ptr("200"), Category: "expense-refund134", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: "150"}, {ProjectID: nil, Amount: "50"}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "refund134cost:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "incurred cost fixture"}, 201))
	cost = data[finance.Cost](t, call("POST", "/v1/costs/"+cost.ID+"/confirm", finance.Base{ExpectedVersion: cost.Version, RevisionID: cost.CurrentRevisionID, ContentHash: cost.ContentHash, Reason: "confirm actual expense"}, 200))
	p := record(draft("HKD", "1000", nil, nil))
	p = data[finance.Payment](t, call("POST", "/v1/payments/"+p.ID+"/allocations", finance.PaymentAllocate{Base: base(p), CostID: cost.ID, CostVersion: cost.Version, CostRevisionID: cost.CurrentRevisionID, CostContentHash: cost.ContentHash, Amount: "780", FX: &finance.PaymentFXInput{CostAmount: "200", Rate: "3.9", Source: "synthetic original EUR settlement", Acknowledged: true}}, 200))
	root := rows(p)[0]
	quote := &finance.PaymentRefundFX{OriginalCurrency: "HKD", OriginalAmount: "780", Rate: "0.1282051282", Source: "synthetic actual USD refund"}
	q := draft("USD", "100", &p, quote)
	q = data[finance.Payment](t, call("POST", "/v1/payments/"+q.ID+"/record", finance.PaymentRecordInput{Base: base(q), ExpectedSourceVersion: &p.Version, RefundFXAcknowledged: true, RefundAllocations: []finance.RefundAllocation{{SourceAllocationID: root.ID, Amount: "560", CostAmount: ptr("143.59")}}}, 200))
	_ = get
	check := func(want string, blocking bool) {
		t.Helper()
		var principal, currency string
		var block bool
		if err := f.owner.QueryRow(ctx, "SELECT payment_refund_unattributed_principal($1,$2)::numeric(20,6)::text,payment_refund_principal_currency($1,$2)", f.org, q.ID).Scan(&principal, &currency); err != nil {
			t.Fatal(err)
		}
		if principal != want || currency != "HKD" {
			t.Fatal("mixed refund units", principal, currency, want)
		}
		if err := f.owner.QueryRow(ctx, "SELECT blocking FROM project_closeout_items($1,$2) WHERE kind='payments' AND id=$3", f.org, *file.File.ProjectID, q.ID).Scan(&block); err != nil {
			t.Fatal(err)
		}
		if block != blocking {
			t.Fatal("wrong current project blocker", block, principal)
		}
	}
	t.Run("USD cash cannot hide outstanding HKD principal", func(t *testing.T) { check("220.000000", true) })
	t.Run("full ordinary attribution clears only the original principal", func(t *testing.T) {
		q = data[finance.Payment](t, call("POST", "/v1/payments/"+q.ID+"/refund-allocations", finance.PaymentRefundAllocate{Base: base(q), RefundAllocation: finance.RefundAllocation{SourceAllocationID: root.ID, Amount: "220", CostAmount: ptr("56.41")}, RefundFXAcknowledged: true}, 200))
		check("0.000000", false)
	})
}
