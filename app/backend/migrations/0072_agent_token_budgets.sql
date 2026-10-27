-- Token accounting, not a provider invoice or an enforceable provider spend cap.
-- No production allowance is invented: an interactive administrator must publish one.
CREATE TABLE agent_budget_policies (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 version bigint NOT NULL CHECK(version>0),
 organization_tokens bigint NOT NULL CHECK(organization_tokens BETWEEN 0 AND 1000000000000),
 user_tokens bigint NOT NULL CHECK(user_tokens BETWEEN 0 AND 1000000000000),
 project_tokens bigint NOT NULL CHECK(project_tokens BETWEEN 0 AND 1000000000000),
 skill_tokens bigint NOT NULL CHECK(skill_tokens BETWEEN 0 AND 1000000000000),
 published_by uuid NOT NULL REFERENCES users(id), reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 120),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,version), UNIQUE(org_id,id)
);
CREATE TABLE agent_budget_reservations (
 run_id uuid PRIMARY KEY REFERENCES agent_runs(id), org_id uuid NOT NULL REFERENCES organizations(id),
 policy_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES users(id), project_id uuid REFERENCES projects(id),
 skill_id text NOT NULL, month_start date NOT NULL,
 estimated_tokens bigint NOT NULL CHECK(estimated_tokens>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,policy_id) REFERENCES agent_budget_policies(org_id,id),
 CHECK(extract(day FROM month_start)=1)
);
CREATE INDEX agent_budget_month ON agent_budget_reservations(org_id,month_start,created_at DESC,run_id DESC);
CREATE TABLE agent_budget_usage (
 run_id uuid PRIMARY KEY REFERENCES agent_budget_reservations(run_id), org_id uuid NOT NULL REFERENCES organizations(id),
 input_tokens bigint NOT NULL CHECK(input_tokens BETWEEN 0 AND 1000000000000),
 output_tokens bigint NOT NULL CHECK(output_tokens BETWEEN 0 AND 1000000000000),
 cached_input_tokens bigint NOT NULL CHECK(cached_input_tokens BETWEEN 0 AND input_tokens),
 cache_write_input_tokens bigint NOT NULL CHECK(cache_write_input_tokens BETWEEN 0 AND 1000000000000),
 reasoning_output_tokens bigint NOT NULL CHECK(reasoning_output_tokens BETWEEN 0 AND 1000000000000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE agent_budget_reconciliations (
 run_id uuid PRIMARY KEY REFERENCES agent_budget_reservations(run_id), org_id uuid NOT NULL REFERENCES organizations(id),
 run_version bigint NOT NULL CHECK(run_version>0),
 input_tokens bigint NOT NULL CHECK(input_tokens BETWEEN 0 AND 1000000000000),
 output_tokens bigint NOT NULL CHECK(output_tokens BETWEEN 0 AND 1000000000000),
 evidence_reference text NOT NULL CHECK(length(evidence_reference) BETWEEN 1 AND 120),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 120),
 reconciled_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION validate_agent_budget_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; c conversations; p agent_runtime_profiles; b agent_budget_reservations;
BEGIN
 IF TG_TABLE_NAME='agent_budget_policies' THEN
  IF NOT EXISTS(SELECT 1 FROM users WHERE id=NEW.published_by AND org_id=NEW.org_id) THEN
   RAISE EXCEPTION 'budget publisher tenant mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id;
 IF r.org_id IS DISTINCT FROM NEW.org_id THEN
  RAISE EXCEPTION 'budget run tenant mismatch' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_budget_reservations' THEN
  SELECT * INTO c FROM conversations WHERE id=r.conversation_id;
  SELECT * INTO p FROM agent_runtime_profiles WHERE id=r.profile_id;
  IF r.state<>'queued' OR NEW.actor_id<>r.actor_id OR NEW.project_id IS DISTINCT FROM c.project_id
   OR NEW.skill_id<>p.skill_id OR NEW.estimated_tokens<>p.max_input_tokens::bigint+p.max_output_tokens
   OR NEW.month_start<>date_trunc('month',clock_timestamp() AT TIME ZONE 'UTC')::date THEN
   RAISE EXCEPTION 'budget reservation must bind the exact admitted run' USING ERRCODE='23514';
  END IF;
 ELSIF TG_TABLE_NAME='agent_budget_usage' THEN
  IF r.state NOT IN ('running','awaiting_approval','committing') THEN
   RAISE EXCEPTION 'usage requires a started live execution' USING ERRCODE='23514';
  END IF;
 ELSE
  IF r.state NOT IN ('failed','cancelled','interrupted') OR r.version<>NEW.run_version
   OR EXISTS(SELECT 1 FROM agent_budget_usage WHERE run_id=r.id)
   OR NOT EXISTS(SELECT 1 FROM agent_run_events WHERE run_id=r.id AND type='run.started')
   OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.reconciled_by) THEN
   RAISE EXCEPTION 'reconciliation requires an exact terminal run with unknown usage' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['agent_budget_policies','agent_budget_reservations','agent_budget_usage','agent_budget_reconciliations'] LOOP
  EXECUTE format('CREATE TRIGGER budget_binding BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION validate_agent_budget_binding()',t);
  EXECUTE format('CREATE TRIGGER budget_immutable BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()',t);
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
   EXECUTE format('GRANT SELECT,INSERT ON %I TO copypaste_app',t);
   EXECUTE format('REVOKE UPDATE,DELETE ON %I FROM copypaste_app',t);
  END IF;
 END LOOP;
END; $$;
-- Settlement follows durable execution state. No crash window or polling write is
-- needed to release unused reservations. Unknown started usage stays conservative.
CREATE VIEW agent_budget_entries AS
 SELECT b.*,r.version AS run_version,r.state AS run_state,
  CASE WHEN r.state IN ('queued','running','awaiting_approval','committing') THEN 'reserved'
   WHEN z.run_id IS NOT NULL THEN 'reconciled' WHEN u.run_id IS NOT NULL THEN 'settled'
   WHEN started.yes THEN 'pending' ELSE 'released' END AS status,
  CASE WHEN r.state IN ('queued','running','awaiting_approval','committing')
   THEN greatest(b.estimated_tokens,coalesce(u.input_tokens+u.output_tokens,0)) ELSE 0 END AS reserved_tokens,
  CASE WHEN r.state IN ('queued','running','awaiting_approval','committing') THEN 0
   ELSE coalesce(z.input_tokens+z.output_tokens,u.input_tokens+u.output_tokens,
    CASE WHEN started.yes THEN b.estimated_tokens ELSE 0 END) END AS charged_tokens,
  CASE WHEN r.state NOT IN ('queued','running','awaiting_approval','committing')
    AND started.yes AND u.run_id IS NULL AND z.run_id IS NULL THEN b.estimated_tokens ELSE 0 END AS pending_tokens,
  coalesce(z.input_tokens,u.input_tokens) AS input_tokens,coalesce(z.output_tokens,u.output_tokens) AS output_tokens,
  z.evidence_reference,z.reason AS reconciliation_reason,z.reconciled_by,z.created_at AS reconciled_at
 FROM agent_budget_reservations b JOIN agent_runs r ON r.id=b.run_id
 LEFT JOIN agent_budget_usage u ON u.run_id=b.run_id LEFT JOIN agent_budget_reconciliations z ON z.run_id=b.run_id
 CROSS JOIN LATERAL(SELECT EXISTS(SELECT 1 FROM agent_run_events e WHERE e.run_id=b.run_id AND e.type='run.started') AS yes) started;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT ON agent_budget_entries TO copypaste_app;
END IF; END; $$;
