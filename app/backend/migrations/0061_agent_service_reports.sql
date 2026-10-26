-- Captured service evidence is a read receipt, never an operational command.
ALTER TABLE agent_runtime_profiles DROP CONSTRAINT agent_runtime_profiles_check;
ALTER TABLE agent_runtime_profiles ADD CONSTRAINT agent_runtime_profiles_telegram_configuration_sha256_check
 CHECK (telegram_configuration_sha256 IS NULL OR (skill_id IN ('project-briefing','incident-summary') AND telegram_configuration_sha256 ~ '^[a-f0-9]{64}$'));
CREATE OR REPLACE FUNCTION guard_telegram_run_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_inbox i
  JOIN telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id
  JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
  JOIN telegram_conversation_scopes s ON s.org_id=i.org_id AND s.conversation_id=NEW.conversation_id
  JOIN agent_runs r ON r.org_id=i.org_id AND r.id=NEW.run_id
  JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
  JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
  JOIN organizations o ON o.id=i.org_id
  JOIN conversation_messages m ON m.org_id=r.org_id AND m.run_id=r.id AND m.role='user'
  WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.kind='request'
   AND i.state IN ('awaiting_executor','dispatched') AND i.requested_skill_id IS NOT NULL
   AND ch.state='active' AND ch.version=i.channel_version AND i.requested_skill_id=ANY(ch.allowed_skill_ids)
   AND b.state='active' AND b.bot_id=i.bot_id AND b.user_id=i.requester_id
   AND b.telegram_user_id=i.telegram_user_id AND ch.bot_id=i.bot_id
   AND ch.chat_id=i.chat_id AND ch.topic_id=i.topic_id
   AND u.status='active' AND u.auth_version=i.actor_auth_version
   AND s.channel_id=i.channel_id AND s.channel_version=i.channel_version AND s.binding_id=i.binding_id
   AND s.actor_id=i.requester_id AND s.actor_auth_version=i.actor_auth_version
   AND s.organization_auth_version=o.authorization_version
   AND r.conversation_id=NEW.conversation_id AND r.channel='telegram' AND r.state='queued'
   AND r.actor_id=i.requester_id AND r.actor_auth_version=i.actor_auth_version
   AND r.organization_auth_version=o.authorization_version AND r.audience_ids=ARRAY[i.requester_id]
   AND r.client_message_id=i.id AND p.skill_id=i.requested_skill_id AND p.state='active'
   AND p.skill_id IN ('project-briefing','incident-summary') AND p.telegram_configuration_sha256 IS NOT NULL
   AND m.body=i.request_text AND m.author_id=i.requester_id AND m.audience_ids=ARRAY[i.requester_id]) THEN
  RAISE EXCEPTION 'Telegram run needs its exact durable inbound request and profile' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
CREATE TABLE agent_service_reports (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,run_id uuid NOT NULL,conversation_id uuid NOT NULL,actor_id uuid NOT NULL,service_id uuid NOT NULL,
 audience uuid[] NOT NULL CHECK(cardinality(audience) BETWEEN 1 AND 30),related_service_ids uuid[] NOT NULL CHECK(cardinality(related_service_ids) BETWEEN 1 AND 201),
 title text NOT NULL,configuration_version bigint NOT NULL CHECK(configuration_version>0),
 content_text text NOT NULL CHECK(octet_length(content_text)<=90000 AND jsonb_typeof(content_text::jsonb)='object'),
 document_sha256 text NOT NULL CHECK(document_sha256~'^[a-f0-9]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,service_id) REFERENCES services(org_id,id),
 CHECK(content_text::jsonb ?& ARRAY['report_id','service_id','captured_at','window','state_as_of_capture','truncated'] AND content_text::jsonb->>'report_id'=id::text AND content_text::jsonb->>'service_id'=service_id::text),
 CHECK(service_id=ANY(related_service_ids))
);
CREATE INDEX agent_service_report_run ON agent_service_reports(org_id,run_id,created_at,id);
CREATE TRIGGER agent_service_report_immutable BEFORE UPDATE OR DELETE ON agent_service_reports FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION bind_agent_service_report() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
  JOIN services s ON s.org_id=r.org_id AND s.id=NEW.service_id
  WHERE r.org_id=NEW.org_id AND r.id=NEW.run_id AND r.actor_id=NEW.actor_id AND r.conversation_id=NEW.conversation_id
  AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
  AND r.audience_ids=NEW.audience AND p.skill_id='incident-summary' AND p.state='active'
  AND s.version=NEW.configuration_version AND s.name=NEW.title) THEN
  RAISE EXCEPTION 'service report requires a live incident task and exact source' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_service_report_binding BEFORE INSERT ON agent_service_reports FOR EACH ROW EXECUTE FUNCTION bind_agent_service_report();
CREATE FUNCTION verify_agent_service_report_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts t WHERE t.org_id=NEW.org_id AND t.run_id=NEW.run_id AND t.tool='read_service_summary'
 AND t.output->'content'->>'report_id'=NEW.id::text
 AND t.output->'content'=NEW.content_text::jsonb
 AND t.source_snapshots @> jsonb_build_array(jsonb_build_object('type','service','id',NEW.service_id,'version_id',NEW.id,'sha256',NEW.document_sha256,'resource_version',NEW.configuration_version))) THEN
 RAISE EXCEPTION 'service report needs its exact successful tool receipt' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_service_report_receipt AFTER INSERT ON agent_service_reports DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verify_agent_service_report_receipt();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_service_reports TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_service_reports FROM copypaste_app;
END IF; END $$;
