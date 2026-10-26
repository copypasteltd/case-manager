-- In-app delivery is a business-event projection, not completion of its target.
CREATE UNIQUE INDEX outbox_organization_identity ON outbox_events(org_id,id);
CREATE TABLE notification_threads (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,recipient_id uuid NOT NULL,
 channel text NOT NULL DEFAULT 'in_app' CHECK(channel='in_app'),
 dedup_key text NOT NULL CHECK(length(dedup_key) BETWEEN 1 AND 200),
 category text NOT NULL CHECK(category IN ('task','approval','request_result','contract_due','subscription_due','replica_failure','incident','secretary')),
 resource_type text NOT NULL CHECK(resource_type IN ('task','request','contract','subscription','file','incident','agent_run')),
 resource_id uuid NOT NULL,context_id uuid,
 delivered_seq bigint NOT NULL CHECK(delivered_seq BETWEEN 1 AND 9007199254740991),
 read_seq bigint NOT NULL DEFAULT 0 CHECK(read_seq BETWEEN 0 AND delivered_seq),
 latest_delivery_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),read_at timestamptz,
 UNIQUE(org_id,id),UNIQUE(org_id,recipient_id,channel,dedup_key),
 FOREIGN KEY(org_id,recipient_id) REFERENCES users(org_id,id),
 CHECK((read_seq=0)=(read_at IS NULL))
);
CREATE INDEX notifications_recipient_order ON notification_threads(org_id,recipient_id,updated_at DESC,id DESC);
CREATE INDEX notifications_unread ON notification_threads(org_id,recipient_id,updated_at DESC,id DESC) WHERE read_seq<delivered_seq;
CREATE TABLE notification_deliveries (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,thread_id uuid NOT NULL,recipient_id uuid NOT NULL,
 channel text NOT NULL CHECK(channel='in_app'),business_event_id uuid NOT NULL,
 template_version integer NOT NULL CHECK(template_version=1),dedup_key text NOT NULL,
 seq bigint NOT NULL CHECK(seq>0),event_occurred_at timestamptz NOT NULL,
 job_id uuid NOT NULL REFERENCES jobs(id),job_fence bigint NOT NULL CHECK(job_fence>0),job_owner text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),UNIQUE(org_id,thread_id,id),UNIQUE(org_id,thread_id,seq),
 UNIQUE(org_id,business_event_id,recipient_id,channel,template_version),
 FOREIGN KEY(org_id,thread_id) REFERENCES notification_threads(org_id,id),
 FOREIGN KEY(org_id,recipient_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,business_event_id) REFERENCES outbox_events(org_id,id)
);
ALTER TABLE notification_threads ADD FOREIGN KEY(org_id,id,latest_delivery_id)
 REFERENCES notification_deliveries(org_id,thread_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER notification_delivery_immutable BEFORE UPDATE OR DELETE ON notification_deliveries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE notification_job_results (
 org_id uuid NOT NULL,job_id uuid PRIMARY KEY REFERENCES jobs(id),business_event_id uuid NOT NULL,
 delivered integer NOT NULL CHECK(delivered BETWEEN 0 AND 100),
 suppressed integer NOT NULL CHECK(suppressed BETWEEN 0 AND 100),
 outcome text NOT NULL CHECK(outcome IN ('delivered','suppressed')),
 safe_code text NOT NULL CHECK(safe_code~'^[A-Z][A-Z0-9_]{1,79}$'),
 job_fence bigint NOT NULL CHECK(job_fence>0),job_owner text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,business_event_id) REFERENCES outbox_events(org_id,id),
 CHECK((outcome='delivered')=(delivered>0))
);
CREATE TRIGGER notification_result_immutable BEFORE UPDATE OR DELETE ON notification_job_results FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE notification_due_events (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,contract_id uuid NOT NULL,revision_id uuid NOT NULL,
 due_on date NOT NULL,days_before integer NOT NULL CHECK(days_before BETWEEN 0 AND 365),
 timezone text NOT NULL,event_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,contract_id,revision_id,due_on,days_before),
 FOREIGN KEY(org_id,contract_id) REFERENCES contracts(org_id,id),
 FOREIGN KEY(org_id,event_id) REFERENCES outbox_events(org_id,id)
);
CREATE TRIGGER notification_due_immutable BEFORE UPDATE OR DELETE ON notification_due_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_notification_due_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
   JOIN outbox_events e ON e.org_id=c.org_id AND e.id=NEW.event_id
   WHERE c.org_id=NEW.org_id AND c.id=NEW.contract_id AND c.lifecycle='effective' AND r.id=NEW.revision_id AND r.end_date=NEW.due_on
   AND NEW.due_on-NEW.days_before <= (clock_timestamp() AT TIME ZONE NEW.timezone)::date
   AND e.event_type='contract.reminder.due' AND e.aggregate_type='contract' AND e.aggregate_id=c.id AND e.aggregate_version=c.version AND e.schema_version=1
   AND e.payload->>'revision_id'=NEW.revision_id::text AND e.payload->>'due_on'=NEW.due_on::text
   AND e.payload->>'days_before'=NEW.days_before::text AND e.payload->>'timezone'=NEW.timezone) THEN
   RAISE EXCEPTION 'contract due notification must pin the current effective revision and actual due event' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_due_guard BEFORE INSERT ON notification_due_events FOR EACH ROW EXECUTE FUNCTION guard_notification_due_insert();

