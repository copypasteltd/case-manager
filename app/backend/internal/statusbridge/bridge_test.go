package statusbridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

type bridgeFixture struct {
	mu                    sync.Mutex
	c                     Config
	pub                   ed25519.PublicKey
	key                   ed25519.PrivateKey
	at                    time.Time
	healthy, duration, up string
	mode                  string
	queries               int
	writes                [][]byte
	sigs                  []string
	response              string
	bearer                string
	status                int
}

func newBridgeFixture(t *testing.T) *bridgeFixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	k := filepath.Join(dir, "private.seed")
	if os.WriteFile(k, []byte(base64.StdEncoding.EncodeToString(key.Seed())), 0600) != nil {
		t.Fatal("key fixture")
	}
	f := &bridgeFixture{pub: pub, key: key, at: time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond), healthy: "1", duration: "0.050", up: "1", status: 200}
	base := func(url string) Endpoint {
		return Endpoint{URL: url, AllowedIPs: []string{"127.0.0.1"}, AllowPrivate: true, AllowHTTP: true}
	}
	prom := httptest.NewServer(http.HandlerFunc(f.prometheus))
	t.Cleanup(prom.Close)
	ingest := httptest.NewServer(http.HandlerFunc(f.ingest))
	t.Cleanup(ingest.Close)
	f.c = Config{Version: 1, Platform: base(ingest.URL), Prometheus: base(prom.URL), StateDirectory: dir, PollSeconds: 15, MaxSampleAge: 45, TimeoutSeconds: 2, Concurrency: 2,
		Targets: []Target{{SourceID: core.NewID(), RevisionID: core.NewID(), KeyFile: k, Job: "blackbox-public", Instance: "https://example.invalid/health", Labels: map[string]string{"region": "test"}}}}
	return f
}

