package servicestatus

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"strconv"
	"strings"
	"time"
)

// SigningMessage binds exact wire bytes to a single source and protocol.
// The sender retains its private key; no employee session is impersonated.
func SigningMessage(source string, body []byte) []byte {
	return append([]byte("CP-STATUS-V1\n"+strings.ToLower(source)+"\n"), body...)
}

type SignedObservation struct {
	RevisionID string    `json:"source_revision_id"`
	Sequence   string    `json:"sequence"`
	ObservedAt time.Time `json:"observed_at"`
	Healthy    bool      `json:"healthy"`
	LatencyMS  *int64    `json:"latency_ms"`
}

func parseObservation(body []byte) (SignedObservation, int64, error) {
	var out SignedObservation
	invalid := func() (SignedObservation, int64, error) {
		return out, 0, core.Invalid("observation", "观测须包含准确修订、序号、观测时间、健康与延迟字段；字段不能重复")
	}
	if len(body) == 0 || len(body) > 4096 {
		return invalid()
	}
	// Reject duplicates, null required values, unknown keys and trailing objects
	// before interpreting a signed payload. All five fields are explicit.
	dec := json.NewDecoder(bytes.NewReader(body))
	token, e := dec.Token()
	if e != nil || token != json.Delim('{') {
		return invalid()
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		t, e := dec.Token()
		if e != nil {
			return invalid()
		}
		name, ok := t.(string)
		if !ok {
			return invalid()
		}
		if _, ok = fields[name]; ok {
			return invalid()
		}
		var raw json.RawMessage
		if e = dec.Decode(&raw); e != nil {
			return invalid()
		}
		fields[name] = raw
	}
	if _, e = dec.Token(); e != nil {
		return invalid()
	}
	if dec.Decode(new(any)) != io.EOF {
		return invalid()
	}
	required := []string{"source_revision_id", "sequence", "observed_at", "healthy", "latency_ms"}
	if len(fields) != len(required) {
		return invalid()
	}
	for _, k := range required {
		v, ok := fields[k]
		if !ok || (k != "latency_ms" && bytes.Equal(bytes.TrimSpace(v), []byte("null"))) {
			return invalid()
		}
	}
	strict := json.NewDecoder(bytes.NewReader(body))
	strict.DisallowUnknownFields()
	if strict.Decode(&out) != nil {
		return invalid()
	}
	seq, e := strconv.ParseInt(out.Sequence, 10, 64)
	if e != nil || seq < 1 || seq > 9007199254740991 || strconv.FormatInt(seq, 10) != out.Sequence || !core.ValidID(out.RevisionID) || out.ObservedAt.IsZero() ||
		(out.Healthy && out.LatencyMS == nil) || (out.LatencyMS != nil && (*out.LatencyMS < 0 || *out.LatencyMS > 3600000)) {
		return invalid()
	}
	return out, seq, nil
}
func (s *Service) Ingest(ctx context.Context, source, signature, request string, body []byte) (ObservationAck, error) {
	out := ObservationAck{}
	denied := func() error { return core.Fail(401, "PROBE_AUTHENTICATION_FAILED", "观测来源凭证无效") }
	if !core.ValidID(source) {
		return out, denied()
	}
	source = strings.ToLower(source)
	sig, e := base64.StdEncoding.Strict().DecodeString(signature)
	if e != nil || len(sig) != ed25519.SignatureSize {
		return out, denied()
	}
	in, sequence, e := parseObservation(body)
	if e != nil {
		return out, e
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var org, service string
	if e = tx.QueryRow(ctx, "SELECT org_id,service_id FROM service_sources WHERE id=$1", source).Scan(&org, &service); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return out, denied()
		}
		return out, e
	}
	// Human configuration locks organization -> project -> service -> source;
	// ingestion takes no project lock and never changes business configuration.
	if _, e = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", org); e != nil {
		return out, e
	}
	var lifecycle string
	if e = tx.QueryRow(ctx, "SELECT lifecycle FROM services WHERE org_id=$1 AND id=$2 FOR UPDATE", org, service).Scan(&lifecycle); e != nil {
		return out, e
	}
	var revision string
	var key []byte
	var enabled bool
	var t Thresholds
	e = tx.QueryRow(ctx, `SELECT h.current_revision_id,r.public_key,r.enabled,r.failure_count,r.recovery_count,r.interval_seconds,r.stale_after_seconds,r.latency_limit_ms
 FROM service_sources h JOIN service_source_revisions r ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id WHERE h.org_id=$1 AND h.id=$2 FOR UPDATE OF h`, org, source).Scan(&revision, &key, &enabled, &t.FailureCount, &t.RecoveryCount, &t.Interval, &t.StaleAfter, &t.LatencyMS)
	if e != nil {
		return out, e
	}
	if lifecycle != "active" || !enabled || !strings.EqualFold(in.RevisionID, revision) || !ed25519.Verify(ed25519.PublicKey(key), SigningMessage(source, body), sig) {
		return out, denied()
	}
	digest := core.Digest(body)
	out.SourceID, out.RevisionID, out.Sequence, out.PayloadSHA256 = source, revision, in.Sequence, digest
	var previousHash string
	e = tx.QueryRow(ctx, "SELECT id,applied,ignored_reason,payload_sha256 FROM service_observations WHERE org_id=$1 AND revision_id=$2 AND sequence=$3", org, revision, sequence).Scan(&out.ID, &out.Applied, &out.IgnoredReason, &previousHash)
	if e == nil {
		if previousHash != digest {
			return ObservationAck{}, core.Fail(409, "OBSERVATION_SEQUENCE_CONFLICT", "该来源修订的序号已有不同观测")
		}
		out.Replayed = true
		return out, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var before Projection
	var observed, received *time.Time
	e = tx.QueryRow(ctx, "SELECT last_sequence,observed_at,received_at,state,failure_streak,success_streak FROM service_source_states WHERE org_id=$1 AND revision_id=$2 FOR UPDATE", org, revision).Scan(&before.Sequence, &observed, &received, &before.State, &before.Failures, &before.Successes)
	if e != nil {
		return out, e
	}
	if observed != nil {
		before.ObservedAt = *observed
	}
	if received != nil {
		before.ReceivedAt = *received
	}
	var now time.Time
	if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return out, e
	}
	after, ignored, e := Advance(t, before, Sample{Sequence: sequence, ObservedAt: in.ObservedAt, ReceivedAt: now, Healthy: in.Healthy, LatencyMS: in.LatencyMS})
	if e != nil {
		return out, core.Invalid("observation", "观测时间或阈值无效；未来时间最多允许15秒偏差")
	}
	out.ID, out.Applied, out.IgnoredReason = core.NewID(), ignored == "", ignored
	_, e = tx.Exec(ctx, `INSERT INTO service_observations(id,org_id,source_id,revision_id,sequence,observed_at,received_at,healthy,latency_ms,signed_payload,signature,payload_sha256,applied,ignored_reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, out.ID, org, source, revision, sequence, in.ObservedAt, now, in.Healthy, in.LatencyMS, body, sig, digest, out.Applied, out.IgnoredReason)
	if e != nil {
		return ObservationAck{}, e
	}
	if out.Applied {
		_, e = tx.Exec(ctx, `UPDATE service_source_states SET last_sequence=$3,observed_at=$4,received_at=$5,state=$6,failure_streak=$7,success_streak=$8 WHERE org_id=$1 AND revision_id=$2`, org, revision, after.Sequence, after.ObservedAt, after.ReceivedAt, after.State, after.Failures, after.Successes)
		if e != nil {
			return ObservationAck{}, e
		}
	}
	visible, visibleReason := VisibleSource(t, true, after, now)
	payload, _ := json.Marshal(map[string]any{"service_id": service, "source_id": source, "source_revision_id": revision, "observation_id": out.ID, "sequence": in.Sequence, "applied": out.Applied, "ignored_reason": ignored, "payload_sha256": digest, "state": visible, "state_reason": visibleReason})
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,reason)
 VALUES($1,$2,'monitor',$3,'service.observation_received','service_source_revision',$4,$5,$6,'succeeded','Signed external observation')`, core.NewID(), org, request, revision, sequence, digest)
	if e != nil {
		return ObservationAck{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
 VALUES($1,$2,'service.observation_received','service_source_revision',$3,$4,$5,$6)`, core.NewID(), org, revision, sequence, payload, request)
	if e != nil {
		return ObservationAck{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return ObservationAck{}, e
	}
	return out, nil
}
