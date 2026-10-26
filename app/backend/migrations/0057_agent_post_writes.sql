-- Generated content is confined to a private draft intent. Publication is a
-- separate exact revision authorization and never sends external messages.
CREATE TABLE agent_post_intents (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL, actor_id uuid NOT NULL,
 conversation_id uuid NOT NULL, session_id uuid NOT NULL,
 tool text NOT NULL CHECK(tool IN ('draft_post','publish_post')), post_id uuid,
 input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),
 before_data jsonb NOT NULL CHECK(jsonb_typeof(before_data)='object'),
 base_sha256 text NOT NULL CHECK(base_sha256 ~ '^[0-9a-f]{64}$'),
 intent text NOT NULL CHECK(octet_length(intent)<=524288 AND jsonb_typeof(intent::jsonb)='object'),
 intent_sha256 text NOT NULL CHECK(intent_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
 FOREIGN KEY(org_id,post_id) REFERENCES posts(org_id,id),
 CHECK(tool<>'publish_post' OR post_id IS NOT NULL),
 CHECK(encode(sha256(convert_to(intent,'UTF8')),'hex')=intent_sha256),
 CHECK(((intent::jsonb)->>'run_id'=run_id::text AND (intent::jsonb)->>'actor_id'=actor_id::text
  AND (intent::jsonb)->>'conversation_id'=conversation_id::text AND (intent::jsonb)->>'session_id'=session_id::text
  AND (intent::jsonb)->'input'=input AND input->>'tool'=tool
  AND input->>'post_id' IS NOT DISTINCT FROM post_id::text
  AND (intent::jsonb)->'before'=before_data AND (intent::jsonb)->>'base_sha256'=base_sha256) IS TRUE)
);
CREATE TABLE agent_post_commits (
 run_id uuid PRIMARY KEY, org_id uuid NOT NULL,
 operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id), post_id uuid NOT NULL,
 arguments jsonb NOT NULL CHECK(jsonb_typeof(arguments)='object'),
 result_data jsonb NOT NULL CHECK(jsonb_typeof(result_data)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_post_intents(org_id,run_id),
 FOREIGN KEY(org_id,post_id) REFERENCES posts(org_id,id)
);
CREATE TRIGGER agent_post_intent_immutable BEFORE UPDATE OR DELETE ON agent_post_intents FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_post_commit_immutable BEFORE UPDATE OR DELETE ON agent_post_commits FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_agent_post_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE x agent_runs; i agent_post_intents; p posts; r post_revisions;
BEGIN
 SELECT * INTO x FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF x.id IS NULL OR x.cancel_requested_at IS NOT NULL OR x.expires_at<=clock_timestamp()
  OR NOT EXISTS(SELECT 1 FROM agent_runtime_profiles t WHERE t.org_id=x.org_id AND t.id=x.profile_id AND t.state='active' AND t.skill_id='project-briefing') THEN
  RAISE EXCEPTION 'Knowledge write requires its active exact runtime' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_post_intents' THEN
  i:=NEW;
  IF x.state<>'queued' OR x.actor_id<>i.actor_id OR x.conversation_id<>i.conversation_id OR x.channel NOT IN ('web','android','wechat','douyin')
   OR (((i.intent::jsonb)->>'actor_auth_version')::bigint=x.actor_auth_version
    AND ((i.intent::jsonb)->>'organization_auth_version')::bigint=x.organization_auth_version
    AND (i.intent::jsonb)->>'conversation_revision_id'=x.conversation_revision_id::text
    AND ((i.intent::jsonb)->>'context_security_version')::bigint=x.context_security_version) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM conversations c JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE c.org_id=x.org_id AND c.id=x.conversation_id AND c.type='private' AND c.creator_id=i.actor_id AND cr.state='active'
    AND c.current_revision_id=x.conversation_revision_id AND cr.context_security_version=x.context_security_version
    AND c.project_id IS NOT DISTINCT FROM nullif(i.before_data->'scope'->>'project_id','')::uuid) THEN
   RAISE EXCEPTION 'Knowledge intent differs from its private original sender' USING ERRCODE='23514';
  END IF;
  IF i.post_id IS NOT NULL THEN
   SELECT * INTO p FROM posts WHERE org_id=i.org_id AND id=i.post_id;
   IF p.id IS NULL OR (p.version=(i.input->>'expected_version')::bigint AND p.current_revision_id::text=i.input->>'base_revision_id'
    AND p.id::text=i.before_data->'post'->>'id' AND p.version=(i.before_data->'post'->>'version')::bigint
    AND p.current_revision_id::text=i.before_data->'post'->>'current_revision_id'
    AND i.input->>'content_hash'=i.before_data->'post'->>'content_hash') IS NOT TRUE THEN
    RAISE EXCEPTION 'Knowledge authorization requires its exact original revision' USING ERRCODE='23514';
   END IF;
  END IF;
 ELSE
  SELECT * INTO i FROM agent_post_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  SELECT * INTO p FROM posts WHERE org_id=NEW.org_id AND id=NEW.post_id;
  SELECT * INTO r FROM post_revisions WHERE org_id=NEW.org_id AND post_id=p.id AND id=p.current_revision_id;
  IF x.state<>'running' OR i.run_id IS NULL OR p.id IS NULL OR r.id IS NULL
   OR (i.post_id IS NOT NULL AND i.post_id<>p.id)
   OR (i.post_id IS NULL AND p.author_id<>i.actor_id)
   OR p.project_id IS DISTINCT FROM nullif(i.before_data->'scope'->>'project_id','')::uuid
   OR (p.id::text=NEW.result_data->>'id' AND p.version=(NEW.result_data->>'version')::bigint
    AND p.current_revision_id::text=NEW.result_data->>'current_revision_id' AND p.state=NEW.result_data->>'state'
    AND p.version=coalesce((i.input->>'expected_version')::bigint,0)+1
    AND NEW.arguments->>'run_id'=i.run_id::text AND NEW.arguments->>'intent_sha256'=i.intent_sha256) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM idempotency_records o WHERE o.org_id=i.org_id AND o.id=NEW.operation_id AND o.actor_id=i.actor_id
    AND o.route_key='/internal/agent-post/'||i.tool AND o.idempotency_key='agent-post:'||i.run_id::text
    AND o.state='succeeded' AND o.response_data=NEW.result_data) THEN
   RAISE EXCEPTION 'Knowledge commit requires its real exact business receipt' USING ERRCODE='23514';
  END IF;
  IF i.tool='draft_post' THEN
   IF (r.title=i.input->>'title' AND p.type=i.input->>'type' AND p.classification=i.input->>'classification'
    AND r.body=NEW.arguments->>'body' AND r.summary=NEW.arguments->>'summary' AND to_jsonb(r.tags)=NEW.arguments->'tags'
    AND r.source_refs=coalesce(i.input->'source_refs','[]'::jsonb)
    AND r.id::text=NEW.result_data->>'current_revision_id') IS NOT TRUE
    OR (i.post_id IS NULL AND (p.state<>'draft' OR p.published_revision_id IS NOT NULL))
    OR (i.post_id IS NOT NULL AND (p.state<>i.before_data->'post'->>'state'
      OR p.published_revision_id IS DISTINCT FROM nullif(i.before_data->'post'->>'published_revision_id','')::uuid
      OR r.base_revision_id IS DISTINCT FROM (i.input->>'base_revision_id')::uuid)) THEN
    RAISE EXCEPTION 'Draft content or visibility exceeds its authorized target' USING ERRCODE='23514';
   END IF;
  ELSE
   IF p.state<>'published' OR p.current_revision_id::text<>i.input->>'base_revision_id' OR p.published_revision_id<>p.current_revision_id
    OR NEW.arguments ?| ARRAY['body','summary','tags'] THEN
    RAISE EXCEPTION 'Publication must retain the exact reviewed content' USING ERRCODE='23514';
   END IF;
  END IF;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
  WHERE s.org_id=i.org_id AND s.id=i.session_id AND s.user_id=i.actor_id AND s.device_type=x.channel
  AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
  AND s.auth_version=u.auth_version AND u.auth_version=x.actor_auth_version AND o.authorization_version=x.organization_auth_version) THEN
  RAISE EXCEPTION 'Knowledge write requires the original live interactive session' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_post_intent_binding BEFORE INSERT ON agent_post_intents FOR EACH ROW EXECUTE FUNCTION validate_agent_post_binding();
