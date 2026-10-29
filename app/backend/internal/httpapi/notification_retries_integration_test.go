package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/project"
)

func TestNotificationManualRetries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	queue := jobqueue.Queue{DB: f.api.DB}
	service := &notification.Service{DB: f.api.DB}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	grant := func(user, role string) {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','internal' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, user, role)
	}
	grant(f.admin, "notification_retry_operator")
	exec("INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'notification.delivery.read' FROM roles WHERE org_id=$1 AND key='project_manager'", f.org)
	admin := f.administratorToken()
	p := f.createProject("Synthetic retry business content stays restricted", []string{f.bob, f.member})
	get := func(t *testing.T, id, token string, status int) notification.DeliveryJob {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/notification-delivery-jobs/"+id, token, "", nil, nil)
		expect(t, r, status, v)
		if status != 200 {
			return notification.DeliveryJob{}
		}
		if strings.Contains(r.Body.String(), p.Name) || strings.Contains(r.Body.String(), "private_diagnostic") {
			t.Fatal("technical detail exposed business input")
		}
		return data[notification.DeliveryJob](t, v)
	}
	post := func(t *testing.T, id, token, key string, in notification.RetryInput, status int) response {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/notification-delivery-jobs/"+id+"/retry", token, key, in, nil)
		expect(t, r, status, v)
		return v
	}
	failed := func(title, recipient string, permanent bool) (string, project.Task) {
		t.Helper()
		r, v := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": title, "priority": "normal", "assignee_id": recipient, "review_required": false}, nil)
		expect(t, r, 201, v)
		task := data[project.Task](t, v)
		for i := 0; i < 1000; i++ {
			d, e := queue.DispatchOne(ctx, notification.Consumer())
			if e != nil {
				t.Fatal(e)
			}
			if d == nil {
				break
			}
			if i == 999 {
				t.Fatal("dispatch did not drain")
			}
		}
		var target string
		if err := f.owner.QueryRow(ctx, `SELECT j.id FROM jobs j JOIN outbox_events e ON e.org_id=j.org_id AND e.id=(j.payload->>'event_id')::uuid WHERE j.org_id=$1 AND j.kind=$2 AND e.aggregate_id=$3 AND e.event_type='task.created'`, f.org, notification.JobKind, task.ID).Scan(&target); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 1000; i++ {
			j, e := queue.Claim(ctx, []string{notification.JobKind}, "notification", "retry.fixture", time.Minute)
			if e != nil || j == nil {
				t.Fatal("missing real leased notification", e)
			}
			if j.ID == target {
				if e = queue.Fail(ctx, *j, "SYNTHETIC_TRANSIENT_FAILURE", permanent, time.Hour); e != nil {
					t.Fatal(e)
				}
				return target, task
			}
			if _, e = service.Process(ctx, *j); e != nil {
				t.Fatal(e)
			}
		}
		t.Fatal("notification target not claimed")
		return "", task
	}
	process := func(id string) notification.JobResult {
		t.Helper()
		for i := 0; i < 1000; i++ {
			j, e := queue.Claim(ctx, []string{notification.JobKind}, "notification", "retry.fixture", time.Minute)
			if e != nil || j == nil {
				t.Fatal("missing retry child", e)
			}
			out, e := service.Process(ctx, *j)
			if e != nil {
				t.Fatal(e)
			}
			if j.ID == id {
				return out
			}
		}
		t.Fatal("child not found")
		return notification.JobResult{}
	}
	source, task := failed("Synthetic manual retry source", f.member, true)
	input := func(id string) notification.RetryInput {
		return notification.RetryInput{ExpectedRevision: get(t, id, admin, 200).Revision, Reason: "Synthetic controlled retry after diagnosis"}
	}
	var receipt notification.RetryReceipt
	var original notification.DeliveryJob
	key := core.NewID()
	t.Run("separate authority and content boundary", func(t *testing.T) {
		get(t, source, f.memberToken, 404)
		get(t, core.NewID(), admin, 404)
		original = get(t, source, admin, 200)
		if !original.CanRetry || original.RetryBlock != "ready" {
			t.Fatal(original)
		}
		readonly := get(t, source, f.bobToken, 200)
		if readonly.CanRetry || readonly.RetryBlock != "not_authorized" {
			t.Fatal("reader acquired retry", readonly)
		}
		post(t, source, f.bobToken, core.NewID(), notification.RetryInput{ExpectedRevision: original.Revision, Reason: "No write authority"}, 404)
		r, v := f.with(t).request("GET", "/v1/tasks/"+task.ID, admin, "", nil, nil)
		expect(t, r, 404, v)
	})
	t.Run("current MFA, exact revision and strict input", func(t *testing.T) {
		in := input(source)
		bad := in
		bad.ExpectedRevision = strings.Repeat("f", 64)
		post(t, source, admin, core.NewID(), bad, 409)
		bad = in
		bad.Reason = " "
		post(t, source, admin, core.NewID(), bad, 422)
		ids, e := identity.NewService(f.api.DB, f.api.Config)
		if e != nil {
			t.Fatal(e)
		}
		actor, e := ids.Authenticate(ctx, admin)
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		post(t, source, admin, core.NewID(), in, 403)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		r, v := f.with(t).request("POST", "/v1/notification-delivery-jobs/"+source+"/retry", admin, core.NewID(), map[string]any{"expected_revision": in.ExpectedRevision, "reason": in.Reason, "payload": map[string]any{}}, nil)
		expect(t, r, 422, v)
	})
	t.Run("atomic child, stable replay and unchanged terminal source", func(t *testing.T) {
		in := input(source)
		receipt = data[notification.RetryReceipt](t, post(t, source, admin, key, in, 201))
		again := data[notification.RetryReceipt](t, post(t, source, admin, key, in, 201))
		if receipt != again {
			t.Fatal("idempotent retry forked")
		}
		parent := get(t, source, admin, 200)
		child := get(t, receipt.RetryJobID, admin, 200)
		if parent.State != "dead" || parent.Attempt != original.Attempt || !parent.UpdatedAt.Equal(original.UpdatedAt) || parent.RetryJobID == nil || *parent.RetryJobID != child.JobID || parent.CanRetry || child.State != "queued" || child.Attempt != 0 || child.ParentJobID == nil || *child.ParentJobID != source || child.BusinessEventID == nil || *child.BusinessEventID != receipt.BusinessEventID {
			t.Fatal("retry rewrote source or lost lineage", parent, child)
		}
		r, v := f.with(t).request("GET", "/v1/notification-delivery-failures?limit=50", admin, "", nil, nil)
		expect(t, r, 200, v)
		for _, item := range data[core.Page[notification.Failure]](t, v).Items {
			if item.JobID == source {
				t.Fatal("retried ancestor still presented as an unresolved leaf")
			}
		}
		var jobs, audits, events int
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM jobs WHERE org_id=$1 AND dedup_key=$2),(SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_id=$3 AND action='notification.delivery.retry'),(SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$3 AND event_type='notification.delivery.retry')`, f.org, "notification/retry/"+source, receipt.ID).Scan(&jobs, &audits, &events); e != nil || jobs != 1 || audits != 1 || events != 1 {
			t.Fatal(jobs, audits, events, e)
		}
	})
	t.Run("real worker results and event delivery deduplication", func(t *testing.T) {
		out := process(receipt.RetryJobID)
		if out.Delivered != 1 || out.Outcome != "delivered" {
			t.Fatal(out)
		}
		got := get(t, receipt.RetryJobID, admin, 200)
		if got.State != "succeeded" || got.Result == nil || got.Result.Delivered != 1 || got.CanRetry {
			t.Fatal(got)
		}
		if notificationFor(t, f, f.memberToken, task.ID) == nil {
			t.Fatal("no actual authorized recipient inbox entry")
		}
		post(t, source, admin, core.NewID(), input(source), 409)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND business_event_id=$2", f.org, receipt.BusinessEventID).Scan(&count); e != nil || count != 1 {
			t.Fatal("duplicate delivery", count, e)
		}
	})
	t.Run("read-only history and bound pagination", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			id, _ := failed("Synthetic history fixture", f.alice, true)
			post(t, id, admin, core.NewID(), input(id), 201)
		}
		r, v := f.with(t).request("GET", "/v1/notification-delivery-retries?limit=1", admin, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[notification.RetryReceipt]](t, v)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal(page)
		}
		r, v = f.with(t).request("GET", "/v1/notification-delivery-retries?limit=1&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, r, 200, v)
		next := data[core.Page[notification.RetryReceipt]](t, v)
		if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
			t.Fatal(next)
		}
		r, v = f.with(t).request("GET", "/v1/notification-delivery-retries?cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, r, 400, v)
		r, v = f.with(t).request("GET", "/v1/notification-delivery-retries?recipient_id="+f.alice, admin, "", nil, nil)
		expect(t, r, 422, v)
	})
	t.Run("four human read channels and executor rejection", func(t *testing.T) {
		for _, channel := range []string{"web", "wechat", "douyin", "android"} {
			if channel != "web" {
				token := f.login("bob", channel).AccessToken
				if token == "" {
					t.Fatal("synthetic channel session missing")
				}
				get(t, source, token, 200)
				continue
			}
			r, v := f.with(t).request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "bob", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
			expect(t, r, 200, v)
			cookies := []string{}
			for _, cookie := range r.Result().Cookies() {
				cookies = append(cookies, cookie.Name+"="+cookie.Value)
			}
			r, v = f.with(t).request("GET", "/v1/notification-delivery-jobs/"+source, "", "", nil, map[string]string{"Cookie": strings.Join(cookies, "; ")})
			expect(t, r, 200, v)
		}
		ids, e := identity.NewService(f.api.DB, f.api.Config)
		if e != nil {
			t.Fatal(e)
		}
		actor, e := ids.Authenticate(ctx, admin)
		if e != nil {
			t.Fatal(e)
		}
		for _, channel := range []string{"telegram", "web"} {
			copy := actor
			copy.Channel = channel
			if channel == "web" {
				copy.RunID = core.NewID()
			}
			_, err := service.DeliveryJob(ctx, copy, source)
			if err == nil || core.PublicError(err).Status != 403 {
				t.Fatal("executor accessed interactive operations", err)
			}
		}
	})
	t.Run("database requires a fresh child and atomic evidence", func(t *testing.T) {
		id, _ := failed("Synthetic database evidence guard", f.alice, true)
		var payload []byte
		if e := f.owner.QueryRow(ctx, "SELECT payload FROM jobs WHERE id=$1", id).Scan(&payload); e != nil {
			t.Fatal(e)
		}
		var fixed struct {
			EventID string `json:"event_id"`
		}
		if e := json.Unmarshal(payload, &fixed); e != nil {
			t.Fatal(e)
		}
		for _, version := range []int{2, 1} {
			tx, e := f.owner.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			child, _, e := queue.Enqueue(ctx, tx, jobqueue.Input{OrgID: f.org, Kind: notification.JobKind, Lane: "notification", DedupKey: "notification/retry/" + id, PayloadVersion: version, Payload: payload, MaxAttempts: 4})
			if e != nil {
				tx.Rollback(ctx)
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, `INSERT INTO notification_retries(id,org_id,source_job_id,retry_job_id,business_event_id,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,'Synthetic missing evidence')`, core.NewID(), f.org, id, child, fixed.EventID, f.admin)
			if version == 2 {
				if e == nil {
					tx.Rollback(ctx)
					t.Fatal("database accepted changed fixed input")
				}
				tx.Rollback(ctx)
				continue
			}
			if e != nil {
				tx.Rollback(ctx)
				t.Fatal(e)
			}
			if e = tx.Commit(ctx); e == nil {
				t.Fatal("database committed a retry without audit/outbox")
			}
		}
	})
	t.Run("retry rechecks current recipient authority", func(t *testing.T) {
		id, _ := failed("Synthetic revoked recipient", f.member, true)
		r := data[notification.RetryReceipt](t, post(t, id, admin, core.NewID(), input(id), 201))
		exec("DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, p.ID, f.member)
		out := process(r.RetryJobID)
		if out.Delivered != 0 || out.Outcome != "suppressed" {
			t.Fatal("revoked recipient received a retry", out)
		}
		got := get(t, r.RetryJobID, admin, 200)
		if got.Result == nil || got.Result.Delivered != 0 {
			t.Fatal(got)
		}
	})
	t.Run("stale business events are not reinterpreted", func(t *testing.T) {
		id, old := failed("Synthetic old assignment", f.alice, true)
		r, v := f.with(t).request("PATCH", "/v1/tasks/"+old.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": old.Version, "title": "Synthetic changed task"}, nil)
		expect(t, r, 200, v)
		rec := data[notification.RetryReceipt](t, post(t, id, admin, core.NewID(), input(id), 201))
		out := process(rec.RetryJobID)
		if out.Delivered != 0 || out.Outcome != "suppressed" {
			t.Fatal("old event became a new assignment", out)
		}
	})
	t.Run("automatic backoff cannot be bypassed", func(t *testing.T) {
		id, _ := failed("Synthetic scheduled automatic retry", f.alice, false)
		got := get(t, id, admin, 200)
		if got.State != "retry_wait" || got.RetryBlock != "not_failed" || got.CanRetry {
			t.Fatal(got)
		}
		post(t, id, admin, core.NewID(), notification.RetryInput{ExpectedRevision: got.Revision, Reason: "No early retry"}, 409)
	})
	t.Run("invalid fixed input and rejected event stay blocked", func(t *testing.T) {
		invalid := core.NewID()
		exec(`INSERT INTO jobs(id,org_id,kind,lane,dedup_key,payload_version,payload,state,max_attempts,last_error) VALUES($1,$2,'notification.deliver.v1','notification',$3,1,'{"private_diagnostic":"never expose payload"}','dead',4,'INVALID_INPUT')`, invalid, f.org, "synthetic/"+invalid)
		got := get(t, invalid, admin, 200)
		if got.RetryBlock != "invalid_input" || got.CanRetry {
			t.Fatal(got)
		}
		post(t, invalid, admin, core.NewID(), notification.RetryInput{ExpectedRevision: got.Revision, Reason: "Cannot change input"}, 409)
		rejected := core.NewID()
		exec(`INSERT INTO jobs(id,org_id,kind,lane,dedup_key,payload_version,payload,state,max_attempts,last_error) VALUES($1,$2,'event.rejected.v1','notification',$3,1,'{"private_diagnostic":"never expose payload"}','dead',4,'UNSUPPORTED_EVENT_VERSION')`, rejected, f.org, "notification-consumer.v1/"+rejected)
		got = get(t, rejected, admin, 200)
		if got.RetryBlock != "rejected_event" || got.CanRetry {
			t.Fatal(got)
		}
		post(t, rejected, admin, core.NewID(), notification.RetryInput{ExpectedRevision: got.Revision, Reason: "Cannot bypass protocol"}, 409)
	})
	t.Run("audit failure rolls back the retry child", func(t *testing.T) {
		id, _ := failed("Synthetic atomic audit rollback", f.alice, true)
		exec(`CREATE FUNCTION deny_retry_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='notification.delivery.retry' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER synthetic_retry_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION deny_retry_audit()`)
		post(t, id, admin, core.NewID(), input(id), 500)
		exec("DROP TRIGGER synthetic_retry_audit ON audit_events; DROP FUNCTION deny_retry_audit()")
		got := get(t, id, admin, 200)
		if !got.CanRetry || got.RetryJobID != nil {
			t.Fatal("partial retry survived", got)
		}
		var n int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE org_id=$1 AND dedup_key=$2", f.org, "notification/retry/"+id).Scan(&n); e != nil || n != 0 {
			t.Fatal("child survived audit rollback", n, e)
		}
	})
	t.Run("concurrent requests cannot fork a failure", func(t *testing.T) {
		id, _ := failed("Synthetic concurrent retry", f.alice, true)
		in := input(id)
		statuses := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/notification-delivery-jobs/"+id+"/retry", admin, core.NewID(), in, nil)
				statuses <- r.Code
			}()
		}
		wg.Wait()
		close(statuses)
		ok, conflict := 0, 0
		for status := range statuses {
			if status == 201 {
				ok++
			} else if status == 409 {
				conflict++
			} else {
				t.Fatal("unexpected concurrent status", status)
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatal(ok, conflict)
		}
	})
	t.Run("immutable retry evidence and authorization before replay", func(t *testing.T) {
		if _, e := f.api.DB.Exec(ctx, "UPDATE notification_retries SET reason='changed' WHERE id=$1", receipt.ID); e == nil {
			t.Fatal("application rewrote retry evidence")
		}
		if _, e := f.owner.Exec(ctx, "DELETE FROM notification_retries WHERE id=$1", receipt.ID); e == nil {
			t.Fatal("owner deleted immutable retry evidence")
		}
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='notification_retry_operator')", f.org, f.admin)
		post(t, source, admin, key, notification.RetryInput{ExpectedRevision: original.Revision, Reason: "Synthetic controlled retry after diagnosis"}, 404)
		grant(f.admin, "notification_retry_operator")
	})
	if os.Getenv("CP_BROWSER_NOTIFICATION_RETRY_FIXTURE") == "1" && !t.Failed() {
		browserJob, _ := failed("Synthetic browser manual retry", f.alice, true)
		grant(f.alice, "notification_retry_operator")
		f.aliceToken = memberMFAToken(f, "alice", "android")
		loopCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-loopCtx.Done():
					return
				case <-ticker.C:
					if _, err := queue.DispatchOne(loopCtx, notification.Consumer()); err != nil {
						if loopCtx.Err() == nil {
							t.Error("owned notification dispatch", err)
						}
						return
					}
					job, err := queue.Claim(loopCtx, []string{notification.JobKind}, "notification", "browser.retry.fixture", time.Minute)
					if err != nil {
						if loopCtx.Err() == nil {
							t.Error("owned notification claim", err)
						}
						return
					}
					if job == nil {
						continue
					}
					if _, err = service.Process(loopCtx, *job); err != nil && loopCtx.Err() == nil {
						if e := queue.Fail(loopCtx, *job, "SYNTHETIC_BROWSER_WORKER_FAILURE", true, time.Second); e != nil {
							t.Error(e)
						}
						return
					}
				}
			}
		}()
		defer func() { cancel(); <-done }()
		holdBrowserBusinessFixture(t, f, map[string]any{"notification_retry_fixture": true, "failed_job_id": browserJob, "project_id": p.ID, "source_job_id": source, "completed_retry_job_id": receipt.RetryJobID})
	}
}
