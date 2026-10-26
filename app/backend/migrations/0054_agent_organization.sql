-- Private, exact and short-lived namespace plans; no object bytes or grants.
CREATE TABLE agent_organization_plans (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 conversation_id uuid NOT NULL, conversation_revision_id uuid NOT NULL,
 context_security_version bigint NOT NULL CHECK(context_security_version>0),
 actor_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL,
 origin_run_id uuid, project_id uuid,
 intent text NOT NULL CHECK(jsonb_typeof(intent::jsonb)='object' AND octet_length(intent)<=100000),
 plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'),
 preview jsonb NOT NULL CHECK(jsonb_typeof(preview)='array' AND jsonb_array_length(preview) BETWEEN 1 AND 10),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 3),
 state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','approved','revoked','committed')),
 operation_id uuid REFERENCES idempotency_records(id), result_data jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id,conversation_revision_id) REFERENCES conversation_revisions(org_id,conversation_id,id),
 FOREIGN KEY(org_id,origin_run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '30 minutes'),
 CHECK(encode(sha256(convert_to(intent,'UTF8')),'hex')=plan_hash),
 CHECK(((intent::jsonb)->'preview'=preview AND (intent::jsonb)->>'id'=id::text
  AND (intent::jsonb)->>'conversation_id'=conversation_id::text AND (intent::jsonb)->>'conversation_revision_id'=conversation_revision_id::text
  AND ((intent::jsonb)->>'context_security_version')::bigint=context_security_version
  AND (intent::jsonb)->>'actor_id'=actor_id::text AND ((intent::jsonb)->>'actor_auth_version')::bigint=actor_auth_version
  AND ((intent::jsonb)->>'organization_auth_version')::bigint=organization_auth_version
  AND ((intent::jsonb)->>'project_id')::uuid IS NOT DISTINCT FROM project_id
  AND ((intent::jsonb)->>'origin_run_id')::uuid IS NOT DISTINCT FROM origin_run_id
  AND ((intent::jsonb)->>'created_at')::timestamptz=created_at AND ((intent::jsonb)->>'expires_at')::timestamptz=expires_at
  AND (intent::jsonb)->>'reason'=reason) IS TRUE),
 CHECK(jsonb_typeof(result_data)='array'),
 CHECK((state='committed' AND operation_id IS NOT NULL AND jsonb_array_length(result_data)=jsonb_array_length(preview))
  OR (state<>'committed' AND operation_id IS NULL AND result_data='[]')),
 CHECK((state='proposed' AND version=1) OR (state='approved' AND version=2)
  OR (state='revoked' AND version IN (2,3)) OR (state='committed' AND version=3))
);
CREATE INDEX agent_organization_plans_conversation ON agent_organization_plans(org_id,conversation_id,actor_id,created_at DESC,id DESC);
CREATE TABLE agent_organization_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL, version bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN ('proposed','approved','revoked','committed')), plan_hash text NOT NULL,
 actor_id uuid NOT NULL, session_id uuid, mfa_verified_at timestamptz,
 operation_id uuid REFERENCES idempotency_records(id), reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(plan_id,version),
 FOREIGN KEY(org_id,plan_id) REFERENCES agent_organization_plans(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
 CHECK(kind<>'approved' OR (session_id IS NOT NULL AND mfa_verified_at IS NOT NULL)),
 CHECK((kind='committed')=(operation_id IS NOT NULL))
);
CREATE TRIGGER agent_organization_event_immutable BEFORE UPDATE OR DELETE ON agent_organization_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_organization_no_delete BEFORE DELETE ON agent_organization_plans FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION bind_agent_organization_plan() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'proposed' OR NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM conversations c
   JOIN conversation_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id
   WHERE c.org_id=NEW.org_id AND c.id=NEW.conversation_id AND c.type='private' AND c.creator_id=NEW.actor_id
   AND c.project_id IS NOT DISTINCT FROM NEW.project_id AND c.current_revision_id=NEW.conversation_revision_id
   AND r.state='active' AND r.context_security_version=NEW.context_security_version)
  OR (NEW.origin_run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
   WHERE r.org_id=NEW.org_id AND r.id=NEW.origin_run_id AND r.actor_id=NEW.actor_id AND r.conversation_id=NEW.conversation_id
   AND r.conversation_revision_id=NEW.conversation_revision_id AND r.state='running' AND r.cancel_requested_at IS NULL
   AND r.expires_at>clock_timestamp() AND p.skill_id='information-organize' AND p.state='active')) THEN
   RAISE EXCEPTION 'organization plan requires current private scope and origin' USING ERRCODE='23514';
  END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['version','state','operation_id','result_data']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','state','operation_id','result_data'])
   OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='proposed' AND NEW.state IN ('approved','revoked')) OR (OLD.state='approved' AND NEW.state IN ('committed','revoked'))) THEN
   RAISE EXCEPTION 'organization intent is immutable and transitions sequential' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_organization_binding BEFORE INSERT OR UPDATE ON agent_organization_plans FOR EACH ROW EXECUTE FUNCTION bind_agent_organization_plan();
