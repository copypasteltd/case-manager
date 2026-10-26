package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
)

func TestOutboxConsumerAtomicity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	queue := jobqueue.Queue{DB: f.api.DB}
	event := func(version int) string {
		id := core.NewID()
		_, err := f.owner.Exec(ctx, "INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,schema_version,payload,request_id) VALUES($1,$2,'storage.verify.requested','upload',$3,1,$4,'{}',$5)", id, f.org, core.NewID(), version, core.NewID())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	consumer := jobqueue.Consumer{Name: "storage.verify.consumer.v1", EventTypes: []string{"storage.verify.requested"}, SchemaVersion: 1, Build: func(e jobqueue.Event) (jobqueue.Input, error) {
		return jobqueue.Input{OrgID: e.OrgID, Kind: "storage.verify.v1", Lane: "large", DedupKey: "verify/" + e.AggregateID, PayloadVersion: 1, Payload: json.RawMessage(fmt.Sprintf(`{"source_id":%q}`, e.AggregateID)), MaxAttempts: 3}, nil
	}}
	t.Run("enqueue and delivery roll back together and each consumer gets one delivery", func(t *testing.T) {
		id := event(1)
		_, err := f.owner.Exec(ctx, `CREATE FUNCTION reject_delivery_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN RAISE EXCEPTION 'delivery fixture failure'; END; $$;
            CREATE TRIGGER reject_delivery_fixture BEFORE INSERT ON processed_events FOR EACH ROW EXECUTE FUNCTION reject_delivery_fixture();`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = queue.DispatchOne(ctx, consumer); err == nil {
			t.Fatal("delivery failure was swallowed")
		}
		var count int
		if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE org_id=$1", f.org).Scan(&count); err != nil || count != 0 {
			t.Fatal("job survived failed delivery transaction", count, err)
		}
		if _, err = f.owner.Exec(ctx, "DROP TRIGGER reject_delivery_fixture ON processed_events; DROP FUNCTION reject_delivery_fixture()"); err != nil {
			t.Fatal(err)
		}
		delivered, err := queue.DispatchOne(ctx, consumer)
		if err != nil || delivered == nil || delivered.EventID != id || delivered.Rejected {
			t.Fatal("event was not delivered after rollback", err)
		}
		if replay, err := queue.DispatchOne(ctx, consumer); err != nil || replay != nil {
			t.Fatal("same consumer delivered event twice", err)
		}
		second := consumer
		second.Name = "storage.secondary.consumer.v1"
		another, err := queue.DispatchOne(ctx, second)
		if err != nil || another == nil || another.EventID != id || another.JobID != delivered.JobID {
			t.Fatal("independent consumer or semantic job deduplication lost", err)
		}
	})
	t.Run("unknown schemas are visible dead letters and do not execute or block later events", func(t *testing.T) {
		id := event(99)
		called := 0
		protected := consumer
		protected.Build = func(e jobqueue.Event) (jobqueue.Input, error) { called++; return consumer.Build(e) }
		rejected, err := queue.DispatchOne(ctx, protected)
		if err != nil || rejected == nil || !rejected.Rejected || rejected.EventID != id || called != 0 {
			t.Fatal("unknown schema executed or disappeared", err)
		}
		var state, kind, code string
		if err = f.owner.QueryRow(ctx, "SELECT state,kind,last_error FROM jobs WHERE org_id=$1 AND id=$2", f.org, rejected.JobID).Scan(&state, &kind, &code); err != nil || state != "dead" || kind != "event.rejected.v1" || code != "UNSUPPORTED_EVENT_VERSION" {
			t.Fatal("dead letter lacks controlled evidence", state, kind, code, err)
		}
		next := event(1)
		delivered, err := queue.DispatchOne(ctx, protected)
		if err != nil || delivered == nil || delivered.EventID != next || delivered.Rejected || called != 1 {
			t.Fatal("unknown schema blocked a later valid event", err)
		}
	})
}
