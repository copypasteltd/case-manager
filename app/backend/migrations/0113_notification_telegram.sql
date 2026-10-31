-- Optional personal Telegram delivery supplements, and never replaces, in-app notices.
-- No credentials, business titles, message bodies or group destinations are stored here.
CREATE TABLE notification_telegram_policies (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL, bot_id text NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 enabled boolean NOT NULL, categories text[] NOT NULL,
 binding_id uuid, binding_version bigint,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,user_id,bot_id,version),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,binding_id) REFERENCES telegram_bindings(org_id,id),
 CHECK(bot_id ~ '^[1-9][0-9]{0,15}$'),
 CHECK((binding_id IS NULL)=(binding_version IS NULL)),
 CHECK(NOT enabled OR (binding_id IS NOT NULL AND cardinality(categories)>0)),
 CHECK(cardinality(categories)<=10 AND array_position(categories,NULL) IS NULL),
 CHECK(categories <@ ARRAY['mention','task','approval','request_result','contract_due','subscription_due','replica_failure','incident','secretary','service_resource']::text[])
);
CREATE FUNCTION guard_notification_telegram_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE previous_version bigint;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'Telegram consent history is immutable' USING ERRCODE='23514'; END IF;
 PERFORM 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.user_id FOR UPDATE;
 SELECT COALESCE(max(version),0) INTO previous_version FROM notification_telegram_policies WHERE org_id=NEW.org_id AND user_id=NEW.user_id AND bot_id=NEW.bot_id;
 IF NEW.version<>previous_version+1 OR cardinality(NEW.categories)<>(SELECT count(DISTINCT x) FROM unnest(NEW.categories) x)
 OR (NEW.enabled AND NOT EXISTS(SELECT 1 FROM telegram_bindings b JOIN users u ON u.org_id=b.org_id AND u.id=b.user_id
  WHERE b.org_id=NEW.org_id AND b.id=NEW.binding_id AND b.version=NEW.binding_version AND b.user_id=NEW.user_id
  AND b.bot_id=NEW.bot_id AND b.state='active' AND u.status='active')) THEN
  RAISE EXCEPTION 'invalid Telegram consent revision' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_telegram_policy_guard BEFORE INSERT OR UPDATE OR DELETE ON notification_telegram_policies
 FOR EACH ROW EXECUTE FUNCTION guard_notification_telegram_policy();

