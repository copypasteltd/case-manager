package servicestatus

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ResourceHealthEvent struct {
	ID            string    `json:"id"`
	SourceID      string    `json:"source_id"`
	RevisionID    string    `json:"source_revision_id"`
	RecipientID   string    `json:"recipient_id"`
	Sequence      int64     `json:"sequence"`
	BaseEventID   *string   `json:"base_event_id"`
	PreviousState string    `json:"previous_state"`
	State         string    `json:"state"`
	StateReason   string    `json:"state_reason"`
	Attention     bool      `json:"attention_required"`
	Notify        bool      `json:"notification_event_created"`
	ChangeKind    string    `json:"change_kind"`
	ObservationID *string   `json:"observation_id"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

// ProjectResourceHealth serializes on the original service/source. Signed
// ingestion, human configuration and the clock scheduler use the same code.
// It records state changes, not one notification for every metric sample.
func ProjectResourceHealth(ctx context.Context, tx pgx.Tx, org, source string) (bool, error) {
	var service, owner, revision, state, reason string
	var serviceVersion int64
	var now, ready time.Time
	var observation *string
	if _, e := tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", org); e != nil {
		return false, e
	}
	if e := tx.QueryRow(ctx, `SELECT s.id,s.owner_id,s.version FROM services s JOIN service_resource_sources h ON h.org_id=s.org_id AND h.service_id=s.id WHERE h.org_id=$1 AND h.id=$2 FOR UPDATE OF s`, org, source).Scan(&service, &owner, &serviceVersion); e != nil {
		return false, e
	}
	if _, e := tx.Exec(ctx, "SELECT id FROM service_resource_sources WHERE org_id=$1 AND id=$2 FOR UPDATE", org, source); e != nil {
		return false, e
	}
	if e := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return false, e
	}
	var attention bool
	if e := tx.QueryRow(ctx, `SELECT revision_id,state,state_reason,observation_id,ready_after,resource_needs_attention(state,state_reason,ready_after,$3) FROM service_resource_current($1,$2,$3)`, org, source, now).Scan(&revision, &state, &reason, &observation, &ready, &attention); e != nil {
		return false, e
	}
	var seq int64
	var base *string
	previous, priorRevision, priorOwner, priorReason, priorAttention := "unconfigured", "", "", "", false
	e := tx.QueryRow(ctx, `SELECT seq,event_id,revision_id,recipient_id,state,state_reason,attention FROM service_resource_health WHERE org_id=$1 AND source_id=$2 FOR UPDATE`, org, source).Scan(&seq, &base, &priorRevision, &priorOwner, &previous, &priorReason, &priorAttention)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return false, e
	}
	if e == nil && priorRevision == revision && priorOwner == owner && previous == state && priorReason == reason && priorAttention == attention {
		return false, nil
	}
	kind := "state"
	if base == nil {
		kind = "initial"
	} else if priorRevision != revision {
		kind = "configuration"
	} else if priorOwner != owner {
		kind = "responsibility"
	}
	notify := attention || priorRevision == revision && priorOwner == owner && priorAttention && state == "operational"
	id := core.NewID()
	seq++
	payload, _ := json.Marshal(map[string]any{"source_id": source, "source_revision_id": revision, "state": state, "state_reason": reason, "observation_id": observation, "attention_required": attention, "notification_event_created": notify})
	if _, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id) VALUES($1,$2,'service.resource_health.changed','service_resource_source',$3,$4,$5,$6)`, id, org, source, seq, payload, id); e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO service_resource_health_events(id,org_id,source_id,revision_id,recipient_id,seq,base_event_id,previous_state,state,state_reason,attention,notify,change_kind,observation_id,evaluated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, org, source, revision, owner, seq, base, previous, state, reason, attention, notify, kind, observation, now); e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO service_resource_health(source_id,org_id,revision_id,recipient_id,seq,event_id,state,state_reason,attention,evaluated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(source_id) DO UPDATE SET revision_id=EXCLUDED.revision_id,recipient_id=EXCLUDED.recipient_id,seq=EXCLUDED.seq,event_id=EXCLUDED.event_id,state=EXCLUDED.state,state_reason=EXCLUDED.state_reason,attention=EXCLUDED.attention,evaluated_at=EXCLUDED.evaluated_at`, source, org, revision, owner, seq, id, state, reason, attention, now); e != nil {
		return false, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,reason) VALUES($1,$2,'monitor',$3,'service.resource_health_changed','service',$4,$5,$6,'succeeded','Host resource state projection')`, core.NewID(), org, id, service, serviceVersion, core.Digest(payload))
	return e == nil, e
}

