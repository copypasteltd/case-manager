-- Forecasts do not create incurred costs, payments or proof of vendor cancellation.
CREATE TABLE subscriptions (
    id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),source_key text NOT NULL,
    project_id uuid,creator_id uuid NOT NULL,classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    state text NOT NULL CHECK(state IN ('draft','active','paused','cancelled','archived')),current_revision_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),cancel_effective_on date,paused_on date,
    created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,source_key),FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    CHECK(source_key~'^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$'),CHECK(classification<>'project_restricted' OR project_id IS NOT NULL),
    CHECK((state IN ('cancelled','archived'))=(cancel_effective_on IS NOT NULL)),CHECK(state<>'paused' OR paused_on IS NOT NULL)
);
CREATE TABLE subscription_revisions (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,subscription_id uuid NOT NULL,seq bigint NOT NULL CHECK(seq>0),base_revision_id uuid,
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    product text GENERATED ALWAYS AS (payload->>'product') STORED NOT NULL CHECK(length(btrim(product)) BETWEEN 1 AND 200),
    supplier_id uuid GENERATED ALWAYS AS ((payload->'supplier'->>'party_id')::uuid) STORED,
    supplier_revision_id uuid GENERATED ALWAYS AS ((payload->'supplier'->>'revision_id')::uuid) STORED,
    owner_id uuid GENERATED ALWAYS AS ((payload->>'owner_id')::uuid) STORED,category text GENERATED ALWAYS AS (payload->>'category') STORED NOT NULL CHECK(length(category)<=50),
    author_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,subscription_id,id),UNIQUE(org_id,subscription_id,seq),
    FOREIGN KEY(org_id,subscription_id) REFERENCES subscriptions(org_id,id),FOREIGN KEY(org_id,subscription_id,base_revision_id) REFERENCES subscription_revisions(org_id,subscription_id,id),
    FOREIGN KEY(org_id,supplier_id,supplier_revision_id) REFERENCES party_revisions(org_id,party_id,id),FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)),CHECK((supplier_id IS NULL)=(supplier_revision_id IS NULL)),
    CHECK(jsonb_typeof(payload->'prices')='array' AND jsonb_array_length(payload->'prices')<=1000),
    CHECK(jsonb_typeof(payload->'reminder_days')='array' AND jsonb_array_length(payload->'reminder_days')<=10),
    CHECK(jsonb_typeof(payload->'vendor_evidence_version_ids')='array' AND jsonb_array_length(payload->'vendor_evidence_version_ids')<=20)
);
ALTER TABLE subscriptions ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES subscription_revisions(org_id,subscription_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE subscription_prices (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,revision_id uuid NOT NULL,position integer NOT NULL CHECK(position BETWEEN 1 AND 1000),
    effective_from date NOT NULL,effective_to date,currency text NOT NULL REFERENCES currency_definitions(code),amount numeric(20,6) NOT NULL CHECK(amount>=0),plan text NOT NULL CHECK(length(plan)<=200),
    UNIQUE(org_id,id),UNIQUE(org_id,revision_id,id),UNIQUE(org_id,revision_id,position),
    FOREIGN KEY(org_id,revision_id) REFERENCES subscription_revisions(org_id,id),CHECK(effective_to IS NULL OR effective_to>effective_from)
);
CREATE TABLE subscription_revision_files (
    org_id uuid NOT NULL,revision_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,revision_id,version_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES subscription_revisions(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE subscription_periods (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,subscription_id uuid NOT NULL,start_date date NOT NULL,end_date date NOT NULL,
    charge_type text NOT NULL CHECK(charge_type='recurring'),current_revision_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    state text NOT NULL CHECK(state IN ('forecast','confirmed','waived','void')),review_fingerprint text,review_reason text,invoice_missing boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(org_id,id),UNIQUE(org_id,subscription_id,id),
    UNIQUE(org_id,subscription_id,start_date,end_date,charge_type),FOREIGN KEY(org_id,subscription_id) REFERENCES subscriptions(org_id,id),CHECK(end_date>start_date),
    CHECK((state='confirmed')=(review_fingerprint IS NOT NULL)),CHECK(review_reason IS NULL OR length(btrim(review_reason)) BETWEEN 1 AND 200)
);
CREATE TABLE subscription_period_revisions (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,period_id uuid NOT NULL,seq bigint NOT NULL CHECK(seq>0),base_revision_id uuid,
    configuration_revision_id uuid NOT NULL,price_id uuid NOT NULL,content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    currency text NOT NULL REFERENCES currency_definitions(code),amount numeric(20,6) NOT NULL CHECK(amount>=0),timezone text NOT NULL,category text NOT NULL,
    owner_id uuid NOT NULL,supplier_id uuid NOT NULL,supplier_revision_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,period_id,id),UNIQUE(org_id,period_id,seq),
    FOREIGN KEY(org_id,period_id) REFERENCES subscription_periods(org_id,id),FOREIGN KEY(org_id,period_id,base_revision_id) REFERENCES subscription_period_revisions(org_id,period_id,id),
    FOREIGN KEY(org_id,configuration_revision_id,price_id) REFERENCES subscription_prices(org_id,revision_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,supplier_id,supplier_revision_id) REFERENCES party_revisions(org_id,party_id,id),CHECK((seq=1)=(base_revision_id IS NULL))
);
ALTER TABLE subscription_periods ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES subscription_period_revisions(org_id,period_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE subscription_events (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,subscription_id uuid NOT NULL,subscription_version bigint NOT NULL CHECK(subscription_version>0),revision_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('draft_created','config_revised','price_changed','activated','paused','resumed','cancelled','archived','restored','periods_generated','period_changed')),
    state text NOT NULL CHECK(state IN ('draft','active','paused','cancelled','archived')),period_id uuid,effective_on date,
    actor_id uuid,session_id uuid,mfa_verified_at timestamptz,job_id uuid,job_fence bigint,job_owner text,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,subscription_id,subscription_version),FOREIGN KEY(org_id,subscription_id,revision_id) REFERENCES subscription_revisions(org_id,subscription_id,id),
    FOREIGN KEY(org_id,subscription_id,period_id) REFERENCES subscription_periods(org_id,subscription_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),FOREIGN KEY(job_id) REFERENCES jobs(id),
    CHECK((actor_id IS NOT NULL AND session_id IS NOT NULL AND job_id IS NULL AND job_fence IS NULL AND job_owner IS NULL) OR (actor_id IS NULL AND session_id IS NULL AND mfa_verified_at IS NULL AND job_id IS NOT NULL AND job_fence>0 AND job_owner IS NOT NULL AND kind='periods_generated')),
    CHECK((kind='period_changed')=(period_id IS NOT NULL)),CHECK((kind IN ('paused','resumed','cancelled'))=(effective_on IS NOT NULL))
);
CREATE TABLE subscription_period_events (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,period_id uuid NOT NULL,period_version bigint NOT NULL CHECK(period_version>0),revision_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('generated','unexpected','reforecast','cost_linked','confirmed','waived','void','reopened')),state text NOT NULL CHECK(state IN ('forecast','confirmed','waived','void')),
    subscription_event_id uuid NOT NULL REFERENCES subscription_events(id) DEFERRABLE INITIALLY DEFERRED,
    cost_link_id uuid,review_fingerprint text,cost_snapshot jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(cost_snapshot)='array' AND jsonb_array_length(cost_snapshot)<=100),acknowledge_difference boolean NOT NULL DEFAULT false,invoice_missing boolean NOT NULL DEFAULT false,
    actor_id uuid,session_id uuid,mfa_verified_at timestamptz,job_id uuid,job_fence bigint,job_owner text,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,period_id,period_version),FOREIGN KEY(org_id,period_id,revision_id) REFERENCES subscription_period_revisions(org_id,period_id,id),FOREIGN KEY(org_id,cost_link_id) REFERENCES cost_links(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),FOREIGN KEY(job_id) REFERENCES jobs(id),
    CHECK((actor_id IS NOT NULL AND session_id IS NOT NULL AND job_id IS NULL AND job_fence IS NULL AND job_owner IS NULL) OR (actor_id IS NULL AND session_id IS NULL AND mfa_verified_at IS NULL AND job_id IS NOT NULL AND job_fence>0 AND job_owner IS NOT NULL AND kind='generated')),
    CHECK((kind='cost_linked')=(cost_link_id IS NOT NULL))
);
CREATE UNIQUE INDEX subscription_period_event_cost_once ON subscription_period_events(org_id,cost_link_id) WHERE cost_link_id IS NOT NULL;
CREATE TABLE subscription_period_event_files (
    org_id uuid NOT NULL,event_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,event_id,version_id),
    FOREIGN KEY(event_id) REFERENCES subscription_period_events(id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE subscription_reminders (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,period_id uuid NOT NULL,days_before integer NOT NULL CHECK(days_before BETWEEN 0 AND 90),remind_on date NOT NULL,recipient_id uuid NOT NULL,
    outbox_id uuid NOT NULL,job_id uuid NOT NULL,job_fence bigint NOT NULL CHECK(job_fence>0),job_owner text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,period_id,days_before),FOREIGN KEY(org_id,period_id) REFERENCES subscription_periods(org_id,id),FOREIGN KEY(org_id,recipient_id) REFERENCES users(org_id,id),
    FOREIGN KEY(outbox_id) REFERENCES outbox_events(id),FOREIGN KEY(job_id) REFERENCES jobs(id)
);
CREATE INDEX subscription_list ON subscriptions(org_id,updated_at DESC,id DESC);
CREATE INDEX subscription_period_date ON subscription_periods(org_id,start_date,id);
CREATE INDEX subscription_period_parent ON subscription_periods(org_id,subscription_id,start_date,id);
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','deliverable','recovery_set'));
CREATE FUNCTION register_subscription_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF TG_TABLE_NAME='subscription_revision_files' THEN
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'subscription_revision',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    ELSE
        IF NOT EXISTS(SELECT 1 FROM subscription_period_events WHERE id=NEW.event_id AND org_id=NEW.org_id) THEN RAISE EXCEPTION 'subscription evidence must share its event organization' USING ERRCODE='23514'; END IF;
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'subscription_period_event',NEW.event_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    END IF; RETURN NEW;
END; $$;
CREATE TRIGGER subscription_file_reference AFTER INSERT ON subscription_revision_files FOR EACH ROW EXECUTE FUNCTION register_subscription_file();
CREATE TRIGGER subscription_event_file_reference AFTER INSERT ON subscription_period_event_files FOR EACH ROW EXECUTE FUNCTION register_subscription_file();

