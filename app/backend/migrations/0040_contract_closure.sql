-- Human registration of legal completion/termination; financial facts remain independent.
CREATE TABLE contract_closures (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,contract_id uuid NOT NULL,revision_id uuid NOT NULL,
    contract_version bigint NOT NULL CHECK(contract_version>1),finance_version bigint NOT NULL CHECK(finance_version>=0),
    action text NOT NULL CHECK(action IN ('completed','terminated')),occurred_on date NOT NULL,
    basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),fingerprint text NOT NULL CHECK(fingerprint~'^[0-9a-f]{32}$'),
    terms_verified boolean NOT NULL CHECK(terms_verified),attestation text NOT NULL CHECK(length(btrim(attestation)) BETWEEN 1 AND 20000),
    acknowledge_unsettled boolean NOT NULL,unsettled_disposition text NOT NULL CHECK(length(unsettled_disposition)<=20000),
    evidence_count integer NOT NULL CHECK(evidence_count BETWEEN 1 AND 20),
    actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id),UNIQUE(org_id,contract_id),
    FOREIGN KEY(org_id,contract_id,revision_id) REFERENCES contract_revisions(org_id,contract_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TABLE contract_closure_files (
    org_id uuid NOT NULL,closure_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,closure_id,version_id),
    FOREIGN KEY(org_id,closure_id) REFERENCES contract_closures(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE FUNCTION contract_closure_snapshot(oid uuid,cid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('revision_id',c.active_revision_id,'finance_version',COALESCE(h.version,0),'schedule_mode',r.payment_schedule->>'mode',
        'nodes',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'title',m.title,'direction',m.direction,'currency',m.currency,'amount',m.amount::text,
            'cash_amount',COALESCE((SELECT sum(contract_finance_effective(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind IN ('cash','refund')),0)::text,
            'has_unattributed_refunds',EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind='cash' AND contract_unattributed_refunds(oid,e.payment_id)>0),
            'review_id',v.id,'review_current',COALESCE(v.review_fingerprint=md5(contract_finance_snapshot(oid,m.id)::text),false),
            'conditions_satisfied',v.conditions_satisfied,'sources',contract_finance_snapshot(oid,m.id)) ORDER BY m.position)
            FROM contract_milestones m LEFT JOIN LATERAL(SELECT id,review_fingerprint,conditions_satisfied FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind='review' ORDER BY seq DESC LIMIT 1)v ON true
            WHERE m.org_id=oid AND m.contract_id=cid),'[]'::jsonb),
        'costs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'version',d.version,'revision_id',cr.id,'content_hash',cr.content_hash,'state',d.state,
            'currency',cr.currency,'amount',cr.amount::text,'paid_amount',COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id),0)::text,
            'allocation_fingerprint',md5(COALESCE((SELECT string_agg(a.id::text||':'||a.effect::text||':'||a.amount::text||':'||p.version::text||':'||p.state,',' ORDER BY a.id)
                FROM payment_allocations a JOIN payment_records p ON p.org_id=a.org_id AND p.id=a.payment_id WHERE a.org_id=oid AND a.cost_id=d.id),''))) ORDER BY d.id)
            FROM actual_cost_entries d JOIN cost_revisions cr ON cr.org_id=d.org_id AND cr.id=d.current_revision_id
            WHERE d.org_id=oid AND EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.source_type='contract' AND l.source_id=cid AND l.cost_id=d.id)),'[]'::jsonb),
        'children',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',ch.id,'version',ch.version,'lifecycle',ch.lifecycle) ORDER BY ch.id)
            FROM contracts ch WHERE ch.org_id=oid AND ch.parent_contract_id=cid AND ch.deleted_at IS NULL),'[]'::jsonb))
    FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
    LEFT JOIN contract_finance_heads h ON h.org_id=c.org_id AND h.contract_id=c.id WHERE c.org_id=oid AND c.id=cid
