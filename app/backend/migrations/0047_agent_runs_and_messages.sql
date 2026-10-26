-- Profiles are installed by the controlled deployment identity. An application
-- user cannot turn an unpublished skill or an unconfined runtime into a worker.
CREATE TABLE agent_runtime_profiles (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 skill_id text NOT NULL CHECK(skill_id IN ('project-create','project-update','file-update','information-organize','oa-assist','subscription-review','project-briefing','incident-summary')),
 skill_version text NOT NULL CHECK(length(skill_version) BETWEEN 1 AND 100),
 skill_sha256 text NOT NULL CHECK(skill_sha256 ~ '^[a-f0-9]{64}$'),
 runtime_version text NOT NULL CHECK(length(runtime_version) BETWEEN 1 AND 100),
 configuration_sha256 text NOT NULL CHECK(configuration_sha256 ~ '^[a-f0-9]{64}$'),
 state text NOT NULL CHECK(state IN ('active','retired')),
 max_input_tokens integer NOT NULL CHECK(max_input_tokens BETWEEN 1 AND 200000),
 max_output_tokens integer NOT NULL CHECK(max_output_tokens BETWEEN 1 AND 32000),
 max_tool_calls integer NOT NULL CHECK(max_tool_calls BETWEEN 1 AND 100),
 timeout_seconds integer NOT NULL CHECK(timeout_seconds BETWEEN 30 AND 1800),
 installed_by uuid NOT NULL, installation_reason text NOT NULL CHECK(length(btrim(installation_reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), FOREIGN KEY(org_id,installed_by) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX agent_profile_active ON agent_runtime_profiles(org_id,skill_id) WHERE state='active';
CREATE FUNCTION preserve_agent_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM organizations WHERE id=OLD.org_id FOR UPDATE;
 IF NEW.id<>OLD.id OR NEW.org_id<>OLD.org_id OR NEW.skill_id<>OLD.skill_id OR NEW.skill_version<>OLD.skill_version
 OR NEW.skill_sha256<>OLD.skill_sha256 OR NEW.runtime_version<>OLD.runtime_version OR NEW.configuration_sha256<>OLD.configuration_sha256
 OR NEW.max_input_tokens<>OLD.max_input_tokens OR NEW.max_output_tokens<>OLD.max_output_tokens OR NEW.max_tool_calls<>OLD.max_tool_calls
 OR NEW.timeout_seconds<>OLD.timeout_seconds OR NEW.installed_by<>OLD.installed_by OR NEW.installation_reason<>OLD.installation_reason
 OR NEW.created_at<>OLD.created_at OR (OLD.state='retired' AND NEW.state<>'retired') THEN
  RAISE EXCEPTION 'runtime profiles are immutable and retired profiles cannot be reactivated' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_profile_identity BEFORE UPDATE ON agent_runtime_profiles FOR EACH ROW EXECUTE FUNCTION preserve_agent_profile();
CREATE TRIGGER agent_profile_no_delete BEFORE DELETE ON agent_runtime_profiles FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TABLE agent_runs (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), conversation_id uuid NOT NULL,
 conversation_revision_id uuid NOT NULL, context_security_version bigint NOT NULL CHECK(context_security_version>0),
 actor_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL,
 channel text NOT NULL CHECK(channel IN ('web','wechat','douyin','android')),
 profile_id uuid NOT NULL, client_message_id uuid NOT NULL, input_sha256 text NOT NULL CHECK(input_sha256 ~ '^[a-f0-9]{64}$'),
 audience_ids uuid[] NOT NULL CHECK(cardinality(audience_ids) BETWEEN 1 AND 100),
 state text NOT NULL CHECK(state IN ('queued','running','awaiting_approval','committing','succeeded','failed','cancelled','interrupted')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), last_event_seq bigint NOT NULL DEFAULT 1 CHECK(last_event_seq>0),
 cancel_requested_at timestamptz, safe_error_code text,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(org_id,conversation_id,actor_id,client_message_id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,profile_id) REFERENCES agent_runtime_profiles(org_id,id),
 FOREIGN KEY(org_id,conversation_id,conversation_revision_id) REFERENCES conversation_revisions(org_id,conversation_id,id),
 CHECK(actor_id=ANY(audience_ids)), CHECK(expires_at>created_at), CHECK(version=last_event_seq AND version<=9007199254740991),
 CHECK(safe_error_code IS NULL OR safe_error_code ~ '^[A-Z][A-Z0-9_]{0,99}$')
);
CREATE UNIQUE INDEX agent_run_one_active_conversation ON agent_runs(org_id,conversation_id) WHERE state IN ('queued','running','awaiting_approval','committing');
CREATE INDEX agent_run_queue ON agent_runs(state,created_at,id);
CREATE INDEX agent_run_actor ON agent_runs(org_id,actor_id,state);

CREATE TABLE conversation_messages (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, conversation_id uuid NOT NULL, run_id uuid NOT NULL,
 seq bigint NOT NULL CHECK(seq>0), role text NOT NULL CHECK(role IN ('user','assistant','system')),
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 20000), author_id uuid NOT NULL,
 audience_ids uuid[] NOT NULL CHECK(cardinality(audience_ids) BETWEEN 1 AND 100),
 source_snapshots jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(source_snapshots)='array' AND jsonb_array_length(source_snapshots)<=20),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(org_id,conversation_id,seq),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id), FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX agent_run_user_message ON conversation_messages(run_id) WHERE role='user';
