package finance

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (s *Service) ReportExports(ctx context.Context, a core.Actor, kind, state, cursor string, limit int) (core.Page[ReportExport], error) {
	out := core.Page[ReportExport]{Items: []ReportExport{}}
	if a.RunID != "" || !slices.Contains([]string{"", "incurred", "cash", "forecast"}, kind) || !slices.Contains([]string{"", "queued", "running", "retry_wait", "succeeded", "dead", "cancelled", "expired"}, state) || limit < 1 || limit > 100 {
		return out, core.Invalid("filter", "导出类型、状态或分页无效")
	}
	bound := core.CursorBinding(a, "financial-report-exports", kind, state)
	var at *time.Time
	var before *string
	if cursor != "" {
		position, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at, before = &position.UpdatedAt, &position.ID
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = reportIdentity(ctx, tx, a); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT e.id FROM finance_report_exports e JOIN jobs j ON j.org_id=e.org_id AND j.id=e.job_id LEFT JOIN finance_report_snapshots fs ON fs.org_id=e.org_id AND fs.export_id=e.id
        WHERE e.org_id=$1 AND e.requester_id=$2 AND e.auth_version=$4 AND e.authorization_version=$5
        AND can_access(e.org_id,$2,'report.read','report',NULL,(e.filters->>'project_id')::uuid,e.requester_id,'internal',$3,false)
        AND can_access(e.org_id,$2,'report.export','report',NULL,(e.filters->>'project_id')::uuid,e.requester_id,'internal',$3,false)
        AND ($6='' OR e.filters->>'kind'=$6) AND ($7='' OR (CASE WHEN e.expires_at<=clock_timestamp() THEN 'expired' ELSE j.state END)=$7)
        AND (fs.export_id IS NULL OR e.expires_at<=clock_timestamp() OR `+reportSnapshotReadable+`)
        AND ($8::timestamptz IS NULL OR (e.created_at,e.id)<($8,$9::uuid)) ORDER BY e.created_at DESC,e.id DESC LIMIT $10`, a.OrgID, a.UserID, a.Channel, a.AuthVersion, a.OrgAuthVersion, kind, state, at, before, limit+1)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	for _, id := range ids {
		item, e := reportExportInTx(ctx, tx, a, id, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	if more {
		last := out.Items[len(out.Items)-1]
		value := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &value
	}
	return out, tx.Commit(ctx)
}
