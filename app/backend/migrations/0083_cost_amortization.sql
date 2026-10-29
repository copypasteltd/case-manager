-- Optional, explicitly published allocation schedules. Incurred and cash
-- facts are unchanged. Every version, monthly entry and project share is fixed.
CREATE TABLE cost_amortization_versions (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 cost_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 state text NOT NULL CHECK(state IN ('active','stopped')),
 content text NOT NULL CHECK(octet_length(content)<=2097152),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[a-f0-9]{64}$'),
 period_count integer NOT NULL CHECK(period_count BETWEEN 1 AND 120),
 actor_id uuid NOT NULL, session_id uuid NOT NULL, mfa_verified_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 write_txid bigint NOT NULL DEFAULT txid_current(),
 UNIQUE(org_id,id), UNIQUE(org_id,cost_id,version),
 FOREIGN KEY(org_id,cost_id) REFERENCES actual_cost_entries(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 CHECK(encode(sha256(convert_to(content,'UTF8')),'hex')=content_sha256)
);
CREATE INDEX cost_amortization_history ON cost_amortization_versions(org_id,cost_id,created_at DESC,id DESC);
CREATE TABLE cost_amortization_entries (
 id uuid PRIMARY KEY, reversal_id uuid NOT NULL UNIQUE CHECK(id<>reversal_id),
 org_id uuid NOT NULL, plan_id uuid NOT NULL,
 seq integer NOT NULL CHECK(seq BETWEEN 1 AND 120),
 service_from date NOT NULL, service_to date NOT NULL, recognized_on date NOT NULL,
 amount numeric(20,6) NOT NULL CHECK(amount>=0),
 UNIQUE(org_id,id), UNIQUE(org_id,plan_id,seq),
 FOREIGN KEY(org_id,plan_id) REFERENCES cost_amortization_versions(org_id,id),
 CHECK(service_to>service_from AND recognized_on=service_to-1)
);
CREATE TABLE cost_amortization_shares (
 org_id uuid NOT NULL, entry_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 0 AND 99),
 project_id uuid, amount numeric(20,6) NOT NULL CHECK(amount>0),
 PRIMARY KEY(org_id,entry_id,position),
 FOREIGN KEY(org_id,entry_id) REFERENCES cost_amortization_entries(org_id,id),
 FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE TRIGGER cost_amortization_version_immutable BEFORE UPDATE OR DELETE ON cost_amortization_versions
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER cost_amortization_entry_immutable BEFORE UPDATE OR DELETE ON cost_amortization_entries
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER cost_amortization_share_immutable BEFORE UPDATE OR DELETE ON cost_amortization_shares
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_cost_amortization_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE writer bigint; active boolean;
BEGIN
 IF TG_TABLE_NAME='cost_amortization_entries' THEN
  SELECT write_txid,state='active' INTO writer,active FROM cost_amortization_versions WHERE org_id=NEW.org_id AND id=NEW.plan_id;
 ELSE
  SELECT v.write_txid,v.state='active' INTO writer,active FROM cost_amortization_entries e
   JOIN cost_amortization_versions v ON v.org_id=e.org_id AND v.id=e.plan_id WHERE e.org_id=NEW.org_id AND e.id=NEW.entry_id;
 END IF;
 IF writer IS DISTINCT FROM txid_current() OR active IS DISTINCT FROM true THEN
  RAISE EXCEPTION 'schedule children belong to the creating transaction only' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER cost_amortization_entry_writer BEFORE INSERT ON cost_amortization_entries
 FOR EACH ROW EXECUTE FUNCTION guard_cost_amortization_child();
CREATE TRIGGER cost_amortization_share_writer BEFORE INSERT ON cost_amortization_shares
 FOR EACH ROW EXECUTE FUNCTION guard_cost_amortization_child();
CREATE FUNCTION guard_cost_amortization_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; previous cost_amortization_versions; source actual_cost_entries; revision cost_revisions;
BEGIN
 SELECT * INTO source FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=NEW.cost_id FOR UPDATE;
 SELECT * INTO previous FROM cost_amortization_versions WHERE org_id=NEW.org_id AND cost_id=NEW.cost_id ORDER BY version DESC LIMIT 1;
 IF NEW.version<>coalesce(previous.version,0)+1 OR NEW.write_txid<>txid_current()
  OR NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id AND u.status='active'
    AND s.mfa_verified_at=NEW.mfa_verified_at) THEN
  RAISE EXCEPTION 'schedule requires next version and attributed interactive session' USING ERRCODE='23514';
 END IF;
 body:=NEW.content::jsonb;
 IF NEW.state='stopped' THEN
  IF previous.state IS DISTINCT FROM 'active' OR previous.content<>NEW.content OR previous.period_count<>NEW.period_count THEN
   RAISE EXCEPTION 'stopping retains the exact preceding schedule' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO revision FROM cost_revisions WHERE org_id=NEW.org_id AND id=source.current_revision_id AND cost_id=source.id;
 IF source.state IS DISTINCT FROM 'confirmed' OR source.entry_kind<>'expense'
  OR jsonb_typeof(body) IS DISTINCT FROM 'object'
  OR NOT (body ?& ARRAY['cost_id','cost_version','cost_revision_id','cost_sha256','currency','exponent','amount','method','rounding','service_from','service_to','periods'])
  OR body->>'cost_id' IS DISTINCT FROM NEW.cost_id::text OR (body->>'cost_version')::bigint IS DISTINCT FROM source.version
  OR body->>'cost_revision_id' IS DISTINCT FROM source.current_revision_id::text
  OR body->>'cost_sha256' IS DISTINCT FROM revision.content_hash
  OR body->>'currency' IS DISTINCT FROM revision.currency OR (body->>'amount')::numeric IS DISTINCT FROM revision.amount
  OR body->>'rounding' IS DISTINCT FROM 'allocation_cumulative_half_up'
  OR coalesce(body->>'method','') NOT IN ('monthly_equal','daily_weighted')
  OR body->>'service_from' IS NULL OR body->>'service_to' IS NULL
  OR (body->>'service_from')::date NOT BETWEEN date '1900-01-01' AND date '2200-12-31'
  OR (body->>'service_to')::date NOT BETWEEN date '1900-01-01' AND date '2200-12-31'
  OR (body->>'exponent')::int IS DISTINCT FROM (SELECT exponent FROM currency_definitions WHERE code=revision.currency)
  OR jsonb_typeof(body->'periods') IS DISTINCT FROM 'array' OR jsonb_array_length(body->'periods')<>NEW.period_count
  OR (SELECT count(*) FROM jsonb_object_keys(body))<>12 THEN
  RAISE EXCEPTION 'schedule must pin the exact confirmed original cost and explicit method' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER cost_amortization_version_binding BEFORE INSERT ON cost_amortization_versions
 FOR EACH ROW EXECUTE FUNCTION guard_cost_amortization_version();
