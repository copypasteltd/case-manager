package httpapi

import (
	"context"
	"net/http/httptest"
	"os"
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

type cleanupCloud struct {
	mu                       sync.Mutex
	present                  bool
	aborts, inspections      int
	inspectError, abortError error
}

func (c *cleanupCloud) MultipartAbsent(context.Context, objectstore.Multipart) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inspections++
	if c.inspectError != nil {
		return false, c.inspectError
	}
	return !c.present, nil
}
func (c *cleanupCloud) Abort(context.Context, objectstore.Multipart) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aborts++
	if c.abortError != nil {
		e := c.abortError
		c.abortError = nil
		return e
	}
	c.present = false
	return nil
}

type cleanupFixture struct {
	f       *uploadFixture
	u       storage.Upload
	service *storage.MultipartCleanupService
	cloud   *cleanupCloud
	headers map[string]string
}

func newCleanupFixture(t *testing.T, channel string) *cleanupFixture {
	t.Helper()
	f := newUploadFixture(t)
	u := f.prepared(f.create("abandoned-multipart.bin", 15))
	ctx := context.Background()
	if _, e := f.f.owner.Exec(ctx, "UPDATE upload_sessions SET created_at=clock_timestamp()-interval '4 days',expires_at=clock_timestamp()-interval '2 days' WHERE id=$1", u.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.f.owner.Exec(ctx, `INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason)
  VALUES($1,$2,1,'multipart',true,86400,86400,$3,'isolated fixture policy, not a company decision')`, core.NewID(), f.f.org, f.f.admin); e != nil {
		t.Fatal(e)
	}
	if _, e := f.f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
  SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='file_retention_manager'`, core.NewID(), f.f.org, f.f.alice); e != nil {
		t.Fatal(e)
	}
	var headers map[string]string
	if channel == "web" {
		challenge := f.f.login("alice", channel)
		r, v := f.f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
		expect(t, r, 200, v)
		enrollment := data[identity.LoginResult](t, v)
		r, v = f.f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(enrollment.MFASecret, time.Now())}, map[string]string{"Origin": "http://localhost:5178"})
		expect(t, r, 200, v)
		headers = map[string]string{"Origin": "http://localhost:5178", "X-CSRF-Token": data[identity.LoginResult](t, v).CSRFToken}
		for _, cookie := range r.Result().Cookies() {
			if cookie.Name == "cp_session" {
				headers["Cookie"] = "cp_session=" + cookie.Value
			}
		}
		f.f.aliceToken = ""
	} else {
		f.f.aliceToken = memberMFAToken(f.f, "alice", channel)
	}
	cloud := &cleanupCloud{present: true}
	backends := map[string]*storage.CleanupBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", BindingSHA256: strings.Repeat("a", 64), Cleaner: cloud}}
	f.f.api.MultipartCleanup = backends
	f.f.handler = f.f.api.Handler()
	u = f.getForCleanup(u.ID)
	return &cleanupFixture{f: f, u: u, cloud: cloud, headers: headers, service: &storage.MultipartCleanupService{Files: f.service.Files, Backends: backends}}
}

func (f *cleanupFixture) request(method, path, token, key string, body any, headers map[string]string) (*httptest.ResponseRecorder, response) {
	if headers == nil && f.headers != nil && token == f.f.f.aliceToken {
		headers = f.headers
	}
	return f.f.f.request(method, path, token, key, body, headers)
}

// The ordinary upload API is intentionally Web/Android only; cleanup is human
// administrative work supported by all four clients.
func (f *uploadFixture) getForCleanup(id string) storage.Upload {
	f.f.t.Helper()
	u := storage.Upload{}
	if e := f.f.owner.QueryRow(context.Background(), "SELECT version,state,created_at,expires_at FROM upload_sessions WHERE id=$1", id).Scan(&u.Version, &u.State, &u.CreatedAt, &u.ExpiresAt); e != nil {
		f.f.t.Fatal(e)
	}
	u.ID = id
	return u
}
func (f *cleanupFixture) create() storage.CleanupPlan {
	f.f.f.t.Helper()
	r, v := f.request("POST", "/v1/folders/"+f.f.root+"/upload-cleanup-plans", f.f.f.aliceToken, core.NewID(), map[string]any{"items": []map[string]any{{"upload_id": f.u.ID, "expected_version": f.u.Version}}, "reason": "isolated exact selection"}, nil)
	if r.Code != 201 {
		actor, err := f.f.f.api.Identity.Authenticate(context.Background(), f.f.f.aliceToken)
		if err == nil {
			inventory, err := f.service.Candidates(context.Background(), actor, f.f.root, storage.Filter{})
			f.f.f.t.Logf("candidate diagnostics: %+v %v", inventory, err)
		}
	}
	expect(f.f.f.t, r, 201, v)
	if strings.Contains(r.Body.String(), "fixture-bucket") || strings.Contains(r.Body.String(), "fixture-upload-") || strings.Contains(r.Body.String(), "org/"+f.f.f.org+"/drafts/") {
		f.f.f.t.Fatal("cloud operation references disclosed")
	}
	return data[storage.CleanupPlan](f.f.f.t, v)
}
func (f *cleanupFixture) ready(id string) {
	f.f.f.t.Helper()
	ctx := context.Background()
	tx, e := f.f.f.owner.Begin(ctx)
	if e != nil {
		f.f.f.t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	// Isolated fixture time transition, never a production observation bypass.
	if _, e = tx.Exec(ctx, "ALTER TABLE upload_cleanup_plans DISABLE TRIGGER upload_cleanup_plan_guard"); e != nil {
		f.f.f.t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "UPDATE upload_cleanup_plans SET created_at=clock_timestamp()-interval '2 days',ready_after=clock_timestamp()-interval '1 second',expires_at=clock_timestamp()+interval '6 days' WHERE id=$1", id); e != nil {
		f.f.f.t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "ALTER TABLE upload_cleanup_plans ENABLE TRIGGER upload_cleanup_plan_guard"); e != nil {
		f.f.f.t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		f.f.f.t.Fatal(e)
	}
}
func (f *cleanupFixture) approve(p storage.CleanupPlan) storage.CleanupPlan {
	f.f.f.t.Helper()
	f.ready(p.ID)
	r, v := f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "explicit fixture approval"}, nil)
	expect(f.f.f.t, r, 202, v)
	return data[storage.CleanupPlan](f.f.f.t, v)
}
func (f *cleanupFixture) plan(id string) storage.CleanupPlan {
	f.f.f.t.Helper()
	r, v := f.request("GET", "/v1/upload-cleanup-plans/"+id, f.f.f.aliceToken, "", nil, nil)
	expect(f.f.f.t, r, 200, v)
	return data[storage.CleanupPlan](f.f.f.t, v)
}
func (f *cleanupFixture) run() {
	f.f.f.t.Helper()
	worked, e := f.service.RunOne(context.Background(), "cleanup-fixture")
	if !worked || e != nil {
		f.f.f.t.Fatal("cleanup worker did not complete iteration", worked, e)
	}
}

func TestMultipartCleanupFourHumanClients(t *testing.T) {
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			f := newCleanupFixture(t, channel)
			r, v := f.request("GET", "/v1/folders/"+f.f.root+"/upload-cleanup-candidates?limit=1", f.f.f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			candidates := data[core.Page[storage.CleanupCandidate]](t, v)
			if len(candidates.Items) != 1 || !candidates.Items[0].Eligible {
				t.Fatal("eligible upload missing", candidates)
			}
			p := f.create()
			if p.State != "observing" || len(p.Items) != 1 || p.ReadyAfter.Sub(p.CreatedAt) < 24*time.Hour {
				t.Fatal("selection or observation not fixed", p)
			}
			r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "premature approval"}, nil)
			expect(t, r, 409, v)
			if v.Error.Code != "CLEANUP_OBSERVATION_ACTIVE" {
				t.Fatal(v.Error)
			}
			p = f.approve(p)
			f.run()
			p = f.plan(p.ID)
			if p.Items[0].State != "succeeded" || p.Items[0].ConfirmedAt == nil || f.cloud.aborts != 1 || f.cloud.inspections != 2 {
				t.Fatal("absence not confirmed", p.Items, f.cloud)
			}
			r, v = f.request("GET", "/v1/upload-cleanup-items/"+p.Items[0].ID+"/receipts?limit=1", f.f.f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			receipts := data[core.Page[storage.CleanupReceipt]](t, v)
			if len(receipts.Items) != 1 || receipts.Items[0].Outcome != "confirmed_absent" || receipts.NextCursor == nil {
				t.Fatal("receipt history lost", receipts)
			}
			r, v = f.request("GET", "/v1/upload-cleanup-items/"+p.Items[0].ID+"/receipts?limit=1&cursor="+*receipts.NextCursor, f.f.f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			if page := data[core.Page[storage.CleanupReceipt]](t, v); len(page.Items) != 1 || page.Items[0].Outcome != "started" {
				t.Fatal("start evidence missing", page)
			}
			if _, e := f.f.f.api.DB.Exec(context.Background(), "UPDATE upload_cleanup_receipts SET outcome='failed' WHERE item_id=$1", p.Items[0].ID); e == nil {
				t.Fatal("immutable receipts changed")
			}
			if _, e := f.f.f.api.DB.Exec(context.Background(), "UPDATE upload_cleanup_items SET state='queued',version=version+1,confirmed_at=NULL WHERE id=$1", p.Items[0].ID); e == nil {
				t.Fatal("confirmed cleanup rewritten")
			}
			if channel == "android" && os.Getenv("CP_BROWSER_MULTIPART_CLEANUP_FIXTURE") == "1" {
				fresh := f.f.prepared(f.f.create("Synthetic abandoned upload.bin", 20))
				if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE upload_sessions SET created_at=clock_timestamp()-interval '4 days',expires_at=clock_timestamp()-interval '2 days' WHERE id=$1", fresh.ID); e != nil {
					t.Fatal(e)
				}
				holdBrowserBusinessFixture(t, f.f.f, map[string]any{"multipart_cleanup_fixture": true, "folder_id": f.f.root, "completed_plan_id": p.ID, "eligible_upload_id": fresh.ID, "fixture_scope": "isolated native PG, synthetic cloud receipts; observation clock advanced by fixture; no live cloud deletion"})
			}
		})
	}
}
func TestMultipartCleanupAuthorizationAndSourceChanges(t *testing.T) {
	t.Run("hidden scope stale revision and changed policy cannot authorize cleanup", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		r, v := f.request("GET", "/v1/folders/"+f.f.root+"/upload-cleanup-candidates", f.f.f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		p := f.create()
		r, v = f.request("GET", "/v1/upload-cleanup-plans/"+p.ID, f.f.f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		if _, e := f.f.f.api.DB.Exec(context.Background(), "UPDATE upload_cleanup_plans SET snapshot='{}',version=version+1 WHERE id=$1", p.ID); e == nil {
			t.Fatal("immutable selection rewritten")
		}
		f.ready(p.ID)
		r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": strings.Repeat("b", 64), "reason": "wrong digest"}, nil)
		expect(t, r, 409, v)
		if _, e := f.f.f.owner.Exec(context.Background(), `INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason)
   VALUES($1,$2,2,'multipart',true,172800,86400,$3,'changed isolated policy')`, core.NewID(), f.f.f.org, f.f.f.admin); e != nil {
			t.Fatal(e)
		}
		r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "stale policy"}, nil)
		expect(t, r, 409, v)
		if v.Error.Code != "CLEANUP_SELECTION_CHANGED" || f.cloud.aborts != 0 {
			t.Fatal("changed selection used", v.Error)
		}
	})
	t.Run("expired MFA cannot approve and cancellation prevents queued execution", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.create()
		f.ready(p.ID)
		if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
			t.Fatal(e)
		}
		r, v := f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "requires fresh MFA"}, nil)
		expect(t, r, 403, v)
		if v.Error.Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(v.Error)
		}
		if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
			t.Fatal(e)
		}
		p = f.approve(p)
		r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/cancel", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "stop queued work"}, nil)
		expect(t, r, 200, v)
		f.run()
		p = f.plan(p.ID)
		if p.State != "cancelled" || p.Items[0].State != "blocked" || f.cloud.aborts != 0 {
			t.Fatal("cancelled plan executed", p)
		}
	})
	t.Run("revoked approval session blocks worker before cloud access", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE sessions SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.f.f.org, f.f.f.alice); e != nil {
			t.Fatal(e)
		}
		f.run()
		var state string
		if e := f.f.f.owner.QueryRow(context.Background(), "SELECT state FROM upload_cleanup_items WHERE id=$1", p.Items[0].ID).Scan(&state); e != nil || state != "blocked" || f.cloud.inspections != 0 {
			t.Fatal("revoked approval executed", state, e)
		}
	})
}
func TestMultipartCleanupRetriesAndReceipts(t *testing.T) {
	t.Run("denial stays failed and explicit retry preserves old results", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		f.cloud.inspectError = &objectstore.Error{Code: "PROVIDER_ACCESS_DENIED"}
		f.run()
		p = f.plan(p.ID)
		if p.Items[0].State != "failed" || f.cloud.aborts != 0 {
			t.Fatal("provider denial claimed success", p.Items)
		}
		old := p.Items[0]
		f.cloud.inspectError = nil
		r, v := f.request("POST", "/v1/upload-cleanup-items/"+old.ID+"/retry", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": old.Version}, nil)
		expect(t, r, 202, v)
		f.run()
		p = f.plan(p.ID)
		if p.Items[0].State != "succeeded" || p.Items[0].Generation != 2 || p.Items[0].Attempts != 2 {
			t.Fatal("explicit retry lost generation history", p.Items)
		}
	})
	t.Run("uncertain abort retries instead of inventing released bytes", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		f.cloud.abortError = &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true, Uncertain: true}
		f.run()
		p = f.plan(p.ID)
		if p.Items[0].State != "retry_wait" || p.Items[0].ConfirmedAt != nil {
			t.Fatal("uncertain abort counted as success", p.Items)
		}
		if _, e := f.f.f.owner.Exec(context.Background(), "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE kind=$1", storage.CleanupJobKind); e != nil {
			t.Fatal(e)
		}
		f.run()
		p = f.plan(p.ID)
		if p.Items[0].State != "succeeded" || p.Items[0].Attempts != 2 {
			t.Fatal(p.Items)
		}
	})
	t.Run("receipt audit rollback reconciles absence without a second abort", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		ctx := context.Background()
		if _, e := f.f.f.owner.Exec(ctx, `CREATE FUNCTION reject_cleanup_receipt_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
   IF NEW.action='upload.cleanup.confirmed_absent' THEN RAISE EXCEPTION 'isolated fixture'; END IF; RETURN NEW; END; $$;
   CREATE TRIGGER reject_cleanup_receipt_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_cleanup_receipt_fixture();`); e != nil {
			t.Fatal(e)
		}
		worked, e := f.service.RunOne(ctx, "cleanup-fixture")
		if !worked || e == nil {
			t.Fatal("audit failure not surfaced", worked, e)
		}
		p = f.plan(p.ID)
		if p.Items[0].State != "retry_wait" || p.Items[0].ConfirmedAt != nil || f.cloud.aborts != 1 {
			t.Fatal("audit rollback misreported cleanup", p.Items)
		}
		if _, e = f.f.f.owner.Exec(ctx, "DROP TRIGGER reject_cleanup_receipt_fixture ON audit_events; DROP FUNCTION reject_cleanup_receipt_fixture(); UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE kind='upload.multipart.cleanup.v1'"); e != nil {
			t.Fatal(e)
		}
		f.run()
		p = f.plan(p.ID)
		if p.Items[0].State != "succeeded" || f.cloud.aborts != 1 {
			t.Fatal("unknown outcome not reconciled", p.Items, f.cloud.aborts)
		}
	})
	t.Run("expired lease cannot perform cloud operations", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		q := jobqueue.Queue{DB: f.f.f.api.DB}
		ctx := context.Background()
		j, e := q.Claim(ctx, []string{storage.CleanupJobKind}, storage.CleanupJobLane, "old-cleaner", time.Minute)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		if _, e = f.f.f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); e != nil {
			t.Fatal(e)
		}
		if e = f.service.Process(ctx, *j); e == nil || core.PublicError(e).Code != "JOB_LEASE_LOST" || f.cloud.inspections != 0 {
			t.Fatal("stale fence executed", e)
		}
		f.run()
		if current := f.plan(p.ID); current.Items[0].State != "succeeded" || f.cloud.aborts != 1 {
			t.Fatal(current.Items)
		}
	})
}

func TestMultipartCleanupAtomicityAndExhaustion(t *testing.T) {
	t.Run("approval audit failure rolls back reservations and replay queues exactly once", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.create()
		f.ready(p.ID)
		ctx := context.Background()
		if _, e := f.f.f.owner.Exec(ctx, `CREATE FUNCTION reject_cleanup_approval_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
  IF NEW.action='upload.cleanup.approved' THEN RAISE EXCEPTION 'isolated fixture'; END IF; RETURN NEW; END; $$;
  CREATE TRIGGER reject_cleanup_approval_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_cleanup_approval_fixture();`); e != nil {
			t.Fatal(e)
		}
		input := map[string]any{"expected_version": p.Version, "snapshot_sha256": p.SnapshotSHA256, "reason": "atomic fixture approval"}
		key := core.NewID()
		r, v := f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, key, input, nil)
		expect(t, r, 500, v)
		if current := f.plan(p.ID); current.State != "observing" || current.Items[0].State != "observing" {
			t.Fatal("approval escaped rollback", current.State, current.Items)
		}
		var count int
		if e := f.f.f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind=$1", storage.CleanupJobKind).Scan(&count); e != nil || count != 0 {
			t.Fatal("jobs escaped rollback", count, e)
		}
		if _, e := f.f.f.owner.Exec(ctx, "DROP TRIGGER reject_cleanup_approval_fixture ON audit_events; DROP FUNCTION reject_cleanup_approval_fixture()"); e != nil {
			t.Fatal(e)
		}
		r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, key, input, nil)
		expect(t, r, 202, v)
		r, v = f.request("POST", "/v1/upload-cleanup-plans/"+p.ID+"/approve", f.f.f.aliceToken, key, input, nil)
		expect(t, r, 202, v)
		if r.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("approval not replayed")
		}
		if e := f.f.f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind=$1", storage.CleanupJobKind).Scan(&count); e != nil || count != 1 {
			t.Fatal("duplicate cleanup jobs", count, e)
		}
	})
	t.Run("changed provider binding blocks approved work before I O", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		f.service.Backends["oss"].BindingSHA256 = strings.Repeat("b", 64)
		f.run()
		if current := f.plan(p.ID); current.Items[0].State != "blocked" || f.cloud.inspections != 0 || f.cloud.aborts != 0 {
			t.Fatal("changed binding executed", current.Items)
		}
	})
	t.Run("crashed final attempt has an immutable failure receipt and explicit retry", func(t *testing.T) {
		f := newCleanupFixture(t, "android")
		p := f.approve(f.create())
		ctx := context.Background()
		q := jobqueue.Queue{DB: f.f.f.api.DB}
		j, e := q.Claim(ctx, []string{storage.CleanupJobKind}, storage.CleanupJobLane, "crashed-cleaner", time.Minute)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		if _, e = f.f.f.owner.Exec(ctx, "UPDATE jobs SET attempt=max_attempts,lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); e != nil {
			t.Fatal(e)
		}
		worked, e := f.service.RunOne(ctx, "sweeper")
		if e != nil || worked {
			t.Fatal(worked, e)
		}
		p = f.plan(p.ID)
		if p.Items[0].State != "failed" || p.Items[0].LastError == nil || *p.Items[0].LastError != "CLEANUP_ATTEMPTS_EXHAUSTED" || f.cloud.inspections != 0 {
			t.Fatal(p.Items)
		}
		r, v := f.request("GET", "/v1/upload-cleanup-items/"+p.Items[0].ID+"/receipts", f.f.f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		rows := data[core.Page[storage.CleanupReceipt]](t, v)
		if len(rows.Items) != 1 || rows.Items[0].Outcome != "failed" || rows.Items[0].Attempt != 8 {
			t.Fatal(rows)
		}
		r, v = f.request("POST", "/v1/upload-cleanup-items/"+p.Items[0].ID+"/retry", f.f.f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Items[0].Version}, nil)
		expect(t, r, 202, v)
		f.run()
		if current := f.plan(p.ID); current.Items[0].State != "succeeded" || current.Items[0].Generation != 2 {
			t.Fatal(current.Items)
		}
	})
}
