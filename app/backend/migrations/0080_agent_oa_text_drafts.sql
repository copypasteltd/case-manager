-- Prose suggestions are immutable; only an authenticated human can adopt them.
ALTER TABLE agent_oa_intents DROP CONSTRAINT agent_oa_intents_tool_check;
ALTER TABLE agent_oa_intents ADD CONSTRAINT agent_oa_intents_tool_check CHECK(tool IN ('draft_request','submit_request','draft_business_text'));
ALTER TABLE agent_oa_intents ADD CONSTRAINT agent_oa_text_no_request CHECK(tool<>'draft_business_text' OR request_id IS NULL);
CREATE OR REPLACE FUNCTION validate_agent_oa_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE x agent_runs; i agent_oa_intents; q requests;
BEGIN
 SELECT * INTO x FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF x.id IS NULL OR x.cancel_requested_at IS NOT NULL OR x.expires_at<=clock_timestamp()
  OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=x.org_id AND p.id=x.profile_id AND p.state='active' AND p.skill_id='oa-assist') THEN
  RAISE EXCEPTION 'OA intent requires its active exact runtime' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_oa_intents' THEN
  i := NEW;
  IF x.state<>'queued' OR x.actor_id<>i.actor_id OR x.conversation_id<>i.conversation_id
   OR x.channel NOT IN ('web','android','wechat','douyin')
   OR (((i.intent::jsonb)->>'actor_auth_version')::bigint=x.actor_auth_version
    AND ((i.intent::jsonb)->>'organization_auth_version')::bigint=x.organization_auth_version
    AND (i.intent::jsonb)->>'conversation_revision_id'=x.conversation_revision_id::text
    AND ((i.intent::jsonb)->>'context_security_version')::bigint=x.context_security_version
    AND i.before_data->'business'->>'id'=i.business_id::text
    AND i.before_data->'business'->>'kind'=i.kind) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=x.org_id AND c.id=x.conversation_id AND c.type='private' AND c.creator_id=i.actor_id AND cr.state='active'
    AND c.current_revision_id=x.conversation_revision_id AND cr.context_security_version=x.context_security_version
    AND c.project_id IS NOT DISTINCT FROM nullif(i.before_data->'business'->>'project_id','')::uuid
    AND classification_rank(i.before_data->'business'->>'classification')<=classification_rank(c.classification)) THEN
   RAISE EXCEPTION 'OA intent does not match the original interactive sender and scope' USING ERRCODE='23514';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM (
    SELECT org_id,id,'contract'::text kind,version,current_revision_id FROM contracts WHERE deleted_at IS NULL
    UNION ALL SELECT org_id,id,kind,version,current_revision_id FROM spend_documents WHERE deleted_at IS NULL) b
   WHERE b.org_id=i.org_id AND b.id=i.business_id AND b.kind=i.kind
   AND b.version=(i.before_data->'business'->>'version')::bigint
   AND b.current_revision_id::text=i.before_data->'business'->>'current_revision_id') THEN
   RAISE EXCEPTION 'OA intent requires the exact current business revision' USING ERRCODE='23514';
  END IF;
  IF i.tool IN ('draft_request','draft_business_text') THEN
   IF (i.arguments->>'kind'=i.kind AND i.arguments->>'business_id'=i.business_id::text
    AND (i.arguments->>'expected_business_version')::bigint=(i.before_data->'business'->>'version')::bigint
    AND i.arguments->>'business_revision_id'=i.before_data->'business'->>'current_revision_id'
    AND i.arguments->>'business_content_hash'=i.before_data->'business'->>'content_hash') IS NOT TRUE THEN
    RAISE EXCEPTION 'OA draft arguments differ from their exact business basis' USING ERRCODE='23514';
   END IF;
   IF i.request_id IS NOT NULL AND (i.arguments->>'request_id'=i.request_id::text
    AND (i.arguments->>'expected_version')::bigint=(i.before_data->'request'->>'version')::bigint) IS NOT TRUE THEN
    RAISE EXCEPTION 'OA draft revision requires its exact original request' USING ERRCODE='23514';
   END IF;
  ELSE
   IF (i.arguments->>'request_id'=i.request_id::text AND i.before_data->'request'->>'id'=i.request_id::text
    AND (i.arguments->>'expected_version')::bigint=(i.before_data->'request'->>'version')::bigint
    AND i.arguments->>'revision_id'=i.before_data->'request'->>'current_revision_id'
    AND i.arguments->>'content_hash'=i.before_data->'request'->>'content_hash'
    AND i.arguments->>'workflow_definition_id'=i.before_data->'preview'->'workflow'->>'id'
    AND i.arguments->>'preview_hash'=i.before_data->'preview'->>'preview_hash') IS NOT TRUE THEN
    RAISE EXCEPTION 'OA submission requires the exact reviewed application and policy preview' USING ERRCODE='23514';
   END IF;
  END IF;
  IF i.request_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM requests r JOIN request_revisions rr ON rr.org_id=r.org_id AND rr.id=r.current_revision_id WHERE r.org_id=i.org_id AND r.id=i.request_id
   AND r.applicant_id=i.actor_id AND r.kind=i.kind AND r.business_id=i.business_id
   AND r.version=(i.before_data->'request'->>'version')::bigint
   AND r.current_revision_id::text=i.before_data->'request'->>'current_revision_id'
   AND rr.content_hash=i.before_data->'request'->>'content_hash') THEN
   RAISE EXCEPTION 'OA request basis is stale or belongs to another applicant' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO i FROM agent_oa_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  SELECT * INTO q FROM requests WHERE org_id=NEW.org_id AND id=NEW.request_id;
  IF i.run_id IS NULL OR i.tool='draft_business_text' OR x.state<>'running' OR q.id IS NULL OR q.kind<>i.kind OR q.business_id<>i.business_id OR q.applicant_id<>i.actor_id
   OR (i.request_id IS NOT NULL AND i.request_id<>q.id)
   OR (q.id::text=NEW.result_data->>'id' AND q.version=(NEW.result_data->>'version')::bigint
    AND q.current_revision_id::text=NEW.result_data->>'current_revision_id' AND (SELECT rr.content_hash FROM request_revisions rr WHERE rr.org_id=q.org_id AND rr.id=q.current_revision_id)=NEW.result_data->>'content_hash'
    AND q.state=NEW.result_data->>'state'
    AND q.version=coalesce((i.arguments->>'expected_version')::bigint,0)+1) IS NOT TRUE
   OR (i.tool='draft_request' AND q.state<>'draft')
   OR (i.tool='submit_request' AND (q.state<>'in_review' OR q.active_instance_id IS NULL
    OR NOT EXISTS(SELECT 1 FROM approval_steps s WHERE s.org_id=q.org_id AND s.request_id=q.id AND s.instance_id=q.active_instance_id)))
   OR NOT EXISTS(SELECT 1 FROM idempotency_records o WHERE o.org_id=i.org_id AND o.id=NEW.operation_id AND o.actor_id=i.actor_id
    AND o.route_key='/internal/agent-oa/'||i.tool AND o.idempotency_key='agent-oa:'||i.run_id::text
    AND o.state='succeeded' AND o.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'OA commit requires its exact real business request and receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
  WHERE s.org_id=i.org_id AND s.id=i.session_id AND s.user_id=i.actor_id AND s.device_type=x.channel
  AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
  AND s.auth_version=u.auth_version AND u.auth_version=x.actor_auth_version AND o.authorization_version=x.organization_auth_version) THEN
  RAISE EXCEPTION 'OA delegation requires the original live human session' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;

