package servicestatus

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (s *Service) PublishResourceConfig(ctx context.Context, c core.Command, service string, in ResourceConfigInput) (core.Result, error) {
	if e := in.ResourceConfig.validate(); e != nil {
		return core.Result{}, e
	}
	key, e := base64.StdEncoding.Strict().DecodeString(in.PublicKey)
	if e != nil || len(key) != 32 || core.Name(in.Reason) != nil || in.ExpectedSourceVersion < 0 ||
		(in.ExpectedSourceVersion == 0) != (in.BaseRevisionID == nil) || in.BaseRevisionID != nil && !core.ValidID(*in.BaseRevisionID) {
		return core.Result{}, core.Invalid("resource_monitor", "请固定当前来源修订并提供32字节Ed25519公钥及发布原因")
	}
	return s.mutate(ctx, c, service, in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if d.Lifecycle != "active" {
			return core.Result{}, core.UnexpectedState(d.Lifecycle, "publish_resource_monitor")
		}
		var id, base string
		var version int64
		e := tx.QueryRow(ctx, `SELECT id,current_revision_id,version FROM service_resource_sources WHERE org_id=$1 AND service_id=$2 FOR UPDATE`, c.Actor.OrgID, service).Scan(&id, &base, &version)
		revision := core.NewID()
		if errors.Is(e, pgx.ErrNoRows) {
			if in.ExpectedSourceVersion != 0 {
				return core.Result{}, core.Conflict()
			}
			id = core.NewID()
			_, e = tx.Exec(ctx, `INSERT INTO service_resource_sources(id,org_id,service_id,creator_id,current_revision_id,version) VALUES($1,$2,$3,$4,$5,1)`, id, c.Actor.OrgID, service, c.Actor.UserID, revision)
		} else if e == nil {
			if version != in.ExpectedSourceVersion || in.BaseRevisionID == nil || !strings.EqualFold(base, *in.BaseRevisionID) {
				return core.Result{}, core.Conflict()
			}
			_, e = tx.Exec(ctx, `UPDATE service_resource_sources SET current_revision_id=$3,version=version+1 WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, revision)
		}
		if e != nil {
			return core.Result{}, e
		}
		raw, _ := json.Marshal(in.ResourceConfig)
		_, e = tx.Exec(ctx, `INSERT INTO service_resource_revisions(id,org_id,source_id,seq,base_revision_id,config,public_key,enabled,published_by,reason)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, revision, c.Actor.OrgID, id, version+1, in.BaseRevisionID, raw, key, in.Enabled, c.Actor.UserID, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, `UPDATE services SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, service); e != nil {
			return core.Result{}, e
		}
		if _, e = ProjectResourceHealth(ctx, tx, c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		out, e := s.changed(ctx, tx, c.Actor, service, "service.resource_config_published", in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		out.Event.Payload["source_id"] = id
		out.Event.Payload["source_revision_id"] = revision
		return out, e
	})
}
func resourceSource(row pgx.Row) (ResourceSource, error) {
	var out ResourceSource
	var raw []byte
	e := row.Scan(&out.ID, &out.RevisionID, &out.Version, &raw, &out.PublishedBy, &out.Reason, &out.CreatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &out.ResourceConfig)
	}
	return out, e
}

const resourceSourceColumns = `h.id,r.id,r.seq,r.config,r.published_by,r.reason,r.created_at`
const resourceObservationColumns = `o.id,o.source_id,o.revision_id,o.sequence::text,o.observed_at,o.received_at,o.sample,o.payload_sha256,o.applied,o.ignored_reason`

func scanResourceObservation(row pgx.Row) (ResourceObservation, error) {
	var out ResourceObservation
	var raw []byte
	e := row.Scan(&out.ID, &out.SourceID, &out.RevisionID, &out.Sequence, &out.ObservedAt, &out.ReceivedAt, &raw, &out.PayloadSHA256, &out.Applied, &out.IgnoredReason)
	if e == nil {
		e = json.Unmarshal(raw, &out.ResourceMetrics)
	}
	return out, e
}
func (s *Service) resourceRead(ctx context.Context, a core.Actor, service string) (pgx.Tx, Record, error) {
	tx, e := identity.BeginInteractiveRead(ctx, s.DB, a)
	if e != nil {
		return nil, Record{}, e
	}
	d, e := metadata(ctx, tx, a.OrgID, service)
	if e == nil {
		e = authz.Require(ctx, tx, a, "service.read", resource(d))
	}
	if e != nil {
		tx.Rollback(ctx)
		return nil, d, e
	}
	return tx, d, nil
}
func (s *Service) ResourceMonitor(ctx context.Context, a core.Actor, service string) (ResourceView, error) {
	out := ResourceView{ServiceID: service, State: "unknown", StateReason: "NOT_CONFIGURED"}
	tx, d, e := s.resourceRead(ctx, a, service)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	out.ServiceVersion = d.Version
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&out.EvaluatedAt); e != nil {
		return out, e
	}
	out.RecheckAt = out.EvaluatedAt.Add(10 * time.Second)
	out.CanManage = d.Lifecycle == "active" && projectWritable(ctx, tx, a.OrgID, d.ProjectID, d.Classification, false) == nil && authz.Require(ctx, tx, a, "service.manage", resource(d)) == nil
	src, e := resourceSource(tx.QueryRow(ctx, `SELECT `+resourceSourceColumns+` FROM service_resource_sources h JOIN service_resource_revisions r
  ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id WHERE h.org_id=$1 AND h.service_id=$2`, a.OrgID, service))
	if errors.Is(e, pgx.ErrNoRows) {
		return out, tx.Commit(ctx)
	}
	if e != nil {
		return out, e
	}
	out.Source = &src
	if d.Lifecycle != "active" {
		out.StateReason = "SERVICE_ARCHIVED"
		return out, tx.Commit(ctx)
	}
	if !src.Enabled {
		out.StateReason = "SOURCE_DISABLED"
		return out, tx.Commit(ctx)
	}
	last, e := scanResourceObservation(tx.QueryRow(ctx, `SELECT `+resourceObservationColumns+` FROM service_resource_observations o
  WHERE o.org_id=$1 AND o.revision_id=$2 AND o.applied ORDER BY o.sequence DESC LIMIT 1`, a.OrgID, src.RevisionID))
	if errors.Is(e, pgx.ErrNoRows) {
		out.StateReason = "NO_OBSERVATION"
		return out, tx.Commit(ctx)
	}
	if e != nil {
		return out, e
	}
	stale := time.Duration(src.StaleAfter) * time.Second
	for _, t := range []time.Time{last.ObservedAt.Add(stale), last.ReceivedAt.Add(stale)} {
		if t.Before(out.RecheckAt) {
			out.RecheckAt = t
		}
	}
	if last.ObservedAt.After(out.EvaluatedAt) || last.ReceivedAt.After(out.EvaluatedAt) {
		out.StateReason = "CLOCK_SKEW"
	} else if !last.ObservedAt.Add(stale).After(out.EvaluatedAt) || !last.ReceivedAt.Add(stale).After(out.EvaluatedAt) {
		out.StateReason = "OBSERVATION_STALE"
	} else {
		out.State = resourceState(src.ResourceConfig, last.ResourceMetrics)
		out.StateReason = ""
		out.Observation = &last
	}
	// Unknown snapshots remain short-lived too; no negative client deadline.
	if !out.RecheckAt.After(out.EvaluatedAt) {
		out.RecheckAt = out.EvaluatedAt.Add(10 * time.Second)
	}
	return out, tx.Commit(ctx)
}
func (s *Service) ResourceObservations(ctx context.Context, a core.Actor, service, cursor string, limit int) (core.Page[ResourceObservation], error) {
	out := core.Page[ResourceObservation]{Items: []ResourceObservation{}}
	tx, _, e := s.resourceRead(ctx, a, service)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	bound, at, id, limit, e := s.position(a, "resource-observations/"+service, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT `+resourceObservationColumns+` FROM service_resource_observations o JOIN service_resource_sources h ON h.org_id=o.org_id AND h.id=o.source_id
  WHERE o.org_id=$1 AND h.service_id=$2 AND ($3::timestamptz IS NULL OR (o.received_at,o.id)<($3,$4::uuid)) ORDER BY o.received_at DESC,o.id DESC LIMIT $5`, a.OrgID, service, at, id, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		o, err := scanResourceObservation(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, o)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.ReceivedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, tx.Commit(ctx)
}
func (s *Service) ResourceRevisions(ctx context.Context, a core.Actor, service, cursor string, limit int) (core.Page[ResourceRevision], error) {
	out := core.Page[ResourceRevision]{Items: []ResourceRevision{}}
	tx, _, e := s.resourceRead(ctx, a, service)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	bound, at, id, limit, e := s.position(a, "resource-revisions/"+service, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT `+resourceSourceColumns+`,r.base_revision_id,(r.id=h.current_revision_id) FROM service_resource_sources h JOIN service_resource_revisions r
  ON r.org_id=h.org_id AND r.source_id=h.id WHERE h.org_id=$1 AND h.service_id=$2 AND ($3::timestamptz IS NULL OR (r.created_at,r.id)<($3,$4::uuid)) ORDER BY r.created_at DESC,r.id DESC LIMIT $5`, a.OrgID, service, at, id, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v ResourceRevision
		var raw []byte
		e = rows.Scan(&v.ID, &v.RevisionID, &v.Version, &raw, &v.PublishedBy, &v.Reason, &v.CreatedAt, &v.BaseRevisionID, &v.IsCurrent)
		if e == nil {
			e = json.Unmarshal(raw, &v.ResourceConfig)
		}
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.RevisionID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, tx.Commit(ctx)
}
func (s *Service) ResourceObservationProof(ctx context.Context, a core.Actor, service, id string) (ResourceProof, error) {
	var out ResourceProof
	tx, _, e := s.resourceRead(ctx, a, service)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	out.ResourceObservation, e = scanResourceObservation(tx.QueryRow(ctx, `SELECT `+resourceObservationColumns+` FROM service_resource_observations o JOIN service_resource_sources h
  ON h.org_id=o.org_id AND h.id=o.source_id WHERE o.org_id=$1 AND h.service_id=$2 AND o.id=$3`, a.OrgID, service, id))
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	var raw, sig, key []byte
	e = tx.QueryRow(ctx, `SELECT o.signed_payload,o.signature,r.public_key FROM service_resource_observations o JOIN service_resource_revisions r ON r.org_id=o.org_id AND r.source_id=o.source_id AND r.id=o.revision_id WHERE o.org_id=$1 AND o.id=$2`, a.OrgID, id).Scan(&raw, &sig, &key)
	if e != nil {
		return out, e
	}
	if core.Digest(raw) != out.PayloadSHA256 || !ed25519.Verify(key, ResourceSigningMessage(out.SourceID, raw), sig) {
		return out, core.Fail(409, "RESOURCE_EVIDENCE_INVALID", "原始签名证据未通过校验")
	}
	out.SignedPayload = base64.StdEncoding.EncodeToString(raw)
	out.Signature = base64.StdEncoding.EncodeToString(sig)
	out.PublicKey = base64.StdEncoding.EncodeToString(key)
	return out, tx.Commit(ctx)
}
func (s *Service) IngestResources(ctx context.Context, source, signature, request string, body []byte) (ObservationAck, error) {
	var out ObservationAck
	denied := func() error { return core.Fail(401, "PROBE_AUTHENTICATION_FAILED", "观测来源凭证无效") }
	if !core.ValidID(source) {
		return out, denied()
	}
	source = strings.ToLower(source)
	sig, e := base64.StdEncoding.Strict().DecodeString(signature)
	if e != nil || len(sig) != ed25519.SignatureSize {
		return out, denied()
	}
	in, e := ParseResourceObservation(body)
	if e != nil {
		return out, e
	}
	seq, _ := strconv.ParseInt(in.Sequence, 10, 64)
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var org, service string
	e = tx.QueryRow(ctx, `SELECT org_id,service_id FROM service_resource_sources WHERE id=$1`, source).Scan(&org, &service)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, denied()
	}
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR SHARE`, org); e != nil {
		return out, e
	}
	var life string
	if e = tx.QueryRow(ctx, `SELECT lifecycle FROM services WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, service).Scan(&life); e != nil {
		return out, e
	}
	var revision string
	var key, raw []byte
	var enabled bool
	e = tx.QueryRow(ctx, `SELECT r.id,r.public_key,r.enabled,r.config FROM service_resource_sources h JOIN service_resource_revisions r
  ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id WHERE h.org_id=$1 AND h.id=$2 FOR UPDATE OF h`, org, source).Scan(&revision, &key, &enabled, &raw)
	if e != nil {
		return out, e
	}
	if life != "active" || !enabled || !strings.EqualFold(in.RevisionID, revision) || !ed25519.Verify(key, ResourceSigningMessage(source, body), sig) {
		return out, denied()
	}
	var conf ResourceConfig
	if e = json.Unmarshal(raw, &conf); e != nil {
		return out, e
	}
	if e = in.ResourceMetrics.Validate(conf.Mountpoints); e != nil {
		return out, e
	}
	digest := core.Digest(body)
	out.SourceID = source
	out.RevisionID = revision
	out.Sequence = in.Sequence
	out.PayloadSHA256 = digest
	var priorHash string
	e = tx.QueryRow(ctx, `SELECT id,applied,ignored_reason,payload_sha256 FROM service_resource_observations WHERE org_id=$1 AND revision_id=$2 AND sequence=$3`, org, revision, seq).Scan(&out.ID, &out.Applied, &out.IgnoredReason, &priorHash)
	if e == nil {
		if priorHash != digest {
			return ObservationAck{}, core.Fail(409, "OBSERVATION_SEQUENCE_CONFLICT", "该来源修订的序号已有不同观测")
		}
		out.Replayed = true
		return out, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if in.ObservedAt.After(now.Add(15 * time.Second)) {
		return out, core.Invalid("observed_at", "观测时间最多允许15秒未来偏差")
	}
	var lastSeq int64
	var lastAt time.Time
	e = tx.QueryRow(ctx, `SELECT sequence,observed_at FROM service_resource_observations WHERE org_id=$1 AND revision_id=$2 AND applied ORDER BY sequence DESC LIMIT 1`, org, revision).Scan(&lastSeq, &lastAt)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	switch {
	case seq <= lastSeq:
		out.IgnoredReason = "LATE_SEQUENCE"
	case !in.ObservedAt.After(lastAt):
		out.IgnoredReason = "LATE_OBSERVATION"
	case !in.ObservedAt.Add(time.Duration(conf.StaleAfter) * time.Second).After(now):
		out.IgnoredReason = "STALE_SAMPLE"
	}
	out.ID = core.NewID()
	out.Applied = out.IgnoredReason == ""
	metrics, _ := json.Marshal(in.ResourceMetrics)
	_, e = tx.Exec(ctx, `INSERT INTO service_resource_observations(id,org_id,source_id,revision_id,sequence,observed_at,received_at,sample,signed_payload,signature,payload_sha256,applied,ignored_reason)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, out.ID, org, source, revision, seq, in.ObservedAt, now, metrics, body, sig, digest, out.Applied, out.IgnoredReason)
	if e != nil {
		return out, e
	}
	payload, _ := json.Marshal(map[string]any{"service_id": service, "source_id": source, "source_revision_id": revision, "observation_id": out.ID, "sequence": in.Sequence, "applied": out.Applied, "ignored_reason": out.IgnoredReason, "payload_sha256": digest})
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,reason)
  VALUES($1,$2,'monitor',$3,'service.resource_observation_received','service',$4,$5,$6,'succeeded','Signed host resource observation')`, core.NewID(), org, request, service, seq, digest)
	if e != nil {
		return ObservationAck{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
  VALUES($1,$2,'service.resource_observation_received','service_resource_revision',$3,$4,$5,$6)`, core.NewID(), org, revision, seq, payload, request)
	if e != nil {
		return ObservationAck{}, e
	}
	if _, e = ProjectResourceHealth(ctx, tx, org, source); e != nil {
		return ObservationAck{}, e
	}
	return out, tx.Commit(ctx)
}
