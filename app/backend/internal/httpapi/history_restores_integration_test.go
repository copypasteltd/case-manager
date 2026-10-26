package httpapi

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

func restorationFixture(t *testing.T, size int64) (*uploadFixture, storage.CommitResult, storage.CommitResult, *promotionCloud, *promotionCloud) {
	t.Helper()
	f, old, primary, _, _ := committedReplicaFixture(t, size)
	current := replacementVersion(t, f, old)
	return f, old, current, primary, newPromotionCloud(f)
}
func replacementVersion(t *testing.T, f *uploadFixture, old storage.CommitResult) storage.CommitResult {
	t.Helper()
	rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{
		"folder_id": f.root, "name": "current.bin", "size_bytes": "17", "media_type": "application/octet-stream", "source_fingerprint": "later-version",
		"file_id": old.File.ID, "base_version_id": old.Version.ID}, nil)
	expect(t, rec, 201, result)
	u := f.freezeUpload(data[storage.Upload](t, result))
	from := scannedUpload(f, u)
	if worked, err := promotionHost(f, from, newPromotionCloud(f)).RunOne(context.Background()); err != nil || !worked {
		t.Fatal("later candidate", err)
	}
	u = f.get(u.ID)
	rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
	expect(t, rec, 201, result)
	return data[storage.CommitResult](t, result)
}
func requestRestoration(t *testing.T, f *uploadFixture, old, current storage.CommitResult) storage.HistoryRestore {
	t.Helper()
	rec, result := f.f.request("POST", "/v1/files/"+current.File.ID+"/history-restores", f.f.aliceToken, core.NewID(),
		map[string]any{"expected_version": current.File.Version, "source_version_id": old.Version.ID, "reason": "恢复准确历史内容"}, nil)
	expect(t, rec, 202, result)
	return data[storage.HistoryRestore](t, result)
}
func getRestoration(t *testing.T, f *uploadFixture, id string) storage.HistoryRestore {
	t.Helper()
	rec, result := f.f.request("GET", "/v1/history-restores/"+id, f.f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	return data[storage.HistoryRestore](t, result)
}
func restoreHost(f *uploadFixture, from, to *promotionCloud, size int64) *uploadworker.Runner {
	mode := "restore-small"
	if size > 100000000 {
		mode = "restore-large"
	}
	return &uploadworker.Runner{Service: f.service, Store: from, TargetStore: to, Mode: mode, Owner: "restore-fixture", Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}
func runRestoration(t *testing.T, host *uploadworker.Runner) {
	t.Helper()
	if worked, err := host.RunOne(context.Background()); err != nil || !worked {
		t.Fatal("restoration not executed", worked, err)
	}
}
func restorationCommit(r storage.HistoryRestore) map[string]any {
	return map[string]any{"expected_version": r.Version, "base_version_id": r.BaseVersionID, "sha256": r.SHA256}
}
func restoreClaim(t *testing.T, f *uploadFixture) jobqueue.Job {
	t.Helper()
	ctx := context.Background()
	delivery, err := f.service.Queue.DispatchOne(ctx, storage.RestoreConsumer())
	if err != nil || delivery == nil || delivery.Rejected {
		t.Fatal("restoration delivery", delivery, err)
	}
	j, err := f.service.Queue.Claim(ctx, storage.RestoreJobKinds, storage.RestoreSmallLane, "restore-first-lease", time.Minute)
	if err != nil || j == nil {
		t.Fatal("restoration claim", err)
	}
	return *j
}
func TestHistoryRestoration(t *testing.T) {
	ctx := context.Background()
	t.Run("waived history preserves its original report and a newer disabled policy blocks another publication", func(t *testing.T) {
		f := newUploadFixture(t)
		var projectID string
		if err := f.f.owner.QueryRow(ctx, "SELECT project_id FROM folders WHERE id=$1", f.root).Scan(&projectID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling)
            SELECT $1,$2,$3,id,'project',$4,'project_restricted' FROM roles WHERE org_id=$2 AND key='file_scan_reviewer'`,
			core.NewID(), f.f.org, f.f.bob, projectID); err != nil {
			t.Fatal(err)
		}
		admin := f.f.administratorToken()
		policyInput := map[string]any{"expected_version": 0, "classification": "project_restricted", "enabled": true, "media_types": []string{"application/octet-stream"},
			"allowed_codes": []string{"SCANNER_NOT_CONFIGURED"}, "max_bytes": "120000000000", "reviewer_id": f.f.bob, "reason": "isolated restore policy fixture"}
		rec, result := f.f.request("POST", "/v1/storage/scan-waiver-policies", admin, core.NewID(), policyInput, nil)
		expect(t, rec, 201, result)
		policy := data[storage.WaiverPolicy](t, result)
		f.f.aliceToken = f.f.login("alice", "android").AccessToken
		u, cloud := blockedWaiverUpload(f)
		w := requestWaiver(f, u)
		bob := memberMFAToken(f.f, "bob", "wechat")
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, core.NewID(), waiverDecision(w), nil)
		expect(t, rec, 200, result)
		to := newPromotionCloud(f)
		if worked, err := promotionHost(f, cloud, to).RunOne(ctx); err != nil || !worked {
			t.Fatal("waived source candidate", err)
		}
		u = f.get(u.ID)
		configureStorageTargets(f)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		expect(t, rec, 201, result)
		old := data[storage.CommitResult](t, result)
		current := replacementVersion(t, f, old)
		r := requestRestoration(t, f, old, current)
		target := newPromotionCloud(f)
		runRestoration(t, restoreHost(f, to, target, 15))
		r = getRestoration(t, f, r.ID)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, core.NewID(), restorationCommit(r), nil)
		expect(t, rec, 201, result)
		committed := data[storage.CommitResult](t, result)
		if committed.Version.ScanState != "waived" || committed.Version.ScanWaiverRef == nil || *committed.Version.ScanWaiverRef != w.ID {
			t.Fatal("restore falsely clean", committed.Version)
		}
		r = requestRestoration(t, f, old, committed)
		runRestoration(t, restoreHost(f, to, newPromotionCloud(f), 15))
		r = getRestoration(t, f, r.ID)
		policyInput["expected_version"] = policy.Version
		policyInput["enabled"] = false
		policyInput["reason"] = "disable future restorations in fixture"
		rec, result = f.f.request("POST", "/v1/storage/scan-waiver-policies", admin, core.NewID(), policyInput, nil)
		expect(t, rec, 201, result)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, core.NewID(), restorationCommit(r), nil)
		expect(t, rec, 412, result)
		if result.Error.Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("policy change did not fence old task", result.Error)
		}
		f.f.aliceToken = f.f.login("alice", "android").AccessToken
		rec, result = f.f.request("POST", "/v1/files/"+committed.File.ID+"/history-restores", f.f.aliceToken, core.NewID(), map[string]any{
			"expected_version": committed.File.Version, "source_version_id": old.Version.ID, "reason": "retry after policy disabled"}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "RESTORE_SCAN_EVIDENCE_REQUIRED" || getRestoration(t, f, r.ID).State != "ready" {
			t.Fatal("obsolete policy accepted or candidate lost", result.Error)
		}
		var state string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM upload_scan_reports WHERE id=$1", w.ScanReportID).Scan(&state); err != nil || state != "unscannable" {
			t.Fatal("original report changed", state, err)
		}
	})
	t.Run("immutable fixed inputs and archived project block publication while cancellation remains available", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE file_history_restores SET expires_at=clock_timestamp()+interval '1 year' WHERE id=$1", r.ID); err == nil {
			t.Fatal("restore expiry extended")
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE file_history_restores SET source_version_id=$2 WHERE id=$1", r.ID, current.Version.ID); err == nil {
			t.Fatal("restore source retargeted")
		}
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		archiveProjectFixture(t, f.f, *old.File.ProjectID)
		rec, result := f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, core.NewID(), restorationCommit(r), nil)
		expect(t, rec, 409, result)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/cancel", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": r.Version, "reason": "archive stopped restoration"}, nil)
		expect(t, rec, 200, result)
	})
	t.Run("storage epoch change during copy records a domain failure without publishing", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		to.onWrite = func() {
			if _, err := f.f.owner.Exec(ctx, `INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence)
            VALUES($1,2,'r2',$2,'controlled fixture only')`, f.f.org, f.f.admin); err != nil {
				t.Fatal(err)
			}
		}
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		if r.State != "failed" || r.ErrorCode == nil || *r.ErrorCode != "STORAGE_EPOCH_CHANGED" {
			t.Fatal("old epoch became ready", r)
		}
	})
	t.Run("fixed bytes become a new audited version only after explicit idempotent commit and queue backup", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		if r.State != "ready" || r.CopiedBytes != "15" || r.VerifiedBytes != "15" || len(to.offsets) != 1 || len(from.offsets) != 1 {
			t.Fatal("ready without full readback", r, to.offsets, from.offsets)
		}
		var pointer string
		if err := f.f.owner.QueryRow(ctx, "SELECT current_version_id FROM files WHERE id=$1", current.File.ID).Scan(&pointer); err != nil || pointer != current.Version.ID {
			t.Fatal("copy published silently", err, pointer)
		}
		key := core.NewID()
		body := restorationCommit(r)
		rec, result := f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 201, result)
		committed := data[storage.CommitResult](t, result)
		if !committed.Committed || committed.Version.ID == old.Version.ID || committed.Version.ID == current.Version.ID || committed.Version.Seq != 3 ||
			committed.Version.SHA256 != old.Version.SHA256 || committed.Version.BaseVersionID == nil || *committed.Version.BaseVersionID != current.Version.ID ||
			committed.Version.RestoredFromVersionID == nil || *committed.Version.RestoredFromVersionID != old.Version.ID || committed.Version.Source != "restore" {
			t.Fatal("history was rewound or lost ancestry", committed)
		}
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 201, result)
		if data[storage.CommitResult](t, result).Version.ID != committed.Version.ID {
			t.Fatal("duplicate restored version")
		}
		backup := newPromotionCloud(f)
		// Consume earlier retained-history backups first, then the restored version.
		for n := 0; n < 3; n++ {
			runReplica(t, replicaHost(f, to, backup, 15))
		}
		var count int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE file_id=$1", current.File.ID).Scan(&count); err != nil || count != 3 {
			t.Fatal("history changed", count, err)
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_replicas WHERE version_id=$1 AND state='verified'", committed.Version.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("restored backup not verified", count, err)
		}
		rec, result = f.f.request("GET", "/v1/files/"+current.File.ID+"/versions", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if page := data[core.Page[storage.Version]](t, result); len(page.Items) != 3 || page.Items[0].RestoredFromVersionID == nil {
			t.Fatal("history DTO lost source", page)
		}
	})
	t.Run("commit conflict preserves candidate and current bytes", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		rec, result := f.f.request("POST", "/v1/files/"+current.File.ID+"/move", f.f.aliceToken, core.NewID(), map[string]any{
			"expected_version": current.File.Version, "new_name": "renamed.bin", "reason": "changed during restoration"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, core.NewID(), restorationCommit(r), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "BASE_VERSION_CONFLICT" || getRestoration(t, f, r.ID).State != "ready" {
			t.Fatal("conflict discarded candidate", result.Error)
		}
	})
	t.Run("audit failure rolls back publication and permits the exact command to retry", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_restore_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action='file.history.restore.committed' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END; $$;
            CREATE TRIGGER reject_restore_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_restore_fixture()`); err != nil {
			t.Fatal(err)
		}
		key := core.NewID()
		body := restorationCommit(r)
		rec, result := f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 500, result)
		var count int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE file_id=$1", current.File.ID).Scan(&count); err != nil || count != 2 || getRestoration(t, f, r.ID).State != "ready" {
			t.Fatal("partial publication", count, err)
		}
		if _, err := f.f.owner.Exec(ctx, "DROP TRIGGER reject_restore_fixture ON audit_events; DROP FUNCTION reject_restore_fixture()"); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/commit", f.f.aliceToken, key, body, nil)
		expect(t, rec, 201, result)
	})
	t.Run("restore and upload share quota reservations and cancellation releases only its own reservation", func(t *testing.T) {
		f, old, current, _, _ := restorationFixture(t, 15)
		if _, err := f.f.owner.Exec(ctx, "UPDATE storage_quotas SET logical_limit_bytes=47 WHERE org_id=$1", f.f.org); err != nil {
			t.Fatal(err)
		}
		r := requestRestoration(t, f, old, current)
		rec, result := f.f.request("POST", "/v1/uploads", f.f.aliceToken, core.NewID(), map[string]any{"folder_id": f.root, "name": "over.bin", "size_bytes": "1", "media_type": "application/octet-stream", "source_fingerprint": "over"}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "STORAGE_QUOTA_EXCEEDED" {
			t.Fatal(result.Error)
		}
		rec, result = f.f.request("POST", "/v1/files/"+current.File.ID+"/history-restores", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.File.Version, "source_version_id": old.Version.ID, "reason": "second"}, nil)
		expect(t, rec, 409, result)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/cancel", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": r.Version, "reason": "release reservation"}, nil)
		expect(t, rec, 200, result)
		_ = f.create("fits.bin", 15)
	})
	t.Run("another employee cannot view cancel or replay the original task and paging binds identity", func(t *testing.T) {
		f, old, current, _, _ := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		rec, result := f.f.request("GET", "/v1/history-restores/"+r.ID, f.f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("POST", "/v1/history-restores/"+r.ID+"/cancel", f.f.bobToken, core.NewID(), map[string]any{"expected_version": r.Version, "reason": "not owner"}, nil)
		expect(t, rec, 404, result)
		_ = requestRestoration(t, f, old, current)
		rec, result = f.f.request("GET", "/v1/files/"+current.File.ID+"/history-restores?limit=1", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.HistoryRestore]](t, result)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("missing cursor", page)
		}
		rec, result = f.f.request("GET", "/v1/files/"+current.File.ID+"/history-restores?limit=1&cursor="+*page.NextCursor, f.f.bobToken, "", nil, nil)
		expect(t, rec, 400, result)
		if result.Error.Code != "INVALID_CURSOR" {
			t.Fatal(result.Error)
		}
		rec, result = f.f.request("GET", "/v1/files/"+current.File.ID+"/history-restores", f.f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.HistoryRestore]](t, result).Items) != 0 {
			t.Fatal("owner filter after pagination")
		}
	})
	t.Run("cancel during cloud write fences progress and preserves all formal history", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		to.onWrite = func() {
			now := getRestoration(t, f, r.ID)
			rec, result := f.f.request("POST", "/v1/history-restores/"+r.ID+"/cancel", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": now.Version, "reason": "cancel in cloud RPC"}, nil)
			expect(t, rec, 200, result)
		}
		runRestoration(t, restoreHost(f, from, to, 15))
		now := getRestoration(t, f, r.ID)
		if now.State != "cancelled" || now.VerifiedBytes != "0" {
			t.Fatal("cancelled work published progress", now)
		}
		var count int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE file_id=$1", current.File.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal(count, err)
		}
	})
	t.Run("revoked authorization during cloud read is recorded as a failed restore", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		from.onRead = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.f.org, f.f.alice); err != nil {
				t.Fatal(err)
			}
		}
		runRestoration(t, restoreHost(f, from, to, 15))
		var state, code string
		if err := f.f.owner.QueryRow(ctx, "SELECT state,last_error FROM file_history_restores WHERE id=$1", r.ID).Scan(&state, &code); err != nil || state != "failed" || code != "AUTHORIZATION_CHANGED" {
			t.Fatal(state, code, err)
		}
	})
	t.Run("corrupted target never becomes ready", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		to.corrupt = true
		runRestoration(t, restoreHost(f, from, to, 15))
		r = getRestoration(t, f, r.ID)
		if r.State != "failed" || r.ErrorCode == nil || *r.ErrorCode != "CONTENT_DIGEST_MISMATCH" {
			t.Fatal("corrupt target accepted", r)
		}
	})
	t.Run("known multipart resumes while unknown initialization uses a new physical key", func(t *testing.T) {
		size := objectstore.DefaultPartBytes + 17
		f, old, current, from, to := restorationFixture(t, size)
		r := requestRestoration(t, f, old, current)
		to.failPart = 2
		host := restoreHost(f, from, to, size)
		runRestoration(t, host)
		if now := getRestoration(t, f, r.ID); now.State != "retry_wait" || now.CopiedBytes != strconv.FormatInt(objectstore.DefaultPartBytes, 10) {
			t.Fatal("missing durable prefix", now)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='file.history.restore.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		runRestoration(t, host)
		if now := getRestoration(t, f, r.ID); now.State != "ready" || to.begins != 1 {
			t.Fatal("known transfer restarted", now, to.begins)
		}
		// Start a separate restoration and make Begin's response unknown.
		r2 := requestRestoration(t, f, old, current)
		to.beginError = &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true, Uncertain: true}
		runRestoration(t, host)
		to.beginError = nil
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='file.history.restore.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		runRestoration(t, host)
		var keys int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(DISTINCT object_key) FROM object_transfers WHERE restore_id=$1", r2.ID).Scan(&keys); err != nil || keys != 2 || getRestoration(t, f, r2.ID).State != "ready" {
			t.Fatal("uncertain initialization key reused", keys, err)
		}
	})
	t.Run("old lease cannot persist a new receipt and exhausted unstarted work gets a domain failure", func(t *testing.T) {
		f, old, current, from, to := restorationFixture(t, 15)
		r := requestRestoration(t, f, old, current)
		j := restoreClaim(t, f)
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET attempt=max_attempts,lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.service.RunRestoreJob(ctx, j, from, to); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("stale lease executed", err)
		}
		host := restoreHost(f, from, to, 15)
		if _, err := host.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		if now := getRestoration(t, f, r.ID); now.State != "failed" || now.ErrorCode == nil || *now.ErrorCode != "ATTEMPTS_EXHAUSTED" {
			t.Fatal("exhaustion left queued forever", now)
		}
	})
}
