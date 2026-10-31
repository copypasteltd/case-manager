package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
)

type originalCloud struct {
	mu      sync.Mutex
	sources map[string]objectstore.Source
	calls   int
	failure error
}

func (c *originalCloud) RemoveFrozen(_ context.Context, s objectstore.Source) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failure != nil {
		e := c.failure
		c.failure = nil
		return false, e
	}
	if current, ok := c.sources[s.Key]; ok {
		if current != s {
			return false, &objectstore.Error{Code: "SOURCE_CHANGED"}
		}
		delete(c.sources, s.Key)
	}
	return true, nil
}

type retirementFixture struct {
	f            *uploadFixture
	old, current storage.CommitResult
	service      *storage.ObjectRetirementService
	cloud        *originalCloud
	headers      map[string]string
	draftID      string
}

func newRetirementFixture(t *testing.T, channel string) *retirementFixture {
	t.Helper()
	ctx := context.Background()
	f, old, primary, secondary, _ := committedReplicaFixture(t, 15)
	runReplica(t, replicaHost(f, primary, secondary, 15))
	current := replacementVersion(t, f, old)
	// Synthetic native-version / age arrangement only. The HTTP upload, checksum,
	// scan, promotion and replica prerequisites above are executed normally. This
	// does not establish actual OSS bucket versioning or R2 IAM.
	tx, e := f.f.owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{"ALTER TABLE file_versions DISABLE TRIGGER file_version_immutable", "ALTER TABLE upload_sessions DISABLE TRIGGER upload_input_immutable", "ALTER TABLE upload_sessions DISABLE TRIGGER upload_commit_time"} {
		if _, e = tx.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	var draft string
	if e = tx.QueryRow(ctx, "SELECT id FROM upload_sessions WHERE org_id=$1 AND committed_version_id=$2", f.f.org, old.Version.ID).Scan(&draft); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `UPDATE file_versions SET created_at=clock_timestamp()-interval '4 days',provider_source_id=jsonb_set(provider_source_id::jsonb,'{VersionID}','"synthetic-native-version"')::text WHERE id=$1`, old.Version.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `UPDATE file_replicas SET source_id=jsonb_set(source_id::jsonb,'{VersionID}','"synthetic-native-version"')::text WHERE org_id=$1 AND version_id=$2 AND provider='oss'`, f.f.org, old.Version.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `UPDATE upload_sessions SET committed_at=clock_timestamp()-interval '3 days',expires_at=clock_timestamp()-interval '2 days',frozen_source_id=jsonb_set(frozen_source_id::jsonb,'{VersionID}','"synthetic-native-draft"')::text,frozen_native_version='synthetic-native-draft' WHERE id=$1`, draft); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"ALTER TABLE file_versions ENABLE TRIGGER file_version_immutable", "ALTER TABLE upload_sessions ENABLE TRIGGER upload_input_immutable", "ALTER TABLE upload_sessions ENABLE TRIGGER upload_commit_time"} {
		if _, e = tx.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = f.f.owner.Exec(ctx, `INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason)
 SELECT gen_random_uuid(),$1,1,kind,true,86400,86400,$2,'isolated test policy, not company adoption' FROM unnest(ARRAY['formal_history','draft']) kind`, f.f.org, f.f.admin); e != nil {
		t.Fatal(e)
	}
	if _, e = f.f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='file_retention_manager'`, f.f.org, f.f.alice); e != nil {
		t.Fatal(e)
	}
	cloud := &originalCloud{sources: map[string]objectstore.Source{}}
	rows, e := f.f.owner.Query(ctx, "SELECT source_id FROM file_replicas WHERE org_id=$1 AND version_id=$2 UNION ALL SELECT frozen_source_id FROM upload_sessions WHERE id=$3", f.f.org, old.Version.ID, draft)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var raw string
		var source objectstore.Source
		if e = rows.Scan(&raw); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal([]byte(raw), &source); e != nil {
			t.Fatal(e)
		}
		cloud.sources[source.Key] = source
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	backends := map[string]*storage.OriginalCleanupBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", BindingSHA256: strings.Repeat("a", 64), Cleaner: cloud}, "r2": {Provider: "r2", Bucket: "fixture-r2-bucket", BindingSHA256: strings.Repeat("b", 64), ExclusiveNamespace: true, Cleaner: cloud}}
	f.f.api.OriginalCleanup = backends
	f.f.handler = f.f.api.Handler()
	fixture := &retirementFixture{f: f, old: old, current: current, cloud: cloud, draftID: draft, service: &storage.ObjectRetirementService{Files: f.service.Files, Backends: backends}}
	if channel == "web" {
		challenge := f.f.login("alice", "web")
		r, v := f.f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
		expect(t, r, 200, v)
		enrollment := data[identity.LoginResult](t, v)
		r, v = f.f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(enrollment.MFASecret, time.Now())}, map[string]string{"Origin": "http://localhost:5178"})
		expect(t, r, 200, v)
		fixture.headers = map[string]string{"Origin": "http://localhost:5178", "X-CSRF-Token": data[identity.LoginResult](t, v).CSRFToken}
		for _, cookie := range r.Result().Cookies() {
			if cookie.Name == "cp_session" {
				fixture.headers["Cookie"] = "cp_session=" + cookie.Value
			}
		}
		f.f.aliceToken = ""
	} else {
		f.f.aliceToken = memberMFAToken(f.f, "alice", channel)
	}
	return fixture
}
func (f *retirementFixture) request(method, path, key string, body any) (*httptest.ResponseRecorder, response) {
	return f.f.f.request(method, path, f.f.f.aliceToken, key, body, f.headers)
}
func (f *retirementFixture) candidates() []storage.RetirementCandidate {
	t := f.f.f.t
	r, v := f.request("GET", "/v1/files/"+f.current.File.ID+"/object-retirement-candidates?limit=20", "", nil)
	if r.Code != 200 {
		a, _ := f.f.f.api.Identity.Authenticate(context.Background(), f.f.f.aliceToken)
		_, e := f.service.Candidates(context.Background(), a, f.current.File.ID, storage.Filter{})
		t.Logf("candidate diagnostic: %v", e)
	}
	expect(t, r, 200, v)
	return data[core.Page[storage.RetirementCandidate]](t, v).Items
}
func (f *retirementFixture) create(kind string) storage.RetirementPlan {
	t := f.f.f.t
	target := f.old.Version.ID
	if kind == "committed_draft" {
		target = f.draftID
	}
	body := map[string]any{"expected_file_version": f.current.File.Version, "kind": kind, "target_id": target, "reason": "explicit isolated original selection"}
	r, v := f.request("POST", "/v1/files/"+f.current.File.ID+"/object-retirement-plans", core.NewID(), body)
	if r.Code != 201 {
		t.Logf("candidates: %+v", f.candidates())
	}
	expect(t, r, 201, v)
	if strings.Contains(r.Body.String(), "fixture-bucket") || strings.Contains(r.Body.String(), "synthetic-native") || strings.Contains(r.Body.String(), "org/"+f.f.f.org) {
		t.Fatal("private storage evidence disclosed")
	}
	return data[storage.RetirementPlan](t, v)
}
func (f *retirementFixture) ready(id string) {
	t := f.f.f.t
	ctx := context.Background()
	tx, e := f.f.f.owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	// Isolated fixture clock transition; no corresponding production bypass.
	if _, e = tx.Exec(ctx, "ALTER TABLE object_retirement_plans DISABLE TRIGGER retirement_plan_input_immutable"); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "ALTER TABLE object_retirement_plans DISABLE TRIGGER retirement_plan_state"); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "UPDATE object_retirement_plans SET created_at=clock_timestamp()-interval '2 days',ready_after=clock_timestamp()-interval '1 second',expires_at=clock_timestamp()+interval '6 days' WHERE id=$1", id); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "ALTER TABLE object_retirement_plans ENABLE TRIGGER retirement_plan_input_immutable"); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "ALTER TABLE object_retirement_plans ENABLE TRIGGER retirement_plan_state"); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func (f *retirementFixture) decision(p storage.RetirementPlan, action string, status int) storage.RetirementPlan {
	t := f.f.f.t
	r, v := f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/"+action, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "explicit fixture " + action})
	expect(t, r, status, v)
	if status >= 400 {
		return p
	}
	return data[storage.RetirementPlan](t, v)
}
func (f *retirementFixture) plan(id string) storage.RetirementPlan {
	r, v := f.request("GET", "/v1/object-retirement-plans/"+id, "", nil)
	expect(f.f.f.t, r, 200, v)
	return data[storage.RetirementPlan](f.f.f.t, v)
}
func (f *retirementFixture) run() {
	f.f.f.t.Helper()
	worked, e := f.service.RunOne(context.Background(), "original-fixture")
	if !worked || e != nil {
		f.f.f.t.Fatal("original iteration", worked, e)
	}
}
func TestObjectRetirementFourClients(t *testing.T) {
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			f := newRetirementFixture(t, channel)
			p := f.create("history")
			if len(p.Items) != 2 || p.State != "observing" {
				t.Fatal(p)
			}
			f.decision(p, "approve", 409)
			f.ready(p.ID)
			p = f.decision(p, "approve", 202)
			// Database-level dependency guards protect non-HTTP business writers too.
			for _, q := range []string{"UPDATE files SET current_version_id=$1 WHERE id=$2", "INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),org_id,id,$1,'recovery_set',gen_random_uuid() FROM files WHERE id=$2"} {
				if _, e := f.f.f.api.DB.Exec(context.Background(), q, f.old.Version.ID, f.current.File.ID); e == nil {
					t.Fatal("retired version acquired dependency")
				}
			}
			r, v := f.request("GET", "/v1/files/"+f.old.File.ID+"/versions/"+f.old.Version.ID+"/text?offset=0&limit=100", "", nil)
			if r.Code != 409 || v.Error.Code != "FILE_NOT_DOWNLOADABLE" {
				t.Fatal("retired text guard missing", r.Code, v)
			}
			f.run()
			p = f.plan(p.ID)
			if p.State != "approved" || len(slices.DeleteFunc(slices.Clone(p.Items), func(v storage.RetirementItem) bool { return v.State != "succeeded" })) != 1 {
				t.Fatal("partial delete falsely completed", p)
			}
			f.run()
			p = f.plan(p.ID)
			if p.State != "completed" || !slices.ContainsFunc(p.Items, func(i storage.RetirementItem) bool { return i.State == "succeeded" }) {
				t.Fatal(p)
			}
			var availability string
			var versions int
			_ = f.f.f.owner.QueryRow(context.Background(), "SELECT availability FROM file_versions WHERE id=$1", f.old.Version.ID).Scan(&availability)
			_ = f.f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM file_versions WHERE file_id=$1", f.current.File.ID).Scan(&versions)
			if availability != "purged" || versions != 2 || f.cloud.calls != 2 {
				t.Fatal(availability, versions, f.cloud.calls)
			}
			r, v = f.request("GET", "/v1/object-retirement-items/"+p.Items[0].ID+"/receipts?limit=20", "", nil)
			expect(t, r, 200, v)
			if len(data[core.Page[storage.CleanupReceipt]](t, v).Items) != 2 {
				t.Fatal("receipt missing")
			}
			if _, e := f.f.f.api.DB.Exec(context.Background(), "UPDATE object_retirement_receipts SET outcome='failed' WHERE item_id=$1", p.Items[0].ID); e == nil {
				t.Fatal("receipt rewritten")
			}
			if channel == "android" && os.Getenv("CP_BROWSER_OBJECT_RETIREMENT_FIXTURE") == "1" {
				holdBrowserBusinessFixture(t, f.f.f, map[string]any{"object_retirement_fixture": true, "file_id": f.current.File.ID, "completed_plan_id": p.ID, "eligible_target_id": f.draftID, "fixture_scope": "isolated native PG; synthetic cloud/native-version evidence; observation clock advanced only in prior fixture; no actual cloud deletion"})
			}
		})
	}
}

func TestObjectRetirementAtomicityAndCapacity(t *testing.T) {
	t.Run("approval audit rollback and replay preserve exactly two jobs", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		f.ready(p.ID)
		ctx := context.Background()
		if _, e := f.f.f.owner.Exec(ctx, `CREATE FUNCTION reject_retirement_approval_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='file.original.retirement.approved' THEN RAISE EXCEPTION 'isolated fixture'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER reject_retirement_approval_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_retirement_approval_fixture();`); e != nil {
			t.Fatal(e)
		}
		key := core.NewID()
		body := map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "explicit atomic approval"}
		r, v := f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/approve", key, body)
		expect(t, r, 500, v)
		var barriers, jobs int
		if e := f.f.f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM object_retirement_barriers),(SELECT count(*) FROM jobs WHERE kind=$1)", storage.RetirementJobKind).Scan(&barriers, &jobs); e != nil {
			t.Fatal(e)
		}
		if barriers != 0 || jobs != 0 || f.plan(p.ID).State != "observing" {
			t.Fatal("approval escaped rollback", barriers, jobs)
		}
		if _, e := f.f.f.owner.Exec(ctx, "DROP TRIGGER reject_retirement_approval_fixture ON audit_events; DROP FUNCTION reject_retirement_approval_fixture()"); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 2; i++ {
			r, v = f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/approve", key, body)
			expect(t, r, 202, v)
		}
		if r.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("approval not replayed")
		}
		if e := f.f.f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind=$1", storage.RetirementJobKind).Scan(&jobs); e != nil || jobs != 2 {
			t.Fatal("duplicate/missing jobs", jobs, e)
		}
	})
	t.Run("unknown cloud result retains charge until all absence receipts then permits draft cleanup", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		ctx := context.Background()
		if _, e := f.f.f.owner.Exec(ctx, "UPDATE storage_quotas SET logical_limit_bytes=32 WHERE org_id=$1", f.f.f.org); e != nil {
			t.Fatal(e)
		}
		assertCapacity := func(status int) {
			r, v := f.request("POST", "/v1/uploads", core.NewID(), map[string]any{"folder_id": f.f.root, "name": "capacity.bin", "size_bytes": "1", "media_type": "application/octet-stream", "source_fingerprint": "retirement-capacity"})
			expect(t, r, status, v)
			if status == 409 && v.Error.Code != "STORAGE_QUOTA_EXCEEDED" {
				t.Fatal(v.Error)
			}
		}
		p := f.create("history")
		f.ready(p.ID)
		f.decision(p, "approve", 202)
		assertCapacity(409)
		if _, e := f.f.f.owner.Exec(ctx, `CREATE FUNCTION reject_retirement_receipt_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='file.original.retirement.confirmed_absent' THEN RAISE EXCEPTION 'isolated fixture'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER reject_retirement_receipt_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_retirement_receipt_fixture();`); e != nil {
			t.Fatal(e)
		}
		worked, e := f.service.RunOne(ctx, "retirement-crash")
		if !worked || e == nil {
			t.Fatal("receipt rollback not surfaced", worked, e)
		}
		if p = f.plan(p.ID); p.State != "approved" || slices.ContainsFunc(p.Items, func(i storage.RetirementItem) bool { return i.State == "succeeded" }) {
			t.Fatal("unknown cloud outcome counted complete", p)
		}
		assertCapacity(409)
		if _, e = f.f.f.owner.Exec(ctx, "DROP TRIGGER reject_retirement_receipt_fixture ON audit_events; DROP FUNCTION reject_retirement_receipt_fixture(); UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE kind='file.original.retirement.v1'"); e != nil {
			t.Fatal(e)
		}
		f.run()
		assertCapacity(409)
		f.run()
		assertCapacity(201)
		if f.plan(p.ID).State != "completed" || len(f.cloud.sources) != 1 {
			t.Fatal("reconciliation lost originals", f.plan(p.ID), f.cloud.sources)
		}
		draft := f.create("committed_draft")
		f.ready(draft.ID)
		f.decision(draft, "approve", 202)
		f.run()
		if f.plan(draft.ID).State != "completed" || len(f.cloud.sources) != 0 {
			t.Fatal("committed draft after retired history failed")
		}
	})
	t.Run("candidate cursor and plan authority stay bound to the current reader", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		r, v := f.request("GET", "/v1/files/"+f.current.File.ID+"/object-retirement-candidates?limit=1", "", nil)
		expect(t, r, 200, v)
		page := data[core.Page[storage.RetirementCandidate]](t, v)
		if page.NextCursor == nil {
			t.Fatal("missing candidate cursor")
		}
		r, v = f.f.f.request("GET", "/v1/object-retirement-plans/"+p.ID, f.f.f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.request("GET", "/v1/files/"+f.current.File.ID+"/object-retirement-plans?limit=1&cursor="+*page.NextCursor, "", nil)
		expect(t, r, 400, v)
		if _, e := f.f.f.owner.Exec(context.Background(), "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
			t.Fatal(e)
		}
		r, v = f.request("GET", "/v1/object-retirement-plans/"+p.ID, "", nil)
		if r.Code != 401 && r.Code != 404 {
			t.Fatal("revoked reader retained plan", r.Code, v)
		}
	})
}
func TestCommittedDraftRetirement(t *testing.T) {
	f := newRetirementFixture(t, "android")
	p := f.create("committed_draft")
	f.ready(p.ID)
	p = f.decision(p, "approve", 202)
	f.run()
	p = f.plan(p.ID)
	if p.State != "completed" || f.cloud.calls != 1 {
		t.Fatal(p)
	}
	var availability string
	_ = f.f.f.owner.QueryRow(context.Background(), "SELECT availability FROM file_versions WHERE id=$1", f.old.Version.ID).Scan(&availability)
	if availability != "available" {
		t.Fatal("draft removal retired formal content")
	}
	candidates := f.candidates()
	for _, c := range candidates {
		if c.TargetID == f.draftID && c.Eligible {
			t.Fatal("deleted draft offered again")
		}
	}
}
func TestObjectRetirementProtections(t *testing.T) {
	for _, scenario := range []string{"hold", "reference", "policy", "revoke", "session", "namespace", "source"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRetirementFixture(t, "android")
			p := f.create("history")
			f.ready(p.ID)
			ctx := context.Background()
			switch scenario {
			case "hold":
				_, e := f.f.f.owner.Exec(ctx, "INSERT INTO retention_holds(id,org_id,file_id,version_id,kind,reason,owner_id) VALUES($1,$2,$3,$4,'business','isolated hold',$5)", core.NewID(), f.f.f.org, f.current.File.ID, f.old.Version.ID, f.f.f.alice)
				if e != nil {
					t.Fatal(e)
				}
			case "reference":
				_, e := f.f.f.owner.Exec(ctx, "INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) VALUES($1,$2,$3,$4,'recovery_set',$5)", core.NewID(), f.f.f.org, f.current.File.ID, f.old.Version.ID, core.NewID())
				if e != nil {
					t.Fatal(e)
				}
			case "policy":
				_, e := f.f.f.owner.Exec(ctx, "INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason) VALUES($1,$2,2,'formal_history',false,86400,86400,$3,'explicit stop')", core.NewID(), f.f.f.org, f.f.f.admin)
				if e != nil {
					t.Fatal(e)
				}
			case "namespace":
				f.service.Backends["r2"].ExclusiveNamespace = false
			}
			if slices.Contains([]string{"hold", "reference", "policy", "namespace"}, scenario) {
				f.decision(p, "approve", 409)
				if f.cloud.calls != 0 {
					t.Fatal("protected source removed")
				}
				return
			}
			p = f.decision(p, "approve", 202)
			switch scenario {
			case "revoke":
				if _, e := f.f.f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
					t.Fatal(e)
				}
			case "session":
				if _, e := f.f.f.owner.Exec(ctx, "UPDATE sessions SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
					t.Fatal(e)
				}
			case "source":
				for key, s := range f.cloud.sources {
					s.ETag = "changed-original"
					f.cloud.sources[key] = s
				}
			}
			f.run()
			if f.cloud.calls != 0 && scenario != "source" {
				t.Fatal("revoked authority removed original")
			}
			if scenario == "source" {
				p = f.plan(p.ID)
				if !slices.ContainsFunc(p.Items, func(v storage.RetirementItem) bool {
					return v.State == "failed" && v.LastError != nil && *v.LastError == "SOURCE_CHANGED"
				}) {
					t.Fatal(p)
				}
			}
		})
	}
}
func TestObjectRetirementRetriesAndStop(t *testing.T) {
	t.Run("failure recovery cannot revive a human stopped plan", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		f.ready(p.ID)
		p = f.decision(p, "approve", 202)
		ctx := context.Background()
		q := jobqueue.Queue{DB: f.f.f.api.DB}
		j, e := q.Claim(ctx, []string{storage.RetirementJobKind}, storage.RetirementJobLane, "stopped-recovery", time.Minute)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		if _, e = f.f.f.owner.Exec(ctx, `CREATE FUNCTION reject_retirement_start_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.action='file.original.retirement.started' THEN RAISE EXCEPTION 'isolated fixture'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER reject_retirement_start_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_retirement_start_fixture();`); e != nil {
			t.Fatal(e)
		}
		cause := f.service.Process(ctx, *j)
		if cause == nil {
			t.Fatal("worker transaction did not roll back")
		}
		f.decision(p, "cancel", 200)
		if e = f.service.Fail(ctx, *j, cause); e != nil {
			t.Fatal(e)
		}
		p = f.plan(p.ID)
		if p.State != "cancelled" || f.cloud.calls != 0 || slices.ContainsFunc(p.Items, func(i storage.RetirementItem) bool { return i.State != "blocked" }) {
			t.Fatal("recovery revived stopped work", p)
		}
	})
	t.Run("retry reconciles original and preserves failure", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		f.ready(p.ID)
		p = f.decision(p, "approve", 202)
		f.cloud.failure = &objectstore.Error{Code: "PROVIDER_ACCESS_DENIED"}
		f.run()
		p = f.plan(p.ID)
		index := slices.IndexFunc(p.Items, func(v storage.RetirementItem) bool { return v.State == "failed" })
		if index < 0 {
			t.Fatal("no failed item", p)
		}
		item := p.Items[index]
		if item.State != "failed" {
			t.Fatal(item)
		}
		r, v := f.request("POST", "/v1/object-retirement-items/"+item.ID+"/retry", core.NewID(), map[string]any{"expected_version": item.Version})
		expect(t, r, 202, v)
		f.run()
		f.run()
		if f.plan(p.ID).State != "completed" {
			t.Fatal("explicit retry did not finish")
		}
	})
	t.Run("stop retains partial receipts and never resurrects", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		f.ready(p.ID)
		p = f.decision(p, "approve", 202)
		f.run()
		p = f.plan(p.ID)
		p = f.decision(p, "cancel", 200)
		f.run()
		p = f.plan(p.ID)
		if p.State != "cancelled" || !slices.ContainsFunc(p.Items, func(v storage.RetirementItem) bool { return v.State == "succeeded" }) || !slices.ContainsFunc(p.Items, func(v storage.RetirementItem) bool { return v.State == "blocked" }) || f.cloud.calls != 1 {
			t.Fatal(p)
		}
		if _, e := f.f.f.api.DB.Exec(context.Background(), "UPDATE file_versions SET availability='available' WHERE id=$1", f.old.Version.ID); e == nil {
			t.Fatal("retired version resurrected")
		}
	})
	t.Run("fenced late owner cannot execute", func(t *testing.T) {
		f := newRetirementFixture(t, "android")
		p := f.create("history")
		f.ready(p.ID)
		f.decision(p, "approve", 202)
		q := jobqueue.Queue{DB: f.f.f.api.DB}
		j, e := q.Claim(context.Background(), []string{storage.RetirementJobKind}, storage.RetirementJobLane, "old-retirement", time.Minute)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		j.FencingToken++
		if e = f.service.Process(context.Background(), *j); e == nil || f.cloud.calls != 0 {
			t.Fatal("stale lease executed", e)
		}
	})
}
