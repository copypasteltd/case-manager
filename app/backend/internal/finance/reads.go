package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

const columns = `c.id,c.version,c.source_key,c.project_id,c.creator_id,c.classification,c.entry_kind,c.state,c.reverses_id,c.current_revision_id,r.content_hash,r.payload,c.created_at,c.updated_at,
    (SELECT x.id FROM actual_cost_entries x WHERE x.org_id=c.org_id AND x.reverses_id=c.id)`
const currentJoin = ` FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.cost_id=c.id AND r.id=c.current_revision_id `

func scan(row pgx.Row) (Cost, error) {
	c := Cost{}
	var payload []byte
	err := row.Scan(&c.ID, &c.Version, &c.SourceKey, &c.ProjectID, &c.CreatorID, &c.Classification, &c.EntryKind, &c.State, &c.ReversesID, &c.CurrentRevisionID, &c.ContentHash, &payload, &c.CreatedAt, &c.UpdatedAt, &c.ReversalID)
	if err != nil {
		return c, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &c.Content)
	return c, err
}
func metadata(ctx context.Context, db platform.DBTX, org, id string) (Cost, error) {
	if !core.ValidID(id) {
		return Cost{}, core.Missing()
	}
	return scan(db.QueryRow(ctx, "SELECT "+columns+currentJoin+"WHERE c.org_id=$1 AND c.id=$2", org, id))
}
func readable(ctx context.Context, db platform.DBTX, a core.Actor, c Cost, revision string) error {
	var ok bool
	if err := db.QueryRow(ctx, "SELECT can_read_cost($1,$2,$3,$4,$5,$6)", a.OrgID, a.UserID, c.ID, revision, a.Channel, a.RunID != "").Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return core.Missing()
	}
	return nil
}
func writable(ctx context.Context, db platform.DBTX, a core.Actor, c Cost) bool {
	if a.RunID != "" || projects(ctx, db, a, c, c.Content, false) != nil {
		return false
	}
	for _, id := range projectIDs(c, c.Content) {
		var state string
		if db.QueryRow(ctx, "SELECT state FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&state) != nil || slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
			return false
		}
	}
	return true
}
func present(ctx context.Context, db platform.DBTX, a core.Actor, c Cost) (Cost, error) {
	if err := readable(ctx, db, a, c, c.CurrentRevisionID); err != nil {
		return Cost{}, err
	}
	var err error
	c.CounterpartyEvidence, c.Attachments, err = references(ctx, db, a, c, c.Content, false, false)
	if err != nil {
		return Cost{}, err
	}
	c.Links, err = links(ctx, db, a, c)
	if err != nil {
		return Cost{}, err
	}
	if err = costSettlement(ctx, db, a, &c); err != nil {
		return Cost{}, err
	}
	write := writable(ctx, db, a, c)
	c.CanEdit = write && c.State == "draft" && authz.Require(ctx, db, a, "cost.record", resource(c)) == nil
	c.CanConfirm = write && c.State == "draft" && authz.Require(ctx, db, a, "cost.confirm", resource(c)) == nil
	c.CanReverse = write && c.State == "confirmed" && c.EntryKind == "expense" && authz.Require(ctx, db, a, "cost.reverse", resource(c)) == nil
	var unpaid bool
	if err = db.QueryRow(ctx, "SELECT COALESCE(sum(effect*amount),0)=0 FROM payment_allocations WHERE org_id=$1 AND cost_id=$2", a.OrgID, c.ID).Scan(&unpaid); err != nil {
		return Cost{}, err
	}
	c.CanReverse = c.CanReverse && unpaid
	c.CanLink = write && c.State == "confirmed" && c.EntryKind == "expense" && authz.Require(ctx, db, a, "cost.record", resource(c)) == nil
	return c, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Cost, error) {
	c, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return c, err
	}
	return present(ctx, s.DB, a, c)
}
func validateFilter(f Filter) error {
	if len([]rune(f.Query)) > 200 || len([]rune(f.Category)) > 50 || !slices.Contains([]string{"", "draft", "confirmed", "reversed", "incurred"}, f.State) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) || (f.CompanyOnly && f.ProjectID != nil) || core.DateRange(f.From, f.To) != nil {
		return core.Invalid("filter", "费用筛选无效")
	}
	return nil
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Cost], error) {
	out := core.Page[Cost]{Items: []Cost{}}
	if err := validateFilter(f); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	bound := core.CursorBinding(a, "costs", f.Query, f.State, f.Currency, f.Category, f.From, f.To, fmt.Sprint(f.ProjectID != nil), value(f.ProjectID), fmt.Sprint(f.CompanyOnly))
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		v, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+columns+currentJoin+`WHERE c.org_id=$1 AND can_read_cost(c.org_id,$2,c.id,r.id,$3,$4)
        AND ($5='' OR position(lower($5) in lower(r.title))>0 OR c.source_key=$5) AND ($6='' OR c.state=$6 OR ($6='incurred' AND c.state<>'draft')) AND ($7='' OR r.currency=$7)
        AND ($8='' OR r.category=$8) AND ($9::date IS NULL OR r.occurred_on>=$9) AND ($10::date IS NULL OR r.occurred_on<$10)
        AND (($11::uuid IS NULL AND NOT $12) OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=c.org_id AND a.revision_id=r.id AND a.project_id IS NOT DISTINCT FROM $11::uuid))
        AND ($13::timestamptz IS NULL OR (c.updated_at,c.id)<($13,$14::uuid)) ORDER BY c.updated_at DESC,c.id DESC LIMIT $15`, a.OrgID, a.UserID, a.Channel, a.RunID != "", f.Query, f.State, f.Currency, f.Category, optional(f.From), optional(f.To), f.ProjectID, f.CompanyOnly, at, before, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		c, e := scan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &v
	}
	for i, c := range out.Items {
		out.Items[i], err = present(ctx, s.DB, a, c)
		if err != nil {
			return core.Page[Cost]{Items: []Cost{}}, err
		}
	}
	return out, nil
}
func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func scanRevision(row pgx.Row) (Revision, error) {
	var r Revision
	var payload []byte
	err := row.Scan(&r.ID, &r.Seq, &r.BaseRevisionID, &r.ContentHash, &payload, &r.AuthorID, &r.Reason, &r.CreatedAt)
	if err != nil {
		return r, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &r.Content)
	return r, err
}

