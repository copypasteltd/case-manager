package knowledge

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"time"
)

func (s *Service) History(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	p, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = accessible(ctx, s.DB, a, p); err != nil {
		return out, err
	}
	if err = authz.Require(ctx, s.DB, a, "post.write", resource(p)); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.CursorBinding(a, "post-history", id)
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id FROM post_revisions WHERE org_id=$1 AND post_id=$2
		AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var rev string
		if err = rows.Scan(&rev); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, rev)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, rev := range ids {
		r, err := revision(ctx, s.DB, a.OrgID, id, rev)
		if err != nil {
			return out, err
		}
		r.SourceRefs, err = filterSources(ctx, s.DB, a, r.SourceRefs)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, r)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		r := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: r.ID, UpdatedAt: r.CreatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}
