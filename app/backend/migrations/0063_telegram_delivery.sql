-- Outbound tokens never enter the database, ingress or model process.
ALTER TABLE telegram_channels ADD COLUMN delivery_enabled boolean NOT NULL DEFAULT false;

CREATE FUNCTION telegram_delivery_authorized(w_org uuid,w_inbox uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM telegram_inbox i
 JOIN telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
 JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
 JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
 LEFT JOIN telegram_conversation_scopes s ON s.org_id=l.org_id AND s.conversation_id=l.conversation_id
 JOIN organizations o ON o.id=i.org_id
 WHERE i.org_id=w_org AND i.id=w_inbox AND i.kind='request' AND i.state IN ('awaiting_executor','dispatched')
 AND c.state='active' AND c.delivery_enabled AND c.disclosure_mode='portal_only' AND c.version=i.channel_version
 AND c.bot_id=i.bot_id AND b.state='active' AND b.user_id=i.requester_id AND b.bot_id=i.bot_id
 AND b.telegram_user_id=i.telegram_user_id AND u.status='active' AND u.auth_version=i.actor_auth_version
 AND (l.run_id IS NULL OR (s.actor_id=u.id AND s.actor_auth_version=u.auth_version
  AND s.organization_auth_version=o.authorization_version AND s.channel_version=c.version)))
$$;

