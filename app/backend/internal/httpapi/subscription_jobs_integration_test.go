package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/party"
)

func TestSubscriptionJobLeases(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	exec := func(t *testing.T, q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"subscription_manager", "forecast_reporter", "party_manager"} {
		exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, key)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	r, v := f.with(t).request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic lease supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "fixed worker billing basis"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	today := time.Now().In(mustLocation(t, "Asia/Hong_Kong")).Format(time.DateOnly)
	one := 1
	cycle := "custom"
	in := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "Synthetic daily forecast", Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: "lease-billing", StartDate: &today, FirstChargeDate: &today, Cycle: &cycle, CustomIntervalDays: &one, Timezone: "Asia/Hong_Kong", RenewalMode: "manual", VendorState: "unknown", ReminderDays: []int{0, 7}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: today, Currency: "HKD", Amount: "1", Plan: "Daily isolated service"}}}, SourceKey: "vendor:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated durable subscription jobs"}
	r, v = f.with(t).request("POST", "/v1/subscriptions", f.aliceToken, core.NewID(), in, nil)
	expect(t, r, 201, v)
	sub := data[finance.Subscription](t, v)
	base := func(s finance.Subscription) finance.Base {
		return finance.Base{ExpectedVersion: s.Version, RevisionID: s.CurrentRevisionID, ContentHash: s.ContentHash, Reason: "exact human configuration"}
	}
	r, v = f.with(t).request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.aliceToken, core.NewID(), finance.SubscriptionTransition{Base: base(sub), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
	expect(t, r, 200, v)
	sub = data[finance.Subscription](t, v)
	get := func() finance.Subscription {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/subscriptions/"+sub.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[finance.Subscription](t, v)
	}
	service := &finance.Service{DB: f.api.DB}
	queue := jobqueue.Queue{DB: f.api.DB}
	owner := "subscription.test." + core.NewID()
	claim := func(t *testing.T) jobqueue.Job {
		t.Helper()
		j, e := queue.Claim(ctx, []string{"subscription.generate.v1"}, "finance", owner, 2*time.Minute)
		if e != nil || j == nil {
			t.Fatal(j, e)
		}
		return *j
	}
	enqueue := func(t *testing.T, payload []byte) jobqueue.Job {
		t.Helper()
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, _, e = queue.Enqueue(ctx, tx, jobqueue.Input{OrgID: f.org, Kind: "subscription.generate.v1", Lane: "finance", DedupKey: "isolated/" + core.NewID(), PayloadVersion: 1, Payload: payload, MaxAttempts: 8})
		if e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		return claim(t)
	}
	var original, reclaimed jobqueue.Job
	t.Run("daily scheduling fixes inputs and the stale fence cannot write", func(t *testing.T) {
		scheduled, e := service.ScheduleSubscriptions(ctx)
		if e != nil || scheduled.Enqueued != 1 || scheduled.LimitReached {
			t.Fatal(scheduled, e)
		}
		again, e := service.ScheduleSubscriptions(ctx)
		if e != nil || again.Enqueued != 0 {
			t.Fatal(again, e)
		}
		original = claim(t)
		forged := original
		forged.FencingToken++
		if _, e = service.ProcessSubscriptionJob(ctx, forged); core.PublicError(e).Code != "JOB_LEASE_LOST" {
			t.Fatal(e)
		}
		if get().Version != sub.Version {
			t.Fatal("forged fence advanced subscription")
		}
	})
	t.Run("expired lease is reclaimed and only its new holder generates forecasts and queued reminders", func(t *testing.T) {
		exec(t, "UPDATE jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", original.ID)
		reclaimed = claim(t)
		if reclaimed.ID != original.ID || reclaimed.FencingToken != original.FencingToken+1 {
			t.Fatal(reclaimed)
		}
		if _, e := service.ProcessSubscriptionJob(ctx, original); core.PublicError(e).Code != "JOB_LEASE_LOST" {
			t.Fatal(e)
		}
		result, e := service.ProcessSubscriptionJob(ctx, reclaimed)
		// Catch up missed reminders: days_before=7 is due for days 0..7,
		// plus the day-0 reminder for today. Queued is not delivered.
		if e != nil || result.Generated == nil || len(result.Generated.GeneratedIDs) != 366 || result.RemindersQueued != 9 || result.ReminderLimitReached {
			t.Fatal(result, e)
		}
		var periods, reminders, costs, payments int
		if e = f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM subscription_periods),(SELECT count(*) FROM subscription_reminders),(SELECT count(*) FROM actual_cost_entries),(SELECT count(*) FROM payment_records)").Scan(&periods, &reminders, &costs, &payments); e != nil || periods != 366 || reminders != 9 || costs != 0 || payments != 0 {
			t.Fatal(periods, reminders, costs, payments, e)
		}
		var falseActor int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM subscription_events WHERE job_id=$1 AND (actor_id IS NOT NULL OR session_id IS NOT NULL OR mfa_verified_at IS NOT NULL)", reclaimed.ID).Scan(&falseActor); e != nil || falseActor != 0 {
			t.Fatal(falseActor, e)
		}
	})
	t.Run("repeating the fixed window cannot duplicate periods reminders or claim delivery", func(t *testing.T) {
		if _, e := service.ProcessSubscriptionJob(ctx, reclaimed); core.PublicError(e).Code != "JOB_LEASE_LOST" {
			t.Fatal(e)
		}
		j := enqueue(t, reclaimed.Payload)
		result, e := service.ProcessSubscriptionJob(ctx, j)
		if e != nil || result.Generated == nil || len(result.Generated.GeneratedIDs) != 0 || result.Generated.ExistingCount != 366 || result.RemindersQueued != 0 {
			t.Fatal(result, e)
		}
		var queued int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE event_type='subscription.reminder.due' AND payload->>'delivery_state'='queued'").Scan(&queued); e != nil || queued != 9 {
			t.Fatal(queued, e)
		}
		r, v := f.with(t).request("GET", "/v1/subscriptions/"+sub.ID+"/reminders?limit=2", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[finance.SubscriptionReminder]](t, v)
		if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].DeliveryState != "queued" {
			t.Fatal(page)
		}
		r, v = f.with(t).request("GET", "/v1/subscriptions/"+sub.ID+"/reminders", f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
	})
	t.Run("queued subscription reminders deliver in-app without inventing a payment", func(t *testing.T) {
		drainNotifications(t, f)
		var period string
		if e := f.owner.QueryRow(ctx, "SELECT period_id FROM subscription_reminders WHERE days_before=0").Scan(&period); e != nil {
			t.Fatal(e)
		}
		n := requireBusinessNotification(t, f, f.aliceToken, period, "subscription_due", true)
		if n.Target.Type != "subscription_period" || n.Target.ContainerID == nil || *n.Target.ContainerID != sub.ID || notificationFor(t, f, f.bobToken, period) != nil {
			t.Fatal("subscription notification ignored the actual period owner", n)
		}
		r, v := f.with(t).request("GET", "/v1/subscriptions/"+sub.ID+"/reminders", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[finance.SubscriptionReminder]](t, v)
		if len(page.Items) != 9 {
			t.Fatal(page)
		}
		for _, item := range page.Items {
			if item.DeliveryState != "delivered" {
				t.Fatal("actual delivery still labelled queued", item)
			}
		}
		var payments int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM payment_records").Scan(&payments); e != nil || payments != 0 {
			t.Fatal("reminder invented a payment", payments, e)
		}
	})
	t.Run("reminder date recipient and running lease are independently enforced by PostgreSQL", func(t *testing.T) {
		j := enqueue(t, reclaimed.Payload)
		var period, outbox string
		if e := f.owner.QueryRow(ctx, "SELECT period_id,outbox_id FROM subscription_reminders WHERE days_before=0").Scan(&period, &outbox); e != nil {
			t.Fatal(e)
		}
		_, e := f.api.DB.Exec(ctx, `INSERT INTO subscription_reminders(id,org_id,period_id,days_before,remind_on,recipient_id,outbox_id,job_id,job_fence,job_owner) VALUES($1,$2,$3,0,$4::date+1,$5,$6,$7,$8,$9)`, core.NewID(), f.org, period, today, f.bob, outbox, j.ID, j.FencingToken, j.LeaseOwner)
		if e == nil || !strings.Contains(e.Error(), "reminder must pin") {
			t.Fatal(e)
		}
		if _, e = service.ProcessSubscriptionJob(ctx, j); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("superseded configuration completes without generating against a newer snapshot", func(t *testing.T) {
		j := enqueue(t, reclaimed.Payload)
		s := get()
		c := s.SubscriptionContent
		c.Notes = "New exact configuration after scheduling"
		r, v := f.with(t).request("PATCH", "/v1/subscriptions/"+s.ID, f.aliceToken, core.NewID(), finance.SubscriptionPatch{SubscriptionContent: c, Base: base(s)}, nil)
		expect(t, r, 200, v)
		s = data[finance.Subscription](t, v)
		result, e := service.ProcessSubscriptionJob(ctx, j)
		if e != nil || !result.Skipped || result.Generated != nil || get().Version != s.Version {
			t.Fatal(result, e)
		}
		_, e = f.api.DB.Exec(ctx, `INSERT INTO subscription_events(id,org_id,subscription_id,subscription_version,revision_id,kind,state,job_id,job_fence,job_owner,reason) VALUES($1,$2,$3,$4,$5,'periods_generated','active',$6,$7,$8,'expired worker identity')`, core.NewID(), f.org, s.ID, s.Version+1, s.CurrentRevisionID, j.ID, j.FencingToken, j.LeaseOwner)
		if e == nil || !strings.Contains(e.Error(), "actual current job lease") {
			t.Fatal(e)
		}
	})
	t.Run("audit failure rolls back generation head reminders and job completion together", func(t *testing.T) {
		s := get()
		date, _ := time.Parse(time.DateOnly, today)
		var control finance.SubscriptionGenerationJob
		if e := json.Unmarshal(reclaimed.Payload, &control); e != nil {
			t.Fatal(e)
		}
		input := finance.SubscriptionGenerationJob{SubscriptionID: s.ID, ConfigurationRevisionID: s.CurrentRevisionID, ConfigurationHash: s.ContentHash, ControlVersion: control.ControlVersion, BusinessDate: today, Window: "forward", From: date.AddDate(0, 0, 366).Format(time.DateOnly), To: date.AddDate(0, 0, 367).Format(time.DateOnly)}
		payload, _ := json.Marshal(input)
		j := enqueue(t, payload)
		exec(t, `CREATE FUNCTION fail_subscription_worker_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='subscription.periods.generated' THEN RAISE EXCEPTION 'isolated system audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_subscription_worker_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_subscription_worker_audit()`)
		defer exec(t, "DROP TRIGGER IF EXISTS fail_subscription_worker_audit ON audit_events; DROP FUNCTION IF EXISTS fail_subscription_worker_audit()")
		if _, e := service.ProcessSubscriptionJob(ctx, j); e == nil {
			t.Fatal("worker audit failure ignored")
		}
		if get().Version != s.Version {
			t.Fatal("failed worker changed head")
		}
		var count int
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM subscription_periods),state FROM jobs WHERE id=$1", j.ID).Scan(&count, &state); e != nil || count != 366 || state != "running" {
			t.Fatal(count, state, e)
		}
		exec(t, "DROP TRIGGER fail_subscription_worker_audit ON audit_events; DROP FUNCTION fail_subscription_worker_audit()")
		result, e := service.ProcessSubscriptionJob(ctx, j)
		if e != nil || result.Generated == nil || len(result.Generated.GeneratedIDs) != 1 {
			t.Fatal(result, e)
		}
	})
}
