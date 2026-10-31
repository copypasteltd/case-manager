package files

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Organization inventory grants metadata discovery, never file content access.
// Do not add provider coordinates, names, hashes, raw errors or job payloads.
type StorageJob struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	Attempt     int       `json:"attempt"`
	MaxAttempts int       `json:"max_attempts"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type StorageReplicaSnapshot struct {
	ID              string      `json:"id"`
	Version         int64       `json:"version"`
	Provider        string      `json:"provider"`
	State           string      `json:"state"`
	FileID          string      `json:"file_id"`
	FileVersionID   string      `json:"file_version_id"`
	FileRevision    int64       `json:"file_revision"`
	FileState       string      `json:"file_state"`
	Availability    string      `json:"availability"`
	IsCurrent       bool        `json:"is_current"`
	StorageEpoch    int64       `json:"storage_epoch"`
	PrimaryProvider string      `json:"primary_provider"`
	SourceAvailable bool        `json:"source_available"`
	ErrorCode       *string     `json:"error_code"`
	Job             *StorageJob `json:"job"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}
type StorageReplicaOperation struct {
	StorageReplicaSnapshot
	BasisSHA256 string `json:"basis_sha256"`
}
type StorageReplicaDetails struct {
	Item       StorageReplicaOperation `json:"item"`
	CanOpen    bool                    `json:"can_open"`
	CanRetry   bool                    `json:"can_retry"`
	RetryBlock string                  `json:"retry_block"`
}
type StorageReplicaRetry struct {
	ExpectedVersion int64  `json:"expected_version"`
	BasisSHA256     string `json:"basis_sha256"`
	Reviewed        bool   `json:"reviewed"`
	Reason          string `json:"reason"`
}
type StorageReplicaReceipt struct {
	ID                  string                 `json:"id"`
	ReplicaID           string                 `json:"replica_id"`
	FileID              string                 `json:"file_id"`
	FileVersionID       string                 `json:"file_version_id"`
	ActorID             string                 `json:"actor_id"`
	BusinessOperationID string                 `json:"business_operation_id"`
	ExpectedVersion     int64                  `json:"expected_version"`
	ResultVersion       int64                  `json:"result_version"`
	BasisSHA256         string                 `json:"basis_sha256"`
	Before              StorageReplicaSnapshot `json:"before"`
	After               StorageReplicaSnapshot `json:"after"`
	CreatedAt           time.Time              `json:"created_at"`
}
type StorageOperationFilter struct {
	Provider, State, JobState, Cursor string
	Limit                             int
}

var storageHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func storageAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, write); err != nil {
		return err
	}
	if err := authz.RequireOrganization(ctx, tx, a, "storage.operations.read"); err != nil {
		return err
	}
	if write {
		return authz.RequireOrganization(ctx, tx, a, "storage.operations.retry")
	}
	return nil
}

const storageReplicaSelect = `SELECT r.id,r.version,r.provider,r.state,v.file_id,r.version_id,f.version,f.state,v.availability,
 coalesce(f.current_version_id=v.id,false),e.epoch,e.active_primary,
 EXISTS(SELECT 1 FROM file_replicas src WHERE src.org_id=r.org_id AND src.version_id=r.version_id
  AND src.provider=e.active_primary AND src.state='verified' AND src.expected_sha256=v.sha256
  AND src.actual_sha256=v.sha256 AND src.source_id IS NOT NULL AND src.verified_at IS NOT NULL),
 r.last_error,r.created_at,r.updated_at,j.id,j.state,j.attempt,j.max_attempts,j.updated_at
 FROM file_replicas r JOIN file_versions v ON v.org_id=r.org_id AND v.id=r.version_id
 JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
 JOIN LATERAL(SELECT epoch,active_primary FROM storage_epochs WHERE org_id=r.org_id ORDER BY epoch DESC LIMIT 1)e ON true
 LEFT JOIN jobs j ON j.org_id=r.org_id AND j.id=r.active_job_id`