CREATE TABLE agent_run_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0),
 type text NOT NULL CHECK(type IN ('run.queued','run.started','run.cancel_requested','run.cancelled','run.interrupted','run.failed','run.completed','run.awaiting_approval','run.committing')),
 state text NOT NULL CHECK(state IN ('queued','running','awaiting_approval','committing','succeeded','failed','cancelled','interrupted')),
 safe_code text CHECK(safe_code IS NULL OR safe_code ~ '^[A-Z][A-Z0-9_]{0,99}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,run_id,seq), FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 CHECK(state=CASE type WHEN 'run.queued' THEN 'queued' WHEN 'run.started' THEN 'running' WHEN 'run.cancel_requested' THEN 'running'
  WHEN 'run.cancelled' THEN 'cancelled' WHEN 'run.interrupted' THEN 'interrupted' WHEN 'run.failed' THEN 'failed'
  WHEN 'run.completed' THEN 'succeeded' WHEN 'run.awaiting_approval' THEN 'awaiting_approval' WHEN 'run.committing' THEN 'committing' END)
);
CREATE TRIGGER conversation_message_immutable BEFORE UPDATE OR DELETE ON conversation_messages FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_run_event_immutable BEFORE UPDATE OR DELETE ON agent_run_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_agent_run_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id<>OLD.id OR NEW.org_id<>OLD.org_id OR NEW.conversation_id<>OLD.conversation_id OR NEW.conversation_revision_id<>OLD.conversation_revision_id
 OR NEW.context_security_version<>OLD.context_security_version OR NEW.actor_id<>OLD.actor_id OR NEW.actor_auth_version<>OLD.actor_auth_version
 OR NEW.organization_auth_version<>OLD.organization_auth_version OR NEW.channel<>OLD.channel OR NEW.profile_id<>OLD.profile_id
 OR NEW.client_message_id<>OLD.client_message_id OR NEW.input_sha256<>OLD.input_sha256 OR NEW.audience_ids<>OLD.audience_ids
 OR NEW.expires_at<>OLD.expires_at OR NEW.created_at<>OLD.created_at OR NEW.version<>OLD.version+1 OR NEW.last_event_seq<>OLD.last_event_seq+1
 OR (OLD.cancel_requested_at IS NOT NULL AND NEW.cancel_requested_at IS DISTINCT FROM OLD.cancel_requested_at)
 OR OLD.state IN ('succeeded','failed','cancelled','interrupted') THEN
  RAISE EXCEPTION 'run binding, terminal facts and event sequence are immutable' USING ERRCODE='23514';
 END IF;
 IF NOT ((OLD.state='queued' AND NEW.state IN ('running','cancelled','failed','interrupted'))
 OR (OLD.state='running' AND NEW.state IN ('running','awaiting_approval','committing','succeeded','failed','cancelled','interrupted'))
 OR (OLD.state='awaiting_approval' AND NEW.state IN ('committing','running','cancelled','failed','interrupted'))
 OR (OLD.state='committing' AND NEW.state IN ('succeeded','failed','interrupted'))) THEN
  RAISE EXCEPTION 'invalid run transition' USING ERRCODE='23514';
 END IF;
 IF NEW.state=OLD.state AND NOT (OLD.state='running' AND OLD.cancel_requested_at IS NULL AND NEW.cancel_requested_at IS NOT NULL) THEN
  RAISE EXCEPTION 'a same-state update requires an actual cancellation request' USING ERRCODE='23514';
 END IF;
 IF OLD.cancel_requested_at IS NOT NULL AND NEW.state NOT IN ('cancelled','failed','interrupted') THEN
  RAISE EXCEPTION 'a cancelled intent cannot start a new commit' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_run_binding BEFORE UPDATE ON agent_runs FOR EACH ROW EXECUTE FUNCTION preserve_agent_run_binding();
