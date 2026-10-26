-- Telegram credentials are supplied to the ingress process, never stored here.
CREATE TABLE telegram_binding_challenges (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL,
 bot_id text NOT NULL CHECK(bot_id ~ '^[1-9][0-9]{0,15}$'), session_id uuid NOT NULL,
 actor_auth_version bigint NOT NULL CHECK(actor_auth_version>0), version bigint NOT NULL DEFAULT 1,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 telegram_user_id text CHECK(telegram_user_id ~ '^[1-9][0-9]{0,15}$'),
 claimed_at timestamptz, confirmed_at timestamptz, cancelled_at timestamptz,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
 CHECK((telegram_user_id IS NULL)=(claimed_at IS NULL)),
 CHECK(confirmed_at IS NULL OR (claimed_at IS NOT NULL AND cancelled_at IS NULL)),
 CHECK(expires_at>created_at)
);
CREATE INDEX telegram_challenge_owner ON telegram_binding_challenges(org_id,user_id,bot_id,created_at DESC);
-- A numeric identity cannot be silently reassigned to another employee/org,
-- even after an old binding was revoked. Such a transfer needs a separate policy.
CREATE TABLE telegram_identity_owners (
 bot_id text NOT NULL, telegram_user_id text NOT NULL, org_id uuid NOT NULL REFERENCES organizations(id),
 user_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(bot_id,telegram_user_id), UNIQUE(org_id,bot_id,telegram_user_id,user_id),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE TABLE telegram_bindings (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL, bot_id text NOT NULL,
 telegram_user_id text NOT NULL, challenge_id uuid NOT NULL UNIQUE,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), state text NOT NULL CHECK(state IN ('active','revoked')),
 created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,challenge_id) REFERENCES telegram_binding_challenges(org_id,id),
 FOREIGN KEY(org_id,bot_id,telegram_user_id,user_id) REFERENCES telegram_identity_owners(org_id,bot_id,telegram_user_id,user_id),
 CHECK((state='revoked')=(revoked_at IS NOT NULL))
);
CREATE UNIQUE INDEX telegram_active_user ON telegram_bindings(org_id,user_id,bot_id) WHERE state='active';
CREATE UNIQUE INDEX telegram_active_identity ON telegram_bindings(bot_id,telegram_user_id) WHERE state='active';
CREATE TABLE telegram_channels (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), bot_id text NOT NULL,
 chat_id text NOT NULL CHECK(chat_id ~ '^-[1-9][0-9]{0,15}$'), topic_id bigint NOT NULL CHECK(topic_id BETWEEN 0 AND 2147483647),
 label text NOT NULL CHECK(length(btrim(label)) BETWEEN 1 AND 200), project_id uuid,
 allowed_skill_ids text[] NOT NULL CHECK(cardinality(allowed_skill_ids) BETWEEN 1 AND 2 AND allowed_skill_ids <@ ARRAY['project-briefing','incident-summary']::text[]),
 disclosure_mode text NOT NULL DEFAULT 'portal_only' CHECK(disclosure_mode='portal_only'),
 state text NOT NULL CHECK(state IN ('active','paused')), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(bot_id,chat_id,topic_id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE TABLE telegram_channel_revisions (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, channel_id uuid NOT NULL, version bigint NOT NULL,
 actor_id uuid, source text NOT NULL CHECK(source IN ('portal','membership','migration')),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(channel_id,version),
 FOREIGN KEY(org_id,channel_id) REFERENCES telegram_channels(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TABLE telegram_inbox (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), bot_id text NOT NULL,
 update_id bigint NOT NULL CHECK(update_id BETWEEN 0 AND 9007199254740991), payload_sha256 text NOT NULL CHECK(payload_sha256 ~ '^[a-f0-9]{64}$'),
 kind text NOT NULL CHECK(kind IN ('request','binding_proof','membership','ignored')),
 state text NOT NULL CHECK(state IN ('awaiting_executor','withdrawn','binding_proved','paused','ignored')),
 reason_code text NOT NULL, requester_id uuid, binding_id uuid, actor_auth_version bigint,
 channel_id uuid, channel_version bigint, telegram_user_id text, chat_id text, topic_id bigint,
 message_id bigint, request_text text, received_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(bot_id,update_id), FOREIGN KEY(org_id,requester_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,binding_id) REFERENCES telegram_bindings(org_id,id),
 FOREIGN KEY(org_id,channel_id) REFERENCES telegram_channels(org_id,id),
 CHECK((kind='request')=(request_text IS NOT NULL)),
 CHECK(kind<>'request' OR (state IN ('awaiting_executor','withdrawn') AND length(request_text) BETWEEN 1 AND 4000
   AND requester_id IS NOT NULL AND binding_id IS NOT NULL AND actor_auth_version IS NOT NULL
   AND channel_id IS NOT NULL AND channel_version IS NOT NULL AND telegram_user_id IS NOT NULL
   AND chat_id IS NOT NULL AND topic_id IS NOT NULL AND message_id IS NOT NULL))
);
CREATE INDEX telegram_request_owner ON telegram_inbox(org_id,requester_id,received_at DESC,id DESC) WHERE kind='request';

CREATE FUNCTION guard_telegram_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c telegram_binding_challenges;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram identity history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME='telegram_binding_challenges' THEN
  IF (NEW.id,NEW.org_id,NEW.user_id,NEW.bot_id,NEW.session_id,NEW.actor_auth_version,NEW.expires_at,NEW.created_at)
    IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.user_id,OLD.bot_id,OLD.session_id,OLD.actor_auth_version,OLD.expires_at,OLD.created_at)
   OR NEW.version<>OLD.version+1 OR OLD.cancelled_at IS NOT NULL OR OLD.confirmed_at IS NOT NULL
   OR (OLD.claimed_at IS NOT NULL AND (NEW.claimed_at,NEW.telegram_user_id) IS DISTINCT FROM (OLD.claimed_at,OLD.telegram_user_id)) THEN
   RAISE EXCEPTION 'Telegram proof is single use and keeps its original owner' USING ERRCODE='23514';
  END IF;
 ELSIF TG_OP='UPDATE' THEN
  IF (NEW.id,NEW.org_id,NEW.user_id,NEW.bot_id,NEW.telegram_user_id,NEW.challenge_id,NEW.created_at)
   IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.user_id,OLD.bot_id,OLD.telegram_user_id,OLD.challenge_id,OLD.created_at)
   OR OLD.state<>'active' OR NEW.state<>'revoked' OR NEW.version<>OLD.version+1 THEN
   RAISE EXCEPTION 'Telegram binding may only be revoked' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO c FROM telegram_binding_challenges WHERE org_id=NEW.org_id AND id=NEW.challenge_id;
  IF c.user_id IS DISTINCT FROM NEW.user_id OR c.bot_id IS DISTINCT FROM NEW.bot_id OR c.telegram_user_id IS DISTINCT FROM NEW.telegram_user_id
   OR c.claimed_at IS NULL OR c.confirmed_at IS NOT NULL OR c.cancelled_at IS NOT NULL OR c.expires_at<=clock_timestamp()
   OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
    WHERE s.org_id=c.org_id AND s.id=c.session_id AND s.user_id=c.user_id AND s.revoked_at IS NULL
    AND s.expires_at>clock_timestamp() AND u.status='active' AND u.auth_version=c.actor_auth_version AND s.auth_version=u.auth_version) THEN
   RAISE EXCEPTION 'Telegram binding requires the actual proof and its live original login' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_challenge_guard BEFORE UPDATE OR DELETE ON telegram_binding_challenges FOR EACH ROW EXECUTE FUNCTION guard_telegram_binding();
