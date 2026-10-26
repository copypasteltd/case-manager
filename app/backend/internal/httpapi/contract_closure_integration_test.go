package httpapi

import (
	"context"
	"encoding/json"
	"slices"
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

func TestContractCompletionAndTermination(t *testing.T) {
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
	for _, key := range []string{"contract_manager", "contract_operator", "contract_reconciler", "party_manager", "request_manager", "cost_recorder", "cost_confirmer", "cost_reverser", "payment_recorder", "payment_reverser"} {
		bind(f.alice, key)
	}
	for _, key := range []string{"contract_reader", "party_manager", "approval_reviewer"} {
		bind(f.bob, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "contract-close", Name: "Synthetic legal closure review", Kind: "contract", Category: "closure", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.bob}}, Reason: "isolated closure policy"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic closure counterparty", Kind: "organization", Relationships: []string{"supplier", "customer"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated closure source"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if e := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); e != nil {
		t.Fatal(e)
	}
	getContract := func(id string) contracts.Contract {
		r, v := f.request("GET", "/v1/contracts/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[contracts.Contract](t, v)
	}
	makeContract := func(mode string, parent *string, active bool) contracts.Contract {
		in := contracts.Create{Content: contracts.Content{Title: "Synthetic closure " + mode, Direction: ptr("payable"), Type: ptr("service"), Category: "closure", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("100"), StartDate: &today, EndDate: ptr("2030-01-01"), DraftFileVersionID: &file.Version.ID}, ProjectID: file.File.ProjectID, ParentContractID: parent, Classification: "project_restricted", Reason: "fixed synthetic closure contract"}
		if mode == "scheduled" {
			in.PaymentSchedule = &contracts.PaymentSchedule{Mode: mode, Items: []contracts.PaymentPlanItem{{ID: core.NewID(), Title: "Delivery and settlement", Amount: ptr("100"), DueDate: &today, Conditions: "Verify complete delivery"}}}
		}
		if mode == "no_payment" {
			in.Direction = ptr("none")
			in.Amount = ptr("0")
			in.PaymentSchedule = &contracts.PaymentSchedule{Mode: mode, Reason: "Verified agreement has no payment obligations", Items: []contracts.PaymentPlanItem{}}
		}
		r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		c := data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: c.ID, ExpectedBusinessVersion: c.Version, BusinessRevisionID: c.CurrentRevisionID, BusinessContentHash: c.ContentHash, Reason: "approve exact closure basis"}, nil)
		expect(t, r, 201, v)
		request := data[workflow.Request](t, v)
		r, v = f.request("GET", "/v1/requests/"+request.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.request("POST", "/v1/requests/"+request.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "human confirms exact revision"}, nil)
		expect(t, r, 200, v)
		request = data[workflow.Request](t, v)
		r, v = f.request("POST", "/v1/approval-steps/"+request.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: request.Steps[0].Version, RequestVersion: request.Version, RevisionID: request.CurrentRevisionID, ContentHash: request.ContentHash, Decision: "approve", Reason: "independent fixed approval"}, nil)
		expect(t, r, 200, v)
		c = getContract(c.ID)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/signature", f.aliceToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignedOn: today, SignedFileVersionID: file.Version.ID, Attestation: "Actual synthetic proof checked against approved terms", Reason: "human signature evidence"}, nil)
		expect(t, r, 201, v)
		c = data[contracts.Contract](t, v)
		if active {
			r, v = f.request("POST", "/v1/contracts/"+c.ID+"/transition", f.aliceToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: c.Version, Action: "activate", RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignatureID: c.Signature.ID, Reason: "explicit effective contract"}, nil)
			expect(t, r, 200, v)
			c = data[contracts.Contract](t, v)
		}
		return c
	}
	preview := func(c contracts.Contract) finance.ContractClosurePreview {
		r, v := f.request("GET", "/v1/contracts/"+c.ID+"/closure-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.ContractClosurePreview](t, v)
	}
	input := func(c contracts.Contract, action string) finance.ContractClosureInput {
		p := preview(c)
		if p.Basis == nil || p.Fingerprint == nil {
			t.Fatal("closure basis unavailable", p)
		}
		return finance.ContractClosureInput{ExpectedVersion: p.ContractVersion, RevisionID: p.Basis.RevisionID, ExpectedFinanceVersion: p.Basis.FinanceVersion, ExpectedFingerprint: *p.Fingerprint, Action: action, OccurredOn: today, TermsVerified: true, Attestation: "Human checked exact terms and the listed synthetic evidence", EvidenceVersionIDs: []string{file.Version.ID}, Reason: "human closure registration"}
	}
	closeContract := func(t *testing.T, c contracts.Contract, in finance.ContractClosureInput, code int) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/contracts/"+c.ID+"/closure", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, code, v)
	}
	record := func(c contracts.Contract) finance.ContractClosure {
		r, v := f.request("GET", "/v1/contracts/"+c.ID+"/closure", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.ContractClosure](t, v)
	}
	cashBase := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "exact financial proof"}
	}
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "exact expense proof"}
	}
	newPayment := func(amount string, original *finance.Payment) finance.Payment {
		content := finance.PaymentContent{Title: "Synthetic legal closure cash", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-01T02:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "closure", MethodLabel: "Synthetic bank evidence", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, EvidenceVersionIDs: []string{file.Version.ID}}
		if original != nil {
			content.Direction = ptr("refund")
			content.RefundOfID = &original.ID
		}
		r, v := f.request("POST", "/v1/payments", f.aliceToken, core.NewID(), finance.PaymentCreate{PaymentContent: content, ProjectID: file.File.ProjectID, Classification: "project_restricted", SourceKey: "closure-cash:" + core.NewID(), Reason: "isolated actual cash"}, nil)
		expect(t, r, 201, v)
		p := data[finance.Payment](t, v)
		in := finance.PaymentRecordInput{Base: cashBase(p), RefundAllocations: []finance.RefundAllocation{}}
		if original != nil {
			in.ExpectedSourceVersion = &original.Version
		}
		r, v = f.request("POST", "/v1/payments/"+p.ID+"/record", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, v)
		return data[finance.Payment](t, v)
	}
	financeView := func(c contracts.Contract) finance.ContractFinance {
		r, v := f.request("GET", "/v1/contracts/"+c.ID+"/finance", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.ContractFinance](t, v)
	}
	linkCash := func(c contracts.Contract, p finance.Payment, amount string) {
		view := financeView(c)
		r, v := f.request("POST", "/v1/contracts/"+c.ID+"/finance/entries", f.aliceToken, core.NewID(), finance.ContractFinanceLink{ContractFinanceBase: finance.ContractFinanceBase{ExpectedVersion: view.Version, ContractVersion: view.ContractVersion, Reason: "exact actual cash attribution"}, MilestoneID: view.Nodes[0].ID, Kind: "cash", SourceID: p.ID, SourceVersion: p.Version, SourceRevisionID: p.CurrentRevisionID, SourceContentHash: p.ContentHash, Amount: amount}, nil)
		expect(t, r, 200, v)
	}
	review := func(c contracts.Contract, conditions bool) {
		view := financeView(c)
		r, v := f.request("POST", "/v1/contracts/"+c.ID+"/finance/milestones/"+view.Nodes[0].ID+"/review", f.aliceToken, core.NewID(), finance.ContractFinanceReview{ContractFinanceBase: finance.ContractFinanceBase{ExpectedVersion: view.Version, ContractVersion: view.ContractVersion, Reason: "human verifies exact delivery and financial basis"}, ExpectedFingerprint: *view.Nodes[0].Fingerprint, ConditionsSatisfied: conditions, AcknowledgeDifference: true, AcknowledgeUnattributedRefunds: true, EvidenceVersionIDs: []string{file.Version.ID}}, nil)
		expect(t, r, 200, v)
	}
	newCost := func(c contracts.Contract, amount string) finance.Cost {
		r, v := f.request("POST", "/v1/costs", f.aliceToken, core.NewID(), finance.Create{Content: finance.Content{Title: "Synthetic closure expense", OccurredOn: ptr("2025-01-15"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "closure", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", SourceKey: "closure-cost:" + core.NewID(), Reason: "actual expense"}, nil)
		expect(t, r, 201, v)
		d := data[finance.Cost](t, v)
		r, v = f.request("POST", "/v1/costs/"+d.ID+"/confirm", f.aliceToken, core.NewID(), costBase(d), nil)
		expect(t, r, 200, v)
		d = data[finance.Cost](t, v)
		r, v = f.request("POST", "/v1/costs/"+d.ID+"/links", f.aliceToken, core.NewID(), finance.LinkInput{Base: costBase(d), SourceType: "contract", SourceID: c.ID, SourceRevisionID: *c.ActiveRevisionID, Relation: "contractual_basis"}, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	scheduled := makeContract("scheduled", nil, true)
	var closer string
	t.Run("independent closer starts unbound and limited readers see unknown basis", func(t *testing.T) {
		p := preview(scheduled)
		if p.CanComplete || p.CanTerminate {
			t.Fatal(p)
		}
		r, v := f.with(t).request("GET", "/v1/contracts/"+scheduled.ID+"/closure-preview", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		limited := data[finance.ContractClosurePreview](t, v)
		if limited.Basis != nil || limited.Fingerprint != nil || limited.CanTerminate || limited.Visibility != "restricted" {
			t.Fatal(limited)
		}
		r, v = f.with(t).request("POST", "/v1/contracts/"+scheduled.ID+"/closure", admin, core.NewID(), input(scheduled, "terminated"), nil)
		expect(t, r, 404, v)
		closer = bind(f.alice, "contract_closer")
		if !preview(scheduled).CanTerminate {
			t.Fatal("explicit closer unavailable")
		}
	})
	t.Run("unpaid nodes and unchecked conditions prevent completion", func(t *testing.T) {
		p := preview(scheduled)
		if !slices.Contains(p.Blockers, "node_amount_difference") || !slices.Contains(p.Blockers, "conditions_unconfirmed") || p.CanComplete {
			t.Fatal(p)
		}
		closeContract(t, scheduled, input(scheduled, "completed"), 409)
	})
	t.Run("termination requires explicit unresolved acknowledgement and disposition", func(t *testing.T) {
		in := input(scheduled, "terminated")
		closeContract(t, scheduled, in, 422)
		in.AcknowledgeUnsettled = true
		closeContract(t, scheduled, in, 422)
		in.UnsettledDisposition = "Delivery remains disputed; responsible owner will reconcile cash separately."
		closeContract(t, scheduled, in, 201)
		z := record(scheduled)
		if z.BasisState != "current" || z.Action != "terminated" || !z.AcknowledgeUnsettled || len(z.Attachments) != 1 {
			t.Fatal(z)
		}
		assertProjectCloseoutItem(t, f, *file.File.ProjectID, "contracts", scheduled.ID, true)
	})
	t.Run("late actual financial attribution changes basis without reopening legal stage", func(t *testing.T) {
		p := newPayment("100", nil)
		linkCash(scheduled, p, "100")
		z := record(scheduled)
		c := getContract(scheduled.ID)
		if z.BasisState != "changed" || c.Lifecycle != "terminated" || c.Version != z.ContractVersion {
			t.Fatal(z, c)
		}
		_ = newCost(c, "10")
		if record(scheduled).BasisState != "changed" {
			t.Fatal("late actual cost omitted")
		}
	})
	t.Run("scheduled completion requires current review and verified conditions", func(t *testing.T) {
		c := makeContract("scheduled", nil, true)
		p := newPayment("100", nil)
		linkCash(c, p, "100")
		closeContract(t, c, input(c, "completed"), 409)
		review(c, false)
		closeContract(t, c, input(c, "completed"), 409)
		review(c, true)
		if !preview(c).CanComplete {
			t.Fatal(preview(c))
		}
		closeContract(t, c, input(c, "completed"), 201)
		z := record(c)
		if z.BasisState != "current" || !z.Basis.Nodes[0].ReviewCurrent || getContract(c.ID).Lifecycle != "completed" {
			t.Fatal(z)
		}
	})
	t.Run("new refund invalidates an old closure basis without changing financial head", func(t *testing.T) {
		c := makeContract("scheduled", nil, true)
		p := newPayment("100", nil)
		linkCash(c, p, "100")
		review(c, true)
		in := input(c, "completed")
		_ = newPayment("10", &p)
		after := preview(c)
		if after.Basis.FinanceVersion != in.ExpectedFinanceVersion || *after.Fingerprint == in.ExpectedFingerprint || !slices.Contains(after.Blockers, "unattributed_refunds") {
			t.Fatal(after)
		}
		closeContract(t, c, in, 409)
		closeContract(t, c, input(c, "completed"), 409)
	})
	t.Run("linked expense must be settled even without node cost attribution", func(t *testing.T) {
		c := makeContract("scheduled", nil, true)
		p := newPayment("100", nil)
		linkCash(c, p, "100")
		review(c, true)
		d := newCost(c, "20")
		old := input(c, "terminated")
		if !slices.Contains(preview(c).Blockers, "cost_unsettled") {
			t.Fatal(preview(c))
		}
		closeContract(t, c, input(c, "completed"), 409)
		payment := newPayment("20", nil)
		r, v := f.with(t).request("POST", "/v1/payments/"+payment.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: cashBase(payment), CostID: d.ID, CostVersion: d.Version, CostRevisionID: d.CurrentRevisionID, CostContentHash: d.ContentHash, Amount: "20"}, nil)
		expect(t, r, 200, v)
		closeContract(t, c, old, 409)
		if !preview(c).CanComplete {
			t.Fatal(preview(c))
		}
		closeContract(t, c, input(c, "completed"), 201)
		z := record(c)
		if len(z.Basis.Costs) != 1 || z.Basis.Costs[0].PaidAmount != "20.000000" {
			t.Fatal(z)
		}
		assertProjectCloseoutItem(t, f, *file.File.ProjectID, "contracts", c.ID, false)
	})
	t.Run("explicit no payment agreement can complete with no invented financial facts", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		p := preview(c)
		if !p.CanComplete || len(p.Basis.Nodes) != 0 || p.Basis.ScheduleMode == nil || *p.Basis.ScheduleMode != "no_payment" {
			t.Fatal(p)
		}
		closeContract(t, c, input(c, "completed"), 201)
		r, v := f.with(t).request("GET", "/v1/contracts/"+c.ID+"/closure", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.ContractClosure](t, v).Basis.ScheduleMode == nil {
			t.Fatal("explicit non-financial contract became unknown")
		}
	})
	t.Run("unknown payment plan stays unknown and may only terminate with disposition", func(t *testing.T) {
		c := makeContract("unknown", nil, true)
		p := preview(c)
		if p.Basis.ScheduleMode != nil || !slices.Contains(p.Blockers, "plan_unknown") {
			t.Fatal(p)
		}
		closeContract(t, c, input(c, "completed"), 409)
		in := input(c, "terminated")
		in.AcknowledgeUnsettled = true
		in.UnsettledDisposition = "Resolve the unknown plan from retained original evidence."
		closeContract(t, c, in, 201)
	})
	t.Run("effective supplements block parent until separately terminated", func(t *testing.T) {
		parent := makeContract("no_payment", nil, true)
		child := makeContract("no_payment", &parent.ID, true)
		p := preview(parent)
		if p.CanTerminate || !slices.Contains(p.Blockers, "supplement_effective") {
			t.Fatal(p)
		}
		in := input(parent, "terminated")
		in.AcknowledgeUnsettled = true
		in.UnsettledDisposition = "This cannot bypass the child."
		closeContract(t, parent, in, 409)
		closeContract(t, child, input(child, "terminated"), 201)
		closeContract(t, parent, input(parent, "completed"), 201)
	})
	t.Run("parent termination and child activation serialize their legal facts", func(t *testing.T) {
		parent := makeContract("no_payment", nil, true)
		child := makeContract("no_payment", &parent.ID, false)
		in := input(parent, "terminated")
		in.AcknowledgeUnsettled = true
		in.UnsettledDisposition = "Pending supplement will be handled separately."
		counts := map[int]int{}
		var lock sync.Mutex
		var wg sync.WaitGroup
		for _, which := range []string{"close", "activate"} {
			wg.Add(1)
			go func(which string) {
				defer wg.Done()
				path := "/v1/contracts/" + parent.ID + "/closure"
				var body any = in
				if which == "activate" {
					path = "/v1/contracts/" + child.ID + "/transition"
					body = contracts.ActivationInput{ExpectedVersion: child.Version, Action: "activate", RevisionID: child.CurrentRevisionID, ContentHash: child.ContentHash, SignatureID: child.Signature.ID, Reason: "concurrent exact child activation"}
				}
				r, _ := f.with(t).request("POST", path, f.aliceToken, core.NewID(), body, nil)
				lock.Lock()
				counts[r.Code]++
				lock.Unlock()
			}(which)
		}
		wg.Wait()
		if counts[409] != 1 || counts[200]+counts[201] != 1 {
			t.Fatal(counts)
		}
		if getContract(parent.ID).Lifecycle != "effective" && getContract(child.ID).Lifecycle == "effective" {
			t.Fatal("effective child survived under closed parent")
		}
	})
	t.Run("actual date manual verification and fixed evidence are mandatory", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		in := input(c, "completed")
		in.TermsVerified = false
		closeContract(t, c, in, 422)
		in = input(c, "completed")
		in.EvidenceVersionIDs = []string{}
		closeContract(t, c, in, 422)
		in = input(c, "completed")
		in.OccurredOn = "2020-01-01"
		closeContract(t, c, in, 422)
		in = input(c, "completed")
		in.OccurredOn = "2099-01-01"
		closeContract(t, c, in, 422)
	})
	t.Run("concurrent devices append exactly one immutable closure", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		in := input(c, "completed")
		results := make(chan int, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/contracts/"+c.ID+"/closure", f.aliceToken, core.NewID(), in, nil)
				results <- r.Code
			}()
		}
		wg.Wait()
		close(results)
		count := map[int]int{}
		for code := range results {
			count[code]++
		}
		if count[201] != 1 || count[409] != 1 {
			t.Fatal(count)
		}
	})
	t.Run("audit failure rolls back legal head proof record and receipt", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		in := input(c, "completed")
		key := core.NewID()
		path := "/v1/contracts/" + c.ID + "/closure"
		exec(`CREATE FUNCTION fail_closure_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='contract.completed' THEN RAISE EXCEPTION 'isolated closure audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_closure_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_closure_audit()`)
		defer exec("DROP TRIGGER IF EXISTS fail_closure_audit ON audit_events;DROP FUNCTION IF EXISTS fail_closure_audit()")
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 500, v)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM contract_closures WHERE contract_id=$1)+(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$2)", c.ID, key).Scan(&count); e != nil || count != 0 || getContract(c.ID).Lifecycle != "effective" {
			t.Fatal(count, e)
		}
		exec("DROP TRIGGER fail_closure_audit ON audit_events;DROP FUNCTION fail_closure_audit()")
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
	})
	t.Run("database rejects detached terminal heads and immutable closure rewrites", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		if _, e := f.owner.Exec(ctx, "UPDATE contracts SET lifecycle='completed',version=version+1 WHERE id=$1", c.ID); e == nil {
			t.Fatal("closed without human record")
		}
		closeContract(t, c, input(c, "completed"), 201)
		z := record(c)
		for _, q := range []string{"UPDATE contract_closures SET reason='forged' WHERE contract_id=$1", "DELETE FROM contract_closures WHERE contract_id=$1", "UPDATE contracts SET lifecycle='effective',version=version+1 WHERE id=$1"} {
			if _, e := f.owner.Exec(ctx, q, c.ID); e == nil {
				t.Fatal("rewrote legal facts", q)
			}
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM contract_closure_files WHERE closure_id=$1", z.ID); e == nil {
			t.Fatal("deleted fixed proof")
		}
		var refs int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='contract_closure' AND owner_id=$1 AND version_id=$2", z.ID, file.Version.ID).Scan(&refs); e != nil || refs != 1 {
			t.Fatal(refs, e)
		}
	})
	t.Run("caller supplied MFA cannot replace the actual current database session", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		in := input(c, "completed")
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
		now := time.Now()
		actor.MFAVerifiedAt = &now
		raw, _ := json.Marshal(in)
		service := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		_, e = service.CloseContract(ctx, core.Command{Actor: actor, RequestID: core.NewID(), Key: core.NewID(), Route: "POST /v1/contracts/" + c.ID + "/closure", Body: raw}, c.ID, in)
		if core.PublicError(e).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(e)
		}
	})
	t.Run("database validates exact financial basis actual MFA and complete evidence set", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		p := preview(c)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"forged fingerprint", "forged basis", "forged MFA", "missing fixed evidence"} {
			tx, err := f.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			basis := *p.Basis
			fingerprint := *p.Fingerprint
			if mode == "forged fingerprint" {
				fingerprint = "00000000000000000000000000000000"
			}
			if mode == "forged basis" {
				basis.FinanceVersion++
			}
			raw, _ := json.Marshal(basis)
			_, err = tx.Exec(ctx, `INSERT INTO contract_closures(id,org_id,contract_id,revision_id,contract_version,finance_version,action,occurred_on,basis,fingerprint,
                terms_verified,attestation,acknowledge_unsettled,unsettled_disposition,evidence_count,actor_id,session_id,mfa_verified_at,reason)
                SELECT $1,$2,$3,$4,$5,$6,'completed',$7,$8,$9,true,'Synthetic exact proof',false,'',1,$10,s.id,
                    CASE WHEN $12 THEN s.mfa_verified_at-interval '1 hour' ELSE s.mfa_verified_at END,'Attempt synthetic forgery'
                FROM sessions s WHERE s.org_id=$2 AND s.id=$11`, core.NewID(), f.org, c.ID, *c.ActiveRevisionID, c.Version+1, p.Basis.FinanceVersion, today, raw, fingerprint, f.alice, actor.SessionID, mode == "forged MFA")
			if err == nil {
				_, err = tx.Exec(ctx, "UPDATE contracts SET lifecycle='completed',version=version+1 WHERE id=$1", c.ID)
			}
			if err == nil {
				err = tx.Commit(ctx)
			}
			_ = tx.Rollback(ctx)
			if err == nil {
				t.Fatal("database accepted", mode)
			}
		}
		if getContract(c.ID).Lifecycle != "effective" {
			t.Fatal("forged closure changed actual head")
		}
	})
	t.Run("historical basis and cached receipts reauthorize their actual scopes", func(t *testing.T) {
		c := makeContract("no_payment", nil, true)
		in := input(c, "completed")
		key := core.NewID()
		path := "/v1/contracts/" + c.ID + "/closure"
		r, v := f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		first := data[finance.ContractClosureReceipt](t, v)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[finance.ContractClosureReceipt](t, v).ClosureID != first.ClosureID {
			t.Fatal("duplicate closure on replay")
		}
		r, v = f.with(t).request("GET", "/v1/contracts/"+scheduled.ID+"/closure", f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", closer)
		defer exec("UPDATE role_bindings SET valid_until=NULL WHERE id=$1", closer)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 404, v)
	})
}
