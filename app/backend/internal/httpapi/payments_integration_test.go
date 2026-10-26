package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func TestPaymentsAndSettlement(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	ptr := func(s string) *string { return &s }
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
	admin := f.administratorToken()
	input := func(category, amount string) finance.PaymentCreate {
		return finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic actual cash", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-01T02:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr(amount), Category: category, MethodLabel: "Synthetic transfer label", EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "bank:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated actual cash proof"}
	}
	base := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "human verifies exact cash snapshot"}
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "exact settlement source"}
	}
	create := func(t *testing.T, in finance.PaymentCreate) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[finance.Payment](t, v)
	}
	record := func(t *testing.T, p finance.Payment) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	get := func(t *testing.T, id string) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/payments/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	allocations := func(t *testing.T, id string) []finance.PaymentAllocation {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/payments/"+id+"/allocations?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[finance.PaymentAllocation]](t, v).Items
	}
	var supplier party.Party
	var recorder string
	t.Run("cash roles start unbound and system administration does not grant business access", func(t *testing.T) {
		in := input("unbound", "100")
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 404, v)
		recorder = bind(f.alice, "payment_recorder")
		for _, key := range []string{"payment_reverser", "cash_reporter", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "claim_submitter", "request_manager"} {
			bind(f.alice, key)
		}
		bind(f.bob, "payment_reader")
		bind(f.bob, "cost_reader")
		bind(f.bob, "claim_reader")
		bind(f.bob, "approval_reviewer")
		bind(f.bob, "party_manager")
		f.aliceToken = memberMFAToken(f, "alice", "android")
		r, v = f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic settlement supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated cash supplier"}, nil)
		expect(t, r, 201, v)
		supplier = data[party.Party](t, v)
		in.PaymentContent = finance.PaymentContent{Title: "Incomplete cash draft", EvidenceVersionIDs: []string{}}
		p := create(t, in)
		if p.State != "draft" || p.UnallocatedAmount != nil {
			t.Fatal(p)
		}
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID, admin, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 422, v)
	})
	full := func(category, amount string) finance.PaymentCreate {
		in := input(category, amount)
		in.Counterparty = &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}
		return in
	}
	newCost := func(t *testing.T, category, amount string, shares []finance.Allocation) finance.Cost {
		t.Helper()
		if shares == nil {
			shares = []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}
		}
		in := finance.Create{Content: finance.Content{Title: "Synthetic settled cost", OccurredOn: ptr("2025-01-15"), Currency: ptr("HKD"), Amount: ptr(amount), Category: category, Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: shares, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "cash-cost:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated incurred basis"}
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		c := data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), costBase(c), nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	allocInput := func(p finance.Payment, c finance.Cost, amount string) finance.PaymentAllocate {
		return finance.PaymentAllocate{Base: base(p), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: amount}
	}
	allocate := func(t *testing.T, p finance.Payment, c finance.Cost, amount string) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocInput(p, c, amount), nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	reverse := func(t *testing.T, p finance.Payment, at string) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: base(p), EffectiveAt: at}, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	cash := func(t *testing.T, category, from, to, extra string) finance.CashReport {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/reports/cash?category="+category+"&from="+from+"&to="+to+extra, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.CashReport](t, v)
	}
	t.Run("draft revisions fixed history source deduplication and stale bases", func(t *testing.T) {
		in := full("draft", "100")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		p := data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[finance.Payment](t, v).ID != p.ID {
			t.Fatal("replay created cash twice")
		}
		exec(t, "DELETE FROM idempotency_records WHERE idempotency_key=$1", key)
		r, v = f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		old := p
		content := p.PaymentContent
		content.Title = "Updated cash draft"
		r, v = f.with(t).request("PATCH", "/v1/payments/"+p.ID, f.aliceToken, core.NewID(), finance.PaymentPatch{Base: base(p), PaymentContent: content}, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		r, v = f.with(t).request("PATCH", "/v1/payments/"+p.ID, f.aliceToken, core.NewID(), finance.PaymentPatch{Base: base(old), PaymentContent: content}, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID+"/revisions/"+old.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.PaymentRevision](t, v).Title != old.Title {
			t.Fatal("mutable old revision")
		}
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID+"/revisions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[finance.PaymentRevision]](t, v)
		if page.NextCursor == nil {
			t.Fatal(page)
		}
		other := create(t, full("other", "1"))
		r, v = f.with(t).request("GET", "/v1/payments/"+other.ID+"/revisions?cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
		p = record(t, p)
		r, v = f.with(t).request("PATCH", "/v1/payments/"+p.ID, f.aliceToken, core.NewID(), finance.PaymentPatch{Base: base(p), PaymentContent: content}, nil)
		expect(t, r, 409, v)
		if p.PaidOn == nil || *p.PaidOn != "2025-02-01" || valueForTest(p.PaidAt) != "2025-01-31T18:00:00Z" || len(p.Attachments) != 1 {
			t.Fatal(p)
		}
	})
	t.Run("actual recording rejects future amounts missing evidence and invalid direction recipients", func(t *testing.T) {
		for _, amount := range []string{"0.001", "-1", "1e3"} {
			r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), full("invalid", amount), nil)
			expect(t, r, 422, v)
		}
		for _, kind := range []string{"future", "zero", "no-evidence", "transfer-recipient", "refund-parent"} {
			in := full("invalid-"+kind, "1")
			switch kind {
			case "future":
				in.PaidAt = ptr(time.Now().Add(time.Hour).Format(time.RFC3339))
			case "zero":
				in.Amount = ptr("0")
			case "no-evidence":
				in.EvidenceVersionIDs = []string{}
			case "transfer-recipient":
				in.Direction = ptr("transfer")
			case "refund-parent":
				in.Direction = ptr("refund")
			}
			p := create(t, in)
			r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, nil)
			expect(t, r, 422, v)
		}
	})
	t.Run("partial cash allocation and release preserve incurred totals and prevent premature cost reversal", func(t *testing.T) {
		c := newCost(t, "partial", "100", nil)
		p := record(t, create(t, full("partial", "100")))
		p = allocate(t, p, c, "60")
		if p.EffectiveAllocatedAmount != "60.000000" || valueForTest(p.UnallocatedAmount) != "40.000000" {
			t.Fatal(p)
		}
		r, v := f.with(t).request("GET", "/v1/costs/"+c.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		c = data[finance.Cost](t, v)
		if valueForTest(c.PaidAmount) != "60.000000" || valueForTest(c.OutstandingAmount) != "40.000000" || c.CanReverse {
			t.Fatal(c)
		}
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: costBase(c), EffectiveOn: "2025-02-02"}, nil)
		expect(t, r, 409, v)
		a := allocations(t, p.ID)[0]
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations/"+a.ID+"/release", f.aliceToken, core.NewID(), finance.PaymentRelease{Base: base(p), Amount: "20"}, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		if p.EffectiveAllocatedAmount != "40.000000" || len(allocations(t, p.ID)) != 2 {
			t.Fatal(p)
		}
		totals := cash(t, "partial", "2025-02-01", "2025-03-01", "")
		if len(totals.Totals) != 1 || totals.Totals[0].NetAmount != "100.000000" || totals.Totals[0].UnallocatedAmount != "60.000000" {
			t.Fatal(totals)
		}
		totals = cash(t, "partial", "2025-02-01", "2025-03-01", "&project_id="+*file.File.ProjectID)
		if totals.Totals[0].NetAmount != "40.000000" {
			t.Fatal(totals)
		}
		p = reverse(t, p, "2025-03-01T01:00:00+08:00")
		if p.State != "reversed" || p.ReversalID == nil {
			t.Fatal(p)
		}
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		c = data[finance.Cost](t, v)
		if valueForTest(c.PaidAmount) != "0.000000" || !c.CanReverse {
			t.Fatal(c)
		}
		totals = cash(t, "partial", "2025-03-01", "2025-04-01", "")
		if totals.Totals[0].NetAmount != "-100.000000" || totals.Totals[0].CorrectionAmount != "-100.000000" {
			t.Fatal(totals)
		}
	})
	t.Run("successive smallest payments and partial releases conserve fixed project shares", func(t *testing.T) {
		c := newCost(t, "rounding", "0.03", []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: "0.01"}, {Amount: "0.02"}})
		p := record(t, create(t, full("rounding", "0.03")))
		for i := 0; i < 3; i++ {
			p = allocate(t, p, c, "0.01")
		}
		var project, company core.Decimal
		for _, a := range allocations(t, p.ID) {
			for _, s := range a.Shares {
				v, _ := core.ParseDecimal(s.Amount)
				if s.ProjectID == nil {
					company = company.Add(v)
				} else {
					project = project.Add(v)
				}
			}
		}
		if project.String() != "0.010000" || company.String() != "0.020000" {
			t.Fatal(project.String(), company.String())
		}
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocInput(p, c, "0.01"), nil)
		expect(t, r, 409, v)
	})
	t.Run("two payments cannot concurrently overpay one cost", func(t *testing.T) {
		c := newCost(t, "race-cost", "100", nil)
		payments := []finance.Payment{record(t, create(t, full("race-cost", "100"))), record(t, create(t, full("race-cost", "100")))}
		start := make(chan struct{})
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for _, p := range payments {
			wg.Add(1)
			go func(p finance.Payment) {
				defer wg.Done()
				<-start
				r, _ := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocInput(p, c, "100"), nil)
				codes <- r.Code
			}(p)
		}
		close(start)
		wg.Wait()
		close(codes)
		success, conflict := 0, 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code == 409 {
				conflict++
			} else {
				t.Fatal(code)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal(success, conflict)
		}
	})
	t.Run("one cash head cannot concurrently allocate beyond its remaining balance", func(t *testing.T) {
		p := record(t, create(t, full("race-payment", "100")))
		costs := []finance.Cost{newCost(t, "race-payment", "100", nil), newCost(t, "race-payment", "100", nil)}
		start := make(chan struct{})
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for _, c := range costs {
			wg.Add(1)
			go func(c finance.Cost) {
				defer wg.Done()
				<-start
				r, _ := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocInput(p, c, "100"), nil)
				codes <- r.Code
			}(c)
		}
		close(start)
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code != 409 {
				t.Fatal(code)
			}
		}
		if success != 1 {
			t.Fatal(success)
		}
	})
	t.Run("refund records atomically release original allocations and full reversal restores them", func(t *testing.T) {
		c := newCost(t, "refund", "100", nil)
		original := allocate(t, record(t, create(t, full("refund", "100"))), c, "100")
		source := allocations(t, original.ID)[0]
		in := full("refund", "20")
		in.Direction = ptr("refund")
		in.PaidAt = ptr("2025-02-02T00:00:00Z")
		in.RefundOfID = &original.ID
		refund := create(t, in)
		recordInput := finance.PaymentRecordInput{Base: base(refund), ExpectedSourceVersion: &original.Version, RefundAllocations: []finance.RefundAllocation{}}
		r, v := f.with(t).request("POST", "/v1/payments/"+refund.ID+"/record", f.aliceToken, core.NewID(), recordInput, nil)
		expect(t, r, 409, v)
		recordInput.RefundAllocations = []finance.RefundAllocation{{SourceAllocationID: source.ID, Amount: "20"}}
		r, v = f.with(t).request("POST", "/v1/payments/"+refund.ID+"/record", f.aliceToken, core.NewID(), recordInput, nil)
		expect(t, r, 200, v)
		refund = data[finance.Payment](t, v)
		current := get(t, original.ID)
		if current.RefundedAmount != "20.000000" || current.EffectiveAllocatedAmount != "80.000000" || valueForTest(current.UnallocatedAmount) != "0.000000" || current.CanReverse {
			t.Fatal(current)
		}
		r, v = f.with(t).request("POST", "/v1/payments/"+current.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: base(current), EffectiveAt: "2025-03-01T00:00:00Z"}, nil)
		expect(t, r, 409, v)
		totals := cash(t, "refund", "2025-02-01", "2025-03-01", "")
		if totals.Totals[0].NetAmount != "80.000000" || totals.Totals[0].RefundAmount != "20.000000" || totals.Totals[0].UnallocatedAmount != "0.000000" {
			t.Fatal(totals)
		}
		refund = reverse(t, refund, "2025-03-01T00:00:00Z")
		current = get(t, original.ID)
		if current.RefundedAmount != "0.000000" || current.EffectiveAllocatedAmount != "100.000000" || !current.CanReverse {
			t.Fatal(current)
		}
		original = reverse(t, current, "2025-03-02T00:00:00Z")
		if original.State != "reversed" {
			t.Fatal(original)
		}
	})
	t.Run("unallocated refund can later pin an original allocation without over-releasing it", func(t *testing.T) {
		c := newCost(t, "late-refund", "100", nil)
		original := allocate(t, record(t, create(t, full("late-refund", "100"))), c, "60")
		source := allocations(t, original.ID)[0]
		in := full("late-refund", "20")
		in.Direction = ptr("refund")
		in.PaidAt = ptr("2025-02-02T00:00:00Z")
		in.RefundOfID = &original.ID
		p := create(t, in)
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), ExpectedSourceVersion: &original.Version, RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/refund-allocations", f.aliceToken, core.NewID(), finance.PaymentRefundAllocate{Base: base(p), RefundAllocation: finance.RefundAllocation{SourceAllocationID: source.ID, Amount: "20"}}, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/refund-allocations", f.aliceToken, core.NewID(), finance.PaymentRefundAllocate{Base: base(p), RefundAllocation: finance.RefundAllocation{SourceAllocationID: source.ID, Amount: "0.01"}}, nil)
		expect(t, r, 409, v)
		current := get(t, original.ID)
		if current.EffectiveAllocatedAmount != "40.000000" || valueForTest(current.UnallocatedAmount) != "40.000000" {
			t.Fatal(current)
		}
	})
	t.Run("current MFA and agent exclusion are evaluated before recorded replay", func(t *testing.T) {
		p := create(t, full("mfa", "1"))
		in := finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		actual := actor.MFAVerifiedAt
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		defer exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		raw, _ := json.Marshal(in)
		svc := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		cmd := core.Command{Actor: actor, Key: key, RequestID: core.NewID(), Route: "POST /v1/payments/" + p.ID + "/record", Body: raw}
		if _, err = svc.RecordPayment(ctx, cmd, p.ID, in); core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		cmd.Actor.RunID = core.NewID()
		if _, err = svc.RecordPayment(ctx, cmd, p.ID, in); err == nil {
			t.Fatal("agent recorded cash")
		}
	})
	t.Run("cash facts and fixed file references are immutable and require actual DB session", func(t *testing.T) {
		p := record(t, create(t, full("sql-guards", "1")))
		for _, q := range []string{"UPDATE payment_revisions SET reason='rewrite' WHERE id=$1", "DELETE FROM payment_revision_files WHERE revision_id=$1", "DELETE FROM payment_revisions WHERE id=$1"} {
			if _, err := f.api.DB.Exec(ctx, q, p.CurrentRevisionID); err == nil {
				t.Fatal(q)
			}
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='payment_revision' AND owner_id=$1 AND version_id=$2", p.CurrentRevisionID, file.Version.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE payment_records SET state='reversed',version=version+1 WHERE id=$1", p.ID); err == nil {
			t.Fatal("detached cash reversal")
		}
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		for _, user := range []string{f.alice, f.bob} {
			tx, err := f.api.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO payment_events(id,org_id,payment_id,payment_version,revision_id,kind,state,actor_id,session_id,mfa_verified_at,reason) SELECT $1,$2,$3,3,$4,'reversed','reversed',$5,s.id,s.mfa_verified_at+interval '1 second','forged session rejected' FROM sessions s WHERE s.id=$6`, core.NewID(), f.org, p.ID, p.CurrentRevisionID, user, actor.SessionID)
			tx.Rollback(ctx)
			if err == nil || !strings.Contains(err.Error(), "actual active human session") {
				t.Fatal(err)
			}
		}
	})
	t.Run("audit rejection rolls back allocation balances journal and replay receipt", func(t *testing.T) {
		c := newCost(t, "audit", "100", nil)
		p := record(t, create(t, full("audit", "100")))
		exec(t, `CREATE FUNCTION fail_payment_allocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='payment.allocation.added' THEN RAISE EXCEPTION 'isolated payment audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_payment_allocation BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_payment_allocation()`)
		defer exec(t, "DROP TRIGGER IF EXISTS fail_payment_allocation ON audit_events;DROP FUNCTION IF EXISTS fail_payment_allocation()")
		key := core.NewID()
		in := allocInput(p, c, "60")
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, key, in, nil)
		expect(t, r, 500, v)
		if get(t, p.ID).Version != p.Version || len(allocations(t, p.ID)) != 0 {
			t.Fatal("allocation survived failed audit")
		}
		exec(t, "DROP TRIGGER fail_payment_allocation ON audit_events;DROP FUNCTION fail_payment_allocation()")
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
	})
	t.Run("cash reports keep currencies separate and exclude incoming and internal transfers", func(t *testing.T) {
		for _, direction := range []string{"incoming", "transfer"} {
			in := full("cash-kinds", "10")
			in.Direction = &direction
			if direction == "transfer" {
				in.Counterparty = nil
			}
			record(t, create(t, in))
		}
		record(t, create(t, full("cash-kinds", "20")))
		in := full("cash-kinds", "30")
		in.Currency = ptr("USD")
		record(t, create(t, in))
		report := cash(t, "cash-kinds", "2025-02-01", "2025-03-01", "")
		if len(report.Totals) != 2 || report.Totals[0].Currency != "HKD" || report.Totals[0].NetAmount != "20.000000" || report.Totals[1].Currency != "USD" || report.Totals[1].NetAmount != "30.000000" || report.BaseCurrency != nil {
			t.Fatal(report)
		}
	})
	t.Run("employee settlement requires the exact approved advance line and derives partial and settled", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "cash-claim", Name: "Synthetic reimbursement approval", Kind: "claim", Category: "cash-claim", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent reimbursement check", AssignedUserID: f.bob}}, Reason: "isolated settlement policy"}, nil)
		expect(t, r, 201, v)
		line := core.NewID()
		content := spend.Content{Title: "Synthetic reimbursable advance", Category: "cash-claim", Purpose: "Exact employee advance", OwnerID: &f.alice, Currency: ptr("HKD"), RoundingMode: "half_up", PricingBasis: "gross_includes_tax", PeriodStart: ptr("2025-01-15"), PeriodEnd: ptr("2025-01-15"), Lines: []spend.Line{{ID: line, Description: "Synthetic service", DeclaredAmount: ptr("100"), ExpenseDate: ptr("2025-01-15"), Category: "cash-claim", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{file.Version.ID}, Funding: "employee_advance"}}}
		r, v = f.with(t).request("POST", "/v1/claims", f.aliceToken, core.NewID(), spend.Create{Content: content, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "employee advance"}, nil)
		expect(t, r, 201, v)
		claim := data[spend.Record](t, v)
		c := newCost(t, "claim", "100", nil)
		in := full("claim", "100")
		in.Counterparty = nil
		in.EmployeeID = &f.alice
		p := record(t, create(t, in))
		allocation := allocInput(p, c, "60")
		allocation.ClaimID = &claim.ID
		allocation.ClaimRevisionID = &claim.CurrentRevisionID
		allocation.ClaimLineID = &line
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocation, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "claim", BusinessID: claim.ID, ExpectedBusinessVersion: claim.Version, BusinessRevisionID: claim.CurrentRevisionID, BusinessContentHash: claim.ContentHash, Reason: "exact reimbursement basis"}, nil)
		expect(t, r, 201, v)
		q := data[workflow.Request](t, v)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit exact advance"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "approved employee advance"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), finance.LinkInput{Base: costBase(c), SourceType: "claim", SourceID: claim.ID, SourceRevisionID: claim.CurrentRevisionID, Relation: "reimbursed_via"}, nil)
		expect(t, r, 200, v)
		c = data[finance.Cost](t, v)
		allocation.CostVersion = c.Version
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocation, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		r, v = f.with(t).request("GET", "/v1/claims/"+claim.ID+"/settlement", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		settlement := data[finance.ClaimSettlement](t, v)
		if valueForTest(settlement.State) != "partial" || valueForTest(settlement.PaidAmount) != "60.000000" || valueForTest(settlement.OutstandingAmount) != "40.000000" {
			t.Fatal(settlement)
		}
		assertProjectCloseoutItem(t, f, *file.File.ProjectID, "claims", claim.ID, true)
		allocation.Base = base(p)
		allocation.Amount = "40"
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), allocation, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1/claims/"+claim.ID+"/settlement", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if valueForTest(data[finance.ClaimSettlement](t, v).State) != "settled" {
			t.Fatal(v)
		}
		assertProjectCloseoutItem(t, f, *file.File.ProjectID, "claims", claim.ID, false)
	})
	t.Run("historical allocation projects fence all payment reads even after release", func(t *testing.T) {
		second := f.with(t).createProject("Private payment allocation", []string{})
		c := newCost(t, "acl", "100", []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: "60"}, {ProjectID: &second.ID, Amount: "40"}})
		p := allocate(t, record(t, create(t, full("acl", "100"))), c, "100")
		for _, suffix := range []string{"", "/revisions", "/events", "/allocations"} {
			r, v := f.with(t).request("GET", "/v1/payments/"+p.ID+suffix, f.bobToken, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v := f.with(t).request("GET", "/v1/payments?category=acl", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.Payment]](t, v).Items) != 0 {
			t.Fatal("hidden payment leaked in list")
		}
		source := allocations(t, p.ID)[0]
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations/"+source.ID+"/release", f.aliceToken, core.NewID(), finance.PaymentRelease{Base: base(p), Amount: "100"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		exec(t, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'member')", f.org, second.ID, f.bob)
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		visible := data[finance.Payment](t, v)
		if visible.CanAllocate || visible.CanEdit || visible.CanReverse {
			t.Fatal("reader acquired write capability")
		}
	})
	t.Run("concurrent refunds serialize against their original funding balance", func(t *testing.T) {
		original := record(t, create(t, full("refund-race", "100")))
		in := full("refund-race", "60")
		in.Direction = ptr("refund")
		in.RefundOfID = &original.ID
		in.PaidAt = ptr("2025-02-02T00:00:00Z")
		a := create(t, in)
		in.SourceKey = "bank:" + core.NewID()
		b := create(t, in)
		start := make(chan struct{})
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for _, p := range []finance.Payment{a, b} {
			wg.Add(1)
			go func(p finance.Payment) {
				defer wg.Done()
				<-start
				r, _ := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), ExpectedSourceVersion: &original.Version, RefundAllocations: []finance.RefundAllocation{}}, nil)
				codes <- r.Code
			}(p)
		}
		close(start)
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code != 409 {
				t.Fatal(code)
			}
		}
		if success != 1 || get(t, original.ID).RefundedAmount != "60.000000" {
			t.Fatal(success)
		}
	})
	t.Run("SQL funding guard rejects a complete journal for an excessive refund", func(t *testing.T) {
		original := record(t, create(t, full("sql-refund", "100")))
		in := full("sql-refund", "101")
		in.Direction = ptr("refund")
		in.RefundOfID = &original.ID
		in.PaidAt = ptr("2025-02-02T00:00:00Z")
		p := create(t, in)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "UPDATE payment_records SET state='recorded',version=version+1 WHERE id=$1", p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO payment_events(id,org_id,payment_id,payment_version,revision_id,kind,state,actor_id,session_id,mfa_verified_at,reason) SELECT $1,$2,$3,2,$4,'recorded','recorded',$5,s.id,s.mfa_verified_at,'independent funding guard' FROM sessions s WHERE s.id=$6`, core.NewID(), f.org, p.ID, p.CurrentRevisionID, f.alice, actor.SessionID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil || !strings.Contains(err.Error(), "net outgoing payment balance") {
			t.Fatal("SQL accepted excessive refund", err)
		}
	})
	t.Run("SQL allocation guard rejects excess and fractional minor units with a complete journal", func(t *testing.T) {
		c := newCost(t, "sql-allocation", "100", nil)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		for _, amount := range []string{"101", "0.001"} {
			p := record(t, create(t, full("sql-allocation", "200")))
			id := core.NewID()
			tx, err := f.api.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, "UPDATE payment_records SET version=version+1 WHERE id=$1", p.ID); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO payment_allocations(id,org_id,payment_id,cost_id,cost_revision_id,kind,effect,amount,root_id,actor_id,reason) VALUES($1,$2,$3,$4,$5,'allocation',1,$6,$1,$7,'SQL ledger guard')`, id, f.org, p.ID, c.ID, c.CurrentRevisionID, amount, f.alice); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, "INSERT INTO payment_allocation_shares(org_id,allocation_id,position,project_id,amount) VALUES($1,$2,1,$3,$4)", f.org, id, file.File.ProjectID, amount); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO payment_events(id,org_id,payment_id,payment_version,revision_id,kind,state,allocation_id,actor_id,session_id,mfa_verified_at,reason) SELECT $1,$2,$3,3,$4,'allocation_added','recorded',$5,$6,s.id,s.mfa_verified_at,'independent allocation guard' FROM sessions s WHERE s.id=$7`, core.NewID(), f.org, p.ID, p.CurrentRevisionID, id, f.alice, actor.SessionID); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err == nil {
				t.Fatal("SQL accepted invalid allocation", amount)
			}
		}
	})
	t.Run("reversing a refund cannot overpay a cost settled by a later payment", func(t *testing.T) {
		c := newCost(t, "refund-reversal-cap", "100", nil)
		original := allocate(t, record(t, create(t, full("refund-reversal-cap", "100"))), c, "100")
		source := allocations(t, original.ID)[0]
		in := full("refund-reversal-cap", "20")
		in.Direction = ptr("refund")
		in.RefundOfID = &original.ID
		in.PaidAt = ptr("2025-02-02T00:00:00Z")
		refund := create(t, in)
		r, v := f.with(t).request("POST", "/v1/payments/"+refund.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(refund), ExpectedSourceVersion: &original.Version, RefundAllocations: []finance.RefundAllocation{{SourceAllocationID: source.ID, Amount: "20"}}}, nil)
		expect(t, r, 200, v)
		refund = data[finance.Payment](t, v)
		later := allocate(t, record(t, create(t, full("refund-reversal-cap", "20"))), c, "20")
		r, v = f.with(t).request("POST", "/v1/payments/"+refund.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: base(refund), EffectiveAt: "2025-03-01T00:00:00Z"}, nil)
		expect(t, r, 409, v)
		reverse(t, later, "2025-03-01T00:00:00Z")
		reverse(t, refund, "2025-03-02T00:00:00Z")
	})
	t.Run("cash reports require explicit nonempty bounded dates", func(t *testing.T) {
		for _, query := range []string{"from=2025-01-01", "from=2025-02-01&to=2025-01-01", "from=2024-01-01&to=2026-01-01"} {
			r, v := f.with(t).request("GET", "/v1/reports/cash?"+query, f.aliceToken, "", nil, nil)
			if r.Code != 400 && r.Code != 422 {
				t.Fatal(r.Code, v)
			}
		}
	})
	t.Run("cash aggregates can exceed the maximum amount of one entry", func(t *testing.T) {
		for range 2 {
			in := full("aggregate-range", "90000000000000")
			in.Category = "aggregate-range"
			record(t, create(t, in))
		}
		r, v := f.with(t).request("GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&category=aggregate-range", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report := data[finance.CashReport](t, v)
		if len(report.Totals) != 1 || report.Totals[0].OutgoingAmount != "180000000000000.000000" || report.Totals[0].NetAmount != "180000000000000.000000" || report.Totals[0].UnallocatedAmount != "180000000000000.000000" || report.Totals[0].EntryCount != 2 {
			t.Fatal(report)
		}
	})
	t.Run("replay retains its original monetary snapshot after later allocation and reversal", func(t *testing.T) {
		c := newCost(t, "receipt", "100", nil)
		p := create(t, full("receipt", "100"))
		key := core.NewID()
		input := finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, key, input, nil)
		expect(t, r, 200, v)
		receipt := data[finance.Payment](t, v)
		current := allocate(t, receipt, c, "40")
		reverse(t, current, "2025-03-01T00:00:00Z")
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, key, input, nil)
		expect(t, r, 200, v)
		replayed := data[finance.Payment](t, v)
		if replayed.Version != receipt.Version || replayed.State != receipt.State || replayed.EffectiveAllocatedAmount != receipt.EffectiveAllocatedAmount || valueForTest(replayed.UnallocatedAmount) != valueForTest(receipt.UnallocatedAmount) || replayed.CanReverse || replayed.CanAllocate {
			t.Fatal("receipt mixed cached head and live balances", replayed)
		}
		if live := get(t, p.ID); live.State != "reversed" || valueForTest(live.UnallocatedAmount) != "0.000000" {
			t.Fatal(live)
		}
	})
	t.Run("a cost reader without funding authority gets unknown settlement rather than zero", func(t *testing.T) {
		c := newCost(t, "restricted-settlement", "100", nil)
		allocate(t, record(t, create(t, full("restricted-settlement", "100"))), c, "40")
		exec(t, "DELETE FROM role_bindings b USING roles r WHERE r.org_id=b.org_id AND r.id=b.role_id AND b.user_id=$1 AND r.key='payment_reader'", f.bob)
		defer bind(f.bob, "payment_reader")
		r, v := f.with(t).request("GET", "/v1/costs/"+c.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		hidden := data[finance.Cost](t, v)
		if hidden.SettlementVisibility != "restricted" || hidden.PaidAmount != nil || hidden.OutstandingAmount != nil || hidden.SettlementState != nil {
			t.Fatal(hidden)
		}
	})
	t.Run("project read only phase and revoked recording authority also fence replays", func(t *testing.T) {
		in := full("revoke", "1")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		p := data[finance.Payment](t, v)
		exec(t, "UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", file.File.ProjectID)
		r, v = f.with(t).request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(p), RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 409, v)
		exec(t, "UPDATE projects SET state='active',version=version+1 WHERE id=$1", file.File.ProjectID)
		exec(t, "DELETE FROM role_bindings WHERE id=$1", recorder)
		r, v = f.with(t).request("POST", "/v1/payments", f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 && r.Code != 412 {
			t.Fatal(r.Code, v)
		}
	})
}

func valueForTest(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