CREATE FUNCTION can_read_subscription(p_org uuid,p_user uuid,p_id uuid,p_channel text,p_ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM subscriptions s WHERE s.org_id=p_org AND s.id=p_id AND can_access(s.org_id,p_user,'subscription.read','subscription',s.id,s.project_id,s.creator_id,s.classification,p_channel,p_ai)
        AND NOT EXISTS(SELECT 1 FROM subscription_periods p JOIN cost_links l ON l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id
            JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id WHERE p.org_id=s.org_id AND p.subscription_id=s.id AND NOT can_read_cost(c.org_id,p_user,c.id,c.current_revision_id,p_channel,p_ai)))
$$;
-- Date-only calendar: anchoring uses the original month/day, never the previous capped date.
CREATE FUNCTION subscription_window_end(payload jsonb,charge_date date) RETURNS date LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE origin date:=(payload->>'first_charge_date')::date; cycle text:=payload->>'cycle'; anchor integer:=(payload->>'anchor_day')::integer;
    days integer:=(payload->>'custom_interval_days')::integer; months integer; base date; following date; expected date;
BEGIN
    IF origin IS NULL OR charge_date<origin THEN RETURN NULL; END IF;
    IF cycle='custom' THEN
        IF days IS NULL OR days<1 OR days>3660 OR (charge_date-origin)%days<>0 THEN RETURN NULL; END IF;
        RETURN charge_date+days;
    END IF;
    months:=(extract(year FROM charge_date)::integer-extract(year FROM origin)::integer)*12+extract(month FROM charge_date)::integer-extract(month FROM origin)::integer;
    IF cycle NOT IN ('month','year') OR anchor IS NULL OR anchor NOT BETWEEN 1 AND 31 OR (cycle='year' AND months%12<>0) THEN RETURN NULL; END IF;
    base:=(date_trunc('month',origin::timestamp)+make_interval(months=>months))::date;
    following:=(base+make_interval(months=>CASE WHEN cycle='year' THEN 12 ELSE 1 END))::date;
    expected:=base+least(anchor,extract(day FROM base+interval '1 month -1 day')::integer)-1;
    IF expected<>charge_date THEN RETURN NULL; END IF;
    RETURN following+least(anchor,extract(day FROM following+interval '1 month -1 day')::integer)-1;
END; $$;
CREATE FUNCTION subscription_actual_fingerprint(p_org uuid,p_period uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT md5(COALESCE(string_agg(c.id::text||':'||c.current_revision_id::text||':'||c.state,',' ORDER BY c.id),'')) FROM cost_links l
        JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id WHERE l.org_id=p_org AND l.source_type='subscription_period' AND l.source_id=p_period
$$;
CREATE FUNCTION subscription_charge_eligible(p_org uuid,p_sub uuid,p_date date) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM subscriptions s WHERE s.org_id=p_org AND s.id=p_sub AND s.state<>'draft' AND (s.cancel_effective_on IS NULL OR p_date<s.cancel_effective_on)
        AND NOT EXISTS(SELECT 1 FROM subscription_events e WHERE e.org_id=s.org_id AND e.subscription_id=s.id AND e.kind='paused' AND e.effective_on<=p_date
            AND NOT EXISTS(SELECT 1 FROM subscription_events r WHERE r.org_id=e.org_id AND r.subscription_id=e.subscription_id AND r.kind='resumed' AND r.subscription_version>e.subscription_version AND r.effective_on<=p_date)))
