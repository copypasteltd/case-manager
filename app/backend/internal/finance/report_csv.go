package finance

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Amounts are validated database decimal strings, never evaluated formulas.
// Untrusted text cells are prefixed before CSV quoting to prevent spreadsheet
// formula execution, including formulas hidden behind whitespace/control text.
func csvText(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.ContainsAny(value, "\t\r") {
		return "'" + value
	}
	return value
}

func reportCSV(snapshot reportSnapshot) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&output)
	w.UseCRLF = true
	header := []string{"section", "report_kind", "source_type", "source_id", "resource_id", "revision_id", "version", "period_start", "period_end", "title", "state", "entry_kind", "currency", "original_amount", "contribution_amount", "category", "supplier_id", "owner_id", "project_id", "created_at", "metadata_key", "metadata_value", "timezone"}
	if snapshot.Report.Valuation != nil {
		header = append(header, "base_currency", "fx_status", "fx_date", "base_amount", "fx_rate", "fx_rate_index", "fx_rate_date", "fx_rate_source", "fx_valid_from", "fx_valid_to")
	}
	if snapshot.Report.Filters.Kind == "amortization" {
		header = append(header, "amortization_plan_id", "amortization_plan_version", "amortization_plan_sha256", "amortization_sequence", "amortization_method", "service_from", "service_to", "original_recognition_date")
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	metadata := func(key, value string) error {
		row := make([]string, len(header))
		row[0], row[1], row[20], row[21] = "metadata", snapshot.Report.Filters.Kind, key, csvText(value)
		return w.Write(row)
	}
	filters, _ := json.Marshal(snapshot.Report.Filters)
	filters, _ = core.CanonicalJSON(filters)
	summary, err := core.CanonicalJSON(snapshot.Report.Summary)
	if err != nil {
		return nil, err
	}
	for _, item := range [][2]string{{"protocol", "1"}, {"scope", snapshot.Report.Scope}, {"filters", string(filters)}, {"summary", string(summary)}, {"amount_encoding", "decimal strings; import amount columns as text"}, {"source_rows", strconv.Itoa(len(snapshot.Report.Items))}} {
		if err = metadata(item[0], item[1]); err != nil {
			return nil, err
		}
	}
	if snapshot.Report.Valuation != nil {
		b, e := json.Marshal(snapshot.Report.Valuation)
		if e != nil {
			return nil, e
		}
		b, e = core.CanonicalJSON(b)
		if e != nil {
			return nil, e
		}
		if e = metadata("valuation", string(b)); e != nil {
			return nil, e
		}
	}
	for _, v := range snapshot.Report.Items {
		row := []string{"source", snapshot.Report.Filters.Kind, v.SourceType, v.ID, v.ResourceID, v.RevisionID, strconv.FormatInt(v.Version, 10), v.From, value(v.To), csvText(v.Title), v.State, v.EntryKind, value(v.Currency), value(v.OriginalAmount), value(v.ContributionAmount), csvText(v.Category), value(v.SupplierID), v.OwnerID, value(v.ProjectID), v.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), "", "", v.Timezone}
		if snapshot.Report.Valuation != nil {
			fx := v.Valuation
			if fx == nil {
				return nil, core.Fail(500, "FX_SOURCE_MISSING", "报表折算来源缺失")
			}
			index := ""
			if fx.RateIndex != nil {
				index = strconv.Itoa(*fx.RateIndex)
			}
			row = append(row, snapshot.Report.Valuation.Book.BaseCurrency, fx.Status, fx.FXDate, value(fx.BaseAmount), value(fx.Rate), index, value(fx.RateDate), csvText(value(fx.RateSource)), value(fx.ValidFrom), value(fx.ValidTo))
		}
		if snapshot.Report.Filters.Kind == "amortization" {
			plan := v.Amortization
			if plan == nil {
				return nil, core.Fail(500, "AMORTIZATION_SOURCE_MISSING", "摊销导出缺少准确计划版本")
			}
			row = append(row, plan.PlanID, strconv.FormatInt(plan.PlanVersion, 10), plan.PlanSHA256, strconv.Itoa(plan.Sequence), plan.Method, plan.ServiceFrom, plan.ServiceTo, plan.OriginalDate)
		}
		if err = w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return output.Bytes(), w.Error()
}