CREATE TABLE telegram_delivery_budgets (
 bot_id text PRIMARY KEY CHECK(bot_id ~ '^[1-9][0-9]{0,15}$'), org_id uuid NOT NULL REFERENCES organizations(id),
 next_allowed_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE telegram_deliveries (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, inbox_id uuid NOT NULL, channel_id uuid NOT NULL,
 bot_id text NOT NULL, run_id uuid, phase text NOT NULL CHECK(phase IN ('accepted','progress','result')),
 state text NOT NULL CHECK(state IN ('pending','sending','retry_wait','sent','delivery_unknown','confirmed_sent','failed','superseded','suppressed','abandoned')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 reason_code text NOT NULL CHECK(length(reason_code) BETWEEN 1 AND 80),
 next_attempt_at timestamptz NOT NULL DEFAULT now(), lease_until timestamptz, payload_sha256 text,
 message_id bigint CHECK(message_id BETWEEN 1 AND 9007199254740991), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(inbox_id,phase),
 FOREIGN KEY(org_id,inbox_id) REFERENCES telegram_inbox(org_id,id),
 FOREIGN KEY(org_id,channel_id) REFERENCES telegram_channels(org_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES telegram_run_links(org_id,run_id),
 CHECK(payload_sha256 IS NULL OR payload_sha256 ~ '^[a-f0-9]{64}$'),
 CHECK(state<>'sending' OR (lease_until IS NOT NULL AND payload_sha256 IS NOT NULL AND attempts>0)),
 CHECK(state NOT IN ('sent','confirmed_sent') OR message_id IS NOT NULL),
 CHECK(phase='accepted' OR run_id IS NOT NULL)
);
CREATE UNIQUE INDEX telegram_delivery_run_phase ON telegram_deliveries(run_id,channel_id,phase) WHERE run_id IS NOT NULL;
CREATE INDEX telegram_delivery_due ON telegram_deliveries(org_id,bot_id,next_attempt_at) WHERE state IN ('pending','retry_wait','sending');

-- Each state revision has an immutable journal row, including prepared sends.
CREATE TABLE telegram_delivery_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, delivery_id uuid NOT NULL, version bigint NOT NULL,
 state text NOT NULL, attempts integer NOT NULL, reason_code text NOT NULL,
 payload_sha256 text, message_id bigint, actor_id uuid, reason text,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,delivery_id,version),
 FOREIGN KEY(org_id,delivery_id) REFERENCES telegram_deliveries(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 CHECK((actor_id IS NULL)=(reason IS NULL)), CHECK(reason IS NULL OR length(btrim(reason)) BETWEEN 1 AND 200)
);
ALTER TABLE telegram_deliveries ADD CONSTRAINT telegram_delivery_current_event
 FOREIGN KEY(org_id,id,version) REFERENCES telegram_delivery_events(org_id,delivery_id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER telegram_delivery_event_immutable BEFORE UPDATE OR DELETE ON telegram_delivery_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_telegram_delivery_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_deliveries d WHERE d.org_id=NEW.org_id AND d.id=NEW.delivery_id
 AND (d.version,d.state,d.attempts,d.reason_code,d.payload_sha256,d.message_id)
 IS NOT DISTINCT FROM (NEW.version,NEW.state,NEW.attempts,NEW.reason_code,NEW.payload_sha256,NEW.message_id)) THEN
 RAISE EXCEPTION 'delivery event must describe its exact revision' USING ERRCODE='23514'; END IF;
 IF NEW.reason_code IN ('manual_confirmed_sent','manual_retry','manual_abandoned') AND NEW.actor_id IS NULL THEN
 RAISE EXCEPTION 'manual reconciliation requires an actor and reason' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_delivery_event_binding BEFORE INSERT ON telegram_delivery_events
 FOR EACH ROW EXECUTE FUNCTION guard_telegram_delivery_event();
CREATE FUNCTION guard_telegram_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'delivery history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempts<>0 OR NOT EXISTS(
   SELECT 1 FROM telegram_inbox i LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
   LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
   WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.channel_id=NEW.channel_id AND i.bot_id=NEW.bot_id
    AND NEW.run_id IS NOT DISTINCT FROM l.run_id AND telegram_delivery_authorized(i.org_id,i.id)
    AND NEW.phase=CASE WHEN r.state IS NULL OR r.state='queued' THEN 'accepted' WHEN r.state IN ('running','awaiting_approval','committing') THEN 'progress' ELSE 'result' END) THEN
   RAISE EXCEPTION 'delivery must originate in its current authorized request' USING ERRCODE='23514'; END IF;
 ELSE
  IF (NEW.id,NEW.org_id,NEW.inbox_id,NEW.channel_id,NEW.bot_id,NEW.run_id,NEW.phase,NEW.created_at)
    IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.inbox_id,OLD.channel_id,OLD.bot_id,OLD.run_id,OLD.phase,OLD.created_at)
    OR NEW.version<>OLD.version+1 THEN
   RAISE EXCEPTION 'delivery identity is immutable and revisions are sequential' USING ERRCODE='23514'; END IF;
  IF NOT ((OLD.state IN ('pending','retry_wait') AND NEW.state IN ('sending','superseded','suppressed'))
    OR (OLD.state='sending' AND NEW.state IN ('sent','delivery_unknown','retry_wait','failed','suppressed'))
    OR (OLD.state='delivery_unknown' AND NEW.state IN ('confirmed_sent','retry_wait','abandoned','suppressed'))) THEN
   RAISE EXCEPTION 'invalid delivery transition' USING ERRCODE='23514'; END IF;
  IF NEW.attempts<>OLD.attempts+(CASE WHEN NEW.state='sending' THEN 1 ELSE 0 END) THEN
   RAISE EXCEPTION 'every outbound attempt must be prepared exactly once' USING ERRCODE='23514'; END IF;
  IF NEW.state='sending' AND NOT telegram_delivery_authorized(NEW.org_id,NEW.inbox_id) THEN
   RAISE EXCEPTION 'outbound request authorization changed' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_delivery_guard BEFORE INSERT OR UPDATE OR DELETE ON telegram_deliveries
 FOR EACH ROW EXECUTE FUNCTION guard_telegram_delivery();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT,UPDATE ON telegram_deliveries,telegram_delivery_budgets TO copypaste_app;
 GRANT SELECT,INSERT ON telegram_delivery_events TO copypaste_app;
 REVOKE DELETE ON telegram_deliveries,telegram_delivery_budgets FROM copypaste_app;
 REVOKE UPDATE,DELETE ON telegram_delivery_events FROM copypaste_app;
END IF; END $$;
