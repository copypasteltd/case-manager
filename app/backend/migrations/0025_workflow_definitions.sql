CREATE TABLE workflow_definitions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    key text NOT NULL CHECK(key~'^[a-z][a-z0-9_.-]{2,63}$'), revision bigint NOT NULL CHECK(revision>0),
    name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    kind text NOT NULL CHECK(kind IN ('contract','purchase','claim','change')),
    category text NOT NULL CHECK(length(trim(category)) BETWEEN 1 AND 50),
    currency text NOT NULL REFERENCES currency_definitions(code),
    min_amount numeric(20,6) NOT NULL CHECK(min_amount>=0), max_amount numeric(20,6),
    enabled boolean NOT NULL, self_approval_policy text NOT NULL CHECK(self_approval_policy IN ('deny','allow')),
    withdrawal_allowed boolean NOT NULL, published_by uuid NOT NULL,
    steps jsonb NOT NULL CHECK(jsonb_typeof(steps)='array' AND jsonb_array_length(steps) BETWEEN 1 AND 10),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,key,revision),
    FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id),
    CHECK(max_amount IS NULL OR max_amount>min_amount)
);
CREATE TABLE workflow_definition_steps (
    org_id uuid NOT NULL, definition_id uuid NOT NULL, step_no integer NOT NULL CHECK(step_no BETWEEN 1 AND 10),
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    assigned_user_id uuid NOT NULL, requires_mfa boolean NOT NULL,
    PRIMARY KEY(definition_id,step_no), UNIQUE(definition_id,assigned_user_id),
    FOREIGN KEY(org_id,definition_id) REFERENCES workflow_definitions(org_id,id),
    FOREIGN KEY(org_id,assigned_user_id) REFERENCES users(org_id,id)
);
CREATE INDEX workflow_current_revision ON workflow_definitions(org_id,key,revision DESC);
CREATE TRIGGER workflow_definition_immutable BEFORE UPDATE OR DELETE ON workflow_definitions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER workflow_definition_step_immutable BEFORE UPDATE OR DELETE ON workflow_definition_steps
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
-- The immutable parent fixes the entire sequence before any normalized rows
-- are inserted. A later transaction cannot append a new step to a published
-- revision by using INSERT on the otherwise append-only child table.
CREATE FUNCTION bind_workflow_definition_step() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected jsonb;
BEGIN
    SELECT steps->(NEW.step_no-1) INTO expected FROM workflow_definitions
        WHERE org_id=NEW.org_id AND id=NEW.definition_id;
    IF expected IS NULL OR expected->>'title' IS DISTINCT FROM NEW.title
        OR expected->>'assigned_user_id' IS DISTINCT FROM NEW.assigned_user_id::text
        OR (expected->>'requires_mfa')::boolean IS DISTINCT FROM NEW.requires_mfa THEN
        RAISE EXCEPTION 'step does not match immutable definition' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER workflow_step_matches_parent BEFORE INSERT ON workflow_definition_steps
    FOR EACH ROW EXECUTE FUNCTION bind_workflow_definition_step();
CREATE FUNCTION require_workflow_definition_steps() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n integer; last_no integer;
BEGIN
    SELECT count(*),max(step_no) INTO n,last_no FROM workflow_definition_steps
        WHERE org_id=NEW.org_id AND definition_id=NEW.id;
    IF n<>jsonb_array_length(NEW.steps) OR last_no<>n THEN
        RAISE EXCEPTION 'definition requires its complete contiguous sequence' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE CONSTRAINT TRIGGER workflow_complete_steps AFTER INSERT ON workflow_definitions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_workflow_definition_steps();
-- No company policy, approver, threshold or permission binding is seeded.
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON workflow_definitions,workflow_definition_steps TO copypaste_app;
    END IF;
END;
$$;
