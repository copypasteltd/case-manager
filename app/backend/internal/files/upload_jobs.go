package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const UploadLane = "storage.control"

var UploadJobKinds = []string{"upload.prepare.v1", "upload.finalize.v1"}

type uploadJobInput struct {
	SessionID      string `json:"session_id"`
	StorageEpoch   int64  `json:"storage_epoch"`
	SessionVersion int64  `json:"session_version"`
	SourceDigest   string `json:"source_digest,omitempty"`
	SizeBytes      string `json:"size_bytes,omitempty"`
}

func UploadConsumer() jobqueue.Consumer {
	return jobqueue.Consumer{Name: "upload.lifecycle.v1", EventTypes: []string{"upload.created", "upload.finalize.requested"}, SchemaVersion: 1,
		Build: func(e jobqueue.Event) (jobqueue.Input, error) {
			var in uploadJobInput
			decoder := json.NewDecoder(bytes.NewReader(e.Payload))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&in); err != nil {
				return jobqueue.Input{}, err
			}
			if decoder.Decode(new(any)) != io.EOF || in.SourceDigest != "" || in.SizeBytes != "" {
				return jobqueue.Input{}, errors.New("invalid control event payload")
			}
			in.SessionVersion = e.AggregateVersion
			if !core.ValidID(in.SessionID) || in.SessionID != e.AggregateID || e.AggregateType != "upload" || in.StorageEpoch < 1 || in.SessionVersion < 1 {
				return jobqueue.Input{}, errors.New("invalid upload event reference")
			}
			kind := "upload.prepare.v1"
			if e.Type == "upload.finalize.requested" {
				kind = "upload.finalize.v1"
			} else if e.Type != "upload.created" {
				return jobqueue.Input{}, errors.New("unsupported upload event")
			}
			payload, _ := json.Marshal(in)
			return jobqueue.Input{OrgID: e.OrgID, Kind: kind, Lane: UploadLane, DedupKey: fmt.Sprintf("%s/%s/%d", kind, in.SessionID, in.SessionVersion), PayloadVersion: 1, Payload: payload, MaxAttempts: 8}, nil
		}}
}
func decodeUploadJob(j jobqueue.Job) (uploadJobInput, error) {
	var in uploadJobInput
	decoder := json.NewDecoder(bytes.NewReader(j.Payload))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&in)
	if err == nil {
		if decoder.Decode(new(any)) != io.EOF {
			err = errors.New("trailing payload")
		}
	}
	laneValid := j.Lane == UploadLane && in.SourceDigest == "" && in.SizeBytes == ""
	if j.Kind == "upload.verify.v1" {
		laneValid = validIntegrityInput(in, j.Lane)
	}
	if j.Kind == "upload.promote.v1" {
		size, _ := strconv.ParseInt(in.SizeBytes, 10, 64)
		laneValid = validIntegrityInput(in, VerifyLane(size)) && j.Lane == PromotionLane(size)
	}
	if err != nil || j.PayloadVersion != 1 || !core.ValidID(in.SessionID) || in.StorageEpoch < 1 || in.SessionVersion < 1 || !laneValid {
		return in, core.Fail(409, "INVALID_JOB_INPUT", "上传任务输入无效")
	}
	return in, nil
}

