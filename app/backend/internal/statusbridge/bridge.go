package statusbridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

type Result struct {
	SourceID      string `json:"source_id"`
	Code          string `json:"code"`
	Sequence      string `json:"sequence,omitempty"`
	ObservationID string `json:"observation_id,omitempty"`
	Applied       bool   `json:"applied"`
	Replayed      bool   `json:"replayed"`
}

type Bridge struct {
	mu                   sync.Mutex
	config               Config
	prom, platform       *http.Client
	promURL, platformURL *url.URL
	bearer               string
	stores               []*journal
	next                 int
	closed               bool
	halted               error
}

func Open(c Config) (*Bridge, error) {
	// The caller cannot mutate live target labels/endpoints after validation.
	b, _ := json.Marshal(c)
	var frozen Config
	if json.Unmarshal(b, &frozen) != nil || frozen.Validate() != nil {
		return nil, fault("CONFIG_INVALID")
	}
	c = frozen
	x := &Bridge{config: c}
	ok := false
	defer func() {
		if !ok {
			x.Close()
		}
	}()
	var err error
	x.prom, x.promURL, err = endpointClient(c.Prometheus, timeout(c), nil)
	if err != nil {
		return nil, err
	}
	x.platform, x.platformURL, err = endpointClient(c.Platform, timeout(c), nil)
	if err != nil {
		return nil, err
	}
	if c.BearerFile != "" {
		b, err := boundedFile(c.BearerFile, 4096, true)
		if err != nil {
			return nil, err
		}
		x.bearer = strings.TrimSpace(string(b))
		if x.bearer == "" || strings.ContainsAny(x.bearer, "\r\n\x00\t ") {
			return nil, fault("BEARER_FILE_INVALID")
		}
	}
	for _, t := range c.Targets {
		key, err := loadKey(t.KeyFile)
		if err != nil {
			return nil, err
		}
		store, err := openJournal(c, t, key)
		if err != nil {
			return nil, err
		}
		x.stores = append(x.stores, store)
	}
	ok = true
	return x, nil
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.stores {
		s.Close()
	}
	if b.prom != nil {
		b.prom.CloseIdleConnections()
	}
	if b.platform != nil {
		b.platform.CloseIdleConnections()
	}
	b.closed = true
	return nil
}

func (b *Bridge) Cycle(ctx context.Context) ([]Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, fault("BRIDGE_CLOSED")
	}
	if b.halted != nil {
		return nil, b.halted
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(b.config.PollSeconds)*time.Second)
	defer cancel()
	n := len(b.stores)
	results := make([]Result, n)
	errs := make([]error, n)
	jobs := make(chan int, n)
	for i := range n {
		jobs <- (b.next + i) % n
	}
	close(jobs)
	b.next = (b.next + b.config.Concurrency) % n
	var wg sync.WaitGroup
	for range min(n, b.config.Concurrency) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					results[i] = Result{SourceID: b.stores[i].target.SourceID, Code: "CYCLE_CANCELLED"}
					continue
				}
				work, stop := context.WithTimeout(ctx, timeout(b.config))
				results[i], errs[i] = b.cycleTarget(work, b.stores[i])
				stop()
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			b.halted = err
			return results, err
		}
	}
	return results, nil
}