func (f *bridgeFixture) prometheus(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries++
	if f.bearer != "" && r.Header.Get("Authorization") != "Bearer "+f.bearer {
		w.WriteHeader(401)
		return
	}
	if r.URL.Path != "/api/v1/query" {
		http.Error(w, "unexpected path", 400)
		return
	}
	q := r.URL.Query().Get("query")
	stamp := strings.HasPrefix(q, "timestamp(")
	if stamp {
		q = strings.TrimSuffix(strings.TrimPrefix(q, "timestamp("), ")")
	}
	metric := ""
	for _, name := range []string{"up", "probe_success", "probe_duration_seconds"} {
		if q == selector(name, f.c.Targets[0]) {
			metric = name
		}
	}
	if metric == "" {
		http.Error(w, "unexpected selector", 400)
		return
	}
	value := map[string]string{"up": f.up, "probe_success": f.healthy, "probe_duration_seconds": f.duration}[metric]
	at := f.at
	if f.mode == "stale" {
		at = at.Add(-time.Hour)
	}
	if f.mode == "future" {
		at = at.Add(time.Hour)
	}
	if f.mode == "mismatch" && metric == "probe_duration_seconds" {
		at = at.Add(-time.Millisecond)
	}
	if stamp {
		value = strconv.FormatInt(at.UnixMilli()/1000, 10) + "." + strconv.FormatInt(1000+at.UnixMilli()%1000, 10)[1:]
	}
	eval, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("time"))
	if err != nil {
		http.Error(w, "time missing", 400)
		return
	}
	m := labels(f.c.Targets[0])
	if !stamp {
		m["__name__"] = metric
	}
	if f.mode == "wrong_label" {
		m["region"] = "other"
	}
	row := map[string]any{"metric": m, "value": []any{float64(eval.UnixMilli()) / 1000, value}}
	if f.mode == "histogram" {
		row["histogram"] = []any{1, map[string]any{}}
	}
	rows := []any{row}
	if f.mode == "multiple" {
		rows = append(rows, row)
	}
	if f.mode == "missing" {
		rows = []any{}
	}
	w.Header().Set("Content-Type", "application/json")
	if f.mode == "unavailable" {
		w.WriteHeader(503)
		return
	}
	if f.mode == "oversize" {
		_, _ = w.Write([]byte(strings.Repeat("x", (64<<10)+1)))
		return
	}
	if f.mode == "duplicate_json" {
		_, _ = w.Write([]byte(`{"status":"success","status":"error"}`))
		return
	}
	result := map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": rows}}
	if f.mode == "warnings" {
		result["warnings"] = []string{"partial response"}
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (f *bridgeFixture) ingest(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := io.ReadAll(r.Body)
	f.writes = append(f.writes, append([]byte(nil), b...))
	f.sigs = append(f.sigs, r.Header.Get("X-CP-Status-Signature"))
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("Content-Type") != "application/json" || r.URL.Path != "/v1/status-sources/"+f.c.Targets[0].SourceID+"/observations" {
		http.Error(w, "unexpected authorization", 400)
		return
	}
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get("X-CP-Status-Signature"))
	if err != nil || !ed25519.Verify(f.pub, servicestatus.SigningMessage(f.c.Targets[0].SourceID, b), sig) {
		http.Error(w, "bad signature", 401)
		return
	}
	if f.status != 200 {
		w.WriteHeader(f.status)
		return
	}
	var in servicestatus.SignedObservation
	if json.Unmarshal(b, &in) != nil {
		http.Error(w, "wire", 400)
		return
	}
	ack := map[string]any{"id": core.NewID(), "source_id": f.c.Targets[0].SourceID, "source_revision_id": in.RevisionID, "sequence": in.Sequence, "payload_sha256": core.Digest(b), "applied": true, "replayed": len(f.writes) > 1, "ignored_reason": ""}
	switch f.response {
	case "incomplete":
		delete(ack, "payload_sha256")
	case "wrong_source":
		ack["source_id"] = core.NewID()
	case "wrong_revision":
		ack["source_revision_id"] = core.NewID()
	case "wrong_sequence":
		ack["sequence"] = "900"
	case "wrong_hash":
		ack["payload_sha256"] = strings.Repeat("0", 64)
	case "missing_false":
		delete(ack, "replayed")
	case "null_false":
		ack["replayed"] = nil
	case "contradiction":
		ack["ignored_reason"] = "STALE_SAMPLE"
	case "stale":
		ack["applied"] = false
		ack["ignored_reason"] = "STALE_SAMPLE"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": ack})
}

func openFixture(t *testing.T, f *bridgeFixture) *Bridge {
	t.Helper()
	b, err := Open(f.c)
	if err != nil {
		t.Fatal(SafeCode(err))
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func cycleFixture(t *testing.T, b *Bridge) Result {
	t.Helper()
	out, err := b.Cycle(context.Background())
	if err != nil {
		t.Fatal(SafeCode(err))
	}
	if len(out) != 1 {
		t.Fatal("missing result")
	}
	return out[0]
}

func TestStatusBridgeDeliveryRecovery(t *testing.T) {
	f := newBridgeFixture(t)
	b := openFixture(t, f)
	f.response = "incomplete"
	first := cycleFixture(t, b)
	if first.Code != "RECEIPT_UNCONFIRMED" || first.Sequence != "1" || b.stores[0].current.Ack != nil {
		t.Fatal("incomplete receipt accepted")
	}
	queries := f.queries
	original := string(f.writes[0])
	signature := f.sigs[0]
	b.Close()
	f.at = f.at.Add(time.Millisecond)
	f.healthy = "0"
	f.response = ""
	b = openFixture(t, f)
	recovered := cycleFixture(t, b)
	if recovered.Code != "ACKNOWLEDGED" || !recovered.Replayed || f.queries != queries || len(f.writes) != 2 || string(f.writes[1]) != original || f.sigs[1] != signature {
		t.Fatal("restart did not replay exact original bytes before new sampling")
	}
	second := cycleFixture(t, b)
	if second.Sequence != "2" || second.Code != "ACKNOWLEDGED" {
		t.Fatal("next sequence missing")
	}
	var raw servicestatus.SignedObservation
	_ = json.Unmarshal(f.writes[2], &raw)
	if raw.Healthy || raw.Sequence != "2" || raw.LatencyMS == nil || *raw.LatencyMS != 50 {
		t.Fatal("new sample not preserved")
	}
	count := len(f.writes)
	for range 3 {
		if cycleFixture(t, b).Code != "NO_NEW_SAMPLE" {
			t.Fatal("same sample counted twice")
		}
	}
	if len(f.writes) != count {
		t.Fatal("duplicate sample delivered")
	}
	f.duration = "0.1"
	if cycleFixture(t, b).Code != "SAMPLE_REWRITTEN" || len(f.writes) != count {
		t.Fatal("rewritten timestamp accepted")
	}
}

func TestStatusBridgeSampleBoundaries(t *testing.T) {
	for _, mode := range []string{"stale", "future", "mismatch", "wrong_label", "histogram", "multiple", "missing", "unavailable", "oversize", "duplicate_json", "warnings"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeFixture(t)
			f.mode = mode
			b := openFixture(t, f)
			r := cycleFixture(t, b)
			if r.Code == "ACKNOWLEDGED" || len(f.writes) != 0 || b.stores[0].current.Generation != 0 {
				t.Fatal("unknown collection became a health observation")
			}
		})
	}
	for _, value := range []string{"NaN", "+Inf", "-Inf", "-1", "3600.01"} {
		t.Run("duration_"+value, func(t *testing.T) {
			f := newBridgeFixture(t)
			f.duration = value
			b := openFixture(t, f)
			cycleFixture(t, b)
			if len(f.writes) != 0 {
				t.Fatal("invalid duration accepted")
			}
		})
	}
	t.Run("scrape_failure_is_unknown", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.up = "0"
		b := openFixture(t, f)
		if cycleFixture(t, b).Code != "SCRAPE_UNAVAILABLE" || len(f.writes) != 0 {
			t.Fatal("failed scrape treated as target down")
		}
	})
	t.Run("failed_probe_is_real_sample", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.healthy = "0"
		f.duration = "0.0001"
		b := openFixture(t, f)
		if cycleFixture(t, b).Code != "ACKNOWLEDGED" {
			t.Fatal("failed probe dropped")
		}
		var in servicestatus.SignedObservation
		_ = json.Unmarshal(f.writes[0], &in)
		if in.Healthy || *in.LatencyMS != 1 {
			t.Fatal("duration rounding or probe fact changed")
		}
	})
}

