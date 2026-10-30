package files

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

type TextComparisonVersion struct {
	NativeEvidence *NativeTextEvidence `json:"native_evidence,omitempty"`
	VersionID      string              `json:"version_id"`
	Extractor      string              `json:"extractor"`
	Seq            int64               `json:"seq"`
	MediaType      string              `json:"media_type"`
	SizeBytes      string              `json:"size_bytes"`
	SourceSHA256   string              `json:"source_sha256"`
	TextSHA256     string              `json:"text_sha256"`
	TextCharacters int                 `json:"text_characters"`
	IndexedAt      time.Time           `json:"indexed_at"`
}

type TextComparison struct {
	FileID           string                `json:"file_id"`
	FileVersion      int64                 `json:"file_version"`
	Extractor        string                `json:"extractor"`
	Base             TextComparisonVersion `json:"base"`
	Target           TextComparisonVersion `json:"target"`
	SourceEqual      bool                  `json:"source_equal"`
	TextEqual        bool                  `json:"text_equal"`
	Algorithm        string                `json:"algorithm"`
	Complete         bool                  `json:"complete"`
	View             string                `json:"view"`
	BaseLines        int                   `json:"base_lines"`
	TargetLines      int                   `json:"target_lines"`
	AddedLines       int                   `json:"added_lines"`
	RemovedLines     int                   `json:"removed_lines"`
	UnchangedLines   int                   `json:"unchanged_lines"`
	ComparisonSHA256 string                `json:"comparison_sha256"`
	Offset           int                   `json:"offset"`
	BaseOffset       int                   `json:"base_offset"`
	TargetOffset     int                   `json:"target_offset"`
	TotalRows        int                   `json:"total_rows"`
	Rows             []TextComparisonRow   `json:"rows"`
	NextOffset       *int                  `json:"next_offset"`
	AsOf             time.Time             `json:"as_of"`
}

type TextComparisonInput struct {
	BaseVersionID, TargetVersionID, View, ComparisonSHA256 string
	Offset, Limit                                          int
}

func comparisonBody(ctx context.Context, tx pgx.Tx, a core.Actor, fileID, versionID string) (TextComparisonVersion, string, error) {
	out := TextComparisonVersion{VersionID: versionID}
	v, err := downloadVersion(ctx, tx, a, versionID, "attachment")
	if err != nil {
		return out, "", err
	}
	if v.File.ID != fileID {
		return out, "", core.Missing()
	}
	if v.ScanState != "clean" {
		return out, "", core.Fail(422, "FILE_COMPARISON_SCAN_REQUIRED", "比较要求两个准确版本均扫描通过；人工放行不能代替正文检查")
	}
	out.Extractor = TextExtractorFor(v.MediaType)
	var state, body string
	var hash *string
	var count *int
	var indexed *time.Time
	var engine *string
	var pages, ocr *int
	err = tx.QueryRow(ctx, `SELECT state,coalesce(body,''),text_sha256,character_count,indexed_at,engine_sha256,page_count,ocr_page_count
 FROM file_text_derivations WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4 AND source_sha256=$5`,
		a.OrgID, fileID, versionID, out.Extractor, v.SHA256).Scan(&state, &body, &hash, &count, &indexed, &engine, &pages, &ocr)
	if err == pgx.ErrNoRows || err == nil && state == "queued" {
		return out, "", core.Fail(409, "FILE_COMPARISON_TEXT_PENDING", "所选版本正文尚未完成提取，请稍后重新核对")
	}
	if err != nil {
		return out, "", err
	}
	if state != "ready" || hash == nil || count == nil || indexed == nil {
		return out, "", core.Fail(422, "FILE_COMPARISON_TEXT_UNAVAILABLE", "所选版本没有完整可用的提取正文，请查阅格式与提取状态")
	}
	if out.NativeEvidence, err = nativeTextEvidence(out.Extractor, state, engine, pages, ocr); err != nil {
		return out, "", err
	}
	if len(body) > TextOutputMaxBytes || !utf8.ValidString(body) || strings.IndexByte(body, 0) >= 0 || core.Digest([]byte(body)) != *hash || utf8.RuneCountInString(body) != *count {
		return out, "", core.Fail(502, "FILE_COMPARISON_TEXT_INVALID", "完整正文摘要或字符总数核对失败，没有返回部分差异")
	}
	if err = tx.QueryRow(ctx, "SELECT seq,size_bytes::text FROM file_versions WHERE org_id=$1 AND id=$2", a.OrgID, versionID).Scan(&out.Seq, &out.SizeBytes); err != nil {
		return out, "", err
	}
	out.MediaType, out.SourceSHA256, out.TextSHA256, out.TextCharacters, out.IndexedAt = v.MediaType, v.SHA256, *hash, *count, *indexed
	return out, body, nil
}

