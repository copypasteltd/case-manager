package notification

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type RetryInput struct {
	ExpectedRevision string `json:"expected_revision"`
	Reason           string `json:"reason"`
}
type RetryReceipt struct {
	ID              string    `json:"id"`
	SourceJobID     string    `json:"source_job_id"`
	RetryJobID      string    `json:"retry_job_id"`
	BusinessEventID string    `json:"business_event_id"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
type DeliveryOutcome struct {
	Delivered   int       `json:"delivered"`
	Suppressed  int       `json:"suppressed"`
	Outcome     string    `json:"outcome"`
	SafeCode    string    `json:"safe_code"`
	CompletedAt time.Time `json:"completed_at"`
}
type DeliveryJob struct {
	JobID           string           `json:"job_id"`
	Kind            string           `json:"kind"`
	State           string           `json:"state"`
	SafeCode        *string          `json:"safe_code"`
	Attempt         int              `json:"attempt"`
	MaxAttempts     int              `json:"max_attempts"`
	FirstFailedAt   *time.Time       `json:"first_failed_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	AvailableAt     time.Time        `json:"available_at"`
	Revision        string           `json:"revision"`
	BusinessEventID *string          `json:"business_event_id"`
	EventType       *string          `json:"event_type"`
	CanRetry        bool             `json:"can_retry"`
	RetryBlock      string           `json:"retry_block"`
	RetryID         *string          `json:"retry_id"`
	RetryJobID      *string          `json:"retry_job_id"`
	ParentJobID     *string          `json:"parent_job_id"`
	Result          *DeliveryOutcome `json:"result"`
}
type deliveryRecord struct {
	DeliveryJob
	payload        []byte
	payloadVersion int
	fence          int64
	leaseOwner     *string
	leaseUntil     *time.Time
}

func deliveryAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, retry bool) error {
	if err := authz.RequireOrganization(ctx, tx, a, "notification.delivery.read"); err != nil {
		return err
	}
	if retry {
		if err := authz.RequireOrganization(ctx, tx, a, "notification.delivery.retry"); err != nil {
			return err
		}
	}
	return identity.RequireInteractiveSession(ctx, tx, a, retry)
}

// The revision is an optimistic precondition, never a permission or a job lease.
func deliveryRevision(v deliveryRecord) string {
	b, _ := json.Marshal([]any{v.JobID, v.State, v.Attempt, v.MaxAttempts, v.fence, v.UpdatedAt.UTC().Format(time.RFC3339Nano), core.Digest(v.payload), v.RetryJobID})
	return core.Digest(b)
}
func deliveryRecordAt(ctx context.Context, tx pgx.Tx, org, id, lock string) (deliveryRecord, error) {
	var v deliveryRecord
	if !core.ValidID(id) {
		return v, core.Missing()
	}
	suffix := ""
	if lock == "update" {
		suffix = " FOR UPDATE OF j"
	}
	if lock == "share" {
		suffix = " FOR SHARE OF j"
	}
	err := tx.QueryRow(ctx, `SELECT j.id,j.kind,j.state,j.last_error,j.attempt,j.max_attempts,j.first_failed_at,j.updated_at,j.available_at,
 j.payload,j.payload_version,j.fencing_token,j.lease_owner,j.lease_until,
 (SELECT id FROM notification_retries WHERE org_id=j.org_id AND source_job_id=j.id),
 (SELECT retry_job_id FROM notification_retries WHERE org_id=j.org_id AND source_job_id=j.id),
 (SELECT source_job_id FROM notification_retries WHERE org_id=j.org_id AND retry_job_id=j.id)
 FROM jobs j WHERE j.org_id=$1 AND j.id=$2
 AND ((j.kind='notification.deliver.v1' AND j.lane='notification') OR (j.kind='event.rejected.v1' AND j.dedup_key LIKE 'notification-consumer.v1/%'))`+suffix,
		org, id).Scan(&v.JobID, &v.Kind, &v.State, &v.SafeCode, &v.Attempt, &v.MaxAttempts, &v.FirstFailedAt, &v.UpdatedAt, &v.AvailableAt,
		&v.payload, &v.payloadVersion, &v.fence, &v.leaseOwner, &v.leaseUntil, &v.RetryID, &v.RetryJobID, &v.ParentJobID)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	v.Revision = deliveryRevision(v)
	v.RetryBlock = "ready"
	if v.Kind != JobKind {
		v.RetryBlock = "rejected_event"
		return v, nil
	}
	in, err := decodeJob(jobqueue.Job{Kind: v.Kind, Lane: "notification", PayloadVersion: v.payloadVersion, Payload: v.payload})
	if err != nil {
		v.RetryBlock = "invalid_input"
		return v, nil
	}
	v.BusinessEventID = &in.EventID
	var eventType string
	var schema int
	err = tx.QueryRow(ctx, "SELECT event_type,schema_version FROM outbox_events WHERE org_id=$1 AND id=$2", org, in.EventID).Scan(&eventType, &schema)
	if err == pgx.ErrNoRows {
		v.RetryBlock = "missing_event"
		return v, nil
	}
	if err != nil {
		return v, err
	}
	v.EventType = &eventType
	if schema != 1 || !slices.Contains(EventTypes, eventType) {
		v.RetryBlock = "unsupported_event"
	}
	if v.RetryBlock == "ready" {
		switch {
		case v.RetryJobID != nil:
			v.RetryBlock = "already_retried"
		case v.State != "dead" || v.leaseOwner != nil || v.leaseUntil != nil:
			v.RetryBlock = "not_failed"
		}
	}
	var out DeliveryOutcome
	err = tx.QueryRow(ctx, `SELECT delivered,suppressed,outcome,safe_code,created_at FROM notification_job_results WHERE org_id=$1 AND job_id=$2`, org, id).Scan(&out.Delivered, &out.Suppressed, &out.Outcome, &out.SafeCode, &out.CompletedAt)
	if err != nil && err != pgx.ErrNoRows {
		return v, err
	}
	if err == nil {
		v.Result = &out
	}
	return v, nil
}

