-- Match Go strings.TrimSpace for the complete authorized deliverable text.
-- Migration0067 has already been applied to development; preserve its ledger.
CREATE OR REPLACE FUNCTION validate_agent_mutation_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m agent_run_mutations; r agent_runs; actual jsonb; expected jsonb; field text; target text; trim_chars constant text := U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
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
 IF m.tool='create_milestone' AND TG_TABLE_NAME='agent_mutation_commits' THEN
  SELECT to_jsonb(x) INTO actual FROM milestones x
   WHERE x.org_id=m.org_id AND x.id=(NEW.result_data->>'id')::uuid AND x.project_id=m.project_id;
  IF actual IS NULL OR NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.version=m.base_version)
   OR (actual->>'version')::bigint<>1 OR NEW.result_version<>1 OR actual->>'state'<>'open'
   OR actual->>'creator_id'<>m.actor_id::text
   OR actual->>'title' IS DISTINCT FROM btrim(m.arguments->>'title')
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
