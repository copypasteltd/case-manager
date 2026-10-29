package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func TestCrossCurrencySettlement(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	ptr := func(s string) *string { return &s }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"payment_recorder", "payment_reverser", "cash_reporter", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "claim_submitter", "request_manager"} {
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
	var p finance.Payment
	var c finance.Cost
	t.Run("review quote is explicit and same currency protocol stays compatible", func(t *testing.T) {
		same := newCost(t, "HKD", "1", nil)
		cash := record(t, draft(t, "HKD", "1", nil, nil))
		cash = allocate(t, cash, same, "1", "", "")
		a := root(t, cash.ID)
		if a.FX != nil || a.CostAmount != "1.000000" || a.CostCurrency != "HKD" || a.PaymentCurrency != "HKD" {
			t.Fatal(a)
		}
		c = newCost(t, "USD", "100", []finance.Allocation{{ProjectID: nil, Amount: "40"}, {ProjectID: file.File.ProjectID, Amount: "60"}})
		p = record(t, draft(t, "HKD", "780", nil, nil))
		call(t, "POST", "/v1/payments/"+p.ID+"/allocations", allocInput(p, c, "780", "", ""), 409)
		for _, bad := range []finance.PaymentFXInput{{CostAmount: "100", Rate: "7.8", Source: "bank", Acknowledged: false}, {CostAmount: "100", Rate: "7.7", Source: "bank", Acknowledged: true}, {CostAmount: "100", Rate: "0", Source: "bank", Acknowledged: true}, {CostAmount: "100.001", Rate: "7.8", Source: "bank", Acknowledged: true}, {CostAmount: "100", Rate: "7.8", Source: "", Acknowledged: true}} {
			in := allocInput(p, c, "780", "100", "7.8")
			in.FX = &bad
			call(t, "POST", "/v1/payments/"+p.ID+"/allocations", in, 422)
		}
		if len(rows(t, p.ID)) != 0 || get(t, p.ID).Version != p.Version {
			t.Fatal("invalid quote changed ledger")
		}
	})
	t.Run("allocation conserves independent cash cost and project units", func(t *testing.T) {
		p = allocate(t, p, c, "780", "100", "7.8")
		a := root(t, p.ID)
		if a.CostAmount != "100.000000" || a.Amount != "780.000000" || a.RemainingCostAmount != "100.000000" || a.FX == nil || a.FX.Rate != "7.8000000000" || a.FX.QuotedOn != "2025-02-02" || a.CostExponent != 2 || a.PaymentExponent != 2 || len(a.CostShares) != 2 {
			t.Fatal(a)
		}
		cash, cost := map[string]string{}, map[string]string{}
		for i, s := range a.Shares {
			key := valueForTest(s.ProjectID)
			cash[key] = s.Amount
			cost[key] = a.CostShares[i].Amount
			if key != valueForTest(a.CostShares[i].ProjectID) {
				t.Fatal(a)
			}
		}
		if cash[""] != "312.000000" || cost[""] != "40.000000" || cash[*file.File.ProjectID] != "468.000000" || cost[*file.File.ProjectID] != "60.000000" {
			t.Fatal(a)
		}
		assertCost(t, c, "100.000000", "0.000000")
		if p.EffectiveAllocatedAmount != "780.000000" || valueForTest(p.UnallocatedAmount) != "0.000000" {
			t.Fatal(p)
		}
	})
	t.Run("inverse review is required stale or forged cost units roll back", func(t *testing.T) {
		a := root(t, p.ID)
		for _, expected := range []*string{nil, ptr("49.99")} {
			status := 409
			if expected == nil {
				status = 422
			}
			call(t, "POST", "/v1/payments/"+p.ID+"/allocations/"+a.ID+"/release", finance.PaymentRelease{Base: base(p), Amount: "390", CostAmount: expected}, status)
		}
		if len(rows(t, p.ID)) != 1 {
			t.Fatal("failed review wrote ledger")
		}
		p = release(t, p, a, "390", "50")
		assertCost(t, c, "50.000000", "50.000000")
		a = root(t, p.ID)
		if a.RemainingAmount != "390.000000" || a.RemainingCostAmount != "50.000000" {
			t.Fatal(a)
		}
	})
	t.Run("atomic refund reverses the original quote and both project axes", func(t *testing.T) {
		a := root(t, p.ID)
		refund := draft(t, "HKD", "195", nil, &p)
		v := p.Version
		refund = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+refund.ID+"/record", finance.PaymentRecordInput{Base: base(refund), ExpectedSourceVersion: &v, RefundAllocations: []finance.RefundAllocation{{SourceAllocationID: a.ID, Amount: "195", CostAmount: ptr("25")}}}, 200))
		r := rows(t, refund.ID)
		if len(r) != 1 || r[0].CostAmount != "25.000000" || r[0].FX == nil || r[0].FX.Rate != a.FX.Rate || r[0].Effect != -1 {
			t.Fatal(r)
		}
		assertCost(t, c, "25.000000", "75.000000")
		reverse(t, refund, 200)
		assertCost(t, c, "50.000000", "50.000000")
	})
	t.Run("full cash reversal copies exact dual ledger and preserves incurred fact", func(t *testing.T) {
		original := rows(t, p.ID)
		p = get(t, p.ID)
		p = reverse(t, p, 200)
		rev := get(t, valueForTest(p.ReversalID))
		r := rows(t, rev.ID)
		if len(r) != len(original) {
			t.Fatal(r)
		}
		by := map[string]finance.PaymentAllocation{}
		for _, v := range original {
			by[v.ID] = v
		}
		for _, v := range r {
			o := by[valueForTest(v.SourceAllocationID)]
			if v.CostAmount != o.CostAmount || v.Amount != o.Amount || v.Effect != -o.Effect || v.FX == nil || v.FX.Source != o.FX.Source {
				t.Fatal(v, o)
			}
			x, _ := json.Marshal(v.CostShares)
			y, _ := json.Marshal(o.CostShares)
			if string(x) != string(y) {
				t.Fatal("reversal changed cost shares")
			}
		}
		assertCost(t, c, "0.000000", "100.000000")
	})
	t.Run("integer cost currency keeps zero partial and consumes final rounding remainder", func(t *testing.T) {
		cost := newCost(t, "JPY", "1", nil)
		cash := record(t, draft(t, "HKD", "100", nil, nil))
		cash = allocate(t, cash, cost, "100", "1", "100")
		a := root(t, cash.ID)
		cash = release(t, cash, a, "1", "0")
		assertCost(t, cost, "1.000000", "0.000000")
		cash = release(t, cash, root(t, cash.ID), "99", "1")
		assertCost(t, cost, "0.000000", "1.000000")
		a = root(t, cash.ID)
		if a.RemainingAmount != "0.000000" || a.RemainingCostAmount != "0.000000" {
			t.Fatal(a)
		}
	})
	t.Run("zero cost balance with residual cash still blocks expense reversal", func(t *testing.T) {
		cost := newCost(t, "JPY", "1", nil)
		cash := record(t, draft(t, "HKD", "100", nil, nil))
		cash = allocate(t, cash, cost, "100", "1", "100")
		cash = release(t, cash, root(t, cash.ID), "51", "1")
		cost = getCost(t, cost.ID)
		if cost.CanReverse {
			t.Fatal("rounded-zero cost enabled reversal while cash remains")
		}
		call(t, "POST", "/v1/costs/"+cost.ID+"/reverse", finance.Reverse{Base: cb(cost), EffectiveOn: "2025-02-03"}, 409)
		release(t, cash, root(t, cash.ID), "49", "0")
	})
	t.Run("different payment currency minor units retain zero cash project shares", func(t *testing.T) {
		cost := newCost(t, "USD", "0.03", []finance.Allocation{{ProjectID: nil, Amount: "0.01"}, {ProjectID: file.File.ProjectID, Amount: "0.02"}})
		cash := record(t, draft(t, "JPY", "1", nil, nil))
		cash = allocate(t, cash, cost, "1", "0.03", "33.3333333333")
		a := root(t, cash.ID)
		if len(a.Shares) != 2 || a.Shares[0].Amount != "0.000000" || a.CostShares[0].Amount != "0.010000" {
			t.Fatal(a)
		}
		release(t, cash, a, "1", "0.03")
		assertCost(t, cost, "0.000000", "0.030000")
	})
	t.Run("cost balance is shared across payments in different currencies", func(t *testing.T) {
		cost := newCost(t, "USD", "100", nil)
		one := record(t, draft(t, "HKD", "780", nil, nil))
		allocate(t, one, cost, "780", "100", "7.8")
		two := record(t, draft(t, "EUR", "90", nil, nil))
		call(t, "POST", "/v1/payments/"+two.ID+"/allocations", allocInput(two, cost, "90", "100", "0.9"), 409)
		assertCost(t, cost, "100.000000", "0.000000")
	})
	t.Run("concurrent allocations cannot overdraw expense units", func(t *testing.T) {
		cost := newCost(t, "USD", "100", nil)
		payments := []finance.Payment{record(t, draft(t, "HKD", "468", nil, nil)), record(t, draft(t, "HKD", "468", nil, nil))}
		statuses := make(chan int, 2)
		var wg sync.WaitGroup
		for _, cash := range payments {
			wg.Add(1)
			go func(cash finance.Payment) {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/payments/"+cash.ID+"/allocations", f.aliceToken, core.NewID(), allocInput(cash, cost, "468", "60", "7.8"), nil)
				statuses <- r.Code
			}(cash)
		}
		wg.Wait()
		close(statuses)
		pass, conflict := 0, 0
		for s := range statuses {
			if s == 200 {
				pass++
			}
			if s == 409 {
				conflict++
			}
		}
		if pass != 1 || conflict != 1 {
			t.Fatal(pass, conflict)
		}
		assertCost(t, cost, "60.000000", "40.000000")
	})
	t.Run("direct SQL cannot forge quotation or inverse cost units", func(t *testing.T) {
		cost := newCost(t, "USD", "10", nil)
		cash := record(t, draft(t, "HKD", "78", nil, nil))
		cash = allocate(t, cash, cost, "78", "10", "7.8")
		a := root(t, cash.ID)
		for _, scenario := range []string{"root quote", "inverse amount"} {
			tx, e := f.api.DB.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			id := core.NewID()
			kind, effect, amount, costAmount, rate, source := "allocation", 1, "78", "10", "7.7", (*string)(nil)
			rootID := id
			if scenario == "inverse amount" {
				kind, effect, amount, costAmount, rate, source, rootID = "release", -1, "39", "4.99", "7.8", &a.ID, a.ID
			}
			_, e = tx.Exec(ctx, `INSERT INTO payment_allocations(id,org_id,payment_id,cost_id,cost_revision_id,kind,effect,amount,cost_amount,root_id,source_allocation_id,actor_id,reason,fx_rate,fx_source,fx_quoted_on) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'SQL forged units',$13,'synthetic reviewed bank settlement','2025-02-02')`, id, f.org, cash.ID, cost.ID, cost.CurrentRevisionID, kind, effect, amount, costAmount, rootID, source, f.alice, rate)
			if e == nil {
				_, e = tx.Exec(ctx, `INSERT INTO payment_allocation_shares(org_id,allocation_id,position,project_id,amount,cost_amount) VALUES($1,$2,1,$3,$4,$5)`, f.org, id, file.File.ProjectID, amount, costAmount)
			}
			if e == nil {
				_, e = tx.Exec(ctx, "SET CONSTRAINTS validate_payment_fx_allocation,validate_payment_fx_share IMMEDIATE")
			}
			tx.Rollback(ctx)
			if e == nil || !strings.Contains(e.Error(), map[string]string{"root quote": "exact rounded quote", "inverse amount": "exact remaining dual units"}[scenario]) {
				t.Fatal(scenario, e)
			}
		}
	})
	t.Run("later refund allocation rechecks the original remaining dual units", func(t *testing.T) {
		cost := newCost(t, "USD", "100", nil)
		cash := record(t, draft(t, "HKD", "1000", nil, nil))
		cash = allocate(t, cash, cost, "780", "100", "7.8")
		a := root(t, cash.ID)
		refund := draft(t, "HKD", "78", nil, &cash)
		version := cash.Version
		refund = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+refund.ID+"/record", finance.PaymentRecordInput{Base: base(refund), ExpectedSourceVersion: &version, RefundAllocations: []finance.RefundAllocation{}}, 200))
		refund = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+refund.ID+"/refund-allocations", finance.PaymentRefundAllocate{Base: base(refund), RefundAllocation: finance.RefundAllocation{SourceAllocationID: a.ID, Amount: "78", CostAmount: ptr("10")}}, 200))
		assertCost(t, cost, "90.000000", "10.000000")
		// The payment head did not change when a separate refund changed the root.
		// Its old version cannot authorize an old inverse cost calculation.
		call(t, "POST", "/v1/payments/"+cash.ID+"/allocations/"+a.ID+"/release", finance.PaymentRelease{Base: base(cash), Amount: "702", CostAmount: ptr("100")}, 409)
		cash = release(t, cash, a, "702", "90")
		assertCost(t, cost, "0.000000", "100.000000")
		reverse(t, refund, 200)
		assertCost(t, cost, "10.000000", "90.000000")
		_ = cash
	})
	t.Run("reports and project closeout preserve their own monetary units", func(t *testing.T) {
		cost := newCost(t, "USD", "100", nil)
		cash := record(t, draft(t, "HKD", "780", nil, nil))
		cash = allocate(t, cash, cost, "780", "100", "7.8")
		var paid string
		var blocked bool
		if e := f.owner.QueryRow(ctx, "SELECT blocking,facts->>'paid_amount' FROM project_closeout_items($1,$2) WHERE kind='costs' AND id=$3", f.org, file.File.ProjectID, cost.ID).Scan(&blocked, &paid); e != nil || blocked || paid != "100.000000" {
			t.Fatal(blocked, paid, e)
		}
		report := data[finance.CashReport](t, call(t, "GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&q="+cash.SourceKey, nil, 200))
		if len(report.Totals) != 1 || report.Totals[0].Currency != "HKD" || report.Totals[0].NetAmount != "780.000000" || report.Totals[0].UnallocatedAmount != "0.000000" {
			t.Fatal(report)
		}
	})
	t.Run("cross currency replay retains original fact and requires current human MFA", func(t *testing.T) {
		cost := newCost(t, "USD", "10", nil)
		cash := record(t, draft(t, "HKD", "78", nil, nil))
		in := allocInput(cash, cost, "78", "10", "7.8")
		key, path := core.NewID(), "/v1/payments/"+cash.ID+"/allocations"
		for i := 0; i < 2; i++ {
			r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
			expect(t, r, 200, v)
		}
		if len(rows(t, cash.ID)) != 1 {
			t.Fatal("replay duplicated dual settlement")
		}
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		defer exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actor.MFAVerifiedAt)
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 403, v)
	})
	t.Run("approved employee claim settles in its own currency", func(t *testing.T) {
		admin := f.administratorToken()
		r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "fx-claim", Name: "Synthetic FX claim check", Kind: "claim", Category: "fx-claim", Currency: "USD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.bob}}, Reason: "synthetic policy"}, nil)
		expect(t, r, 201, v)
		line := core.NewID()
		content := spend.Content{Title: "Synthetic USD employee advance", Category: "fx-claim", Purpose: "employee paid USD supplier", OwnerID: &f.alice, Currency: ptr("USD"), RoundingMode: "half_up", PricingBasis: "gross_includes_tax", PeriodStart: ptr("2025-01-15"), PeriodEnd: ptr("2025-01-15"), Lines: []spend.Line{{ID: line, Description: "service", DeclaredAmount: ptr("100"), ExpenseDate: ptr("2025-01-15"), Category: "fx-claim", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{file.Version.ID}, Funding: "employee_advance"}}}
		claim := data[spend.Record](t, call(t, "POST", "/v1/claims", spend.Create{Content: content, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic approved claim"}, 201))
		q := data[workflow.Request](t, call(t, "POST", "/v1/requests", workflow.RequestCreate{Kind: "claim", BusinessID: claim.ID, ExpectedBusinessVersion: claim.Version, BusinessRevisionID: claim.CurrentRevisionID, BusinessContentHash: claim.ContentHash, Reason: "review exact USD claim"}, 201))
		preview := data[workflow.SubmissionPreview](t, call(t, "GET", "/v1/requests/"+q.ID+"/submission-preview", nil, 200))
		q = data[workflow.Request](t, call(t, "POST", "/v1/requests/"+q.ID+"/submit", workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit reviewed claim"}, 200))
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "independent approval"}, nil)
		expect(t, r, 200, v)
		cost := newCost(t, "USD", "100", nil)
		cost = data[finance.Cost](t, call(t, "POST", "/v1/costs/"+cost.ID+"/links", finance.LinkInput{Base: cb(cost), SourceType: "claim", SourceID: claim.ID, SourceRevisionID: claim.CurrentRevisionID, Relation: "reimbursed_via"}, 200))
		cash := record(t, draft(t, "HKD", "780", &f.alice, nil))
		in := allocInput(cash, cost, "468", "60", "7.8")
		in.ClaimID = &claim.ID
		in.ClaimRevisionID = &claim.CurrentRevisionID
		in.ClaimLineID = &line
		cash = data[finance.Payment](t, call(t, "POST", "/v1/payments/"+cash.ID+"/allocations", in, 200))
		settled := data[finance.ClaimSettlement](t, call(t, "GET", "/v1/claims/"+claim.ID+"/settlement", nil, 200))
		if valueForTest(settled.PaidAmount) != "60.000000" || valueForTest(settled.OutstandingAmount) != "40.000000" || valueForTest(settled.Currency) != "USD" {
			t.Fatal(settled)
		}
		release(t, cash, root(t, cash.ID), "234", "30")
		settled = data[finance.ClaimSettlement](t, call(t, "GET", "/v1/claims/"+claim.ID+"/settlement", nil, 200))
		if valueForTest(settled.PaidAmount) != "30.000000" {
			t.Fatal(settled)
		}
	})
}
