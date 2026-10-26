package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
)

func TestAgentConversations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Conversation scope", []string{f.bob, f.member})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(user, scope string) {
		t.Helper()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type)
        SELECT $1,$2,$3,id,$4 FROM roles WHERE org_id=$2 AND key='agent_user'`, core.NewID(), f.org, user, scope)
	}
	create := func(token string, in agent.ConversationCreate) agent.Conversation {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations", token, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		return data[agent.Conversation](t, res)
	}
	get := func(token, id string) agent.Conversation {
		t.Helper()
		rec, res := f.request("GET", "/v1/conversations/"+id, token, "", nil, nil)
		expect(t, rec, 200, res)
		return data[agent.Conversation](t, res)
	}
	change := func(c agent.Conversation, user, action, token string) agent.AccessResult {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/members", token, core.NewID(), agent.MemberChange{ExpectedVersion: c.Version, UserID: user, Action: action, Reason: "isolated membership change"}, nil)
		expect(t, rec, 200, res)
		return data[agent.AccessResult](t, res)
	}
	privateInput := agent.ConversationCreate{Type: "private", Title: "Private % workspace", MemberIDs: []string{}}
	var private, shared agent.Conversation
	var initialShared string
	t.Run("unbound roles and explicit conversation membership protect personal and shared metadata", func(t *testing.T) {
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), privateInput, nil)
		expect(t, rec, 404, res)
		bind(f.alice, "org")
		bind(f.bob, "org")
		bind(f.member, "org")
		key := core.NewID()
		rec, res = f.request("POST", "/v1/conversations", f.aliceToken, key, privateInput, nil)
		expect(t, rec, 201, res)
		private = data[agent.Conversation](t, res)
		rec, res = f.request("POST", "/v1/conversations", f.aliceToken, key, privateInput, nil)
		expect(t, rec, 201, res)
		if data[agent.Conversation](t, res).ID != private.ID || len(private.Members) != 1 || !private.Members[0].Creator || private.CanManageMembers {
			t.Fatal(private)
		}
		shared = create(f.aliceToken, agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Project shared workspace", MemberIDs: []string{}})
		initialShared = shared.CurrentRevisionID
		for _, token := range []string{f.bobToken, f.memberToken, f.administratorToken()} {
			for _, id := range []string{private.ID, shared.ID} {
				rec, res = f.request("GET", "/v1/conversations/"+id, token, "", nil, nil)
				expect(t, rec, 404, res)
			}
		}
		rec, res = f.request("GET", "/v1/conversations", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[agent.Conversation]](t, res).Items) != 0 {
			t.Fatal("org role bypassed explicit membership")
		}
		rec, res = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Invalid sharing", MemberIDs: []string{f.bob}}, nil)
		expect(t, rec, 422, res)
	})
	t.Run("candidate identities are bounded to eligible current project members and adding does not grant business roles", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/conversations/"+shared.ID+"/member-candidates", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		candidates := data[struct {
			Items []agent.Member `json:"items"`
		}](t, res)
		if len(candidates.Items) != 2 {
			t.Fatal(candidates)
		}
		old := shared
		result := change(shared, f.bob, "add", f.aliceToken)
		shared = get(f.aliceToken, shared.ID)
		if result.ContextSecurityVersion != 2 || result.AccessLost || shared.ContextSecurityVersion != 2 || len(shared.Members) != 2 {
			t.Fatal(result, shared)
		}
		get(f.bobToken, shared.ID)
		var oldMembers int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE revision_id=$1", old.CurrentRevisionID).Scan(&oldMembers); err != nil || oldMembers != 1 {
			t.Fatal("old membership was rewritten", oldMembers, err)
		}
		var granted int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM resource_grants WHERE resource_id=$1", shared.ID).Scan(&granted); err != nil || granted != 0 {
			t.Fatal("member operation manufactured business grants")
		}
		rec, res = f.request("POST", "/v1/conversations/"+shared.ID+"/members", f.aliceToken, core.NewID(), agent.MemberChange{ExpectedVersion: shared.Version, UserID: f.alice, Action: "remove", Reason: "must retain creator"}, nil)
		expect(t, rec, 422, res)
	})
	t.Run("rename and archive use immutable complete revisions without changing the security epoch", func(t *testing.T) {
		before := shared
		input := agent.ConversationPatch{ExpectedVersion: shared.Version, Title: "Renamed project workspace", State: "archived", Reason: "isolated archive"}
		key := core.NewID()
		rec, res := f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		shared = data[agent.Conversation](t, res)
		if shared.State != "archived" || shared.ContextSecurityVersion != before.ContextSecurityVersion || shared.CanManageMembers || len(shared.Members) != 2 {
			t.Fatal(shared)
		}
		rec, res = f.request("POST", "/v1/conversations/"+shared.ID+"/members", f.aliceToken, core.NewID(), agent.MemberChange{ExpectedVersion: shared.Version, UserID: f.member, Action: "add", Reason: "archived sharing blocked"}, nil)
		expect(t, rec, 409, res)
		input.ExpectedVersion = shared.Version
		input.State = "active"
		input.Reason = "isolated restore"
		rec, res = f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 200, res)
		shared = data[agent.Conversation](t, res)
		rec, res = f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, key, agent.ConversationPatch{ExpectedVersion: before.Version, Title: "Renamed project workspace", State: "archived", Reason: "isolated archive"}, nil)
		expect(t, rec, 200, res)
		if data[agent.Conversation](t, res).State != "active" {
			t.Fatal("idempotent replay disclosed obsolete state")
		}
		for _, sql := range []string{"UPDATE conversation_revisions SET title='changed' WHERE id=$1", "DELETE FROM conversation_revisions WHERE id=$1", "DELETE FROM conversation_members WHERE revision_id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, before.CurrentRevisionID); err == nil {
				t.Fatal("mutable conversation snapshot")
			}
		}
	})
	t.Run("old snapshots cannot be expanded and scope identity cannot be rewritten", func(t *testing.T) {
		for _, revision := range []string{initialShared, shared.CurrentRevisionID} {
			if _, err := f.owner.Exec(ctx, "INSERT INTO conversation_members(org_id,conversation_id,revision_id,user_id) VALUES($1,$2,$3,$4)", f.org, shared.ID, revision, f.member); err == nil {
				t.Fatal("sealed membership was extended without a new context epoch")
			}
		}
		for _, sql := range []string{"UPDATE conversations SET creator_id=$2 WHERE id=$1", "UPDATE conversations SET project_id=NULL WHERE id=$1", "UPDATE conversations SET classification='internal' WHERE id=$1"} {
			var err error
			if sql == "UPDATE conversations SET creator_id=$2 WHERE id=$1" {
				_, err = f.api.DB.Exec(ctx, sql, shared.ID, f.bob)
			} else {
				_, err = f.api.DB.Exec(ctx, sql, shared.ID)
			}
			if err == nil {
				t.Fatal("conversation scope changed")
			}
		}
	})
	if !t.Run("a waiting editor reads the published revision after acquiring the head lock", func(t *testing.T) {
		before := shared
		waitCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		holder, err := f.owner.Begin(waitCtx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(context.Background())
		var holderPID int
		if err = holder.QueryRow(waitCtx, "SELECT pg_backend_pid() FROM conversations WHERE org_id=$1 AND id=$2 FOR UPDATE", f.org, before.ID).Scan(&holderPID); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			status int
			code   string
		}
		finished := make(chan outcome, 1)
		go func() {
			rec, res := f.request("PATCH", "/v1/conversations/"+before.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: before.Version, Title: "Waiting editor", State: "active", Reason: "deterministic concurrent edit"}, nil)
			code := ""
			if res.Error != nil {
				code = res.Error.Code
			}
			finished <- outcome{rec.Code, code}
		}()
		deadline := time.NewTimer(4 * time.Second)
		defer deadline.Stop()
		for {
			var waiting bool
			if err = f.owner.QueryRow(waitCtx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event_type='Lock'
				AND $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%conversations%')`, holderPID).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			select {
			case early := <-finished:
				t.Fatalf("editor bypassed held head lock: %+v", early)
			case <-deadline.C:
				t.Fatal("editor did not reach held head lock")
			case <-time.After(10 * time.Millisecond):
			}
		}
		// Publish a complete successor in the lock holder, ensuring the waiting
		// statement started before the new immutable revision existed.
		revisionID := core.NewID()
		if _, err = holder.Exec(waitCtx, `INSERT INTO conversation_revisions(id,org_id,conversation_id,seq,base_revision_id,title,state,member_ids,context_security_version,author_id,reason)
			SELECT $1,org_id,conversation_id,seq+1,id,'Published while waiting',state,member_ids,context_security_version,author_id,'isolated lock fixture'
			FROM conversation_revisions WHERE org_id=$2 AND id=$3`, revisionID, f.org, before.CurrentRevisionID); err != nil {
			t.Fatal(err)
		}
		if _, err = holder.Exec(waitCtx, `INSERT INTO conversation_members(org_id,conversation_id,revision_id,user_id)
			SELECT org_id,conversation_id,$1,user_id FROM conversation_members WHERE org_id=$2 AND revision_id=$3`, revisionID, f.org, before.CurrentRevisionID); err != nil {
			t.Fatal(err)
		}
		if _, err = holder.Exec(waitCtx, "UPDATE conversations SET version=version+1,current_revision_id=$3,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, before.ID, revisionID); err != nil {
			t.Fatal(err)
		}
		if err = holder.Commit(waitCtx); err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-finished:
			if result.status != 409 || result.code != "VERSION_CONFLICT" {
				t.Fatalf("waiting editor lost the committed revision: %+v", result)
			}
		case <-waitCtx.Done():
			t.Fatal("editor did not finish after head publication")
		}
		shared = get(f.aliceToken, before.ID)
		if shared.Version != before.Version+1 || shared.CurrentRevisionID != revisionID || shared.Title != "Published while waiting" {
			t.Fatal("losing editor changed the published successor", shared)
		}
	}) {
		t.FailNow()
	}
	t.Run("concurrent metadata edits have one successor and preserve the losing version", func(t *testing.T) {
		before := shared
		var group sync.WaitGroup
		codes := make(chan int, 2)
		for _, title := range []string{"First edit", "Second edit"} {
			group.Add(1)
			go func(title string) {
				defer group.Done()
				rec, _ := f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: before.Version, Title: title, State: "active", Reason: "concurrent edit"}, nil)
				codes <- rec.Code
			}(title)
		}
		group.Wait()
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
			t.Fatal("multiple conversation successors")
		}
		shared = get(f.aliceToken, shared.ID)
		if shared.Version != before.Version+1 || shared.ContextSecurityVersion != before.ContextSecurityVersion {
			t.Fatal(shared)
		}
	})
	t.Run("audit failure rolls back membership context organization epoch and idempotent result together", func(t *testing.T) {
		before := shared
		var orgVersion int64
		if err := f.owner.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", f.org).Scan(&orgVersion); err != nil {
			t.Fatal(err)
		}
		exec(`CREATE FUNCTION fail_conversation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'conversation.%' THEN RAISE EXCEPTION 'isolated conversation audit failure'; END IF; RETURN NEW; END $$;
            CREATE TRIGGER fail_conversation_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_conversation_audit()`)
		input := agent.MemberChange{ExpectedVersion: shared.Version, UserID: f.member, Action: "add", Reason: "atomic member audit"}
		key := core.NewID()
		rec, res := f.request("POST", "/v1/conversations/"+shared.ID+"/members", f.aliceToken, key, input, nil)
		expect(t, rec, 500, res)
		actual := get(f.aliceToken, shared.ID)
		var current int64
		if err := f.owner.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", f.org).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current != orgVersion || actual.CurrentRevisionID != before.CurrentRevisionID || actual.ContextSecurityVersion != before.ContextSecurityVersion {
			t.Fatal("partial membership or epoch commit")
		}
		exec("DROP TRIGGER fail_conversation_audit ON audit_events; DROP FUNCTION fail_conversation_audit()")
		rec, res = f.request("POST", "/v1/conversations/"+shared.ID+"/members", f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		shared = get(f.aliceToken, shared.ID)
	})
	t.Run("removal prevents fresh reads and replay disclosure while invalidating old scoped cursors", func(t *testing.T) {
		other := create(f.bobToken, agent.ConversationCreate{Type: "private", Title: "Bob own personal space", MemberIDs: []string{}})
		rec, res := f.request("GET", "/v1/conversations?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[agent.Conversation]](t, res)
		if page.NextCursor == nil {
			t.Fatal("expected bounded cursor")
		}
		input := agent.ConversationPatch{ExpectedVersion: shared.Version, Title: "Bob shared title", State: "active", Reason: "member edit"}
		key := core.NewID()
		rec, res = f.request("PATCH", "/v1/conversations/"+shared.ID, f.bobToken, key, input, nil)
		expect(t, rec, 200, res)
		shared = data[agent.Conversation](t, res)
		change(shared, f.bob, "remove", f.aliceToken)
		shared = get(f.aliceToken, shared.ID)
		rec, res = f.request("GET", "/v1/conversations/"+shared.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("PATCH", "/v1/conversations/"+shared.ID, f.bobToken, key, input, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/conversations?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, rec, 400, res)
		rec, res = f.request("GET", "/v1/conversations?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page = data[core.Page[agent.Conversation]](t, res)
		if len(page.Items) != 1 || page.Items[0].ID != other.ID || page.NextCursor != nil {
			t.Fatal("removed conversation affected page")
		}
	})
	t.Run("project membership and revoked roles remain independent gates for a stored conversation member", func(t *testing.T) {
		change(shared, f.bob, "add", f.aliceToken)
		shared = get(f.aliceToken, shared.ID)
		exec("DELETE FROM project_members WHERE project_id=$1 AND user_id=$2", p.ID, f.bob)
		rec, res := f.request("GET", "/v1/conversations/"+shared.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
		exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'member')", f.org, p.ID, f.bob)
		get(f.bobToken, shared.ID)
		exec("UPDATE role_bindings rb SET revoked_at=clock_timestamp() FROM roles r WHERE rb.role_id=r.id AND rb.org_id=$1 AND rb.user_id=$2 AND r.key='agent_user'", f.org, f.bob)
		rec, res = f.request("GET", "/v1/conversations/"+shared.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("concurrent readers never combine a prior security epoch with a newer membership snapshot", func(t *testing.T) {
		c := create(f.aliceToken, agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Concurrent membership reads", MemberIDs: []string{}})
		var readers sync.WaitGroup
		problems := make(chan string, 2)
		counts := make(chan int, 2)
		for range 2 {
			readers.Add(1)
			go func() {
				defer readers.Done()
				completed := 0
				defer func() { counts <- completed }()
				for range 40 {
					rec, res := f.request("GET", "/v1/conversations/"+c.ID, f.aliceToken, "", nil, nil)
					if rec.Code == 412 || rec.Code == 409 {
						continue // Authorization changed during admission; the read returns no mixed data.
					}
					var snapshot agent.Conversation
					if rec.Code != 200 || json.Unmarshal(res.Data, &snapshot) != nil {
						problems <- fmt.Sprintf("unexpected concurrent read response %d", rec.Code)
						return
					}
					expected := 1
					if snapshot.ContextSecurityVersion%2 == 0 {
						expected = 2
					}
					if snapshot.Version != snapshot.ContextSecurityVersion || len(snapshot.Members) != expected {
						problems <- fmt.Sprintf("mixed version=%d epoch=%d members=%d", snapshot.Version, snapshot.ContextSecurityVersion, len(snapshot.Members))
						return
					}
					completed++
				}
			}()
		}
		// The readers retain only the stable ID; updates use the writer's version.
		current := c
		for i := range 20 {
			action := "add"
			if i%2 == 1 {
				action = "remove"
			}
			result := change(current, f.member, action, f.aliceToken)
			current.Version = result.Version
		}
		readers.Wait()
		close(problems)
		close(counts)
		for problem := range problems {
			t.Error(problem)
		}
		completed := 0
		for count := range counts {
			completed += count
		}
		if completed == 0 {
			t.Fatal("no concurrent snapshot was observed")
		}
	})
}
