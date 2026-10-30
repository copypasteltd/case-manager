package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/notification"
	status "copypaste.hk/platform/internal/servicestatus"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The browser fixture uses this same real consumer/queue, never a fabricated
// delivery receipt. Scheduling, leased processing and results remain durable.
func drainResourceJobs(ctx context.Context, db *pgxpool.Pool) error {
	service := &notification.Service{DB: db}
	queue := jobqueue.Queue{DB: db}
	if _, _, e := service.ScheduleResources(ctx, ""); e != nil {
		return e
	}
	for i := 0; i < 1000; i++ {
		d, e := queue.DispatchOne(ctx, notification.Consumer())
		if e != nil {
			return e
		}
		if d == nil {
			break
		}
		if i == 999 {
			return fmt.Errorf("resource fixture dispatch limit")
		}
	}
	for i := 0; i < 1000; i++ {
		j, e := queue.Claim(ctx, []string{notification.JobKind}, "notification", "resource.notification.fixture", time.Minute)
		if e != nil {
			return e
		}
		if j == nil {
			return nil
		}
		if _, e = service.Process(ctx, *j); e != nil {
			return e
		}
	}
	return fmt.Errorf("resource fixture delivery limit")
}

func TestResourceNotifications(t *testing.T) {
	f := setup(t)
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	})
	ctx := context.Background()
	operationsRole(t, f, f.alice, "status_operator")
	reader := operationsRole(t, f, f.bob, "status_reader")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = f.login("bob", "android").AccessToken
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	run := func(name string, fn func(*testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	create := func(name string) status.Record {
		t.Helper()
		r, v := f.request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{Content: status.Content{Name: name, OwnerID: f.alice, HostLabel: "synthetic", EndpointLabel: "no external target"}, Classification: "internal", Reason: "isolated resource notification journey"}, nil)
		expect(t, r, 201, v)
		return data[status.Record](t, v)
	}
	get := func(d status.Record) status.ResourceView {
		t.Helper()
		r, v := f.request("GET", "/v1/services/"+d.ID+"/resource-monitor", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[status.ResourceView](t, v)
	}
	publish := func(d status.Record, enabled bool) status.ResourceView {
		t.Helper()
		base := get(d)
		in := status.ResourceConfigInput{ExpectedVersion: base.ServiceVersion, ResourceConfig: status.ResourceConfig{Name: "显式资源提醒策略", Enabled: enabled, PublicKey: base64.StdEncoding.EncodeToString(pub), StaleAfter: 45, CPU: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Memory: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Disk: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Mountpoints: []string{"/"}}, Reason: "explicit immutable source policy"}
		if base.Source != nil {
			in.ExpectedSourceVersion = base.Source.Version
			in.BaseRevisionID = &base.Source.RevisionID
		}
		r, v := f.request("POST", "/v1/services/"+d.ID+"/resource-monitor/revisions", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 200, v)
		return get(d)
	}
	push := func(t *testing.T, d status.Record, seq string, cpu int, readonly bool, at time.Time, change ...func(*status.ResourceMetrics)) status.ObservationAck {
		t.Helper()
		source := get(d).Source
		in := status.SignedResourceObservation{RevisionID: source.RevisionID, Sequence: seq, ObservedAt: at, ResourceMetrics: status.ResourceMetrics{CPUBP: cpu, Memory: status.MemorySample{Total: "9223372036854775807", Available: "9000000000000000000"}, Filesystems: []status.DiskSample{{Mountpoint: "/", MemorySample: status.MemorySample{Total: "10000000000", Available: "8000000000"}, ReadOnly: readonly}}}}
		for _, apply := range change {
			apply(&in.ResourceMetrics)
		}
		raw, _ := json.Marshal(in)
		r, v := f.with(t).request("POST", "/v1/resource-sources/"+source.ID+"/observations", "", "", in, map[string]string{"X-CP-Status-Signature": base64.StdEncoding.EncodeToString(ed25519.Sign(key, status.ResourceSigningMessage(source.ID, raw)))})
		expect(t, r, 200, v)
		return data[status.ObservationAck](t, v)
	}
	history := func(t *testing.T, d status.Record) []status.ResourceHealthEvent {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/services/"+d.ID+"/resource-monitor/health-events?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[core.Page[status.ResourceHealthEvent]](t, v).Items
	}
	work := func(t *testing.T, token, id string) bool {
		t.Helper()
		for _, item := range operationsGroup(t, f, token, "resource_attention").Items {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	drain := func(t *testing.T) {
		t.Helper()
		if e := drainResourceJobs(ctx, f.api.DB); e != nil {
			t.Fatal(e)
		}
	}
	update := func(t *testing.T, d status.Record, owner, life string) status.Record {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/services/"+d.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		d = data[status.Record](t, v)
		d.OwnerID = owner
		r, v = f.with(t).request("PATCH", "/v1/services/"+d.ID, f.aliceToken, core.NewID(), status.Patch{ExpectedVersion: d.Version, Content: status.Content{Name: d.Name, OwnerID: owner, HostLabel: d.HostLabel, EndpointLabel: d.EndpointLabel, Description: d.Description}, Lifecycle: life, Reason: "explicit duty update"}, nil)
		expect(t, r, 200, v)
		return data[status.Record](t, v)
	}
	service := create("合成主机告警 <b>原样标题</b>")
	initial := publish(service, true)
	waiting := create("等待首次采样的显式来源")
	waitingView := publish(waiting, true)
	var first *notification.Notification
	var criticalObservation status.ObservationAck
	run("initial no-sample grace is unknown without a notification or work", func(t *testing.T) {
		drain(t)
		if work(t, f.aliceToken, service.ID) || notificationFor(t, f, f.aliceToken, service.ID) != nil {
			t.Fatal("initial waiting fabricated an alarm")
		}
		events := history(t, service)
		if len(events) != 1 || events[0].State != "unknown" || events[0].Notify || events[0].Attention {
			t.Fatal(events)
		}
	})
	run("healthy baseline agrees with exact SQL capacity and sends no alarm", func(t *testing.T) {
		push(t, service, "1", 3500, false, time.Now().UTC())
		drain(t)
		v := get(service)
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT state FROM service_resource_current($1,$2,clock_timestamp())", f.org, v.Source.ID).Scan(&state); e != nil || state != v.State || state != "operational" {
			t.Fatal(state, v.State, e)
		}
		if work(t, f.aliceToken, service.ID) || notificationFor(t, f, f.aliceToken, service.ID) != nil {
			t.Fatal("healthy baseline created work")
		}
	})
	run("threshold and readonly transition creates one immutable addressed delivery", func(t *testing.T) {
		criticalObservation = push(t, service, "2", 9700, true, time.Now().UTC())
		drain(t)
		first = requireBusinessNotification(t, f, f.aliceToken, service.ID, "service_resource", true)
		if first.Target.Type != "service" || first.Target.WebPath != "/services/"+service.ID || first.CurrentState != "resource_attention" || first.DeliverySeq != 1 || !work(t, f.aliceToken, service.ID) || work(t, f.bobToken, service.ID) || notificationFor(t, f, f.bobToken, service.ID) != nil {
			t.Fatal(first)
		}
		events := history(t, service)
		if len(events) != 3 || events[0].ID != first.BusinessEventID || events[0].ObservationID == nil || *events[0].ObservationID != criticalObservation.ID || !events[0].Notify || events[0].RecipientID != f.alice {
			t.Fatal(events)
		}
	})
	run("same-state samples and concurrent schedulers do not spam recipients", func(t *testing.T) {
		push(t, service, "3", 9800, true, time.Now().UTC())
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, e := (&notification.Service{DB: f.api.DB}).ScheduleResources(ctx, "")
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		drain(t)
		n := requireBusinessNotification(t, f, f.aliceToken, service.ID, "service_resource", true)
		if n.DeliverySeq != 1 || len(history(t, service)) != 3 {
			t.Fatal("repeated state notified again", n)
		}
	})
	run("reading records only readership and does not resolve resource work", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/notifications/"+first.ID+"/read", f.aliceToken, core.NewID(), notification.ReadInput{DeliverySeq: 1}, nil)
		expect(t, r, 200, v)
		n := data[notification.Notification](t, v)
		if !n.Read || !n.AttentionRequired || !work(t, f.aliceToken, service.ID) {
			t.Fatal(n)
		}
	})
	run("real recovery removes work and creates a fresh unread delivery in the same revision", func(t *testing.T) {
		push(t, service, "4", 3500, false, time.Now().UTC())
		if work(t, f.aliceToken, service.ID) {
			t.Fatal("recovery waits for delivery worker")
		}
		drain(t)
		n := requireBusinessNotification(t, f, f.aliceToken, service.ID, "service_resource", false)
		if n.ID != first.ID || n.DeliverySeq != 2 || n.Read || n.ReadSeq != 1 || n.CurrentState != "resource_record" {
			t.Fatal(n)
		}
	})
	run("queued old alert is suppressed after newer recovery", func(t *testing.T) {
		push(t, service, "5", 9700, false, time.Now().UTC())
		old := history(t, service)[0]
		push(t, service, "6", 3500, false, time.Now().UTC())
		drain(t)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND business_event_id=$2", f.org, old.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal(count, e)
		}
		if n := requireBusinessNotification(t, f, f.aliceToken, service.ID, "service_resource", false); n.DeliverySeq != 3 {
			t.Fatal(n)
		}
	})
	run("current owner transfer never reinterprets the old queued recipient", func(t *testing.T) {
		push(t, service, "7", 9700, false, time.Now().UTC())
		old := history(t, service)[0]
		service = update(t, service, f.bob, "active")
		if work(t, f.aliceToken, service.ID) || !work(t, f.bobToken, service.ID) {
			t.Fatal("assignment did not move actual work")
		}
		drain(t)
		bob := requireBusinessNotification(t, f, f.bobToken, service.ID, "service_resource", true)
		if bob.BusinessEventID == old.ID || bob.DeliverySeq != 1 {
			t.Fatal(bob)
		}
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND business_event_id=$2", f.org, old.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal(count, e)
		}
		if n := notificationFor(t, f, f.aliceToken, service.ID); n == nil || n.AttentionRequired {
			t.Fatal(n)
		}
	})
	run("archive and explicit disable clear duty without manufacturing a recovery", func(t *testing.T) {
		service = update(t, service, f.bob, "archived")
		if work(t, f.bobToken, service.ID) {
			t.Fatal("archived resource remains work")
		}
		events := history(t, service)
		if events[0].Notify || events[0].StateReason != "SERVICE_ARCHIVED" {
			t.Fatal(events[0])
		}
		service = update(t, service, f.alice, "active")
		drain(t)
		publish(service, false)
		drain(t)
		if work(t, f.aliceToken, service.ID) {
			t.Fatal("disabled resource remains work")
		}
		events = history(t, service)
		if events[0].Notify || events[0].StateReason != "SOURCE_DISABLED" {
			t.Fatal(events[0])
		}
	})
	run("new configuration has its own exact context and never inherits old samples", func(t *testing.T) {
		v := publish(service, true)
		if v.Source.RevisionID == initial.Source.RevisionID || v.Observation != nil || work(t, f.aliceToken, service.ID) {
			t.Fatal("new source inherited an old alert", v)
		}
		push(t, service, "1", 8500, false, time.Now().UTC())
		drain(t)
		n := requireBusinessNotification(t, f, f.aliceToken, service.ID, "service_resource", true)
		if n.ID == first.ID || n.DeliverySeq != 1 {
			t.Fatal("new revision reused old delivery context", n)
		}
	})
	run("history paginates exact immutable bases and original signed evidence", func(t *testing.T) {
		path := "/v1/services/" + service.ID + "/resource-monitor/health-events"
		r, v := f.with(t).request("GET", path+"?limit=2", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[status.ResourceHealthEvent]](t, v)
		if len(page.Items) != 2 || page.NextCursor == nil {
			t.Fatal(page)
		}
		r, v = f.with(t).request("GET", path+"?limit=2&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		next := data[core.Page[status.ResourceHealthEvent]](t, v)
		if len(next.Items) != 2 || next.Items[0].Sequence >= page.Items[1].Sequence {
			t.Fatal(next)
		}
		r, v = f.with(t).request("GET", "/v1/services/"+waiting.ID+"/resource-monitor/health-events?cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, r, 400, v)
		r, v = f.with(t).request("GET", "/v1/services/"+service.ID+"/resource-monitor/observations/"+criticalObservation.ID+"/evidence", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[status.ResourceProof](t, v).ID != criticalObservation.ID {
			t.Fatal("basis was substituted")
		}
		if _, e := f.owner.Exec(ctx, "UPDATE service_resource_health_events SET attention=false WHERE org_id=$1 AND id=$2", f.org, page.Items[0].ID); e == nil {
			t.Fatal("history changed")
		}
		if _, e := f.owner.Exec(ctx, "UPDATE service_resource_health SET state='operational' WHERE org_id=$1 AND source_id=$2", f.org, get(service).Source.ID); e == nil {
			t.Fatal("head detached from event")
		}
	})
	run("memory and disk thresholds use exact integer parity and never conflate availability", func(t *testing.T) {
		for i, sample := range []struct {
			state  string
			change func(*status.ResourceMetrics)
		}{
			{"degraded", func(m *status.ResourceMetrics) { m.Memory = status.MemorySample{Total: "10000", Available: "2000"} }},
			{"critical", func(m *status.ResourceMetrics) { m.Memory.Available = "1" }},
			{"degraded", func(m *status.ResourceMetrics) {
				m.Filesystems[0].MemorySample = status.MemorySample{Total: "10000", Available: "2000"}
			}},
			{"critical", func(m *status.ResourceMetrics) {
				m.Filesystems[0].MemorySample = status.MemorySample{Total: "10000", Available: "500"}
			}},
			{"critical", func(m *status.ResourceMetrics) { m.Filesystems[0].ReadOnly = true }},
		} {
			push(t, service, fmt.Sprint(i+2), 3500, false, time.Now().UTC(), sample.change)
			v := get(service)
			var state string
			if e := f.owner.QueryRow(ctx, "SELECT state FROM service_resource_current($1,$2,clock_timestamp())", f.org, v.Source.ID).Scan(&state); e != nil || state != sample.state || state != v.State {
				t.Fatal(sample.state, state, v.State, e)
			}
		}
		var capacity int
		if e := f.owner.QueryRow(ctx, `SELECT resource_capacity_bp('{"total_bytes":"9223372036854775807","available_bytes":"9223372036854775806"}'::jsonb)::int`).Scan(&capacity); e != nil || capacity != 1 {
			t.Fatal("capacity rounded away real bytes", capacity, e)
		}
	})
	run("actual sample expiry and first-sample deadline become work before queue delivery", func(t *testing.T) {
		publish(service, true)
		at := time.Now().UTC().Add(-42 * time.Second)
		push(t, service, "1", 3500, false, at)
		if work(t, f.aliceToken, service.ID) {
			t.Fatal("fresh sample was stale early")
		}
		if delay := time.Until(at.Add(45*time.Second + 100*time.Millisecond)); delay > 0 {
			time.Sleep(delay)
		}
		if !work(t, f.aliceToken, service.ID) {
			t.Fatal("expired resource hidden behind delivery cache")
		}
		drain(t)
		events := history(t, service)
		if events[0].StateReason != "OBSERVATION_STALE" || !events[0].Notify {
			t.Fatal(events[0])
		}
		if delay := time.Until(waitingView.Source.CreatedAt.Add(45*time.Second + 100*time.Millisecond)); delay > 0 {
			time.Sleep(delay)
		}
		if !work(t, f.aliceToken, waiting.ID) {
			t.Fatal("never-sampled enabled source silently ignored")
		}
		drain(t)
		events = history(t, waiting)
		if events[0].StateReason != "NO_OBSERVATION" || !events[0].Notify || !events[0].Attention {
			t.Fatal(events[0])
		}
	})
	run("revoked target authority suppresses queued delivery and hides existing inbox and evidence", func(t *testing.T) {
		service = update(t, service, f.bob, "active")
		drain(t)
		n := requireBusinessNotification(t, f, f.bobToken, service.ID, "service_resource", true)
		queued := create("转交前撤销权限的合成来源")
		publish(queued, true)
		push(t, queued, "1", 9700, false, time.Now().UTC())
		queued = update(t, queued, f.bob, "active")
		event := history(t, queued)[0]
		if _, e := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND id=$2", f.org, reader); e != nil {
			t.Fatal(e)
		}
		drain(t)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND business_event_id=$2", f.org, event.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal(count, e)
		}
		for _, path := range []string{"/v1/notifications/" + n.ID, "/v1/services/" + service.ID + "/resource-monitor/health-events", "/v1/services/" + service.ID + "/resource-monitor/observations/" + criticalObservation.ID + "/evidence"} {
			r, _ := f.with(t).request("GET", path, f.bobToken, "", nil, nil)
			if r.Code == 200 {
				t.Fatal("revoked access survived", path)
			}
		}
	})
}
