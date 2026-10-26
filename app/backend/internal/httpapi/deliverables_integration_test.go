package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProjectDeliverables(t *testing.T) {
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
	if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,u.id,r.id,'project',$4,'project_restricted' FROM users u JOIN roles r ON r.org_id=u.org_id AND r.key='deliverable_reviewer' WHERE u.org_id=$1 AND u.id IN($2,$3)`, f.org, f.alice, f.bob, pid); err != nil {
		t.Fatal(err)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	exec := func(t *testing.T, q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	get := func(t *testing.T, id string) project.Deliverable {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/deliverables/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Deliverable](t, v)
	}
	create := func(t *testing.T, milestone *string) project.Deliverable {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/deliverables", f.aliceToken, core.NewID(), project.DeliverableCreate{Name: "Synthetic fixed deliverable", AcceptanceCriteria: "Read the complete fixed bytes and verify the agreed result.", OwnerID: f.alice, MilestoneID: milestone, Reason: "isolated deliverable"}, nil)
		expect(t, r, 201, v)
		return get(t, data[project.DeliverableReceipt](t, v).ID)
	}
	revision := func(t *testing.T, d project.Deliverable, rid string) project.DeliverableRevision {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/deliverables/"+d.ID+"/revisions/"+rid, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.DeliverableRevision](t, v)
	}
	submit := func(t *testing.T, d project.Deliverable, file string) (project.Deliverable, project.DeliverableRevision) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: d.Version, ExpectedRevisionID: d.CurrentRevisionID, FileVersionIDs: []string{file}, Description: "Synthetic complete result for the stated criteria", Reason: "fixed human submission"}, nil)
		expect(t, r, 200, v)
		d = get(t, d.ID)
		return d, revision(t, d, *d.CurrentRevisionID)
	}
	input := func(d project.Deliverable, r project.DeliverableRevision, decision string) project.DeliverableDecisionInput {
		return project.DeliverableDecisionInput{ExpectedVersion: d.Version, RevisionID: r.ID, ContentHash: r.ContentHash, Decision: decision, FilesVerified: true, CriteriaVerified: true, Reason: "independent human verification against the exact fixed files and criteria"}
	}
	decide := func(t *testing.T, d project.Deliverable, r project.DeliverableRevision, decision string) project.Deliverable {
		t.Helper()
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), input(d, r, decision), nil)
		expect(t, rec, 200, v)
		return get(t, d.ID)
	}
	t.Run("creation is a real draft with immutable creator and explicit capabilities", func(t *testing.T) {
		d := create(t, nil)
		if d.State != "draft" || d.CurrentRevisionID != nil || d.CreatorID == nil || *d.CreatorID != f.alice || !d.CanEdit || !d.CanSubmit || d.CanDecide {
			t.Fatal(d)
		}
	})
	t.Run("submission pins an old formal version without following the current file pointer", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		if d.State != "submitted" || d.CanEdit || d.CanSubmit || !d.CanDecide || r.Files[0].VersionID != old.Version.ID || r.Files[0].VersionID == current.Version.ID || r.ContentHash == "" || r.Decision != nil {
			t.Fatal(d, r)
		}
		var refs int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='deliverable' AND owner_id=$2 AND version_id=$3", f.org, r.ID, old.Version.ID).Scan(&refs); e != nil || refs != 1 {
			t.Fatal(refs, e)
		}
	})
	t.Run("return and revised criteria produce a new submission while preserving the original decision", func(t *testing.T) {
		d, r1 := submit(t, create(t, nil), old.Version.ID)
		d = decide(t, d, r1, "request_changes")
		patch := project.DeliverablePatch{ExpectedVersion: d.Version, DeliverableCreate: project.DeliverableCreate{Name: d.Name, AcceptanceCriteria: "Revised measurable result with a complete replacement file.", OwnerID: f.alice, Reason: "address precise review comments"}}
		rec, v := f.with(t).request("PATCH", "/v1/deliverables/"+d.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 200, v)
		d = get(t, d.ID)
		d, r2 := submit(t, d, current.Version.ID)
		r1 = revision(t, d, r1.ID)
		if r1.Decision == nil || r1.Decision.Decision != "request_changes" || r1.AcceptanceCriteria == r2.AcceptanceCriteria || r1.Files[0].VersionID != old.Version.ID || r2.Seq != 2 {
			t.Fatal(r1, r2)
		}
	})
	t.Run("acceptance is independent and requires two explicit verifications", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		in := input(d, r, "accept")
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 403, v)
		in.FilesVerified = false
		rec, v = f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), in, nil)
		expect(t, rec, 422, v)
		d = decide(t, d, r, "accept")
		r = revision(t, d, r.ID)
		if d.State != "accepted" || r.Decision == nil || r.Decision.DecidedBy != f.bob {
			t.Fatal(d, r)
		}
	})
	t.Run("accepted history survives an explicitly submitted new revision", func(t *testing.T) {
		d, r1 := submit(t, create(t, nil), old.Version.ID)
		d = decide(t, d, r1, "accept")
		d, r2 := submit(t, d, current.Version.ID)
		r1 = revision(t, d, r1.ID)
		if d.State != "submitted" || r1.Decision == nil || r1.Decision.Decision != "accept" || r2.Decision != nil || r1.ID == r2.ID {
			t.Fatal(d, r1, r2)
		}
	})
	t.Run("exact hash and version reject a stale decision without replacing its basis", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		in := input(d, r, "accept")
		in.ContentHash = strings.Repeat("a", 64)
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), in, nil)
		expect(t, rec, 409, v)
		if get(t, d.ID).Version != d.Version {
			t.Fatal("stale decision changed the head")
		}
	})
	t.Run("two concurrent decisions commit only one immutable outcome", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		codes := make(chan int, 2)
		var group sync.WaitGroup
		for i := 0; i < 2; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				rec, _ := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), input(d, r, "accept"), nil)
				codes <- rec.Code
			}()
		}
		group.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
	})
	t.Run("recent MFA is checked on actual current sessions", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '20 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		defer exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), input(d, r, "accept"), nil)
		expect(t, rec, 403, v)
	})
	t.Run("file state changes prevent acceptance and historical partial disclosure", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		exec(t, "UPDATE files SET state='trashed' WHERE org_id=$1 AND id=$2", f.org, old.File.ID)
		defer exec(t, "UPDATE files SET state='active' WHERE org_id=$1 AND id=$2", f.org, old.File.ID)
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), input(d, r, "accept"), nil)
		expect(t, rec, 404, v)
		rec, v = f.with(t).request("GET", "/v1/deliverables/"+d.ID+"/revisions/"+r.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, v)
	})
	t.Run("database rejects a fake accepted head and immutable revision rewrites", func(t *testing.T) {
		d := create(t, nil)
		_, e := f.owner.Exec(ctx, "UPDATE deliverables SET state='accepted',version=version+1 WHERE org_id=$1 AND id=$2", f.org, d.ID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal(e)
		}
		d, r := submit(t, d, old.Version.ID)
		_, e = f.owner.Exec(ctx, "UPDATE deliverable_revisions SET description='rewrite' WHERE org_id=$1 AND id=$2", f.org, r.ID)
		if e == nil {
			t.Fatal("revision was mutable")
		}
	})
	t.Run("achieved milestones freeze their accepted deliverables until reopening", func(t *testing.T) {
		rec, v := f.with(t).request("POST", "/v1/projects/"+pid+"/milestones", f.aliceToken, core.NewID(), project.MilestoneCreate{Title: "Synthetic accepted delivery milestone", OwnerID: f.alice, DueAt: time.Now().Add(time.Hour)}, nil)
		expect(t, rec, 201, v)
		m := data[project.Milestone](t, v)
		d, r := submit(t, create(t, &m.ID), old.Version.ID)
		d = decide(t, d, r, "accept")
		rec, v = f.with(t).request("POST", "/v1/milestones/"+m.ID+"/transition", f.aliceToken, core.NewID(), project.Transition{ExpectedVersion: m.Version, Action: "achieve", Reason: "accepted fixed delivery"}, nil)
		expect(t, rec, 200, v)
		rec, v = f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: d.Version, ExpectedRevisionID: d.CurrentRevisionID, FileVersionIDs: []string{current.Version.ID}, Description: "new result", Reason: "must first reopen milestone"}, nil)
		expect(t, rec, 409, v)
	})
	t.Run("audit failure rolls back submission and all file references", func(t *testing.T) {
		d := create(t, nil)
		var referencesBefore int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='deliverable'", f.org).Scan(&referencesBefore); err != nil {
			t.Fatal(err)
		}
		exec(t, `CREATE FUNCTION reject_deliverable_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='deliverable.submitted' THEN RAISE EXCEPTION 'synthetic deliverable audit refusal'; END IF; RETURN NEW; END; $$; CREATE TRIGGER synthetic_deliverable_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_deliverable_audit()`)
		defer exec(t, "DROP TRIGGER synthetic_deliverable_audit ON audit_events; DROP FUNCTION reject_deliverable_audit()")
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: d.Version, FileVersionIDs: []string{old.Version.ID}, Description: "atomic fixed submission", Reason: "audit must commit"}, nil)
		expect(t, rec, 500, v)
		if get(t, d.ID).Version != d.Version {
			t.Fatal("partial head survived")
		}
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM deliverable_revisions WHERE org_id=$1 AND deliverable_id=$2", f.org, d.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal(count, e)
		}
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='deliverable'", f.org).Scan(&count); err != nil || count != referencesBefore {
			t.Fatal("partial references survived", count, referencesBefore, err)
		}
	})
	t.Run("expired business authority is checked before an idempotent replay", func(t *testing.T) {
		d := create(t, nil)
		in := project.DeliverableSubmit{ExpectedVersion: d.Version, FileVersionIDs: []string{old.Version.ID}, Description: "explicit replay test", Reason: "fixed request identity"}
		key := core.NewID()
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, key, in, nil)
		expect(t, rec, 200, v)
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
		defer exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
		rec, v = f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, key, in, nil)
		expect(t, rec, 404, v)
	})
	t.Run("viewer membership prevents acceptance despite an organization manager role", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		exec(t, "UPDATE project_members SET project_role='viewer' WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, pid, f.bob)
		defer exec(t, "UPDATE project_members SET project_role='member' WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, pid, f.bob)
		rec, v := f.with(t).request("GET", "/v1/deliverables/"+d.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 200, v)
		if data[project.Deliverable](t, v).CanDecide {
			t.Fatal("viewer gained a write capability")
		}
		rec, v = f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), input(d, r, "accept"), nil)
		expect(t, rec, 404, v)
	})
	t.Run("read only project phase blocks fresh submission", func(t *testing.T) {
		d := create(t, nil)
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT state FROM projects WHERE org_id=$1 AND id=$2", f.org, pid).Scan(&state); e != nil {
			t.Fatal(e)
		}
		exec(t, "UPDATE projects SET state='cancelled' WHERE org_id=$1 AND id=$2", f.org, pid)
		defer exec(t, "UPDATE projects SET state=$3 WHERE org_id=$1 AND id=$2", f.org, pid, state)
		rec, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: d.Version, FileVersionIDs: []string{old.Version.ID}, Description: "cannot bypass project phase", Reason: "phase boundary"}, nil)
		expect(t, rec, 409, v)
	})
	t.Run("automatic actors cannot record a human acceptance", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		a := core.Actor{OrgID: f.org, UserID: f.bob, Channel: "android", RunID: core.NewID()}
		if e := f.owner.QueryRow(ctx, "SELECT u.auth_version,o.authorization_version FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.org_id=$1 AND u.id=$2", f.org, f.bob).Scan(&a.AuthVersion, &a.OrgAuthVersion); e != nil {
			t.Fatal(e)
		}
		_, e := f.api.Projects.DecideDeliverable(ctx, core.Command{Actor: a, Key: core.NewID(), RequestID: core.NewID(), Route: "POST /v1/deliverables/{id}/decisions", Body: []byte(`{"decision":"accept"}`)}, d.ID, input(d, r, "accept"))
		if e == nil || (core.PublicError(e).Status != 403 && core.PublicError(e).Status != 404) {
			t.Fatal("automatic acceptance did not fail with the explicit human authorization boundary", e)
		}
	})
	t.Run("revision pagination binds its exact deliverable", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		d = decide(t, d, r, "request_changes")
		_, _ = submit(t, d, current.Version.ID)
		rec, v := f.with(t).request("GET", "/v1/deliverables/"+d.ID+"/revisions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, v)
		page := data[core.Page[project.DeliverableRevision]](t, v)
		if page.NextCursor == nil || len(page.Items) != 1 {
			t.Fatal(page)
		}
		other := create(t, nil)
		rec, v = f.with(t).request("GET", "/v1/deliverables/"+other.ID+"/revisions?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, rec, 400, v)
	})
	t.Run("SQL acceptance has a valid control and rejects missing actual MFA", func(t *testing.T) {
		d, r := submit(t, create(t, nil), old.Version.ID)
		insert := `INSERT INTO acceptance_decisions(id,org_id,revision_id,decided_by,decision,reason,protocol_version,session_id,mfa_verified_at,revision_hash,head_version,files_verified,criteria_verified)
          SELECT $1,$2,$3,$4,'accept','independent SQL control',1,s.id,NULL,$5,$6,true,true FROM sessions s WHERE s.org_id=$2 AND s.user_id=$4 AND s.revoked_at IS NULL AND s.mfa_verified_at IS NOT NULL ORDER BY s.created_at DESC LIMIT 1`
		_, e := f.owner.Exec(ctx, insert, core.NewID(), f.org, r.ID, f.bob, r.ContentHash, d.Version+1)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("missing actual MFA was not independently denied", e)
		}
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		insert = strings.Replace(insert, "s.id,NULL,$5", "s.id,s.mfa_verified_at,$5", 1)
		if _, e = tx.Exec(ctx, insert, core.NewID(), f.org, r.ID, f.bob, r.ContentHash, d.Version+1); e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, "UPDATE deliverables SET state='accepted',version=version+1 WHERE org_id=$1 AND id=$2", f.org, d.ID); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if get(t, d.ID).State != "accepted" {
			t.Fatal("valid SQL control did not complete")
		}
	})
}