const reportSnapshotReadable = ` NOT EXISTS(SELECT 1 FROM jsonb_array_elements(fs.payload->'references') ref WHERE NOT COALESCE(CASE
    WHEN ref->>'kind'='cost' THEN EXISTS(SELECT 1 FROM actual_cost_entries c WHERE c.org_id=$1 AND c.id=(ref->>'id')::uuid
        AND can_read_cost(c.org_id,$2,c.id,(ref->>'revision_id')::uuid,$3,false) AND can_access(c.org_id,$2,'report.read','cost',c.id,c.project_id,c.creator_id,c.classification,$3,false))
    WHEN ref->>'kind'='payment' THEN EXISTS(SELECT 1 FROM payment_records p WHERE p.org_id=$1 AND p.id=(ref->>'id')::uuid
        AND can_read_payment(p.org_id,$2,p.id,$3,false) AND can_access(p.org_id,$2,'report.read','payment',p.id,p.project_id,p.creator_id,p.classification,$3,false))
    WHEN ref->>'kind' IN ('subscription','subscription_period') THEN EXISTS(SELECT 1 FROM subscriptions s WHERE s.org_id=$1 AND s.id=(ref->>'id')::uuid
        AND can_read_subscription(s.org_id,$2,s.id,$3,false) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,false))
    ELSE false END,false)) `

func loadReportSnapshot(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (reportSnapshot, string, int64, error) {
	var snapshot reportSnapshot
	var raw []byte
	var snapshotSHA, csvSHA string
	var length int64
	err := tx.QueryRow(ctx, `SELECT fs.payload::text,fs.snapshot_sha256,fs.csv_sha256,fs.csv_bytes FROM finance_report_snapshots fs JOIN finance_report_exports e ON e.org_id=fs.org_id AND e.id=fs.export_id
        WHERE fs.org_id=$1 AND e.requester_id=$2 AND e.id=$4 AND e.expires_at>clock_timestamp() AND `+reportSnapshotReadable, a.OrgID, a.UserID, a.Channel, id).Scan(&raw, &snapshotSHA, &csvSHA, &length)
	if err == pgx.ErrNoRows {
		return snapshot, "", 0, core.Missing()
	}
	if err != nil {
		return snapshot, "", 0, err
	}
	if core.Digest(raw) != snapshotSHA || json.Unmarshal(raw, &snapshot) != nil || snapshot.Protocol != 1 || snapshot.Report.Scope != "authorized_records_only" {
		return snapshot, "", 0, core.Fail(500, "EXPORT_INTEGRITY_FAILED", "报表快照校验失败，请重新生成")
	}
	return snapshot, csvSHA, length, nil
}

type ReportDownload struct {
	ID       string `json:"export_id"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Bytes    string `json:"total_size_bytes"`
	Content  []byte `json:"-"`
}

func (s *Service) DownloadReportExport(ctx context.Context, a core.Actor, id string) (ReportDownload, error) {
	var out ReportDownload
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	f, err := reportExportOwner(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	snapshot, hash, length, err := loadReportSnapshot(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	content, err := reportCSV(snapshot)
	if err != nil {
		return out, err
	}
	if int64(len(content)) != length || core.Digest(content) != hash || len(content) > ReportExportMaxBytes {
		return out, core.Fail(500, "EXPORT_INTEGRITY_FAILED", "报表内容校验失败，请重新生成")
	}
	out = ReportDownload{ID: id, Filename: reportExportName(f.Kind, id), SHA256: hash, Bytes: strconv.Itoa(len(content)), Content: content}
	return out, tx.Commit(ctx)
}
