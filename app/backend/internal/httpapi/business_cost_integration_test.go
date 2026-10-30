package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func businessCostFixture(t *testing.T) (*fixture, storage.CommitResult, party.Party) {
	t.Helper()
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	for _, role := range []string{"cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "purchase_manager", "purchase_operator", "claim_submitter", "request_manager", "contract_manager", "contract_operator"} {
		if _, e := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, role); e != nil {
			t.Fatal(e)
		}
	}
	for _, role := range []string{"cost_reader", "approval_reviewer", "purchase_reader", "claim_reader", "contract_reader", "party_manager"} {
		if _, e := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.bob, role); e != nil {
			t.Fatal(e)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	for _, kind := range []string{"purchase", "claim", "contract"} {
		r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "business-cost-" + kind, Name: "Synthetic independent business review", Kind: kind, Category: "business-cost", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent approval", AssignedUserID: f.bob}}, Reason: "isolated business cost policy"}, nil)
		expect(t, r, 201, v)
	}
	r, v := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "业务履行演示供应方", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic business expense supplier"}, nil)
	expect(t, r, 201, v)
	return f, file, data[party.Party](t, v)
}

func businessCostSourceFixture(t *testing.T, f *fixture, file storage.CommitResult, supplier party.Party, kind, amount string, approve bool) finance.BusinessCostSource {
	t.Helper()
	p := func(v string) *string { return &v }
	title := "业务履行 · " + map[string]string{"purchase": "采购", "claim": "报销", "contract": "合同"}[kind]
	t.Logf("ordinary approved source journey: %s", kind)
	content := spend.Content{Title: title, Category: "business-cost", Purpose: "Synthetic approved basis, not an actual cost", OwnerID: &f.alice, Currency: p("HKD"), RoundingMode: "half_up"}
	if kind == "purchase" {
		content.PricingBasis, content.ExpectedDeliveryDate = "net_plus_tax", p("2025-01-15")
		content.SupplierCandidates = []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}
		content.QuoteVersionIDs = []string{file.Version.ID}
		content.Lines = []spend.Line{{ID: core.NewID(), Description: "Synthetic service", Quantity: p("1"), Unit: "service", UnitPrice: &amount, TaxAmount: p("0"), DiscountAmount: p("0")}}
	} else {
		content.PricingBasis, content.PeriodStart, content.PeriodEnd = "gross_includes_tax", p("2025-01-15"), p("2025-01-15")
		content.Lines = []spend.Line{{ID: core.NewID(), Description: "Synthetic service", DeclaredAmount: &amount, ExpenseDate: p("2025-01-15"), Category: "business-cost", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{file.Version.ID}, Funding: "employee_advance"}}
	}
	route := map[string]string{"contract": "/contracts", "purchase": "/purchases", "claim": "/claims"}[kind]
	var input any = spend.Create{Content: content, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic independent business basis"}
	if kind == "contract" {
		input = contracts.Create{Content: contracts.Content{Title: title, Direction: p("payable"), Type: p("service"), Category: "business-cost", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: p("HKD"), Amount: &amount, StartDate: p("2025-01-15"), EndDate: p("2027-12-31"), DraftFileVersionID: &file.Version.ID,
			PaymentSchedule: &contracts.PaymentSchedule{Mode: "scheduled", Items: []contracts.PaymentPlanItem{{ID: core.NewID(), Title: "Actual delivery review", Amount: &amount, DueDate: p("2025-02-10"), Conditions: "Confirm actual delivery separately"}}}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic effective payable basis"}
	}
	type head struct {
		ID                string `json:"id"`
		Version           int64  `json:"version"`
		CurrentRevisionID string `json:"current_revision_id"`
		ContentHash       string `json:"content_hash"`
	}
	r, v := f.request("POST", "/v1"+route, f.aliceToken, core.NewID(), input, nil)
	expect(t, r, 201, v)
	d := data[head](t, v)
	if approve {
		r, v = f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: kind, BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "exact approved cost basis"}, nil)
		expect(t, r, 201, v)
		request := data[workflow.Request](t, v)
		r, v = f.request("GET", "/v1/requests/"+request.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		if r.Code != 200 {
			t.Logf("%s submission preview: %s", kind, r.Body.String())
		}
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.request("POST", "/v1/requests/"+request.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit approved basis"}, nil)
		expect(t, r, 200, v)
		request = data[workflow.Request](t, v)
		r, v = f.request("POST", "/v1/approval-steps/"+request.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: request.Steps[0].Version, RequestVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, Decision: "approve", Reason: "independent commercial approval only"}, nil)
		expect(t, r, 200, v)
		r, v = f.request("GET", "/v1"+route+"/"+d.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		d = data[head](t, v)
		if kind == "contract" {
			r, v = f.request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, SignedOn: "2025-01-15", SignedFileVersionID: file.Version.ID, Attestation: "Exact synthetic signed evidence", Reason: "record signed basis"}, nil)
			expect(t, r, 201, v)
			signed := data[contracts.Contract](t, v)
			r, v = f.request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: signed.Version, Action: "activate", RevisionID: signed.CurrentRevisionID, ContentHash: signed.ContentHash, SignatureID: signed.Signature.ID, Reason: "activate reached date"}, nil)
			expect(t, r, 200, v)
			d = data[head](t, v)
		}
	}
	return finance.BusinessCostSource{Type: kind, ID: d.ID, Version: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash}
}

