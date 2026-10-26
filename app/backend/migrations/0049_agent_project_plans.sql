-- Exact, short-lived grants. This first write workflow is confined to the
-- initiator's private company conversation; no shared/TG authority is implied.
CREATE TABLE agent_plans (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 conversation_id uuid NOT NULL, conversation_revision_id uuid NOT NULL,
 context_security_version bigint NOT NULL CHECK(context_security_version>0),
 actor_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL,
 origin_run_id uuid, kind text NOT NULL CHECK(kind='project.create'),
 intent text NOT NULL CHECK(jsonb_typeof(intent::jsonb)='object' AND octet_length(intent)<=100000),
 plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 template_snapshot jsonb NOT NULL CHECK(jsonb_typeof(template_snapshot)='object'),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 3),
 state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','approved','revoked','committed')),
 operation_id uuid, project_id uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id,conversation_revision_id) REFERENCES conversation_revisions(org_id,conversation_id,id),
 FOREIGN KEY(org_id,origin_run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 FOREIGN KEY(operation_id) REFERENCES idempotency_records(id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '1 hour'),
 CHECK(encode(sha256(convert_to(intent,'UTF8')),'hex')=plan_hash),
 CHECK(((intent::jsonb)->'project'=payload AND (intent::jsonb)->'template_snapshot'=template_snapshot
   AND (intent::jsonb)->>'id'=id::text AND (intent::jsonb)->>'conversation_id'=conversation_id::text
   AND (intent::jsonb)->>'conversation_revision_id'=conversation_revision_id::text AND (intent::jsonb)->>'actor_id'=actor_id::text
   AND ((intent::jsonb)->>'actor_auth_version')::bigint=actor_auth_version
   AND ((intent::jsonb)->>'organization_auth_version')::bigint=organization_auth_version
   AND ((intent::jsonb)->>'context_security_version')::bigint=context_security_version
   AND ((intent::jsonb)->>'created_at')::timestamptz=created_at AND ((intent::jsonb)->>'expires_at')::timestamptz=expires_at
   AND ((intent::jsonb)->>'origin_run_id')::uuid IS NOT DISTINCT FROM origin_run_id
   AND (intent::jsonb)->>'kind'=kind AND (intent::jsonb)->>'reason'=reason) IS TRUE),
 CHECK((state='committed')=(operation_id IS NOT NULL AND project_id IS NOT NULL)),
 CHECK(state='committed' OR (operation_id IS NULL AND project_id IS NULL)),
 CHECK((state='proposed' AND version=1) OR (state='approved' AND version=2) OR (state='revoked' AND version IN (2,3)) OR (state='committed' AND version=3))
);
CREATE INDEX agent_plans_conversation ON agent_plans(org_id,conversation_id,created_at DESC,id DESC);
CREATE FUNCTION validate_agent_plan_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state<>'proposed' OR NEW.version<>1 OR NOT EXISTS(SELECT 1 FROM conversations c
   JOIN conversation_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id
   WHERE c.org_id=NEW.org_id AND c.id=NEW.conversation_id AND c.type='private' AND c.project_id IS NULL
   AND c.creator_id=NEW.actor_id AND c.current_revision_id=NEW.conversation_revision_id AND r.state='active' AND r.context_security_version=NEW.context_security_version)
 OR (NEW.origin_run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
   WHERE r.org_id=NEW.org_id AND r.id=NEW.origin_run_id AND r.actor_id=NEW.actor_id AND r.conversation_id=NEW.conversation_id
   AND r.conversation_revision_id=NEW.conversation_revision_id AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
   AND p.skill_id='project-create' AND p.state='active')) THEN
  RAISE EXCEPTION 'project plan requires a private current conversation and exact origin task' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_plan_insert_binding BEFORE INSERT ON agent_plans FOR EACH ROW EXECUTE FUNCTION validate_agent_plan_insert();
CREATE TABLE agent_plan_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL, version bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN ('proposed','approved','revoked','committed')),
 plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'), actor_id uuid NOT NULL,
 session_id uuid, mfa_verified_at timestamptz, operation_id uuid, project_id uuid,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(plan_id,version),
 FOREIGN KEY(org_id,plan_id) REFERENCES agent_plans(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(operation_id) REFERENCES idempotency_records(id),
 FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 CHECK(kind<>'approved' OR (session_id IS NOT NULL AND mfa_verified_at IS NOT NULL)),
 CHECK((kind='committed')=(operation_id IS NOT NULL AND project_id IS NOT NULL)),
 CHECK(kind='committed' OR (operation_id IS NULL AND project_id IS NULL))
);
CREATE TRIGGER agent_plan_event_immutable BEFORE UPDATE OR DELETE ON agent_plan_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_plan_no_delete BEFORE DELETE ON agent_plans FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_agent_plan() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-ARRAY['version','state','operation_id','project_id']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','state','operation_id','project_id'])
 OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='proposed' AND NEW.state IN ('approved','revoked')) OR (OLD.state='approved' AND NEW.state IN ('committed','revoked'))) THEN
  RAISE EXCEPTION 'plan intent is immutable and transitions must be sequential' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_plan_identity BEFORE UPDATE ON agent_plans FOR EACH ROW EXECUTE FUNCTION preserve_agent_plan();