CREATE TRIGGER agent_post_commit_binding BEFORE INSERT ON agent_post_commits FOR EACH ROW EXECUTE FUNCTION validate_agent_post_binding();
CREATE FUNCTION validate_agent_post_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE i agent_post_intents;
BEGIN
 SELECT * INTO i FROM agent_post_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
 IF TG_TABLE_NAME='agent_post_intents' THEN
  IF EXISTS(SELECT 1 FROM agent_run_mutations m WHERE m.org_id=i.org_id AND m.run_id=i.run_id)
   OR EXISTS(SELECT 1 FROM agent_file_intents f WHERE f.org_id=i.org_id AND f.run_id=i.run_id)
   OR EXISTS(SELECT 1 FROM agent_oa_intents o WHERE o.org_id=i.org_id AND o.run_id=i.run_id)
   OR NOT EXISTS(SELECT 1 FROM conversation_messages m WHERE m.org_id=i.org_id AND m.run_id=i.run_id AND m.role='user'
    AND m.body=(i.intent::jsonb)->>'request' AND m.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_post_write','id',i.run_id,'resource_version',1,'sha256',i.intent_sha256))) THEN
   RAISE EXCEPTION 'Knowledge authorization requires its original sender message' USING ERRCODE='23514';
  END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts t WHERE t.org_id=i.org_id AND t.run_id=i.run_id AND t.tool=i.tool
   AND t.output->>'committed'='true' AND t.output->>'status'='succeeded' AND t.output->>'operation_id'=NEW.operation_id::text
   AND t.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_post_write','id',i.run_id,'resource_version',1,'sha256',i.intent_sha256))) THEN
   RAISE EXCEPTION 'Knowledge commit requires its atomic successful tool receipt' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_post_sender_evidence AFTER INSERT ON agent_post_intents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_post_evidence();
CREATE CONSTRAINT TRIGGER agent_post_tool_evidence AFTER INSERT ON agent_post_commits DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_post_evidence();
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post'));
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_post_intents,agent_post_commits TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_post_intents,agent_post_commits FROM copypaste_app;
END IF; END; $$;
