package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActualCosts(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	p := func(s string) *string { return &s }
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	admin := f.administratorToken()
	input := func(category, amount string) finance.Create {
		return finance.Create{Content: finance.Content{Title: "Synthetic canonical actual cost", OccurredOn: p("2025-01-15"), Currency: p("HKD"), Amount: p(amount), Category: category, Allocations: []finance.Allocation{{ProjectID: original.File.ProjectID, Amount: amount}}, EvidenceVersionIDs: []string{original.Version.ID}}, SourceKey: "manual:" + core.NewID(), ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated native cost proof"}
	}
	base := func(c finance.Cost) finance.Base {
		return finance.Base{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, Reason: "human checks exact cost snapshot"}
	}
	get := func(t *testing.T, id string) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/costs/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	create := func(t *testing.T, in finance.Create) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[finance.Cost](t, v)
	}
	confirm := func(t *testing.T, c finance.Cost) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	update := func(t *testing.T, c finance.Cost, content finance.Content) finance.Cost {
		t.Helper()
		r, v := f.with(t).request("PATCH", "/v1/costs/"+c.ID, f.aliceToken, core.NewID(), finance.Patch{Base: base(c), Content: content}, nil)
		expect(t, r, 200, v)
		return data[finance.Cost](t, v)
	}
	report := func(t *testing.T, category, from, to, extra string) finance.Report {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/reports/expenses?kind=incurred&category="+category+"&from="+from+"&to="+to+extra, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Report](t, v)
	}
	var recorder string
	var supplier party.Party
	t.Run("new financial roles are unbound and source identity survives replay cache expiry", func(t *testing.T) {
		in := input("identity", "100")
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 404, v)
		recorder = bind(f.alice, "cost_recorder")
		for _, key := range []string{"cost_confirmer", "cost_reverser", "finance_reporter", "party_manager", "purchase_manager", "claim_submitter", "request_manager", "contract_manager", "contract_operator"} {
			bind(f.alice, key)
		}
		f.aliceToken = memberMFAToken(f, "alice", "android")
		bind(f.bob, "cost_reader")
		bind(f.bob, "approval_reviewer")
		bind(f.bob, "purchase_reader")
		bind(f.bob, "claim_reader")
		r, v = f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic actual-cost supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated supplier"}, nil)
		expect(t, r, 201, v)
		supplier = data[party.Party](t, v)
		in.Content = finance.Content{Title: "Incomplete but persistent draft", Allocations: []finance.Allocation{}, EvidenceVersionIDs: []string{}}
		key := core.NewID()
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		c := data[finance.Cost](t, v)
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		if data[finance.Cost](t, v).ID != c.ID {
			t.Fatal("duplicate replay")
		}
		changed := in
		changed.Title = "Different input"
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, key, changed, nil)
		expect(t, r, 409, v)
		exec(t, "DELETE FROM idempotency_records WHERE idempotency_key=$1", key)
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID, admin, "", nil, nil)
		expect(t, r, 404, v)
	})
	fullInput := func(category, amount string) finance.Create {
		in := input(category, amount)
		in.Counterparty = &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}
		return in
	}
	t.Run("draft revisions preserve exact history and stale bases cannot overwrite", func(t *testing.T) {
		c := create(t, fullInput("revisions", "100"))
		old := c
		content := c.Content
		content.Title = "Revised canonical cost"
		c = update(t, c, content)
		r, v := f.with(t).request("GET", "/v1/costs/"+c.ID+"/revisions/"+old.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.Revision](t, v).Title != old.Title {
			t.Fatal("history changed")
		}
		r, v = f.with(t).request("PATCH", "/v1/costs/"+c.ID, f.aliceToken, core.NewID(), finance.Patch{Base: base(old), Content: content}, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID+"/revisions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[finance.Revision]](t, v)
		if page.NextCursor == nil || len(page.Items) != 1 {
			t.Fatal(page)
		}
		other := create(t, fullInput("other-history", "1"))
		r, v = f.with(t).request("GET", "/v1/costs/"+other.ID+"/revisions?cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
	})
	t.Run("confirmation requires actual balanced facts currency precision and fixed evidence", func(t *testing.T) {
		in := fullInput("balanced", "100")
		in.Allocations[0].Amount = "99"
		c := create(t, in)
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
		expect(t, r, 422, v)
		content := c.Content
		content.Allocations[0].Amount = "100"
		c = update(t, c, content)
		c = confirm(t, c)
		if c.State != "confirmed" || c.CanEdit || c.BaseCurrency != nil || c.BaseAmount != nil || len(c.Attachments) != 1 || c.Attachments[0].VersionID != original.Version.ID {
			t.Fatal(c)
		}
		r, v = f.with(t).request("PATCH", "/v1/costs/"+c.ID, f.aliceToken, core.NewID(), finance.Patch{Base: base(c), Content: c.Content}, nil)
		expect(t, r, 409, v)
		for _, amount := range []string{"0.001", "-1", "1e3"} {
			in = fullInput("bad-precision", amount)
			r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, core.NewID(), in, nil)
			expect(t, r, 422, v)
		}
		in = fullInput("zero", "0")
		in.Allocations = []finance.Allocation{}
		zero := create(t, in)
		r, v = f.with(t).request("POST", "/v1/costs/"+zero.ID+"/confirm", f.aliceToken, core.NewID(), base(zero), nil)
		expect(t, r, 422, v)
	})
	t.Run("confirmation rejects future dates and a file whose available bytes lost evidence", func(t *testing.T) {
		in := fullInput("future", "1")
		in.OccurredOn = p("2099-01-01")
		c := create(t, in)
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
		expect(t, r, 422, v)
		c = create(t, fullInput("lost-evidence", "1"))
		exec(t, "UPDATE file_versions SET availability='missing' WHERE id=$1", original.Version.ID)
		defer exec(t, "UPDATE file_versions SET availability='available' WHERE id=$1", original.Version.ID)
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
		if r.Code != 409 && r.Code != 422 {
			t.Fatal(r.Code, v)
		}
	})
	t.Run("all allocation projects fence detail lists history and report totals", func(t *testing.T) {
		second := f.with(t).createProject("Other financial allocation", []string{})
		in := fullInput("two-projects", "100")
		in.Allocations = []finance.Allocation{{ProjectID: original.File.ProjectID, Amount: "60"}, {ProjectID: &second.ID, Amount: "40"}}
		c := confirm(t, create(t, in))
		r, v := f.with(t).request("GET", "/v1/costs/"+c.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("GET", "/v1/costs?q=Synthetic&category=two-projects", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.Cost]](t, v).Items) != 0 {
			t.Fatal("hidden allocation in list")
		}
		exec(t, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'member')", f.org, second.ID, f.bob)
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[finance.Cost](t, v).CounterpartyEvidence != nil {
			t.Fatal("supplier read inherited from cost")
		}
		result := report(t, "two-projects", "2025-01-01", "2025-02-01", "&project_id="+second.ID)
		if len(result.Totals) != 1 || result.Totals[0].NetAmount != "40.000000" {
			t.Fatal(result)
		}
		r, v = f.with(t).request("GET", "/v1/costs?state=incurred&category=two-projects&from=2025-01-01&to=2025-02-01&project_id="+second.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		drill := data[core.Page[finance.Cost]](t, v)
		if len(drill.Items) != 1 || drill.Items[0].ID != c.ID {
			t.Fatal("report allocation drilldown", drill)
		}
		exec(t, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, second.ID, f.bob)
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID+"/revisions", f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
	})
	t.Run("full reversal preserves original month and inherits company and project shares", func(t *testing.T) {
		in := fullInput("month-correction", "1000")
		in.Allocations = []finance.Allocation{{ProjectID: original.File.ProjectID, Amount: "600"}, {Amount: "400"}}
		c := confirm(t, create(t, in))
		old := c
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: base(c), EffectiveOn: "2025-02-10"}, nil)
		expect(t, r, 200, v)
		c = data[finance.Cost](t, v)
		if c.State != "reversed" || c.ReversalID == nil || c.CanReverse {
			t.Fatal(c)
		}
		reversal := get(t, *c.ReversalID)
		if reversal.EntryKind != "reversal" || reversal.ReversesID == nil || *reversal.ReversesID != c.ID || *reversal.Amount != *old.Amount || reversal.Allocations[1].Amount != "400.000000" || reversal.CanReverse {
			t.Fatal(reversal)
		}
		corrected := fullInput("month-correction", "1200")
		corrected.OccurredOn = p("2025-02-11")
		corrected.Allocations = []finance.Allocation{{ProjectID: original.File.ProjectID, Amount: "720"}, {Amount: "480"}}
		confirm(t, create(t, corrected))
		jan := report(t, "month-correction", "2025-01-01", "2025-02-01", "")
		feb := report(t, "month-correction", "2025-02-01", "2025-03-01", "")
		all := report(t, "month-correction", "2025-01-01", "2025-03-01", "")
		company := report(t, "month-correction", "2025-02-01", "2025-03-01", "&company_only=true")
		if jan.Totals[0].NetAmount != "1000.000000" || feb.Totals[0].NetAmount != "200.000000" || all.Totals[0].NetAmount != "1200.000000" || company.Totals[0].NetAmount != "80.000000" {
			t.Fatal(jan, feb, all, company)
		}
		r, v = f.with(t).request("POST", "/v1/costs/"+reversal.ID+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: base(reversal), EffectiveOn: "2025-02-12"}, nil)
		expect(t, r, 409, v)
	})
	t.Run("currencies remain separate and unconfirmed costs do not enter incurred totals", func(t *testing.T) {
		confirm(t, create(t, fullInput("separate-currency", "10")))
		usd := fullInput("separate-currency", "20")
		usd.Currency = p("USD")
		confirm(t, create(t, usd))
		create(t, fullInput("separate-currency", "99"))
		r := report(t, "separate-currency", "2025-01-01", "2025-02-01", "")
		if len(r.Totals) != 2 || r.UnconfirmedCount != 1 || r.BaseCurrency != nil || r.Totals[0].Currency != "HKD" || r.Totals[1].Currency != "USD" {
			t.Fatal(r)
		}
		rec, v := f.with(t).request("GET", "/v1/reports/expenses?kind=cash", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, v)
		rec, v = f.with(t).request("GET", "/v1/reports/expenses", f.bobToken, "", nil, nil)
		expect(t, rec, 404, v)
	})
	t.Run("simultaneous confirmation and reversal have at most one successor", func(t *testing.T) {
		c := create(t, fullInput("concurrency", "100"))
		for _, path := range []string{"confirm", "reverse"} {
			var body any = base(c)
			if path == "reverse" {
				body = finance.Reverse{Base: base(c), EffectiveOn: "2025-02-10"}
			}
			var wg sync.WaitGroup
			codes := make(chan int, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r, _ := f.with(t).request("POST", "/v1/costs/"+c.ID+"/"+path, f.aliceToken, core.NewID(), body, nil)
					codes <- r.Code
				}()
			}
			wg.Wait()
			close(codes)
			success := 0
			for code := range codes {
				if code == 200 {
					success++
				} else if code != 409 {
					t.Fatal(code)
				}
			}
			if success != 1 {
				t.Fatal(success)
			}
			c = get(t, c.ID)
		}
	})
	t.Run("current database MFA and manual identity are checked before cached confirmation", func(t *testing.T) {
		c := create(t, fullInput("mfa-replay", "1"))
		in := base(c)
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, key, in, nil)
		expect(t, r, 200, v)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		actual := actor.MFAVerifiedAt
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		defer exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		raw, _ := json.Marshal(in)
		svc := finance.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		cmd := core.Command{Actor: actor, Key: key, RequestID: core.NewID(), Route: "POST /v1/costs/" + c.ID + "/confirm", Body: raw}
		_, err = svc.Confirm(ctx, cmd, c.ID, in)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		cmd.Actor.RunID = core.NewID()
		_, err = svc.Confirm(ctx, cmd, c.ID, in)
		if status := core.PublicError(err).Status; status != 403 && status != 404 {
			t.Fatal(err)
		}
	})
	t.Run("fixed cost evidence protects retention and response reference counts", func(t *testing.T) {
		c := create(t, fullInput("retention-cost", "1"))
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='cost_revision' AND owner_id=$1 AND version_id=$2", c.CurrentRevisionID, original.Version.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
		bind(f.alice, "file_retention_manager")
		r, v := f.with(t).request("GET", "/v1/files/"+original.File.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		file := data[storage.File](t, v)
		r, v = f.with(t).request("POST", "/v1/file-versions/"+original.Version.ID+"/retention-assessments", f.aliceToken, core.NewID(), map[string]any{"expected_file_version": file.Version, "reason": "fixed cost reference proof"}, nil)
		expect(t, r, 201, v)
	})
	t.Run("audit failure rolls back revision head event and the replay receipt", func(t *testing.T) {
		c := create(t, fullInput("audit", "1"))
		exec(t, `CREATE FUNCTION fail_cost_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='cost.confirmed' THEN RAISE EXCEPTION 'isolated cost audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_cost_confirmation BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_cost_confirmation()`)
		defer exec(t, "DROP TRIGGER IF EXISTS fail_cost_confirmation ON audit_events; DROP FUNCTION IF EXISTS fail_cost_confirmation()")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, key, base(c), nil)
		expect(t, r, 500, v)
		if now := get(t, c.ID); now.Version != c.Version || now.State != "draft" {
			t.Fatal(now)
		}
		exec(t, "DROP TRIGGER fail_cost_confirmation ON audit_events; DROP FUNCTION fail_cost_confirmation()")
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, key, base(c), nil)
		expect(t, r, 200, v)
	})
	t.Run("database rejects immutable rewrites detached head and unbalanced confirmation", func(t *testing.T) {
		c := create(t, fullInput("database-guards", "100"))
		for _, sql := range []string{"UPDATE cost_revisions SET reason='rewrite' WHERE id=$1", "DELETE FROM cost_revisions WHERE id=$1", "UPDATE cost_allocations SET amount=1 WHERE revision_id=$1", "DELETE FROM cost_revision_files WHERE revision_id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, c.CurrentRevisionID); err == nil {
				t.Fatal("mutable fact", sql)
			}
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE actual_cost_entries SET version=version+1,state='confirmed' WHERE id=$1", c.ID); err == nil {
			t.Fatal("detached confirmation accepted")
		}
		in := fullInput("bad-sum-sql", "100")
		in.Allocations[0].Amount = "99"
		c = create(t, in)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "UPDATE actual_cost_entries SET version=2,state='confirmed' WHERE id=$1", c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO cost_events(id,org_id,cost_id,cost_version,revision_id,kind,state,actor_id,session_id,mfa_verified_at,reason) SELECT $1,$2,$3,2,$4,'confirmed','confirmed',$5,s.id,s.mfa_verified_at,'independent SQL guard' FROM sessions s WHERE s.id=$6`, core.NewID(), f.org, c.ID, c.CurrentRevisionID, f.alice, actor.SessionID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil || !strings.Contains(err.Error(), "complete balanced actual facts") {
			t.Fatal("SQL sum guard", err)
		}
	})
	t.Run("database rejects forged MFA and another humans session before deferred journal checks", func(t *testing.T) {
		c := create(t, fullInput("session-sql", "1"))
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		for _, attempt := range []struct {
			actor  string
			offset time.Duration
		}{{f.alice, time.Second}, {f.bob, 0}} {
			tx, err := f.api.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO cost_events(id,org_id,cost_id,cost_version,revision_id,kind,state,actor_id,session_id,mfa_verified_at,reason)
			SELECT $1,$2,$3,2,$4,'confirmed','confirmed',$5,s.id,s.mfa_verified_at+($7::bigint*interval '1 microsecond'),'forged session rejected' FROM sessions s WHERE s.id=$6`, core.NewID(), f.org, c.ID, c.CurrentRevisionID, attempt.actor, actor.SessionID, attempt.offset.Microseconds())
			tx.Rollback(ctx)
			if err == nil || !strings.Contains(err.Error(), "actual active human session") {
				t.Fatal("forged financial session", err)
			}
		}
	})
	t.Run("project read only phase fences actual cost mutations", func(t *testing.T) {
		c := create(t, fullInput("project-fence", "1"))
		exec(t, "UPDATE projects SET state='cancelled',version=version+1 WHERE id=$1", *original.File.ProjectID)
		defer exec(t, "UPDATE projects SET state='active',version=version+1 WHERE id=$1", *original.File.ProjectID)
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/confirm", f.aliceToken, core.NewID(), base(c), nil)
		expect(t, r, 409, v)
		if get(t, c.ID).CanConfirm {
			t.Fatal("readonly confirmation capability")
		}
	})
	t.Run("revoked recorder cannot replay a previously successful draft creation", func(t *testing.T) {
		in := fullInput("revoked-replay", "1")
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/costs", f.aliceToken, key, in, nil)
		expect(t, r, 201, v)
		exec(t, "DELETE FROM role_bindings WHERE id=$1", recorder)
		r, v = f.with(t).request("POST", "/v1/costs", f.aliceToken, key, in, nil)
		if r.Code != 403 && r.Code != 404 && r.Code != 412 {
			t.Fatal(r.Code, v)
		}
		recorder = bind(f.alice, "cost_recorder")
	})
	bind(f.bob, "party_manager")
	bind(f.bob, "contract_reader")
	type sourceRecord struct {
		ID                string `json:"id"`
		Kind              string `json:"kind"`
		Version           int64  `json:"version"`
		CurrentRevisionID string `json:"current_revision_id"`
		ContentHash       string `json:"content_hash"`
	}
	newSource := func(t *testing.T, kind string, approved bool, amount string) sourceRecord {
		t.Helper()
		key := "cost-source-" + kind
		var exists bool
		if err := f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workflow_definitions WHERE org_id=$1 AND key=$2)", f.org, key).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: key, Name: "Synthetic cost source policy", Kind: kind, Category: "cost-source", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent expense basis", AssignedUserID: f.bob}}, Reason: "isolated exact financial basis"}, nil)
			expect(t, r, 201, v)
		}
		content := spend.Content{Title: "Synthetic financial source", Category: "cost-source", Purpose: "Actual expense reference", OwnerID: &f.alice, Currency: p("HKD"), RoundingMode: "half_up"}
		if kind == "purchase" {
			content.PricingBasis = "net_plus_tax"
			content.ExpectedDeliveryDate = p("2025-01-15")
			content.SupplierCandidates = []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}
			content.QuoteVersionIDs = []string{original.Version.ID}
			content.Lines = []spend.Line{{ID: core.NewID(), Description: "Synthetic service", Quantity: p("1"), Unit: "service", UnitPrice: &amount, TaxAmount: p("0"), DiscountAmount: p("0")}}
		} else {
			content.PricingBasis = "gross_includes_tax"
			content.PeriodStart = p("2025-01-15")
			content.PeriodEnd = p("2025-01-15")
			content.Lines = []spend.Line{{ID: core.NewID(), Description: "Synthetic service", DeclaredAmount: &amount, ExpenseDate: p("2025-01-15"), Category: "cost-source", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{original.Version.ID}, Funding: "employee_advance"}}
		}
		route := "/purchases"
		if kind == "claim" {
			route = "/claims"
		}
		var body any = spend.Create{Content: content, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated source draft"}
		if kind == "contract" {
			route = "/contracts"
			body = contracts.Create{Content: contracts.Content{Title: "Synthetic financial contract", Direction: p("payable"), Type: p("service"), Category: "cost-source", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: p("HKD"), Amount: &amount, StartDate: p("2025-01-15"), EndDate: p("2027-12-31"), DraftFileVersionID: &original.Version.ID}, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated financial contract"}
		}
		r, v := f.with(t).request("POST", "/v1"+route, f.aliceToken, core.NewID(), body, nil)
		expect(t, r, 201, v)
		d := data[sourceRecord](t, v)
		d.Kind = kind
		if !approved {
			return d
		}
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: kind, BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "accurate financial source"}, nil)
		expect(t, r, 201, v)
		q := data[workflow.Request](t, v)
		r, v = f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit exact source"}, nil)
		expect(t, r, 200, v)
		q = data[workflow.Request](t, v)
		r, v = f.with(t).request("POST", "/v1/approval-steps/"+q.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: q.Steps[0].Version, RequestVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, Decision: "approve", Reason: "independent source confirmation"}, nil)
		expect(t, r, 200, v)
		r, v = f.with(t).request("GET", "/v1"+route+"/"+d.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		d = data[sourceRecord](t, v)
		if kind == "contract" {
			r, v = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, SignedOn: "2025-01-15", SignedFileVersionID: original.Version.ID, Attestation: "Exact actual signed fixture evidence", Reason: "record cost source signature"}, nil)
			expect(t, r, 201, v)
			signed := data[contracts.Contract](t, v)
			r, v = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: signed.Version, Action: "activate", RevisionID: signed.CurrentRevisionID, ContentHash: signed.ContentHash, SignatureID: signed.Signature.ID, Reason: "actual reached effective date"}, nil)
			expect(t, r, 200, v)
			d = data[sourceRecord](t, v)
		}
		d.Kind = kind
		return d
	}
	linkInput := func(c finance.Cost, d sourceRecord) finance.LinkInput {
		relation := "incurred_for"
		if d.Kind == "claim" {
			relation = "reimbursed_via"
		}
		if d.Kind == "contract" {
			relation = "contractual_basis"
		}
		return finance.LinkInput{Base: base(c), SourceType: d.Kind, SourceID: d.ID, SourceRevisionID: d.CurrentRevisionID, Relation: relation}
	}
	t.Run("purchase reimbursement and effective contract share one cost without report multiplication", func(t *testing.T) {
		purchase := newSource(t, "purchase", true, "200")
		claim := newSource(t, "claim", true, "200")
		contract := newSource(t, "contract", true, "200")
		c := confirm(t, create(t, fullInput("canonical-links", "100")))
		for _, d := range []sourceRecord{purchase, claim, contract} {
			r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), linkInput(c, d), nil)
			expect(t, r, 200, v)
			c = data[finance.Cost](t, v)
		}
		if len(c.Links) != 3 {
			t.Fatal(c.Links)
		}
		result := report(t, "canonical-links", "2025-01-01", "2025-02-01", "")
		if len(result.Totals) != 1 || result.Totals[0].NetAmount != "100.000000" || result.Totals[0].EntryCount != 1 {
			t.Fatal(result)
		}
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), linkInput(c, purchase), nil)
		expect(t, r, 409, v)
		exec(t, "DELETE FROM role_bindings b USING roles r WHERE b.role_id=r.id AND b.user_id=$1 AND r.key='purchase_reader'", f.bob)
		defer func() { bind(f.bob, "purchase_reader") }()
		r, v = f.with(t).request("GET", "/v1/costs/"+c.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		view := data[finance.Cost](t, v)
		for _, l := range view.Links {
			if l.SourceType == "purchase" && l.Title != nil {
				t.Fatal("source permission inherited")
			}
		}
	})
	t.Run("unapproved wrong currency and stale source revision cannot link", func(t *testing.T) {
		draft := newSource(t, "purchase", false, "200")
		c := confirm(t, create(t, fullInput("bad-source", "100")))
		r, v := f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), linkInput(c, draft), nil)
		expect(t, r, 409, v)
		approved := newSource(t, "purchase", true, "200")
		in := linkInput(c, approved)
		in.SourceRevisionID = core.NewID()
		r, v = f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 404, v)
		usd := fullInput("wrong-currency-source", "10")
		usd.Currency = p("USD")
		other := confirm(t, create(t, usd))
		r, v = f.with(t).request("POST", "/v1/costs/"+other.ID+"/links", f.aliceToken, core.NewID(), linkInput(other, approved), nil)
		expect(t, r, 409, v)
	})
	t.Run("source locking prevents concurrent over-linking and reversal releases its active balance", func(t *testing.T) {
		source := newSource(t, "purchase", true, "500")
		a := confirm(t, create(t, fullInput("source-cap", "300")))
		b := confirm(t, create(t, fullInput("source-cap", "300")))
		codes := make(chan struct {
			id   string
			code int
		}, 2)
		var wg sync.WaitGroup
		for _, c := range []finance.Cost{a, b} {
			wg.Add(1)
			go func(c finance.Cost) {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/costs/"+c.ID+"/links", f.aliceToken, core.NewID(), linkInput(c, source), nil)
				codes <- struct {
					id   string
					code int
				}{c.ID, r.Code}
			}(c)
		}
		wg.Wait()
		close(codes)
		winner, loser := "", ""
		for result := range codes {
			if result.code == 200 {
				winner = result.id
			} else if result.code == 409 {
				loser = result.id
			} else {
				t.Fatal(result)
			}
		}
		if winner == "" || loser == "" {
			t.Fatal("aggregate overspend")
		}
		win := get(t, winner)
		r, v := f.with(t).request("POST", "/v1/costs/"+winner+"/reverse", f.aliceToken, core.NewID(), finance.Reverse{Base: base(win), EffectiveOn: "2025-02-10"}, nil)
		expect(t, r, 200, v)
		remaining := get(t, loser)
		r, v = f.with(t).request("POST", "/v1/costs/"+loser+"/links", f.aliceToken, core.NewID(), linkInput(remaining, source), nil)
		expect(t, r, 200, v)
	})
}
