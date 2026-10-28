package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/jobqueue"
)

// Reuse the financial source fixture: native source totals and the pre-FX CSV
// are verified first, then new books must not change either historical fact.
func exerciseFXValuation(t *testing.T, f *fixture, pid string, original finance.ReportExport) finance.FXBook {
	ctx := context.Background()
	post := func(token, key string, in finance.FXBookCreate, status int) finance.FXBook {
		t.Helper()
		r, v := f.request("POST", "/v1/finance/fx-books", token, key, in, nil)
		expect(t, r, status, v)
		if status != 201 {
			return finance.FXBook{}
		}
		return data[finance.FXBook](t, v)
	}
	get := func(path, token string, status int) response {
		t.Helper()
		r, v := f.request("GET", path, token, "", nil, nil)
		expect(t, r, status, v)
		return v
	}
	read := func(kind, from, to, book string) finance.ReportPage {
		t.Helper()
		path := "/v1/finance/reports?kind=" + kind + "&from=" + from + "&to=" + to + "&limit=100"
		if book != "" {
			path += "&fx_book_id=" + book
		}
		return data[finance.ReportPage](t, get(path, f.aliceToken, 200))
	}
	original = data[finance.ReportExport](t, get("/v1/finance/report-exports/"+original.ID, f.aliceToken, 200))
	input := finance.FXBookCreate{ExpectedVersion: 0, Acknowledged: true, Reason: "synthetic manually reviewed quotation", Content: finance.FXContent{Title: "Synthetic FX January", BaseCurrency: "HKD", Method: "manual_interval", Rounding: "scope_line_half_up", Rates: []finance.FXRate{{Currency: "USD", ValidFrom: "2025-01-01", ValidTo: "2025-02-01", RateDate: "2024-12-31", Rate: "7.8", Source: "synthetic treasury quotation"}}}}
	t.Run("fx_no_default_policy_and_publishing_requires_specific_authority", func(t *testing.T) {
		p := data[finance.FXPolicy](t, get("/v1/finance/fx-policy", f.aliceToken, 200))
		if p.Version != 0 || p.Current != nil {
			t.Fatal("implicit base currency")
		}
		post(f.bobToken, core.NewID(), input, 404)
		// A quotation changes organization-wide policy; owning a report or a
		// project never grants authority over the company's base currency.
		for _, scope := range []string{"own", "project", "resource"} {
			var binding string
			if err := f.owner.QueryRow(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,$3,CASE WHEN $3='own' THEN NULL ELSE $4::uuid END,'project_restricted' FROM roles WHERE org_id=$1 AND key='finance_fx_manager' RETURNING id`, f.org, f.alice, scope, pid).Scan(&binding); err != nil {
				t.Fatal(err)
			}
			post(f.aliceToken, core.NewID(), input, 404)
			if _, err := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND id=$2", f.org, binding); err != nil {
				t.Fatal(err)
			}
		}
		p = data[finance.FXPolicy](t, get("/v1/finance/fx-policy", f.aliceToken, 200))
		if p.Version != 0 || p.Current != nil {
			t.Fatal("scoped denial changed organization policy")
		}
	})
	if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='finance_fx_manager'`, f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	var first finance.FXBook
	key := core.NewID()
	t.Run("fx_publish_is_attributed_idempotent_and_updates_explicit_company_base", func(t *testing.T) {
		first = post(f.aliceToken, key, input, 201)
		again := post(f.aliceToken, key, input, 201)
		if first.ID != again.ID || first.Revision != 1 || first.Content == nil || first.CreatorID != f.alice || first.RateCount != 1 {
			t.Fatal("bad immutable quote receipt")
		}
		var base string
		if err := f.owner.QueryRow(ctx, "SELECT base_currency FROM organizations WHERE id=$1", f.org).Scan(&base); err != nil || base != "HKD" {
			t.Fatal(base, err)
		}
	})
	t.Run("fx_totals_fold_every_authorized_fact_and_keep_three_bases_independent", func(t *testing.T) {
		r := read("incurred", "2025-01-01", "2025-02-01", first.ID)
		// 100 + 24*1 + later native test's 2 HKD + 10*7.8 USD.
		if r.Valuation == nil || r.Valuation.Coverage != "complete" || r.Valuation.TotalAmount == nil || *r.Valuation.TotalAmount != "204.000000" || r.Valuation.UnconfirmedCount != 1 {
			t.Fatalf("incurred FX %+v", r.Valuation)
		}
		for _, v := range r.Items {
			if v.Currency != nil && *v.Currency == "USD" && (v.Valuation == nil || *v.Valuation.BaseAmount != "78.000000" || *v.Valuation.RateDate != "2024-12-31") {
				t.Fatal("source quote lost")
			}
		}
		cash := read("cash", "2025-01-01", "2025-02-01", first.ID)
		if cash.Valuation == nil || cash.Valuation.TotalAmount == nil || *cash.Valuation.TotalAmount != "90.000000" {
			t.Fatal("cash conflated with expense", cash.Valuation)
		}
		forecast := read("forecast", "2025-01-01", "2025-04-01", first.ID)
		if forecast.Valuation == nil || forecast.Valuation.CoveredAmount != "200.000000" || forecast.Valuation.SourceCoverageComplete || forecast.Valuation.TotalAmount != nil {
			t.Fatal("incomplete source coverage called a full total", forecast.Valuation)
		}
	})
	t.Run("fx_quote_gaps_do_not_turn_into_one_or_zero", func(t *testing.T) {
		r := read("incurred", "2025-01-01", "2025-02-01", first.ID)
		// A later book with no foreign quotes is an explicit incomplete valuation.
		next := input
		next.ExpectedVersion = 1
		next.Content.Title = "Synthetic deliberate missing USD"
		next.Content.Rates = []finance.FXRate{}
		missing := post(f.aliceToken, core.NewID(), next, 201)
		r = read("incurred", "2025-01-01", "2025-02-01", missing.ID)
		if r.Valuation.Coverage != "partial" || r.Valuation.TotalAmount != nil || r.Valuation.MissingRateCount != 1 || r.Valuation.CoveredAmount != "126.000000" {
			t.Fatal("missing quote fabricated a total", r.Valuation)
		}
		// Old exact idempotent receipt remains valid after another policy publish.
		if post(f.aliceToken, key, input, 201).ID != first.ID {
			t.Fatal("replay followed latest book")
		}
		post(f.aliceToken, core.NewID(), input, 409)
	})
	t.Run("fx_reversal_inherits_original_quote_date_outside_its_own_month", func(t *testing.T) {
		jan := read("incurred", "2025-01-01", "2025-02-01", first.ID)
		for _, row := range jan.Items {
			if row.Currency != nil && *row.Currency == "USD" {
				cost := data[finance.Cost](t, get("/v1/costs/"+row.ID, f.aliceToken, 200))
				r, v := f.request("POST", "/v1/costs/"+row.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: finance.Base{ExpectedVersion: cost.Version, RevisionID: cost.CurrentRevisionID, ContentHash: cost.ContentHash, Reason: "synthetic foreign correction"}, EffectiveOn: "2025-02-01"}, nil)
				expect(t, r, 200, v)
			}
		}
		r := read("incurred", "2025-02-01", "2025-03-01", first.ID)
		if r.Valuation.TotalAmount == nil || *r.Valuation.TotalAmount != "-178.000000" {
			t.Fatal(r.Valuation)
		}
		for _, v := range r.Items {
			expected := "2025-01-15"
			if v.Currency != nil && *v.Currency == "USD" {
				expected = "2025-01-16"
				if v.Valuation.Status != "converted" || v.Valuation.BaseAmount == nil || *v.Valuation.BaseAmount != "-78.000000" {
					t.Fatal("foreign reversal lost original quote", v.Valuation)
				}
			}
			if v.EntryKind == "reversal" && v.Valuation.FXDate != expected {
				t.Fatal("cost correction changed quote date")
			}
		}
		r = read("cash", "2025-02-01", "2025-03-01", first.ID)
		for _, v := range r.Items {
			if v.EntryKind == "reversal" && v.Valuation.FXDate != "2025-01-18" {
				t.Fatal("cash correction changed quote date")
			}
		}
	})
	t.Run("fx_read_context_does_not_expand_project_only_finance_access", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, `UPDATE role_bindings SET scope_type='project',scope_id=$3 WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='synthetic_report_only')`, f.org, f.bob, pid); err != nil {
			t.Fatal(err)
		}
		get("/v1/finance/fx-policy", f.bobToken, 404)
		get("/v1/finance/fx-policy?project_id="+pid, f.bobToken, 200)
		get("/v1/finance/fx-books/"+first.ID+"?project_id="+pid, f.bobToken, 200)
		if _, err := f.owner.Exec(ctx, `UPDATE role_bindings SET scope_type='org',scope_id=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='synthetic_report_only')`, f.org, f.bob); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("fx_overlaps_zero_quotes_and_unacknowledged_updates_are_rejected", func(t *testing.T) {
		bad := input
		bad.ExpectedVersion = 2
		bad.Content.Rates = append([]finance.FXRate{}, input.Content.Rates...)
		bad.Content.Rates = append(bad.Content.Rates, bad.Content.Rates[0])
		post(f.aliceToken, core.NewID(), bad, 422)
		bad.Content.Rates = []finance.FXRate{input.Content.Rates[0]}
		bad.Content.Rates[0].Rate = "0"
		post(f.aliceToken, core.NewID(), bad, 422)
		bad = input
		bad.ExpectedVersion = 2
		bad.Acknowledged = false
		post(f.aliceToken, core.NewID(), bad, 422)
	})
	t.Run("fx_concurrent_publish_serializes_before_shared_org_locks", func(t *testing.T) {
		next := input
		next.ExpectedVersion = 2
		next.Content.Title = "Synthetic concurrent quote revision"
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.request("POST", "/v1/finance/fx-books", f.aliceToken, core.NewID(), next, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for c := range codes {
			counts[c]++
		}
		if counts[201] != 1 || counts[409] != 1 {
			t.Fatal("concurrent publish", counts)
		}
	})
	t.Run("fx_books_are_immutable_and_pagination_binds_project_context", func(t *testing.T) {
		for _, q := range []string{"UPDATE finance_fx_books SET title='changed' WHERE id=$1", "DELETE FROM finance_fx_books WHERE id=$1"} {
			if _, err := f.owner.Exec(ctx, q, first.ID); err == nil {
				t.Fatal("rewritable quote book")
			}
		}
		if _, err := f.owner.Exec(ctx, "UPDATE organizations SET base_currency='USD' WHERE id=$1", f.org); err == nil {
			t.Fatal("unpublished base currency")
		}
		page := data[core.Page[finance.FXBook]](t, get("/v1/finance/fx-books?limit=1", f.aliceToken, 200))
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("history not paged")
		}
		get("/v1/finance/fx-books?limit=1&cursor="+url.QueryEscape(*page.NextCursor)+"&project_id="+pid, f.aliceToken, 400)
		get("/v1/finance/fx-books?limit=1&limit=2", f.aliceToken, 422)
		get("/v1/finance/fx-books/"+core.NewID(), f.aliceToken, 404)
	})
	t.Run("fx_export_freezes_book_quotes_and_preserves_legacy_csv", func(t *testing.T) {
		q := finance.ReportQuery{Kind: "incurred", From: "2025-01-01", To: "2025-02-01", FXBookID: &first.ID}
		r, v := f.request("POST", "/v1/finance/report-exports", f.aliceToken, core.NewID(), finance.ReportExportCreate{Filters: q, Reason: "fixed immutable FX snapshot"}, nil)
		expect(t, r, 202, v)
		record := data[finance.ReportExport](t, v)
		queue := jobqueue.Queue{DB: f.api.DB}
		job, err := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", "fx-test", 2*time.Minute)
		if err != nil || job == nil || job.ID != record.JobID {
			t.Fatal("claim", err)
		}
		s := finance.Service{DB: f.api.DB}
		if err = s.ProcessReportExport(ctx, *job); err != nil {
			t.Fatal(err)
		}
		// Read the immutable snapshot via the same native HTTP download authority.
		download := func(id string) string {
			t.Helper()
			req := httptest.NewRequest("GET", "/v1/finance/report-exports/"+id+"/download", nil)
			req.Header.Set("Authorization", "Bearer "+f.aliceToken)
			req.Header.Set("X-Client-Channel", "android")
			rr := httptest.NewRecorder()
			f.handler.ServeHTTP(rr, req)
			if rr.Code != 200 {
				t.Fatalf("download %d", rr.Code)
			}
			return rr.Body.String()
		}
		csv := download(record.ID)
		if !strings.Contains(csv, "base_currency,fx_status,fx_date,base_amount") || !strings.Contains(csv, first.ContentSHA256) || !strings.Contains(csv, "synthetic treasury quotation") {
			t.Fatal("CSV lost pinned FX evidence")
		}
		legacy := download(original.ID)
		if strings.Contains(strings.SplitN(legacy, "\r\n", 2)[0], "base_currency") || core.Digest([]byte(legacy)) != *original.CSVSHA256 {
			t.Fatal("legacy CSV bytes changed")
		}
		var payload string
		if err = f.owner.QueryRow(ctx, "SELECT payload::text FROM finance_report_snapshots WHERE export_id=$1", record.ID).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var snap struct {
			Report finance.ReportPage `json:"report"`
		}
		if json.Unmarshal([]byte(payload), &snap) != nil || snap.Report.Valuation.Book.ID != first.ID {
			t.Fatal("snapshot follows current policy")
		}
	})
	return first
}
