package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

const RestoreSmallLane = "storage.restore.small"
const RestoreLargeLane = "storage.restore.large"

var RestoreJobKinds = []string{"file.history.restore.v1"}

func RestoreLane(size int64) string {
	if size <= smallFileLimit {
		return RestoreSmallLane
	}
	return RestoreLargeLane
}

type restoreJobInput struct {
	RestoreID      string `json:"restore_id"`
	RestoreVersion int64  `json:"restore_version"`
	SourceDigest   string `json:"source_digest"`
	SizeBytes      string `json:"size_bytes"`
}

func restoreRequested(r HistoryRestore) core.Event {
	return core.Event{Type: "file.history.restore.requested", ResourceType: "file_history_restore", ResourceID: r.ID, Version: r.Version,
		Payload: map[string]any{"restore_id": r.ID, "restore_version": r.Version, "source_digest": r.SourceDigest, "size_bytes": r.SizeBytes}}
}
func validRestoreInput(in restoreJobInput, lane string) bool {
	size, err := strconv.ParseInt(in.SizeBytes, 10, 64)
	return err == nil && strconv.FormatInt(size, 10) == in.SizeBytes && size > 0 && size <= objectstore.MaxObjectBytes &&
		core.ValidID(in.RestoreID) && in.RestoreVersion > 0 && shaPattern.MatchString(in.SourceDigest) && lane == RestoreLane(size)
}
func RestoreConsumer() jobqueue.Consumer {
	return jobqueue.Consumer{Name: "file.history.restoration.v1", EventTypes: []string{"file.history.restore.requested"}, SchemaVersion: 1,
		Build: func(e jobqueue.Event) (jobqueue.Input, error) {
			j := jobqueue.Job{Kind: "file.history.restore.v1", PayloadVersion: 1, Payload: e.Payload}
			var provisional restoreJobInput
			if json.Unmarshal(e.Payload, &provisional) != nil {
				return jobqueue.Input{}, errors.New("invalid restoration event")
			}
			size, _ := strconv.ParseInt(provisional.SizeBytes, 10, 64)
			j.Lane = RestoreLane(size)
			in, err := decodeRestoreJob(j)
			if err != nil || e.AggregateType != "file_history_restore" || e.AggregateID != in.RestoreID || e.AggregateVersion != in.RestoreVersion {
				return jobqueue.Input{}, errors.New("invalid restoration reference")
			}
			return jobqueue.Input{OrgID: e.OrgID, Kind: j.Kind, Lane: j.Lane, DedupKey: j.Kind + "/" + in.RestoreID,
				PayloadVersion: 1, Payload: e.Payload, MaxAttempts: 8}, nil
		}}
}
func decodeRestoreJob(j jobqueue.Job) (restoreJobInput, error) {
	var in restoreJobInput
	d := json.NewDecoder(bytes.NewReader(j.Payload))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || j.Kind != "file.history.restore.v1" || j.PayloadVersion != 1 || !validRestoreInput(in, j.Lane) {
		return in, core.Fail(409, "INVALID_JOB_INPUT", "历史恢复任务输入无效")
	}
	return in, nil
}
func restoreOwns(r HistoryRestore, j jobqueue.Job, in restoreJobInput) bool {
	return (r.ActiveJobID == nil && r.Version == in.RestoreVersion) || (r.ActiveJobID != nil && *r.ActiveJobID == j.ID)
}
func restoreTerminal(r HistoryRestore) bool {
	return r.State == "committed" || r.State == "cancelled" || r.State == "failed" || r.State == "ready"
}

