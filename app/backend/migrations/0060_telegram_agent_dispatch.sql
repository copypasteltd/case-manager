-- TG uses a separately inspected read-only SDK configuration on the same immutable release.
ALTER TABLE agent_runtime_profiles ADD COLUMN telegram_configuration_sha256 text
 CHECK (telegram_configuration_sha256 IS NULL OR (skill_id='project-briefing' AND telegram_configuration_sha256 ~ '^[a-f0-9]{64}$'));
CREATE OR REPLACE FUNCTION preserve_agent_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM organizations WHERE id=OLD.org_id FOR UPDATE;
 IF NEW.id<>OLD.id OR NEW.org_id<>OLD.org_id OR NEW.skill_id<>OLD.skill_id OR NEW.skill_version<>OLD.skill_version
 OR NEW.skill_sha256<>OLD.skill_sha256 OR NEW.runtime_version<>OLD.runtime_version OR NEW.configuration_sha256<>OLD.configuration_sha256
 OR NEW.telegram_configuration_sha256 IS DISTINCT FROM OLD.telegram_configuration_sha256
 OR NEW.max_input_tokens<>OLD.max_input_tokens OR NEW.max_output_tokens<>OLD.max_output_tokens OR NEW.max_tool_calls<>OLD.max_tool_calls
 OR NEW.timeout_seconds<>OLD.timeout_seconds OR NEW.installed_by<>OLD.installed_by OR NEW.installation_reason<>OLD.installation_reason
 OR NEW.created_at<>OLD.created_at OR (OLD.state='retired' AND NEW.state<>'retired') THEN
  RAISE EXCEPTION 'runtime profiles are immutable and retired profiles cannot be reactivated' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;

-- Incoming requests acquire their own channel scope, never a portal session.
ALTER TABLE telegram_channels ADD COLUMN default_skill_id text
 CHECK(default_skill_id IS NULL OR default_skill_id=ANY(allowed_skill_ids));
ALTER TABLE telegram_inbox ADD COLUMN requested_skill_id text
 CHECK(requested_skill_id IN ('project-briefing','incident-summary'));
ALTER TABLE telegram_inbox ADD COLUMN dispatch_after timestamptz NOT NULL DEFAULT now();
ALTER TABLE telegram_inbox ADD CONSTRAINT telegram_inbox_org_identity UNIQUE(org_id,id);
-- Old receipts did not capture a task selection; do not invent that authority.
UPDATE telegram_inbox SET state='withdrawn',reason_code='legacy_dispatch_scope_missing'
 WHERE kind='request' AND state='awaiting_executor';
ALTER TABLE telegram_inbox DROP CONSTRAINT telegram_inbox_state_check;
ALTER TABLE telegram_inbox DROP CONSTRAINT telegram_inbox_check1;
ALTER TABLE telegram_inbox ADD CONSTRAINT telegram_inbox_state_check
 CHECK(state IN ('awaiting_executor','dispatched','withdrawn','binding_proved','paused','ignored'));
ALTER TABLE telegram_inbox ADD CONSTRAINT telegram_request_scope_check
 CHECK(kind<>'request' OR (state IN ('awaiting_executor','dispatched','withdrawn')
  AND length(request_text) BETWEEN 1 AND 4000 AND requester_id IS NOT NULL
  AND binding_id IS NOT NULL AND actor_auth_version IS NOT NULL
  AND channel_id IS NOT NULL AND channel_version IS NOT NULL
  AND telegram_user_id IS NOT NULL AND chat_id IS NOT NULL
  AND topic_id IS NOT NULL AND message_id IS NOT NULL));
CREATE INDEX telegram_dispatch_due ON telegram_inbox(bot_id,dispatch_after,received_at,id)
 WHERE kind='request' AND state='awaiting_executor';
ALTER TABLE conversations DROP CONSTRAINT conversations_type_check;
ALTER TABLE conversations ADD CONSTRAINT conversations_type_check CHECK(type IN ('private','project','telegram'));
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_channel_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_channel_check CHECK(channel IN ('web','wechat','douyin','android','telegram'));

