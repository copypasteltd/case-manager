package auditlog

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// The column allowlist is shared by live reads and immutable exports. Never use
// SELECT e.*: free-form reason and session_id belong to a different data plane.
const eventColumns = `e.id,e.occurred_at,e.actor_id,e.executor_id,e.channel,e.action,e.result,e.resource_type,e.resource_id,e.resource_version::text,e.request_id,e.run_id,e.job_id,e.before_digest,e.after_digest`
const eventWhere = ` e.org_id=$1 AND e.occurred_at>=$2 AND e.occurred_at<$3
 AND ($4='' OR e.actor_id=NULLIF($4,'')::uuid) AND ($5='' OR e.executor_id=NULLIF($5,'')::uuid)
 AND ($6='' OR e.channel=$6) AND ($7='' OR e.action=$7) AND ($8='' OR e.result=$8)
 AND ($9='' OR e.resource_type=$9) AND ($10='' OR e.resource_id=NULLIF($10,'')::uuid)
 AND ($11='' OR e.request_id=$11) AND ($12='' OR e.run_id=NULLIF($12,'')::uuid)
 AND ($13='' OR e.job_id=NULLIF($13,'')::uuid)`

func filterArgs(org string, f Filter) []any {
	from, _ := time.Parse(time.RFC3339Nano, f.From)
	to, _ := time.Parse(time.RFC3339Nano, f.To)
	return []any{org, from, to, f.ActorID, f.ExecutorID, f.Channel, f.Action, f.Result, f.ResourceType, f.ResourceID, f.RequestID, f.RunID, f.JobID}
}

func scanEvent(row pgx.Row) (Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.OccurredAt, &e.ActorID, &e.ExecutorID, &e.Channel, &e.Action, &e.Result, &e.ResourceType, &e.ResourceID, &e.ResourceVersion, &e.RequestID, &e.RunID, &e.JobID, &e.BeforeDigest, &e.AfterDigest)
	return e, platform.DatabaseError(err)
}

func queryEvents(ctx context.Context, db platform.DBTX, org string, f Filter, before *time.Time, beforeID *string, limit int) ([]Event, error) {
	args := append(filterArgs(org, f), before, beforeID, limit)
	rows, err := db.Query(ctx, `SELECT `+eventColumns+` FROM audit_events e WHERE `+eventWhere+`
 AND ($14::timestamptz IS NULL OR (e.occurred_at,e.id)<($14,$15::uuid)) ORDER BY e.occurred_at DESC,e.id DESC LIMIT $16`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) Events(ctx context.Context, a core.Actor, f Filter, cursor string, limit int) (Page, error) {
	out := Page{Page: core.Page[Event]{Items: []Event{}}, Filters: f, Disclosure: Disclosure}
	if err := f.Validate(); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100项")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, false, false); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(f)
	binding := core.CursorBinding(a, "audit-events", string(raw))
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	items, err := queryEvents(ctx, tx, a.OrgID, f, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.OccurredAt, Binding: binding})
		out.NextCursor = &next
	}
	out.Items = items
	return out, tx.Commit(ctx)
}

func (s *Service) Event(ctx context.Context, a core.Actor, id string) (Event, error) {
	if !core.ValidID(id) {
		return Event{}, core.Missing()
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, false, false); err != nil {
		return Event{}, err
	}
	out, err := scanEvent(tx.QueryRow(ctx, `SELECT `+eventColumns+` FROM audit_events e WHERE e.org_id=$1 AND e.id=$2`, a.OrgID, id))
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
