package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

type promotionCloud struct {
	*integrityCloud
	failPart int
	writes   []int
	corrupt  bool
	onWrite  func()
}

func newPromotionCloud(f *uploadFixture) *promotionCloud {
	return &promotionCloud{integrityCloud: &integrityCloud{uploadCloudFixture: &uploadCloudFixture{
		uploads: map[string]objectstore.Multipart{}, parts: map[string][]objectstore.Part{}, objects: map[string]objectstore.Source{},
	}}}
}
func (s *promotionCloud) WritePart(ctx context.Context, m objectstore.Multipart, number int, reader io.Reader, size int64) (objectstore.Part, error) {
	s.writes = append(s.writes, number)
	if s.failPart == number {
		s.failPart = 0
		return objectstore.Part{}, &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true, Uncertain: true}
	}
	n, err := io.Copy(io.Discard, reader)
	if err != nil {
		return objectstore.Part{}, err
	}
	if n != size {
		return objectstore.Part{}, fmt.Errorf("fixture short content")
	}
	part := objectstore.Part{Number: number, Size: size, ETag: fmt.Sprintf("target-part-%d", number)}
	s.mu.Lock()
	found := false
	for i, p := range s.parts[m.UploadID] {
		if p.Number == number {
			s.parts[m.UploadID][i] = part
			found = true
		}
	}
	if !found {
		s.parts[m.UploadID] = append(s.parts[m.UploadID], part)
	}
	s.mu.Unlock()
	if callback := s.onWrite; callback != nil {
		s.onWrite = nil
		callback()
	}
	return part, nil
}

type alteredBytes struct{ io.ReadCloser }

