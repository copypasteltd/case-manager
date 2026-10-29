package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const ReportExportKind = "finance.report_export.v1"
const ReportExportMaxRows = 50000
const ReportExportMaxBytes = 16 << 20

type ReportExportCreate struct {
	Filters ReportQuery `json:"filters"`
	Reason  string      `json:"reason"`
}
type ReportExport struct {
	ID          string      `json:"id"`
	JobID       string      `json:"job_id"`
	Filters     ReportQuery `json:"filters"`
	State       string      `json:"state"`
	Attempt     int         `json:"attempt"`
	ErrorCode   *string     `json:"error_code"`
	CreatedAt   time.Time   `json:"created_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
	AsOf        *time.Time  `json:"as_of"`
	RowCount    *int        `json:"row_count"`
	CSVBytes    *int64      `json:"csv_bytes"`
	CSVSHA256   *string     `json:"csv_sha256"`
	CanCancel   bool        `json:"can_cancel"`
	CanDownload bool        `json:"can_download"`
}
type reportReference struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	RevisionID string `json:"revision_id"`
}
type reportSnapshot struct {
	Protocol   int               `json:"protocol"`
	Report     ReportPage        `json:"report"`
	References []reportReference `json:"references"`
}

func (s *Service) CreateReportExport(ctx context.Context, c core.Command, in ReportExportCreate) (core.Result, error) {
	if err := in.Filters.Validate(); err != nil {
		return core.Result{}, err
	}
	if len([]rune(in.Reason)) < 1 || len([]rune(in.Reason)) > 2000 || c.Actor.RunID != "" {
		return core.Result{}, core.Invalid("reason", "由员工明确填写1至2000字的导出用途")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := reportAuthority(ctx, tx, c.Actor, in.Filters, true); err != nil {
			return err
		}
		if in.Filters.FXBookID != nil {
			_, err := readFXBook(ctx, tx, c.Actor.OrgID, *in.Filters.FXBookID, false)
			return err
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		id := core.NewID()
		payload, _ := json.Marshal(map[string]string{"export_id": id})
		job, _, err := (jobqueue.Queue{DB: s.DB}).Enqueue(ctx, tx, jobqueue.Input{OrgID: c.Actor.OrgID, Kind: ReportExportKind, Lane: "reports", DedupKey: "finance-report-export/" + id, PayloadVersion: 1, Payload: payload, MaxAttempts: 4})
		if err != nil {
			return core.Result{}, err
		}
		filters, _ := json.Marshal(in.Filters)
		_, err = tx.Exec(ctx, `INSERT INTO finance_report_exports(id,org_id,requester_id,channel,auth_version,authorization_version,filters,job_id,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.Actor.OrgID, c.Actor.UserID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, filters, job, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		out, err := reportExportInTx(ctx, tx, c.Actor, id, false)
		return core.Result{Status: 202, Data: out, Event: core.Event{Type: "finance.report_export.queued", ResourceType: "finance_report_export", ResourceID: id, Version: 1, Payload: map[string]any{"reason": in.Reason, "job_id": job}}}, err
	})
}