$$;
CREATE FUNCTION subscription_actual_snapshot(p_org uuid,p_period uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT COALESCE(jsonb_agg(jsonb_build_object('cost_id',c.id,'revision_id',r.id,'state',c.state,'content_hash',r.content_hash,'currency',r.currency,'amount',r.amount::text) ORDER BY c.id),'[]') FROM cost_links l
        JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE l.org_id=p_org AND l.source_type='subscription_period' AND l.source_id=p_period
$$;
CREATE FUNCTION verify_subscription_event_identity() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE s sessions;u users;j jobs;sid uuid;draft_edit boolean:=false; BEGIN
    IF NEW.job_id IS NOT NULL THEN
        IF TG_TABLE_NAME='subscription_events' THEN sid:=NEW.subscription_id; ELSE SELECT subscription_id INTO sid FROM subscription_periods WHERE org_id=NEW.org_id AND id=NEW.period_id; END IF;
        SELECT * INTO j FROM jobs WHERE org_id=NEW.org_id AND id=NEW.job_id FOR UPDATE;
        IF j.id IS NULL OR j.kind<>'subscription.generate.v1' OR j.lane<>'finance' OR j.state<>'running' OR j.lease_until<=clock_timestamp() OR j.fencing_token<>NEW.job_fence OR j.lease_owner IS DISTINCT FROM NEW.job_owner
            OR j.payload->>'subscription_id' IS DISTINCT FROM sid::text THEN RAISE EXCEPTION 'subscription generation requires actual current job lease' USING ERRCODE='23514'; END IF;
        IF TG_TABLE_NAME='subscription_events' THEN
            IF NEW.revision_id::text IS DISTINCT FROM j.payload->>'configuration_revision_id' OR NOT EXISTS(SELECT 1 FROM subscription_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id AND content_hash=j.payload->>'configuration_hash')
                OR (j.payload->>'control_version')::bigint IS DISTINCT FROM (SELECT max(subscription_version) FROM subscription_events WHERE org_id=NEW.org_id AND subscription_id=sid AND kind IN ('activated','resumed','restored')) THEN RAISE EXCEPTION 'job configuration must match fixed subscription revision and control version' USING ERRCODE='23514'; END IF;
        ELSIF NOT EXISTS(SELECT 1 FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.id=NEW.revision_id WHERE p.org_id=NEW.org_id AND p.id=NEW.period_id AND r.configuration_revision_id::text=j.payload->>'configuration_revision_id' AND p.start_date>=(j.payload->>'from')::date AND p.start_date<(j.payload->>'to')::date AND ((j.payload->>'to')::date-(j.payload->>'from')::date) BETWEEN 1 AND 366) THEN
            RAISE EXCEPTION 'job period must lie inside its fixed generation window' USING ERRCODE='23514'; END IF;
    ELSE
        IF TG_TABLE_NAME='subscription_events' AND NEW.kind='config_revised' THEN SELECT state='draft' INTO draft_edit FROM subscriptions WHERE org_id=NEW.org_id AND id=NEW.subscription_id; END IF;
        SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
        SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
        IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR u.id IS NULL OR u.status<>'active' OR u.auth_version<>s.auth_version OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at
            OR (NEW.kind<>'draft_created' AND NOT draft_edit AND (s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp())) THEN
            RAISE EXCEPTION 'subscription event requires actual human session and recent MFA' USING ERRCODE='23514'; END IF;
    END IF; RETURN NEW;
END; $$;
CREATE TRIGGER subscription_event_identity BEFORE INSERT ON subscription_events FOR EACH ROW EXECUTE FUNCTION verify_subscription_event_identity();
CREATE TRIGGER subscription_period_event_identity BEFORE INSERT ON subscription_period_events FOR EACH ROW EXECUTE FUNCTION verify_subscription_event_identity();
CREATE FUNCTION verify_subscription_period_operation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p subscription_periods;r subscription_period_revisions;s subscriptions;cfg subscription_revisions;actual numeric;cost_count integer;today date;BEGIN
    SELECT * INTO STRICT p FROM subscription_periods WHERE org_id=NEW.org_id AND id=NEW.period_id;
    SELECT * INTO STRICT r FROM subscription_period_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id;
    SELECT * INTO STRICT s FROM subscriptions WHERE org_id=p.org_id AND id=p.subscription_id;
    SELECT * INTO STRICT cfg FROM subscription_revisions WHERE org_id=r.org_id AND id=r.configuration_revision_id;
    today:=(clock_timestamp() AT TIME ZONE r.timezone)::date;
    IF NEW.kind='generated' AND (s.state NOT IN ('active','cancelled') OR NOT EXISTS(SELECT 1 FROM subscription_events WHERE org_id=s.org_id AND subscription_id=s.id AND kind='activated') OR NOT subscription_charge_eligible(s.org_id,s.id,p.start_date) OR cfg.id<>s.current_revision_id) THEN
        RAISE EXCEPTION 'normal generation must respect current enabled calendar and cancellation' USING ERRCODE='23514'; END IF;
    IF NEW.kind='unexpected' AND (s.state NOT IN ('paused','cancelled') OR p.start_date>today OR cfg.id<>s.current_revision_id) THEN
        RAISE EXCEPTION 'unexpected charge requires stopped subscription and actual past billing anchor' USING ERRCODE='23514'; END IF;
    IF NEW.kind='reforecast' AND (p.start_date<today OR cfg.id<>s.current_revision_id OR s.state NOT IN ('active','cancelled') OR NOT subscription_charge_eligible(s.org_id,s.id,p.start_date)) THEN
        RAISE EXCEPTION 'reforecast requires eligible future unlinked period and current configuration' USING ERRCODE='23514'; END IF;
    IF NEW.kind='confirmed' THEN
        SELECT COALESCE(sum(v.amount) FILTER(WHERE c.state='confirmed'),0),count(*) FILTER(WHERE c.state='confirmed') INTO actual,cost_count FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions v ON v.org_id=c.org_id AND v.id=c.current_revision_id WHERE l.org_id=p.org_id AND l.source_type='subscription_period' AND l.source_id=p.id;
        IF cost_count=0 OR NEW.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(p.org_id,p.id) OR NEW.cost_snapshot IS DISTINCT FROM subscription_actual_snapshot(p.org_id,p.id) OR (actual<>r.amount AND NOT NEW.acknowledge_difference) THEN
            RAISE EXCEPTION 'review must acknowledge exact current actual costs and any forecast difference' USING ERRCODE='23514'; END IF;
    ELSIF NEW.review_fingerprint IS NOT NULL OR NEW.cost_snapshot<>'[]'::jsonb THEN RAISE EXCEPTION 'only actual review may fix its cost snapshot' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER subscription_period_operation BEFORE INSERT ON subscription_period_events FOR EACH ROW EXECUTE FUNCTION verify_subscription_period_operation();
CREATE FUNCTION preserve_subscription_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.source_key,NEW.project_id,NEW.creator_id,NEW.classification,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.source_key,OLD.project_id,OLD.creator_id,OLD.classification,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR (OLD.state='archived' AND (NEW.state<>'cancelled' OR NEW.current_revision_id<>OLD.current_revision_id))
        OR (OLD.state='draft' AND NEW.state NOT IN ('draft','active','cancelled')) OR (OLD.state='active' AND NEW.state NOT IN ('active','paused','cancelled'))
        OR (OLD.state='paused' AND NEW.state NOT IN ('paused','active','cancelled')) OR (OLD.state='cancelled' AND NEW.state NOT IN ('cancelled','archived'))
        OR (OLD.cancel_effective_on IS NOT NULL AND NEW.cancel_effective_on IS DISTINCT FROM OLD.cancel_effective_on) THEN
        RAISE EXCEPTION 'subscription identity or lifecycle cannot be rewritten' USING ERRCODE='23514';
    END IF; RETURN NEW;
END; $$;
CREATE TRIGGER subscription_head_identity BEFORE UPDATE ON subscriptions FOR EACH ROW EXECUTE FUNCTION preserve_subscription_head();
CREATE TRIGGER subscription_head_no_delete BEFORE DELETE ON subscriptions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_subscription_period_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.subscription_id,NEW.start_date,NEW.end_date,NEW.charge_type,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.subscription_id,OLD.start_date,OLD.end_date,OLD.charge_type,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR (NEW.current_revision_id<>OLD.current_revision_id AND (OLD.state<>'forecast' OR EXISTS(SELECT 1 FROM cost_links WHERE org_id=OLD.org_id AND source_type='subscription_period' AND source_id=OLD.id))) THEN
        RAISE EXCEPTION 'billing identity or linked forecast snapshot cannot be rewritten' USING ERRCODE='23514'; END IF; RETURN NEW;
END; $$;
CREATE TRIGGER subscription_period_head_identity BEFORE UPDATE ON subscription_periods FOR EACH ROW EXECUTE FUNCTION preserve_subscription_period_head();
CREATE TRIGGER subscription_period_head_no_delete BEFORE DELETE ON subscription_periods FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_subscription() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid;oid uuid:=NEW.org_id; s subscriptions;r subscription_revisions;e subscription_events;prices jsonb;files jsonb;initial jsonb;activated_seq bigint;
    old_prices jsonb;new_prices jsonb;old_last jsonb;new_start date;expected_to text;old_count integer;item record;
BEGIN
    IF TG_TABLE_NAME='subscriptions' THEN sid:=NEW.id;
    ELSIF TG_TABLE_NAME IN ('subscription_prices','subscription_revision_files') THEN SELECT subscription_id INTO sid FROM subscription_revisions WHERE org_id=oid AND id=NEW.revision_id;
    ELSE sid:=NEW.subscription_id; END IF;
    SELECT * INTO STRICT s FROM subscriptions WHERE org_id=oid AND id=sid;
    SELECT * INTO STRICT r FROM subscription_revisions WHERE org_id=oid AND id=s.current_revision_id;
    SELECT * INTO e FROM subscription_events WHERE org_id=oid AND subscription_id=sid ORDER BY subscription_version DESC LIMIT 1;
    IF e.id IS NULL OR ROW(e.subscription_version,e.state,e.revision_id) IS DISTINCT FROM ROW(s.version,s.state,s.current_revision_id)
        OR (SELECT count(*) FROM subscription_events WHERE org_id=oid AND subscription_id=sid)<>s.version
        OR r.seq<>(SELECT count(*) FROM subscription_revisions WHERE org_id=oid AND subscription_id=sid) THEN RAISE EXCEPTION 'subscription requires complete immutable journal' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM subscription_events x LEFT JOIN subscription_events p ON p.org_id=x.org_id AND p.subscription_id=x.subscription_id AND p.subscription_version=x.subscription_version-1
        JOIN subscription_revisions v ON v.org_id=x.org_id AND v.id=x.revision_id LEFT JOIN subscription_revisions pv ON pv.org_id=p.org_id AND pv.id=p.revision_id
        WHERE x.org_id=oid AND x.subscription_id=sid AND NOT COALESCE(
            (x.kind='draft_created' AND x.subscription_version=1 AND x.state='draft' AND v.seq=1) OR
            (x.subscription_version>1 AND ((x.revision_id=p.revision_id) OR (v.seq=pv.seq+1 AND v.base_revision_id=p.revision_id)) AND (
                (x.kind='config_revised' AND p.state IN ('draft','active','paused','cancelled') AND x.state=p.state) OR
                (x.kind='price_changed' AND p.state IN ('active','paused') AND x.state=p.state AND x.revision_id<>p.revision_id) OR
                (x.kind='activated' AND p.state='draft' AND x.state='active') OR (x.kind='paused' AND p.state='active' AND x.state='paused') OR
                (x.kind='resumed' AND p.state='paused' AND x.state='active') OR (x.kind='cancelled' AND p.state IN ('draft','active','paused') AND x.state='cancelled') OR
                (x.kind='archived' AND p.state='cancelled' AND x.state='archived') OR (x.kind='restored' AND p.state='archived' AND x.state='cancelled' AND x.revision_id=p.revision_id) OR (x.kind IN ('periods_generated','period_changed') AND x.state=p.state AND x.revision_id=p.revision_id))) ,false)) THEN
        RAISE EXCEPTION 'subscription journal transition is invalid' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM subscription_events x JOIN subscription_events prev ON prev.org_id=x.org_id AND prev.subscription_id=x.subscription_id AND prev.subscription_version=x.subscription_version-1
        JOIN subscription_revisions v ON v.org_id=x.org_id AND v.id=x.revision_id JOIN subscription_revisions pv ON pv.org_id=prev.org_id AND pv.id=prev.revision_id WHERE x.org_id=oid AND x.subscription_id=sid AND prev.state<>'draft' AND x.kind<>'price_changed' AND v.payload->'prices' IS DISTINCT FROM pv.payload->'prices') THEN
        RAISE EXCEPTION 'activated prices require explicit immutable repricing event' USING ERRCODE='23514'; END IF;
    FOR e IN SELECT * FROM subscription_events WHERE org_id=oid AND subscription_id=sid AND kind='price_changed' LOOP
        SELECT v.payload->'prices' INTO old_prices FROM subscription_events prev JOIN subscription_revisions v ON v.org_id=prev.org_id AND v.id=prev.revision_id WHERE prev.org_id=oid AND prev.subscription_id=sid AND prev.subscription_version=e.subscription_version-1;
        SELECT * INTO STRICT r FROM subscription_revisions WHERE org_id=oid AND id=e.revision_id;
        new_prices:=r.payload->'prices';old_count:=jsonb_array_length(old_prices);old_last:=old_prices->(old_count-1);new_start:=(new_prices->old_count->>'effective_from')::date;
        IF old_count<1 OR jsonb_array_length(new_prices)<>old_count+1 OR new_start<=(old_last->>'effective_from')::date OR new_start<(e.created_at AT TIME ZONE (r.payload->>'timezone'))::date THEN
            RAISE EXCEPTION 'repricing appends a future price and preserves the existing timeline' USING ERRCODE='23514'; END IF;
        expected_to:=CASE WHEN old_last->>'effective_to' IS NULL OR (old_last->>'effective_to')::date>new_start THEN new_start::text ELSE old_last->>'effective_to' END;
        FOR item IN SELECT value,ordinality FROM jsonb_array_elements(old_prices) WITH ORDINALITY LOOP
            IF (item.ordinality<old_count AND item.value IS DISTINCT FROM new_prices->(item.ordinality::integer-1)) OR
                (item.ordinality=old_count AND ((item.value-'effective_to') IS DISTINCT FROM ((new_prices->(old_count-1))-'effective_to') OR (new_prices->(old_count-1)->>'effective_to') IS DISTINCT FROM expected_to)) THEN
                RAISE EXCEPTION 'repricing cannot rewrite prior price facts or fill an explicit gap' USING ERRCODE='23514'; END IF;
        END LOOP;
    END LOOP;
    IF s.cancel_effective_on IS DISTINCT FROM (SELECT effective_on FROM subscription_events WHERE org_id=oid AND subscription_id=sid AND kind='cancelled') OR
        (s.state='paused' AND s.paused_on IS DISTINCT FROM (SELECT effective_on FROM subscription_events WHERE org_id=oid AND subscription_id=sid AND kind='paused' ORDER BY subscription_version DESC LIMIT 1)) THEN
        RAISE EXCEPTION 'stopping dates must match their immutable lifecycle event' USING ERRCODE='23514'; END IF;
    SELECT v.payload,v.seq INTO initial,activated_seq FROM subscription_events x JOIN subscription_revisions v ON v.org_id=x.org_id AND v.id=x.revision_id WHERE x.org_id=oid AND x.subscription_id=sid AND x.kind='activated';
    FOR r IN SELECT * FROM subscription_revisions WHERE org_id=oid AND subscription_id=sid LOOP
        SELECT COALESCE(jsonb_agg(jsonb_build_object('effective_from',to_char(p.effective_from,'YYYY-MM-DD'),'effective_to',to_char(p.effective_to,'YYYY-MM-DD'),'currency',p.currency,'amount',p.amount::text,'plan',p.plan) ORDER BY position),'[]') INTO prices FROM subscription_prices p WHERE org_id=oid AND revision_id=r.id;
        SELECT COALESCE(jsonb_agg(version_id::text ORDER BY version_id::text),'[]') INTO files FROM subscription_revision_files WHERE org_id=oid AND revision_id=r.id;
        IF prices IS DISTINCT FROM r.payload->'prices' OR files IS DISTINCT FROM r.payload->'vendor_evidence_version_ids' OR length(r.payload->>'notes')>20000
            OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM subscription_revisions p WHERE p.org_id=oid AND p.id=r.base_revision_id AND p.subscription_id=sid AND p.seq=r.seq-1)) THEN
            RAISE EXCEPTION 'subscription normalized facts differ from fixed configuration' USING ERRCODE='23514'; END IF;
        IF EXISTS(SELECT 1 FROM subscription_prices a JOIN currency_definitions c ON c.code=a.currency WHERE a.org_id=oid AND a.revision_id=r.id AND a.amount*power(10::numeric,c.exponent)<>trunc(a.amount*power(10::numeric,c.exponent)))
            OR EXISTS(SELECT 1 FROM subscription_prices a JOIN subscription_prices b ON b.org_id=a.org_id AND b.revision_id=a.revision_id AND a.position<b.position WHERE a.org_id=oid AND a.revision_id=r.id AND (a.effective_from>=b.effective_from OR a.effective_to IS NULL OR a.effective_to>b.effective_from)) THEN
            RAISE EXCEPTION 'subscription price windows or precision are invalid' USING ERRCODE='23514'; END IF;
        IF (r.payload->>'vendor_state') NOT IN ('unknown','active','paused','cancelled') OR ((r.payload->>'vendor_state')<>'unknown' AND ((r.payload->>'vendor_checked_at') IS NULL OR jsonb_array_length(files)=0))
            OR ((r.payload->>'vendor_checked_at') IS NOT NULL AND ((r.payload->>'vendor_checked_at')::timestamptz>r.created_at OR jsonb_array_length(files)=0)) THEN RAISE EXCEPTION 'vendor state requires fixed actual verification evidence' USING ERRCODE='23514'; END IF;
        IF r.payload->>'timezone'<>'' AND NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=r.payload->>'timezone' AND name<>'Local') THEN RAISE EXCEPTION 'subscription requires explicit IANA timezone' USING ERRCODE='23514'; END IF;
        IF initial IS NOT NULL AND r.seq>=activated_seq AND
            (SELECT jsonb_object_agg(k,initial->k) FROM unnest(ARRAY['start_date','first_charge_date','cycle','timezone','anchor_day','custom_interval_days']) k) IS DISTINCT FROM
            (SELECT jsonb_object_agg(k,r.payload->k) FROM unnest(ARRAY['start_date','first_charge_date','cycle','timezone','anchor_day','custom_interval_days']) k) THEN
            RAISE EXCEPTION 'activated billing calendar is immutable' USING ERRCODE='23514'; END IF;
    END LOOP;
    SELECT * INTO STRICT r FROM subscription_revisions WHERE org_id=oid AND id=s.current_revision_id;
    IF s.state IN ('active','paused') AND (r.supplier_id IS NULL OR r.owner_id IS NULL OR btrim(r.category)='' OR (r.payload->>'start_date') IS NULL OR (r.payload->>'renewal_mode') NOT IN ('automatic','manual')
        OR subscription_window_end(r.payload,(r.payload->>'first_charge_date')::date) IS NULL OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=r.payload->>'timezone')
        OR NOT EXISTS(SELECT 1 FROM subscription_prices WHERE org_id=oid AND revision_id=r.id AND effective_from<=(r.payload->>'first_charge_date')::date AND (effective_to IS NULL OR effective_to>(r.payload->>'first_charge_date')::date))) THEN
        RAISE EXCEPTION 'activation requires complete billing facts and initial price' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM projects p WHERE p.org_id=oid AND p.id=s.project_id AND classification_rank(p.classification)>classification_rank(s.classification)) THEN RAISE EXCEPTION 'subscription cannot lower project classification' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END; $$;

