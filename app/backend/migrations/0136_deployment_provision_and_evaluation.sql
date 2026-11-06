-- Deployment-only receipts. Application roles cannot provision profiles/storage.
CREATE TABLE deployment_configuration_receipts (
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),
 kind text NOT NULL CHECK(kind IN ('initial_storage','agent_profile')),
 input_sha256 text NOT NULL CHECK(input_sha256 ~ '^[a-f0-9]{64}$'),
 subject_id uuid,actor_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,kind,input_sha256),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TRIGGER deployment_configuration_immutable BEFORE UPDATE OR DELETE ON deployment_configuration_receipts
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE agent_skill_releases ADD COLUMN evaluation jsonb;
ALTER TABLE agent_skill_releases ADD CONSTRAINT evaluation_shape CHECK(evaluation IS NULL OR
 (evaluation ?& ARRAY['set_version','set_sha256','results_sha256','reference','passed','failed','skipped'] AND jsonb_typeof(evaluation)='object' AND octet_length(evaluation::text)<=4096
 AND evaluation->>'set_sha256' ~ '^[a-f0-9]{64}$' AND evaluation->>'results_sha256' ~ '^[a-f0-9]{64}$'
 AND length(btrim(evaluation->>'set_version')) BETWEEN 1 AND 200 AND length(btrim(evaluation->>'reference')) BETWEEN 1 AND 200
 AND (evaluation->>'passed')::integer>=0 AND (evaluation->>'failed')::integer>=0 AND (evaluation->>'skipped')::integer>=0
 AND (evaluation->>'passed')::integer+(evaluation->>'failed')::integer+(evaluation->>'skipped')::integer BETWEEN 1 AND 10000));
CREATE FUNCTION require_new_skill_evaluation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.stage='published' AND (NEW.evaluation IS NULL OR coalesce((NEW.evaluation->>'passed')::integer,0)<1
  OR coalesce((NEW.evaluation->>'failed')::integer,-1)<>0 OR coalesce((NEW.evaluation->>'skipped')::integer,-1)<>0) THEN
  RAISE EXCEPTION 'new published release requires exact external assessment identity and results' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER skill_evaluation_required BEFORE INSERT ON agent_skill_releases
 FOR EACH ROW EXECUTE FUNCTION require_new_skill_evaluation();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 REVOKE ALL ON deployment_configuration_receipts FROM copypaste_app;
 END IF; END $$;