$$;
CREATE FUNCTION contract_closure_has_unsettled(b jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
    SELECT b->>'schedule_mode' IS NULL OR
        EXISTS(SELECT 1 FROM jsonb_array_elements(b->'nodes')n WHERE (n->>'amount')::numeric<>(n->>'cash_amount')::numeric
            OR (n->>'has_unattributed_refunds')::boolean OR NOT (n->>'review_current')::boolean OR (n->>'conditions_satisfied')::boolean IS NOT TRUE) OR
        EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')d WHERE d->>'state'='confirmed' AND (d->>'amount')::numeric<>(d->>'paid_amount')::numeric) OR
        EXISTS(SELECT 1 FROM jsonb_array_elements(b->'children')ch WHERE ch->>'lifecycle' NOT IN ('completed','terminated'))
$$;
CREATE FUNCTION can_read_contract_closure(oid uuid,uid uuid,cid uuid,ch text,ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT can_access(oid,uid,'contract.read','contract',c.id,c.project_id,c.creator_id,c.classification,ch,ai)
        AND (r.payment_schedule->>'mode'='no_payment' AND NOT EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=oid AND e.contract_id=cid)
            AND NOT EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.source_type='contract' AND l.source_id=cid)
            OR (can_access(oid,uid,'payment.read','payment',c.id,c.project_id,c.creator_id,c.classification,ch,ai)
                AND (r.direction='receivable' OR can_access(oid,uid,'cost.read','cost',c.id,c.project_id,c.creator_id,c.classification,ch,ai))))
        AND NOT EXISTS(SELECT 1 FROM actual_cost_entries d WHERE d.org_id=oid
            AND (EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.cost_id=d.id AND l.source_type='contract' AND l.source_id=cid)
                OR EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=oid AND e.contract_id=cid AND e.cost_id=d.id))
            AND (NOT can_read_cost(oid,uid,d.id,d.current_revision_id,ch,ai) OR EXISTS(SELECT 1 FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id AND NOT can_read_payment(oid,uid,a.payment_id,ch,ai))))
        AND NOT EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=oid AND e.contract_id=cid AND e.payment_id IS NOT NULL
            AND (NOT can_read_payment(oid,uid,e.payment_id,ch,ai) OR EXISTS(SELECT 1 FROM payment_records p JOIN payment_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id
                WHERE p.org_id=oid AND pr.refund_of_id=e.payment_id AND p.state IN ('recorded','reversed') AND NOT can_read_payment(oid,uid,p.id,ch,ai))))
        AND NOT EXISTS(SELECT 1 FROM contracts child WHERE child.org_id=oid AND (child.parent_contract_id=cid AND child.deleted_at IS NULL OR child.id IN
            (SELECT (x->>'id')::uuid FROM contract_closures z CROSS JOIN LATERAL jsonb_array_elements(z.basis->'children')x WHERE z.org_id=oid AND z.contract_id=cid))
            AND NOT can_access(oid,uid,'contract.read','contract',child.id,child.project_id,child.creator_id,child.classification,ch,ai))
    FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id WHERE c.org_id=oid AND c.id=cid
