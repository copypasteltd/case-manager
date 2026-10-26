package files

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type HistoryRestoreCreate struct {
	ExpectedVersion int64  `json:"expected_version"`
	SourceVersionID string `json:"source_version_id"`
	Reason          string `json:"reason"`
}
type HistoryRestoreCommit struct {
	ExpectedVersion int64  `json:"expected_version"`
	BaseVersionID   string `json:"base_version_id"`
	SHA256          string `json:"sha256"`
}
type HistoryRestoreCancel struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type HistoryRestore struct {
	ID                                                              string    `json:"id"`
	Version                                                         int64     `json:"version"`
	CreatedAt                                                       time.Time `json:"created_at"`
	UpdatedAt                                                       time.Time `json:"updated_at"`
	FileID                                                          string    `json:"file_id"`
	SourceSeq                                                       int64     `json:"source_seq"`
	BaseSeq                                                         int64     `json:"base_seq"`
	SourceVersionID                                                 string    `json:"source_version_id"`
	BaseVersionID                                                   string    `json:"base_version_id"`
	ResultVersionID                                                 *string   `json:"result_version_id"`
	State                                                           string    `json:"state"`
	SizeBytes                                                       string    `json:"size_bytes"`
	SHA256                                                          string    `json:"sha256"`
	CopiedBytes                                                     string    `json:"copied_bytes"`
	VerifiedBytes                                                   string    `json:"verified_bytes"`
	ErrorCode                                                       *string   `json:"error_code"`
	Reason                                                          string    `json:"reason"`
	ExpiresAt                                                       time.Time `json:"expires_at"`
	OwnerID, FolderID, Classification, PlannedVersionID             string    `json:"-"`
	SourceReplicaID, Provider, Bucket, SourceDigest                 string    `json:"-"`
	OwnerAuthVersion, OrgAuthVersion, BaseFileVersion, StorageEpoch int64     `json:"-"`
	SourceChannel                                                   string    `json:"-"`
	FixedSource                                                     []byte    `json:"-"`
	ActiveJobID, CandidateTransferID                                *string   `json:"-"`
}

const restoreColumns = `r.id,r.version,r.created_at,r.updated_at,r.file_id,(SELECT seq FROM file_versions WHERE org_id=r.org_id AND id=r.source_version_id),
    (SELECT seq FROM file_versions WHERE org_id=r.org_id AND id=r.base_version_id),r.source_version_id,r.base_version_id,r.result_version_id,
    r.state,r.size_bytes::text,r.sha256,
    coalesce((SELECT sum(p.size_bytes) FROM object_transfer_parts p WHERE p.transfer_id=(
        SELECT t.id FROM object_transfers t WHERE t.org_id=r.org_id AND t.restore_id=r.id ORDER BY t.created_at DESC,t.id DESC LIMIT 1)),0)::text,
    coalesce((SELECT h.completed_bytes FROM object_transfer_hashes h WHERE h.transfer_id=(
        SELECT t.id FROM object_transfers t WHERE t.org_id=r.org_id AND t.restore_id=r.id ORDER BY t.created_at DESC,t.id DESC LIMIT 1)),0)::text,
    r.last_error,r.reason,r.expires_at,r.owner_id,r.folder_id,r.classification,r.planned_version_id,
    r.source_replica_id,r.source_provider,r.source_bucket,r.source_digest,r.owner_auth_version,r.org_auth_version,
    r.base_file_version,r.storage_epoch,r.source_channel,r.fixed_source,r.active_job_id,r.candidate_transfer_id`

