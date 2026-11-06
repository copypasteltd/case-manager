-- Explicit one-use fixed task-field TG grants. No policy or runtime is auto-enabled.
ALTER TABLE agent_runtime_profiles ADD COLUMN telegram_write_configuration_sha256 text
 CHECK(telegram_write_configuration_sha256 IS NULL OR (skill_id='project-update' AND telegram_write_configuration_sha256 ~ '^[a-f0-9]{64}$'));
CREATE TABLE telegram_write_grants (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,run_id uuid NOT NULL UNIQUE,conversation_id uuid NOT NULL,actor_id uuid NOT NULL,
 actor_auth_version bigint NOT NULL,organization_auth_version bigint NOT NULL,session_id uuid NOT NULL,
 origin_channel text NOT NULL CHECK(origin_channel IN ('web','android','wechat','douyin')),channel_id uuid NOT NULL,channel_version bigint NOT NULL,
 binding_id uuid NOT NULL,binding_version bigint NOT NULL CHECK(binding_version>0),profile_id uuid NOT NULL,configuration_sha256 text NOT NULL CHECK(configuration_sha256 ~ '^[a-f0-9]{64}$'),
 input text NOT NULL CHECK(jsonb_typeof(input::jsonb)='object' AND octet_length(input)<=524288),
 input_sha256 text NOT NULL CHECK(input_sha256=encode(sha256(convert_to(input,'UTF8')),'hex')),reason text NOT NULL,
 expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,id),UNIQUE(org_id,run_id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '10 minutes'),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,channel_id) REFERENCES telegram_channels(org_id,id),FOREIGN KEY(org_id,binding_id) REFERENCES telegram_bindings(org_id,id),
 FOREIGN KEY(org_id,profile_id) REFERENCES agent_runtime_profiles(org_id,id)
);
CREATE TABLE telegram_write_activations (
 org_id uuid NOT NULL,grant_id uuid PRIMARY KEY,inbox_id uuid NOT NULL UNIQUE,run_id uuid NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,grant_id) REFERENCES telegram_write_grants(org_id,id),FOREIGN KEY(org_id,inbox_id) REFERENCES telegram_inbox(org_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES telegram_write_grants(org_id,run_id)
);
CREATE TRIGGER telegram_write_grant_fixed BEFORE UPDATE OR DELETE ON telegram_write_grants FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER telegram_write_activation_fixed BEFORE UPDATE OR DELETE ON telegram_write_activations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION agent_original_channel(org uuid,run uuid,actual text) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN actual='telegram' THEN (SELECT origin_channel FROM telegram_write_grants WHERE org_id=org AND run_id=run) ELSE actual END
$$;
CREATE FUNCTION validate_telegram_write_grant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE g telegram_write_grants;r agent_runs;m agent_run_mutations;
BEGIN
 IF TG_TABLE_NAME='telegram_write_grants' THEN g:=NEW;
 ELSE SELECT * INTO g FROM telegram_write_grants WHERE org_id=NEW.org_id AND id=NEW.grant_id;END IF;
 SELECT * INTO r FROM agent_runs WHERE org_id=g.org_id AND id=g.run_id;
 SELECT * INTO m FROM agent_run_mutations WHERE org_id=g.org_id AND run_id=g.run_id;
 IF r.id IS NULL OR r.channel<>'telegram' OR r.actor_id<>g.actor_id OR r.conversation_id<>g.conversation_id OR r.profile_id<>g.profile_id
 OR r.actor_auth_version<>g.actor_auth_version OR r.organization_auth_version<>g.organization_auth_version
 OR m.tool IS DISTINCT FROM 'update_task' OR m.session_id IS DISTINCT FROM g.session_id
 OR (g.input::jsonb->'run'->'mutation'->'arguments') IS DISTINCT FROM m.arguments
 OR (g.input::jsonb->'run'->>'client_message_id') IS DISTINCT FROM r.client_message_id::text
 OR (g.input::jsonb->>'channel_id') IS DISTINCT FROM g.channel_id::text OR (g.input::jsonb->>'binding_id') IS DISTINCT FROM g.binding_id::text
 OR (g.input::jsonb->>'binding_version')::bigint IS DISTINCT FROM g.binding_version OR (g.input::jsonb->>'channel_version')::bigint IS DISTINCT FROM g.channel_version
  OR NOT EXISTS(SELECT 1 FROM sessions s WHERE s.org_id=g.org_id AND s.id=g.session_id AND s.user_id=g.actor_id AND s.device_type=g.origin_channel)
 THEN RAISE EXCEPTION 'TG grant requires an exact fixed task instruction and original human session' USING ERRCODE='23514';END IF;
 IF TG_TABLE_NAME='telegram_write_activations' THEN
 IF (NEW.run_id<>g.run_id OR g.expires_at<=clock_timestamp() OR r.state<>'queued' OR r.cancel_requested_at IS NOT NULL
 OR NOT EXISTS(SELECT 1 FROM telegram_inbox i JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id
 JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id
 AND i.requester_id=g.actor_id AND i.channel_id=g.channel_id AND i.channel_version=g.channel_version AND i.binding_id=g.binding_id
 AND i.actor_auth_version=g.actor_auth_version AND i.state='dispatched' AND ch.state='active' AND ch.version=g.channel_version
 AND ch.bot_id=i.bot_id AND ch.chat_id=i.chat_id AND ch.topic_id=i.topic_id AND b.state='active' AND b.telegram_user_id=i.telegram_user_id
 AND b.version=g.binding_version AND b.user_id=g.actor_id AND b.bot_id=ch.bot_id AND i.requested_skill_id='project-update'
 AND i.request_text ~ ('^/task(@[A-Za-z0-9_]+)?[[:space:]]+project-update[[:space:]]+authorize:'||g.id::text||'[[:space:]]*$'))) THEN
 RAISE EXCEPTION 'TG grant trigger is stale, reused or differs from its owner/group/input' USING ERRCODE='23514';END IF;
 END IF;
 RETURN NEW;