$$;
CREATE FUNCTION prepare_contract_closure() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts;s sessions;u users;b jsonb;start_on date;BEGIN
    SELECT * INTO STRICT c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.contract_id FOR UPDATE;
    IF c.lifecycle<>'effective' OR c.deleted_at IS NOT NULL OR c.version+1<>NEW.contract_version OR c.active_revision_id IS DISTINCT FROM NEW.revision_id THEN
        RAISE EXCEPTION 'closure requires the exact currently effective contract' USING ERRCODE='23514';END IF;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
    IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR u.id IS NULL OR u.status<>'active' OR u.auth_version<>s.auth_version
        OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at OR s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp() THEN
        RAISE EXCEPTION 'closure requires the actual current human MFA session' USING ERRCODE='23514';END IF;
    SELECT start_date INTO STRICT start_on FROM contract_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id;
    IF NEW.occurred_on<start_on OR NEW.occurred_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date THEN
        RAISE EXCEPTION 'closure occurrence date is outside the effective interval' USING ERRCODE='23514';END IF;
    PERFORM d.id FROM actual_cost_entries d WHERE d.org_id=NEW.org_id AND (EXISTS(
        SELECT 1 FROM cost_links l WHERE l.org_id=d.org_id AND l.cost_id=d.id AND l.source_type='contract' AND l.source_id=c.id)
        OR EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=d.org_id AND e.contract_id=c.id AND e.cost_id=d.id)) ORDER BY d.id FOR UPDATE;
    PERFORM p.id FROM payment_records p WHERE p.org_id=NEW.org_id AND (p.id IN(
        SELECT e.payment_id FROM contract_finance_events e WHERE e.org_id=NEW.org_id AND e.contract_id=c.id)
        OR p.id IN(SELECT a.payment_id FROM payment_allocations a WHERE a.org_id=NEW.org_id AND a.cost_id IN(
            SELECT l.cost_id FROM cost_links l WHERE l.org_id=NEW.org_id AND l.source_type='contract' AND l.source_id=c.id))
        OR p.id IN(SELECT pr.payment_id FROM payment_revisions pr WHERE pr.org_id=NEW.org_id AND pr.refund_of_id IN(
            SELECT e.payment_id FROM contract_finance_events e WHERE e.org_id=NEW.org_id AND e.contract_id=c.id))) ORDER BY p.id FOR UPDATE;
    b:=contract_closure_snapshot(NEW.org_id,NEW.contract_id);
    IF jsonb_array_length(b->'costs')>1000 OR jsonb_array_length(b->'children')>1000 THEN
        RAISE EXCEPTION 'closure basis exceeds its explicitly bounded review scope' USING ERRCODE='23514';END IF;
    IF b IS NULL OR b IS DISTINCT FROM NEW.basis OR md5(b::text)<>NEW.fingerprint OR (b->>'finance_version')::bigint<>NEW.finance_version THEN
        RAISE EXCEPTION 'closure must pin the actual current financial and legal basis' USING ERRCODE='23514';END IF;
    IF EXISTS(SELECT 1 FROM contracts child WHERE child.org_id=NEW.org_id AND child.parent_contract_id=c.id AND child.lifecycle='effective' AND child.deleted_at IS NULL FOR SHARE) THEN
        RAISE EXCEPTION 'effective supplements must be resolved before closing their parent' USING ERRCODE='23514';END IF;
    IF (NEW.action='completed' AND contract_closure_has_unsettled(b)) OR
        (NEW.action='terminated' AND contract_closure_has_unsettled(b) AND (NOT NEW.acknowledge_unsettled OR length(btrim(NEW.unsettled_disposition))<1)) THEN
        RAISE EXCEPTION 'closure cannot erase its unresolved obligations' USING ERRCODE='23514';END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_closure_prepare BEFORE INSERT ON contract_closures FOR EACH ROW EXECUTE FUNCTION prepare_contract_closure();
CREATE OR REPLACE FUNCTION preserve_contract_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF OLD.lifecycle<>'draft' AND (NEW.current_revision_id<>OLD.current_revision_id OR NEW.approved_revision_id IS DISTINCT FROM OLD.approved_revision_id OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at) THEN
        RAISE EXCEPTION 'approved contract content must use a separate amendment' USING ERRCODE='23514';END IF;
    IF OLD.active_revision_id IS NOT NULL AND NEW.active_revision_id IS DISTINCT FROM OLD.active_revision_id THEN
        RAISE EXCEPTION 'active content cannot be rewritten' USING ERRCODE='23514';END IF;
    IF NEW.lifecycle<>OLD.lifecycle AND NOT((OLD.lifecycle='draft' AND NEW.lifecycle='pending_signature') OR
        (OLD.lifecycle='pending_signature' AND NEW.lifecycle='effective') OR (OLD.lifecycle='effective' AND NEW.lifecycle IN ('completed','terminated'))) THEN
        RAISE EXCEPTION 'invalid contract lifecycle transition' USING ERRCODE='23514';END IF;RETURN NEW;