func TestBusinessCostRecording(t *testing.T) {
	f, file, supplier := businessCostFixture(t)
	ctx := context.Background()
	p := func(v string) *string { return &v }
	sources := map[string]finance.BusinessCostSource{}
	for _, kind := range []string{"purchase", "claim", "contract"} {
		sources[kind] = businessCostSourceFixture(t, f, file, supplier, kind, "100", true)
	}
	input := func(source finance.BusinessCostSource, amount string) finance.BusinessCostRecord {
		return finance.BusinessCostRecord{Source: source, SourceKey: "manual:" + core.NewID(), Reason: "Human verified actual expense and exact approved source", ActualIncurred: true, SourceReviewed: true,
			Content: finance.Content{Title: "Actual business expense", OccurredOn: p("2025-02-10"), Currency: p("HKD"), Amount: &amount, Category: "business-cost", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}}
	}
	post := func(t *testing.T, in finance.BusinessCostRecord, key string, status int) response {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs/from-business", f.aliceToken, key, in, nil)
		expect(t, r, status, v)
		return v
	}
	count := func(t *testing.T, query string, args ...any) int {
		t.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	t.Run("preview pins all three approved sources without creating costs or reservation", func(t *testing.T) {
		before := count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org)
		for kind, source := range sources {
			r, v := f.with(t).request("GET", "/v1/cost-business-sources/"+kind+"/"+source.ID, f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			b := data[finance.BusinessCostBasis](t, v)
			if b.Source != source || b.Currency != "HKD" || b.ApprovedAmount != "100.000000" || b.ProjectID == nil || *b.ProjectID != *file.File.ProjectID || b.Classification != "project_restricted" || b.AsOf.IsZero() {
				t.Fatal(b)
			}
		}
		if count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org) != before {
			t.Fatal("preview created a cost")
		}
	})
	t.Run("each source records actual expense and immutable reference in one transaction", func(t *testing.T) {
		for kind, source := range sources {
			in := input(source, "25")
			c := data[finance.Cost](t, post(t, in, core.NewID(), 201))
			if c.Version != 3 || c.State != "confirmed" || c.EntryKind != "expense" || *c.Amount != "25.000000" || *c.OccurredOn != "2025-02-10" || c.ProjectID == nil || *c.ProjectID != *file.File.ProjectID || c.Classification != "project_restricted" || len(c.Links) != 1 || c.Links[0].SourceID != source.ID || c.Links[0].SourceRevisionID != source.RevisionID || c.Links[0].SourceType != kind {
				t.Fatal(c)
			}
			if count(t, "SELECT count(*) FROM cost_events WHERE cost_id=$1", c.ID) != 3 || count(t, "SELECT count(*) FROM cost_revisions WHERE cost_id=$1", c.ID) != 1 || count(t, "SELECT count(*) FROM audit_events WHERE resource_type='cost' AND resource_id=$1", c.ID) != 4 || count(t, "SELECT count(*) FROM outbox_events WHERE aggregate_type='cost' AND aggregate_id=$1", c.ID) != 4 || count(t, "SELECT count(*) FROM file_version_references WHERE owner_type='cost_revision' AND owner_id=$1", c.CurrentRevisionID) != 1 {
				t.Fatal("ordinary journals/references/audit missing")
			}
			if c.PaidAmount != nil && *c.PaidAmount != "0.000000" {
				t.Fatal("expense created payment", c)
			}
		}
		r, v := f.with(t).request("GET", "/v1/reports/expenses?kind=incurred&category=business-cost&from=2025-02-01&to=2025-03-01", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		report := data[finance.Report](t, v)
		if len(report.Totals) != 1 || report.Totals[0].NetAmount != "75.000000" || report.UnconfirmedCount != 0 {
			t.Fatal(report)
		}
	})
	t.Run("late source ceiling failure rolls back draft confirmation evidence audit and keys", func(t *testing.T) {
		in := input(sources["claim"], "76")
		before := count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org)
		key := core.NewID()
		v := post(t, in, key, 409)
		if v.Error.Code != "COST_SOURCE_AMOUNT_EXCEEDED" || count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org) != before || count(t, "SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND idempotency_key=$2", f.org, key) != 0 {
			t.Fatal("partial write escaped failed composed command", v)
		}
	})
	t.Run("current execution version changes invalidate the selected basis", func(t *testing.T) {
		source := businessCostSourceFixture(t, f, file, supplier, "purchase", "100", true)
		r, v := f.with(t).request("GET", "/v1/purchases/"+source.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		purchase := data[spend.Record](t, v)
		r, v = f.with(t).request("POST", "/v1/purchases/"+source.ID+"/orders", f.aliceToken, core.NewID(), spend.OrderInput{ExecutionBase: spend.ExecutionBase{ExpectedVersion: source.Version, RevisionID: source.RevisionID, ContentHash: source.ContentHash, Reason: "current ordinary order changes execution basis"}, Supplier: spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ExternalRef: "synthetic-order", OrderedOn: "2025-02-01", ExpectedDeliveryOn: "2025-02-10", EvidenceVersionIDs: []string{file.Version.ID}, Lines: []spend.OrderLineInput{{PurchaseLineID: purchase.Lines[0].ID, Quantity: "1", UnitPrice: "100", TaxAmount: "0", DiscountAmount: "0"}}}, nil)
		expect(t, r, 201, v)
		v = post(t, input(source, "1"), core.NewID(), 409)
		if v.Error.Code != "COST_SOURCE_CHANGED" {
			t.Fatal(v)
		}
	})
	t.Run("concurrent independent expenses share the ordinary source ceiling", func(t *testing.T) {
		source := businessCostSourceFixture(t, f, file, supplier, "purchase", "100", true)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/costs/from-business", f.aliceToken, core.NewID(), input(source, "60"), nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 201 {
				success++
			} else if code != 409 {
				t.Fatal(code)
			}
		}
		if success != 1 || count(t, "SELECT count(*) FROM cost_links WHERE org_id=$1 AND source_id=$2", f.org, source.ID) != 1 {
			t.Fatal("concurrent ceiling exceeded")
		}
	})
	t.Run("invalid source scope facts and acknowledgements cannot create records", func(t *testing.T) {
		before := count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org)
		for _, change := range []func(*finance.BusinessCostRecord){
			func(in *finance.BusinessCostRecord) { in.Source.Version++ },
			func(in *finance.BusinessCostRecord) { in.Source.ContentHash = strings.Repeat("0", 64) },
			func(in *finance.BusinessCostRecord) { in.Currency = p("USD") },
		} {
			in := input(sources["contract"], "1")
			change(&in)
			post(t, in, core.NewID(), 409)
		}
		for _, change := range []func(*finance.BusinessCostRecord){
			func(in *finance.BusinessCostRecord) { in.ActualIncurred = false },
			func(in *finance.BusinessCostRecord) { in.SourceReviewed = false },
			func(in *finance.BusinessCostRecord) { in.EvidenceVersionIDs = []string{} },
			func(in *finance.BusinessCostRecord) { in.Allocations[0].Amount = "2" },
			func(in *finance.BusinessCostRecord) { in.OccurredOn = p("2099-01-01") },
		} {
			in := input(sources["contract"], "1")
			change(&in)
			post(t, in, core.NewID(), 422)
		}
		unapproved := businessCostSourceFixture(t, f, file, supplier, "claim", "10", false)
		post(t, input(unapproved, "1"), core.NewID(), 409)
		raw, _ := json.Marshal(input(sources["contract"], "1"))
		var replaced map[string]any
		json.Unmarshal(raw, &replaced)
		replaced["project_id"], replaced["classification"] = core.NewID(), "internal"
		r, v := f.with(t).request("POST", "/v1/costs/from-business", f.aliceToken, core.NewID(), replaced, nil)
		expect(t, r, 422, v)
		if count(t, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org) != before {
			t.Fatal("invalid intent left costs")
		}
	})
	t.Run("authority and query isolation apply before discovery and cached replay", func(t *testing.T) {
		source := sources["contract"]
		for _, route := range []string{"/v1/cost-business-sources/contract/" + core.NewID(), "/v1/cost-business-sources/payment/" + source.ID} {
			r, v := f.with(t).request("GET", route, f.aliceToken, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v := f.with(t).request("GET", "/v1/cost-business-sources/contract/"+source.ID+"?project_id="+core.NewID(), f.aliceToken, "", nil, nil)
		expect(t, r, 422, v)
		r, v = f.with(t).request("GET", "/v1/cost-business-sources/contract/"+source.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		in, key := input(source, "1"), core.NewID()
		first := data[finance.Cost](t, post(t, in, key, 201))
		second := data[finance.Cost](t, post(t, in, key, 201))
		if first.ID != second.ID || second.Version != 3 || count(t, "SELECT count(*) FROM cost_events WHERE cost_id=$1", first.ID) != 3 {
			t.Fatal("replay duplicated business cost")
		}
		changed := in
		changed.Notes = "changed body"
		post(t, changed, key, 409)
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		actual := actor.MFAVerifiedAt
		if _, e = f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID); e != nil {
			t.Fatal(e)
		}
		defer f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		post(t, in, key, 403)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		raw, _ := json.Marshal(in)
		service := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		command := core.Command{Actor: actor, Key: key, RequestID: core.NewID(), Route: "POST /v1/costs/from-business", Body: raw}
		if _, e = service.RecordBusinessCost(ctx, command, in); core.PublicError(e).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("forged in-memory MFA bypass", e)
		}
		command.Actor.RunID = core.NewID()
		if _, e = service.RecordBusinessCost(ctx, command, in); core.PublicError(e).Status != 403 {
			t.Fatal("agent entered human business command", e)
		}
	})
}

func TestBusinessCostBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_BUSINESS_COST") != "1" {
		t.Skip("explicit owned browser opt-in only")
	}
	f, file, supplier := businessCostFixture(t)
	sources := []map[string]any{}
	for _, kind := range []string{"contract", "purchase", "claim"} {
		source := businessCostSourceFixture(t, f, file, supplier, kind, "100", true)
		sources = append(sources, map[string]any{"kind": kind, "id": source.ID})
	}
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "human-business-cost", "business_cost_fixture": true, "project_id": file.File.ProjectID,
		"file_id": file.File.ID, "file_version_id": file.Version.ID, "supplier_id": supplier.ID, "supplier_name": supplier.Name, "business_sources": sources})
}
