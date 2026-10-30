package files

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const VerifySmallLane = "storage.verify.small"
const VerifyLargeLane = "storage.verify.large"
const smallFileLimit int64 = 100000000

var VerifyJobKinds = []string{"upload.verify.v1"}
var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var scanCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,79}$`)

func VerifyLane(size int64) string {
	if size <= smallFileLimit {
		return VerifySmallLane
	}
	return VerifyLargeLane
}
func validIntegrityInput(in uploadJobInput, lane string) bool {
	size, err := strconv.ParseInt(in.SizeBytes, 10, 64)
	return err == nil && strconv.FormatInt(size, 10) == in.SizeBytes && size > 0 && size <= objectstore.MaxObjectBytes &&
		shaPattern.MatchString(in.SourceDigest) && lane == VerifyLane(size)
}
func UploadIntegrityConsumer() jobqueue.Consumer {
	return jobqueue.Consumer{Name: "upload.integrity.v1", EventTypes: []string{"upload.source.frozen", "upload.rescan.requested"}, SchemaVersion: 1,
		Build: func(e jobqueue.Event) (jobqueue.Input, error) {
			var event struct {
				SessionID    string  `json:"session_id"`
				StorageEpoch int64   `json:"storage_epoch"`
				SourceDigest string  `json:"source_digest"`
				SizeBytes    string  `json:"size_bytes"`
				JobID        *string `json:"job_id,omitempty"`
				Reason       *string `json:"reason,omitempty"`
				WaiverID     *string `json:"waiver_id,omitempty"`
			}
			decoder := json.NewDecoder(bytes.NewReader(e.Payload))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&event); err != nil {
				return jobqueue.Input{}, err
			}
			if decoder.Decode(new(any)) != io.EOF {
				return jobqueue.Input{}, core.Fail(409, "INVALID_JOB_INPUT", "固定源事件包含多条记录")
			}
			in := uploadJobInput{SessionID: event.SessionID, StorageEpoch: event.StorageEpoch, SessionVersion: e.AggregateVersion, SourceDigest: event.SourceDigest, SizeBytes: event.SizeBytes}
			size, _ := strconv.ParseInt(in.SizeBytes, 10, 64)
			lane := VerifyLane(size)
			if !core.ValidID(in.SessionID) || in.SessionID != e.AggregateID || e.AggregateType != "upload" || in.StorageEpoch < 1 || in.SessionVersion < 1 || !validIntegrityInput(in, lane) {
				return jobqueue.Input{}, core.Fail(409, "INVALID_JOB_INPUT", "固定源校验事件无效")
			}
			payload, _ := json.Marshal(in)
			return jobqueue.Input{OrgID: e.OrgID, Kind: "upload.verify.v1", Lane: lane, DedupKey: fmt.Sprintf("upload.verify.v1/%s/%d", in.SessionID, in.SessionVersion), PayloadVersion: 1, Payload: payload, MaxAttempts: 8}, nil
		}}
}
func fixedSource(u Upload, in uploadJobInput) (objectstore.Source, error) {
	var source objectstore.Source
	if u.FrozenSourceID == nil || u.FrozenETag == nil || core.Digest([]byte(*u.FrozenSourceID)) != in.SourceDigest || u.SizeBytes != in.SizeBytes {
		return source, core.Fail(409, "SOURCE_CHECKPOINT_MISMATCH", "任务与固定来源不一致")
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(*u.FrozenSourceID)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return source, core.Fail(409, "INVALID_PROVIDER_SOURCE", "固定来源记录无效")
	}
	canonical, _ := json.Marshal(source)
	native := ""
	if u.FrozenNativeVersion != nil {
		native = *u.FrozenNativeVersion
	}
	if core.Digest(canonical) != in.SourceDigest || source.Key != u.DraftKey || source.ETag != *u.FrozenETag || source.VersionID != native || source.Size != u.layout().Size {
		return source, core.Fail(409, "SOURCE_CHECKPOINT_MISMATCH", "固定来源证据不一致")
	}
	return source, nil
}
func (s *UploadService) Rescan(ctx context.Context, c core.Command, id string, in UploadExpected) (core.Result, error) {
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"blocked", "failed"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		if u.FrozenSourceID == nil {
			return core.Result{}, core.Fail(409, "UPLOAD_SOURCE_NOT_FROZEN", "上传尚无可重新校验的固定来源")
		}
		before := digest(u)
		if _, err := tx.Exec(ctx, "UPDATE upload_sessions SET state='verifying',scan_state='pending',scan_result_ref=NULL,scan_waiver_ref=NULL,last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1", id); err != nil {
			return core.Result{}, err
		}
		current, err := upload(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		event := uploadEvent("upload.rescan.requested", current, map[string]any{"source_digest": core.Digest([]byte(*u.FrozenSourceID)), "size_bytes": u.SizeBytes})
		event.BeforeDigest = before
		return core.Result{Status: 202, Data: current, Event: event}, nil
	})
}
func (s *UploadService) RunIntegrityJob(ctx context.Context, j jobqueue.Job, store objectstore.Store, scanner filescan.Scanner) error {
	in, err := decodeUploadJob(j)
	if err != nil {
		return err
	}
	if j.Kind != "upload.verify.v1" || store == nil {
		return core.Fail(409, "INVALID_JOB_INPUT", "固定源校验任务配置无效")
	}
	if scanner == nil {
		scanner = filescan.Disabled{}
	}
	var current Upload
	var progress objectstore.HashCheckpoint
	skip := false
	err = s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "verifying" || u.Version != in.SessionVersion {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.workerAllowed(ctx, tx, j, u); err != nil {
			return err
		}
		current = u
		var sha *string
		err := tx.QueryRow(ctx, `SELECT source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256
            FROM upload_hash_checkpoints WHERE org_id=$1 AND session_id=$2`, j.OrgID, u.ID).Scan(
			&progress.SourceDigest, &progress.RuntimeVersion, &progress.CompletedBytes, &progress.State, &progress.Checksum, &sha)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if sha != nil {
			progress.SHA256 = *sha
		}
		return nil
	})
	if err != nil || skip {
		return err
	}
	source, err := fixedSource(current, in)
	if err != nil {
		return err
	}
	var authority objectstore.Digest
	if current.SHA256 != nil {
		if !shaPattern.MatchString(*current.SHA256) || progress.SourceDigest != in.SourceDigest || progress.CompletedBytes != source.Size || progress.SHA256 != *current.SHA256 {
			return core.Fail(409, "HASH_CHECKPOINT_INVALID", "已完成的摘要证据不一致")
		}
		authority, err = objectstore.CompletedHash(source, progress)
		if err != nil {
			return err
		}
	} else {
		authority, err = objectstore.ResumeHashSource(ctx, store, source, progress, func(p objectstore.HashCheckpoint) error {
			return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
				if u.State != "verifying" || u.Version != in.SessionVersion {
					return core.Fail(409, "UPLOAD_STATE_CONFLICT", "校验阶段已改变")
				}
				if err := s.workerAllowed(ctx, tx, j, u); err != nil {
					return err
				}
				if _, err := fixedSource(u, in); err != nil {
					return err
				}
				tag, err := tx.Exec(ctx, `INSERT INTO upload_hash_checkpoints(org_id,session_id,source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256,job_id,fencing_token)
                    VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)
                    ON CONFLICT(org_id,session_id) DO UPDATE SET runtime_version=excluded.runtime_version,completed_bytes=excluded.completed_bytes,
                    hash_state=excluded.hash_state,checksum=excluded.checksum,sha256=excluded.sha256,job_id=excluded.job_id,
                    fencing_token=excluded.fencing_token,updated_at=clock_timestamp()
                    WHERE upload_hash_checkpoints.source_digest=excluded.source_digest`,
					j.OrgID, u.ID, p.SourceDigest, p.RuntimeVersion, p.CompletedBytes, p.State, p.Checksum, p.SHA256, j.ID, j.FencingToken)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					return core.Fail(409, "SOURCE_CHECKPOINT_MISMATCH", "摘要检查点属于其他来源")
				}
				_, err = tx.Exec(ctx, `UPDATE upload_sessions SET verified_bytes=$2,
                    final_digest=coalesce(NULLIF($3,''),final_digest),updated_at=clock_timestamp() WHERE id=$1`, u.ID, p.CompletedBytes, p.SHA256)
				return err
			})
		})
		if err != nil {
			return err
		}
	}
	if strings.HasPrefix(current.SourceFingerprint, "office-artifact-v1:") && (source.Size > OfficeAuthorMaxOutput || current.SourceFingerprint != "office-artifact-v1:"+authority.SHA256) {
		return core.Fail(409, "OFFICE_ARTIFACT_HASH_MISMATCH", "上传正文与确认的Office候选摘要不一致")
	}
	// Delay opening cloud data until the scanner actually reads. A policy-size
	// rejection or disabled engine therefore performs no extra object read.
	lazy := newLazySource(ctx, store, source)
	scanned, err := scanner.Scan(ctx, lazy, source.Size)
	closeErr := lazy.Close()
	if sourceErr := lazy.sourceError(); sourceErr != nil {
		return sourceErr
	}
	if err != nil {
		return err
	}
	if closeErr != nil {
		return &objectstore.Error{Code: "SOURCE_READ_FAILED", Retryable: true}
	}
	if !validScanEvidence(scanned, source.Size) || (scanned.State == "clean" && !lazy.complete(source.Size)) {
		return core.Fail(409, "INVALID_SCAN_EVIDENCE", "扫描回执缺少所需证据")
	}
	return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "verifying" || u.Version != in.SessionVersion {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.workerAllowed(ctx, tx, j, u); err != nil {
			return err
		}
		if _, err := fixedSource(u, in); err != nil {
			return err
		}
		if u.SHA256 == nil || *u.SHA256 != authority.SHA256 {
			return core.Fail(409, "HASH_CHECKPOINT_INVALID", "扫描未绑定权威摘要")
		}
		reportID := core.NewID()
		_, err := tx.Exec(ctx, `INSERT INTO upload_scan_reports(id,org_id,session_id,source_digest,sha256,size_bytes,state,code,engine,engine_version,
            signature_revision,policy_sha256,streamed_bytes,job_id,fencing_token)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,$15)`,
			reportID, j.OrgID, u.ID, in.SourceDigest, authority.SHA256, source.Size, scanned.State, scanned.Code, scanned.Engine, scanned.EngineVersion,
			scanned.SignatureRevision, scanned.PolicySHA256, scanned.StreamedBytes, j.ID, j.FencingToken)
		if err != nil {
			return err
		}
		before := digest(u)
		state := "blocked"
		var code *string = &scanned.Code
		kind := "upload.scan.blocked"
		if scanned.State == "clean" {
			state = "verifying"
			code = nil
			kind = "upload.scan.passed"
		}
		if _, err = tx.Exec(ctx, `UPDATE upload_sessions SET state=$2,scan_state=$3,scan_result_ref=$4,last_error=$5,
            version=version+1,updated_at=clock_timestamp() WHERE id=$1`, u.ID, state, scanned.State, reportID, code); err != nil {
			return err
		}
		u, err = upload(ctx, tx, j.OrgID, u.ID, false)
		if err != nil {
			return err
		}
		if err = workerEvent(ctx, tx, j, u, kind, before); err != nil {
			return err
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}
func validScanEvidence(r filescan.Result, size int64) bool {
	if !scanCodePattern.MatchString(r.Code) || r.StreamedBytes < 0 || r.StreamedBytes > size {
		return false
	}
	for _, field := range []string{r.Engine, r.EngineVersion, r.SignatureRevision} {
		if field != "" && !validOpaque(field) {
			return false
		}
	}
	if r.PolicySHA256 != "" && !shaPattern.MatchString(r.PolicySHA256) {
		return false
	}
	switch r.State {
	case "clean":
		return r.Code == "SCAN_PASSED" && r.Engine != "" && r.EngineVersion != "" && r.SignatureRevision != "" && r.PolicySHA256 != "" && r.StreamedBytes == size
	case "quarantined", "unscannable":
		return r.Code != "SCAN_PASSED"
	default:
		return false
	}
}

type ScanReport struct {
	ID                string    `json:"id"`
	UploadID          string    `json:"upload_id"`
	SHA256            string    `json:"sha256"`
	SizeBytes         string    `json:"size_bytes"`
	State             string    `json:"state"`
	Code              string    `json:"code"`
	Engine            *string   `json:"engine"`
	EngineVersion     *string   `json:"engine_version"`
	SignatureRevision *string   `json:"signature_revision"`
	PolicySHA256      *string   `json:"policy_sha256"`
	StreamedBytes     string    `json:"streamed_bytes"`
	CreatedAt         time.Time `json:"created_at"`
}

func (s *UploadService) ScanReport(ctx context.Context, a core.Actor, id string) (ScanReport, error) {
	u, _, err := s.authorize(ctx, s.Files.DB, a, id)
	if err != nil {
		return ScanReport{}, err
	}
	if u.ScanResultID == nil {
		return ScanReport{}, core.Missing()
	}
	return s.readScanReport(ctx, a.OrgID, id, *u.ScanResultID)
}
func (s *UploadService) readScanReport(ctx context.Context, org, uploadID, reportID string) (ScanReport, error) {
	var report ScanReport
	err := s.Files.DB.QueryRow(ctx, `SELECT id,session_id,sha256,size_bytes::text,state,code,engine,engine_version,signature_revision,
        policy_sha256,streamed_bytes::text,created_at FROM upload_scan_reports WHERE org_id=$1 AND session_id=$2 AND id=$3`,
		org, uploadID, reportID).Scan(&report.ID, &report.UploadID, &report.SHA256, &report.SizeBytes, &report.State, &report.Code,
		&report.Engine, &report.EngineVersion, &report.SignatureRevision, &report.PolicySHA256, &report.StreamedBytes, &report.CreatedAt)
	if report.State == "quarantined" {
		report.State = "blocked"
	}
	return report, platform.DatabaseError(err)
}

type lazySource struct {
	ctx       context.Context
	cancel    context.CancelFunc
	store     objectstore.Store
	source    objectstore.Source
	mu        sync.Mutex
	body      io.ReadCloser
	closed    bool
	failure   error
	bytesRead int64
	eof       bool
}

func newLazySource(ctx context.Context, store objectstore.Store, source objectstore.Source) *lazySource {
	ctx, cancel := context.WithCancel(ctx)
	return &lazySource{ctx: ctx, cancel: cancel, store: store, source: source}
}
func (s *lazySource) Read(p []byte) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	if s.body == nil {
		read, err := s.store.Read(s.ctx, s.source, 0, s.source.Size)
		if err == nil && (read.Body == nil || read.Size != s.source.Size || read.ETag != s.source.ETag || read.VersionID != s.source.VersionID) {
			if read.Body != nil {
				_ = read.Body.Close()
			}
			err = &objectstore.Error{Code: "SOURCE_EVIDENCE_MISMATCH"}
		}
		if err != nil {
			s.failure = err
			s.mu.Unlock()
			return 0, err
		}
		s.body = read.Body
	}
	body := s.body
	s.mu.Unlock()
	n, err := body.Read(p)
	s.mu.Lock()
	s.bytesRead += int64(n)
	if err == io.EOF {
		s.eof = true
	}
	if err != nil && err != io.EOF {
		s.failure = &objectstore.Error{Code: "SOURCE_READ_FAILED", Retryable: true}
	}
	s.mu.Unlock()
	return n, err
}
func (s *lazySource) Close() error {
	s.cancel()
	s.mu.Lock()
	s.closed = true
	body := s.body
	s.body = nil
	s.mu.Unlock()
	if body != nil {
		return body.Close()
	}
	return nil
}
func (s *lazySource) sourceError() error { s.mu.Lock(); defer s.mu.Unlock(); return s.failure }
func (s *lazySource) complete(size int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytesRead == size && s.eof
}
