-- Fixed, typed human draft payloads. No approval, activation, cost or cash facts.
CREATE TABLE agent_business_draft_intents (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,actor_id uuid NOT NULL,conversation_id uuid NOT NULL,session_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('contract','purchase','claim','subscription')),
 arguments jsonb NOT NULL CHECK(jsonb_typeof(arguments)='object' AND octet_length(arguments::text)<=131072),
 intent text NOT NULL CHECK(octet_length(intent) BETWEEN 1 AND 196608 AND jsonb_typeof(intent::jsonb)='object'),
 intent_sha256 text NOT NULL CHECK(intent_sha256=encode(sha256(convert_to(intent,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id)
);
CREATE TABLE agent_business_draft_commits (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),business_id uuid NOT NULL,
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_business_draft_intents(org_id,run_id)
);
CREATE TRIGGER agent_business_draft_intent_immutable BEFORE UPDATE OR DELETE ON agent_business_draft_intents FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_business_draft_commit_immutable BEFORE UPDATE OR DELETE ON agent_business_draft_commits FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_agent_business_draft() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs;i agent_business_draft_intents;x jsonb;o idempotency_records;b jsonb;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF r.id IS NULL THEN RAISE EXCEPTION 'exact run required' USING ERRCODE='23514';END IF;
 IF TG_TABLE_NAME='agent_business_draft_intents' THEN
  x:=NEW.intent::jsonb;
  IF r.state<>'queued' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR r.channel NOT IN ('web','android','wechat','douyin') OR r.actor_id<>NEW.actor_id OR r.conversation_id<>NEW.conversation_id
   OR r.audience_ids<>ARRAY[NEW.actor_id]
   OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=r.org_id AND p.id=r.profile_id AND p.skill_id='oa-assist' AND p.state='active')
   OR (x->>'run_id'=r.id::text AND x->>'actor_id'=r.actor_id::text AND x->>'session_id'=NEW.session_id::text
    AND x->>'conversation_id'=r.conversation_id::text AND x->>'conversation_revision_id'=r.conversation_revision_id::text
    AND (x->>'context_security_version')::bigint=r.context_security_version AND (x->>'actor_auth_version')::bigint=r.actor_auth_version
    AND (x->>'organization_auth_version')::bigint=r.organization_auth_version AND x->'input'->>'kind'=NEW.kind AND x->'input'->'arguments'=NEW.arguments) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=r.actor_id
    AND c.current_revision_id=r.conversation_revision_id AND cr.context_security_version=r.context_security_version AND cr.state='active'
    AND c.project_id::text IS NOT DISTINCT FROM NEW.arguments->>'project_id' AND c.classification=NEW.arguments->>'classification')
   OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations org ON org.id=s.org_id
    WHERE s.org_id=r.org_id AND s.id=NEW.session_id AND s.user_id=r.actor_id AND s.device_type=r.channel AND s.revoked_at IS NULL
    AND s.expires_at>clock_timestamp() AND u.status='active' AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version
    AND org.authorization_version=r.organization_auth_version)
   OR EXISTS(SELECT 1 FROM agent_run_mutations m WHERE m.org_id=r.org_id AND m.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_file_intents f WHERE f.org_id=r.org_id AND f.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_oa_intents q WHERE q.org_id=r.org_id AND q.run_id=r.id)
   OR EXISTS(SELECT 1 FROM agent_post_intents w WHERE w.org_id=r.org_id AND w.run_id=r.id) THEN
    RAISE EXCEPTION 'business draft requires exact private human authorization' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO i FROM agent_business_draft_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  SELECT * INTO o FROM idempotency_records WHERE org_id=NEW.org_id AND id=NEW.operation_id;
  b:=NEW.result_data;
  IF i.run_id IS NULL OR r.state<>'running' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR o.id IS NULL OR o.actor_id<>r.actor_id OR o.route_key<>'/internal/agent-business-draft/create'
   OR o.idempotency_key<>'agent-business-draft:'||r.id::text OR o.state<>'succeeded'
   OR (b->>'id'=NEW.business_id::text AND b->>'kind'=i.kind AND (b->>'version')::bigint=1 AND b->>'state'='draft'
    AND b->>'classification'=i.arguments->>'classification' AND b->>'project_id' IS NOT DISTINCT FROM i.arguments->>'project_id'
    AND o.response_data->>'id'=NEW.business_id::text AND o.response_data->>'current_revision_id'=b->>'revision_id'
    AND o.response_data->>'content_hash'=b->>'content_hash' AND o.response_data->'version'=b->'version') IS NOT TRUE THEN
    RAISE EXCEPTION 'business draft requires its ordinary immutable create receipt' USING ERRCODE='23514';
  END IF;
 END IF;RETURN NEW;
END;$$;
CREATE TRIGGER agent_business_draft_intent_binding BEFORE INSERT ON agent_business_draft_intents FOR EACH ROW EXECUTE FUNCTION validate_agent_business_draft();
CREATE TRIGGER agent_business_draft_commit_binding BEFORE INSERT ON agent_business_draft_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_business_draft();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_business_draft_intents,agent_business_draft_commits TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_business_draft_intents,agent_business_draft_commits FROM copypaste_app;
END IF;END;$$;

-- Preserve every earlier tool and admit only this fixed human-authorized draft command.
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_office_workspace','submit_deliverable','read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone','create_business_draft'));
