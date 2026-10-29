package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestVersionedCostAmortization(t *testing.T) {
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
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"finance_reporter", "finance_exporter", "party_manager", "cost_recorder", "cost_confirmer", "cost_reverser"} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3", f.org, f.alice, role)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	at := t
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(current *testing.T) { at = current; defer func() { at = t }(); fn(current) })
	}
	post := func(path string, in any, status int) response {
		t.Helper()
		r, v := f.with(at).request("POST", path, f.aliceToken, core.NewID(), in, nil)
		expect(at, r, status, v)
		return v
	}
	get := func(path string) response {
		t.Helper()
		r, v := f.with(at).request("GET", path, f.aliceToken, "", nil, nil)
		expect(at, r, 200, v)
		return v
	}
	ptr := func(s string) *string { return &s }
	base := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "synthetic explicitly reviewed schedule"}
	}
	supplier := data[party.Party](t, post("/v1/parties", party.Create{Content: party.Content{Name: "Synthetic amortization supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &pid, Classification: "project_restricted", Reason: "isolated schedule source"}, 201))
	cost := data[finance.Cost](t, post("/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic three month service", OccurredOn: ptr("2025-01-01"), Currency: ptr("HKD"), Amount: ptr("100"), Category: "software", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: &pid, Amount: "60"}, {Amount: "40"}}, EvidenceVersionIDs: []string{formal.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "isolated allocation arithmetic"}, 201))
	path := "/v1/costs/" + cost.ID + "/amortization"
	input := finance.AmortizationInput{Base: base(cost), ExpectedPlanVersion: 0, Method: "monthly_equal", ServiceFrom: "2025-01-01", ServiceTo: "2025-04-01"}
	run("no_implicit_plan_or_implicit_management_authority", func(t *testing.T) {
		overview := data[finance.AmortizationOverview](t, get(path))
		if overview.Version != 0 || overview.Current != nil || overview.CanManage {
			t.Fatal("draft unexpectedly has an active or manageable schedule")
		}
		r, v := f.request("POST", path+"/preview", f.aliceToken, core.NewID(), input, nil)
		expect(t, r, 404, v)
	})
	exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='finance_amortization_manager'", f.org, f.alice)
	post(path+"/preview", input, 409)
	cost = data[finance.Cost](t, post("/v1/costs/"+cost.ID+"/confirm", base(cost), 200))
	input.Base = base(cost)
	var first finance.AmortizationVersion
	run("schema_queries_current_permissions_and_audit_are_enforced", func(t *testing.T) {
		for _, query := range []string{"?unknown=1", "?limit=1", "?limit=1&limit=2"} {
			r, v := f.with(t).request("GET", path+query, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		r, v := f.with(t).request("GET", path, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		preview := data[finance.AmortizationPreview](t, post(path+"/preview", input, 200))
		publish := finance.AmortizationPublish{AmortizationInput: input, PreviewSHA256: preview.ContentSHA256, Acknowledged: true}
		publish.Acknowledged = false
		post(path, publish, 422)
		publish.Acknowledged = true
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		post(path, publish, 403)
		exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actor.MFAVerifiedAt)
		exec("CREATE FUNCTION fail_amortization_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='cost.amortization.active' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_amortization_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_amortization_audit()")
		post(path, publish, 500)
		exec("DROP TRIGGER fail_amortization_audit ON audit_events; DROP FUNCTION fail_amortization_audit()")
		var count int
		if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM cost_amortization_versions WHERE cost_id=$1", cost.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("failed audit left a published schedule", err)
		}
	})
	run("preview_is_audited_but_does_not_publish_or_change_cost", func(t *testing.T) {
		preview := data[finance.AmortizationPreview](t, post(path+"/preview", input, 200))
		if preview.PlanVersion != 1 || len(preview.Content.Periods) != 3 || preview.Content.Periods[0].Amount != "33.330000" || preview.Content.Periods[1].Amount != "33.340000" || preview.Content.Periods[2].Amount != "33.330000" {
			t.Fatalf("unexpected project-rounded schedule: %#v", preview.Content.Periods)
		}
		if data[finance.AmortizationOverview](t, get(path)).Version != 0 {
			t.Fatal("preview published")
		}
		c := data[finance.Cost](t, get("/v1/costs/"+cost.ID))
		if c.Version != cost.Version || c.ContentHash != cost.ContentHash {
			t.Fatal("preview rewrote source")
		}
		in := finance.AmortizationPublish{AmortizationInput: input, PreviewSHA256: strings.Repeat("0", 64), Acknowledged: true}
		post(path, in, 409)
		in.PreviewSHA256 = preview.ContentSHA256
		key := core.NewID()
		r, v := f.request("POST", path, f.aliceToken, key, in, nil)
		if r.Code == 500 {
			actor, _ := f.api.Identity.Authenticate(ctx, f.aliceToken)
			raw, _ := json.Marshal(in)
			service := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
			_, err := service.PublishAmortization(ctx, core.Command{Actor: actor, Key: core.NewID(), RequestID: core.NewID(), Route: "POST " + path, Body: raw}, cost.ID, in)
			t.Fatalf("isolated schedule publication: %+v", err)
		}
		expect(t, r, 201, v)
		first = data[finance.AmortizationVersion](t, v)
		r, v = f.request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[finance.AmortizationVersion](t, v).ID != first.ID {
			t.Fatal("retry duplicated publication")
		}
		actor, _ := f.api.Identity.Authenticate(ctx, f.aliceToken)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		r, v = f.with(t).request("POST", path, f.aliceToken, key, in, nil)
		expect(t, r, 403, v)
		exec("UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actor.MFAVerifiedAt)
		if first.Version != 1 || first.State != "active" || first.ActorID != f.alice {
			t.Fatal("unattributed publication")
		}
		post(path, in, 409)
	})
	read := func(extra string) finance.ReportPage {
		return data[finance.ReportPage](t, get("/v1/finance/reports?kind=amortization&from=2025-01-01&to=2025-04-02&limit=100"+extra))
	}
	run("independent_scope_report_retains_source_plan_and_exact_company_share", func(t *testing.T) {
		page := read("")
		var summary finance.AmortizationReport
		if err := json.Unmarshal(page.Summary, &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Basis != "latest_active_schedules" || len(summary.Totals) != 1 || summary.Totals[0].NetAmount != "100.000000" || len(page.Items) != 3 {
			t.Fatalf("invalid schedule summary %#v", summary)
		}
		for _, item := range page.Items {
			if item.SourceType != "cost_amortization" || item.ResourceID != cost.ID || item.Amortization == nil || item.Amortization.PlanID != first.ID {
				t.Fatal("missing exact source")
			}
		}
		company := read("&company_only=true")
		if err := json.Unmarshal(company.Summary, &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Totals[0].NetAmount != "40.000000" {
			t.Fatal("company allocation lost")
		}
		project := read("&project_id=" + pid)
		if err := json.Unmarshal(project.Summary, &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Totals[0].NetAmount != "60.000000" {
			t.Fatal("project allocation lost")
		}
		incurred := data[finance.ReportPage](t, get("/v1/finance/reports?kind=incurred&from=2025-01-01&to=2025-04-02"))
		if len(incurred.Items) != 1 || incurred.Items[0].ID != cost.ID {
			t.Fatal("amortization duplicated incurred facts")
		}
	})
	var frozen finance.ReportExport
	var frozenSHA string
	run("export_pins_plan_and_cost_references_without_changing_legacy_csv", func(t *testing.T) {
		query := finance.ReportQuery{Kind: "amortization", From: "2025-01-01", To: "2025-04-02"}
		frozen = data[finance.ReportExport](t, post("/v1/finance/report-exports", finance.ReportExportCreate{Filters: query, Reason: "synthetic exact monthly allocation export"}, 202))
		queue := jobqueue.Queue{DB: f.api.DB}
		job, err := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", "amortization-test", 2*time.Minute)
		if err != nil || job == nil {
			t.Fatalf("claim %v", err)
		}
		service := finance.Service{DB: f.api.DB}
		if err = service.ProcessReportExport(ctx, *job); err != nil {
			t.Fatal(err)
		}
		ready := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+frozen.ID))
		if !ready.CanDownload || ready.CSVSHA256 == nil {
			t.Fatal("export not completed")
		}
		frozenSHA = *ready.CSVSHA256
		req := httptest.NewRequest("GET", "/v1/finance/report-exports/"+frozen.ID+"/download", nil)
		req.Header.Set("Authorization", "Bearer "+f.aliceToken)
		req.Header.Set("X-Client-Channel", "android")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != 200 || core.Digest(w.Body.Bytes()) != frozenSHA {
			t.Fatal("download digest")
		}
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
		if err != nil || len(rows[0]) != 31 || !strings.Contains(w.Body.String(), first.ContentSHA256) {
			t.Fatalf("amortization CSV evidence %v", err)
		}
	})
	run("immutable_rows_cannot_be_edited_or_appended_in_later_transaction", func(t *testing.T) {
		_, invalid := f.owner.Exec(ctx, "INSERT INTO cost_amortization_versions(id,org_id,cost_id,version,state,content,content_sha256,period_count,actor_id,session_id,mfa_verified_at,reason) SELECT gen_random_uuid(),org_id,cost_id,2,state,b.raw,encode(sha256(convert_to(b.raw,'UTF8')),'hex'),period_count,actor_id,session_id,mfa_verified_at,'synthetic null service boundary' FROM cost_amortization_versions v CROSS JOIN LATERAL(SELECT (v.content::jsonb||'{\"service_from\":null}'::jsonb)::text raw)b WHERE id=$1", first.ID)
		if e, ok := invalid.(*pgconn.PgError); !ok || e.Code != "23514" || e.Message != "schedule must pin the exact confirmed original cost and explicit method" {
			t.Fatal("invalid service boundary did not fail the source binding guard", invalid)
		}
		for _, q := range []string{
			"UPDATE cost_amortization_versions SET reason='changed' WHERE id=$1",
			"DELETE FROM cost_amortization_entries WHERE plan_id=$1",
			"UPDATE cost_amortization_shares SET amount=amount+1 WHERE entry_id IN(SELECT id FROM cost_amortization_entries WHERE plan_id=$1)",
			"INSERT INTO cost_amortization_entries(id,reversal_id,org_id,plan_id,seq,service_from,service_to,recognized_on,amount) SELECT gen_random_uuid(),gen_random_uuid(),org_id,id,4,'2025-04-01','2025-05-01','2025-04-30',0 FROM cost_amortization_versions WHERE id=$1",
		} {
			if _, err := f.owner.Exec(ctx, q, first.ID); err == nil {
				t.Fatal("immutable schedule accepted a change")
			}
		}
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, "INSERT INTO cost_amortization_versions(id,org_id,cost_id,version,state,content,content_sha256,period_count,actor_id,session_id,mfa_verified_at,reason) SELECT gen_random_uuid(),org_id,cost_id,2,state,content,content_sha256,period_count,actor_id,session_id,mfa_verified_at,'synthetic missing child rows' FROM cost_amortization_versions WHERE id=$1", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil {
			t.Fatal("incomplete schedule committed without monthly evidence")
		}
	})
	run("next_version_concurrency_history_and_stopping_are_explicit", func(t *testing.T) {
		input.ExpectedPlanVersion = 1
		input.Method = "daily_weighted"
		preview := data[finance.AmortizationPreview](t, post(path+"/preview", input, 200))
		publish := finance.AmortizationPublish{AmortizationInput: input, PreviewSHA256: preview.ContentSHA256, Acknowledged: true}
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.request("POST", path, f.aliceToken, core.NewID(), publish, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		success, conflict := 0, 0
		for code := range codes {
			if code == 201 {
				success++
			}
			if code == 409 {
				conflict++
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("publication race %d/%d", success, conflict)
		}
		history := data[core.Page[finance.AmortizationVersion]](t, get(path+"/versions?limit=1"))
		if len(history.Items) != 1 || history.NextCursor == nil {
			t.Fatal("missing history cursor")
		}
		old := data[finance.AmortizationVersion](t, get(path+"/versions/"+first.ID))
		if old.ContentSHA256 != first.ContentSHA256 || old.Content == nil {
			t.Fatal("old content changed")
		}
		current := data[finance.AmortizationOverview](t, get(path))
		if current.Version != 2 || current.Current.Content.Method != "daily_weighted" {
			t.Fatal("latest version not selected")
		}
		stopped := data[finance.AmortizationVersion](t, post(path+"/stop", finance.AmortizationStop{Base: base(cost), ExpectedPlanVersion: 2, Acknowledged: true}, 201))
		if stopped.Version != 3 || stopped.State != "stopped" {
			t.Fatal("stop not recorded")
		}
		if len(read("").Items) != 0 {
			t.Fatal("stopped plan still drives current view")
		}
		ready := data[finance.ReportExport](t, get("/v1/finance/report-exports/"+frozen.ID))
		if ready.CSVSHA256 == nil || *ready.CSVSHA256 != frozenSHA {
			t.Fatal("plan change altered completed export")
		}
		input.ExpectedPlanVersion = 3
		input.Method = "monthly_equal"
		preview = data[finance.AmortizationPreview](t, post(path+"/preview", input, 200))
		post(path, finance.AmortizationPublish{AmortizationInput: input, PreviewSHA256: preview.ContentSHA256, Acknowledged: true}, 201)
	})
	run("reversal_retains_prior_month_and_negates_on_actual_date_only", func(t *testing.T) {
		post("/v1/costs/"+cost.ID+"/reverse", finance.Reverse{Base: base(cost), EffectiveOn: "2025-02-15"}, 200)
		page := read("")
		if len(page.Items) != 2 {
			t.Fatalf("future months survived reversal: %d", len(page.Items))
		}
		var summary finance.AmortizationReport
		json.Unmarshal(page.Summary, &summary)
		if summary.Totals[0].ExpenseAmount != "33.330000" || summary.Totals[0].ReversalAmount != "33.330000" || summary.Totals[0].NetAmount != "0.000000" {
			t.Fatal("reversal erased or duplicated prior month")
		}
		for _, item := range page.Items {
			if item.Amortization.OriginalDate != "2025-01-31" {
				t.Fatal("original recognition date lost")
			}
			if item.EntryKind == "reversal" && (item.From != "2025-02-15" || *item.ContributionAmount != "-33.330000") {
				t.Fatal("wrong reversal date or sign")
			}
		}
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='finance_fx_manager'", f.org, f.alice)
		book := data[finance.FXBook](t, post("/v1/finance/fx-books", finance.FXBookCreate{ExpectedVersion: 0, Acknowledged: true, Reason: "synthetic January-only quote verifies reversal identity", Content: finance.FXContent{Title: "Synthetic January-only quote", BaseCurrency: "USD", Method: "manual_interval", Rounding: "scope_line_half_up", Rates: []finance.FXRate{{Currency: "HKD", ValidFrom: "2025-01-01", ValidTo: "2025-02-01", RateDate: "2024-12-31", Rate: "2", Source: "synthetic exact quote"}}}}, 201))
		valued := read("&fx_book_id=" + book.ID)
		if valued.Valuation == nil || valued.Valuation.CoveredCount != 2 || valued.Valuation.MissingRateCount != 0 || valued.Valuation.TotalAmount == nil || *valued.Valuation.TotalAmount != "0.000000" {
			t.Fatal("reversal did not use original month quotation", valued.Valuation)
		}
		input.Base = base(data[finance.Cost](t, get("/v1/costs/"+cost.ID)))
		input.ExpectedPlanVersion = 4
		post(path+"/preview", input, 409)
	})
	if os.Getenv("CP_BROWSER_AMORTIZATION_FIXTURE") == "1" && !t.Failed() {
		browserCost := data[finance.Cost](t, post("/v1/costs", finance.Create{Content: finance.Content{Title: "Synthetic annual project service", OccurredOn: ptr("2025-01-01"), Currency: ptr("HKD"), Amount: ptr("100"), Category: "software", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: &pid, Amount: "60"}, {Amount: "40"}}, EvidenceVersionIDs: []string{formal.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "isolated human-operated browser source"}, 201))
		browserCost = data[finance.Cost](t, post("/v1/costs/"+browserCost.ID+"/confirm", base(browserCost), 200))
		workerCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			timer := time.NewTicker(150 * time.Millisecond)
			defer timer.Stop()
			queue := jobqueue.Queue{DB: f.api.DB}
			service := finance.Service{DB: f.api.DB}
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-timer.C:
					job, err := queue.Claim(workerCtx, []string{finance.ReportExportKind}, "reports", "amortization-browser", 2*time.Minute)
					if err == nil && job != nil {
						_ = service.ProcessReportExport(workerCtx, *job)
					}
				}
			}
		}()
		func() {
			defer func() { stop(); <-done }()
			holdBrowserBusinessFixture(t, f, map[string]any{"amortization_fixture": true, "cost_id": browserCost.ID, "project_id": pid, "owner_id": f.alice})
		}()
	}
}
