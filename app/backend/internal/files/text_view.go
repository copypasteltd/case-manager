package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
	"time"
)

type TextView struct {
	FileID          string              `json:"file_id"`
	VersionID       string              `json:"version_id"`
	Extractor       string              `json:"extractor"`
	State           string              `json:"state"`
	SourceSHA256    string              `json:"source_sha256"`
	TextSHA256      *string             `json:"text_sha256"`
	IndexedAt       *time.Time          `json:"indexed_at"`
	ErrorCode       *string             `json:"error_code"`
	TotalCharacters *int                `json:"total_characters"`
	Offset          int                 `json:"offset"`
	Text            string              `json:"text"`
	NextOffset      *int                `json:"next_offset"`
	NativeEvidence  *NativeTextEvidence `json:"native_evidence,omitempty"`
}

func (s *Service) Text(ctx context.Context, a core.Actor, fileID, versionID string, offset, limit int) (TextView, error) {
	out := TextView{FileID: fileID, VersionID: versionID, Extractor: TextExtractor, State: "pending", Offset: offset}
	if !core.ValidID(fileID) || !core.ValidID(versionID) {
		return out, core.Missing()
	}
	if offset < 0 || offset > TextOutputMaxBytes || limit < 1 || limit > 8000 {
		return out, core.Invalid("offset", "正文位置需有效，每次最多读取8000个字符")
	}
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
	if _, err = file(ctx, tx, a.OrgID, fileID, true); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "SELECT lock_file_text_version($1,$2,$3)", a.OrgID, fileID, versionID); err != nil {
		return out, err
	}
	v, err := downloadVersion(ctx, tx, a, versionID, "attachment")
	if err != nil {
		return out, err
	}
	if v.File.ID != fileID {
		return out, core.Missing()
	}
	out.SourceSHA256 = v.SHA256
	out.Extractor = TextExtractorFor(v.MediaType)
	if v.ScanState != "clean" {
		out.State = "blocked"
		code := "FILE_TEXT_SCAN_REQUIRED"
		out.ErrorCode = &code
		return out, tx.Commit(ctx)
	}
	var body string
	var engine *string
	var pages, ocr *int
	err = tx.QueryRow(ctx, `SELECT state,text_sha256,indexed_at,error_code,character_count,
        coalesce(substring(body FROM $5::int FOR $6::int),''),engine_sha256,page_count,ocr_page_count FROM file_text_derivations
        WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4 AND source_sha256=$7`,
		a.OrgID, fileID, versionID, out.Extractor, offset+1, limit, v.SHA256).
		Scan(&out.State, &out.TextSHA256, &out.IndexedAt, &out.ErrorCode, &out.TotalCharacters, &body, &engine, &pages, &ocr)
	if err != nil && err != pgx.ErrNoRows {
		return out, err
	}
	if out.State == "queued" {
		out.State = "pending"
	}
	if out.NativeEvidence, err = nativeTextEvidence(out.Extractor, out.State, engine, pages, ocr); err != nil {
		return out, err
	}
	if out.State == "ready" && out.TotalCharacters != nil {
		if offset > *out.TotalCharacters {
			return out, core.Invalid("offset", "正文位置超过此固定版本的字符总数")
		}
		out.Text = body
		next := offset + len([]rune(body))
		if next < *out.TotalCharacters {
			out.NextOffset = &next
		}
	}
	return out, tx.Commit(ctx)
}