func reportExportOwner(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (ReportQuery, error) {
	var f ReportQuery
	if !core.ValidID(id) || a.RunID != "" {
		return f, core.Missing()
	}
	var raw []byte
	var version, auth int64
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT filters,auth_version,authorization_version,expires_at FROM finance_report_exports WHERE org_id=$1 AND id=$2 AND requester_id=$3`, a.OrgID, id, a.UserID).Scan(&raw, &version, &auth, &expires)
	if err != nil {
		return f, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		return f, err
	}
	if err = reportAuthority(ctx, tx, a, f, true); err != nil {
		return f, err
	}
	if version != a.AuthVersion || auth != a.OrgAuthVersion {
		return f, core.Fail(409, "EXPORT_AUTHORIZATION_CHANGED", "生成后权限依据已变化，请重新生成；旧文件不再提供下载")
	}
	return f, nil
}

func reportExportInTx(ctx context.Context, tx pgx.Tx, a core.Actor, id string, check bool) (ReportExport, error) {
	var out ReportExport
	if check {
		if _, err := reportExportOwner(ctx, tx, a, id); err != nil {
			return out, err
		}
	}
	var filters []byte
	err := tx.QueryRow(ctx, `SELECT e.id,e.job_id,e.filters,CASE WHEN e.expires_at<=clock_timestamp() THEN 'expired' ELSE j.state END,j.attempt,j.last_error,e.created_at,e.expires_at,
        (s.payload->'report'->'summary'->>'as_of')::timestamptz,s.row_count,s.csv_bytes,s.csv_sha256
        FROM finance_report_exports e JOIN jobs j ON j.org_id=e.org_id AND j.id=e.job_id LEFT JOIN finance_report_snapshots s ON s.org_id=e.org_id AND s.export_id=e.id
        WHERE e.org_id=$1 AND e.id=$2 AND e.requester_id=$3`, a.OrgID, id, a.UserID).Scan(&out.ID, &out.JobID, &filters, &out.State, &out.Attempt, &out.ErrorCode, &out.CreatedAt, &out.ExpiresAt, &out.AsOf, &out.RowCount, &out.CSVBytes, &out.CSVSHA256)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(filters, &out.Filters); err != nil {
		return out, err
	}
	if out.State == "expired" {
		out.AsOf, out.RowCount, out.CSVBytes, out.CSVSHA256 = nil, nil, nil, nil
	}
	out.CanCancel = out.State == "queued" || out.State == "running" || out.State == "retry_wait"
	out.CanDownload = out.State == "succeeded" && out.CSVSHA256 != nil
	return out, nil
}

func (s *Service) ReportExport(ctx context.Context, a core.Actor, id string) (ReportExport, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return ReportExport{}, err
	}
	defer tx.Rollback(ctx)
	out, err := reportExportInTx(ctx, tx, a, id, true)
	if err == nil && out.CanDownload {
		_, _, _, err = loadReportSnapshot(ctx, tx, a, id)
	}
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) ProcessReportExport(ctx context.Context, j jobqueue.Job) error {
	var input struct {
		ExportID string `json:"export_id"`
	}
	if j.Kind != ReportExportKind || j.Lane != "reports" || j.PayloadVersion != 1 || json.Unmarshal(j.Payload, &input) != nil || !core.ValidID(input.ExportID) || j.DedupKey != "finance-report-export/"+input.ExportID {
		return core.Invalid("job", "导出任务固定输入无效")
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var actor core.Actor
	var filters []byte
	var expired bool
	actor.OrgID = j.OrgID
	if err = tx.QueryRow(ctx, `SELECT requester_id,channel,auth_version,authorization_version,filters,expires_at<=clock_timestamp() FROM finance_report_exports WHERE org_id=$1 AND id=$2`, j.OrgID, input.ExportID).Scan(&actor.UserID, &actor.Channel, &actor.AuthVersion, &actor.OrgAuthVersion, &filters, &expired); err != nil {
		return platform.DatabaseError(err)
	}
	var f ReportQuery
	if err = json.Unmarshal(filters, &f); err != nil {
		return err
	}
	if expired {
		return core.Fail(410, "EXPORT_EXPIRED", "导出已超过保留期限")
	}
	if err = reportAuthority(ctx, tx, actor, f, true); err != nil {
		return err
	}
	if _, err = reportExportOwner(ctx, tx, actor, input.ExportID); err != nil {
		return err
	}
	queue := jobqueue.Queue{DB: s.DB}
	if err = queue.CheckInTx(ctx, tx, j); err != nil {
		return err
	}
	snapshot := reportSnapshot{Protocol: 1, Report: ReportPage{Filters: f, Scope: "authorized_records_only"}, References: []reportReference{}}
	if snapshot.Report.Summary, err = reportSummary(ctx, tx, actor, f); err != nil {
		return err
	}
	if snapshot.Report.Items, err = reportSources(ctx, tx, actor, f, nil, nil, ReportExportMaxRows+1); err != nil {
		return err
	}
	if len(snapshot.Report.Items) > ReportExportMaxRows {
		return core.Fail(422, "EXPORT_SCOPE_TOO_LARGE", "导出超过50000条，请缩小期间或筛选；不会生成截断文件")
	}
	if err = applyReportValuation(ctx, tx, actor, &snapshot.Report); err != nil {
		return err
	}
	for _, row := range snapshot.Report.Items {
		kind := row.SourceType
		if kind == "cost_amortization" {
			kind = "cost"
		}
		snapshot.References = append(snapshot.References, reportReference{Kind: kind, ID: row.ResourceID, RevisionID: row.RevisionID})
	}
	if f.Kind == "forecast" {
		if err = reportCoverageReferences(ctx, tx, actor, f, &snapshot); err != nil {
			return err
		}
	}
	content, err := reportCSV(snapshot)
	if err != nil {
		return err
	}
	if len(content) > ReportExportMaxBytes {
		return core.Fail(422, "EXPORT_SCOPE_TOO_LARGE", "CSV超过16MiB，请缩小期间或筛选；不会生成截断文件")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > 30<<20 {
		return core.Fail(422, "EXPORT_SCOPE_TOO_LARGE", "快照超过保留范围，请缩小筛选")
	}
	_, err = tx.Exec(ctx, `INSERT INTO finance_report_snapshots(org_id,export_id,job_fencing_token,payload,snapshot_sha256,csv_sha256,csv_bytes,row_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, j.OrgID, input.ExportID, j.FencingToken, raw, core.Digest(raw), core.Digest(content), len(content), len(snapshot.Report.Items))
	if err != nil {
		return err
	}
	if err = queue.CompleteInTx(ctx, tx, j); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,channel,request_id,job_id,action,resource_type,resource_id,resource_version,after_digest,result,reason) VALUES($1,$2,$3,'system',$4,$5,'finance.report_export.generated','finance_report_export',$6,1,$7,'succeeded','固定授权范围，限期报表快照')`, core.NewID(), j.OrgID, actor.UserID, "job/"+j.ID, j.ID, input.ExportID, core.Digest(content))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id) VALUES($1,$2,'finance.report_export.generated','finance_report_export',$3,1,$4,$5)`, core.NewID(), j.OrgID, input.ExportID, j.Payload, "job/"+j.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func reportCoverageReferences(ctx context.Context, tx pgx.Tx, a core.Actor, f ReportQuery, snapshot *reportSnapshot) error {
	var report ForecastReport
	if err := json.Unmarshal(snapshot.Report.Summary, &report); err != nil {
		return err
	}
	if report.CoverageLimitReached {
		return core.Fail(422, "EXPORT_SCOPE_TOO_LARGE", "预测完整性核查超过1000个订阅，请缩小筛选")
	}
	rows, err := tx.Query(ctx, "SELECT s.id,s.current_revision_id"+subscriptionJoin+forecastReportScope+" ORDER BY s.id LIMIT 1001", a.OrgID, a.UserID, a.Channel, false, f.Category, f.ProjectID, f.CompanyOnly, f.SupplierID, f.OwnerID, f.Query)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var ref reportReference
		ref.Kind = "subscription"
		if err = rows.Scan(&ref.ID, &ref.RevisionID); err != nil {
			return err
		}
		n++
		if n > 1000 {
			return core.Fail(422, "EXPORT_SCOPE_TOO_LARGE", "预测来源超过1000个订阅，请缩小筛选")
		}
		snapshot.References = append(snapshot.References, ref)
	}
	return rows.Err()
}

