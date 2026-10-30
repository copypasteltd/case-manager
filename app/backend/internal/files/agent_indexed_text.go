package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// IndexedTextEvidence identifies immutable derived text, independently of the
// formal original. It contains neither plaintext nor cloud locations.
type IndexedTextEvidence struct {
	Extractor       string     `json:"extractor"`
	State           string     `json:"state"`
	SourceSHA256    string     `json:"source_sha256"`
	TextSHA256      *string    `json:"text_sha256"`
	IndexedAt       *time.Time `json:"indexed_at"`
	TotalCharacters *int       `json:"total_characters"`
	ErrorCode       *string    `json:"error_code"`
}

// AgentTextIndex requires an internal AI actor. The conversation layer also
// checks every current participant, classification and exact file dependency.
func AgentTextIndex(ctx context.Context, db platform.DBTX, a core.Actor, fileID, versionID string) (IndexedTextEvidence, error) {
	out := IndexedTextEvidence{Extractor: TextExtractor, State: "pending"}
	if a.RunID == "" || !core.ValidID(fileID) || !core.ValidID(versionID) {
		return out, core.Forbidden()
	}
	v, err := downloadVersion(ctx, db, a, versionID, "attachment")
	if err != nil {
		return out, err
	}
	if v.File.ID != fileID {
		return out, core.Missing()
	}
	if v.ScanState != "clean" {
		return out, core.Fail(422, "AGENT_TEXT_SCAN_REQUIRED", "秘书正文处理要求此准确版本的扫描结果为通过")
	}
	out.SourceSHA256 = v.SHA256
	out.Extractor = TextExtractorFor(v.MediaType)
	err = db.QueryRow(ctx, `SELECT state,text_sha256,indexed_at,character_count,error_code
 FROM file_text_derivations WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4 AND source_sha256=$5`,
		a.OrgID, fileID, versionID, out.Extractor, v.SHA256).
		Scan(&out.State, &out.TextSHA256, &out.IndexedAt, &out.TotalCharacters, &out.ErrorCode)
	if err != nil && err != pgx.ErrNoRows {
		return out, err
	}
	if err == pgx.ErrNoRows {
		if _, formatErr := textFormat(v.MediaType, v.Size); formatErr != nil {
			out.State = "unsupported"
			code := core.PublicError(formatErr).Code
			out.ErrorCode = &code
		}
	}
	if out.State == "queued" {
		out.State = "pending"
	}
	return out, nil
}

// ReadAgentIndexedText never downloads or parses originals during model reads.
// A ready derivation is immutable; bounded SQL substring returns Unicode chars.
func ReadAgentIndexedText(ctx context.Context, db platform.DBTX, a core.Actor, fileID, versionID string, expected IndexedTextEvidence, offset, limit int) (string, *int, int, error) {
	if offset < 0 || offset > TextOutputMaxBytes || limit < 1 || limit > 10000 {
		return "", nil, 0, core.Invalid("offset", "请指定有效字符位置及最多10000字符的范围")
	}
	current, err := AgentTextIndex(ctx, db, a, fileID, versionID)
	if err != nil {
		return "", nil, 0, err
	}
	left, _ := json.Marshal(expected)
	right, _ := json.Marshal(current)
	if string(left) != string(right) {
		return "", nil, 0, core.Fail(409, "CONTEXT_SOURCE_CHANGED", "正文提取证据已变化，请重新选择准确版本")
	}
	if current.State != "ready" || current.TextSHA256 == nil || current.TotalCharacters == nil || current.IndexedAt == nil {
		if current.State == "pending" {
			return "", nil, 0, core.Fail(409, "AGENT_TEXT_INDEX_PENDING", "此准确版本的正文尚未完成提取，请稍后重试")
		}
		return "", nil, 0, core.Fail(422, "AGENT_TEXT_INDEX_UNAVAILABLE", "此版本没有可供秘书读取的已验证派生正文，请核对格式和提取状态")
	}
	total := *current.TotalCharacters
	if offset > total {
		return "", nil, 0, core.Invalid("offset", "字符位置超过此准确版本的正文")
	}
	var body string
	err = db.QueryRow(ctx, `SELECT substring(body FROM $7::int FOR $8::int) FROM file_text_derivations
 WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4 AND source_sha256=$5 AND text_sha256=$6 AND state='ready'`,
		a.OrgID, fileID, versionID, current.Extractor, current.SourceSHA256, *current.TextSHA256, offset+1, limit).Scan(&body)
	if err != nil {
		return "", nil, 0, err
	}
	var next *int
	end := offset + len([]rune(body))
	if end < total {
		next = &end
	}
	return body, next, total, nil
}