const revisionColumns = `id,seq,base_revision_id,content_hash,payload,author_id,reason,created_at`

func (s *Service) Revision(ctx context.Context, a core.Actor, id, revision string) (Revision, error) {
	c, err := s.Get(ctx, a, id)
	if err != nil {
		return Revision{}, err
	}
	if !core.ValidID(revision) {
		return Revision{}, core.Missing()
	}
	if err = readable(ctx, s.DB, a, c, revision); err != nil {
		return Revision{}, err
	}
	r, err := scanRevision(s.DB.QueryRow(ctx, "SELECT "+revisionColumns+" FROM cost_revisions WHERE org_id=$1 AND cost_id=$2 AND id=$3", a.OrgID, id, revision))
	if err != nil {
		return r, err
	}
	r.CounterpartyEvidence, r.Attachments, err = references(ctx, s.DB, a, c, r.Content, false, false)
	return r, err
}
func (s *Service) History(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	c, err := s.Get(ctx, a, id)
	if err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "cost-history", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+` FROM cost_revisions WHERE org_id=$1 AND cost_id=$2 AND can_read_cost(org_id,$3,cost_id,id,$4,$5) AND ($6::timestamptz IS NULL OR (created_at,id)<($6,$7::uuid)) ORDER BY created_at DESC,id DESC LIMIT $8`, a.OrgID, id, a.UserID, a.Channel, a.RunID != "", at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		r, e := scanRevision(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		r := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: r.ID, UpdatedAt: r.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	for i := range out.Items {
		r := &out.Items[i]
		r.CounterpartyEvidence, r.Attachments, err = references(ctx, s.DB, a, c, r.Content, false, false)
		if err != nil {
			return core.Page[Revision]{Items: []Revision{}}, err
		}
	}
	return out, nil
}
func (s *Service) Events(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Event], error) {
	out := core.Page[Event]{Items: []Event{}}
	if _, err := s.Get(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "cost-events", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,cost_version,revision_id,kind,state,actor_id,reason,created_at FROM cost_events WHERE org_id=$1 AND cost_id=$2 AND can_read_cost(org_id,$3,cost_id,revision_id,$4,$5) AND ($6::timestamptz IS NULL OR (created_at,id)<($6,$7::uuid)) ORDER BY created_at DESC,id DESC LIMIT $8`, a.OrgID, id, a.UserID, a.Channel, a.RunID != "", at, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.ID, &e.Version, &e.RevisionID, &e.Kind, &e.State, &e.ActorID, &e.Reason, &e.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		e := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: e.ID, UpdatedAt: e.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	return out, nil
}