CREATE TRIGGER agent_run_no_delete BEFORE DELETE ON agent_runs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_agent_run_ledger() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; e agent_run_events; c conversation_revisions;
BEGIN
 IF TG_TABLE_NAME='agent_runs' THEN SELECT * INTO r FROM agent_runs WHERE id=NEW.id;
 ELSE SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id; END IF;
 SELECT * INTO e FROM agent_run_events WHERE org_id=r.org_id AND run_id=r.id ORDER BY seq DESC LIMIT 1;
 SELECT * INTO c FROM conversation_revisions WHERE org_id=r.org_id AND id=r.conversation_revision_id;
 IF TG_TABLE_NAME='conversation_messages' THEN
  IF NEW.org_id<>r.org_id OR NEW.conversation_id<>r.conversation_id OR NEW.audience_ids<>r.audience_ids OR NEW.author_id<>r.actor_id
  OR NEW.seq<>(SELECT count(*) FROM conversation_messages WHERE org_id=NEW.org_id AND conversation_id=NEW.conversation_id AND seq<=NEW.seq) THEN
   RAISE EXCEPTION 'message scope and audience must match its run' USING ERRCODE='23514';
  END IF;
 END IF;
 IF e.id IS NULL OR e.seq<>r.last_event_seq OR e.state<>r.state
 OR (SELECT count(*) FROM agent_run_events WHERE run_id=r.id)<>r.last_event_seq
 OR NOT EXISTS(SELECT 1 FROM conversation_messages m WHERE m.run_id=r.id AND m.role='user' AND m.org_id=r.org_id
  AND m.conversation_id=r.conversation_id AND m.author_id=r.actor_id AND m.audience_ids=r.audience_ids)
 OR c.conversation_id<>r.conversation_id OR c.context_security_version<>r.context_security_version
 OR r.audience_ids<>ARRAY(SELECT user_id FROM conversation_members WHERE revision_id=r.conversation_revision_id ORDER BY user_id) THEN
  RAISE EXCEPTION 'run head requires exact immutable message, audience and contiguous events' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_run_ledger AFTER INSERT OR UPDATE ON agent_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_run_ledger();
CREATE CONSTRAINT TRIGGER agent_event_ledger AFTER INSERT ON agent_run_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_run_ledger();
CREATE CONSTRAINT TRIGGER agent_message_ledger AFTER INSERT ON conversation_messages DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_run_ledger();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT ON agent_runtime_profiles TO copypaste_app;
 GRANT SELECT,INSERT,UPDATE ON agent_runs TO copypaste_app;
 GRANT SELECT,INSERT ON conversation_messages,agent_run_events TO copypaste_app;
 REVOKE INSERT,UPDATE,DELETE ON agent_runtime_profiles FROM copypaste_app;
 REVOKE DELETE ON agent_runs FROM copypaste_app;
 REVOKE UPDATE,DELETE ON conversation_messages,agent_run_events FROM copypaste_app;
END IF; END; $$;
