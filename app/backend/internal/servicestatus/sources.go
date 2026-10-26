package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/base64"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

func readSources(ctx context.Context, db platform.DBTX, org, service string, now time.Time) ([]Source, error) {
	out := []Source{}
	rows, err := db.Query(ctx, `SELECT h.id,h.service_id,r.id,r.seq,r.name,r.provider,r.external_ref,r.public_key,r.enabled,
 r.failure_count,r.recovery_count,r.interval_seconds,r.stale_after_seconds,r.latency_limit_ms,r.published_by,r.reason,r.created_at,
 COALESCE(p.last_sequence,0),p.observed_at,p.received_at,COALESCE(p.state,'unknown'),COALESCE(p.failure_streak,0),COALESCE(p.success_streak,0)
 FROM service_sources h JOIN service_source_revisions r ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id
 LEFT JOIN service_source_states p ON p.org_id=h.org_id AND p.revision_id=r.id WHERE h.org_id=$1 AND h.service_id=$2 ORDER BY h.created_at,h.id`, org, service)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Source
		var key []byte
		var p Projection
		var observed, received *time.Time
		if err = rows.Scan(&v.ID, &v.ServiceID, &v.RevisionID, &v.Version, &v.Name, &v.Provider, &v.ExternalRef, &key, &v.Enabled,
			&v.FailureCount, &v.RecoveryCount, &v.Interval, &v.StaleAfter, &v.LatencyMS, &v.PublishedBy, &v.Reason, &v.CreatedAt,
			&p.Sequence, &observed, &received, &p.State, &p.Failures, &p.Successes); err != nil {
			return out, err
		}
		if observed != nil {
			p.ObservedAt = *observed
		}
		if received != nil {
			p.ReceivedAt = *received
		}
		v.PublicKey = base64.StdEncoding.EncodeToString(key)
		v.State, v.StateReason = VisibleSource(v.Thresholds, v.Enabled, p, now)
		if p.Sequence > 0 {
			v.Projection = &p
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) PublishSource(ctx context.Context, c core.Command, service, id string, in SourceInput) (core.Result, error) {
	key, e := base64.StdEncoding.Strict().DecodeString(in.PublicKey)
	if e != nil || len(key) != 32 || core.Name(in.Name) != nil || core.Name(in.ExternalRef) != nil || core.Name(in.Reason) != nil || in.Thresholds.Validate() != nil ||
		!slices.Contains([]string{"uptime_kuma", "prometheus", "adapter"}, in.Provider) || strings.ContainsAny(in.Name+in.ExternalRef, "\r\n\x00") {
		return core.Result{}, core.Invalid("source", "请明确来源、32字节Ed25519公钥及完整观测阈值")
	}
	if (id == "" && (in.ExpectedSourceVersion != 0 || in.BaseRevisionID != nil)) || (id != "" && (!core.ValidID(id) || core.RequireVersion(in.ExpectedSourceVersion) != nil || in.BaseRevisionID == nil || !core.ValidID(*in.BaseRevisionID))) {
		return core.Result{}, core.Invalid("base_revision_id", "请固定来源的准确基础修订")
	}
	return s.mutate(ctx, c, service, in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if d.Lifecycle != "active" {
			return core.Result{}, core.UnexpectedState(d.Lifecycle, "publish_source")
		}
		revision := core.NewID()
		seq := int64(1)
		if id == "" {
			var count int
			if e := tx.QueryRow(ctx, "SELECT count(*) FROM service_sources WHERE org_id=$1 AND service_id=$2", c.Actor.OrgID, service).Scan(&count); e != nil {
				return core.Result{}, e
			}
			if count >= 20 {
				return core.Result{}, core.Invalid("source", "每项服务最多20个观测来源")
			}
			id = core.NewID()
			if _, e := tx.Exec(ctx, "INSERT INTO service_sources(id,org_id,service_id,creator_id,current_revision_id) VALUES($1,$2,$3,$4,$5)", id, c.Actor.OrgID, service, c.Actor.UserID, revision); e != nil {
				return core.Result{}, e
			}
		} else {
			var version int64
			var base string
			if e := tx.QueryRow(ctx, "SELECT version,current_revision_id FROM service_sources WHERE org_id=$1 AND service_id=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, service, id).Scan(&version, &base); e != nil {
				return core.Result{}, platform.DatabaseError(e)
			}
			if version != in.ExpectedSourceVersion || !strings.EqualFold(base, *in.BaseRevisionID) {
				return core.Result{}, core.Conflict()
			}
			seq = version + 1
			if _, e := tx.Exec(ctx, "UPDATE service_sources SET current_revision_id=$3,version=version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision); e != nil {
				return core.Result{}, e
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO service_source_revisions(id,org_id,source_id,seq,base_revision_id,name,provider,external_ref,public_key,enabled,
 failure_count,recovery_count,interval_seconds,stale_after_seconds,latency_limit_ms,published_by,reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, revision, c.Actor.OrgID, id, seq, in.BaseRevisionID, strings.TrimSpace(in.Name), in.Provider, in.ExternalRef, key, in.Enabled, in.FailureCount, in.RecoveryCount, in.Interval, in.StaleAfter, in.LatencyMS, c.Actor.UserID, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO service_source_states(org_id,source_id,revision_id) VALUES($1,$2,$3)", c.Actor.OrgID, id, revision); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE services SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, service); e != nil {
			return core.Result{}, e
		}
		out, e := s.changed(ctx, tx, c.Actor, service, "service.source_published", in.Reason)
		out.Event.Payload["source_id"] = id
		out.Event.Payload["source_revision_id"] = revision
		return out, e
	})
}
