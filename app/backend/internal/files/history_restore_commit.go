package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

func (s *UploadService) CommitHistoryRestore(ctx context.Context, c core.Command, id string, in HistoryRestoreCommit) (core.Result, error) {
	if in.ExpectedVersion < 1 || !core.ValidID(in.BaseVersionID) || !shaPattern.MatchString(in.SHA256) {
		return core.Result{}, core.Invalid("base_version_id", "请确认恢复任务版本、基础版本与摘要")
	}
	authorize := func(tx pgx.Tx) error { _, err := s.authorizeRestore(ctx, tx, c.Actor, id); return err }
	return s.Files.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, err := historyRestore(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		f, err := file(ctx, tx, c.Actor.OrgID, r.FileID, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = namespaceLock(ctx, tx, c.Actor, f.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		f, err = file(ctx, tx, c.Actor.OrgID, r.FileID, true)
		if err != nil {
			return core.Result{}, err
		}
		r, err = historyRestore(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if r.State != "ready" || r.CandidateTransferID == nil {
			return core.Result{}, core.Fail(409, "RESTORE_STATE_CONFLICT", "恢复候选尚未就绪")
		}
		if in.BaseVersionID != r.BaseVersionID {
			return core.Result{}, core.Fail(409, "BASE_VERSION_CONFLICT", "提交基础版与固定恢复任务不一致")
		}
		if in.SHA256 != r.SHA256 {
			return core.Result{}, core.Fail(409, "CONTENT_DIGEST_MISMATCH", "确认摘要与恢复来源不一致")
		}
		_, origin, err := s.restoreAllowed(ctx, tx, c.Actor.OrgID, r, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = reserveFileCapacity(ctx, tx, c.Actor.OrgID, 0); err != nil {
			return core.Result{}, err
		}
		t, err := scanTransfer(tx.QueryRow(ctx, "SELECT "+transferColumns+` FROM object_transfers WHERE org_id=$1 AND id=$2 AND restore_id=$3
            AND kind='restore' AND state='complete' AND target_provider=$4 AND target_bucket=$5`, c.Actor.OrgID, *r.CandidateTransferID, r.ID, r.Provider, r.Bucket))
		if err != nil {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "恢复候选证据不完整")
		}
		key, _ := objectstore.ReplicaKey(c.Actor.OrgID, r.FileID, r.PlannedVersionID, t.ID)
		var source objectstore.Source
		if t.VersionID != r.PlannedVersionID || t.FileID != r.FileID || t.Key != key || t.SourceDigest != r.SourceDigest ||
			json.Unmarshal(t.Frozen, &source) != nil || source.Key != t.Key || source.Size != origin.Size || objectstore.ValidateDownloadSource(source) != nil {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "恢复候选对象身份不一致")
		}
		var progress objectstore.HashCheckpoint
		var sha *string
		if err = tx.QueryRow(ctx, `SELECT source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256
            FROM object_transfer_hashes WHERE transfer_id=$1`, t.ID).Scan(&progress.SourceDigest, &progress.RuntimeVersion,
			&progress.CompletedBytes, &progress.State, &progress.Checksum, &sha); err != nil {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "恢复目标缺少完整摘要证据")
		}
		if sha != nil {
			progress.SHA256 = *sha
		}
		authority, err := objectstore.CompletedHash(source, progress)
		if err != nil || authority.SHA256 != r.SHA256 || authority.Size != origin.Size {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "恢复目标摘要证据不一致")
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT coalesce(max(seq),0)+1 FROM file_versions WHERE org_id=$1 AND file_id=$2", c.Actor.OrgID, r.FileID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		sourceRaw, _ := json.Marshal(source)
		var created time.Time
		if err = tx.QueryRow(ctx, `INSERT INTO file_versions(id,org_id,file_id,seq,base_version_id,object_key,provider_source_id,storage_epoch,size_bytes,
            sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name,scan_report_ref,scan_waiver_ref,restored_from_version_id)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'restore',$12,$12,$13,'available',$14,$15,$16,$17) RETURNING created_at`,
			r.PlannedVersionID, c.Actor.OrgID, r.FileID, seq, r.BaseVersionID, t.Key, string(sourceRaw), r.StorageEpoch, origin.Size, r.SHA256,
			origin.MediaType, c.Actor.UserID, origin.ScanState, f.Name, origin.ReportID, origin.WaiverID, r.SourceVersionID).Scan(&created); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE files SET current_version_id=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$1", r.FileID, r.PlannedVersionID); err != nil {
			return core.Result{}, err
		}
		secondary := "r2"
		if r.Provider == "r2" {
			secondary = "oss"
		}
		var secondaryBucket string
		if err = tx.QueryRow(ctx, "SELECT bucket FROM storage_targets WHERE org_id=$1 AND provider=$2 AND enabled", c.Actor.OrgID, secondary).Scan(&secondaryBucket); err != nil {
			return core.Result{}, err
		}
		primaryID, secondaryID := core.NewID(), core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,source_id,state,expected_sha256,actual_sha256,verified_at)
            VALUES($1,$2,$3,$4,$5,$6,$7,'verified',$8,$8,$9)`, primaryID, c.Actor.OrgID, r.PlannedVersionID, r.Provider, r.Bucket, t.Key, string(sourceRaw), r.SHA256, created); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256)
            VALUES($1,$2,$3,$4,$5,$6,'queued',$7)`, secondaryID, c.Actor.OrgID, r.PlannedVersionID, secondary, secondaryBucket, t.Key, r.SHA256); err != nil {
			return core.Result{}, err
		}
		before := digest(r)
		if _, err = tx.Exec(ctx, `UPDATE file_history_restores SET state='committed',result_version_id=$2,version=version+1,
            updated_at=clock_timestamp() WHERE id=$1`, r.ID, r.PlannedVersionID); err != nil {
			return core.Result{}, err
		}
		current, err := historyRestore(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		f, err = file(ctx, tx, c.Actor.OrgID, r.FileID, false)
		if err != nil {
			return core.Result{}, err
		}
		f, err = presentFile(ctx, tx, c.Actor, f)
		if err != nil {
			return core.Result{}, err
		}
		overall := "queued"
		v := Version{ID: r.PlannedVersionID, FileID: r.FileID, Seq: seq, BaseVersionID: &r.BaseVersionID, RestoredFromVersionID: &r.SourceVersionID,
			OriginalName: &f.Name, SizeBytes: r.SizeBytes, SHA256: r.SHA256, MediaType: origin.MediaType, Source: "restore", AuthorID: c.Actor.UserID,
			ExecutorID: c.Actor.UserID, CreatedAt: created, ScanState: origin.ScanState, ScanWaiverRef: origin.WaiverID, Availability: "available", ReplicaState: &overall,
			Replicas: []Replica{{ID: primaryID, Version: 1, Provider: r.Provider, State: "verified", ActualSHA256: &r.SHA256, VerifiedAt: &created, UpdatedAt: created},
				{ID: secondaryID, Version: 1, Provider: secondary, State: "queued", UpdatedAt: created}}}
		return core.Result{Status: 201, Data: CommitResult{File: f, Version: v, Committed: true}, Event: core.Event{
			Type: "file.history.restore.committed", ResourceType: "file_history_restore", ResourceID: r.ID, Version: current.Version, BeforeDigest: before,
			Payload: map[string]any{"restore_id": r.ID, "file_id": r.FileID, "version_id": r.PlannedVersionID, "replica_id": secondaryID, "replica_version": int64(1),
				"source_provider": r.Provider, "target_provider": secondary, "sha256": r.SHA256, "size_bytes": r.SizeBytes, "storage_epoch": r.StorageEpoch}},
			Events: []core.Event{{Type: "file.version.committed", ResourceType: "file", ResourceID: f.ID, Version: f.Version, Payload: map[string]any{
				"file_id": f.ID, "version_id": v.ID, "restored_from_version_id": r.SourceVersionID, "restore_id": r.ID, "storage_epoch": r.StorageEpoch}}}}, nil
	})
}
