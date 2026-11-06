-- Exact human draft changes, including an independently acknowledged target scope.
CREATE TABLE agent_business_revision_intents (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,actor_id uuid NOT NULL,conversation_id uuid NOT NULL,session_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('contract','purchase','claim','subscription')),resource_id uuid NOT NULL,
 input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),
 arguments jsonb NOT NULL CHECK(jsonb_typeof(arguments)='object' AND octet_length(arguments::text)<=131072),
 intent text NOT NULL CHECK(octet_length(intent) BETWEEN 1 AND 393216 AND jsonb_typeof(intent::jsonb)='object'),
 intent_sha256 text NOT NULL CHECK(intent_sha256=encode(sha256(convert_to(intent,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id)
);
CREATE TABLE agent_business_revision_commits (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),business_id uuid NOT NULL,
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_business_revision_intents(org_id,run_id)
);
CREATE TRIGGER agent_business_revision_intent_immutable BEFORE UPDATE OR DELETE ON agent_business_revision_intents FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_business_revision_commit_immutable BEFORE UPDATE OR DELETE ON agent_business_revision_commits FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_agent_business_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs;i agent_business_revision_intents;x jsonb;o idempotency_records;b jsonb;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF r.id IS NULL THEN RAISE EXCEPTION 'exact run required' USING ERRCODE='23514';END IF;
 IF TG_TABLE_NAME='agent_business_revision_intents' THEN
  x:=NEW.intent::jsonb;
  IF r.state<>'queued' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR r.channel NOT IN ('web','android','wechat','douyin') OR r.actor_id<>NEW.actor_id OR r.conversation_id<>NEW.conversation_id
   OR r.audience_ids<>ARRAY[NEW.actor_id]
   OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=r.org_id AND p.id=r.profile_id AND p.skill_id='oa-assist' AND p.state='active')
   OR (x->>'run_id'=r.id::text AND x->>'actor_id'=r.actor_id::text AND x->>'session_id'=NEW.session_id::text
    AND x->>'conversation_id'=r.conversation_id::text AND x->>'conversation_revision_id'=r.conversation_revision_id::text
    AND (x->>'context_security_version')::bigint=r.context_security_version AND (x->>'actor_auth_version')::bigint=r.actor_auth_version
    AND (x->>'organization_auth_version')::bigint=r.organization_auth_version AND x->'input'=NEW.input AND x->'basis'=NEW.basis
    AND NEW.input->>'kind'=NEW.kind AND NEW.input->>'resource_id'=NEW.resource_id::text AND NEW.input->'arguments'=NEW.arguments
    AND NEW.basis->>'kind'=NEW.kind AND NEW.basis->>'id'=NEW.resource_id::text AND NEW.basis->'version'=NEW.input->'expected_version'
    AND NEW.basis->'revision_id'=NEW.input->'base_revision_id' AND NEW.basis->'content_hash'=NEW.input->'content_hash'
    AND NEW.basis->'scope_sha256'=NEW.input->'scope_sha256' AND NEW.basis->>'classification' IN ('internal','project_restricted')
    AND (NEW.basis->>'cross_scope'<>'true' OR NEW.input->>'cross_scope_reviewed'='true')) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=r.actor_id
    AND c.current_revision_id=r.conversation_revision_id AND cr.context_security_version=r.context_security_version AND cr.state='active'
    AND (NEW.basis->>'cross_scope')::boolean=((c.project_id::text IS DISTINCT FROM NEW.basis->>'project_id') OR c.classification<>NEW.basis->>'classification'))
   OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations org ON org.id=s.org_id
    WHERE s.org_id=r.org_id AND s.id=NEW.session_id AND s.user_id=r.actor_id AND s.device_type=r.channel AND s.revoked_at IS NULL
    AND s.expires_at>clock_timestamp() AND u.status='active' AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version
    AND org.authorization_version=r.organization_auth_version AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes'
    AND s.mfa_verified_at<clock_timestamp()+interval '1 minute')
   OR EXISTS(SELECT 1 FROM agent_run_mutations m WHERE m.org_id=r.org_id AND m.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_file_intents f WHERE f.org_id=r.org_id AND f.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_oa_intents q WHERE q.org_id=r.org_id AND q.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_post_intents w WHERE w.org_id=r.org_id AND w.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_business_draft_intents d WHERE d.org_id=r.org_id AND d.run_id=r.id) THEN
    RAISE EXCEPTION 'business revision requires exact private human authorization' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO i FROM agent_business_revision_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  SELECT * INTO o FROM idempotency_records WHERE org_id=NEW.org_id AND id=NEW.operation_id;
  b:=NEW.result_data;
  IF i.run_id IS NULL OR r.state<>'running' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR o.id IS NULL OR o.actor_id<>r.actor_id OR o.route_key<>'/internal/agent-business-revision/update'
   OR o.idempotency_key<>'agent-business-revision:'||r.id::text OR o.state<>'succeeded'
   OR (NEW.business_id=i.resource_id AND b->>'id'=i.resource_id::text AND b->>'kind'=i.kind
    AND (b->>'version')::bigint=(i.input->>'expected_version')::bigint+1 AND b->>'state'='draft'
    AND b->'classification'=i.basis->'classification' AND b->>'project_id' IS NOT DISTINCT FROM i.basis->>'project_id'
    AND o.response_data->>'id'=NEW.business_id::text AND o.response_data->>'current_revision_id'=b->>'revision_id'
    AND o.response_data->>'content_hash'=b->>'content_hash' AND o.response_data->'version'=b->'version') IS NOT TRUE THEN
    RAISE EXCEPTION 'business revision requires its ordinary immutable update receipt' USING ERRCODE='23514';
  END IF;
 END IF;RETURN NEW;
END;$$;
CREATE TRIGGER agent_business_revision_validate BEFORE INSERT ON agent_business_revision_intents FOR EACH ROW EXECUTE FUNCTION validate_agent_business_revision();
CREATE TRIGGER agent_business_revision_commit_validate BEFORE INSERT ON agent_business_revision_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_business_revision();
CREATE FUNCTION reject_mixed_business_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM agent_business_revision_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id) THEN
  RAISE EXCEPTION 'one exact write authorization per run' USING ERRCODE='23514';
 END IF;RETURN NEW;
END;$$;
CREATE TRIGGER revision_exclusive BEFORE INSERT ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION reject_mixed_business_revision();
CREATE TRIGGER revision_exclusive BEFORE INSERT ON agent_file_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_business_revision();
CREATE TRIGGER revision_exclusive BEFORE INSERT ON agent_oa_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_business_revision();
CREATE TRIGGER revision_exclusive BEFORE INSERT ON agent_post_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_business_revision();
CREATE TRIGGER revision_exclusive BEFORE INSERT ON agent_business_draft_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_business_revision();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_business_revision_intents,agent_business_revision_commits TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_business_revision_intents,agent_business_revision_commits FROM copypaste_app;
END IF;END;$$;
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_office_workspace','submit_deliverable','read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone','create_business_draft','revise_business_draft'));
