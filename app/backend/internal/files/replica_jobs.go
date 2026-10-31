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

const ReplicaSmallLane = "storage.replica.small"
const ReplicaLargeLane = "storage.replica.large"

var ReplicaJobKinds = []string{"file.replicate.v1"}

func ReplicaLane(size int64) string {
	if size <= smallFileLimit {
		return ReplicaSmallLane
	}
	return ReplicaLargeLane
}

type replicaJobInput struct {
	ReplicaID      string `json:"replica_id"`
	FileID         string `json:"file_id"`
	VersionID      string `json:"version_id"`
	ReplicaVersion int64  `json:"replica_version"`
	StorageEpoch   int64  `json:"storage_epoch"`
	SourceProvider string `json:"source_provider"`
	TargetProvider string `json:"target_provider"`
	SHA256         string `json:"sha256"`
	SizeBytes      string `json:"size_bytes"`
}

func validReplicaInput(in replicaJobInput, lane string) bool {
	size, err := strconv.ParseInt(in.SizeBytes, 10, 64)
	providers := (in.SourceProvider == "oss" && in.TargetProvider == "r2") || (in.SourceProvider == "r2" && in.TargetProvider == "oss")
	return err == nil && strconv.FormatInt(size, 10) == in.SizeBytes && size > 0 && size <= objectstore.MaxObjectBytes &&
		core.ValidID(in.ReplicaID) && core.ValidID(in.FileID) && core.ValidID(in.VersionID) &&
		in.ReplicaVersion > 0 && in.StorageEpoch > 0 && providers && shaPattern.MatchString(in.SHA256) && lane == ReplicaLane(size)
}
func ReplicaConsumer() jobqueue.Consumer {
	return jobqueue.Consumer{Name: "file.replication.v1", EventTypes: []string{"upload.committed", "replica.retry.requested", "file.history.restore.committed"}, SchemaVersion: 1,
		Build: func(e jobqueue.Event) (jobqueue.Input, error) {
			var event struct {
				replicaJobInput
				SessionID string `json:"session_id,omitempty"`
				RestoreID string `json:"restore_id,omitempty"`
				Reason    string `json:"reason,omitempty"`
			}
			d := json.NewDecoder(bytes.NewReader(e.Payload))
			d.DisallowUnknownFields()
			if err := d.Decode(&event); err != nil {
				return jobqueue.Input{}, err
			}
			size, _ := strconv.ParseInt(event.SizeBytes, 10, 64)
			if d.Decode(new(any)) != io.EOF || !validReplicaInput(event.replicaJobInput, ReplicaLane(size)) {
				return jobqueue.Input{}, errors.New("invalid replica event input")
			}
			if (e.Type == "upload.committed" && (e.AggregateType != "upload" || !core.ValidID(event.SessionID) || event.SessionID != e.AggregateID)) ||
				(e.Type == "replica.retry.requested" && (e.AggregateType != "file_replica" || event.ReplicaID != e.AggregateID || event.ReplicaVersion != e.AggregateVersion)) ||
				(e.Type == "file.history.restore.committed" && (e.AggregateType != "file_history_restore" || !core.ValidID(event.RestoreID) || event.RestoreID != e.AggregateID)) ||
				(e.Type != "upload.committed" && e.Type != "replica.retry.requested" && e.Type != "file.history.restore.committed") {
				return jobqueue.Input{}, errors.New("invalid replica event reference")
			}
			raw, _ := json.Marshal(event.replicaJobInput)
			return jobqueue.Input{OrgID: e.OrgID, Kind: "file.replicate.v1", Lane: ReplicaLane(size),
				DedupKey:       fmt.Sprintf("file.replicate.v1/%s/%d", event.ReplicaID, event.ReplicaVersion),
				PayloadVersion: 1, Payload: raw, MaxAttempts: 8}, nil
		}}
}
func decodeReplicaJob(j jobqueue.Job) (replicaJobInput, error) {
	var in replicaJobInput
	d := json.NewDecoder(bytes.NewReader(j.Payload))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil || d.Decode(new(any)) != io.EOF || j.PayloadVersion != 1 || j.Kind != "file.replicate.v1" || !validReplicaInput(in, j.Lane) {
		return in, core.Fail(409, "INVALID_JOB_INPUT", "副本任务固定输入无效")
	}
	return in, nil
}