CREATE TABLE notification_telegram_deliveries (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL, bot_id text NOT NULL,
 source_delivery_id uuid NOT NULL, thread_id uuid NOT NULL, policy_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','sending','retry_wait','sent','delivery_unknown','confirmed_sent','failed','suppressed','abandoned')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 reason_code text NOT NULL CHECK(length(reason_code) BETWEEN 1 AND 80),
 next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(), lease_until timestamptz,
 payload_sha256 text CHECK(payload_sha256 ~ '^[a-f0-9]{64}$'),
 message_id bigint CHECK(message_id BETWEEN 1 AND 9007199254740991),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,source_delivery_id,bot_id),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,thread_id,source_delivery_id) REFERENCES notification_deliveries(org_id,thread_id,id),
 FOREIGN KEY(org_id,policy_id) REFERENCES notification_telegram_policies(org_id,id),
 CHECK(state<>'sending' OR (lease_until IS NOT NULL AND payload_sha256 IS NOT NULL AND attempts>0)),
 CHECK(state IN ('sending') OR lease_until IS NULL),
 CHECK(state NOT IN ('sent','confirmed_sent') OR message_id IS NOT NULL)
);
CREATE INDEX notification_telegram_due ON notification_telegram_deliveries(org_id,bot_id,next_attempt_at) WHERE state IN ('pending','retry_wait','sending');
CREATE INDEX notification_telegram_owner ON notification_telegram_deliveries(org_id,user_id,created_at DESC,id DESC);
CREATE TABLE notification_telegram_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, delivery_id uuid NOT NULL, version bigint NOT NULL,
 state text NOT NULL, attempts integer NOT NULL, reason_code text NOT NULL, payload_sha256 text, message_id bigint,
 actor_id uuid, reason text CHECK(reason IS NULL OR length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,delivery_id,version),
 FOREIGN KEY(org_id,delivery_id) REFERENCES notification_telegram_deliveries(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TRIGGER notification_telegram_events_immutable BEFORE UPDATE OR DELETE ON notification_telegram_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

-- This is the structural half of admission. The worker additionally calls the
-- same current domain/recipient authorization used by the in-app delivery.
CREATE FUNCTION notification_telegram_authorized(w_org uuid,w_id uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM notification_telegram_deliveries d
 JOIN notification_telegram_policies p ON p.org_id=d.org_id AND p.id=d.policy_id AND p.user_id=d.user_id AND p.bot_id=d.bot_id
 JOIN telegram_bindings b ON b.org_id=p.org_id AND b.id=p.binding_id AND b.version=p.binding_version AND b.user_id=p.user_id AND b.bot_id=p.bot_id
 JOIN users u ON u.org_id=d.org_id AND u.id=d.user_id
 JOIN notification_threads n ON n.org_id=d.org_id AND n.id=d.thread_id AND n.recipient_id=d.user_id AND n.latest_delivery_id=d.source_delivery_id
 JOIN notification_deliveries source ON source.org_id=d.org_id AND source.id=d.source_delivery_id AND source.recipient_id=d.user_id
 WHERE d.org_id=w_org AND d.id=w_id AND p.enabled AND b.state='active' AND u.status='active'
 AND n.category=ANY(p.categories) AND n.read_seq<n.delivered_seq
 AND source.created_at>=p.created_at AND source.created_at>clock_timestamp()-interval '24 hours'
 AND NOT EXISTS(SELECT 1 FROM notification_telegram_policies newer WHERE newer.org_id=p.org_id AND newer.user_id=p.user_id AND newer.bot_id=p.bot_id AND newer.version>p.version))
$$;
CREATE FUNCTION guard_notification_telegram_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram receipts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempts<>0 OR NEW.payload_sha256 IS NOT NULL OR NEW.message_id IS NOT NULL THEN
   RAISE EXCEPTION 'invalid initial Telegram receipt' USING ERRCODE='23514';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM notification_telegram_policies p JOIN notification_deliveries s ON s.org_id=p.org_id
    WHERE p.org_id=NEW.org_id AND p.id=NEW.policy_id AND p.user_id=NEW.user_id AND p.bot_id=NEW.bot_id
    AND s.id=NEW.source_delivery_id AND s.thread_id=NEW.thread_id AND s.recipient_id=NEW.user_id) THEN
   RAISE EXCEPTION 'Telegram receipt owner mismatch' USING ERRCODE='23514'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['state','version','attempts','reason_code','next_attempt_at','lease_until','payload_sha256','message_id','updated_at'])
   IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','attempts','reason_code','next_attempt_at','lease_until','payload_sha256','message_id','updated_at'])
   OR NEW.version<>OLD.version+1 OR NOT (
    (OLD.state IN ('pending','retry_wait') AND NEW.state IN ('sending','suppressed','abandoned')) OR
    (OLD.state='sending' AND NEW.state IN ('sent','retry_wait','failed','delivery_unknown')) OR
    (OLD.state IN ('delivery_unknown','failed') AND NEW.state IN ('confirmed_sent','retry_wait','abandoned','suppressed'))) THEN
   RAISE EXCEPTION 'invalid Telegram receipt transition' USING ERRCODE='23514'; END IF;
  IF NEW.attempts<>OLD.attempts+(CASE WHEN NEW.state='sending' THEN 1 ELSE 0 END)
   OR (NEW.state='sending' AND NOT notification_telegram_authorized(NEW.org_id,NEW.id)) THEN
   RAISE EXCEPTION 'Telegram attempt is no longer authorized' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_telegram_delivery_guard BEFORE INSERT OR UPDATE OR DELETE ON notification_telegram_deliveries FOR EACH ROW EXECUTE FUNCTION guard_notification_telegram_delivery();
CREATE FUNCTION validate_notification_telegram_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM notification_telegram_events e WHERE e.org_id=NEW.org_id AND e.delivery_id=NEW.id AND e.version=NEW.version
  AND e.state=NEW.state AND e.attempts=NEW.attempts AND e.reason_code=NEW.reason_code AND e.payload_sha256 IS NOT DISTINCT FROM NEW.payload_sha256 AND e.message_id IS NOT DISTINCT FROM NEW.message_id) THEN
  RAISE EXCEPTION 'Telegram receipt journal is missing' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER notification_telegram_delivery_evidence AFTER INSERT OR UPDATE ON notification_telegram_deliveries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_telegram_evidence();
CREATE FUNCTION guard_notification_telegram_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM notification_telegram_deliveries d WHERE d.org_id=NEW.org_id AND d.id=NEW.delivery_id AND d.version=NEW.version
  AND d.state=NEW.state AND d.attempts=NEW.attempts AND d.reason_code=NEW.reason_code AND d.payload_sha256 IS NOT DISTINCT FROM NEW.payload_sha256
  AND d.message_id IS NOT DISTINCT FROM NEW.message_id AND (NEW.actor_id IS NULL OR NEW.actor_id=d.user_id)) THEN
  RAISE EXCEPTION 'Telegram event does not match receipt' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_telegram_event_guard BEFORE INSERT ON notification_telegram_events FOR EACH ROW EXECUTE FUNCTION guard_notification_telegram_event();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON notification_telegram_policies,notification_telegram_events TO copypaste_app;
 GRANT SELECT,INSERT,UPDATE ON notification_telegram_deliveries TO copypaste_app;
 REVOKE UPDATE,DELETE ON notification_telegram_policies,notification_telegram_events FROM copypaste_app;
 REVOKE DELETE ON notification_telegram_deliveries FROM copypaste_app;
END IF; END $$;
