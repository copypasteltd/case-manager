-- Complete multi-subscription evidence reuses the immutable finance ledger.
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));

ALTER TABLE agent_finance_reports DROP CONSTRAINT agent_finance_reports_payload;
ALTER TABLE agent_finance_reports ADD CONSTRAINT agent_finance_reports_payload CHECK((
 content_text::jsonb ?& ARRAY['report_id','view','scope','captured_at','report','input_count','coverage_notice']
 AND content_text::jsonb->>'report_id'=id::text
 AND (content_text::jsonb->>'input_count')::int=jsonb_array_length(evidence)
 AND content_text::jsonb->>'project_id' IS NOT DISTINCT FROM project_id::text
 AND ((content_text::jsonb->>'view' IN ('forecast','actual','cash') AND content_text::jsonb ? 'month')
 OR (content_text::jsonb->>'schema_version'='2' AND content_text::jsonb->>'view' IN ('subscription_list','subscription_review','subscription_portfolio','finance_record')))) IS TRUE);
CREATE OR REPLACE FUNCTION verify_agent_finance_report_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_tool_receipts t WHERE t.org_id=NEW.org_id AND t.run_id=NEW.run_id
 AND t.tool=CASE NEW.content_text::jsonb->>'view' WHEN 'subscription_list' THEN 'list_subscriptions' WHEN 'subscription_review' THEN 'read_subscription_review' WHEN 'subscription_portfolio' THEN 'read_subscription_portfolio' WHEN 'finance_record' THEN 'read_finance_record' ELSE 'read_finance_summary' END
 AND t.output->>'status'='succeeded' AND t.output->'content'->>'report_id'=NEW.id::text AND t.output->'content'=NEW.content_text::jsonb
 AND t.source_snapshots @> jsonb_build_array(jsonb_build_object('type','agent_finance_report','id',NEW.id,'sha256',NEW.document_sha256,'resource_version',1))) THEN
 RAISE EXCEPTION 'finance report needs its exact successful typed tool receipt' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
