package files

import (
	"context"
	"strconv"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ReplicaRetry struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

func (s *Service) RetryReplica(ctx context.Context, c core.Command, id string, in ReplicaRetry) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if in.ExpectedVersion < 1 || strings.TrimSpace(in.Reason) == "" || len([]rune(in.Reason)) > 200 {
		return core.Result{}, core.Invalid("reason", "请填写副本版本与重试原因")
	}
	if c.Actor.RunID != "" || (c.Actor.Channel != "web" && c.Actor.Channel != "android" && c.Actor.Channel != "wechat" && c.Actor.Channel != "douyin") {
		return core.Result{}, core.Forbidden()
	}
	authorize := func(tx pgx.Tx) error {
		r, err := replica(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return err
		}
		_, err = authorizeFile(ctx, tx, c.Actor, r.FileID, "file.update")
		return err
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, err := replica(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = replicaNamespace(ctx, tx, c.Actor.OrgID, r.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		r, err = replica(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if r.State == "verified" {
			return core.Result{}, core.Fail(409, "REPLICA_ALREADY_VERIFIED", "副本已有完整验证证据")
		}
		exhausted := false
		if r.ActiveJob != nil {
			var state string
			if err = tx.QueryRow(ctx, "SELECT state FROM jobs WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, *r.ActiveJob).Scan(&state); err != nil {
				return core.Result{}, err
			}
			exhausted = state == "dead" || state == "cancelled"
			if !exhausted {
				return core.Result{}, core.Fail(409, "REPLICA_JOB_ACTIVE", "现有任务仍在运行或等待重试")
			}
		}
		if r.State != "failed" && r.State != "missing" && !exhausted {
			return core.Result{}, core.Fail(409, "REPLICA_STATE_CONFLICT", "当前副本不需要手动重试")
		}
		epoch, provider, err := currentEpoch(ctx, tx, c.Actor.OrgID)
		if err != nil {
			return core.Result{}, err
		}
		if provider == r.Provider {
			return core.Result{}, core.Fail(409, "STORAGE_REPAIR_PLAN_REQUIRED", "存储主端已切换，需先建立专门恢复计划")
		}
		var available bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_replicas WHERE org_id=$1 AND version_id=$2 AND provider=$3
            AND state='verified' AND expected_sha256=$4 AND actual_sha256=$4 AND source_id IS NOT NULL AND verified_at IS NOT NULL)`,
			c.Actor.OrgID, r.VersionID, provider, r.SHA256).Scan(&available); err != nil {
			return core.Result{}, err
		}
		if !available {
			return core.Result{}, core.Fail(409, "REPLICA_SOURCE_UNAVAILABLE", "缺少可验证的来源副本")
		}
		before := digest(r.Replica)
		if _, err = tx.Exec(ctx, `UPDATE file_replicas SET state='queued',active_job_id=NULL,last_error=NULL,actual_sha256=NULL,verified_at=NULL,
            source_id=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1`, id); err != nil {
			return core.Result{}, err
		}
		r, err = replica(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		payload := map[string]any{"replica_id": r.ID, "file_id": r.FileID, "version_id": r.VersionID, "replica_version": r.Version,
			"storage_epoch": epoch, "source_provider": provider, "target_provider": r.Provider, "sha256": r.SHA256, "size_bytes": strconv.FormatInt(r.Size, 10), "reason": in.Reason}
		return core.Result{Data: r.Replica, Event: core.Event{Type: "replica.retry.requested", ResourceType: "file_replica", ResourceID: r.ID,
			Version: r.Version, BeforeDigest: before, Payload: payload}}, nil
	})
}