func (b *Bridge) cycleTarget(ctx context.Context, j *journal) (Result, error) {
	r := Result{SourceID: j.target.SourceID}
	if j.current.Generation == 0 || j.current.Ack != nil {
		if j.current.Sequence == 9007199254740991 {
			return r, fault("SEQUENCE_EXHAUSTED")
		}
		seq := j.current.Sequence + 1
		observed, digest, body, signing, err := b.targetSample(ctx, j, seq)
		if err != nil {
			r.Code = SafeCode(err)
			return r, nil
		}
		if j.current.Generation != 0 && !observed.After(j.current.ObservedAt) {
			r.Code = "NO_NEW_SAMPLE"
			if observed.Equal(j.current.ObservedAt) && digest != j.current.SampleSHA {
				r.Code = "SAMPLE_REWRITTEN"
			}
			return r, nil
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(j.key, signing))
		next := checkpoint{Binding: j.binding, Sequence: seq, ObservedAt: observed, SampleSHA: digest, Body: body, Signature: sig}
		if err = j.save(next); err != nil {
			return r, err
		}
	}
	r.Sequence = strconv.FormatInt(j.current.Sequence, 10)
	ack, terminal, err := b.deliver(ctx, j)
	if err != nil {
		r.Code = SafeCode(err)
		if terminal {
			return r, err
		}
		return r, nil
	}
	next := j.current
	next.Ack = &ack
	if err = j.save(next); err != nil {
		return r, err
	}
	r.Code = "ACKNOWLEDGED"
	r.ObservationID = ack.ID
	r.Applied = ack.Applied
	r.Replayed = ack.Replayed
	return r, nil
}

func (b *Bridge) deliver(ctx context.Context, j *journal) (servicestatus.ObservationAck, bool, error) {
	var empty servicestatus.ObservationAck
	u := *b.platformURL
	u.Path += "/v1/status-sources/" + j.target.SourceID + "/observations"
	if j.target.Kind == "host" {
		u.Path = b.platformURL.Path + "/v1/resource-sources/" + j.target.SourceID + "/observations"
	}
	r, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(j.current.Body))
	if err != nil {
		return empty, true, fault("INGEST_REQUEST_INVALID")
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	r.Header.Set("X-CP-Status-Signature", j.current.Signature)
	res, err := b.platform.Do(r)
	if err != nil {
		return empty, false, fault("DELIVERY_PENDING")
	}
	body, err := responseBody(res, 16<<10)
	if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != 408 && res.StatusCode != 429 {
		return empty, true, fault("INGEST_REJECTED")
	}
	if err != nil || res.StatusCode != 200 || uniqueJSON(body) != nil {
		return empty, false, fault("DELIVERY_PENDING")
	}
	// Pointers distinguish explicit false from missing/null receipt fields.
	var wire struct {
		Data *struct {
			ID       string  `json:"id"`
			Source   string  `json:"source_id"`
			Revision string  `json:"source_revision_id"`
			Sequence string  `json:"sequence"`
			SHA      string  `json:"payload_sha256"`
			Applied  *bool   `json:"applied"`
			Replayed *bool   `json:"replayed"`
			Reason   *string `json:"ignored_reason"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wire) != nil || wire.Data == nil || wire.Data.Applied == nil || wire.Data.Replayed == nil || wire.Data.Reason == nil {
		return empty, false, fault("RECEIPT_UNCONFIRMED")
	}
	w := wire.Data
	ack := servicestatus.ObservationAck{ID: w.ID, SourceID: w.Source, RevisionID: w.Revision, Sequence: w.Sequence, PayloadSHA256: w.SHA, Applied: *w.Applied, Replayed: *w.Replayed, IgnoredReason: *w.Reason}
	if !validAck(ack, j.target, strconv.FormatInt(j.current.Sequence, 10), core.Digest(j.current.Body)) {
		return empty, false, fault("RECEIPT_UNCONFIRMED")
	}
	return ack, false, nil
}

func validAck(a servicestatus.ObservationAck, t Target, seq, sha string) bool {
	return core.ValidID(a.ID) && a.SourceID == t.SourceID && a.RevisionID == t.RevisionID && a.Sequence == seq && a.PayloadSHA256 == sha &&
		(a.Applied && a.IgnoredReason == "" || !a.Applied && (a.IgnoredReason == "LATE_SEQUENCE" || a.IgnoredReason == "LATE_OBSERVATION" || a.IgnoredReason == "STALE_SAMPLE"))
}

func SafeCode(err error) string {
	var e *Fault
	if errors.As(err, &e) {
		return e.Code
	}
	return "BRIDGE_FAILED"
}
