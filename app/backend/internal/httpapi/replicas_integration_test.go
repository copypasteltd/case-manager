package httpapi

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

func committedReplicaFixture(t *testing.T, size int64) (*uploadFixture, storage.CommitResult, *promotionCloud, *promotionCloud, string) {
	f := newUploadFixture(t)
	u, _, primary := readyUpload(f, "replica-source.bin", size)
	configureStorageTargets(f)
	rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
	expect(t, rec, 201, result)
	committed := data[storage.CommitResult](t, result)
	secondary := newPromotionCloud(f)
	id := ""
	for _, r := range committed.Version.Replicas {
		if r.Provider == "r2" {
			id = r.ID
		}
	}
	if id == "" {
		t.Fatal("missing queued secondary")
	}
	primary.offsets = nil
	f.service.ReplicaTarget = &storage.UploadBackend{Provider: "r2", Bucket: "fixture-r2-bucket"}
	return f, committed, primary, secondary, id
}
func replicaHost(f *uploadFixture, primary, secondary *promotionCloud, size int64) *uploadworker.Runner {
	mode := "replica-small"
	if size > 100000000 {
		mode = "replica-large"
	}
	return &uploadworker.Runner{Service: f.service, Store: primary, TargetStore: secondary, Owner: "replica-fixture", Mode: mode, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}
func replicaState(t *testing.T, f *uploadFixture, id string) (state string, version int64, code *string) {
	t.Helper()
	if err := f.f.owner.QueryRow(context.Background(), "SELECT state,version,last_error FROM file_replicas WHERE id=$1", id).Scan(&state, &version, &code); err != nil {
		t.Fatal(err)
	}
	return
}
func runReplica(t *testing.T, host *uploadworker.Runner) {
	t.Helper()
	if worked, err := host.RunOne(context.Background()); err != nil || !worked {
		t.Fatal("replica was not executed", worked, err)
	}
}
func replicaRetryDue(t *testing.T, f *uploadFixture) {
	t.Helper()
	if _, err := f.f.owner.Exec(context.Background(), "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='file.replicate.v1'", f.f.org); err != nil {
		t.Fatal(err)
	}
}
func replicaClaim(t *testing.T, f *uploadFixture) jobqueue.Job {
	t.Helper()
	ctx := context.Background()
	delivery, err := f.service.Queue.DispatchOne(ctx, storage.ReplicaConsumer())
	if err != nil || delivery == nil || delivery.Rejected {
		t.Fatal("replica event not accepted", delivery, err)
	}
	j, err := f.service.Queue.Claim(ctx, storage.ReplicaJobKinds, storage.ReplicaSmallLane, "replica-lease-first", time.Minute)
	if err != nil || j == nil {
		t.Fatal("replica not claimed", err)
	}
	return *j
}
func TestCrossCloudReplicas(t *testing.T) {
	ctx := context.Background()
	t.Run("backup survives owner deactivation archived project and trash while requiring complete target bytes", func(t *testing.T) {
		f, committed, primary, secondary, id := committedReplicaFixture(t, 15)
		archiveProjectFixture(t, f.f, *committed.File.ProjectID)
		if _, err := f.f.owner.Exec(ctx, "UPDATE users SET status='disabled',auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}

		if _, err := f.f.owner.Exec(ctx, "UPDATE files SET state='trashed',trashed_at=clock_timestamp() WHERE id=$1", committed.File.ID); err != nil {
			t.Fatal(err)
		}
		runReplica(t, replicaHost(f, primary, secondary, 15))
		if state, _, _ := replicaState(t, f, id); state != "verified" {
			t.Fatal("retained version not backed up", state)
		}
		var actual *string
		var verified *time.Time
		var versions int
		var source string
		if err := f.f.owner.QueryRow(ctx, "SELECT actual_sha256,verified_at,source_id FROM file_replicas WHERE id=$1", id).Scan(&actual, &verified, &source); err != nil {
			t.Fatal(err)
		}
		if actual == nil || *actual != committed.Version.SHA256 || verified == nil || source == "" || len(primary.offsets) != 1 || len(secondary.offsets) != 1 {
			t.Fatal("replica label without complete evidence", actual, verified, primary.offsets, secondary.offsets)
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&versions); err != nil || versions != 1 {
			t.Fatal("replica created another logical version", versions, err)
		}
	})
	t.Run("partial cross cloud copy resumes known multipart and preserves confirmed prefix", func(t *testing.T) {
		size := objectstore.DefaultPartBytes + 17
		f, _, primary, secondary, id := committedReplicaFixture(t, size)
		secondary.failPart = 2
		host := replicaHost(f, primary, secondary, size)
		runReplica(t, host)
		if state, _, _ := replicaState(t, f, id); state != "retry_wait" {
			t.Fatal("interruption not recorded", state)
		}
		// A manual retry cannot create a second task beside the scheduled retry.
		_, version, _ := replicaState(t, f, id)
		rec, result := f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "reason": "fixture"}, nil)
		expect(t, rec, 409, result)
		replicaRetryDue(t, f)
		runReplica(t, host)
		if state, _, _ := replicaState(t, f, id); state != "verified" || secondary.begins != 1 || len(secondary.writes) != 3 {
			t.Fatal("partial transfer did not resume", state, secondary.begins, secondary.writes)
		}
		if len(primary.offsets) != 3 || primary.offsets[0] != 0 || primary.offsets[1] != objectstore.DefaultPartBytes || primary.offsets[2] != objectstore.DefaultPartBytes {
			t.Fatal("source prefix reread", primary.offsets)
		}
	})
	t.Run("uncertain initialization uses another physical key", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		secondary.beginError = &objectstore.Error{Code: "PROVIDER_TIMEOUT", Retryable: true, Uncertain: true}
		host := replicaHost(f, primary, secondary, 15)
		runReplica(t, host)
		secondary.beginError = nil
		replicaRetryDue(t, f)
		runReplica(t, host)
		var attempts, keys, unknown int
		if err := f.f.owner.QueryRow(ctx, `SELECT count(*),count(DISTINCT object_key),count(*) FILTER(WHERE state='uncertain')
            FROM object_transfers WHERE org_id=$1 AND replica_id=$2`, f.f.org, id).Scan(&attempts, &keys, &unknown); err != nil {
			t.Fatal(err)
		}
		if state, _, _ := replicaState(t, f, id); state != "verified" || attempts != 2 || keys != 2 || unknown != 1 {
			t.Fatal("unknown initialization reused key", state, attempts, keys, unknown)
		}
	})
	t.Run("lost completion response is reconciled before repeating the mutation", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		secondary.completeTimeout = true
		host := replicaHost(f, primary, secondary, 15)
		runReplica(t, host)
		if state, _, _ := replicaState(t, f, id); state != "retry_wait" {
			t.Fatal("unknown completion reported success", state)
		}
		replicaRetryDue(t, f)
		runReplica(t, host)
		if state, _, _ := replicaState(t, f, id); state != "verified" || secondary.completes != 1 || secondary.begins != 1 || len(secondary.offsets) != 1 {
			t.Fatal("unknown completion not reconciled", state, secondary.completes, secondary.begins)
		}
	})
	t.Run("corrupt target stays failed primary remains available and authorized retry is idempotent", func(t *testing.T) {
		f, committed, primary, secondary, id := committedReplicaFixture(t, 15)
		secondary.corrupt = true
		host := replicaHost(f, primary, secondary, 15)
		runReplica(t, host)
		state, version, code := replicaState(t, f, id)
		if state != "failed" || code == nil || *code != "CONTENT_DIGEST_MISMATCH" {
			t.Fatal("corrupt target verified", state, code)
		}
		drainNotifications(t, f.f)
		notice := requireBusinessNotification(t, f.f, f.f.aliceToken, committed.File.ID, "replica_failure", true)
		rec, result := f.f.request("GET", "/v1/files/"+committed.File.ID+"/versions", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		versions := data[core.Page[storage.Version]](t, result)
		if len(versions.Items) != 1 || versions.Items[0].Availability != "available" || versions.Items[0].ReplicaState == nil || *versions.Items[0].ReplicaState != "failed" {
			t.Fatal("replica failure hid primary", versions)
		}
		body := map[string]any{"expected_version": version, "reason": "Retry after fixing isolated provider"}
		rec, result = f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.administratorToken(), core.NewID(), body, nil)
		expect(t, rec, 404, result)
		wrong := map[string]any{"expected_version": version - 1, "reason": "stale"}
		rec, result = f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.aliceToken, core.NewID(), wrong, nil)
		expect(t, rec, 409, result)
		key := core.NewID()
		rec, result = f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		rec, result = f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		if rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("retry duplicated")
		}
		secondary.corrupt = false
		runReplica(t, host)
		if state, _, _ := replicaState(t, f, id); state != "verified" || secondary.begins != 2 {
			t.Fatal("controlled retry not completed", state, secondary.begins)
		}
		current := requireBusinessNotification(t, f.f, f.f.aliceToken, committed.File.ID, "replica_failure", false)
		if current.ID != notice.ID || current.CurrentState != "verified" {
			t.Fatal("notification retained stale failed replica state", current)
		}
	})
	t.Run("old lease cannot persist a part or verification after takeover", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		old := replicaClaim(t, f)
		var next *jobqueue.Job
		secondary.onWrite = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", old.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			next, err = f.service.Queue.Claim(ctx, storage.ReplicaJobKinds, storage.ReplicaSmallLane, "replica-lease-next", time.Minute)
			if err != nil || next == nil {
				t.Fatal("lease not reclaimed", err)
			}
		}
		err := f.service.RunReplicaJob(ctx, old, primary, secondary)
		if core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old fence accepted", err)
		}
		if err = f.service.RecordReplicaFailure(ctx, old, "PROVIDER_TIMEOUT", false, time.Second); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old fence changed failure", err)
		}
		var parts int
		if err = f.f.owner.QueryRow(ctx, "SELECT count(*) FROM object_transfer_parts p JOIN object_transfers t ON t.id=p.transfer_id WHERE t.replica_id=$1", id).Scan(&parts); err != nil || parts != 0 {
			t.Fatal("old receipt persisted", parts, err)
		}
		if err = f.service.RunReplicaJob(ctx, *next, primary, secondary); err != nil {
			t.Fatal(err)
		}
		if state, _, _ := replicaState(t, f, id); state != "verified" || secondary.begins != 1 || len(secondary.writes) != 1 {
			t.Fatal("takeover did not adopt known multipart", state, secondary.begins, secondary.writes)
		}
	})
	t.Run("epoch switch during cloud write prevents further authoritative progress", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		secondary.onWrite = func() {
			if _, err := f.f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,2,'r2',$2,'test-only epoch fence')", f.f.org, f.f.admin); err != nil {
				t.Fatal(err)
			}
		}
		runReplica(t, replicaHost(f, primary, secondary, 15))
		state, _, code := replicaState(t, f, id)
		if state != "failed" || code == nil || *code != "STORAGE_EPOCH_CHANGED" || secondary.completes != 0 {
			t.Fatal("old epoch progressed", state, code, secondary.completes)
		}
	})
	t.Run("verification and task completion roll back with audit rejection", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		j := replicaClaim(t, f)
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_replica_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action='replica.verified' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
            CREATE TRIGGER reject_replica_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_replica_fixture();`); err != nil {
			t.Fatal(err)
		}
		if err := f.service.RunReplicaJob(ctx, j, primary, secondary); err == nil {
			t.Fatal("audit failure ignored")
		}
		var jobState string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE id=$1", j.ID).Scan(&jobState); err != nil || jobState != "running" {
			t.Fatal("task completion escaped transaction", jobState, err)
		}
		if state, _, _ := replicaState(t, f, id); state == "verified" {
			t.Fatal("unaudited verification persisted")
		}
		if _, err := f.f.owner.Exec(ctx, "DROP TRIGGER reject_replica_fixture ON audit_events; DROP FUNCTION reject_replica_fixture()"); err != nil {
			t.Fatal(err)
		}
		if err := f.service.RunReplicaJob(ctx, j, primary, secondary); err != nil {
			t.Fatal(err)
		}
		if state, _, _ := replicaState(t, f, id); state != "verified" || secondary.begins != 1 || secondary.completes != 1 || len(secondary.offsets) != 1 {
			t.Fatal("durable verification did not recover", state)
		}
	})
	t.Run("exhaustion before the first domain transaction does not strand the queued replica", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		j := replicaClaim(t, f)
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET attempt=max_attempts,lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); err != nil {
			t.Fatal(err)
		}
		if worked, err := replicaHost(f, primary, secondary, 15).RunOne(ctx); err != nil || worked {
			t.Fatal("dead initial job executed", worked, err)
		}
		if state, _, code := replicaState(t, f, id); state != "failed" || code == nil || *code != "ATTEMPTS_EXHAUSTED" || secondary.begins != 0 {
			t.Fatal("initial exhaustion stranded replica", state, code)
		}
	})
	t.Run("exhausted final lease is reflected in domain state and can be manually repaired", func(t *testing.T) {
		f, _, primary, secondary, id := committedReplicaFixture(t, 15)
		j := replicaClaim(t, f)
		secondary.onWrite = func() {
			if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET attempt=max_attempts,lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.service.RunReplicaJob(ctx, j, primary, secondary); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal(err)
		}
		if worked, err := replicaHost(f, primary, secondary, 15).RunOne(ctx); err != nil || worked {
			t.Fatal("exhaustion unexpectedly ran cloud task", worked, err)
		}
		state, version, code := replicaState(t, f, id)
		if state != "failed" || code == nil || *code != "ATTEMPTS_EXHAUSTED" {
			t.Fatal("dead job left copying forever", state, code)
		}
		rec, result := f.f.request("POST", "/v1/file-replicas/"+id+"/retry", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "reason": "Explicit recovery after exhausted lease"}, nil)
		expect(t, rec, 200, result)
		runReplica(t, replicaHost(f, primary, secondary, 15))
		if state, _, _ := replicaState(t, f, id); state != "verified" {
			t.Fatal("exhausted task not repairable", state)
		}
	})
}
