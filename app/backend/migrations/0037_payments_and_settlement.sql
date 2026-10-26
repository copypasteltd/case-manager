-- Cash facts, immutable allocation legs and exact settlement are distinct from incurred cost.
CREATE TABLE payment_records (
    id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),source_key text NOT NULL,
    project_id uuid,creator_id uuid NOT NULL,classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    record_kind text NOT NULL CHECK(record_kind IN ('payment','reversal')),reverses_id uuid,
    state text NOT NULL CHECK(state IN ('draft','recorded','reversed')),current_revision_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,source_key),UNIQUE(org_id,reverses_id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,reverses_id) REFERENCES payment_records(org_id,id),
    CHECK(source_key~'^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$'),CHECK((record_kind='reversal')=(reverses_id IS NOT NULL)),
    CHECK(record_kind<>'reversal' OR state='recorded'),CHECK(reverses_id IS NULL OR reverses_id<>id),CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE TABLE payment_revisions (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,payment_id uuid NOT NULL,seq bigint NOT NULL CHECK(seq>0),base_revision_id uuid,
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    title text GENERATED ALWAYS AS (payload->>'title') STORED NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
    direction text GENERATED ALWAYS AS (payload->>'direction') STORED CHECK(direction IN ('outgoing','incoming','refund','transfer')),
    currency text GENERATED ALWAYS AS (payload->>'currency') STORED REFERENCES currency_definitions(code),
    amount numeric(20,6) GENERATED ALWAYS AS ((payload->>'amount')::numeric) STORED CHECK(amount>=0),
    paid_at timestamptz,paid_on date,category text GENERATED ALWAYS AS (payload->>'category') STORED NOT NULL CHECK(length(category)<=50),
    counterparty_id uuid GENERATED ALWAYS AS ((payload->'counterparty'->>'party_id')::uuid) STORED,
    counterparty_revision_id uuid GENERATED ALWAYS AS ((payload->'counterparty'->>'revision_id')::uuid) STORED,
    employee_id uuid GENERATED ALWAYS AS ((payload->>'employee_id')::uuid) STORED,
    refund_of_id uuid GENERATED ALWAYS AS ((payload->>'refund_of_id')::uuid) STORED,
    author_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,payment_id,id),UNIQUE(org_id,payment_id,seq),
    FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),FOREIGN KEY(org_id,payment_id,base_revision_id) REFERENCES payment_revisions(org_id,payment_id,id),
    FOREIGN KEY(org_id,counterparty_id,counterparty_revision_id) REFERENCES party_revisions(org_id,party_id,id),
    FOREIGN KEY(org_id,employee_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,refund_of_id) REFERENCES payment_records(org_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),CHECK((seq=1)=(base_revision_id IS NULL)),
    CHECK((counterparty_id IS NULL)=(counterparty_revision_id IS NULL)),CHECK(counterparty_id IS NULL OR employee_id IS NULL),
    CHECK(amount IS NULL OR currency IS NOT NULL),CHECK(paid_on IS NOT DISTINCT FROM (paid_at AT TIME ZONE 'Asia/Hong_Kong')::date),
    CHECK(jsonb_typeof(payload->'evidence_version_ids')='array' AND jsonb_array_length(payload->'evidence_version_ids')<=20)
);
ALTER TABLE payment_records ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES payment_revisions(org_id,payment_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE payment_revision_files (
    org_id uuid NOT NULL,revision_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,revision_id,version_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES payment_revisions(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE payment_allocations (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,payment_id uuid NOT NULL,cost_id uuid NOT NULL,cost_revision_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('allocation','release','refund','reversal')),effect smallint NOT NULL CHECK(effect IN (-1,1)),
    amount numeric(20,6) NOT NULL CHECK(amount>0),root_id uuid NOT NULL,source_allocation_id uuid,
    claim_id uuid,claim_revision_id uuid,claim_line_id uuid,actor_id uuid NOT NULL,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),FOREIGN KEY(org_id,payment_id) REFERENCES payment_records(org_id,id),
    FOREIGN KEY(org_id,cost_id,cost_revision_id) REFERENCES cost_revisions(org_id,cost_id,id),
    FOREIGN KEY(org_id,root_id) REFERENCES payment_allocations(org_id,id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(org_id,source_allocation_id) REFERENCES payment_allocations(org_id,id),
    FOREIGN KEY(org_id,claim_id,claim_revision_id) REFERENCES spend_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),CHECK((claim_id IS NULL)=(claim_revision_id IS NULL) AND (claim_id IS NULL)=(claim_line_id IS NULL)),
    CHECK((kind='allocation' AND source_allocation_id IS NULL AND root_id=id AND effect=1) OR (kind IN ('release','refund') AND source_allocation_id IS NOT NULL AND effect=-1) OR (kind='reversal' AND source_allocation_id IS NOT NULL))
);
CREATE UNIQUE INDEX payment_reversal_leg_once ON payment_allocations(org_id,payment_id,source_allocation_id) WHERE kind='reversal';
CREATE TABLE payment_allocation_shares (
    org_id uuid NOT NULL,allocation_id uuid NOT NULL,position integer NOT NULL CHECK(position BETWEEN 1 AND 100),project_id uuid,amount numeric(20,6) NOT NULL CHECK(amount>0),
    PRIMARY KEY(org_id,allocation_id,position),FOREIGN KEY(org_id,allocation_id) REFERENCES payment_allocations(org_id,id),FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id)
);
CREATE UNIQUE INDEX payment_share_scope ON payment_allocation_shares(org_id,allocation_id,COALESCE(project_id,'00000000-0000-0000-0000-000000000000'));
CREATE TABLE payment_events (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,payment_id uuid NOT NULL,payment_version bigint NOT NULL CHECK(payment_version>0),revision_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('draft_created','draft_revised','recorded','allocation_added','allocation_released','refund_allocated','reversed','reversal_created')),
    state text NOT NULL CHECK(state IN ('draft','recorded','reversed')),allocation_id uuid,actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,payment_id,payment_version),FOREIGN KEY(org_id,payment_id,revision_id) REFERENCES payment_revisions(org_id,payment_id,id),
    FOREIGN KEY(org_id,allocation_id) REFERENCES payment_allocations(org_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    CHECK((kind IN ('allocation_added','allocation_released','refund_allocated'))=(allocation_id IS NOT NULL)),CHECK(kind IN ('draft_created','draft_revised') OR mfa_verified_at IS NOT NULL)
);
CREATE UNIQUE INDEX payment_event_allocation_once ON payment_events(org_id,allocation_id) WHERE allocation_id IS NOT NULL;
CREATE INDEX payment_list ON payment_records(org_id,updated_at DESC,id DESC);
CREATE INDEX payment_date ON payment_revisions(org_id,paid_on,currency);
CREATE INDEX payment_refund ON payment_revisions(org_id,refund_of_id);
CREATE INDEX payment_allocation_cost ON payment_allocations(org_id,cost_id);
CREATE INDEX payment_allocation_payment ON payment_allocations(org_id,payment_id);
CREATE INDEX payment_allocation_root ON payment_allocations(org_id,root_id);
CREATE INDEX payment_allocation_claim ON payment_allocations(org_id,claim_id,claim_revision_id,claim_line_id);
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','deliverable','recovery_set'));
CREATE FUNCTION register_payment_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'payment_revision',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER payment_file_reference AFTER INSERT ON payment_revision_files FOR EACH ROW EXECUTE FUNCTION register_payment_file();
CREATE FUNCTION can_read_payment(p_org uuid,p_user uuid,p_id uuid,p_channel text,p_ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS(SELECT 1 FROM payment_records p WHERE p.org_id=p_org AND p.id=p_id
        AND can_access(p.org_id,p_user,'payment.read','payment',p.id,p.project_id,p.creator_id,p.classification,p_channel,p_ai)
        AND NOT EXISTS(SELECT 1 FROM payment_allocations a JOIN actual_cost_entries c ON c.org_id=a.org_id AND c.id=a.cost_id
            WHERE a.org_id=p.org_id AND a.payment_id=p.id AND NOT can_read_cost(c.org_id,p_user,c.id,c.current_revision_id,p_channel,p_ai)))
$$;
CREATE FUNCTION preserve_payment_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    PERFORM q.id FROM payment_records q WHERE q.org_id=NEW.org_id AND q.id=(SELECT refund_of_id FROM payment_revisions WHERE org_id=NEW.org_id AND id=NEW.current_revision_id) FOR UPDATE;
    IF ROW(NEW.id,NEW.org_id,NEW.source_key,NEW.project_id,NEW.creator_id,NEW.classification,NEW.record_kind,NEW.reverses_id,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.source_key,OLD.project_id,OLD.creator_id,OLD.classification,OLD.record_kind,OLD.reverses_id,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR OLD.state='reversed' OR OLD.record_kind='reversal'
        OR (OLD.state='draft' AND (NEW.state NOT IN ('draft','recorded') OR (NEW.state='recorded' AND NEW.current_revision_id<>OLD.current_revision_id)))
        OR (OLD.state='recorded' AND (NEW.state NOT IN ('recorded','reversed') OR NEW.current_revision_id<>OLD.current_revision_id)) THEN
        RAISE EXCEPTION 'payment identity and confirmed cash facts are immutable' USING ERRCODE='23514';
    END IF; RETURN NEW;
END; $$;
CREATE TRIGGER payment_head_identity BEFORE UPDATE ON payment_records FOR EACH ROW EXECUTE FUNCTION preserve_payment_head();
CREATE TRIGGER payment_head_no_delete BEFORE DELETE ON payment_records FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION verify_payment_event_session() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE s sessions; u users; BEGIN
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
    IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR u.id IS NULL OR u.status<>'active' OR u.auth_version<>s.auth_version
        OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at OR (NEW.kind NOT IN ('draft_created','draft_revised') AND (s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp())) THEN
        RAISE EXCEPTION 'payment event requires actual active human session and recent MFA' USING ERRCODE='23514';
    END IF; RETURN NEW;
END; $$;
CREATE TRIGGER payment_event_session BEFORE INSERT ON payment_events FOR EACH ROW EXECUTE FUNCTION verify_payment_event_session();
-- Lock costs before payment heads; concurrent receipts cannot each consume the same balance.
CREATE FUNCTION lock_payment_allocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    PERFORM id FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.claim_id FOR UPDATE;
    PERFORM id FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=NEW.cost_id FOR UPDATE;
    PERFORM p.id FROM payment_records p WHERE p.org_id=NEW.org_id AND (p.id=NEW.payment_id OR p.id=(SELECT refund_of_id FROM payment_revisions WHERE org_id=NEW.org_id AND id=(SELECT current_revision_id FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.payment_id))) ORDER BY p.id FOR UPDATE;
    RETURN NEW;
END; $$;
CREATE TRIGGER payment_allocation_lock BEFORE INSERT ON payment_allocations FOR EACH ROW EXECUTE FUNCTION lock_payment_allocation();
CREATE FUNCTION validate_cost_settlement(p_org uuid,p_cost uuid) RETURNS void LANGUAGE plpgsql AS $$ DECLARE c actual_cost_entries;r cost_revisions;paid numeric;BEGIN
    SELECT * INTO STRICT c FROM actual_cost_entries WHERE org_id=p_org AND id=p_cost;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=p_org AND id=c.current_revision_id;
    SELECT COALESCE(sum(effect*amount),0) INTO paid FROM payment_allocations WHERE org_id=p_org AND cost_id=p_cost;
    IF paid<0 OR paid>COALESCE(r.amount,0) OR (paid<>0 AND (c.state<>'confirmed' OR c.entry_kind<>'expense')) THEN
        RAISE EXCEPTION 'cost effective settlement cannot exceed its confirmed amount or survive cost reversal' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM (SELECT s.project_id,sum(a.effect*s.amount) paid FROM payment_allocations a JOIN payment_allocation_shares s ON s.org_id=a.org_id AND s.allocation_id=a.id WHERE a.org_id=p_org AND a.cost_id=p_cost GROUP BY s.project_id) totals
        LEFT JOIN cost_allocations x ON x.org_id=p_org AND x.revision_id=r.id AND x.project_id IS NOT DISTINCT FROM totals.project_id WHERE totals.paid<0 OR totals.paid>COALESCE(x.amount,0)) THEN
        RAISE EXCEPTION 'cost project settlement share exceeds its confirmed allocation' USING ERRCODE='23514';
    END IF;
END; $$;
CREATE FUNCTION check_cost_settlement_trigger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM validate_cost_settlement(NEW.org_id,NEW.id);RETURN NULL;END; $$;
CREATE CONSTRAINT TRIGGER cost_settlement_guard AFTER INSERT OR UPDATE ON actual_cost_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_cost_settlement_trigger();
-- Check the funding family whenever a refund, inverse or allocation changes.
CREATE FUNCTION validate_payment_balance(oid uuid,pid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p payment_records;r payment_revisions;refunds numeric;allocated numeric;BEGIN
    SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    IF p.record_kind<>'payment' OR r.direction IS DISTINCT FROM 'outgoing' THEN RETURN;END IF;
    SELECT COALESCE(sum(v.amount) FILTER(WHERE q.state='recorded' AND q.record_kind='payment'),0) INTO refunds FROM payment_records q JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id WHERE q.org_id=oid AND v.refund_of_id=pid;
    SELECT COALESCE(sum(x.effect*x.amount),0) INTO allocated FROM payment_allocations x JOIN payment_records q ON q.org_id=x.org_id AND q.id=x.payment_id JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id
        WHERE x.org_id=oid AND (q.id=pid OR q.reverses_id=pid OR v.refund_of_id=pid);
    IF refunds>COALESCE(r.amount,0) OR (p.state='reversed' AND (refunds<>0 OR allocated<>0)) OR allocated<0 OR allocated>COALESCE(r.amount,0)-refunds THEN
        RAISE EXCEPTION 'effective allocations or refunds exceed net outgoing payment balance' USING ERRCODE='23514';END IF;
END; $$;
CREATE FUNCTION validate_payment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE oid uuid:=NEW.org_id;pid uuid;p payment_records;r payment_revisions;e payment_events;original payment_records;oldr payment_revisions;a payment_allocations;src payment_allocations;
    evidence jsonb;exp integer;allocated numeric;refunds numeric;root_balance numeric;claim spend_documents;claimrev spend_revisions;line jsonb;
BEGIN
    IF TG_TABLE_NAME='payment_records' THEN pid:=NEW.id;
    ELSIF TG_TABLE_NAME='payment_revision_files' THEN SELECT payment_id INTO pid FROM payment_revisions WHERE org_id=oid AND id=NEW.revision_id;
    ELSIF TG_TABLE_NAME='payment_allocation_shares' THEN SELECT payment_id INTO pid FROM payment_allocations WHERE org_id=oid AND id=NEW.allocation_id;
    ELSE pid:=NEW.payment_id;END IF;
    SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    SELECT * INTO e FROM payment_events WHERE org_id=oid AND payment_id=pid ORDER BY payment_version DESC LIMIT 1;
    IF e.id IS NULL OR e.payment_version<>p.version OR e.state<>p.state OR e.revision_id<>p.current_revision_id
        OR (SELECT count(*) FROM payment_events WHERE org_id=oid AND payment_id=pid)<>p.version
        OR r.seq<>(SELECT count(*) FROM payment_revisions WHERE org_id=oid AND payment_id=pid) OR r.seq<>(SELECT max(seq) FROM payment_revisions WHERE org_id=oid AND payment_id=pid) THEN
        RAISE EXCEPTION 'payment needs a complete immutable journal' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM payment_events x LEFT JOIN payment_events prev ON prev.org_id=x.org_id AND prev.payment_id=x.payment_id AND prev.payment_version=x.payment_version-1
        LEFT JOIN payment_revisions rev ON rev.org_id=x.org_id AND rev.id=x.revision_id LEFT JOIN payment_revisions previous ON previous.org_id=prev.org_id AND previous.id=prev.revision_id
        WHERE x.org_id=oid AND x.payment_id=pid AND NOT COALESCE((
        (x.kind='draft_created' AND x.payment_version=1 AND x.state='draft' AND rev.seq=1 AND p.record_kind='payment') OR
        (x.kind='reversal_created' AND x.payment_version=1 AND x.state='recorded' AND rev.seq=1 AND p.record_kind='reversal') OR
        (x.kind='draft_revised' AND x.state='draft' AND prev.state='draft' AND rev.base_revision_id=prev.revision_id AND rev.seq=previous.seq+1) OR
        (x.kind='recorded' AND x.state='recorded' AND prev.state='draft' AND x.revision_id=prev.revision_id) OR
        (x.kind IN ('allocation_added','allocation_released','refund_allocated') AND x.state='recorded' AND prev.state='recorded' AND x.revision_id=prev.revision_id AND EXISTS(SELECT 1 FROM payment_allocations q WHERE q.org_id=x.org_id AND q.id=x.allocation_id AND q.payment_id=pid)) OR
        (x.kind='reversed' AND x.state='reversed' AND prev.state='recorded' AND x.revision_id=prev.revision_id AND p.record_kind='payment')),false)) THEN
        RAISE EXCEPTION 'invalid payment journal transition' USING ERRCODE='23514';
    END IF;
    FOR r IN SELECT * FROM payment_revisions WHERE org_id=oid AND payment_id=pid LOOP
        SELECT COALESCE(jsonb_agg(version_id::text ORDER BY version_id::text),'[]') INTO evidence FROM payment_revision_files WHERE org_id=oid AND revision_id=r.id;
        SELECT exponent INTO exp FROM currency_definitions WHERE code=r.currency;
        IF evidence IS DISTINCT FROM r.payload->'evidence_version_ids' OR r.paid_at IS DISTINCT FROM (r.payload->>'paid_at')::timestamptz OR length(r.payload->>'notes')>20000
            OR length(r.payload->>'method_label')>100 OR length(r.payload->>'external_ref')>200
            OR r.refund_of_id IS DISTINCT FROM (SELECT refund_of_id FROM payment_revisions WHERE org_id=oid AND payment_id=pid AND seq=1)
            OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM payment_revisions prev WHERE prev.org_id=oid AND prev.payment_id=pid AND prev.id=r.base_revision_id AND prev.seq=r.seq-1))
            OR (r.amount IS NOT NULL AND r.amount*power(10::numeric,exp)<>trunc(r.amount*power(10::numeric,exp))) THEN
            RAISE EXCEPTION 'payment fixed revision differs from normalized facts or currency precision' USING ERRCODE='23514';
        END IF;
    END LOOP;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    IF EXISTS(SELECT 1 FROM projects x WHERE x.org_id=oid AND x.id=p.project_id AND classification_rank(x.classification)>classification_rank(p.classification)) THEN RAISE EXCEPTION 'payment classification below project' USING ERRCODE='23514';END IF;
    IF p.state<>'draft' AND (r.direction IS NULL OR r.currency IS NULL OR r.amount IS NULL OR r.amount<=0 OR r.paid_at IS NULL OR r.paid_at>clock_timestamp() OR btrim(r.category)='' OR btrim(r.payload->>'method_label')=''
        OR jsonb_array_length(r.payload->'evidence_version_ids')=0
        OR (r.direction='transfer' AND (r.counterparty_id IS NOT NULL OR r.employee_id IS NOT NULL))
        OR (r.direction<>'transfer' AND r.counterparty_id IS NULL AND r.employee_id IS NULL)
        OR (r.direction='incoming' AND r.employee_id IS NOT NULL) OR (r.direction='refund')<>(r.refund_of_id IS NOT NULL)) THEN
        RAISE EXCEPTION 'recorded payment requires complete actual cash facts and evidence' USING ERRCODE='23514';
    END IF;
    IF p.record_kind='reversal' THEN
        SELECT * INTO STRICT original FROM payment_records WHERE org_id=oid AND id=p.reverses_id;
        SELECT * INTO STRICT oldr FROM payment_revisions WHERE org_id=oid AND id=original.current_revision_id;
        IF original.state<>'reversed' OR original.record_kind<>'payment' OR p.source_key<>'reversal:'||original.id::text
            OR ROW(p.project_id,p.creator_id,p.classification) IS DISTINCT FROM ROW(original.project_id,original.creator_id,original.classification)
            OR (r.payload-'paid_at') IS DISTINCT FROM (oldr.payload-'paid_at') OR r.paid_at<oldr.paid_at
            OR (SELECT count(*) FROM payment_allocations WHERE org_id=oid AND payment_id=pid)<>(SELECT count(*) FROM payment_allocations WHERE org_id=oid AND payment_id=original.id) THEN
            RAISE EXCEPTION 'payment reversal must inherit every original cash and allocation fact' USING ERRCODE='23514';
        END IF;
    ELSIF p.state='reversed' AND NOT EXISTS(SELECT 1 FROM payment_records x WHERE x.org_id=oid AND x.reverses_id=pid AND x.record_kind='reversal' AND x.state='recorded') THEN RAISE EXCEPTION 'reversed payment missing its inverse' USING ERRCODE='23514';END IF;
    IF p.state<>'draft' AND r.refund_of_id IS NOT NULL THEN
        SELECT * INTO STRICT original FROM payment_records WHERE org_id=oid AND id=r.refund_of_id;
        SELECT * INTO STRICT oldr FROM payment_revisions WHERE org_id=oid AND id=original.current_revision_id;
        IF original.record_kind<>'payment' OR original.state NOT IN ('recorded','reversed') OR oldr.direction<>'outgoing' OR r.paid_at<oldr.paid_at OR r.currency<>oldr.currency
            OR ROW(r.counterparty_id,r.counterparty_revision_id,r.employee_id,p.project_id,p.classification) IS DISTINCT FROM ROW(oldr.counterparty_id,oldr.counterparty_revision_id,oldr.employee_id,original.project_id,original.classification) THEN
            RAISE EXCEPTION 'refund must pin the original outgoing cash identity and scope' USING ERRCODE='23514';
        END IF;
    END IF;
    PERFORM validate_payment_balance(oid,pid);
    IF r.refund_of_id IS NOT NULL THEN PERFORM validate_payment_balance(oid,r.refund_of_id);END IF;
    IF p.reverses_id IS NOT NULL THEN PERFORM validate_payment_balance(oid,p.reverses_id);END IF;
    SELECT COALESCE(sum(effect*amount),0) INTO allocated FROM payment_allocations WHERE org_id=oid AND payment_id=pid;
    IF (r.direction='refund' AND (allocated>0 OR -allocated>r.amount) AND p.record_kind='payment') OR (r.direction NOT IN ('outgoing','refund') AND allocated<>0) THEN RAISE EXCEPTION 'allocation direction differs from actual cash fact' USING ERRCODE='23514';END IF;
    FOR a IN SELECT * FROM payment_allocations WHERE org_id=oid AND payment_id=pid LOOP
        PERFORM validate_cost_settlement(oid,a.cost_id);
        SELECT exponent INTO exp FROM currency_definitions WHERE code=r.currency;
        IF r.currency IS DISTINCT FROM (SELECT currency FROM cost_revisions WHERE org_id=oid AND id=a.cost_revision_id)
            OR a.cost_revision_id IS DISTINCT FROM (SELECT current_revision_id FROM actual_cost_entries WHERE org_id=oid AND id=a.cost_id)
            OR p.state='draft' OR a.amount*power(10::numeric,exp)<>trunc(a.amount*power(10::numeric,exp))
            OR EXISTS(SELECT 1 FROM payment_allocation_shares s WHERE s.org_id=oid AND s.allocation_id=a.id AND s.amount*power(10::numeric,exp)<>trunc(s.amount*power(10::numeric,exp)))
            OR classification_rank(p.classification)<(SELECT classification_rank(classification) FROM actual_cost_entries WHERE org_id=oid AND id=a.cost_id)
            OR (SELECT COALESCE(sum(amount),0) FROM payment_allocation_shares WHERE org_id=oid AND allocation_id=a.id)<>a.amount THEN
            RAISE EXCEPTION 'allocation requires same currency fixed cost and balanced project shares' USING ERRCODE='23514';END IF;
        IF a.kind='allocation' THEN
            IF p.record_kind<>'payment' OR r.direction<>'outgoing' OR p.state='draft' OR NOT EXISTS(SELECT 1 FROM payment_events x WHERE x.org_id=oid AND x.payment_id=pid AND x.allocation_id=a.id AND x.kind='allocation_added') THEN RAISE EXCEPTION 'allocation requires recorded outgoing and own event' USING ERRCODE='23514';END IF;
            IF r.employee_id IS NOT NULL THEN
                SELECT * INTO claim FROM spend_documents WHERE org_id=oid AND id=a.claim_id AND kind='claim';
                SELECT * INTO claimrev FROM spend_revisions WHERE org_id=oid AND id=a.claim_revision_id;
                SELECT v INTO line FROM jsonb_array_elements(claimrev.payload->'lines') v WHERE v->>'id'=a.claim_line_id::text;
                IF claim.id IS NULL OR claim.creator_id<>r.employee_id OR claim.approved_revision_id IS DISTINCT FROM a.claim_revision_id OR claimrev.currency<>r.currency OR line IS NULL OR line->>'funding'<>'employee_advance'
                    OR NOT EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.cost_id=a.cost_id AND l.source_type='claim' AND l.source_id=a.claim_id AND l.revision_id=a.claim_revision_id)
                    OR (line->'supplier'->>'party_id')::uuid IS DISTINCT FROM (SELECT counterparty_id FROM cost_revisions WHERE org_id=oid AND id=a.cost_revision_id)
                    OR (SELECT COALESCE(sum(effect*amount),0) FROM payment_allocations WHERE org_id=oid AND claim_id=a.claim_id AND claim_revision_id=a.claim_revision_id AND claim_line_id=a.claim_line_id)>(line->>'declared_amount')::numeric THEN
                    RAISE EXCEPTION 'reimbursement requires approved employee advance line and available balance' USING ERRCODE='23514';END IF;
            ELSIF a.claim_id IS NOT NULL OR r.counterparty_id IS DISTINCT FROM (SELECT counterparty_id FROM cost_revisions WHERE org_id=oid AND id=a.cost_revision_id) THEN RAISE EXCEPTION 'direct payment must match cost supplier and cannot impersonate employee settlement' USING ERRCODE='23514';END IF;
        ELSE
            SELECT * INTO STRICT src FROM payment_allocations WHERE org_id=oid AND id=a.source_allocation_id;
            IF ROW(a.cost_id,a.cost_revision_id,a.root_id,a.claim_id,a.claim_revision_id,a.claim_line_id) IS DISTINCT FROM ROW(src.cost_id,src.cost_revision_id,src.root_id,src.claim_id,src.claim_revision_id,src.claim_line_id)
                OR (a.kind='release' AND (src.kind<>'allocation' OR src.payment_id<>pid OR r.direction<>'outgoing' OR p.record_kind<>'payment'))
                OR (a.kind='refund' AND (src.kind<>'allocation' OR src.payment_id IS DISTINCT FROM r.refund_of_id OR r.direction<>'refund' OR p.record_kind<>'payment'))
                OR (a.kind='reversal' AND (p.reverses_id IS DISTINCT FROM src.payment_id OR a.effect<>-src.effect OR a.amount<>src.amount)) THEN
                RAISE EXCEPTION 'allocation inverse must pin its exact source and relation' USING ERRCODE='23514';END IF;
            IF a.kind IN ('release','refund') AND NOT EXISTS(SELECT 1 FROM payment_events x WHERE x.org_id=oid AND x.payment_id=pid AND ((x.allocation_id=a.id AND x.kind=CASE a.kind WHEN 'release' THEN 'allocation_released' ELSE 'refund_allocated' END) OR (a.kind='refund' AND x.kind='recorded' AND x.created_at>=a.created_at))) THEN
                RAISE EXCEPTION 'allocation correction requires its own recorded event' USING ERRCODE='23514';END IF;
            IF a.kind='reversal' AND EXISTS(SELECT position,project_id,amount FROM payment_allocation_shares WHERE org_id=oid AND allocation_id=a.id EXCEPT SELECT position,project_id,amount FROM payment_allocation_shares WHERE org_id=oid AND allocation_id=src.id) THEN RAISE EXCEPTION 'allocation reversal changed its project shares' USING ERRCODE='23514';END IF;
        END IF;
        SELECT COALESCE(sum(effect*amount),0) INTO root_balance FROM payment_allocations WHERE org_id=oid AND root_id=a.root_id;
        IF root_balance<0 OR root_balance>(SELECT amount FROM payment_allocations WHERE org_id=oid AND id=a.root_id) THEN RAISE EXCEPTION 'allocation source balance exhausted' USING ERRCODE='23514';END IF;
        IF EXISTS(SELECT 1 FROM (SELECT s.project_id,sum(q.effect*s.amount) remaining FROM payment_allocations q JOIN payment_allocation_shares s ON s.org_id=q.org_id AND s.allocation_id=q.id WHERE q.org_id=oid AND q.root_id=a.root_id GROUP BY s.project_id) t
            LEFT JOIN payment_allocation_shares orig ON orig.org_id=oid AND orig.allocation_id=a.root_id AND orig.project_id IS NOT DISTINCT FROM t.project_id WHERE t.remaining<0 OR t.remaining>COALESCE(orig.amount,0)) THEN RAISE EXCEPTION 'allocation source project balance exhausted' USING ERRCODE='23514';END IF;
    END LOOP;
    RETURN NULL;
