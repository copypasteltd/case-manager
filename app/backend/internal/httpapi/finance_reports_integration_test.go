package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/party"
)

func TestFinancialReportsAndExports(t *testing.T) {
	u, formal, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	pid := *formal.File.ProjectID
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"finance_reporter", "finance_exporter", "party_manager", "cost_recorder", "cost_confirmer", "cost_reverser", "payment_recorder", "payment_reverser", "cash_reporter", "subscription_manager", "forecast_reporter"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, role)
	}
	exec(`INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES(gen_random_uuid(),$1,'synthetic_report_only','Synthetic report permission only',true)`, f.org)
	exec(`INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'report.read' FROM roles WHERE org_id=$1 AND key='synthetic_report_only'`, f.org)
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='synthetic_report_only'`, f.org, f.bob)
	f.bobToken = memberMFAToken(f, "bob", "android")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	at := t
	run := func(name string, check func(*testing.T)) {
		t.Run(name, func(current *testing.T) { at = current; defer func() { at = t }(); check(current) })
	}
	post := func(path, token string, input any, status int) response {
		t.Helper()
		r, v := f.with(at).request("POST", path, token, core.NewID(), input, nil)
		if r.Code != status {
			at.Logf("failed synthetic business path: %s", path)
		}
		expect(at, r, status, v)
		return v
	}
	get := func(path, token string, status int) response {
		t.Helper()
		r, v := f.with(at).request("GET", path, token, "", nil, nil)
		if r.Code != status {
			at.Logf("failed synthetic business path: %s", path)
		}
		expect(at, r, status, v)
		return v
	}
	ptr := func(s string) *string { return &s }
	supplier := data[party.Party](t, post("/v1/parties", f.aliceToken, party.Create{Content: party.Content{Name: "Synthetic report supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &pid, Classification: "project_restricted", Reason: "isolated report source"}, 201))
	base := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "human verifies canonical source"}
	}
	cost := func(title, date, currency, amount, share, company string, confirmed bool) finance.Cost {
		t.Helper()
		category := "software"
		if currency == "USD" {
			category = "cloud"
		}
		allocations := []finance.Allocation{{ProjectID: &pid, Amount: share}}
		if company != "" {
			allocations = append(allocations, finance.Allocation{Amount: company})
		}
		v := data[finance.Cost](t, post("/v1/costs", f.aliceToken, finance.Create{Content: finance.Content{Title: title, OccurredOn: ptr(date), Currency: ptr(currency), Amount: ptr(amount), Category: category, Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: allocations, EvidenceVersionIDs: []string{formal.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "fixed report arithmetic"}, 201))
		if confirmed {
			v = data[finance.Cost](t, post("/v1/costs/"+v.ID+"/confirm", f.aliceToken, base(v), 200))
		}
		return v
	}
	main := cost("=Synthetic spreadsheet text", "2025-01-15", "HKD", "100", "60", "40", true)
	post("/v1/costs/"+main.ID+"/reverse", f.aliceToken, finance.Reverse{Base: base(main), EffectiveOn: "2025-02-01"}, 200)
	foreign := cost("Synthetic separate USD", "2025-01-16", "USD", "10", "10", "", true)
	cost("Synthetic unconfirmed draft", "2025-01-20", "HKD", "25", "25", "", false)
	for i := 0; i < 24; i++ {
		cost(fmt.Sprintf("Synthetic source %02d", i+1), "2025-01-17", "HKD", "1", "0.60", "0.40", true)
	}

	paymentBase := func(p finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: p.Version, RevisionID: p.CurrentRevisionID, ContentHash: p.ContentHash, Reason: "explicit synthetic cash verification"}
	}
	payment := func(amount string, refund *finance.Payment, recorded bool) finance.Payment {
		t.Helper()
		direction := "outgoing"
		var original *string
		if refund != nil {
			direction = "refund"
			original = &refund.ID
		}
		p := data[finance.Payment](t, post("/v1/payments", f.aliceToken, finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Synthetic report cash " + amount, Direction: ptr(direction), PaidAt: ptr("2025-01-18T12:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr(amount), Category: "software", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, RefundOfID: original, MethodLabel: "synthetic recorded fact", ExternalRef: "synthetic-" + core.NewID(), EvidenceVersionIDs: []string{formal.Version.ID}}, SourceKey: "bank:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "independent cash is not incurred expenditure"}, 201))
		if recorded {
			input := finance.PaymentRecordInput{Base: paymentBase(p), RefundAllocations: []finance.RefundAllocation{}}
			if refund != nil {
				input.ExpectedSourceVersion = &refund.Version
			}
			p = data[finance.Payment](t, post("/v1/payments/"+p.ID+"/record", f.aliceToken, input, 200))
		}
		return p
	}
	outgoing := payment("100", nil, true)
	payment("20", &outgoing, true)
	correction := payment("10", nil, true)
	post("/v1/payments/"+correction.ID+"/reverse", f.aliceToken, finance.PaymentReverseInput{Base: paymentBase(correction), EffectiveAt: "2025-02-01T12:00:00+08:00"}, 200)
	payment("5", nil, false)
	anchor := 31
	sub := data[finance.Subscription](t, post("/v1/subscriptions", f.aliceToken, finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic report recurring service", Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: "software", StartDate: ptr("2025-01-01"), Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: ptr("2025-01-31"), AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0, 7}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "100", Plan: "fixed synthetic price"}}}, SourceKey: "vendor:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "stored forecast remains distinct"}, 201))
	subBase := func(s finance.Subscription) finance.Base {
		return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "reviewed recurring forecast"}
	}
	sub = data[finance.Subscription](t, post("/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, finance.SubscriptionTransition{Base: subBase(sub), Action: "activate", VendorEvidenceVersionIDs: []string{}}, 200))
	post("/v1/subscriptions/"+sub.ID+"/periods/generate", f.aliceToken, finance.SubscriptionGenerate{Base: subBase(sub), From: "2025-01-01", To: "2025-03-01"}, 200)
	post("/v1/subscriptions", f.aliceToken, finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic incomplete subscription", VendorState: "unknown", Prices: []finance.SubscriptionPriceInput{}, ReminderDays: []int{}, VendorEvidenceVersionIDs: []string{}}, SourceKey: "vendor:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "missing configuration is not a zero forecast"}, 201)
	query := finance.ReportQuery{Kind: "incurred", From: "2025-01-01", To: "2025-02-01", ProjectID: &pid}
	path := func(q finance.ReportQuery, extra url.Values) string {
		values := url.Values{"kind": {q.Kind}, "from": {q.From}, "to": {q.To}}
		if q.ProjectID != nil {
			values.Set("project_id", *q.ProjectID)
		}
		if q.CompanyOnly {
			values.Set("company_only", "true")
		}
		for k, v := range map[string]string{"currency": q.Currency, "category": q.Category, "q": q.Query} {
			if v != "" {
				values.Set(k, v)
			}
		}
		if q.SupplierID != nil {
			values.Set("supplier_id", *q.SupplierID)
		}
		if q.OwnerID != nil {
			values.Set("owner_id", *q.OwnerID)
		}
		for k, items := range extra {
			values[k] = items
		}
		return "/v1/finance/reports?" + values.Encode()
	}
	read := func(q finance.ReportQuery) finance.ReportPage {
		t.Helper()
		return data[finance.ReportPage](t, get(path(q, nil), f.aliceToken, 200))
	}
	incurred := func(q finance.ReportQuery) finance.Report {
		t.Helper()
		page := read(q)
		var summary finance.Report
		if err := json.Unmarshal(page.Summary, &summary); err != nil {
			t.Fatal(err)
		}
		return summary
	}
	service := finance.Service{DB: f.api.DB}
	queue := jobqueue.Queue{DB: f.api.DB}
	var first finance.ReportExport
	run("original_currency_project_shares_reversals_and_drafts_match_fixed_manual_totals", func(t *testing.T) {
		r := incurred(query)
		if len(r.Totals) != 2 || r.Totals[0].Currency != "HKD" || r.Totals[0].NetAmount != "74.400000" || r.Totals[1].NetAmount != "10.000000" || r.UnconfirmedCount != 1 || r.FXCoverage != "unconfigured" || r.BaseCurrency != nil {
			t.Fatalf("bad independent totals: %+v", r)
		}
		q := query
		q.From = "2025-02-01"
		q.To = "2025-03-01"
		r = incurred(q)
		if len(r.Totals) != 1 || r.Totals[0].NetAmount != "-60.000000" {
			t.Fatalf("reversal date: %+v", r)
		}
		q = query
		q.ProjectID = nil
		q.CompanyOnly = true
		r = incurred(q)
		if len(r.Totals) != 1 || r.Totals[0].NetAmount != "49.600000" {
			t.Fatalf("company share: %+v", r)
		}
	})
	run("all_filters_are_applied_and_source_pagination_is_signed", func(t *testing.T) {
		q := query
		q.Currency = "USD"
		q.SupplierID = &supplier.ID
		q.OwnerID = &f.alice
		q.Category = "cloud"
		q.Query = "separate"
		p := read(q)
		if len(p.Items) != 1 || p.Items[0].ID != foreign.ID || p.Items[0].ContributionAmount == nil || *p.Items[0].ContributionAmount != "10.000000" {
			t.Fatalf("filtered source: %+v", p)
		}
		q.OwnerID = &f.bob
		if len(read(q).Items) != 0 {
			t.Fatal("owner filter ignored")
		}
		p = data[finance.ReportPage](t, get(path(query, url.Values{"limit": {"1"}}), f.aliceToken, 200))
		if p.NextCursor == nil || len(p.Items) != 1 {
			t.Fatal("missing real pagination")
		}
		second := data[finance.ReportPage](t, get(path(query, url.Values{"limit": {"1"}, "cursor": {*p.NextCursor}}), f.aliceToken, 200))
		if len(second.Items) != 1 || second.Items[0].ID == p.Items[0].ID {
			t.Fatal("duplicate source page")
		}
		q = query
		q.Currency = "USD"
		get(path(q, url.Values{"cursor": {*p.NextCursor}}), f.aliceToken, 400)
	})
	run("strict_queries_reject_unknown_duplicate_malformed_and_incompatible_scopes", func(t *testing.T) {
		for _, tail := range []string{"&supplier_id=bad", "&currency=hkd", "&kind=cash", "&company_only=true", "&mystery=1", "&currency=%ZZ", ";currency=USD", "&limit=0"} {
			get(path(query, nil)+tail, f.aliceToken, 422)
		}
	})
	run("cash_and_forecast_views_keep_independent_facts_and_honest_coverage", func(t *testing.T) {
		q := query
		q.Kind = "cash"
		q.ProjectID = nil
		q.SupplierID = &supplier.ID
		q.OwnerID = &f.alice
		q.Category = "software"
		page := read(q)
		var cash finance.CashReport
		if err := json.Unmarshal(page.Summary, &cash); err != nil {
			t.Fatal(err)
		}
		if len(cash.Totals) != 1 || cash.Totals[0].NetAmount != "90.000000" || cash.Totals[0].UnallocatedAmount != "90.000000" || cash.UnconfirmedCount != 1 || len(page.Items) != 4 {
			t.Fatalf("cash manual example: %+v, rows %d", cash, len(page.Items))
		}
		q.From = "2025-02-01"
		q.To = "2025-03-01"
		page = read(q)
		if err := json.Unmarshal(page.Summary, &cash); err != nil {
			t.Fatal(err)
		}
		if len(cash.Totals) != 1 || cash.Totals[0].NetAmount != "-10.000000" {
			t.Fatalf("cash correction month: %+v", cash)
		}
		q = query
		q.Kind = "forecast"
		q.To = "2025-04-01"
		page = read(q)
		var forecast finance.ForecastReport
		if err := json.Unmarshal(page.Summary, &forecast); err != nil {
			t.Fatal(err)
		}
		if len(forecast.Totals) != 1 || forecast.Totals[0].ExpectedAmount != "200.000000" || forecast.MissingPeriodCount != 1 || forecast.DraftCount != 1 || forecast.CoverageComplete || len(page.Items) != 2 {
			t.Fatalf("forecast and missing coverage: %+v, rows %d", forecast, len(page.Items))
		}
		q.SupplierID = &supplier.ID
		q.OwnerID = &f.alice
		q.Query = "recurring"
		page = read(q)
		if len(page.Items) != 2 {
			t.Fatal("forecast supplier/owner/keyword filters lost periods")
		}
		q.OwnerID = &f.bob
		if len(read(q).Items) != 0 {
			t.Fatal("forecast owner filter ignored")
		}
	})
	run("source_derived_filter_choices_are_paged_and_do_not_reveal_hidden_records", func(t *testing.T) {
		optionPath := strings.Replace(path(query, nil), "/finance/reports?", "/finance/report-options?", 1)
		page := data[core.Page[finance.ReportOption]](t, get(optionPath+"&field=category&limit=1", f.aliceToken, 200))
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("category choices were truncated or not actually paged")
		}
		next := data[core.Page[finance.ReportOption]](t, get(optionPath+"&field=category&limit=1&cursor="+url.QueryEscape(*page.NextCursor), f.aliceToken, 200))
		if len(next.Items) != 1 || next.Items[0].Value == page.Items[0].Value {
			t.Fatal("category choice pagination duplicated a choice")
		}
		supplierChoices := data[core.Page[finance.ReportOption]](t, get(optionPath+"&field=supplier", f.aliceToken, 200))
		if len(supplierChoices.Items) != 1 || supplierChoices.Items[0].Value != supplier.ID || supplierChoices.Items[0].Label != "Synthetic report supplier" {
			t.Fatalf("bad source-derived supplier choices: %+v", supplierChoices)
		}
		hidden := data[finance.ReportPage](t, get(path(query, nil), f.bobToken, 200))
		if len(hidden.Items) != 0 || len(data[core.Page[finance.ReportOption]](t, get(optionPath+"&field=owner", f.bobToken, 200)).Items) != 0 {
			t.Fatal("report permission alone leaked source records or employee options")
		}
		post("/v1/finance/report-exports", f.bobToken, finance.ReportExportCreate{Filters: query, Reason: "export permission must be independent"}, 404)
	})
	run("queued_export_is_idempotent_and_not_a_synchronous_download", func(t *testing.T) {
		input := finance.ReportExportCreate{Filters: query, Reason: "monthly authorized report"}
		key := core.NewID()
		r, v := f.request("POST", "/v1/finance/report-exports", f.aliceToken, key, input, nil)
		expect(t, r, 202, v)
		first = data[finance.ReportExport](t, v)
		r, v = f.request("POST", "/v1/finance/report-exports", f.aliceToken, key, input, nil)
		expect(t, r, 202, v)
		if data[finance.ReportExport](t, v).ID != first.ID {
			t.Fatal("idempotent enqueue duplicated job")
		}
		if first.State != "queued" || first.CanDownload || first.CSVSHA256 != nil {
			t.Fatal("queued job claimed a generated file")
		}
		get("/v1/finance/report-exports/"+first.ID+"/chunks?offset=0&length=10", f.aliceToken, 404)
	})
	run("native_fenced_job_freezes_complete_sources_and_exact_safe_csv", func(t *testing.T) {
		job, err := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", "report-test", 2*time.Minute)
		if err != nil || job == nil {
			t.Fatalf("claim: %v", err)
		}
		stale := *job
		stale.FencingToken++
		if service.ProcessReportExport(ctx, stale) == nil {
			t.Fatal("stale job fence accepted")
		}
		if err = service.ProcessReportExport(ctx, *job); err != nil {
			t.Fatal(err)
		}
		ready := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+first.ID, f.aliceToken, 200))
		if ready.State != "succeeded" || !ready.CanDownload || ready.RowCount == nil || *ready.RowCount != 27 {
			t.Fatalf("incomplete generated report: %+v", ready)
		}
		req := httptest.NewRequest("GET", "/v1/finance/report-exports/"+first.ID+"/download", nil)
		req.Header.Set("Authorization", "Bearer "+f.aliceToken)
		req.Header.Set("X-Client-Channel", "android")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != 200 || core.Digest(w.Body.Bytes()) != *ready.CSVSHA256 || !strings.Contains(w.Body.String(), "'=Synthetic spreadsheet text") {
			t.Fatalf("bad actual CSV response: status %d", w.Code)
		}
		rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(w.Body.Bytes(), []byte{0xef, 0xbb, 0xbf}))).ReadAll()
		if err != nil || len(rows) != 34 {
			t.Fatalf("CSV complete sources: %d %v", len(rows), err)
		}
		chunk := get("/v1/finance/report-exports/"+first.ID+"/chunks?offset=0&length=11", f.aliceToken, 200)
		if !bytes.Contains(chunk.Data, []byte(`"length":"11"`)) {
			t.Fatal("chunk did not retain exact byte length")
		}
		exec(`SELECT 1`)
	})
	run("later_facts_do_not_rewrite_ready_export", func(t *testing.T) {
		before := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+first.ID, f.aliceToken, 200))
		cost("Synthetic later fact", "2025-01-21", "HKD", "2", "2", "", true)
		after := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+first.ID, f.aliceToken, 200))
		if before.CSVSHA256 == nil || after.CSVSHA256 == nil || before.RowCount == nil || after.RowCount == nil {
			t.Fatal("prior generation did not produce a snapshot")
		}
		if *after.CSVSHA256 != *before.CSVSHA256 || *after.RowCount != *before.RowCount {
			t.Fatal("fixed snapshot followed new data")
		}
	})
	run("cancelled_export_cannot_be_claimed_or_downloaded", func(t *testing.T) {
		v := data[finance.ReportExport](t, post("/v1/finance/report-exports", f.aliceToken, finance.ReportExportCreate{Filters: query, Reason: "cancel explicit queued export"}, 202))
		out := data[finance.ReportExport](t, post("/v1/finance/report-exports/"+v.ID+"/cancel", f.aliceToken, map[string]string{"reason": "not required"}, 200))
		if out.State != "cancelled" {
			t.Fatal("cancel not persisted")
		}
		get("/v1/finance/report-exports/"+v.ID+"/chunks?offset=0&length=10", f.aliceToken, 404)
	})
	run("cash_and_forecast_exports_use_their_own_complete_sources", func(t *testing.T) {
		for _, kind := range []string{"cash", "forecast"} {
			q := query
			q.Kind, q.ProjectID = kind, nil
			if kind == "forecast" {
				q.To = "2025-04-01"
			}
			v := data[finance.ReportExport](t, post("/v1/finance/report-exports", f.aliceToken, finance.ReportExportCreate{Filters: q, Reason: "verify separate original facts"}, 202))
			job, err := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", "other-report-test", 2*time.Minute)
			if err != nil || job == nil || job.ID != v.JobID {
				t.Fatalf("claim %s: %v", kind, err)
			}
			if err = service.ProcessReportExport(ctx, *job); err != nil {
				t.Fatal(err)
			}
			ready := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+v.ID, f.aliceToken, 200))
			want := 4
			if kind == "forecast" {
				want = 2
			}
			if ready.RowCount == nil || *ready.RowCount != want || !ready.CanDownload {
				t.Fatalf("wrong %s export: %+v", kind, ready)
			}
		}
	})
	run("export_history_is_owner_only_signed_and_actually_paged", func(t *testing.T) {
		page := data[core.Page[finance.ReportExport]](t, get("/v1/finance/report-exports?limit=1", f.aliceToken, 200))
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("history truncated")
		}
		next := data[core.Page[finance.ReportExport]](t, get("/v1/finance/report-exports?limit=1&cursor="+url.QueryEscape(*page.NextCursor), f.aliceToken, 200))
		if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
			t.Fatal("duplicate history")
		}
		get("/v1/finance/report-exports?kind=cash&cursor="+url.QueryEscape(*page.NextCursor), f.aliceToken, 400)
		get("/v1/finance/report-exports/"+first.ID, f.bobToken, 404)
		hidden := data[core.Page[finance.ReportExport]](t, get("/v1/finance/report-exports?limit=1", f.bobToken, 200))
		if len(hidden.Items) != 0 || hidden.NextCursor != nil {
			t.Fatal("another owner's export leaked")
		}
		for _, suffix := range []string{"offset=-1&length=1", "offset=00&length=1", "offset=0&length=524289", "offset=0&length=1&length=2", "offset=0&length=1&extra=x"} {
			get("/v1/finance/report-exports/"+first.ID+"/chunks?"+suffix, f.aliceToken, 422)
		}
	})
	run("database_guards_preserve_fixed_requests_and_live_snapshots", func(t *testing.T) {
		for _, sql := range []string{`UPDATE finance_report_exports SET reason='rewrite' WHERE id=$1`, `UPDATE finance_report_snapshots SET row_count=0 WHERE export_id=$1`, `DELETE FROM finance_report_snapshots WHERE export_id=$1`} {
			if _, err := f.api.DB.Exec(ctx, sql, first.ID); err == nil {
				t.Fatal("immutable live report changed")
			}
		}
	})
	// The browser reuses these actual fixed arithmetic fixtures and the same
	// processor, with no cloud credentials or fabricated API responses.
	if os.Getenv("CP_BROWSER_FINANCIAL_REPORTS_FIXTURE") == "1" && !t.Failed() {
		workerCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			tick := time.NewTicker(150 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-tick.C:
					job, err := queue.Claim(workerCtx, []string{finance.ReportExportKind}, "reports", "browser-reports", 2*time.Minute)
					if err == nil && job != nil {
						_ = service.ProcessReportExport(workerCtx, *job)
					}
				}
			}
		}()
		func() {
			defer func() { stop(); <-done }()
			holdBrowserBusinessFixture(t, f, map[string]any{"project_id": pid, "supplier_id": supplier.ID, "owner_id": f.alice, "financial_reports_fixture": true, "first_export_id": first.ID})
		}()
	}
	run("expiry_hides_derived_metadata_and_purges_only_payload", func(t *testing.T) {
		// A privileged fixture simulates elapsed retention without sleeping for
		// 24 hours. Production roles cannot update either fixed timestamp.
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, sql := range []string{`ALTER TABLE finance_report_exports DISABLE TRIGGER immutable_finance_report_export`, `UPDATE finance_report_exports SET created_at=clock_timestamp()-interval '25 hours',expires_at=clock_timestamp()-interval '1 hour' WHERE id=$1`, `ALTER TABLE finance_report_exports ENABLE TRIGGER immutable_finance_report_export`} {
			var args []any
			if strings.Contains(sql, "$1") {
				args = []any{first.ID}
			}
			if _, err = tx.Exec(ctx, sql, args...); err != nil {
				t.Fatal(err)
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+first.ID, f.aliceToken, 200))
		if v.State != "expired" || v.CanDownload || v.CSVSHA256 != nil || v.CSVBytes != nil || v.RowCount != nil || v.AsOf != nil {
			t.Fatal("expired derived report still exposed")
		}
		get("/v1/finance/report-exports/"+first.ID+"/chunks?offset=0&length=1", f.aliceToken, 404)
		if _, err = service.PurgeReportSnapshots(ctx); err != nil {
			t.Fatal(err)
		}
		var rows int
		if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM finance_report_exports WHERE id=$1`, first.ID).Scan(&rows); err != nil || rows != 1 {
			t.Fatal("request audit metadata removed")
		}
		if err = f.owner.QueryRow(ctx, `SELECT count(*) FROM finance_report_snapshots WHERE export_id=$1`, first.ID).Scan(&rows); err != nil || rows != 0 {
			t.Fatal("expired payload not purged")
		}
	})
	run("changed_authority_revokes_generation_and_old_downloads", func(t *testing.T) {
		q := query
		v := data[finance.ReportExport](t, post("/v1/finance/report-exports", f.aliceToken, finance.ReportExportCreate{Filters: q, Reason: "authority changes before worker execution"}, 202))
		job, err := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", "revoke-report-test", 2*time.Minute)
		if err != nil || job == nil || job.ID != v.JobID {
			t.Fatal("missing revocation job", err)
		}
		exec(`INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'party.read' FROM roles WHERE org_id=$1 AND key='synthetic_report_only'`, f.org)
		exec(`UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1`, f.org)
		get("/v1/finance/report-exports/"+v.ID, f.aliceToken, 409)
		stale := data[core.Page[finance.ReportExport]](t, get("/v1/finance/report-exports?limit=1", f.aliceToken, 200))
		if len(stale.Items) != 0 || stale.NextCursor != nil {
			t.Fatal("old authorization export still listed")
		}
		exec(`DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='finance_exporter')`, f.org, f.alice)
		if err = service.ProcessReportExport(ctx, *job); err == nil {
			t.Fatal("old-authority worker produced snapshot")
		}
		get("/v1/finance/report-exports/"+v.ID+"/chunks?offset=0&length=1", f.aliceToken, 404)
	})
}
