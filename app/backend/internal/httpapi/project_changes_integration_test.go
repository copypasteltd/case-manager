package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestProjectChangesAndBudgets(t *testing.T) {
	u, old, current, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	pid := *old.File.ProjectID
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	var bindings int
	if e := f.owner.QueryRow(ctx, `SELECT count(*) FROM role_bindings b JOIN roles r ON r.org_id=b.org_id AND r.id=b.role_id WHERE b.org_id=$1 AND r.key IN('project_change_reader','project_change_implementer')`, f.org).Scan(&bindings); e != nil || bindings != 0 {
		t.Fatal("new roles must not auto-bind", bindings, e)
	}
	for _, role := range []string{"project_change_implementer", "request_manager"} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key=$4`, f.org, f.alice, pid, role)
	}
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key='approval_reviewer'`, f.org, f.bob, pid)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	for _, currency := range []string{"HKD", "USD"} {
		r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "project-change-" + strings.ToLower(currency), Name: "Synthetic independent project change", Kind: "change", Category: "project-control", Currency: currency, MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Verify exact proposal and baseline", AssignedUserID: f.bob}}, Reason: "isolated project change rule"}, nil)
		expect(t, r, 201, v)
	}
	getProject := func(t *testing.T) project.Project {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+pid, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Project](t, v)
	}
	p := getProject(t)
	r, v := f.request("POST", "/v1/projects/"+pid+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: "activate", Reason: "exercise live controlled project"}, nil)
	expect(t, r, 200, v)
	get := func(t *testing.T, id string) project.Change {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/changes/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Change](t, v)
	}
	basis := func(t *testing.T, currency string) project.ChangeBasis {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+pid+"/change-basis?currency="+currency, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.ChangeBasis](t, v)
	}
	input := func(t *testing.T, currency string, lines []project.BudgetLine) project.ChangeCreate {
		b := basis(t, currency)
		return project.ChangeCreate{BaseProjectVersion: b.ProjectVersion, BaseBudgetID: b.BudgetID, Reason: "complete exact synthetic proposal", Content: project.ChangeContent{Title: "Synthetic controlled project change", Category: "project-control", Currency: currency, Scope: b.Scope + "\nExplicit reviewed addition.", ScopeDelta: "Add the agreed measurable scope.", ScheduleDelta: "Keep the fixed project dates.", StartDate: b.StartDate, DueDate: b.DueDate, BudgetLines: lines, FileVersionIDs: []string{old.Version.ID}}}
	}
	create := func(t *testing.T, currency string, lines []project.BudgetLine) project.Change {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/changes", f.aliceToken, core.NewID(), input(t, currency, lines), nil)
		expect(t, r, 201, v)
		return get(t, data[project.ChangeReceipt](t, v).ID)
	}
	budgets := func(t *testing.T, state, currency string) []project.ProjectBudget {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+pid+"/budgets?state="+state+"&currency="+currency, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[project.ProjectBudget]](t, v).Items
	}
	overview := func(t *testing.T) project.Overview {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+pid+"/overview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Overview](t, v)
	}
	prepare := func(t *testing.T, d project.Change) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.ChangeSubmit{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.Revision.ContentHash, Reason: "prepare exact independent approval"}, nil)
		expect(t, r, 201, v)
		return data[workflow.Request](t, v)
	}
	submit := func(t *testing.T, q workflow.Request) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		pre := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: pre.Workflow.ID, PreviewHash: pre.PreviewHash, Reason: "send exact complete proposal"}, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	decide := func(t *testing.T, q workflow.Request, decision string) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: decision, Reason: "independent verification of the exact fixed proposal"}, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	approve := func(t *testing.T, d project.Change) (project.Change, workflow.Request) {
		q := decide(t, submit(t, prepare(t, d)), "approve")
		return get(t, d.ID), q
	}
	implementation := func(d project.Change, q workflow.Request) project.ChangeImplement {
		return project.ChangeImplement{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.Revision.ContentHash, RequestRevisionID: q.CurrentRevisionID, BaseProjectVersion: d.Revision.Basis.ProjectVersion, ScopeVerified: true, BudgetVerified: true, Reason: "explicit human implementation of reviewed scope and budget"}
	}
	apply := func(t *testing.T, d project.Change, q workflow.Request) project.Change {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), implementation(d, q), nil)
		expect(t, r, 200, v)
		return get(t, d.ID)
	}
	line := func(category, amount string) project.BudgetLine {
		return project.BudgetLine{Category: category, PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31", Amount: amount}
	}
	t.Run("proposal pins complete baseline ordered old files and an inactive budget", func(t *testing.T) {
		before := getProject(t)
		d := create(t, "HKD", []project.BudgetLine{line("equipment", "100")})
		if d.Revision.Basis.ProjectVersion != before.Version || d.Revision.ApprovalAmount != "100.000000" || d.Revision.BudgetDelta != "100.000000" || d.Revision.Attachments[0].VersionID != old.Version.ID || d.Revision.Attachments[0].VersionID == current.Version.ID || d.Revision.TargetBudgetID == nil || !d.CanEdit || !d.CanPrepareRequest || d.CanImplement {
			t.Fatal(d)
		}
		if getProject(t).Version != before.Version || len(budgets(t, "approved", "HKD")) != 0 {
			t.Fatal("draft changed live project or budget")
		}
	})
	t.Run("approval alone leaves project and budget unchanged", func(t *testing.T) {
		before := getProject(t)
		d, q := approve(t, create(t, "HKD", []project.BudgetLine{line("equipment", "100")}))
		if d.ApprovalState != "approved" || d.Lifecycle != "requested" || !d.CanImplement || d.CanEdit || getProject(t).Version != before.Version || len(budgets(t, "approved", "HKD")) != 0 || q.Payload.ProjectChangeBasis == nil {
			t.Fatal(d, q)
		}
		if len(overview(t).Budgets) != 0 {
			t.Fatal("overview counted an unimplemented approval as live budget")
		}
		d = apply(t, d, q)
		p := getProject(t)
		bs := budgets(t, "approved", "HKD")
		if d.State != "implemented" || d.Lifecycle != "effective" || d.Implementation == nil || d.Implementation.ActorID != f.alice || p.Version != before.Version+1 || p.Scope != d.Revision.Content.Scope || len(bs) != 1 || bs[0].Total != "100.000000" {
			t.Fatal(d, p, bs)
		}
		o := overview(t)
		if len(o.Budgets) != 1 || o.Budgets[0].BudgetID != bs[0].ID || o.Budgets[0].Revision != bs[0].Revision || o.Budgets[0].Amount != "100.000000" || o.Budgets[0].Currency != "HKD" {
			t.Fatal("overview lost exact implemented budget", o.Budgets)
		}
	})
	t.Run("decreases do not hide increases in a different category", func(t *testing.T) {
		d := create(t, "HKD", []project.BudgetLine{line("equipment", "60"), line("service", "40")})
		if d.Revision.BudgetDelta != "0.000000" || d.Revision.ApprovalAmount != "40.000000" {
			t.Fatal(d.Revision)
		}
		oldID := *d.Revision.Basis.BudgetID
		d, q := approve(t, d)
		apply(t, d, q)
		bs := budgets(t, "superseded", "HKD")
		if len(bs) != 1 || bs[0].ID != oldID || bs[0].Total != "100.000000" {
			t.Fatal(bs)
		}
		current := budgets(t, "approved", "HKD")
		if len(current) != 1 || current[0].Revision <= bs[0].Revision || current[0].Revision <= 1 {
			t.Fatal("expected a real later budget revision, including inactive proposal history", current)
		}
		if o := overview(t); len(o.Budgets) != 1 || o.Budgets[0].BudgetID != current[0].ID || o.Budgets[0].Amount != "100.000000" {
			t.Fatal("overview double counted superseded or draft budget revisions", o.Budgets)
		}
		r, v := f.with(t).request("GET", "/v1/projects/"+pid+"/closeout", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[project.CloseoutPreview](t, v)
		seen := map[string]int64{}
		for _, group := range preview.Groups {
			if group.Kind == "budgets" {
				for _, item := range group.Items {
					seen[item.ID] = item.Version
				}
			}
		}
		if seen[bs[0].ID] != bs[0].Revision || seen[current[0].ID] != current[0].Revision {
			t.Fatal("closeout references must agree with actual budget history", seen, bs, current)
		}
	})
	t.Run("null budget preserves current lines while empty budget explicitly clears them", func(t *testing.T) {
		b := basis(t, "HKD")
		d := create(t, "HKD", nil)
		if d.Revision.TargetBudgetID != nil || d.Revision.Content.BudgetLines != nil || d.Revision.ApprovalAmount != "0.000000" {
			t.Fatal(d)
		}
		d, q := approve(t, d)
		apply(t, d, q)
		after := basis(t, "HKD")
		if after.BudgetID == nil || *after.BudgetID != *b.BudgetID || after.BudgetTotal != b.BudgetTotal {
			t.Fatal(after, b)
		}
		d = create(t, "HKD", []project.BudgetLine{})
		if d.Revision.TargetBudgetID == nil || d.Revision.BudgetDelta != "-100.000000" {
			t.Fatal(d)
		}
		d, q = approve(t, d)
		apply(t, d, q)
		after = basis(t, "HKD")
		if after.BudgetTotal != "0.000000" || len(after.BudgetLines) != 0 || *after.BudgetID == *b.BudgetID {
			t.Fatal(after)
		}
		if o := overview(t); len(o.Budgets) != 1 || o.Budgets[0].BudgetID != *after.BudgetID || o.Budgets[0].Amount != "0" {
			t.Fatal("explicit zero budget was hidden or replaced", o.Budgets)
		}
	})
	t.Run("currencies remain independent and implementation never creates actual costs", func(t *testing.T) {
		hkd := basis(t, "HKD")
		var before, after int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org).Scan(&before); err != nil {
			t.Fatal(err)
		}
		d, q := approve(t, create(t, "USD", []project.BudgetLine{line("service", "20")}))
		apply(t, d, q)
		hkdAfter := basis(t, "HKD")
		if *hkdAfter.BudgetID != *hkd.BudgetID || hkdAfter.BudgetTotal != hkd.BudgetTotal || basis(t, "USD").BudgetTotal != "20.000000" {
			t.Fatal("cross currency budget changed")
		}
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM actual_cost_entries WHERE org_id=$1", f.org).Scan(&after); e != nil || after != before {
			t.Fatal("implementation fabricated actual costs", after, before, e)
		}
	})
	t.Run("stale approved proposal cannot inherit implementation authority", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", nil))
		before := getProject(t)
		r, v := f.with(t).request("PATCH", "/v1/projects/"+pid, f.aliceToken, core.NewID(), map[string]any{"expected_version": before.Version, "name": "Changed live project baseline"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), implementation(d, q), nil)
		expect(t, r, 409, v)
		if get(t, d.ID).Lifecycle != "requested" || get(t, d.ID).CanImplement {
			t.Fatal("stale approved proposal was implemented")
		}
	})
	t.Run("stale draft must be revised before submitting its fixed request", func(t *testing.T) {
		d := create(t, "HKD", nil)
		q := prepare(t, d)
		exec("UPDATE projects SET version=version+1 WHERE org_id=$1 AND id=$2", f.org, pid)
		r, v := f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 409, v)
	})
	t.Run("draft revision preserves immutable history and binds pagination", func(t *testing.T) {
		d := create(t, "HKD", nil)
		oldRev := d.CurrentRevisionID
		in := input(t, "HKD", []project.BudgetLine{line("service", "3")})
		in.Content.FileVersionIDs = []string{current.Version.ID}
		r, v := f.with(t).request("PATCH", "/v1/changes/"+d.ID, f.aliceToken, core.NewID(), project.ChangePatch{ChangeCreate: in, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID}, nil)
		expect(t, r, 200, v)
		d = get(t, d.ID)
		if d.Revision.Seq != 2 || *d.Revision.BaseRevisionID != oldRev {
			t.Fatal(d)
		}
		r, v = f.with(t).request("GET", "/v1/changes/"+d.ID+"/revisions/"+oldRev, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		hist := data[project.ChangeRevision](t, v)
		if hist.Attachments[0].VersionID != old.Version.ID || hist.TargetBudgetID != nil {
			t.Fatal(hist)
		}
		r, v = f.with(t).request("GET", "/v1/changes/"+d.ID+"/revisions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[project.ChangeRevision]](t, v)
		if page.NextCursor == nil {
			t.Fatal(page)
		}
		other := create(t, "HKD", nil)
		r, v = f.with(t).request("GET", "/v1/changes/"+other.ID+"/revisions?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
	})
	t.Run("returned request can only resubmit the explicitly updated business revision", func(t *testing.T) {
		d := create(t, "HKD", nil)
		q := decide(t, submit(t, prepare(t, d)), "return")
		d = get(t, d.ID)
		in := input(t, "HKD", nil)
		in.Content.ScopeDelta = "Address the independent reviewer comments."
		r, v := f.with(t).request("PATCH", "/v1/changes/"+d.ID, f.aliceToken, core.NewID(), project.ChangePatch{ChangeCreate: in, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID}, nil)
		expect(t, r, 200, v)
		d = get(t, d.ID)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/revisions", f.aliceToken, core.NewID(), workflow.RequestRevise{ExpectedVersion: q.Version, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.Revision.ContentHash, Reason: "explicit new revision after return"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		decide(t, submit(t, q), "approve")
	})
	t.Run("ordinary live-project edit cannot bypass controlled scope", func(t *testing.T) {
		p := getProject(t)
		scope := p.Scope + " bypass"
		r, v := f.with(t).request("PATCH", "/v1/projects/"+pid, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "scope": scope}, nil)
		expect(t, r, 409, v)
		_, e := f.owner.Exec(ctx, "UPDATE projects SET scope=scope||' SQL bypass',version=version+1 WHERE org_id=$1 AND id=$2", f.org, pid)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal(e)
		}
	})
	t.Run("exact versions hash request revision and explicit confirmations are mandatory", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", nil))
		in := implementation(d, q)
		in.BudgetVerified = false
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
		in = implementation(d, q)
		in.ContentHash = strings.Repeat("a", 64)
		r, v = f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in = implementation(d, q)
		in.RequestRevisionID = core.NewID()
		r, v = f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
	})
	t.Run("implementation checks actual recent MFA and unavailable fixed files", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", nil))
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), implementation(d, q), nil)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		expect(t, r, 403, v)
		exec("UPDATE files SET state='trashed' WHERE org_id=$1 AND id=$2", f.org, old.File.ID)
		r, v = f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), implementation(d, q), nil)
		exec("UPDATE files SET state='active' WHERE org_id=$1 AND id=$2", f.org, old.File.ID)
		expect(t, r, 404, v)
	})
	t.Run("budget validation rejects overlaps excessive precision duplicate files and missing exact keys", func(t *testing.T) {
		for _, mode := range []string{"overlap", "precision", "duplicate", "missing"} {
			in := input(t, "HKD", []project.BudgetLine{line("same", "1")})
			switch mode {
			case "overlap":
				in.Content.BudgetLines = append(in.Content.BudgetLines, line("same", "2"))
			case "precision":
				in.Content.BudgetLines[0].Amount = "1.001"
			case "duplicate":
				in.Content.FileVersionIDs = append(in.Content.FileVersionIDs, old.Version.ID)
			}
			var body any = in
			if mode == "missing" {
				raw, _ := json.Marshal(in)
				var o map[string]any
				json.Unmarshal(raw, &o)
				delete(o["content"].(map[string]any), "budget_lines")
				body = o
			}
			r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/changes", f.aliceToken, core.NewID(), body, nil)
			expect(t, r, 422, v)
		}
	})
	t.Run("a draft without fixed evidence cannot pass submission preview", func(t *testing.T) {
		in := input(t, "HKD", nil)
		in.Content.FileVersionIDs = []string{}
		r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/changes", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d := get(t, data[project.ChangeReceipt](t, v).ID)
		q := prepare(t, d)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 422, v)
	})
	t.Run("SQL implementation has an exact valid control and independently rejects invented MFA", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", []project.BudgetLine{line("control", "10")}))
		insert := `INSERT INTO project_change_implementations(id,org_id,change_id,revision_id,request_revision_id,project_id,before_project_version,after_project_version,change_version,actor_id,session_id,mfa_verified_at,scope_verified,budget_verified,reason) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,s.id,NULL,true,true,'valid SQL control' FROM sessions s WHERE s.org_id=$2 AND s.user_id=$10 AND s.revoked_at IS NULL AND s.mfa_verified_at IS NOT NULL ORDER BY s.created_at DESC LIMIT 1`
		args := []any{core.NewID(), f.org, d.ID, d.CurrentRevisionID, q.CurrentRevisionID, pid, d.Revision.Basis.ProjectVersion, d.Revision.Basis.ProjectVersion + 1, d.Version + 1, f.alice}
		_, e := f.owner.Exec(ctx, insert, args...)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("missing actual MFA was not independently denied", e)
		}
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		insert = strings.Replace(insert, "s.id,NULL,true", "s.id,s.mfa_verified_at,true", 1)
		if _, e = tx.Exec(ctx, insert, args...); e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); e != nil {
			t.Fatal(e)
		}
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		if get(t, d.ID).Lifecycle != "requested" {
			t.Fatal("SQL control rollback leaked")
		}
	})
	t.Run("audit failure rolls back project budget head implementation and operation result", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", []project.BudgetLine{line("audit", "6")}))
		before := getProject(t)
		exec(`CREATE FUNCTION reject_project_change_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='change.implemented' THEN RAISE EXCEPTION 'synthetic change audit refusal'; END IF;RETURN NEW;END;$$;CREATE TRIGGER synthetic_project_change_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_project_change_audit()`)
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, key, implementation(d, q), nil)
		exec("DROP TRIGGER synthetic_project_change_audit ON audit_events;DROP FUNCTION reject_project_change_audit()")
		expect(t, r, 500, v)
		if getProject(t).Version != before.Version || get(t, d.ID).Version != d.Version || get(t, d.ID).Implementation != nil {
			t.Fatal("partial implementation survived audit failure")
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND idempotency_key=$2", f.org, key).Scan(&n); e != nil || n != 0 {
			t.Fatal(n, e)
		}
	})
	t.Run("two concurrent manual implementations commit one project and budget", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", []project.BudgetLine{line("concurrent", "8")}))
		before := getProject(t)
		codes := make(chan int, 2)
		var group sync.WaitGroup
		for i := 0; i < 2; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				r, _ := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, core.NewID(), implementation(d, q), nil)
				codes <- r.Code
			}()
		}
		group.Wait()
		close(codes)
		counts := map[int]int{}
		for c := range codes {
			counts[c]++
		}
		if counts[200] != 1 || counts[409] != 1 || getProject(t).Version != before.Version+1 {
			t.Fatal(counts)
		}
	})
	t.Run("idempotent implementation rechecks current budget authority and actual session", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", nil))
		key := core.NewID()
		in := implementation(d, q)
		for i := 0; i < 2; i++ {
			r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, key, in, nil)
			expect(t, r, 200, v)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		r, v := f.with(t).request("POST", "/v1/changes/"+d.ID+"/implement", f.aliceToken, key, in, nil)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		expect(t, r, 403, v)
	})
	t.Run("immutable proposals budgets and implemented heads cannot be rewritten", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", []project.BudgetLine{line("immutable", "7")}))
		d = apply(t, d, q)
		for _, sql := range []string{"UPDATE project_change_revisions SET reason='rewrite' WHERE org_id=$1 AND change_id=$2", "UPDATE project_budgets SET state='draft' WHERE org_id=$1 AND change_id=$2", "UPDATE change_requests SET lifecycle='draft',version=version+1 WHERE org_id=$1 AND id=$2"} {
			if _, e := f.owner.Exec(ctx, sql, f.org, d.ID); e == nil {
				t.Fatal("immutable source was changed", sql)
			}
		}
	})
	t.Run("approval lists filter budget authority before pagination and direct reads", func(t *testing.T) {
		d := create(t, "HKD", nil)
		q := submit(t, prepare(t, d))
		role, binding := core.NewID(), core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'synthetic_change_only','Synthetic change without budget')", role, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'change.read')", f.org, role)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) VALUES($1,$2,$3,$4,'project',$5,'project_restricted')", binding, f.org, f.bob, role, pid)
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.bob)
		defer exec("UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.bob)
		defer exec("DELETE FROM role_bindings WHERE org_id=$1 AND id=$2", f.org, binding)
		r, v := f.with(t).request("GET", "/v1/requests/"+q.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("GET", "/v1/requests?kind=change&limit=1", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		requests := data[core.Page[workflow.Request]](t, v)
		if len(requests.Items) != 0 || requests.NextCursor != nil {
			t.Fatal("hidden budget leaked a request or cursor", requests)
		}
		r, v = f.with(t).request("GET", "/v1/approval-tasks?limit=1", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		tasks := data[core.Page[workflow.ApprovalStep]](t, v)
		if len(tasks.Items) != 0 || tasks.NextCursor != nil {
			t.Fatal("hidden budget leaked an approval task", tasks)
		}
	})
	t.Run("SQL revision rejects missing proposal keys before creating budget or file references", func(t *testing.T) {
		d := create(t, "HKD", nil)
		_, e := f.owner.Exec(ctx, `INSERT INTO project_change_revisions(id,org_id,change_id,seq,base_revision_id,payload,basis,content_hash,approval_amount,budget_delta,author_id,reason) SELECT $1,org_id,change_id,seq+1,id,payload-'budget_lines',basis,content_hash,0,0,author_id,'incomplete SQL proposal' FROM project_change_revisions WHERE org_id=$2 AND id=$3`, core.NewID(), f.org, d.CurrentRevisionID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("missing exact proposal key was not independently rejected", e)
		}
		if get(t, d.ID).Revision.Seq != 1 {
			t.Fatal("invalid revision changed current proposal")
		}
	})
	t.Run("revision cannot relabel an existing change into another currency", func(t *testing.T) {
		d := create(t, "HKD", []project.BudgetLine{line("currency", "1")})
		in := input(t, "USD", []project.BudgetLine{line("currency", "1")})
		r, v := f.with(t).request("PATCH", "/v1/changes/"+d.ID, f.aliceToken, core.NewID(), project.ChangePatch{ChangeCreate: in, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID}, nil)
		expect(t, r, 422, v)
		if get(t, d.ID).Revision.Content.Currency != "HKD" {
			t.Fatal("currency relabel changed the draft")
		}
	})
	t.Run("automated actors cannot assert human implementation", func(t *testing.T) {
		d, q := approve(t, create(t, "HKD", nil))
		a := core.Actor{OrgID: f.org, UserID: f.alice, Channel: "android", RunID: core.NewID()}
		if e := f.owner.QueryRow(ctx, "SELECT u.auth_version,o.authorization_version FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.org_id=$1 AND u.id=$2", f.org, f.alice).Scan(&a.AuthVersion, &a.OrgAuthVersion); e != nil {
			t.Fatal(e)
		}
		in := implementation(d, q)
		raw, _ := json.Marshal(in)
		_, e := f.api.Projects.ImplementChange(ctx, core.Command{Actor: a, Key: core.NewID(), RequestID: core.NewID(), Route: "POST /v1/changes/{id}/implement", Body: raw}, d.ID, in)
		if e == nil || (core.PublicError(e).Status != 403 && core.PublicError(e).Status != 404) {
			t.Fatal(e)
		}
	})
}
