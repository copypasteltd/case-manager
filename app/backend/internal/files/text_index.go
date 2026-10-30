package files

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/documenttext"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TextJobKind = "file.text.extract.v1"
const TextJobLane = "file-text"

type TextIndexService struct {
	DB *pgxpool.Pool
	// An independent read-only identity, never the human or agent credential.
	Backends map[string]*AgentTextBackend
	Native   *documenttext.Engine
}
type textJobInput struct {
	FileID       string `json:"file_id"`
	VersionID    string `json:"version_id"`
	Extractor    string `json:"extractor"`
	EngineSHA256 string `json:"engine_sha256,omitempty"`
}

func textDedup(version, extractor string) string {
	return TextJobKind + "/" + version + "/" + extractor
}

// Schedule atomically records a version-specific derivation and its job. It is
// infrastructure scheduling, not a user mutation or an arbitrary object read.
func (s *TextIndexService) Schedule(ctx context.Context) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT v.org_id,v.file_id,v.id,v.sha256,file_text_extractor(v.media_type) FROM file_versions v
        JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
        WHERE v.availability='available' AND v.scan_state='clean' AND f.state='active'
		AND (file_text_extractor(v.media_type)<>'document-ocr-v1' OR $1::boolean)
        AND NOT EXISTS(SELECT 1 FROM file_text_derivations d WHERE d.org_id=v.org_id AND d.version_id=v.id AND d.extractor=file_text_extractor(v.media_type))
        ORDER BY v.created_at,v.id LIMIT 50`, s.Native != nil)
	if err != nil {
		return 0, err
	}
	type candidate struct{ org, file, version, sha, extractor string }
	items := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.org, &c.file, &c.version, &c.sha, &c.extractor); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	q := jobqueue.Queue{DB: s.DB}
	count := 0
	for _, c := range items {
		engine := ""
		if c.extractor == DocumentTextExtractor {
			engine = s.Native.Fingerprint()
		}
		tag, e := tx.Exec(ctx, `INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256,engine_sha256)
            VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) ON CONFLICT DO NOTHING`, c.org, c.file, c.version, c.extractor, c.sha, engine)
		if e != nil {
			return 0, e
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		payload, _ := json.Marshal(textJobInput{FileID: c.file, VersionID: c.version, Extractor: c.extractor, EngineSHA256: engine})
		if _, _, e = q.Enqueue(ctx, tx, jobqueue.Input{OrgID: c.org, Kind: TextJobKind, Lane: TextJobLane, DedupKey: textDedup(c.version, c.extractor), PayloadVersion: 1, Payload: payload, MaxAttempts: 8}); e != nil {
			return 0, e
		}
		count++
	}
	return count, tx.Commit(ctx)
}

func (s *TextIndexService) textTx(ctx context.Context, j jobqueue.Job, in textJobInput) (pgx.Tx, downloadVersionRow, error) {
	var v downloadVersionRow
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, v, err
	}
	fail := func(e error) (pgx.Tx, downloadVersionRow, error) { _ = tx.Rollback(ctx); return nil, v, e }
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&id); err != nil {
		return fail(err)
	}
	f, err := file(ctx, tx, j.OrgID, in.FileID, false)
	if err != nil {
		return fail(err)
	}
	if err = replicaNamespace(ctx, tx, j.OrgID, f.ProjectID); err != nil {
		return fail(err)
	}
	if v.File, err = file(ctx, tx, j.OrgID, in.FileID, true); err != nil {
		return fail(err)
	}
	if _, err = tx.Exec(ctx, "SELECT lock_file_text_version($1,$2,$3)", j.OrgID, in.FileID, in.VersionID); err != nil {
		return fail(err)
	}
	if err = tx.QueryRow(ctx, `SELECT id,coalesce(original_name,''),sha256,media_type,scan_state,availability,size_bytes
        FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3`, j.OrgID, in.FileID, in.VersionID).
		Scan(&v.ID, &v.Filename, &v.SHA256, &v.MediaType, &v.ScanState, &v.Availability, &v.Size); err != nil {
		return fail(err)
	}
	if in.Extractor != TextExtractor && in.Extractor != TextExtractorFor(v.MediaType) {
		return fail(core.Invalid("extractor", "正文提取版本与准确来源不一致"))
	}
	var state, engine string
	if err = tx.QueryRow(ctx, `SELECT state,coalesce(engine_sha256,'') FROM file_text_derivations WHERE org_id=$1 AND file_id=$2 AND version_id=$3
		AND extractor=$4 AND source_sha256=$5 FOR UPDATE`, j.OrgID, in.FileID, in.VersionID, in.Extractor, v.SHA256).Scan(&state, &engine); err != nil {
		return fail(err)
	}
	if engine != in.EngineSHA256 || (in.Extractor == DocumentTextExtractor) != (engine != "") {
		return fail(core.Invalid("engine", "正文任务与固定引擎依据不一致"))
	}
	if state != "queued" {
		return fail(core.Fail(409, "FILE_TEXT_ALREADY_FINAL", "此正文提取已有最终结果"))
	}
	if err = (jobqueue.Queue{DB: s.DB}).CheckInTx(ctx, tx, j); err != nil {
		return fail(err)
	}
	return tx, v, nil
}

func textAvailable(ctx context.Context, db platform.DBTX, org string, v downloadVersionRow) bool {
	return v.File.State == "active" && v.ScanState == "clean" && v.Availability == "available" && activeAncestors(ctx, db, org, v.File.FolderID) == nil
}

func (s *TextIndexService) prepareText(ctx context.Context, db platform.DBTX, org string, v downloadVersionRow) (AgentTextSource, error) {
	var out AgentTextSource
	rows, err := db.Query(ctx, `SELECT id FROM file_replicas WHERE org_id=$1 AND version_id=$2 AND state='verified'
        AND expected_sha256=$3 AND actual_sha256=$3 AND verified_at IS NOT NULL AND source_id IS NOT NULL ORDER BY provider,id`, org, v.ID, v.SHA256)
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
		r, e := replica(ctx, db, org, id, false)
		if e != nil {
			return out, e
		}
		backend := s.Backends[r.Provider]
		if backend == nil || backend.Provider != r.Provider || backend.Bucket != r.Bucket || backend.Reader == nil {
			continue
		}
		var enabled bool
		if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1 AND provider=$2 AND bucket=$3 AND enabled)", org, r.Provider, r.Bucket).Scan(&enabled); e != nil {
			return out, e
		}
		var source objectstore.Source
		if !enabled || !downloadSourceEvidence(org, v, r, &source) {
			continue
		}
		frozen, _ := json.Marshal([]any{v.File.ID, v.ID, v.SHA256, v.MediaType, v.Size, r.ID, r.Version, r.Provider, r.Bucket, source})
		return AgentTextSource{FileID: v.File.ID, VersionID: v.ID, SHA256: v.SHA256, MediaType: v.MediaType, Size: v.Size, Fingerprint: core.Digest(frozen), source: source, reader: backend.Reader}, nil
	}
	return out, core.Fail(503, "FILE_TEXT_SOURCE_UNAVAILABLE", "尚无完整校验且配置了独立读取身份的副本")
}

func readTextBytes(ctx context.Context, source AgentTextSource) ([]byte, error) {
	if source.reader == nil || source.Fingerprint == "" || source.Size < 1 || source.Size > TextSourceMaxBytes || source.source.Size != source.Size {
		return nil, core.Forbidden()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := source.reader.Read(ctx, source.source, 0, source.Size)
	if err != nil {
		return nil, core.Fail(503, "FILE_TEXT_READ_FAILED", "准确来源暂时无法读取")
	}
	if result.Body == nil {
		return nil, textError("CONTENT_INVALID")
	}
	defer result.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = result.Body.Close() })
	defer stop()
	if result.Size != source.Size || result.ETag != source.source.ETag || source.source.VersionID != "" && result.VersionID != source.source.VersionID {
		return nil, textError("CONTENT_INVALID")
	}
	body, err := io.ReadAll(io.LimitReader(result.Body, source.Size+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, core.Fail(503, "FILE_TEXT_READ_FAILED", "准确来源未完整读取")
	}
	if int64(len(body)) != source.Size || core.Digest(body) != source.SHA256 {
		return nil, textError("CONTENT_INVALID")
	}
	return body, nil
}

func (s *TextIndexService) finishText(ctx context.Context, tx pgx.Tx, j jobqueue.Job, in textJobInput, state, body, code string, native ...documenttext.Result) error {
	var content, hash *string
	var count *int
	var pages, ocr *int
	if state == "ready" {
		content = &body
		h := core.Digest([]byte(body))
		hash = &h
		c := len([]rune(body))
		count = &c
		if in.Extractor == DocumentTextExtractor {
			if len(native) != 1 || native[0].ProfileSHA256 != in.EngineSHA256 || native[0].Text != body || native[0].PageCount < 1 || native[0].PageCount > documenttext.MaxPages || native[0].OCRPageCount != native[0].PageCount {
				return core.Fail(502, "FILE_TEXT_NATIVE_OUTPUT_INVALID", "正文引擎未返回完整页数与准确依据")
			}
			pages, ocr = &native[0].PageCount, &native[0].OCRPageCount
		}
	}
	_, err := tx.Exec(ctx, `UPDATE file_text_derivations SET state=$5,body=$6,text_sha256=$7,character_count=$8,
        error_code=NULLIF($9,''),indexed_at=clock_timestamp(),page_count=$10,ocr_page_count=$11 WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4 AND state='queued'`,
		j.OrgID, in.FileID, in.VersionID, in.Extractor, state, content, hash, count, code, pages, ocr)
	if err == nil {
		err = (jobqueue.Queue{DB: s.DB}).CompleteInTx(ctx, tx, j)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

// Process releases all SQL locks before object I/O, then reacquires namespace,
// version and lease locks and rechecks the exact source before publication.
func (s *TextIndexService) Process(ctx context.Context, j jobqueue.Job) error {
	var in textJobInput
	if j.Kind != TextJobKind || j.Lane != TextJobLane || j.PayloadVersion != 1 || json.Unmarshal(j.Payload, &in) != nil || !core.ValidID(in.FileID) || !core.ValidID(in.VersionID) || (in.Extractor != TextExtractor && in.Extractor != OfficeTextExtractor && in.Extractor != DOCXTextExtractor && in.Extractor != DocumentTextExtractor) || j.DedupKey != textDedup(in.VersionID, in.Extractor) {
		return core.Invalid("job", "正文任务固定输入无效")
	}
	if in.Extractor == DocumentTextExtractor && (s.Native == nil || in.EngineSHA256 != s.Native.Fingerprint()) {
		return core.Fail(503, "FILE_TEXT_ENGINE_UNAVAILABLE", "固定正文引擎暂不可用")
	}
	tx, v, err := s.textTx(ctx, j, in)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !textAvailable(ctx, tx, j.OrgID, v) {
		return s.finishText(ctx, tx, j, in, "blocked", "", "FILE_TEXT_SOURCE_BLOCKED")
	}
	// Legacy queued text-v1 Office jobs retain their original unsupported result.
	if in.Extractor == TextExtractor && (TextExtractorFor(v.MediaType) == OfficeTextExtractor || TextExtractorFor(v.MediaType) == DocumentTextExtractor) {
		return s.finishText(ctx, tx, j, in, "unsupported", "", "FILE_TEXT_FORMAT_UNSUPPORTED")
	}
	if _, err = textFormat(v.MediaType, v.Size); err != nil {
		return s.finishText(ctx, tx, j, in, "unsupported", "", core.PublicError(err).Code)
	}
	source, err := s.prepareText(ctx, tx, j.OrgID, v)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	body, readErr := readTextBytes(ctx, source)
	text := ""
	var native documenttext.Result
	if readErr == nil {
		if in.Extractor == DocumentTextExtractor {
			native, readErr = s.Native.Extract(ctx, v.MediaType, body)
			text = native.Text
			var failure *documenttext.Error
			if errors.As(readErr, &failure) {
				status := 422
				if failure.Code == "FILE_TEXT_ENGINE_CHANGED" || failure.Code == "FILE_TEXT_ENGINE_PROFILE_INVALID" {
					status = 503
				}
				readErr = core.Fail(status, failure.Code, "此准确版本未生成完整识别正文，请核对原件或稍后重试")
			}
		} else {
			text, readErr = extractTextWith(ctx, v.MediaType, body, in.Extractor)
		}
	}
	if readErr != nil && core.PublicError(readErr).Status != 422 {
		return readErr
	}
	tx, v, err = s.textTx(ctx, j, in)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !textAvailable(ctx, tx, j.OrgID, v) {
		return s.finishText(ctx, tx, j, in, "blocked", "", "FILE_TEXT_SOURCE_BLOCKED")
	}
	current, err := s.prepareText(ctx, tx, j.OrgID, v)
	if err != nil || current.Fingerprint != source.Fingerprint {
		return s.finishText(ctx, tx, j, in, "blocked", "", "FILE_TEXT_SOURCE_CHANGED")
	}
	if readErr != nil {
		code := core.PublicError(readErr).Code
		state := "failed"
		if strings.HasSuffix(code, "_UNSUPPORTED") {
			state = "unsupported"
		}
		return s.finishText(ctx, tx, j, in, state, "", code)
	}
	return s.finishText(ctx, tx, j, in, "ready", text, "", native)
}

func (s *TextIndexService) Fail(ctx context.Context, j jobqueue.Job, cause error) error {
	var in textJobInput
	if json.Unmarshal(j.Payload, &in) != nil {
		return cause
	}
	tx, _, err := s.textTx(ctx, j, in)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	public := core.PublicError(cause)
	permanent := public.Status >= 400 && public.Status < 500
	code := public.Code
	if !strings.HasPrefix(code, "FILE_TEXT_") {
		code = "FILE_TEXT_PROCESS_FAILED"
	}
	if permanent || j.Attempt >= j.MaxAttempts {
		if _, err = tx.Exec(ctx, `UPDATE file_text_derivations SET state='failed',error_code=$4,indexed_at=clock_timestamp()
            WHERE org_id=$1 AND version_id=$2 AND extractor=$3 AND state='queued'`, j.OrgID, in.VersionID, in.Extractor, code); err != nil {
			return err
		}
	}
	if err = (jobqueue.Queue{DB: s.DB}).FailInTx(ctx, tx, j, code, permanent, time.Duration(1<<min(j.Attempt, 6))*time.Second); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *TextIndexService) SealDead(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `UPDATE file_text_derivations d SET state='failed',error_code='FILE_TEXT_ATTEMPTS_EXHAUSTED',indexed_at=clock_timestamp()
        WHERE state='queued' AND extractor IN ('text-v1','office-v1','docx-v2','document-ocr-v1') AND EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=d.org_id AND j.kind=$1 AND j.lane=$2
        AND j.dedup_key=$1||'/'||d.version_id::text||'/'||d.extractor AND j.state='dead')`, TextJobKind, TextJobLane)
	return err
}