CREATE FUNCTION validate_subscription_period() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pid uuid;oid uuid:=NEW.org_id;p subscription_periods;r subscription_period_revisions;s subscriptions;cfg subscription_revisions;price subscription_prices;e subscription_period_events;
BEGIN
    IF TG_TABLE_NAME='subscription_periods' THEN pid:=NEW.id;
    ELSIF TG_TABLE_NAME='subscription_period_event_files' THEN SELECT period_id INTO pid FROM subscription_period_events WHERE org_id=oid AND id=NEW.event_id;
    ELSE pid:=NEW.period_id; END IF;
    SELECT * INTO STRICT p FROM subscription_periods WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT s FROM subscriptions WHERE org_id=oid AND id=p.subscription_id;
    SELECT * INTO STRICT r FROM subscription_period_revisions WHERE org_id=oid AND id=p.current_revision_id;
    SELECT * INTO e FROM subscription_period_events WHERE org_id=oid AND period_id=pid ORDER BY period_version DESC LIMIT 1;
    IF e.id IS NULL OR ROW(e.period_version,e.state,e.revision_id,e.review_fingerprint,e.invoice_missing) IS DISTINCT FROM ROW(p.version,p.state,p.current_revision_id,p.review_fingerprint,p.invoice_missing)
        OR (SELECT count(*) FROM subscription_period_events WHERE org_id=oid AND period_id=pid)<>p.version OR r.seq<>(SELECT count(*) FROM subscription_period_revisions WHERE org_id=oid AND period_id=pid) THEN
        RAISE EXCEPTION 'billing period requires complete immutable journal' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM subscription_period_events x LEFT JOIN subscription_events parent ON parent.id=x.subscription_event_id WHERE x.org_id=oid AND x.period_id=pid AND
        (parent.id IS NULL OR parent.org_id<>oid OR parent.subscription_id<>p.subscription_id OR (x.kind='generated' AND parent.kind<>'periods_generated') OR
        (x.kind<>'generated' AND (parent.kind<>'period_changed' OR parent.period_id IS DISTINCT FROM pid)) OR ROW(x.actor_id,x.session_id,x.job_id,x.job_fence,x.job_owner) IS DISTINCT FROM ROW(parent.actor_id,parent.session_id,parent.job_id,parent.job_fence,parent.job_owner))) THEN
        RAISE EXCEPTION 'billing change must share its exact subscription event identity' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM subscription_period_events x LEFT JOIN subscription_period_events prev ON prev.org_id=x.org_id AND prev.period_id=x.period_id AND prev.period_version=x.period_version-1
        JOIN subscription_period_revisions v ON v.org_id=x.org_id AND v.id=x.revision_id LEFT JOIN subscription_period_revisions pv ON pv.org_id=prev.org_id AND pv.id=prev.revision_id
        WHERE x.org_id=oid AND x.period_id=pid AND NOT COALESCE(
            (x.kind IN ('generated','unexpected') AND x.period_version=1 AND x.state='forecast' AND v.seq=1) OR
            (x.kind='reforecast' AND prev.state='forecast' AND x.state='forecast' AND v.seq=pv.seq+1 AND v.base_revision_id=prev.revision_id AND NOT EXISTS(SELECT 1 FROM cost_links WHERE org_id=oid AND source_type='subscription_period' AND source_id=pid)) OR
            (x.kind='cost_linked' AND prev.state IN ('forecast','confirmed') AND x.state='forecast' AND x.revision_id=prev.revision_id AND EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.id=x.cost_link_id AND l.source_type='subscription_period' AND l.source_id=pid AND l.revision_id=x.revision_id)) OR
            (x.kind='confirmed' AND prev.state IN ('forecast','confirmed') AND x.state='confirmed' AND x.revision_id=prev.revision_id AND x.review_fingerprint IS NOT NULL AND EXISTS(SELECT 1 FROM subscription_period_event_files f WHERE f.org_id=oid AND f.event_id=x.id)) OR
            (x.kind IN ('waived','void') AND prev.state='forecast' AND x.state=x.kind AND x.revision_id=prev.revision_id AND NOT EXISTS(SELECT 1 FROM cost_links WHERE org_id=oid AND source_type='subscription_period' AND source_id=pid)) OR
            (x.kind='reopened' AND prev.state IN ('confirmed','waived','void') AND x.state='forecast' AND x.revision_id=prev.revision_id),false)) THEN RAISE EXCEPTION 'billing period journal transition is invalid' USING ERRCODE='23514'; END IF;
    FOR r IN SELECT * FROM subscription_period_revisions WHERE org_id=oid AND period_id=pid LOOP
        SELECT * INTO STRICT cfg FROM subscription_revisions WHERE org_id=oid AND id=r.configuration_revision_id;
        SELECT * INTO STRICT price FROM subscription_prices WHERE org_id=oid AND id=r.price_id;
        IF cfg.subscription_id<>s.id OR subscription_window_end(cfg.payload,p.start_date) IS DISTINCT FROM p.end_date
            OR ROW(r.currency,r.amount,r.timezone,r.category,r.owner_id,r.supplier_id,r.supplier_revision_id) IS DISTINCT FROM ROW(price.currency,price.amount,cfg.payload->>'timezone',cfg.category,cfg.owner_id,cfg.supplier_id,cfg.supplier_revision_id)
            OR price.effective_from>p.start_date OR (price.effective_to IS NOT NULL AND price.effective_to<=p.start_date)
            OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM subscription_period_revisions pr WHERE pr.org_id=oid AND pr.period_id=pid AND pr.id=r.base_revision_id AND pr.seq=r.seq-1)) THEN
            RAISE EXCEPTION 'period must pin exact calendar configuration and price' USING ERRCODE='23514'; END IF;
    END LOOP;
    RETURN NULL;