func (r alteredBytes) Read(p []byte) (int, error) {
	n, e := r.ReadCloser.Read(p)
	for i := 0; i < n; i++ {
		p[i] = 1
	}
	return n, e
}
func (s *promotionCloud) Read(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
	result, err := s.integrityCloud.Read(ctx, source, offset, length)
	if err == nil && s.corrupt {
		result.Body = alteredBytes{result.Body}
	}
	return result, err
}
func promotionHost(f *uploadFixture, from *integrityCloud, to *promotionCloud) *uploadworker.Runner {
	return &uploadworker.Runner{Service: f.service, Store: from, TargetStore: to, Owner: "promotion-host-fixture", Mode: "promote-small", Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}
func scannedUpload(f *uploadFixture, u storage.Upload) *integrityCloud {
	f.f.t.Helper()
	from := &integrityCloud{uploadCloudFixture: f.cloud}
	if worked, err := integrityHost(f, from, cleanIntegrityScanner()).RunOne(context.Background()); err != nil || !worked {
		f.f.t.Fatal("scan prerequisite failed", err)
	}
	current := f.get(u.ID)
	if current.ScanState != "clean" || current.SHA256 == nil {
		f.f.t.Fatal("scan prerequisite incomplete", current)
	}
	return from
}
func readyUpload(f *uploadFixture, name string, size int64) (storage.Upload, *integrityCloud, *promotionCloud) {
	u := f.frozen(name, size)
	from := scannedUpload(f, u)
	to := newPromotionCloud(f)
	if worked, err := promotionHost(f, from, to).RunOne(context.Background()); err != nil || !worked {
		f.f.t.Fatal("promotion not executed", err)
	}
	u = f.get(u.ID)
	if u.State != "ready" {
		f.f.t.Fatal("promotion did not produce ready candidate", u)
	}
	return u, from, to
}
func configureStorageTargets(f *uploadFixture) {
	f.f.t.Helper()
	if _, err := f.f.owner.Exec(context.Background(), "INSERT INTO storage_targets(org_id,provider,bucket) VALUES($1,'oss','fixture-bucket'),($1,'r2','fixture-r2-bucket')", f.f.org); err != nil {
		f.f.t.Fatal(err)
	}
}
func commitInput(u storage.Upload) map[string]any {
	body := map[string]any{"expected_version": u.Version, "sha256": *u.SHA256}
	if u.BaseVersionID != nil {
		body["base_version_id"] = *u.BaseVersionID
	}
	return body
}
func TestUploadPromotionAndCommit(t *testing.T) {
	ctx := context.Background()
	t.Run("ready requires target readback and commit is atomic idempotent and queues a separate replica", func(t *testing.T) {
		f := newUploadFixture(t)
		u, from, to := readyUpload(f, "formal.bin", 15)
		if len(to.writes) != 1 || len(to.offsets) != 1 || len(from.offsets) != 3 {
			t.Fatal("candidate was not copied and read back", to.writes, to.offsets, from.offsets)
		}
		var versions int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&versions); err != nil || versions != 0 {
			t.Fatal("ready created formal metadata", versions, err)
		}
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		expect(t, rec, 503, result)
		configureStorageTargets(f)
		body := commitInput(u)
		body["sha256"] = core.Digest([]byte("wrong"))
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 409, result)
		key := core.NewID()
		body = commitInput(u)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 201, result)
		committed := data[storage.CommitResult](t, result)
		if !committed.Committed || committed.File.CurrentVersionID == nil || *committed.File.CurrentVersionID != committed.Version.ID || committed.Version.ReplicaState == nil || *committed.Version.ReplicaState != "queued" {
			t.Fatal("formal result misrepresented", committed)
		}
		current := f.get(u.ID)
		if current.State != "committed" || current.ResultVersionID == nil || *current.ResultVersionID != committed.Version.ID {
			t.Fatal("session commit missing", current)
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 201, result)
		if rec.Header().Get("X-Idempotent-Replay") != "true" || data[storage.CommitResult](t, result).Version.ID != committed.Version.ID {
			t.Fatal("commit replay duplicated")
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&versions); err != nil || versions != 1 {
			t.Fatal("formal version count", versions, err)
		}
		var primary, secondary int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FILTER(WHERE state='verified'),count(*) FILTER(WHERE state='queued') FROM file_replicas WHERE version_id=$1", committed.Version.ID).Scan(&primary, &secondary); err != nil || primary != 1 || secondary != 1 {
			t.Fatal("replica evidence incorrect", primary, secondary, err)
		}
		if to.completes != 1 || len(to.offsets) != 1 {
			t.Fatal("commit made additional cloud calls")
		}
	})
	t.Run("partial copy resumes the same multipart and skips confirmed source ranges", func(t *testing.T) {
		f := newUploadFixture(t)
		size := objectstore.DefaultPartBytes + 17
		u := f.frozen("resume-copy.bin", size)
		from := scannedUpload(f, u)
		from.offsets = nil
		to := newPromotionCloud(f)
		to.failPart = 2
		host := promotionHost(f, from, to)
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("copy interruption not attempted", err)
		}
		if current := f.get(u.ID); current.State != "verifying" {
			t.Fatal("partial copy became ready", current)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='upload.promote.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("copy retry not attempted", err)
		}
		if current := f.get(u.ID); current.State != "ready" {
			t.Fatal("copy did not recover", current)
		}
		if to.begins != 1 || len(from.offsets) != 3 || from.offsets[0] != 0 || from.offsets[1] != objectstore.DefaultPartBytes || from.offsets[2] != objectstore.DefaultPartBytes {
			t.Fatal("copy restarted a confirmed prefix", to.begins, from.offsets)
		}
		if len(to.writes) != 3 || to.writes[0] != 1 || to.writes[1] != 2 || to.writes[2] != 2 {
			t.Fatal("cloud confirmed part recopied", to.writes)
		}
	})
	t.Run("unknown candidate completion is reconciled by unique key before another completion", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("uncertain-candidate.bin", 15)
		from := scannedUpload(f, u)
		to := newPromotionCloud(f)
		to.completeTimeout = true
		host := promotionHost(f, from, to)
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		if current := f.get(u.ID); current.State != "verifying" {
			t.Fatal("unknown completion accepted", current)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='upload.promote.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		if current := f.get(u.ID); current.State != "ready" || to.completes != 1 || to.begins != 1 {
			t.Fatal("unknown completion repeated mutation", current, to.completes, to.begins)
		}
	})
	t.Run("matching length and ETag cannot hide corrupted target bytes", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("corrupt-target.bin", 15)
		from := scannedUpload(f, u)
		to := newPromotionCloud(f)
		to.corrupt = true
		if worked, err := promotionHost(f, from, to).RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		current := f.get(u.ID)
		if current.State != "failed" || current.ResultVersionID != nil || current.ErrorCode == nil || *current.ErrorCode != "CONTENT_DIGEST_MISMATCH" {
			t.Fatal("corrupt candidate accepted", current)
		}
	})
	t.Run("name collision preserves ready candidate and audit rejection rolls back commit", func(t *testing.T) {
		f := newUploadFixture(t)
		u, _, _ := readyUpload(f, "collision.bin", 15)
		configureStorageTargets(f)
		rec, result := f.f.request("POST", "/v1/folders", f.f.aliceToken, core.NewID(), map[string]any{"parent_id": f.root, "name": u.Name, "classification": "project_restricted"}, nil)
		expect(t, rec, 201, result)
		body := commitInput(u)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 409, result)
		if current := f.get(u.ID); current.State != "ready" {
			t.Fatal("collision destroyed candidate", current)
		}
		u2, _, _ := readyUpload(f, "atomic-commit.bin", 15)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_commit_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action='upload.committed' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
			CREATE TRIGGER reject_commit_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_commit_fixture();`); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+u2.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u2), nil)
		expect(t, rec, 500, result)
		var count int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM files WHERE org_id=$1", f.f.org).Scan(&count); err != nil || count != 0 {
			t.Fatal("commit audit left file metadata", count, err)
		}
		if current := f.get(u2.ID); current.State != "ready" {
			t.Fatal("failed audit consumed candidate", current)
		}
	})
	t.Run("two ready replacements cannot both advance one base version", func(t *testing.T) {
		f := newUploadFixture(t)
		first, _, _ := readyUpload(f, "base.bin", 15)
		configureStorageTargets(f)
		rec, result := f.f.request("POST", "/v1/uploads/"+first.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(first), nil)
		expect(t, rec, 201, result)
		base := data[storage.CommitResult](t, result)
		var candidates []storage.Upload
		for n := 0; n < 2; n++ {
			rec, result = f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{
				"folder_id": f.root, "name": "replace-" + strconv.Itoa(n) + ".bin", "size_bytes": "15", "media_type": "application/octet-stream",
				"source_fingerprint": "replace-" + strconv.Itoa(n), "file_id": base.File.ID, "base_version_id": base.Version.ID}, nil)
			expect(t, rec, 201, result)
			u := f.freezeUpload(data[storage.Upload](t, result))
			from := scannedUpload(f, u)
			if worked, err := promotionHost(f, from, newPromotionCloud(f)).RunOne(ctx); err != nil || !worked {
				t.Fatal(err)
			}
			candidates = append(candidates, f.get(u.ID))
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+candidates[0].ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(candidates[0]), nil)
		expect(t, rec, 201, result)
		if version := data[storage.CommitResult](t, result).Version; version.Seq != 2 || version.BaseVersionID == nil || *version.BaseVersionID != base.Version.ID {
			t.Fatal("replacement lost ancestry", version)
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+candidates[1].ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(candidates[1]), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "BASE_VERSION_CONFLICT" {
			t.Fatal("wrong conflict", result.Error)
		}
		if current := f.get(candidates[1].ID); current.State != "ready" {
			t.Fatal("conflicting candidate lost", current)
		}
	})
	t.Run("abort during candidate upload cannot create ready or formal state", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("abort-copy.bin", 15)
		from := scannedUpload(f, u)
		to := newPromotionCloud(f)
		to.onWrite = func() {
			current := f.get(u.ID)
			rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/abort", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.Version, "action": "abort", "reason": "interrupt fixture"}, nil)
			expect(t, rec, 200, result)
		}
		if worked, err := promotionHost(f, from, to).RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		if current := f.get(u.ID); current.State != "aborted" || current.ResultVersionID != nil || to.completes != 0 {
			t.Fatal("abort reactivated copy", current, to.completes)
		}
		var state string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE org_id=$1 AND kind='upload.promote.v1'", f.f.org).Scan(&state); err != nil || state != "succeeded" {
			t.Fatal("obsolete copy not acknowledged", state, err)
		}
	})
}
