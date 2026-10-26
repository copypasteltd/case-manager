package jobqueue

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID, OrgID, Type, AggregateType, AggregateID string
	AggregateVersion                            int64
	SchemaVersion                               int
	Payload                                     json.RawMessage
}
type Consumer struct {
	Name          string
	EventTypes    []string
	SchemaVersion int
	// Build validates and maps fixed event metadata into a typed job reference.
	// It must be pure: no external calls or domain writes.
	Build func(Event) (Input, error)
}
type Delivery struct {
	EventID, JobID string
	Rejected       bool
}

// DispatchOne atomically enqueues and records a single consumer delivery.
// It deliberately does not mark the event globally published: other consumers
// have independent processed_events records.
func (q Queue) DispatchOne(ctx context.Context, consumer Consumer) (*Delivery, error) {
	if !identifier.MatchString(consumer.Name) || consumer.SchemaVersion < 1 || consumer.SchemaVersion > 10000 || len(consumer.EventTypes) < 1 || len(consumer.EventTypes) > 64 || consumer.Build == nil {
		return nil, core.Invalid("consumer", "事件消费者配置无效")
	}
	for _, kind := range consumer.EventTypes {
		if !identifier.MatchString(kind) {
			return nil, core.Invalid("event_type", "事件类型无效")
		}
	}
	tx, err := q.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var e Event
	err = tx.QueryRow(ctx, `SELECT e.id,e.org_id,e.event_type,e.aggregate_type,e.aggregate_id,e.aggregate_version,e.schema_version,e.payload
        FROM outbox_events e WHERE e.event_type=ANY($2::text[])
        AND NOT EXISTS(SELECT 1 FROM processed_events p WHERE p.consumer=$1 AND p.event_id=e.id)
        ORDER BY e.occurred_at,e.id FOR UPDATE OF e SKIP LOCKED LIMIT 1`, consumer.Name, consumer.EventTypes).
		Scan(&e.ID, &e.OrgID, &e.Type, &e.AggregateType, &e.AggregateID, &e.AggregateVersion, &e.SchemaVersion, &e.Payload)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reject := ""
	var in Input
	if e.SchemaVersion != consumer.SchemaVersion {
		reject = "UNSUPPORTED_EVENT_VERSION"
	} else {
		in, err = consumer.Build(e)
		if err != nil {
			reject = "INVALID_EVENT_PAYLOAD"
		} else if in.OrgID != e.OrgID || validateInput(in) != nil {
			reject = "INVALID_JOB_INPUT"
		}
	}
	var jobID string
	if reject == "" {
		jobID, _, err = q.Enqueue(ctx, tx, in)
		if err != nil {
			if core.PublicError(err).Code == "JOB_DEDUP_MISMATCH" {
				reject = "JOB_INPUT_CONFLICT"
			} else {
				return nil, err
			}
		}
	}
	if reject != "" {
		payload, err := json.Marshal(map[string]any{"event_id": e.ID, "consumer": consumer.Name, "event_schema_version": e.SchemaVersion, "error_code": reject})
		if err != nil {
			return nil, err
		}
		jobID, _, err = q.Enqueue(ctx, tx, Input{OrgID: e.OrgID, Kind: "event.rejected.v1", Lane: "system", DedupKey: consumer.Name + "/" + e.ID, PayloadVersion: 1, Payload: payload, MaxAttempts: 1})
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET state='dead',last_error=$3,first_failed_at=clock_timestamp(),updated_at=clock_timestamp()
            WHERE org_id=$1 AND id=$2 AND state='queued'`, e.OrgID, jobID, reject); err != nil {
			return nil, err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO processed_events(consumer,event_id,result_ref) VALUES($1,$2,$3)", consumer.Name, e.ID, jobID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Delivery{EventID: e.ID, JobID: jobID, Rejected: reject != ""}, nil
}