END; $$;

DO $$ DECLARE n text;BEGIN
    FOREACH n IN ARRAY ARRAY['subscription_revisions','subscription_prices','subscription_revision_files','subscription_events'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||n,n);
        EXECUTE format('CREATE CONSTRAINT TRIGGER %I AFTER INSERT ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_subscription()','validate_'||n,n);
    END LOOP;
    FOREACH n IN ARRAY ARRAY['subscription_period_revisions','subscription_period_events','subscription_period_event_files'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||n,n);
        EXECUTE format('CREATE CONSTRAINT TRIGGER %I AFTER INSERT ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_subscription_period()','validate_'||n,n);
    END LOOP;
END; $$;
CREATE CONSTRAINT TRIGGER validate_subscription_head AFTER INSERT OR UPDATE ON subscriptions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_subscription();
CREATE CONSTRAINT TRIGGER validate_subscription_period_head AFTER INSERT OR UPDATE ON subscription_periods DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_subscription_period();
CREATE TRIGGER immutable_subscription_reminders BEFORE UPDATE OR DELETE ON subscription_reminders FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION verify_subscription_reminder() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE p subscription_periods;r subscription_period_revisions;j jobs;e outbox_events;BEGIN
    SELECT * INTO STRICT p FROM subscription_periods WHERE org_id=NEW.org_id AND id=NEW.period_id;
    SELECT * INTO STRICT r FROM subscription_period_revisions WHERE org_id=p.org_id AND id=p.current_revision_id;
    SELECT * INTO j FROM jobs WHERE org_id=NEW.org_id AND id=NEW.job_id FOR UPDATE;
    SELECT * INTO e FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.outbox_id;
    IF j.id IS NULL OR j.kind<>'subscription.generate.v1' OR j.lane<>'finance' OR j.state<>'running' OR j.lease_until<=clock_timestamp() OR j.fencing_token<>NEW.job_fence OR j.lease_owner IS DISTINCT FROM NEW.job_owner OR j.payload->>'subscription_id' IS DISTINCT FROM p.subscription_id::text
        OR NEW.recipient_id<>r.owner_id OR NEW.remind_on<>p.start_date-NEW.days_before OR NEW.remind_on>(clock_timestamp() AT TIME ZONE r.timezone)::date OR p.state<>'forecast' OR NOT subscription_charge_eligible(p.org_id,p.subscription_id,p.start_date)
        OR NOT EXISTS(SELECT 1 FROM subscriptions s JOIN subscription_revisions cfg ON cfg.org_id=s.org_id AND cfg.id=s.current_revision_id WHERE s.org_id=p.org_id AND s.id=p.subscription_id AND cfg.payload->'reminder_days' @> jsonb_build_array(NEW.days_before))
        OR e.id IS NULL OR e.event_type<>'subscription.reminder.due' OR e.aggregate_type<>'subscription_period' OR e.aggregate_id<>p.id OR e.aggregate_version<>p.version
        OR e.payload->>'recipient_id' IS DISTINCT FROM NEW.recipient_id::text OR e.payload->>'period_id' IS DISTINCT FROM p.id::text OR e.payload->>'remind_on' IS DISTINCT FROM NEW.remind_on::text OR e.payload->>'days_before' IS DISTINCT FROM NEW.days_before::text OR e.payload->>'delivery_state' IS DISTINCT FROM 'queued' THEN
        RAISE EXCEPTION 'reminder must pin its actual job lease recipient date and queued outbox' USING ERRCODE='23514'; END IF; RETURN NEW;