CREATE FUNCTION validate_agent_plan_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p agent_plans; previous text; verified timestamptz;
BEGIN
 SELECT * INTO p FROM agent_plans WHERE org_id=NEW.org_id AND id=NEW.plan_id;
 IF p.id IS NULL OR NEW.plan_hash<>p.plan_hash OR NEW.actor_id<>p.actor_id OR NEW.version<>p.version OR NEW.kind<>p.state
 OR NEW.operation_id IS DISTINCT FROM p.operation_id OR NEW.project_id IS DISTINCT FROM p.project_id THEN
  RAISE EXCEPTION 'plan event must exactly match its head and actor' USING ERRCODE='23514';
 END IF;
 IF NEW.version=1 THEN
  IF NEW.kind<>'proposed' THEN RAISE EXCEPTION 'plan starts proposed' USING ERRCODE='23514'; END IF;
 ELSE
  SELECT kind INTO previous FROM agent_plan_events WHERE plan_id=p.id AND version=NEW.version-1;
  IF previous IS NULL OR NOT ((previous='proposed' AND NEW.kind IN ('approved','revoked')) OR (previous='approved' AND NEW.kind IN ('revoked','committed'))) THEN
   RAISE EXCEPTION 'plan event history must be contiguous' USING ERRCODE='23514';
  END IF;
 END IF;
 IF NEW.kind='approved' THEN
  SELECT mfa_verified_at INTO verified FROM sessions WHERE org_id=p.org_id AND id=NEW.session_id AND user_id=p.actor_id
   AND revoked_at IS NULL AND expires_at>clock_timestamp() AND auth_version=p.actor_auth_version;
  IF verified IS NULL OR verified IS DISTINCT FROM NEW.mfa_verified_at OR verified<clock_timestamp()-interval '10 minutes' OR verified>clock_timestamp()+interval '1 minute' THEN
   RAISE EXCEPTION 'plan approval requires current interactive MFA evidence' USING ERRCODE='23514';
  END IF;
 END IF;
 IF NEW.kind='committed' AND NOT EXISTS(SELECT 1 FROM idempotency_records i JOIN projects x ON x.org_id=i.org_id AND x.id=p.project_id
   WHERE i.id=p.operation_id AND i.org_id=p.org_id AND i.actor_id=p.actor_id AND i.state='succeeded'
   AND i.route_key='/internal/agent-plans/project.create' AND i.idempotency_key='agent-plan:'||p.id::text
   AND i.response_data->>'id'=p.project_id::text AND x.creator_id=p.actor_id) THEN
  RAISE EXCEPTION 'plan commit requires an exact successful business operation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_plan_event_binding BEFORE INSERT ON agent_plan_events FOR EACH ROW EXECUTE FUNCTION validate_agent_plan_event();
CREATE FUNCTION validate_agent_plan_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p agent_plans;
BEGIN
 SELECT * INTO p FROM agent_plans WHERE id=NEW.id;
 IF NOT EXISTS(SELECT 1 FROM agent_plan_events e WHERE e.plan_id=p.id AND e.org_id=p.org_id AND e.version=p.version AND e.kind=p.state
   AND e.plan_hash=p.plan_hash AND e.operation_id IS NOT DISTINCT FROM p.operation_id AND e.project_id IS NOT DISTINCT FROM p.project_id)
 OR (SELECT count(*) FROM agent_plan_events WHERE plan_id=p.id)<>p.version THEN
  RAISE EXCEPTION 'plan head requires an exact durable history' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_plan_history AFTER INSERT OR UPDATE ON agent_plans DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_plan_head();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'agent_plan_operator','秘书项目计划操作人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a FROM roles r CROSS JOIN unnest(ARRAY['agent.plan.read','agent.plan.propose','agent.plan.approve','agent.plan.commit','agent.plan.revoke'])a WHERE r.key='agent_plan_operator' ON CONFLICT DO NOTHING;
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project'));
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT,UPDATE ON agent_plans TO copypaste_app;
 GRANT SELECT,INSERT ON agent_plan_events TO copypaste_app;
 REVOKE DELETE ON agent_plans FROM copypaste_app;
 REVOKE UPDATE,DELETE ON agent_plan_events FROM copypaste_app;
END IF; END; $$;
