package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

const incidentColumns = `i.id,i.creator_id,i.owner_id,i.title,i.severity,i.description,i.service_ids,i.dedup_key,i.state,i.resolution,i.postmortem,i.version,i.created_at,i.updated_at,i.resolved_at`

func scanIncident(row pgx.Row) (Incident, error) {
	var v Incident
	e := row.Scan(&v.ID, &v.CreatorID, &v.OwnerID, &v.Title, &v.Severity, &v.Description, &v.ServiceIDs, &v.DedupKey, &v.State, &v.Resolution, &v.Postmortem, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ResolvedAt)
	return v, platform.DatabaseError(e)
}
func incidentMetadata(ctx context.Context, db platform.DBTX, org, id string) (Incident, error) {
	if !core.ValidID(id) {
		return Incident{}, core.Missing()
	}
	return scanIncident(db.QueryRow(ctx, "SELECT "+incidentColumns+" FROM incidents i WHERE org_id=$1 AND id=$2", org, id))
}
func incidentAccess(ctx context.Context, db platform.DBTX, a core.Actor, ids []string, write, lock bool) error {
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	if len(ordered) < 1 || len(ordered) > 20 {
		return core.Missing()
	}
	for _, id := range ordered {
		if lock {
			var locked string
			if e := db.QueryRow(ctx, "SELECT id FROM services WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, id).Scan(&locked); e != nil {
				return platform.DatabaseError(e)
			}
		}
		d, e := metadata(ctx, db, a.OrgID, id)
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, db, a, "service.read", resource(d)); e != nil {
			return e
		}
		if write {
			if e = authz.Require(ctx, db, a, "incident.update", resource(d)); e != nil {
				return e
			}
		}
	}
	if write {
		return identity.RequireInteractiveSession(ctx, db, a, true)
	}
	return nil
}
func (s *Service) GetIncident(ctx context.Context, a core.Actor, id string) (Incident, error) {
	v, e := incidentMetadata(ctx, s.DB, a.OrgID, id)
	if e != nil {
		return v, e
	}
	if e = incidentAccess(ctx, s.DB, a, v.ServiceIDs, false, false); e != nil {
		return Incident{}, e
	}
	v.CanUpdate = incidentCapability(ctx, s.DB, a, v.ServiceIDs) == nil
	return v, nil
}

