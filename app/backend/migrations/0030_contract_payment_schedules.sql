ALTER TABLE contract_revisions ADD COLUMN payment_schedule jsonb;
ALTER TABLE contract_revisions DROP CONSTRAINT contract_revisions_payload_schema_version_check;
ALTER TABLE contract_revisions ADD CONSTRAINT contract_revisions_payload_schema_version_check CHECK(payload_schema_version IN (1,2));
ALTER TABLE contract_revisions ADD CONSTRAINT contract_revision_schedule_schema CHECK(
    (payload_schema_version=1 AND payment_schedule IS NULL) OR payload_schema_version=2);
ALTER TABLE contract_revisions ADD CONSTRAINT contract_schedule_shape CHECK(payment_schedule IS NULL OR ((
    jsonb_typeof(payment_schedule)='object' AND payment_schedule->>'mode' IN ('scheduled','no_payment')
    AND jsonb_typeof(payment_schedule->'items')='array' AND jsonb_array_length(payment_schedule->'items')<=100
    AND ((payment_schedule->>'mode'='scheduled' AND jsonb_array_length(payment_schedule->'items')>0)
        OR (payment_schedule->>'mode'='no_payment' AND jsonb_array_length(payment_schedule->'items')=0))) IS TRUE));

CREATE FUNCTION validate_contract_schedule_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p jsonb; item jsonb; v numeric; total numeric:=0; known integer:=0; exponent integer;
BEGIN
    p:=NEW.payment_schedule;
    IF p IS NULL THEN RETURN NEW; END IF;
    IF jsonb_typeof(p->'reason') IS DISTINCT FROM 'string' OR length(p->>'reason')>20000 THEN
        RAISE EXCEPTION 'schedule reason is required' USING ERRCODE='23514'; END IF;
    IF p->>'mode'='no_payment' THEN
        IF length(trim(p->>'reason'))=0 OR (NEW.direction IS NOT NULL AND NEW.direction<>'none') OR (NEW.amount IS NOT NULL AND NEW.amount<>0) THEN
            RAISE EXCEPTION 'no payment terms must be explicit and consistent' USING ERRCODE='23514'; END IF;
    ELSE
        IF NEW.direction='none' THEN RAISE EXCEPTION 'scheduled contract needs money direction' USING ERRCODE='23514'; END IF;
        SELECT c.exponent INTO exponent FROM currency_definitions c WHERE code=NEW.currency;
        FOR item IN SELECT value FROM jsonb_array_elements(p->'items') LOOP
            IF jsonb_typeof(item) IS DISTINCT FROM 'object' OR NOT item ?& ARRAY['id','title','amount','due_date','conditions']
                OR jsonb_typeof(item->'id') IS DISTINCT FROM 'string' OR jsonb_typeof(item->'title') IS DISTINCT FROM 'string'
                OR length(trim(item->>'title')) NOT BETWEEN 1 AND 200 OR jsonb_typeof(item->'conditions') IS DISTINCT FROM 'string'
                OR length(item->>'conditions')>20000 THEN RAISE EXCEPTION 'invalid schedule item' USING ERRCODE='23514'; END IF;
            PERFORM (item->>'id')::uuid;
            IF jsonb_typeof(item->'amount') NOT IN ('null','string') OR jsonb_typeof(item->'due_date') NOT IN ('null','string') THEN
                RAISE EXCEPTION 'invalid schedule item amount or date' USING ERRCODE='23514'; END IF;
            IF item->>'amount' IS NOT NULL THEN
                IF NEW.currency IS NULL OR item->>'amount' !~ '^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$' THEN
                    RAISE EXCEPTION 'invalid schedule currency or amount' USING ERRCODE='23514'; END IF;
                v:=(item->>'amount')::numeric;
                IF v<=0 OR v*power(10::numeric,exponent)<>trunc(v*power(10::numeric,exponent)) THEN
                    RAISE EXCEPTION 'invalid positive amount or currency precision' USING ERRCODE='23514'; END IF;
                total:=total+v; known:=known+1;
            END IF;
            IF item->>'due_date' IS NOT NULL THEN
                IF item->>'due_date' !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN RAISE EXCEPTION 'invalid date' USING ERRCODE='23514'; END IF;
                PERFORM (item->>'due_date')::date;
            END IF;
        END LOOP;
        IF known=jsonb_array_length(p->'items') AND NEW.amount IS NOT NULL AND total<>NEW.amount THEN
            RAISE EXCEPTION 'schedule total must equal contract total' USING ERRCODE='23514'; END IF;
        IF (SELECT count(DISTINCT (value->>'id')::uuid) FROM jsonb_array_elements(p->'items'))<>jsonb_array_length(p->'items') THEN
            RAISE EXCEPTION 'duplicate schedule item identity' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_schedule_revision_validate BEFORE INSERT ON contract_revisions FOR EACH ROW EXECUTE FUNCTION validate_contract_schedule_revision();