func scanStorageReplica(row pgx.Row) (StorageReplicaOperation, error) {
	var v StorageReplicaOperation
	var jobID, jobState *string
	var attempt, maxAttempts *int
	var jobUpdated *time.Time
	err := row.Scan(&v.ID, &v.Version, &v.Provider, &v.State, &v.FileID, &v.FileVersionID, &v.FileRevision, &v.FileState, &v.Availability,
		&v.IsCurrent, &v.StorageEpoch, &v.PrimaryProvider, &v.SourceAvailable, &v.ErrorCode, &v.CreatedAt, &v.UpdatedAt,
		&jobID, &jobState, &attempt, &maxAttempts, &jobUpdated)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	if jobID != nil {
		v.Job = &StorageJob{ID: *jobID, State: *jobState, Attempt: *attempt, MaxAttempts: *maxAttempts, UpdatedAt: *jobUpdated}
	}
	if v.ErrorCode != nil && !slices.Contains([]string{"REPLICA_HASH_MISMATCH", "REPLICA_SIZE_MISMATCH", "REPLICA_SOURCE_UNAVAILABLE", "REPLICA_SOURCE_MISMATCH", "STORAGE_EPOCH_CHANGED", "STORAGE_CONFIGURATION_CHANGED", "STORAGE_TARGETS_NOT_CONFIGURED", "STORAGE_UNAVAILABLE", "OBJECT_NOT_FOUND", "LEASE_EXHAUSTED", "ACCESS_DENIED", "RETRY_EXHAUSTED", "PROVIDER_UNAVAILABLE"}, *v.ErrorCode) {
		code := "OTHER_STORAGE_FAILURE"
		v.ErrorCode = &code
	}
	raw, err := json.Marshal(v.StorageReplicaSnapshot)
	if err != nil {
		return v, err
	}
	v.BasisSHA256 = core.Digest(raw)
	return v, nil
}
func storageReplica(ctx context.Context, tx pgx.Tx, org, id string) (StorageReplicaOperation, error) {
	return scanStorageReplica(tx.QueryRow(ctx, storageReplicaSelect+` WHERE r.org_id=$1 AND r.id=$2`, org, id))
}
func (s *Service) StorageReplicas(ctx context.Context, a core.Actor, f StorageOperationFilter) (core.Page[StorageReplicaOperation], error) {
	out := core.Page[StorageReplicaOperation]{Items: []StorageReplicaOperation{}}
	if f.Limit < 1 || f.Limit > 100 || len(f.Cursor) > 2048 || f.Provider != "" && !slices.Contains([]string{"oss", "r2"}, f.Provider) || f.State != "" && !slices.Contains([]string{"queued", "copying", "verifying", "verified", "retry_wait", "failed", "missing"}, f.State) || f.JobState != "" && !slices.Contains([]string{"queued", "running", "succeeded", "retry_wait", "dead", "cancelled"}, f.JobState) {
		return out, core.Invalid("filters", "请核对存储端、副本／任务状态和分页条件")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = storageAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "storage-replicas", f.Provider, f.State, f.JobState)
	var before *time.Time
	var id *string
	if f.Cursor != "" {
		v, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before, id = &v.UpdatedAt, &v.ID
	}
	rows, err := tx.Query(ctx, storageReplicaSelect+` WHERE r.org_id=$1 AND ($2='' OR r.provider=$2) AND ($3='' OR r.state=$3)
 AND ($4='' OR j.state=$4) AND ($5::timestamptz IS NULL OR (r.created_at,r.id)<($5,$6::uuid))
 ORDER BY r.created_at DESC,r.id DESC LIMIT $7`, a.OrgID, f.Provider, f.State, f.JobState, before, id, f.Limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanStorageReplica(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		v := out.Items[f.Limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}
func storageFileAllowed(ctx context.Context, tx pgx.Tx, a core.Actor, id, action string) (bool, error) {
	_, err := authorizeFile(ctx, tx, a, id, action)
	if err == nil {
		return true, nil
	}
	var public *core.Error
	if errors.As(err, &public) && (public.Status == 403 || public.Status == 404) {
		return false, nil
	}
	return false, err
}
func (s *Service) StorageReplica(ctx context.Context, a core.Actor, id string) (StorageReplicaDetails, error) {
	var out StorageReplicaDetails
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = storageAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	if out.Item, err = storageReplica(ctx, tx, a.OrgID, id); err != nil {
		return out, err
	}
	if out.CanOpen, err = storageFileAllowed(ctx, tx, a, out.Item.FileID, "file.read"); err != nil {
		return out, err
	}
	write, err := storageFileAllowed(ctx, tx, a, out.Item.FileID, "file.update")
	if err != nil {
		return out, err
	}
	permission := authz.RequireOrganization(ctx, tx, a, "storage.operations.retry")
	if permission != nil {
		var denied *core.Error
		if !errors.As(permission, &denied) || (denied.Status != 403 && denied.Status != 404) {
			return out, permission
		}
	}
	v := out.Item
	switch {
	case permission != nil || !write:
		out.RetryBlock = "NO_FILE_RETRY_AUTHORITY"
	case v.Availability == "retiring" || v.Availability == "purged":
		out.RetryBlock = "ORIGINAL_RETIRED"
	case v.State == "verified":
		out.RetryBlock = "REPLICA_ALREADY_VERIFIED"
	case v.Job != nil && v.Job.State != "dead" && v.Job.State != "cancelled":
		out.RetryBlock = "REPLICA_JOB_ACTIVE"
	case v.State != "failed" && v.State != "missing" && (v.Job == nil || v.Job.State != "dead" && v.Job.State != "cancelled"):
		out.RetryBlock = "REPLICA_STATE_CONFLICT"
	case v.PrimaryProvider == v.Provider:
		out.RetryBlock = "STORAGE_REPAIR_PLAN_REQUIRED"
	case !v.SourceAvailable:
		out.RetryBlock = "REPLICA_SOURCE_UNAVAILABLE"
	default:
		out.CanRetry = true
		out.RetryBlock = "READY_FOR_REVIEW"
	}
	return out, nil
}

func (s *Service) RetryStorageReplica(ctx context.Context, c core.Command, id string, in StorageReplicaRetry) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if in.ExpectedVersion < 1 || !storageHash.MatchString(in.BasisSHA256) || !in.Reviewed || strings.TrimSpace(in.Reason) == "" || len([]rune(in.Reason)) > 200 {
		return core.Result{}, core.Invalid("review", "请核对准确副本、版本、任务状态及重试原因")
	}
	authorize := func(tx pgx.Tx) error {
		if err := storageAuthority(ctx, tx, c.Actor, true); err != nil {
			return err
		}
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
		project := r.ProjectID
		if err = replicaNamespace(ctx, tx, c.Actor.OrgID, project); err != nil {
			return core.Result{}, err
		}
		r, err = replica(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if project == nil && r.ProjectID != nil || project != nil && (r.ProjectID == nil || *project != *r.ProjectID) {
			return core.Result{}, core.Conflict()
		}
		var availability string
		if err = tx.QueryRow(ctx, "SELECT availability FROM file_versions WHERE org_id=$1 AND id=$2 FOR SHARE", c.Actor.OrgID, r.VersionID).Scan(&availability); err != nil {
			return core.Result{}, err
		}
		if r.ActiveJob != nil {
			var job string
			if err = tx.QueryRow(ctx, "SELECT id FROM jobs WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, *r.ActiveJob).Scan(&job); err != nil {
				return core.Result{}, err
			}
		}
		before, err := storageReplica(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if before.Version != in.ExpectedVersion || before.BasisSHA256 != in.BasisSHA256 {
			return core.Result{}, core.Conflict()
		}
		inner := c
		inner.Route = "/internal/storage-replica-retry/" + id
		inner.Key = "storage-retry:" + core.Digest([]byte(c.Key))
		input := ReplicaRetry{ExpectedVersion: in.ExpectedVersion, Reason: in.Reason}
		inner.Body, _ = json.Marshal(input)
		ordinary := Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}, Pager: s.Pager}
		saved, err := ordinary.RetryReplica(ctx, inner, id, input)
		if err != nil {
			return core.Result{}, err
		}
		if saved.Replayed || saved.Event.Type != "replica.retry.requested" {
			return core.Result{}, core.Conflict()
		}
		after, err := storageReplica(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		out := StorageReplicaReceipt{ID: core.NewID(), ReplicaID: id, FileID: r.FileID, FileVersionID: r.VersionID, ActorID: c.Actor.UserID, BusinessOperationID: saved.OperationID, ExpectedVersion: before.Version, ResultVersion: after.Version, BasisSHA256: before.BasisSHA256, Before: before.StorageReplicaSnapshot, After: after.StorageReplicaSnapshot}
		beforeRaw, _ := json.Marshal(out.Before)
		afterRaw, _ := json.Marshal(out.After)
		normalRaw, _ := json.Marshal(saved.Data)
		err = tx.QueryRow(ctx, `INSERT INTO storage_replica_retries(id,org_id,replica_id,file_id,file_version_id,actor_id,session_id,business_operation_id,
 expected_version,result_version,basis_sha256,before_text,after_data,reason,normal_before_sha256,normal_after_sha256)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING created_at`, out.ID, c.Actor.OrgID, id, r.FileID, r.VersionID, c.Actor.UserID, c.Actor.SessionID, saved.OperationID, before.Version, after.Version, before.BasisSHA256, string(beforeRaw), afterRaw, in.Reason, saved.Event.BeforeDigest, core.Digest(normalRaw)).Scan(&out.CreatedAt)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: out, Event: core.Event{Type: "storage.replica.retry.recorded", ResourceType: "storage_replica_retry", ResourceID: out.ID, Version: 1, Payload: map[string]any{"replica_id": id, "file_id": r.FileID, "file_version_id": r.VersionID, "business_operation_id": saved.OperationID, "reason": in.Reason}}}, nil
	})
}

func (s *Service) StorageReplicaRetries(ctx context.Context, a core.Actor, replicaID, cursor string, limit int) (core.Page[StorageReplicaReceipt], error) {
	out := core.Page[StorageReplicaReceipt]{Items: []StorageReplicaReceipt{}}
	if !core.ValidID(replicaID) || limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("filters", "请核对副本及分页条件")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = storageAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	if _, err = storageReplica(ctx, tx, a.OrgID, replicaID); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "storage-replica-retries", replicaID)
	var before *time.Time
	var id *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, id = &v.UpdatedAt, &v.ID
	}
	rows, err := tx.Query(ctx, `SELECT id,replica_id,file_id,file_version_id,actor_id,business_operation_id,expected_version,result_version,basis_sha256,before_text,after_data,created_at
 FROM storage_replica_retries WHERE org_id=$1 AND replica_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, replicaID, before, id, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v StorageReplicaReceipt
		var b string
		var after []byte
		if err = rows.Scan(&v.ID, &v.ReplicaID, &v.FileID, &v.FileVersionID, &v.ActorID, &v.BusinessOperationID, &v.ExpectedVersion, &v.ResultVersion, &v.BasisSHA256, &b, &after, &v.CreatedAt); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(b), &v.Before); err != nil {
			return out, err
		}
		if err = json.Unmarshal(after, &v.After); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}
