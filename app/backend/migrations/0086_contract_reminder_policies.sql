-- Explicit organization policies override deployment fallback, including disable.
CREATE TABLE notification_contract_policies (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 enabled boolean NOT NULL, timezone text NOT NULL,
 days_before integer[] NOT NULL,
 effective_on date NOT NULL,
 created_by uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,version),
 FOREIGN KEY(org_id,created_by) REFERENCES users(org_id,id),
 CHECK(cardinality(days_before) BETWEEN 0 AND 12),
 CHECK(NOT enabled OR cardinality(days_before)>0)
);
CREATE INDEX notification_contract_policy_history ON notification_contract_policies(org_id,version DESC);
CREATE TRIGGER notification_contract_policy_immutable BEFORE UPDATE OR DELETE ON notification_contract_policies
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION guard_notification_contract_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected bigint; normalized integer[];
BEGIN
 PERFORM id FROM organizations WHERE id=NEW.org_id FOR UPDATE;
 SELECT coalesce(max(version),0)+1 INTO expected FROM notification_contract_policies WHERE org_id=NEW.org_id;
 SELECT coalesce(array_agg(DISTINCT d ORDER BY d),'{}'::integer[]) INTO normalized FROM unnest(NEW.days_before) d;
 IF NEW.version<>expected OR NEW.days_before IS DISTINCT FROM normalized
  OR EXISTS(SELECT 1 FROM unnest(NEW.days_before) d WHERE d IS NULL OR d<0 OR d>365)
  OR NEW.timezone IN ('','Local') OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.timezone)
  OR NEW.effective_on IS DISTINCT FROM (NEW.created_at AT TIME ZONE NEW.timezone)::date THEN
  RAISE EXCEPTION 'contract reminder policy must use the next revision and explicit valid business rules' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_contract_policy_input BEFORE INSERT ON notification_contract_policies
 FOR EACH ROW EXECUTE FUNCTION guard_notification_contract_policy();
CREATE FUNCTION validate_notification_contract_policy_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM audit_events WHERE org_id=NEW.org_id AND resource_id=NEW.id
  AND resource_version=NEW.version AND actor_id=NEW.created_by AND action='notification.contract_policy.publish')
 OR NOT EXISTS(SELECT 1 FROM outbox_events WHERE org_id=NEW.org_id AND aggregate_id=NEW.id
  AND aggregate_version=NEW.version AND event_type='notification.contract_policy.publish') THEN
  RAISE EXCEPTION 'contract reminder policy requires atomic audit and outbox' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER notification_contract_policy_evidence AFTER INSERT ON notification_contract_policies
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_notification_contract_policy_evidence();

ALTER TABLE notification_due_events ADD COLUMN policy_id uuid;
ALTER TABLE notification_due_events ADD CONSTRAINT notification_due_policy_fk
 FOREIGN KEY(org_id,policy_id) REFERENCES notification_contract_policies(org_id,id);
CREATE FUNCTION guard_notification_due_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current notification_contract_policies%ROWTYPE;
BEGIN
 PERFORM id FROM organizations WHERE id=NEW.org_id FOR SHARE;
 SELECT * INTO current FROM notification_contract_policies WHERE org_id=NEW.org_id ORDER BY version DESC LIMIT 1;
 IF (current.id IS NULL AND NEW.policy_id IS NOT NULL)
  OR (current.id IS NOT NULL AND (NOT current.enabled OR NEW.policy_id IS DISTINCT FROM current.id
   OR NEW.timezone<>current.timezone OR NOT NEW.days_before=ANY(current.days_before)
   OR NEW.due_on-NEW.days_before<current.effective_on
   OR NOT EXISTS(SELECT 1 FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.event_id
    AND payload->>'policy_id'=current.id::text AND payload->>'policy_version'=current.version::text))) THEN
  RAISE EXCEPTION 'contract reminder must bind the current enabled policy and its activation boundary' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER notification_due_policy BEFORE INSERT ON notification_due_events
 FOR EACH ROW EXECUTE FUNCTION guard_notification_due_policy();

INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'notification_policy_reader','合同提醒策略查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'notification_policy_manager','合同提醒策略管理人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'notification.policy.read' FROM roles WHERE key IN ('notification_policy_reader','notification_policy_manager');
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'notification.policy.manage' FROM roles WHERE key='notification_policy_manager';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON notification_contract_policies TO copypaste_app;
 END IF;
END; $$;
