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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/statusbridge"
)

func TestHostResources(t *testing.T) {
	f := setup(t)
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	})
	ctx := context.Background()
	for _, x := range []struct{ user, role string }{{f.alice, "status_operator"}, {f.bob, "status_reader"}} {
		if _, e := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','internal' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, x.user, x.role); e != nil {
			t.Fatal(e)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = f.login("bob", "android").AccessToken
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	r, res := f.request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{Content: status.Content{Name: "合成主机资源 / Synthetic host", OwnerID: f.alice, HostLabel: "sample-server", EndpointLabel: "no external target", Description: "Native signed CPU, memory and filesystem monitoring"}, Classification: "internal", Reason: "controlled resource fixture"}, nil)
	expect(t, r, 201, res)
	service := data[status.Record](t, res)
	path := "/v1/services/" + service.ID + "/resource-monitor"
	get := func(t *testing.T) status.ResourceView {
		t.Helper()
		r, res := f.with(t).request("GET", path, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		return data[status.ResourceView](t, res)
	}
	conf := status.ResourceConfigInput{ExpectedVersion: service.Version, ResourceConfig: status.ResourceConfig{Name: "公司主机资源", PublicKey: base64.StdEncoding.EncodeToString(public), Enabled: true, StaleAfter: 90, CPU: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Memory: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Disk: status.ResourceThreshold{Warning: 8000, Critical: 9500}, Mountpoints: []string{"/", "/data"}}, Reason: "明确发布完整监控范围"}
	run := func(name string, fn func(*testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	run("configuration is explicit versioned and MFA authorized", func(t *testing.T) {
		v := get(t)
		if v.State != "unknown" || v.Observation != nil || v.Source != nil {
			t.Fatal("unconfigured host appeared healthy")
		}
		r, res := f.with(t).request("POST", path+"/revisions", f.bobToken, core.NewID(), conf, nil)
		expect(t, r, 404, res)
		r, res = f.with(t).request("POST", path+"/revisions", f.aliceToken, core.NewID(), conf, nil)
		expect(t, r, 200, res)
		r, res = f.with(t).request("POST", path+"/revisions", f.aliceToken, core.NewID(), conf, nil)
		expect(t, r, 409, res)
		v = get(t)
		if v.Source == nil || v.Source.Version != 1 || v.StateReason != "NO_OBSERVATION" {
			t.Fatal("new config inherited health")
		}
	})
	source := get(t).Source
	var mu sync.Mutex
	at := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Millisecond)
	cpu, mem, disk, readonly, up := "35.25", "12582912", "6442450944", "0", "1"
	rewrite := false
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query().Get("query")
		eval, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("time"))
		if err != nil || r.URL.Path != "/api/v1/query" {
			w.WriteHeader(400)
			return
		}
		labels := map[string]string{"instance": "fixture-node:9100", "job": "node"}
		name, value := "", ""
		timestamp := false
		cpuQ := `100*(1-avg by(instance,job)(rate(node_cpu_seconds_total{instance="fixture-node:9100",job="node",mode="idle"}[5m])))`
		cpuAt := `min by(instance,job)(timestamp(node_cpu_seconds_total{instance="fixture-node:9100",job="node",mode="idle"}))`
		if q == cpuQ {
			value = cpu
		} else if q == cpuAt {
			timestamp = true
		} else {
			timestamp = strings.HasPrefix(q, "timestamp(")
			if timestamp {
				q = strings.TrimSuffix(strings.TrimPrefix(q, "timestamp("), ")")
			}
			for _, metric := range []string{"up", "node_memory_MemTotal_bytes", "node_memory_MemAvailable_bytes"} {
				if q == metric+`{instance="fixture-node:9100",job="node"}` {
					name = metric
				}
			}
			for _, mount := range []string{"/", "/data"} {
				for _, metric := range []string{"node_filesystem_size_bytes", "node_filesystem_avail_bytes", "node_filesystem_readonly"} {
					device := "/dev/root"
					if mount == "/data" {
						device = "/dev/data"
					}
					if q == metric+`{device="`+device+`",fstype="ext4",instance="fixture-node:9100",job="node",mountpoint="`+mount+`"}` {
						name = metric
						labels["mountpoint"] = mount
						labels["device"] = device
						labels["fstype"] = "ext4"
					}
				}
			}
			if name == "" {
				w.WriteHeader(400)
				return
			}
			value = map[string]string{"up": up, "node_memory_MemTotal_bytes": "16777216", "node_memory_MemAvailable_bytes": mem, "node_filesystem_size_bytes": "8589934592", "node_filesystem_avail_bytes": disk, "node_filesystem_readonly": readonly}[name]
		}
		if timestamp {
			value = fmt.Sprintf("%d.%03d", at.Unix(), at.UnixMilli()%1000)
		} else if name != "" {
			labels["__name__"] = name
		}
		if rewrite {
			labels["unexpected"] = "unbound"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": labels, "value": []any{float64(eval.UnixMilli()) / 1000, value}}}}})
	}))
	defer prom.Close()
	api := httptest.NewServer(f.handler)
	defer api.Close()
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	seed := filepath.Join(dir, "host.seed")
	if os.WriteFile(seed, []byte(base64.StdEncoding.EncodeToString(key.Seed())), 0600) != nil {
		t.Fatal("private host key")
	}
	cfg := statusbridge.Config{Version: 1, Platform: statusbridge.Endpoint{URL: api.URL, AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}, Prometheus: statusbridge.Endpoint{URL: prom.URL, AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}, StateDirectory: dir, PollSeconds: 15, MaxSampleAge: 45, TimeoutSeconds: 10, Concurrency: 1, Targets: []statusbridge.Target{{Kind: "host", SourceID: source.ID, RevisionID: source.RevisionID, KeyFile: seed, Job: "node", Instance: "fixture-node:9100", Filesystems: []statusbridge.FilesystemTarget{{Mountpoint: "/", Device: "/dev/root", FSType: "ext4"}, {Mountpoint: "/data", Device: "/dev/data", FSType: "ext4"}}}}}
	bridge, e := statusbridge.Open(cfg)
	if e != nil {
		t.Fatal(statusbridge.SafeCode(e))
	}
	defer func() { bridge.Close() }()
	cycle := func(t *testing.T) statusbridge.Result {
		t.Helper()
		out, e := bridge.Cycle(ctx)
		if e != nil || len(out) != 1 {
			t.Fatal("resource bridge", statusbridge.SafeCode(e))
		}
		return out[0]
	}
	advance := func() { at = at.Add(time.Second) }
	run("native node exporter queries sign complete metrics and receipts", func(t *testing.T) {
		if x := cycle(t); x.Code != "ACKNOWLEDGED" || !x.Applied {
			t.Fatal("host sample not applied", x.Code)
		}
		v := get(t)
		if v.State != "operational" || v.Observation == nil || v.Observation.CPUBP != 3525 || len(v.Observation.Filesystems) != 2 || v.Observation.Memory.Available != "12582912" {
			t.Fatal("wrong native metrics", v.State)
		}
		if cycle(t).Code != "NO_NEW_SAMPLE" {
			t.Fatal("old scrape refreshed")
		}
	})
	var original status.ResourceObservation
	run("thresholds and readonly filesystem cause warnings and recover", func(t *testing.T) {
		mu.Lock()
		cpu = "80"
		advance()
		mu.Unlock()
		cycle(t)
		if get(t).State != "degraded" {
			t.Fatal("warning threshold")
		}
		mu.Lock()
		cpu = "35.25"
		readonly = "1"
		advance()
		mu.Unlock()
		cycle(t)
		if get(t).State != "critical" {
			t.Fatal("read-only disk hidden")
		}
		mu.Lock()
		readonly = "0"
		mem = "1"
		advance()
		mu.Unlock()
		cycle(t)
		if get(t).State != "critical" {
			t.Fatal("memory exhaustion hidden")
		}
		mu.Lock()
		mem = "12582912"
		disk = "1"
		advance()
		mu.Unlock()
		cycle(t)
		if get(t).State != "critical" {
			t.Fatal("disk exhaustion hidden")
		}
		mu.Lock()
		disk = "6442450944"
		advance()
		mu.Unlock()
		cycle(t)
		v := get(t)
		if v.State != "operational" {
			t.Fatal("resources did not recover")
		}
		original = *v.Observation
	})
	push := func(t *testing.T, raw []byte, sign []byte) (*httptest.ResponseRecorder, response) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/resource-sources/"+source.ID+"/observations", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CP-Status-Signature", base64.StdEncoding.EncodeToString(sign))
		r := httptest.NewRecorder()
		f.handler.ServeHTTP(r, req)
		var res response
		if json.Unmarshal(r.Body.Bytes(), &res) != nil {
			t.Fatal("invalid response")
		}
		return r, res
	}
	run("loss and unbound labels never fabricate fresh zero metrics", func(t *testing.T) {
		mu.Lock()
		up = "0"
		advance()
		mu.Unlock()
		if cycle(t).Code != "SCRAPE_UNAVAILABLE" {
			t.Fatal("scrape failure hidden")
		}
		mu.Lock()
		up = "1"
		rewrite = true
		mu.Unlock()
		if cycle(t).Code != "SAMPLE_LABELS_INVALID" {
			t.Fatal("wrong labels accepted")
		}
		mu.Lock()
		rewrite = false
		mu.Unlock()
		if get(t).Observation.ID != original.ID {
			t.Fatal("collector loss refreshed observation")
		}
	})
	run("signed history exact replay proof pagination and immutable records", func(t *testing.T) {
		r, res := f.with(t).request("GET", path+"/observations?limit=2", f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		page := data[core.Page[status.ResourceObservation]](t, res)
		if len(page.Items) != 2 || page.NextCursor == nil {
			t.Fatal("history pagination")
		}
		r, res = f.with(t).request("GET", path+"/observations/"+original.ID+"/evidence", f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		proof := data[status.ResourceProof](t, res)
		raw, _ := base64.StdEncoding.DecodeString(proof.SignedPayload)
		sig, _ := base64.StdEncoding.DecodeString(proof.Signature)
		if core.Digest(raw) != proof.PayloadSHA256 || !ed25519.Verify(public, status.ResourceSigningMessage(source.ID, raw), sig) {
			t.Fatal("original evidence mismatch")
		}
		r, res = push(t, raw, sig)
		expect(t, r, 200, res)
		if !data[status.ObservationAck](t, res).Replayed {
			t.Fatal("exact replay changed facts")
		}
		changed := bytes.Replace(raw, []byte(`"cpu_usage_bp":3525`), []byte(`"cpu_usage_bp":3526`), 1)
		r, res = push(t, changed, ed25519.Sign(key, status.ResourceSigningMessage(source.ID, changed)))
		expect(t, r, 409, res)
		if _, e := f.owner.Exec(ctx, `UPDATE service_resource_observations SET sample='{}' WHERE id=$1`, original.ID); e == nil {
			t.Fatal("observation mutable")
		}
		if _, e := f.owner.Exec(ctx, `DELETE FROM service_resource_revisions WHERE id=$1`, source.RevisionID); e == nil {
			t.Fatal("revision mutable")
		}
	})
	run("domain signatures inventory and late observations cannot override", func(t *testing.T) {
		in := original.SignedResourceObservation
		in.Sequence = "100"
		in.ObservedAt = time.Now().UTC().Add(-time.Second)
		raw, _ := json.Marshal(in)
		r, res := push(t, raw, ed25519.Sign(key, status.SigningMessage(source.ID, raw)))
		expect(t, r, 401, res)
		in.Filesystems = in.Filesystems[:1]
		raw, _ = json.Marshal(in)
		r, res = push(t, raw, ed25519.Sign(key, status.ResourceSigningMessage(source.ID, raw)))
		expect(t, r, 422, res)
		in = original.SignedResourceObservation
		in.Sequence = "101"
		in.ObservedAt = time.Now().Add(-10 * time.Minute)
		raw, _ = json.Marshal(in)
		r, res = push(t, raw, ed25519.Sign(key, status.ResourceSigningMessage(source.ID, raw)))
		expect(t, r, 200, res)
		ack := data[status.ObservationAck](t, res)
		if ack.Applied || ack.IgnoredReason != "LATE_OBSERVATION" {
			t.Fatal("late sample applied")
		}
		if get(t).Observation.ID != original.ID {
			t.Fatal("late sample replaced current")
		}
	})
	run("restart validates journal and preserves exact receipt", func(t *testing.T) {
		bridge.Close()
		var e error
		bridge, e = statusbridge.Open(cfg)
		if e != nil {
			t.Fatal(statusbridge.SafeCode(e))
		}
		if cycle(t).Code != "ACKNOWLEDGED" {
			t.Fatal("restart did not deliver next pending sample")
		}
		changed := cfg
		changed.Targets = append([]statusbridge.Target{}, cfg.Targets...)
		changed.Targets[0].Filesystems = append([]statusbridge.FilesystemTarget{}, cfg.Targets[0].Filesystems...)
		changed.Targets[0].Filesystems[0].Device = "/dev/changed"
		bridge.Close()
		if other, e := statusbridge.Open(changed); e == nil {
			other.Close()
			t.Fatal("changed host binding adopted journal")
		}
		bridge, e = statusbridge.Open(cfg)
		if e != nil {
			t.Fatal(statusbridge.SafeCode(e))
		}
	})
	if os.Getenv("CP_BROWSER_HOST_RESOURCES") == "1" && !t.Failed() {
		if _, e := f.owner.Exec(ctx, `WITH r AS (INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES(gen_random_uuid(),$1,'host_browser_mfa','Actionless browser MFA',true) RETURNING id) INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT gen_random_uuid(),$1,$2,id,'org' FROM r`, f.org, f.alice); e != nil {
			t.Fatal(e)
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "human-host-resources", "host_resource_fixture": true, "service_id": service.ID}, func(mux *http.ServeMux, nonce string) {
			mux.HandleFunc("POST /__fixture/resources", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+nonce {
					w.WriteHeader(403)
					return
				}
				var change struct {
					State string `json:"state"`
				}
				if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&change) != nil {
					w.WriteHeader(400)
					return
				}
				mu.Lock()
				at = time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
				cpu, mem, disk, readonly, up = "35.25", "12582912", "6442450944", "0", "1"
				switch change.State {
				case "critical":
					readonly = "1"
				case "degraded":
					cpu = "85"
				case "normal":
				case "stale":
					at = at.Add(-10 * time.Minute)
				default:
					mu.Unlock()
					w.WriteHeader(400)
					return
				}
				mu.Unlock()
				// The fixture intentionally exercises the same bridge and signed receiver.
				out, e := bridge.Cycle(r.Context())
				if e != nil || len(out) != 1 {
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"code": out[0].Code})
			})
		})
		return
	}
	run("actual sample expiry hides previous current metrics", func(t *testing.T) {
		v := get(t)
		conf.ExpectedVersion = v.ServiceVersion
		conf.ExpectedSourceVersion = v.Source.Version
		conf.BaseRevisionID = &v.Source.RevisionID
		conf.StaleAfter = 45
		conf.Reason = "explicit short freshness policy"
		r, res := f.with(t).request("POST", path+"/revisions", f.aliceToken, core.NewID(), conf, nil)
		expect(t, r, 200, res)
		v = get(t)
		in := original.SignedResourceObservation
		in.RevisionID = v.Source.RevisionID
		in.Sequence = "1"
		in.ObservedAt = time.Now().UTC().Add(-38 * time.Second).Truncate(time.Millisecond)
		raw, _ := json.Marshal(in)
		r, res = push(t, raw, ed25519.Sign(key, status.ResourceSigningMessage(source.ID, raw)))
		expect(t, r, 200, res)
		if get(t).State != "operational" {
			t.Fatal("fresh observation rejected")
		}
		if wait := time.Until(in.ObservedAt.Add(45*time.Second + 100*time.Millisecond)); wait > 0 {
			time.Sleep(wait)
		}
		v = get(t)
		if v.State != "unknown" || v.StateReason != "OBSERVATION_STALE" || v.Observation != nil {
			t.Fatal("expired snapshot retained metrics")
		}
	})
	run("new revisions reset metrics revoke old signer and keep history", func(t *testing.T) {
		v := get(t)
		conf.ExpectedVersion = v.ServiceVersion
		conf.ExpectedSourceVersion = v.Source.Version
		conf.BaseRevisionID = &v.Source.RevisionID
		conf.Enabled = false
		conf.Reason = "explicit disable"
		r, res := f.with(t).request("POST", path+"/revisions", f.aliceToken, core.NewID(), conf, nil)
		expect(t, r, 200, res)
		v = get(t)
		if v.State != "unknown" || v.StateReason != "SOURCE_DISABLED" || v.Observation != nil {
			t.Fatal("disabled source retained metrics")
		}
		mu.Lock()
		advance()
		mu.Unlock()
		if _, e := bridge.Cycle(ctx); statusbridge.SafeCode(e) != "INGEST_REJECTED" {
			t.Fatal("old signer still accepted")
		}
		r, res = f.with(t).request("GET", path+"/revisions?limit=20", f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		if len(data[core.Page[status.ResourceRevision]](t, res).Items) != 3 {
			t.Fatal("config history lost")
		}
	})
	run("current role withdrawal denies all resource read paths", func(t *testing.T) {
		if _, e := f.owner.Exec(ctx, `DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2`, f.org, f.bob); e != nil {
			t.Fatal(e)
		}
		for _, suffix := range []string{"", "/revisions", "/observations", "/observations/" + original.ID + "/evidence"} {
			r, _ := f.with(t).request("GET", path+suffix, f.bobToken, "", nil, nil)
			if r.Code == 200 {
				t.Fatal("withdrawn reader got resources")
			}
		}
	})
}
