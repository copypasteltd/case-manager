-- Renew only execution authority, without rewriting the first approval or its
-- observation window. Every renewal is immutable and fixes the remaining items.
CREATE TABLE object_retirement_authorizations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK(sequence>0), plan_version bigint NOT NULL CHECK(plan_version>1),
    previous_id uuid, snapshot_sha256 text NOT NULL CHECK(snapshot_sha256~'^[a-f0-9]{64}$'),
    items jsonb NOT NULL CHECK(jsonb_typeof(items)='array' AND jsonb_array_length(items) BETWEEN 1 AND 2),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    authorized_at timestamptz NOT NULL DEFAULT clock_timestamp(), authorized_by uuid NOT NULL,
    session_id uuid NOT NULL, channel text NOT NULL CHECK(channel IN ('web','android','wechat','douyin')),
    auth_version bigint NOT NULL, org_version bigint NOT NULL,
    UNIQUE(org_id,id), UNIQUE(org_id,plan_id,id), UNIQUE(org_id,plan_id,sequence), UNIQUE(org_id,plan_id,plan_version),
    FOREIGN KEY(org_id,plan_id) REFERENCES object_retirement_plans(org_id,id),
    FOREIGN KEY(org_id,authorized_by) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,plan_id,previous_id) REFERENCES object_retirement_authorizations(org_id,plan_id,id)
);
CREATE INDEX retirement_authorization_history ON object_retirement_authorizations(org_id,plan_id,authorized_at,id);
CREATE TRIGGER retirement_authorization_immutable BEFORE UPDATE OR DELETE ON object_retirement_authorizations
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE object_retirement_plans ADD COLUMN execution_authorization_id uuid;
ALTER TABLE object_retirement_plans ADD FOREIGN KEY(org_id,id,execution_authorization_id)
    REFERENCES object_retirement_authorizations(org_id,plan_id,id);
ALTER TABLE object_retirement_receipts ADD COLUMN authorization_id uuid;
ALTER TABLE object_retirement_receipts ADD FOREIGN KEY(org_id,authorization_id) REFERENCES object_retirement_authorizations(org_id,id);

CREATE OR REPLACE FUNCTION protect_retirement_plan_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.version<>OLD.version+1 OR OLD.state IN ('cancelled','completed') OR
       (OLD.state='observing' AND NEW.state NOT IN ('approved','cancelled')) OR
       (OLD.state='approved' AND NEW.state NOT IN ('approved','completed','cancelled')) THEN
        RAISE EXCEPTION 'invalid retirement plan transition' USING ERRCODE='23514';
    END IF;
    IF OLD.approved_at IS NOT NULL AND ROW(NEW.approved_at,NEW.approved_by,NEW.approved_session_id,NEW.approved_channel,NEW.approved_auth_version,NEW.approved_org_version,NEW.approval_reason)
        IS DISTINCT FROM ROW(OLD.approved_at,OLD.approved_by,OLD.approved_session_id,OLD.approved_channel,OLD.approved_auth_version,OLD.approved_org_version,OLD.approval_reason) THEN
        RAISE EXCEPTION 'retirement approval is immutable' USING ERRCODE='23514';
    END IF;
    IF (OLD.state='approved' AND NEW.state='approved') OR NEW.execution_authorization_id IS DISTINCT FROM OLD.execution_authorization_id THEN
        IF OLD.state<>'approved' OR NEW.state<>'approved' OR NEW.execution_authorization_id IS NOT DISTINCT FROM OLD.execution_authorization_id OR
           NOT EXISTS(SELECT 1 FROM object_retirement_authorizations a WHERE a.org_id=NEW.org_id AND a.plan_id=NEW.id
             AND a.id=NEW.execution_authorization_id AND a.plan_version=NEW.version AND a.snapshot_sha256=NEW.snapshot_sha256
             AND a.previous_id IS NOT DISTINCT FROM OLD.execution_authorization_id
             AND a.sequence=coalesce((SELECT sequence FROM object_retirement_authorizations WHERE id=OLD.execution_authorization_id),0)+1) THEN
            RAISE EXCEPTION 'retirement renewal requires new fixed authorization' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON object_retirement_authorizations TO copypaste_app;
    END IF;
END $$;
