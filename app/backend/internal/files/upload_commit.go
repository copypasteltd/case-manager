package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type UploadCommit struct {
	ExpectedVersion int64   `json:"expected_version"`
	BaseVersionID   *string `json:"base_version_id,omitempty"`
	SHA256          string  `json:"sha256"`
}
type CommitResult struct {
	File      File    `json:"file"`
	Version   Version `json:"file_version"`
	Committed bool    `json:"committed"`
}

func (s *UploadService) Commit(ctx context.Context, c core.Command, id string, in UploadCommit) (core.Result, error) {
	// Recheck import provenance before idempotency replay as well as inside the
	// transaction. Ordinary uploads have no import row and retain normal checks.
	u, err := upload(ctx, s.Files.DB, c.Actor.OrgID, id, false)
	if err != nil {
		return core.Result{}, err
	}
	if err = s.checkImportCommit(ctx, s.Files.DB, c.Actor, u, in.SHA256, false); err != nil {
		return core.Result{}, err
	}
	return s.commitMode(ctx, c, id, in, false)
}

// Only the trusted agent coordinator can reach this method with a server-built
// actor RunID. Public upload commands keep their existing human-only boundary.
func (s *UploadService) CommitAgentDraft(ctx context.Context, c core.Command, id string, in UploadCommit) (core.Result, error) {
	if !core.ValidID(c.Actor.RunID) {
		return core.Result{}, core.Forbidden()
	}
	return s.commitMode(ctx, c, id, in, true)
}
func (s *UploadService) commitMode(ctx context.Context, c core.Command, id string, in UploadCommit, agent bool) (core.Result, error) {
	if !shaPattern.MatchString(in.SHA256) {
		return core.Result{}, core.Invalid("sha256", "请提交已确认的完整摘要")
	}
	return s.mutateMode(ctx, c, id, in.ExpectedVersion, []string{"ready"}, agent, func(tx pgx.Tx, u Upload) (core.Result, error) {
		if err := s.validateImportCommit(ctx, tx, c.Actor, u, in.SHA256); err != nil {
			return core.Result{}, err
		}
		if agent && u.ScanState != "clean" {
			return core.Result{}, core.Fail(422, "AGENT_TEXT_SCAN_REQUIRED", "秘书提交要求准确草稿扫描通过")
		}
		if u.SHA256 == nil || in.SHA256 != *u.SHA256 {
			return core.Result{}, core.Fail(409, "CONTENT_DIGEST_MISMATCH", "提交摘要与已验证内容不一致")
		}
		if !sameID(in.BaseVersionID, u.BaseVersionID) {
			return core.Result{}, core.Fail(409, "BASE_VERSION_CONFLICT", "提交基础版本与上传固定输入不一致")
		}
		if err := scanAccepted(ctx, tx, c.Actor.OrgID, u); err != nil {
			return core.Result{}, err
		}
		var transferID *string
		var candidateKey, candidateSource *string
		if err := tx.QueryRow(ctx, "SELECT candidate_transfer_id,candidate_key,candidate_source_id FROM upload_sessions WHERE id=$1", id).Scan(&transferID, &candidateKey, &candidateSource); err != nil {
			return core.Result{}, err
		}
		if transferID == nil || candidateKey == nil || candidateSource == nil {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "正式候选证据不完整")
		}
		t, err := scanTransfer(tx.QueryRow(ctx, "SELECT "+transferColumns+" FROM object_transfers WHERE org_id=$1 AND id=$2 AND session_id=$3 AND state='complete'", c.Actor.OrgID, *transferID, u.ID))
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		var source objectstore.Source
		if err = json.Unmarshal([]byte(*candidateSource), &source); err != nil {
			return core.Result{}, err
		}
		var frozen objectstore.Source
		if err = json.Unmarshal(t.Frozen, &frozen); err != nil {
			return core.Result{}, err
		}
		var progress objectstore.HashCheckpoint
		var finalSHA *string
		if err = tx.QueryRow(ctx, "SELECT source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256 FROM object_transfer_hashes WHERE transfer_id=$1", t.ID).Scan(
			&progress.SourceDigest, &progress.RuntimeVersion, &progress.CompletedBytes, &progress.State, &progress.Checksum, &finalSHA); err != nil {
			return core.Result{}, err
		}
		if finalSHA != nil {
			progress.SHA256 = *finalSHA
		}
		authority, e := objectstore.CompletedHash(source, progress)
		if e != nil {
			return core.Result{}, e
		}
		if source != frozen || source.Key != t.Key || source.Key != *candidateKey || source.Size != u.layout().Size || authority.SHA256 != in.SHA256 || authority.Size != source.Size {
			return core.Result{}, core.Fail(409, "CANDIDATE_EVIDENCE_MISSING", "正式候选来源与摘要不一致")
		}
		if u.Provider == nil || u.Bucket == nil || s.Backend == nil || s.Backend.Provider != *u.Provider || s.Backend.Bucket != *u.Bucket {
			return core.Result{}, core.Fail(503, "STORAGE_CONFIGURATION_CHANGED", "存储配置与正式候选不一致")
		}
		primary := *u.Provider
		secondary := "r2"
		if primary == "r2" {
			secondary = "oss"
		}
		var primaryBucket, secondaryBucket string
		if err = tx.QueryRow(ctx, "SELECT bucket FROM storage_targets WHERE org_id=$1 AND provider=$2 AND enabled", c.Actor.OrgID, primary).Scan(&primaryBucket); err != nil {
			if err == pgx.ErrNoRows {
				return core.Result{}, core.Fail(503, "STORAGE_TARGETS_NOT_CONFIGURED", "双云存储目标尚未配置")
			}
			return core.Result{}, err
		}
		if err = tx.QueryRow(ctx, "SELECT bucket FROM storage_targets WHERE org_id=$1 AND provider=$2 AND enabled", c.Actor.OrgID, secondary).Scan(&secondaryBucket); err != nil {
			if err == pgx.ErrNoRows {
				return core.Result{}, core.Fail(503, "STORAGE_TARGETS_NOT_CONFIGURED", "双云存储目标尚未配置")
			}
			return core.Result{}, err
		}
		if primaryBucket != *u.Bucket {
			return core.Result{}, core.Fail(409, "STORAGE_CONFIGURATION_CHANGED", "主存储目标与固定候选不一致")
		}
		seq := int64(1)
		if u.FileID == nil {
			nameKey, _ := core.StorageNameKey(u.Name)
			if _, err = tx.Exec(ctx, `INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id)
				VALUES($1,$2,$3,$4,$5,$6,'active',$7)`, t.FileID, c.Actor.OrgID, u.FolderID, u.Name, nameKey, *u.Classification, u.OwnerID); err != nil {
				return core.Result{}, err
			}
		} else {
			f, e := file(ctx, tx, c.Actor.OrgID, *u.FileID, true)
			if e != nil {
				return core.Result{}, e
			}
			if f.ID != t.FileID || !sameID(f.CurrentVersionID, u.BaseVersionID) {
				return core.Result{}, core.Fail(409, "BASE_VERSION_CONFLICT", "目标文件已有新版本，请保留候选后重新处理")
			}
			if f.Classification != *u.Classification {
				return core.Result{}, core.Fail(409, "UPLOAD_TARGET_CHANGED", "目标文件分类已改变")
			}
			if err = tx.QueryRow(ctx, "SELECT seq+1 FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3", c.Actor.OrgID, f.ID, *u.BaseVersionID).Scan(&seq); err != nil {
				return core.Result{}, err
			}
		}
		var created time.Time
		sourceKind := "upload"
		if agent {
			sourceKind = "agent"
		}
		if err = tx.QueryRow(ctx, `INSERT INTO file_versions(id,org_id,file_id,seq,base_version_id,object_key,provider_source_id,storage_epoch,size_bytes,
			sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name,scan_report_ref,scan_waiver_ref)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$18,$12,$13,$16,'available',$14,$15,$17) RETURNING created_at`,
			t.VersionID, c.Actor.OrgID, t.FileID, seq, u.BaseVersionID, t.Key, *candidateSource, u.StorageEpoch, source.Size, in.SHA256, u.MediaType, u.OwnerID, c.Actor.UserID, u.Name, u.ScanResultID, u.ScanState, u.ScanWaiverRef, sourceKind).Scan(&created); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE files SET current_version_id=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$1", t.FileID, t.VersionID); err != nil {
			return core.Result{}, err
		}
		primaryID, secondaryID := core.NewID(), core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,source_id,state,expected_sha256,actual_sha256,verified_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,'verified',$8,$8,$9)`, primaryID, c.Actor.OrgID, t.VersionID, primary, primaryBucket, t.Key, *candidateSource, in.SHA256, created); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256)
			VALUES($1,$2,$3,$4,$5,$6,'queued',$7)`, secondaryID, c.Actor.OrgID, t.VersionID, secondary, secondaryBucket, t.Key, in.SHA256); err != nil {
			return core.Result{}, err
		}
		before := digest(u)
		if _, err = tx.Exec(ctx, "UPDATE upload_sessions SET committed_version_id=$2,state='committed',version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID, t.VersionID); err != nil {
			return core.Result{}, err
		}
		current, err := upload(ctx, tx, c.Actor.OrgID, u.ID, false)
		if err != nil {
			return core.Result{}, err
		}
		f, err := file(ctx, tx, c.Actor.OrgID, t.FileID, false)
		if err != nil {
			return core.Result{}, err
		}
		f, err = presentFile(ctx, tx, c.Actor, f)
		if err != nil {
			return core.Result{}, err
		}
		overall := "queued"
		original := u.Name
		version := Version{ID: t.VersionID, FileID: t.FileID, Seq: seq, BaseVersionID: u.BaseVersionID, OriginalName: &original, SizeBytes: u.SizeBytes,
			SHA256: in.SHA256, MediaType: u.MediaType, Source: sourceKind, AuthorID: u.OwnerID, ExecutorID: c.Actor.UserID, CreatedAt: created,
			ScanState: u.ScanState, ScanWaiverRef: u.ScanWaiverRef, Availability: "available", ReplicaState: &overall, Replicas: []Replica{
				{ID: primaryID, Version: 1, Provider: primary, State: "verified", ActualSHA256: &in.SHA256, VerifiedAt: &created, UpdatedAt: created},
				{ID: secondaryID, Version: 1, Provider: secondary, State: "queued", UpdatedAt: created},
			}}
		event := uploadEvent("upload.committed", current, map[string]any{"file_id": t.FileID, "version_id": t.VersionID, "replica_id": secondaryID,
			"source_provider": primary, "target_provider": secondary, "size_bytes": u.SizeBytes, "sha256": in.SHA256, "replica_version": int64(1)})
		event.BeforeDigest = before
		fileEvent := core.Event{Type: "file.version.committed", ResourceType: "file", ResourceID: f.ID, Version: f.Version, Payload: map[string]any{
			"file_id": f.ID, "version_id": t.VersionID, "upload_id": u.ID, "storage_epoch": u.StorageEpoch}}
		if agent {
			fileEvent.Payload["agent_run_id"] = c.Actor.RunID
		}
		return core.Result{Status: 201, Data: CommitResult{File: f, Version: version, Committed: true}, Event: event, Events: []core.Event{fileEvent}}, nil
	})
}
