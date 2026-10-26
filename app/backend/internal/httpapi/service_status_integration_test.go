package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	status "copypaste.hk/platform/internal/servicestatus"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestServiceStatus(t *testing.T) {
	run := func(name string, fn func(*testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	f := setup(t)
	ctx := context.Background()
	exec := func(t *testing.T, q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	project := f.createProject("Status shared scope", []string{f.bob, f.member})
	hiddenProject := f.createProject("Status hidden scope", nil)
	in := status.Create{Content: status.Content{Name: "Synthetic business API", OwnerID: f.alice, HostLabel: "fixture-host", EndpointLabel: "fixture-only; no network target", Description: "Isolated signed monitoring fixture"}, ProjectID: &project.ID, Classification: "project_restricted", Reason: "explicit isolated service fixture"}
	var d, hidden status.Record
	var source status.Source
	var incident status.Incident
	var adminToken string
	public, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	sourceInput := status.SourceInput{SourceContent: status.SourceContent{Name: "Synthetic mature-tool bridge", Provider: "adapter", ExternalRef: "fixture-monitor-1", PublicKey: base64.StdEncoding.EncodeToString(public), Enabled: true, Thresholds: status.Thresholds{FailureCount: 3, RecoveryCount: 2, Interval: 60, StaleAfter: 180, LatencyMS: 1000}}, Reason: "publish explicit isolated monitoring policy"}
	get := func(t *testing.T) status.Record {
		t.Helper()
		rec, res := f.with(t).request("GET", "/v1/services/"+d.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[status.Record](t, res)
	}
	rawPush := func(t *testing.T, body []byte, sig []byte) (*httptest.ResponseRecorder, response) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/status-sources/"+source.ID+"/observations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CP-Status-Signature", base64.StdEncoding.EncodeToString(sig))
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		var res response
		if e := json.Unmarshal(rec.Body.Bytes(), &res); e != nil {
			t.Fatal(e)
		}
		return rec, res
	}
	base := time.Now().UTC().Add(-5 * time.Second)
	payload := func(seq int64, healthy bool, observed time.Time) []byte {
		latency := int64(50)
		wire, _ := json.Marshal(status.SignedObservation{RevisionID: source.RevisionID, Sequence: fmt.Sprint(seq), ObservedAt: observed, Healthy: healthy, LatencyMS: &latency})
		return wire
	}
	push := func(t *testing.T, seq int64, healthy bool, observed time.Time) status.ObservationAck {
		t.Helper()
		wire := payload(seq, healthy, observed)
		rec, res := rawPush(t, wire, ed25519.Sign(private, status.SigningMessage(source.ID, wire)))
		expect(t, rec, 200, res)
		return data[status.ObservationAck](t, res)
	}
	run("unbound roles and administrator do not grant service content access", func(t *testing.T) {
		ff := f.with(t)
		rec, res := ff.request("POST", "/v1/services", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 404, res)
		for _, binding := range []struct{ user, role string }{{f.alice, "status_operator"}, {f.bob, "status_reader"}, {f.member, "status_reader"}} {
			exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, binding.user, binding.role)
		}
		rec, res = ff.request("POST", "/v1/services", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 401, res)
		f.aliceToken = memberMFAToken(ff, "alice", "android")
		f.bobToken = ff.login("bob", "android").AccessToken
		f.memberToken = ff.login("member", "android").AccessToken
		adminToken = ff.administratorToken()
		rec, res = ff.request("GET", "/v1/services", adminToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[status.Record]](t, res).Items) != 0 {
			t.Fatal("administrator received business content")
		}
	})
	run("creation is idempotent and no source means unknown", func(t *testing.T) {
		ff := f.with(t)
		key := core.NewID()
		rec, res := ff.request("POST", "/v1/services", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
		d = data[status.Record](t, res)
		if d.State != status.Unknown || d.ObservedState != status.Unknown || len(d.Sources) != 0 || !d.CanManage {
			t.Fatal(d)
		}
		rec, res = ff.request("POST", "/v1/services", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
		if data[status.Record](t, res).ID != d.ID || rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("duplicate creation")
		}
		hiddenIn := in
		hiddenIn.Name = "Synthetic restricted service"
		hiddenIn.ProjectID = &hiddenProject.ID
		rec, res = ff.request("POST", "/v1/services", f.aliceToken, core.NewID(), hiddenIn, nil)
		expect(t, rec, 201, res)
		hidden = data[status.Record](t, res)
		rec, res = ff.request("GET", "/v1/services/"+d.ID, adminToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = ff.request("GET", "/v1/services/"+d.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[status.Record](t, res).CanManage {
			t.Fatal("reader can manage")
		}
		rec, res = ff.request("GET", "/v1/services?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[status.Record]](t, res)
		if len(page.Items) != 1 || page.Items[0].ID != d.ID || page.NextCursor != nil {
			t.Fatal("ACL was applied after pagination")
		}
	})
	run("published source is immutable and requires exact service version", func(t *testing.T) {
		ff := f.with(t)
		sourceInput.ExpectedVersion = d.Version
		key := core.NewID()
		rec, res := ff.request("POST", "/v1/services/"+d.ID+"/sources", f.aliceToken, key, sourceInput, nil)
		expect(t, rec, 200, res)
		d = data[status.Record](t, res)
		source = d.Sources[0]
		if source.Projection != nil || source.State != status.Unknown || d.UnknownSources != 1 {
			t.Fatal("unobserved source became healthy")
		}
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/sources", f.aliceToken, key, sourceInput, nil)
		expect(t, rec, 200, res)
		if len(data[status.Record](t, res).Sources) != 1 {
			t.Fatal("duplicate source")
		}
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/sources", f.aliceToken, core.NewID(), sourceInput, nil)
		expect(t, rec, 409, res)
		if _, e := f.api.DB.Exec(ctx, "UPDATE service_source_revisions SET enabled=false WHERE id=$1", source.RevisionID); e == nil {
			t.Fatal("mutable monitoring policy")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM service_source_revisions WHERE id=$1", source.RevisionID); e == nil {
			t.Fatal("deletable monitoring policy")
		}
	})
	run("signature scope wire fields and replay are checked before persistence", func(t *testing.T) {
		wire := payload(1, true, base.Add(time.Millisecond))
		sig := ed25519.Sign(private, status.SigningMessage(source.ID, wire))
		bad := append([]byte(nil), sig...)
		bad[0] ^= 1
		rec, res := rawPush(t, wire, bad)
		expect(t, rec, 401, res)
		rec, res = rawPush(t, wire, ed25519.Sign(private, status.SigningMessage(core.NewID(), wire)))
		expect(t, rec, 401, res)
		duplicate := append([]byte(`{"healthy":false,`), wire[1:]...)
		rec, res = rawPush(t, duplicate, ed25519.Sign(private, status.SigningMessage(source.ID, duplicate)))
		expect(t, rec, 422, res)
		future := payload(1, true, time.Now().Add(time.Minute))
		rec, res = rawPush(t, future, ed25519.Sign(private, status.SigningMessage(source.ID, future)))
		expect(t, rec, 422, res)
		rec, res = rawPush(t, wire, sig)
		expect(t, rec, 200, res)
		first := data[status.ObservationAck](t, res)
		if !first.Applied || first.Replayed {
			t.Fatal(first)
		}
		rec, res = rawPush(t, wire, sig)
		expect(t, rec, 200, res)
		again := data[status.ObservationAck](t, res)
		if again.ID != first.ID || !again.Replayed {
			t.Fatal("replay duplicated observation")
		}
		conflicting := payload(1, false, base.Add(time.Millisecond))
		rec, res = rawPush(t, conflicting, ed25519.Sign(private, status.SigningMessage(source.ID, conflicting)))
		expect(t, rec, 409, res)
		var facts, audits, events int
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM service_observations WHERE source_id=$1),(SELECT count(*) FROM audit_events WHERE resource_id=$2 AND action='service.observation_received'),(SELECT count(*) FROM outbox_events WHERE aggregate_id=$2 AND event_type='service.observation_received')`, source.ID, source.RevisionID).Scan(&facts, &audits, &events); e != nil {
			t.Fatal(e)
		}
		if facts != 1 || audits != 1 || events != 1 {
			t.Fatal(facts, audits, events)
		}
		req := httptest.NewRequest("POST", "/v1/status-sources/"+source.ID+"/observations", bytes.NewReader(wire))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.aliceToken)
		req.Header.Set("X-CP-Status-Signature", base64.StdEncoding.EncodeToString(sig))
		record := httptest.NewRecorder()
		f.handler.ServeHTTP(record, req)
		if record.Code != 403 {
			t.Fatal("employee session accepted as probe")
		}
	})
	run("three failures and two recoveries project durable state without changing configuration version", func(t *testing.T) {
		checks := []struct {
			seq     int64
			healthy bool
			want    status.State
		}{{2, true, status.Operational}, {3, false, status.Degraded}, {4, false, status.Degraded}, {5, false, status.Down}, {6, true, status.Down}, {7, true, status.Operational}}
		for _, check := range checks {
			push(t, check.seq, check.healthy, base.Add(time.Duration(check.seq)*time.Millisecond))
			view := get(t)
			if view.State != check.want || view.Version != d.Version {
				t.Fatalf("seq %d: state=%s version=%d", check.seq, view.State, view.Version)
			}
		}
		f.handler = f.api.Handler()
		if get(t).State != status.Operational {
			t.Fatal("new service instance lost durable projection")
		}
		projection := *get(t).Sources[0].Projection
		exec(t, "DELETE FROM service_source_states WHERE org_id=$1 AND revision_id=$2", f.org, source.RevisionID)
		missing := get(t)
		if len(missing.Sources) != 1 || missing.UnknownSources != 1 || missing.State != status.Unknown {
			t.Fatal("missing projection silently dropped a required source")
		}
		exec(t, `INSERT INTO service_source_states(org_id,source_id,revision_id,last_sequence,observed_at,received_at,state,failure_streak,success_streak) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, f.org, source.ID, source.RevisionID, projection.Sequence, projection.ObservedAt, projection.ReceivedAt, projection.State, projection.Failures, projection.Successes)
		rec, res := f.with(t).request("GET", "/v1/services/"+d.ID+"/observations?limit=2", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[status.Observation]](t, res)
		if len(page.Items) != 2 || page.NextCursor == nil {
			t.Fatal(page)
		}
		rec, res = f.with(t).request("GET", "/v1/services/"+d.ID+"/observations/"+page.Items[0].ID+"/evidence", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		proof := data[status.ObservationProof](t, res)
		body, _ := base64.StdEncoding.DecodeString(proof.SignedPayload)
		signature, _ := base64.StdEncoding.DecodeString(proof.Signature)
		key, _ := base64.StdEncoding.DecodeString(proof.PublicKey)
		if core.Digest(body) != proof.PayloadSHA256 || !ed25519.Verify(ed25519.PublicKey(key), status.SigningMessage(proof.SourceID, body), signature) {
			t.Fatal("saved evidence cannot be independently verified")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE service_observations SET healthy=false WHERE id=$1", proof.ID); e == nil {
			t.Fatal("mutable observation")
		}
	})
	run("late data and sequence gaps retain facts without reviving a previous green state", func(t *testing.T) {
		before := get(t)
		ack := push(t, 8, false, base.Add(-time.Second))
		if ack.Applied || ack.IgnoredReason != "LATE_OBSERVATION" || get(t).Sources[0].Projection.Sequence != 7 {
			t.Fatal(ack)
		}
		push(t, 20, true, base.Add(20*time.Millisecond))
		if get(t).State != status.Unknown {
			t.Fatal("gap inherited previous healthy streak")
		}
		ack = push(t, 19, false, base.Add(19*time.Millisecond))
		if ack.Applied || ack.IgnoredReason != "LATE_SEQUENCE" {
			t.Fatal(ack)
		}
		for _, seq := range []int64{21, 22, 23} {
			push(t, seq, false, base.Add(time.Duration(seq)*time.Millisecond))
		}
		if current := get(t); current.State != status.Down || current.Version != before.Version {
			t.Fatal(current)
		}
	})
	run("maintenance overlays raw down state and cancellation is append only", func(t *testing.T) {
		ff := f.with(t)
		d = get(t)
		window := status.MaintenanceInput{ExpectedVersion: d.Version, Start: time.Now().UTC().Add(-time.Second), End: time.Now().UTC().Add(8 * time.Second), Reason: "isolated scheduled maintenance"}
		key := core.NewID()
		rec, res := ff.request("POST", "/v1/services/"+d.ID+"/maintenance-windows", f.aliceToken, key, window, nil)
		expect(t, rec, 200, res)
		d = data[status.Record](t, res)
		if d.State != status.Maintenance || d.ObservedState != status.Down || d.MaintenanceID == nil {
			t.Fatal(d)
		}
		if d.RecheckAt.Sub(window.End) > time.Microsecond || window.End.Sub(d.RecheckAt) > time.Microsecond {
			t.Fatal("client recheck deadline passed the maintenance boundary", d.RecheckAt, window.End)
		}
		id := *d.MaintenanceID
		window.ExpectedVersion = d.Version
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/maintenance-windows", f.aliceToken, core.NewID(), window, nil)
		expect(t, rec, 409, res)
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/maintenance-windows/"+id+"/cancel", f.aliceToken, core.NewID(), status.MaintenanceCancel{ExpectedVersion: d.Version, Reason: "maintenance cancelled"}, nil)
		expect(t, rec, 200, res)
		d = data[status.Record](t, res)
		if d.State != status.Down || d.MaintenanceID != nil {
			t.Fatal(d)
		}
		// Replaying the original scheduling command returns the current facts.
		window.ExpectedVersion = sourceInput.ExpectedVersion + 1
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/maintenance-windows", f.aliceToken, key, window, nil)
		expect(t, rec, 200, res)
		if data[status.Record](t, res).State != status.Down {
			t.Fatal("replay returned obsolete maintenance state")
		}
		rec, res = ff.request("GET", "/v1/services/"+d.ID+"/maintenance-windows", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		windows := data[core.Page[status.MaintenanceWindow]](t, res)
		if len(windows.Items) != 1 || windows.Items[0].CancelledAt == nil {
			t.Fatal(windows)
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM maintenance_cancellations WHERE window_id=$1", id); e == nil {
			t.Fatal("cancel history deletable")
		}
	})
	run("incident impacts are checked as a whole and business deduplication is durable", func(t *testing.T) {
		ff := f.with(t)
		create := status.IncidentCreate{Title: "Synthetic multi-service incident", ServiceIDs: []string{d.ID, hidden.ID}, OwnerID: f.alice, DedupKey: "fixture-incident-1", Severity: "high", Description: "Synthetic scope", TimelineNote: "investigation opened", Reason: "record actual isolated incident"}
		rec, res := ff.request("POST", "/v1/incidents", f.aliceToken, core.NewID(), create, nil)
		expect(t, rec, 201, res)
		incident = data[status.Incident](t, res)
		drainNotifications(t, f)
		requireBusinessNotification(t, f, f.aliceToken, incident.ID, "incident", true)
		if notificationFor(t, f, f.bobToken, incident.ID) != nil {
			t.Fatal("incident notification bypassed an affected service's scope")
		}
		rec, res = ff.request("POST", "/v1/incidents", f.aliceToken, core.NewID(), create, nil)
		expect(t, rec, 200, res)
		if data[status.Incident](t, res).ID != incident.ID {
			t.Fatal("business dedup created another incident")
		}
		create.Title = "different incident"
		rec, res = ff.request("POST", "/v1/incidents", f.aliceToken, core.NewID(), create, nil)
		expect(t, rec, 409, res)
		for _, path := range []string{"/v1/incidents/" + incident.ID, "/v1/incidents/" + incident.ID + "/timeline"} {
			rec, res = ff.request("GET", path, f.bobToken, "", nil, nil)
			expect(t, rec, 404, res)
		}
		rec, res = ff.request("GET", "/v1/incidents?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[status.Incident]](t, res)
		if len(page.Items) != 0 || page.NextCursor != nil {
			t.Fatal("hidden incident leaked before pagination")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE incidents SET service_ids=ARRAY[$2]::uuid[] WHERE id=$1", incident.ID, d.ID); e == nil {
			t.Fatal("mutable incident impact")
		}
		extraIn := in
		extraIn.Name = "Synthetic additional service"
		rec, res = ff.request("POST", "/v1/services", f.aliceToken, core.NewID(), extraIn, nil)
		expect(t, rec, 201, res)
		extra := data[status.Record](t, res)
		if _, e := f.api.DB.Exec(ctx, "INSERT INTO incident_services(org_id,incident_id,service_id,position) VALUES($1,$2,$3,3)", f.org, incident.ID, extra.ID); e == nil {
			t.Fatal("arbitrary impact appended")
		}
	})
	run("incident timeline is versioned and a resolved incident cannot silently reopen", func(t *testing.T) {
		ff := f.with(t)
		patch := status.IncidentPatch{ExpectedVersion: incident.Version, State: "investigating", TimelineNote: "root cause investigation", Reason: "isolated investigation"}
		rec, res := ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 200, res)
		incident = data[status.Incident](t, res)
		rec, res = ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 409, res)
		patch.ExpectedVersion = incident.Version
		patch.State = "resolved"
		rec, res = ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 422, res)
		patch.Resolution = "verified synthetic recovery"
		rec, res = ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 200, res)
		incident = data[status.Incident](t, res)
		if incident.ResolvedAt == nil {
			t.Fatal("resolution missing timestamp")
		}
		drainNotifications(t, f)
		requireBusinessNotification(t, f, f.aliceToken, incident.ID, "incident", false)
		patch.ExpectedVersion = incident.Version
		patch.State = "open"
		rec, res = ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 409, res)
		patch.State = "resolved"
		patch.Postmortem = "isolation prevented cross-project disclosure"
		patch.TimelineNote = "postmortem appended"
		rec, res = ff.request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 200, res)
		incident = data[status.Incident](t, res)
		rec, res = ff.request("GET", "/v1/incidents/"+incident.ID+"/timeline?limit=2", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[status.IncidentEntry]](t, res)
		if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Seq != incident.Version {
			t.Fatal(page)
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE incident_timeline SET note='tampered' WHERE incident_id=$1", incident.ID); e == nil {
			t.Fatal("mutable incident timeline")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE incidents SET version=version+1 WHERE id=$1", incident.ID); e == nil {
			t.Fatal("incident head advanced without an exact timeline entry")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE incidents SET resolved_at=resolved_at+interval '1 second' WHERE id=$1", incident.ID); e == nil {
			t.Fatal("original resolution time was mutable")
		}
	})
	run("source rotation rejects old signed replays and disabled sources remain unknown", func(t *testing.T) {
		ff := f.with(t)
		oldSource := source
		oldBody := payload(23, false, base.Add(23*time.Millisecond))
		oldSig := ed25519.Sign(private, status.SigningMessage(source.ID, oldBody))
		sourceInput.ExpectedVersion = get(t).Version
		sourceInput.ExpectedSourceVersion = source.Version
		sourceInput.BaseRevisionID = &source.RevisionID
		sourceInput.Enabled = false
		rec, res := ff.request("POST", "/v1/services/"+d.ID+"/sources/"+source.ID+"/revisions", f.aliceToken, core.NewID(), sourceInput, nil)
		expect(t, rec, 200, res)
		d = data[status.Record](t, res)
		source = d.Sources[0]
		if source.RevisionID == oldSource.RevisionID || source.Projection != nil || source.StateReason != "SOURCE_DISABLED" || d.State != status.Unknown {
			t.Fatal(source)
		}
		rec, res = rawPush(t, oldBody, oldSig)
		expect(t, rec, 401, res)
		sourceInput.ExpectedVersion = d.Version
		sourceInput.ExpectedSourceVersion = source.Version
		sourceInput.BaseRevisionID = &source.RevisionID
		sourceInput.Enabled = true
		sourceInput.Interval = 15
		sourceInput.StaleAfter = 45
		rec, res = ff.request("POST", "/v1/services/"+d.ID+"/sources/"+source.ID+"/revisions", f.aliceToken, core.NewID(), sourceInput, nil)
		expect(t, rec, 200, res)
		d = data[status.Record](t, res)
		source = d.Sources[0]
		ack := push(t, 1, true, time.Now().Add(-10*time.Minute))
		if ack.Applied || ack.IgnoredReason != "STALE_SAMPLE" || get(t).Sources[0].Projection != nil {
			t.Fatal(ack)
		}
		rec, res = ff.request("GET", "/v1/services/"+d.ID+"/sources/"+source.ID+"/revisions?limit=2", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		history := data[core.Page[status.SourceRevision]](t, res)
		if len(history.Items) != 2 || history.NextCursor == nil || !history.Items[0].IsCurrent || history.Items[1].Enabled || history.Items[0].Seq != 3 {
			t.Fatal(history)
		}
		rec, res = ff.request("GET", "/v1/services/"+d.ID+"/sources/"+source.ID+"/revisions?limit=2&cursor="+*history.NextCursor, f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		older := data[core.Page[status.SourceRevision]](t, res)
		if len(older.Items) != 1 || older.Items[0].Seq != 1 || older.Items[0].BaseRevisionID != nil || older.Items[0].IsCurrent {
			t.Fatal(older)
		}
	})
	run("freshness expires on reads without an incoming sample or process cache", func(t *testing.T) {
		observed := time.Now().UTC().Add(-42 * time.Second)
		push(t, 2, true, observed)
		push(t, 3, true, observed.Add(time.Millisecond))
		initial := get(t)
		if initial.State != status.Operational {
			t.Fatal("fixture never reached operational")
		}
		expires := initial.Sources[0].Projection.ObservedAt.Add(45 * time.Second)
		if !initial.RecheckAt.Equal(expires) || !initial.RecheckAt.After(initial.EvaluatedAt) {
			t.Fatal("client recheck deadline extended the evidence lifetime", initial.RecheckAt, expires)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			view := get(t)
			if view.State == status.Unknown {
				if view.Sources[0].StateReason != "OBSERVATION_STALE" {
					t.Fatal(view)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("read returned stale green state")
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	run("current MFA revocation and concurrent configuration writes remain authoritative", func(t *testing.T) {
		ff := f.with(t)
		d = get(t)
		patch := status.Patch{Content: d.Content, ExpectedVersion: d.Version, Lifecycle: "active", Reason: "isolated concurrent edit"}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL", f.org, f.alice)
		rec, res := ff.request("PATCH", "/v1/services/"+d.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 403, res)
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL", f.org, f.alice)
		replies := make(chan int, 2)
		var workers sync.WaitGroup
		for _, name := range []string{"Concurrent service A", "Concurrent service B"} {
			workers.Add(1)
			go func(name string) {
				defer workers.Done()
				input := patch
				input.Name = name
				raw, _ := json.Marshal(input)
				req := httptest.NewRequest("PATCH", "/v1/services/"+d.ID, bytes.NewReader(raw))
				req.Header.Set("Authorization", "Bearer "+f.aliceToken)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", core.NewID())
				rec := httptest.NewRecorder()
				f.handler.ServeHTTP(rec, req)
				replies <- rec.Code
			}(name)
		}
		workers.Wait()
		close(replies)
		successes, conflicts := 0, 0
		for code := range replies {
			if code == 200 {
				successes++
			}
			if code == 409 {
				conflicts++
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal(successes, conflicts)
		}
		exec(t, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, project.ID, f.bob)
		rec, res = ff.request("GET", "/v1/services/"+d.ID+"/observations", f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = ff.request("GET", "/v1/services?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[status.Record]](t, res).Items) != 0 {
			t.Fatal("removed membership retained service metadata")
		}
	})
}
