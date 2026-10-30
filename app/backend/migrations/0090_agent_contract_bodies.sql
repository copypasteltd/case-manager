-- Full contract text is an ordinary new scanned file. Its adoption is a separate
-- human contract revision, with immutable intent, original facts and receipt.
CREATE TABLE agent_contract_body_intents (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, contract_id uuid NOT NULL,
 actor_id uuid NOT NULL, conversation_id uuid NOT NULL, session_id uuid NOT NULL,
 input_data jsonb NOT NULL CHECK(jsonb_typeof(input_data)='object'),
 before_data jsonb NOT NULL CHECK(jsonb_typeof(before_data)='object'),
 base_sha256 text NOT NULL CHECK(base_sha256 ~ '^[0-9a-f]{64}$'),
 file_intent_sha256 text NOT NULL CHECK(file_intent_sha256 ~ '^[0-9a-f]{64}$'),
 intent text NOT NULL CHECK(octet_length(intent) BETWEEN 1 AND 524288 AND jsonb_typeof(intent::jsonb)='object'),
 intent_sha256 text NOT NULL CHECK(intent_sha256=encode(sha256(convert_to(intent,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_file_intents(org_id,run_id),
 FOREIGN KEY(org_id,contract_id) REFERENCES contracts(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id)
);
CREATE TABLE agent_contract_body_adoptions (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL,
 operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),
 version_id uuid NOT NULL, content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 reviewed boolean NOT NULL CHECK(reviewed), reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_contract_body_intents(org_id,run_id),
 FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TRIGGER agent_contract_body_intent_immutable BEFORE UPDATE OR DELETE ON agent_contract_body_intents
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_contract_body_adoption_immutable BEFORE UPDATE OR DELETE ON agent_contract_body_adoptions
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_agent_contract_body_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; i agent_file_intents; b agent_contract_body_intents; x jsonb;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_contract_body_intents' THEN
  x:=NEW.intent::jsonb;
  IF r.id IS NULL OR i.run_id IS NULL OR i.mode<>'create' OR i.actor_id<>NEW.actor_id OR r.actor_id<>NEW.actor_id
   OR r.conversation_id<>NEW.conversation_id OR r.audience_ids<>ARRAY[NEW.actor_id]
   OR r.state<>'queued' OR r.cancel_requested_at IS NOT NULL OR r.expires_at<=clock_timestamp()
   OR r.channel NOT IN ('web','android','wechat','douyin') OR i.intent_sha256<>NEW.file_intent_sha256
   OR EXISTS(SELECT 1 FROM agent_file_recoveries f WHERE f.org_id=NEW.org_id AND f.run_id=NEW.run_id)
   OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=r.org_id AND p.id=r.profile_id AND p.skill_id='file-update' AND p.state='active')
   OR (x->'input'=NEW.input_data AND x->'before'=NEW.before_data AND x->>'base_sha256'=NEW.base_sha256
    AND x->>'file_intent_sha256'=i.intent_sha256 AND x->>'run_id'=r.id::text AND x->>'actor_id'=r.actor_id::text
    AND x->>'session_id'=NEW.session_id::text AND x->>'conversation_id'=r.conversation_id::text
    AND x->>'conversation_revision_id'=r.conversation_revision_id::text
    AND (x->>'context_security_version')::bigint=r.context_security_version
    AND (x->>'actor_auth_version')::bigint=r.actor_auth_version
    AND (x->>'organization_auth_version')::bigint=r.organization_auth_version
    AND NEW.input_data->>'contract_id'=NEW.contract_id::text AND NEW.before_data->>'kind'='contract'
    AND NEW.before_data->>'id'=NEW.contract_id::text AND NEW.before_data->>'classification'=i.target_data->>'classification'
    AND NEW.input_data->>'base_revision_id'=NEW.before_data->>'current_revision_id'
    AND NEW.input_data->>'content_hash'=NEW.before_data->>'content_hash'
    AND NEW.input_data->'expected_version'=NEW.before_data->'version') IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM contracts c JOIN contract_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=NEW.org_id AND c.id=NEW.contract_id AND c.deleted_at IS NULL AND c.lifecycle='draft'
    AND c.approval_state IN ('not_submitted','returned','withdrawn')
    AND c.version=(NEW.before_data->>'version')::bigint AND c.current_revision_id::text=NEW.before_data->>'current_revision_id'
    AND cr.content_hash=NEW.before_data->>'content_hash' AND c.classification=NEW.before_data->>'classification'
    AND c.project_id::text IS NOT DISTINCT FROM NEW.before_data->>'project_id')
   OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=r.actor_id
    AND c.current_revision_id=r.conversation_revision_id AND cr.context_security_version=r.context_security_version AND cr.state='active'
    AND c.classification=NEW.before_data->>'classification')
   OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
    WHERE s.org_id=r.org_id AND s.id=NEW.session_id AND s.user_id=r.actor_id AND s.device_type=r.channel
    AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
    AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version AND o.authorization_version=r.organization_auth_version) THEN
   RAISE EXCEPTION 'contract body requires an exact editable contract, new file and private human authority' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO b FROM agent_contract_body_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  IF b.run_id IS NULL OR r.id IS NULL OR r.state<>'succeeded' OR r.outcome<>'completed' OR r.cancel_requested_at IS NOT NULL
   OR NOT EXISTS(SELECT 1 FROM agent_file_commits fc JOIN agent_file_drafts d ON d.org_id=fc.org_id AND d.run_id=fc.run_id AND d.id=fc.draft_id
    JOIN file_versions v ON v.org_id=d.org_id AND v.id=NEW.version_id
    WHERE fc.org_id=NEW.org_id AND fc.run_id=NEW.run_id AND fc.result_data->'file_version'->>'id'=v.id::text
    AND v.sha256=NEW.content_sha256 AND d.content_sha256=v.sha256 AND d.size_bytes=v.size_bytes
    AND v.scan_state='clean' AND v.availability='available' AND v.base_version_id IS NULL)
   OR (NEW.result_data->>'id'=b.contract_id::text AND NEW.result_data->>'kind'='contract'
    AND (NEW.result_data->>'version')::bigint=(b.before_data->>'version')::bigint+1
    AND NEW.result_data->>'lifecycle'='draft'
    AND NEW.result_data->'payload'->'content'=jsonb_set(b.before_data->'payload'->'content','{draft_file_version_id}',to_jsonb(NEW.version_id::text))) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM idempotency_records o WHERE o.org_id=NEW.org_id AND o.id=NEW.operation_id AND o.actor_id=b.actor_id
    AND o.route_key='/internal/agent-contract-body-adopt' AND o.idempotency_key='agent-contract-body:'||b.run_id::text AND o.state='succeeded'
    AND o.response_data->>'id'=b.contract_id::text AND o.response_data->'version'=NEW.result_data->'version'
    AND o.response_data->>'current_revision_id'=NEW.result_data->>'current_revision_id'
    AND o.response_data->>'content_hash'=NEW.result_data->>'content_hash'
    AND o.response_data->>'draft_file_version_id'=NEW.version_id::text) THEN
   RAISE EXCEPTION 'contract body adoption requires its exact scanned file and normal human revision receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_contract_body_intent_binding BEFORE INSERT ON agent_contract_body_intents
 FOR EACH ROW EXECUTE FUNCTION validate_agent_contract_body_binding();
CREATE TRIGGER agent_contract_body_adoption_binding BEFORE INSERT ON agent_contract_body_adoptions
 FOR EACH ROW EXECUTE FUNCTION validate_agent_contract_body_binding();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_contract_body_intents,agent_contract_body_adoptions TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_contract_body_intents,agent_contract_body_adoptions FROM copypaste_app;
END IF; END; $$;
