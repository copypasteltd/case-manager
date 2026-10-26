-- L1 authority is an exact, immutable instruction from the authenticated sender.
-- Model text, a source citation, and an earlier run never confer this authority.
CREATE TABLE agent_run_mutations (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, actor_id uuid NOT NULL,
 conversation_id uuid NOT NULL, project_id uuid NOT NULL,
 tool text NOT NULL CHECK(tool IN ('update_project','update_task','transition_task')),
 resource_type text NOT NULL CHECK(resource_type IN ('project','task')), resource_id uuid NOT NULL,
 base_version bigint NOT NULL CHECK(base_version>0), base_sha256 text NOT NULL CHECK(base_sha256 ~ '^[0-9a-f]{64}$'),
 arguments jsonb NOT NULL CHECK(jsonb_typeof(arguments)='object'),
 before_data jsonb NOT NULL CHECK(jsonb_typeof(before_data)='object'),
 intent text NOT NULL CHECK(octet_length(intent)<=100000 AND jsonb_typeof(intent::jsonb)='object'),
 intent_sha256 text NOT NULL CHECK(intent_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,run_id), FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 CHECK((tool='update_project' AND resource_type='project' AND resource_id=project_id) OR (tool IN ('update_task','transition_task') AND resource_type='task')),
 CHECK(encode(sha256(convert_to(intent,'UTF8')),'hex')=intent_sha256),
 CHECK(((intent::jsonb)->>'run_id'=run_id::text AND (intent::jsonb)->>'actor_id'=actor_id::text
  AND (intent::jsonb)->>'conversation_id'=conversation_id::text AND (intent::jsonb)->>'project_id'=project_id::text
  AND (intent::jsonb)->>'tool'=tool AND (intent::jsonb)->>'resource_type'=resource_type
  AND (intent::jsonb)->>'resource_id'=resource_id::text AND ((intent::jsonb)->>'base_version')::bigint=base_version
  AND (intent::jsonb)->>'base_sha256'=base_sha256 AND (intent::jsonb)->'arguments'=arguments
  AND (intent::jsonb)->'before'=before_data AND arguments->>'id'=resource_id::text
  AND (arguments->>'expected_version')::bigint=base_version) IS TRUE)
);
CREATE TABLE agent_mutation_commits (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),
 result_version bigint NOT NULL CHECK(result_version>1), result_sha256 text NOT NULL CHECK(result_sha256 ~ '^[0-9a-f]{64}$'),
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_run_mutations(org_id,run_id)
);
CREATE TRIGGER agent_mutation_immutable BEFORE UPDATE OR DELETE ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_mutation_commit_immutable BEFORE UPDATE OR DELETE ON agent_mutation_commits FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_agent_mutation_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; m agent_run_mutations;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF TG_TABLE_NAME='agent_run_mutations' THEN
  IF r.id IS NULL OR r.actor_id<>NEW.actor_id OR r.conversation_id<>NEW.conversation_id OR r.state<>'queued'
   OR (((NEW.intent::jsonb)->>'actor_auth_version')::bigint=r.actor_auth_version
    AND ((NEW.intent::jsonb)->>'organization_auth_version')::bigint=r.organization_auth_version
    AND (NEW.intent::jsonb)->>'conversation_revision_id'=r.conversation_revision_id::text
    AND ((NEW.intent::jsonb)->>'context_security_version')::bigint=r.context_security_version) IS NOT TRUE
   OR r.channel='telegram' OR r.cancel_requested_at IS NOT NULL OR NOT EXISTS(
    SELECT 1 FROM conversations c JOIN agent_runtime_profiles p ON p.org_id=c.org_id AND p.id=r.profile_id
    WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.project_id=NEW.project_id
     AND c.current_revision_id=r.conversation_revision_id AND p.skill_id='project-update' AND p.state='active') THEN
   RAISE EXCEPTION 'mutation requires an exact sender, project conversation and queued update run' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO m FROM agent_run_mutations WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  IF m.run_id IS NULL OR r.state<>'running' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR NEW.result_version<>m.base_version+1 OR (NEW.result_data->>'id'=m.resource_id::text
   AND (NEW.result_data->>'version')::bigint=NEW.result_version) IS NOT TRUE OR NOT EXISTS(
    SELECT 1 FROM idempotency_records i WHERE i.id=NEW.operation_id AND i.org_id=r.org_id AND i.actor_id=r.actor_id
    AND i.state='succeeded' AND i.route_key='/internal/agent-mutations/'||m.tool
    AND i.idempotency_key='agent-mutation:'||r.id::text AND i.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'mutation commit requires the exact successful versioned business command' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_mutation_binding BEFORE INSERT ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_binding();
CREATE TRIGGER agent_mutation_commit_binding BEFORE INSERT ON agent_mutation_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_binding();
CREATE FUNCTION validate_agent_mutation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m agent_run_mutations;
BEGIN
 SELECT * INTO m FROM agent_run_mutations WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_run_mutations' THEN
  IF NOT EXISTS(SELECT 1 FROM conversation_messages x WHERE x.org_id=m.org_id AND x.run_id=m.run_id AND x.role='user'
   AND x.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_mutation','id',m.run_id,'resource_version',1,'sha256',m.intent_sha256))) THEN
   RAISE EXCEPTION 'mutation intent must be cited by its immutable sender message' USING ERRCODE='23514';
  END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts x WHERE x.org_id=NEW.org_id AND x.run_id=NEW.run_id AND x.tool=m.tool
   AND x.output->>'operation_id'=NEW.operation_id::text AND x.output->>'committed'='true'
   AND x.output->>'status'='succeeded'
   AND x.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_mutation','id',m.run_id,'resource_version',1,'sha256',m.intent_sha256))) THEN
   RAISE EXCEPTION 'mutation commit must have its exact durable tool receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_mutation_sender_evidence AFTER INSERT ON agent_run_mutations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_evidence();
CREATE CONSTRAINT TRIGGER agent_mutation_tool_evidence AFTER INSERT ON agent_mutation_commits DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_evidence();
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task'));
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_run_mutations,agent_mutation_commits TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_run_mutations,agent_mutation_commits FROM copypaste_app;
END IF; END; $$;
