package files

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
)

const AgentTextMaxBytes int64 = 1 << 20

// Agent text reads use an explicit streaming identity. No signer or writer is
// needed, and this backend is never passed to the model process.
type AgentTextBackend struct {
	Provider, Bucket string
	Reader           DownloadReader
}
type AgentTextService struct{ Backends map[string]*AgentTextBackend }

// AgentTextSource is opaque outside the trusted Go layer. Its fingerprint binds
// the exact verified replica, while the public citation binds the formal version.
type AgentTextSource struct {
	FileID, VersionID, SHA256, MediaType, Fingerprint string
	Size                                              int64
	source                                            objectstore.Source
	reader                                            DownloadReader
}

func AgentTextFormat(mediaType string, size int64) error {
	base, params, err := mime.ParseMediaType(mediaType)
	if err != nil || !strings.EqualFold(params["charset"], "utf-8") && params["charset"] != "" ||
		(base != "text/plain" && base != "text/markdown" && base != "text/csv" && base != "application/json") {
		return core.Fail(422, "AGENT_TEXT_FORMAT_UNSUPPORTED", "当前秘书正文读取支持UTF-8文本、Markdown、CSV及JSON；此格式需要独立适配")
	}
	if size < 1 || size > AgentTextMaxBytes {
		return core.Fail(422, "AGENT_TEXT_SIZE_UNSUPPORTED", "当前秘书正文读取支持1字节至1MiB的文本版本，请提供有界文本资料或使用人工文件阅读入口")
	}
	return nil
}

func (s *AgentTextService) Prepare(ctx context.Context, db platform.DBTX, a core.Actor, fileID, versionID string) (AgentTextSource, error) {
	var out AgentTextSource
	if a.RunID == "" {
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
		return out, core.Fail(422, "AGENT_TEXT_SCAN_REQUIRED", "秘书正文处理要求此准确版本的扫描结果为通过；人工放行不能代替正文处理检查")
	}
	if err = AgentTextFormat(v.MediaType, v.Size); err != nil {
		return out, err
	}
	if s == nil {
		return out, core.Fail(503, "AGENT_TEXT_READER_UNAVAILABLE", "秘书正文读取身份尚未配置")
	}
	rows, err := db.Query(ctx, `SELECT id FROM file_replicas WHERE org_id=$1 AND version_id=$2 AND state='verified'
 AND expected_sha256=$3 AND actual_sha256=$3 AND verified_at IS NOT NULL AND source_id IS NOT NULL ORDER BY provider,id`, a.OrgID, versionID, v.SHA256)
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
	for _, id := range ids {
		r, e := replica(ctx, db, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		backend := s.Backends[r.Provider]
		if backend == nil || backend.Provider != r.Provider || backend.Bucket != r.Bucket || backend.Reader == nil {
			continue
		}
		var enabled bool
		if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1 AND provider=$2 AND bucket=$3 AND enabled)", a.OrgID, r.Provider, r.Bucket).Scan(&enabled); e != nil {
			return out, e
		}
		var source objectstore.Source
		if !enabled || !downloadSourceEvidence(a.OrgID, v, r, &source) {
			continue
		}
		identity, _ := json.Marshal([]any{v.File.ID, v.ID, v.SHA256, v.MediaType, v.Size, r.ID, r.Version, r.Provider, r.Bucket, source})
		return AgentTextSource{FileID: v.File.ID, VersionID: v.ID, SHA256: v.SHA256, MediaType: v.MediaType, Size: v.Size, Fingerprint: core.Digest(identity), source: source, reader: backend.Reader}, nil
	}
	return out, core.Fail(503, "AGENT_TEXT_SOURCE_UNAVAILABLE", "没有可供秘书读取且具有完整校验证据的文件副本")
}

// Read performs no SQL. Callers release authorization locks before this bounded
// transfer and must revalidate the current audience and exact source afterwards.
func (s *AgentTextService) Read(ctx context.Context, source AgentTextSource) (string, error) {
	if source.reader == nil || source.Fingerprint == "" || source.Size < 1 || source.Size > AgentTextMaxBytes || source.source.Size != source.Size {
		return "", core.Forbidden()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := source.reader.Read(ctx, source.source, 0, source.Size)
	if err != nil {
		return "", core.Fail(503, "AGENT_TEXT_READ_FAILED", "准确文件版本暂时无法读取，请稍后重新核对")
	}
	if result.Body == nil {
		return "", core.Fail(502, "AGENT_TEXT_CONTENT_INVALID", "文件正文读取证据不完整")
	}
	defer result.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = result.Body.Close() })
	defer stop()
	if result.Size != source.Size || result.ETag != source.source.ETag || source.source.VersionID != "" && result.VersionID != source.source.VersionID {
		return "", core.Fail(502, "AGENT_TEXT_CONTENT_INVALID", "文件正文与准确来源不一致")
	}
	body, err := io.ReadAll(io.LimitReader(result.Body, source.Size+1))
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil || int64(len(body)) != source.Size || core.Digest(body) != source.SHA256 {
		return "", core.Fail(502, "AGENT_TEXT_CONTENT_INVALID", "文件正文长度或完整摘要核验失败")
	}
	if !utf8.Valid(body) || strings.IndexByte(string(body), 0) >= 0 {
		return "", core.Fail(422, "AGENT_TEXT_ENCODING_UNSUPPORTED", "文件正文不是可支持的UTF-8文本，请保留原件并提供明确文本版本")
	}
	return string(body), nil
}
