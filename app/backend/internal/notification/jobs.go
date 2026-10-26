package notification

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"slices"
	"strings"
	"time"
)

const JobKind = "notification.deliver.v1"

var EventTypes = []string{"task.created", "task.updated", "task.transitioned", "task.dependencies.changed",
	"request.submitted", "approval.step.decided", "subscription.reminder.due", "contract.reminder.due",
	"replica.failed", "incident.opened", "incident.updated", "run.completed", "run.failed", "run.cancelled", "run.interrupted"}

type jobInput struct {
	EventID string `json:"event_id"`
}
type event struct {
	jobqueue.Event
	OccurredAt time.Time
}
type JobResult struct {
	Delivered  int    `json:"delivered"`
	Suppressed int    `json:"suppressed"`
	Outcome    string `json:"outcome"`
	SafeCode   string `json:"safe_code"`
}

func Consumer() jobqueue.Consumer {
	return jobqueue.Consumer{Name: "notification-consumer.v1", EventTypes: slices.Clone(EventTypes), SchemaVersion: 1, Build: func(e jobqueue.Event) (jobqueue.Input, error) {
		if !core.ValidID(e.ID) || !core.ValidID(e.AggregateID) || e.AggregateVersion < 1 || !slices.Contains(EventTypes, e.Type) {
			return jobqueue.Input{}, core.Invalid("event", "通知业务事件无效")
		}
		raw, _ := json.Marshal(jobInput{EventID: e.ID})
		return jobqueue.Input{OrgID: e.OrgID, Kind: JobKind, Lane: "notification", DedupKey: "notification/event/" + e.ID, PayloadVersion: 1, Payload: raw, MaxAttempts: 4}, nil
	}}
}
func decodeJob(j jobqueue.Job) (jobInput, error) {
	var in jobInput
	d := json.NewDecoder(bytes.NewReader(j.Payload))
	d.DisallowUnknownFields()
	if j.Kind != JobKind || j.Lane != "notification" || j.PayloadVersion != 1 || len(j.Payload) > 1024 || d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || !core.ValidID(in.EventID) {
		return in, core.Invalid("job", "通知任务固定输入无效")
	}
	return in, nil
}
func newCandidate(e event, recipient, category, resourceType, resourceID string, context *string) thread {
	key := category + ":" + resourceID
	if context != nil && slices.Contains([]string{"approval", "contract_due", "subscription_due", "replica_failure"}, category) {
		key += ":" + *context
	}
	return thread{OrgID: e.OrgID, RecipientID: recipient, Category: category, ResourceType: resourceType, ResourceID: resourceID,
		ContextID: context, DedupKey: key, EventID: e.ID, DeliveredSeq: 1, EventOccurredAt: e.OccurredAt}
}

