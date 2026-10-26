package files

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

const PromoteSmallLane = "storage.promote.small"
const PromoteLargeLane = "storage.promote.large"

var PromotionJobKinds = []string{"upload.promote.v1"}

func PromotionLane(size int64) string {
	if size <= smallFileLimit {
		return PromoteSmallLane
	}
	return PromoteLargeLane
}
func UploadPromotionConsumer() jobqueue.Consumer {
	base := UploadIntegrityConsumer()
	build := base.Build
	base.Name = "upload.promotion.v1"
	base.EventTypes = []string{"upload.scan.passed", "upload.scan.waived"}
	base.Build = func(e jobqueue.Event) (jobqueue.Input, error) {
		input, err := build(e)
		if err != nil {
			return input, err
		}
		var in uploadJobInput
		if err = json.Unmarshal(input.Payload, &in); err != nil {
			return input, err
		}
		size, _ := strconv.ParseInt(in.SizeBytes, 10, 64)
		input.Kind = "upload.promote.v1"
		input.Lane = PromotionLane(size)
		input.DedupKey = fmt.Sprintf("upload.promote.v1/%s/%d", in.SessionID, in.SessionVersion)
		return input, nil
	}
	return base
}

type transferRow struct {
	ID, FileID, VersionID, Key, State, SourceDigest string
	UploadID                                        *string
	Manifest                                        []byte
	Frozen                                          []byte
}

func scanTransfer(row pgx.Row) (transferRow, error) {
	var t transferRow
	err := row.Scan(&t.ID, &t.FileID, &t.VersionID, &t.Key, &t.State, &t.UploadID, &t.Manifest, &t.Frozen, &t.SourceDigest)
	return t, err
}

const transferColumns = `id,file_id,version_id,object_key,state,provider_upload_id,manifest,frozen_source,source_digest`

func (s *UploadService) promotionAllowed(ctx context.Context, tx pgx.Tx, j jobqueue.Job, u Upload, in uploadJobInput) error {
	if u.State != "verifying" || u.Version != in.SessionVersion {
		return core.Fail(409, "UPLOAD_STATE_CONFLICT", "正式候选阶段已改变")
	}
	if err := s.workerAllowed(ctx, tx, j, u); err != nil {
		return err
	}
	if _, err := fixedSource(u, in); err != nil {
		return err
	}
	return scanAccepted(ctx, tx, j.OrgID, u)
}
func scanAccepted(ctx context.Context, tx pgx.Tx, org string, u Upload) error {
	if u.SHA256 == nil || u.ScanResultID == nil || u.FrozenSourceID == nil || (u.ScanState != "clean" && u.ScanState != "waived") {
		return core.Fail(409, "SCAN_APPROVAL_REQUIRED", "文件尚未取得有效扫描或例外放行")
	}
	var valid bool
	if u.ScanState == "waived" {
		if u.ScanWaiverRef == nil || u.Classification == nil {
			return core.Fail(409, "SCAN_EVIDENCE_MISMATCH", "扫描例外证据不完整")
		}
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_waivers w JOIN upload_scan_reports r ON r.org_id=w.org_id AND r.id=w.report_id
			JOIN scan_waiver_decisions d ON d.org_id=w.org_id AND d.waiver_id=w.id AND d.decision='approve'
				AND d.actor_id=w.reviewer_id AND d.acknowledged_risk AND d.mfa_verified_at IS NOT NULL
			JOIN scan_waiver_policies p ON p.org_id=w.org_id AND p.id=w.policy_id AND p.version=w.policy_version
			WHERE w.org_id=$1 AND w.session_id=$2 AND w.id=$3 AND w.state='approved' AND w.owner_id=$4 AND w.owner_id<>w.reviewer_id
			AND w.report_id=$5 AND w.sha256=$6 AND w.source_digest=$7 AND w.classification=$8 AND w.size_bytes=$9
			AND w.expires_at>clock_timestamp() AND r.state='unscannable' AND r.sha256=w.sha256 AND r.source_digest=w.source_digest
			AND r.size_bytes=w.size_bytes)`, org, u.ID, *u.ScanWaiverRef, u.OwnerID, *u.ScanResultID, *u.SHA256, core.Digest([]byte(*u.FrozenSourceID)), *u.Classification, u.layout().Size).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return core.Fail(409, "SCAN_EVIDENCE_MISMATCH", "扫描例外与固定来源不一致")
		}
		return nil
	}
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM upload_scan_reports WHERE org_id=$1 AND session_id=$2 AND id=$3
		AND sha256=$4 AND source_digest=$5 AND state='clean' AND code='SCAN_PASSED' AND streamed_bytes=size_bytes AND size_bytes=$6)`,
		// The source is already fixed by the caller and immutable in PostgreSQL.
		org, u.ID, u.ScanResultID, u.SHA256, core.Digest([]byte(*u.FrozenSourceID)), u.layout().Size).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return core.Fail(409, "SCAN_EVIDENCE_MISMATCH", "扫描证据与固定源不一致")
	}
	return nil
}