END; $$;
CREATE FUNCTION check_contract_closure_consistency() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE oid uuid:=NEW.org_id;cid uuid;c contracts;z contract_closures;parent_state text;BEGIN
    IF TG_TABLE_NAME='contracts' THEN cid:=NEW.id;
    ELSIF TG_TABLE_NAME='contract_closures' THEN cid:=NEW.contract_id;
    ELSE SELECT contract_id INTO STRICT cid FROM contract_closures WHERE org_id=oid AND id=NEW.closure_id;END IF;
    SELECT * INTO STRICT c FROM contracts WHERE org_id=oid AND id=cid;
    SELECT * INTO z FROM contract_closures WHERE org_id=oid AND contract_id=cid;
    IF c.lifecycle IN ('completed','terminated') THEN
        IF z.id IS NULL OR z.action<>c.lifecycle OR z.contract_version<>c.version OR z.revision_id IS DISTINCT FROM c.active_revision_id OR
            z.evidence_count<>(SELECT count(*) FROM contract_closure_files WHERE org_id=oid AND closure_id=z.id) THEN
            RAISE EXCEPTION 'closed contract head requires its complete immutable human record' USING ERRCODE='23514';END IF;
    ELSIF z.id IS NOT NULL THEN RAISE EXCEPTION 'closure record is detached from the contract head' USING ERRCODE='23514';END IF;
    IF c.lifecycle='effective' AND c.parent_contract_id IS NOT NULL THEN
        SELECT lifecycle INTO STRICT parent_state FROM contracts WHERE org_id=oid AND id=c.parent_contract_id FOR SHARE;
        IF parent_state<>'effective' THEN RAISE EXCEPTION 'supplement cannot remain effective under a closed parent' USING ERRCODE='23514';END IF;
    END IF;RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER contract_closure_head AFTER INSERT OR UPDATE ON contracts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_contract_closure_consistency();