// The failure checkpoint uses the same lock order but deliberately permits a
// revoked owner or read-only project: failure must remain durably reportable.
func (s *UploadService) restoreTx(ctx context.Context, j jobqueue.Job, in restoreJobInput, leased bool, fn func(pgx.Tx, HistoryRestore) error) error {
	tx, err := s.Files.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&org); err != nil {
		return err
	}
	r, err := historyRestore(ctx, tx, j.OrgID, in.RestoreID, false)
	if err != nil {
		return err
	}
	f, err := file(ctx, tx, j.OrgID, r.FileID, false)
	if err != nil {
		return err
	}
	if err = replicaNamespace(ctx, tx, j.OrgID, f.ProjectID); err != nil {
		return err
	}
	r, err = historyRestore(ctx, tx, j.OrgID, in.RestoreID, true)
	if err != nil {
		return err
	}
	if r.SourceDigest != in.SourceDigest || r.SizeBytes != in.SizeBytes {
		return core.Fail(409, "INVALID_JOB_INPUT", "恢复任务与固定来源不一致")
	}
	if leased {
		if err = s.Queue.CheckInTx(ctx, tx, j); err != nil {
			return err
		}
	} else {
		var id string
		if err = tx.QueryRow(ctx, "SELECT id FROM jobs WHERE org_id=$1 AND id=$2 AND state='dead' FOR UPDATE", j.OrgID, j.ID).Scan(&id); err != nil {
			return err
		}
	}
	if err = fn(tx, r); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *UploadService) restoreAllowed(ctx context.Context, tx pgx.Tx, org string, r HistoryRestore, publish bool) (objectstore.Source, restoreOrigin, error) {
	var source objectstore.Source
	var origin restoreOrigin
	var current bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.org_id=$1 AND u.id=$2
        AND u.status='active' AND u.auth_version=$3 AND o.authorization_version=$4)`, org, r.OwnerID, r.OwnerAuthVersion, r.OrgAuthVersion).Scan(&current)
	if err != nil {
		return source, origin, err
	}
	if !current {
		return source, origin, core.Fail(412, "AUTHORIZATION_CHANGED", "恢复发起人权限已改变")
	}
	a := core.Actor{OrgID: org, UserID: r.OwnerID, AuthVersion: r.OwnerAuthVersion, OrgAuthVersion: r.OrgAuthVersion, Channel: r.SourceChannel}
	f, err := restoreAccess(ctx, tx, a, r.FileID)
	if err != nil {
		return source, origin, err
	}
	if !writable(ctx, tx, org, f.ProjectID) || f.State != "active" {
		return source, origin, core.Fail(409, "RESTORE_TARGET_READ_ONLY", "恢复目标当前只读或在回收站")
	}
	if f.FolderID != r.FolderID || f.Classification != r.Classification {
		return source, origin, core.Fail(409, "RESTORE_TARGET_CHANGED", "恢复目标目录或分类已改变")
	}
	if publish && (f.Version != r.BaseFileVersion || f.CurrentVersionID == nil || *f.CurrentVersionID != r.BaseVersionID) {
		return source, origin, core.Fail(409, "BASE_VERSION_CONFLICT", "当前文件已改变，请保留恢复候选并重新处理")
	}
	var expired bool
	if err = tx.QueryRow(ctx, "SELECT expires_at<=clock_timestamp() FROM file_history_restores WHERE id=$1", r.ID).Scan(&expired); err != nil {
		return source, origin, err
	}
	if expired {
		return source, origin, core.Fail(410, "RESTORE_EXPIRED", "恢复任务已过期，请重新申请")
	}
	epoch, provider, err := currentEpoch(ctx, tx, org)
	if err != nil {
		return source, origin, err
	}
	if epoch != r.StorageEpoch || provider != r.Provider {
		return source, origin, core.Fail(409, "STORAGE_EPOCH_CHANGED", "恢复存储纪元已改变")
	}
	if s.Backend == nil || s.Backend.Provider != r.Provider || s.Backend.Bucket != r.Bucket {
		return source, origin, core.Fail(503, "STORAGE_CONFIGURATION_CHANGED", "恢复执行器与固定主端不一致")
	}
	if err = restoreTargets(ctx, tx, org, r.Provider, r.Bucket); err != nil {
		return source, origin, err
	}
	origin, err = restoreOriginEvidence(ctx, tx, org, r.FileID, r.SourceVersionID, r.Classification)
	if err != nil {
		return source, origin, err
	}
	if origin.SHA256 != r.SHA256 || origin.Size != restoreSize(r) {
		return source, origin, core.Fail(409, "RESTORE_SOURCE_MISMATCH", "恢复来源摘要已改变")
	}
	rr, err := replica(ctx, tx, org, r.SourceReplicaID, false)
	if err != nil {
		return source, origin, err
	}
	if rr.Provider != r.Provider || rr.Bucket != r.Bucket || rr.VersionID != origin.ID ||
		!downloadSourceEvidence(org, downloadVersionRow{File: f, ID: origin.ID, Size: origin.Size, SHA256: origin.SHA256}, rr, &source) {
		return source, origin, core.Fail(409, "RESTORE_SOURCE_UNAVAILABLE", "恢复来源副本不再可用")
	}
	var fixed objectstore.Source
	raw, _ := json.Marshal(source)
	if json.Unmarshal(r.FixedSource, &fixed) != nil || fixed != source || core.Digest(raw) != r.SourceDigest {
		return source, origin, core.Fail(409, "RESTORE_SOURCE_MISMATCH", "恢复固定对象身份不一致")
	}
	return source, origin, nil
}
func restoreWorkerEvent(ctx context.Context, tx pgx.Tx, j jobqueue.Job, r HistoryRestore, kind, before string) error {
	raw, _ := json.Marshal(map[string]any{"restore_id": r.ID, "file_id": r.FileID, "source_version_id": r.SourceVersionID, "state": r.State, "job_id": j.ID})
	requestID := "job/" + j.ID
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,job_id,action,resource_type,resource_id,resource_version,before_digest,after_digest,result)
        VALUES($1,$2,$3,NULL,'worker',$4,$5,$6,'file_history_restore',$7,$8,$9,$10,'succeeded')`,
		core.NewID(), j.OrgID, r.OwnerID, requestID, j.ID, kind, r.ID, r.Version, before, digest(r)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
        VALUES($1,$2,$3,'file_history_restore',$4,$5,$6,$7)`, core.NewID(), j.OrgID, kind, r.ID, r.Version, raw, requestID)
	return err
}
func (s *UploadService) RunRestoreJob(ctx context.Context, j jobqueue.Job, from, to objectstore.Store) error {
	in, err := decodeRestoreJob(j)
	if err != nil {
		return err
	}
	if from == nil || to == nil {
		return core.Fail(503, "STORAGE_NOT_CONFIGURED", "恢复用途身份未配置")
	}
	var current HistoryRestore
	var origin restoreOrigin
	var source objectstore.Source
	var transfer transferRow
	skip := false
	err = s.restoreTx(ctx, j, in, true, func(tx pgx.Tx, r HistoryRestore) error {
		if !restoreOwns(r, j, in) || restoreTerminal(r) {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		var e error
		source, origin, e = s.restoreAllowed(ctx, tx, j.OrgID, r, false)
		if e != nil {
			return e
		}
		t, e := scanTransfer(tx.QueryRow(ctx, "SELECT "+transferColumns+` FROM object_transfers WHERE org_id=$1 AND job_id=$2 AND kind='restore'
            AND restore_id=$3 AND state IN ('active','sealed','complete') ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, j.OrgID, j.ID, r.ID))
		if e == nil {
			if t.SourceDigest != r.SourceDigest {
				return core.Fail(409, "RESTORE_SOURCE_MISMATCH", "恢复检查点来源不一致")
			}
			transfer = t
			if _, e = tx.Exec(ctx, "UPDATE object_transfers SET fencing_token=$2,updated_at=clock_timestamp() WHERE id=$1", t.ID, j.FencingToken); e != nil {
				return e
			}
		} else if e == pgx.ErrNoRows {
			if _, e = tx.Exec(ctx, `UPDATE object_transfers SET state='uncertain',updated_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND kind='restore' AND state='starting'`, j.OrgID, j.ID); e != nil {
				return e
			}
			transfer = transferRow{ID: core.NewID(), FileID: r.FileID, VersionID: r.PlannedVersionID, SourceDigest: r.SourceDigest, State: "starting"}
			transfer.Key, _ = objectstore.ReplicaKey(j.OrgID, r.FileID, r.PlannedVersionID, transfer.ID)
			if _, e = tx.Exec(ctx, `INSERT INTO object_transfers(id,org_id,restore_id,job_id,file_id,version_id,source_digest,object_key,state,fencing_token,kind,target_provider,target_bucket)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,'starting',$9,'restore',$10,$11)`,
				transfer.ID, j.OrgID, r.ID, j.ID, r.FileID, r.PlannedVersionID, r.SourceDigest, transfer.Key, j.FencingToken, r.Provider, r.Bucket); e != nil {
				return e
			}
		} else {
			return e
		}
		before := digest(r)
		changed := r.State != "copying"
		if _, e = tx.Exec(ctx, `UPDATE file_history_restores SET active_job_id=$2,state='copying',last_error=NULL,
            version=version+CASE WHEN state<>'copying' THEN 1 ELSE 0 END,updated_at=clock_timestamp() WHERE id=$1`, r.ID, j.ID); e != nil {
			return e
		}
		current, e = historyRestore(ctx, tx, j.OrgID, r.ID, false)
		if e != nil {
			return e
		}
		if changed {
			return restoreWorkerEvent(ctx, tx, j, current, "file.history.restore.copying", before)
		}
		return nil
	})
	if err != nil || skip {
		return err
	}
	checkpoint := func(fn func(pgx.Tx) error) error {
		return s.restoreTx(ctx, j, in, true, func(tx pgx.Tx, r HistoryRestore) error {
			if !restoreOwns(r, j, in) || restoreTerminal(r) {
				return core.Fail(409, "RESTORE_STATE_CONFLICT", "恢复任务已结束")
			}
			now, _, e := s.restoreAllowed(ctx, tx, j.OrgID, r, false)
			if e != nil {
				return e
			}
			if now != source {
				return core.Fail(409, "RESTORE_SOURCE_MISMATCH", "恢复来源已改变")
			}
			var id string
			if e = tx.QueryRow(ctx, `SELECT id FROM object_transfers WHERE org_id=$1 AND id=$2 AND job_id=$3 AND fencing_token=$4 AND kind='restore'
                AND restore_id=$5 FOR UPDATE`, j.OrgID, transfer.ID, j.ID, j.FencingToken, r.ID).Scan(&id); e != nil {
				if e == pgx.ErrNoRows {
					return core.Fail(409, "JOB_LEASE_LOST", "恢复执行凭证已失效")
				}
				return e
			}
			return fn(tx)
		})
	}
	if transfer.UploadID == nil {
		m, e := to.Begin(ctx, transfer.Key, origin.MediaType)
		if e != nil {
			return e
		}
		if m.Key != transfer.Key || m.UploadID == "" {
			return core.Fail(409, "INVALID_PROVIDER_SOURCE", "恢复初始化回执无效")
		}
		if err = checkpoint(func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "UPDATE object_transfers SET provider_upload_id=$2,state='active',updated_at=clock_timestamp() WHERE id=$1", transfer.ID, m.UploadID)
			return e
		}); err != nil {
			return err
		}
		transfer.UploadID = &m.UploadID
	}
	layout, err := objectstore.Plan(restoreSize(current), objectstore.DefaultPartBytes)
	if err != nil {
		return err
	}
	state := objectstore.CopyState{Source: source, Target: objectstore.Multipart{Key: transfer.Key, UploadID: *transfer.UploadID}, Layout: layout, ExpectedSHA256: current.SHA256}
	if len(transfer.Manifest) > 0 {
		if err = json.Unmarshal(transfer.Manifest, &state.Manifest); err != nil {
			return err
		}
	}
	if len(transfer.Frozen) > 0 {
		state.Frozen = &objectstore.Source{}
		if err = json.Unmarshal(transfer.Frozen, state.Frozen); err != nil {
			return err
		}
	}
	if err = loadTransferHash(ctx, transfer.ID, &state, checkpoint); err != nil {
		return err
	}
	hooks := transferHooks(ctx, transfer.ID, j.FencingToken, checkpoint)
	frozenHook := hooks.Frozen
	hooks.Frozen = func(fixed objectstore.Source) error {
		if err := frozenHook(fixed); err != nil {
			return err
		}
		return checkpoint(func(tx pgx.Tx) error {
			r, e := historyRestore(ctx, tx, j.OrgID, current.ID, false)
			if e != nil {
				return e
			}
			if r.State == "verifying" {
				return nil
			}
			before := digest(r)
			if _, e = tx.Exec(ctx, "UPDATE file_history_restores SET state='verifying',version=version+1,updated_at=clock_timestamp() WHERE id=$1", r.ID); e != nil {
				return e
			}
			r, e = historyRestore(ctx, tx, j.OrgID, r.ID, false)
			if e != nil {
				return e
			}
			return restoreWorkerEvent(ctx, tx, j, r, "file.history.restore.verifying", before)
		})
	}
	_, evidence, err := objectstore.CopyFrozen(ctx, from, to, state, hooks)
	if err != nil {
		return err
	}
	return checkpoint(func(tx pgx.Tx) error {
		if evidence.SHA256 != current.SHA256 || evidence.Size != restoreSize(current) {
			return core.Fail(409, "CONTENT_DIGEST_MISMATCH", "恢复目标摘要不一致")
		}
		r, e := historyRestore(ctx, tx, j.OrgID, current.ID, false)
		if e != nil {
			return e
		}
		before := digest(r)
		if _, e = tx.Exec(ctx, `UPDATE file_history_restores SET state='ready',candidate_transfer_id=$2,version=version+1,
            updated_at=clock_timestamp() WHERE id=$1`, r.ID, transfer.ID); e != nil {
			return e
		}
		r, e = historyRestore(ctx, tx, j.OrgID, r.ID, false)
		if e != nil {
			return e
		}
		if e = restoreWorkerEvent(ctx, tx, j, r, "file.history.restore.ready", before); e != nil {
			return e
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}
func (s *UploadService) RecordRestoreFailure(ctx context.Context, j jobqueue.Job, code string, permanent bool, retryAfter time.Duration) error {
	in, err := decodeRestoreJob(j)
	if err != nil {
		return err
	}
	return s.restoreTx(ctx, j, in, true, func(tx pgx.Tx, r HistoryRestore) error {
		if !restoreOwns(r, j, in) || restoreTerminal(r) {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		state, kind := "retry_wait", "file.history.restore.retry_scheduled"
		if permanent || j.Attempt >= j.MaxAttempts {
			state, kind = "failed", "file.history.restore.failed"
		}
		before := digest(r)
		if _, err := tx.Exec(ctx, `UPDATE file_history_restores SET state=$2,last_error=$3,active_job_id=$4,
            version=version+1,updated_at=clock_timestamp() WHERE id=$1`, r.ID, state, code, j.ID); err != nil {
			return err
		}
		current, err := historyRestore(ctx, tx, j.OrgID, r.ID, false)
		if err != nil {
			return err
		}
		if err = restoreWorkerEvent(ctx, tx, j, current, kind, before); err != nil {
			return err
		}
		return s.Queue.FailInTx(ctx, tx, j, code, permanent, retryAfter)
	})
}
func (s *UploadService) ReconcileRestoreFailures(ctx context.Context, lane string) error {
	rows, err := s.Files.DB.Query(ctx, `SELECT j.id,j.org_id,j.kind,j.lane,j.payload_version,j.payload FROM jobs j
        JOIN file_history_restores r ON r.org_id=j.org_id AND j.payload->>'restore_id'=r.id::text
            AND (r.active_job_id=j.id OR (r.active_job_id IS NULL AND j.payload->>'restore_version'=r.version::text))
        WHERE j.kind='file.history.restore.v1' AND j.lane=$1 AND j.state='dead'
        AND r.state IN ('queued','copying','verifying','retry_wait') ORDER BY j.id LIMIT 20`, lane)
	if err != nil {
		return err
	}
	jobs := []jobqueue.Job{}
	for rows.Next() {
		var j jobqueue.Job
		if err = rows.Scan(&j.ID, &j.OrgID, &j.Kind, &j.Lane, &j.PayloadVersion, &j.Payload); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		in, e := decodeRestoreJob(j)
		if e != nil {
			return e
		}
		if e = s.restoreTx(ctx, j, in, false, func(tx pgx.Tx, r HistoryRestore) error {
			if !restoreOwns(r, j, in) || restoreTerminal(r) {
				return nil
			}
			before := digest(r)
			if _, err := tx.Exec(ctx, "UPDATE file_history_restores SET state='failed',last_error='ATTEMPTS_EXHAUSTED',version=version+1,updated_at=clock_timestamp() WHERE id=$1", r.ID); err != nil {
				return err
			}
			current, err := historyRestore(ctx, tx, j.OrgID, r.ID, false)
			if err != nil {
				return err
			}
			return restoreWorkerEvent(ctx, tx, j, current, "file.history.restore.failed", before)
		}); e != nil {
			return e
		}
	}
	return nil
}
