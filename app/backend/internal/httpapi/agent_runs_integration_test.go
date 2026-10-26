package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
)

func TestAgentRunLedger(t *testing.T) {
	f := setup(t)
	currentTest := t
	ctx := context.Background()
	p := f.createProject("Synthetic run sources", []string{f.bob, f.member})
	exec := func(query string, args ...any) {
		currentTest.Helper()
		if _, e := f.owner.Exec(ctx, query, args...); e != nil {
			currentTest.Fatal(e)
		}
	}
	for _, user := range []string{f.alice, f.bob, f.member} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, user)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	policy := func(enabled bool) {
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',$3,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org, enabled)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	}
	conversation := func(members []string) agent.Conversation {
		currentTest.Helper()
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Synthetic bounded secretary", MemberIDs: members}, nil)
		expect(currentTest, rec, 201, res)
		return data[agent.Conversation](currentTest, res)
	}
	c := conversation([]string{f.bob})
	profileID := core.NewID()
	input := func(c agent.Conversation, refs []agent.SourceReference) agent.RunCreate {
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profileID, ClientMessageID: core.NewID(), Message: "Read the selected project and report the source; do not change business records.", ContextRefs: refs}
	}
	send := func(c agent.Conversation, in agent.RunCreate, key string, status int) agent.Run {
		currentTest.Helper()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, key, in, nil)
		expect(currentTest, rec, status, res)
		if status != 202 {
			return agent.Run{}
		}
		return data[agent.Run](currentTest, res)
	}
	cancel := func(r agent.Run) agent.Run {
		currentTest.Helper()
		rec, res := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: r.Version, Action: "cancel", Reason: "Synthetic explicit cancellation"}, nil)
		expect(currentTest, rec, 200, res)
		return data[agent.Run](currentTest, res)
	}
	get := func(r agent.Run, token string, status int) agent.Run {
		currentTest.Helper()
		rec, res := f.request("GET", "/v1/runs/"+r.ID, token, "", nil, nil)
		expect(currentTest, rec, status, res)
		if status != 200 {
			return agent.Run{}
		}
		return data[agent.Run](currentTest, res)
	}
	messages := func(c agent.Conversation, token string) core.Page[agent.Message] {
		currentTest.Helper()
		rec, res := f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=20", token, "", nil, nil)
		expect(currentTest, rec, 200, res)
		return data[core.Page[agent.Message]](currentTest, res)
	}
	sqlRejected := func(query string, args ...any) {
		currentTest.Helper()
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			currentTest.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, query, args...)
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e == nil {
			currentTest.Fatal("database accepted invalid run ledger mutation")
		}
	}
	transition := func(r agent.Run, state, kind string) agent.Run {
		currentTest.Helper()
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			currentTest.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, "UPDATE agent_runs SET state=$3,version=version+1,last_event_seq=last_event_seq+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, r.ID, state); e != nil {
			currentTest.Fatal(e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO agent_run_events(id,org_id,run_id,seq,type,state) VALUES($1,$2,$3,$4,$5,$6)", core.NewID(), f.org, r.ID, r.LastEventSeq+1, kind, state); e != nil {
			currentTest.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			currentTest.Fatal(e)
		}
		return get(r, f.aliceToken, 200)
	}
	runCase := func(name string, body func(*testing.T)) {
		t.Run(name, func(child *testing.T) {
			currentTest = child
			previous := f
			f = f.with(child)
			defer func() { currentTest = t; f = previous }()
			body(child)
		})
	}

	runCase("manual access does not enable AI and no profile creates no fake queued message", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/conversations/"+c.ID+"/runtime-options", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[agent.RuntimeOptions](t, res).Available {
			t.Fatal("AI enabled by conversation creation")
		}
		send(c, input(c, nil), core.NewID(), 404)
		if len(messages(c, f.aliceToken).Items) != 0 {
			t.Fatal("denied request saved a fake message")
		}
		policy(true)
		rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/runtime-options", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		options := data[agent.RuntimeOptions](t, res)
		if options.Available || options.Reason == nil || *options.Reason != "RUNTIME_UNAVAILABLE" {
			t.Fatal(options)
		}
		send(c, input(c, nil), core.NewID(), 503)
	})
	// A controlled deployment record in the isolated database, never a real
	// publication/confinement assertion or a model call.
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-briefing','controlled-test',repeat('a',64),'controlled-test',repeat('b',64),'active',10000,1000,5,600,$3,'Isolated native fixture only')`, profileID, f.org, f.admin)
	var run agent.Run
	var intent agent.RunCreate
	key := core.NewID()
	runCase("one atomic queued run and message bind actor profile sources and duplicate intents", func(t *testing.T) {
		intent = input(c, []agent.SourceReference{{Type: "project", ID: p.ID}})
		run = send(c, intent, key, 202)
		if run.State != "queued" || run.Version != 1 || run.LastEventSeq != 1 || run.ActorID != f.alice || run.ProfileID != profileID || !run.CanCancel {
			t.Fatal(run)
		}
		if again := send(c, intent, key, 202); again.ID != run.ID {
			t.Fatal("idempotent retry duplicated the run")
		}
		if again := send(c, intent, core.NewID(), 202); again.ID != run.ID {
			t.Fatal("client message deduplication failed")
		}
		changed := intent
		changed.Message = "A different request"
		send(c, changed, key, 409)
		send(c, changed, core.NewID(), 409)
		send(c, input(c, nil), core.NewID(), 409)
		items := messages(c, f.bobToken).Items
		if len(items) != 1 || items[0].Body != intent.Message || items[0].Role != "user" || len(items[0].SourceRefs) != 1 || items[0].SourceRefs[0].ResourceVersion != p.Version || items[0].Visibility != "available" {
			t.Fatal(items)
		}
		get(run, f.memberToken, 404)
	})
	runCase("ordered resumable JSON and SSE contain only public ledger events", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		events := data[agent.EventPage](t, res)
		if len(events.Items) != 1 || events.Items[0].Type != "run.queued" || events.AfterSeq != 1 || events.HasMore {
			t.Fatal(events)
		}
		req := httptest.NewRequest("GET", "/v1/runs/"+run.ID+"/events", nil)
		req.Header.Set("Authorization", "Bearer "+f.aliceToken)
		sse := httptest.NewRecorder()
		f.handler.ServeHTTP(sse, req)
		if sse.Code != 200 || !strings.HasPrefix(sse.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(sse.Body.String(), "event: run.queued\n") || strings.Contains(sse.Body.String(), intent.Message) {
			t.Fatal("SSE did not contain the bounded safe event")
		}
		rec, res = f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=2", f.aliceToken, "", nil, nil)
		expect(t, rec, 422, res)
	})
	runCase("only the requester cancels queued work and exact event history remains", func(t *testing.T) {
		body := agent.RunCancel{ExpectedVersion: 1, Action: "cancel", Reason: "Synthetic stop"}
		rec, res := f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.bobToken, core.NewID(), body, nil)
		expect(t, rec, 403, res)
		wrong := body
		wrong.ExpectedVersion = 2
		rec, res = f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, core.NewID(), wrong, nil)
		expect(t, rec, 409, res)
		cancelKey := core.NewID()
		rec, res = f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, cancelKey, body, nil)
		expect(t, rec, 200, res)
		run = data[agent.Run](t, res)
		if run.State != "cancelled" || run.Version != 2 || run.CancelRequestedAt == nil || run.CanCancel {
			t.Fatal(run)
		}
		rec, res = f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, cancelKey, body, nil)
		expect(t, rec, 200, res)
		rec, res = f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		events := data[agent.EventPage](t, res)
		if len(events.Items) != 1 || events.Items[0].Seq != 2 || events.Items[0].Type != "run.cancelled" {
			t.Fatal(events)
		}
		sqlRejected("UPDATE agent_runs SET state='running',version=version+1,last_event_seq=last_event_seq+1 WHERE id=$1", run.ID)
	})
	runCase("running cancellation requests stop commits but committing cannot claim rollback", func(t *testing.T) {
		r := send(c, input(c, nil), core.NewID(), 202)
		r = transition(r, "running", "run.started")
		r = cancel(r)
		if r.State != "running" || r.CancelRequestedAt == nil || r.CanCancel {
			t.Fatal("running task falsely reported as stopped", r)
		}
		sqlRejected("UPDATE agent_runs SET state='committing',version=version+1,last_event_seq=last_event_seq+1 WHERE id=$1", r.ID)
		r = transition(r, "cancelled", "run.cancelled")
		r = send(c, input(c, nil), core.NewID(), 202)
		r = transition(r, "running", "run.started")
		r = transition(r, "committing", "run.committing")
		rec, res := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: r.Version, Action: "cancel", Reason: "Do not claim rollback"}, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "COMMIT_RECONCILIATION_REQUIRED" {
			t.Fatal(res.Error)
		}
		transition(r, "interrupted", "run.interrupted")
	})
	runCase("late members cannot see old messages or runs and old cursors lose their audience binding", func(t *testing.T) {
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/members", f.aliceToken, core.NewID(), agent.MemberChange{ExpectedVersion: c.Version, UserID: f.member, Action: "add", Reason: "Explicit later audience"}, nil)
		expect(t, rec, 200, res)
		rec, res = f.request("GET", "/v1/conversations/"+c.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		c = data[agent.Conversation](t, res)
		if len(messages(c, f.memberToken).Items) != 0 {
			t.Fatal("new member received pre-join message history")
		}
		get(run, f.memberToken, 404)
		r := send(c, input(c, nil), core.NewID(), 202)
		if len(messages(c, f.memberToken).Items) != 1 {
			t.Fatal("new member could not see a subsequent shared message")
		}
		cancel(r)
	})
	runCase("source changes redact dependent text and references without blocking cancellation", func(t *testing.T) {
		r := send(c, input(c, []agent.SourceReference{{Type: "project", ID: p.ID}}), core.NewID(), 202)
		exec("UPDATE projects SET scope=scope||' changed',version=version+1 WHERE id=$1", p.ID)
		found := false
		for _, m := range messages(c, f.aliceToken).Items {
			if m.RunID == r.ID {
				found = true
				if m.Visibility != "unavailable" || m.Body != "" || len(m.SourceRefs) != 0 {
					t.Fatal("stale dependent text leaked", m)
				}
			}
		}
		if !found {
			t.Fatal("missing redacted placeholder")
		}
		get(r, f.aliceToken, 404)
		cancel(r)
	})
	runCase("database guards bind scope audience event continuity and deployment ownership", func(t *testing.T) {
		r := send(c, input(c, nil), core.NewID(), 202)
		sqlRejected("UPDATE agent_runs SET audience_ids=ARRAY[$2::uuid],version=version+1,last_event_seq=last_event_seq+1 WHERE id=$1", r.ID, f.bob)
		sqlRejected("UPDATE agent_runs SET state='running',version=version+1,last_event_seq=last_event_seq+1 WHERE id=$1", r.ID)
		sqlRejected("UPDATE conversation_messages SET body='overwrite' WHERE run_id=$1", r.ID)
		sqlRejected("UPDATE agent_runtime_profiles SET skill_sha256=repeat('c',64) WHERE id=$1", profileID)
		var canInstall bool
		if e := f.owner.QueryRow(ctx, "SELECT has_table_privilege('copypaste_app','agent_runtime_profiles','INSERT')").Scan(&canInstall); e != nil {
			t.Fatal(e)
		}
		if canInstall {
			t.Fatal("application can install an unverified runtime profile")
		}
		cancel(r)
		exec("UPDATE agent_runtime_profiles SET state='retired' WHERE id=$1", profileID)
		send(c, input(c, nil), core.NewID(), 503)
		sqlRejected("UPDATE agent_runtime_profiles SET state='active' WHERE id=$1", profileID)
	})
	runCase("rejected outbox rolls back message run event and idempotency reservation", func(t *testing.T) {
		exec("UPDATE agent_runtime_profiles SET state='retired' WHERE org_id=$1", f.org)
		// A new immutable fixture publication; retired identities remain retired.
		old := profileID
		profileID = core.NewID()
		exec(`INSERT INTO agent_runtime_profiles SELECT $1,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,'active',max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason,created_at FROM agent_runtime_profiles WHERE id=$2`, profileID, old)
		exec(`CREATE FUNCTION deny_agent_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='agent.run.queued' THEN RAISE EXCEPTION 'controlled outbox failure'; END IF; RETURN NEW; END; $$`)
		exec("CREATE TRIGGER deny_agent_outbox BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION deny_agent_outbox()")
		in := input(c, nil)
		var jobsBefore int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind=$1", agent.ExecutionJobKind).Scan(&jobsBefore); e != nil {
			t.Fatal(e)
		}
		rec, _ := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		if rec.Code != 500 {
			t.Fatalf("expected controlled rollback, got %d", rec.Code)
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_runs WHERE client_message_id=$1", in.ClientMessageID).Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n != 0 {
			t.Fatal("outbox failure left partial run")
		}
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind=$1", agent.ExecutionJobKind).Scan(&n); e != nil || n != jobsBefore {
			t.Fatal("outbox failure left an orphan execution job", e)
		}
		exec("DROP TRIGGER deny_agent_outbox ON outbox_events")
		exec("DROP FUNCTION deny_agent_outbox()")
		r := send(c, in, core.NewID(), 202)
		cancel(r)
	})
}
