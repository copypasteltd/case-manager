package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"os"
	"strings"
	"testing"
	"time"
)

// The notification tests use real HTTP business writes, the durable outbox,
// native PostgreSQL, real leased jobs, domain authorization and strict DTOs.
func drainNotifications(t *testing.T, f *fixture) {
	t.Helper()
	ctx := context.Background()
	queue := jobqueue.Queue{DB: f.api.DB}
	service := &notification.Service{DB: f.api.DB}
	for i := 0; i < 1000; i++ {
		delivery, err := queue.DispatchOne(ctx, notification.Consumer())
		if err != nil {
			t.Fatal(err)
		}
		if delivery == nil {
			break
		}
		if i == 999 {
			t.Fatal("fixture notification dispatch did not drain")
		}
	}
	for i := 0; i < 1000; i++ {
		job, err := queue.Claim(ctx, []string{notification.JobKind}, "notification", "notification.fixture", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if job == nil {
			return
		}
		if _, err = service.Process(ctx, *job); err != nil {
			t.Fatal("actual notification delivery failed", err)
		}
	}
	t.Fatal("fixture notification jobs did not drain")
}
func notificationFor(t *testing.T, f *fixture, token, resource string) *notification.Notification {
	t.Helper()
	rec, res := f.request("GET", "/v1/notifications?limit=50", token, "", nil, nil)
	expect(t, rec, 200, res)
	page := data[core.Page[notification.Notification]](t, res)
	for _, n := range page.Items {
		if n.Target.ID == resource {
			copy := n
			return &copy
		}
	}
	return nil
}
func requireBusinessNotification(t *testing.T, f *fixture, token, resource, category string, attention bool) *notification.Notification {
	t.Helper()
	n := notificationFor(t, f, token, resource)
	if n == nil || n.Category != category || n.AttentionRequired != attention {
		t.Fatalf("actual %s notification did not reflect current business duty: %+v", category, n)
	}
	return n
}
func TestInAppNotifications(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	queue := jobqueue.Queue{DB: f.api.DB}
	service := &notification.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}, Pager: platform.Pager{Key: f.api.Config.CursorKey}}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	p := f.createProject("通知使用真实权限和任务", []string{f.bob, f.member})
	create := func(title, assignee string) project.Task {
		t.Helper()
		rec, res := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": title, "priority": "normal", "assignee_id": assignee, "review_required": false}, nil)
		expect(t, rec, 201, res)
		return data[project.Task](t, res)
	}
	patch := func(task project.Task, body map[string]any) project.Task {
		t.Helper()
		body["expected_version"] = task.Version
		rec, res := f.request("PATCH", "/v1/tasks/"+task.ID, f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 200, res)
		return data[project.Task](t, res)
	}
	task := create("通知不能授予业务权限", f.alice)
	other := create("分页的第二项真实任务", f.alice)
	drainNotifications(t, f)
	first := notificationFor(t, f, f.aliceToken, task.ID)
	if first == nil || first.Read || !first.AttentionRequired || first.DeliverySeq != 1 || first.ResourceLabel != task.Title || first.Target.WebPath != "/projects/"+p.ID+"?task="+task.ID {
		t.Fatal("actual task delivery missing or misrepresented", first)
	}
	if notificationFor(t, f, f.bobToken, task.ID) != nil {
		t.Fatal("task notification broadcast to an unrelated recipient")
	}
	rec, res := f.request("GET", "/v1/notifications/"+first.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 404, res)
	rec, res = f.request("GET", "/v1/notifications", "", "", nil, nil)
	expect(t, rec, 401, res)
	rec, res = f.request("GET", "/v1/notifications?recipient_id="+f.alice, f.bobToken, "", nil, nil)
	expect(t, rec, 422, res)
	rec, res = f.request("GET", "/v1/notifications?state=all&state=unread", f.aliceToken, "", nil, nil)
	expect(t, rec, 422, res)

	rec, res = f.request("GET", "/v1/notifications?limit=1", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	page := data[core.Page[notification.Notification]](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("notification keyset page missing")
	}
	rec, res = f.request("GET", "/v1/notifications?state=unread&limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
	expect(t, rec, 400, res)
	rec, res = f.request("GET", "/v1/notifications?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	secondPage := data[core.Page[notification.Notification]](t, res)
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID == page.Items[0].ID {
		t.Fatal("notification page repeated or skipped an actual target")
	}

	key := core.NewID()
	rec, res = f.request("POST", "/v1/notifications/"+first.ID+"/read", f.aliceToken, key, notification.ReadInput{DeliverySeq: 1}, nil)
	expect(t, rec, 200, res)
	read := data[notification.Notification](t, res)
	if !read.Read || !read.AttentionRequired || read.ReadSeq != 1 || read.ReadAt == nil {
		t.Fatal("reading incorrectly completed the task", read)
	}
	rec, res = f.request("POST", "/v1/notifications/"+first.ID+"/read", f.aliceToken, key, notification.ReadInput{DeliverySeq: 1}, nil)
	expect(t, rec, 200, res)
	if rec.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatal("read receipt was not idempotently replayed")
	}
	task = patch(task, map[string]any{"title": "任务变化产生新的未读投递"})
	drainNotifications(t, f)
	current := notificationFor(t, f, f.aliceToken, task.ID)
	if current == nil || current.ID != first.ID || current.DeliverySeq != 2 || current.ReadSeq != 1 || current.Read || current.ResourceLabel != task.Title {
		t.Fatal("new event did not coalesce without losing unread state", current)
	}
	rec, res = f.request("POST", "/v1/notifications/"+first.ID+"/read", f.aliceToken, core.NewID(), notification.ReadInput{DeliverySeq: 1}, nil)
	expect(t, rec, 200, res)
	if data[notification.Notification](t, res).Read {
		t.Fatal("old acknowledgement swallowed a new delivery")
	}
	rec, res = f.request("POST", "/v1/notifications/"+first.ID+"/read", f.aliceToken, core.NewID(), notification.ReadInput{DeliverySeq: 99}, nil)
	expect(t, rec, 409, res)
	var count int
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE thread_id=$1", first.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("event replay added deliveries", count, err)
	}

	atomic := create("失败必须原子回滚通知与投递回执", f.alice)
	for {
		delivery, err := queue.DispatchOne(ctx, notification.Consumer())
		if err != nil {
			t.Fatal(err)
		}
		if delivery == nil {
			break
		}
	}
	job, err := queue.Claim(ctx, []string{notification.JobKind}, "notification", "notification.atomic", time.Minute)
	if err != nil || job == nil {
		t.Fatal("real notification job not claimed", err)
	}
	stolen := *job
	stolen.FencingToken++
	if _, err = service.Process(ctx, stolen); err == nil {
		t.Fatal("forged fencing token accepted")
	}
	exec(`CREATE FUNCTION reject_notification_result_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled receipt failure'; END; $$;
  CREATE TRIGGER reject_notification_result_fixture BEFORE INSERT ON notification_job_results FOR EACH ROW EXECUTE FUNCTION reject_notification_result_fixture()`)
	if _, err = service.Process(ctx, *job); err == nil {
		t.Fatal("notification receipt failure swallowed")
	}
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_threads WHERE resource_id=$1", atomic.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("thread survived a failed receipt transaction", count, err)
	}
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE job_id=$1", job.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("delivery survived failed receipt transaction", count, err)
	}
	exec("DROP TRIGGER reject_notification_result_fixture ON notification_job_results; DROP FUNCTION reject_notification_result_fixture()")
	if _, err = service.Process(ctx, *job); err != nil {
		t.Fatal("exact live attempt did not recover after rollback", err)
	}
	if _, err = service.Process(ctx, *job); err == nil {
		t.Fatal("terminal lease executed twice")
	}
	if notificationFor(t, f, f.aliceToken, atomic.ID) == nil {
		t.Fatal("atomic recovery did not actually deliver")
	}

	stale := create("旧分配不会继续提醒", f.alice)
	stale = patch(stale, map[string]any{"assignee_id": nil})
	drainNotifications(t, f)
	if notificationFor(t, f, f.aliceToken, stale.ID) != nil {
		t.Fatal("stale or unassigned task produced a reminder")
	}
	task = patch(task, map[string]any{"assignee_id": f.bob})
	drainNotifications(t, f)
	if notificationFor(t, f, f.aliceToken, task.ID) != nil || notificationFor(t, f, f.bobToken, task.ID) == nil {
		t.Fatal("reassignment retained the wrong responsibility")
	}

	// Append-only evidence is protected independently of the application code.
	if _, err = f.api.DB.Exec(ctx, "UPDATE notification_deliveries SET template_version=1 WHERE thread_id=$1", first.ID); err == nil {
		t.Fatal("application rewrote immutable delivery evidence")
	}
	if _, err = f.owner.Exec(ctx, "UPDATE notification_deliveries SET template_version=1 WHERE thread_id=$1", first.ID); err == nil {
		t.Fatal("owner bypassed immutable delivery trigger")
	}
	exec(`INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,schema_version,payload,request_id)
  VALUES($1,$2,'task.created','task',$3,1,99,'{"private_diagnostic":"never include this body"}',$4)`, core.NewID(), f.org, core.NewID(), core.NewID())
	drainNotifications(t, f)
	rec, res = f.request("GET", "/v1/notification-delivery-failures", f.bobToken, "", nil, nil)
	expect(t, rec, 404, res)
	exec("INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'notification.delivery.read' FROM roles WHERE org_id=$1 AND key='project_manager'", f.org)
	rec, res = f.request("GET", "/v1/notification-delivery-failures", f.bobToken, "", nil, nil)
	expect(t, rec, 200, res)
	failures := data[core.Page[notification.Failure]](t, res)
	if len(failures.Items) != 1 || failures.Items[0].State != "dead" || failures.Items[0].SafeCode == nil || *failures.Items[0].SafeCode != "UNSUPPORTED_EVENT_VERSION" || strings.Contains(rec.Body.String(), "private_diagnostic") {
		t.Fatal("failed delivery vanished or disclosed business payload", rec.Body.String())
	}
	if os.Getenv("CP_BROWSER_NOTIFICATION_FIXTURE") == "1" && !t.Failed() {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
 SELECT $1,$2,$3,id,'org','internal' FROM roles WHERE org_id=$2 AND key='notification_operator'`, core.NewID(), f.org, f.alice)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		own := requireBusinessNotification(t, f, f.aliceToken, other.ID, "task", true)
		foreign := requireBusinessNotification(t, f, f.bobToken, task.ID, "task", true)
		holdBrowserBusinessFixture(t, f, map[string]any{"notification_fixture": true, "notification_id": own.ID, "foreign_notification_id": foreign.ID, "task_id": other.ID, "task_title": other.Title, "project_id": p.ID, "foreign_task_title": task.Title})
	}

	exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL", f.org, f.alice)
	rec, res = f.request("GET", "/v1/notifications/"+notificationFor(t, f, f.bobToken, task.ID).ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 404, res)
	rec, res = f.request("GET", "/v1/notifications/"+notificationForBeforeRevocation(t, f, other.ID), f.aliceToken, "", nil, nil)
	expect(t, rec, 404, res)
	rec, res = f.request("GET", "/v1/notifications/summary", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	if summary := data[notification.Summary](t, res); summary.UnreadCount != 0 || summary.AttentionCount != 0 {
		t.Fatal("summary counted targets after authority loss", summary)
	}
	rec, res = f.request("GET", "/v1/notifications", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	if strings.Contains(rec.Body.String(), other.Title) || len(data[core.Page[notification.Notification]](t, res).Items) != 0 {
		t.Fatal("revoked target title or link remained visible")
	}
}

func notificationForBeforeRevocation(t *testing.T, f *fixture, resource string) string {
	t.Helper()
	var id string
	if err := f.owner.QueryRow(context.Background(), "SELECT id FROM notification_threads WHERE org_id=$1 AND recipient_id=$2 AND resource_id=$3", f.org, f.alice, resource).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
