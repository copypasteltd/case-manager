-- Each discovery page has one immutable observation reference, with at most 20
-- exact candidate dependencies. Replays must revalidate every dependency.
CREATE TABLE agent_project_catalogs (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,run_id uuid NOT NULL,
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=90000),
 sha256 text NOT NULL CHECK(sha256=encode(sha256(convert_to(payload::text,'UTF8')),'hex')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,id),
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 CHECK(payload->>'kind' IN ('people','templates') AND jsonb_typeof(payload->'page'->'items')='array'
   AND jsonb_array_length(payload->'page'->'items')<=20)
);
CREATE INDEX agent_project_catalog_run ON agent_project_catalogs(org_id,run_id,created_at,id);
CREATE TRIGGER agent_project_catalog_immutable BEFORE UPDATE OR DELETE ON agent_project_catalogs
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION bind_agent_project_catalog() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
  JOIN conversations c ON c.org_id=r.org_id AND c.id=r.conversation_id
  WHERE r.org_id=NEW.org_id AND r.id=NEW.run_id AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
  AND p.skill_id='project-create' AND p.state='active' AND c.type='private' AND c.project_id IS NULL AND c.creator_id=r.actor_id
  AND c.current_revision_id=r.conversation_revision_id) THEN
  RAISE EXCEPTION 'catalog observation requires current private project planning execution' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_project_catalog_binding BEFORE INSERT ON agent_project_catalogs
 FOR EACH ROW EXECUTE FUNCTION bind_agent_project_catalog();

ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_project_catalogs TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_project_catalogs FROM copypaste_app;
END IF; END $$;
