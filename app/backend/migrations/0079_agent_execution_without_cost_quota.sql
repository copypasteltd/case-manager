-- User scope revision99 excludes AI fee quotas, not execution safeguards.
-- Keep every immutable legacy row; bind technical telemetry to its tenant/run.
ALTER TABLE agent_budget_usage DROP CONSTRAINT agent_budget_usage_run_id_fkey;
ALTER TABLE agent_budget_usage ADD CONSTRAINT agent_execution_usage_run_fkey
 FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id);

-- The application may read old cost-control records but cannot create new ones.
-- Ordinary execution usage remains append-only under the existing tenant/state
-- binding and immutable triggers. No historical usage or charges are fabricated.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 REVOKE INSERT ON agent_budget_policies,agent_budget_reservations,agent_budget_reconciliations FROM copypaste_app;
END IF; END; $$;
