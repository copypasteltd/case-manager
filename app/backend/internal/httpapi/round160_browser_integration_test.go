package httpapi

import (
	"bytes"
	"context"
	"os"

	"log/slog"

	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"

	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func TestRound160BrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_BATCH160_FIXTURE") != "1" {
		t.Skip("Explicit isolated browser fixture only")
	}
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	var diagnostic bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostic, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostic.String())
		}
	})
	ctx := context.Background()
	var unbound int
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM role_bindings b JOIN roles r ON r.org_id=b.org_id AND r.id=b.role_id WHERE r.key='purchase_completer'").Scan(&unbound); err != nil || unbound != 0 {
		t.Fatal("completion role must start unbound", unbound, err)
	}
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, key := range []string{"purchase_manager", "party_manager", "request_manager", "purchase_variance_manager", "purchase_plan_activator", "cost_recorder", "cost_confirmer", "cost_reverser", "payment_recorder", "payment_reverser"} {
		bind(f.alice, key)
	}
	bind(f.alice, "purchase_operator")
	completer := bind(f.alice, "purchase_completer")
	for _, key := range []string{"purchase_reader", "purchase_variance_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "fulfillment", Name: "Synthetic purchase fulfillment policy", Kind: "purchase", Category: "fulfillment", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent buyer review", AssignedUserID: f.bob}}, Reason: "isolated approved order scope"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic purchase supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated purchase supplier"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if err := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
		t.Fatal(err)
	}
	p := func(v string) *string { return &v }
	get := func(t *testing.T, id string) spend.Record {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Record](t, v)
	}
	newPurchase := func(t *testing.T, approved bool, quantity, price string) spend.Record {
		t.Helper()
		in := spend.Create{Content: spend.Content{Title: "Synthetic bounded order", Category: "fulfillment", Purpose: "Isolated project tools", OwnerID: &f.alice, Currency: p("HKD"), PricingBasis: "net_plus_tax", RoundingMode: "half_up", ExpectedDeliveryDate: &today, SupplierCandidates: []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}, QuoteVersionIDs: []string{original.Version.ID}, Lines: []spend.Line{{ID: core.NewID(), Description: "Project tool", Quantity: &quantity, Unit: "unit", UnitPrice: &price, TaxAmount: p("0"), DiscountAmount: p("0")}}}, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated commercial approval"}
		r, v := f.with(t).request("POST", "/v1/purchases", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d := data[spend.Record](t, v)
		if !approved {
			return d
		}
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "purchase", BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "fix approved order basis"}, nil)
		expect(t, r, 201, v)
		q := data[workflow.Request](t, v)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit exact order scope"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "independent exact scope approval"}, nil)
		expect(t, r, 200, v)
		return get(t, d.ID)
	}
	base := func(d spend.Record) spend.ExecutionBase {
		return spend.ExecutionBase{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, Reason: "register isolated real-world fact"}
	}
	orderInput := func(d spend.Record, quantity string) spend.OrderInput {
		return spend.OrderInput{ExecutionBase: base(d), Supplier: d.SupplierCandidates[0], ExternalRef: "SYNTH-" + core.NewID(), OrderedOn: today, ExpectedDeliveryOn: today, EvidenceVersionIDs: []string{original.Version.ID}, Lines: []spend.OrderLineInput{{PurchaseLineID: d.Lines[0].ID, Quantity: quantity, UnitPrice: *d.Lines[0].UnitPrice, TaxAmount: "0", DiscountAmount: "0"}}}
	}
	orders := func(t *testing.T, d spend.Record) []spend.Order {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[spend.Order]](t, v).Items
	}
	order := func(t *testing.T, d spend.Record, qty string) (spend.Record, spend.Order) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, qty), nil)
		expect(t, r, 201, v)
		d = data[spend.Record](t, v)
		return d, orders(t, d)[0]
	}
	receiptInput := func(d spend.Record, o spend.Order, qty, result string, source *string) spend.ReceiptInput {
		return spend.ReceiptInput{ExecutionBase: base(d), OrderID: o.ID, ReceivedOn: today, EvidenceVersionIDs: []string{original.Version.ID}, Lines: []spend.ReceiptLineInput{{OrderLineID: o.Lines[0].ID, Quantity: qty, Result: result, ReversesLineID: source, Reason: "isolated acceptance or return"}}}
	}
	receipts := func(t *testing.T, d spend.Record) []spend.Receipt {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/receipts?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[spend.Receipt]](t, v).Items
	}
	receipt := func(t *testing.T, d spend.Record, o spend.Order, qty, result string, source *string) (spend.Record, spend.Receipt) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), receiptInput(d, o, qty, result, source), nil)
		expect(t, r, 201, v)
		d = data[spend.Record](t, v)
		return d, receipts(t, d)[0]
	}
	summary := func(t *testing.T, d spend.Record) spend.Fulfillment {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/fulfillment", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Fulfillment](t, v)
	}
	cancelOrder := func(t *testing.T, d spend.Record, o spend.Order) spend.Record {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders/"+o.ID+"/cancel", f.aliceToken, core.NewID(), base(d), nil)
		expect(t, r, 200, v)
		return data[spend.Record](t, v)
	}

	preview := func(t *testing.T, d spend.Record) spend.CompletionPreview {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/completion-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.CompletionPreview](t, v)
	}
	input := func(t *testing.T, d spend.Record) spend.CompletionInput {
		t.Helper()
		p := preview(t, d)
		if p.Basis == nil || p.Fingerprint == nil {
			t.Fatal(p)
		}
		mode := "no_payable_obligation"
		if len(p.SettlementModes) > 0 {
			mode = p.SettlementModes[0]
		}
		return spend.CompletionInput{ExecutionBase: base(d), ExpectedFingerprint: *p.Fingerprint, OccurredOn: today, DeliveryVerified: true, CostsVerified: true, DifferencesVerified: true, SettlementMode: mode, Attestation: "A real human verifies complete expense collection and the exact delivery and settlement evidence.", DifferenceDisposition: "Unused approved scope and every listed difference are explicitly closed after verification.", EvidenceVersionIDs: []string{original.Version.ID}}
	}
	complete := func(t *testing.T, d spend.Record, in spend.CompletionInput, status int) spend.CompletionCommandReceipt {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/completion", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, status, v)
		if status != 201 {
			return spend.CompletionCommandReceipt{}
		}
		return data[spend.CompletionCommandReceipt](t, v)
	}
	record := func(t *testing.T, d spend.Record) spend.Completion {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/completion", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Completion](t, v)
	}
	delivered := func(t *testing.T) (spend.Record, spend.Order, spend.Receipt) {
		t.Helper()
		d, o := order(t, newPurchase(t, true, "10", "2"), "10")
		d, q := receipt(t, d, o, "10", "accepted", nil)
		return d, o, q
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "current actual cost evidence"}
	}
	cashBase := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "current actual payment evidence"}
	}
	newCost := func(t *testing.T, d spend.Record, amount string) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), finance.Create{Content: finance.Content{Title: "Synthetic completion expense", OccurredOn: &today, Currency: p("HKD"), Amount: &amount, Category: "completion", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: original.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{original.Version.ID}}, ProjectID: original.File.ProjectID, Classification: "project_restricted", SourceKey: "completion-cost:" + core.NewID(), Reason: "isolated incurred expense"}, nil)
		expect(t, r, 201, v)
		c := data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), costBase(c), nil)
		expect(t, r, 200, v)
		c = data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), finance.LinkInput{Base: costBase(c), SourceType: "purchase", SourceID: d.ID, SourceRevisionID: d.CurrentRevisionID, Relation: "incurred_for"}, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	newPayment := func(t *testing.T, amount string, originalPayment *finance.Payment) finance.Payment {
		t.Helper()
		content := finance.PaymentContent{Title: "Synthetic completion cash", Direction: p("outgoing"), PaidAt: p(today + "T00:00:00+08:00"), Currency: p("HKD"), Amount: &amount, Category: "completion", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "Synthetic bank proof", EvidenceVersionIDs: []string{original.Version.ID}}
		if originalPayment != nil {
			content.Direction = p("refund")
			content.RefundOfID = &originalPayment.ID
		}
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), finance.PaymentCreate{PaymentContent: content, ProjectID: original.File.ProjectID, Classification: "project_restricted", SourceKey: "completion-cash:" + core.NewID(), Reason: "isolated cash evidence"}, nil)
		expect(t, r, 201, v)
		cash := data[finance.Payment](t, v)
		in := finance.PaymentRecordInput{Base: cashBase(cash), RefundAllocations: []finance.RefundAllocation{}}
		if originalPayment != nil {
			in.ExpectedSourceVersion = &originalPayment.Version
		}
		r, v = f.with(t).request("POST", "/v1/payments/"+cash.ID+"/record", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	allocate := func(t *testing.T, c finance.Cost, cash finance.Payment, amount string) finance.Payment {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/payments/"+cash.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: cashBase(cash), CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostContentHash: c.ContentHash, Amount: amount}, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	paid := func(t *testing.T, d spend.Record) (finance.Cost, finance.Payment) {
		t.Helper()
		c := newCost(t, d, "20")
		cash := allocate(t, c, newPayment(t, "20", nil), "20")
		return c, cash
	}

	_ = summary
	_ = cancelOrder
	_ = completer
	_ = admin
	_ = input
	_ = preview
	_ = record
	_ = complete
	d, o, _ := delivered(t)
	paid(t, d)
	d = get(t, d.ID)
	operationsRole(t, f, f.alice, "status_operator")
	service := operationsService(t, f, original.File.ProjectID)
	for range 7 {
		operationsIncident(t, f, f.alice, service.ID)
	}
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch160-functional-wave", "purchase_id": d.ID, "order_id": o.ID, "project_id": *original.File.ProjectID})
}
