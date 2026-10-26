package httpapi

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
)

func TestUploadWorkerHost(t *testing.T) {
	ctx := context.Background()
	runner := func(f *uploadFixture) *uploadworker.Runner {
		return &uploadworker.Runner{Service: f.service, Store: f.cloud, Owner: "native-fixture-worker", Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	}
	t.Run("relay claim execution and uncertain retry form one persistent control loop", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("hosted.bin", 15)
		host := runner(f)
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("worker did not initialize session", err)
		}
		current := f.get(u.ID)
		if current.State != "uploading" || f.cloud.begins != 1 {
			t.Fatal("worker progress missing", current)
		}
		if worked, err := host.RunOne(ctx); err != nil || worked {
			t.Fatal("idle worker repeated completed task", err)
		}
		var uploadID string
		if err := f.f.owner.QueryRow(ctx, "SELECT provider_upload_id FROM upload_sessions WHERE id=$1", u.ID).Scan(&uploadID); err != nil {
			t.Fatal(err)
		}
		f.cloud.parts[uploadID] = []objectstore.Part{{Number: 1, Size: 15, ETag: "hosted-part"}}
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/finalize", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.Version}, nil)
		expect(t, rec, 202, result)
		f.cloud.completeTimeout = true
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("worker did not checkpoint uncertain result", err)
		}
		current = f.get(u.ID)
		if current.State != "finalizing" || current.ErrorCode == nil || *current.ErrorCode != "PROVIDER_TIMEOUT" {
			t.Fatal("retry diagnosis missing", current)
		}
		var state string
		var attempts int
		if err := f.f.owner.QueryRow(ctx, "SELECT state,attempt FROM jobs WHERE org_id=$1 AND kind='upload.finalize.v1'", f.f.org).Scan(&state, &attempts); err != nil || state != "retry_wait" || attempts != 1 {
			t.Fatal("retry task not persisted", state, attempts, err)
		}
		if _, err := f.f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND kind='upload.finalize.v1'", f.f.org); err != nil {
			t.Fatal(err)
		}
		if worked, err := host.RunOne(ctx); err != nil || !worked {
			t.Fatal("worker failed completion reconciliation", err)
		}
		current = f.get(u.ID)
		if current.State != "verifying" || current.ErrorCode != nil || f.cloud.completes != 1 {
			t.Fatal("retry did not reconcile fixed source", current, f.cloud.completes)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err := host.Run(cancelled); err != nil {
			t.Fatal("worker did not shut down cleanly", err)
		}
	})
	t.Run("permanent provider denial commits session audit and dead task together", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("denied.bin", 15)
		f.cloud.beginError = &objectstore.Error{Code: "PROVIDER_ACCESS_DENIED"}
		if worked, err := runner(f).RunOne(ctx); err != nil || !worked {
			t.Fatal("worker did not process denial", err)
		}
		current := f.get(u.ID)
		if current.State != "failed" || current.ErrorCode == nil || *current.ErrorCode != "PROVIDER_ACCESS_DENIED" {
			t.Fatal("permanent failure missing", current)
		}
		var state string
		var audit bool
		if err := f.f.owner.QueryRow(ctx, `SELECT j.state,EXISTS(SELECT 1 FROM audit_events a WHERE a.org_id=j.org_id AND a.job_id=j.id
            AND a.action='upload.failed' AND a.channel='worker' AND a.actor_id=$2 AND a.executor_id IS NULL)
            FROM jobs j WHERE j.org_id=$1 AND j.kind='upload.prepare.v1'`, f.f.org, f.f.alice).Scan(&state, &audit); err != nil || state != "dead" || !audit {
			t.Fatal("failure transaction incomplete", state, audit, err)
		}
	})
	t.Run("failure audit rejection leaves both domain and task unacknowledged for recovery", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.create("failure-rollback.bin", 15)
		f.cloud.beginError = &objectstore.Error{Code: "PROVIDER_ACCESS_DENIED"}
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_worker_failure_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action='upload.failed' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
            CREATE TRIGGER reject_worker_failure_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_worker_failure_fixture();`); err != nil {
			t.Fatal(err)
		}
		if worked, err := runner(f).RunOne(ctx); err != nil || !worked {
			t.Fatal("failure fixture not attempted", err)
		}
		current := f.get(u.ID)
		if current.State != "created" || current.ErrorCode != nil {
			t.Fatal("failed audit left partial domain failure", current)
		}
		var state string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE org_id=$1 AND kind='upload.prepare.v1'", f.f.org).Scan(&state); err != nil || state != "running" {
			t.Fatal("task was separately acknowledged after failure rollback", state, err)
		}
	})
}
