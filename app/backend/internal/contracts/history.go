package contracts

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const revisionColumns = `r.id,r.contract_id,r.seq,r.base_revision_id,r.payload_schema_version,r.content_hash,r.author_id,r.reason,r.created_at,` + contentColumns

func scanRevision(row pgx.Row) (Revision, error) {
	var r Revision
	targets := []any{&r.ID, &r.ContractID, &r.Seq, &r.BaseRevisionID, &r.PayloadSchemaVersion, &r.ContentHash, &r.AuthorID, &r.Reason, &r.CreatedAt}
	err := row.Scan(append(targets, contentTargets(&r.Content)...)...)
	return r, platform.DatabaseError(err)
}
func historyAccess(ctx context.Context, db platform.DBTX, a core.Actor, id string) error {
	d, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return err
	}
	if err = authz.Require(ctx, db, a, "contract.read", resource(d)); err != nil {
		return err
	}
	return authz.Require(ctx, db, a, "contract.update", resource(d))
}
func (s *Service) Revision(ctx context.Context, a core.Actor, id, revision string) (Revision, error) {
	if err := historyAccess(ctx, s.DB, a, id); err != nil {
		return Revision{}, err
	}
	if !core.ValidID(revision) {
		return Revision{}, core.Missing()
	}
	r, err := scanRevision(s.DB.QueryRow(ctx, "SELECT "+revisionColumns+" FROM contract_revisions r WHERE org_id=$1 AND contract_id=$2 AND id=$3", a.OrgID, id, revision))
	if err != nil {
		return r, err
	}
	d, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return Revision{}, err
	}
	err = references(ctx, s.DB, a, d, r.Content, &r.Counterparty, &r.DraftFile)
	return r, err
}
func (s *Service) History(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	if err := historyAccess(ctx, s.DB, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := binding(a, "history/"+id)
	var at *time.Time
	var before *string
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+` FROM contract_revisions r WHERE org_id=$1 AND contract_id=$2
        AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
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
	d, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		r := &out.Items[i]
		if err = references(ctx, s.DB, a, d, r.Content, &r.Counterparty, &r.DraftFile); err != nil {
			return core.Page[Revision]{Items: []Revision{}}, err
		}
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		r := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: r.ID, UpdatedAt: r.CreatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}