CREATE TABLE telegram_conversation_scopes (
 org_id uuid NOT NULL, conversation_id uuid PRIMARY KEY, channel_id uuid NOT NULL,
 channel_version bigint NOT NULL CHECK(channel_version>0), binding_id uuid NOT NULL,
 actor_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,conversation_id),
 UNIQUE(channel_id,channel_version,binding_id,actor_auth_version,organization_auth_version),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,channel_id) REFERENCES telegram_channels(org_id,id),
 FOREIGN KEY(org_id,binding_id) REFERENCES telegram_bindings(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TABLE telegram_run_links (
 org_id uuid NOT NULL, inbox_id uuid PRIMARY KEY, run_id uuid NOT NULL UNIQUE,
 conversation_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,run_id), FOREIGN KEY(org_id,inbox_id) REFERENCES telegram_inbox(org_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES telegram_conversation_scopes(org_id,conversation_id)
);
CREATE TRIGGER telegram_scope_immutable BEFORE UPDATE OR DELETE ON telegram_conversation_scopes
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER telegram_run_link_immutable BEFORE UPDATE OR DELETE ON telegram_run_links
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION guard_telegram_conversation_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM conversations c
  JOIN telegram_channels ch ON ch.org_id=c.org_id AND ch.id=NEW.channel_id
  JOIN telegram_bindings b ON b.org_id=c.org_id AND b.id=NEW.binding_id
  JOIN users u ON u.org_id=c.org_id AND u.id=NEW.actor_id
  JOIN organizations o ON o.id=c.org_id
  WHERE c.org_id=NEW.org_id AND c.id=NEW.conversation_id AND c.type='telegram'
   AND c.creator_id=NEW.actor_id AND c.project_id IS NOT DISTINCT FROM ch.project_id
   AND ch.state='active' AND ch.version=NEW.channel_version
   AND b.bot_id=ch.bot_id AND b.user_id=NEW.actor_id AND b.state='active'
   AND u.status='active' AND u.auth_version=NEW.actor_auth_version
   AND o.authorization_version=NEW.organization_auth_version
   AND can_access(c.org_id,u.id,'conversation.create','conversation',c.id,c.project_id,c.creator_id,c.classification,'telegram',true)) THEN
  RAISE EXCEPTION 'Telegram conversation needs its actual current channel and actor' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_scope_binding BEFORE INSERT ON telegram_conversation_scopes
 FOR EACH ROW EXECUTE FUNCTION guard_telegram_conversation_scope();
CREATE FUNCTION validate_telegram_conversation_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.type='telegram' AND (NOT EXISTS(SELECT 1 FROM telegram_conversation_scopes s
    WHERE s.org_id=NEW.org_id AND s.conversation_id=NEW.id AND s.actor_id=NEW.creator_id)
  OR NOT EXISTS(SELECT 1 FROM conversation_revisions r WHERE r.id=NEW.current_revision_id
    AND r.member_ids=ARRAY[NEW.creator_id])) THEN
  RAISE EXCEPTION 'Telegram context is isolated to its proven requester' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER telegram_conversation_origin AFTER INSERT OR UPDATE ON conversations
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_conversation_origin();
CREATE FUNCTION guard_telegram_run_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_inbox i
  JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id
  JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
  JOIN telegram_conversation_scopes s ON s.org_id=i.org_id AND s.conversation_id=NEW.conversation_id
  JOIN agent_runs r ON r.org_id=i.org_id AND r.id=NEW.run_id
  JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
  JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
  JOIN organizations o ON o.id=i.org_id
  JOIN conversation_messages m ON m.org_id=r.org_id AND m.run_id=r.id AND m.role='user'
  WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.kind='request'
   AND i.state IN ('awaiting_executor','dispatched') AND i.requested_skill_id IS NOT NULL
   AND ch.state='active' AND ch.version=i.channel_version AND i.requested_skill_id=ANY(ch.allowed_skill_ids)
   AND b.state='active' AND b.bot_id=i.bot_id AND b.user_id=i.requester_id
   AND b.telegram_user_id=i.telegram_user_id AND ch.bot_id=i.bot_id
   AND ch.chat_id=i.chat_id AND ch.topic_id=i.topic_id
   AND u.status='active' AND u.auth_version=i.actor_auth_version
   AND s.channel_id=i.channel_id AND s.channel_version=i.channel_version AND s.binding_id=i.binding_id
   AND s.actor_id=i.requester_id AND s.actor_auth_version=i.actor_auth_version
   AND s.organization_auth_version=o.authorization_version
   AND r.conversation_id=NEW.conversation_id AND r.channel='telegram' AND r.state='queued'
   AND r.actor_id=i.requester_id AND r.actor_auth_version=i.actor_auth_version
   AND r.organization_auth_version=o.authorization_version AND r.audience_ids=ARRAY[i.requester_id]
   AND r.client_message_id=i.id AND p.skill_id=i.requested_skill_id AND p.state='active'
   AND p.skill_id='project-briefing' AND p.telegram_configuration_sha256 IS NOT NULL
   AND m.body=i.request_text AND m.author_id=i.requester_id AND m.audience_ids=ARRAY[i.requester_id]) THEN
  RAISE EXCEPTION 'Telegram run needs its exact durable inbound request and profile' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_run_link_binding BEFORE INSERT ON telegram_run_links
 FOR EACH ROW EXECUTE FUNCTION guard_telegram_run_link();
