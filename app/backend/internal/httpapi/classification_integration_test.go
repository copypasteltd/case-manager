package httpapi

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func TestClassificationPolicyPublication(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	token := f.administratorToken()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	current := func(level string) identity.ClassificationPolicy {
		t.Helper()
		rec, res := f.request("GET", "/v1/classification-policies", token, "", nil, nil)
		expect(t, rec, 200, res)
		for _, p := range data[core.Page[identity.ClassificationPolicy]](t, res).Items {
			if p.Level == level {
				return p
			}
		}
		t.Fatal("missing classification", level)
		return identity.ClassificationPolicy{}
	}
	input := func(p identity.ClassificationPolicy) identity.ClassificationPolicyInput {
		return identity.ClassificationPolicyInput{ExpectedVersion: p.Version, BasePolicyID: p.ID, Level: p.Level, AIAllowed: !p.AIAllowed, AllowedChannels: slices.Clone(p.AllowedChannels), Reason: "Synthetic explicit classification revision"}
	}
	publish := func(in identity.ClassificationPolicyInput, status int) identity.ClassificationPolicy {
		t.Helper()
		rec, res := f.request("POST", "/v1/classification-policies", token, core.NewID(), in, nil)
		expect(t, rec, status, res)
		if status != 201 {
			return identity.ClassificationPolicy{}
		}
		return data[identity.ClassificationPolicy](t, res)
	}
	version := func() int64 {
		t.Helper()
		var v int64
		if err := f.owner.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", f.org).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Run("configuration is separate from content and AI remains disabled initially", func(t *testing.T) {
		for _, level := range []string{"internal", "project_restricted", "confidential"} {
			p := current(level)
			if p.Version != 1 || p.AIAllowed || p.PublishedBy != nil || p.Reason != nil || p.BasePolicyID != nil || len(p.AllowedChannels) != 4 {
				t.Fatal("bootstrap fabricated publication or enabled AI", p)
			}
		}
		p := f.createProject("Synthetic policy content isolation", nil)
		rec, res := f.request("GET", "/v1/projects/"+p.ID, token, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/classification-policies", f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("POST", "/v1/classification-policies", f.aliceToken, core.NewID(), input(current("internal")), nil)
		expect(t, rec, 404, res)
	})
	t.Run("publication requires actual recent interactive proof including replay", func(t *testing.T) {
		actor, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		publish(input(current("internal")), 403)
		// A forged in-memory proof cannot override the current database session.
		body, _ := json.Marshal(input(current("internal")))
		command := core.Command{Actor: actor, Key: core.NewID(), Route: "classification.synthetic", RequestID: core.NewID(), Body: body}
		if _, err = f.api.Identity.PublishClassificationPolicy(ctx, command, input(current("internal"))); core.PublicError(err).Status != 403 {
			t.Fatal("stale database proof bypassed", err)
		}
		exec("UPDATE sessions SET mfa_verified_at=now() WHERE id=$1", actor.SessionID)
		actor.RunID = core.NewID()
		command.Actor = actor
		if _, err = f.api.Identity.PublishClassificationPolicy(ctx, command, input(current("internal"))); core.PublicError(err).Status != 403 {
			t.Fatal("agent modified classification", err)
		}
	})
	t.Run("unsupported channels confidentiality expansion duplicate and unchanged input are rejected", func(t *testing.T) {
		p := current("internal")
		in := input(p)
		in.AllowedChannels = []string{"android"}
		publish(in, 422)
		in = input(p)
		in.AllowedChannels = []string{"web", "web"}
		publish(in, 422)
		in = input(p)
		in.AllowedChannels = []string{"web", "telegram"}
		publish(in, 422)
		in = input(current("confidential"))
		publish(in, 422)
		in = input(p)
		in.BasePolicyID = core.NewID()
		publish(in, 409)
		in = input(p)
		in.AIAllowed = p.AIAllowed
		publish(in, 409)
		in = input(p)
		in.Reason = " "
		publish(in, 422)
	})
	t.Run("a publication atomically records provenance authority audit and one replay result", func(t *testing.T) {
		p := current("internal")
		before := version()
		key := core.NewID()
		in := input(p)
		in.BasePolicyID = strings.ToUpper(in.BasePolicyID)
		rec, res := f.request("POST", "/v1/classification-policies", token, key, in, nil)
		expect(t, rec, 201, res)
		next := data[identity.ClassificationPolicy](t, res)
		if next.Version != p.Version+1 || next.BasePolicyID == nil || *next.BasePolicyID != p.ID || next.PublishedBy == nil || *next.PublishedBy != f.admin || next.Reason == nil || *next.Reason != in.Reason || !next.AIAllowed || version() != before+1 {
			t.Fatal("policy publication incomplete", next)
		}
		rec, res = f.request("POST", "/v1/classification-policies", token, key, in, nil)
		expect(t, rec, 201, res)
		if data[identity.ClassificationPolicy](t, res).ID != next.ID || version() != before+1 {
			t.Fatal("replay created another policy")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND action='identity.classification_policy.published' AND resource_id=$2", f.org, next.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal("missing exact audit", n, err)
		}
		actor, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		rec, res = f.request("POST", "/v1/classification-policies", token, key, in, nil)
		expect(t, rec, 403, res)
		exec("UPDATE sessions SET mfa_verified_at=now() WHERE id=$1", actor.SessionID)
		publish(in, 409)
	})
	t.Run("concurrent publications share one exact predecessor", func(t *testing.T) {
		p := current("project_restricted")
		in := input(p)
		before := version()
		codes := make(chan int, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				rec, _ := f.request("POST", "/v1/classification-policies", token, core.NewID(), in, nil)
				codes <- rec.Code
			}()
		}
		group.Wait()
		close(codes)
		ok, conflict := 0, 0
		for code := range codes {
			if code == 201 {
				ok++
			} else if code == 409 || code == 412 {
				conflict++
			} else {
				t.Fatal("unexpected publication status", code)
			}
		}
		if ok != 1 || conflict != 1 || current(p.Level).Version != p.Version+1 || version() != before+1 {
			t.Fatal("concurrent policy overwrite", ok, conflict)
		}
	})
	t.Run("old policies are append only even through the migration identity", func(t *testing.T) {
		p := current("internal")
		if _, err := f.owner.Exec(ctx, "UPDATE classification_policies SET ai_allowed=false WHERE id=$1", p.ID); err == nil {
			t.Fatal("immutable policy overwritten")
		}
		if _, err := f.owner.Exec(ctx, "DELETE FROM classification_policies WHERE id=$1", p.ID); err == nil {
			t.Fatal("immutable policy deleted")
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO classification_policies(id,org_id,level,policy_version,ai_allowed) VALUES($1,$2,'internal',$3,false)", core.NewID(), f.org, p.Version+2); err == nil {
			t.Fatal("policy sequence skipped")
		}
		if _, err := f.api.DB.Exec(ctx, `INSERT INTO classification_policies(id,org_id,level,policy_version,ai_allowed) VALUES($1,$2,'internal',$3,false)`, core.NewID(), f.org, p.Version+1); err == nil {
			t.Fatal("application publication omitted provenance")
		}
	})
	t.Run("audit failure rolls back revision authority outbox and idempotency", func(t *testing.T) {
		p := current("project_restricted")
		before := version()
		key := core.NewID()
		exec(`CREATE FUNCTION fail_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='identity.classification_policy.published' THEN RAISE EXCEPTION 'synthetic policy audit failure'; END IF; RETURN NEW; END $$;
            CREATE TRIGGER fail_policy_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_policy_audit()`)
		rec, _ := f.request("POST", "/v1/classification-policies", token, key, input(p), nil)
		if rec.Code != 500 {
			t.Fatal("audit failure accepted", rec.Code)
		}
		exec("DROP TRIGGER fail_policy_audit ON audit_events; DROP FUNCTION fail_policy_audit()")
		if current(p.Level).ID != p.ID || version() != before {
			t.Fatal("policy escaped rollback")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", key).Scan(&n); err != nil || n != 0 {
			t.Fatal("failed publication left receipt", n, err)
		}
	})
	t.Run("history pages bind classification identity channel and current authority", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/classification-policies/internal/history?limit=1", token, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[identity.ClassificationPolicy]](t, res)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("history cursor missing")
		}
		cursor := url.QueryEscape(*page.NextCursor)
		rec, res = f.request("GET", "/v1/classification-policies/internal/history?limit=1&cursor="+cursor, token, "", nil, nil)
		expect(t, rec, 200, res)
		if data[core.Page[identity.ClassificationPolicy]](t, res).Items[0].Version != 1 {
			t.Fatal("history did not retain initial policy")
		}
		rec, res = f.request("GET", "/v1/classification-policies/project_restricted/history?limit=1&cursor="+cursor, token, "", nil, nil)
		expect(t, rec, 400, res)
		a, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		a.Channel = "web"
		if _, err = f.api.Identity.ClassificationPolicyHistory(ctx, a, "internal", *page.NextCursor, 1); core.PublicError(err).Status != 400 {
			t.Fatal("history cursor crossed channel", err)
		}
		publish(input(current("internal")), 201)
		rec, res = f.request("GET", "/v1/classification-policies/internal/history?limit=1&cursor="+cursor, token, "", nil, nil)
		expect(t, rec, 400, res)
	})
	t.Run("current policy changes AI and channel gates without granting business actions", func(t *testing.T) {
		p := f.createProject("Synthetic policy admission boundary", nil)
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		actor.RunID = core.NewID()
		if _, err = f.api.Projects.Get(ctx, actor, p.ID); err != nil {
			t.Fatal("explicit project AI policy did not admit authorized read", err)
		}
		in := input(current("project_restricted"))
		in.AIAllowed = false
		in.AllowedChannels = []string{"android"}
		publish(in, 201)
		if _, err = f.api.Projects.Get(ctx, actor, p.ID); core.PublicError(err).Status != 404 {
			t.Fatal("withdrawn AI policy admitted read", err)
		}
		rec, res := f.request("GET", "/v1/projects/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/projects/"+p.ID, token, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/classification-policies", token, "", nil, nil)
		expect(t, rec, 200, res)
	})
}
