package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
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

func TestPurchaseVariance(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	var diagnostic bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostic, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostic.String())
		}
	})
	ctx := context.Background()
	ptr := func(s string) *string { return &s }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	var initial int
	if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM role_bindings b JOIN roles r ON r.org_id=b.org_id AND r.id=b.role_id WHERE b.org_id=$1 AND r.key IN ('purchase_variance_manager','purchase_variance_reader','purchase_plan_activator')`, f.org).Scan(&initial); err != nil || initial != 0 {
		t.Fatal("new roles must be unbound", initial, err)
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	aliceBindings := map[string]string{}
	for _, key := range []string{"purchase_manager", "purchase_operator", "purchase_variance_manager", "purchase_plan_activator", "party_manager", "request_manager", "cost_recorder", "cost_confirmer"} {
		aliceBindings[key] = bind(f.alice, key)
	}
	parentReader := bind(f.bob, "purchase_reader")
	for _, key := range []string{"purchase_variance_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	policy := func(kind, key string) {
		r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: key, Name: "Synthetic " + kind + " policy", Kind: kind, Category: "variance", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent exact commercial review", AssignedUserID: f.bob}}, Reason: "isolated independent variance policy"}, nil)
		expect(t, r, 201, v)
	}
	policy("purchase", "variance-purchase")
	r, v := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic variance supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated supplier"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if err := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
		t.Fatal(err)
	}
	getPurchase := func(t *testing.T, id string) spend.Record {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Record](t, v)
	}
	getVariance := func(t *testing.T, id string) spend.Variance {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchase-variances/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Variance](t, v)
	}
	getPlan := func(t *testing.T, id string) spend.ExecutionPlan {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/purchases/"+id+"/execution-plan", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.ExecutionPlan](t, v)
	}
	prepare := func(t *testing.T, kind, id string, version int64, revision, hash string) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: kind, BusinessID: id, ExpectedBusinessVersion: version, BusinessRevisionID: revision, BusinessContentHash: hash, Reason: "fix independent exact proposal"}, nil)
		expect(t, r, 201, v)
		return data[workflow.Request](t, v)
	}
	submit := func(t *testing.T, q workflow.Request) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit accurately reviewed scope"}, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	decide := func(t *testing.T, q workflow.Request, decision string) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: decision, Reason: "independent exact proposal decision"}, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	newPurchase := func(t *testing.T) spend.Record {
		t.Helper()
		in := spend.Create{Content: spend.Content{Title: "Synthetic original commercial purchase", Category: "variance", Purpose: "Isolated project equipment", OwnerID: &f.alice, Currency: ptr("HKD"), PricingBasis: "net_plus_tax", RoundingMode: "half_up", ExpectedDeliveryDate: &today, SupplierCandidates: []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}, QuoteVersionIDs: []string{file.Version.ID}, Lines: []spend.Line{{ID: core.NewID(), Description: "Fixed approved tool", Quantity: ptr("10"), Unit: "unit", UnitPrice: ptr("2"), TaxAmount: ptr("0"), DiscountAmount: ptr("0")}}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated original approval"}
		r, v := f.with(t).request("POST", "/v1/purchases", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d := data[spend.Record](t, v)
		decide(t, submit(t, prepare(t, "purchase", d.ID, d.Version, d.CurrentRevisionID, d.ContentHash)), "approve")
		return getPurchase(t, d.ID)
	}
	proposalInput := func(t *testing.T, d spend.Record, qty, price string) spend.VarianceCreate {
		plan := getPlan(t, d.ID)
		var content spend.Content
		raw, _ := json.Marshal(plan.Content)
		if err := json.Unmarshal(raw, &content); err != nil {
			t.Fatal(err)
		}
		content.Lines[0].Quantity = ptr(qty)
		content.Lines[0].UnitPrice = ptr(price)
		return spend.VarianceCreate{Content: content, ExpectedPurchaseVersion: d.Version, ExecutionPlanRevisionID: plan.VarianceRevisionID, Reason: "synthetic price and quantity variance"}
	}
	proposal := func(t *testing.T, d spend.Record, qty, price string) spend.Variance {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), proposalInput(t, d, qty, price), nil)
		expect(t, r, 201, v)
		return data[spend.Variance](t, v)
	}
	approve := func(t *testing.T, d spend.Variance) spend.Variance {
		t.Helper()
		decide(t, submit(t, prepare(t, "purchase_variance", d.ID, d.Version, d.CurrentRevisionID, d.ContentHash)), "approve")
		return getVariance(t, d.ID)
	}
	activation := func(d spend.Variance) spend.VarianceActivation {
		return spend.VarianceActivation{ExecutionBase: spend.ExecutionBase{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, Reason: "explicitly activate reviewed execution scope"}, ExpectedPurchaseVersion: d.Basis.PurchaseVersion}
	}
	activate := func(t *testing.T, d spend.Variance) spend.Variance {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchase-variances/"+d.ID+"/activate", f.aliceToken, core.NewID(), activation(d), nil)
		expect(t, r, 200, v)
		return data[spend.Variance](t, v)
	}
	orderInput := func(t *testing.T, d spend.Record, qty string) spend.OrderInput {
		plan := getPlan(t, d.ID)
		return spend.OrderInput{ExecutionBase: spend.ExecutionBase{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, Reason: "register exact enabled order"}, ExecutionPlanRevisionID: plan.VarianceRevisionID, Supplier: plan.Content.SupplierCandidates[0], ExternalRef: "SYNTH-" + core.NewID(), OrderedOn: today, ExpectedDeliveryOn: today, EvidenceVersionIDs: []string{file.Version.ID}, Lines: []spend.OrderLineInput{{PurchaseLineID: d.Lines[0].ID, Quantity: qty, UnitPrice: *plan.Content.Lines[0].UnitPrice, TaxAmount: "0", DiscountAmount: "0"}}}
	}
	order := func(t *testing.T, d spend.Record, qty string) spend.Record {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(t, d, qty), nil)
		expect(t, r, 201, v)
		return data[spend.Record](t, v)
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "isolated current incurred cost"}
	}
	newCost := func(t *testing.T, amount string) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), finance.Create{Content: finance.Content{Title: "Synthetic actual purchase cost", OccurredOn: &today, Currency: ptr("HKD"), Amount: &amount, Category: "variance", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "purchase-variance:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated cost"}, nil)
		expect(t, r, 201, v)
		c := data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), costBase(c), nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	link := func(t *testing.T, c finance.Cost, d spend.Record, status int) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), finance.LinkInput{Base: costBase(c), SourceType: "purchase", SourceID: d.ID, SourceRevisionID: d.CurrentRevisionID, Relation: "incurred_for"}, nil)
		expect(t, r, status, v)
	}

	t.Run("ordinary purchase policy cannot approve a variance", func(t *testing.T) {
		d := newPurchase(t)
		v := proposal(t, d, "12", "3")
		q := prepare(t, "purchase_variance", v.ID, v.Version, v.CurrentRevisionID, v.ContentHash)
		r, body := f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 409, body)
		if getPlan(t, d.ID).VarianceID != nil {
			t.Fatal("unreviewed proposal became execution plan")
		}
	})
	policy("purchase_variance", "variance-independent")
	t.Run("approved proposal remains inactive until explicit activation", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "12", "3"))
		plan := getPlan(t, d.ID)
		if plan.VarianceID != nil || !v.CanActivate || v.Basis.PurchaseVersion != d.Version {
			t.Fatal(v, plan)
		}
		v = activate(t, v)
		p := getPurchase(t, d.ID)
		plan = getPlan(t, d.ID)
		if v.Lifecycle != "effective" || v.CanActivate || p.Version != d.Version+1 || p.Lifecycle != "requested" || p.CurrentRevisionID != d.CurrentRevisionID || p.ContentHash != d.ContentHash || *p.Amount != "20.000000" || plan.VarianceRevisionID == nil || *plan.VarianceRevisionID != v.CurrentRevisionID || *plan.Amount != "36.000000" {
			t.Fatal(v, p, plan)
		}
		r, body := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/execution-events", f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		events := data[core.Page[spend.ExecutionEvent]](t, body)
		if len(events.Items) != 1 || events.Items[0].Kind != "plan_activated" || events.Items[0].Lifecycle != "requested" {
			t.Fatal(events)
		}
	})
	t.Run("future orders use enabled prices while old orders preserve their facts", func(t *testing.T) {
		d := order(t, newPurchase(t), "4")
		oldVersion := d.Version
		v := activate(t, approve(t, proposal(t, d, "12", "3")))
		d = getPurchase(t, d.ID)
		d = order(t, d, "8")
		r, body := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		items := data[core.Page[spend.Order]](t, body).Items
		if len(items) != 2 {
			t.Fatal(items)
		}
		for _, o := range items {
			if o.PurchaseVersion == oldVersion {
				if o.ExecutionPlanRevisionID != nil || o.Lines[0].UnitPrice != "2.000000" || o.Amount != "8.000000" {
					t.Fatal(o)
				}
			} else if o.ExecutionPlanRevisionID == nil || *o.ExecutionPlanRevisionID != v.CurrentRevisionID || o.Lines[0].UnitPrice != "3.000000" || o.Amount != "24.000000" {
				t.Fatal(o)
			}
		}
		r, body = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/fulfillment", f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		summary := data[spend.Fulfillment](t, body)
		if summary.Lines[0].ApprovedQuantity != "12.000000" || summary.Lines[0].ReservedQuantity != "12.000000" || summary.Lines[0].ReservedAmount != "32.000000" {
			t.Fatal(summary)
		}
		r, body = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), orderInput(t, d, "1"), nil)
		expect(t, r, 409, body)
	})
	t.Run("old order forms cannot omit an enabled plan", func(t *testing.T) {
		d := newPurchase(t)
		in := orderInput(t, d, "1")
		activate(t, approve(t, proposal(t, d, "10", "3")))
		d = getPurchase(t, d.ID)
		in.ExpectedVersion = d.Version
		r, body := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_EXECUTION_PLAN_CHANGED" {
			t.Fatal(body.Error)
		}
	})
	t.Run("execution changes invalidate even an approved proposal", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "12", "3"))
		d = order(t, d, "1")
		in := activation(v)
		in.ExpectedPurchaseVersion = d.Version
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_VARIANCE_BASIS_CHANGED" || getPlan(t, d.ID).VarianceID != nil {
			t.Fatal(body)
		}
	})
	t.Run("new caps cannot undercut existing commitments", func(t *testing.T) {
		d := order(t, newPurchase(t), "4")
		v := approve(t, proposal(t, d, "4", "1"))
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), activation(v), nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_VARIANCE_BELOW_COMMITMENTS" {
			t.Fatal(body.Error)
		}
	})
	t.Run("actual costs use the enabled cap and stop reductions below actual spend", func(t *testing.T) {
		d := newPurchase(t)
		c := newCost(t, "30")
		link(t, c, d, 409)
		activate(t, approve(t, proposal(t, d, "10", "4")))
		d = getPurchase(t, d.ID)
		link(t, c, d, 200)
		v := approve(t, proposal(t, d, "10", "2"))
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), activation(v), nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_VARIANCE_BELOW_ACTUAL_COSTS" {
			t.Fatal(body.Error)
		}
	})
	t.Run("returned proposals rebase explicitly and preserve commercial history", func(t *testing.T) {
		d := newPurchase(t)
		v := proposal(t, d, "12", "3")
		old := v
		decide(t, submit(t, prepare(t, "purchase_variance", v.ID, v.Version, v.CurrentRevisionID, v.ContentHash)), "return")
		v = getVariance(t, v.ID)
		d = order(t, d, "1")
		in := spend.VariancePatch{VarianceCreate: proposalInput(t, d, "14", "3"), ExpectedVersion: v.Version, BaseRevisionID: v.CurrentRevisionID}
		r, body := f.with(t).request("PATCH", "/v1/purchase-variances/"+v.ID, f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, body)
		v = data[spend.Variance](t, body)
		if v.Basis.PurchaseVersion != d.Version || v.CurrentRevisionID == old.CurrentRevisionID {
			t.Fatal(v)
		}
		r, body = f.with(t).request("GET", "/v1/purchase-variances/"+v.ID+"/revisions/"+old.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		history := data[spend.VarianceRevision](t, body)
		if history.Basis.PurchaseVersion != old.Basis.PurchaseVersion || *history.Lines[0].Quantity != *old.Lines[0].Quantity || len(history.Attachments) != 1 {
			t.Fatal(history)
		}
	})
	t.Run("currency original line identity and pricing rules stay fixed", func(t *testing.T) {
		d := newPurchase(t)
		for _, change := range []func(*spend.VarianceCreate){func(in *spend.VarianceCreate) { in.Currency = ptr("USD") }, func(in *spend.VarianceCreate) { in.Lines[0].ID = core.NewID() }, func(in *spend.VarianceCreate) { in.Lines[0].Description = "A different item" }, func(in *spend.VarianceCreate) { in.RoundingMode = "half_even" }} {
			in := proposalInput(t, d, "12", "3")
			change(&in)
			r, body := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), in, nil)
			expect(t, r, 422, body)
		}
	})
	t.Run("current parent access governs requests queue history and replay", func(t *testing.T) {
		d := newPurchase(t)
		v := proposal(t, d, "12", "3")
		q := submit(t, prepare(t, "purchase_variance", v.ID, v.Version, v.CurrentRevisionID, v.ContentHash))
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", parentReader)
		defer func() { parentReader = bind(f.bob, "purchase_reader") }()
		for _, path := range []string{"/v1/purchase-variances/" + v.ID, "/v1/requests/" + q.ID} {
			r, body := f.with(t).request("GET", path, f.bobToken, "", nil, nil)
			if r.Code != 403 && r.Code != 404 {
				t.Fatal(r.Code, body)
			}
		}
		for _, path := range []string{"/v1/requests?kind=purchase_variance", "/v1/approval-tasks"} {
			r, body := f.with(t).request("GET", path, f.bobToken, "", nil, nil)
			expect(t, r, 200, body)
			if strings.Contains(string(body.Data), q.ID) {
				t.Fatal("revoked parent leaked through list", path)
			}
		}
	})
	t.Run("only one concurrent exact proposal activation wins", func(t *testing.T) {
		d := newPurchase(t)
		a := approve(t, proposal(t, d, "12", "3"))
		b := approve(t, proposal(t, d, "14", "3"))
		var wg sync.WaitGroup
		statuses := make(chan int, 2)
		for _, v := range []spend.Variance{a, b} {
			wg.Add(1)
			go func(v spend.Variance) {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), activation(v), nil)
				statuses <- r.Code
			}(v)
		}
		wg.Wait()
		close(statuses)
		counts := map[int]int{}
		for status := range statuses {
			counts[status]++
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
	})
	t.Run("audit failure rolls back plan head event and idempotency receipt", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "12", "3"))
		key := core.NewID()
		exec(`CREATE FUNCTION fail_variance_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='purchase_variance.activated' THEN RAISE EXCEPTION 'isolated variance audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_variance_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_variance_audit()`)
		defer exec("DROP TRIGGER IF EXISTS fail_variance_audit ON audit_events;DROP FUNCTION IF EXISTS fail_variance_audit()")
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, key, activation(v), nil)
		expect(t, r, 500, body)
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM purchase_plan_activations WHERE purchase_id=$1)+(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2)", d.ID, key).Scan(&count); err != nil || count != 0 || getPurchase(t, d.ID).Version != d.Version {
			t.Fatal(count, err)
		}
		exec("DROP TRIGGER fail_variance_audit ON audit_events;DROP FUNCTION fail_variance_audit()")
		r, body = f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, key, activation(v), nil)
		expect(t, r, 200, body)
	})
	t.Run("enabled plans cannot be rewritten or detached from their ledger", func(t *testing.T) {
		d := newPurchase(t)
		v := activate(t, approve(t, proposal(t, d, "12", "3")))
		for _, sql := range []string{"UPDATE purchase_plan_activations SET reason='forged' WHERE variance_id=$1", "DELETE FROM purchase_plan_activations WHERE variance_id=$1", "UPDATE purchase_variance_revisions SET amount=amount+1 WHERE document_id=$1", "UPDATE purchase_variances SET lifecycle='requested',version=version+1 WHERE id=$1"} {
			if _, err := f.owner.Exec(ctx, sql, v.ID); err == nil {
				t.Fatal("mutable enabled plan", sql)
			}
		}
		var refs int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='purchase_variance_revision' AND owner_id=$1 AND version_id=$2", v.CurrentRevisionID, file.Version.ID).Scan(&refs); err != nil || refs != 1 {
			t.Fatal(refs, err)
		}
		r, body := f.with(t).request("GET", "/v1/purchase-variances/"+v.ID+"/revisions", f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		if len(data[core.Page[spend.VarianceRevision]](t, body).Items) != 1 {
			t.Fatal(body)
		}
	})
	t.Run("successive enabled baselines fix new suppliers and preserve historical orders", func(t *testing.T) {
		d := newPurchase(t)
		first := activate(t, approve(t, proposal(t, d, "12", "3")))
		d = order(t, getPurchase(t, d.ID), "2")
		r, body := f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic alternate supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "independent alternate commercial candidate"}, nil)
		expect(t, r, 201, body)
		other := data[party.Party](t, body)
		in := proposalInput(t, d, "14", "4")
		in.SupplierCandidates = []spend.Supplier{{PartyID: other.ID, RevisionID: other.CurrentRevisionID}}
		r, body = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/variances", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, body)
		second := activate(t, approve(t, data[spend.Variance](t, body)))
		plan := getPlan(t, d.ID)
		if plan.VarianceRevisionID == nil || *plan.VarianceRevisionID != second.CurrentRevisionID || len(plan.Suppliers) != 1 || plan.Suppliers[0].PartyID != other.ID {
			t.Fatal("enabled plan omitted its actual current supplier evidence", plan)
		}
		d = order(t, getPurchase(t, d.ID), "2")
		r, body = f.with(t).request("GET", "/v1/purchases/"+d.ID+"/orders", f.aliceToken, "", nil, nil)
		expect(t, r, 200, body)
		orders := data[core.Page[spend.Order]](t, body).Items
		if len(orders) != 2 || orders[0].Supplier.PartyID != other.ID || *orders[0].ExecutionPlanRevisionID != second.CurrentRevisionID || *orders[1].ExecutionPlanRevisionID != first.CurrentRevisionID || orders[1].Supplier.PartyID != supplier.ID || orders[1].Amount != "6.000000" {
			t.Fatal("successive execution basis rewrote old supplier or prices", orders)
		}
	})
	t.Run("actual costs linked after review still block an insufficient plan", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "10", "1"))
		link(t, newCost(t, "15"), d, 200)
		if getPurchase(t, d.ID).Version != d.Version {
			t.Fatal("cost link unexpectedly changed the commercial purchase version")
		}
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), activation(v), nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_VARIANCE_BELOW_ACTUAL_COSTS" || getPlan(t, d.ID).VarianceID != nil {
			t.Fatal(body)
		}
	})
	t.Run("activation cannot silently omit costs outside current access", func(t *testing.T) {
		d := newPurchase(t)
		link(t, newCost(t, "10"), d, 200)
		v := approve(t, proposal(t, d, "12", "3"))
		rows, err := f.owner.Query(ctx, `DELETE FROM role_actions a WHERE a.org_id=$1 AND a.action='cost.read' AND EXISTS(SELECT 1 FROM role_bindings b WHERE b.org_id=a.org_id AND b.role_id=a.role_id AND b.user_id=$2) RETURNING role_id`, f.org, f.alice)
		if err != nil {
			t.Fatal(err)
		}
		var roleIDs []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			roleIDs = append(roleIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || len(roleIDs) == 0 {
			t.Fatal("missing fixture cost authorization", err)
		}
		defer func() {
			for _, id := range roleIDs {
				exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'cost.read')", f.org, id)
			}
		}()
		r, body := f.with(t).request("POST", "/v1/purchase-variances/"+v.ID+"/activate", f.aliceToken, core.NewID(), activation(v), nil)
		expect(t, r, 409, body)
		if body.Error.Code != "PURCHASE_VARIANCE_COST_SCOPE_UNAVAILABLE" {
			t.Fatal(body)
		}
	})
	t.Run("replayed proposals reauthorize current creation permission", func(t *testing.T) {
		d := newPurchase(t)
		in := proposalInput(t, d, "12", "3")
		key := core.NewID()
		path := "/v1/purchases/" + d.ID + "/variances"
		r, body := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, body)
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", aliceBindings["purchase_variance_manager"])
		defer exec("UPDATE role_bindings SET valid_until=NULL WHERE id=$1", aliceBindings["purchase_variance_manager"])
		r, body = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 {
			t.Fatal("cached proposal escaped revoked creation scope", r.Code, body)
		}
	})
	t.Run("actual current MFA and human session govern activation and cached receipts", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "12", "3"))
		in := activation(v)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		actual := actor.MFAVerifiedAt
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		defer exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		raw, _ := json.Marshal(in)
		service := spend.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		command := core.Command{Actor: actor, RequestID: core.NewID(), Key: core.NewID(), Route: "POST /v1/purchase-variances/" + v.ID + "/activate", Body: raw}
		_, err = service.ActivateVariance(ctx, command, v.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		command.Actor.RunID = core.NewID()
		_, err = service.ActivateVariance(ctx, command, v.ID, in)
		if status := core.PublicError(err).Status; status != 403 && status != 404 {
			t.Fatal(err)
		}
		command.Actor.RunID = ""
		if _, err = service.ActivateVariance(ctx, command, v.ID, in); err != nil {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		_, err = service.ActivateVariance(ctx, command, v.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("cached receipt bypassed actual MFA", err)
		}
	})
	t.Run("database independently rejects missing proof wrong arithmetic and forged MFA", func(t *testing.T) {
		d := newPurchase(t)
		v := approve(t, proposal(t, d, "12", "3"))
		for _, mode := range []string{"missing proof", "wrong arithmetic", "missing quantity", "wrong line identity"} {
			tx, err := f.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id, revision := core.NewID(), core.NewID()
			_, err = tx.Exec(ctx, `INSERT INTO purchase_variances(id,org_id,purchase_id,document_no,project_id,creator_id,classification,current_revision_id) SELECT $1,org_id,purchase_id,'CP-SYNTH-'||$1::uuid::text,project_id,creator_id,classification,$2 FROM purchase_variances WHERE id=$3`, id, revision, v.ID)
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO purchase_variance_revisions(id,org_id,document_id,purchase_id,seq,payload,basis,line_values,content_hash,amount,author_id,reason)
                SELECT $1,org_id,$2,purchase_id,1,CASE $4::text WHEN 'missing quantity' THEN payload#-'{lines,0,quantity}' WHEN 'wrong line identity' THEN jsonb_set(payload,'{lines,0,id}',to_jsonb($5::text)) ELSE payload END,basis,line_values,content_hash,CASE WHEN $4='wrong arithmetic' THEN amount+1 ELSE amount END,author_id,'isolated SQL invariant probe' FROM purchase_variance_revisions WHERE id=$3`, revision, id, v.CurrentRevisionID, mode, core.NewID())
			}
			if err == nil && mode != "missing proof" {
				_, err = tx.Exec(ctx, `INSERT INTO purchase_variance_files(org_id,revision_id,version_id) SELECT org_id,$1,version_id FROM purchase_variance_files WHERE revision_id=$2`, revision, v.CurrentRevisionID)
			}
			if err == nil {
				err = tx.Commit(ctx)
			}
			_ = tx.Rollback(ctx)
			pgerr, ok := err.(*pgconn.PgError)
			if !ok || pgerr.Code != "23514" {
				t.Fatal("unexpected independent SQL result", mode, err)
			}
		}
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		var requestRevision string
		if err = f.owner.QueryRow(ctx, "SELECT current_revision_id FROM requests WHERE id=$1", v.RequestID).Scan(&requestRevision); err != nil {
			t.Fatal(err)
		}
		_, err = f.owner.Exec(ctx, `INSERT INTO purchase_plan_activations(id,org_id,purchase_id,purchase_version,variance_id,variance_revision_id,request_revision_id,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,s.id,s.mfa_verified_at-interval '1 hour','isolated forged MFA' FROM sessions s WHERE s.id=$9`, core.NewID(), f.org, d.ID, d.Version+1, v.ID, v.CurrentRevisionID, requestRevision, f.alice, actor.SessionID)
		pgerr, ok := err.(*pgconn.PgError)
		if !ok || pgerr.Code != "23514" {
			t.Fatal("database accepted forged MFA", err)
		}
	})
}
