package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
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
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPurchaseCompletion(t *testing.T) {
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

	t.Run("unbound role and missing delivery never imply completion", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		p := preview(t, d)
		if p.CanComplete || !slices.Contains(p.Blockers, "no_accepted_delivery") {
			t.Fatal(p)
		}
		complete(t, d, input(t, d), 409)
		d, o := order(t, d, "10")
		d, _ = receipt(t, d, o, "4", "accepted", nil)
		p = preview(t, d)
		if p.CanComplete || !slices.Contains(p.Blockers, "order_unfulfilled") {
			t.Fatal(p)
		}
		complete(t, d, input(t, d), 409)
	})
	t.Run("unsettled confirmed expense cannot be waived by a declaration", func(t *testing.T) {
		d, _, _ := delivered(t)
		_ = newCost(t, d, "20")
		p := preview(t, d)
		if p.CanComplete || !slices.Contains(p.Blockers, "cost_unsettled") {
			t.Fatal(p)
		}
		in := input(t, d)
		in.SettlementMode = "no_payable_obligation"
		complete(t, d, in, 409)
	})
	t.Run("settled delivery pins immutable complete facts and exact original approval", func(t *testing.T) {
		d, _, _ := delivered(t)
		_, _ = paid(t, d)
		p := preview(t, d)
		if !p.CanComplete || p.HasDifference || len(p.Basis.Costs) != 1 || len(p.Basis.Payments) != 1 {
			t.Fatal(p)
		}
		in := input(t, d)
		in.DifferenceDisposition = ""
		receipt := complete(t, d, in, 201)
		now := get(t, d.ID)
		z := record(t, d)
		if now.Lifecycle != "completed" || now.ContentHash != d.ContentHash || now.Version != d.Version+1 || now.CanCreateVariance || receipt.Version != now.Version || z.BasisState != "current" || len(z.Attachments) != 1 || z.PurchaseVersion != now.Version {
			t.Fatal(now, z, receipt)
		}
		assertProjectCloseoutItem(t, f, *original.File.ProjectID, "purchases", d.ID, false)
		for _, q := range []string{"UPDATE purchase_completions SET attestation='rewrite' WHERE id=$1", "DELETE FROM purchase_completions WHERE id=$1"} {
			if _, err := f.api.DB.Exec(ctx, q, z.ID); err == nil {
				t.Fatal("app can rewrite completion")
			}
			if _, err := f.owner.Exec(ctx, q, z.ID); err == nil {
				t.Fatal("owner can rewrite immutable completion")
			}
		}
		var pins int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='purchase_completion' AND owner_id=$1 AND version_id=$2", z.ID, original.Version.ID).Scan(&pins); err != nil || pins != 1 {
			t.Fatal(pins, err)
		}
	})
	t.Run("explicit no payable obligation requires human evidence and difference disposition", func(t *testing.T) {
		d, _, _ := delivered(t)
		p := preview(t, d)
		if !p.CanComplete || !p.HasDifference || len(p.Basis.Costs) != 0 {
			t.Fatal(p)
		}
		in := input(t, d)
		in.DifferenceDisposition = ""
		complete(t, d, in, 422)
		in.DifferenceDisposition = "Verified supplier waived the payable amount; no actual cost or cash is invented."
		in.CostsVerified = false
		complete(t, d, in, 422)
		in.CostsVerified = true
		complete(t, d, in, 201)
		z := record(t, d)
		if z.SettlementMode != "no_payable_obligation" || len(z.Basis.Costs) != 0 || len(z.Basis.Payments) != 0 {
			t.Fatal(z)
		}
	})
	t.Run("cancelled remainder retains accepted delivery and must be explicitly resolved", func(t *testing.T) {
		d, o := order(t, newPurchase(t, true, "10", "2"), "10")
		d, _ = receipt(t, d, o, "4", "accepted", nil)
		d = cancelOrder(t, d, o)
		p := preview(t, d)
		if !p.CanComplete || !p.HasDifference || !p.Basis.Orders[0].Cancelled || p.Basis.Orders[0].Lines[0].NetAcceptedQuantity != "4.000000" {
			t.Fatal(p)
		}
		complete(t, d, input(t, d), 201)
		if summary(t, get(t, d.ID)).Lifecycle != "completed" {
			t.Fatal("completion lost its execution lifecycle")
		}
	})
	t.Run("financial changes reject stale basis even when purchase version stays fixed", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
		_, _ = paid(t, d)
		if get(t, d.ID).Version != d.Version {
			t.Fatal("financial event changed purchase version")
		}
		complete(t, d, in, 409)
		complete(t, d, input(t, d), 201)
	})
	t.Run("unattributed refund blocks otherwise fully paid purchase", func(t *testing.T) {
		d, _, _ := delivered(t)
		c := newCost(t, d, "20")
		cash := allocate(t, c, newPayment(t, "30", nil), "20")
		_ = newPayment(t, "5", &cash)
		p := preview(t, d)
		if !slices.Contains(p.Blockers, "unattributed_refund") || slices.Contains(p.Blockers, "cost_unsettled") || p.CanComplete || len(p.Basis.Payments) != 2 {
			t.Fatal(p)
		}
		complete(t, d, input(t, d), 409)
	})
	t.Run("completed purchase allows source returns while retaining exact completion history", func(t *testing.T) {
		d, o, q := delivered(t)
		complete(t, d, input(t, d), 201)
		z := record(t, d)
		d = get(t, d.ID)
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "1"), nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/cancel", f.aliceToken, core.NewID(), base(d), nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), spend.ReceiptInput{ExecutionBase: base(d), OrderID: o.ID, ReceivedOn: today, EvidenceVersionIDs: []string{original.Version.ID}, Lines: []spend.ReceiptLineInput{{OrderLineID: o.Lines[0].ID, Quantity: "1", Result: "accepted", Reason: "must not reacquire delivery"}}}, nil)
		expect(t, r, 409, v)
		d, _ = receipt(t, d, o, "1", "returned", &q.Lines[0].ID)
		after := record(t, d)
		if d.Lifecycle != "completed" || after.BasisState != "changed" || after.Fingerprint != z.Fingerprint || after.PurchaseVersion != z.PurchaseVersion || after.Basis.Orders[0].Lines[0].NetAcceptedQuantity != "10.000000" {
			t.Fatal(d, after)
		}
		assertProjectCloseoutItem(t, f, *original.File.ProjectID, "purchases", d.ID, true)
	})
	t.Run("late actual cost does not rewrite prior no payable completion", func(t *testing.T) {
		d, _, _ := delivered(t)
		complete(t, d, input(t, d), 201)
		old := record(t, d)
		_ = newCost(t, get(t, d.ID), "10")
		now := record(t, d)
		if now.BasisState != "changed" || now.Fingerprint != old.Fingerprint || len(now.Basis.Costs) != 0 || get(t, d.ID).Lifecycle != "completed" {
			t.Fatal(now)
		}
	})
	t.Run("concurrent humans create exactly one completion", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
		statuses := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/completion", f.aliceToken, core.NewID(), in, nil)
				statuses <- r.Code
			}()
		}
		wg.Wait()
		close(statuses)
		ok, conflict := 0, 0
		for s := range statuses {
			if s == 201 {
				ok++
			} else if s == 409 {
				conflict++
			} else {
				t.Fatal(s)
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatal(ok, conflict)
		}
	})
	t.Run("audit failure atomically rolls back completion head proof and receipt", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
		key := core.NewID()
		path := "/v1/purchases/" + d.ID + "/completion"
		exec(t, `CREATE FUNCTION fail_purchase_completion_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='purchase.completed' THEN RAISE EXCEPTION 'isolated completion audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_purchase_completion_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_purchase_completion_audit()`)
		defer exec(t, "DROP TRIGGER IF EXISTS fail_purchase_completion_audit ON audit_events;DROP FUNCTION IF EXISTS fail_purchase_completion_audit()")
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 500, v)
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM purchase_completions WHERE purchase_id=$1)+(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2)", d.ID, key).Scan(&n); err != nil || n != 0 || get(t, d.ID).Version != d.Version {
			t.Fatal(n, err)
		}
		exec(t, "DROP TRIGGER fail_purchase_completion_audit ON audit_events;DROP FUNCTION fail_purchase_completion_audit()")
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
	})
	t.Run("actual current MFA human channel and cached receipts are independently checked", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
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
		service := spend.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		command := core.Command{Actor: actor, RequestID: core.NewID(), Key: core.NewID(), Route: "POST /v1/purchases/" + d.ID + "/completion", Body: raw}
		_, err = service.CompletePurchase(ctx, command, d.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		command.Actor.RunID = core.NewID()
		_, err = service.CompletePurchase(ctx, command, d.ID, in)
		if status := core.PublicError(err).Status; status != 403 && status != 404 {
			t.Fatal(err)
		}
		command.Actor.RunID = ""
		if _, err = service.CompletePurchase(ctx, command, d.ID, in); err != nil {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		_, err = service.CompletePurchase(ctx, command, d.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("cached receipt bypassed current MFA", err)
		}
	})
	t.Run("idempotent replay must respect revoked completion authorization", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
		key := core.NewID()
		path := "/v1/purchases/" + d.ID + "/completion"
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		first := data[spend.CompletionCommandReceipt](t, v)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[spend.CompletionCommandReceipt](t, v) != first {
			t.Fatal("receipt changed")
		}
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", completer)
		defer exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE id=$1", completer)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 {
			t.Fatal(r.Code, v)
		}
	})
	t.Run("limited readers see unknown rather than empty financial evidence", func(t *testing.T) {
		d, _, _ := delivered(t)
		complete(t, d, input(t, d), 201)
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/completion-preview", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		p := data[spend.CompletionPreview](t, v)
		if p.Visibility != "restricted" || p.Basis != nil || p.Fingerprint != nil || p.CanComplete || len(p.SettlementModes) != 0 {
			t.Fatal(p)
		}
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/completion", f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
	})
	t.Run("explicit date evidence and all manual attestations are required", func(t *testing.T) {
		d, _, _ := delivered(t)
		in := input(t, d)
		in.ExpectedFingerprint = strings.Repeat("0", 32)
		complete(t, d, in, 409)
		in = input(t, d)
		for _, mode := range []string{"delivery", "costs", "differences", "attestation", "evidence", "date"} {
			bad := in
			switch mode {
			case "delivery":
				bad.DeliveryVerified = false
			case "costs":
				bad.CostsVerified = false
			case "differences":
				bad.DifferencesVerified = false
			case "attestation":
				bad.Attestation = ""
			case "evidence":
				bad.EvidenceVersionIDs = []string{}
			case "date":
				bad.OccurredOn = "2099-01-01"
			}
			complete(t, d, bad, 422)
		}
	})
	t.Run("financial access revocation hides history and prevents cached receipt replay", func(t *testing.T) {
		d, _, _ := delivered(t)
		_, _ = paid(t, d)
		in, key := input(t, d), core.NewID()
		path := "/v1/purchases/" + d.ID + "/completion"
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		rows, err := f.owner.Query(ctx, `UPDATE role_bindings b SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND EXISTS(SELECT 1 FROM role_actions a WHERE a.org_id=b.org_id AND a.role_id=b.role_id AND a.action='cost.read') RETURNING id::text`, f.org, f.alice)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		if rows.Err() != nil || len(ids) == 0 {
			t.Fatal("no actual cost access revoked", rows.Err())
		}
		defer exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE id=ANY($1::uuid[])", ids)
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/completion-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		p := data[spend.CompletionPreview](t, v)
		if p.Visibility != "restricted" || p.Basis != nil || p.Fingerprint != nil || len(p.SettlementModes) != 0 {
			t.Fatal(p)
		}
		r, v = f.with(t).request("GET", path, f.aliceToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 {
			t.Fatal("cached receipt leaked after financial revocation", r.Code, v)
		}
	})
	t.Run("database alone rejects forged completion head", func(t *testing.T) {
		d, _, _ := delivered(t)
		_, err := f.owner.Exec(ctx, "UPDATE spend_documents SET lifecycle='completed',version=version+1 WHERE id=$1", d.ID)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
			t.Fatal("missing independent SQL head guard", err)
		}
		if get(t, d.ID).Version != d.Version {
			t.Fatal("forged head persisted")
		}
	})
	t.Run("database completion constraints require actual basis session evidence and matching head", func(t *testing.T) {
		for _, mode := range []string{"valid", "basis", "fingerprint", "mfa", "declaration", "evidence", "head", "unsettled"} {
			t.Run(mode, func(t *testing.T) {
				d, _, _ := delivered(t)
				if mode == "unsettled" {
					_ = newCost(t, d, "20")
				}
				in, p := input(t, d), preview(t, d)
				actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(p.Basis)
				if mode == "basis" {
					raw = []byte(`{}`)
				}
				if mode == "fingerprint" {
					in.ExpectedFingerprint = strings.Repeat("0", 32)
				}
				if mode == "declaration" {
					in.CostsVerified = false
				}
				tx, err := f.owner.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				id := core.NewID()
				_, err = tx.Exec(ctx, `INSERT INTO purchase_completions(id,org_id,purchase_id,revision_id,purchase_version,occurred_on,basis,fingerprint,delivery_verified,costs_verified,differences_verified,settlement_mode,attestation,difference_disposition,evidence_count,actor_id,session_id,mfa_verified_at,reason)
                    SELECT $1,$2,$3,$4,$5,$6,$7::jsonb,$8,true,$9,true,$10,$11,$12,1,$13,s.id,CASE WHEN $16='mfa' THEN s.mfa_verified_at-interval '11 minutes' ELSE s.mfa_verified_at END,$14 FROM sessions s WHERE s.id=$15`,
					id, f.org, d.ID, d.CurrentRevisionID, d.Version+1, today, raw, in.ExpectedFingerprint, in.CostsVerified, in.SettlementMode, in.Attestation, in.DifferenceDisposition, f.alice, in.Reason, actor.SessionID, mode)
				if err == nil && mode != "evidence" {
					_, err = tx.Exec(ctx, "INSERT INTO purchase_completion_files(org_id,completion_id,version_id) VALUES($1,$2,$3)", f.org, id, original.Version.ID)
				}
				if err == nil && mode != "head" {
					_, err = tx.Exec(ctx, `INSERT INTO purchase_execution_events(id,org_id,purchase_id,seq,purchase_version,kind,lifecycle,actor_id,session_id,mfa_verified_at,reason)
                        SELECT $1,$2,$3,(SELECT max(seq)+1 FROM purchase_execution_events WHERE org_id=$2 AND purchase_id=$3),$4,'purchase_completed','completed',$5,s.id,s.mfa_verified_at,$6 FROM sessions s WHERE s.id=$7`, core.NewID(), f.org, d.ID, d.Version+1, f.alice, in.Reason, actor.SessionID)
					if err == nil {
						_, err = tx.Exec(ctx, "UPDATE spend_documents SET lifecycle='completed',version=version+1 WHERE id=$1", d.ID)
					}
				}
				if err == nil {
					err = tx.Commit(ctx)
				} else {
					_ = tx.Rollback(ctx)
				}
				if mode == "valid" {
					if err != nil {
						t.Fatal("valid SQL control failed", err)
					}
					if record(t, d).BasisState != "current" {
						t.Fatal("valid SQL control changed basis")
					}
					return
				}
				var pgerr *pgconn.PgError
				if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
					t.Fatal("SQL constraint did not independently reject "+mode, err)
				}
				if get(t, d.ID).Version != d.Version {
					t.Fatal("invalid SQL completion persisted")
				}
			})
		}
	})
	t.Run("in review difference blocks completion and unused approved proposal stays immutable", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "completion-variance", Name: "Synthetic completion difference policy", Kind: "purchase_variance", Category: "fulfillment", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent variance review", AssignedUserID: f.bob}}, Reason: "isolated completion proposal boundary"}, nil)
		expect(t, r, 201, v)
		d, _, _ := delivered(t)
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/execution-plan", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		plan := data[spend.ExecutionPlan](t, v)
		creation := spend.VarianceCreate{Content: plan.Content, ExpectedPurchaseVersion: d.Version, ExecutionPlanRevisionID: plan.VarianceRevisionID, Reason: "unused proposed scope is retained"}
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), creation, nil)
		expect(t, r, 201, v)
		proposal := data[spend.Variance](t, v)
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "purchase_variance", BusinessID: proposal.ID, ExpectedBusinessVersion: proposal.Version, BusinessRevisionID: proposal.CurrentRevisionID, BusinessContentHash: proposal.ContentHash, Reason: "independent difference approval"}, nil)
		expect(t, r, 201, v)
		q := data[workflow.Request](t, v)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		pre := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: pre.Workflow.ID, PreviewHash: pre.PreviewHash, Reason: "submit accurate complete difference"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		if p := preview(t, d); p.CanComplete || !slices.Contains(p.Blockers, "variance_in_review") {
			t.Fatal(p)
		}
		complete(t, d, input(t, d), 409)
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "approved proposal still needs explicit activation"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1/purchase-variances/"+proposal.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		proposal = data[spend.Variance](t, v)
		p := preview(t, d)
		if !p.CanComplete || !p.HasDifference || len(p.Basis.Variances) != 1 || p.Basis.Variances[0].Lifecycle == "effective" {
			t.Fatal(p)
		}
		in := input(t, d)
		in.DifferenceDisposition = ""
		complete(t, d, in, 422)
		complete(t, d, input(t, d), 201)
		now := get(t, d.ID)
		r, v = f.with(t).request("POST", "/v1/purchase-variances/"+proposal.ID+"/activate", f.aliceToken, core.NewID(), spend.VarianceActivation{ExecutionBase: spend.ExecutionBase{ExpectedVersion: proposal.Version, RevisionID: proposal.CurrentRevisionID, ContentHash: proposal.ContentHash, Reason: "must not reactivate closed purchase"}, ExpectedPurchaseVersion: now.Version}, nil)
		expect(t, r, 409, v)
		creation.ExpectedPurchaseVersion = now.Version
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), creation, nil)
		expect(t, r, 409, v)
		_, err := f.owner.Exec(ctx, "UPDATE purchase_variances SET version=version+1 WHERE id=$1", proposal.ID)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
			t.Fatal("SQL terminal proposal guard missing", err)
		}
		if z := record(t, d); len(z.Basis.Variances) != 1 || z.Basis.Variances[0].RevisionID != proposal.CurrentRevisionID {
			t.Fatal(z)
		}
	})
	t.Run("unapproved preview has a valid explicit nonfinancial contract", func(t *testing.T) {
		d := newPurchase(t, false, "10", "2")
		p := preview(t, d)
		if p.Visibility != "not_approved" || p.CanComplete || p.Basis != nil || p.Fingerprint != nil || !slices.Equal(p.Blockers, []string{"not_approved"}) {
			t.Fatal(p)
		}
	})
	t.Run("later refund changes the current basis without rewriting historical settlement", func(t *testing.T) {
		d, _, _ := delivered(t)
		cost := newCost(t, d, "20")
		cash := allocate(t, cost, newPayment(t, "30", nil), "20")
		complete(t, d, input(t, d), 201)
		old := record(t, d)
		_ = newPayment(t, "5", &cash)
		now := record(t, d)
		p := preview(t, get(t, d.ID))
		if now.BasisState != "changed" || now.Fingerprint != old.Fingerprint || len(now.Basis.Payments) != 1 || !slices.Contains(p.Blockers, "unattributed_refund") || len(p.Basis.Payments) != 2 {
			t.Fatal(old, now, p)
		}
	})
	t.Run("fully delivered old price orders require disposition for unused value in a newer plan", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "completion-unused-value", Name: "Synthetic unused value review", Kind: "purchase_variance", Category: "unused-value", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent revised price review", AssignedUserID: f.bob}}, Reason: "isolated unused monetary scope"}, nil)
		expect(t, r, 201, v)
		d, _, _ := delivered(t)
		_, _ = paid(t, d)
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/execution-plan", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		plan := data[spend.ExecutionPlan](t, v)
		plan.Content.Category = "unused-value"
		plan.Content.Lines[0].UnitPrice = p("3")
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), spend.VarianceCreate{Content: plan.Content, ExpectedPurchaseVersion: d.Version, ExecutionPlanRevisionID: plan.VarianceRevisionID, Reason: "new cap does not change old fixed order price"}, nil)
		expect(t, r, 201, v)
		proposal := data[spend.Variance](t, v)
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "purchase_variance", BusinessID: proposal.ID, ExpectedBusinessVersion: proposal.Version, BusinessRevisionID: proposal.CurrentRevisionID, BusinessContentHash: proposal.ContentHash, Reason: "independent revised monetary scope"}, nil)
		expect(t, r, 201, v)
		q := data[workflow.Request](t, v)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		pre := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: pre.Workflow.ID, PreviewHash: pre.PreviewHash, Reason: "submit new fixed monetary cap"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "approve revised cap without repricing old order"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1/purchase-variances/"+proposal.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		proposal = data[spend.Variance](t, v)
		r, v = f.with(t).request("POST", "/v1/purchase-variances/"+proposal.ID+"/activate", f.aliceToken, core.NewID(), spend.VarianceActivation{ExecutionBase: spend.ExecutionBase{ExpectedVersion: proposal.Version, RevisionID: proposal.CurrentRevisionID, ContentHash: proposal.ContentHash, Reason: "explicit human activation of revised cap"}, ExpectedPurchaseVersion: d.Version}, nil)
		expect(t, r, 200, v)
		d = get(t, d.ID)
		preCompletion := preview(t, d)
		if !preCompletion.CanComplete || !preCompletion.HasDifference || preCompletion.Basis.ExecutionAmount != "30.000000" || preCompletion.Basis.Lines[0].ReservedAmount != "20.000000" || preCompletion.Basis.Lines[0].NetAcceptedQuantity != "10.000000" {
			t.Fatal(preCompletion)
		}
		in := input(t, d)
		in.DifferenceDisposition = ""
		complete(t, d, in, 422)
		var difference bool
		if err := f.owner.QueryRow(ctx, "SELECT purchase_completion_has_difference(purchase_completion_snapshot($1,$2))", f.org, d.ID).Scan(&difference); err != nil || !difference {
			t.Fatal("independent SQL ignored unused monetary cap", difference, err)
		}
		in.DifferenceDisposition = "The remaining HKD 10 approved monetary cap is unused; the old fully delivered order and actual expense remain HKD 20."
		complete(t, d, in, 201)
		z := record(t, d)
		if z.BasisState != "current" || z.Basis.ExecutionAmount != "30.000000" || z.Basis.Costs[0].Amount == nil || *z.Basis.Costs[0].Amount != "20.000000" {
			t.Fatal(z)
		}
	})
}