END; $$;
CREATE TRIGGER subscription_reminder_identity BEFORE INSERT ON subscription_reminders FOR EACH ROW EXECUTE FUNCTION verify_subscription_reminder();

-- The new source has no approved-amount cap: overcharges must remain representable.
ALTER TABLE cost_links DROP CONSTRAINT cost_links_source_type_check;
ALTER TABLE cost_links DROP CONSTRAINT cost_links_relation_check;
ALTER TABLE cost_links DROP CONSTRAINT cost_links_check;
ALTER TABLE cost_links ADD CHECK(source_type IN ('purchase','claim','contract','subscription_period'));
ALTER TABLE cost_links ADD CHECK(relation IN ('incurred_for','reimbursed_via','contractual_basis','subscription_charge'));
ALTER TABLE cost_links ADD CHECK((source_type='purchase' AND relation='incurred_for') OR (source_type='claim' AND relation='reimbursed_via') OR (source_type='contract' AND relation='contractual_basis') OR (source_type='subscription_period' AND relation='subscription_charge'));
CREATE UNIQUE INDEX one_subscription_period_per_cost ON cost_links(org_id,cost_id) WHERE source_type='subscription_period';
CREATE OR REPLACE FUNCTION lock_cost_link_source() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.source_type='subscription_period' THEN
        PERFORM s.id FROM subscriptions s JOIN subscription_periods p ON p.org_id=s.org_id AND p.subscription_id=s.id WHERE p.org_id=NEW.org_id AND p.id=NEW.source_id FOR UPDATE OF s;
        PERFORM id FROM subscription_periods WHERE org_id=NEW.org_id AND id=NEW.source_id FOR UPDATE;
    ELSIF NEW.source_type='contract' THEN PERFORM id FROM contracts WHERE org_id=NEW.org_id AND id=NEW.source_id FOR UPDATE;
    ELSE PERFORM id FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.source_id AND kind=NEW.source_type FOR UPDATE; END IF;
    IF NOT FOUND THEN RAISE EXCEPTION 'cost link source does not exist' USING ERRCODE='23514'; END IF; RETURN NEW;
