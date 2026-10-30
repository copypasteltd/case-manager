package notification

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
	"time"
)

// ScheduleResources walks a bounded source page, including disabled sources.
// A source's published threshold is its explicit reminder policy. Each source
// commits independently through the same locks as signed ingestion/configuration.
// The returned cursor is internal, not an employee-supplied authorization token.
func (s *Service) ScheduleResources(ctx context.Context, after string) (next string, changed int, err error) {
	if after != "" && !core.ValidID(after) {
		return "", 0, core.Invalid("cursor", "资源调度游标无效")
	}
	rows, err := s.DB.Query(ctx, `SELECT org_id,id FROM service_resource_sources WHERE NULLIF($1,'')::uuid IS NULL OR id>NULLIF($1,'')::uuid ORDER BY id LIMIT 101`, after)
	if err != nil {
		return "", 0, err
	}
	type source struct{ org, id string }
	sources := []source{}
	for rows.Next() {
		var v source
		if err = rows.Scan(&v.org, &v.id); err != nil {
			rows.Close()
			return "", 0, err
		}
		sources = append(sources, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", 0, err
	}
	if len(sources) > 100 {
		sources = sources[:100]
		next = sources[99].id
	}
	for _, v := range sources {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		tx, e := s.DB.Begin(bounded)
		if e != nil {
			cancel()
			return after, changed, e
		}
		applied, e := servicestatus.ProjectResourceHealth(bounded, tx, v.org, v.id)
		if e == nil {
			e = tx.Commit(bounded)
		}
		tx.Rollback(context.WithoutCancel(ctx))
		cancel()
		if e != nil {
			return after, changed, e
		}
		if applied {
			changed++
		}
	}
	return next, changed, nil
}