type replicaRow struct {
	Replica
	VersionID, FileID, Bucket, Key, Expected, SHA256, MediaType, AuthorID string
	Size                                                                  int64
	SourceRef, ActiveJob                                                  *string
	ProjectID                                                             *string
}

func replica(ctx context.Context, db platform.DBTX, org, id string, lock bool) (replicaRow, error) {
	var r replicaRow
	if !core.ValidID(id) {
		return r, core.Missing()
	}
	query := `SELECT r.id,r.version,r.provider,r.state,r.actual_sha256,r.verified_at,r.updated_at,r.last_error,
        r.version_id,v.file_id,r.bucket,r.object_key,r.expected_sha256,v.sha256,v.media_type,v.author_id,
        v.size_bytes,r.source_id,r.active_job_id,d.project_id
        FROM file_replicas r JOIN file_versions v ON v.org_id=r.org_id AND v.id=r.version_id
        JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
        JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE r.org_id=$1 AND r.id=$2`
	if lock {
		query += " FOR UPDATE OF r"
	}
	err := db.QueryRow(ctx, query, org, id).Scan(&r.ID, &r.Version, &r.Provider, &r.State, &r.ActualSHA256, &r.VerifiedAt, &r.UpdatedAt, &r.ErrorCode,
		&r.VersionID, &r.FileID, &r.Bucket, &r.Key, &r.Expected, &r.SHA256, &r.MediaType, &r.AuthorID,
		&r.Size, &r.SourceRef, &r.ActiveJob, &r.ProjectID)
	return r, platform.DatabaseError(err)
}