END; $$;

INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES
    ('subscription_manager','订阅管理人',true),('subscription_reader','订阅查阅人',false),('forecast_reporter','预测报表查阅人',false)) r(key,name,mfa) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'subscription.read' FROM roles WHERE key IN ('subscription_manager','subscription_reader','forecast_reporter') ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('subscription.create'),('subscription.update'),('subscription.manage')) a(action) WHERE key='subscription_manager' ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'report.read' FROM roles WHERE key='forecast_reporter' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT,UPDATE ON subscriptions,subscription_periods TO copypaste_app;
    GRANT SELECT,INSERT ON subscription_revisions,subscription_prices,subscription_revision_files,subscription_events,subscription_period_revisions,subscription_period_events,subscription_period_event_files,subscription_reminders TO copypaste_app;
    REVOKE UPDATE,DELETE ON subscription_revisions,subscription_prices,subscription_revision_files,subscription_events,subscription_period_revisions,subscription_period_events,subscription_period_event_files,subscription_reminders FROM copypaste_app;
    REVOKE DELETE ON subscriptions,subscription_periods FROM copypaste_app;
END IF; END; $$;

-- Preserve all existing canonical-cost validations; extend the one source branch.
CREATE OR REPLACE FUNCTION validate_cost() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cid uuid; oid uuid:=NEW.org_id; c actual_cost_entries; r cost_revisions; e cost_events; original actual_cost_entries; original_revision cost_revisions; link cost_links;
    alloc jsonb; evidence jsonb; exponent integer; source_currency text; source_amount numeric; source_class text; source_project uuid; used numeric;
