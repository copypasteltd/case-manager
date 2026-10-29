package auditlog

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func text(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// CSV cells are text. Guard spreadsheet formulas after leading whitespace,
// preserve embedded CSV quotes/newlines, and never include unselected columns.
func csvCell(s string) string {
	trimmed := strings.TrimLeftFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == 0xfeff })
	if strings.ContainsAny(s, "\t\r\n") || len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + s
	}
	return s
}

func renderCSV(events []Event) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if err := w.Write([]string{"event_id", "occurred_at_utc", "actor_id", "executor_id", "channel", "action", "result", "resource_type", "resource_id", "resource_version", "request_id", "run_id", "job_id", "before_digest", "after_digest"}); err != nil {
		return nil, err
	}
	for _, e := range events {
		cells := []string{e.ID, e.OccurredAt.UTC().Format(time.RFC3339Nano), text(e.ActorID), text(e.ExecutorID), e.Channel, e.Action, e.Result, e.ResourceType, text(e.ResourceID), text(e.ResourceVersion), e.RequestID, text(e.RunID), text(e.JobID), text(e.BeforeDigest), text(e.AfterDigest)}
		for i := range cells {
			cells[i] = csvCell(cells[i])
		}
		if err := w.Write(cells); err != nil {
			return nil, err
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, err
		}
		if b.Len() > MaxExportBytes {
			return nil, core.Fail(409, "AUDIT_EXPORT_TOO_LARGE", "导出超过8 MiB，请缩小时间或筛选范围")
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func (s *Service) CreateExport(ctx context.Context, c core.Command, in ExportCreate) (core.Result, error) {
	if err := in.Filters.Validate(); err != nil {
		return core.Result{}, err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if err := core.Name(in.Reason); err != nil {
		return core.Result{}, core.Invalid("reason", "请填写1至200字的导出用途")
	}
	out, err := s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, true, true) }, func(tx pgx.Tx) (core.Result, error) {
		events, err := queryEvents(ctx, tx, c.Actor.OrgID, in.Filters, nil, nil, MaxExportRows+1)
		if err != nil {
			return core.Result{}, err
		}
		if len(events) > MaxExportRows {
			return core.Result{}, core.Fail(409, "AUDIT_EXPORT_TOO_MANY_ROWS", "结果超过10,000条，请缩小时间或筛选范围；未截断导出")
		}
		content, err := renderCSV(events)
		if err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		raw, _ := json.Marshal(in.Filters)
		_, err = tx.Exec(ctx, `INSERT INTO audit_exports(id,org_id,requester_id,channel,auth_version,authorization_version,filters,disclosure,reason,row_count,csv_bytes,csv_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, c.Actor.OrgID, c.Actor.UserID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, raw, Disclosure, in.Reason, len(events), len(content), core.Digest(content))
		if err != nil {
			return core.Result{}, err
		}
		if len(events) > 0 {
			rows := make([][]any, len(events))
			for i, e := range events {
				rows[i] = []any{c.Actor.OrgID, id, i + 1, e.ID}
			}
			if _, err = tx.CopyFrom(ctx, pgx.Identifier{"audit_export_events"}, []string{"org_id", "export_id", "position", "event_id"}, pgx.CopyFromRows(rows)); err != nil {
				return core.Result{}, err
			}
		}
		receipt, err := exportInTx(ctx, tx, c.Actor, id)
		return core.Result{Status: 201, Data: receipt, Event: core.Event{Type: "audit.export.created", ResourceType: "audit_export", ResourceID: id, Version: 1, Payload: map[string]any{"reason": in.Reason, "row_count": len(events), "csv_sha256": receipt.CSVSHA256}}}, err
	})
	if err == nil && out.Replayed {
		raw, e := json.Marshal(out.Data)
		if e != nil {
			return core.Result{}, e
		}
		var old Export
		if e = json.Unmarshal(raw, &old); e != nil {
			return core.Result{}, e
		}
		out.Data, err = s.Export(ctx, c.Actor, old.ID)
	}
	return out, err
}

func exportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Export, error) {
	var out Export
	var raw []byte
	var av, ov int64
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	err := tx.QueryRow(ctx, `SELECT id,filters,disclosure,created_at,expires_at,row_count,csv_bytes,csv_sha256,auth_version,authorization_version FROM audit_exports WHERE org_id=$1 AND id=$2 AND requester_id=$3`, a.OrgID, id, a.UserID).Scan(&out.ID, &raw, &out.Disclosure, &out.CreatedAt, &out.ExpiresAt, &out.RowCount, &out.CSVBytes, &out.CSVSHA256, &av, &ov)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(raw, &out.Filters); err != nil {
		return out, err
	}
	out.State = "ready"
	if !out.ExpiresAt.After(time.Now()) {
		out.State = "expired"
	} else if av != a.AuthVersion || ov != a.OrgAuthVersion {
		out.State = "authorization_changed"
	}
	out.CanDownload = out.State == "ready"
	return out, nil
}

func (s *Service) Export(ctx context.Context, a core.Actor, id string) (Export, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, true, false); err != nil {
		return Export{}, err
	}
	out, err := exportInTx(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) Exports(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[Export], error) {
	out := core.Page[Export]{Items: []Export{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100项")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, true, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "audit-exports")
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
	rows, err := tx.Query(ctx, `SELECT id FROM audit_exports WHERE org_id=$1 AND requester_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, a.UserID, before, beforeID, limit+1)
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
	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	for _, id := range ids {
		e, err := exportInTx(ctx, tx, a, id)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	if hasMore {
		last := out.Items[len(out.Items)-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, tx.Commit(ctx)
}

// ValidateDownload is deliberately lightweight, including before each chunk of
// a streamed response. It never treats an old in-memory Actor as live authority.
func (s *Service) ValidateDownload(ctx context.Context, a core.Actor, id string) (Export, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, true, true); err != nil {
		return Export{}, err
	}
	out, err := exportInTx(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	if !out.CanDownload {
		return out, core.Fail(409, "AUDIT_EXPORT_UNAVAILABLE", "导出已过期或权限依据已变化，请重新生成")
	}
	return out, tx.Commit(ctx)
}

func (s *Service) Download(ctx context.Context, a core.Actor, id, requestID string) (Download, error) {
	var out Download
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = authority(ctx, tx, a, true, true); err != nil {
		return out, err
	}
	e, err := exportInTx(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	if !e.CanDownload {
		return out, core.Fail(409, "AUDIT_EXPORT_UNAVAILABLE", "导出已过期或权限依据已变化，请重新生成")
	}
	rows, err := tx.Query(ctx, `SELECT `+eventColumns+` FROM audit_export_events x JOIN audit_events e ON e.org_id=x.org_id AND e.id=x.event_id WHERE x.org_id=$1 AND x.export_id=$2 ORDER BY x.position LIMIT $3`, a.OrgID, id, MaxExportRows+1)
	if err != nil {
		return out, err
	}
	events := []Event{}
	for rows.Next() {
		v, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		events = append(events, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	content, err := renderCSV(events)
	if err != nil {
		return out, err
	}
	if len(events) != e.RowCount || len(content) != e.CSVBytes || core.Digest(content) != e.CSVSHA256 {
		return out, core.Fail(409, "AUDIT_EXPORT_INTEGRITY", "固定导出完整性校验失败，未提供文件")
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,session_id) VALUES($1,$2,$3,$3,$4,$5,'audit.export.read','audit_export',$6,1,$7,'succeeded',$8)`, core.NewID(), a.OrgID, a.UserID, a.Channel, requestID, id, e.CSVSHA256, a.SessionID)
	if err != nil {
		return out, err
	}
	out = Download{ExportID: id, Filename: "audit-" + id + ".csv", TotalBytes: strconv.Itoa(len(content)), SHA256: e.CSVSHA256, Content: content}
	return out, tx.Commit(ctx)
}