func (s *Service) DeliveryJob(ctx context.Context, a core.Actor, id string) (DeliveryJob, error) {
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return DeliveryJob{}, err
	}
	defer tx.Rollback(ctx)
	if err = deliveryAuthority(ctx, tx, a, false); err != nil {
		return DeliveryJob{}, err
	}
	v, err := deliveryRecordAt(ctx, tx, a.OrgID, id, "share")
	if err != nil {
		return DeliveryJob{}, err
	}
	// A read-only operator can inspect diagnostics without gaining retry authority.
	v.CanRetry = v.RetryBlock == "ready" && authz.RequireOrganization(ctx, tx, a, "notification.delivery.retry") == nil
	if v.RetryBlock == "ready" && !v.CanRetry {
		v.RetryBlock = "not_authorized"
	}
	return v.DeliveryJob, platform.DatabaseError(tx.Commit(ctx))
}

func (s *Service) RetryDelivery(ctx context.Context, c core.Command, id string, in RetryInput) (core.Result, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.ExpectedRevision) != 64 || strings.Trim(in.ExpectedRevision, "0123456789abcdef") != "" || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("retry", "请核对准确任务版本并填写1至200字的重试原因")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := deliveryAuthority(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		_, err := deliveryRecordAt(ctx, tx, c.Actor.OrgID, id, "")
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		v, err := deliveryRecordAt(ctx, tx, c.Actor.OrgID, id, "update")
		if err != nil {
			return core.Result{}, err
		}
		if v.Revision != in.ExpectedRevision {
			return core.Result{}, core.Conflict()
		}
		if v.RetryBlock != "ready" {
			return core.Result{}, core.Fail(409, "NOTIFICATION_RETRY_UNAVAILABLE", "该任务尚未最终失败、已创建重试，或固定输入不能安全重放，请刷新核对")
		}
		queue := jobqueue.Queue{DB: s.DB}
		child, inserted, err := queue.Enqueue(ctx, tx, jobqueue.Input{OrgID: c.Actor.OrgID, Kind: JobKind, Lane: "notification", DedupKey: "notification/retry/" + v.JobID, PayloadVersion: 1, Payload: v.payload, MaxAttempts: 4})
		if err != nil {
			return core.Result{}, err
		}
		if !inserted {
			return core.Result{}, core.Fail(409, "NOTIFICATION_RETRY_UNAVAILABLE", "该失败任务已有重试，请查看已有记录")
		}
		receipt := RetryReceipt{ID: core.NewID(), SourceJobID: v.JobID, RetryJobID: child, BusinessEventID: *v.BusinessEventID, CreatedBy: c.Actor.UserID}
		err = tx.QueryRow(ctx, `INSERT INTO notification_retries(id,org_id,source_job_id,retry_job_id,business_event_id,created_by,reason)
   VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, receipt.ID, c.Actor.OrgID, receipt.SourceJobID, receipt.RetryJobID, receipt.BusinessEventID, receipt.CreatedBy, in.Reason).Scan(&receipt.CreatedAt)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: receipt, Event: core.Event{Type: "notification.delivery.retry", ResourceType: "notification_retry", ResourceID: receipt.ID, Version: 1,
			Payload: map[string]any{"source_job_id": v.JobID, "retry_job_id": child, "business_event_id": receipt.BusinessEventID, "reason": in.Reason}}}, nil
	})
}

func (s *Service) RetryHistory(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[RetryReceipt], error) {
	out := core.Page[RetryReceipt]{Items: []RetryReceipt{}}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条重试记录")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = deliveryAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	bound := core.CursorBinding(a, "notification-retries")
	var at *time.Time
	var id *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		id = &v.ID
	}
	rows, err := tx.Query(ctx, `SELECT id,source_job_id,retry_job_id,business_event_id,created_by,created_at FROM notification_retries
 WHERE org_id=$1 AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT $4`, a.OrgID, at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v RetryReceipt
		if err = rows.Scan(&v.ID, &v.SourceJobID, &v.RetryJobID, &v.BusinessEventID, &v.CreatedBy, &v.CreatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &next
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