CREATE TABLE agent_oa_text_drafts (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, fields text NOT NULL,
 source_snapshots jsonb NOT NULL CHECK(jsonb_typeof(source_snapshots)='array' AND jsonb_array_length(source_snapshots) BETWEEN 1 AND 100),
 draft_sha256 text NOT NULL CHECK(draft_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_oa_intents(org_id,run_id),
 CHECK(octet_length(fields)<=262144 AND jsonb_typeof(fields::jsonb)='object'),
 CHECK(encode(sha256(convert_to(fields,'UTF8')),'hex')=draft_sha256)
);
CREATE TABLE agent_oa_text_adoptions (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL,
 operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_oa_text_drafts(org_id,run_id)
);
CREATE TRIGGER agent_oa_text_draft_immutable BEFORE UPDATE OR DELETE ON agent_oa_text_drafts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_oa_text_adoption_immutable BEFORE UPDATE OR DELETE ON agent_oa_text_adoptions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_agent_oa_text() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_oa_intents; x agent_runs; wanted jsonb; actual jsonb; pair record;
BEGIN
 SELECT * INTO i FROM agent_oa_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 SELECT * INTO x FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF i.tool IS DISTINCT FROM 'draft_business_text' OR x.actor_id IS DISTINCT FROM i.actor_id THEN
  RAISE EXCEPTION 'text proposal requires its exact delegated intent' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_oa_text_drafts' THEN
  IF x.state<>'running' OR x.cancel_requested_at IS NOT NULL OR x.expires_at<=clock_timestamp() THEN
   RAISE EXCEPTION 'text proposal requires an active run' USING ERRCODE='23514';
  END IF;
  SELECT jsonb_agg(v ORDER BY v) INTO wanted FROM jsonb_array_elements_text(i.arguments->'fields') f(v);
  SELECT jsonb_agg(v ORDER BY v) INTO actual FROM jsonb_object_keys(NEW.fields::jsonb) f(v);
  IF wanted IS NULL OR actual IS DISTINCT FROM wanted OR jsonb_array_length(wanted) NOT BETWEEN 1 AND 3 THEN
   RAISE EXCEPTION 'text fields must match exact human selection' USING ERRCODE='23514';
  END IF;
  FOR pair IN SELECT key,value FROM jsonb_each(NEW.fields::jsonb) LOOP
   IF NOT (pair.key=ANY(CASE WHEN i.kind='contract' THEN ARRAY['renewal_terms','termination_terms','notes'] ELSE ARRAY['purpose','notes'] END))
    OR jsonb_typeof(pair.value)<>'string' OR length(btrim(pair.value#>>'{}'))=0
    OR length(pair.value#>>'{}')>(CASE WHEN pair.key='purpose' THEN 4000 ELSE 20000 END) THEN
    RAISE EXCEPTION 'invalid business prose field' USING ERRCODE='23514';
   END IF;
  END LOOP;
 ELSE
  IF x.state<>'succeeded' OR x.cancel_requested_at IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM idempotency_records o WHERE o.org_id=i.org_id AND o.id=NEW.operation_id AND o.actor_id=i.actor_id
    AND o.route_key='/internal/agent-oa-text-adopt/'||i.kind AND o.idempotency_key='agent-oa-text:'||i.run_id::text
    AND o.state='succeeded' AND o.response_data->>'id'=i.business_id::text
    AND o.response_data->>'current_revision_id'=NEW.result_data->>'current_revision_id'
    AND o.response_data->>'content_hash'=NEW.result_data->>'content_hash'
    AND (o.response_data->>'version')::bigint=(i.before_data->'business'->>'version')::bigint+1
    AND o.response_data->'version'=NEW.result_data->'version')
   OR (NEW.result_data->>'id'=i.business_id::text AND NEW.result_data->>'kind'=i.kind) IS NOT TRUE THEN
   RAISE EXCEPTION 'adoption requires exact normal human business receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_oa_text_draft_binding BEFORE INSERT ON agent_oa_text_drafts FOR EACH ROW EXECUTE FUNCTION validate_agent_oa_text();
CREATE TRIGGER agent_oa_text_adoption_binding BEFORE INSERT ON agent_oa_text_adoptions FOR EACH ROW EXECUTE FUNCTION validate_agent_oa_text();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_oa_text_drafts,agent_oa_text_adoptions TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_oa_text_drafts,agent_oa_text_adoptions FROM copypaste_app;
END IF; END; $$;

ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));

ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
