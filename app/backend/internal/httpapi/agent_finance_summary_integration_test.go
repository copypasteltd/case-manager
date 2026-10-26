package httpapi

import (
	"bytes"
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
func TestAgentFinanceSummary(t *testing.T) {
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
	for _, key := range []string{"agent_user", "subscription_reader", "forecast_reporter", "finance_reporter"} {
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
	start := func(members []string) (agent.Conversation, agent.Run, agent.ExecutionLease) {
		t.Helper()
		r, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: file.File.ProjectID, Title: "月度财务核对", MemberIDs: members}, nil)
		expect(t, r, 201, v)
		c := data[agent.Conversation](t, v)
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "请分别核对2025年1月订阅预测、发生支出和现金，保留原币种。", ContextRefs: []agent.SourceReference{{Type: "project", ID: *file.File.ProjectID}}}, nil)
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
	c, run, lease := start([]string{f.bob})
	call := func(tool string, args any) (json.RawMessage, error) {
		raw, _ := json.Marshal(args)
		return svc.ExecutionTool(ctx, lease, core.NewID(), tool, raw)
	}
	t.Run("general reads do not prove a monthly review", func(t *testing.T) {
		if _, err := call("read_resource", map[string]any{"type": "project", "id": *file.File.ProjectID}); err != nil {
			t.Fatal(err)
		}
		err := svc.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Unverified monthly claim", SourceRefs: []agent.ResultReference{{Type: "project", ID: *file.File.ProjectID}}, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{})
		if err == nil {
			t.Fatal("general project evidence proved finance completion")
		}
	})
	t.Run("strict month currency scope and writes", func(t *testing.T) {
		for _, args := range []any{map[string]any{"month": "2025-13", "view": "actual"}, map[string]any{"month": "2025-1", "view": "actual"}, map[string]any{"month": "2025-01", "view": "mixed"}, map[string]any{"month": "2025-01", "view": "actual", "currency": "hkd"}, map[string]any{"month": "2025-01", "view": "actual", "project_id": core.NewID()}} {
			if _, err := call("read_finance_summary", args); err == nil {
				t.Fatal("invalid or enlarged scope accepted")
			}
		}
		if _, err := call("submit_request", map[string]any{}); err == nil {
			t.Fatal("finance review exposed a write")
		}
	})
	t.Run("all participants must read every contributing record", func(t *testing.T) {
		if _, err := call("read_finance_summary", map[string]any{"month": "2025-01", "view": "cash"}); err == nil {
			t.Fatal("cash leaked to participant without payment read")
		}
	})
	invocation := core.NewID()
	args := json.RawMessage(`{"month":"2025-01","view":"actual"}`)
	out, err := svc.ExecutionTool(ctx, lease, invocation, "read_finance_summary", args)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Content struct {
			ReportID string         `json:"report_id"`
			Report   finance.Report `json:"report"`
			Scope    string         `json:"scope"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	}
	if err = json.Unmarshal(out, &actual); err != nil {
		t.Fatal(err)
	}
	t.Run("incurred decimals and currencies match ordinary report", func(t *testing.T) {
		if actual.Content.Report.Kind != "incurred" || actual.Content.Scope != "project_allocations" || len(actual.Content.Report.Totals) != 2 || actual.Content.Report.Totals[0].NetAmount != "100.120000" || actual.Content.Report.Totals[1].NetAmount != "7.010000" {
			t.Fatal(string(out))
		}
		if strings.Contains(string(out), "revision_id") || strings.Contains(string(out), hkd.ID) {
			t.Fatal("private input catalogue entered model content")
		}
		r, v := f.request("GET", "/v1/reports/expenses?from=2025-01-01&to=2025-02-01&project_id="+*file.File.ProjectID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		current := data[finance.Report](t, v)
		left, _ := json.Marshal(current.Totals)
		right, _ := json.Marshal(actual.Content.Report.Totals)
		if !bytes.Equal(left, right) {
			t.Fatal("ordinary incurred report diverged")
		}
	})
	forecastOut, err := call("read_finance_summary", map[string]any{"month": "2025-01", "view": "forecast"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing forecast periods remain incomplete rather than zero expenditure", func(t *testing.T) {
		out, err := call("read_finance_summary", map[string]any{"month": "2025-02", "view": "forecast"})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Content struct {
				Report finance.ForecastReport `json:"report"`
			} `json:"content"`
		}
		if err = json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		if result.Content.Report.CoverageComplete || result.Content.Report.MissingPeriodCount != 2 || len(result.Content.Report.Totals) != 0 {
			t.Fatal(string(out))
		}
	})
	t.Run("a successful finance tool must be cited for finance completion", func(t *testing.T) {
		err := svc.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Monthly review without its financial source", SourceRefs: []agent.ResultReference{{Type: "project", ID: *file.File.ProjectID}}, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{})
		if err == nil {
			t.Fatal("finance completion accepted omitted financial snapshot")
		}
	})
	t.Run("stored forecast coverage remains separate from actual cost", func(t *testing.T) {
		var result struct {
			Content struct {
				Report finance.ForecastReport `json:"report"`
			} `json:"content"`
		}
		if err := json.Unmarshal(forecastOut, &result); err != nil {
			t.Fatal(err)
		}
		if result.Content.Report.Kind != "forecast" || len(result.Content.Report.Totals) != 2 || result.Content.Report.Totals[0].ExpectedAmount != "100.000000" || !result.Content.Report.CoverageComplete {
			t.Fatal(string(forecastOut))
		}
	})
	t.Run("snapshot is immutable and requires exact receipt", func(t *testing.T) {
		for _, q := range []string{"UPDATE agent_finance_reports SET title='changed' WHERE id=$1", "DELETE FROM agent_finance_reports WHERE id=$1"} {
			if _, err := f.owner.Exec(ctx, q, actual.Content.ReportID); err == nil {
				t.Fatal("financial evidence changed")
			}
		}
		_, err := f.owner.Exec(ctx, `INSERT INTO agent_finance_reports SELECT $2,org_id,run_id,conversation_id,actor_id,project_id,classification,audience,title,jsonb_set(content_text::jsonb,'{report_id}',to_jsonb($2::text))::text,evidence,document_sha256,created_at FROM agent_finance_reports WHERE id=$1`, actual.Content.ReportID, core.NewID())
		if err == nil {
			t.Fatal("unreceipted finance report committed")
		}
	})
	_ = newCost("HKD", "10")
	t.Run("new actual cost does not rewrite historical evidence", func(t *testing.T) {
		replay, err := svc.ExecutionTool(ctx, lease, invocation, "read_finance_summary", args)
		left, _ := core.CanonicalJSON(out)
		right, _ := core.CanonicalJSON(replay)
		if err != nil || !bytes.Equal(left, right) {
			t.Fatal("historical finance receipt changed", err)
		}
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if !strings.Contains(data[agent.ContextView](t, v).Text, "100.120000") {
			t.Fatal("snapshot followed current totals")
		}
	})
	if err = svc.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "2025年1月已按原币种核对：发生支出 HKD 100.120000、USD 7.010000；订阅预测与现金需分别查看。此为生成时刻的历史证据。", SourceRefs: actual.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{InputTokens: 100, OutputTokens: 40}); err != nil {
		t.Fatal(err)
	}
	solo, _, cashLease := start([]string{})
	cashArgs := json.RawMessage(`{"month":"2025-01","view":"cash","currency":"HKD"}`)
	cashOut, err := svc.ExecutionTool(ctx, cashLease, core.NewID(), "read_finance_summary", cashArgs)
	if err != nil {
		t.Fatal(err)
	}
	var cash struct {
		Content struct {
			Report finance.CashReport `json:"report"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	}
	if err = json.Unmarshal(cashOut, &cash); err != nil {
		t.Fatal(err)
	}
	t.Run("cash uses actual paid shares rather than incurred totals", func(t *testing.T) {
		if cash.Content.Report.Kind != "cash" || len(cash.Content.Report.Totals) != 1 || cash.Content.Report.Totals[0].NetAmount != "40.000000" {
			t.Fatal(string(cashOut))
		}
	})
	if err = svc.CompleteExecution(ctx, cashLease, agent.ProposedResult{Status: "completed", Summary: "已核对2025年1月现金 HKD 40；不等同于发生支出。", SourceRefs: cash.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); err != nil {
		t.Fatal(err)
	}
	t.Run("snapshots stay inside original conversation", func(t *testing.T) {
		r, v := f.request("GET", "/v1/conversations/"+solo.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("cross-conversation finance report exposed", v)
		}
	})
	if os.Getenv("CP_BROWSER_FINANCE_SUMMARY_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"finance_summary_fixture": true, "finance_report_id": actual.Content.ReportID, "conversation_id": c.ID, "completed_run_id": run.ID, "profile_id": profile, "project_id": *file.File.ProjectID})
	}
	t.Run("current financial and AI access can withdraw old snapshots", func(t *testing.T) {
		exec(`DELETE FROM role_bindings b USING roles r WHERE b.org_id=$1 AND b.user_id=$2 AND r.org_id=b.org_id AND r.id=b.role_id AND r.key='finance_reporter'`, f.org, f.bob)
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.bobToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("lost cost access did not withdraw financial evidence", v)
		}
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,'project_restricted',false,ARRAY['web','wechat','douyin','android'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'`, core.NewID(), f.org)
		r, v = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/agent_finance_report/"+actual.Content.ReportID+"?context_security_version=1&max_chars=4000", f.aliceToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("revoked AI policy still exposed report", v)
		}
	})
}
