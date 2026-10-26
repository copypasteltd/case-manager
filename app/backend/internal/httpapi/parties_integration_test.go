package httpapi

import (
	"context"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
)

func TestBusinessParties(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	project := f.createProject("Party scope", []string{f.bob, f.member})
	in := party.Create{Content: party.Content{Name: "Isolated supplier", Kind: "organization", Relationships: []string{"supplier"}, ContactName: "Test contact", Email: "test@example.invalid", Notes: "synthetic business fixture"}, ProjectID: &project.ID, Classification: "project_restricted", Reason: "isolated creation"}
	var p party.Party
	var original string
	var patch party.Patch
	var updateKey string
	bind := func(user, scope string) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
        SELECT $1,$2,$3,id,$4,'project_restricted' FROM roles WHERE org_id=$2 AND key='party_manager'`, core.NewID(), f.org, user, scope); err != nil {
			t.Fatal(err)
		}
	}
	get := func() party.Party {
		t.Helper()
		rec, res := f.request("GET", "/v1/parties/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[party.Party](t, res)
	}
	t.Run("business roles are unbound and administrators have no implicit content rights", func(t *testing.T) {
		rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 404, res)
		bind(f.alice, "org")
		bind(f.bob, "own")
		id := core.NewID()
		if _, err := f.owner.Exec(ctx, `INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'party_reader_fixture','Isolated reader');
            `, id, f.org); err != nil {
			t.Fatal(err)
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'party.read')", f.org, id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')", core.NewID(), f.org, f.member, id); err != nil {
			t.Fatal(err)
		}
		key := core.NewID()
		rec, res = f.request("POST", "/v1/parties", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
		p = data[party.Party](t, res)
		original = p.CurrentRevisionID
		rec, res = f.request("POST", "/v1/parties", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
		if data[party.Party](t, res).ID != p.ID {
			t.Fatal("duplicate party")
		}
		rec, res = f.request("GET", "/v1/parties/"+p.ID, f.administratorToken(), "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/parties/"+p.ID, f.memberToken, "", nil, nil)
		expect(t, rec, 200, res)
		view := data[party.Party](t, res)
		if view.CanEdit || view.CanArchive || view.CanReadHistory {
			t.Fatal(view)
		}
		rec, res = f.request("GET", "/v1/parties/"+p.ID+"/revisions", f.memberToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/parties/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("revisions preserve exact previous contacts and reject stale bases", func(t *testing.T) {
		patch = party.Patch{ExpectedVersion: p.Version, BaseRevisionID: p.CurrentRevisionID, Content: p.Content, Reason: "contact revised"}
		patch.Name = "Revised supplier"
		patch.Email = "new@example.invalid"
		updateKey = core.NewID()
		rec, res := f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, updateKey, patch, nil)
		expect(t, rec, 200, res)
		p = data[party.Party](t, res)
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, updateKey, patch, nil)
		expect(t, rec, 200, res)
		rec, res = f.request("GET", "/v1/parties/"+p.ID+"/revisions/"+original, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		r := data[party.Revision](t, res)
		if r.Name != in.Name || r.Email != in.Email || r.Seq != 1 || r.BaseRevisionID != nil {
			t.Fatal(r)
		}
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 409, res)
		for _, sql := range []string{"UPDATE party_revisions SET name='tampered' WHERE id=$1", "DELETE FROM party_revisions WHERE id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, original); err == nil {
				t.Fatal("mutable revision")
			}
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE parties SET creator_id=$2 WHERE id=$1", p.ID, f.bob); err == nil {
			t.Fatal("creator replaced")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE parties SET classification='internal' WHERE id=$1", p.ID); err == nil {
			t.Fatal("classification changed by ordinary mutation")
		}
	})
	t.Run("concurrent edits have one successor to the fixed revision", func(t *testing.T) {
		input := party.Patch{ExpectedVersion: p.Version, BaseRevisionID: p.CurrentRevisionID, Content: p.Content, Reason: "concurrent revision"}
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for _, name := range []string{"First contender", "Second contender"} {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				copy := input
				copy.Name = name
				rec, _ := f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, core.NewID(), copy, nil)
				codes <- rec.Code
			}(name)
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
		p = get()
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM party_revisions WHERE party_id=$1", p.ID).Scan(&count); err != nil || count != 3 {
			t.Fatal(count, err)
		}
	})
	t.Run("archive preserves the revision and restore does not invent a content edit", func(t *testing.T) {
		revision := p.CurrentRevisionID
		rec, res := f.request("POST", "/v1/parties/"+p.ID+"/state", f.aliceToken, core.NewID(), party.StateInput{ExpectedVersion: p.Version, Action: "archive", Reason: "stop new business references"}, nil)
		expect(t, rec, 200, res)
		p = data[party.Party](t, res)
		if p.State != "archived" || p.CanEdit || p.CurrentRevisionID != revision {
			t.Fatal(p)
		}
		input := party.Patch{ExpectedVersion: p.Version, BaseRevisionID: revision, Content: p.Content, Reason: "must remain read only"}
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, res)
		rec, res = f.request("GET", "/v1/parties?state=active", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[party.Party]](t, res).Items) != 0 {
			t.Fatal("archive in active picker")
		}
		rec, res = f.request("GET", "/v1/parties?state=archived", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[party.Party]](t, res).Items) != 1 {
			t.Fatal("archive missing")
		}
		rec, res = f.request("POST", "/v1/parties/"+p.ID+"/state", f.aliceToken, core.NewID(), party.StateInput{ExpectedVersion: p.Version, Action: "restore", Reason: "business relationship resumed"}, nil)
		expect(t, rec, 200, res)
		p = data[party.Party](t, res)
		if p.State != "active" || !p.CanEdit || p.CurrentRevisionID != revision {
			t.Fatal(p)
		}
	})
	t.Run("audit failure rolls back create edit and archive without partial history", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, `CREATE FUNCTION fail_party_audit() RETURNS trigger LANGUAGE plpgsql AS $x$
            BEGIN IF NEW.action LIKE 'party.%' THEN RAISE EXCEPTION 'isolated party failure'; END IF; RETURN NEW; END $x$;
            CREATE TRIGGER fail_party_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_party_audit()`); err != nil {
			t.Fatal(err)
		}
		rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 500, res)
		input := party.Patch{ExpectedVersion: p.Version, BaseRevisionID: p.CurrentRevisionID, Content: p.Content, Reason: "audit retry"}
		input.Name = "Audit retry supplier"
		key := core.NewID()
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, key, input, nil)
		expect(t, rec, 500, res)
		rec, res = f.request("POST", "/v1/parties/"+p.ID+"/state", f.aliceToken, core.NewID(), party.StateInput{ExpectedVersion: p.Version, Action: "archive", Reason: "must roll back"}, nil)
		expect(t, rec, 500, res)
		actual := get()
		if actual.Version != p.Version || actual.CurrentRevisionID != p.CurrentRevisionID || actual.State != "active" {
			t.Fatal(actual)
		}
		var parties, revisions int
		if err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM parties),(SELECT count(*) FROM party_revisions)").Scan(&parties, &revisions); err != nil || parties != 1 || revisions != 3 {
			t.Fatal(parties, revisions, err)
		}
		if _, err := f.owner.Exec(ctx, "DROP TRIGGER fail_party_audit ON audit_events; DROP FUNCTION fail_party_audit()"); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		p = data[party.Party](t, res)
	})
	t.Run("project security own scope and literal search are applied before paging", func(t *testing.T) {
		weak := in
		weak.Classification = "internal"
		rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), weak, nil)
		expect(t, rec, 422, res)
		own := in
		own.Name = "Bob-owned % supplier"
		rec, res = f.request("POST", "/v1/parties", f.bobToken, core.NewID(), own, nil)
		expect(t, rec, 201, res)
		b := data[party.Party](t, res)
		if b.CreatorID != f.bob {
			t.Fatal(b)
		}
		rec, res = f.request("GET", "/v1/parties?q=%25&relationship=supplier", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[party.Party]](t, res)
		if len(page.Items) != 1 || page.Items[0].ID != b.ID {
			t.Fatal("search treated percent as wildcard", page)
		}
		rec, res = f.request("GET", "/v1/parties?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page = data[core.Page[party.Party]](t, res)
		if len(page.Items) != 1 || page.Items[0].ID != b.ID || page.NextCursor != nil {
			t.Fatal("own scope leaked", page)
		}
		archived := archiveProjectFixture(t, f, project.ID)
		input := party.Patch{ExpectedVersion: p.Version, BaseRevisionID: p.CurrentRevisionID, Content: p.Content, Reason: "archived project"}
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "PROJECT_READ_ONLY" {
			t.Fatal(res.Error)
		}
		if get().CanEdit {
			t.Fatal("read only project edit capability")
		}
		rec, res = f.request("POST", "/v1/projects/"+project.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": archived.Version, "action": "reopen", "reason": "Continue isolated party history checks after actual project archive"}, nil)
		expect(t, rec, 200, res)
	})
	t.Run("history cursors bind identity and revoked authority blocks old successful replay", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/parties/"+p.ID+"/revisions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		history := data[core.Page[party.Revision]](t, res)
		if history.NextCursor == nil {
			t.Fatal(history)
		}
		bind(f.bob, "org")
		rec, res = f.request("GET", "/v1/parties/"+p.ID+"/revisions?cursor="+*history.NextCursor, f.bobToken, "", nil, nil)
		expect(t, rec, 400, res)
		if _, err := f.owner.Exec(ctx, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='party_manager')", f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("PATCH", "/v1/parties/"+p.ID, f.aliceToken, updateKey, patch, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/parties/"+p.ID+"/revisions/"+original, f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
}