// Backup maintenance deliberately ignores project writeability, trash and the
// original employee's status. Its authority is the retained committed version.
func replicaNamespace(ctx context.Context, tx pgx.Tx, org string, project *string) error {
	if project == nil {
		_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", org+"/company-files")
		return err
	}
	var id string
	return tx.QueryRow(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", org, *project).Scan(&id)
}
func (s *UploadService) replicaTx(ctx context.Context, j jobqueue.Job, in replicaJobInput, leased bool, fn func(pgx.Tx, replicaRow) error) error {
	tx, err := s.Files.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&org); err != nil {
		return err
	}
	r, err := replica(ctx, tx, j.OrgID, in.ReplicaID, false)
	if err != nil {
		return err
	}
	if err = replicaNamespace(ctx, tx, j.OrgID, r.ProjectID); err != nil {
		return err
	}
	r, err = replica(ctx, tx, j.OrgID, in.ReplicaID, true)
	if err != nil {
		return err
	}
	if r.FileID != in.FileID || r.VersionID != in.VersionID || r.Provider != in.TargetProvider || r.SHA256 != in.SHA256 ||
		r.Expected != in.SHA256 || strconv.FormatInt(r.Size, 10) != in.SizeBytes {
		return core.Fail(409, "REPLICA_INPUT_CHANGED", "副本固定输入与正式版本不一致")
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
func replicaOwns(r replicaRow, j jobqueue.Job, in replicaJobInput) bool {
	return (r.ActiveJob == nil && r.Version == in.ReplicaVersion) || (r.ActiveJob != nil && *r.ActiveJob == j.ID)
}
func (s *UploadService) replicaAllowed(ctx context.Context, tx pgx.Tx, j jobqueue.Job, r replicaRow, in replicaJobInput) (objectstore.Source, error) {
	var source objectstore.Source
	var availability string
	if err := tx.QueryRow(ctx, "SELECT availability FROM file_versions WHERE org_id=$1 AND id=$2 FOR SHARE", j.OrgID, r.VersionID).Scan(&availability); err != nil {
		return source, err
	}
	if availability == "retiring" || availability == "purged" {
		return source, core.Fail(409, "ORIGINAL_RETIRED", "已停用的历史原件不能产生新副本")
	}
	if !replicaOwns(r, j, in) || r.State == "verified" || r.State == "failed" || r.State == "missing" {
		return source, core.Fail(409, "REPLICA_STATE_CONFLICT", "副本任务阶段已改变")
	}
	epoch, provider, err := currentEpoch(ctx, tx, j.OrgID)
	if err != nil {
		return source, err
	}
	if epoch != in.StorageEpoch || provider != in.SourceProvider {
		return source, core.Fail(409, "STORAGE_EPOCH_CHANGED", "副本任务存储纪元已改变")
	}
	if s.Backend == nil || s.ReplicaTarget == nil || s.Backend.Provider != in.SourceProvider || s.ReplicaTarget.Provider != r.Provider ||
		s.ReplicaTarget.Bucket != r.Bucket {
		return source, core.Fail(503, "STORAGE_CONFIGURATION_CHANGED", "副本执行器用途与固定目标不一致")
	}
	var sourceBucket, sourceKey string
	var sourceRef *string
	err = tx.QueryRow(ctx, `SELECT bucket,object_key,source_id FROM file_replicas
        WHERE org_id=$1 AND version_id=$2 AND provider=$3 AND state='verified' AND expected_sha256=$4
        AND actual_sha256=$4 AND verified_at IS NOT NULL ORDER BY id LIMIT 1`, j.OrgID, r.VersionID, in.SourceProvider, r.SHA256).Scan(&sourceBucket, &sourceKey, &sourceRef)
	if err == pgx.ErrNoRows {
		return source, core.Fail(409, "REPLICA_SOURCE_UNAVAILABLE", "没有已验证的固定源副本")
	}
	if err != nil {
		return source, err
	}
	if sourceRef == nil || json.Unmarshal([]byte(*sourceRef), &source) != nil || source.Key != sourceKey || source.Size != r.Size || source.ETag == "" ||
		sourceBucket != s.Backend.Bucket {
		return source, core.Fail(409, "REPLICA_SOURCE_MISMATCH", "副本来源证据不完整")
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM storage_targets WHERE org_id=$1 AND enabled
        AND ((provider=$2 AND bucket=$3) OR (provider=$4 AND bucket=$5))`, j.OrgID, in.SourceProvider, sourceBucket, r.Provider, r.Bucket).Scan(&count); err != nil {
		return source, err
	}
	if count != 2 {
		return source, core.Fail(503, "STORAGE_TARGETS_NOT_CONFIGURED", "双云目标配置与固定副本不一致")
	}
	return source, nil
}
func replicaWorkerEvent(ctx context.Context, tx pgx.Tx, j jobqueue.Job, r replicaRow, kind, before string) error {
	raw, _ := json.Marshal(map[string]any{"replica_id": r.ID, "file_id": r.FileID, "version_id": r.VersionID, "provider": r.Provider, "state": r.State, "job_id": j.ID})
	requestID := "job/" + j.ID
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,job_id,action,resource_type,resource_id,resource_version,before_digest,after_digest,result)
        VALUES($1,$2,$3,NULL,'worker',$4,$5,$6,'file_replica',$7,$8,$9,$10,'succeeded')`, core.NewID(), j.OrgID, r.AuthorID, requestID, j.ID, kind, r.ID, r.Version, before, digest(r.Replica)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
        VALUES($1,$2,$3,'file_replica',$4,$5,$6,$7)`, core.NewID(), j.OrgID, kind, r.ID, r.Version, raw, requestID)
	return err
}
func (s *UploadService) RunReplicaJob(ctx context.Context, j jobqueue.Job, from, to objectstore.Store) error {
	in, err := decodeReplicaJob(j)
	if err != nil {
		return err
	}
	if from == nil || to == nil {
		return core.Fail(503, "STORAGE_NOT_CONFIGURED", "副本执行器尚未配置")
	}
	var current replicaRow
	var source objectstore.Source
	var transfer transferRow
	skip := false
	err = s.replicaTx(ctx, j, in, true, func(tx pgx.Tx, r replicaRow) error {
		if !replicaOwns(r, j, in) || r.State == "verified" || r.State == "failed" || r.State == "missing" {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		var e error
		source, e = s.replicaAllowed(ctx, tx, j, r, in)
		if e != nil {
			return e
		}
		sourceRaw, _ := json.Marshal(source)
		sourceDigest := core.Digest(sourceRaw)
		t, e := scanTransfer(tx.QueryRow(ctx, "SELECT "+transferColumns+" FROM object_transfers WHERE org_id=$1 AND job_id=$2 AND kind='replica' AND replica_id=$3 AND state IN ('active','sealed','complete') ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE", j.OrgID, j.ID, r.ID))
		if e == nil {
			if t.SourceDigest != sourceDigest {
				return core.Fail(409, "REPLICA_SOURCE_CHANGED", "副本复制来源已改变")
			}
			transfer = t
			if _, e = tx.Exec(ctx, "UPDATE object_transfers SET fencing_token=$2,updated_at=clock_timestamp() WHERE id=$1", t.ID, j.FencingToken); e != nil {
				return e
			}
		} else if e == pgx.ErrNoRows {
			if _, e = tx.Exec(ctx, "UPDATE object_transfers SET state='uncertain',updated_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND kind='replica' AND state='starting'", j.OrgID, j.ID); e != nil {
				return e
			}
			transfer = transferRow{ID: core.NewID(), FileID: r.FileID, VersionID: r.VersionID, State: "starting", SourceDigest: sourceDigest}
			transfer.Key, _ = objectstore.ReplicaKey(j.OrgID, r.FileID, r.VersionID, transfer.ID)
			if _, e = tx.Exec(ctx, `INSERT INTO object_transfers(id,org_id,replica_id,job_id,file_id,version_id,source_digest,object_key,state,fencing_token,kind,target_provider,target_bucket)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,'starting',$9,'replica',$10,$11)`, transfer.ID, j.OrgID, r.ID, j.ID, r.FileID, r.VersionID, sourceDigest, transfer.Key, j.FencingToken, r.Provider, r.Bucket); e != nil {
				return e
			}
		} else {
			return e
		}
		before := digest(r.Replica)
		changed := r.State != "copying"
		if _, e = tx.Exec(ctx, `UPDATE file_replicas SET active_job_id=$2,object_key=$3,state='copying',last_error=NULL,
            version=version+CASE WHEN state<>'copying' THEN 1 ELSE 0 END,updated_at=clock_timestamp() WHERE id=$1`, r.ID, j.ID, transfer.Key); e != nil {
			return e
		}
		current, e = replica(ctx, tx, j.OrgID, r.ID, false)
		if e != nil {
			return e
		}
		if changed {
			return replicaWorkerEvent(ctx, tx, j, current, "replica.copy.started", before)
		}
		return nil
	})
	if err != nil || skip {
		return err
	}
	checkpoint := func(fn func(pgx.Tx) error) error {
		return s.replicaTx(ctx, j, in, true, func(tx pgx.Tx, r replicaRow) error {
			now, e := s.replicaAllowed(ctx, tx, j, r, in)
			if e != nil {
				return e
			}
			if now != source {
				return core.Fail(409, "REPLICA_SOURCE_CHANGED", "副本复制来源已改变")
			}
			var id string
			if e = tx.QueryRow(ctx, `SELECT id FROM object_transfers WHERE org_id=$1 AND id=$2 AND job_id=$3 AND fencing_token=$4
                AND kind='replica' AND replica_id=$5 FOR UPDATE`, j.OrgID, transfer.ID, j.ID, j.FencingToken, r.ID).Scan(&id); e != nil {
				if e == pgx.ErrNoRows {
					return core.Fail(409, "JOB_LEASE_LOST", "副本执行凭证已失效")
				}
				return e
			}
			return fn(tx)
		})
	}
	if transfer.UploadID == nil {
		multipart, e := to.Begin(ctx, transfer.Key, current.MediaType)
		if e != nil {
			return e
		}
		if multipart.Key != transfer.Key || multipart.UploadID == "" {
			return core.Fail(409, "INVALID_PROVIDER_SOURCE", "副本初始化回执无效")
		}
		if err = checkpoint(func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "UPDATE object_transfers SET provider_upload_id=$2,state='active',updated_at=clock_timestamp() WHERE id=$1", transfer.ID, multipart.UploadID)
			return e
		}); err != nil {
			return err
		}
		transfer.UploadID = &multipart.UploadID
	}
	layout, err := objectstore.Plan(current.Size, objectstore.DefaultPartBytes)
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
			r, e := replica(ctx, tx, j.OrgID, current.ID, false)
			if e != nil {
				return e
			}
			if r.State == "verifying" {
				return nil
			}
			before := digest(r.Replica)
			if _, e = tx.Exec(ctx, "UPDATE file_replicas SET state='verifying',version=version+1,updated_at=clock_timestamp() WHERE id=$1", r.ID); e != nil {
				return e
			}
			r, e = replica(ctx, tx, j.OrgID, r.ID, false)
			if e != nil {
				return e
			}
			return replicaWorkerEvent(ctx, tx, j, r, "replica.verification.started", before)
		})
	}
	fixed, evidence, err := objectstore.CopyFrozen(ctx, from, to, state, hooks)
	if err != nil {
		return err
	}
	return checkpoint(func(tx pgx.Tx) error {
		if evidence.SHA256 != current.SHA256 || evidence.Size != current.Size {
			return core.Fail(409, "CONTENT_DIGEST_MISMATCH", "副本摘要不一致")
		}
		r, e := replica(ctx, tx, j.OrgID, current.ID, false)
		if e != nil {
			return e
		}
		before := digest(r.Replica)
		raw, _ := json.Marshal(fixed)
		if _, e = tx.Exec(ctx, `UPDATE file_replicas SET state='verified',object_key=$2,source_id=$3,actual_sha256=$4,verified_at=clock_timestamp(),
            last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1`, r.ID, fixed.Key, string(raw), evidence.SHA256); e != nil {
			return e
		}
		r, e = replica(ctx, tx, j.OrgID, r.ID, false)
		if e != nil {
			return e
		}
		if e = replicaWorkerEvent(ctx, tx, j, r, "replica.verified", before); e != nil {
			return e
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}

func (s *UploadService) RecordReplicaFailure(ctx context.Context, j jobqueue.Job, code string, permanent bool, retryAfter time.Duration) error {
	in, err := decodeReplicaJob(j)
	if err != nil {
		return err
	}
	return s.replicaTx(ctx, j, in, true, func(tx pgx.Tx, r replicaRow) error {
		if !replicaOwns(r, j, in) || r.State == "verified" || r.State == "failed" || r.State == "missing" {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		before := digest(r.Replica)
		state := "retry_wait"
		kind := "replica.retry.scheduled"
		if permanent || j.Attempt >= j.MaxAttempts {
			state = "failed"
			kind = "replica.failed"
		}
		if _, err = tx.Exec(ctx, `UPDATE file_replicas SET state=$2,last_error=$3,active_job_id=$4,version=version+1,updated_at=clock_timestamp()
            WHERE id=$1`, r.ID, state, code, j.ID); err != nil {
			return err
		}
		r, err = replica(ctx, tx, j.OrgID, r.ID, false)
		if err != nil {
			return err
		}
		if err = replicaWorkerEvent(ctx, tx, j, r, kind, before); err != nil {
			return err
		}
		return s.Queue.FailInTx(ctx, tx, j, code, permanent, retryAfter)
	})
}

// A process can die on its final lease before recording a domain failure.
// Reconcile only exhausted jobs, in bounded batches, with the usual lock order.
func (s *UploadService) ReconcileReplicaFailures(ctx context.Context, lane string) error {
	rows, err := s.Files.DB.Query(ctx, `SELECT j.id,j.org_id,j.kind,j.lane,j.payload_version,j.payload
        FROM file_replicas r JOIN jobs j ON j.org_id=r.org_id AND j.payload->>'replica_id'=r.id::text
            AND (j.id=r.active_job_id OR (r.active_job_id IS NULL AND j.payload->>'replica_version'=r.version::text))
        WHERE j.state='dead' AND j.kind='file.replicate.v1' AND j.lane=$1 AND r.state NOT IN ('failed','verified','missing')
        ORDER BY j.updated_at,j.id LIMIT 20`, lane)
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
		in, e := decodeReplicaJob(j)
		if e != nil {
			return e
		}
		if e = s.replicaTx(ctx, j, in, false, func(tx pgx.Tx, r replicaRow) error {
			if !replicaOwns(r, j, in) || r.State == "failed" || r.State == "verified" || r.State == "missing" {
				return nil
			}
			before := digest(r.Replica)
			if _, e := tx.Exec(ctx, `UPDATE file_replicas SET state='failed',last_error='ATTEMPTS_EXHAUSTED',active_job_id=$2,version=version+1,
                updated_at=clock_timestamp() WHERE id=$1`, r.ID, j.ID); e != nil {
				return e
			}
			r, e := replica(ctx, tx, j.OrgID, r.ID, false)
			if e != nil {
				return e
			}
			return replicaWorkerEvent(ctx, tx, j, r, "replica.failed", before)
		}); e != nil {
			return e
		}
	}
	return nil
}
