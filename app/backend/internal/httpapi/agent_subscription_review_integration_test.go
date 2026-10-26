package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
)

// Actual native HTTP finance commands, a durable controller lease and receipts.
// Cloud fixtures are isolated in memory; no provider, bot or virtual environment.
func TestAgentSubscriptionReview(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	ptr := func(v string) *string { return &v }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	for _, key := range []string{"agent_user", "subscription_manager", "forecast_reporter", "cost_recorder", "cost_confirmer", "finance_reporter", "payment_recorder", "cash_reporter", "party_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, key)
	}
	for _, key := range []string{"agent_user", "subscription_reader", "forecast_reporter", "finance_reporter", "party_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.bob, key)
	}
	for _, class := range []string{"internal", "project_restricted"} {
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,$3,true,ARRAY['web','wechat','douyin','android'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3`, core.NewID(), f.org, class)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	r, v := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Monthly finance evidence vendor", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "controlled native finance evidence"}, nil)
	expect(t, r, 201, v)
	vendor := data[party.Party](t, v)
	supplier := &finance.PartyRef{PartyID: vendor.ID, RevisionID: vendor.CurrentRevisionID}
	base := func(version int64, revision, hash string) finance.Base {
		return finance.Base{ExpectedVersion: version, RevisionID: revision, ContentHash: hash, Reason: "human verifies exact financial facts"}
	}
	newCost := func(currency, amount string) finance.Cost {
		t.Helper()
		in := finance.Create{Content: finance.Content{Title: "Monthly native incurred cost", OccurredOn: ptr("2025-01-15"), Currency: ptr(currency), Amount: ptr(amount), Category: "secretary-monthly", Counterparty: supplier, Allocations: []finance.Allocation{{ProjectID: file.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "native-summary:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "controlled native financial basis"}
		r, v := f.request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		cost := data[finance.Cost](t, v)
		r, v = f.request("POST", "/v1/costs/"+cost.ID+"/confirm", f.aliceToken, core.NewID(), base(cost.Version, cost.CurrentRevisionID, cost.ContentHash), nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	hkd := newCost("HKD", "100.12")
	_ = newCost("USD", "7.01")
	subscriptions := []finance.Subscription{}
	periodIDs := []string{}
	for _, price := range [][2]string{{"HKD", "100"}, {"USD", "7"}} {
		anchor := 31
		in := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Native monthly subscription", Supplier: supplier, OwnerID: &f.alice, Category: "secretary-monthly", StartDate: ptr("2025-01-01"), Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: ptr("2025-01-31"), AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0, 7}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: price[0], Amount: price[1], Plan: "Exact fixed plan"}}}, SourceKey: "native-summary-sub:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "controlled subscription basis"}
		r, v := f.request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		sub := data[finance.Subscription](t, v)
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(sub.Version, sub.CurrentRevisionID, sub.ContentHash), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		sub = data[finance.Subscription](t, v)
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(sub.Version, sub.CurrentRevisionID, sub.ContentHash), From: "2025-01-01", To: "2025-02-01"}, nil)
		expect(t, r, 200, v)
		generation := data[finance.SubscriptionGeneration](t, v)
		periodIDs = append(periodIDs, generation.GeneratedIDs[0])
		subscriptions = append(subscriptions, sub)
	}
	paymentInput := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Native monthly cash", Direction: ptr("outgoing"), PaidAt: ptr("2025-01-20T12:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr("40"), Category: "secretary-monthly", Counterparty: supplier, MethodLabel: "Controlled transfer", EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "native-summary-payment:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "actual cash differs from incurred costs"}
	r, v = f.request("POST", "/v1/payments", f.aliceToken, core.NewID(), paymentInput, nil)
	expect(t, r, 201, v)
	payment := data[finance.Payment](t, v)
	r, v = f.request("POST", "/v1/payments/"+payment.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: base(payment.Version, payment.CurrentRevisionID, payment.ContentHash), RefundAllocations: []finance.RefundAllocation{}}, nil)
	expect(t, r, 200, v)
	payment = data[finance.Payment](t, v)
	r, v = f.request("POST", "/v1/payments/"+payment.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: base(payment.Version, payment.CurrentRevisionID, payment.ContentHash), CostID: hkd.ID, CostVersion: hkd.Version, CostRevisionID: hkd.CurrentRevisionID, CostContentHash: hkd.ContentHash, Amount: "40"}, nil)
	expect(t, r, 200, v)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'subscription-review','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,30,600,$4,'Native monthly finance summary fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	svc := &agent.Service{DB: f.api.DB}
	start := func(projectID *string, members []string) (agent.Conversation, agent.Run, agent.ExecutionLease) {
		t.Helper()
		kind := "project"
		if projectID == nil {
			kind = "private"
		}
		r, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: kind, ProjectID: projectID, Title: "月度财务核对", MemberIDs: members}, nil)
		expect(t, r, 201, v)
		c := data[agent.Conversation](t, v)
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "请分别核对2025年1月订阅预测、发生支出和现金，保留原币种。", ContextRefs: []agent.SourceReference{}}, nil)
		expect(t, r, 202, v)
		run := data[agent.Run](t, v)
		job, err := svc.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "finance-summary.test", agent.ExecutionLeaseDuration)
		if err != nil || job == nil {
			t.Fatal("claim", err)
		}
		execution, err := svc.IssueExecution(ctx, *job)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := svc.LeaseForCredential(ctx, execution.Credential)
		if err != nil {
			t.Fatal(err)
		}
		if err = svc.RecordVerifiedConfinement(ctx, lease, execution.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err = svc.StartExecution(ctx, lease); err != nil {
			t.Fatal(err)
		}
		return c, run, lease
	}
	getCost := func(id string) finance.Cost {
		t.Helper()
		r, v := f.request("GET", "/v1/costs/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	getPeriod := func(id string) finance.SubscriptionPeriod {
		t.Helper()
		r, v := f.request("GET", "/v1/subscription-periods/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.SubscriptionPeriod](t, v)
	}
	hkd = getCost(hkd.ID)
	period := getPeriod(periodIDs[0])
	periodBase := func(p finance.SubscriptionPeriod) finance.SubscriptionPeriodBase {
		return finance.SubscriptionPeriodBase{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "verify actual linked subscription charge"}
	}
	r, v = f.request("POST", "/v1/subscription-periods/"+period.ID+"/costs", f.aliceToken, core.NewID(), finance.SubscriptionPeriodLink{SubscriptionPeriodBase: periodBase(period), CostID: hkd.ID, CostVersion: hkd.Version, CostRevisionID: hkd.CurrentRevisionID, CostContentHash: hkd.ContentHash}, nil)
	expect(t, r, 200, v)
	period = data[finance.SubscriptionPeriod](t, v)
	r, v = f.request("POST", "/v1/subscription-periods/"+period.ID+"/review", f.aliceToken, core.NewID(), finance.SubscriptionPeriodDecision{SubscriptionPeriodBase: periodBase(period), ExpectedActualFingerprint: &period.ActualFingerprint, Action: "confirm", AcknowledgeDifference: true, InvoiceMissing: true, EvidenceVersionIDs: []string{file.Version.ID}}, nil)
	expect(t, r, 200, v)
	period = data[finance.SubscriptionPeriod](t, v)
	type snapshot struct {
		Content struct {
			ReportID string          `json:"report_id"`
			View     string          `json:"view"`
			Report   json.RawMessage `json:"report"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	}
	decode := func(raw []byte) snapshot {
		t.Helper()
		var out snapshot
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	call := func(lease agent.ExecutionLease, tool string, in any) ([]byte, error) {
		t.Helper()
		args, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return svc.ExecutionTool(ctx, lease, core.NewID(), tool, args)
	}
	finish := func(lease agent.ExecutionLease, sources []agent.ResultReference) {
		t.Helper()
		if err := svc.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "已逐笔核对原始账期、费用、现金分配；金额按原币种保留，候选异常需人工判断。", SourceRefs: sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); err != nil {
			t.Fatal(err)
		}
	}
	reviewInput := finance.SubscriptionReviewInput{SubscriptionID: subscriptions[0].ID, From: "2025-01-01", To: "2025-02-01"}
	shared, _, sharedLease := start(file.File.ProjectID, []string{f.bob})
	discovery, err := call(sharedLease, "list_subscriptions", finance.SubscriptionDiscovery{})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("discovery uses real subscription IDs and cannot prove completion", func(t *testing.T) {
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Complete bool `json:"list_complete_on_this_page"`
		}
		if err := json.Unmarshal(decode(discovery).Content.Report, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 2 || !page.Complete {
			t.Fatal(string(discovery))
		}
		if strings.Contains(string(discovery), "100.000000") {
			t.Fatal("discovery disclosed full finance details")
		}
		if err := svc.CompleteExecution(ctx, sharedLease, agent.ProposedResult{Status: "completed", Summary: "Only a discovery page", SourceRefs: decode(discovery).Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); err == nil {
			t.Fatal("discovery proved a completed review")
		}
	})
	t.Run("one participant without payment access cannot receive another participant settlement", func(t *testing.T) {
		if _, err := call(sharedLease, "read_subscription_review", reviewInput); err == nil {
			t.Fatal("rich settlement leaked into shared review")
		}
		if _, err := call(sharedLease, "read_finance_record", finance.FinanceRecordInput{SubscriptionID: subscriptions[0].ID, PeriodID: period.ID, Kind: "payment", ID: payment.ID}); err == nil {
			t.Fatal("payment leaked into shared record")
		}
	})
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='cash_reporter'`, core.NewID(), f.org, f.bob)
	sharedDetail, err := call(sharedLease, "read_subscription_review", reviewInput)
	if err != nil {
		t.Fatal("equal reading rights with different editing rights must work", err)
	}
	finish(sharedLease, decode(sharedDetail).Sources)
	c, run, lease := start(file.File.ProjectID, []string{})
	invocation := core.NewID()
	args, _ := json.Marshal(reviewInput)
	raw, err := svc.ExecutionTool(ctx, lease, invocation, "read_subscription_review", args)
	if err != nil {
		t.Fatal(err)
	}
	actual := decode(raw)
	var report struct {
		Subscription finance.Subscription         `json:"subscription"`
		Periods      []finance.SubscriptionPeriod `json:"periods"`
		Costs        []struct {
			ID          string `json:"id"`
			Amount      string `json:"original_amount"`
			Paid        string `json:"paid_amount"`
			Outstanding string `json:"outstanding_amount"`
		} `json:"costs"`
		Payments []struct {
			ID     string `json:"id"`
			Amount string `json:"original_amount"`
		} `json:"payments"`
		Allocations []finance.PaymentAllocation `json:"allocations"`
		Complete    bool                        `json:"coverage_complete"`
		Expected    int                         `json:"expected_scheduled_period_count"`
	}
	if err = json.Unmarshal(actual.Content.Report, &report); err != nil {
		t.Fatal(err)
	}
	t.Run("exact forecast incurred cash and allocations match their ordinary APIs", func(t *testing.T) {
		if actual.Content.View != "subscription_review" || report.Subscription.ID != subscriptions[0].ID || !report.Complete || report.Expected != 1 || len(report.Periods) != 1 || len(report.Costs) != 1 || len(report.Payments) != 1 || len(report.Allocations) != 1 {
			t.Fatal(string(raw))
		}
		cost := getCost(hkd.ID)
		if report.Periods[0].ID != period.ID || report.Periods[0].ActualFingerprint != period.ActualFingerprint || report.Periods[0].Amount != "100.000000" || report.Costs[0].Amount != *cost.Amount || report.Costs[0].Paid != *cost.PaidAmount || report.Costs[0].Outstanding != *cost.OutstandingAmount || report.Payments[0].Amount != "40.000000" || report.Allocations[0].Amount != "40.000000" || report.Allocations[0].CostID != hkd.ID || report.Allocations[0].Shares[0].Amount != "40.000000" {
			t.Fatal(string(raw))
		}
		if !strings.Contains(string(raw), "invoice_missing") || !strings.Contains(string(raw), "overcharge") || report.Costs[0].Outstanding != "60.120000" {
			t.Fatal(string(raw))
		}
		if strings.Contains(string(raw), "cost_settlement") || strings.Contains(string(raw), "dependency_project_id") {
			t.Fatal("private authorization catalogue leaked")
		}
	})
	t.Run("missing generated periods remain missing obligations rather than zero", func(t *testing.T) {
		body, err := call(lease, "read_subscription_review", finance.SubscriptionReviewInput{SubscriptionID: subscriptions[0].ID, From: "2025-02-01", To: "2025-03-01"})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Complete bool                         `json:"coverage_complete"`
			Missing  []finance.SubscriptionWindow `json:"missing_scheduled_periods"`
			Costs    []any                        `json:"costs"`
		}
		if err = json.Unmarshal(decode(body).Content.Report, &result); err != nil {
			t.Fatal(err)
		}
		if result.Complete || len(result.Missing) != 1 || len(result.Costs) != 0 {
			t.Fatal(string(body))
		}
	})
	t.Run("dates scopes and actual record relationships cannot be supplied arbitrarily", func(t *testing.T) {
		for _, in := range []any{map[string]any{"subscription_id": subscriptions[0].ID, "from": "2025-02-30", "to": "2025-03-01"}, map[string]any{"subscription_id": subscriptions[0].ID, "from": "2025-01-01", "to": "2027-01-01"}, map[string]any{"subscription_id": subscriptions[0].ID, "from": "2025-01-01", "to": "2025-02-01", "project_id": file.File.ProjectID}, map[string]any{"subscription_id": core.NewID(), "from": "2025-01-01", "to": "2025-02-01"}} {
			if _, err := call(lease, "read_subscription_review", in); err == nil {
				t.Fatal("invalid scope accepted", in)
			}
		}
		for _, in := range []finance.FinanceRecordInput{{SubscriptionID: subscriptions[1].ID, PeriodID: period.ID, Kind: "cost", ID: hkd.ID}, {SubscriptionID: subscriptions[0].ID, PeriodID: periodIDs[1], Kind: "payment", ID: payment.ID}, {SubscriptionID: subscriptions[0].ID, PeriodID: period.ID, Kind: "cost", ID: core.NewID()}} {
			if _, err := call(lease, "read_finance_record", in); err == nil {
				t.Fatal("unrelated record accepted", in)
			}
		}
	})
	costBody, err := call(lease, "read_finance_record", finance.FinanceRecordInput{SubscriptionID: subscriptions[0].ID, PeriodID: period.ID, Kind: "cost", ID: hkd.ID})
	if err != nil {
		t.Fatal(err)
	}
	paymentBody, err := call(lease, "read_finance_record", finance.FinanceRecordInput{SubscriptionID: subscriptions[0].ID, PeriodID: period.ID, Kind: "payment", ID: payment.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("associated original records retain exact original amounts and fixed evidence", func(t *testing.T) {
		for _, item := range []struct {
			raw        []byte
			id, amount string
		}{{costBody, hkd.ID, "100.120000"}, {paymentBody, payment.ID, "40.000000"}} {
			var detail struct {
				Record struct {
					ID          string `json:"id"`
					Amount      string `json:"amount"`
					Attachments []struct {
						VersionID string `json:"version_id"`
					} `json:"attachments"`
				} `json:"record"`
			}
			if err := json.Unmarshal(decode(item.raw).Content.Report, &detail); err != nil {
				t.Fatal(err)
			}
			if detail.Record.ID != item.id || detail.Record.Amount != item.amount || len(detail.Record.Attachments) != 1 || detail.Record.Attachments[0].VersionID != file.Version.ID {
				t.Fatal(string(item.raw))
			}
		}
	})
	t.Run("snapshots are immutable receipted and exact invocation replay is stable", func(t *testing.T) {
		for _, q := range []string{"UPDATE agent_finance_reports SET title='changed' WHERE id=$1", "DELETE FROM agent_finance_reports WHERE id=$1"} {
			if _, err := f.owner.Exec(ctx, q, actual.Content.ReportID); err == nil {
				t.Fatal("financial snapshot mutated")
			}
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO agent_finance_reports SELECT $2,org_id,run_id,conversation_id,actor_id,project_id,classification,audience,title,jsonb_set(content_text::jsonb,'{report_id}',to_jsonb($2::text))::text,evidence,document_sha256,created_at FROM agent_finance_reports WHERE id=$1`, actual.Content.ReportID, core.NewID()); err == nil {
			t.Fatal("unreceipted detailed snapshot committed")
		}
		replay, err := svc.ExecutionTool(ctx, lease, invocation, "read_subscription_review", args)
		left, _ := core.CanonicalJSON(raw)
		right, _ := core.CanonicalJSON(replay)
		if err != nil || string(left) != string(right) {
			t.Fatal("idempotent snapshot changed", err)
		}
	})
	t.Run("receipt storage failure rolls back the detailed snapshot", func(t *testing.T) {
		var before, after int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_finance_reports WHERE run_id=$1", run.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		exec(`CREATE FUNCTION reject_review_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.tool='read_subscription_review' THEN RAISE EXCEPTION 'controlled receipt failure'; END IF; RETURN NEW; END $$`)
		exec(`CREATE TRIGGER reject_review_receipt BEFORE INSERT ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION reject_review_receipt()`)
		_, err := call(lease, "read_subscription_review", reviewInput)
		exec(`DROP TRIGGER reject_review_receipt ON agent_tool_receipts`)
		exec(`DROP FUNCTION reject_review_receipt()`)
		if err == nil {
			t.Fatal("failed receipt was accepted")
		}
		if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_finance_reports WHERE run_id=$1", run.ID).Scan(&after); err != nil || after != before {
			t.Fatal("receipt failure left an orphan snapshot", before, after, err)
		}
	})
	sources := append(append(actual.Sources, decode(costBody).Sources...), decode(paymentBody).Sources...)
	finish(lease, sources)
	t.Run("completed history keeps snapshots inside the original conversation", func(t *testing.T) {
		r, v := f.request("GET", "/v1/conversations/"+shared.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("cross conversation detailed report exposed", v)
		}
		r, v = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
	})
	company, _, companyLease := start(nil, []string{})
	t.Run("company conversations cannot silently discover project subscriptions", func(t *testing.T) {
		body, err := call(companyLease, "list_subscriptions", finance.SubscriptionDiscovery{})
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Items []any `json:"items"`
		}
		if err = json.Unmarshal(decode(body).Content.Report, &page); err != nil || len(page.Items) != 0 {
			t.Fatal(string(body), err)
		}
		if _, err = call(companyLease, "read_subscription_review", reviewInput); err == nil {
			t.Fatal("company conversation escaped into a project")
		}
		body, err = call(companyLease, "read_finance_summary", map[string]any{"month": "2025-01", "view": "actual"})
		if err != nil {
			t.Fatal(err)
		}
		finish(companyLease, decode(body).Sources)
		if company.ProjectID != nil {
			t.Fatal("company fixture has project scope")
		}
	})
	newSubscription := func(content finance.SubscriptionContent) finance.Subscription {
		t.Helper()
		r, v := f.request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), finance.SubscriptionCreate{SubscriptionContent: content, SourceKey: "review-boundary:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "controlled detail review boundary"}, nil)
		expect(t, r, 201, v)
		return data[finance.Subscription](t, v)
	}
	_, _, pagingLease := start(file.File.ProjectID, []string{})
	t.Run("discovery really continues after twenty rows and cursor binds query and conversation", func(t *testing.T) {
		for i := 0; i < 21; i++ {
			content := subscriptions[0].SubscriptionContent
			content.Product = "Paged review " + core.NewID()
			newSubscription(content)
		}
		type discoveryPage struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Next      *string `json:"next_cursor"`
			Complete  bool    `json:"list_complete_on_this_page"`
			Exhausted bool    `json:"page_exhausted"`
		}
		firstBody, err := call(pagingLease, "list_subscriptions", finance.SubscriptionDiscovery{Query: "Paged review"})
		if err != nil {
			t.Fatal(err)
		}
		var first, last discoveryPage
		if err = json.Unmarshal(decode(firstBody).Content.Report, &first); err != nil {
			t.Fatal(err)
		}
		if len(first.Items) != 20 || first.Next == nil || first.Complete || first.Exhausted {
			t.Fatal(string(firstBody))
		}
		lastBody, err := call(pagingLease, "list_subscriptions", finance.SubscriptionDiscovery{Query: "Paged review", Cursor: *first.Next})
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(decode(lastBody).Content.Report, &last); err != nil {
			t.Fatal(err)
		}
		if len(last.Items) != 1 || last.Next != nil || last.Complete || !last.Exhausted {
			t.Fatal(string(lastBody))
		}
		for _, item := range first.Items {
			if item.ID == last.Items[0].ID {
				t.Fatal("cursor duplicated a row")
			}
		}
		if _, err = call(pagingLease, "list_subscriptions", finance.SubscriptionDiscovery{Query: "Different", Cursor: *first.Next}); err == nil {
			t.Fatal("cursor accepted another query")
		}
		// Same actor and project still do not permit reuse in another conversation.
		_, _, otherLease := start(file.File.ProjectID, []string{})
		if _, err = call(otherLease, "list_subscriptions", finance.SubscriptionDiscovery{Query: "Paged review", Cursor: *first.Next}); err == nil {
			t.Fatal("cursor accepted another conversation")
		}
		body, err := call(otherLease, "read_subscription_review", reviewInput)
		if err != nil {
			t.Fatal(err)
		}
		finish(otherLease, decode(body).Sources)
	})
	t.Run("more than one hundred periods fail without a persisted partial review", func(t *testing.T) {
		content := subscriptions[0].SubscriptionContent
		content.Product = "Bounded daily review"
		content.Cycle = ptr("custom")
		content.AnchorDay = nil
		days := 1
		content.CustomIntervalDays = &days
		content.FirstChargeDate = ptr("2025-01-01")
		sub := newSubscription(content)
		r, v := f.request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(sub.Version, sub.CurrentRevisionID, sub.ContentHash), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		sub = data[finance.Subscription](t, v)
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/periods/generate", f.aliceToken, core.NewID(), finance.SubscriptionGenerate{Base: base(sub.Version, sub.CurrentRevisionID, sub.ContentHash), From: "2025-01-01", To: "2025-05-01"}, nil)
		expect(t, r, 200, v)
		if len(data[finance.SubscriptionGeneration](t, v).GeneratedIDs) != 120 {
			t.Fatal("fixture did not exceed bound")
		}
		var before, after int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_finance_reports WHERE org_id=$1", f.org).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if _, err := call(pagingLease, "read_subscription_review", finance.SubscriptionReviewInput{SubscriptionID: sub.ID, From: "2025-01-01", To: "2025-05-01"}); err == nil {
			t.Fatal("a partial prefix claimed detailed review")
		}
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_finance_reports WHERE org_id=$1", f.org).Scan(&after); err != nil || before != after {
			t.Fatal("limit failure persisted a report", err)
		}
	})
	pagingBody, err := call(pagingLease, "read_subscription_review", reviewInput)
	if err != nil {
		t.Fatal(err)
	}
	finish(pagingLease, decode(pagingBody).Sources)
	if os.Getenv("CP_BROWSER_SUBSCRIPTION_REVIEW_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"subscription_review_fixture": true, "conversation_id": c.ID, "completed_run_id": run.ID, "profile_id": profile, "project_id": *file.File.ProjectID, "finance_report_id": actual.Content.ReportID, "cost_report_id": decode(costBody).Content.ReportID, "payment_report_id": decode(paymentBody).Content.ReportID, "subscription_id": subscriptions[0].ID, "period_id": period.ID, "cost_id": hkd.ID, "payment_id": payment.ID})
	}
	t.Run("fixed file dependencies withdraw old detailed metadata when a file is trashed", func(t *testing.T) {
		exec("UPDATE files SET state='trashed',trashed_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, file.File.ID)
		r, v := f.request("GET", "/v1/costs/"+hkd.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[finance.Cost](t, v).Attachments) != 0 {
			t.Fatal("trashed attachment still visible")
		}
		r, v = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/agent_finance_report/"+decode(costBody).Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("old rich evidence bypassed fixed file dependency", v)
		}
		exec("UPDATE files SET state='active',trashed_at=NULL WHERE org_id=$1 AND id=$2", f.org, file.File.ID)
	})
	t.Run("payment permission revocation withdraws old rich snapshots even with cost reading retained", func(t *testing.T) {
		exec(`DELETE FROM role_bindings b USING roles r WHERE b.org_id=$1 AND b.user_id=$2 AND r.org_id=b.org_id AND r.id=b.role_id AND r.key='cash_reporter'`, f.org, f.bob)
		r, v := f.request("GET", "/v1/conversations/"+shared.ID+"/context-sources/agent_finance_report/"+decode(sharedDetail).Content.ReportID+"?context_security_version=1&max_chars=4000", f.bobToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("revoked settlement access retained old richer snapshot", v)
		}
		r, v = f.request("GET", "/v1/costs/"+hkd.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.Cost](t, v).SettlementVisibility != "restricted" {
			t.Fatal("fixture did not retain only cost access")
		}
	})
}