CREATE FUNCTION contract_schedule_ready() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r contract_revisions%ROWTYPE;
BEGIN
    IF NEW.lifecycle IN ('pending_signature','effective') THEN
        SELECT * INTO r FROM contract_revisions WHERE org_id=NEW.org_id AND contract_id=NEW.id AND id=NEW.current_revision_id;
        IF r.payment_schedule->>'mode'='scheduled' AND (r.direction NOT IN ('receivable','payable') OR r.direction IS NULL OR r.currency IS NULL OR r.amount IS NULL
            OR EXISTS(SELECT 1 FROM jsonb_array_elements(r.payment_schedule->'items') p WHERE p->>'amount' IS NULL OR p->>'due_date' IS NULL)) THEN
            RAISE EXCEPTION 'approved schedule must be complete' USING ERRCODE='23514'; END IF;
        IF r.payment_schedule->>'mode'='no_payment' AND (r.direction IS DISTINCT FROM 'none' OR r.amount IS DISTINCT FROM 0) THEN
            RAISE EXCEPTION 'approved no payment terms must be complete' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_schedule_readiness BEFORE UPDATE ON contracts FOR EACH ROW EXECUTE FUNCTION contract_schedule_ready();

CREATE TABLE contract_milestones (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, contract_id uuid NOT NULL, revision_id uuid NOT NULL,
    plan_item_id uuid NOT NULL, position integer NOT NULL CHECK(position BETWEEN 1 AND 100),
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200), direction text NOT NULL CHECK(direction IN ('receivable','payable')),
    currency text NOT NULL REFERENCES currency_definitions(code), amount numeric(20,6) NOT NULL CHECK(amount>0),
    due_date date NOT NULL, conditions text NOT NULL CHECK(length(conditions)<=20000), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,contract_id,revision_id,plan_item_id), UNIQUE(org_id,contract_id,revision_id,position),
    FOREIGN KEY(org_id,contract_id,revision_id) REFERENCES contract_revisions(org_id,contract_id,id)
);
CREATE TRIGGER contract_milestone_immutable BEFORE UPDATE OR DELETE ON contract_milestones FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_contract_milestone() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS(SELECT 1 FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.contract_id=c.id AND r.id=c.active_revision_id
        CROSS JOIN LATERAL jsonb_array_elements(r.payment_schedule->'items') WITH ORDINALITY p(item,position)
        WHERE c.org_id=NEW.org_id AND c.id=NEW.contract_id AND c.lifecycle='effective' AND c.active_revision_id=NEW.revision_id
        AND r.payment_schedule->>'mode'='scheduled' AND p.position=NEW.position AND (p.item->>'id')::uuid=NEW.plan_item_id
        AND p.item->>'title'=NEW.title AND r.direction=NEW.direction AND r.currency=NEW.currency
        AND (p.item->>'amount')::numeric=NEW.amount AND (p.item->>'due_date')::date=NEW.due_date AND p.item->>'conditions'=NEW.conditions)
    THEN RAISE EXCEPTION 'execution milestone must match exact approved schedule' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_milestone_validate BEFORE INSERT ON contract_milestones FOR EACH ROW EXECUTE FUNCTION validate_contract_milestone();
CREATE FUNCTION materialize_contract_milestones() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r contract_revisions%ROWTYPE;
BEGIN
    IF OLD.lifecycle='pending_signature' AND NEW.lifecycle='effective' THEN
        SELECT * INTO r FROM contract_revisions WHERE org_id=NEW.org_id AND contract_id=NEW.id AND id=NEW.active_revision_id;
        IF r.payment_schedule->>'mode'='scheduled' THEN
            INSERT INTO contract_milestones(id,org_id,contract_id,revision_id,plan_item_id,position,title,direction,currency,amount,due_date,conditions)
                SELECT gen_random_uuid(),NEW.org_id,NEW.id,r.id,(p.item->>'id')::uuid,p.position,p.item->>'title',r.direction,r.currency,
                    (p.item->>'amount')::numeric,(p.item->>'due_date')::date,p.item->>'conditions'
                FROM jsonb_array_elements(r.payment_schedule->'items') WITH ORDINALITY p(item,position);
        END IF;
    END IF;
    RETURN NULL;
END; $$;
CREATE TRIGGER contract_milestones_activate AFTER UPDATE ON contracts FOR EACH ROW EXECUTE FUNCTION materialize_contract_milestones();
CREATE FUNCTION contract_milestone_set_complete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts%ROWTYPE; expected integer; actual integer;
BEGIN
    SELECT * INTO c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.id;
    IF c.lifecycle='effective' THEN
        SELECT coalesce(jsonb_array_length(payment_schedule->'items'),0) INTO expected FROM contract_revisions
            WHERE org_id=c.org_id AND contract_id=c.id AND id=c.active_revision_id;
        SELECT count(*) INTO actual FROM contract_milestones WHERE org_id=c.org_id AND contract_id=c.id AND revision_id=c.active_revision_id;
        IF expected<>actual THEN RAISE EXCEPTION 'effective contract requires complete fixed milestone set' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER contract_milestones_complete AFTER UPDATE ON contracts DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION contract_milestone_set_complete();
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON contract_milestones TO copypaste_app;
        REVOKE UPDATE,DELETE ON contract_milestones FROM copypaste_app;
    END IF;
END; $$;
