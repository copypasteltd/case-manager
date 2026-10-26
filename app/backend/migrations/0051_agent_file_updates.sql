-- A sender authorizes one recoverable update to an exact current file version.
-- Draft edits are bounded business intent; full draft bytes live in the existing
-- isolated object namespace and follow the normal hash/scan/promotion pipeline.
ALTER TABLE upload_sessions DROP CONSTRAINT upload_sessions_source_channel_check;
ALTER TABLE upload_sessions ADD CONSTRAINT upload_sessions_source_channel_check CHECK(source_channel IN ('web','android','agent'));
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update'));

CREATE TABLE agent_file_intents (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, actor_id uuid NOT NULL, conversation_id uuid NOT NULL,
 file_id uuid NOT NULL, base_version_id uuid NOT NULL, file_version bigint NOT NULL CHECK(file_version>0),
 target_data jsonb NOT NULL CHECK(jsonb_typeof(target_data)='object'),
 base_sha256 text NOT NULL CHECK(base_sha256~'^[0-9a-f]{64}$'),
 intent text NOT NULL CHECK(octet_length(intent) BETWEEN 1 AND 100000),
 intent_sha256 text NOT NULL CHECK(intent_sha256=encode(sha256(convert_to(intent,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,file_id,base_version_id) REFERENCES file_versions(org_id,file_id,id)
);
CREATE TABLE agent_file_drafts (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL, invocation_key text NOT NULL CHECK(length(invocation_key) BETWEEN 1 AND 200),
 input_sha256 text NOT NULL CHECK(input_sha256~'^[0-9a-f]{64}$'),
 edits jsonb NOT NULL CHECK(jsonb_typeof(edits)='array' AND jsonb_array_length(edits) BETWEEN 1 AND 64 AND octet_length(edits::text)<=100000),
 content_sha256 text NOT NULL CHECK(content_sha256~'^[0-9a-f]{64}$'), size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 1048576),
 diff jsonb NOT NULL CHECK(jsonb_typeof(diff)='object' AND octet_length(diff::text)<=100000),
 source_fingerprint text NOT NULL CHECK(source_fingerprint~'^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,id), UNIQUE(org_id,run_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_file_intents(org_id,run_id),
 FOREIGN KEY(org_id,id) REFERENCES upload_sessions(org_id,id) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX agent_file_drafts_by_run ON agent_file_drafts(org_id,run_id,created_at,id);
CREATE TABLE agent_file_stages (
 draft_id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL,
 source jsonb NOT NULL CHECK(jsonb_typeof(source)='object' AND octet_length(source::text)<=10000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id,draft_id) REFERENCES agent_file_drafts(org_id,run_id,id)
);
CREATE TABLE agent_file_proposals (
 draft_id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL,
 plan text NOT NULL CHECK(octet_length(plan) BETWEEN 1 AND 100000),
 plan_hash text NOT NULL CHECK(plan_hash=encode(sha256(convert_to(plan,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,draft_id),
 FOREIGN KEY(org_id,run_id,draft_id) REFERENCES agent_file_drafts(org_id,run_id,id)
);
CREATE TABLE agent_file_commits (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, draft_id uuid NOT NULL UNIQUE, operation_id uuid NOT NULL UNIQUE,
 version_id uuid NOT NULL, result_data jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id,draft_id) REFERENCES agent_file_drafts(org_id,run_id,id),
 FOREIGN KEY(org_id,draft_id) REFERENCES agent_file_proposals(org_id,draft_id),
 FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id),
 FOREIGN KEY(operation_id) REFERENCES idempotency_records(id)
);

CREATE FUNCTION validate_agent_file_update_binding() RETURNS trigger LANGUAGE plpgsql AS $$
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
     AND (NEW.intent::jsonb)->>'file_id'=NEW.file_id::text
     AND (NEW.intent::jsonb)->>'base_version_id'=NEW.base_version_id::text
     AND ((NEW.intent::jsonb)->>'file_version')::bigint=NEW.file_version
     AND (NEW.intent::jsonb)->>'base_sha256'=NEW.base_sha256
     AND (NEW.intent::jsonb)->'target'=NEW.target_data) IS NOT TRUE THEN
   RAISE EXCEPTION 'file intent does not match the sender and exact base' USING ERRCODE='23514';
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
 IF d.id IS NULL OR u.id IS NULL OR u.owner_id<>i.actor_id OR u.file_id<>i.file_id OR u.base_version_id<>i.base_version_id
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
    AND (NEW.plan::jsonb)->>'file_id'=i.file_id::text AND (NEW.plan::jsonb)->>'base_version_id'=i.base_version_id::text
    AND ((NEW.plan::jsonb)->>'file_version')::bigint=i.file_version AND ((NEW.plan::jsonb)->>'size_bytes')::bigint=d.size_bytes
    AND (NEW.plan::jsonb)->'diff'=d.diff AND (NEW.plan::jsonb)->'target'=i.target_data
    AND (NEW.plan::jsonb)->>'stage_sha256'=(SELECT encode(sha256(convert_to(s.source::text,'UTF8')),'hex') FROM agent_file_stages s WHERE s.org_id=d.org_id AND s.draft_id=d.id)) IS NOT TRUE THEN
   RAISE EXCEPTION 'proposal must fix the staged draft and sender intent' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_commits' THEN
  IF u.state<>'committed' OR u.committed_version_id<>NEW.version_id OR NOT EXISTS(
    SELECT 1 FROM file_versions v WHERE v.org_id=d.org_id AND v.id=NEW.version_id AND v.file_id=i.file_id
     AND v.base_version_id=i.base_version_id AND v.sha256=d.content_sha256 AND v.scan_state='clean' AND v.source='agent')
   OR NOT EXISTS(SELECT 1 FROM idempotency_records x WHERE x.id=NEW.operation_id AND x.org_id=d.org_id AND x.actor_id=i.actor_id
    AND x.state='succeeded' AND x.route_key='/internal/agent-files/commit' AND x.idempotency_key='agent-file-update:'||r.id::text
    AND x.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'file commit requires the exact scanned formal business result' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;

CREATE FUNCTION validate_agent_file_update_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_file_intents; d agent_file_drafts;
BEGIN
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_file_intents' THEN
  IF NOT EXISTS(SELECT 1 FROM conversation_messages x WHERE x.org_id=i.org_id AND x.run_id=i.run_id AND x.role='user'
    AND x.body=(i.intent::jsonb)->>'request'
    AND x.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_file_update','id',i.run_id,'resource_version',1,'sha256',i.intent_sha256))) THEN
   RAISE EXCEPTION 'file intent must be fixed by the sender message' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_drafts' THEN
  SELECT * INTO d FROM agent_file_drafts WHERE org_id=NEW.org_id AND id=NEW.id;
  IF NOT EXISTS(SELECT 1 FROM upload_sessions u WHERE u.org_id=d.org_id AND u.id=d.id AND u.owner_id=i.actor_id
    AND u.file_id=i.file_id AND u.base_version_id=i.base_version_id AND u.source_channel='agent'
    AND u.size_bytes=d.size_bytes AND u.source_fingerprint=d.content_sha256) THEN
   RAISE EXCEPTION 'draft reservation requires its own isolated upload' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_file_commits' THEN
  IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts x WHERE x.org_id=NEW.org_id AND x.run_id=NEW.run_id AND x.tool='commit_file_update'
    AND x.output->>'committed'='true' AND x.output->>'operation_id'=NEW.operation_id::text) THEN
   RAISE EXCEPTION 'formal file commit requires its atomic tool receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['agent_file_intents','agent_file_drafts','agent_file_stages','agent_file_proposals','agent_file_commits'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()',t||'_immutable',t);
  EXECUTE format('CREATE TRIGGER %I BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION validate_agent_file_update_binding()',t||'_binding',t);
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
   EXECUTE format('GRANT SELECT,INSERT ON %I TO copypaste_app',t);
  END IF;
 END LOOP;
END; $$;
CREATE CONSTRAINT TRIGGER agent_file_intent_evidence AFTER INSERT ON agent_file_intents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_file_update_evidence();
CREATE CONSTRAINT TRIGGER agent_file_draft_upload_evidence AFTER INSERT ON agent_file_drafts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_file_update_evidence();
CREATE CONSTRAINT TRIGGER agent_file_commit_tool_evidence AFTER INSERT ON agent_file_commits DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_file_update_evidence();
