package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/spend"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func assertProjectCloseoutItem(t *testing.T, f *fixture, pid, kind, id string, want bool) {
	t.Helper()
	var actual bool
	if e := f.owner.QueryRow(context.Background(), "SELECT blocking FROM project_closeout_items($1,$2) WHERE kind=$3 AND id=$4", f.org, pid, kind, id).Scan(&actual); e != nil || actual != want {
		t.Fatal("project closeout disagrees with actual business settlement", kind, id, actual, want, e)
	}
}
func TestProjectCloseoutAndArchive(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	pid := *file.File.ProjectID
	var logs bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(logs.String())
		}
	})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, key, scope string) *string {
		id := core.NewID()
		if scope == "" {
			exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		} else {
			exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT $1,$2,$3,id,'project',$5,'project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key, scope)
		}
		return &id
	}
	get := func(t *testing.T, id string) project.Project {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Project](t, v)
	}
	preview := func(t *testing.T, id string) project.CloseoutPreview {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/projects/"+id+"/closeout", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.CloseoutPreview](t, v)
	}
	transition := func(t *testing.T, id, action string) project.Project {
		t.Helper()
		p := get(t, id)
		r, v := f.with(t).request("POST", "/v1/projects/"+id+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: action, Reason: "explicit synthetic project lifecycle"}, nil)
		expect(t, r, 200, v)
		return data[project.Project](t, v)
	}
	newProject := func(t *testing.T) project.Project {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/projects", f.aliceToken, core.NewID(), project.Create{Name: "Synthetic archive " + core.NewID(), Scope: "Measurable synthetic deliverables", Type: "internal", OwnerID: f.alice, MemberIDs: []string{f.alice, f.bob}, Classification: "project_restricted"}, nil)
		expect(t, r, 201, v)
		return transition(t, data[project.Project](t, v).ID, "activate")
	}
	input := func(t *testing.T, id string) project.ArchiveInput {
		t.Helper()
		p := preview(t, id)
		if p.Fingerprint == nil {
			t.Fatal("complete preview has no exact fingerprint")
		}
		return project.ArchiveInput{ExpectedVersion: p.Project.Version, ExpectedFingerprint: *p.Fingerprint, Summary: "Reviewed outcomes and actual accepted delivery", LessonsLearned: "Documented implementation and review lessons", FollowUp: "No unresolved follow-up remains in this review", BusinessVerified: true, FinancialVerified: true, FileVersionIDs: []string{}, Reason: "human reviews exact complete closeout"}
	}
	group := func(t *testing.T, p project.CloseoutPreview, kind string) project.CloseoutGroup {
		t.Helper()
		for _, g := range p.Groups {
			if g.Kind == kind {
				return g
			}
		}
		t.Fatal("missing checklist category", kind)
		return project.CloseoutGroup{}
	}
	blocked := func(t *testing.T, id, kind string) {
		t.Helper()
		p := get(t, id)
		r, v := f.with(t).request("POST", "/v1/projects/"+id+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: "close", Reason: "must not hide open obligations"}, nil)
		expect(t, r, 422, v)
		g := group(t, preview(t, id), kind)
		if g.OpenCount == nil || *g.OpenCount == 0 {
			t.Fatal("missing actual blocker", kind)
		}
	}
	var archiver string
	t.Run("new roles start unbound and unavailable categories disclose neither counts nor fingerprints", func(t *testing.T) {
		var n int
		if e := f.owner.QueryRow(ctx, `SELECT count(*) FROM role_bindings b JOIN roles r ON r.org_id=b.org_id AND r.id=b.role_id WHERE b.org_id=$1 AND r.key IN('project_closeout_reader','project_archiver')`, f.org).Scan(&n); e != nil || n != 0 {
			t.Fatal(n, e)
		}
		p := preview(t, pid)
		g := group(t, p, "costs")
		if p.Visibility != "restricted" || p.Fingerprint != nil || g.Count != nil || g.OpenCount != nil || len(g.Items) != 0 || p.CanArchive {
			t.Fatal("restricted checklist leaked a balance", p)
		}
		r, v := f.with(t).request("GET", "/v1/projects/"+pid+"/closeout", f.administratorToken(), "", nil, nil)
		expect(t, r, 404, v)
		archiver = *bind(f.alice, "project_archiver", "")
		for _, key := range []string{"cost_recorder", "cost_confirmer", "cost_reverser", "payment_recorder", "payment_reverser", "party_manager", "subscription_manager", "contract_manager", "purchase_manager", "claim_submitter"} {
			bind(f.alice, key, "")
		}
		f.aliceToken = memberMFAToken(f, "alice", "android")
		p = preview(t, pid)
		if p.Visibility != "complete" || p.Fingerprint == nil || len(p.Groups) != 13 {
			t.Fatal("complete archive reader not correctly provisioned", p)
		}
	})
	t.Run("legacy transition cannot archive and direct SQL cannot invent an archive", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: "archive", Reason: "missing retrospective"}, nil)
		expect(t, r, 422, v)
		_, e := f.owner.Exec(ctx, "UPDATE projects SET state='archived',version=version+1 WHERE org_id=$1 AND id=$2", f.org, p.ID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("SQL bypass accepted", e)
		}
	})
	t.Run("open and missed milestones remain explicit archive blockers", func(t *testing.T) {
		p := newProject(t)
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/milestones", f.aliceToken, core.NewID(), project.MilestoneCreate{Title: "Unfinished real milestone", OwnerID: f.alice, DueAt: time.Now().Add(-time.Hour)}, nil)
		expect(t, r, 201, v)
		m := data[project.Milestone](t, v)
		blocked(t, p.ID, "milestones")
		exec("UPDATE milestones SET state='missed',version=version+1 WHERE org_id=$1 AND id=$2", f.org, m.ID)
		blocked(t, p.ID, "milestones")
		exec("UPDATE milestones SET state='cancelled',version=version+1 WHERE org_id=$1 AND id=$2", f.org, m.ID)
		transition(t, p.ID, "close")
	})
	t.Run("stale version stale exact basis and missing independent declarations are denied", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		in := input(t, p.ID)
		in.ExpectedFingerprint = strings.Repeat("0", 32)
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in = input(t, p.ID)
		in.ExpectedVersion--
		r, v = f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		in = input(t, p.ID)
		in.FinancialVerified = false
		r, v = f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 422, v)
	})
	for _, kind := range []string{"contracts", "purchases", "claims"} {
		t.Run("unclosed draft "+kind+" blocks its own project", func(t *testing.T) {
			p := newProject(t)
			var body any = spend.Create{Content: spend.Content{Title: "Unfinished commercial document"}, ProjectID: &p.ID, Classification: "project_restricted", Reason: "preserve unfinished source"}
			if kind == "contracts" {
				body = contracts.Create{Content: contracts.Content{Title: "Unfinished legal contract"}, ProjectID: &p.ID, Classification: "project_restricted", Reason: "preserve unfinished legal work"}
			}
			r, v := f.with(t).request("POST", "/v1/"+kind, f.aliceToken, core.NewID(), body, nil)
			expect(t, r, 201, v)
			blocked(t, p.ID, kind)
		})
	}
	t.Run("SQL cannot invent legacy acceptance as a substitute for real evidence", func(t *testing.T) {
		p := newProject(t)
		_, e := f.owner.Exec(ctx, "INSERT INTO deliverables(id,org_id,project_id,name,state,acceptance_criteria,owner_id) VALUES($1,$2,$3,'Legacy delivery','accepted','Old unverified acceptance',$4)", core.NewID(), f.org, p.ID, f.alice)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("invented legacy acceptance was accepted", e)
		}
	})
	t.Run("actual current MFA is required before a review can commit", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		in := input(t, p.ID)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), in, nil)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		expect(t, r, 403, v)
		if get(t, p.ID).State != "closing" {
			t.Fatal("stale MFA changed state")
		}
	})
	t.Run("SQL review accepts an exact control and rejects invented MFA or missing atomic state", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		statement := `INSERT INTO project_archives(id,org_id,project_id,project_version,basis,fingerprint,summary,lessons_learned,follow_up,business_verified,financial_verified,evidence_count,actor_id,session_id,mfa_verified_at,reason)
 SELECT $1,$2,$3,$4,b,md5(b::text),'Exact SQL control','Actual lessons','No follow-up',true,true,0,$5,s.id,s.mfa_verified_at,'exact control' FROM sessions s CROSS JOIN LATERAL project_closeout_snapshot($2,$3)b WHERE s.org_id=$2 AND s.user_id=$5 AND s.device_type='android' AND s.revoked_at IS NULL ORDER BY s.created_at DESC LIMIT 1`
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, statement, core.NewID(), f.org, p.ID, p.Version+1, f.alice)
		if e != nil {
			t.Fatal("valid SQL control refused", e)
		}
		tx.Rollback(ctx)
		_, e = f.owner.Exec(ctx, strings.Replace(statement, "s.id,s.mfa_verified_at", "s.id,NULL", 1), core.NewID(), f.org, p.ID, p.Version+1, f.alice)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("SQL fabricated MFA accepted", e)
		}
		_, e = f.owner.Exec(ctx, statement, core.NewID(), f.org, p.ID, p.Version+1, f.alice)
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("review committed without archived state", e)
		}
	})
	t.Run("audit failure rolls back review state evidence audit outbox and operation result", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		in := input(t, p.ID)
		key := core.NewID()
		exec(`CREATE FUNCTION reject_project_archive_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='project.archived' THEN RAISE EXCEPTION 'synthetic archive audit refusal'; END IF;RETURN NEW;END;$$;CREATE TRIGGER synthetic_project_archive_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_project_archive_audit()`)
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, key, in, nil)
		exec("DROP TRIGGER synthetic_project_archive_audit ON audit_events;DROP FUNCTION reject_project_archive_audit()")
		expect(t, r, 500, v)
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM project_archives WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal(n, e)
		}
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND idempotency_key=$2", f.org, key).Scan(&n); e != nil || n != 0 {
			t.Fatal(n, e)
		}
		if get(t, p.ID).Version != p.Version || get(t, p.ID).State != "closing" {
			t.Fatal("partial archive survived")
		}
	})
	var historyProject, archiveID string
	t.Run("archive and replay commit one exact immutable retrospective", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		historyProject = p.ID
		in := input(t, p.ID)
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		result := data[project.ArchiveReceipt](t, v)
		archiveID = result.Archive.ID
		if result.Project.State != "archived" || result.Project.Version != p.Version+1 || result.Archive.CurrentBasisState != "current" || result.Archive.Summary != in.Summary {
			t.Fatal(result)
		}
		r, v = f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[project.ArchiveReceipt](t, v).Archive.ID != archiveID {
			t.Fatal("replay duplicated review")
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM project_archives WHERE org_id=$1 AND project_id=$2", f.org, p.ID).Scan(&n); e != nil || n != 1 {
			t.Fatal(n, e)
		}
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='project.archived'", f.org, p.ID).Scan(&n); e != nil || n != 1 {
			t.Fatal(n, e)
		}
		_, e := f.owner.Exec(ctx, "UPDATE project_archives SET summary='rewritten history' WHERE org_id=$1 AND id=$2", f.org, archiveID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("mutable archive history", e)
		}
	})
	t.Run("reopening preserves old review and rearchiving creates a new version with bound cursors", func(t *testing.T) {
		p := transition(t, historyProject, "reopen")
		r, v := f.with(t).request("GET", "/v1/projects/"+p.ID+"/archives", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		list := data[project.ArchiveList](t, v)
		if len(list.Items) != 1 || list.Items[0].CurrentBasisState != "changed" {
			t.Fatal(list)
		}
		p = transition(t, p.ID, "close")
		r, v = f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), input(t, p.ID), nil)
		expect(t, r, 201, v)
		r, v = f.with(t).request("GET", "/v1/projects/"+p.ID+"/archives?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		list = data[project.ArchiveList](t, v)
		if len(list.Items) != 1 || list.NextCursor == nil || list.Items[0].CurrentBasisState != "current" {
			t.Fatal(list)
		}
		r, v = f.with(t).request("GET", "/v1/projects/"+p.ID+"/archives?limit=1&cursor="+*list.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		older := data[project.ArchiveList](t, v)
		if len(older.Items) != 1 || older.Items[0].ID != archiveID || older.NextCursor != nil {
			t.Fatal(older)
		}
		other := newProject(t)
		r, v = f.with(t).request("GET", "/v1/projects/"+other.ID+"/archives?cursor="+*list.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
	})
	t.Run("concurrent independent archives commit exactly one version", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		in := input(t, p.ID)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for n := 0; n < 2; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, core.NewID(), in, nil)
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
			t.Fatal(success, conflict)
		}
	})
	ptr := func(s string) *string { return &s }
	var supplier party.Party
	var cost finance.Cost
	var payment finance.Payment
	costBase := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "exact current expense"}
	}
	cashBase := func(c finance.Payment) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "exact recorded cash"}
	}
	t.Run("unconfirmed and unpaid shared actual costs block closure without using approved budgets", func(t *testing.T) {
		p := get(t, pid)
		if p.State == "draft" {
			transition(t, pid, "activate")
		}
		other := newProject(t)
		r, v := f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic archive supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &pid, Classification: "project_restricted", Reason: "exact supplier"}, nil)
		expect(t, r, 201, v)
		supplier = data[party.Party](t, v)
		in := finance.Create{Content: finance.Content{Title: "Shared actual closeout expense", OccurredOn: ptr("2025-01-15"), Currency: ptr("HKD"), Amount: ptr("100"), Category: "closeout", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Allocations: []finance.Allocation{{ProjectID: &pid, Amount: "60"}, {ProjectID: &other.ID, Amount: "40"}}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "archive-cost:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "actual incurred expense"}
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		cost = data[finance.Cost](t, v)
		blocked(t, pid, "costs")
		r, v = f.with(t).request("POST", "/v1/costs/"+cost.ID+"/confirm", f.aliceToken, core.NewID(), costBase(cost), nil)
		expect(t, r, 200, v)
		cost = data[finance.Cost](t, v)
		blocked(t, pid, "costs")
		bind(f.bob, "project_closeout_reader", pid)
		exec(`DELETE FROM role_bindings b USING roles r WHERE b.org_id=$1 AND b.user_id=$2 AND r.org_id=b.org_id AND r.id=b.role_id AND r.key<>'project_closeout_reader'`, f.org, f.bob)
		f.bobToken = f.with(t).login("bob", "android").AccessToken
		r, v = f.with(t).request("GET", "/v1/projects/"+pid+"/closeout", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		limited := data[project.CloseoutPreview](t, v)
		g := group(t, limited, "costs")
		if g.Visibility != "restricted" || g.Count != nil || len(g.Items) > 0 || limited.Fingerprint != nil {
			t.Fatal("shared scope leaked", limited)
		}
	})
	t.Run("actual allocation settles the cost and unattributed refunds independently block closure", func(t *testing.T) {
		in := finance.PaymentCreate{PaymentContent: finance.PaymentContent{Title: "Actual archive outgoing payment", Direction: ptr("outgoing"), PaidAt: ptr("2025-02-01T02:00:00+08:00"), Currency: ptr("HKD"), Amount: ptr("100"), Category: "closeout", MethodLabel: "Synthetic transfer", Counterparty: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, EvidenceVersionIDs: []string{file.Version.ID}}, SourceKey: "archive-cash:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "actual paid cash"}
		r, v := f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		payment = data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+payment.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: cashBase(payment), RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 200, v)
		payment = data[finance.Payment](t, v)
		blocked(t, pid, "costs")
		r, v = f.with(t).request("POST", "/v1/payments/"+payment.ID+"/allocations", f.aliceToken, core.NewID(), finance.PaymentAllocate{Base: cashBase(payment), CostID: cost.ID, CostVersion: cost.Version, CostRevisionID: cost.CurrentRevisionID, CostContentHash: cost.ContentHash, Amount: "100"}, nil)
		expect(t, r, 200, v)
		payment = data[finance.Payment](t, v)
		if g := group(t, preview(t, pid), "costs"); g.OpenCount == nil || *g.OpenCount != 0 {
			t.Fatal(g)
		}
		in.SourceKey = "archive-refund-root:" + core.NewID()
		in.Title = "Separate refundable advance"
		in.Amount = ptr("20")
		r, v = f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		root := data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+root.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: cashBase(root), RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 200, v)
		root = data[finance.Payment](t, v)
		in.SourceKey = "archive-refund:" + core.NewID()
		in.Title = "Unattributed real refund"
		in.Direction = ptr("refund")
		in.Amount = ptr("20")
		in.RefundOfID = &root.ID
		r, v = f.with(t).request("POST", "/v1/payments", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		refund := data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+refund.ID+"/record", f.aliceToken, core.NewID(), finance.PaymentRecordInput{Base: cashBase(refund), ExpectedSourceVersion: &root.Version, RefundAllocations: []finance.RefundAllocation{}}, nil)
		expect(t, r, 200, v)
		refund = data[finance.Payment](t, v)
		blocked(t, pid, "payments")
		r, v = f.with(t).request("POST", "/v1/payments/"+refund.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: cashBase(refund), EffectiveAt: "2025-02-02T02:00:00+08:00"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1/payments/"+root.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		root = data[finance.Payment](t, v)
		r, v = f.with(t).request("POST", "/v1/payments/"+root.ID+"/reverse", f.aliceToken, core.NewID(), finance.PaymentReverseInput{Base: cashBase(root), EffectiveAt: "2025-02-03T02:00:00+08:00"}, nil)
		expect(t, r, 200, v)

	})
	t.Run("settled financial project archives with fixed evidence and cannot append evidence later", func(t *testing.T) {
		p := transition(t, pid, "close")
		rv, vv := f.with(t).request("POST", "/v1/costs/"+cost.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: costBase(cost), EffectiveOn: "2025-02-03"}, nil)
		expect(t, rv, 409, vv)
		in := input(t, p.ID)
		in.FileVersionIDs = []string{file.Version.ID}
		r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/archive", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		a := data[project.ArchiveReceipt](t, v).Archive
		rv, vv = f.with(t).request("POST", "/v1/costs/"+cost.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: costBase(cost), EffectiveOn: "2025-02-03"}, nil)
		expect(t, rv, 409, vv)
		if len(a.Attachments) != 1 || a.Attachments[0].VersionID != file.Version.ID || a.CurrentBasisState != "current" {
			t.Fatal(a)
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='project_archive' AND owner_id=$2 AND version_id=$3", f.org, a.ID, file.Version.ID).Scan(&n); e != nil || n != 1 {
			t.Fatal("missing fixed file reference", n, e)
		}
		_, e := f.owner.Exec(ctx, "INSERT INTO project_archive_files(org_id,archive_id,version_id) SELECT $1,$2,id FROM file_versions WHERE org_id=$1 AND file_id=$3 AND id<>$4 LIMIT 1", f.org, a.ID, file.File.ID, file.Version.ID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("appended evidence to committed review", e)
		}
		_, e = f.api.DB.Exec(ctx, "UPDATE project_archives SET summary='app rewrite' WHERE org_id=$1 AND id=$2", f.org, a.ID)
		if !errors.As(e, &pe) || pe.Code != "42501" {
			t.Fatal("application has immutable-history mutation authority", e)
		}
	})
	t.Run("active and future cancellation subscriptions block even with no cost currently payable", func(t *testing.T) {
		transition(t, pid, "reopen")
		anchor := 31
		in := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic final recurring subscription", Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: "closeout", StartDate: ptr("2025-01-01"), Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: ptr("2025-01-31"), AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: "2025-01-01", Currency: "HKD", Amount: "100", Plan: "Synthetic monthly"}}}, SourceKey: "archive-sub:" + core.NewID(), ProjectID: &pid, Classification: "project_restricted", Reason: "review recurring commitment"}
		r, v := f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		s := data[finance.Subscription](t, v)
		base := func() finance.Base {
			return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "explicit vendor disposition"}
		}
		blocked(t, pid, "subscriptions")
		r, v = f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		s = data[finance.Subscription](t, v)
		blocked(t, pid, "subscriptions")
		future := time.Now().Add(24 * time.Hour).In(time.FixedZone("HK", 8*3600)).Format("2006-01-02")
		r, v = f.with(t).request("POST", "/v1/subscriptions/"+s.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(), Action: "cancel", EffectiveOn: &future, VendorState: "cancelled", VendorCheckedAt: ptr(time.Now().UTC().Format(time.RFC3339)), VendorEvidenceVersionIDs: []string{file.Version.ID}}, nil)
		expect(t, r, 200, v)
		s = data[finance.Subscription](t, v)
		blocked(t, pid, "subscriptions")
	})
	t.Run("revoked source authority denies historical records and idempotent result replay", func(t *testing.T) {
		p := newProject(t)
		p = transition(t, p.ID, "close")
		in := input(t, p.ID)
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND id=$2", f.org, archiver)
		exec(`DELETE FROM role_bindings b WHERE b.org_id=$1 AND b.user_id=$2 AND EXISTS(SELECT 1 FROM role_actions a WHERE a.org_id=b.org_id AND a.role_id=b.role_id AND a.action='payment.read')`, f.org, f.alice)
		r, v = f.with(t).request("GET", "/v1/projects/"+p.ID+"/archives", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		list := data[project.ArchiveList](t, v)
		raw, _ := json.Marshal(list)
		if list.Visibility != "restricted" || len(list.Items) != 0 || list.NextCursor != nil || bytes.Contains(raw, []byte(in.Summary)) {
			t.Fatal("historical result leaked after authority narrowed", list)
		}
		r, v = f.with(t).request("POST", "/v1/projects/"+p.ID+"/archive", f.aliceToken, key, in, nil)
		expect(t, r, 403, v)
	})
}