CREATE FUNCTION validate_telegram_run_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.channel='telegram' AND NOT EXISTS(SELECT 1 FROM telegram_run_links l
  JOIN telegram_inbox i ON i.org_id=l.org_id AND i.id=l.inbox_id
  JOIN telegram_conversation_scopes s ON s.org_id=l.org_id AND s.conversation_id=l.conversation_id
  JOIN agent_runtime_profiles p ON p.org_id=NEW.org_id AND p.id=NEW.profile_id
  WHERE l.org_id=NEW.org_id AND l.run_id=NEW.id AND l.conversation_id=NEW.conversation_id
   AND i.requester_id=NEW.actor_id AND i.actor_auth_version=NEW.actor_auth_version
   AND i.id=NEW.client_message_id AND p.skill_id=i.requested_skill_id
   AND s.actor_id=NEW.actor_id AND s.organization_auth_version=NEW.organization_auth_version) THEN
  RAISE EXCEPTION 'Telegram task origin cannot be invented or replaced' USING ERRCODE='23514';
 END IF;
 IF NEW.channel<>'telegram' AND EXISTS(SELECT 1 FROM conversations c
  WHERE c.org_id=NEW.org_id AND c.id=NEW.conversation_id AND c.type='telegram') THEN
  RAISE EXCEPTION 'Portal tasks cannot enter a Telegram context' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER telegram_run_origin AFTER INSERT OR UPDATE ON agent_runs
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_run_origin();

CREATE OR REPLACE FUNCTION guard_telegram_inbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram inbox cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'state'-'reason_code'-'dispatch_after') IS DISTINCT FROM (to_jsonb(OLD)-'state'-'reason_code'-'dispatch_after')
   OR NEW.dispatch_after<OLD.dispatch_after OR NOT (
    (OLD.state='awaiting_executor' AND NEW.state IN ('awaiting_executor','dispatched','withdrawn'))
    OR (OLD.state='dispatched' AND NEW.state='withdrawn' AND EXISTS(SELECT 1 FROM telegram_run_links l
      JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
      WHERE l.org_id=OLD.org_id AND l.inbox_id=OLD.id AND r.state IN ('queued','running','awaiting_approval','committing')))) THEN
   RAISE EXCEPTION 'Telegram receipt content and identity are immutable' USING ERRCODE='23514';
  END IF;
  IF NEW.state='dispatched' AND NOT EXISTS(SELECT 1 FROM telegram_run_links l WHERE l.org_id=NEW.org_id AND l.inbox_id=NEW.id) THEN
   RAISE EXCEPTION 'Telegram dispatch requires its actual run receipt' USING ERRCODE='23514';
  END IF;
 ELSIF NEW.kind='request' THEN
  IF NEW.requested_skill_id IS NULL OR NOT EXISTS(SELECT 1 FROM telegram_bindings b JOIN users u ON u.org_id=b.org_id AND u.id=b.user_id
    JOIN telegram_channels c ON c.org_id=b.org_id AND c.id=NEW.channel_id
    WHERE b.org_id=NEW.org_id AND b.id=NEW.binding_id AND b.bot_id=NEW.bot_id AND b.telegram_user_id=NEW.telegram_user_id
    AND b.user_id=NEW.requester_id AND b.state='active' AND u.status='active' AND u.auth_version=NEW.actor_auth_version
    AND c.bot_id=NEW.bot_id AND c.chat_id=NEW.chat_id AND c.topic_id=NEW.topic_id AND c.state='active'
    AND c.version=NEW.channel_version AND NEW.requested_skill_id=ANY(c.allowed_skill_ids)) THEN
   RAISE EXCEPTION 'Telegram request requires its actual active binding and exact channel' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON telegram_conversation_scopes,telegram_run_links TO copypaste_app;
 END IF;
END $$;
