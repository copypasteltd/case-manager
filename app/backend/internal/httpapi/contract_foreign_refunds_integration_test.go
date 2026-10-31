package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContractForeignRefundAttribution(t *testing.T) {
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
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "contract-refunds133", Name: "Synthetic financial contract review", Kind: "contract", Category: "reconciliation", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.bob}}, Reason: "isolated reconciliation policy"}, nil)
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
		in := contracts.Create{Content: contracts.Content{Title: "Synthetic " + direction + " contract", Direction: ptr(direction), Type: ptr("service"), Category: "reconciliation", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("780"), StartDate: &today, EndDate: ptr("2030-01-01"), DraftFileVersionID: &file.Version.ID, PaymentSchedule: &contracts.PaymentSchedule{Mode: "scheduled", Items: []contracts.PaymentPlanItem{{ID: core.NewID(), Title: "Synthetic final delivery", Amount: ptr("780"), DueDate: &today, Conditions: "Confirm delivery"}}}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated contract with fixed milestones"}
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

	var headers map[string]string
	call := func(method, path string, body any, status int) response {
		t.Helper()
		r, v := f.request(method, path, f.aliceToken, core.NewID(), body, headers)
		expect(t, r, status, v)
		return v
	}
	summary := func(c contracts.Contract) finance.ContractFinance {
		return data[finance.ContractFinance](t, call("GET", "/v1/contracts/"+c.ID+"/finance", nil, 200))
	}
	c := newContract("payable")
	bind(f.alice, "contract_reconciler")
	view := summary(c)
	n := view.Nodes[0]
	base := func() finance.ContractFinanceBase {
		return finance.ContractFinanceBase{ExpectedVersion: summary(c).Version, ContractVersion: c.Version, Reason: "review synthetic principal attribution"}
	}
	draft := func(direction, currency, amount string, original *finance.Payment) finance.Payment {
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic contract foreign refund", Direction: ptr(direction), PaidAt: ptr("2025-02-02T12:00:00+08:00"), Currency: ptr(currency), Amount: ptr(amount), Category: "contract-refunds133", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, MethodLabel: "synthetic bank fact", EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "contract-refund133:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated contract refund"}
		if original != nil {
			in.RefundOfID = &original.ID
			in.RefundFX = &finance.PaymentRefundFX{OriginalCurrency: "HKD", OriginalAmount: "780", Rate: "0.1282051282", Source: "synthetic immutable refund statement"}
		}
		return data[finance.Payment](t, call("POST", "/v1/payments", in, 201))
	}
	paymentBase := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "review original cash and principal"}
	}
	record := func(p finance.Payment, original *finance.Payment) finance.Payment {
		in := finance.PaymentRecordInput{Base: paymentBase(p), RefundAllocations: []finance.RefundAllocation{}}
		if original != nil {
			in.ExpectedSourceVersion = &original.Version
			in.RefundFXAcknowledged = true
		}
		return data[finance.Payment](t, call("POST", "/v1/payments/"+p.ID+"/record", in, 200))
	}
	p := record(draft("outgoing", "HKD", "780", nil), nil)
	linkInput := func(kind, amount string, p finance.Payment, root *string) finance.ContractFinanceLink {
		return finance.ContractFinanceLink{ContractFinanceBase: base(), MilestoneID: n.ID, Kind: kind, SourceID: p.ID, SourceVersion: p.Version, SourceRevisionID: p.CurrentRevisionID, SourceContentHash: p.ContentHash, SourceEventID: root, Amount: amount}
	}
	root := data[finance.ContractFinanceReceipt](t, call("POST", "/v1/contracts/"+c.ID+"/finance/entries", linkInput("cash", "780", p, nil), 200)).EventID
	var unchanged bool
	if e := f.owner.QueryRow(ctx, "SELECT contract_finance_snapshot($1,$2)=contract_finance_snapshot_0099_base($1,$2)", f.org, n.ID).Scan(&unchanged); e != nil || !unchanged {
		t.Fatal("same-currency snapshot changed", e)
	}
	refund := record(draft("refund", "USD", "100", &p), &p)
	var links []string
	t.Run("fresh acknowledgement and principal units cannot be bypassed", func(t *testing.T) {
		before := summary(c).Version
		in := linkInput("refund", "195", refund, &root)
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries", in, 422)
		in.RefundFXAcknowledged = true
		in.Amount = "780.01"
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries", in, 409)
		if summary(c).Version != before {
			t.Fatal("rejected command changed immutable history")
		}
		in = linkInput("cash", "1", p, nil)
		in.RefundFXAcknowledged = true
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries", in, 422)
	})
	loginChannel := func(channel string) {
		// This is an isolated arithmetic fixture, not a TOTP replay test.
		// Reset only its synthetic credential's replay cursor, then perform a
		// real challenge/verification and obtain the actual channel session.
		var encrypted []byte
		if e := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.alice).Scan(&encrypted); e != nil {
			t.Fatal(e)
		}
		secret, e := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.alice)
		if e != nil {
			t.Fatal("synthetic MFA fixture open failed")
		}
		exec("UPDATE mfa_credentials SET last_step=-1 WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		challenge := f.login("alice", channel)
		h := map[string]string{"Origin": "http://localhost:5178"}
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, h)
		expect(t, r, 200, v)
		result := data[identity.LoginResult](t, v)
		headers = nil
		f.aliceToken = result.AccessToken
		if channel == "web" {
			headers = map[string]string{"Origin": "http://localhost:5178", "X-CSRF-Token": result.CSRFToken}
			for _, cookie := range r.Result().Cookies() {
				if cookie.Name == "cp_session" {
					headers["Cookie"] = "cp_session=" + cookie.Value
				}
			}
		}
	}
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		t.Run(channel, func(t *testing.T) {
			loginChannel(channel)
			in := linkInput("refund", "195", refund, &root)
			in.RefundFXAcknowledged = true
			key := core.NewID()
			r, v := f.request("POST", "/v1/contracts/"+c.ID+"/finance/entries", f.aliceToken, key, in, headers)
			expect(t, r, 200, v)
			receipt := data[finance.ContractFinanceReceipt](t, v)
			links = append(links, receipt.EventID)
			r, v = f.request("POST", "/v1/contracts/"+c.ID+"/finance/entries", f.aliceToken, key, in, headers)
			expect(t, r, 200, v)
			if data[finance.ContractFinanceReceipt](t, v) != receipt {
				t.Fatal("exact replay changed attribution")
			}
		})
	}
	events := func() []finance.ContractFinanceEvent {
		return data[core.Page[finance.ContractFinanceEvent]](t, call("GET", "/v1/contracts/"+c.ID+"/finance/events?limit=100", nil, 200)).Items
	}
	t.Run("whole received cash and contract principal stay distinct", func(t *testing.T) {
		view = summary(c)
		if *view.Nodes[0].CashAmount != "0.000000" || *view.Nodes[0].HasUnattributedRefunds {
			t.Fatal(view)
		}
		count := 0
		for _, e := range events() {
			if e.Kind == "refund" {
				count++
				if e.RefundCash == nil || e.RefundCash.Currency != "USD" || e.RefundCash.Amount != "100.000000" || e.RefundCash.RefundFX.OriginalAmount != "780.000000" || !e.RefundFXAcknowledged || *e.Amount != "195.000000" {
					t.Fatal(e)
				}
			}
		}
		if count != 4 {
			t.Fatal(count)
		}
		in := linkInput("refund", "0.01", refund, &root)
		in.RefundFXAcknowledged = true
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries", in, 409)
		cash := data[finance.CashReport](t, call("GET", "/v1/reports/cash?from=2025-02-01&to=2025-03-01&category=contract-refunds133", nil, 200))
		found := false
		for _, total := range cash.Totals {
			if total.Currency == "USD" {
				found = true
				if total.RefundAmount != "100.000000" || total.UnallocatedAmount != "-100.000000" {
					t.Fatal(cash)
				}
			}
		}
		if !found {
			t.Fatal(cash)
		}
	})
	if os.Getenv("CP_BROWSER_CONTRACT_REFUNDS_FIXTURE") == "1" && !t.Failed() {
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "contract-foreign-refunds", "contract_refunds_fixture": true, "contract": c, "node": n, "refund": refund, "root_id": root, "refund_event_id": links[0], "project_id": file.File.ProjectID})
		return
	}
	review := func() finance.ContractFinanceReceipt {
		v := summary(c)
		node := v.Nodes[0]
		return data[finance.ContractFinanceReceipt](t, call("POST", "/v1/contracts/"+c.ID+"/finance/milestones/"+n.ID+"/review", finance.ContractFinanceReview{ContractFinanceBase: base(), ExpectedFingerprint: *node.Fingerprint, ConditionsSatisfied: true, AcknowledgeDifference: true, AcknowledgeUnattributedRefunds: true, EvidenceVersionIDs: []string{file.Version.ID}}, 200))
	}
	t.Run("review pins both units and releasing principal invalidates current review", func(t *testing.T) {
		receipt := review()
		var snapshot json.RawMessage
		for _, e := range events() {
			if e.ID == receipt.EventID {
				snapshot = e.ReviewSnapshot
			}
		}
		var proof struct {
			Sources []struct {
				Kind                string                      `json:"kind"`
				AttributionCurrency string                      `json:"attribution_currency"`
				RefundCash          *finance.ContractRefundCash `json:"refund_cash"`
			}
		}
		if e := json.Unmarshal(snapshot, &proof); e != nil {
			t.Fatal(e)
		}
		for _, s := range proof.Sources {
			if s.Kind == "refund" && (s.AttributionCurrency != "HKD" || s.RefundCash == nil || s.RefundCash.Currency != "USD") {
				t.Fatal(s)
			}
		}
		if summary(c).Nodes[0].ReviewState != "current" {
			t.Fatal(summary(c))
		}
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries/"+links[0]+"/release", finance.ContractFinanceRelease{ContractFinanceBase: base(), Amount: "195"}, 200)
		v := summary(c)
		if *v.Nodes[0].CashAmount != "195.000000" || !*v.Nodes[0].HasUnattributedRefunds || v.Nodes[0].ReviewState != "stale" {
			t.Fatal(v)
		}
		for _, e := range events() {
			if e.Kind == "release" && (e.RefundCash == nil || e.RefundCash.Amount != "100.000000") {
				t.Fatal(e)
			}
		}
		for _, e := range events() {
			if e.ID == receipt.EventID && string(e.ReviewSnapshot) != string(snapshot) {
				t.Fatal("old review changed")
			}
		}
	})
	t.Run("concurrent commands cannot oversubscribe released principal", func(t *testing.T) {
		in := linkInput("refund", "150", refund, &root)
		in.RefundFXAcknowledged = true
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.request("POST", "/v1/contracts/"+c.ID+"/finance/entries", f.aliceToken, core.NewID(), in, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
		if *summary(c).Nodes[0].CashAmount != "45.000000" {
			t.Fatal(summary(c))
		}
	})
	t.Run("SQL guard and audit rollback preserve journal and current cash", func(t *testing.T) {
		before := summary(c).Version
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO contract_finance_events(id,org_id,contract_id,milestone_id,contract_revision_id,seq,kind,cost_id,payment_id,source_version,source_revision_id,source_content_hash,source_event_id,amount,actor_id,session_id,mfa_verified_at,reason,refund_fx_acknowledged)
   SELECT gen_random_uuid(),e.org_id,e.contract_id,e.milestone_id,e.contract_revision_id,$3,e.kind,e.cost_id,e.payment_id,e.source_version,e.source_revision_id,e.source_content_hash,e.source_event_id,1,e.actor_id,e.session_id,e.mfa_verified_at,'forged missing acknowledgement',false FROM contract_finance_events e WHERE e.org_id=$1 AND e.id=$2`, f.org, links[3], before+1)
		tx.Rollback(ctx)
		if e == nil || !strings.Contains(e.Error(), "principal acknowledgement") {
			t.Fatal("SQL bypass accepted missing human acknowledgement", e)
		}
		exec(`CREATE FUNCTION reject_contract_refund_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='contract.finance.refund' THEN RAISE EXCEPTION 'isolated audit rejection'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_contract_refund_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_contract_refund_audit()`)
		in := linkInput("refund", "45", refund, &root)
		in.RefundFXAcknowledged = true
		call("POST", "/v1/contracts/"+c.ID+"/finance/entries", in, 500)
		exec("DROP TRIGGER reject_contract_refund_audit ON audit_events; DROP FUNCTION reject_contract_refund_audit()")
		if summary(c).Version != before {
			t.Fatal("audit rollback left attribution")
		}
	})
	t.Run("full cash reversal restores contract cash without rewriting old attribution", func(t *testing.T) {
		review()
		before := refund.ContentHash
		refund = data[finance.Payment](t, call("POST", "/v1/payments/"+refund.ID+"/reverse", finance.PaymentReverseInput{Base: paymentBase(refund), EffectiveAt: "2025-02-03T12:00:00+08:00"}, 200))
		v := summary(c)
		if *v.Nodes[0].CashAmount != "780.000000" || *v.Nodes[0].HasUnattributedRefunds || v.Nodes[0].ReviewState != "stale" || refund.ContentHash != before {
			t.Fatal(v, refund)
		}
		for _, e := range events() {
			if e.Kind == "refund" && e.EffectiveAmount != "0.000000" && e.EffectiveAmount != "0" {
				t.Fatal(e)
			}
		}
	})
}
