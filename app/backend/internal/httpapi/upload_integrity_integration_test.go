package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

// Bytes are generated on demand, never a real cloud or 120 GB transport claim.
type integrityCloud struct {
	*uploadCloudFixture
	readMu  sync.Mutex
	offsets []int64
	failAt  *int64
	onRead  func()
	closed  int
}
type generatedZeros struct{ ctx context.Context }

func (r generatedZeros) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	clear(p)
	return len(p), nil
}

type integrityBody struct {
	io.Reader
	closeFn func()
}

func (r *integrityBody) Close() error {
	if r.closeFn != nil {
		r.closeFn()
		r.closeFn = nil
	}
	return nil
}
func (s *integrityCloud) Read(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
	s.readMu.Lock()
	s.offsets = append(s.offsets, offset)
	fail := s.failAt != nil && *s.failAt == offset
	if fail {
		s.failAt = nil
	}
	callback := s.onRead
	s.onRead = nil
	s.readMu.Unlock()
	if callback != nil {
		callback()
	}
	if fail {
		return objectstore.ReadResult{}, &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true}
	}
	if offset < 0 || length < 1 || offset+length > source.Size {
		return objectstore.ReadResult{}, &objectstore.Error{Code: "SOURCE_SIZE_MISMATCH"}
	}
	body := &integrityBody{Reader: io.LimitReader(generatedZeros{ctx}, length), closeFn: func() { s.readMu.Lock(); s.closed++; s.readMu.Unlock() }}
	return objectstore.ReadResult{Body: body, Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
}
func zeroSHA(size int64) string {
	h := sha256.New()
	_, _ = io.CopyN(h, generatedZeros{context.Background()}, size)
	return hex.EncodeToString(h.Sum(nil))
}

type integrityScanner func(context.Context, io.ReadCloser, int64) (filescan.Result, error)

