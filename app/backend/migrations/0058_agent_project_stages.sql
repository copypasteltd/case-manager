-- Extend exact L1 commands; historical instructions remain immutable.
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_tool_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_tool_check CHECK(tool IN ('update_project','update_task','transition_task','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_resource_type_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_resource_type_check CHECK(resource_type IN ('project','task','milestone'));
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_scope_check CHECK(
 (tool IN ('update_project','transition_project') AND resource_type='project' AND resource_id=project_id)
 OR (tool IN ('update_task','transition_task') AND resource_type='task')
 OR (tool IN ('update_milestone','transition_milestone') AND resource_type='milestone'));

-- NULL is retained only on pre-migration immutable history. New authority cannot
-- be issued or executed without its actual originating interactive login.
ALTER TABLE agent_run_mutations ADD COLUMN session_id uuid;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_session_fk FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id);
CREATE FUNCTION validate_agent_mutation_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m agent_run_mutations; r agent_runs; actual jsonb; expected jsonb; field text; target text;
BEGIN
 IF TG_TABLE_NAME='agent_run_mutations' THEN m:=NEW;
 ELSE SELECT * INTO m FROM agent_run_mutations WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 END IF;
 SELECT * INTO r FROM agent_runs WHERE org_id=m.org_id AND id=m.run_id;
 IF m.session_id IS NULL OR ((m.intent::jsonb)->>'session_id'=m.session_id::text) IS NOT TRUE
  OR r.channel NOT IN ('web','android','wechat','douyin') OR NOT EXISTS(
   SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
   WHERE s.org_id=m.org_id AND s.id=m.session_id AND s.user_id=m.actor_id AND s.device_type=r.channel
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
   AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version AND o.authorization_version=r.organization_auth_version) THEN
  RAISE EXCEPTION 'project delegation requires its original live interactive session' USING ERRCODE='23514';
 END IF;
 IF m.resource_type='project' THEN
  SELECT to_jsonb(p) INTO actual FROM projects p WHERE p.org_id=m.org_id AND p.id=m.resource_id AND p.id=m.project_id;
 ELSIF m.resource_type='task' THEN
  SELECT to_jsonb(t) INTO actual FROM tasks t WHERE t.org_id=m.org_id AND t.id=m.resource_id AND t.project_id=m.project_id;
 ELSE
  SELECT to_jsonb(x) INTO actual FROM milestones x WHERE x.org_id=m.org_id AND x.id=m.resource_id AND x.project_id=m.project_id;
 END IF;
 IF actual IS NULL THEN RAISE EXCEPTION 'project command target is outside the exact project' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME='agent_run_mutations' THEN
  IF (actual->>'version')::bigint<>m.base_version OR (m.before_data->>'id'=m.resource_id::text AND (m.before_data->>'version')::bigint=m.base_version) IS NOT TRUE THEN
   RAISE EXCEPTION 'project instruction requires its exact current target' USING ERRCODE='23514';
  END IF;
 ELSE
  IF (actual->>'version')::bigint<>NEW.result_version THEN
   RAISE EXCEPTION 'project receipt requires the actual committed version' USING ERRCODE='23514';
  END IF;
  FOREACH field IN ARRAY ARRAY['id','version','state',CASE WHEN m.resource_type='project' THEN 'name' ELSE 'title' END] LOOP
   IF actual->field IS DISTINCT FROM NEW.result_data->field THEN
    RAISE EXCEPTION 'project receipt differs from the actual resource' USING ERRCODE='23514';
   END IF;
  END LOOP;
  IF m.tool='update_milestone' THEN
   FOR field,expected IN SELECT key,value FROM jsonb_each(m.arguments) WHERE key NOT IN ('id','expected_version','reason') LOOP
    IF field IN ('due_at','start_at') AND expected<>'null'::jsonb THEN
     IF (actual->>field)::timestamptz IS DISTINCT FROM (expected#>>'{}')::timestamptz THEN
      RAISE EXCEPTION 'project timestamp differs from the authorized change' USING ERRCODE='23514';
     END IF;
    ELSIF field IN ('name','title','type') AND jsonb_typeof(expected)='string' THEN
     IF actual->>field IS DISTINCT FROM btrim(expected#>>'{}') THEN
      RAISE EXCEPTION 'project name differs from the authorized change' USING ERRCODE='23514';
     END IF;
    ELSIF actual->field IS DISTINCT FROM expected THEN
     RAISE EXCEPTION 'project field differs from the authorized change' USING ERRCODE='23514';
    END IF;
   END LOOP;
  ELSIF m.tool IN ('transition_project','transition_milestone') THEN
   target:=CASE m.tool
    WHEN 'transition_project' THEN CASE m.arguments->>'action' WHEN 'activate' THEN 'active' WHEN 'hold' THEN 'on_hold' WHEN 'resume' THEN 'active' WHEN 'close' THEN 'closing' WHEN 'cancel' THEN 'cancelled' WHEN 'reopen' THEN 'active' END
    WHEN 'transition_milestone' THEN CASE m.arguments->>'action' WHEN 'achieve' THEN 'achieved' WHEN 'miss' THEN 'missed' WHEN 'cancel' THEN 'cancelled' WHEN 'reopen' THEN 'open' END
   END;
   IF target IS NULL OR actual->>'state' IS DISTINCT FROM target THEN
    RAISE EXCEPTION 'project state differs from the authorized action' USING ERRCODE='23514';
   END IF;
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_mutation_original_session BEFORE INSERT ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_session();
CREATE TRIGGER agent_mutation_commit_original_session BEFORE INSERT ON agent_mutation_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_mutation_session();

ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
