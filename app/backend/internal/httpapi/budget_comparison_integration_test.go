package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/workflow"
)

func TestProjectBudgetComparison(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	pid := *original.File.ProjectID
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
	for _, role := range []string{"project_change_implementer", "request_manager", "cost_recorder", "cost_confirmer", "cost_reverser", "finance_reporter", "party_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key=$3`, f.org, f.alice, role)
	}
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key='approval_reviewer'`, f.org, f.bob, pid)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	post := func(path, token string, in any, status int) response {
		t.Helper()
		r, v := f.request("POST", path, token, core.NewID(), in, nil)
		expect(t, r, status, v)
		return v
	}
	get := func(path, token string, status int) response {
		t.Helper()
		r, v := f.request("GET", path, token, "", nil, nil)
		expect(t, r, status, v)
		return v
	}
	admin := f.administratorToken()
	for _, currency := range []string{"HKD", "USD"} {
		post("/v1/workflow-definitions", admin, workflow.DefinitionInput{Key: "comparison-" + strings.ToLower(currency), Name: "Synthetic comparison budget approval", Kind: "change", Category: "project-control", Currency: currency, MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent budget review", AssignedUserID: f.bob}}, Reason: "isolated comparison rule"}, 201)
	}
	p := data[project.Project](t, get("/v1/projects/"+pid, f.aliceToken, 200))
	post("/v1/projects/"+pid+"/transition", f.aliceToken, project.Transition{ExpectedVersion: p.Version, Action: "activate", Reason: "exercise implemented plan"}, 200)
	apply := func(currency string, lines []project.BudgetLine) {
		t.Helper()
		basis := data[project.ChangeBasis](t, get("/v1/projects/"+pid+"/change-basis?currency="+currency, f.aliceToken, 200))
		receipt := data[project.ChangeReceipt](t, post("/v1/projects/"+pid+"/changes", f.aliceToken, project.ChangeCreate{BaseProjectVersion: basis.ProjectVersion, BaseBudgetID: basis.BudgetID, Reason: "complete controlled comparison plan", Content: project.ChangeContent{Title: "Project comparison budget", Category: "project-control", Currency: currency, Scope: basis.Scope + "\nExplicit reviewed comparison scope.", ScopeDelta: "Keep the reviewed scope.", ScheduleDelta: "Keep the reviewed dates.", StartDate: basis.StartDate, DueDate: basis.DueDate, BudgetLines: lines, FileVersionIDs: []string{original.Version.ID}}}, 201))
		change := data[project.Change](t, get("/v1/changes/"+receipt.ID, f.aliceToken, 200))
		q := data[workflow.Request](t, post("/v1/changes/"+change.ID+"/submit", f.aliceToken, project.ChangeSubmit{ExpectedVersion: change.Version, RevisionID: change.CurrentRevisionID, ContentHash: change.Revision.ContentHash, Reason: "prepare fixed complete proposal"}, 201))
		preview := data[workflow.SubmissionPreview](t, get("/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, 200))
		q = data[workflow.Request](t, post("/v1/requests/"+q.ID+"/submit", f.aliceToken, workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit independently reviewed plan"}, 200))
		q = data[workflow.Request](t, post("/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "independent exact budget approval"}, 200))
		change = data[project.Change](t, get("/v1/changes/"+change.ID, f.aliceToken, 200))
		post("/v1/changes/"+change.ID+"/implement", f.aliceToken, project.ChangeImplement{ExpectedVersion: change.Version, RevisionID: change.CurrentRevisionID, ContentHash: change.Revision.ContentHash, RequestRevisionID: q.CurrentRevisionID, BaseProjectVersion: change.Revision.Basis.ProjectVersion, ScopeVerified: true, BudgetVerified: true, Reason: "human explicitly implements approved budget"}, 200)
	}
	read := func(currency, token string) project.BudgetComparison {
		t.Helper()
		return data[project.BudgetComparison](t, get("/v1/projects/"+pid+"/budget-comparison?currency="+currency, token, 200))
	}
	entries := func(report project.BudgetComparison, extra, token string) project.BudgetComparisonEntries {
		t.Helper()
		return data[project.BudgetComparisonEntries](t, get("/v1/projects/"+pid+"/budget-comparison/entries?currency="+report.Currency+"&basis_hash="+report.BasisHash+extra, token, 200))
	}
	t.Run("no approved plan stays distinct from a zero budget", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		if r.Budget != nil || r.BudgetVariance != nil || len(r.Lines) != 0 || r.Lifetime.NetAmount != "0" || r.Scope != "authorized_records_only" {
			t.Fatal("invented budget", r)
		}
	})
	lines := []project.BudgetLine{{Category: "software", PeriodStart: "2025-01-01", PeriodEnd: "2025-01-31", Amount: "100"}, {Category: "software", PeriodStart: "2025-02-01", PeriodEnd: "2025-02-28", Amount: "200"}, {Category: "equipment", PeriodStart: "2025-01-01", PeriodEnd: "2025-01-31", Amount: "1000"}}
	apply("HKD", lines)
	supplier := data[party.Party](t, post("/v1/parties", f.aliceToken, party.Create{Content: party.Content{Name: "Synthetic budget comparison supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &pid, Classification: "project_restricted", Reason: "fixed synthetic supplier"}, 201))
	other := f.createProject("Other allocation is not this project", nil)
	ptr := func(s string) *string { return &s }
	base := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "verify exact synthetic fact"}
	}
	cost := func(category, date, currency, amount string, allocations []finance.Allocation, confirmed bool) finance.Cost {
		t.Helper()
		c := data[finance.Cost](t, post("/v1/costs", f.aliceToken, finance.Create{Content: finance.Content{Title: "Synthetic " + category + " " + date, OccurredOn: ptr(date), Currency: ptr(currency), Amount: ptr(amount), Category: category, Allocations: allocations, Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, EvidenceVersionIDs: []string{original.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "canonical cost comparison fact"}, 201))
		if confirmed {
			c = data[finance.Cost](t, post("/v1/costs/"+c.ID+"/confirm", f.aliceToken, base(c), 200))
		}
		return c
	}
	share := func(amount string) []finance.Allocation {
		return []finance.Allocation{{ProjectID: &pid, Amount: amount}}
	}
	cost("software", "2025-01-15", "HKD", "100", []finance.Allocation{{ProjectID: &pid, Amount: "60"}, {ProjectID: &other.ID, Amount: "40"}}, true)
	boundary := cost("software", "2025-01-31", "HKD", "50", share("50"), true)
	cost("software", "2025-02-01", "HKD", "75", share("75"), true)
	cost("equipment", "2025-01-01", "HKD", "200", share("200"), true)
	cost("software", "2025-03-01", "HKD", "30", share("30"), true)
	cost("travel", "2025-01-20", "HKD", "25", share("25"), true)
	draft := cost("software", "2025-01-10", "HKD", "999", share("999"), false)
	originalCost := cost("equipment", "2025-01-10", "HKD", "1000", share("1000"), true)
	reversedOriginal := data[finance.Cost](t, post("/v1/costs/"+originalCost.ID+"/reverse", f.aliceToken, finance.Reverse{Base: base(originalCost), EffectiveOn: "2025-02-15"}, 200))
	reversed := data[finance.Cost](t, get("/v1/costs/"+*reversedOriginal.ReversalID, f.aliceToken, 200))
	cost("software", "2025-01-15", "USD", "9999", share("9999"), true)
	cost("software", "2025-01-15", "HKD", "5000", []finance.Allocation{{ProjectID: &other.ID, Amount: "5000"}}, true)
	partial := data[finance.Cost](t, post("/v1/costs", f.aliceToken, finance.Create{Content: finance.Content{Title: "Incomplete date and category", Currency: ptr("HKD"), Allocations: share("1"), EvidenceVersionIDs: []string{}}, SourceKey: "manual:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "incomplete draft remains explicit"}, 201))
	t.Run("incomplete draft dates and categories stay unknown and never break drilldown", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		page := entries(r, "&line=outside&state=draft", f.aliceToken)
		if r.Outside.UnconfirmedCount != 1 || len(page.Items) != 1 || page.Items[0].ID != partial.ID || page.Items[0].OccurredOn != nil || page.Items[0].Category != "" || page.Items[0].SignedAmount != "0" {
			t.Fatal("incomplete draft was invented or omitted", r, page)
		}
	})
	t.Run("exact shares inclusive periods and reversal dates reconcile the lifetime", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		if len(r.Lines) != 3 || r.Budget == nil || r.Budget.Total != "1300.000000" || *r.BudgetVariance != "-85.000000" {
			t.Fatal("wrong fixed plan", r)
		}
		if r.Lines[0].NetAmount != "110.000000" || !r.Lines[0].OverBudget || r.Lines[0].Variance != "-10.000000" || r.Lines[0].UnconfirmedCount != 1 || r.Lines[1].NetAmount != "75.000000" || r.Lines[2].NetAmount != "1200.000000" {
			t.Fatal("wrong line incidence", r.Lines)
		}
		if r.Matched.NetAmount != "1385.000000" || r.Outside.ExpenseAmount != "55.000000" || r.Outside.ReversalAmount != "1000.000000" || r.Outside.NetAmount != "-945.000000" || r.Lifetime.NetAmount != "440.000000" || r.Lifetime.EntryCount != 8 {
			t.Fatal("double count or wrong signs", r)
		}
	})
	t.Run("source drilldown separates drafts reversal and outside scope", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		page := entries(r, "&line=0&state=draft", f.aliceToken)
		if len(page.Items) != 1 || page.Items[0].ID != draft.ID || page.Items[0].SignedAmount != "0" {
			t.Fatal("draft became incurred", page)
		}
		page = entries(r, "&line=outside", f.aliceToken)
		found := false
		for _, item := range page.Items {
			if item.LineIndex != nil {
				t.Fatal("outside matched a line")
			}
			if item.ID == reversed.ID {
				found = true
				if item.SignedAmount != "-1000.000000" {
					t.Fatal("lost reversal sign")
				}
			}
		}
		if len(page.Items) != 3 || !found {
			t.Fatal("missing outside sources", page)
		}
		page = entries(r, "&line=0", f.aliceToken)
		if len(page.Items) != 2 {
			t.Fatal("boundary cost excluded", page)
		}
	})
	t.Run("original currencies do not merge and missing plans stay absent", func(t *testing.T) {
		r := read("USD", f.aliceToken)
		if r.Budget != nil || r.Lifetime.NetAmount != "9999.000000" || r.Outside.EntryCount != 1 || len(r.Lines) != 0 {
			t.Fatal("invented conversion or budget", r)
		}
		r = read("CNY", f.aliceToken)
		if r.Budget != nil || r.Lifetime.EntryCount != 0 {
			t.Fatal("invented CNY costs")
		}
	})
	t.Run("signed pagination binds currency line state project and plan", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		page := entries(r, "&limit=1", f.aliceToken)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("missing pagination")
		}
		seen := map[string]bool{page.Items[0].ID: true}
		cursor := *page.NextCursor
		for page.NextCursor != nil {
			page = entries(r, "&limit=1&cursor="+url.QueryEscape(*page.NextCursor), f.aliceToken)
			for _, item := range page.Items {
				if seen[item.ID] {
					t.Fatal("duplicate canonical source")
				}
				seen[item.ID] = true
			}
		}
		if len(seen) != 8 {
			t.Fatal("source count does not reconcile", len(seen))
		}
		for _, extra := range []string{"&line=0", "&state=draft"} {
			get("/v1/projects/"+pid+"/budget-comparison/entries?currency=HKD&basis_hash="+r.BasisHash+"&cursor="+url.QueryEscape(cursor)+extra, f.aliceToken, 400)
		}
		usd := read("USD", f.aliceToken)
		get("/v1/projects/"+pid+"/budget-comparison/entries?currency=USD&basis_hash="+usd.BasisHash+"&cursor="+url.QueryEscape(cursor), f.aliceToken, 400)
	})
	t.Run("malformed duplicate and incomplete queries reject without widening", func(t *testing.T) {
		for _, query := range []string{"", "?currency=ZZZ", "?currency=HKD&currency=USD", "?currency=HKD&project_id=" + other.ID} {
			get("/v1/projects/"+pid+"/budget-comparison"+query, f.aliceToken, 422)
		}
		r := read("HKD", f.aliceToken)
		for _, extra := range []string{"&line=-1", "&line=03", "&line=3", "&state=all", "&limit=0", "&limit=101", "&line=0&line=1", "&line=%ZZ", "&line=0;line=outside"} {
			get("/v1/projects/"+pid+"/budget-comparison/entries?currency=HKD&basis_hash="+r.BasisHash+extra, f.aliceToken, 422)
		}
		get("/v1/projects/"+pid+"/budget-comparison/entries?currency=HKD", f.aliceToken, 422)
		get("/v1/projects/"+pid+"/budget-comparison/entries?currency=HKD&basis_hash="+strings.Repeat("0", 64), f.aliceToken, 409)
	})
	t.Run("report permission alone never leaks costs or their existence", func(t *testing.T) {
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.member)
		exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer') ON CONFLICT DO NOTHING", f.org, pid, f.member)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'comparison_narrow','Narrow budget reader')", role, f.org)
		for _, action := range []string{"project.read", "budget.read", "report.read"} {
			exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action)
		}
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) VALUES($1,$2,$3,$4,'project',$5,'project_restricted')", core.NewID(), f.org, f.member, role, pid)
		token := f.login("member", "android").AccessToken
		r := read("HKD", token)
		if r.Lifetime.EntryCount != 0 || r.Lifetime.UnconfirmedCount != 0 || r.Lifetime.NetAmount != "0" || len(entries(r, "", token).Items) != 0 {
			t.Fatal("hidden cost signal", r)
		}
		costRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'comparison_one_cost','Read one cost')", costRole, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'cost.read')", f.org, costRole)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) VALUES($1,$2,$3,$4,'resource',$5,'project_restricted')", core.NewID(), f.org, f.member, costRole, boundary.ID)
		exec(`INSERT INTO resource_grants(id,org_id,user_id,resource_type,resource_id,action,classification_ceiling,grantor_id,reason) VALUES($1,$2,$3,'cost',$4,'cost.read','project_restricted',$5,'one visible comparison source')`, core.NewID(), f.org, f.member, boundary.ID, f.alice)
		token = f.login("member", "android").AccessToken
		r = read("HKD", token)
		page := entries(r, "", token)
		if r.Lifetime.EntryCount != 1 || r.Lifetime.NetAmount != "50.000000" || len(page.Items) != 1 || page.Items[0].ID != boundary.ID {
			t.Fatal("record grant widened scope", r, page)
		}
		exec("DELETE FROM resource_grants WHERE org_id=$1 AND user_id=$2 AND resource_id=$3", f.org, f.member, boundary.ID)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id=$3", f.org, f.member, costRole)
		token = f.login("member", "android").AccessToken
		if len(entries(r, "", token).Items) != 0 {
			t.Fatal("revoked cost appeared in a later source read")
		}
	})
	t.Run("missing project or budget authority denies rather than returning zeros", func(t *testing.T) {
		get("/v1/projects/"+other.ID+"/budget-comparison?currency=HKD", f.login("member", "android").AccessToken, 404)
		exec("DELETE FROM role_actions WHERE org_id=$1 AND action='budget.read' AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='comparison_narrow')", f.org)
		get("/v1/projects/"+pid+"/budget-comparison?currency=HKD", f.login("member", "android").AccessToken, 404)
	})
	t.Run("all one hundred approved lines are compared without preview truncation", func(t *testing.T) {
		many := []project.BudgetLine{}
		for i := 0; i < 100; i++ {
			many = append(many, project.BudgetLine{Category: fmt.Sprintf("slot-%03d", i), PeriodStart: "2025-01-01", PeriodEnd: "2025-12-31", Amount: "1"})
		}
		apply("USD", many)
		cost("slot-099", "2025-12-31", "USD", "0.01", share("0.01"), true)
		r := read("USD", f.aliceToken)
		if len(r.Lines) != 100 || r.Lines[99].NetAmount != "0.010000" || r.Lines[99].Variance != "0.990000" || r.Matched.NetAmount != "0.010000" {
			t.Fatal("truncated plan or floating-point amount", r)
		}
	})
	t.Run("an implemented revision invalidates summary basis and old detail cursors", func(t *testing.T) {
		r := read("HKD", f.aliceToken)
		page := entries(r, "&limit=1", f.aliceToken)
		apply("HKD", []project.BudgetLine{})
		get("/v1/projects/"+pid+"/budget-comparison/entries?currency=HKD&basis_hash="+r.BasisHash+"&cursor="+url.QueryEscape(*page.NextCursor), f.aliceToken, 409)
		current := read("HKD", f.aliceToken)
		if current.Budget == nil || current.Budget.Total != "0.000000" || current.BudgetVariance == nil || len(current.Lines) != 0 || current.Outside.EntryCount != 8 || current.Lifetime.NetAmount != "440.000000" {
			t.Fatal("clear plan erased costs or became absent", current)
		}
	})
	if os.Getenv("CP_BROWSER_BUDGET_COMPARISON_FIXTURE") == "1" && !t.Failed() {
		apply("HKD", lines)
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "budget-comparison", "budget_comparison_fixture": true, "project_id": pid, "cost_id": boundary.ID})
	}
}