END; $$;
DO $$ DECLARE n text;BEGIN FOREACH n IN ARRAY ARRAY['payment_revisions','payment_revision_files','payment_allocations','payment_allocation_shares','payment_events'] LOOP
    EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||n,n);
    EXECUTE format('CREATE CONSTRAINT TRIGGER %I AFTER INSERT ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_payment()','validate_'||n,n);
END LOOP;END; $$;
CREATE CONSTRAINT TRIGGER validate_payment_head AFTER INSERT OR UPDATE ON payment_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_payment();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES ('payment_recorder','资金登记人',false),('payment_reverser','资金冲销人',true),('payment_reader','资金查阅人',false),('cash_reporter','现金报表查阅人',false)) r(key,name,mfa) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'payment.read' FROM roles WHERE key IN ('payment_recorder','payment_reverser','payment_reader','cash_reporter') ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,CASE key WHEN 'payment_recorder' THEN 'payment.record' WHEN 'payment_reverser' THEN 'payment.reverse' ELSE 'report.read' END FROM roles WHERE key IN ('payment_recorder','payment_reverser','cash_reporter') ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT,UPDATE ON payment_records TO copypaste_app;REVOKE DELETE ON payment_records FROM copypaste_app;
    GRANT SELECT,INSERT ON payment_revisions,payment_revision_files,payment_allocations,payment_allocation_shares,payment_events TO copypaste_app;
    REVOKE UPDATE,DELETE ON payment_revisions,payment_revision_files,payment_allocations,payment_allocation_shares,payment_events FROM copypaste_app;
END IF;END; $$;
