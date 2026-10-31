package httpapi

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
)

func retirementRenewalInput(p storage.RetirementPlan) storage.RetirementRenewal {
	in := storage.RetirementRenewal{ExpectedVersion: p.Version, SnapshotSHA256: p.SnapshotSHA256, Reason: "Explicit adoption of stopped remaining originals", Items: []storage.RetirementRenewalItem{}}
	for _, item := range p.Items {
		if item.State != "succeeded" {
			in.Items = append(in.Items, storage.RetirementRenewalItem{ID: item.ID, ExpectedVersion: item.Version})
		}
	}
	return in
}
func retirementRenewalLogin(f *retirementFixture, channel string) {
	t := f.f.f.t
	ctx := context.Background()
	base := f.f.f
	var cipher []byte
	if e := base.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", base.org, base.alice).Scan(&cipher); e != nil {
		t.Fatal(e)
	}
	secret, e := identity.Open(base.api.Config.MFAKey, cipher, base.org+":"+base.alice)
	if e != nil {
		t.Fatal(e)
	}
	// Only the isolated fixture resets the consumed TOTP time; production has no reset.
	if _, e = base.owner.Exec(ctx, "UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", base.org, base.alice, time.Now().Unix()/30-1); e != nil {
		t.Fatal(e)
	}
	challenge := base.login("alice", channel)
	headers := map[string]string{}
	if channel == "web" {
		headers["Origin"] = "http://localhost:5178"
	}
	r, v := base.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, headers)
	expect(t, r, 200, v)
	result := data[identity.LoginResult](t, v)
	if channel == "web" {
		headers["X-CSRF-Token"] = result.CSRFToken
		for _, cookie := range r.Result().Cookies() {
			if cookie.Name == "cp_session" {
				headers["Cookie"] = "cp_session=" + cookie.Value
			}
		}
		f.headers = headers
		base.aliceToken = ""
	} else {
		base.aliceToken = result.AccessToken
	}
}
func stopRetirementAuthority(f *retirementFixture, p storage.RetirementPlan) {
	if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT coalesce((SELECT session_id FROM object_retirement_authorizations WHERE id=p.execution_authorization_id),p.approved_session_id) FROM object_retirement_plans p WHERE id=$1)", p.ID); e != nil {
		f.f.f.t.Fatal(e)
	}
	for _, item := range p.Items {
		if item.State != "succeeded" {
			f.run()
		}
	}
}
func TestObjectRetirementRenewalFourClients(t *testing.T) {
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			f := newRetirementFixture(t, channel)
			p := f.create("history")
			f.ready(p.ID)
			p = f.decision(p, "approve", 202)
			original := p
			f.run()
			p = f.plan(p.ID)
			stopRetirementAuthority(f, p)
			retirementRenewalLogin(f, channel)
			p = f.plan(p.ID)
			if f.cloud.calls != 1 || !slices.ContainsFunc(p.Items, func(v storage.RetirementItem) bool { return v.State == "blocked" }) {
				t.Fatal("not a stopped partial result", p)
			}
			if os.Getenv("CP_BROWSER_RETIREMENT_RENEWAL_FIXTURE") == "1" && channel == "android" {
				holdBrowserBusinessFixture(t, f.f.f, map[string]any{"fixture_kind": "retirement-reauthorization", "file_id": p.Snapshot.FileID, "plan_id": p.ID, "snapshot_sha256": p.SnapshotSHA256, "fixture_scope": "Native disposable durable PostgreSQL; synthetic exact cloud sources and isolated retention/observation clock; initial approval revoked after one confirmed result. No live cloud deletion."})
				p = f.plan(p.ID)
				if p.Authorization == nil || p.Authorization.Sequence != 1 {
					t.Fatal("Web did not commit a renewal", p)
				}
			} else {
				in := retirementRenewalInput(p)
				key := core.NewID()
				for range 2 {
					r, v := f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/reauthorize", key, in)
					expect(t, r, 202, v)
					p = data[storage.RetirementPlan](t, v)
				}
			}
			if p.Authorization == nil || p.Authorization.Sequence != 1 || p.Version != original.Version+1 || !p.ApprovedAt.Equal(*original.ApprovedAt) || *p.ApprovalReason != *original.ApprovalReason || p.SnapshotSHA256 != original.SnapshotSHA256 || len(p.Authorization.Items) != 1 {
				t.Fatal("initial approval or remaining scope changed", p)
			}
			for _, item := range p.Items {
				if item.State == "succeeded" && item.Generation != 1 || item.State == "queued" && item.Generation != 2 {
					t.Fatal(item)
				}
			}
			r, v := f.request("GET", "/v1/object-retirement-plans/"+p.ID+"/authorizations?limit=1", "", nil)
			expect(t, r, 200, v)
			history := data[core.Page[storage.RetirementAuthorization]](t, v)
			if len(history.Items) != 1 || strings.Contains(r.Body.String(), "session_id") || strings.Contains(r.Body.String(), "fixture-bucket") {
				t.Fatal("private grant evidence disclosed")
			}
			f.run()
			p = f.plan(p.ID)
			if p.State != "completed" || f.cloud.calls != 2 {
				t.Fatal("renewal repeated a confirmed deletion or did not finish", p, f.cloud.calls)
			}
			var linked int
			if e := f.f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM object_retirement_receipts WHERE authorization_id=$1 AND outcome='confirmed_absent'", p.Authorization.ID).Scan(&linked); e != nil || linked != 1 {
				t.Fatal("receipt not bound to the renewed grant", linked, e)
			}
			if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE object_retirement_authorizations SET reason='replacement' WHERE id=$1", p.Authorization.ID); e == nil {
				t.Fatal("grant history rewritten")
			}
		})
	}
}
func TestObjectRetirementRenewalGuards(t *testing.T) {
	for _, scenario := range []string{"stale-plan", "stale-item", "extra-item", "duplicate-item", "missing-item", "digest", "hold", "policy", "namespace", "mfa", "stopped", "audit-rollback", "second-renewal"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRetirementFixture(t, "android")
			p := f.create("history")
			f.ready(p.ID)
			p = f.decision(p, "approve", 202)
			stopRetirementAuthority(f, p)
			retirementRenewalLogin(f, "android")
			p = f.plan(p.ID)
			in := retirementRenewalInput(p)
			ctx := context.Background()
			expected := 409
			exec := func(q string, args ...any) {
				t.Helper()
				if _, e := f.f.f.owner.Exec(ctx, q, args...); e != nil {
					t.Fatal(e)
				}
			}
			switch scenario {
			case "stale-plan":
				in.ExpectedVersion--
			case "stale-item":
				in.Items[0].ExpectedVersion--
			case "extra-item":
				in.Items[0].ID = core.NewID()
			case "duplicate-item":
				in.Items[1] = in.Items[0]
				expected = 422
			case "missing-item":
				in.Items = in.Items[:1]
			case "digest":
				in.SnapshotSHA256 = strings.Repeat("0", 64)
			case "hold":
				exec("INSERT INTO retention_holds(id,org_id,file_id,version_id,kind,reason,owner_id) VALUES($1,$2,$3,$4,'business','later protected evidence',$5)", core.NewID(), f.f.f.org, f.current.File.ID, f.old.Version.ID, f.f.f.alice)
			case "policy":
				exec("INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason) VALUES($1,$2,2,'formal_history',false,86400,86400,$3,'later policy stop')", core.NewID(), f.f.f.org, f.f.f.admin)
			case "namespace":
				f.service.Backends["r2"].ExclusiveNamespace = false
			case "mfa":
				exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '1 hour' WHERE user_id=$1", f.f.f.alice)
				expected = 403
			case "stopped":
				p = f.decision(p, "cancel", 200)
				in = retirementRenewalInput(p)
			case "audit-rollback":
				exec(`CREATE FUNCTION reject_retirement_renewal_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='file.original.retirement.reauthorized' THEN RAISE EXCEPTION 'isolated fixture';END IF;RETURN NEW;END;$$;CREATE TRIGGER reject_retirement_renewal_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_retirement_renewal_fixture()`)
				expected = 500
			case "second-renewal":
				expected = 202
			}
			r, v := f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/reauthorize", core.NewID(), in)
			expect(t, r, expected, v)
			if scenario == "second-renewal" {
				p = data[storage.RetirementPlan](t, v)
				first := p.Authorization.ID
				stopRetirementAuthority(f, p)
				retirementRenewalLogin(f, "android")
				p = f.plan(p.ID)
				r, v = f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/reauthorize", core.NewID(), retirementRenewalInput(p))
				expect(t, r, 202, v)
				p = data[storage.RetirementPlan](t, v)
				if p.Authorization.Sequence != 2 || p.Authorization.ID == first || p.Items[0].Generation != 3 {
					t.Fatal(p)
				}
				r, v = f.request("GET", "/v1/object-retirement-plans/"+p.ID+"/authorizations?limit=1", "", nil)
				expect(t, r, 200, v)
				page := data[core.Page[storage.RetirementAuthorization]](t, v)
				if page.NextCursor == nil || page.Items[0].Sequence != 2 {
					t.Fatal(page)
				}
				r, v = f.request("GET", "/v1/object-retirement-plans/"+p.ID+"/authorizations?limit=1&cursor="+*page.NextCursor, "", nil)
				expect(t, r, 200, v)
				if data[core.Page[storage.RetirementAuthorization]](t, v).Items[0].ID != first {
					t.Fatal("renewal history missing")
				}
				// A later logout still stops the new grant; renewal is never a blanket bypass.
				stopRetirementAuthority(f, p)
				if f.cloud.calls != 0 {
					t.Fatal("revoked new grant executed")
				}
				return
			}
			var grants, jobs int
			if e := f.f.f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM object_retirement_authorizations),(SELECT count(*) FROM jobs WHERE kind=$1)", storage.RetirementJobKind).Scan(&grants, &jobs); e != nil || grants != 0 || jobs != 2 || f.cloud.calls != 0 {
				t.Fatal("rejected renewal escaped rollback", grants, jobs, f.cloud.calls, e)
			}
		})
	}
}
