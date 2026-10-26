package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
	"time"
)

// AgentSummary is deliberately separate from the operator view: collector keys,
// external references, signatures and raw signed payloads never enter AI context.
// Callers hold the organization lock and supply a common-audience authorization
// callback. A multi-service incident is included only when every service passes.
func AgentSummary(ctx context.Context, tx pgx.Tx, a core.Actor, id string, from, to *time.Time, allow func(Record) error) (map[string]any, []string, error) {
	d, err := metadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return nil, nil, err
	}
	if err = allow(d); err != nil {
		return nil, nil, err
	}
	// Signed ingestion locks these heads before changing its projections. Freeze
	// them in identity order before obtaining the single evaluation timestamp.
	rows, err := tx.Query(ctx, `SELECT id FROM service_sources WHERE org_id=$1 AND service_id=$2 ORDER BY id FOR SHARE`, a.OrgID, id)
	if err != nil {
		return nil, nil, err
	}
	count := 0
	for rows.Next() {
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if count > 100 {
		return nil, nil, core.Invalid("service_id", "服务监控源超出本次摘要上限")
	}
	d, err = (&Service{}).present(ctx, tx, a, d)
	if err != nil {
		return nil, nil, err
	}
	end := d.EvaluatedAt
	start := end.Add(-24 * time.Hour)
	if from != nil {
		start = *from
	}
	if to != nil {
		end = *to
	}
	if (from == nil) != (to == nil) || !start.Before(end) || end.After(d.EvaluatedAt) || end.Sub(start) > 31*24*time.Hour {
		return nil, nil, core.Invalid("window", "时间窗必须成对提供、不得晚于服务器时间，且不超过31天")
	}
	truncated := map[string]bool{"sources": len(d.Sources) > 20, "observations": false, "maintenance": false, "incidents": false, "text": false}
	clip := func(s string) string {
		r := []rune(s)
		if len(r) > 500 {
			truncated["text"] = true
			return string(r[:500])
		}
		return s
	}
	sources := []map[string]any{}
	for _, s := range d.Sources[:min(len(d.Sources), 20)] {
		sources = append(sources, map[string]any{"id": s.ID, "revision_id": s.RevisionID, "name": s.Name, "enabled": s.Enabled, "state": s.State, "state_reason": s.StateReason, "projection": s.Projection})
	}
	observations := []Observation{}
	rows, err = tx.Query(ctx, `SELECT o.id,o.source_id,o.revision_id,o.sequence::text,o.observed_at,o.received_at,o.healthy,o.latency_ms,o.payload_sha256,o.applied,o.ignored_reason
 FROM service_observations o JOIN service_sources h ON h.org_id=o.org_id AND h.id=o.source_id
 WHERE o.org_id=$1 AND h.service_id=$2 AND o.received_at>=$3 AND o.received_at<=$4 ORDER BY o.received_at DESC,o.id DESC LIMIT 21`, a.OrgID, id, start, end)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var o Observation
		if err = rows.Scan(&o.ID, &o.SourceID, &o.RevisionID, &o.Sequence, &o.ObservedAt, &o.ReceivedAt, &o.Healthy, &o.LatencyMS, &o.PayloadSHA256, &o.Applied, &o.IgnoredReason); err != nil {
			rows.Close()
			return nil, nil, err
		}
		observations = append(observations, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(observations) > 20 {
		truncated["observations"] = true
		observations = observations[:20]
	}
	windows := []MaintenanceWindow{}
	rows, err = tx.Query(ctx, `SELECT w.id,w.service_id,w.start_at,w.end_at,w.reason,w.created_by,w.created_at,c.cancelled_by,c.cancelled_at,c.reason
 FROM maintenance_windows w LEFT JOIN maintenance_cancellations c ON c.org_id=w.org_id AND c.window_id=w.id
 WHERE w.org_id=$1 AND w.service_id=$2 AND w.start_at<=$4 AND w.end_at>$3 ORDER BY w.start_at DESC,w.id DESC LIMIT 21`, a.OrgID, id, start, end)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var w MaintenanceWindow
		if err = rows.Scan(&w.ID, &w.ServiceID, &w.Start, &w.End, &w.Reason, &w.CreatedBy, &w.CreatedAt, &w.CancelledBy, &w.CancelledAt, &w.CancellationReason); err != nil {
			rows.Close()
			return nil, nil, err
		}
		w.Reason = clip(w.Reason)
		if w.CancellationReason != nil {
			v := clip(*w.CancellationReason)
			w.CancellationReason = &v
		}
		windows = append(windows, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(windows) > 20 {
		truncated["maintenance"] = true
		windows = windows[:20]
	}
	// Include currently unresolved incidents even if they began before the window.
	rows, err = tx.Query(ctx, `SELECT `+incidentColumns+` FROM incidents i WHERE i.org_id=$1 AND $2::uuid=ANY(i.service_ids)
 AND i.created_at<=$4 AND (i.state<>'resolved' OR i.resolved_at>=$3) ORDER BY i.updated_at DESC,i.id DESC LIMIT 101`, a.OrgID, id, start, end)
	if err != nil {
		return nil, nil, err
	}
	candidates := []Incident{}
	for rows.Next() {
		v, e := scanIncident(rows)
		if e != nil {
			rows.Close()
			return nil, nil, e
		}
		candidates = append(candidates, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	incidents := []map[string]any{}
	related := []string{id}
	if len(candidates) > 100 {
		truncated["incidents"] = true
		candidates = candidates[:100]
	}
	for _, v := range candidates {
		permitted := true
		for _, sid := range v.ServiceIDs {
			r, e := metadata(ctx, tx, a.OrgID, sid)
			if e == nil {
				e = allow(r)
			}
			if e != nil {
				if core.PublicError(e).Status >= 500 {
					return nil, nil, e
				}
				permitted = false
				break
			}
		}
		if !permitted {
			continue
		}
		if len(incidents) == 10 {
			truncated["incidents"] = true
			break
		}
		entries := []map[string]any{}
		rows, err = tx.Query(ctx, `SELECT id,seq,state,note,resolution,postmortem,created_at FROM incident_timeline WHERE org_id=$1 AND incident_id=$2 AND created_at>=$3 AND created_at<=$4 ORDER BY seq DESC LIMIT 11`, a.OrgID, v.ID, start, end)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var eid, state, note, resolution, postmortem string
			var seq int64
			var at time.Time
			if err = rows.Scan(&eid, &seq, &state, &note, &resolution, &postmortem, &at); err != nil {
				rows.Close()
				return nil, nil, err
			}
			entries = append(entries, map[string]any{"id": eid, "seq": seq, "state": state, "note": clip(note), "resolution": clip(resolution), "postmortem": clip(postmortem), "created_at": at})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, nil, err
		}
		entryTruncated := len(entries) > 10
		if entryTruncated {
			entries = entries[:10]
		}
		incidents = append(incidents, map[string]any{"id": v.ID, "title": v.Title, "severity": v.Severity, "state_as_of_capture": v.State, "service_ids": v.ServiceIDs, "description": clip(v.Description), "resolution": clip(v.Resolution), "postmortem": clip(v.Postmortem), "version": v.Version, "created_at": v.CreatedAt, "updated_at": v.UpdatedAt, "resolved_at": v.ResolvedAt, "updates": entries, "updates_truncated": entryTruncated})
		related = append(related, v.ServiceIDs...)
	}
	return map[string]any{"service_id": id, "name": d.Name, "description": clip(d.Description), "configuration_version": d.Version, "lifecycle": d.Lifecycle, "captured_at": d.EvaluatedAt, "recheck_at": d.RecheckAt, "window": map[string]any{"from": start, "to": end}, "state_as_of_capture": d.Summary, "sources": sources, "observations": observations, "maintenance": windows, "incidents": incidents, "truncated": truncated, "untrusted_data": true, "historical_snapshot": true, "coverage_notice": "当前状态按 captured_at 计算；时间窗按接收时间筛选观测，不重建窗末状态。包含未解决旧事件；仅展示共同获授权的完整事件。截断项不是完整记录。"}, related, nil
}
