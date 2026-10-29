-- Manual retries append a new leased job; terminal failures and fixed input stay intact.
CREATE TABLE notification_retries (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 source_job_id uuid NOT NULL, retry_job_id uuid NOT NULL,
 business_event_id uuid NOT NULL, created_by uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,source_job_id), UNIQUE(org_id,retry_job_id),
 FOREIGN KEY(org_id,source_job_id) REFERENCES jobs(org_id,id),
 FOREIGN KEY(org_id,retry_job_id) REFERENCES jobs(org_id,id),
 FOREIGN KEY(org_id,business_event_id) REFERENCES outbox_events(org_id,id),
 FOREIGN KEY(org_id,created_by) REFERENCES users(org_id,id),
 CHECK(source_job_id<>retry_job_id)
);
CREATE INDEX notification_retries_history ON notification_retries(org_id,created_at DESC,id DESC);
CREATE TRIGGER notification_retry_immutable BEFORE UPDATE OR DELETE ON notification_retries
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION guard_notification_retry() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source jobs%ROWTYPE; child jobs%ROWTYPE;
BEGIN
 SELECT * INTO STRICT source FROM jobs WHERE org_id=NEW.org_id AND id=NEW.source_job_id FOR UPDATE;
 SELECT * INTO STRICT child FROM jobs WHERE org_id=NEW.org_id AND id=NEW.retry_job_id FOR UPDATE;
 IF source.kind<>'notification.deliver.v1' OR source.lane<>'notification' OR source.state<>'dead'
  OR source.lease_owner IS NOT NULL OR source.lease_until IS NOT NULL
  OR source.payload_version<>1 OR source.payload<>jsonb_build_object('event_id',source.payload->>'event_id')
  OR (source.payload->>'event_id')::uuid IS DISTINCT FROM NEW.business_event_id
  OR child.kind<>source.kind OR child.lane<>source.lane OR child.payload_version<>source.payload_version
  OR child.payload<>source.payload OR child.max_attempts<>4 OR child.attempt<>0 OR child.fencing_token<>0
  OR child.state<>'queued' OR child.lease_owner IS NOT NULL OR child.lease_until IS NOT NULL
  OR child.dedup_key<>'notification/retry/'||NEW.source_job_id::text THEN
  RAISE EXCEPTION 'notification retry must bind an unchanged terminal input and a fresh child' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.business_event_id AND schema_version=1) THEN
  RAISE EXCEPTION 'notification retry requires an existing supported event version' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_retry_input BEFORE INSERT ON notification_retries
 FOR EACH ROW EXECUTE FUNCTION guard_notification_retry();

CREATE FUNCTION validate_notification_retry_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM audit_events WHERE org_id=NEW.org_id AND resource_id=NEW.id
  AND actor_id=NEW.created_by AND action='notification.delivery.retry')
 OR NOT EXISTS(SELECT 1 FROM outbox_events WHERE org_id=NEW.org_id AND aggregate_id=NEW.id
  AND event_type='notification.delivery.retry') THEN
  RAISE EXCEPTION 'notification retry requires atomic audit and outbox evidence' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER notification_retry_evidence AFTER INSERT ON notification_retries
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_retry_evidence();

-- Existing read-only operators do not silently acquire write authority.
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'notification_retry_operator','通知重试经办',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,a FROM roles CROSS JOIN unnest(ARRAY['notification.delivery.read','notification.delivery.retry']) a
 WHERE key='notification_retry_operator';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON notification_retries TO copypaste_app;
 END IF;
END; $$;
