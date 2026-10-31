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
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/party"
)

// Actual native HTTP finance commands, a durable controller lease and receipts.
// Cloud fixtures are isolated in memory; no provider, bot or virtual environment.
func TestAgentSubscriptionPortfolio(t *testing.T) {
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
		in := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "合成订阅 · " + price[0], Supplier: supplier, OwnerID: &f.alice, Category: "secretary-monthly", StartDate: ptr("2025-01-01"), Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: ptr("2025-01-31"), AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0, 7}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: price[0], Amount: price[1], Plan: "Exact fixed plan"}}}, SourceKey: "native-summary-sub:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "controlled subscription basis"}
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
		lease.Job = *job
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

	r, v = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: file.File.ProjectID, Title: "合成演示 · 跨订阅完整审阅", MemberIDs: []string{}}, nil)
	expect(t, r, 201, v)
	c := data[agent.Conversation](t, v)
	path := "/v1/conversations/" + c.ID + "/subscription-review?from=2025-01-01&to=2025-03-01"
	current := t
	run := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { current = st; f.t = st; defer func() { current = t; f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	preview := func(token string, h map[string]string) agent.SubscriptionPortfolioPreview {
		r, v := f.request("GET", path, token, "", nil, h)
		expect(current, r, 200, v)
		out := data[agent.SubscriptionPortfolioPreview](current, v)
		if out.ConversationID != c.ID || out.ConversationRevisionID != c.CurrentRevisionID || out.From != "2025-01-01" || out.To != "2025-03-01" || !out.UntrustedData || core.Digest([]byte(out.ContentText)) != out.ContentSHA256 {
			current.Fatal("wrong preview identity or exact bytes")
		}
		s := out.Summary
		if s.SubscriptionCount != 2 || len(s.Items) != 2 || s.StoredPeriodCount != 2 || s.ExpectedPeriodCount != 4 || s.MissingPeriodCount != 2 || !s.ScheduleCoverageKnown || s.ScheduleCoverageComplete || !s.SubscriptionSetComplete || s.UniqueCostCount != 1 || s.UniquePaymentCount != 1 {
			current.Fatalf("incomplete portfolio: %+v", s)
		}
		if !strings.Contains(out.ContentText, hkd.ID) || !strings.Contains(out.ContentText, payment.ID) || !strings.Contains(out.ContentText, period.ID) {
			current.Fatal("missing actual finance evidence")
		}
		return out
	}
	login := func(device string) (string, map[string]string) {
		var cipher []byte
		if e := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.alice).Scan(&cipher); e != nil {
			current.Fatal(e)
		}
		secret, e := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.alice)
		if e != nil {
			current.Fatal(e)
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.alice, time.Now().Unix()/30-1)
		challenge := f.login("alice", device)
		h := map[string]string{}
		if device == "web" {
			h["Origin"] = "http://localhost:5178"
		}
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, h)
		expect(current, r, 200, v)
		result := data[identity.LoginResult](current, v)
		if device == "web" {
			h["X-CSRF-Token"] = result.CSRFToken
			for _, cookie := range r.Result().Cookies() {
				if cookie.Name == "cp_session" {
					h["Cookie"] = "cp_session=" + cookie.Value
				}
			}
			return "", h
		}
		return result.AccessToken, h
	}
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		run(channel+" complete scope exact dependencies and missing periods", func() { token, h := login(channel); preview(token, h) })
	}
	run("human previews do not persist reports or change financial facts", func() {
		preview(f.aliceToken, nil)
		var reports, costs, payments, periods int
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_finance_reports WHERE org_id=$1),(SELECT count(*) FROM actual_cost_entries WHERE org_id=$1),(SELECT count(*) FROM payment_records WHERE org_id=$1),(SELECT count(*) FROM subscription_periods WHERE org_id=$1)`, f.org).Scan(&reports, &costs, &payments, &periods); e != nil {
			current.Fatal(e)
		}
		if reports != 0 || costs != 2 || payments != 1 || periods != 2 {
			current.Fatal("preview changed ordinary ledger", reports, costs, payments, periods)
		}
	})
	run("explicit strict dates owner audience and no extra scope", func() {
		for _, suffix := range []string{"?from=2025-02-30&to=2025-03-01", "?from=2025-01-01&to=2026-01-03", "?from=2025-01-01&to=2025-01-01", "?from=2025-01-01&from=2025-01-02&to=2025-02-01", "?from=2025-01-01&to=2025-02-01&project_id=" + *file.File.ProjectID} {
			r, v := f.request("GET", "/v1/conversations/"+c.ID+"/subscription-review"+suffix, f.aliceToken, "", nil, nil)
			expect(current, r, 422, v)
		}
		r, v := f.request("GET", path, f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	run("browser_workbench", func() {
		if os.Getenv("CP_BROWSER_SUBSCRIPTION_PORTFOLIO_FIXTURE") != "1" {
			return
		}
		holdBrowserBusinessFixture(current, f, map[string]any{"fixture_kind": "subscription-portfolio", "conversation_id": c.ID, "from": "2025-01-01", "to": "2025-03-01", "subscription_ids": []string{subscriptions[0].ID, subscriptions[1].ID}})
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_finance_reports WHERE org_id=$1", f.org).Scan(&n); e != nil || n != 0 {
			current.Fatal("browser preview persisted model reports", e)
		}
	})
	type toolSnapshot struct {
		Content struct {
			ReportID string `json:"report_id"`
			View     string `json:"view"`
			Report   struct {
				Summary finance.SubscriptionPortfolioSummary `json:"summary"`
			} `json:"report"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	}
	decode := func(raw []byte) toolSnapshot {
		var out toolSnapshot
		if e := json.Unmarshal(raw, &out); e != nil {
			current.Fatal(e)
		}
		return out
	}
	args := []byte(`{"from":"2025-01-01","to":"2025-03-01"}`)
	run("model full portfolio has exact immutable report receipt and completed source", func() {
		cc, rr, l := start(file.File.ProjectID, []string{})
		key := core.NewID()
		raw, e := svc.ExecutionTool(ctx, l, key, "read_subscription_portfolio", args)
		if e != nil {
			current.Fatal(e)
		}
		replay, e := svc.ExecutionTool(ctx, l, key, "read_subscription_portfolio", args)
		if e != nil || string(canonicalCatalogJSON(raw)) != string(canonicalCatalogJSON(replay)) {
			current.Fatal("unstable replay", e)
		}
		result := decode(raw)
		if result.Content.View != "subscription_portfolio" || result.Content.Report.Summary.SubscriptionCount != 2 || len(result.Sources) != 1 {
			current.Fatal("missing complete view or source")
		}
		var text string
		if e = f.owner.QueryRow(ctx, "SELECT content_text FROM agent_finance_reports WHERE org_id=$1 AND run_id=$2 AND id=$3", f.org, rr.ID, result.Content.ReportID).Scan(&text); e != nil || !strings.Contains(text, period.ID) {
			current.Fatal("missing durable full evidence", e)
		}
		if _, e = f.owner.Exec(ctx, "UPDATE agent_finance_reports SET title='rewritten' WHERE id=$1", result.Content.ReportID); e == nil {
			current.Fatal("report mutable")
		}
		if e = svc.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "已完成所选会话范围内跨订阅审阅；存在两个缺失账期，不证明供应商停费或付清。", SourceRefs: result.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			current.Fatal(e)
		}
		r, v := f.request("GET", "/v1/conversations/"+cc.ID+"/context-sources/agent_finance_report/"+result.Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
	})
	run("audit failure rolls back full report and typed receipt", func() {
		_, _, l := start(file.File.ProjectID, []string{})
		defer svc.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED")
		exec(`CREATE FUNCTION fail_portfolio_143() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='agent.tool.read' THEN RAISE EXCEPTION 'synthetic audit fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_portfolio_143 BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_portfolio_143()`)
		_, e := svc.ExecutionTool(ctx, l, core.NewID(), "read_subscription_portfolio", args)
		exec("DROP TRIGGER fail_portfolio_143 ON audit_events; DROP FUNCTION fail_portfolio_143()")
		if e == nil {
			current.Fatal("audit fault accepted")
		}
		var reports, receipts int
		if e = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_finance_reports WHERE run_id=$1),(SELECT count(*) FROM agent_tool_receipts WHERE run_id=$1)`, l.RunID).Scan(&reports, &receipts); e != nil || reports != 0 || receipts != 0 {
			current.Fatal("orphan full report", e, reports, receipts)
		}
	})
	run("one unreadable participant dependency rejects the whole review", func() {
		cc, _, l := start(file.File.ProjectID, []string{f.bob})
		defer svc.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED")
		r, v := f.request("GET", "/v1/conversations/"+cc.ID+"/subscription-review?from=2025-01-01&to=2025-03-01", f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
		if _, e := svc.ExecutionTool(ctx, l, core.NewID(), "read_subscription_portfolio", args); e == nil {
			current.Fatal("shared audience got partial or private cash evidence")
		}
	})
	run("complete beyond discovery page all states exact company scope and whole limit", func() {
		cc, _, l := start(nil, []string{})
		if e := svc.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED"); e != nil {
			current.Fatal(e)
		}
		companyPath := "/v1/conversations/" + cc.ID + "/subscription-review?from=2025-01-01&to=2025-02-01"
		r, v := f.request("GET", companyPath, f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		empty := data[agent.SubscriptionPortfolioPreview](current, v).Summary
		if empty.SubscriptionCount != 0 || empty.ScheduleCoverageKnown || empty.ScheduleCoverageComplete || !empty.SubscriptionSetComplete {
			current.Fatal("empty scope claimed zero obligations")
		}
		for i := 0; i < 101; i++ {
			r, v := f.request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic draft subscription", VendorState: "unknown", Timezone: "Asia/Hong_Kong", ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{}}, SourceKey: "portfolio-limit:" + core.NewID(), Classification: "internal", Reason: "controlled full-set limit"}, nil)
			expect(current, r, 201, v)
			if i == 20 {
				r, v = f.request("GET", companyPath, f.aliceToken, "", nil, nil)
				expect(current, r, 200, v)
				out := data[agent.SubscriptionPortfolioPreview](current, v).Summary
				if out.SubscriptionCount != 21 || len(out.Items) != 21 || out.ScheduleCoverageKnown || out.ScheduleCoverageComplete {
					current.Fatal("only first discovery page or project scope leaked", out)
				}
			}
		}
		r, v = f.request("GET", companyPath, f.aliceToken, "", nil, nil)
		expect(current, r, 422, v)
		_, _, fresh := start(nil, []string{})
		defer svc.FinishStoppedExecution(ctx, fresh.Job, "RUNTIME_EXECUTOR_EXITED")
		_, e := svc.ExecutionTool(ctx, fresh, core.NewID(), "read_subscription_portfolio", []byte(`{"from":"2025-01-01","to":"2025-02-01"}`))
		rejection := core.PublicError(e)
		if e == nil || rejection.Code != "REQUIRED_BUSINESS_FIELD" || len(rejection.Details) != 1 || rejection.Details[0].Field != "scope" || !strings.Contains(rejection.Message, "100") {
			current.Fatal("full-set limit did not cause the model rejection", e)
		}
	})
}
