package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

// These are disposable native PostgreSQL fixtures. No model/provider, Telegram,
// object storage, virtualization or deployment is involved.
func TestAgentPlans(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	current := t
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			current.Fatal(e)
		}
	}
	for _, role := range []string{"agent_user", "agent_plan_operator"} {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", core.NewID(), f.org, f.alice, role)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	fresh := func() {
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL", f.org, f.alice)
	}
	fresh()
	conversation := func() agent.Conversation {
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Synthetic project plans", MemberIDs: []string{}}, nil)
		expect(current, rec, 201, res)
		return data[agent.Conversation](current, res)
	}
	propose := func(c agent.Conversation, input project.Create, key string) agent.Plan {
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/plans", f.aliceToken, key, agent.ProjectPlanCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, Project: input, Reason: "Synthetic exact project intent"}, nil)
		expect(current, rec, 201, res)
		return data[agent.Plan](current, res)
	}
	input := func() project.Create {
		return project.Create{Name: "Synthetic planned " + core.NewID(), Scope: "Bound project initialization", Type: "general", OwnerID: f.alice, Classification: "internal"}
	}
	act := func(p agent.Plan, action string, status int) agent.Plan {
		rec, res := f.request("POST", "/v1/plans/"+p.ID+"/"+action, f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "Synthetic explicit " + action}, nil)
		expect(current, rec, status, res)
		if status != 200 {
			return agent.Plan{}
		}
		return data[agent.Plan](current, res)
	}
	count := func(sql string, args ...any) int {
		current.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, sql, args...).Scan(&n); e != nil {
			current.Fatal(e)
		}
		return n
	}
	caseOf := func(name string, fn func(*testing.T)) {
		t.Run(name, func(child *testing.T) {
			old := f
			f = f.with(child)
			current = child
			defer func() { f = old; current = t }()
			fn(child)
		})
	}
	caseOf("proposal stores no project and needs explicit fresh interactive approval", func(t *testing.T) {
		c := conversation()
		in := input()
		key := core.NewID()
		p := propose(c, in, key)
		if p.State != "proposed" || !p.CanApprove || p.CanCommit || len(p.PlanHash) != 64 || p.ProjectID != nil {
			t.Fatal("proposal incorrectly looks committed")
		}
		if count("SELECT count(*) FROM projects WHERE org_id=$1 AND name=$2", f.org, in.Name) != 0 {
			t.Fatal("proposal created a project")
		}
		if propose(c, in, key).ID != p.ID {
			t.Fatal("proposal replay duplicated plan")
		}
		fresh()
		act(p, "commit", 409)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		rec, res := f.request("POST", "/v1/plans/"+p.ID+"/approve", f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "No current MFA proof"}, nil)
		expect(t, rec, 403, res)
		if res.Error.Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(res.Error.Code)
		}
		fresh()
		p = act(p, "approve", 200)
		if p.State != "approved" || !p.CanCommit || p.ProjectID != nil {
			t.Fatal("approval created project or missing commit capability")
		}
		p = act(p, "commit", 200)
		if p.State != "committed" || p.OperationID == nil || p.ProjectID == nil || p.Version != 3 {
			t.Fatal("missing business receipt")
		}
		if count("SELECT count(*) FROM folders WHERE org_id=$1 AND project_id=$2", f.org, *p.ProjectID) != 4 {
			t.Fatal("business project initialization not reused")
		}
		if count("SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_id=$2 AND action='project.created'", f.org, *p.ProjectID) != 1 {
			t.Fatal("missing project audit")
		}
		if count("SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='project.created'", f.org, *p.ProjectID) != 1 {
			t.Fatal("missing project outbox")
		}
		if count("SELECT count(*) FROM agent_plan_events WHERE plan_id=$1", p.ID) != 3 {
			t.Fatal("missing immutable history")
		}
	})
	caseOf("bound hash actor version and payload prevent substituted authority", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		for _, in := range []agent.PlanAction{{ExpectedVersion: p.Version, PlanHash: strings.Repeat("f", 64), Reason: "wrong hash"}, {ExpectedVersion: 2, PlanHash: p.PlanHash, Reason: "wrong version"}} {
			rec, res := f.request("POST", "/v1/plans/"+p.ID+"/approve", f.aliceToken, core.NewID(), in, nil)
			expect(t, rec, 409, res)
		}
		rec, res := f.request("POST", "/v1/plans/"+p.ID+"/approve", f.bobToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "another actor"}, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("POST", "/v1/plans/"+p.ID+"/approve", f.aliceToken, core.NewID(), map[string]any{"expected_version": 1, "plan_hash": p.PlanHash, "reason": "no substituted payload", "project": input()}, nil)
		expect(t, rec, 422, res)
		if count("SELECT count(*) FROM agent_plan_events WHERE plan_id=$1", p.ID) != 1 {
			t.Fatal("denied approval changed history")
		}
	})
	caseOf("revocation is final and old approvals cannot be reused", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		p = act(p, "approve", 200)
		p = act(p, "revoke", 200)
		act(p, "commit", 409)
		if count("SELECT count(*) FROM projects WHERE org_id=$1 AND name=$2", f.org, p.Project.Name) != 0 {
			t.Fatal("revoked plan created project")
		}
	})
	caseOf("conversation revisions invalidate approved plans", func(t *testing.T) {
		c := conversation()
		p := propose(c, input(), core.NewID())
		fresh()
		p = act(p, "approve", 200)
		rec, res := f.request("PATCH", "/v1/conversations/"+c.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: c.Version, Title: "Changed context", State: "active", Reason: "invalidate exact intent"}, nil)
		expect(t, rec, 200, res)
		act(p, "commit", 409)
		act(p, "revoke", 200)
	})
	caseOf("published template cannot change and retirement invalidates the approved snapshot", func(t *testing.T) {
		id := core.NewID()
		exec("INSERT INTO project_templates(id,org_id,key,name,revision,state,task_and_folder_schema) VALUES($1,$2,$3,'Synthetic template',1,'published',$4)", id, f.org, "fixture-"+id, `{"folders":["Evidence"],"tasks":[],"milestones":[]}`)
		in := input()
		in.TemplateID = &id
		p := propose(conversation(), in, core.NewID())
		fresh()
		p = act(p, "approve", 200)
		if _, err := f.owner.Exec(ctx, "UPDATE project_templates SET task_and_folder_schema=$2,version=version+1 WHERE id=$1", id, `{"folders":["Substituted"],"tasks":[],"milestones":[]}`); err == nil {
			t.Fatal("published template content was replaced")
		}
		exec("UPDATE project_templates SET state='archived',version=version+1,retired_at=clock_timestamp(),retired_by=$2 WHERE id=$1", id, f.admin)
		act(p, "commit", 409)
	})
	caseOf("business and plan receipt roll back together and retry is safe", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		p = act(p, "approve", 200)
		exec("CREATE FUNCTION fail_fixture_plan_receipt() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'controlled receipt failure'; END;$$")
		exec("CREATE TRIGGER fail_fixture_receipt BEFORE INSERT ON agent_plan_events FOR EACH ROW WHEN(NEW.kind='committed') EXECUTE FUNCTION fail_fixture_plan_receipt()")
		key := core.NewID()
		body := agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "atomic rollback test"}
		rec, res := f.request("POST", "/v1/plans/"+p.ID+"/commit", f.aliceToken, key, body, nil)
		expect(t, rec, 500, res)
		if count("SELECT count(*) FROM projects WHERE org_id=$1 AND name=$2", f.org, p.Project.Name) != 0 || count("SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", "agent-plan:"+p.ID) != 0 {
			t.Fatal("partial business commit survived receipt rollback")
		}
		exec("DROP TRIGGER fail_fixture_receipt ON agent_plan_events")
		exec("DROP FUNCTION fail_fixture_plan_receipt()")
		rec, res = f.request("POST", "/v1/plans/"+p.ID+"/commit", f.aliceToken, key, body, nil)
		expect(t, rec, 200, res)
		created := data[agent.Plan](t, res)
		rec, res = f.request("POST", "/v1/plans/"+p.ID+"/commit", f.aliceToken, key, body, nil)
		expect(t, rec, 200, res)
		if data[agent.Plan](t, res).OperationID == nil || *data[agent.Plan](t, res).OperationID != *created.OperationID {
			t.Fatal("lost acknowledgement changed receipt")
		}
		if count("SELECT count(*) FROM projects WHERE org_id=$1 AND name=$2", f.org, p.Project.Name) != 1 {
			t.Fatal("retry duplicated project")
		}
	})
	caseOf("two concurrent commit keys consume one exact grant", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		p = act(p, "approve", 200)
		type reply struct {
			status int
			result response
		}
		out := make(chan reply, 2)
		for i := 0; i < 2; i++ {
			go func() {
				rec, res := f.request("POST", "/v1/plans/"+p.ID+"/commit", f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "concurrent same grant"}, nil)
				out <- reply{rec.Code, res}
			}()
		}
		first, second := <-out, <-out
		if first.status != 200 || second.status != 200 {
			t.Fatalf("concurrent statuses %d %d", first.status, second.status)
		}
		a, b := data[agent.Plan](t, first.result), data[agent.Plan](t, second.result)
		if *a.ProjectID != *b.ProjectID || *a.OperationID != *b.OperationID || count("SELECT count(*) FROM projects WHERE org_id=$1 AND name=$2", f.org, p.Project.Name) != 1 {
			t.Fatal("grant consumed twice")
		}
	})
	caseOf("expired immutable intent cannot be approved", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		var raw string
		if e := f.owner.QueryRow(ctx, "SELECT intent FROM agent_plans WHERE id=$1", p.ID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var intent map[string]any
		if e := json.Unmarshal([]byte(raw), &intent); e != nil {
			t.Fatal(e)
		}
		id := core.NewID()
		created := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
		expires := created.Add(30 * time.Minute)
		intent["id"], intent["created_at"], intent["expires_at"] = id, created, expires
		encoded, _ := json.Marshal(intent)
		canonical, _ := core.CanonicalJSON(encoded)
		hash := core.Digest(canonical)
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `INSERT INTO agent_plans(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,kind,intent,plan_hash,payload,template_snapshot,reason,created_at,expires_at)
 SELECT $2,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,kind,$3,$4,payload,template_snapshot,reason,$5,$6 FROM agent_plans WHERE id=$1`, p.ID, id, string(canonical), hash, created, expires)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, "INSERT INTO agent_plan_events(id,org_id,plan_id,version,kind,plan_hash,actor_id,reason) VALUES($1,$2,$3,1,'proposed',$4,$5,'Synthetic expired fixture')", core.NewID(), f.org, id, hash, f.alice)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		p.ID, p.PlanHash = id, hash
		act(p, "approve", 409)
	})
	caseOf("database prevents rewritten intent history and fabricated commits", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		for _, sql := range []string{"UPDATE agent_plans SET payload='{}' WHERE id=$1", "DELETE FROM agent_plans WHERE id=$1", "UPDATE agent_plan_events SET reason='rewritten' WHERE plan_id=$1", "UPDATE agent_plans SET version=2,state='approved' WHERE id=$1"} {
			tx, e := f.owner.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, sql, p.ID)
			if e == nil {
				e = tx.Commit(ctx)
			}
			tx.Rollback(ctx)
			if e == nil {
				t.Fatal("database accepted invalid plan history")
			}
		}
	})
	caseOf("authority changes invalidate even previously approved plans", func(t *testing.T) {
		p := propose(conversation(), input(), core.NewID())
		fresh()
		p = act(p, "approve", 200)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		act(p, "commit", 412)
		rec, res := f.request("GET", "/v1/plans/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[agent.Plan](t, res).BlockedReason == nil {
			t.Fatal("stale grant looks usable")
		}
		act(p, "revoke", 200)
	})
	if os.Getenv("CP_BROWSER_AGENT_PLAN_FIXTURE") == "1" {
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		c := conversation()
		holdBrowserBusinessFixture(t, f, map[string]any{"plan_fixture": true, "plan_conversation_id": c.ID})
	}
}