func (s *UploadService) RunPromotionJob(ctx context.Context, j jobqueue.Job, from, to objectstore.Store) error {
	in, err := decodeUploadJob(j)
	if err != nil {
		return err
	}
	if j.Kind != "upload.promote.v1" || from == nil || to == nil {
		return core.Fail(409, "INVALID_JOB_INPUT", "正式候选任务无效")
	}
	var current Upload
	var transfer transferRow
	skip := false
	err = s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		if u.State != "verifying" || u.Version != in.SessionVersion {
			skip = true
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		if err := s.promotionAllowed(ctx, tx, j, u, in); err != nil {
			return err
		}
		current = u
		t, e := scanTransfer(tx.QueryRow(ctx, "SELECT "+transferColumns+" FROM object_transfers WHERE org_id=$1 AND job_id=$2 AND state IN ('active','sealed','complete') ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE", j.OrgID, j.ID))
		if e == nil {
			transfer = t
			_, e = tx.Exec(ctx, "UPDATE object_transfers SET fencing_token=$2,updated_at=clock_timestamp() WHERE id=$1", t.ID, j.FencingToken)
			return e
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE object_transfers SET state='uncertain',updated_at=clock_timestamp() WHERE org_id=$1 AND job_id=$2 AND state='starting'", j.OrgID, j.ID); e != nil {
			return e
		}
		transfer = transferRow{ID: core.NewID(), FileID: core.NewID(), VersionID: core.NewID(), State: "starting"}
		if u.FileID != nil {
			transfer.FileID = *u.FileID
		}
		transfer.Key, _ = objectstore.VersionKey(j.OrgID, transfer.FileID, transfer.VersionID)
		_, e = tx.Exec(ctx, `INSERT INTO object_transfers(id,org_id,session_id,job_id,file_id,version_id,source_digest,object_key,state,fencing_token,target_provider,target_bucket)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'starting',$9,$10,$11)`, transfer.ID, j.OrgID, u.ID, j.ID, transfer.FileID, transfer.VersionID, in.SourceDigest, transfer.Key, j.FencingToken, *u.Provider, *u.Bucket)
		return e
	})
	if err != nil || skip {
		return err
	}
	checkpoint := func(fn func(pgx.Tx, Upload) error) error {
		return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
			if err := s.promotionAllowed(ctx, tx, j, u, in); err != nil {
				return err
			}
			var id string
			if err := tx.QueryRow(ctx, "SELECT id FROM object_transfers WHERE org_id=$1 AND id=$2 AND job_id=$3 AND fencing_token=$4 FOR UPDATE", j.OrgID, transfer.ID, j.ID, j.FencingToken).Scan(&id); err != nil {
				if err == pgx.ErrNoRows {
					return core.Fail(409, "JOB_LEASE_LOST", "传输执行凭证已失效")
				}
				return err
			}
			return fn(tx, u)
		})
	}
	if transfer.UploadID == nil {
		m, e := to.Begin(ctx, transfer.Key, current.MediaType)
		if e != nil {
			return e
		}
		if m.Key != transfer.Key || m.UploadID == "" {
			return core.Fail(409, "INVALID_PROVIDER_SOURCE", "候选初始化回执无效")
		}
		if err = checkpoint(func(tx pgx.Tx, u Upload) error {
			_, e := tx.Exec(ctx, "UPDATE object_transfers SET provider_upload_id=$2,state='active',updated_at=clock_timestamp() WHERE id=$1", transfer.ID, m.UploadID)
			return e
		}); err != nil {
			return err
		}
		transfer.UploadID = &m.UploadID
	}
	source, err := fixedSource(current, in)
	if err != nil {
		return err
	}
	state := objectstore.CopyState{Source: source, Target: objectstore.Multipart{Key: transfer.Key, UploadID: *transfer.UploadID}, Layout: current.layout(), ExpectedSHA256: *current.SHA256}
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
	guard := func(fn func(pgx.Tx) error) error {
		return checkpoint(func(tx pgx.Tx, _ Upload) error { return fn(tx) })
	}
	if err = loadTransferHash(ctx, transfer.ID, &state, guard); err != nil {
		return err
	}
	hooks := transferHooks(ctx, transfer.ID, j.FencingToken, guard)
	target, evidence, err := objectstore.CopyFrozen(ctx, from, to, state, hooks)
	if err != nil {
		return err
	}
	return checkpoint(func(tx pgx.Tx, u Upload) error {
		if evidence.SHA256 != *u.SHA256 || evidence.Size != u.layout().Size {
			return core.Fail(409, "CONTENT_DIGEST_MISMATCH", "正式候选摘要不一致")
		}
		raw, _ := json.Marshal(target)
		before := digest(u)
		if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET candidate_key=$2,candidate_source_id=$3,candidate_transfer_id=$4,
			state='ready',last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1`, u.ID, target.Key, string(raw), transfer.ID); err != nil {
			return err
		}
		u, err := upload(ctx, tx, j.OrgID, u.ID, false)
		if err != nil {
			return err
		}
		if err = workerEvent(ctx, tx, j, u, "upload.ready", before); err != nil {
			return err
		}
		return s.Queue.CompleteInTx(ctx, tx, j)
	})
}
