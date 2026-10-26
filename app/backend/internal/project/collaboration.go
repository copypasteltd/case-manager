package project

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Table and column fragments are fixed at call sites, never client supplied.
func listProjectResources[T any](ctx context.Context, s *Service, a core.Actor, id, table, kind, columns string, f Filter, scan func(pgx.Row) (T, error), key func(T) (string, time.Time)) (core.Page[T], error) {
	out := core.Page[T]{Items: []T{}}
	if err := authorizeProject(ctx, s.DB, a, id, "project.read"); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, kind, id, f.State, f.Query)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+columns+" FROM "+table+` r JOIN projects p ON p.org_id=r.org_id AND p.id=r.project_id
		WHERE r.org_id=$1 AND r.project_id=$2 AND can_access(r.org_id,$3,$4,$5,r.id,p.id,r.creator_id,p.classification,$6,$12)
		AND ($7='' OR r.state=$7) AND ($8='' OR position(lower($8) IN lower(r.title))>0)
		AND ($9::timestamptz IS NULL OR (r.updated_at,r.id)<($9,$10::uuid)) ORDER BY r.updated_at DESC,r.id DESC LIMIT $11`,
		a.OrgID, id, a.UserID, kind+".read", kind, a.Channel, f.State, f.Query, before, beforeID, f.Limit+1, a.RunID != "")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, value)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last, updated := key(out.Items[f.Limit-1])
		c := s.Pager.Encode(platform.Cursor{ID: last, UpdatedAt: updated, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}

func collaborationResource(p Project, kind, id, creator string) authz.Resource {
	return authz.Resource{Type: kind, ID: id, ProjectID: &p.ID, OwnerID: &creator, Classification: p.Classification}
}

func collaborationProject(ctx context.Context, db platform.DBTX, org, id, table string, lock bool) (Project, error) {
	if !core.ValidID(id) {
		return Project{}, core.Missing()
	}
	var projectID string
	if err := db.QueryRow(ctx, "SELECT project_id FROM "+table+" WHERE org_id=$1 AND id=$2", org, id).Scan(&projectID); err != nil {
		return Project{}, platform.DatabaseError(err)
	}
	return getProject(ctx, db, org, projectID, lock)
}
