package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/statusbridge"
)

func TestStatusBridgeNativeFlow(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, x := range []struct{ user, role string }{{f.alice, "status_operator"}, {f.bob, "status_reader"}} {
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','internal' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, x.user, x.role); err != nil {
			t.Fatal(err)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = f.login("bob", "android").AccessToken
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	in := status.Create{Content: status.Content{Name: "Synthetic monitored API", OwnerID: f.alice, HostLabel: "isolated test host", EndpointLabel: "controlled Blackbox sample; no target connection", Description: "Actual native adapter and signed database projection"}, Classification: "internal", Reason: "explicit monitoring integration fixture"}
	rec, res := f.request("POST", "/v1/services", f.aliceToken, core.NewID(), in, nil)
	expect(t, rec, 201, res)
	service := data[status.Record](t, res)
	sourceInput := status.SourceInput{ExpectedVersion: service.Version, SourceContent: status.SourceContent{Name: "Prometheus Blackbox bridge", Provider: "prometheus", ExternalRef: "blackbox-fixture:controlled-only", PublicKey: base64.StdEncoding.EncodeToString(public), Enabled: true, Thresholds: status.Thresholds{FailureCount: 3, RecoveryCount: 2, Interval: 15, StaleAfter: 45, LatencyMS: 1000}}, Reason: "publish exact synthetic monitor binding"}
	rec, res = f.request("POST", "/v1/services/"+service.ID+"/sources", f.aliceToken, core.NewID(), sourceInput, nil)
	expect(t, rec, 200, res)
	service = data[status.Record](t, res)
	source := service.Sources[0]
	var mu sync.Mutex
	at := time.Now().UTC().Add(-35 * time.Second).Truncate(time.Millisecond)
	healthy, up, duration := "1", "1", "0.025"
	dropReceipt := false
	control := identity.Token()
	const job = "blackbox-fixture"
	const instance = "https://controlled.invalid/health"
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/__control" && os.Getenv("CP_BROWSER_STATUS_BRIDGE_FIXTURE") == "1" {
			if r.Method != "POST" || r.Header.Get("X-CP-Fixture-Control") != control {
				w.WriteHeader(403)
				return
			}
			var change struct{ Healthy, Up, Duration string }
			d := json.NewDecoder(io.LimitReader(r.Body, 1024))
			d.DisallowUnknownFields()
			if d.Decode(&change) != nil || change.Healthy != "0" && change.Healthy != "1" || change.Up != "0" && change.Up != "1" {
				w.WriteHeader(400)
				return
			}
			healthy, up, duration = change.Healthy, change.Up, change.Duration
			now := time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
			if now.After(at) {
				at = now
			} else {
				at = at.Add(time.Millisecond)
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.Path != "/api/v1/query" {
			w.WriteHeader(404)
			return
		}
		q := r.URL.Query().Get("query")
		stamp := strings.HasPrefix(q, "timestamp(")
		if stamp {
			q = strings.TrimSuffix(strings.TrimPrefix(q, "timestamp("), ")")
		}
		name := ""
		for _, metric := range []string{"up", "probe_success", "probe_duration_seconds"} {
			if q == metric+`{instance="`+instance+`",job="`+job+`"}` {
				name = metric
			}
		}
		if name == "" {
			w.WriteHeader(400)
			return
		}
		eval, e := time.Parse(time.RFC3339Nano, r.URL.Query().Get("time"))
		if e != nil {
			w.WriteHeader(400)
			return
		}
		value := map[string]string{"up": up, "probe_success": healthy, "probe_duration_seconds": duration}[name]
		if stamp {
			value = fmt.Sprintf("%d.%03d", at.Unix(), at.UnixMilli()%1000)
		}
		labels := map[string]string{"job": job, "instance": instance}
		if !stamp {
			labels["__name__"] = name
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": labels, "value": []any{float64(eval.UnixMilli()) / 1000, value}}}}})
	}))
	defer prom.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		drop := dropReceipt
		dropReceipt = false
		mu.Unlock()
		if !drop {
			f.handler.ServeHTTP(w, r)
			return
		}
		buffer := httptest.NewRecorder()
		f.handler.ServeHTTP(buffer, r)
		var value map[string]any
		_ = json.Unmarshal(buffer.Body.Bytes(), &value)
		if object, ok := value["data"].(map[string]any); ok {
			delete(object, "payload_sha256")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(buffer.Code)
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer api.Close()
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	seed := filepath.Join(dir, "source.seed")
	if os.WriteFile(seed, []byte(base64.StdEncoding.EncodeToString(key.Seed())), 0600) != nil {
		t.Fatal("private fixture file")
	}
	endpoint := func(u string) statusbridge.Endpoint {
		return statusbridge.Endpoint{URL: u, AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}
	}
	c := statusbridge.Config{Version: 1, Platform: endpoint(api.URL), Prometheus: endpoint(prom.URL), StateDirectory: dir, PollSeconds: 15, MaxSampleAge: 45, TimeoutSeconds: 3, Concurrency: 2, Targets: []statusbridge.Target{{SourceID: source.ID, RevisionID: source.RevisionID, KeyFile: seed, Job: job, Instance: instance}}}
	b, err := statusbridge.Open(c)
	if err != nil {
		t.Fatal(statusbridge.SafeCode(err))
	}
	defer func() { b.Close() }()
	get := func(tt *testing.T) status.Record {
		tt.Helper()
		rec, res := f.with(tt).request("GET", "/v1/services/"+service.ID, f.aliceToken, "", nil, nil)
		expect(tt, rec, 200, res)
		return data[status.Record](tt, res)
	}
	cycle := func(tt *testing.T) statusbridge.Result {
		tt.Helper()
		out, e := b.Cycle(ctx)
		if e != nil || len(out) != 1 {
			tt.Fatal("native bridge cycle failed", statusbridge.SafeCode(e))
		}
		return out[0]
	}
	advance := func(success string) { mu.Lock(); at = at.Add(time.Second); healthy = success; mu.Unlock() }
	count := func() int {
		var n int
		if f.owner.QueryRow(ctx, "SELECT count(*) FROM service_observations WHERE source_id=$1", source.ID).Scan(&n) != nil {
			t.Fatal("fact count")
		}
		return n
	}
	run := func(name string, fn func(*testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	run("real signed samples warm up and recover; duplicates do not advance streak", func(t *testing.T) {
		if r := cycle(t); r.Code != "ACKNOWLEDGED" || !r.Applied || get(t).State != status.Unknown {
			t.Fatal("first success skipped warmup")
		}
		advance("1")
		cycle(t)
		if get(t).State != status.Operational {
			t.Fatal("second success did not recover")
		}
		n := count()
		if cycle(t).Code != "NO_NEW_SAMPLE" || count() != n {
			t.Fatal("lookback sample counted again")
		}
	})
	run("three real failed probes cause down; a failed scrape makes no false sample", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			advance("0")
			cycle(t)
			want := status.Degraded
			if i == 2 {
				want = status.Down
			}
			if get(t).State != want {
				t.Fatal("failure threshold changed")
			}
		}
		mu.Lock()
		up = "0"
		mu.Unlock()
		n := count()
		if cycle(t).Code != "SCRAPE_UNAVAILABLE" || count() != n {
			t.Fatal("collector loss fabricated down observation")
		}
		mu.Lock()
		up = "1"
		mu.Unlock()
	})
	run("two actual successes recover; measured high latency degrades", func(t *testing.T) {
		advance("1")
		cycle(t)
		if get(t).State != status.Down {
			t.Fatal("one success recovered too early")
		}
		advance("1")
		cycle(t)
		if get(t).State != status.Operational {
			t.Fatal("two successes not applied")
		}
		advance("1")
		mu.Lock()
		duration = "1.001"
		mu.Unlock()
		cycle(t)
		if get(t).State != status.Degraded {
			t.Fatal("high latency not preserved")
		}
	})
	run("actual commit with incomplete receipt survives process reopen as exact replay", func(t *testing.T) {
		advance("1")
		mu.Lock()
		duration = "0.025"
		dropReceipt = true
		mu.Unlock()
		if cycle(t).Code != "RECEIPT_UNCONFIRMED" {
			t.Fatal("missing proof accepted")
		}
		n := count()
		b.Close()
		b, err = statusbridge.Open(c)
		if err != nil {
			t.Fatal(statusbridge.SafeCode(err))
		}
		r := cycle(t)
		if r.Code != "ACKNOWLEDGED" || !r.Replayed || count() != n {
			t.Fatal("restart duplicated committed fact")
		}
		var seq, digest string
		if f.owner.QueryRow(ctx, "SELECT sequence::text,payload_sha256 FROM service_observations WHERE id=$1", r.ObservationID).Scan(&seq, &digest) != nil || seq != r.Sequence || len(digest) != 64 {
			t.Fatal("receipt not bound to actual database fact")
		}
	})
	run("actual expiry becomes unknown without synthetic failure", func(t *testing.T) {
		mu.Lock()
		until := at.Add(45*time.Second + 100*time.Millisecond)
		mu.Unlock()
		if wait := time.Until(until); wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				t.Fatal("expiry interrupted")
			}
		}
		n := count()
		if get(t).State != status.Unknown || cycle(t).Code == "ACKNOWLEDGED" || count() != n {
			t.Fatal("expired probe retained green or manufactured failure")
		}
	})
	run("published source rotation rejects old revision and new revision starts unknown", func(t *testing.T) {
		current := get(t)
		sourceInput.ExpectedVersion = current.Version
		sourceInput.ExpectedSourceVersion = current.Sources[0].Version
		sourceInput.BaseRevisionID = &source.RevisionID
		sourceInput.Reason = "explicit native source rotation"
		rec, res := f.with(t).request("POST", "/v1/services/"+service.ID+"/sources/"+source.ID+"/revisions", f.aliceToken, core.NewID(), sourceInput, nil)
		expect(t, rec, 200, res)
		service = data[status.Record](t, res)
		if service.State != status.Unknown {
			t.Fatal("new revision inherited old health")
		}
		mu.Lock()
		at = time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
		mu.Unlock()
		if _, e := b.Cycle(ctx); statusbridge.SafeCode(e) != "INGEST_REJECTED" {
			t.Fatal("rotated revision accepted old signer")
		}
		b.Close()
		c.Targets[0].RevisionID = service.Sources[0].RevisionID
	})
	if os.Getenv("CP_BROWSER_STATUS_BRIDGE_FIXTURE") == "1" {
		name := filepath.Join(dir, "bridge.json")
		body, _ := json.Marshal(c)
		if os.WriteFile(name, body, 0600) != nil {
			t.Fatal("private bridge fixture configuration")
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "native-status-bridge", "service_id": service.ID, "source_id": source.ID, "source_revision_id": c.Targets[0].RevisionID, "bridge_config": name, "prometheus_control_url": prom.URL + "/__control", "prometheus_control_token": control})
	}
}