func (s integrityScanner) Scan(ctx context.Context, source io.ReadCloser, size int64) (filescan.Result, error) {
	return s(ctx, source, size)
}
func cleanIntegrityScanner() filescan.Scanner {
	return integrityScanner(func(ctx context.Context, source io.ReadCloser, size int64) (filescan.Result, error) {
		defer source.Close()
		n, err := io.Copy(io.Discard, source)
		return filescan.Result{State: "clean", Code: "SCAN_PASSED", Engine: "fixture", EngineVersion: "1", SignatureRevision: "7", PolicySHA256: core.Digest([]byte("fixture-policy")), StreamedBytes: n}, err
	})
}
func (f *uploadFixture) frozen(name string, size int64) storage.Upload {
	f.f.t.Helper()
	return f.freezeUpload(f.create(name, size))
}
func (f *uploadFixture) freezeUpload(created storage.Upload) storage.Upload {
	f.f.t.Helper()
	u := f.prepared(created)
	size, _ := strconv.ParseInt(u.SizeBytes, 10, 64)
	var uploadID string
	if err := f.f.owner.QueryRow(context.Background(), "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&uploadID); err != nil {
		f.f.t.Fatal(err)
	}
	parts := make([]objectstore.Part, 0, u.PartCount)
	remaining := size
	for n := 1; n <= u.PartCount; n++ {
		length := u.PartBytes
		if remaining < length {
			length = remaining
		}
		parts = append(parts, objectstore.Part{Number: n, Size: length, ETag: "frozen-part-" + strconv.Itoa(n)})
		remaining -= length
	}
	f.cloud.parts[uploadID] = parts
	rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/finalize", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version}, nil)
	expect(f.f.t, rec, 202, result)
	j := f.job()
	if err := f.service.RunUploadJob(context.Background(), j, f.cloud); err != nil {
		f.f.t.Fatal(err)
	}
	u = f.get(u.ID)
	if u.State != "verifying" || u.SHA256 != nil {
		f.f.t.Fatal("source not frozen independently of hash", u)
	}
	return u
}
func (f *uploadFixture) integrityJob(size int64) jobqueue.Job {
	f.f.t.Helper()
	delivery, err := f.service.Queue.DispatchOne(context.Background(), storage.UploadIntegrityConsumer())
	if err != nil || delivery == nil || delivery.Rejected {
		f.f.t.Fatal("integrity event rejected", err, delivery)
	}
	j, err := f.service.Queue.Claim(context.Background(), storage.VerifyJobKinds, storage.VerifyLane(size), "integrity-fixture", 2*time.Minute)
	if err != nil || j == nil {
		f.f.t.Fatal("integrity job missing", err)
	}
	return *j
}
func integrityHost(f *uploadFixture, cloud *integrityCloud, scanner filescan.Scanner) *uploadworker.Runner {
	return &uploadworker.Runner{Service: f.service, Store: cloud, Owner: "integrity-host-fixture", Mode: "verify-small", Scanner: scanner, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}
func TestUploadIntegrity(t *testing.T) {
	ctx := context.Background()
	t.Run("disabled scanning blocks while rescan preserves authoritative hash and append only reports", func(t *testing.T) {
		f := newUploadFixture(t)
		const size int64 = 1048583
		u := f.frozen("disabled.bin", size)
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		host := integrityHost(f, cloud, filescan.Disabled{})
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("verification not executed", err)
		}
		current := f.get(u.ID)
		if current.State != "blocked" || current.ScanState != "unscannable" || current.ScanResultID == nil || current.SHA256 == nil || *current.SHA256 != zeroSHA(size) || current.VerifiedBytes != strconv.FormatInt(size, 10) {
			t.Fatal("false acceptance or missing authority", current)
		}
		if len(cloud.offsets) != 1 || cloud.closed != 1 {
			t.Fatal("disabled scanner opened cloud content", cloud.offsets, cloud.closed)
		}
		rec, result := f.f.request("GET", "/v1/uploads/"+u.ID+"/scan-report", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		report := data[storage.ScanReport](t, result)
		if report.Code != "SCANNER_NOT_CONFIGURED" || report.StreamedBytes != "0" || report.Engine != nil || report.SHA256 != *current.SHA256 {
			t.Fatal("misleading scan report", report)
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE upload_scan_reports SET code='SCAN_PASSED' WHERE id=$1", report.ID); err == nil {
			t.Fatal("scan evidence was mutable")
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE upload_sessions SET final_digest=$2 WHERE id=$1", u.ID, core.Digest([]byte("other"))); err == nil {
			t.Fatal("authoritative digest was mutable")
		}
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/rescan", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.Version}, nil)
		expect(t, rec, 202, result)
		host.Scanner = cleanIntegrityScanner()
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("rescan not executed", err)
		}
		current = f.get(u.ID)
		if current.ScanState != "clean" || current.State != "verifying" || current.ScanResultID == nil || *current.ScanResultID == report.ID || current.ErrorCode != nil {
			t.Fatal("rescan did not create separate evidence", current)
		}
		if len(cloud.offsets) != 2 || cloud.closed != 2 {
			t.Fatal("rescan rehashed a finished authority", cloud.offsets, cloud.closed)
		}
		var reports, versions int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_scan_reports WHERE session_id=$1", u.ID).Scan(&reports); err != nil {
			t.Fatal(err)
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&versions); err != nil {
			t.Fatal(err)
		}
		if reports != 2 || versions != 0 {
			t.Fatal("scan silently committed a formal version", reports, versions)
		}
	})
	t.Run("retry reads only ranges after the durable checkpoint", func(t *testing.T) {
		f := newUploadFixture(t)
		size := objectstore.DefaultPartBytes + 17
		u := f.frozen("checkpoint.bin", size)
		failure := objectstore.DefaultPartBytes
		cloud := &integrityCloud{uploadCloudFixture: f.cloud, failAt: &failure}
		host := integrityHost(f, cloud, filescan.Disabled{})
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("checkpoint fixture not attempted", err)
		}
		current := f.get(u.ID)
		if current.VerifiedBytes != strconv.FormatInt(objectstore.DefaultPartBytes, 10) || current.SHA256 != nil || current.State != "verifying" {
			t.Fatal("partial digest became authoritative", current)
		}
		var state string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE org_id=$1 AND kind='upload.verify.v1'", f.f.org).Scan(&state); err != nil || state != "retry_wait" {
			t.Fatal("retry not durable", state, err)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='upload.verify.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("checkpoint retry not attempted", err)
		}
		current = f.get(u.ID)
		if current.SHA256 == nil || *current.SHA256 != zeroSHA(size) || current.State != "blocked" {
			t.Fatal("resumed authority invalid", current)
		}
		if len(cloud.offsets) != 3 || cloud.offsets[0] != 0 || cloud.offsets[1] != objectstore.DefaultPartBytes || cloud.offsets[2] != objectstore.DefaultPartBytes || cloud.closed != 2 {
			t.Fatal("confirmed prefix re-read or body leaked", cloud.offsets, cloud.closed)
		}
	})
	t.Run("clean label without provenance cannot produce a scan receipt", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("false-clean.bin", 15)
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		scanner := integrityScanner(func(ctx context.Context, body io.ReadCloser, size int64) (filescan.Result, error) {
			body.Close()
			return filescan.Result{State: "clean", Code: "SCAN_PASSED", StreamedBytes: size}, nil
		})
		if worked, err := integrityHost(f, cloud, scanner).RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		current := f.get(u.ID)
		if current.State != "failed" || current.ScanResultID != nil || current.ErrorCode == nil || *current.ErrorCode != "INVALID_SCAN_EVIDENCE" {
			t.Fatal("unproven clean was accepted", current)
		}
		var reports int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_scan_reports WHERE session_id=$1", u.ID).Scan(&reports); err != nil || reports != 0 {
			t.Fatal("invalid evidence persisted", reports, err)
		}
	})
	t.Run("complete looking receipt cannot replace reading the source", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("unread.bin", 15)
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		scanner := integrityScanner(func(ctx context.Context, body io.ReadCloser, size int64) (filescan.Result, error) {
			body.Close()
			return filescan.Result{State: "clean", Code: "SCAN_PASSED", Engine: "fixture", EngineVersion: "1", SignatureRevision: "7", PolicySHA256: core.Digest([]byte("policy")), StreamedBytes: size}, nil
		})
		if worked, err := integrityHost(f, cloud, scanner).RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		current := f.get(u.ID)
		if current.State != "failed" || current.ScanResultID != nil || current.ErrorCode == nil || *current.ErrorCode != "INVALID_SCAN_EVIDENCE" {
			t.Fatal("unread source accepted", current)
		}
	})
	t.Run("superseded lease cannot advance a hash checkpoint", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("fenced.bin", 15)
		j := f.integrityJob(15)
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		cloud.onRead = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); err != nil {
				t.Fatal(err)
			}
			replacement, err := f.service.Queue.Claim(ctx, storage.VerifyJobKinds, storage.VerifySmallLane, "replacement-integrity", 2*time.Minute)
			if err != nil || replacement == nil || replacement.FencingToken <= j.FencingToken {
				t.Fatal("lease not superseded", err)
			}
		}
		err := f.service.RunIntegrityJob(ctx, j, cloud, filescan.Disabled{})
		if err == nil || core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old lease advanced integrity", err)
		}
		current := f.get(u.ID)
		if current.VerifiedBytes != "0" || current.SHA256 != nil || current.ScanResultID != nil {
			t.Fatal("stale writer persisted progress", current)
		}
	})
	t.Run("revocation during stream prevents authoritative progress", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("revoked.bin", 15)
		j := f.integrityJob(15)
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		cloud.onRead = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE users SET status='disabled' WHERE id=$1", f.f.alice); err != nil {
				t.Fatal(err)
			}
		}
		err := f.service.RunIntegrityJob(ctx, j, cloud, filescan.Disabled{})
		if err == nil || core.PublicError(err).Status != 403 {
			t.Fatal("revoked owner advanced integrity", err)
		}
		var verified int64
		var digest *string
		if err := f.f.owner.QueryRow(ctx, "SELECT verified_bytes,final_digest FROM upload_sessions WHERE id=$1", u.ID).Scan(&verified, &digest); err != nil || verified != 0 || digest != nil {
			t.Fatal("revoked progress persisted", verified, digest, err)
		}
	})
	t.Run("scan report audit and job completion roll back together", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("atomic-scan.bin", 15)
		j := f.integrityJob(15)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_scan_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action='upload.scan.passed' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
			CREATE TRIGGER reject_scan_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_scan_fixture();`); err != nil {
			t.Fatal(err)
		}
		cloud := &integrityCloud{uploadCloudFixture: f.cloud}
		if err := f.service.RunIntegrityJob(ctx, j, cloud, cleanIntegrityScanner()); err == nil {
			t.Fatal("audit rejection ignored")
		}
		current := f.get(u.ID)
		var reports int
		var state string
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_scan_reports WHERE session_id=$1", u.ID).Scan(&reports); err != nil {
			t.Fatal(err)
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE id=$1", j.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if current.State != "verifying" || current.ScanState != "pending" || current.ScanResultID != nil || current.SHA256 == nil || reports != 0 || state != "running" {
			t.Fatal("scan transaction partially committed", current, reports, state)
		}
	})
}
