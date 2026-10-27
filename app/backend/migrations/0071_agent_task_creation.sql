-- Exact complete task creation. Reinstall existing guards with the additive tool; applied migrations remain immutable.
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_tool_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','update_project','update_task','transition_task','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_scope_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_scope_check CHECK(
 (tool IN ('create_task','create_risk','create_issue','create_deliverable','create_milestone','update_project','transition_project') AND resource_type='project' AND resource_id=project_id)
 OR (tool IN ('update_task','transition_task') AND resource_type='task')
 OR (tool IN ('update_milestone','transition_milestone') AND resource_type='milestone')
 OR (tool='update_deliverable' AND resource_type='deliverable')
 OR (tool='update_risk' AND resource_type='risk') OR (tool='update_issue' AND resource_type='issue'));
ALTER TABLE agent_run_mutations DROP CONSTRAINT agent_run_mutations_resource_type_check;
ALTER TABLE agent_run_mutations ADD CONSTRAINT agent_run_mutations_resource_type_check CHECK(resource_type IN ('project','task','milestone','deliverable','risk','issue'));
CREATE OR REPLACE FUNCTION validate_agent_mutation_binding() RETURNS trigger LANGUAGE plpgsql AS $$
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
   OR (CASE WHEN m.tool='create_task' THEN
    (NEW.result_version=1 AND NEW.result_data->>'project_id'=m.project_id::text
     AND NEW.result_data->>'id'<>m.resource_id::text AND NEW.result_data->>'version'='1'
     AND NEW.result_data->>'state'='todo') IS NOT TRUE
    WHEN m.tool IN ('create_risk','create_issue') THEN
    (NEW.result_version=1 AND NEW.result_data->>'project_id'=m.project_id::text
     AND NEW.result_data->>'id'<>m.resource_id::text AND NEW.result_data->>'version'='1'
     AND NEW.result_data->>'state'='open') IS NOT TRUE
    WHEN m.tool='create_milestone' THEN
    (NEW.result_version=1 AND NEW.result_data->>'project_id'=m.project_id::text
     AND NEW.result_data->>'id'<>m.resource_id::text AND NEW.result_data->>'version'='1'
     AND NEW.result_data->>'state'='open') IS NOT TRUE
    WHEN m.tool='create_deliverable' THEN
    (NEW.result_version=1 AND NEW.result_data->>'id'<>m.resource_id::text
     AND NEW.result_data->>'version'='1' AND NEW.result_data->'revision_id'='null'::jsonb) IS NOT TRUE
    ELSE NEW.result_version<>m.base_version+1 OR (NEW.result_data->>'id'=m.resource_id::text
     AND (NEW.result_data->>'version')::bigint=NEW.result_version) IS NOT TRUE END) OR NOT EXISTS(
    SELECT 1 FROM idempotency_records i WHERE i.id=NEW.operation_id AND i.org_id=r.org_id AND i.actor_id=r.actor_id
    AND i.state='succeeded' AND i.route_key='/internal/agent-mutations/'||m.tool
    AND i.idempotency_key='agent-mutation:'||r.id::text AND i.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'mutation commit requires the exact successful versioned business command' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE OR REPLACE FUNCTION validate_agent_mutation_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m agent_run_mutations; r agent_runs; actual jsonb; expected jsonb; field text; target text; child jsonb; is_creation boolean; risk boolean; required_fields text[]; target_id uuid; trim_chars constant text := U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
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

 -- A complete task is allocated by the original task command, with no hidden
 -- assignee, parent, milestone, date, review setting, dependency or result ID.
 IF m.tool='create_task' THEN
  required_fields:=ARRAY['id','expected_version','reason','title','body','assignee_id','parent_id','milestone_id','start_at','due_at','priority','review_required'];
  IF NOT (m.arguments ?& required_fields) OR (SELECT count(*) FROM jsonb_object_keys(m.arguments))<>12
   OR (m.arguments->>'id'=m.resource_id::text AND (m.arguments->>'expected_version')::bigint=m.base_version) IS NOT TRUE
   OR jsonb_typeof(m.arguments->'review_required') IS DISTINCT FROM 'boolean' THEN
   RAISE EXCEPTION 'task creation requires complete exact fields' USING ERRCODE='23514';
  END IF;
  SELECT to_jsonb(p) INTO actual FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.id=m.resource_id;
  IF actual IS NULL OR (actual->>'version')::bigint<>m.base_version OR actual->>'state' NOT IN ('draft','active','on_hold') THEN
   RAISE EXCEPTION 'task creation requires its writable exact parent project' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='agent_run_mutations' THEN
   IF (m.before_data->>'id'=m.resource_id::text AND (m.before_data->>'version')::bigint=m.base_version) IS NOT TRUE THEN
    RAISE EXCEPTION 'task creation requires the actual project snapshot' USING ERRCODE='23514';
   END IF;
   RETURN NEW;
  END IF;
  SELECT to_jsonb(t) INTO actual FROM tasks t WHERE t.org_id=m.org_id AND t.project_id=m.project_id AND t.id=(NEW.result_data->>'id')::uuid;
  IF actual IS NULL OR actual->>'id'=m.resource_id::text OR actual->>'version' IS DISTINCT FROM '1' OR NEW.result_version<>1
   OR actual->>'creator_id' IS DISTINCT FROM m.actor_id::text OR actual->>'state' IS DISTINCT FROM 'todo'
   OR actual->>'title' IS DISTINCT FROM btrim(m.arguments->>'title',trim_chars)
   OR actual->'body' IS DISTINCT FROM m.arguments->'body'
   OR actual->'assignee_id' IS DISTINCT FROM m.arguments->'assignee_id'
   OR actual->'parent_id' IS DISTINCT FROM m.arguments->'parent_id'
   OR actual->'milestone_id' IS DISTINCT FROM m.arguments->'milestone_id'
   OR actual->'priority' IS DISTINCT FROM m.arguments->'priority'
   OR actual->'review_required' IS DISTINCT FROM m.arguments->'review_required'
   OR actual->'completed_at' IS DISTINCT FROM 'null'::jsonb THEN
   RAISE EXCEPTION 'created task differs from exact authorized metadata' USING ERRCODE='23514';
  END IF;
  FOREACH field IN ARRAY ARRAY['id','project_id','version','creator_id','title','body','assignee_id','parent_id','milestone_id','priority','state','review_required','created_at','updated_at','start_at','due_at'] LOOP
   IF field IN ('created_at','updated_at','start_at','due_at') THEN
    IF (actual->>field)::timestamptz IS DISTINCT FROM (NEW.result_data->>field)::timestamptz THEN
     RAISE EXCEPTION 'task receipt timestamp differs from the actual resource' USING ERRCODE='23514';
    END IF;
    IF field IN ('start_at','due_at') AND (actual->>field)::timestamptz IS DISTINCT FROM (m.arguments->>field)::timestamptz THEN
     RAISE EXCEPTION 'task timestamp differs from the authorized instant' USING ERRCODE='23514';
    END IF;
   ELSIF actual->field IS DISTINCT FROM NEW.result_data->field THEN
    RAISE EXCEPTION 'task receipt differs from the actual resource' USING ERRCODE='23514';
   END IF;
  END LOOP;
  RETURN NEW;
 END IF;
 -- The four concern tools have complete metadata. State is explicit only when
 -- changing it; creation cannot accept a state or a caller-selected child ID.
 IF m.tool IN ('create_risk','update_risk','create_issue','update_issue') THEN
  is_creation:=m.tool IN ('create_risk','create_issue'); risk:=m.tool IN ('create_risk','update_risk');
  required_fields:=ARRAY['id','expected_version','reason','title','description','owner_id','severity','due_at'];
  required_fields:=required_fields || (CASE WHEN risk THEN ARRAY['probability','mitigation'] ELSE ARRAY['resolution'] END);
  IF NOT (m.arguments ?& required_fields) OR EXISTS(
   SELECT 1 FROM jsonb_object_keys(m.arguments) key WHERE key<>ALL(required_fields)
    AND (key<>'state' OR is_creation))
   OR (m.arguments->>'id'=m.resource_id::text AND (m.arguments->>'expected_version')::bigint=m.base_version) IS NOT TRUE THEN
   RAISE EXCEPTION 'concern instruction requires complete exact fields' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='agent_run_mutations' AND is_creation THEN
   SELECT to_jsonb(p) INTO actual FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.id=m.resource_id;
  ELSE
   IF is_creation THEN target_id:=(NEW.result_data->>'id')::uuid; ELSE target_id:=m.resource_id; END IF;
   IF risk THEN SELECT to_jsonb(x) INTO actual FROM risks x WHERE x.org_id=m.org_id AND x.id=target_id AND x.project_id=m.project_id;
   ELSE SELECT to_jsonb(x) INTO actual FROM issues x WHERE x.org_id=m.org_id AND x.id=target_id AND x.project_id=m.project_id;
   END IF;
  END IF;
  IF actual IS NULL THEN RAISE EXCEPTION 'concern target is outside the exact project' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='agent_run_mutations' THEN
   IF (actual->>'version')::bigint<>m.base_version OR
    (m.before_data->>'id'=m.resource_id::text AND (m.before_data->>'version')::bigint=m.base_version) IS NOT TRUE THEN
    RAISE EXCEPTION 'concern instruction requires the actual base' USING ERRCODE='23514';
   END IF;
   RETURN NEW;
  END IF;
  IF (actual->>'version')::bigint<>NEW.result_version
   OR (is_creation AND ((actual->>'version')::bigint<>1 OR actual->>'creator_id'<>m.actor_id::text
     OR actual->>'id'=m.resource_id::text OR NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.version=m.base_version)))
   OR (NOT is_creation AND (actual->>'creator_id' IS DISTINCT FROM m.before_data->>'creator_id'))
   OR actual->>'state' IS DISTINCT FROM (CASE WHEN is_creation THEN 'open' ELSE coalesce(m.arguments->>'state',m.before_data->>'state') END)
   OR actual->>'title' IS DISTINCT FROM btrim(m.arguments->>'title',trim_chars)
   OR actual->'description' IS DISTINCT FROM m.arguments->'description'
   OR actual->'owner_id' IS DISTINCT FROM m.arguments->'owner_id'
   OR actual->'severity' IS DISTINCT FROM m.arguments->'severity'
   OR (actual->>'due_at')::timestamptz IS DISTINCT FROM (m.arguments->>'due_at')::timestamptz
   OR (risk AND (actual->'probability' IS DISTINCT FROM m.arguments->'probability' OR actual->'mitigation' IS DISTINCT FROM m.arguments->'mitigation'))
   OR (NOT risk AND actual->'resolution' IS DISTINCT FROM m.arguments->'resolution') THEN
   RAISE EXCEPTION 'concern commit differs from exact metadata, state or parent' USING ERRCODE='23514';
  END IF;
  IF risk THEN
   IF NOT is_creation AND m.arguments->>'state'='realized' THEN
    SELECT to_jsonb(x) INTO child FROM issues x WHERE x.org_id=m.org_id AND x.id=(actual->>'issue_id')::uuid AND x.project_id=m.project_id;
    IF child IS NULL OR child->>'id'=actual->>'id' OR child->>'version'<>'1' OR child->>'state'<>'open'
     OR child->>'creator_id'<>m.actor_id::text OR child->>'resolution'<>''
     OR child->'title' IS DISTINCT FROM actual->'title' OR child->'description' IS DISTINCT FROM actual->'description'
     OR child->'owner_id' IS DISTINCT FROM actual->'owner_id' OR child->'severity' IS DISTINCT FROM actual->'severity'
     OR (child->>'due_at')::timestamptz IS DISTINCT FROM (actual->>'due_at')::timestamptz
     OR coalesce(m.before_data->'issue_id','null'::jsonb)<>'null'::jsonb THEN
     RAISE EXCEPTION 'realized risk requires its exact newly created issue in the same transaction' USING ERRCODE='23514';
    END IF;
   ELSIF coalesce(actual->'issue_id','null'::jsonb)<>coalesce(m.before_data->'issue_id','null'::jsonb) THEN
    RAISE EXCEPTION 'concern command cannot change an unapproved issue link' USING ERRCODE='23514';
   END IF;
  END IF;
  FOREACH field IN ARRAY (ARRAY['id','project_id','version','creator_id','title','description','owner_id','severity','state','due_at','created_at','updated_at'] ||
   CASE WHEN risk THEN ARRAY['probability','mitigation','issue_id'] ELSE ARRAY['resolution'] END) LOOP
   IF field IN ('due_at','created_at','updated_at') THEN
    IF (actual->>field)::timestamptz IS DISTINCT FROM (NEW.result_data->>field)::timestamptz THEN
     RAISE EXCEPTION 'concern receipt timestamp differs from actual resource' USING ERRCODE='23514';
    END IF;
   ELSIF coalesce(actual->field,'null'::jsonb) IS DISTINCT FROM coalesce(NEW.result_data->field,'null'::jsonb) THEN
    -- Optional empty text is omitted by the original public Go representation.
    IF field NOT IN ('mitigation','resolution') OR actual->>field<>'' OR NEW.result_data ? field THEN
     RAISE EXCEPTION 'concern receipt differs from actual resource' USING ERRCODE='23514';
    END IF;
   END IF;
  END LOOP;
  RETURN NEW;
 END IF;
 IF m.tool='create_deliverable' AND TG_TABLE_NAME='agent_mutation_commits' THEN
  SELECT to_jsonb(d) INTO actual FROM deliverables d
   WHERE d.org_id=m.org_id AND d.id=(NEW.result_data->>'id')::uuid AND d.project_id=m.project_id;
  IF actual IS NULL OR NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.version=m.base_version)
   OR actual->>'version' IS DISTINCT FROM '1' OR NEW.result_version<>1
   OR actual->>'state' IS DISTINCT FROM 'draft' OR actual->>'protocol_version' IS DISTINCT FROM '1'
   OR actual->>'creator_id' IS DISTINCT FROM m.actor_id::text
   OR actual->'current_revision_id' IS DISTINCT FROM 'null'::jsonb
   OR EXISTS(SELECT 1 FROM deliverable_revisions v WHERE v.org_id=m.org_id AND v.deliverable_id=(actual->>'id')::uuid)
   OR (SELECT count(*) FROM jsonb_object_keys(m.arguments))<>7
   OR NOT (m.arguments ?& ARRAY['id','expected_version','reason','name','acceptance_criteria','owner_id','milestone_id'])
   OR actual->>'name' IS DISTINCT FROM btrim(m.arguments->>'name',trim_chars)
   OR actual->>'acceptance_criteria' IS DISTINCT FROM btrim(m.arguments->>'acceptance_criteria',trim_chars)
   OR actual->'owner_id' IS DISTINCT FROM m.arguments->'owner_id'
   OR actual->'milestone_id' IS DISTINCT FROM m.arguments->'milestone_id'
   OR NEW.result_data->'id' IS DISTINCT FROM actual->'id'
   OR NEW.result_data->'version' IS DISTINCT FROM actual->'version'
   OR NEW.result_data->'revision_id' IS DISTINCT FROM 'null'::jsonb
   OR (SELECT count(*) FROM jsonb_object_keys(NEW.result_data))<>3 THEN
   RAISE EXCEPTION 'deliverable creation differs from the exact parent, metadata or draft receipt' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 IF m.tool='create_milestone' AND TG_TABLE_NAME='agent_mutation_commits' THEN
  SELECT to_jsonb(x) INTO actual FROM milestones x
   WHERE x.org_id=m.org_id AND x.id=(NEW.result_data->>'id')::uuid AND x.project_id=m.project_id;
  IF actual IS NULL OR NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.version=m.base_version)
   OR (actual->>'version')::bigint<>1 OR NEW.result_version<>1 OR actual->>'state'<>'open'
   OR actual->>'creator_id'<>m.actor_id::text
   OR actual->>'title' IS DISTINCT FROM btrim(m.arguments->>'title',trim_chars)
   OR actual->>'owner_id' IS DISTINCT FROM m.arguments->>'owner_id'
   OR (actual->>'due_at')::timestamptz IS DISTINCT FROM (m.arguments->>'due_at')::timestamptz THEN
   RAISE EXCEPTION 'milestone creation differs from exact parent and authorized fields' USING ERRCODE='23514';
  END IF;
  FOREACH field IN ARRAY ARRAY['id','project_id','version','state','title','owner_id','due_at','creator_id'] LOOP
   IF actual->field IS DISTINCT FROM NEW.result_data->field THEN
    -- JSON time formatting may differ; compare actual instants.
    IF field<>'due_at' OR (actual->>'due_at')::timestamptz IS DISTINCT FROM (NEW.result_data->>'due_at')::timestamptz THEN
     RAISE EXCEPTION 'milestone creation result differs from actual resource' USING ERRCODE='23514';
    END IF;
   END IF;
  END LOOP;
  RETURN NEW;
 END IF;
 IF m.resource_type='project' THEN
  SELECT to_jsonb(p) INTO actual FROM projects p WHERE p.org_id=m.org_id AND p.id=m.resource_id AND p.id=m.project_id;
 ELSIF m.resource_type='deliverable' THEN
  SELECT to_jsonb(d) INTO actual FROM deliverables d WHERE d.org_id=m.org_id AND d.id=m.resource_id AND d.project_id=m.project_id;
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
  IF m.tool='update_deliverable' THEN
   IF actual->>'protocol_version'<>'1' OR actual->>'state' NOT IN ('draft','changes_requested')
    OR actual->'current_revision_id' IS DISTINCT FROM m.before_data->'current_revision_id'
    OR actual->>'state' IS DISTINCT FROM m.before_data->>'state'
    OR NEW.result_data->'revision_id' IS DISTINCT FROM actual->'current_revision_id'
    OR (SELECT count(*) FROM jsonb_object_keys(m.arguments))<>7
    OR NOT (m.arguments ?& ARRAY['id','expected_version','reason','name','acceptance_criteria','owner_id','milestone_id'])
    OR actual->>'name' IS DISTINCT FROM btrim(m.arguments->>'name',trim_chars)
    OR actual->>'acceptance_criteria' IS DISTINCT FROM btrim(m.arguments->>'acceptance_criteria',trim_chars)
    OR actual->'owner_id' IS DISTINCT FROM m.arguments->'owner_id'
    OR actual->'milestone_id' IS DISTINCT FROM m.arguments->'milestone_id' THEN
    RAISE EXCEPTION 'deliverable receipt differs from exact metadata or changes a human revision' USING ERRCODE='23514';
   END IF;
   RETURN NEW;
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
     IF actual->>field IS DISTINCT FROM btrim(expected#>>'{}',trim_chars) THEN
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
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