func TestStatusBridgeReceiptBoundaries(t *testing.T) {
	t.Run("prometheus_read_token_never_reaches_ingest", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.bearer = "synthetic-read-only-token"
		f.c.BearerFile = filepath.Join(f.c.StateDirectory, "read.token")
		if os.WriteFile(f.c.BearerFile, []byte(f.bearer), 0600) != nil {
			t.Fatal("private read credential fixture")
		}
		b := openFixture(t, f)
		if cycleFixture(t, b).Code != "ACKNOWLEDGED" || f.queries != 6 {
			t.Fatal("query/ingest credentials not isolated")
		}
	})
	for _, response := range []string{"incomplete", "wrong_source", "wrong_revision", "wrong_sequence", "wrong_hash", "missing_false", "null_false", "contradiction"} {
		t.Run(response, func(t *testing.T) {
			f := newBridgeFixture(t)
			f.response = response
			b := openFixture(t, f)
			r := cycleFixture(t, b)
			if r.Code != "RECEIPT_UNCONFIRMED" || b.stores[0].current.Ack != nil {
				t.Fatal("unbound receipt cleared pending")
			}
			f.response = ""
			if cycleFixture(t, b).Sequence != "1" || string(f.writes[0]) != string(f.writes[1]) {
				t.Fatal("uncertain receipt advanced sequence")
			}
		})
	}
	t.Run("stale_ack_is_retained_fact_not_current_health", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.response = "stale"
		b := openFixture(t, f)
		r := cycleFixture(t, b)
		if r.Code != "ACKNOWLEDGED" || r.Applied || b.stores[0].current.Ack == nil {
			t.Fatal("valid ignored fact not acknowledged")
		}
	})
	t.Run("temporary_failure_replays", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.status = 503
		b := openFixture(t, f)
		if cycleFixture(t, b).Code != "DELIVERY_PENDING" {
			t.Fatal("temporary error accepted")
		}
		f.status = 200
		if cycleFixture(t, b).Sequence != "1" || string(f.writes[0]) != string(f.writes[1]) {
			t.Fatal("retry replaced bytes")
		}
	})
	t.Run("revocation_halts_without_replay", func(t *testing.T) {
		f := newBridgeFixture(t)
		f.status = 401
		b := openFixture(t, f)
		if _, err := b.Cycle(context.Background()); SafeCode(err) != "INGEST_REJECTED" {
			t.Fatal("rejection not terminal")
		}
		_, _ = b.Cycle(context.Background())
		if len(f.writes) != 1 || b.stores[0].current.Ack != nil {
			t.Fatal("rejected observation automatically replayed")
		}
	})
}