func scanRestore(row pgx.Row) (HistoryRestore, error) {
	var r HistoryRestore
	err := row.Scan(&r.ID, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.FileID, &r.SourceSeq, &r.BaseSeq, &r.SourceVersionID, &r.BaseVersionID, &r.ResultVersionID,
		&r.State, &r.SizeBytes, &r.SHA256, &r.CopiedBytes, &r.VerifiedBytes, &r.ErrorCode, &r.Reason, &r.ExpiresAt,
		&r.OwnerID, &r.FolderID, &r.Classification, &r.PlannedVersionID, &r.SourceReplicaID, &r.Provider, &r.Bucket, &r.SourceDigest,
		&r.OwnerAuthVersion, &r.OrgAuthVersion, &r.BaseFileVersion, &r.StorageEpoch, &r.SourceChannel, &r.FixedSource, &r.ActiveJobID, &r.CandidateTransferID)
	return r, platform.DatabaseError(err)
}
func historyRestore(ctx context.Context, db platform.DBTX, org, id string, lock bool) (HistoryRestore, error) {
	if !core.ValidID(id) {
		return HistoryRestore{}, core.Missing()
	}
	q := "SELECT " + restoreColumns + " FROM file_history_restores r WHERE r.org_id=$1 AND r.id=$2"
	if lock {
		q += " FOR UPDATE OF r"
	}
	return scanRestore(db.QueryRow(ctx, q, org, id))
}
func restoreAccess(ctx context.Context, db platform.DBTX, a core.Actor, id string) (File, error) {
	f, err := authorizeFile(ctx, db, a, id, "file.read")
	if err != nil {
		return f, err
	}
	return f, authz.Require(ctx, db, a, "file.restore", fileResource(f))
}
func (s *UploadService) authorizeRestore(ctx context.Context, db platform.DBTX, a core.Actor, id string) (HistoryRestore, error) {
	var r HistoryRestore
	if err := currentDownloadActor(ctx, db, a); err != nil {
		return r, err
	}
	r, err := historyRestore(ctx, db, a.OrgID, id, false)
	if err != nil {
		return r, err
	}
	if r.OwnerID != a.UserID {
		return r, core.Missing()
	}
	_, err = restoreAccess(ctx, db, a, r.FileID)
	return r, err
}
func (s *UploadService) HistoryRestore(ctx context.Context, a core.Actor, id string) (HistoryRestore, error) {
	return s.authorizeRestore(ctx, s.Files.DB, a, id)
}
func (s *UploadService) HistoryRestores(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[HistoryRestore], error) {
	out := core.Page[HistoryRestore]{Items: []HistoryRestore{}}
	if err := currentDownloadActor(ctx, s.Files.DB, a); err != nil {
		return out, err
	}
	if _, err := restoreAccess(ctx, s.Files.DB, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(actorCursor(a) + "/history-restores/" + id))
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := s.Files.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.Files.DB.Query(ctx, "SELECT "+restoreColumns+` FROM file_history_restores r WHERE r.org_id=$1 AND r.file_id=$2 AND r.owner_id=$3
        AND ($4::timestamptz IS NULL OR (r.created_at,r.id)<($4,$5::uuid)) ORDER BY r.created_at DESC,r.id DESC LIMIT $6`,
		a.OrgID, id, a.UserID, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		r, e := scanRestore(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := s.Files.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}

// Scan authority belongs to the immutable original report, not to the latest
// mutable upload status. An inherited waiver must still be valid for publication.
type restoreOrigin struct {
	ID, MediaType, ScanState, SHA256 string
	Size                             int64
	ReportID, WaiverID               *string
}

func restoreOriginEvidence(ctx context.Context, db platform.DBTX, org, fileID, id, classification string) (restoreOrigin, error) {
	var v restoreOrigin
	err := db.QueryRow(ctx, `SELECT id,media_type,scan_state,sha256,size_bytes,scan_report_ref,scan_waiver_ref
        FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3 AND availability='available'`, org, fileID, id).
		Scan(&v.ID, &v.MediaType, &v.ScanState, &v.SHA256, &v.Size, &v.ReportID, &v.WaiverID)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	var valid bool
	if v.ScanState == "clean" && v.ReportID != nil {
		err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM upload_scan_reports WHERE org_id=$1 AND id=$2 AND sha256=$3
            AND size_bytes=$4 AND state='clean' AND code='SCAN_PASSED' AND streamed_bytes=size_bytes)`,
			org, *v.ReportID, v.SHA256, v.Size).Scan(&valid)
	} else if v.ScanState == "waived" && v.ReportID != nil && v.WaiverID != nil {
		err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_waivers w
            JOIN upload_scan_reports r ON r.org_id=w.org_id AND r.id=w.report_id
            JOIN scan_waiver_policies p ON p.org_id=w.org_id AND p.id=w.policy_id AND p.version=w.policy_version
            JOIN scan_waiver_decisions d ON d.org_id=w.org_id AND d.waiver_id=w.id AND d.decision='approve'
                AND d.actor_id=w.reviewer_id AND d.acknowledged_risk AND d.mfa_verified_at IS NOT NULL
            WHERE w.org_id=$1 AND w.id=$2 AND w.report_id=$3 AND w.sha256=$4 AND w.size_bytes=$5 AND w.classification=$6
            AND w.state='approved' AND w.owner_id<>w.reviewer_id AND w.expires_at>clock_timestamp() AND p.enabled
            AND NOT EXISTS(SELECT 1 FROM scan_waiver_policies newer WHERE newer.org_id=p.org_id AND newer.classification=p.classification AND newer.version>p.version)
            AND r.state='unscannable' AND r.sha256=w.sha256 AND r.size_bytes=w.size_bytes AND r.source_digest=w.source_digest)`,
			org, *v.WaiverID, *v.ReportID, v.SHA256, v.Size, classification).Scan(&valid)
	}
	if err != nil {
		return v, err
	}
	if !valid || v.Size < 1 || v.Size > objectstore.MaxObjectBytes {
		return v, core.Fail(409, "RESTORE_SCAN_EVIDENCE_REQUIRED", "来源缺少完整扫描依据，或原例外已失效，需先重新核验")
	}
	return v, nil
}
func reserveFileCapacity(ctx context.Context, tx pgx.Tx, org string, size int64) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", org+"/upload-quota"); err != nil {
		return err
	}
	var quota, used, reserved int64
	err := tx.QueryRow(ctx, "SELECT logical_limit_bytes FROM storage_quotas WHERE org_id=$1", org).Scan(&quota)
	if err == pgx.ErrNoRows {
		return core.Fail(503, "STORAGE_QUOTA_NOT_CONFIGURED", "组织尚未设置文件容量")
	}
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT coalesce((SELECT sum(size_bytes) FROM file_versions WHERE org_id=$1),0)::bigint,
        (coalesce((SELECT sum(size_bytes) FROM upload_sessions WHERE org_id=$1 AND state IN ('created','uploading','finalizing','verifying','blocked','ready')
            AND expires_at>clock_timestamp()),0)+coalesce((SELECT sum(size_bytes) FROM file_history_restores WHERE org_id=$1
            AND state IN ('queued','copying','verifying','retry_wait','ready') AND expires_at>clock_timestamp()),0))::bigint`, org).Scan(&used, &reserved)
	if err != nil {
		return err
	}
	if used > quota || reserved > quota-used || size > quota-used-reserved {
		return core.Fail(409, "STORAGE_QUOTA_EXCEEDED", "文件历史、上传与恢复预留已超出容量")
	}
	return nil
}
func (s *UploadService) CreateHistoryRestore(ctx context.Context, c core.Command, id string, in HistoryRestoreCreate) (core.Result, error) {
	if in.ExpectedVersion < 1 || !core.ValidID(in.SourceVersionID) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请提供文件版本、来源版本及恢复原因")
	}
	authorize := func(tx pgx.Tx) error {
		if err := currentDownloadActor(ctx, tx, c.Actor); err != nil {
			return err
		}
		_, err := restoreAccess(ctx, tx, c.Actor, id)
		return err
	}
	return s.Files.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		f, err := file(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = namespaceLock(ctx, tx, c.Actor, f.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		f, err = file(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if f.State != "active" || f.CurrentVersionID == nil {
			return core.Result{}, core.Fail(409, "FILE_NOT_ACTIVE", "请先恢复回收站文件")
		}
		if f.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if *f.CurrentVersionID == in.SourceVersionID {
			return core.Result{}, core.Fail(409, "RESTORE_SOURCE_IS_CURRENT", "当前内容已是该版本，无需恢复")
		}
		origin, err := restoreOriginEvidence(ctx, tx, c.Actor.OrgID, id, in.SourceVersionID, f.Classification)
		if err != nil {
			return core.Result{}, err
		}
		epoch, provider, err := currentEpoch(ctx, tx, c.Actor.OrgID)
		if err != nil {
			return core.Result{}, err
		}
		if s.Backend == nil || s.Backend.Provider != provider {
			return core.Result{}, core.Fail(503, "STORAGE_NOT_CONFIGURED", "历史恢复主端尚未配置")
		}
		var replicaID string
		err = tx.QueryRow(ctx, `SELECT id FROM file_replicas WHERE org_id=$1 AND version_id=$2 AND provider=$3 AND bucket=$4 AND state='verified'
            AND expected_sha256=$5 AND actual_sha256=$5 AND verified_at IS NOT NULL AND source_id IS NOT NULL ORDER BY id LIMIT 1`,
			c.Actor.OrgID, origin.ID, provider, s.Backend.Bucket, origin.SHA256).Scan(&replicaID)
		if err == pgx.ErrNoRows {
			return core.Result{}, core.Fail(409, "RESTORE_SOURCE_UNAVAILABLE", "当前主端没有可核验的历史副本")
		}
		if err != nil {
			return core.Result{}, err
		}
		r, err := replica(ctx, tx, c.Actor.OrgID, replicaID, false)
		if err != nil {
			return core.Result{}, err
		}
		var fixed objectstore.Source
		v := downloadVersionRow{File: f, ID: origin.ID, Size: origin.Size, SHA256: origin.SHA256}
		if !downloadSourceEvidence(c.Actor.OrgID, v, r, &fixed) {
			return core.Result{}, core.Fail(409, "RESTORE_SOURCE_MISMATCH", "历史副本来源证据不完整")
		}
		if err = restoreTargets(ctx, tx, c.Actor.OrgID, provider, r.Bucket); err != nil {
			return core.Result{}, err
		}
		if err = reserveFileCapacity(ctx, tx, c.Actor.OrgID, origin.Size); err != nil {
			return core.Result{}, err
		}
		raw, _ := json.Marshal(fixed)
		restoreID := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO file_history_restores(id,org_id,file_id,owner_id,owner_auth_version,org_auth_version,source_channel,
            folder_id,classification,base_file_version,base_version_id,source_version_id,planned_version_id,source_replica_id,source_provider,
            source_bucket,fixed_source,source_digest,sha256,size_bytes,storage_epoch,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
			restoreID, c.Actor.OrgID, id, c.Actor.UserID, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, c.Actor.Channel, f.FolderID, f.Classification,
			f.Version, *f.CurrentVersionID, origin.ID, core.NewID(), replicaID, provider, r.Bucket, raw, core.Digest(raw), origin.SHA256, origin.Size, epoch, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		current, err := historyRestore(ctx, tx, c.Actor.OrgID, restoreID, false)
		return core.Result{Status: 202, Data: current, Event: restoreRequested(current)}, err
	})
}
func restoreTargets(ctx context.Context, db platform.DBTX, org, provider, bucket string) error {
	var count int
	err := db.QueryRow(ctx, `SELECT count(*) FROM storage_targets WHERE org_id=$1 AND enabled
        AND ((provider=$2 AND bucket=$3) OR provider<>$2)`, org, provider, bucket).Scan(&count)
	if err != nil {
		return err
	}
	if count != 2 {
		return core.Fail(503, "STORAGE_TARGETS_NOT_CONFIGURED", "恢复所需双云目标尚未配置")
	}
	return nil
}
func (s *UploadService) CancelHistoryRestore(ctx context.Context, c core.Command, id string, in HistoryRestoreCancel) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写取消原因")
	}
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, err := s.authorizeRestore(ctx, tx, c.Actor, id); return err },
		func(tx pgx.Tx) (core.Result, error) {
			r, err := historyRestore(ctx, tx, c.Actor.OrgID, id, false)
			if err != nil {
				return core.Result{}, err
			}
			f, err := file(ctx, tx, c.Actor.OrgID, r.FileID, false)
			if err != nil {
				return core.Result{}, err
			}
			if err = replicaNamespace(ctx, tx, c.Actor.OrgID, f.ProjectID); err != nil {
				return core.Result{}, err
			}
			if _, err = s.authorizeRestore(ctx, tx, c.Actor, id); err != nil {
				return core.Result{}, err
			}
			r, err = historyRestore(ctx, tx, c.Actor.OrgID, id, true)
			if err != nil {
				return core.Result{}, err
			}
			if r.Version != in.ExpectedVersion {
				return core.Result{}, core.Conflict()
			}
			if r.State == "committed" || r.State == "cancelled" || r.State == "failed" {
				return core.Result{}, core.Fail(409, "RESTORE_STATE_CONFLICT", "任务已结束")
			}
			before := digest(r)
			if _, err = tx.Exec(ctx, "UPDATE file_history_restores SET state='cancelled',version=version+1,updated_at=clock_timestamp() WHERE id=$1", id); err != nil {
				return core.Result{}, err
			}
			current, err := historyRestore(ctx, tx, c.Actor.OrgID, id, false)
			return core.Result{Data: current, Event: core.Event{Type: "file.history.restore.cancelled", ResourceType: "file_history_restore", ResourceID: id, Version: current.Version,
				BeforeDigest: before, Payload: map[string]any{"file_id": r.FileID, "reason": in.Reason}}}, err
		})
}
func restoreSize(r HistoryRestore) int64 { n, _ := strconv.ParseInt(r.SizeBytes, 10, 64); return n }
