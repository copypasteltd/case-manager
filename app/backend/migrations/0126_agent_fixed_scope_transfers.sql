-- Human-reviewed exact ordinary scope plans; old authorization is invalidated on execution.
CREATE TABLE agent_fixed_deliveries (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,actor_id uuid NOT NULL,session_id uuid NOT NULL,
 origin_channel text NOT NULL CHECK(origin_channel IN ('web','android','wechat','douyin')),
 kind text NOT NULL CONSTRAINT agent_fixed_delivery_kind CHECK(kind IN ('scope_transfer')),
 input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),preview jsonb NOT NULL CHECK(jsonb_typeof(preview)='object' AND octet_length(preview::text)<=100000),
 intent text NOT NULL CHECK(jsonb_typeof(intent::jsonb)='object' AND octet_length(intent)<=262144),
 intent_sha256 text NOT NULL CHECK(intent_sha256=encode(sha256(convert_to(intent,'UTF8')),'hex')),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,run_id),FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TABLE agent_fixed_delivery_commits (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_fixed_deliveries(org_id,run_id)
);
CREATE TABLE agent_scope_transfer_terminals (
 run_id uuid PRIMARY KEY,org_id uuid NOT NULL,job_id uuid NOT NULL UNIQUE REFERENCES jobs(id),lease_owner text NOT NULL,fencing_token bigint NOT NULL,
 authority_stamp text NOT NULL,credential_sha256 text NOT NULL CHECK(credential_sha256 ~ '^[a-f0-9]{64}$'),new_organization_version bigint NOT NULL,
 output jsonb NOT NULL CHECK(jsonb_typeof(output)='object'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_fixed_delivery_commits(org_id,run_id)
);
CREATE FUNCTION validate_fixed_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_fixed_deliveries;r agent_runs;b jsonb;
BEGIN
 IF TG_TABLE_NAME='agent_fixed_deliveries' THEN i:=NEW;ELSE SELECT * INTO i FROM agent_fixed_deliveries WHERE org_id=NEW.org_id AND run_id=NEW.run_id;END IF;
 SELECT * INTO r FROM agent_runs WHERE org_id=i.org_id AND id=i.run_id;b:=i.intent::jsonb;
 IF r.id IS NULL OR r.actor_id<>i.actor_id OR r.channel<>i.origin_channel OR r.channel='telegram'
 OR (b->>'run_id'=r.id::text AND b->>'actor_id'=r.actor_id::text AND b->>'session_id'=i.session_id::text
 AND b->>'origin_channel'=r.channel AND (b->>'actor_auth_version')::bigint=r.actor_auth_version
 AND (b->>'organization_auth_version')::bigint=r.organization_auth_version AND b->>'conversation_id'=r.conversation_id::text
 AND b->>'conversation_revision_id'=r.conversation_revision_id::text AND (b->>'context_security_version')::bigint=r.context_security_version
 AND b->>'kind'=i.kind AND b->'input'=i.input AND b->'preview'=i.preview) IS NOT TRUE
 OR EXISTS(SELECT 1 FROM agent_run_mutations x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR EXISTS(SELECT 1 FROM agent_file_intents x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR EXISTS(SELECT 1 FROM agent_oa_intents x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR EXISTS(SELECT 1 FROM agent_post_intents x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR EXISTS(SELECT 1 FROM agent_business_draft_intents x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR EXISTS(SELECT 1 FROM agent_business_revision_intents x WHERE x.org_id=i.org_id AND x.run_id=i.run_id)
  OR NOT EXISTS(SELECT 1 FROM conversations c JOIN agent_runtime_profiles p ON p.org_id=c.org_id AND p.id=r.profile_id
 WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=r.actor_id AND p.skill_id='information-organize'
 AND r.audience_ids=ARRAY[r.actor_id]) THEN RAISE EXCEPTION 'fixed delivery requires its exact original human and private runtime scope' USING ERRCODE='23514';END IF;
 IF TG_TABLE_NAME='agent_fixed_deliveries' AND (r.state<>'queued' OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
 JOIN organizations o ON o.id=s.org_id WHERE s.org_id=r.org_id AND s.id=i.session_id AND s.user_id=r.actor_id AND s.device_type=r.channel
 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version AND u.status='active'
 AND o.authorization_version=r.organization_auth_version AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes')) THEN
 RAISE EXCEPTION 'fixed authorization requires the original recent-MFA human session' USING ERRCODE='23514';END IF;
 IF TG_TABLE_NAME='agent_fixed_delivery_commits' THEN
 IF NOT EXISTS(SELECT 1 FROM idempotency_records o WHERE o.id=NEW.operation_id AND o.org_id=r.org_id AND o.actor_id=r.actor_id
 AND o.route_key='/internal/agent-fixed-deliveries/'||i.kind AND o.idempotency_key='agent-fixed:'||r.id::text AND o.state='succeeded' AND o.response_data=NEW.result_data) THEN
 RAISE EXCEPTION 'fixed delivery commit requires the exact ordinary atomic command receipt' USING ERRCODE='23514';END IF;
 END IF;
 RETURN NEW;
END;$$;
CREATE CONSTRAINT TRIGGER fixed_delivery_binding AFTER INSERT ON agent_fixed_deliveries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_fixed_delivery();
CREATE CONSTRAINT TRIGGER fixed_delivery_commit_binding AFTER INSERT ON agent_fixed_delivery_commits DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_fixed_delivery();
CREATE FUNCTION validate_scope_terminal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_fixed_deliveries i ON i.org_id=r.org_id AND i.run_id=r.id
 JOIN agent_fixed_delivery_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id JOIN file_scope_transfers p ON p.org_id=i.org_id AND p.id=(i.input->>'plan_id')::uuid
 JOIN jobs j ON j.org_id=r.org_id AND j.id=NEW.job_id JOIN agent_run_jobs x ON x.org_id=r.org_id AND x.run_id=r.id AND x.job_id=j.id
 JOIN organizations o ON o.id=r.org_id WHERE r.org_id=NEW.org_id AND r.id=NEW.run_id AND r.state='succeeded' AND r.outcome='completed'
 AND i.kind='scope_transfer' AND p.state='executed' AND p.plan_hash=i.input->>'plan_hash' AND c.result_data->>'id'=p.id::text
 AND NEW.new_organization_version=r.organization_auth_version+1 AND o.authorization_version=NEW.new_organization_version
 AND j.state='succeeded' AND j.fencing_token=NEW.fencing_token
 AND EXISTS(SELECT 1 FROM agent_tool_receipts t WHERE t.org_id=r.org_id AND t.run_id=r.id AND t.authority_stamp=NEW.authority_stamp
 AND t.tool='commit_scope_transfer' AND t.output=NEW.output AND t.output->>'operation_id'=c.operation_id::text)) THEN
 RAISE EXCEPTION 'scope terminal requires the exact completed ordinary change, lease and tool receipt without refreshing old authority' USING ERRCODE='23514';END IF;RETURN NEW;
END;$$;
CREATE CONSTRAINT TRIGGER scope_terminal_binding AFTER INSERT ON agent_scope_transfer_terminals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_scope_terminal();
CREATE FUNCTION reject_mixed_fixed_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM agent_fixed_deliveries WHERE org_id=NEW.org_id AND run_id=NEW.run_id) THEN
 RAISE EXCEPTION 'a fixed delivery is the sole human write authorization of its run' USING ERRCODE='23514';END IF;RETURN NEW;
END;$$;
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_run_mutations FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_file_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_oa_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_post_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_business_draft_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER fixed_delivery_exclusive BEFORE INSERT ON agent_business_revision_intents FOR EACH ROW EXECUTE FUNCTION reject_mixed_fixed_delivery();
CREATE TRIGGER immutable_fixed_delivery BEFORE UPDATE OR DELETE ON agent_fixed_deliveries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_fixed_delivery BEFORE UPDATE OR DELETE ON agent_fixed_delivery_commits FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_fixed_delivery BEFORE UPDATE OR DELETE ON agent_scope_transfer_terminals FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_fixed_deliveries,agent_fixed_delivery_commits,agent_scope_transfer_terminals TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_fixed_deliveries,agent_fixed_delivery_commits,agent_scope_transfer_terminals FROM copypaste_app;
END IF;END;$$;
