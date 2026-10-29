-- Exact saved bytes may be restaged only by a new explicit human request.
-- Old runs, uploads, plans, commits and receipts are never rewritten or reused.
CREATE TABLE agent_file_recoveries (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, source_run_id uuid NOT NULL, source_draft_id uuid NOT NULL,
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 recovery_body text NOT NULL CHECK(octet_length(recovery_body) BETWEEN 1 AND 20000 AND jsonb_typeof(recovery_body::jsonb)='object'),
 recovery_sha256 text NOT NULL CHECK(recovery_sha256=encode(sha256(convert_to(recovery_body,'UTF8')),'hex')),
 depth integer NOT NULL CHECK(depth BETWEEN 1 AND 8), reviewed boolean NOT NULL CHECK(reviewed),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_file_intents(org_id,run_id),
 FOREIGN KEY(org_id,source_run_id,source_draft_id) REFERENCES agent_file_drafts(org_id,run_id,id),
 CHECK(run_id<>source_run_id)
);
CREATE INDEX agent_file_recovery_origin ON agent_file_recoveries(org_id,source_run_id,source_draft_id);
CREATE TRIGGER agent_file_recovery_immutable BEFORE UPDATE OR DELETE ON agent_file_recoveries
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_agent_file_recovery_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_file_intents; original agent_file_intents; r agent_runs; previous agent_runs;
 d agent_file_drafts; u upload_sessions; body jsonb; parent_depth integer;
BEGIN
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 SELECT * INTO original FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.source_run_id;
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 SELECT * INTO previous FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.source_run_id FOR SHARE;
 SELECT * INTO d FROM agent_file_drafts WHERE org_id=NEW.org_id AND run_id=NEW.source_run_id AND id=NEW.source_draft_id;
 SELECT * INTO u FROM upload_sessions WHERE org_id=NEW.org_id AND id=NEW.source_draft_id;
 SELECT depth INTO parent_depth FROM agent_file_recoveries WHERE org_id=NEW.org_id AND run_id=NEW.source_run_id;
 body := NEW.recovery_body::jsonb;
 IF i.run_id IS NULL OR original.run_id IS NULL OR r.id IS NULL OR previous.id IS NULL OR d.id IS NULL OR u.id IS NULL
  OR r.state<>'queued' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
  OR r.channel NOT IN ('web','android','wechat','douyin') OR r.actor_id<>previous.actor_id
  OR r.conversation_id<>previous.conversation_id OR r.conversation_revision_id<>previous.conversation_revision_id
  OR r.context_security_version<>previous.context_security_version OR r.audience_ids<>ARRAY[r.actor_id] OR previous.audience_ids<>r.audience_ids
  OR previous.state NOT IN ('succeeded','failed','cancelled')
  OR EXISTS(SELECT 1 FROM agent_file_commits WHERE org_id=NEW.org_id AND run_id=NEW.source_run_id)
  OR u.state IN ('committed','blocked') OR u.scan_state IN ('quarantined','blocked','infected','unscannable','waived')
  OR u.source_channel<>'agent' OR u.owner_id<>r.actor_id OR d.content_sha256<>NEW.content_sha256
  OR i.actor_id<>original.actor_id OR i.mode<>original.mode OR i.file_id IS DISTINCT FROM original.file_id
  OR i.base_version_id IS DISTINCT FROM original.base_version_id OR i.file_version<>original.file_version
  OR i.target_data<>original.target_data OR i.base_sha256<>original.base_sha256
  OR NEW.depth<>coalesce(parent_depth,0)+1
  OR (body->>'source_run_id'=NEW.source_run_id::text AND body->>'draft_id'=NEW.source_draft_id::text
   AND body->>'content_sha256'=NEW.content_sha256 AND body->>'source_intent_sha256'=original.intent_sha256
   AND (body->>'depth')::integer=NEW.depth AND jsonb_typeof(body->'sources')='array'
   AND jsonb_array_length(body->'sources')<=20
   AND (i.intent::jsonb)->'recovery'=jsonb_build_object('source_run_id',NEW.source_run_id,'draft_id',NEW.source_draft_id,
    'content_sha256',NEW.content_sha256,'recovery_sha256',NEW.recovery_sha256,'depth',NEW.depth)) IS NOT TRUE
  OR EXISTS(SELECT 1 FROM jsonb_array_elements(body->'sources') x WHERE x->>'type' IN ('agent_file_update','agent_file_create'))
  OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
   WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=r.actor_id
   AND c.current_revision_id=r.conversation_revision_id AND cr.context_security_version=r.context_security_version AND cr.state='active')
  OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users usr ON usr.org_id=s.org_id AND usr.id=s.user_id JOIN organizations o ON o.id=s.org_id
   WHERE s.org_id=r.org_id AND s.id::text=body->>'session_id' AND s.user_id=r.actor_id AND s.device_type=r.channel
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND usr.status='active'
   AND s.auth_version=usr.auth_version AND usr.auth_version=r.actor_auth_version AND o.authorization_version=r.organization_auth_version) THEN
  RAISE EXCEPTION 'file recovery requires its exact stopped private source and new interactive authority' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_file_recovery_binding BEFORE INSERT ON agent_file_recoveries
 FOR EACH ROW EXECUTE FUNCTION validate_agent_file_recovery_binding();

CREATE FUNCTION validate_agent_file_recovery_draft() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE recovery agent_file_recoveries; original agent_file_drafts;
BEGIN
 SELECT * INTO recovery FROM agent_file_recoveries WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF recovery.run_id IS NOT NULL THEN
  SELECT * INTO original FROM agent_file_drafts WHERE org_id=NEW.org_id AND run_id=recovery.source_run_id AND id=recovery.source_draft_id;
  IF original.id IS NULL OR NEW.edits<>original.edits OR NEW.content_sha256<>original.content_sha256 OR NEW.size_bytes<>original.size_bytes
   OR NEW.diff<>original.diff OR NEW.source_fingerprint<>original.source_fingerprint THEN
   RAISE EXCEPTION 'recovered draft must reconstruct the exact selected saved bytes and diff' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_file_recovery_draft BEFORE INSERT ON agent_file_drafts
 FOR EACH ROW EXECUTE FUNCTION validate_agent_file_recovery_draft();

CREATE FUNCTION validate_agent_file_recovery_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_file_intents; recovery agent_file_recoveries;
BEGIN
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 SELECT * INTO recovery FROM agent_file_recoveries WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_file_intents' THEN
  IF ((i.intent::jsonb) ? 'recovery') IS DISTINCT FROM (recovery.run_id IS NOT NULL) THEN
   RAISE EXCEPTION 'file recovery intent requires its immutable exact provenance' USING ERRCODE='23514';
  END IF;
 ELSIF recovery.run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_tool_receipts t
  WHERE t.org_id=NEW.org_id AND t.run_id=NEW.run_id AND t.tool='recover_file_draft'
  AND t.output->>'committed'='false' AND t.output->>'status'='succeeded'
  AND t.output->'content'->'recovery'->>'recovery_sha256'=recovery.recovery_sha256
  AND t.output->'resource_refs' @> jsonb_build_array(jsonb_build_object('type','agent_file_draft','id',NEW.draft_id))) THEN
  RAISE EXCEPTION 'recovered stage requires its atomic new-run tool receipt' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER agent_file_recovery_intent_evidence AFTER INSERT ON agent_file_intents
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_file_recovery_evidence();
CREATE CONSTRAINT TRIGGER agent_file_recovery_stage_evidence AFTER INSERT ON agent_file_stages
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_file_recovery_evidence();

ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON agent_file_recoveries TO copypaste_app;
 END IF;
END; $$;