BEGIN
    IF TG_TABLE_NAME='actual_cost_entries' THEN cid:=NEW.id;
    ELSIF TG_TABLE_NAME='cost_revision_files' THEN SELECT cost_id INTO cid FROM cost_revisions WHERE org_id=oid AND id=NEW.revision_id;
    ELSE cid:=NEW.cost_id; END IF;
    SELECT * INTO STRICT c FROM actual_cost_entries WHERE org_id=oid AND id=cid;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=oid AND cost_id=cid AND id=c.current_revision_id;
    SELECT * INTO e FROM cost_events WHERE org_id=oid AND cost_id=cid ORDER BY cost_version DESC LIMIT 1;
    IF e.id IS NULL OR e.cost_version<>c.version OR e.state<>c.state OR e.revision_id<>c.current_revision_id
        OR (SELECT count(*) FROM cost_events WHERE org_id=oid AND cost_id=cid)<>c.version
        OR r.seq<>(SELECT count(*) FROM cost_revisions WHERE org_id=oid AND cost_id=cid)
        OR r.seq<>(SELECT max(seq) FROM cost_revisions WHERE org_id=oid AND cost_id=cid)
        OR (SELECT count(*) FROM cost_links WHERE org_id=oid AND cost_id=cid)>100 THEN
        RAISE EXCEPTION 'cost head requires a complete immutable journal' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM cost_events x LEFT JOIN cost_events prev ON prev.org_id=x.org_id AND prev.cost_id=x.cost_id AND prev.cost_version=x.cost_version-1
        LEFT JOIN cost_revisions revision ON revision.org_id=x.org_id AND revision.id=x.revision_id
        LEFT JOIN cost_revisions previous_revision ON previous_revision.org_id=prev.org_id AND previous_revision.id=prev.revision_id
        WHERE x.org_id=oid AND x.cost_id=cid AND NOT COALESCE((
            (x.kind='draft_created' AND x.cost_version=1 AND x.state='draft' AND revision.seq=1 AND c.entry_kind='expense') OR
            (x.kind='reversal_created' AND x.cost_version=1 AND x.state='confirmed' AND revision.seq=1 AND c.entry_kind='reversal') OR
            (x.kind='draft_revised' AND x.state='draft' AND prev.state='draft' AND revision.seq=previous_revision.seq+1 AND revision.base_revision_id=prev.revision_id) OR
            (x.kind='confirmed' AND x.state='confirmed' AND prev.state='draft' AND x.revision_id=prev.revision_id) OR
            (x.kind='link_added' AND x.state='confirmed' AND prev.state='confirmed' AND x.revision_id=prev.revision_id AND EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=x.org_id AND l.id=x.link_id AND l.cost_id=cid)) OR
            (x.kind='reversed' AND x.state='reversed' AND prev.state='confirmed' AND x.revision_id=prev.revision_id AND c.entry_kind='expense')
        ),false)) THEN RAISE EXCEPTION 'cost journal transition is invalid' USING ERRCODE='23514'; END IF;
    FOR r IN SELECT * FROM cost_revisions WHERE org_id=oid AND cost_id=cid LOOP
        SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',a.project_id,'amount',a.amount::text) ORDER BY position),'[]') INTO alloc FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id;
        SELECT COALESCE(jsonb_agg(version_id::text ORDER BY version_id::text),'[]') INTO evidence FROM cost_revision_files WHERE org_id=oid AND revision_id=r.id;
        IF alloc IS DISTINCT FROM r.payload->'allocations' OR evidence IS DISTINCT FROM r.payload->'evidence_version_ids'
            OR to_char(r.occurred_on,'YYYY-MM-DD') IS DISTINCT FROM r.payload->>'occurred_on'
            OR length(r.payload->>'notes')>20000 OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM cost_revisions p WHERE p.org_id=oid AND p.cost_id=cid AND p.id=r.base_revision_id AND p.seq=r.seq-1)) THEN
            RAISE EXCEPTION 'cost normalized facts differ from fixed revision' USING ERRCODE='23514';
        END IF;
        SELECT d.exponent INTO exponent FROM currency_definitions d WHERE d.code=r.currency;
        IF r.amount IS NOT NULL AND r.amount*power(10::numeric,exponent)<>trunc(r.amount*power(10::numeric,exponent))
            OR EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id AND (r.currency IS NULL OR a.amount*power(10::numeric,exponent)<>trunc(a.amount*power(10::numeric,exponent)))) THEN
            RAISE EXCEPTION 'cost currency precision is invalid' USING ERRCODE='23514';
        END IF;
        IF EXISTS(SELECT 1 FROM cost_allocations a JOIN projects p ON p.org_id=a.org_id AND p.id=a.project_id WHERE a.org_id=oid AND a.revision_id=r.id AND classification_rank(p.classification)>classification_rank(c.classification)) THEN
            RAISE EXCEPTION 'cost allocation cannot lower project classification' USING ERRCODE='23514';
        END IF;
    END LOOP;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=oid AND id=c.current_revision_id;
    IF c.state<>'draft' AND (r.occurred_on IS NULL OR r.occurred_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR r.currency IS NULL OR r.amount IS NULL OR r.amount<=0 OR btrim(r.category)=''
        OR r.counterparty_id IS NULL OR jsonb_array_length(r.payload->'evidence_version_ids')=0 OR jsonb_array_length(r.payload->'allocations')=0
        OR (SELECT COALESCE(sum(amount),0) FROM cost_allocations WHERE org_id=oid AND revision_id=r.id)<>r.amount) THEN
        RAISE EXCEPTION 'confirmed cost requires complete balanced actual facts' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM projects p WHERE p.org_id=oid AND p.id=c.project_id AND classification_rank(p.classification)>classification_rank(c.classification)) THEN
        RAISE EXCEPTION 'cost cannot lower its home project classification' USING ERRCODE='23514';
    END IF;
    IF c.entry_kind='reversal' THEN
        SELECT * INTO STRICT original FROM actual_cost_entries WHERE org_id=oid AND id=c.reverses_id;
        SELECT * INTO STRICT original_revision FROM cost_revisions WHERE org_id=oid AND id=original.current_revision_id;
        IF original.entry_kind<>'expense' OR original.state<>'reversed' OR c.source_key<>'reversal:'||original.id::text
            OR ROW(c.project_id,c.creator_id,c.classification) IS DISTINCT FROM ROW(original.project_id,original.creator_id,original.classification)
            OR (r.payload-'occurred_on') IS DISTINCT FROM (original_revision.payload-'occurred_on') OR r.occurred_on<original_revision.occurred_on THEN
            RAISE EXCEPTION 'reversal must inherit the full original cost snapshot' USING ERRCODE='23514';
        END IF;
    ELSIF c.state='reversed' AND NOT EXISTS(SELECT 1 FROM actual_cost_entries x WHERE x.org_id=oid AND x.reverses_id=cid AND x.entry_kind='reversal' AND x.state='confirmed') THEN
        RAISE EXCEPTION 'reversed cost needs its compensating entry' USING ERRCODE='23514';
    END IF;
    FOR link IN SELECT * FROM cost_links WHERE org_id=oid AND cost_id=cid LOOP
        IF c.entry_kind<>'expense' OR NOT EXISTS(SELECT 1 FROM cost_events x WHERE x.org_id=oid AND x.cost_id=cid AND x.kind='link_added' AND x.link_id=link.id) THEN
            RAISE EXCEPTION 'cost links need their own immutable confirmation event' USING ERRCODE='23514';
        END IF;
        source_currency:=NULL; source_amount:=NULL;
        IF link.source_type='subscription_period' THEN
            SELECT v.currency,v.amount,s.classification,s.project_id INTO source_currency,source_amount,source_class,source_project
                FROM subscription_periods p JOIN subscriptions s ON s.org_id=p.org_id AND s.id=p.subscription_id
                JOIN subscription_period_revisions v ON v.org_id=p.org_id AND v.period_id=p.id AND v.id=link.revision_id
                WHERE p.org_id=oid AND p.id=link.source_id AND v.id=p.current_revision_id AND p.state NOT IN ('waived','void') AND v.supplier_id=r.counterparty_id;
            IF NOT EXISTS(SELECT 1 FROM subscription_period_events x WHERE x.org_id=oid AND x.period_id=link.source_id AND x.cost_link_id=link.id AND x.kind='cost_linked') THEN
                RAISE EXCEPTION 'subscription association needs its immutable period event' USING ERRCODE='23514'; END IF;
        ELSIF link.source_type='contract' THEN
            SELECT v.currency,v.amount,d.classification,d.project_id INTO source_currency,source_amount,source_class,source_project FROM contracts d JOIN contract_revisions v ON v.org_id=d.org_id AND v.id=link.revision_id AND v.contract_id=d.id
                WHERE d.org_id=oid AND d.id=link.source_id AND d.approved_revision_id=v.id AND d.active_revision_id=v.id AND v.direction='payable';
        ELSE
            SELECT v.currency,v.amount,d.classification,d.project_id INTO source_currency,source_amount,source_class,source_project FROM spend_documents d JOIN spend_revisions v ON v.org_id=d.org_id AND v.id=link.revision_id AND v.document_id=d.id
                WHERE d.org_id=oid AND d.id=link.source_id AND d.kind=link.source_type AND d.approved_revision_id=v.id;
        END IF;
        SELECT COALESCE(sum(v.amount),0) INTO used FROM cost_links l JOIN actual_cost_entries d ON d.org_id=l.org_id AND d.id=l.cost_id JOIN cost_revisions v ON v.org_id=d.org_id AND v.id=d.current_revision_id
            WHERE l.org_id=oid AND l.source_type=link.source_type AND l.source_id=link.source_id AND d.state='confirmed' AND d.entry_kind='expense';
        IF source_currency IS NULL OR source_currency<>r.currency OR source_amount IS NULL OR (link.source_type<>'subscription_period' AND used>source_amount)
            OR classification_rank(source_class)>classification_rank(c.classification)
            OR (source_project IS NOT NULL AND source_project IS DISTINCT FROM c.project_id AND NOT EXISTS(SELECT 1 FROM cost_allocations a WHERE a.org_id=oid AND a.revision_id=r.id AND a.project_id=source_project)) THEN
            RAISE EXCEPTION 'cost links must respect exact approved currency scope and total' USING ERRCODE='23514';
        END IF;
    END LOOP;
    RETURN NULL;
END; $$;