// Recipients come from the current business rows, never an unverified payload.
// An old event is not reinterpreted as a new task or a new approval assignment.
func recipients(ctx context.Context, tx pgx.Tx, e event) ([]thread, error) {
	out := []thread{}
	addRows := func(query string, args []any, category, resource string, withContext bool) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var recipient, id string
			var context *string
			if withContext {
				err = rows.Scan(&recipient, &id, &context)
			} else {
				err = rows.Scan(&recipient, &id)
			}
			if err != nil {
				return err
			}
			out = append(out, newCandidate(e, recipient, category, resource, id, context))
			if len(out) > 100 {
				return core.Fail(422, "NOTIFICATION_RECIPIENT_LIMIT", "通知接收范围超过当前单事件上限")
			}
		}
		return rows.Err()
	}
	args := []any{e.OrgID, e.AggregateID, e.AggregateVersion}
	switch {
	case strings.HasPrefix(e.Type, "task."):
		if e.AggregateType != "task" {
			return out, core.Invalid("event", "任务事件目标类型不一致")
		}
		err := addRows(`SELECT assignee_id,id FROM tasks WHERE org_id=$1 AND id=$2 AND version=$3 AND assignee_id IS NOT NULL AND state NOT IN ('done','cancelled')`, args, "task", "task", false)
		return out, err
	case e.Type == "request.submitted" || e.Type == "approval.step.decided":
		if e.AggregateType != "request" {
			return out, core.Invalid("event", "审批事件目标类型不一致")
		}
		if err := addRows(`SELECT s.assigned_user_id,r.id,s.id FROM requests r JOIN approval_steps s ON s.org_id=r.org_id AND s.instance_id=r.active_instance_id AND s.revision_id=r.current_revision_id
   WHERE r.org_id=$1 AND r.id=$2 AND r.version=$3 AND r.state='in_review' AND s.state='active'`, args, "approval", "request", true); err != nil {
			return out, err
		}
		err := addRows(`SELECT applicant_id,id FROM requests WHERE org_id=$1 AND id=$2 AND version=$3 AND state IN ('returned','rejected','approved')`, args, "request_result", "request", false)
		return out, err
	case e.Type == "subscription.reminder.due":
		if e.AggregateType != "subscription_period" {
			return out, core.Invalid("event", "订阅事件目标类型不一致")
		}
		err := addRows(`SELECT r.owner_id,p.subscription_id,p.id FROM subscription_reminders n JOIN subscription_periods p ON p.org_id=n.org_id AND p.id=n.period_id
   JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
   WHERE n.org_id=$1 AND n.outbox_id=$4 AND p.id=$2 AND p.version=$3 AND p.state='forecast' AND r.owner_id IS NOT NULL
   AND n.recipient_id=r.owner_id AND subscription_charge_eligible(p.org_id,p.subscription_id,p.start_date)`, append(args, e.ID), "subscription_due", "subscription", true)
		return out, err
	case e.Type == "contract.reminder.due":
		if e.AggregateType != "contract" {
			return out, core.Invalid("event", "合同事件目标类型不一致")
		}
		err := addRows(`SELECT COALESCE(r.owner_id,c.creator_id),c.id,n.id FROM notification_due_events n JOIN contracts c ON c.org_id=n.org_id AND c.id=n.contract_id
   JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
   WHERE c.org_id=$1 AND c.id=$2 AND c.version=$3 AND n.event_id=$4 AND c.lifecycle='effective' AND n.revision_id=r.id AND n.due_on=r.end_date`, append(args, e.ID), "contract_due", "contract", true)
		return out, err
	case e.Type == "replica.failed":
		if e.AggregateType != "file_replica" {
			return out, core.Invalid("event", "副本事件目标类型不一致")
		}
		err := addRows(`SELECT DISTINCT v.author_id,v.file_id,r.id FROM file_replicas r JOIN file_versions v ON v.org_id=r.org_id AND v.id=r.version_id
   WHERE r.org_id=$1 AND r.id=$2 AND r.version=$3 AND r.state='failed' AND v.author_id IS NOT NULL
   UNION SELECT f.creator_id,f.id,r.id FROM file_replicas r JOIN file_versions v ON v.org_id=r.org_id AND v.id=r.version_id JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
   WHERE r.org_id=$1 AND r.id=$2 AND r.version=$3 AND r.state='failed' AND f.creator_id IS NOT NULL`, args, "replica_failure", "file", true)
		return out, err
	case strings.HasPrefix(e.Type, "incident."):
		if e.AggregateType != "incident" {
			return out, core.Invalid("event", "服务事件目标类型不一致")
		}
		err := addRows(`SELECT i.owner_id,i.id FROM incidents i WHERE i.org_id=$1 AND i.id=$2 AND i.version=$3
   UNION SELECT s.owner_id,i.id FROM incidents i JOIN services s ON s.org_id=i.org_id AND s.id=ANY(i.service_ids)
   WHERE i.org_id=$1 AND i.id=$2 AND i.version=$3`, args, "incident", "incident", false)
		return out, err
	case strings.HasPrefix(e.Type, "run."):
		if e.AggregateType != "agent_run" {
			return out, core.Invalid("event", "秘书事件目标类型不一致")
		}
		err := addRows(`SELECT actor_id,id FROM agent_runs WHERE org_id=$1 AND id=$2 AND version=$3 AND state IN ('succeeded','failed','cancelled','interrupted')`, args, "secretary", "agent_run", false)
		return out, err
	}
	return out, core.Invalid("event", "未接入的通知业务事件")
}