// Comparison is a read: it never writes a version, schedules extraction, issues
// a download permit or gives an AI actor interactive human authority.
func (s *Service) CompareText(ctx context.Context, a core.Actor, fileID string, in TextComparisonInput) (TextComparison, error) {
	out := TextComparison{FileID: fileID, Extractor: TextExtractor, View: in.View, Offset: in.Offset, Rows: []TextComparisonRow{}}
	if !core.ValidID(fileID) || !core.ValidID(in.BaseVersionID) || !core.ValidID(in.TargetVersionID) {
		return out, core.Invalid("version_id", "请选择同一文件的两个准确正式内容版本")
	}
	if in.View != "all" && in.View != "changes" || in.Offset < 0 || in.Offset > TextComparisonMaxLines*2 || in.Limit < 1 || in.Limit > 100 {
		return out, core.Invalid("comparison", "比较范围或分页参数无效，每页最多100行")
	}
	if in.Offset > 0 && in.ComparisonSHA256 == "" || in.ComparisonSHA256 != "" && !shaPattern.MatchString(in.ComparisonSHA256) {
		return out, core.Invalid("comparison_sha256", "继续查阅须固定首次比较摘要")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	f, err := file(ctx, tx, a.OrgID, fileID, false)
	if err != nil {
		return out, err
	}
	if err = replicaNamespace(ctx, tx, a.OrgID, f.ProjectID); err != nil {
		return out, err
	}
	f, err = file(ctx, tx, a.OrgID, fileID, true)
	if err != nil {
		return out, err
	}
	ids := []string{in.BaseVersionID, in.TargetVersionID}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err = tx.Exec(ctx, "SELECT lock_file_text_version($1,$2,$3)", a.OrgID, fileID, id); err != nil {
			return out, err
		}
	}
	base, before, err := comparisonBody(ctx, tx, a, fileID, in.BaseVersionID)
	if err != nil {
		return out, err
	}
	target, after, err := comparisonBody(ctx, tx, a, fileID, in.TargetVersionID)
	if err != nil {
		return out, err
	}
	diff, err := compareTextLines(ctx, before, after, in.View)
	if err != nil {
		return out, err
	}
	out.FileVersion, out.Base, out.Target = f.Version, base, target
	out.Extractor = base.Extractor
	if target.Extractor != base.Extractor {
		out.Extractor = "mixed"
	}
	out.SourceEqual, out.TextEqual = base.SourceSHA256 == target.SourceSHA256, before == after
	out.Algorithm, out.Complete = diff.Algorithm, true
	out.BaseLines, out.TargetLines, out.AddedLines, out.RemovedLines, out.UnchangedLines = diff.BaseLines, diff.TargetLines, diff.Added, diff.Removed, diff.Unchanged
	out.TotalRows = len(diff.Rows)
	identity, _ := json.Marshal([]any{"file-text-comparison-v1", out.Extractor, a.OrgID, a.UserID, a.SessionID, a.AuthVersion, a.OrgAuthVersion, f.ID, f.Version, base, target, in.View, diff.Algorithm})
	out.ComparisonSHA256 = core.Digest(identity)
	if in.ComparisonSHA256 != "" && in.ComparisonSHA256 != out.ComparisonSHA256 {
		return TextComparison{}, core.Fail(409, "FILE_COMPARISON_CHANGED", "比较依据或当前授权已变化，请从第一页重新核对")
	}
	if in.Offset > len(diff.Rows) {
		return TextComparison{}, core.Invalid("offset", "比较位置超过所选版本结果")
	}
	end := min(len(diff.Rows), in.Offset+in.Limit)
	for _, row := range diff.Rows[:in.Offset] {
		if row.BaseLine != nil {
			out.BaseOffset += row.Count
		}
		if row.TargetLine != nil {
			out.TargetOffset += row.Count
		}
	}
	out.Rows = diff.Rows[in.Offset:end]
	if end < len(diff.Rows) {
		out.NextOffset = &end
	}
	out.AsOf = time.Now().UTC()
	return out, tx.Commit(ctx)
}
