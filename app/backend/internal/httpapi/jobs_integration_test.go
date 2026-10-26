package httpapi

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
)

func TestDurableJobLeases(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	queue := jobqueue.Queue{DB: f.api.DB}
	input := func(key, lane string, max int) jobqueue.Input {
		return jobqueue.Input{OrgID: f.org, Kind: "storage.verify.v1", Lane: lane, DedupKey: key, PayloadVersion: 1, Payload: json.RawMessage(`{"source":"immutable-id","expected_size":"120000000000"}`), MaxAttempts: max}
	}
	enqueue := func(in jobqueue.Input) string {
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		id, _, err := queue.Enqueue(ctx, tx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := enqueue(input("exact-job", "large", 3))
	t.Run("deduplication pins input and rolls back with its transaction", func(t *testing.T) {
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		replay, created, err := queue.Enqueue(ctx, tx, input("exact-job", "large", 3))
		if err != nil || created || replay != id {
			t.Fatal("enqueue replay changed identity", err)
		}
		changed := input("exact-job", "large", 3)
		changed.Payload = json.RawMessage(`{"source":"different-id"}`)
		if _, _, err = queue.Enqueue(ctx, tx, changed); core.PublicError(err).Code != "JOB_DEDUP_MISMATCH" {
			t.Fatal("same key changed fixed source", err)
		}
		if _, _, err = queue.Enqueue(ctx, tx, input("rolled-back-job", "small", 3)); err != nil {
			t.Fatal(err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE org_id=$1 AND dedup_key='rolled-back-job'", f.org).Scan(&count); err != nil || count != 0 {
			t.Fatal("rolled back job survived", count, err)
		}
		if _, err = f.api.DB.Exec(ctx, "UPDATE jobs SET payload='{}'::jsonb WHERE org_id=$1 AND id=$2", f.org, id); err == nil {
			t.Fatal("fixed job input changed")
		}
	})
	t.Run("concurrent workers claim once and stale leases cannot renew fail or commit", func(t *testing.T) {
		var group sync.WaitGroup
		claimed := make(chan *jobqueue.Job, 5)
		failures := make(chan error, 5)
		for i := 0; i < 5; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				job, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "large", "worker-a", time.Minute)
				if err != nil {
					failures <- err
				} else if job != nil {
					claimed <- job
				}
			}()
		}
		group.Wait()
		close(claimed)
		close(failures)
		for err := range failures {
			t.Fatal(err)
		}
		var first *jobqueue.Job
		count := 0
		for job := range claimed {
			first = job
			count++
		}
		if count != 1 || first.ID != id || first.FencingToken != 1 || first.Attempt != 1 {
			t.Fatal("job claimed concurrently", count)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2", f.org, id); err != nil {
			t.Fatal(err)
		}
		if _, err := queue.Renew(ctx, *first, time.Minute); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("expired lease renewed", err)
		}
		next, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "large", "worker-b", time.Minute)
		if err != nil || next == nil || next.ID != id || next.FencingToken != 2 || next.Attempt != 2 {
			t.Fatal("expired job not reclaimed with new fence", err)
		}
		if err = queue.Fail(ctx, *first, "STALE_WORKER", false, time.Second); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old worker altered new attempt", err)
		}
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = queue.CheckInTx(ctx, tx, *first); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old worker acquired commit lock", err)
		}
		tx.Rollback(ctx)
		tx, err = f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = queue.CompleteInTx(ctx, tx, *first); core.PublicError(err).Code != "JOB_LEASE_LOST" {
			t.Fatal("old worker committed result", err)
		}
		tx.Rollback(ctx)
		if _, err = queue.Renew(ctx, *next, 2*time.Minute); err != nil {
			t.Fatal(err)
		}
		tx, err = f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = queue.CheckInTx(ctx, tx, *next); err != nil {
			t.Fatal(err)
		}
		if err = queue.CompleteInTx(ctx, tx, *next); err != nil {
			t.Fatal(err)
		}
		tx.Rollback(ctx)
		var state string
		if err = f.owner.QueryRow(ctx, "SELECT state FROM jobs WHERE org_id=$1 AND id=$2", f.org, id).Scan(&state); err != nil || state != "running" {
			t.Fatal("job acknowledgement escaped business rollback", state, err)
		}
		tx, err = f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = queue.CheckInTx(ctx, tx, *next); err != nil {
			t.Fatal(err)
		}
		if err = queue.CompleteInTx(ctx, tx, *next); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if job, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "large", "worker-c", time.Minute); err != nil || job != nil {
			t.Fatal("completed job executed again", err)
		}
	})
	t.Run("lanes retries exhaustion and stored diagnostics remain controlled", func(t *testing.T) {
		largeID := enqueue(input("exhaust-after-crash", "large", 1))
		smallID := enqueue(input("small-file-lane", "small", 3))
		small, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "small", "small-worker", time.Minute)
		if err != nil || small == nil || small.ID != smallID {
			t.Fatal("large task blocked small lane", err)
		}
		if err = queue.Fail(ctx, *small, "https://private-request-with-signature", false, time.Second); err == nil {
			t.Fatal("raw diagnostics accepted")
		}
		if err = queue.Fail(ctx, *small, "PROVIDER_RETRYABLE", false, time.Second); err != nil {
			t.Fatal(err)
		}
		if job, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "small", "small-worker", time.Minute); err != nil || job != nil {
			t.Fatal("retry delay ignored", err)
		}
		if _, err = f.owner.Exec(ctx, "UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2", f.org, smallID); err != nil {
			t.Fatal(err)
		}
		retried, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "small", "small-worker", time.Minute)
		if err != nil || retried == nil || retried.FencingToken != 2 || retried.Attempt != 2 {
			t.Fatal("retry lost attempt or fence", err)
		}
		if err = queue.Fail(ctx, *retried, "SCAN_REJECTED", true, time.Second); err != nil {
			t.Fatal(err)
		}
		large, err := queue.Claim(ctx, []string{"storage.verify.v1"}, "large", "large-worker", time.Minute)
		if err != nil || large == nil || large.ID != largeID {
			t.Fatal(err)
		}
		if _, err = f.owner.Exec(ctx, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND id=$2", f.org, largeID); err != nil {
			t.Fatal(err)
		}
		dead, err := queue.SweepExhausted(ctx, []string{"storage.verify.v1"}, "large")
		if err != nil || dead != 1 {
			t.Fatal("crashed final attempt stayed stranded", dead, err)
		}
		var state, code string
		if err = f.owner.QueryRow(ctx, "SELECT state,last_error FROM jobs WHERE org_id=$1 AND id=$2", f.org, largeID).Scan(&state, &code); err != nil || state != "dead" || code != "ATTEMPTS_EXHAUSTED" {
			t.Fatal(state, code, err)
		}
	})
}
