package spend

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

const revisionColumns = `r.id,r.document_id,r.seq,r.base_revision_id,r.payload_schema_version,r.content_hash,r.payload,r.amount::text,r.employee_reimbursement_amount::text,r.author_id,r.reason,r.created_at,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('line_id',l.line_id,'amount',l.amount::text,'duplicate_receipt',false) ORDER BY l.position) FROM spend_lines l WHERE l.org_id=r.org_id AND l.revision_id=r.id),'[]'::jsonb)`

func scanRevision(row pgx.Row) (Revision, error) {
	var r Revision
	var payload, values []byte
	err := row.Scan(&r.ID, &r.DocumentID, &r.Seq, &r.BaseRevisionID, &r.PayloadSchemaVersion, &r.ContentHash, &payload, &r.Amount, &r.EmployeeReimbursementAmount, &r.AuthorID, &r.Reason, &r.CreatedAt, &values)
	if err != nil {
		return r, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &r.Content); err != nil {
		return r, err
	}
	err = json.Unmarshal(values, &r.LineValues)
	return r, err
}
func historyAccess(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string) (Record, error) {
	d, err := metadata(ctx, db, a.OrgID, kind, id)
	if err != nil {
		return d, err
	}
	for _, action := range []string{kind + ".read", kind + ".update"} {
		if err = authz.Require(ctx, db, a, action, resource(d)); err != nil {
			return Record{}, err
		}
	}
	return d, nil
}
func (s *Service) Revision(ctx context.Context, a core.Actor, kind, id, revision string) (Revision, error) {
	d, err := historyAccess(ctx, s.DB, a, kind, id)
	if err != nil {
		return Revision{}, err
	}
	if !core.ValidID(revision) {
		return Revision{}, core.Missing()
	}
	r, err := scanRevision(s.DB.QueryRow(ctx, "SELECT "+revisionColumns+" FROM spend_revisions r WHERE org_id=$1 AND document_id=$2 AND id=$3", a.OrgID, id, revision))
	if err != nil {
		return r, err
	}
	r.Suppliers, r.Attachments, err = References(ctx, s.DB, a, d, r.Content, false, false)
	return r, err
}
func (s *Service) History(ctx context.Context, a core.Actor, kind, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	d, err := historyAccess(ctx, s.DB, a, kind, id)
	if err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := binding(a, kind+"/history/"+id)
	var at *time.Time
	var before *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+` FROM spend_revisions r WHERE org_id=$1 AND document_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
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
		c := s.Pager.Encode(platform.Cursor{ID: r.ID, UpdatedAt: r.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i := range out.Items {
		r := &out.Items[i]
		r.Suppliers, r.Attachments, err = References(ctx, s.DB, a, d, r.Content, false, false)
		if err != nil {
			return core.Page[Revision]{Items: []Revision{}}, err
		}
	}
	return out, nil
}