func eligible(ctx context.Context, tx pgx.Tx, orgVersion int64, n thread) (bool, error) {
	var auth int64
	var state string
	err := tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", n.OrgID, n.RecipientID).Scan(&auth, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if state != "active" {
		return false, nil
	}
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		a := core.Actor{OrgID: n.OrgID, UserID: n.RecipientID, AuthVersion: auth, OrgAuthVersion: orgVersion, Channel: channel}
		v, e := view(ctx, tx, a, n)
		if e != nil {
			return false, e
		}
		if v == nil {
			continue
		}
		if slices.Contains([]string{"task", "approval", "contract_due", "subscription_due", "replica_failure"}, n.Category) && !v.AttentionRequired {
			continue
		}
		return true, nil
	}
	return false, nil
}
func deliver(ctx context.Context, tx pgx.Tx, j jobqueue.Job, e event, n thread) (bool, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 59312))", n.OrgID+"/"+n.RecipientID+"/"+n.DedupKey); err != nil {
		return false, err
	}
	previous, err := scan(tx.QueryRow(ctx, "SELECT "+threadColumns+threadJoin+"WHERE n.org_id=$1 AND n.recipient_id=$2 AND n.channel='in_app' AND n.dedup_key=$3 FOR UPDATE OF n", n.OrgID, n.RecipientID, n.DedupKey))
	fresh := core.PublicError(err).Status == 404
	if err != nil && !fresh {
		return false, err
	}
	if !fresh {
		if previous.EventOccurredAt.After(e.OccurredAt) || (previous.EventOccurredAt.Equal(e.OccurredAt) && previous.EventID >= e.ID) {
			return false, nil
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM notification_deliveries WHERE org_id=$1 AND business_event_id=$2 AND recipient_id=$3 AND channel='in_app' AND template_version=1)", n.OrgID, e.ID, n.RecipientID).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
		n.ID = previous.ID
		n.DeliveredSeq = previous.DeliveredSeq + 1
	} else {
		n.ID = core.NewID()
		n.DeliveredSeq = 1
	}
	deliveryID := core.NewID()
	if fresh {
		_, err = tx.Exec(ctx, `INSERT INTO notification_threads(id,org_id,recipient_id,dedup_key,category,resource_type,resource_id,context_id,delivered_seq,latest_delivery_id)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$9)`, n.ID, n.OrgID, n.RecipientID, n.DedupKey, n.Category, n.ResourceType, n.ResourceID, n.ContextID, deliveryID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE notification_threads SET delivered_seq=$3,latest_delivery_id=$4,version=version+1,updated_at=greatest(updated_at,clock_timestamp()) WHERE org_id=$1 AND id=$2`, n.OrgID, n.ID, n.DeliveredSeq, deliveryID)
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notification_deliveries(id,org_id,thread_id,recipient_id,channel,business_event_id,template_version,dedup_key,seq,event_occurred_at,job_id,job_fence,job_owner)
  VALUES($1,$2,$3,$4,'in_app',$5,1,$6,$7,$8,$9,$10,$11)`, deliveryID, n.OrgID, n.ID, n.RecipientID, e.ID, n.DedupKey, n.DeliveredSeq, e.OccurredAt, j.ID, j.FencingToken, j.LeaseOwner)
	return err == nil, err
}

func (s *Service) Process(ctx context.Context, j jobqueue.Job) (JobResult, error) {
	out := JobResult{Outcome: "suppressed", SafeCode: "NO_CURRENT_RECIPIENT"}
	in, err := decodeJob(j)
	if err != nil {
		return out, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var orgVersion int64
	if err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&orgVersion); err != nil {
		return out, err
	}
	var e event
	err = tx.QueryRow(ctx, `SELECT id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,schema_version,payload,occurred_at
  FROM outbox_events WHERE org_id=$1 AND id=$2`, j.OrgID, in.EventID).Scan(&e.ID, &e.OrgID, &e.Type, &e.AggregateType, &e.AggregateID, &e.AggregateVersion, &e.SchemaVersion, &e.Payload, &e.OccurredAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if e.SchemaVersion != 1 || !slices.Contains(EventTypes, e.Type) {
		return out, core.Invalid("event", "业务事件版本或类型未接入")
	}
	pending, err := recipients(ctx, tx, e)
	if err != nil {
		return out, err
	}
	slices.SortFunc(pending, func(a, b thread) int {
		return strings.Compare(a.RecipientID+"/"+a.DedupKey, b.RecipientID+"/"+b.DedupKey)
	})
	queue := jobqueue.Queue{DB: s.DB}
	if err = queue.CheckInTx(ctx, tx, j); err != nil {
		return out, err
	}
	for _, n := range pending {
		ok, e2 := eligible(ctx, tx, orgVersion, n)
		if e2 != nil {
			return out, e2
		}
		if !ok {
			out.Suppressed++
			continue
		}
		created, e2 := deliver(ctx, tx, j, e, n)
		if e2 != nil {
			return out, e2
		}
		if created {
			out.Delivered++
		} else {
			out.Suppressed++
		}
	}
	if out.Delivered > 0 {
		out.Outcome = "delivered"
		out.SafeCode = "DELIVERED"
	} else if len(pending) > 0 {
		out.SafeCode = "STALE_OR_INACCESSIBLE"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notification_job_results(org_id,job_id,business_event_id,delivered,suppressed,outcome,safe_code,job_fence,job_owner)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, j.OrgID, j.ID, e.ID, out.Delivered, out.Suppressed, out.Outcome, out.SafeCode, j.FencingToken, j.LeaseOwner); err != nil {
		return out, err
	}
	if err = queue.CompleteInTx(ctx, tx, j); err != nil {
		return out, err
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
