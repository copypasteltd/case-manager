package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
)

func TestPaymentFeeEvidence(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(v string) *string { return &v }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"payment_recorder", "payment_reverser", "cash_reporter", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	readActor := func() core.Actor {
		a, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	call := func(t *testing.T, method, path string, body any, status int) response {
		t.Helper()
		r, v := f.with(t).request(method, path, f.aliceToken, core.NewID(), body, nil)
		expect(t, r, status, v)
		return v
	}
	supplier := data[party.Party](t, call(t, "POST", "/v1/parties", party.Create{Content: party.Content{Name: "Synthetic payment fee provider", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated fee evidence"}, 201))
	ref := &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}
	base := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "review actual fee evidence"}
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "confirm original incurred fee"}
	}
	get := func(t *testing.T, id string) finance.Payment {
		return data[finance.Payment](t, call(t, "GET", "/v1/payments/"+id, nil, 200))
	}
	newPayment := func(t *testing.T, amount string) finance.Payment {
		t.Helper()
		p := data[finance.Payment](t, call(t, "POST", "/v1/payments", finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic actual bank charge", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-02T12:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "fee118", Counterparty: ref, MethodLabel: "isolated bank statement", EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "fee-cash:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "review actual cash"}, 201))
		return data[finance.Payment](t, call(t, "POST", "/v1/payments/"+p.ID+"/record", finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, 200))
	}
	newSource := func(t *testing.T) (finance.Payment, finance.Cost, finance.PaymentAllocation) {
		t.Helper()
		c := data[finance.Cost](t, call(t, "POST", "/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic incurred payment fee", OccurredOn: ptr("2025-01-15"), Currency: ptr("USD"), Amount: ptr("10.00"), Category: "bank_fee", Counterparty: ref, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: "10.00"}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "fee-cost:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "review original fee"}, 201))
		c = data[finance.Cost](t, call(t, "POST", "/v1/costs/"+c.ID+"/confirm", costBase(c), 200))
		p := newPayment(t, "78.00")
		p = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+p.ID+"/allocations", finance.PaymentAllocate{Base: base(p), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: "78.00", FX: &finance.PaymentFXInput{CostAmount: "10.00", Rate: "7.8", Source: "reviewed actual fee settlement", Acknowledged: true}}, 200))
		a := data[core.Page[finance.PaymentAllocation]](t, call(t, "GET", "/v1/payments/"+p.ID+"/allocations?limit=100", nil, 200)).Items[0]
		return p, c, a
	}
	preview := func(t *testing.T, p, fee finance.Payment, a finance.PaymentAllocation) finance.PaymentFeePreview {
		return data[finance.PaymentFeePreview](t, call(t, "GET", "/v1/payments/"+p.ID+"/fees/basis?fee_payment_id="+fee.ID+"&allocation_id="+a.ID, nil, 200))
	}
	input := func(p, fee finance.Payment, a finance.PaymentAllocation, v finance.PaymentFeePreview) finance.PaymentFeeCreate {
		return finance.PaymentFeeCreate{Base: base(p), ExpectedFeeVersion: v.FeeVersion, FeePaymentID: fee.ID, AllocationID: a.ID, BasisSHA256: v.BasisSHA256, Reviewed: true}
	}
	ledger := func(t *testing.T, p finance.Payment) finance.PaymentFeeLedger {
		return data[finance.PaymentFeeLedger](t, call(t, "GET", "/v1/payments/"+p.ID+"/fees?limit=100", nil, 200))
	}
	facts := func() string {
		var v string
		e := f.owner.QueryRow(ctx, `SELECT jsonb_build_object('costs',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM actual_cost_entries x WHERE org_id=$1),'cost_revisions',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM cost_revisions x WHERE org_id=$1),'cash',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM payment_records x WHERE org_id=$1),'cash_revisions',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM payment_revisions x WHERE org_id=$1),'allocations',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM payment_allocations x WHERE org_id=$1),'shares',(SELECT jsonb_agg(to_jsonb(x) ORDER BY allocation_id,position) FROM payment_allocation_shares x WHERE org_id=$1))::text`, f.org).Scan(&v)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	t.Run("exact independent facts and human revocation preserve monetary ledgers", func(t *testing.T) {
		p := newPayment(t, "780.00")
		fee, _, a := newSource(t)
		v := preview(t, p, fee, a)
		var b finance.PaymentFeeBasis
		if json.Unmarshal([]byte(v.Basis), &b) != nil || core.Digest([]byte(v.Basis)) != v.BasisSHA256 || b.Allocation.ID != a.ID || b.FeePayment.ID != fee.ID || b.Payment.ID != p.ID {
			t.Fatal("wrong complete basis")
		}
		before := facts()
		in := input(p, fee, a, v)
		key := core.NewID()
		path := "/v1/payments/" + p.ID + "/fees"
		r, res := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, res)
		saved := data[finance.PaymentFeeLink](t, res)
		if get(t, p.ID).Version != p.Version || saved.Basis != v.Basis || saved.BasisSHA256 != v.BasisSHA256 || saved.Revocation != nil || saved.Version != 1 || facts() != before {
			t.Fatal("classification changed original money or basis")
		}
		r, res = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, res)
		if data[finance.PaymentFeeLink](t, res).ID != saved.ID || len(ledger(t, p).Items) != 1 {
			t.Fatal("replay duplicated link")
		}
		l := ledger(t, p)
		if l.FeeVersion != 1 || l.Items[0].State != "active" || l.Items[0].Current.RemainingAmount != a.RemainingAmount {
			t.Fatal("wrong current status")
		}
		revoke := finance.PaymentFeeRevoke{Base: base(p), ExpectedFeeVersion: l.FeeVersion, BasisSHA256: saved.BasisSHA256, Reviewed: true}
		revoked := data[finance.PaymentFeeLink](t, call(t, "POST", path+"/"+saved.ID+"/revoke", revoke, 200))
		if revoked.Revocation == nil || revoked.Revocation.Version != 2 || revoked.Basis != saved.Basis || facts() != before || ledger(t, p).Items[0].State != "revoked" {
			t.Fatal("revocation rewrote facts or lost history")
		}
		v = preview(t, p, fee, a)
		again := data[finance.PaymentFeeLink](t, call(t, "POST", path, input(p, fee, a, v), 201))
		if again.ID == saved.ID || again.Version != 3 || len(ledger(t, p).Items) != 2 {
			t.Fatal("explicit correction did not retain previous version")
		}
	})
	t.Run("same-payment fee is a selected existing allocation", func(t *testing.T) {
		fee, _, a := newSource(t)
		v := preview(t, fee, fee, a)
		before := facts()
		call(t, "POST", "/v1/payments/"+fee.ID+"/fees", input(fee, fee, a, v), 201)
		if facts() != before {
			t.Fatal("same-payment fee added cash")
		}
	})
	t.Run("review MFA hash and immutable evidence are enforced", func(t *testing.T) {
		p := newPayment(t, "780.00")
		fee, _, a := newSource(t)
		v := preview(t, p, fee, a)
		in := input(p, fee, a, v)
		path := "/v1/payments/" + p.ID + "/fees"
		before := facts()
		in.Reviewed = false
		call(t, "POST", path, in, 422)
		in.Reviewed = true
		in.BasisSHA256 = strings.Repeat("0", 64)
		call(t, "POST", path, in, 409)
		in.BasisSHA256 = v.BasisSHA256
		actor := readActor()
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		call(t, "POST", path, in, 403)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		saved := data[finance.PaymentFeeLink](t, call(t, "POST", path, in, 201))
		if facts() != before {
			t.Fatal("invalid requests changed money")
		}
		for _, q := range []string{"UPDATE payment_fee_links SET reason='rewrite' WHERE id=$1", "DELETE FROM payment_fee_links WHERE id=$1"} {
			if _, e := f.owner.Exec(ctx, q, saved.ID); e == nil {
				t.Fatal("immutable evidence changed")
			}
		}
	})
	t.Run("changed live balances invalidate a prepared classification", func(t *testing.T) {
		p := newPayment(t, "780.00")
		fee, _, a := newSource(t)
		v := preview(t, p, fee, a)
		fee = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+fee.ID+"/allocations/"+a.ID+"/release", finance.PaymentRelease{Base: base(fee), Amount: "39.00", CostAmount: ptr("5.00")}, 200))
		call(t, "POST", "/v1/payments/"+p.ID+"/fees", input(p, fee, a, v), 409)
		if ledger(t, p).FeeVersion != 0 {
			t.Fatal("stale basis wrote a fee")
		}
		v = preview(t, p, fee, a)
		call(t, "POST", "/v1/payments/"+p.ID+"/fees", input(p, fee, a, v), 201)
		fee = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+fee.ID+"/allocations/"+a.ID+"/release", finance.PaymentRelease{Base: base(fee), Amount: "39.00", CostAmount: ptr("5.00")}, 200))
		l := ledger(t, p)
		if l.Items[0].State != "exhausted" || !strings.HasPrefix(l.Items[0].Current.RemainingAmount, "0.") {
			t.Fatal("exhausted original falsely remained paid")
		}
	})
	t.Run("concurrent principals cannot classify one root twice", func(t *testing.T) {
		p, q := newPayment(t, "780.00"), newPayment(t, "780.00")
		fee, _, a := newSource(t)
		pv, qv := preview(t, p, fee, a), preview(t, q, fee, a)
		statuses := make(chan int, 2)
		var wg sync.WaitGroup
		for _, x := range []struct {
			p finance.Payment
			v finance.PaymentFeePreview
		}{{p, pv}, {q, qv}} {
			wg.Add(1)
			go func(p finance.Payment, v finance.PaymentFeePreview) {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/payments/"+p.ID+"/fees", f.aliceToken, core.NewID(), input(p, fee, a, v), nil)
				statuses <- r.Code
			}(x.p, x.v)
		}
		wg.Wait()
		close(statuses)
		seen := map[int]int{}
		for s := range statuses {
			seen[s]++
		}
		if seen[201] != 1 || seen[409] != 1 {
			t.Fatalf("unexpected concurrent statuses %v", seen)
		}
	})
	t.Run("current authority pagination and audit rollback remain real", func(t *testing.T) {
		p := newPayment(t, "780.00")
		fee, _, a := newSource(t)
		v := preview(t, p, fee, a)
		path := "/v1/payments/" + p.ID + "/fees"
		exec(`CREATE FUNCTION reject_fee_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='payment.fee.linked' THEN RAISE EXCEPTION 'isolated fee audit failure'; END IF; RETURN NEW; END $$`)
		exec(`CREATE TRIGGER reject_fee_test_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_fee_test_audit()`)
		call(t, "POST", path, input(p, fee, a, v), 500)
		exec("DROP TRIGGER reject_fee_test_audit ON audit_events")
		exec("DROP FUNCTION reject_fee_test_audit()")
		if ledger(t, p).FeeVersion != 0 {
			t.Fatal("failed audit left fee metadata")
		}
		call(t, "POST", path, input(p, fee, a, v), 201)
		fee2, _, a2 := newSource(t)
		v2 := preview(t, p, fee2, a2)
		second := data[finance.PaymentFeeLink](t, call(t, "POST", path, input(p, fee2, a2, v2), 201))
		firstPage := data[finance.PaymentFeeLedger](t, call(t, "GET", path+"?limit=1", nil, 200))
		if len(firstPage.Items) != 1 || firstPage.NextCursor == nil || firstPage.Items[0].Link.ID != second.ID {
			t.Fatal("bounded history omitted its exact next cursor")
		}
		next := data[finance.PaymentFeeLedger](t, call(t, "GET", path+"?limit=1&cursor="+*firstPage.NextCursor, nil, 200))
		if len(next.Items) != 1 || next.NextCursor != nil || next.Items[0].Link.ID == second.ID || next.FeeVersion != firstPage.FeeVersion {
			t.Fatal("pinned history duplicated or lost an entry")
		}
		other := newPayment(t, "780.00")
		call(t, "GET", "/v1/payments/"+other.ID+"/fees?limit=1&cursor="+*firstPage.NextCursor, nil, 400)
		call(t, "POST", path+"/"+second.ID+"/revoke", finance.PaymentFeeRevoke{Base: base(p), ExpectedFeeVersion: firstPage.FeeVersion, BasisSHA256: second.BasisSHA256, Reviewed: true}, 200)
		call(t, "GET", path+"?limit=1&cursor="+*firstPage.NextCursor, nil, 400)
		r, res := f.with(t).request("GET", path+"?limit=20", f.bobToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatalf("unauthorized reader got %v", res)
		}
		actor := readActor()
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		r, _ = f.with(t).request("GET", path+"?limit=20", f.aliceToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("revoked human still reads fee ledger")
		}
		exec("UPDATE sessions SET revoked_at=NULL WHERE id=$1", actor.SessionID)
	})
	if os.Getenv("CP_BROWSER_PAYMENT_FEE_FIXTURE") == "1" {
		p := newPayment(t, "780.00")
		fee, c, a := newSource(t)
		var name string
		if e := f.owner.QueryRow(ctx, "SELECT name FROM projects WHERE id=$1", file.File.ProjectID).Scan(&name); e != nil {
			t.Fatal(e)
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "payment-fee-evidence", "payment_fee_fixture": true, "payment": p, "fee_payment": fee, "cost": c, "allocation": a, "project_id": file.File.ProjectID, "project_name": name})
	}
}
