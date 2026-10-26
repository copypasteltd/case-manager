-- One execution attempt: losing a model process must never automatically repeat writes.
CREATE TABLE agent_run_jobs (
 org_id uuid NOT NULL, run_id uuid PRIMARY KEY REFERENCES agent_runs(id),
 job_id uuid NOT NULL UNIQUE REFERENCES jobs(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION validate_agent_run_job() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; j jobs;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id;
 SELECT * INTO j FROM jobs WHERE id=NEW.job_id;
 IF r.org_id<>NEW.org_id OR j.org_id<>NEW.org_id OR j.kind<>'agent.execute.v1' OR j.lane<>'agent'
 OR j.max_attempts<>1 OR j.payload_version<>1 OR j.payload<>jsonb_build_object('run_id',r.id::text)
 OR j.dedup_key<>'agent.run:'||r.id::text THEN
  RAISE EXCEPTION 'agent jobs must bind one exact run and a single attempt' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_job_binding BEFORE INSERT ON agent_run_jobs FOR EACH ROW EXECUTE FUNCTION validate_agent_run_job();
CREATE TRIGGER agent_job_immutable BEFORE UPDATE OR DELETE ON agent_run_jobs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
-- Existing queued records are admitted to the same durable queue, not discarded.
INSERT INTO jobs(id,org_id,kind,lane,dedup_key,payload_version,payload,max_attempts,state)
 SELECT gen_random_uuid(),org_id,'agent.execute.v1','agent','agent.run:'||id::text,1,jsonb_build_object('run_id',id::text),1,'queued'
 FROM agent_runs WHERE state='queued';
INSERT INTO agent_run_jobs(org_id,run_id,job_id)
 SELECT r.org_id,r.id,j.id FROM agent_runs r JOIN jobs j ON j.org_id=r.org_id AND j.dedup_key='agent.run:'||r.id::text;

CREATE TABLE agent_execution_leases (
 run_id uuid PRIMARY KEY REFERENCES agent_runs(id), org_id uuid NOT NULL,
 job_id uuid NOT NULL UNIQUE REFERENCES jobs(id), fencing_token bigint NOT NULL CHECK(fencing_token>0),
 lease_owner text NOT NULL CHECK(length(lease_owner) BETWEEN 1 AND 128),
 credential_sha256 text NOT NULL UNIQUE CHECK(credential_sha256 ~ '^[0-9a-f]{64}$'),
 authority_stamp text NOT NULL UNIQUE CHECK(authority_stamp ~ '^[0-9a-f]{64}$'),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(expires_at>created_at)
);
CREATE FUNCTION validate_agent_execution_lease() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; j jobs;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id;
 SELECT * INTO j FROM jobs WHERE id=NEW.job_id;
 IF r.org_id<>NEW.org_id OR j.org_id<>NEW.org_id OR NOT EXISTS(SELECT 1 FROM agent_run_jobs x WHERE x.run_id=r.id AND x.job_id=j.id AND x.org_id=r.org_id)
 OR r.state<>'queued' OR j.state<>'running' OR j.fencing_token<>NEW.fencing_token OR j.lease_owner<>NEW.lease_owner
 OR NEW.expires_at>r.expires_at OR j.lease_until<=clock_timestamp() THEN
  RAISE EXCEPTION 'execution lease requires an exact live one-attempt job' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_execution_lease_binding BEFORE INSERT ON agent_execution_leases FOR EACH ROW EXECUTE FUNCTION validate_agent_execution_lease();
CREATE TRIGGER agent_execution_lease_immutable BEFORE UPDATE OR DELETE ON agent_execution_leases FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE agent_execution_verifications (
 run_id uuid PRIMARY KEY REFERENCES agent_runs(id), org_id uuid NOT NULL,
 authority_stamp text NOT NULL CHECK(authority_stamp ~ '^[0-9a-f]{64}$'),
 configuration_sha256 text NOT NULL CHECK(configuration_sha256 ~ '^[0-9a-f]{64}$'),
 proof_sha256 text NOT NULL CHECK(proof_sha256 ~ '^[0-9a-f]{64}$'),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(expires_at>created_at)
);

CREATE TABLE agent_tool_receipts (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL REFERENCES agent_runs(id),
 invocation_key text NOT NULL CHECK(length(invocation_key) BETWEEN 1 AND 200),
 authority_stamp text NOT NULL CHECK(authority_stamp ~ '^[0-9a-f]{64}$'),
 tool text NOT NULL CHECK(tool IN ('read_resource','search_resources','inspect_operation')),
 input_sha256 text NOT NULL CHECK(input_sha256 ~ '^[0-9a-f]{64}$'),
 source_snapshots jsonb NOT NULL CHECK(jsonb_typeof(source_snapshots)='array' AND jsonb_array_length(source_snapshots)<=20),
 output jsonb NOT NULL CHECK(jsonb_typeof(output)='object' AND octet_length(output::text)<=131072),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(run_id,invocation_key)
);
CREATE TABLE agent_run_results (
 run_id uuid PRIMARY KEY REFERENCES agent_runs(id), org_id uuid NOT NULL,
 authority_stamp text NOT NULL CHECK(authority_stamp ~ '^[0-9a-f]{64}$'),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=131072),
 usage jsonb NOT NULL CHECK(jsonb_typeof(usage)='object' AND octet_length(usage::text)<=4096),
 source_snapshots jsonb NOT NULL CHECK(jsonb_typeof(source_snapshots)='array' AND jsonb_array_length(source_snapshots)<=20),
 body text NOT NULL CHECK(length(btrim(body)) BETWEEN 1 AND 20000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX agent_run_assistant_message ON conversation_messages(run_id) WHERE role='assistant';
CREATE TABLE agent_execution_observations (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL REFERENCES agent_runs(id),
 authority_stamp text NOT NULL CHECK(authority_stamp ~ '^[0-9a-f]{64}$'),
 type text NOT NULL CHECK(type IN ('tool.started','tool.finished','run.interrupted')),
 tool text CHECK(tool IN ('read_resource','search_resources','inspect_operation')),
 safe_code text CHECK(safe_code ~ '^[A-Z][A-Z0-9_]{0,99}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((type='run.interrupted' AND tool IS NULL AND safe_code IS NOT NULL) OR (type<>'run.interrupted' AND tool IS NOT NULL AND safe_code IS NULL))
);
CREATE FUNCTION validate_agent_execution_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; l agent_execution_leases;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id;
 SELECT * INTO l FROM agent_execution_leases WHERE run_id=r.id;
 IF l.run_id IS NULL OR r.org_id<>NEW.org_id OR l.org_id<>NEW.org_id OR l.authority_stamp<>NEW.authority_stamp THEN
  RAISE EXCEPTION 'execution evidence requires the exact issued authority' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='agent_run_results' THEN
  IF r.state<>'succeeded' OR r.outcome IS DISTINCT FROM NEW.result->>'status' OR NOT EXISTS(SELECT 1 FROM conversation_messages m WHERE m.run_id=r.id AND m.role='assistant'
   AND m.body=NEW.body AND m.source_snapshots=NEW.source_snapshots AND m.org_id=r.org_id AND m.audience_ids=r.audience_ids) THEN
   RAISE EXCEPTION 'a result requires its exact persisted response and outcome' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_tool_evidence AFTER INSERT ON agent_tool_receipts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_execution_evidence();
CREATE CONSTRAINT TRIGGER agent_result_evidence AFTER INSERT ON agent_run_results DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_execution_evidence();
CREATE CONSTRAINT TRIGGER agent_observation_evidence AFTER INSERT ON agent_execution_observations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_execution_evidence();
CREATE CONSTRAINT TRIGGER agent_verification_evidence AFTER INSERT ON agent_execution_verifications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_execution_evidence();
CREATE TRIGGER agent_tool_receipt_immutable BEFORE UPDATE OR DELETE ON agent_tool_receipts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_run_result_immutable BEFORE UPDATE OR DELETE ON agent_run_results FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_observation_immutable BEFORE UPDATE OR DELETE ON agent_execution_observations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER agent_verification_immutable BEFORE UPDATE OR DELETE ON agent_execution_verifications FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

ALTER TABLE agent_runs ADD COLUMN outcome text CHECK(outcome IN ('draft','pending','awaiting_input','completed','conflict','denied','failed'));
ALTER TABLE agent_runs ADD CONSTRAINT agent_outcome_requires_response CHECK(outcome IS NULL OR state='succeeded');
CREATE FUNCTION validate_agent_response() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs;
BEGIN
 IF TG_TABLE_NAME='agent_runs' THEN SELECT * INTO r FROM agent_runs WHERE id=NEW.id;
 ELSE SELECT * INTO r FROM agent_runs WHERE id=NEW.run_id; END IF;
 IF r.outcome IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_run_results x WHERE x.run_id=r.id AND x.org_id=r.org_id AND x.result->>'status'=r.outcome) THEN
  RAISE EXCEPTION 'run outcome requires a matching durable result' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME='conversation_messages' THEN
  IF NEW.role='assistant' AND NOT EXISTS(SELECT 1 FROM agent_run_results x WHERE x.run_id=r.id AND x.body=NEW.body AND x.source_snapshots=NEW.source_snapshots) THEN
   RAISE EXCEPTION 'assistant messages require a verified result record' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER agent_response_head AFTER INSERT OR UPDATE ON agent_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_response();
CREATE CONSTRAINT TRIGGER agent_response_message AFTER INSERT ON conversation_messages DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_agent_response();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_run_jobs,agent_execution_leases,agent_tool_receipts,agent_run_results,agent_execution_observations,agent_execution_verifications TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_run_jobs,agent_execution_leases,agent_tool_receipts,agent_run_results,agent_execution_observations,agent_execution_verifications FROM copypaste_app;
END IF; END; $$;
