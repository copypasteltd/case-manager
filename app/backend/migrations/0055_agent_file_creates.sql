-- Extend the immutable single-file pipeline with an explicit create intent.
-- Existing update rows and receipts keep their exact representation.
ALTER TABLE agent_file_intents ADD COLUMN mode text NOT NULL DEFAULT 'update' CHECK(mode IN ('update','create'));
ALTER TABLE agent_file_intents ALTER COLUMN file_id DROP NOT NULL;
ALTER TABLE agent_file_intents ALTER COLUMN base_version_id DROP NOT NULL;
ALTER TABLE agent_file_intents DROP CONSTRAINT agent_file_intents_file_version_check;
ALTER TABLE agent_file_intents ADD CONSTRAINT agent_file_intent_mode_scope CHECK(
 (mode='update' AND file_id IS NOT NULL AND base_version_id IS NOT NULL AND file_version>0)
 OR (mode='create' AND file_id IS NULL AND base_version_id IS NULL AND file_version=0
  AND coalesce((target_data->>'folder_version')::bigint,0)>0));
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create'));

CREATE OR REPLACE FUNCTION validate_agent_file_update_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; i agent_file_intents; d agent_file_drafts; u upload_sessions;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF r.id IS NULL OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
  OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=r.org_id AND p.id=r.profile_id AND p.skill_id='file-update' AND p.state='active') THEN
  RAISE EXCEPTION 'file update requires a current exact runtime binding' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_file_intents' THEN
  IF r.state<>'queued' OR r.actor_id<>NEW.actor_id OR r.conversation_id<>NEW.conversation_id OR r.channel='telegram'
   OR (((NEW.intent::jsonb)->>'actor_auth_version')::bigint=r.actor_auth_version
     AND ((NEW.intent::jsonb)->>'organization_auth_version')::bigint=r.organization_auth_version
     AND (NEW.intent::jsonb)->>'conversation_revision_id'=r.conversation_revision_id::text
     AND ((NEW.intent::jsonb)->>'context_security_version')::bigint=r.context_security_version
     AND (NEW.intent::jsonb)->>'run_id'=r.id::text
     AND (NEW.intent::jsonb)->>'actor_id'=r.actor_id::text
     AND (NEW.intent::jsonb)->>'conversation_id'=r.conversation_id::text
     AND (NEW.intent::jsonb)->>'file_id'=coalesce(NEW.file_id::text,'')
     AND (NEW.intent::jsonb)->>'base_version_id'=coalesce(NEW.base_version_id::text,'')
     AND ((NEW.intent::jsonb)->>'file_version')::bigint=NEW.file_version
     AND (NEW.intent::jsonb)->>'base_sha256'=NEW.base_sha256
     AND (NEW.intent::jsonb)->'target'=NEW.target_data
     AND coalesce((NEW.intent::jsonb)->>'mode','update')=NEW.mode) IS NOT TRUE THEN
   RAISE EXCEPTION 'file intent does not match the sender and exact base' USING ERRCODE='23514';
  END IF;
  IF NEW.mode='create' AND NOT EXISTS(SELECT 1 FROM folders x WHERE x.org_id=NEW.org_id
    AND x.id::text=NEW.target_data->>'folder_id' AND x.version=(NEW.target_data->>'folder_version')::bigint
    AND x.classification=NEW.target_data->>'classification' AND x.state='active') THEN
   RAISE EXCEPTION 'creation intent requires its exact current directory' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF i.run_id IS NULL OR r.state<>'running' THEN
  RAISE EXCEPTION 'file tool requires the active authorized run' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_file_drafts' THEN RETURN NEW; END IF;
 SELECT * INTO d FROM agent_file_drafts WHERE org_id=NEW.org_id AND id=NEW.draft_id AND run_id=NEW.run_id;
 SELECT * INTO u FROM upload_sessions WHERE org_id=NEW.org_id AND id=NEW.draft_id;
 IF d.id IS NULL OR u.id IS NULL OR u.owner_id<>i.actor_id OR u.file_id IS DISTINCT FROM i.file_id OR u.base_version_id IS DISTINCT FROM i.base_version_id
  OR u.target_folder_id::text<>i.target_data->>'folder_id' OR u.name<>i.target_data->>'name'
  OR u.media_type<>i.target_data->>'media_type' OR u.classification IS DISTINCT FROM (i.target_data->>'classification')
  OR u.source_channel<>'agent' OR u.size_bytes<>d.size_bytes OR u.source_fingerprint<>d.content_sha256 THEN
  RAISE EXCEPTION 'draft is not bound to its exact reserved upload' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_file_stages' THEN
  IF u.state<>'finalizing' OR (NEW.source->>'Key'=u.draft_key AND (NEW.source->>'Size')::bigint=d.size_bytes
    AND NEW.source->>'UploadID'=u.provider_upload_id
    AND EXISTS(SELECT 1 FROM upload_parts p WHERE p.session_id=u.id AND p.part_no=1 AND p.size_bytes=d.size_bytes AND p.provider_etag=NEW.source->>'ETag')) IS NOT TRUE THEN
   RAISE EXCEPTION 'draft bytes require an exact staged provider source' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_proposals' THEN
  IF NOT EXISTS(SELECT 1 FROM agent_file_stages s WHERE s.org_id=d.org_id AND s.draft_id=d.id)
   OR ((NEW.plan::jsonb)->>'draft_id'=d.id::text AND (NEW.plan::jsonb)->>'content_sha256'=d.content_sha256
    AND (NEW.plan::jsonb)->>'intent_sha256'=i.intent_sha256 AND (NEW.plan::jsonb)->>'run_id'=r.id::text
    AND (NEW.plan::jsonb)->>'file_id'=coalesce(i.file_id::text,'') AND (NEW.plan::jsonb)->>'base_version_id'=coalesce(i.base_version_id::text,'')
    AND ((NEW.plan::jsonb)->>'file_version')::bigint=i.file_version AND ((NEW.plan::jsonb)->>'size_bytes')::bigint=d.size_bytes
    AND (NEW.plan::jsonb)->'diff'=d.diff AND (NEW.plan::jsonb)->'target'=i.target_data
    AND (NEW.plan::jsonb)->>'stage_sha256'=(SELECT encode(sha256(convert_to(s.source::text,'UTF8')),'hex') FROM agent_file_stages s WHERE s.org_id=d.org_id AND s.draft_id=d.id)) IS NOT TRUE THEN
   RAISE EXCEPTION 'proposal must fix the staged draft and sender intent' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_commits' THEN
  IF u.state<>'committed' OR u.committed_version_id<>NEW.version_id OR NOT EXISTS(
    SELECT 1 FROM file_versions v WHERE v.org_id=d.org_id AND v.id=NEW.version_id AND ((i.mode='update' AND v.file_id=i.file_id) OR (i.mode='create' AND i.file_id IS NULL AND v.seq=1
       AND v.file_id::text=NEW.result_data->'file'->>'id'))
     AND v.base_version_id IS NOT DISTINCT FROM i.base_version_id AND v.sha256=d.content_sha256 AND v.scan_state='clean' AND v.source='agent')
   OR NOT EXISTS(SELECT 1 FROM idempotency_records x WHERE x.id=NEW.operation_id AND x.org_id=d.org_id AND x.actor_id=i.actor_id
    AND x.state='succeeded' AND x.route_key=CASE i.mode WHEN 'create' THEN '/internal/agent-files/create' ELSE '/internal/agent-files/commit' END
    AND x.idempotency_key=(CASE i.mode WHEN 'create' THEN 'agent-file-create:' ELSE 'agent-file-update:' END)||r.id::text
    AND x.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'file commit requires the exact scanned formal business result' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;

CREATE OR REPLACE FUNCTION validate_agent_file_update_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_file_intents; d agent_file_drafts;
BEGIN
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_file_intents' THEN
  IF NOT EXISTS(SELECT 1 FROM conversation_messages x WHERE x.org_id=i.org_id AND x.run_id=i.run_id AND x.role='user'
    AND x.body=(i.intent::jsonb)->>'request'
    AND x.source_snapshots @> jsonb_build_array(jsonb_build_object('type',CASE i.mode WHEN 'create' THEN 'agent_file_create' ELSE 'agent_file_update' END,'id',i.run_id,'resource_version',1,'sha256',i.intent_sha256))) THEN
   RAISE EXCEPTION 'file intent must be fixed by the sender message' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_drafts' THEN
  SELECT * INTO d FROM agent_file_drafts WHERE org_id=NEW.org_id AND id=NEW.id;
  IF NOT EXISTS(SELECT 1 FROM upload_sessions u WHERE u.org_id=d.org_id AND u.id=d.id AND u.owner_id=i.actor_id
    AND u.file_id IS NOT DISTINCT FROM i.file_id AND u.base_version_id IS NOT DISTINCT FROM i.base_version_id AND u.source_channel='agent'
    AND u.target_folder_id::text=i.target_data->>'folder_id' AND u.name=i.target_data->>'name'
    AND u.media_type=i.target_data->>'media_type' AND u.classification IS NOT DISTINCT FROM (i.target_data->>'classification')
    AND u.size_bytes=d.size_bytes AND u.source_fingerprint=d.content_sha256) THEN
   RAISE EXCEPTION 'draft reservation requires its own isolated upload' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_commits' THEN
  IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts x WHERE x.org_id=NEW.org_id AND x.run_id=NEW.run_id AND x.tool=CASE i.mode WHEN 'create' THEN 'commit_file_create' ELSE 'commit_file_update' END
    AND x.output->>'committed'='true' AND x.output->>'operation_id'=NEW.operation_id::text) THEN
   RAISE EXCEPTION 'formal file commit requires its atomic tool receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
