package httpapi

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
)

// Controlled provider metadata, not cloud I/O or a large-file transport proof.
type uploadCloudFixture struct {
	mu                              sync.Mutex
	uploads                         map[string]objectstore.Multipart
	parts                           map[string][]objectstore.Part
	objects                         map[string]objectstore.Source
	begins, signs, lists, completes int
	onBegin                         func()
	completeTimeout                 bool
	beginError                      error
}

func (s *uploadCloudFixture) Begin(ctx context.Context, key, media string) (objectstore.Multipart, error) {
	s.mu.Lock()
	s.begins++
	if s.beginError != nil {
		err := s.beginError
		s.mu.Unlock()
		return objectstore.Multipart{}, err
	}
	m := objectstore.Multipart{Key: key, UploadID: fmt.Sprintf("fixture-upload-%d", s.begins)}
	s.uploads[m.UploadID] = m
	callback := s.onBegin
	s.onBegin = nil
	s.mu.Unlock()
	if callback != nil {
		callback()
	}
	return m, nil
}
func (s *uploadCloudFixture) SignPart(ctx context.Context, m objectstore.Multipart, n int, size int64, ttl time.Duration) (objectstore.Signed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signs++
	return objectstore.Signed{Method: "PUT", URL: fmt.Sprintf("https://fixture.invalid/draft?secret=ephemeral-%d&part=%d", s.signs, n), Headers: map[string]string{}, ExpiresAt: time.Now().UTC().Add(ttl)}, nil
}
func (s *uploadCloudFixture) Parts(ctx context.Context, m objectstore.Multipart, marker string) (objectstore.PartPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists++
	return objectstore.PartPage{Items: append([]objectstore.Part(nil), s.parts[m.UploadID]...)}, nil
}
func (s *uploadCloudFixture) Complete(ctx context.Context, m objectstore.Multipart, parts []objectstore.Part) (objectstore.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completes++
	var size int64
	for _, p := range parts {
		size += p.Size
	}
	source := objectstore.Source{Key: m.Key, ETag: "fixture-frozen-etag", Size: size}
	s.objects[m.Key] = source
	if s.completeTimeout {
		s.completeTimeout = false
		return objectstore.Source{}, &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true, Uncertain: true}
	}
	return source, nil
}
func (s *uploadCloudFixture) Head(ctx context.Context, key string) (objectstore.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.objects[key]
	if !ok {
		return objectstore.Source{}, &objectstore.Error{Code: "OBJECT_NOT_FOUND"}
	}
	return value, nil
}
func (s *uploadCloudFixture) Abort(context.Context, objectstore.Multipart) error { return nil }
func (s *uploadCloudFixture) Read(context.Context, objectstore.Source, int64, int64) (objectstore.ReadResult, error) {
	return objectstore.ReadResult{}, fmt.Errorf("metadata fixture has no object bytes")
}
func (s *uploadCloudFixture) WritePart(context.Context, objectstore.Multipart, int, io.Reader, int64) (objectstore.Part, error) {
	return objectstore.Part{}, fmt.Errorf("metadata fixture cannot transfer bytes")
}

type uploadFixture struct {
	f       *fixture
	service *storage.UploadService
	cloud   *uploadCloudFixture
	root    string
}

