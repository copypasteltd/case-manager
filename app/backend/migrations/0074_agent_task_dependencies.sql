-- Add one exact task dependency delegation; all prior guards stay installed.
CREATE FUNCTION validate_agent_task_dependency() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m agent_run_mutations; t tasks; p tasks; parent projects; present boolean; field text;
BEGIN
 IF TG_TABLE_NAME='agent_run_mutations' THEN m:=NEW;
 ELSE SELECT * INTO m FROM agent_run_mutations WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 END IF;
 IF m.tool<>'update_task_dependency' THEN RETURN NEW; END IF;
 IF NOT (m.arguments ?& ARRAY['id','expected_version','predecessor_id','expected_predecessor_version','action','reason'])
  OR (SELECT count(*) FROM jsonb_object_keys(m.arguments))<>6
  OR jsonb_typeof(m.arguments->'id') IS DISTINCT FROM 'string'
  OR jsonb_typeof(m.arguments->'predecessor_id') IS DISTINCT FROM 'string'
  OR jsonb_typeof(m.arguments->'expected_version') IS DISTINCT FROM 'number'
  OR jsonb_typeof(m.arguments->'expected_predecessor_version') IS DISTINCT FROM 'number'
  OR jsonb_typeof(m.arguments->'action') IS DISTINCT FROM 'string'
  OR jsonb_typeof(m.arguments->'reason') IS DISTINCT FROM 'string'
  OR (m.arguments->>'id'=m.resource_id::text AND (m.arguments->>'expected_version')::bigint=m.base_version
   AND m.arguments->>'predecessor_id'<>m.resource_id::text AND m.arguments->>'action' IN ('add','remove')
   AND (m.arguments->>'expected_predecessor_version')::bigint>0 AND length(btrim(m.arguments->>'reason')) BETWEEN 1 AND 200) IS NOT TRUE THEN
  RAISE EXCEPTION 'dependency delegation requires one exact pair, two versions, action and reason' USING ERRCODE='23514';
 END IF;
 SELECT * INTO parent FROM projects WHERE org_id=m.org_id AND id=m.project_id;
 SELECT * INTO t FROM tasks WHERE org_id=m.org_id AND project_id=m.project_id AND id=m.resource_id;
 SELECT * INTO p FROM tasks WHERE org_id=m.org_id AND project_id=m.project_id AND id=(m.arguments->>'predecessor_id')::uuid;
 SELECT EXISTS(SELECT 1 FROM task_dependencies WHERE org_id=m.org_id AND project_id=m.project_id AND successor_id=t.id AND predecessor_id=p.id) INTO present;
 IF parent.id IS NULL OR parent.state NOT IN ('draft','active','on_hold') OR t.id IS NULL OR p.id IS NULL
  OR p.version IS DISTINCT FROM (m.arguments->>'expected_predecessor_version')::bigint OR t.state IN ('done','cancelled')
  OR (m.before_data->>'project_id'=m.project_id::text AND m.before_data->>'predecessor_id'=p.id::text AND (m.before_data->>'predecessor_version')::bigint=p.version
   AND m.before_data->>'predecessor_title'=p.title AND m.before_data->>'predecessor_state'=p.state
   AND m.before_data->>'predecessor_sha256' ~ '^[0-9a-f]{64}$'
   AND (m.before_data->>'dependency_present')::boolean=(m.arguments->>'action'='remove')) IS NOT TRUE THEN
  RAISE EXCEPTION 'dependency delegation requires the exact current project and predecessor snapshot' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_run_mutations' THEN
  IF t.version<>m.base_version OR present IS DISTINCT FROM (m.arguments->>'action'='remove') THEN
   RAISE EXCEPTION 'dependency delegation is stale or changes no relationship' USING ERRCODE='23514';
  END IF;
 ELSE
  IF m.arguments->>'action'='add' AND (t.state NOT IN ('todo','blocked') AND p.state<>'done' OR EXISTS(
   WITH RECURSIVE reachable(id) AS (
    SELECT successor_id FROM task_dependencies WHERE org_id=m.org_id AND project_id=m.project_id AND predecessor_id=t.id
    UNION SELECT d.successor_id FROM task_dependencies d JOIN reachable r ON d.predecessor_id=r.id WHERE d.org_id=m.org_id AND d.project_id=m.project_id
   ) SELECT 1 FROM reachable WHERE id=p.id)) THEN
   RAISE EXCEPTION 'dependency receipt cannot bypass original cycle or state rules' USING ERRCODE='23514';
  END IF;
  IF t.version<>m.base_version+1 OR NEW.result_version<>t.version OR present IS DISTINCT FROM (m.arguments->>'action'='add')
   OR NEW.result_data->>'predecessor_id' IS DISTINCT FROM p.id::text
   OR (NEW.result_data->>'predecessor_version')::bigint IS DISTINCT FROM p.version
   OR NEW.result_data->>'dependency_action' IS DISTINCT FROM m.arguments->>'action'
   OR (NEW.result_data->>'dependency_present')::boolean IS DISTINCT FROM present THEN
   RAISE EXCEPTION 'dependency receipt differs from the actual committed edge' USING ERRCODE='23514';
  END IF;
  FOREACH field IN ARRAY ARRAY['id','version','project_id','creator_id','title','body','priority','state','assignee_id','parent_id','milestone_id','review_required','created_at','updated_at','start_at','due_at'] LOOP
   IF field IN ('created_at','updated_at','start_at','due_at') THEN
    IF (to_jsonb(t)->>field)::timestamptz IS DISTINCT FROM (NEW.result_data->>field)::timestamptz THEN
     RAISE EXCEPTION 'dependency receipt timestamp differs from the actual task' USING ERRCODE='23514';
    END IF;
   ELSIF to_jsonb(t)->field IS DISTINCT FROM NEW.result_data->field THEN
    RAISE EXCEPTION 'dependency receipt differs from the actual task' USING ERRCODE='23514';
   END IF;
   IF field IN ('title','body','priority','state','assignee_id','parent_id','milestone_id','review_required','start_at','due_at') THEN
    IF field IN ('start_at','due_at') THEN
     IF (to_jsonb(t)->>field)::timestamptz IS DISTINCT FROM (m.before_data->>field)::timestamptz THEN
      RAISE EXCEPTION 'dependency command changed an unauthorized task date' USING ERRCODE='23514';
     END IF;
    ELSIF to_jsonb(t)->field IS DISTINCT FROM m.before_data->field THEN
     RAISE EXCEPTION 'dependency command changed an unauthorized task field' USING ERRCODE='23514';
    END IF;
   END IF;
  END LOOP;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_dependency_intent BEFORE INSERT ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION validate_agent_task_dependency();
CREATE TRIGGER agent_dependency_commit BEFORE INSERT ON agent_mutation_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_task_dependency();
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_tool_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','update_project','update_task','update_task_dependency','transition_task','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_scope_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_scope_check CHECK(
 (tool IN ('create_task','create_risk','create_issue','create_deliverable','create_milestone','update_project','transition_project') AND resource_type='project' AND resource_id=project_id)
 OR (tool IN ('update_task','update_task_dependency','transition_task') AND resource_type='task')
 OR (tool IN ('update_milestone','transition_milestone') AND resource_type='milestone')
 OR (tool='update_deliverable' AND resource_type='deliverable')
 OR (tool='update_risk' AND resource_type='risk') OR (tool='update_issue' AND resource_type='issue'));
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
