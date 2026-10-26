package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func TestPurchaseFulfillment(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
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
	for _, key := range []string{"purchase_manager", "party_manager", "request_manager"} {
		bind(f.alice, key)
	}
	operator := bind(f.alice, "purchase_operator")
	for _, key := range []string{"purchase_reader", "party_manager", "approval_reviewer"} {
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

	t.Run("only approved exact scope can register orders", func(t *testing.T) {
		d := newPurchase(t, false, "10", "2")
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "1"), nil)
		expect(t, r, 409, v)
		d = newPurchase(t, true, "10", "2")
		in := orderInput(d, "1")
		in.ContentHash = strings.Repeat("0", 64)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in = orderInput(d, "1")
		in.Supplier.RevisionID = core.NewID()
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in = orderInput(d, "1")
		in.Lines[0].UnitPrice = "2.01"
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		if len(orders(t, d)) != 0 {
			t.Fatal("invalid scope produced order")
		}
	})
	t.Run("order registration pins exact facts evidence and approval without cash", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		before := d
		in := orderInput(d, "4")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		d = data[spend.Record](t, v)
		o := orders(t, d)[0]
		if o.Amount != "8.000000" || o.State != "open" || len(o.Attachments) != 1 || o.SupplierEvidence == nil || o.ActorID != f.alice || d.Lifecycle != "ordering" || d.CurrentRevisionID != before.CurrentRevisionID || d.EmployeeReimbursementAmount != nil || !d.CanExecute {
			t.Fatal(d, o)
		}
		for _, sql := range []string{"UPDATE purchase_orders SET notes='rewrite' WHERE id=$1", "DELETE FROM purchase_orders WHERE id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, o.ID); err == nil {
				t.Fatal("mutable order")
			}
		}
		if _, err := f.owner.Exec(ctx, "UPDATE purchase_order_lines SET quantity=quantity+1 WHERE order_id=$1", o.ID); err == nil {
			t.Fatal("mutable ordered quantity")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='purchase_order' AND owner_id=$1 AND version_id=$2", o.ID, original.Version.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if len(orders(t, d)) != 1 {
			t.Fatal("replay duplicated order")
		}
		s := summary(t, d)
		if s.Lines[0].ReservedQuantity != "4.000000" || s.Lines[0].ReservedAmount != "8.000000" || s.Lines[0].NetAcceptedQuantity != "0.000000" {
			t.Fatal(s)
		}
		bind(f.alice, "file_retention_manager")
		var fileVersion int64
		if err := f.owner.QueryRow(ctx, "SELECT version FROM files WHERE org_id=$1 AND id=$2", f.org, original.File.ID).Scan(&fileVersion); err != nil {
			t.Fatal(err)
		}
		r, v = f.with(t).request("POST", "/v1/file-versions/"+original.Version.ID+"/retention-assessments", f.aliceToken, core.NewID(), map[string]any{"expected_file_version": fileVersion, "reason": "exact order and purchase references remain protected"}, nil)
		expect(t, r, 201, v)
		assessment := data[storage.RetentionAssessment](t, v)
		if assessment.Snapshot.ReferenceCounts["purchase_revision"] < 1 || assessment.Snapshot.ReferenceCounts["purchase_order"] != 1 || assessment.Snapshot.ExecutionAvailable {
			t.Fatal(assessment)
		}
	})
	t.Run("multiple orders share one bounded approval and duplicate supplier references are refused", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "4")
		in := orderInput(d, "2")
		in.ExternalRef = strings.ToLower(o.ExternalRef)
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		d, _ = order(t, d, "6")
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "0.000001"), nil)
		expect(t, r, 409, v)
		if s := summary(t, d); s.OrderCount != 2 || s.Lines[0].ReservedQuantity != "10.000000" {
			t.Fatal(s)
		}
	})
	t.Run("partial acceptance rejection replacement and source specific returns append", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "10")
		in := receiptInput(d, o, "4", "accepted", nil)
		in.Lines = append(in.Lines, spend.ReceiptLineInput{OrderLineID: o.Lines[0].ID, Quantity: "2", Result: "rejected", Reason: "damaged two units"})
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d = data[spend.Record](t, v)
		first := receipts(t, d)[0]
		if d.Lifecycle != "receiving" || summary(t, d).Lines[0].NetAcceptedQuantity != "4.000000" {
			t.Fatal(d)
		}
		d, _ = receipt(t, d, o, "6", "accepted", nil)
		if orders(t, d)[0].State != "fulfilled" {
			t.Fatal("replacement not accepted")
		}
		d, ret := receipt(t, d, o, "3", "returned", &first.Lines[0].ID)
		if ret.Lines[0].ReversesLineID == nil || orders(t, d)[0].Lines[0].NetAcceptedQuantity != "7.000000" {
			t.Fatal(ret)
		}
		if receipts(t, d)[2].Lines[0].ReturnableQuantity != "1.000000" {
			t.Fatal("return source did not retain remaining quantity")
		}
		in = receiptInput(d, o, "2", "returned", &first.Lines[0].ID)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		d, _ = receipt(t, d, o, "3", "accepted", nil)
		if orders(t, d)[0].State != "fulfilled" || d.Lifecycle != "receiving" {
			t.Fatal("delivery invented financial closure", d)
		}
	})
	t.Run("a delivery batch cannot overreceive through rejection or return mixing", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "10")
		d, accepted := receipt(t, d, o, "8", "accepted", nil)
		for _, result := range []string{"accepted", "rejected"} {
			in := receiptInput(d, o, "3", result, nil)
			r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), in, nil)
			expect(t, r, 409, v)
		}
		in := receiptInput(d, o, "2", "accepted", nil)
		in.Lines = append(in.Lines, spend.ReceiptLineInput{OrderLineID: o.Lines[0].ID, Quantity: "1", Result: "returned", ReversesLineID: &accepted.Lines[0].ID, Reason: "mixed fact"})
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		if len(receipts(t, d)) != 1 {
			t.Fatal("invalid receipt persisted")
		}
	})
	t.Run("returns cannot reverse another order or rejected or returned source", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, a := order(t, d, "5")
		d, b := order(t, d, "5")
		d, accepted := receipt(t, d, a, "2", "accepted", nil)
		d, rejected := receipt(t, d, a, "1", "rejected", nil)
		for _, source := range []struct {
			o  spend.Order
			id string
		}{{b, accepted.Lines[0].ID}, {a, rejected.Lines[0].ID}, {a, core.NewID()}} {
			r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), receiptInput(d, source.o, "1", "returned", &source.id), nil)
			expect(t, r, 404, v)
		}
	})
	t.Run("cancelling remainder releases only unaccepted commitment and preserves order fact", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "10")
		d, accepted := receipt(t, d, o, "4", "accepted", nil)
		d = cancelOrder(t, d, o)
		if s := summary(t, d); s.CancelledOrderCount != 1 || s.Lines[0].ReservedQuantity != "4.000000" || s.Lines[0].ReservedAmount != "8.000000" || s.Lines[0].OutstandingQuantity != "0.000000" {
			t.Fatal(s)
		}
		if original := orders(t, d)[0]; original.Amount != "20.000000" || original.Lines[0].Quantity != "10.000000" || original.State != "cancelled" {
			t.Fatal(original)
		}
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), receiptInput(d, o, "1", "accepted", nil), nil)
		expect(t, r, 409, v)
		d, _ = order(t, d, "6")
		d, _ = receipt(t, d, o, "1", "returned", &accepted.Lines[0].ID)
		if s := summary(t, d); s.Lines[0].ReservedQuantity != "9.000000" || s.Lines[0].ReservedAmount != "18.000000" {
			t.Fatal(s)
		}
	})
	t.Run("rounding reserves conservatively and never manufactures actual cost or refunds", func(t *testing.T) {
		d := newPurchase(t, true, "3", "0.003333")
		d, o := order(t, d, "3")
		d, _ = receipt(t, d, o, "1", "accepted", nil)
		d = cancelOrder(t, d, o)
		if s := summary(t, d); s.Lines[0].ReservedAmount != "0.010000" || s.Lines[0].ReservedQuantity != "1.000000" {
			t.Fatal(s)
		}
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "2"), nil)
		expect(t, r, 409, v)
	})
	t.Run("purchase cancellation refuses open orders and keeps later returns in cancelled lifecycle", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "10")
		d, accepted := receipt(t, d, o, "4", "accepted", nil)
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/cancel", f.aliceToken, core.NewID(), base(d), nil)
		expect(t, r, 409, v)
		d = cancelOrder(t, d, o)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/cancel", f.aliceToken, core.NewID(), base(d), nil)
		expect(t, r, 200, v)
		d = data[spend.Record](t, v)
		d, _ = receipt(t, d, o, "2", "returned", &accepted.Lines[0].ID)
		if d.Lifecycle != "cancelled" || d.ApprovalState != "approved" || len(receipts(t, d)) != 2 {
			t.Fatal(d)
		}
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "1"), nil)
		expect(t, r, 409, v)
	})
	t.Run("returns after a fully accepted purchase was cancelled release its closed commitment", func(t *testing.T) {
		d := newPurchase(t, true, "3", "2")
		d, o := order(t, d, "3")
		d, accepted := receipt(t, d, o, "3", "accepted", nil)
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/cancel", f.aliceToken, core.NewID(), base(d), nil)
		expect(t, r, 200, v)
		d = data[spend.Record](t, v)
		d, _ = receipt(t, d, o, "1", "returned", &accepted.Lines[0].ID)
		if s := summary(t, d); s.Lifecycle != "cancelled" || s.Lines[0].ReservedQuantity != "2.000000" || s.Lines[0].OutstandingQuantity != "0.000000" {
			t.Fatal(s)
		}
	})
	t.Run("concurrent order and receipt commands have one winning version", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		in := orderInput(d, "6")
		statuses := make(chan int, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
				statuses <- r.Code
			}()
		}
		wg.Wait()
		close(statuses)
		ok, conflict := 0, 0
		for code := range statuses {
			if code == 201 {
				ok++
			} else if code == 409 {
				conflict++
			} else {
				t.Fatal(code)
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatal(ok, conflict)
		}
		d = get(t, d.ID)
		o := orders(t, d)[0]
		receiptIn := receiptInput(d, o, "4", "accepted", nil)
		statuses = make(chan int, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/receipts", f.aliceToken, core.NewID(), receiptIn, nil)
				statuses <- r.Code
			}()
		}
		wg.Wait()
		close(statuses)
		ok, conflict = 0, 0
		for code := range statuses {
			if code == 201 {
				ok++
			} else if code == 409 {
				conflict++
			} else {
				t.Fatal(code)
			}
		}
		if ok != 1 || conflict != 1 || len(receipts(t, d)) != 1 {
			t.Fatal(ok, conflict)
		}
	})
	t.Run("history pagination and parent authorization are bound to the current reader", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		for range 3 {
			d, _ = order(t, d, "1")
		}
		r, v := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[spend.Order]](t, v)
		if page.NextCursor == nil {
			t.Fatal("missing cursor")
		}
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[core.Page[spend.Order]](t, v).Items[0].ID == page.Items[0].ID {
			t.Fatal("cursor duplicated item")
		}
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, r, 400, v)
		for _, path := range []string{"/orders", "/orders/" + page.Items[0].ID, "/receipts", "/execution-events", "/fulfillment"} {
			r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+path, admin, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/execution-events?limit=100", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		events := data[core.Page[spend.ExecutionEvent]](t, v).Items
		if len(events) != 3 || events[0].Seq != 3 {
			t.Fatal(events)
		}
	})
	t.Run("reader cannot execute and recent database MFA is checked before cached replay", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		in := orderInput(d, "1")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.bobToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 {
			t.Fatal(r.Code, v)
		}
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
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
		command := core.Command{Actor: actor, RequestID: core.NewID(), Key: key, Route: "POST /v1/purchases/" + d.ID + "/orders", Body: raw}
		_, err = service.RegisterOrder(ctx, command, d.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		command.Actor.RunID = core.NewID()
		_, err = service.RegisterOrder(ctx, command, d.ID, in)
		if status := core.PublicError(err).Status; status != 403 && status != 404 {
			t.Fatal(err)
		}
	})
	t.Run("invalid dates and unavailable evidence leave no execution facts", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		in := orderInput(d, "1")
		in.OrderedOn = "2999-01-01"
		in.ExpectedDeliveryOn = in.OrderedOn
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		in = orderInput(d, "1")
		exec(t, "UPDATE file_versions SET availability='missing' WHERE id=$1", original.Version.ID)
		defer exec(t, "UPDATE file_versions SET availability='available' WHERE id=$1", original.Version.ID)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		if len(orders(t, d)) != 0 {
			t.Fatal("missing evidence committed")
		}
	})
	t.Run("audit failure rolls back order file pin head event and replay receipt", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		in := orderInput(d, "1")
		key := core.NewID()
		exec(t, `CREATE FUNCTION fail_purchase_execution_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='purchase.order.registered' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_purchase_execution_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_purchase_execution_audit()`)
		drop := func() {
			exec(t, "DROP TRIGGER IF EXISTS fail_purchase_execution_audit ON audit_events; DROP FUNCTION IF EXISTS fail_purchase_execution_audit()")
		}
		defer drop()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 500, v)
		if current := get(t, d.ID); current.Version != d.Version || current.Lifecycle != "requested" {
			t.Fatal(current)
		}
		if len(orders(t, d)) != 0 {
			t.Fatal("audit failure persisted order")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM purchase_execution_events WHERE purchase_id=$1", d.ID).Scan(&n); err != nil || n != 0 {
			t.Fatal(n, err)
		}
		drop()
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
	})
	t.Run("database guards reject detached execution heads and immutable fact rewrites", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		if _, err := f.owner.Exec(ctx, "UPDATE spend_documents SET lifecycle='ordering',version=version+1 WHERE id=$1", d.ID); err == nil {
			t.Fatal("head had no execution fact")
		}
		d, o := order(t, d, "5")
		d, a := receipt(t, d, o, "1", "accepted", nil)
		if _, err := f.owner.Exec(ctx, "UPDATE purchase_receipt_lines SET quantity=quantity+1 WHERE receipt_id=$1", a.ID); err == nil {
			t.Fatal("receipt rewrite")
		}
		if _, err := f.api.DB.Exec(ctx, "DELETE FROM purchase_execution_events WHERE purchase_id=$1", d.ID); err == nil {
			t.Fatal("execution delete")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE spend_documents SET lifecycle='cancelled',version=version+1 WHERE id=$1", d.ID); err == nil {
			t.Fatal("head detached from last event")
		}
	})
	t.Run("database independently rejects overcommitted orders and reused event subjects", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		d, o := order(t, d, "5")
		for _, forgeOrder := range []bool{true, false} {
			tx, err := f.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer tx.Rollback(ctx)
				orderID := o.ID
				if forgeOrder {
					orderID = core.NewID()
					_, err = tx.Exec(ctx, `INSERT INTO purchase_orders(id,org_id,purchase_id,revision_id,purchase_version,request_revision_id,supplier_id,supplier_revision_id,external_ref,external_ref_key,ordered_on,expected_delivery_on,currency,amount,line_count,evidence_version_ids,notes)
                        SELECT $1::uuid,org_id,purchase_id,revision_id,$2,request_revision_id,supplier_id,supplier_revision_id,$1::uuid::text,$1::uuid::text,ordered_on,expected_delivery_on,currency,12,1,evidence_version_ids,'' FROM purchase_orders WHERE id=$3`, orderID, d.Version+1, o.ID)
					if err != nil {
						t.Fatal(err)
					}
					_, err = tx.Exec(ctx, `INSERT INTO purchase_order_lines(id,org_id,purchase_id,order_id,revision_id,position,purchase_line_id,description,unit,quantity,unit_price,tax_amount,discount_amount,amount)
                        SELECT $1,org_id,purchase_id,$2,revision_id,1,purchase_line_id,description,unit,6,unit_price,0,0,12 FROM purchase_order_lines WHERE order_id=$3`, core.NewID(), orderID, o.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				_, err = tx.Exec(ctx, `INSERT INTO purchase_execution_events(id,org_id,purchase_id,seq,purchase_version,kind,order_id,lifecycle,actor_id,session_id,mfa_verified_at,reason)
                    SELECT $1,org_id,purchase_id,seq+1,purchase_version+1,'order_registered',$2,'ordering',actor_id,session_id,mfa_verified_at,'isolated invalid SQL fact' FROM purchase_execution_events WHERE org_id=$3 AND purchase_id=$4 ORDER BY seq DESC LIMIT 1`, core.NewID(), orderID, f.org, d.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, "UPDATE spend_documents SET version=version+1 WHERE id=$1", d.ID); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err == nil {
					t.Fatal("database accepted an invalid independently constructed fact")
				}
				reason := "execution events require chronological exact subjects"
				if forgeOrder {
					reason = "cumulative commitments exceed approved purchase scope"
				}
				if !strings.Contains(err.Error(), reason) {
					t.Fatal("unexpected independent database rejection", err)
				}
			}()
			if len(orders(t, d)) != 1 || get(t, d.ID).Version != d.Version {
				t.Fatal("failed SQL guard left an extra order or head")
			}
		}
	})
	t.Run("project read only phase fences execution", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		var state string
		if err := f.owner.QueryRow(ctx, "SELECT state FROM projects WHERE id=$1", *d.ProjectID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		exec(t, "UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", *d.ProjectID)
		defer exec(t, "UPDATE projects SET state=$2,version=version+1 WHERE id=$1", *d.ProjectID, state)
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(d, "1"), nil)
		expect(t, r, 409, v)
		if get(t, d.ID).CanExecute {
			t.Fatal("readonly project offered execution")
		}
	})
	t.Run("revoking execution permission fences an already successful idempotency key", func(t *testing.T) {
		d := newPurchase(t, true, "10", "2")
		in := orderInput(d, "1")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		exec(t, "DELETE FROM role_bindings WHERE id=$1", operator)
		r, v = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 && r.Code != 412 {
			t.Fatal(r.Code, v)
		}
	})
}