// jobTx acquires domain locks before the lease row. Cleanup/checkpointing can
// still record receipts after archival; positive progress separately rechecks ACLs.
func (s *UploadService) jobTx(ctx context.Context, j jobqueue.Job, in uploadJobInput, fn func(pgx.Tx, Upload) error) error {
	tx, err := s.Files.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&org); err != nil {
		return err
	}
	u, err := upload(ctx, tx, j.OrgID, in.SessionID, false)
	if err != nil {
		return err
	}
	target, err := folder(ctx, tx, j.OrgID, u.FolderID, false)
	if err != nil {
		return err
	}
	if target.ProjectID == nil {
		_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", j.OrgID+"/company-files")
	} else {
		err = tx.QueryRow(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", j.OrgID, *target.ProjectID).Scan(&org)
	}
	if err != nil {
		return err
	}
	u, err = upload(ctx, tx, j.OrgID, in.SessionID, true)
	if err != nil {
		return err
	}
	if u.StorageEpoch != in.StorageEpoch {
		return core.Fail(409, "STORAGE_EPOCH_CHANGED", "上传任务存储纪元不一致")
	}
	if err = s.Queue.CheckInTx(ctx, tx, j); err != nil {
		return err
	}
	if err = fn(tx, u); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *UploadService) workerAllowed(ctx context.Context, tx pgx.Tx, j jobqueue.Job, u Upload) error {
	var a core.Actor
	var status string
	a.OrgID = j.OrgID
	a.UserID = u.OwnerID
	if u.SourceChannel == nil {
		return core.Fail(409, "INVALID_UPLOAD_SOURCE", "上传渠道记录缺失")
	}
	a.Channel = *u.SourceChannel
	if err := tx.QueryRow(ctx, `SELECT u.status,u.auth_version,o.authorization_version FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.org_id=$1 AND u.id=$2`, a.OrgID, a.UserID).Scan(&status, &a.AuthVersion, &a.OrgAuthVersion); err != nil {
		return err
	}
	if status != "active" {
		return core.Forbidden()
	}
	if a.Channel == "agent" {
		var actorVersion, orgVersion int64
		var live bool
		err := tx.QueryRow(ctx, `SELECT r.id,r.channel,r.actor_auth_version,r.organization_auth_version,
 r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
 FROM agent_file_drafts d JOIN agent_runs r ON r.org_id=d.org_id AND r.id=d.run_id
 WHERE d.org_id=$1 AND d.id=$2 AND r.actor_id=$3`, a.OrgID, u.ID, a.UserID).Scan(&a.RunID, &a.Channel, &actorVersion, &orgVersion, &live)
		if err != nil {
			return platform.DatabaseError(err)
		}
		if !live || actorVersion != a.AuthVersion || orgVersion != a.OrgAuthVersion {
			return core.Fail(412, "AUTHORIZATION_CHANGED", "秘书草稿的原始授权已变化")
		}
	}
	f, err := uploadTarget(ctx, tx, a, u.FolderID, u.FileID)
	if err != nil {
		return err
	}
	if !writable(ctx, tx, a.OrgID, f.ProjectID) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许继续上传")
	}
	if u.Classification == nil || *u.Classification != f.Classification {
		return core.Fail(409, "UPLOAD_TARGET_CHANGED", "上传目标分类已改变")
	}
	if !time.Now().Before(u.ExpiresAt) {
		return core.Fail(410, "UPLOAD_EXPIRED", "上传会话已过期")
	}
	epoch, provider, err := currentEpoch(ctx, tx, a.OrgID)
	if err != nil {
		return err
	}
	if u.Provider == nil || u.StorageEpoch != epoch || *u.Provider != provider {
		return core.Fail(409, "STORAGE_EPOCH_CHANGED", "上传存储纪元已改变")
	}
	if s.Backend == nil || s.Backend.Provider != *u.Provider || u.Bucket == nil || s.Backend.Bucket != *u.Bucket {
		return core.Fail(503, "STORAGE_CONFIGURATION_CHANGED", "上传 Worker 配置与固定目标不一致")
	}
	return nil
}
func workerEvent(ctx context.Context, tx pgx.Tx, j jobqueue.Job, u Upload, kind, before string) error {
	fields := map[string]any{"session_id": u.ID, "storage_epoch": u.StorageEpoch, "job_id": j.ID}
	if (kind == "upload.source.frozen" || kind == "upload.scan.passed" || kind == "upload.scan.waived") && u.FrozenSourceID != nil {
		fields["source_digest"] = core.Digest([]byte(*u.FrozenSourceID))
		fields["size_bytes"] = u.SizeBytes
	}
	payload, _ := json.Marshal(fields)
	requestID := "job/" + j.ID
	// The initiating owner remains actor; automated executor is identified by
	// job_id and channel, not falsely attributed to a human executor account.
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,job_id,action,resource_type,resource_id,resource_version,before_digest,after_digest,result)
        VALUES($1,$2,$3,NULL,'worker',$4,$5,$6,'upload',$7,$8,$9,$10,'succeeded')`,
		core.NewID(), j.OrgID, u.OwnerID, requestID, j.ID, kind, u.ID, u.Version, before, digest(u)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
        VALUES($1,$2,$3,'upload',$4,$5,$6,$7)`, core.NewID(), j.OrgID, kind, u.ID, u.Version, payload, requestID)
	return err
}
func (s *UploadService) RunUploadJob(ctx context.Context, j jobqueue.Job, store objectstore.Store) error {
	if store == nil {
		return core.Fail(503, "STORAGE_NOT_CONFIGURED", "上传 Worker 尚未配置")
	}
	in, err := decodeUploadJob(j)
	if err != nil {
		return err
	}
	switch j.Kind {
	case "upload.prepare.v1":
		return s.prepare(ctx, j, in, store)
	case "upload.finalize.v1":
		return s.freeze(ctx, j, in, store)
	default:
		return core.Fail(409, "INVALID_JOB_INPUT", "上传任务类型无效")
	}
}
func (s *UploadService) prepare(ctx context.Context, j jobqueue.Job, in uploadJobInput, store objectstore.Store) error {
	attemptID := core.NewID()
	key, _ := objectstore.DraftKey(j.OrgID, attemptID)
	var current Upload
	skip := false
	err := s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "created" || u.Version != in.SessionVersion {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.workerAllowed(ctx, tx, j, u); err != nil {
			return err
		}
		current = u
		_, err := tx.Exec(ctx, `INSERT INTO upload_attempts(id,org_id,session_id,job_id,fencing_token,draft_key,state)
            VALUES($1,$2,$3,$4,$5,$6,'starting')`, attemptID, j.OrgID, u.ID, j.ID, j.FencingToken, key)
		return err
	})
	if err != nil || skip {
		return err
	}
	multipart, err := store.Begin(ctx, key, current.MediaType)
	if err != nil {
		code := "STORAGE_REQUEST_FAILED"
		if e, ok := err.(*objectstore.Error); ok {
			code = e.Code
		}
		_ = s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
			_, e := tx.Exec(ctx, "UPDATE upload_attempts SET state='uncertain',error_code=$3,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", j.OrgID, attemptID, code)
			return e
		})
		return err
	}
	if multipart.Key != key || multipart.UploadID == "" {
		return core.Fail(502, "INVALID_STORAGE_RECEIPT", "云端上传回执无效")
	}
	return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		state := "active"
		allowed := s.workerAllowed(ctx, tx, j, u)
		if u.State != "created" || u.Version != in.SessionVersion || allowed != nil {
			state = "superseded"
		}
		if _, err := tx.Exec(ctx, "UPDATE upload_attempts SET state=$3,provider_upload_id=$4,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", j.OrgID, attemptID, state, multipart.UploadID); err != nil {
			return err
		}
		if state == "active" {
			before := digest(u)
			if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET draft_key=$2,provider_upload_id=$3,state='uploading',last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1`, u.ID, key, multipart.UploadID); err != nil {
				return err
			}
			u, err = upload(ctx, tx, j.OrgID, u.ID, false)
			if err != nil {
				return err
			}
			if err = workerEvent(ctx, tx, j, u, "upload.prepared", before); err != nil {
				return err
			}
		} else if u.State == "created" {
			before := digest(u)
			if _, err := tx.Exec(ctx, "UPDATE upload_sessions SET state='failed',last_error='UPLOAD_AUTHORIZATION_CHANGED',version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID); err != nil {
				return err
			}
			u, err = upload(ctx, tx, j.OrgID, u.ID, false)
			if err != nil {
				return err
			}
			if err = workerEvent(ctx, tx, j, u, "upload.prepare.cancelled", before); err != nil {
				return err
			}
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}
func (s *UploadService) freeze(ctx context.Context, j jobqueue.Job, in uploadJobInput, store objectstore.Store) error {
	var current Upload
	skip := false
	err := s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "finalizing" || u.Version != in.SessionVersion {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.workerAllowed(ctx, tx, j, u); err != nil {
			return err
		}
		current = u
		return nil
	})
	if err != nil || skip {
		return err
	}
	var parts []objectstore.Part
	if len(current.Manifest) == 0 {
		parts, err = cloudParts(ctx, store, current)
		if err != nil {
			if core.PublicError(err).Code == "CLOUD_PARTS_MISMATCH" {
				return s.resumeIncomplete(ctx, j, in)
			}
			return err
		}
		parts, err = current.layout().Reconcile(parts)
		if err != nil {
			return s.resumeIncomplete(ctx, j, in)
		}
		manifest, _ := json.Marshal(parts)
		err = s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
			if u.State != "finalizing" || u.Version != in.SessionVersion {
				return core.Fail(409, "UPLOAD_STATE_CONFLICT", "上传阶段已改变")
			}
			if err := s.workerAllowed(ctx, tx, j, u); err != nil {
				return err
			}
			if len(u.Manifest) == 0 {
				_, err := tx.Exec(ctx, "UPDATE upload_sessions SET finalize_manifest=$2,complete_requested_at=clock_timestamp() WHERE id=$1", u.ID, manifest)
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		current.Manifest = manifest
	} else {
		if err = json.Unmarshal(current.Manifest, &parts); err != nil {
			return core.Fail(409, "INVALID_JOB_INPUT", "封存的上传清单无效")
		}
		if _, err = current.layout().Reconcile(parts); err != nil {
			return core.Fail(409, "INVALID_JOB_INPUT", "封存的上传布局无效")
		}
	}
	// An earlier Complete may have succeeded despite a lost response. Inspect
	// the exact unique key before retrying the sealed manifest.
	source, headErr := store.Head(ctx, current.DraftKey)
	if headErr != nil {
		var providerErr *objectstore.Error
		if !errors.As(headErr, &providerErr) || providerErr.Code != "OBJECT_NOT_FOUND" {
			return headErr
		}
		source, err = store.Complete(ctx, current.multipart(), parts)
		if err != nil {
			var completionError *objectstore.Error
			if errors.As(err, &completionError) && completionError.Code == "OBJECT_NOT_FOUND" {
				return &objectstore.Error{Code: "UPLOAD_COMPLETION_UNKNOWN", Retryable: true, Uncertain: true}
			}
			return err
		}
		source, err = store.Head(ctx, current.DraftKey)
		if err != nil {
			return err
		}
	}
	if source.Key != current.DraftKey || source.Size != current.layout().Size || !validOpaque(source.ETag) {
		return core.Fail(409, "SOURCE_SIZE_MISMATCH", "已完成对象与上传布局不一致")
	}
	sourceRef, _ := json.Marshal(source)
	return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "finalizing" || u.Version != in.SessionVersion {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.workerAllowed(ctx, tx, j, u); err != nil {
			return err
		}
		before := digest(u)
		if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET frozen_source_id=$2,frozen_etag=$3,frozen_native_version=NULLIF($4,''),
            state='verifying',last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1`, u.ID, string(sourceRef), source.ETag, source.VersionID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM upload_parts WHERE session_id=$1", u.ID); err != nil {
			return err
		}
		for _, p := range parts {
			if _, err := tx.Exec(ctx, "INSERT INTO upload_parts(session_id,part_no,size_bytes,provider_etag,cloud_confirmed) VALUES($1,$2,$3,$4,true)", u.ID, p.Number, p.Size, p.ETag); err != nil {
				return err
			}
		}
		u, err = upload(ctx, tx, j.OrgID, u.ID, false)
		if err != nil {
			return err
		}
		if err = workerEvent(ctx, tx, j, u, "upload.source.frozen", before); err != nil {
			return err
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}

// Finalizing before every part arrived is recoverable. No manifest or Complete
// has been issued, so return to uploading with a new lifecycle version.
func (s *UploadService) resumeIncomplete(ctx context.Context, j jobqueue.Job, in uploadJobInput) error {
	return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "finalizing" || u.Version != in.SessionVersion {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if len(u.Manifest) != 0 {
			return core.Fail(409, "UPLOAD_MANIFEST_ALREADY_SEALED", "上传清单已经封存")
		}
		before := digest(u)
		if _, err := tx.Exec(ctx, "UPDATE upload_sessions SET state='uploading',last_error='INCOMPLETE_PARTS',version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID); err != nil {
			return err
		}
		u, err := upload(ctx, tx, j.OrgID, u.ID, false)
		if err != nil {
			return err
		}
		if err = workerEvent(ctx, tx, j, u, "upload.parts.incomplete", before); err != nil {
			return err
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}
