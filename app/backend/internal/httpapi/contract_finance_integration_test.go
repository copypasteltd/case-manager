package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
)

func TestContractFinancialReconciliation(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	ptr := func(v string) *string { return &v }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, key := range []string{"contract_manager", "contract_operator", "party_manager", "request_manager", "cost_recorder", "cost_confirmer", "cost_reverser", "payment_recorder", "payment_reverser", "cash_reporter"} {
		bind(f.alice, key)
	}
	for _, key := range []string{"contract_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "contract-finance", Name: "Synthetic financial contract review", Kind: "contract", Category: "reconciliation", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.bob}}, Reason: "isolated reconciliation policy"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic contractual counterparty", Kind: "organization", Relationships: []string{"supplier", "customer"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated financial source"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if e := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); e != nil {
		t.Fatal(e)
	}
	newContract := func(direction string) contracts.Contract {
		t.Helper()
		in := contracts.Create{Content: contracts.Content{Title: "Synthetic " + direction + " contract", Direction: ptr(direction), Type: ptr("service"), Category: "reconciliation", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("100"), StartDate: &today, EndDate: ptr("2030-01-01"), DraftFileVersionID: &file.Version.ID, PaymentSchedule: &contracts.PaymentSchedule{Mode: "scheduled", Items: []contracts.PaymentPlanItem{{ID: core.NewID(), Title: "Initial delivery", Amount: ptr("60"), DueDate: &today, Conditions: "Confirm delivery"}, {ID: core.NewID(), Title: "Final delivery", Amount: ptr("40"), DueDate: &today, Conditions: "Confirm acceptance"}}}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated contract with fixed milestones"}
		r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		c := data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: c.ID, ExpectedBusinessVersion: c.Version, BusinessRevisionID: c.CurrentRevisionID, BusinessContentHash: c.ContentHash, Reason: "exact financial contract approval"}, nil)
		expect(t, r, 201, v)
		request := data[workflow.Request](t, v)
		r, v = f.request("GET", "/v1/requests/"+request.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.request("POST", "/v1/requests/"+request.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "human confirms exact contract"}, nil)
		expect(t, r, 200, v)
		request = data[workflow.Request](t, v)
		r, v = f.request("POST", "/v1/approval-steps/"+request.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: request.Steps[0].Version, RequestVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, Decision: "approve", Reason: "independent exact approval"}, nil)
		expect(t, r, 200, v)
		r, v = f.request("GET", "/v1/contracts/"+c.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/signature", f.aliceToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignedOn: today, SignedFileVersionID: file.Version.ID, Attestation: "Synthetic evidence verified against the approved contract", Reason: "isolated actual signature"}, nil)
		expect(t, r, 201, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/transition", f.aliceToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: c.Version, Action: "activate", RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignatureID: c.Signature.ID, Reason: "explicit effective contract"}, nil)
		expect(t, r, 200, v)
		return data[contracts.Contract](t, v)
	}
	summary := func(t *testing.T, c contracts.Contract) finance.ContractFinance {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/contracts/"+c.ID+"/finance", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.ContractFinance](t, v)
	}
	base := func(c contracts.Contract) finance.ContractFinanceBase {
		return finance.ContractFinanceBase{ExpectedVersion: summary(t, c).Version, ContractVersion: c.Version, Reason: "human verifies exact financial attribution"}
	}
	paymentBase := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "exact actual cash proof"}
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "exact incurred cost proof"}
	}
	getPayment := func(t *testing.T, id string) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/payments/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	newPayment := func(direction, amount string, original *finance.Payment) finance.Payment {
		t.Helper()
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic " + direction + " financial cash", Direction: ptr(direction), PaidAt: ptr("2025-02-01T02:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "contract-finance", MethodLabel: "Synthetic transfer", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, EvidenceVersionIDs: []string{file.Version.ID}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", SourceKey: "contract-cash:" + core.NewID(), Reason: "isolated actual financial cash"}
		if original != nil {
			in.RefundOfID = &original.ID
		}
		r, v := f.request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		p := data[finance.Payment](t, v)
		record := finance.PaymentRecordInput{Base: paymentBase(p), RefundAllocations: []finance.RefundAllocation{}}
		if original != nil {
			record.ExpectedSourceVersion = &original.Version
		}
		r, v = f.request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), record, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	newCost := func(amount string, c *contracts.Contract) finance.Cost {
		t.Helper()
		in := finance.Create{Content: finance.Content{Title: "Synthetic contractual actual cost", OccurredOn: ptr("2025-01-15"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "contract-finance", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", SourceKey: "contract-cost:" + core.NewID(), Reason: "isolated incurred cost"}
		r, v := f.request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d := data[finance.Cost](t, v)
		r, v = f.request("POST", "/v1/costs/"+d.ID+"/confirm", f.aliceToken, core.NewID(), costBase(d), nil)
		expect(t, r, 200, v)
		d = data[finance.Cost](t, v)
		if c != nil {
			r, v = f.request("POST", "/v1/costs/"+d.ID+"/links", f.aliceToken, core.NewID(), finance.LinkInput{Base: costBase(d), SourceType: "contract", SourceID: c.ID, SourceRevisionID: *c.ActiveRevisionID, Relation: "contractual_basis"}, nil)
			expect(t, r, 200, v)
			d = data[finance.Cost](t, v)
		}
		return d
	}
	linkInput := func(c contracts.Contract, mid, kind, amount string, p finance.Payment, cost *finance.Cost, root *string) finance.ContractFinanceLink {
		in := finance.ContractFinanceLink{ContractFinanceBase: base(c), MilestoneID: mid, Kind: kind, Amount: amount, SourceEventID: root, SourceID: p.ID, SourceVersion: p.Version, SourceRevisionID: p.CurrentRevisionID, SourceContentHash: p.ContentHash}
		if cost != nil {
			in.SourceID = cost.ID
			in.SourceVersion = cost.Version
			in.SourceRevisionID = cost.CurrentRevisionID
			in.SourceContentHash = cost.ContentHash
		}
		return in
	}
	link := func(t *testing.T, c contracts.Contract, in finance.ContractFinanceLink) finance.ContractFinanceReceipt {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/contracts/"+c.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, v)
		return data[finance.ContractFinanceReceipt](t, v)
	}
	events := func(t *testing.T, c contracts.Contract) []finance.ContractFinanceEvent {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/contracts/"+c.ID+"/finance/events?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[finance.ContractFinanceEvent]](t, v).Items
	}
	release := func(t *testing.T, c contracts.Contract, id, amount string) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/contracts/"+c.ID+"/finance/entries/"+id+"/release", f.aliceToken, core.NewID(), finance.ContractFinanceRelease{ContractFinanceBase: base(c), Amount: amount}, nil)
		expect(t, r, 200, v)
	}
	reverse := func(t *testing.T, p finance.Payment) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: paymentBase(p), EffectiveAt: "2025-02-02T02:00:00+08:00"}, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	reviewInput := func(c contracts.Contract, mid string) finance.ContractFinanceReview {
		view := summary(t, c)
		for _, n := range view.Nodes {
			if n.ID == mid {
				return finance.ContractFinanceReview{ContractFinanceBase: base(c), ExpectedFingerprint: *n.Fingerprint, ConditionsSatisfied: true, AcknowledgeDifference: true, AcknowledgeUnattributedRefunds: true, EvidenceVersionIDs: []string{file.Version.ID}}
			}
		}
		t.Fatal("node missing")
		return finance.ContractFinanceReview{}
	}
	review := func(t *testing.T, c contracts.Contract, mid string) finance.ContractFinanceReceipt {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/contracts/"+c.ID+"/finance/milestones/"+mid+"/review", f.aliceToken, core.NewID(), reviewInput(c, mid), nil)
		expect(t, r, 200, v)
		return data[finance.ContractFinanceReceipt](t, v)
	}
	payable := newContract("payable")
	receivable := newContract("receivable")
	nodes := summary(t, payable).Nodes
	mid, other := nodes[0].ID, nodes[1].ID
	var roleID string
	t.Run("reconciliation role starts unbound and contract readers see unknown financial axes", func(t *testing.T) {
		view := summary(t, payable)
		if view.CanReconcile || view.Version != 0 {
			t.Fatal(view)
		}
		r, v := f.with(t).request("GET", "/v1/contracts/"+payable.ID+"/finance", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		limited := data[finance.ContractFinance](t, v)
		if limited.Nodes[0].CashAmount != nil || limited.Nodes[0].CostAmount != nil || limited.Nodes[0].Fingerprint != nil || limited.HistoryVisibility != "restricted" {
			t.Fatal("unknown facts became zero", limited)
		}
		r, v = f.with(t).request("GET", "/v1/contracts/"+payable.ID+"/finance", admin, "", nil, nil)
		expect(t, r, 404, v)
		roleID = bind(f.alice, "contract_reconciler")
		if !summary(t, payable).CanReconcile {
			t.Fatal("explicit reconciler permission missing")
		}
	})
	cost := newCost("100", &payable)
	cash := newPayment("outgoing", "100", nil)
	var root finance.ContractFinanceReceipt
	t.Run("cost and cash remain separate facts with immutable exact source revisions", func(t *testing.T) {
		link(t, payable, linkInput(payable, mid, "cost", "30", finance.Payment{}, &cost, nil))
		root = link(t, payable, linkInput(payable, mid, "cash", "50", cash, nil, nil))
		n := summary(t, payable).Nodes[0]
		if *n.CostAmount != "30.000000" || *n.CashAmount != "50.000000" || *n.OutstandingAmount != "10.000000" {
			t.Fatal(n)
		}
		if len(events(t, payable)) != 2 {
			t.Fatal("attribution history missing")
		}
		r, v := f.with(t).request("GET", "/v1/contracts/"+payable.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[contracts.Contract](t, v).Version != payable.Version {
			t.Fatal("financial attribution revised legal terms")
		}
	})
	t.Run("unlinked costs wrong cash directions and fractional minor units are rejected", func(t *testing.T) {
		stray := newCost("10", nil)
		in := linkInput(payable, mid, "cost", "1", finance.Payment{}, &stray, nil)
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		incoming := newPayment("incoming", "10", nil)
		in = linkInput(payable, mid, "cash", "1", incoming, nil, nil)
		r, v = f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		in = linkInput(payable, mid, "cash", "0.001", cash, nil, nil)
		r, v = f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
	})
	t.Run("duplicate assignment is capped across nodes while excess against plan is explicit", func(t *testing.T) {
		link(t, payable, linkInput(payable, other, "cash", "50", cash, nil, nil))
		n := summary(t, payable).Nodes[1]
		if *n.ExcessAmount != "10.000000" {
			t.Fatal(n)
		}
		in := linkInput(payable, mid, "cash", "0.01", cash, nil, nil)
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		link(t, payable, linkInput(payable, other, "cost", "70", finance.Payment{}, &cost, nil))
		in = linkInput(payable, mid, "cost", "0.01", finance.Payment{}, &cost, nil)
		r, v = f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
	})
	t.Run("concurrent devices advance a single financial head", func(t *testing.T) {
		p := newPayment("outgoing", "10", nil)
		in := linkInput(payable, mid, "cash", "10", p, nil, nil)
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		count := map[int]int{}
		for code := range codes {
			count[code]++
		}
		if count[200] != 1 || count[409] != 1 {
			t.Fatal(count)
		}
	})
	var refund finance.Payment
	var refundLink finance.ContractFinanceReceipt
	t.Run("unattributed actual refunds remain visible until classified", func(t *testing.T) {
		refund = newPayment("refund", "10", &cash)
		if !*summary(t, payable).Nodes[0].HasUnattributedRefunds {
			t.Fatal("silently ignored actual refund")
		}
		in := reviewInput(payable, mid)
		in.AcknowledgeUnattributedRefunds = false
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/milestones/"+mid+"/review", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		refundLink = link(t, payable, linkInput(payable, mid, "refund", "10", refund, nil, &root.EventID))
		n := summary(t, payable).Nodes[0]
		if *n.CashAmount != "50.000000" || *n.HasUnattributedRefunds {
			t.Fatal(n)
		}
	})
	t.Run("releasing sources preserves history and cannot undercut active refunds", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries/"+root.EventID+"/release", f.aliceToken, core.NewID(), finance.ContractFinanceRelease{ContractFinanceBase: base(payable), Amount: "41"}, nil)
		expect(t, r, 409, v)
		release(t, payable, refundLink.EventID, "5")
		if !*summary(t, payable).Nodes[0].HasUnattributedRefunds {
			t.Fatal("released refund must be unknown again")
		}
		release(t, payable, root.EventID, "5")
		if *summary(t, payable).Nodes[0].CashAmount != "50.000000" {
			t.Fatal(summary(t, payable))
		}
		r, v = f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/entries/"+refundLink.EventID+"/release", f.aliceToken, core.NewID(), finance.ContractFinanceRelease{ContractFinanceBase: base(payable), Amount: "6"}, nil)
		expect(t, r, 409, v)
	})
	var fixedReview finance.ContractFinanceReceipt
	t.Run("review stores fixed evidence and explicit differences without closing legal obligations", func(t *testing.T) {
		in := reviewInput(payable, mid)
		in.AcknowledgeDifference = false
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/milestones/"+mid+"/review", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		in = reviewInput(payable, mid)
		in.EvidenceVersionIDs = []string{}
		r, v = f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/milestones/"+mid+"/review", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		fixedReview = review(t, payable, mid)
		n := summary(t, payable).Nodes[0]
		if n.ReviewState != "current" || n.ConditionsSatisfied == nil || !*n.ConditionsSatisfied {
			t.Fatal(n)
		}
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='contract_finance_event' AND owner_id=$1 AND version_id=$2", fixedReview.EventID, file.Version.ID).Scan(&count); e != nil || count != 1 {
			t.Fatal(count, e)
		}
	})
	t.Run("external cash reversal invalidates review without changing the financial head", func(t *testing.T) {
		before := summary(t, payable)
		reverse(t, refund)
		after := summary(t, payable)
		if before.Version != after.Version || after.Nodes[0].ReviewState != "stale" || *after.Nodes[0].CashAmount != "55.000000" {
			t.Fatal(before, after)
		}
		for _, event := range events(t, payable) {
			if event.ID == fixedReview.EventID && (!strings.Contains(string(event.ReviewSnapshot), "recorded") || len(event.Attachments) != 1) {
				t.Fatal("lost original snapshot", event)
			}
		}
	})
	t.Run("stale source fingerprint rejects review even with a current contract head", func(t *testing.T) {
		in := reviewInput(payable, mid)
		p := newPayment("outgoing", "1", nil)
		link(t, payable, linkInput(payable, mid, "cash", "1", p, nil, nil))
		in.ContractFinanceBase = base(payable)
		r, v := f.with(t).request("POST", "/v1/contracts/"+payable.ID+"/finance/milestones/"+mid+"/review", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		if v.Error.Code != "CONTRACT_FINANCE_BASIS_CHANGED" {
			t.Fatal(v.Error)
		}
	})
	t.Run("receivables allocate income and return cash without creating expenses", func(t *testing.T) {
		n := summary(t, receivable).Nodes[0]
		income := newPayment("incoming", "100", nil)
		original := link(t, receivable, linkInput(receivable, n.ID, "cash", "100", income, nil, nil))
		back := newPayment("outgoing", "25", nil)
		link(t, receivable, linkInput(receivable, n.ID, "refund", "25", back, nil, &original.EventID))
		view := summary(t, receivable)
		if view.Nodes[0].CostVisibility != "not_applicable" || view.Nodes[0].CostAmount != nil || *view.Nodes[0].CashAmount != "75.000000" {
			t.Fatal(view)
		}
		reserved := getPayment(t, back.ID)
		if reserved.ContractReturnReservedAmount != "25.000000" || *reserved.UnallocatedAmount != "0.000000" {
			t.Fatal(reserved)
		}
		expense := newCost("25", nil)
		r, v := f.with(t).request("POST", "/v1/payments/"+back.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: paymentBase(reserved), CostID: expense.ID, CostVersion: expense.Version, CostRevisionID: expense.CurrentRevisionID, CostContentHash: expense.ContentHash, Amount: "1"}, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&category=contract-finance&project_id="+*file.File.ProjectID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report := data[finance.CashReport](t, v)
		if len(report.Totals) != 1 || report.Totals[0].NetAmount != "25.000000" {
			t.Fatal(report)
		}
		if getPayment(t, income.ID).CanReverse {
			t.Fatal("original receipt still reversible")
		}
		r, v = f.with(t).request("POST", "/v1/payments/"+income.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: paymentBase(income), EffectiveAt: "2025-02-02T02:00:00+08:00"}, nil)
		expect(t, r, 409, v)
		reverse(t, back)
		if *summary(t, receivable).Nodes[0].CashAmount != "100.000000" {
			t.Fatal("return reversal did not restore income")
		}
		reverse(t, income)
		if *summary(t, receivable).Nodes[0].CashAmount != "0.000000" {
			t.Fatal("receipt reversal did not remove income")
		}
		r, v = f.with(t).request("GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&category=contract-finance&project_id="+*file.File.ProjectID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report = data[finance.CashReport](t, v)
		if report.Totals[0].NetAmount != "0.000000" {
			t.Fatal(report)
		}
	})
	t.Run("cost settlement and contract cash classification never duplicate the economic cash", func(t *testing.T) {
		c := newCost("10", nil)
		p := newPayment("outgoing", "10", nil)
		r, v := f.with(t).request("POST", "/v1/payments/"+p.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: paymentBase(p), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: "10"}, nil)
		expect(t, r, 200, v)
		p = data[finance.Payment](t, v)
		link(t, payable, linkInput(payable, mid, "cash", "10", p, nil, nil))
		current := getPayment(t, p.ID)
		if current.EffectiveAllocatedAmount != "10.000000" || *current.UnallocatedAmount != "0.000000" || current.ContractReturnReservedAmount != "0.000000" {
			t.Fatal(current)
		}
	})
	t.Run("different contracts serialize on the same cash source cap", func(t *testing.T) {
		second := newContract("payable")
		p := newPayment("outgoing", "10", nil)
		in1 := linkInput(payable, mid, "cash", "10", p, nil, nil)
		in2 := linkInput(second, summary(t, second).Nodes[0].ID, "cash", "10", p, nil, nil)
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for _, item := range []struct {
			id    string
			input finance.ContractFinanceLink
		}{{payable.ID, in1}, {second.ID, in2}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/contracts/"+item.id+"/finance/entries", f.aliceToken, core.NewID(), item.input, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		count := map[int]int{}
		for code := range codes {
			count[code]++
		}
		if count[200] != 1 || count[409] != 1 {
			t.Fatal(count)
		}
	})
	t.Run("audit failure rolls back financial journal fixed proof head and idempotency receipt", func(t *testing.T) {
		before := summary(t, payable)
		in := reviewInput(payable, mid)
		key := core.NewID()
		path := "/v1/contracts/" + payable.ID + "/finance/milestones/" + mid + "/review"
		exec(`CREATE FUNCTION fail_contract_finance_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='contract.finance.review' THEN RAISE EXCEPTION 'isolated contract audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_contract_finance_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_contract_finance_audit()`)
		defer exec("DROP TRIGGER IF EXISTS fail_contract_finance_audit ON audit_events;DROP FUNCTION IF EXISTS fail_contract_finance_audit()")
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 500, v)
		if summary(t, payable).Version != before.Version {
			t.Fatal("financial head survived rejected audit")
		}
		var receipts int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", key).Scan(&receipts); e != nil || receipts != 0 {
			t.Fatal(receipts, e)
		}
		exec("DROP TRIGGER fail_contract_finance_audit ON audit_events;DROP FUNCTION fail_contract_finance_audit()")
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
	})
	t.Run("complete forged journals still fail database source caps", func(t *testing.T) {
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		var version int64
		if e = tx.QueryRow(ctx, "SELECT version FROM contract_finance_heads WHERE org_id=$1 AND contract_id=$2 FOR UPDATE", f.org, payable.ID).Scan(&version); e != nil {
			t.Fatal(e)
		}
		id := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO contract_finance_events(id,org_id,contract_id,milestone_id,contract_revision_id,seq,kind,payment_id,source_version,source_revision_id,source_content_hash,amount,actor_id,session_id,mfa_verified_at,reason)
			SELECT $1,e.org_id,e.contract_id,e.milestone_id,e.contract_revision_id,$2,e.kind,e.payment_id,p.version,r.id,r.content_hash,101,e.actor_id,e.session_id,s.mfa_verified_at,'synthetic complete cap forgery' FROM contract_finance_events e JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id JOIN sessions s ON s.org_id=e.org_id AND s.id=e.session_id WHERE e.id=$3`, id, version+1, root.EventID)
		if e == nil {
			_, e = tx.Exec(ctx, "UPDATE contract_finance_heads SET version=version+1,current_event_id=$2 WHERE contract_id=$1", payable.ID, id)
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e == nil {
			t.Fatal("complete source attribution exceeded cash amount")
		}
	})
	t.Run("event cursors bind the contract and historical ACL survives release", func(t *testing.T) {
		r, v := f.with(t).request("GET", "/v1/contracts/"+payable.ID+"/finance/events?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[finance.ContractFinanceEvent]](t, v)
		if page.NextCursor == nil {
			t.Fatal(page)
		}
		r, v = f.with(t).request("GET", "/v1/contracts/"+receivable.ID+"/finance/events?cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
		r, v = f.with(t).request("GET", "/v1/contracts/"+payable.ID+"/finance/events", f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		bind(f.bob, "payment_reader")
		p := newPayment("outgoing", "1", nil)
		receipt := link(t, payable, linkInput(payable, mid, "cash", "1", p, nil, nil))
		release(t, payable, receipt.EventID, "1")
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='contract_reader')", f.org, f.bob)
		defer exec("UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='contract_reader')", f.org, f.bob)
		r, v = f.with(t).request("GET", "/v1/payments/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
	})
	t.Run("external cost reversal preserves the historical review and removes current incurred attribution", func(t *testing.T) {
		review(t, payable, mid)
		before := summary(t, payable)
		r, v := f.with(t).request("POST", "/v1/costs/"+cost.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: costBase(cost), EffectiveOn: "2025-02-03"}, nil)
		expect(t, r, 200, v)
		after := summary(t, payable)
		if after.Version != before.Version || *after.Nodes[0].CostAmount != "0.000000" || after.Nodes[0].ReviewState != "stale" {
			t.Fatal(after)
		}
	})
	t.Run("append only database heads fixed review evidence and missing actual MFA reject forgery", func(t *testing.T) {
		if _, e := f.owner.Exec(ctx, "UPDATE contract_finance_events SET reason='forged' WHERE id=$1", root.EventID); e == nil {
			t.Fatal("rewrote original ledger")
		}
		if _, e := f.owner.Exec(ctx, "UPDATE contract_finance_heads SET version=version+1 WHERE contract_id=$1", payable.ID); e == nil {
			t.Fatal("detached financial head")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM contract_finance_event_files WHERE event_id=$1", fixedReview.EventID); e == nil {
			t.Fatal("deleted fixed review reference")
		}
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		var actual time.Time
		if e = f.owner.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE id=$1", actor.SessionID).Scan(&actual); e != nil {
			t.Fatal(e)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '1 hour' WHERE id=$1", actor.SessionID)
		defer exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		in := reviewInput(payable, mid)
		raw, _ := json.Marshal(in)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		service := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		_, e = service.ReviewContractFinance(ctx, core.Command{Actor: actor, RequestID: core.NewID(), Key: core.NewID(), Route: "POST /v1/contracts/" + payable.ID + "/finance/milestones/" + mid + "/review", Body: raw}, payable.ID, mid, in)
		if core.PublicError(e).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(e)
		}
	})
	t.Run("cached identifier receipts reauthorize after explicit reconciliation scope expiry", func(t *testing.T) {
		p := newPayment("outgoing", "1", nil)
		in := linkInput(payable, mid, "cash", "1", p, nil, nil)
		key := core.NewID()
		path := "/v1/contracts/" + payable.ID + "/finance/entries"
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
		first := data[finance.ContractFinanceReceipt](t, v)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
		if data[finance.ContractFinanceReceipt](t, v).EventID != first.EventID {
			t.Fatal("replay duplicated financial fact")
		}
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", roleID)
		defer exec("UPDATE role_bindings SET valid_until=NULL WHERE id=$1", roleID)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 404, v)
	})
}