CREATE TRIGGER telegram_binding_guard BEFORE INSERT OR UPDATE OR DELETE ON telegram_bindings FOR EACH ROW EXECUTE FUNCTION guard_telegram_binding();
CREATE FUNCTION validate_telegram_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_binding_challenges c WHERE c.org_id=NEW.org_id AND c.id=NEW.challenge_id
   AND c.user_id=NEW.user_id AND c.bot_id=NEW.bot_id AND c.telegram_user_id=NEW.telegram_user_id
   AND c.confirmed_at IS NOT NULL AND c.cancelled_at IS NULL) THEN
  RAISE EXCEPTION 'Telegram binding needs its actual portal confirmation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER telegram_binding_confirmation AFTER INSERT ON telegram_bindings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_confirmation();
CREATE TRIGGER telegram_identity_owner_immutable BEFORE UPDATE OR DELETE ON telegram_identity_owners FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER telegram_channel_history_immutable BEFORE UPDATE OR DELETE ON telegram_channel_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_telegram_channel_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_channels c WHERE c.org_id=NEW.org_id AND c.id=NEW.channel_id AND c.version=NEW.version AND to_jsonb(c)=NEW.snapshot) THEN
  RAISE EXCEPTION 'Telegram history must describe the actual channel revision' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_channel_history_binding BEFORE INSERT ON telegram_channel_revisions FOR EACH ROW EXECUTE FUNCTION guard_telegram_channel_history();
CREATE FUNCTION guard_telegram_channel() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram channel history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (NEW.id,NEW.org_id,NEW.bot_id,NEW.chat_id,NEW.topic_id,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.bot_id,OLD.chat_id,OLD.topic_id,OLD.created_at)
   OR NEW.version<>OLD.version+1 THEN
  RAISE EXCEPTION 'Telegram channel location is fixed and updates require a new version' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_channel_guard BEFORE UPDATE OR DELETE ON telegram_channels FOR EACH ROW EXECUTE FUNCTION guard_telegram_channel();
CREATE FUNCTION guard_telegram_inbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram inbox cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'state'-'reason_code') IS DISTINCT FROM (to_jsonb(OLD)-'state'-'reason_code') OR OLD.state<>'awaiting_executor' OR NEW.state<>'withdrawn' THEN
   RAISE EXCEPTION 'Telegram receipt content and identity are immutable' USING ERRCODE='23514';
  END IF;
 ELSIF NEW.kind='request' THEN
  IF NOT EXISTS(SELECT 1 FROM telegram_bindings b JOIN users u ON u.org_id=b.org_id AND u.id=b.user_id
    JOIN telegram_channels c ON c.org_id=b.org_id AND c.id=NEW.channel_id
    WHERE b.org_id=NEW.org_id AND b.id=NEW.binding_id AND b.bot_id=NEW.bot_id AND b.telegram_user_id=NEW.telegram_user_id
    AND b.user_id=NEW.requester_id AND b.state='active' AND u.status='active' AND u.auth_version=NEW.actor_auth_version
    AND c.bot_id=NEW.bot_id AND c.chat_id=NEW.chat_id AND c.topic_id=NEW.topic_id AND c.state='active' AND c.version=NEW.channel_version) THEN
   RAISE EXCEPTION 'Telegram request requires its actual active binding and exact channel' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_inbox_guard BEFORE INSERT OR UPDATE OR DELETE ON telegram_inbox FOR EACH ROW EXECUTE FUNCTION guard_telegram_inbox();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT,UPDATE ON telegram_binding_challenges,telegram_bindings,telegram_channels,telegram_inbox TO copypaste_app;
  GRANT SELECT,INSERT ON telegram_identity_owners,telegram_channel_revisions TO copypaste_app;
 END IF;
END $$;