CREATE FUNCTION validate_agent_organization_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p agent_organization_plans; previous text; verified timestamptz; item jsonb; act jsonb; n int:=0;
BEGIN
 SELECT * INTO p FROM agent_organization_plans WHERE org_id=NEW.org_id AND id=NEW.plan_id;
 IF p.id IS NULL OR NEW.plan_hash<>p.plan_hash OR NEW.actor_id<>p.actor_id OR NEW.version<>p.version OR NEW.kind<>p.state
  OR NEW.operation_id IS DISTINCT FROM p.operation_id THEN RAISE EXCEPTION 'organization event must match its head' USING ERRCODE='23514'; END IF;
 IF NEW.version>1 THEN
  SELECT kind INTO previous FROM agent_organization_events WHERE plan_id=p.id AND version=NEW.version-1;
  IF previous IS NULL OR NOT ((previous='proposed' AND NEW.kind IN ('approved','revoked')) OR (previous='approved' AND NEW.kind IN ('revoked','committed'))) THEN
   RAISE EXCEPTION 'organization event history must be contiguous' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.kind='approved' THEN
  SELECT mfa_verified_at INTO verified FROM sessions WHERE org_id=p.org_id AND id=NEW.session_id AND user_id=p.actor_id
   AND revoked_at IS NULL AND expires_at>clock_timestamp() AND auth_version=p.actor_auth_version;
  IF verified IS NULL OR verified IS DISTINCT FROM NEW.mfa_verified_at OR verified<clock_timestamp()-interval '10 minutes' OR verified>clock_timestamp()+interval '1 minute' THEN
   RAISE EXCEPTION 'organization approval requires recent interactive MFA' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.kind='committed' THEN
  IF p.operation_id::text IS DISTINCT FROM p.result_data->0->>'operation_id' THEN RAISE EXCEPTION 'organization batch requires its first receipt' USING ERRCODE='23514'; END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(p.result_data) LOOP
   act:=p.preview->n->'action';
   IF NOT EXISTS(SELECT 1 FROM idempotency_records i WHERE i.id=(item->>'operation_id')::uuid AND i.org_id=p.org_id AND i.actor_id=p.actor_id
    AND i.state='succeeded' AND i.route_key='/internal/agent-organization/'||(act->>'kind')
    AND i.idempotency_key='agent-organization:'||p.id::text||':'||n::text
    AND i.response_data=item->'data'
    AND (act->>'kind'='folder.create' OR (i.response_data->>'id'=act->>'id' AND (i.response_data->>'version')::bigint=(act->>'expected_version')::bigint+1))
    AND (act->>'kind'<>'folder.create' OR (i.response_data->>'parent_id'=act->>'id' AND i.response_data->>'name'=act->>'name'))
    AND (act->>'name' IS NULL OR i.response_data->>'name'=act->>'name')
    AND (act->>'target_folder_id' IS NULL OR coalesce(i.response_data->>'folder_id',i.response_data->>'parent_id')=act->>'target_folder_id')) THEN
    RAISE EXCEPTION 'organization commit requires every exact business receipt' USING ERRCODE='23514'; END IF;
   n:=n+1;
  END LOOP;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_organization_event_binding BEFORE INSERT ON agent_organization_events FOR EACH ROW EXECUTE FUNCTION validate_agent_organization_event();
CREATE FUNCTION validate_agent_organization_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p agent_organization_plans;
BEGIN
 SELECT * INTO p FROM agent_organization_plans WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM agent_organization_events e WHERE e.plan_id=p.id AND e.version=p.version AND e.kind=p.state AND e.plan_hash=p.plan_hash
  AND e.operation_id IS NOT DISTINCT FROM p.operation_id) OR (SELECT count(*) FROM agent_organization_events WHERE plan_id=p.id)<>p.version THEN
  RAISE EXCEPTION 'organization head requires durable matching history' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_organization_history AFTER INSERT OR UPDATE ON agent_organization_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_organization_head();

-- Immutable observations remain evidence of what was listed, not current
-- metadata or write authority. Every replay independently rechecks object ACLs.
CREATE TABLE agent_directory_snapshots (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL, folder_id uuid NOT NULL,
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=131072),
 sha256 text NOT NULL CHECK(sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id), FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id),
 CHECK(payload->'folder'->>'id'=folder_id::text)
);
CREATE TRIGGER agent_directory_immutable BEFORE UPDATE OR DELETE ON agent_directory_snapshots FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION bind_agent_directory_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
  JOIN conversations c ON c.org_id=r.org_id AND c.id=r.conversation_id JOIN folders f ON f.org_id=r.org_id AND f.id=NEW.folder_id
  WHERE r.org_id=NEW.org_id AND r.id=NEW.run_id AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
  AND p.skill_id='information-organize' AND p.state='active' AND c.type='private' AND c.creator_id=r.actor_id
  AND c.project_id IS NOT DISTINCT FROM f.project_id AND c.current_revision_id=r.conversation_revision_id) THEN
  RAISE EXCEPTION 'directory observation requires current private organization execution' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_directory_binding BEFORE INSERT ON agent_directory_snapshots FOR EACH ROW EXECUTE FUNCTION bind_agent_directory_snapshot();
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization'));
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT,UPDATE ON agent_organization_plans TO copypaste_app;
 GRANT SELECT,INSERT ON agent_organization_events,agent_directory_snapshots TO copypaste_app;
 REVOKE DELETE ON agent_organization_plans FROM copypaste_app;
 REVOKE UPDATE,DELETE ON agent_organization_events,agent_directory_snapshots FROM copypaste_app;
END IF; END; $$;