CREATE CONSTRAINT TRIGGER contract_closure_complete AFTER INSERT ON contract_closures DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_contract_closure_consistency();
CREATE CONSTRAINT TRIGGER contract_closure_files_complete AFTER INSERT ON contract_closure_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_contract_closure_consistency();
CREATE TRIGGER contract_closure_immutable BEFORE UPDATE OR DELETE ON contract_closures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER contract_closure_files_immutable BEFORE UPDATE OR DELETE ON contract_closure_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CONSTRAINT file_version_references_owner_type_check CHECK(owner_type IN
    ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','contract_closure','deliverable','recovery_set'));
CREATE FUNCTION register_contract_closure_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'contract_closure',NEW.closure_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;RETURN NEW;
END; $$;
CREATE TRIGGER contract_closure_file_reference AFTER INSERT ON contract_closure_files FOR EACH ROW EXECUTE FUNCTION register_contract_closure_file();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'contract_closer','合同完成与终止登记人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN(VALUES('contract.read'),('contract.close'))a(action) WHERE key='contract_closer';
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT ON contract_closures,contract_closure_files TO copypaste_app;REVOKE UPDATE,DELETE ON contract_closures,contract_closure_files FROM copypaste_app;
END IF;END; $$;

-- Financial facts may be reconciled after legal closure without reopening it.
CREATE OR REPLACE FUNCTION prepare_contract_finance_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts;m contract_milestones;r contract_revisions;s sessions;u users;src contract_finance_events;pc actual_cost_entries;cr cost_revisions;p payment_records;pr payment_revisions;original payment_records;op payment_revisions;exp integer;next_seq bigint;BEGIN
    SELECT * INTO STRICT c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.contract_id FOR UPDATE;
    SELECT * INTO STRICT m FROM contract_milestones WHERE org_id=NEW.org_id AND id=NEW.milestone_id;
    SELECT * INTO STRICT r FROM contract_revisions WHERE org_id=NEW.org_id AND id=c.active_revision_id;
    IF c.lifecycle NOT IN ('effective','completed','terminated') OR c.deleted_at IS NOT NULL OR m.contract_id<>c.id OR m.revision_id<>c.active_revision_id OR NEW.contract_revision_id<>m.revision_id THEN
        RAISE EXCEPTION 'financial attribution requires an exact effective milestone' USING ERRCODE='23514';END IF;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
    IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR u.id IS NULL OR u.status<>'active' OR u.auth_version<>s.auth_version
        OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at OR s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp() THEN
        RAISE EXCEPTION 'contract reconciliation needs the actual current human MFA session' USING ERRCODE='23514';END IF;
    SELECT COALESCE(max(seq),0)+1 INTO next_seq FROM contract_finance_events WHERE org_id=NEW.org_id AND contract_id=NEW.contract_id;
    IF NEW.seq<>next_seq THEN RAISE EXCEPTION 'contract finance sequence is not continuous' USING ERRCODE='23514';END IF;
    SELECT exponent INTO STRICT exp FROM currency_definitions WHERE code=m.currency;
    IF NEW.amount IS NOT NULL AND NEW.amount<>trunc(NEW.amount,exp) THEN RAISE EXCEPTION 'attribution has fractional minor units' USING ERRCODE='23514';END IF;
    IF NEW.source_event_id IS NOT NULL THEN
        SELECT * INTO STRICT src FROM contract_finance_events WHERE org_id=NEW.org_id AND id=NEW.source_event_id;
        IF src.contract_id<>NEW.contract_id OR src.milestone_id<>NEW.milestone_id OR src.kind NOT IN ('cost','cash','refund') OR (NEW.kind='refund' AND src.kind<>'cash') THEN
            RAISE EXCEPTION 'inverse attribution must pin its own original milestone source' USING ERRCODE='23514';END IF;
    END IF;
    IF NEW.kind='cost' THEN
        SELECT * INTO STRICT pc FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=NEW.cost_id FOR UPDATE;
        SELECT * INTO STRICT cr FROM cost_revisions WHERE org_id=NEW.org_id AND id=pc.current_revision_id;
        IF m.direction<>'payable' OR pc.state<>'confirmed' OR pc.entry_kind<>'expense' OR pc.version<>NEW.source_version OR cr.id<>NEW.source_revision_id OR cr.content_hash<>NEW.source_content_hash
            OR cr.currency<>m.currency OR cr.counterparty_id IS DISTINCT FROM r.counterparty_id OR classification_rank(pc.classification)<classification_rank(c.classification)
            OR NOT EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=NEW.org_id AND l.cost_id=pc.id AND l.source_type='contract' AND l.source_id=c.id AND l.revision_id=m.revision_id) THEN
            RAISE EXCEPTION 'cost attribution must pin a confirmed matching contractual cost' USING ERRCODE='23514';END IF;
    ELSIF NEW.kind IN ('cash','refund') THEN
        PERFORM q.id FROM payment_records q WHERE q.org_id=NEW.org_id AND q.id IN (NEW.payment_id,src.payment_id) ORDER BY q.id FOR UPDATE;
        SELECT * INTO STRICT p FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.payment_id;
        SELECT * INTO STRICT pr FROM payment_revisions WHERE org_id=NEW.org_id AND id=p.current_revision_id;
        IF p.state<>'recorded' OR p.record_kind<>'payment' OR p.version<>NEW.source_version OR pr.id<>NEW.source_revision_id OR pr.content_hash<>NEW.source_content_hash OR pr.currency<>m.currency
            OR p.project_id IS DISTINCT FROM c.project_id OR classification_rank(p.classification)<classification_rank(c.classification) OR pr.counterparty_id IS DISTINCT FROM r.counterparty_id OR pr.employee_id IS NOT NULL THEN
            RAISE EXCEPTION 'cash attribution must pin recorded matching cash' USING ERRCODE='23514';END IF;
        IF NEW.kind='cash' AND pr.direction IS DISTINCT FROM (CASE m.direction WHEN 'payable' THEN 'outgoing' ELSE 'incoming' END) THEN
            RAISE EXCEPTION 'cash direction differs from contractual obligation' USING ERRCODE='23514';END IF;
        IF NEW.kind='refund' THEN
            SELECT * INTO STRICT original FROM payment_records WHERE org_id=NEW.org_id AND id=src.payment_id;
            SELECT * INTO STRICT op FROM payment_revisions WHERE org_id=NEW.org_id AND id=original.current_revision_id;
            IF original.state<>'recorded' OR pr.paid_at<op.paid_at OR (m.direction='payable' AND (pr.direction<>'refund' OR pr.refund_of_id IS DISTINCT FROM original.id))
                OR (m.direction='receivable' AND (pr.direction<>'outgoing' OR pr.refund_of_id IS NOT NULL)) THEN
                RAISE EXCEPTION 'return must match the exact original cash and direction' USING ERRCODE='23514';END IF;
        END IF;
    ELSIF NEW.kind='release' THEN
        IF src.cost_id IS NOT NULL THEN PERFORM id FROM actual_cost_entries WHERE org_id=NEW.org_id AND id=src.cost_id FOR UPDATE;END IF;
        PERFORM id FROM payment_records WHERE org_id=NEW.org_id AND id IN (src.payment_id,(SELECT payment_id FROM contract_finance_events WHERE org_id=NEW.org_id AND id=src.source_event_id)) ORDER BY id FOR UPDATE;
    ELSIF NEW.kind='review' THEN
        PERFORM q.id FROM actual_cost_entries q WHERE q.org_id=NEW.org_id AND q.id IN
            (SELECT cost_id FROM contract_finance_events WHERE org_id=NEW.org_id AND milestone_id=NEW.milestone_id) ORDER BY q.id FOR UPDATE;
        PERFORM q.id FROM payment_records q WHERE q.org_id=NEW.org_id AND (q.id IN
            (SELECT payment_id FROM contract_finance_events WHERE org_id=NEW.org_id AND milestone_id=NEW.milestone_id)
            OR q.id IN (SELECT v.payment_id FROM payment_revisions v WHERE v.org_id=NEW.org_id AND v.refund_of_id IN
                (SELECT payment_id FROM contract_finance_events WHERE org_id=NEW.org_id AND milestone_id=NEW.milestone_id))) ORDER BY q.id FOR UPDATE;
        IF NEW.review_snapshot IS DISTINCT FROM contract_finance_snapshot(NEW.org_id,NEW.milestone_id) OR NEW.review_fingerprint<>md5(NEW.review_snapshot::text) THEN
            RAISE EXCEPTION 'review snapshot differs from current actual sources' USING ERRCODE='23514';END IF;
        IF NOT NEW.acknowledge_difference AND m.amount<>(SELECT COALESCE(sum(contract_finance_effective(NEW.org_id,e.id)),0) FROM contract_finance_events e WHERE e.org_id=NEW.org_id AND e.milestone_id=NEW.milestone_id AND e.kind IN ('cash','refund')) THEN
            RAISE EXCEPTION 'review must explicitly acknowledge the cash difference' USING ERRCODE='23514';END IF;
        IF NOT NEW.acknowledge_unattributed_refunds AND EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=NEW.org_id AND e.milestone_id=NEW.milestone_id AND e.kind='cash' AND contract_unattributed_refunds(NEW.org_id,e.payment_id)>0) THEN
            RAISE EXCEPTION 'review must explicitly verify unattributed refunds' USING ERRCODE='23514';END IF;
    END IF;
    IF NEW.kind IN ('cost','cash','refund') AND (SELECT count(*) FROM contract_finance_events WHERE org_id=NEW.org_id AND milestone_id=NEW.milestone_id AND kind IN ('cost','cash','refund'))>=100 THEN
        RAISE EXCEPTION 'milestone has reached the explicit source limit' USING ERRCODE='23514';END IF;
    RETURN NEW;
END; $$;