func (s *Service) CancelReportExport(ctx context.Context, c core.Command, id, reason string) (core.Result, error) {
	if reason == "" || len([]rune(reason)) > 2000 {
		return core.Result{}, core.Invalid("reason", "请填写取消原因")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		_, err := reportExportOwner(ctx, tx, c.Actor, id)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		tag, err := tx.Exec(ctx, `UPDATE jobs SET state='cancelled',lease_owner=NULL,lease_until=NULL,fencing_token=fencing_token+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=(SELECT job_id FROM finance_report_exports WHERE org_id=$1 AND id=$2 AND requester_id=$3) AND state IN ('queued','retry_wait','running')`, c.Actor.OrgID, id, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Fail(409, "EXPORT_NOT_CANCELLABLE", "导出已完成、取消或失败，请读取当前状态")
		}
		out, err := reportExportInTx(ctx, tx, c.Actor, id, false)
		return core.Result{Data: out, Event: core.Event{Type: "finance.report_export.cancelled", ResourceType: "finance_report_export", ResourceID: id, Version: 2, Payload: map[string]any{"reason": reason}}}, err
	})
}

// Retain the request/audit trail; delete only expired derived snapshot payloads.
func (s *Service) PurgeReportSnapshots(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM finance_report_snapshots WHERE (org_id,export_id) IN (SELECT s.org_id,s.export_id FROM finance_report_snapshots s JOIN finance_report_exports e ON e.org_id=s.org_id AND e.id=s.export_id WHERE e.expires_at<=clock_timestamp() ORDER BY e.expires_at LIMIT 100)`)
	return tag.RowsAffected(), err
}

func reportExportName(kind, id string) string { return fmt.Sprintf("finance-%s-%s.csv", kind, id) }
