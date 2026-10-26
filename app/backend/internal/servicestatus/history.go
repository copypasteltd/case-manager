package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/base64"
	"time"
)

func (s *Service) authorizeRead(ctx context.Context, a core.Actor, id string) error {
	d, e := metadata(ctx, s.DB, a.OrgID, id)
	if e != nil {
		return e
	}
	return authz.Require(ctx, s.DB, a, "service.read", resource(d))
}
func (s *Service) position(a core.Actor, scope, cursor string, limit int) (string, *time.Time, *string, int, error) {
	bound := core.CursorBinding(a, "status-history", scope)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if cursor == "" {
		return bound, nil, nil, limit, nil
	}
	c, e := s.Pager.Decode(cursor, bound)
	if e != nil {
		return bound, nil, nil, limit, e
	}
	return bound, &c.UpdatedAt, &c.ID, limit, nil
}
func (s *Service) Observations(ctx context.Context, a core.Actor, service, cursor string, limit int) (core.Page[Observation], error) {
	out := core.Page[Observation]{Items: []Observation{}}
	if e := s.authorizeRead(ctx, a, service); e != nil {
		return out, e
	}
	bound, at, id, limit, e := s.position(a, "observations/"+service, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT o.id,o.source_id,o.revision_id,o.sequence::text,o.observed_at,o.received_at,o.healthy,o.latency_ms,o.payload_sha256,o.applied,o.ignored_reason
 FROM service_observations o JOIN service_sources h ON h.org_id=o.org_id AND h.id=o.source_id WHERE o.org_id=$1 AND h.service_id=$2
 AND ($3::timestamptz IS NULL OR (o.received_at,o.id)<($3,$4::uuid)) ORDER BY o.received_at DESC,o.id DESC LIMIT $5`, a.OrgID, service, at, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var o Observation
		if e = rows.Scan(&o.ID, &o.SourceID, &o.RevisionID, &o.Sequence, &o.ObservedAt, &o.ReceivedAt, &o.Healthy, &o.LatencyMS, &o.PayloadSHA256, &o.Applied, &o.IgnoredReason); e != nil {
			return out, e
		}
		out.Items = append(out.Items, o)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.ReceivedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}

type ObservationProof struct {
	Observation
	SignedPayload string `json:"signed_payload"`
	Signature     string `json:"signature"`
	PublicKey     string `json:"public_key"`
}

func (s *Service) ObservationProof(ctx context.Context, a core.Actor, service, id string) (ObservationProof, error) {
	out := ObservationProof{}
	if e := s.authorizeRead(ctx, a, service); e != nil {
		return out, e
	}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	var body, sig, key []byte
	e := s.DB.QueryRow(ctx, `SELECT o.id,o.source_id,o.revision_id,o.sequence::text,o.observed_at,o.received_at,o.healthy,o.latency_ms,o.payload_sha256,o.applied,o.ignored_reason,
 o.signed_payload,o.signature,r.public_key FROM service_observations o JOIN service_sources h ON h.org_id=o.org_id AND h.id=o.source_id
 JOIN service_source_revisions r ON r.org_id=o.org_id AND r.id=o.revision_id AND r.source_id=o.source_id WHERE o.org_id=$1 AND h.service_id=$2 AND o.id=$3`, a.OrgID, service, id).
		Scan(&out.ID, &out.SourceID, &out.RevisionID, &out.Sequence, &out.ObservedAt, &out.ReceivedAt, &out.Healthy, &out.LatencyMS, &out.PayloadSHA256, &out.Applied, &out.IgnoredReason, &body, &sig, &key)
	out.SignedPayload = base64.StdEncoding.EncodeToString(body)
	out.Signature = base64.StdEncoding.EncodeToString(sig)
	out.PublicKey = base64.StdEncoding.EncodeToString(key)
	return out, platform.DatabaseError(e)
}
func (s *Service) MaintenanceWindows(ctx context.Context, a core.Actor, service, cursor string, limit int) (core.Page[MaintenanceWindow], error) {
	out := core.Page[MaintenanceWindow]{Items: []MaintenanceWindow{}}
	if e := s.authorizeRead(ctx, a, service); e != nil {
		return out, e
	}
	bound, at, id, limit, e := s.position(a, "maintenance/"+service, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT w.id,w.service_id,w.start_at,w.end_at,w.reason,w.created_by,w.created_at,c.cancelled_by,c.cancelled_at,c.reason
 FROM maintenance_windows w LEFT JOIN maintenance_cancellations c ON c.org_id=w.org_id AND c.window_id=w.id WHERE w.org_id=$1 AND w.service_id=$2
 AND ($3::timestamptz IS NULL OR (w.created_at,w.id)<($3,$4::uuid)) ORDER BY w.created_at DESC,w.id DESC LIMIT $5`, a.OrgID, service, at, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var w MaintenanceWindow
		if e = rows.Scan(&w.ID, &w.ServiceID, &w.Start, &w.End, &w.Reason, &w.CreatedBy, &w.CreatedAt, &w.CancelledBy, &w.CancelledAt, &w.CancellationReason); e != nil {
			return out, e
		}
		out.Items = append(out.Items, w)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		w := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: w.ID, UpdatedAt: w.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}

func (s *Service) SourceRevisions(ctx context.Context, a core.Actor, service, source, cursor string, limit int) (core.Page[SourceRevision], error) {
	out := core.Page[SourceRevision]{Items: []SourceRevision{}}
	if e := s.authorizeRead(ctx, a, service); e != nil {
		return out, e
	}
	if !core.ValidID(source) {
		return out, core.Missing()
	}
	var current string
	if e := s.DB.QueryRow(ctx, "SELECT current_revision_id FROM service_sources WHERE org_id=$1 AND service_id=$2 AND id=$3", a.OrgID, service, source).Scan(&current); e != nil {
		return out, platform.DatabaseError(e)
	}
	bound, _, id, limit, e := s.position(a, "source-revisions/"+service+"/"+source, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, `SELECT id,source_id,seq,base_revision_id,name,provider,external_ref,public_key,enabled,
 failure_count,recovery_count,interval_seconds,stale_after_seconds,latency_limit_ms,published_by,reason,created_at
 FROM service_source_revisions WHERE org_id=$1 AND source_id=$2
 AND ($3::uuid IS NULL OR seq<(SELECT seq FROM service_source_revisions WHERE org_id=$1 AND source_id=$2 AND id=$3)) ORDER BY seq DESC LIMIT $4`, a.OrgID, source, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v SourceRevision
		var key []byte
		if e = rows.Scan(&v.ID, &v.SourceID, &v.Seq, &v.BaseRevisionID, &v.Name, &v.Provider, &v.ExternalRef, &key, &v.Enabled,
			&v.FailureCount, &v.RecoveryCount, &v.Interval, &v.StaleAfter, &v.LatencyMS, &v.PublishedBy, &v.Reason, &v.CreatedAt); e != nil {
			return out, e
		}
		v.PublicKey = base64.StdEncoding.EncodeToString(key)
		v.IsCurrent = v.ID == current
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}