END;$$;
CREATE CONSTRAINT TRIGGER telegram_write_grant_binding AFTER INSERT ON telegram_write_grants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_write_grant();
CREATE CONSTRAINT TRIGGER telegram_write_activation_binding AFTER INSERT ON telegram_write_activations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_write_grant();
CREATE FUNCTION guard_telegram_write_job() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM telegram_write_grants g WHERE g.org_id=NEW.org_id AND g.run_id=NEW.run_id
 AND NOT EXISTS(SELECT 1 FROM telegram_write_activations a WHERE a.org_id=g.org_id AND a.grant_id=g.id)) THEN
 RAISE EXCEPTION 'a TG write grant cannot enqueue before its fixed trigger' USING ERRCODE='23514';END IF;RETURN NEW;
END;$$;
CREATE TRIGGER telegram_write_job_gate BEFORE INSERT ON agent_run_jobs FOR EACH ROW EXECUTE FUNCTION guard_telegram_write_job();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON telegram_write_grants,telegram_write_activations TO copypaste_app;
 REVOKE UPDATE,DELETE ON telegram_write_grants,telegram_write_activations FROM copypaste_app;
END IF;END;$$;

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
   OR (r.channel='telegram' AND NOT EXISTS(SELECT 1 FROM telegram_write_grants g WHERE g.org_id=r.org_id AND g.run_id=r.id AND g.session_id=NEW.session_id AND NEW.tool='update_task')) OR r.cancel_requested_at IS NOT NULL OR NOT EXISTS(
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
  OR agent_original_channel(r.org_id,r.id,r.channel) NOT IN ('web','android','wechat','douyin') OR NOT EXISTS(
   SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
   WHERE s.org_id=m.org_id AND s.id=m.session_id AND s.user_id=m.actor_id AND s.device_type=agent_original_channel(r.org_id,r.id,r.channel)
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
   AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version AND o.authorization_version=r.organization_auth_version) THEN
  RAISE EXCEPTION 'project delegation requires its original live interactive session' USING ERRCODE='23514';
 END IF;

 -- An exact delegated submission creates a new fixed review revision. It
 -- cannot modify commercial/project metadata, old acceptance or file bytes.
 IF m.tool='submit_deliverable' THEN
  required_fields:=ARRAY['id','expected_version','expected_revision_id','file_version_ids','description','reason'];
  IF NOT (m.arguments ?& required_fields) OR (SELECT count(*) FROM jsonb_object_keys(m.arguments))<>6
   OR jsonb_typeof(m.arguments->'file_version_ids') IS DISTINCT FROM 'array'
   OR jsonb_typeof(m.arguments->'description') IS DISTINCT FROM 'string'
   OR jsonb_typeof(m.arguments->'reason') IS DISTINCT FROM 'string'
   OR jsonb_array_length(m.arguments->'file_version_ids') NOT BETWEEN 1 AND 20
   OR (m.arguments->>'id'=m.resource_id::text AND (m.arguments->>'expected_version')::bigint=m.base_version
    AND length(btrim(m.arguments->>'description',trim_chars))>0 AND length(m.arguments->>'description')<=20000
    AND length(btrim(m.arguments->>'reason',trim_chars)) BETWEEN 1 AND 200) IS NOT TRUE
   OR cardinality(r.audience_ids)<1 OR NOT (m.actor_id=ANY(r.audience_ids)) THEN
   RAISE EXCEPTION 'submission requires complete exact reviewed fields and original audience' USING ERRCODE='23514';
  END IF;
  SELECT to_jsonb(d) INTO actual FROM deliverables d WHERE d.org_id=m.org_id AND d.id=m.resource_id AND d.project_id=m.project_id;
  IF actual IS NULL OR actual->>'protocol_version'<>'1' OR NOT EXISTS(SELECT 1 FROM projects p WHERE p.org_id=m.org_id AND p.id=m.project_id AND p.state IN('draft','active','on_hold'))
   OR (m.before_data->>'id'=m.resource_id::text AND (m.before_data->>'version')::bigint=m.base_version
    AND m.before_data->>'project_id'=m.project_id::text AND m.before_data->>'protocol_version'='1'
    AND m.before_data->>'state' IN('draft','changes_requested','accepted')) IS NOT TRUE
   OR m.before_data->'current_revision_id' IS DISTINCT FROM m.arguments->'expected_revision_id' THEN
   RAISE EXCEPTION 'submission requires the exact writable delivery base' USING ERRCODE='23514';
  END IF;
  FOREACH field IN ARRAY ARRAY['name','acceptance_criteria','owner_id','milestone_id'] LOOP
   IF actual->field IS DISTINCT FROM m.before_data->field THEN
    RAISE EXCEPTION 'submission cannot modify delivery metadata' USING ERRCODE='23514';
   END IF;
  END LOOP;
  IF (SELECT count(DISTINCT value::uuid) FROM jsonb_array_elements_text(m.arguments->'file_version_ids'))<>jsonb_array_length(m.arguments->'file_version_ids')
   OR NOT deliverable_files_available(m.org_id,m.project_id,(SELECT classification FROM projects WHERE org_id=m.org_id AND id=m.project_id),
     ARRAY(SELECT value::uuid FROM jsonb_array_elements_text(m.arguments->'file_version_ids'))) THEN
   RAISE EXCEPTION 'submission attachments require exact distinct available business evidence' USING ERRCODE='23514';
  END IF;
  SELECT jsonb_agg(jsonb_build_object('file_id',f.id,'version_id',v.id,'seq',v.seq,'name',v.original_name,'sha256',v.sha256,
    'size_bytes',v.size_bytes::text,'media_type',v.media_type,'scan_state',v.scan_state,'scan_waiver_ref',v.scan_waiver_ref) ORDER BY selected.ordinal)
   INTO expected FROM jsonb_array_elements_text(m.arguments->'file_version_ids') WITH ORDINALITY selected(id,ordinal)
   JOIN file_versions v ON v.org_id=m.org_id AND v.id=selected.id::uuid JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id;
  IF expected IS DISTINCT FROM m.before_data->'submission_files' OR EXISTS(
   SELECT 1 FROM jsonb_array_elements_text(m.arguments->'file_version_ids') selected(id)
   JOIN file_versions v ON v.org_id=m.org_id AND v.id=selected.id::uuid JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
   JOIN folders folder ON folder.org_id=f.org_id AND folder.id=f.folder_id CROSS JOIN unnest(r.audience_ids) audience(uid)
   WHERE can_access(m.org_id,audience.uid,'file.read','file',f.id,folder.project_id,f.creator_id,f.classification,r.channel,true) IS NOT TRUE) THEN
   RAISE EXCEPTION 'submission attachment snapshot or current audience changed' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='agent_run_mutations' THEN
   IF (actual->>'version')::bigint<>m.base_version OR actual->'current_revision_id' IS DISTINCT FROM m.arguments->'expected_revision_id'
    OR actual->>'state' IS DISTINCT FROM m.before_data->>'state' THEN
    RAISE EXCEPTION 'submission instruction is stale' USING ERRCODE='23514';
   END IF;
  ELSE
   SELECT to_jsonb(rev) INTO child FROM deliverable_revisions rev WHERE rev.org_id=m.org_id AND rev.deliverable_id=m.resource_id AND rev.id=(actual->>'current_revision_id')::uuid;
   IF child IS NULL OR actual->>'state'<>'submitted' OR (actual->>'version')::bigint<>m.base_version+1
    OR NEW.result_version<>m.base_version+1 OR (child->>'head_version')::bigint<>NEW.result_version
    OR child->>'submitted_by'<>m.actor_id::text OR child->>'session_id'<>m.session_id::text
    OR child->'file_version_ids' IS DISTINCT FROM m.arguments->'file_version_ids'
    OR child->>'description' IS DISTINCT FROM btrim(m.arguments->>'description',trim_chars)
    OR child->>'reason' IS DISTINCT FROM m.arguments->>'reason'
    OR child->>'id' IS NOT DISTINCT FROM m.arguments->>'expected_revision_id'
    OR (NEW.result_data->>'id'=m.resource_id::text AND (NEW.result_data->>'version')::bigint=NEW.result_version
     AND NEW.result_data->>'revision_id'=child->>'id') IS NOT TRUE THEN
    RAISE EXCEPTION 'submission receipt must bind the actual new fixed revision' USING ERRCODE='23514';
   END IF;
  END IF;
  RETURN NEW;
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
CREATE OR REPLACE FUNCTION validate_telegram_run_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.channel='telegram' AND EXISTS(SELECT 1 FROM telegram_write_grants g WHERE g.org_id=NEW.org_id AND g.run_id=NEW.id AND g.actor_id=NEW.actor_id AND g.conversation_id=NEW.conversation_id AND g.profile_id=NEW.profile_id AND g.actor_auth_version=NEW.actor_auth_version AND g.organization_auth_version=NEW.organization_auth_version) THEN RETURN NEW;END IF;
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
CREATE OR REPLACE FUNCTION validate_classification_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE previous classification_policies%ROWTYPE; normalized text[];
BEGIN
    -- Serialize policy publication with every other authorization mutation.
    PERFORM 1 FROM organizations WHERE id=NEW.org_id FOR UPDATE;
    SELECT * INTO previous FROM classification_policies
        WHERE org_id=NEW.org_id AND level=NEW.level ORDER BY policy_version DESC LIMIT 1;
    IF previous.id IS NULL THEN
        IF NEW.policy_version<>1 OR NEW.base_policy_id IS NOT NULL THEN
            RAISE EXCEPTION 'initial classification policy requires version one' USING ERRCODE='23514';
        END IF;
    ELSE
        IF NEW.policy_version<>previous.policy_version+1 THEN
            RAISE EXCEPTION 'classification policy sequence must be contiguous' USING ERRCODE='23514';
        END IF;
        -- Controlled/legacy callers may omit this field. All new API
        -- publications provide it; a supplied predecessor must always be exact.
        IF NEW.base_policy_id IS NOT NULL AND NEW.base_policy_id<>previous.id THEN
            RAISE EXCEPTION 'classification policy predecessor changed' USING ERRCODE='23514';
        END IF;
    END IF;
    SELECT coalesce(array_agg(DISTINCT item ORDER BY item),ARRAY[]::text[]) INTO normalized
        FROM unnest(NEW.allowed_channels) AS channel_rows(item);
    IF array_position(NEW.allowed_channels,NULL) IS NOT NULL
        OR cardinality(NEW.allowed_channels)<>cardinality(normalized)
        OR NOT NEW.allowed_channels<@ARRAY['android','douyin','telegram','web','wechat']::text[] THEN
        RAISE EXCEPTION 'classification channels must be a unique canonical set' USING ERRCODE='23514';
    END IF;
    NEW.allowed_channels=normalized;
    IF current_user='copypaste_app' AND NEW.policy_version>1
        AND (NEW.published_by IS NULL OR NEW.reason IS NULL OR NEW.base_policy_id IS NULL) THEN
        RAISE EXCEPTION 'application publication requires exact provenance' USING ERRCODE='23514';
    END IF;
    IF NEW.published_by IS NOT NULL AND (
        (NEW.level='internal' AND NOT 'web'=ANY(NEW.allowed_channels))
        OR (NEW.level='confidential' AND NEW.ai_allowed)) THEN
        RAISE EXCEPTION 'publication exceeds available classification or channel admission' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

-- Read projection only: the original read-only link receipts remain immutable.
CREATE VIEW telegram_effective_run_links AS
 SELECT org_id,inbox_id,run_id,conversation_id FROM telegram_run_links
 UNION ALL SELECT a.org_id,a.inbox_id,a.run_id,r.conversation_id FROM telegram_write_activations a JOIN agent_runs r ON r.org_id=a.org_id AND r.id=a.run_id;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN GRANT SELECT ON telegram_effective_run_links TO copypaste_app;END IF;END;$$;

CREATE OR REPLACE FUNCTION preserve_agent_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM organizations WHERE id=OLD.org_id FOR UPDATE;
 IF NEW.id<>OLD.id OR NEW.org_id<>OLD.org_id OR NEW.skill_id<>OLD.skill_id OR NEW.skill_version<>OLD.skill_version
 OR NEW.skill_sha256<>OLD.skill_sha256 OR NEW.runtime_version<>OLD.runtime_version OR NEW.configuration_sha256<>OLD.configuration_sha256
 OR NEW.telegram_write_configuration_sha256 IS DISTINCT FROM OLD.telegram_write_configuration_sha256 OR NEW.telegram_configuration_sha256 IS DISTINCT FROM OLD.telegram_configuration_sha256
 OR NEW.max_input_tokens<>OLD.max_input_tokens OR NEW.max_output_tokens<>OLD.max_output_tokens OR NEW.max_tool_calls<>OLD.max_tool_calls
 OR NEW.timeout_seconds<>OLD.timeout_seconds OR NEW.installed_by<>OLD.installed_by OR NEW.installation_reason<>OLD.installation_reason
 OR NEW.created_at<>OLD.created_at OR (OLD.state='retired' AND NEW.state<>'retired') THEN
  RAISE EXCEPTION 'runtime profiles are immutable and retired profiles cannot be reactivated' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;

-- Immutable activations and legacy read-only links are equally exact dispatch receipts.
CREATE OR REPLACE FUNCTION guard_telegram_inbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Telegram inbox cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'state'-'reason_code'-'dispatch_after') IS DISTINCT FROM (to_jsonb(OLD)-'state'-'reason_code'-'dispatch_after')
   OR NEW.dispatch_after<OLD.dispatch_after OR NOT (
    (OLD.state='awaiting_executor' AND NEW.state IN ('awaiting_executor','dispatched','withdrawn'))
    OR (OLD.state='dispatched' AND NEW.state='withdrawn' AND EXISTS(SELECT 1 FROM telegram_effective_run_links l
      JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
      WHERE l.org_id=OLD.org_id AND l.inbox_id=OLD.id AND r.state IN ('queued','running','awaiting_approval','committing')))) THEN
   RAISE EXCEPTION 'Telegram receipt content and identity are immutable' USING ERRCODE='23514';
  END IF;
  IF NEW.state='dispatched' AND NOT EXISTS(SELECT 1 FROM telegram_effective_run_links l WHERE l.org_id=NEW.org_id AND l.inbox_id=NEW.id) THEN
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