// A capability lets the interface offer MFA step-up; recent MFA is enforced
// again inside the actual write transaction and is not a visibility rule.
func incidentCapability(ctx context.Context, db platform.DBTX, a core.Actor, ids []string) error {
	if err := incidentAccess(ctx, db, a, ids, false, false); err != nil {
		return err
	}
	for _, id := range ids {
		d, err := metadata(ctx, db, a.OrgID, id)
		if err != nil {
			return err
		}
		if err = authz.Require(ctx, db, a, "incident.update", resource(d)); err != nil {
			return err
		}
	}
	return identity.RequireInteractiveSession(ctx, db, a, false)
}
func (s *Service) refreshIncident(ctx context.Context, a core.Actor, out core.Result, e error) (core.Result, error) {
	if e != nil || !out.Replayed {
		return out, e
	}
	raw, _ := json.Marshal(out.Data)
	var v Incident
	if e = json.Unmarshal(raw, &v); e != nil {
		return core.Result{}, e
	}
	out.Data, e = s.GetIncident(ctx, a, v.ID)
	return out, e
}
func incidentResult(v Incident, event, reason string, status int) core.Result {
	return core.Result{Status: status, Data: v, Event: core.Event{Type: event, ResourceType: "incident", ResourceID: v.ID, Version: v.Version, Payload: map[string]any{"reason": reason, "service_ids": v.ServiceIDs, "state": v.State}}}
}
func insertEntry(ctx context.Context, tx pgx.Tx, a core.Actor, v Incident, note string) error {
	_, e := tx.Exec(ctx, `INSERT INTO incident_timeline(id,org_id,incident_id,seq,state,note,resolution,postmortem,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, core.NewID(), a.OrgID, v.ID, v.Version, v.State, note, v.Resolution, v.Postmortem, a.UserID)
	return e
}
func (s *Service) CreateIncident(ctx context.Context, c core.Command, in IncidentCreate) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.refreshIncident(ctx, c.Actor, out, commandErr) }()
	if core.Name(in.Title) != nil || core.Name(in.DedupKey) != nil || core.Name(in.Reason) != nil || len([]rune(strings.TrimSpace(in.TimelineNote))) < 1 || len([]rune(in.TimelineNote)) > 10000 || len([]rune(in.Description)) > 10000 || !core.ValidID(in.OwnerID) || !slices.Contains([]string{"low", "medium", "high", "critical"}, in.Severity) || len(in.ServiceIDs) < 1 || len(in.ServiceIDs) > 20 {
		return out, core.Invalid("incident", "请明确事件标题、影响服务、负责人、严重程度、去重标识与时间线说明")
	}
	seen := map[string]bool{}
	for i, id := range in.ServiceIDs {
		id = strings.ToLower(id)
		if !core.ValidID(id) || seen[id] {
			return out, core.Invalid("service_ids", "事件影响服务无效或重复")
		}
		seen[id] = true
		in.ServiceIDs[i] = id
	}
	wire, _ := json.Marshal(in)
	hash := core.Digest(wire)
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return incidentAccess(ctx, tx, c.Actor, in.ServiceIDs, true, false) }, func(tx pgx.Tx) (core.Result, error) {
		if e := incidentAccess(ctx, tx, c.Actor, in.ServiceIDs, true, true); e != nil {
			return core.Result{}, e
		}
		// Serialize a business deduplication key before checking it. A different
		// Idempotency-Key must not create a second incident for the same event.
		if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", c.Actor.OrgID+"/incident/"+in.DedupKey); e != nil {
			return core.Result{}, e
		}
		var id, oldHash string
		e := tx.QueryRow(ctx, "SELECT id,creation_sha256 FROM incidents WHERE org_id=$1 AND dedup_key=$2", c.Actor.OrgID, in.DedupKey).Scan(&id, &oldHash)
		if e == nil {
			v, e := incidentMetadata(ctx, tx, c.Actor.OrgID, id)
			if e != nil {
				return core.Result{}, e
			}
			if e = incidentAccess(ctx, tx, c.Actor, v.ServiceIDs, true, false); e != nil {
				return core.Result{}, e
			}
			if oldHash != hash {
				return core.Result{}, core.Fail(409, "INCIDENT_DEDUPLICATION_CONFLICT", "该事件去重标识已用于不同内容")
			}
			v.CanUpdate = true
			return incidentResult(v, "incident.creation_deduplicated", in.Reason, 200), nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return core.Result{}, e
		}
		for _, sid := range in.ServiceIDs {
			d, e := metadata(ctx, tx, c.Actor.OrgID, sid)
			if e != nil {
				return core.Result{}, e
			}
			if e = ownerAllowed(ctx, tx, c.Actor.OrgID, in.OwnerID, d.ProjectID); e != nil {
				return core.Result{}, e
			}
		}
		id = core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO incidents(id,org_id,creator_id,owner_id,title,severity,description,service_ids,dedup_key,creation_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, c.Actor.OrgID, c.Actor.UserID, in.OwnerID, strings.TrimSpace(in.Title), in.Severity, in.Description, in.ServiceIDs, in.DedupKey, hash)
		if e != nil {
			return core.Result{}, e
		}
		for pos, sid := range in.ServiceIDs {
			if _, e = tx.Exec(ctx, "INSERT INTO incident_services(org_id,incident_id,service_id,position) VALUES($1,$2,$3,$4)", c.Actor.OrgID, id, sid, pos+1); e != nil {
				return core.Result{}, e
			}
		}
		v, e := incidentMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if e = insertEntry(ctx, tx, c.Actor, v, in.TimelineNote); e != nil {
			return core.Result{}, e
		}
		v.CanUpdate = true
		return incidentResult(v, "incident.opened", in.Reason, 201), nil
	})
}
func (s *Service) UpdateIncident(ctx context.Context, c core.Command, id string, in IncidentPatch) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.refreshIncident(ctx, c.Actor, out, commandErr) }()
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil || len([]rune(strings.TrimSpace(in.TimelineNote))) < 1 || len([]rune(in.TimelineNote)) > 10000 || len([]rune(in.Resolution)) > 10000 || len([]rune(in.Postmortem)) > 10000 || !slices.Contains([]string{"open", "investigating", "monitoring", "resolved"}, in.State) || (in.State == "resolved" && strings.TrimSpace(in.Resolution) == "") {
		return out, core.Invalid("incident", "请核对事件版本、时间线、状态及解决说明")
	}
	check := func(tx pgx.Tx) (Incident, error) {
		v, e := incidentMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return v, e
		}
		return v, incidentAccess(ctx, tx, c.Actor, v.ServiceIDs, true, false)
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		v, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = incidentAccess(ctx, tx, c.Actor, v.ServiceIDs, true, true); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM incidents WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		v, e = check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if v.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if v.State == "resolved" && in.State != "resolved" {
			return core.Result{}, core.UnexpectedState(v.State, in.State)
		}
		before, _ := json.Marshal(v)
		_, e = tx.Exec(ctx, `UPDATE incidents SET state=$3,resolution=$4,postmortem=$5,version=version+1,updated_at=clock_timestamp(),
 resolved_at=CASE WHEN $3='resolved' THEN COALESCE(resolved_at,clock_timestamp()) ELSE NULL END WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, in.State, in.Resolution, in.Postmortem)
		if e != nil {
			return core.Result{}, e
		}
		v, e = incidentMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if e = insertEntry(ctx, tx, c.Actor, v, in.TimelineNote); e != nil {
			return core.Result{}, e
		}
		v.CanUpdate = true
		result := incidentResult(v, "incident.updated", in.Reason, 200)
		result.Event.BeforeDigest = core.Digest(before)
		return result, nil
	})
}
func (s *Service) Incidents(ctx context.Context, a core.Actor, state, cursor string, limit int) (core.Page[Incident], error) {
	out := core.Page[Incident]{Items: []Incident{}}
	if !slices.Contains([]string{"", "open", "investigating", "monitoring", "resolved"}, state) {
		return out, core.Invalid("state", "事件状态筛选无效")
	}
	bound, at, id, limit, e := s.position(a, "incidents/"+state, cursor, limit)
	if e != nil {
		return out, e
	}
	rows, e := s.DB.Query(ctx, "SELECT "+incidentColumns+` FROM incidents i WHERE i.org_id=$1 AND ($2='' OR i.state=$2)
 AND NOT EXISTS(SELECT 1 FROM incident_services x JOIN services s ON s.org_id=x.org_id AND s.id=x.service_id WHERE x.org_id=i.org_id AND x.incident_id=i.id
 AND NOT can_access(s.org_id,$3,'service.read','service',s.id,s.project_id,s.creator_id,s.classification,$4,$5))
 AND ($6::timestamptz IS NULL OR (i.updated_at,i.id)<($6,$7::uuid)) ORDER BY i.updated_at DESC,i.id DESC LIMIT $8`, a.OrgID, state, a.UserID, a.Channel, a.RunID != "", at, id, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		v, e := scanIncident(rows)
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
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i := range out.Items {
		out.Items[i].CanUpdate = incidentCapability(ctx, s.DB, a, out.Items[i].ServiceIDs) == nil
	}
	return out, nil
}
func (s *Service) IncidentTimeline(ctx context.Context, a core.Actor, incident, cursor string, limit int) (core.Page[IncidentEntry], error) {
	out := core.Page[IncidentEntry]{Items: []IncidentEntry{}}
	if _, e := s.GetIncident(ctx, a, incident); e != nil {
		return out, e
	}
	bound, _, id, limit, e := s.position(a, "timeline/"+incident, cursor, limit)
	if e != nil {
		return out, e
	}
	// The immutable sequence defines event order even if transactions began
	// out of order or a database clock was adjusted.
	rows, e := s.DB.Query(ctx, `SELECT id,incident_id,seq,state,note,resolution,postmortem,actor_id,created_at FROM incident_timeline WHERE org_id=$1 AND incident_id=$2
 AND ($3::uuid IS NULL OR seq<(SELECT seq FROM incident_timeline WHERE org_id=$1 AND incident_id=$2 AND id=$3))
 ORDER BY seq DESC LIMIT $4`, a.OrgID, incident, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v IncidentEntry
		if e = rows.Scan(&v.ID, &v.IncidentID, &v.Seq, &v.State, &v.Note, &v.Resolution, &v.Postmortem, &v.ActorID, &v.CreatedAt); e != nil {
			return out, e
		}
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