func newUploadFixture(t *testing.T) *uploadFixture {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Upload control fixture", []string{f.bob})
	var root string
	if err := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, p.ID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,'oss',$2,'isolated provider metadata fixture')", f.org, f.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO storage_quotas(org_id,logical_limit_bytes) VALUES($1,1000000000000)", f.org); err != nil {
		t.Fatal(err)
	}
	cloud := &uploadCloudFixture{uploads: map[string]objectstore.Multipart{}, parts: map[string][]objectstore.Part{}, objects: map[string]objectstore.Source{}}
	backend := &storage.UploadBackend{Provider: "oss", Bucket: "fixture-bucket", Signer: cloud, Lister: cloud}
	f.api.Storage = backend
	f.handler = f.api.Handler()
	service := &storage.UploadService{Files: &storage.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}, Queue: jobqueue.Queue{DB: f.api.DB}, Backend: backend}
	return &uploadFixture{f: f, service: service, cloud: cloud, root: root}
}
func (f *uploadFixture) create(name string, size int64) storage.Upload {
	f.f.t.Helper()
	rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{
		"folder_id": f.root, "name": name, "size_bytes": strconv.FormatInt(size, 10), "media_type": "application/octet-stream", "source_fingerprint": "fixture-v1/source-" + name}, nil)
	expect(f.f.t, rec, 201, result)
	return data[storage.Upload](f.f.t, result)
}
func (f *uploadFixture) get(id string) storage.Upload {
	rec, result := f.f.request("GET", "/v1/uploads/"+id, f.f.aliceToken, "", nil, nil)
	expect(f.f.t, rec, 200, result)
	return data[storage.Upload](f.f.t, result)
}
func (f *uploadFixture) job() jobqueue.Job {
	f.f.t.Helper()
	ctx := context.Background()
	delivered, err := f.service.Queue.DispatchOne(ctx, storage.UploadConsumer())
	if err != nil || delivered == nil || delivered.Rejected {
		f.f.t.Fatal("upload event not accepted", err, delivered)
	}
	j, err := f.service.Queue.Claim(ctx, storage.UploadJobKinds, storage.UploadLane, "upload-fixture", 2*time.Minute)
	if err != nil || j == nil {
		f.f.t.Fatal("upload task missing", err)
	}
	return *j
}
func (f *uploadFixture) prepared(u storage.Upload) storage.Upload {
	j := f.job()
	if err := f.service.RunUploadJob(context.Background(), j, f.cloud); err != nil {
		f.f.t.Fatal(err)
	}
	return f.get(u.ID)
}
func TestUploadControl(t *testing.T) {
	ctx := context.Background()
	t.Run("admission requires explicit storage and capacity and limits platforms", func(t *testing.T) {
		f := newUploadFixture(t)
		input := map[string]any{"folder_id": f.root, "name": "large.bin", "size_bytes": "120000000000", "media_type": "application/octet-stream", "source_fingerprint": "fixture-v1/large"}
		f.f.api.Storage = nil
		f.f.handler = f.f.api.Handler()
		rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 503, result)
		f.f.api.Storage = f.service.Backend
		f.f.handler = f.f.api.Handler()
		if _, err := f.f.owner.Exec(ctx, "DELETE FROM storage_quotas WHERE org_id=$1", f.f.org); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 503, result)
		if result.Error.Code != "STORAGE_QUOTA_NOT_CONFIGURED" {
			t.Fatal(result.Error)
		}
		if _, err := f.f.owner.Exec(ctx, "INSERT INTO storage_quotas(org_id,logical_limit_bytes) VALUES($1,1000000000000)", f.f.org); err != nil {
			t.Fatal(err)
		}
		mini := f.f.login("alice", "wechat").AccessToken
		rec, result = f.f.request("POST", "/v1/uploads", mini, core.NewID(), input, nil)
		expect(t, rec, 403, result)
		key := core.NewID()
		rec, result = f.f.request("POST", "/v1/uploads", f.f.aliceToken, key, input, nil)
		expect(t, rec, 201, result)
		u := data[storage.Upload](t, result)
		if u.State != "created" || u.PartCount != 3577 || u.PartBytes != 32<<20 || len(u.ConfirmedParts) != 0 || u.SizeBytes != "120000000000" {
			t.Fatal("120 GB metadata layout incorrect", u)
		}
		rec, result = f.f.request("POST", "/v1/uploads", f.f.aliceToken, key, input, nil)
		expect(t, rec, 201, result)
		if data[storage.Upload](t, result).ID != u.ID || rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("session replay duplicated")
		}
		input["size_bytes"] = "120000000001"
		rec, result = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 422, result)
		if f.cloud.begins != 0 {
			t.Fatal("cloud RPC ran inside creation command")
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE upload_sessions SET source_fingerprint='replaced' WHERE id=$1", u.ID); err == nil {
			t.Fatal("source fingerprint was mutable")
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE storage_quotas SET logical_limit_bytes=1 WHERE org_id=$1", f.f.org); err == nil {
			t.Fatal("application changed operator quota")
		}
	})
	t.Run("quota reservations serialize concurrent uploads", func(t *testing.T) {
		f := newUploadFixture(t)
		if _, err := f.f.owner.Exec(ctx, "UPDATE storage_quotas SET logical_limit_bytes=20 WHERE org_id=$1", f.f.org); err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		statuses := make(chan int, 2)
		for _, name := range []string{"one.bin", "two.bin"} {
			group.Add(1)
			go func(name string) {
				defer group.Done()
				rec, _ := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{"folder_id": f.root, "name": name, "size_bytes": "15", "media_type": "application/octet-stream", "source_fingerprint": name}, nil)
				statuses <- rec.Code
			}(name)
		}
		group.Wait()
		close(statuses)
		success, conflict := 0, 0
		for code := range statuses {
			if code == 201 {
				success++
			}
			if code == 409 {
				conflict++
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal("quota overreserved", success, conflict)
		}
	})
	t.Run("part hints do not prove cloud progress and signed secrets are never persisted", func(t *testing.T) {
		f := newUploadFixture(t)
		if _, err := f.f.owner.Exec(ctx, `INSERT INTO classification_policies(id,org_id,level,policy_version,ai_allowed,allowed_channels,base_policy_id)
            SELECT gen_random_uuid(),org_id,level,policy_version+1,ai_allowed,ARRAY['android'],id
            FROM (SELECT DISTINCT ON(level) * FROM classification_policies WHERE org_id=$1 ORDER BY level,policy_version DESC) current_policy`, f.f.org); err != nil {
			t.Fatal(err)
		}
		u := f.prepared(f.create("resume.bin", 15))
		if u.State != "uploading" || u.Version != 2 {
			t.Fatal("prepare not checkpointed", u)
		}
		rec, result := f.f.request("GET", "/v1/uploads/"+u.ID, f.f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		body := map[string]any{"expected_version": u.Version, "part_numbers": []int{1}}
		key := core.NewID()
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		first := data[storage.SignedParts](t, result)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		second := data[storage.SignedParts](t, result)
		if len(first.Items) != 1 || first.Items[0].URL == second.Items[0].URL {
			t.Fatal("ephemeral permits not regenerated")
		}
		var leaked bool
		if err := f.f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM idempotency_records WHERE org_id=$1 AND response_data::text LIKE '%ephemeral-%')", f.f.org).Scan(&leaked); err != nil || leaked {
			t.Fatal("signed URL persisted", err)
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/ack-parts", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "parts": []map[string]any{{"part_number": 1, "size_bytes": "15", "etag": "client-claim"}}}, nil)
		expect(t, rec, 200, result)
		if len(data[storage.Upload](t, result).ConfirmedParts) != 0 || data[storage.Upload](t, result).Version != u.Version {
			t.Fatal("client hint was counted as cloud proof or changed lifecycle")
		}
		var uploadID string
		if err := f.f.owner.QueryRow(ctx, "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&uploadID); err != nil {
			t.Fatal(err)
		}
		f.cloud.parts[uploadID] = []objectstore.Part{{Number: 1, Size: 15, ETag: "cloud-proof"}}
		resume := map[string]any{"expected_version": u.Version, "source_fingerprint": "wrong-file"}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/resume", f.f.aliceToken, core.NewID(), resume, nil)
		expect(t, rec, 409, result)
		if f.cloud.lists != 0 {
			t.Fatal("wrong source performed cloud request")
		}
		resume["source_fingerprint"] = u.SourceFingerprint
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/resume", f.f.aliceToken, core.NewID(), resume, nil)
		expect(t, rec, 200, result)
		current := data[storage.Upload](t, result)
		if len(current.ConfirmedParts) != 1 || current.ConfirmedParts[0] != 1 || current.Version != u.Version {
			t.Fatal("cloud reconciliation missing")
		}
		if strings.Contains(string(result.Data), "fixture-upload-") || strings.Contains(string(result.Data), "fixture-bucket") {
			t.Fatal("private storage metadata exposed")
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "part_numbers": []int{2}}, nil)
		expect(t, rec, 422, result)
	})
	t.Run("incomplete finalization returns to uploading and can finalize a new lifecycle", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.prepared(f.create("incomplete.bin", 15))
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/finalize", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version}, nil)
		expect(t, rec, 202, result)
		first := f.job()
		if err := f.service.RunUploadJob(ctx, first, f.cloud); err != nil {
			t.Fatal(err)
		}
		current := f.get(u.ID)
		if current.State != "uploading" || current.Version != 4 || current.ErrorCode == nil || *current.ErrorCode != "INCOMPLETE_PARTS" || f.cloud.completes != 0 {
			t.Fatal("missing parts stranded or completed session", current)
		}
		var uploadID string
		if err := f.f.owner.QueryRow(ctx, "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&uploadID); err != nil {
			t.Fatal(err)
		}
		f.cloud.parts[uploadID] = []objectstore.Part{{Number: 1, Size: 15, ETag: "late-part"}}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/finalize", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.Version}, nil)
		expect(t, rec, 202, result)
		second := f.job()
		if first.ID == second.ID {
			t.Fatal("new finalization reused a completed lifecycle task")
		}
		if err := f.service.RunUploadJob(ctx, second, f.cloud); err != nil {
			t.Fatal(err)
		}
		current = f.get(u.ID)
		if current.State != "verifying" || current.Version != 6 || current.ErrorCode != nil {
			t.Fatal("repaired upload did not freeze cleanly", current)
		}
	})
	t.Run("uncertain completion reconciles the immutable source before any repeat RPC", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.prepared(f.create("uncertain.bin", 15))
		var uploadID string
		if err := f.f.owner.QueryRow(ctx, "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&uploadID); err != nil {
			t.Fatal(err)
		}
		f.cloud.parts[uploadID] = []objectstore.Part{{Number: 1, Size: 15, ETag: "cloud-part"}}
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/finalize", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version}, nil)
		expect(t, rec, 202, result)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "part_numbers": []int{1}}, nil)
		expect(t, rec, 409, result)
		j := f.job()
		f.cloud.completeTimeout = true
		err := f.service.RunUploadJob(ctx, j, f.cloud)
		if err == nil || f.cloud.completes != 1 {
			t.Fatal("uncertain mutation not surfaced", err)
		}
		current := f.get(u.ID)
		if current.State != "finalizing" {
			t.Fatal("uncertain completion became committed", current)
		}
		if err = f.service.Queue.Fail(ctx, j, "PROVIDER_TIMEOUT", false, time.Second); err != nil {
			t.Fatal(err)
		}
		if _, err = f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); err != nil {
			t.Fatal(err)
		}
		reclaimed, err := f.service.Queue.Claim(ctx, storage.UploadJobKinds, storage.UploadLane, "upload-retry", 2*time.Minute)
		if err != nil || reclaimed == nil {
			t.Fatal("completion retry missing", err)
		}
		if err = f.service.RunUploadJob(ctx, *reclaimed, f.cloud); err != nil {
			t.Fatal(err)
		}
		current = f.get(u.ID)
		if current.State != "verifying" || current.Version != 4 || current.SHA256 != nil || current.ResultVersionID != nil || f.cloud.completes != 1 {
			t.Fatal("source not reconciled or content prematurely published", current, f.cloud.completes)
		}
		var count int
		if err = f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&count); err != nil || count != 0 {
			t.Fatal("unverified source made a file version", err)
		}
		if _, err = f.f.api.DB.Exec(ctx, "UPDATE upload_sessions SET frozen_etag='replaced' WHERE id=$1", u.ID); err == nil {
			t.Fatal("frozen source was mutable")
		}
		if _, err = f.f.api.DB.Exec(ctx, "UPDATE upload_sessions SET finalize_manifest='[]' WHERE id=$1", u.ID); err == nil {
			t.Fatal("sealed manifest was mutable")
		}
	})
	t.Run("old prepare worker cannot publish after lease takeover", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("fence.bin", 15)
		old := f.job()
		f.cloud.onBegin = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", old.ID); err != nil {
				t.Fatal(err)
			}
			fresh, err := f.service.Queue.Claim(ctx, storage.UploadJobKinds, storage.UploadLane, "new-upload-worker", 2*time.Minute)
			if err != nil || fresh == nil {
				t.Fatal("takeover missing", err)
			}
			if err = f.service.RunUploadJob(ctx, *fresh, f.cloud); err != nil {
				t.Fatal(err)
			}
		}
		err := f.service.RunUploadJob(ctx, old, f.cloud)
		if err == nil || core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old result was accepted", err)
		}
		var activeID string
		if err = f.f.owner.QueryRow(ctx, "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&activeID); err != nil || activeID != "fixture-upload-2" {
			t.Fatal("old multipart won", activeID, err)
		}
		var keys int
		if err = f.f.owner.QueryRow(ctx, "SELECT count(DISTINCT draft_key) FROM upload_attempts WHERE session_id=$1", u.ID).Scan(&keys); err != nil || keys != 2 {
			t.Fatal("attempts reused mutable cloud key", keys, err)
		}
	})
	t.Run("abort during external creation records a superseded receipt without reactivating", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("abort.bin", 15)
		j := f.job()
		f.cloud.onBegin = func() {
			rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/abort", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "action": "abort", "reason": "取消本次上传"}, nil)
			expect(t, rec, 200, result)
		}
		if err := f.service.RunUploadJob(ctx, j, f.cloud); err != nil {
			t.Fatal(err)
		}
		if current := f.get(u.ID); current.State != "aborted" {
			t.Fatal("aborted session reactivated", current)
		}
		var state, id string
		if err := f.f.owner.QueryRow(ctx, "SELECT state,provider_upload_id FROM upload_attempts WHERE session_id=$1", u.ID).Scan(&state, &id); err != nil || state != "superseded" || id == "" {
			t.Fatal("orphan cleanup receipt missing", state, err)
		}
	})
	t.Run("storage epoch changes stop fresh signatures and commands", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.prepared(f.create("epoch.bin", 15))
		if _, err := f.f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,2,'r2',$2,'test-only fence receipt')", f.f.org, f.f.admin); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/sign-parts", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version, "part_numbers": []int{1}}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "STORAGE_EPOCH_CHANGED" || f.cloud.signs != 0 {
			t.Fatal("old epoch signed", result.Error)
		}
	})
	t.Run("audit failure rolls back admission and outbox atomically", func(t *testing.T) {
		f := newUploadFixture(t)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_upload_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action='upload.created' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
            CREATE TRIGGER reject_upload_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_upload_fixture();`); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{"folder_id": f.root, "name": "rollback.bin", "size_bytes": "15", "media_type": "application/octet-stream", "source_fingerprint": "rollback-v1"}, nil)
		expect(t, rec, 500, result)
		var sessions, events int
		if err := f.f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM upload_sessions WHERE org_id=$1),
            (SELECT count(*) FROM outbox_events WHERE org_id=$1 AND event_type='upload.created')`, f.f.org).Scan(&sessions, &events); err != nil || sessions != 0 || events != 0 {
			t.Fatal("failed audit left upload state", sessions, events, err)
		}
	})
}
