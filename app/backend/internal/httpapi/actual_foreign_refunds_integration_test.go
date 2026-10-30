package httpapi

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
)

func TestActualForeignRefunds(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(s string) *string { return &s }
	for _, key := range []string{"payment_recorder", "payment_reverser", "cash_reporter", "party_manager", "cost_recorder", "cost_confirmer"} {
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, key); err != nil {
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
	supplier := data[party.Party](t, call("POST", "/v1/parties", party.Create{Content: party.Content{Name: "Synthetic foreign refund supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "actual refund fixture"}, 201))
	base := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "review synthetic actual refund"}
	}
	get := func(id string) finance.Payment {
		return data[finance.Payment](t, call("GET", "/v1/payments/"+id, nil, 200))
	}
	draft := func(currency, amount string, original *finance.Payment, q *finance.PaymentRefundFX) finance.Payment {
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic actual foreign refund", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-02T12:00:00+08:00"), Currency: ptr(currency), Amount: ptr(amount), Category: "actual-refund121", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "synthetic statement", EvidenceVersionIDs: []string{file.Version.ID}, RefundFX: q}, SourceKey: "refund121:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "actual refund fixture"}
		if original != nil {
			in.Direction = ptr("refund")
			in.RefundOfID = &original.ID
		}
		return data[finance.Payment](t, call("POST", "/v1/payments", in, 201))
	}
	recordInput := func(p finance.Payment, original *finance.Payment, ack bool) finance.PaymentRecordInput {
		in := finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}, RefundFXAcknowledged: ack}
		if original != nil {
			in.ExpectedSourceVersion = &original.Version
		}
		return in
	}
	record := func(p finance.Payment, original *finance.Payment, ack bool) finance.Payment {
		return data[finance.Payment](t, call("POST", "/v1/payments/"+p.ID+"/record", recordInput(p, original, ack), 200))
	}
	reverse := func(p finance.Payment) finance.Payment {
		return data[finance.Payment](t, call("POST", "/v1/payments/"+p.ID+"/reverse", finance.PaymentReverseInput{Base: base(p), EffectiveAt: "2025-02-03T12:00:00+08:00"}, 200))
	}
	quote := func(principal, rate string) *finance.PaymentRefundFX {
		return &finance.PaymentRefundFX{OriginalCurrency: "HKD", OriginalAmount: principal, Rate: rate, Source: "synthetic actual refund statement"}
	}
	p := record(draft("HKD", "780", nil, nil), nil, false)
	var refund finance.Payment
	t.Run("fixed draft quote and fresh human acknowledgement are required", func(t *testing.T) {
		missing := draft("USD", "100", &p, nil)
		call("POST", "/v1/payments/"+missing.ID+"/record", recordInput(missing, &p, true), 422)
		refund = draft("USD", "100", &p, quote("780", "0.1282051282"))
		call("POST", "/v1/payments/"+refund.ID+"/record", recordInput(refund, &p, false), 422)
		if get(refund.ID).State != "draft" || get(p.ID).RefundedAmount != "0.000000" {
			t.Fatal("rejected confirmation changed cash")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM payment_events WHERE org_id=$1 AND payment_id=$2 AND refund_fx_acknowledged", f.org, refund.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal(count, err)
		}
	})
	t.Run("actual cash and original principal remain independent immutable facts", func(t *testing.T) {
		refund = record(refund, &p, true)
		current := get(p.ID)
		if refund.CanAllocate || refund.RefundFX == nil || refund.RefundFX.OriginalAmount != "780.000000" || *refund.Amount != "100.000000" || *refund.Currency != "USD" || current.RefundedAmount != "780.000000" || *current.UnallocatedAmount != "0.000000" || current.ContentHash != p.ContentHash {
			t.Fatal(refund, current)
		}
		legacyHistory := data[finance.PaymentRevision](t, call("GET", "/v1/payments/"+p.ID+"/revisions/"+p.CurrentRevisionID, nil, 200))
		foreignHistory := data[finance.PaymentRevision](t, call("GET", "/v1/payments/"+refund.ID+"/revisions/"+refund.CurrentRevisionID, nil, 200))
		if legacyHistory.RefundFX != nil || foreignHistory.RefundFX == nil || *foreignHistory.RefundFX != *refund.RefundFX {
			t.Fatal("history must preserve optional legacy and fixed foreign refund evidence")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM payment_events WHERE org_id=$1 AND payment_id=$2 AND kind='recorded' AND refund_fx_acknowledged AND mfa_verified_at IS NOT NULL", f.org, refund.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
		report := data[finance.CashReport](t, call("GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&category=actual-refund121", nil, 200))
		cash := map[string]finance.CashTotal{}
		for _, v := range report.Totals {
			cash[v.Currency] = v
		}
		if cash["HKD"].OutgoingAmount != "780.000000" || cash["USD"].RefundAmount != "100.000000" || cash["USD"].NetAmount != "-100.000000" {
			t.Fatal(report)
		}
		extra := draft("HKD", "0.01", &p, nil)
		call("POST", "/v1/payments/"+extra.ID+"/record", recordInput(extra, &p, false), 409)
		call("POST", "/v1/payments/"+p.ID+"/reverse", finance.PaymentReverseInput{Base: base(p), EffectiveAt: "2025-02-03T12:00:00+08:00"}, 409)
	})
	t.Run("refund reversal restores original principal and preserves actual cash quote", func(t *testing.T) {
		old := refund
		refund = reverse(refund)
		current := get(p.ID)
		inverse := get(*refund.ReversalID)
		if current.RefundedAmount != "0.000000" || *current.UnallocatedAmount != "780.000000" || inverse.RefundFX == nil || *inverse.Amount != *old.Amount || inverse.RefundFX.OriginalAmount != old.RefundFX.OriginalAmount || inverse.RefundFX.Rate != old.RefundFX.Rate || get(old.ID).ContentHash != old.ContentHash {
			t.Fatal(current, inverse)
		}
	})
	t.Run("same and foreign currency refunds share one exact principal cap", func(t *testing.T) {
		same := record(draft("HKD", "390", &p, nil), &p, false)
		foreign := record(draft("USD", "50", &p, quote("390", "0.1282051282")), &p, true)
		if get(p.ID).RefundedAmount != "780.000000" {
			t.Fatal(get(p.ID))
		}
		reverse(foreign)
		reverse(same)
	})
	t.Run("allocated principal cannot be refunded and cross-unit attribution is blocked", func(t *testing.T) {
		cash := record(draft("HKD", "100", nil, nil), nil, false)
		c := data[finance.Cost](t, call("POST", "/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic refundable service", OccurredOn: ptr("2025-01-15"), Currency: ptr("HKD"), Amount: ptr("100"), Category: "refund121", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: "100"}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "refund121cost:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic incurred cost"}, 201))
		c = data[finance.Cost](t, call("POST", "/v1/costs/"+c.ID+"/confirm", finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "confirmed actual expense"}, 200))
		cash = data[finance.Payment](t, call("POST", "/v1/payments/"+cash.ID+"/allocations", finance.PaymentAllocate{Base: base(cash), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: "100"}, 200))
		rows := data[core.Page[finance.PaymentAllocation]](t, call("GET", "/v1/payments/"+cash.ID+"/allocations?limit=100", nil, 200)).Items
		q := draft("USD", "10", &cash, quote("80", "0.125"))
		in := recordInput(q, &cash, true)
		call("POST", "/v1/payments/"+q.ID+"/record", in, 409)
		in.RefundAllocations = []finance.RefundAllocation{{SourceAllocationID: rows[0].ID, Amount: "80"}}
		call("POST", "/v1/payments/"+q.ID+"/record", in, 409)
		cash = data[finance.Payment](t, call("POST", "/v1/payments/"+cash.ID+"/allocations/"+rows[0].ID+"/release", finance.PaymentRelease{Base: base(cash), Amount: "100"}, 200))
		q = record(q, &cash, true)
		call("POST", "/v1/payments/"+q.ID+"/refund-allocations", finance.PaymentRefundAllocate{Base: base(q), RefundAllocation: finance.RefundAllocation{SourceAllocationID: rows[0].ID, Amount: "10"}}, 409)
		current := data[finance.Cost](t, call("GET", "/v1/costs/"+c.ID, nil, 200))
		if current.ContentHash != c.ContentHash || valueForTest(current.PaidAmount) != "0.000000" {
			t.Fatal(current)
		}
	})

	t.Run("concurrent foreign refunds serialize against original principal", func(t *testing.T) {
		first := draft("USD", "50", &p, quote("400", "0.125"))
		second := draft("USD", "50", &p, quote("400", "0.125"))
		var wg sync.WaitGroup
		statuses := make(chan int, 2)
		for _, q := range []finance.Payment{first, second} {
			wg.Add(1)
			go func(q finance.Payment) {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/payments/"+q.ID+"/record", f.aliceToken, core.NewID(), recordInput(q, &p, true), nil)
				statuses <- r.Code
			}(q)
		}
		wg.Wait()
		close(statuses)
		counts := map[int]int{}
		for code := range statuses {
			counts[code]++
		}
		if counts[200] != 1 || counts[409] != 1 || get(p.ID).RefundedAmount != "400.000000" {
			t.Fatal(counts, get(p.ID))
		}
		for _, q := range []finance.Payment{first, second} {
			if current := get(q.ID); current.State == "recorded" {
				reverse(current)
			}
		}
	})
	t.Run("database quote guard and audit failure preserve every cash fact", func(t *testing.T) {
		q := draft("USD", "100", &p, quote("780", "0.1282051282"))
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO payment_revisions(id,org_id,payment_id,seq,base_revision_id,payload,content_hash,paid_at,paid_on,author_id,reason)
   SELECT gen_random_uuid(),org_id,payment_id,seq+1,id,jsonb_set(payload,'{refund_fx,rate}','"0.12"'),content_hash,paid_at,paid_on,author_id,'forged quote' FROM payment_revisions WHERE org_id=$1 AND id=$2`, f.org, q.CurrentRevisionID)
		tx.Rollback(ctx)
		if e == nil || !strings.Contains(e.Error(), "cash equation") {
			t.Fatal("SQL bypass accepted invalid refund quote", e)
		}
		if _, e = f.owner.Exec(ctx, `CREATE FUNCTION reject_refund_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='payment.recorded' THEN RAISE EXCEPTION 'isolated refund audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_refund_test_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_refund_test_audit()`); e != nil {
			t.Fatal(e)
		}
		call("POST", "/v1/payments/"+q.ID+"/record", recordInput(q, &p, true), 500)
		if _, e = f.owner.Exec(ctx, "DROP TRIGGER reject_refund_test_audit ON audit_events; DROP FUNCTION reject_refund_test_audit()"); e != nil {
			t.Fatal(e)
		}
		if get(q.ID).Version != q.Version || get(q.ID).State != "draft" || get(p.ID).RefundedAmount != "0.000000" {
			t.Fatal("failed audit left cash facts")
		}
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.owner.Exec(ctx, "UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1", actor.SessionID); e != nil {
			t.Fatal(e)
		}
		r, _ := f.with(t).request("POST", "/v1/payments/"+q.ID+"/record", f.aliceToken, core.NewID(), recordInput(q, &p, true), nil)
		if r.Code < 400 {
			t.Fatal("revoked session recorded foreign cash")
		}
		if _, e = f.owner.Exec(ctx, "UPDATE sessions SET revoked_at=NULL WHERE id=$1", actor.SessionID); e != nil {
			t.Fatal(e)
		}
		r, _ = f.with(t).request("GET", "/v1/payments/"+q.ID, f.bobToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("unrelated employee disclosed cash")
		}
	})

	if os.Getenv("CP_BROWSER_REFUND_FX_FIXTURE") == "1" {
		cash := record(draft("HKD", "780", nil, nil), nil, false)
		preview := draft("USD", "100", &cash, quote("780", "0.1282051282"))
		var name string
		if e := f.owner.QueryRow(ctx, "SELECT name FROM projects WHERE id=$1", file.File.ProjectID).Scan(&name); e != nil {
			t.Fatal(e)
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "actual-foreign-refund", "refund_fx_fixture": true, "payment": cash, "refund": preview, "project_id": file.File.ProjectID, "project_name": name})
	}

}
