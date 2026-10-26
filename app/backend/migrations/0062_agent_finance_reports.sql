ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
-- Read-only monthly finance evidence is atomic with its successful tool receipt.
CREATE TABLE agent_finance_reports (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,run_id uuid NOT NULL,conversation_id uuid NOT NULL,actor_id uuid NOT NULL,project_id uuid,
 classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
 audience uuid[] NOT NULL CHECK(cardinality(audience) BETWEEN 1 AND 30),title text NOT NULL CHECK(length(title) BETWEEN 1 AND 200),
 content_text text NOT NULL CHECK(octet_length(content_text)<=90000 AND jsonb_typeof(content_text::jsonb)='object'),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='array' AND jsonb_array_length(evidence)<=500),
 document_sha256 text NOT NULL CHECK(document_sha256~'^[a-f0-9]{64}$'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id),FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 CHECK(content_text::jsonb ?& ARRAY['report_id','month','view','scope','captured_at','report','input_count','coverage_notice'] AND content_text::jsonb->>'report_id'=id::text
   AND content_text::jsonb->>'view' IN ('forecast','actual','cash') AND (content_text::jsonb->>'input_count')::int=jsonb_array_length(evidence)
   AND content_text::jsonb->>'project_id' IS NOT DISTINCT FROM project_id::text)
);
CREATE INDEX agent_finance_report_run ON agent_finance_reports(org_id,run_id,created_at,id);
CREATE TRIGGER agent_finance_report_immutable BEFORE UPDATE OR DELETE ON agent_finance_reports FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION bind_agent_finance_report() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
 JOIN conversations c ON c.org_id=r.org_id AND c.id=r.conversation_id
 WHERE r.org_id=NEW.org_id AND r.id=NEW.run_id AND r.actor_id=NEW.actor_id AND r.conversation_id=NEW.conversation_id
 AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
 AND r.audience_ids=NEW.audience AND p.skill_id='subscription-review' AND p.state='active'
 AND NEW.project_id IS NOT DISTINCT FROM c.project_id AND NEW.classification=c.classification) THEN
  RAISE EXCEPTION 'finance report requires a live subscription review and exact conversation scope' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_finance_report_binding BEFORE INSERT ON agent_finance_reports FOR EACH ROW EXECUTE FUNCTION bind_agent_finance_report();
CREATE FUNCTION verify_agent_finance_report_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts t WHERE t.org_id=NEW.org_id AND t.run_id=NEW.run_id AND t.tool='read_finance_summary'
 AND t.output->'content'->>'report_id'=NEW.id::text AND t.output->'content'=NEW.content_text::jsonb
 AND t.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_finance_report','id',NEW.id,'sha256',NEW.document_sha256,'resource_version',1))) THEN
 RAISE EXCEPTION 'finance report needs its exact successful tool receipt' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_finance_report_receipt AFTER INSERT ON agent_finance_reports DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verify_agent_finance_report_receipt();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_finance_reports TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_finance_reports FROM copypaste_app;
END IF; END $$;