func projectServiceResource(ctx context.Context, tx pgx.Tx, org, service string) error {
	var source string
	e := tx.QueryRow(ctx, "SELECT id FROM service_resource_sources WHERE org_id=$1 AND service_id=$2", org, service).Scan(&source)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = ProjectResourceHealth(ctx, tx, org, source)
	return e
}

const resourceHealthColumns = `e.id,e.source_id,e.revision_id,e.recipient_id,e.seq,e.base_event_id,e.previous_state,e.state,e.state_reason,e.attention,e.notify,e.change_kind,e.observation_id,e.evaluated_at`

func scanResourceHealth(row pgx.Row) (ResourceHealthEvent, error) {
	var v ResourceHealthEvent
	e := row.Scan(&v.ID, &v.SourceID, &v.RevisionID, &v.RecipientID, &v.Sequence, &v.BaseEventID, &v.PreviousState, &v.State, &v.StateReason, &v.Attention, &v.Notify, &v.ChangeKind, &v.ObservationID, &v.EvaluatedAt)
	return v, e
}

func (s *Service) ResourceHealthHistory(ctx context.Context, a core.Actor, service, cursor string, limit int) (core.Page[ResourceHealthEvent], error) {
	out := core.Page[ResourceHealthEvent]{Items: []ResourceHealthEvent{}}
	tx, _, e := s.resourceRead(ctx, a, service)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	binding, at, id, limit, e := s.position(a, "resource-health/"+service, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT `+resourceHealthColumns+` FROM service_resource_health_events e JOIN service_resource_sources h ON h.org_id=e.org_id AND h.id=e.source_id WHERE e.org_id=$1 AND h.service_id=$2 AND ($3::timestamptz IS NULL OR(e.evaluated_at,e.id)<($3,$4::uuid)) ORDER BY e.evaluated_at DESC,e.id DESC LIMIT $5`, a.OrgID, service, at, id, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		v, err := scanResourceHealth(rows)
		if err != nil {
			rows.Close()
			return out, err
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
		last := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.EvaluatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, tx.Commit(ctx)
}

type ResourceNotice struct {
	Service   Record
	Attention bool
}

// NotificationResource rechecks the original recipient and current readable
// service. Delivery additionally rejects any superseded event or assignment.
func NotificationResource(ctx context.Context, tx pgx.Tx, a core.Actor, service, revision, event string, delivery bool) (ResourceNotice, error) {
	var out ResourceNotice
	if a.RunID != "" {
		return out, core.Forbidden()
	}
	if _, e := tx.Exec(ctx, "SELECT id FROM services WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, service); e != nil {
		return out, e
	}
	d, e := metadata(ctx, tx, a.OrgID, service)
	if e != nil {
		return out, e
	}
	if e = authz.Require(ctx, tx, a, "service.read", resource(d)); e != nil {
		return out, e
	}
	var currentRevision, currentEvent, currentState, currentReason string
	var ready, now time.Time
	var notify bool
	e = tx.QueryRow(ctx, `SELECT h.current_revision_id,p.event_id,c.state,c.state_reason,c.ready_after,clock_timestamp(),e.notify
 FROM service_resource_health_events e JOIN service_resource_sources h ON h.org_id=e.org_id AND h.id=e.source_id
 JOIN service_resource_health p ON p.org_id=h.org_id AND p.source_id=h.id
 CROSS JOIN LATERAL service_resource_current(h.org_id,h.id,clock_timestamp()) c
 WHERE e.org_id=$1 AND h.service_id=$2 AND e.revision_id=$3 AND e.id=$4 AND e.recipient_id=$5`, a.OrgID, service, revision, event, a.UserID).Scan(&currentRevision, &currentEvent, &currentState, &currentReason, &ready, &now, &notify)
	if e != nil {
		return out, e
	}
	if delivery && (!notify || currentRevision != revision || currentEvent != event || d.OwnerID != a.UserID || d.Lifecycle != "active") {
		return out, core.Missing()
	}
	out.Service = d
	if currentRevision == revision && d.OwnerID == a.UserID {
		e = tx.QueryRow(ctx, "SELECT resource_needs_attention($1,$2,$3,$4)", currentState, currentReason, ready, now).Scan(&out.Attention)
	}
	return out, e
}