CREATE FUNCTION validate_cost_amortization_schedule() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; item jsonb; actual jsonb; previous_to date; source_total numeric; planned_total numeric;
BEGIN
 IF NEW.state='stopped' THEN RETURN NULL; END IF;
 body:=NEW.content::jsonb;
 IF (SELECT count(*) FROM cost_amortization_entries WHERE org_id=NEW.org_id AND plan_id=NEW.id)<>NEW.period_count
  OR (SELECT sum(amount) FROM cost_amortization_entries WHERE org_id=NEW.org_id AND plan_id=NEW.id)<>(body->>'amount')::numeric THEN
  RAISE EXCEPTION 'schedule months must conserve the exact original amount' USING ERRCODE='23514';
 END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(body->'periods') LOOP
  SELECT jsonb_build_object('sequence',e.seq,'service_from',e.service_from::text,'service_to',e.service_to::text,
   'recognized_on',e.recognized_on::text,'amount',e.amount::text,'shares',coalesce((
    SELECT jsonb_agg(jsonb_build_object('project_id',s.project_id,'amount',s.amount::text) ORDER BY s.position)
    FROM cost_amortization_shares s WHERE s.org_id=e.org_id AND s.entry_id=e.id),'[]'::jsonb))
  INTO actual FROM cost_amortization_entries e WHERE e.org_id=NEW.org_id AND e.plan_id=NEW.id AND e.seq=(item->>'sequence')::int;
  IF actual IS DISTINCT FROM item OR (previous_to IS NOT NULL AND previous_to<>(item->>'service_from')::date)
   OR (SELECT coalesce(sum(s.amount),0) FROM cost_amortization_shares s JOIN cost_amortization_entries e ON e.org_id=s.org_id AND e.id=s.entry_id
       WHERE e.org_id=NEW.org_id AND e.plan_id=NEW.id AND e.seq=(item->>'sequence')::int)<>(item->>'amount')::numeric THEN
   RAISE EXCEPTION 'schedule dates, periods and shares must match immutable content' USING ERRCODE='23514';
  END IF;
  previous_to:=(item->>'service_to')::date;
 END LOOP;
 IF (body->'periods'->0->>'service_from')::date<>(body->>'service_from')::date
  OR previous_to<>(body->>'service_to')::date OR previous_to<=(body->>'service_from')::date
  OR previous_to-(body->>'service_from')::date>3660 THEN
  RAISE EXCEPTION 'schedule requires the entire exact service interval' USING ERRCODE='23514';
 END IF;
 IF EXISTS(
  SELECT 1 FROM (
   SELECT s.project_id,sum(s.amount) amount FROM cost_amortization_shares s JOIN cost_amortization_entries e ON e.org_id=s.org_id AND e.id=s.entry_id
   WHERE e.org_id=NEW.org_id AND e.plan_id=NEW.id GROUP BY s.project_id
  ) planned FULL JOIN (
   SELECT project_id,sum(amount) amount FROM cost_allocations WHERE org_id=NEW.org_id AND revision_id=(body->>'cost_revision_id')::uuid GROUP BY project_id
  ) source ON coalesce(planned.project_id::text,'company')=coalesce(source.project_id::text,'company')
  WHERE coalesce(planned.amount,0)<>coalesce(source.amount,0)
 ) THEN
  RAISE EXCEPTION 'each project and company share must conserve its original amount' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER cost_amortization_schedule_complete AFTER INSERT ON cost_amortization_versions
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_cost_amortization_schedule();
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'finance_amortization_manager','费用摊销管理人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('cost.read'),('cost.amortize'),('report.read')) a(action)
 WHERE key='finance_amortization_manager' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON cost_amortization_versions,cost_amortization_entries,cost_amortization_shares TO copypaste_app;
 REVOKE UPDATE,DELETE ON cost_amortization_versions,cost_amortization_entries,cost_amortization_shares FROM copypaste_app;
END IF; END; $$;
