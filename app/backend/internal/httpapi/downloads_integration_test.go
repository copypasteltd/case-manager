package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

// An isolated cloud protocol fixture; no real provider request or large binary
// allocation. Fixed-source production signing has separate SDK adapter tests.
type downloadCloud struct {
	signs  atomic.Int64
	reads  atomic.Int64
	closed atomic.Int64
	onSign func()
	readFn func(context.Context, objectstore.Source, int64, int64) (objectstore.ReadResult, error)
}

func (s *downloadCloud) SignRead(ctx context.Context, source objectstore.Source, o objectstore.ReadOptions, ttl time.Duration) (objectstore.Signed, error) {
	n := s.signs.Add(1)
	if hook := s.onSign; hook != nil {
		s.onSign = nil
		hook()
	}
	return objectstore.Signed{Method: "GET", URL: fmt.Sprintf("https://fixture.invalid/%s?signature=transient-%d", source.Key, n),
		Headers: map[string]string{"If-Match": source.ETag}, ExpiresAt: time.Now().UTC().Add(ttl)}, nil
}
func (s *downloadCloud) Read(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
	s.reads.Add(1)
	if s.readFn != nil {
		return s.readFn(ctx, source, offset, length)
	}
	body := &downloadTestBody{reader: io.LimitReader(generatedZeros{ctx}, length), onClose: func() { s.closed.Add(1) }}
	return objectstore.ReadResult{Body: body, Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
}

type downloadTestBody struct {
	reader  io.Reader
	once    sync.Once
	onClose func()
}

func (b *downloadTestBody) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *downloadTestBody) Close() error {
	b.once.Do(func() {
		if b.onClose != nil {
			b.onClose()
		}
	})
	return nil
}
func wireDownloads(f *uploadFixture) *downloadCloud {
	cloud := &downloadCloud{}
	f.f.api.Downloads = map[string]*storage.DownloadBackend{
		"oss": {Provider: "oss", Bucket: "fixture-bucket", Signer: cloud, Reader: cloud},
		"r2":  {Provider: "r2", Bucket: "fixture-r2-bucket", Signer: cloud, Reader: cloud},
	}
	f.f.handler = f.f.api.Handler()
	return cloud
}
func downloadFixture(t *testing.T, size int64, confidential bool) (*uploadFixture, storage.CommitResult, *downloadCloud) {
	f, committed, _, _, _ := committedReplicaFixture(t, size)
	cloud := wireDownloads(f)
	if confidential {
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE files SET classification='confidential' WHERE id=$1", committed.File.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE role_bindings SET classification_ceiling='confidential' WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}
		// Confidential access requires an explicit resource grant as well as a
		// sufficient role ceiling. Being a project owner alone does not grant it.
		if _, err := f.f.owner.Exec(context.Background(), `INSERT INTO resource_grants(id,org_id,user_id,resource_type,resource_id,action,classification_ceiling,grantor_id,reason)
            VALUES($1,$2,$3,'file',$4,'file.read','confidential',$5,'isolated confidential download fixture')`,
			core.NewID(), f.f.org, f.f.alice, committed.File.ID, f.f.admin); err != nil {
			t.Fatal(err)
		}
	}
	return f, committed, cloud
}
func createDownload(t *testing.T, f *uploadFixture, version string) storage.DownloadResult {
	t.Helper()
	rec, result := f.f.request("POST", "/v1/file-versions/"+version+"/download", f.f.aliceToken, core.NewID(), map[string]string{"disposition": "attachment"}, nil)
	expect(t, rec, 200, result)
	return data[storage.DownloadResult](t, result)
}
func readDownload(f *uploadFixture, id, token, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/v1/downloads/"+id+"/content", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if value != "" {
		req.Header.Set("Range", value)
	}
	rec := httptest.NewRecorder()
	f.f.handler.ServeHTTP(rec, req)
	return rec
}
func TestFixedVersionDownloads(t *testing.T) {
	ctx := context.Background()
	t.Run("replay signs afresh without persisting a URL and checks current project access", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, false)
		key := core.NewID()
		route := "/v1/file-versions/" + committed.Version.ID + "/download"
		body := map[string]string{"disposition": "attachment"}
		rec, result := f.f.request("POST", route, f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		first := data[storage.DownloadResult](t, result)
		rec, result = f.f.request("POST", route, f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		second := data[storage.DownloadResult](t, result)
		if first.ID != second.ID || first.URL == second.URL || cloud.signs.Load() != 2 || rec.Header().Get("X-Idempotent-Replay") != "true" ||
			second.Mode != "signed_url" || second.VersionID != committed.Version.ID || second.SHA256 != committed.Version.SHA256 ||
			len(second.RequiredHeaders) != 1 || len(second.Warnings) != 1 || second.Warnings[0] != "SIGNED_URL_REVOCATION_DELAY" {
			t.Fatal("grant replay or source contract was incorrect")
		}
		var private bool
		if err := f.f.owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM idempotency_records WHERE org_id=$1 AND response_data::text LIKE '%transient-%')
            OR EXISTS(SELECT 1 FROM outbox_events WHERE org_id=$1 AND payload::text LIKE '%signature=%')
            OR EXISTS(SELECT 1 FROM download_permits WHERE org_id=$1 AND row_to_json(download_permits)::text LIKE '%signature=%')`, f.f.org).Scan(&private); err != nil || private {
			t.Fatal("transient URL persisted", private, err)
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE download_permits SET mode='authenticated_proxy' WHERE id=$1", first.ID); err == nil {
			t.Fatal("download permit was mutable")
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.f.org); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", route, f.f.aliceToken, key, body, nil)
		expect(t, rec, 412, result)
		if cloud.signs.Load() != 2 {
			t.Fatal("old authorization version minted another URL")
		}
		if _, err := f.f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", route, f.f.aliceToken, key, body, nil)
		expect(t, rec, 404, result)
		if cloud.signs.Load() != 2 {
			t.Fatal("replayed signature bypassed revoked project membership")
		}
	})
	t.Run("confidential ranges require the original authenticated session and audit exact written bytes", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, true)
		grant := createDownload(t, f, committed.Version.ID)
		if grant.Mode != "authenticated_proxy" || cloud.signs.Load() != 0 || len(grant.RequiredHeaders) != 0 || strings.Contains(grant.URL, "signature") {
			t.Fatal("confidential file was signed")
		}
		if rec := readDownload(f, grant.ID, "", "bytes=3-5"); rec.Code != 401 {
			t.Fatal("permit ID authorized an anonymous read", rec.Code)
		}
		if rec := readDownload(f, grant.ID, f.f.bobToken, "bytes=3-5"); rec.Code != 404 {
			t.Fatal("another user reused the permit", rec.Code)
		}
		otherSession := f.f.login("alice", "android").AccessToken
		if rec := readDownload(f, grant.ID, otherSession, "bytes=3-5"); rec.Code != 404 {
			t.Fatal("another session reused the permit", rec.Code)
		}
		rec := readDownload(f, grant.ID, f.f.aliceToken, "bytes=3-5")
		if rec.Code != 206 || rec.Body.Len() != 3 || rec.Header().Get("Content-Range") != "bytes 3-5/15" ||
			rec.Header().Get("Content-Length") != "3" || rec.Header().Get("ETag") != `"`+committed.Version.SHA256+`"` ||
			rec.Header().Get("Cache-Control") != "private, no-store" || cloud.closed.Load() != 1 {
			t.Fatal("authenticated range response was incorrect", rec.Code, rec.Header(), rec.Body.Len())
		}
		var started, completed, written int
		if err := f.f.owner.QueryRow(ctx, `SELECT count(*) FILTER(WHERE stage='started'),count(*) FILTER(WHERE stage='completed'),coalesce(sum(bytes_written),0)
            FROM download_read_events WHERE org_id=$1`, f.f.org).Scan(&started, &completed, &written); err != nil || started != 1 || completed != 1 || written != 3 {
			t.Fatal("read evidence misrepresented", started, completed, written, err)
		}
		if _, err := f.f.api.DB.Exec(ctx, "DELETE FROM download_read_events WHERE org_id=$1", f.f.org); err == nil {
			t.Fatal("download audit mutable")
		}
	})
	t.Run("invalid ranges source conditions and trash never start provider reads", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, true)
		grant := createDownload(t, f, committed.Version.ID)
		for _, value := range []string{"bytes=0-", "bytes=-2", "bytes=0-1,3-4", "bytes=0-15"} {
			if rec := readDownload(f, grant.ID, f.f.aliceToken, value); rec.Code != 416 {
				t.Fatal("unsafe range accepted", value, rec.Code)
			}
		}
		req := httptest.NewRequest("GET", "/v1/downloads/"+grant.ID+"/content", nil)
		req.Header.Set("Authorization", "Bearer "+f.f.aliceToken)
		req.Header.Set("If-Range", `"another-object"`)
		rec := httptest.NewRecorder()
		f.f.handler.ServeHTTP(rec, req)
		if rec.Code != 412 || cloud.reads.Load() != 0 {
			t.Fatal("source condition fell back to a full read")
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE files SET state='trashed',trashed_at=clock_timestamp() WHERE id=$1", committed.File.ID); err != nil {
			t.Fatal(err)
		}
		if rec := readDownload(f, grant.ID, f.f.aliceToken, "bytes=0-2"); rec.Code != 409 || cloud.reads.Load() != 0 {
			t.Fatal("trashed file was read", rec.Code)
		}
	})
	t.Run("unverified or quarantined source evidence and administrative privilege do not grant downloads", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, false)
		route := "/v1/file-versions/" + committed.Version.ID + "/download"
		in := map[string]string{"disposition": "attachment"}
		rec, result := f.f.request("POST", route, f.f.administratorToken(), core.NewID(), in, nil)
		expect(t, rec, 404, result)
		if _, err := f.f.owner.Exec(ctx, "UPDATE file_replicas SET state='verifying' WHERE version_id=$1 AND provider='oss'", committed.Version.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", route, f.f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 503, result)
		if _, err := f.f.owner.Exec(ctx, "UPDATE file_versions SET scan_state='quarantined',availability='quarantined' WHERE id=$1", committed.Version.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", route, f.f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 409, result)
		if cloud.signs.Load() != 0 {
			t.Fatal("unsafe version received a URL")
		}
	})
	t.Run("verified secondary can serve reads independently of a failed primary", func(t *testing.T) {
		f, committed, primary, secondary, _ := committedReplicaFixture(t, 15)
		runReplica(t, replicaHost(f, primary, secondary, 15))
		cloud := wireDownloads(f)
		if _, err := f.f.owner.Exec(ctx, "UPDATE file_replicas SET state='missing' WHERE version_id=$1 AND provider='oss'", committed.Version.ID); err != nil {
			t.Fatal(err)
		}
		grant := createDownload(t, f, committed.Version.ID)
		var provider string
		if err := f.f.owner.QueryRow(ctx, "SELECT provider FROM download_permits WHERE id=$1", grant.ID).Scan(&provider); err != nil || provider != "r2" || cloud.signs.Load() != 1 {
			t.Fatal("verified secondary not selected", provider, err)
		}
		var writeProvider string
		if err := f.f.owner.QueryRow(ctx, "SELECT active_primary FROM storage_epochs WHERE org_id=$1 ORDER BY epoch DESC LIMIT 1", f.f.org).Scan(&writeProvider); err != nil || writeProvider != "oss" {
			t.Fatal("read fallback changed write authority")
		}
	})
	t.Run("revocation during signing returns no URL and automated actors cannot obtain raw grants", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, false)
		a, err := f.f.api.Identity.Authenticate(ctx, f.f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		domain := &storage.DownloadService{Files: f.service.Files, Backends: f.f.api.Downloads, PublicOrigin: f.f.api.Config.PublicOrigin}
		for _, channel := range []string{"telegram", "worker"} {
			actor := a
			actor.Channel = channel
			_, err := domain.Create(ctx, core.Command{Actor: actor}, committed.Version.ID, storage.DownloadRequest{Disposition: "attachment"})
			if core.PublicError(err).Status != 403 {
				t.Fatal("automated channel obtained a grant", channel, err)
			}
		}
		actor := a
		actor.RunID = core.NewID()
		_, err = domain.Create(ctx, core.Command{Actor: actor}, committed.Version.ID, storage.DownloadRequest{Disposition: "attachment"})
		if core.PublicError(err).Status != 403 {
			t.Fatal("agent run obtained a bearer grant")
		}
		cloud.onSign = func() {
			if _, err := f.f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice); err != nil {
				t.Fatal(err)
			}
		}
		rec, result := f.f.request("POST", "/v1/file-versions/"+committed.Version.ID+"/download", f.f.aliceToken, core.NewID(), map[string]string{"disposition": "attachment"}, nil)
		expect(t, rec, 404, result)
		if strings.Contains(rec.Body.String(), "signature") {
			t.Fatal("revoked caller received a URL")
		}
	})
	t.Run("audit rejection fails before any stream and outcome distinguishes incomplete provider bytes", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 15, true)
		grant := createDownload(t, f, committed.Version.ID)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_download_start_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action IN ('download.read.started','download.permitted') THEN RAISE EXCEPTION 'fixture audit rejection'; END IF; RETURN NEW; END $$;
            CREATE TRIGGER reject_download_start_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_download_start_fixture()`); err != nil {
			t.Fatal(err)
		}
		rejected, result := f.f.request("POST", "/v1/file-versions/"+committed.Version.ID+"/download", f.f.aliceToken, core.NewID(), map[string]string{"disposition": "attachment"}, nil)
		expect(t, rejected, 500, result)
		var permits int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM download_permits WHERE org_id=$1", f.f.org).Scan(&permits); err != nil || permits != 1 {
			t.Fatal("grant persisted without audit", permits, err)
		}
		rec := readDownload(f, grant.ID, f.f.aliceToken, "")
		if rec.Code != 500 || cloud.reads.Load() != 0 {
			t.Fatal("provider opened without audit", rec.Code)
		}
		if _, err := f.f.owner.Exec(ctx, "DROP TRIGGER reject_download_start_fixture ON audit_events"); err != nil {
			t.Fatal(err)
		}
		cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
			return objectstore.ReadResult{Body: &downloadTestBody{reader: io.LimitReader(generatedZeros{ctx}, length-1), onClose: func() { cloud.closed.Add(1) }},
				Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
		}
		rec = readDownload(f, grant.ID, f.f.aliceToken, "")
		if rec.Code != 502 || cloud.closed.Load() != 1 {
			t.Fatal("short provider body reported success", rec.Code)
		}
		var failed, completed int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FILTER(WHERE stage='failed'),count(*) FILTER(WHERE stage='completed') FROM download_read_events WHERE org_id=$1", f.f.org).Scan(&failed, &completed); err != nil || failed != 1 || completed != 0 {
			t.Fatal("incomplete body audit was incorrect", failed, completed, err)
		}
		cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
			return objectstore.ReadResult{Body: &downloadTestBody{reader: io.LimitReader(generatedZeros{ctx}, length+1), onClose: func() { cloud.closed.Add(1) }},
				Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
		}
		rec = readDownload(f, grant.ID, f.f.aliceToken, "")
		if rec.Code != 502 || cloud.closed.Load() != 2 {
			t.Fatal("excess provider bytes accepted", rec.Code)
		}
	})
	t.Run("live revocation interrupts a blocked stream closes its body and preserves one active stream per user", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 2<<20, true)
		grant := createDownload(t, f, committed.Version.ID)
		blocked := make(chan struct{})
		cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
			reader := &blockedDownloadReader{ctx: ctx, blocked: blocked}
			return objectstore.ReadResult{Body: &downloadTestBody{reader: reader, onClose: func() { cloud.closed.Add(1) }},
				Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
		}
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- readDownload(f, grant.ID, f.f.aliceToken, "") }()
		select {
		case <-blocked:
		case <-time.After(3 * time.Second):
			t.Fatal("fixture did not reach blocked second chunk")
		}
		if rec := readDownload(f, grant.ID, f.f.aliceToken, "bytes=0-2"); rec.Code != 429 {
			t.Fatal("same user opened parallel proxy streams", rec.Code)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE users SET status='disabled',auth_version=auth_version+1 WHERE id=$1", f.f.alice); err != nil {
			t.Fatal(err)
		}
		select {
		case rec := <-done:
			if rec.Body.Len() != proxyBufferBytes || rec.Code != 200 || strings.Contains(rec.Body.String(), "UNAUTHENTICATED") || cloud.closed.Load() != 1 {
				t.Fatal("revocation did not stop after the already-written prefix", rec.Code, rec.Body.Len(), cloud.closed.Load())
			}
		case <-time.After(6 * time.Second):
			t.Fatal("revocation did not interrupt blocked provider read")
		}
		var stage, code string
		var written int64
		if err := f.f.owner.QueryRow(ctx, "SELECT stage,result_code,bytes_written FROM download_read_events WHERE org_id=$1 AND stage<>'started'", f.f.org).Scan(&stage, &code, &written); err != nil ||
			stage != "partial" || code != "UNAUTHENTICATED" || written != proxyBufferBytes {
			t.Fatal("revocation outcome was incorrect", stage, code, written, err)
		}
	})
}
func TestDownloadPermitExpiryAndClassificationChanges(t *testing.T) {
	f, committed, cloud := downloadFixture(t, 15, true)
	grant := createDownload(t, f, committed.Version.ID)
	expired := core.NewID()
	_, err := f.f.owner.Exec(context.Background(), `INSERT INTO download_permits(id,org_id,user_id,session_id,file_id,version_id,replica_id,provider,bucket,source,filename,
        classification,mode,disposition,channel,auth_version,org_auth_version,session_family_id,created_at,expires_at)
        SELECT $1,org_id,user_id,session_id,file_id,version_id,replica_id,provider,bucket,source,filename,classification,mode,disposition,channel,auth_version,org_auth_version,session_family_id,
        now()-interval '13 hours',now()-interval '1 hour' FROM download_permits WHERE id=$2`, expired, grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec := readDownload(f, expired, f.f.aliceToken, "bytes=0-1"); rec.Code != 410 {
		t.Fatal("expired permit still usable", rec.Code)
	}
	if _, err = f.f.owner.Exec(context.Background(), "UPDATE files SET classification='internal' WHERE id=$1", committed.File.ID); err != nil {
		t.Fatal(err)
	}
	if rec := readDownload(f, grant.ID, f.f.aliceToken, "bytes=0-1"); rec.Code != 412 || cloud.reads.Load() != 0 {
		t.Fatal("stale classification snapshot still usable", rec.Code)
	}
}
func TestDownloadSurvivesOnlyNormalDeviceSessionRotation(t *testing.T) {
	f, committed, cloud := downloadFixture(t, 15, true)
	login := f.f.login("alice", "android")
	f.f.aliceToken = login.AccessToken
	grant := createDownload(t, f, committed.Version.ID)
	refreshed, err := f.f.api.Identity.Refresh(context.Background(), login.RefreshToken, "", false, "download-session-rotation-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if rec := readDownload(f, grant.ID, login.AccessToken, "bytes=0-2"); rec.Code != 401 {
		t.Fatal("rotated old access token survived", rec.Code)
	}
	if rec := readDownload(f, grant.ID, refreshed.AccessToken, "bytes=0-2"); rec.Code != 206 || rec.Body.Len() != 3 {
		t.Fatal("normal device rotation lost the grant", rec.Code)
	}
	a, err := f.f.api.Identity.Authenticate(context.Background(), refreshed.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.f.api.Identity.Logout(context.Background(), a, "download-device-logout-fixture"); err != nil {
		t.Fatal(err)
	}
	if rec := readDownload(f, grant.ID, refreshed.AccessToken, "bytes=0-2"); rec.Code != 401 {
		t.Fatal("logout did not end the download", rec.Code)
	}
	newLogin := f.f.login("alice", "android")
	if rec := readDownload(f, grant.ID, newLogin.AccessToken, "bytes=0-2"); rec.Code != 404 || cloud.reads.Load() != 1 {
		t.Fatal("new device family reused the old permit", rec.Code)
	}
}

type blockedDownloadReader struct {
	ctx     context.Context
	blocked chan struct{}
	reads   int
}

func (b *blockedDownloadReader) Read(p []byte) (int, error) {
	b.reads++
	if b.reads == 1 {
		clear(p)
		return len(p), nil
	}
	if b.reads == 2 {
		close(b.blocked)
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

// Keep schema decoding in this integration suite: binary errors still use the
// same envelope, while successful streams never get wrapped in JSON.
func TestDownloadBinaryErrorsUseContractEnvelope(t *testing.T) {
	f, committed, _ := downloadFixture(t, 15, true)
	grant := createDownload(t, f, committed.Version.ID)
	rec := readDownload(f, grant.ID, f.f.aliceToken, "bytes=0-15")
	var result response
	if json.Unmarshal(rec.Body.Bytes(), &result) != nil || result.Error == nil || result.Error.Code != "DOWNLOAD_RANGE_INVALID" {
		t.Fatal("binary route error envelope was not parseable")
	}
	if err := f.f.api.Contract.Validate("Error", rec.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
}