CREATE FUNCTION guard_notification_head() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification heads cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.org_id,NEW.id,NEW.recipient_id,NEW.channel,NEW.dedup_key,NEW.category,NEW.resource_type,NEW.resource_id,NEW.context_id,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.recipient_id,OLD.channel,OLD.dedup_key,OLD.category,OLD.resource_type,OLD.resource_id,OLD.context_id,OLD.created_at)
   OR NEW.version<>OLD.version+1 OR NEW.read_seq<OLD.read_seq OR NEW.delivered_seq NOT IN (OLD.delivered_seq,OLD.delivered_seq+1)
   OR (NEW.delivered_seq=OLD.delivered_seq AND (NEW.latest_delivery_id<>OLD.latest_delivery_id OR NEW.updated_at<>OLD.updated_at))
   OR (NEW.delivered_seq>OLD.delivered_seq AND NEW.read_seq<>OLD.read_seq) THEN
   RAISE EXCEPTION 'notification identity and acknowledged delivery boundaries are immutable' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_head_guard BEFORE UPDATE OR DELETE ON notification_threads FOR EACH ROW EXECUTE FUNCTION guard_notification_head();
CREATE FUNCTION guard_notification_worker_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j jobs; h notification_threads;
BEGIN
 SELECT * INTO STRICT j FROM jobs WHERE id=NEW.job_id;
 IF j.org_id<>NEW.org_id OR j.kind<>'notification.deliver.v1' OR j.lane<>'notification' OR j.state<>'running'
  OR j.lease_owner IS DISTINCT FROM NEW.job_owner OR j.fencing_token<>NEW.job_fence OR j.lease_until<=clock_timestamp()
  OR j.payload->>'event_id' IS DISTINCT FROM NEW.business_event_id::text THEN
  RAISE EXCEPTION 'notification insert requires the live exact worker lease' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='notification_deliveries' THEN
  SELECT * INTO STRICT h FROM notification_threads WHERE id=NEW.thread_id;
  IF h.org_id<>NEW.org_id OR h.recipient_id<>NEW.recipient_id OR h.channel<>NEW.channel OR h.dedup_key<>NEW.dedup_key
   OR NEW.event_occurred_at IS DISTINCT FROM (SELECT occurred_at FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.business_event_id) THEN
   RAISE EXCEPTION 'notification delivery must bind its thread and business event' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_delivery_worker BEFORE INSERT ON notification_deliveries FOR EACH ROW EXECUTE FUNCTION guard_notification_worker_insert();
CREATE TRIGGER notification_result_worker BEFORE INSERT ON notification_job_results FOR EACH ROW EXECUTE FUNCTION guard_notification_worker_insert();
CREATE FUNCTION validate_notification_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE h notification_threads; d notification_deliveries; j jobs; r notification_job_results;
BEGIN
 IF TG_TABLE_NAME='notification_threads' THEN
  SELECT * INTO STRICT h FROM notification_threads WHERE id=NEW.id;
  SELECT * INTO STRICT d FROM notification_deliveries WHERE id=h.latest_delivery_id;
  IF d.org_id<>h.org_id OR d.thread_id<>h.id OR d.recipient_id<>h.recipient_id OR d.channel<>h.channel OR d.dedup_key<>h.dedup_key
   OR d.seq<>h.delivered_seq OR h.delivered_seq<>(SELECT max(seq) FROM notification_deliveries WHERE thread_id=h.id)
   OR h.delivered_seq<>(SELECT count(*) FROM notification_deliveries WHERE thread_id=h.id) THEN
   RAISE EXCEPTION 'notification head must match contiguous real deliveries' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO STRICT j FROM jobs WHERE id=NEW.job_id;
  SELECT * INTO STRICT r FROM notification_job_results WHERE job_id=j.id;
  IF j.org_id<>NEW.org_id OR j.kind<>'notification.deliver.v1' OR j.lane<>'notification' OR j.state<>'succeeded'
   OR j.fencing_token<>NEW.job_fence OR j.fencing_token<>r.job_fence OR r.org_id<>j.org_id
   OR j.payload->>'event_id'<>NEW.business_event_id::text OR r.business_event_id<>NEW.business_event_id
   OR r.job_owner<>NEW.job_owner OR r.delivered<>(SELECT count(*) FROM notification_deliveries WHERE job_id=j.id) THEN
   RAISE EXCEPTION 'notification evidence requires its atomic fenced successful job' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER notification_head_evidence AFTER INSERT OR UPDATE ON notification_threads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_evidence();
CREATE CONSTRAINT TRIGGER notification_delivery_evidence AFTER INSERT ON notification_deliveries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_evidence();
CREATE CONSTRAINT TRIGGER notification_result_evidence AFTER INSERT ON notification_job_results DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_evidence();

INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'notification_operator','通知投递运维',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'notification.delivery.read' FROM roles WHERE key='notification_operator';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT,UPDATE ON notification_threads TO copypaste_app;
  GRANT SELECT,INSERT ON notification_deliveries,notification_job_results,notification_due_events TO copypaste_app;
 END IF;
END; $$;
