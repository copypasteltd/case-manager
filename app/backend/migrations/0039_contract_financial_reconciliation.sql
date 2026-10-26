-- Contract attribution is a second view of existing facts, never another expense or cash entry.
CREATE TABLE contract_finance_heads (
    org_id uuid NOT NULL,contract_id uuid NOT NULL,version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
    current_event_id uuid NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(org_id,contract_id),
    FOREIGN KEY(org_id,contract_id) REFERENCES contracts(org_id,id)
);
CREATE TABLE contract_finance_events (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,contract_id uuid NOT NULL,milestone_id uuid NOT NULL,contract_revision_id uuid NOT NULL,
    seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991),kind text NOT NULL CHECK(kind IN ('cost','cash','refund','release','review')),
    cost_id uuid,payment_id uuid,source_version bigint,source_revision_id uuid,source_content_hash text,
    source_event_id uuid,amount numeric(20,6),review_snapshot jsonb,review_fingerprint text,
    conditions_satisfied boolean,acknowledge_difference boolean,acknowledge_unattributed_refunds boolean,
    actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id),UNIQUE(org_id,contract_id,id),UNIQUE(org_id,contract_id,seq),
    FOREIGN KEY(org_id,contract_id,contract_revision_id) REFERENCES contract_revisions(org_id,contract_id,id),
    FOREIGN KEY(org_id,milestone_id) REFERENCES contract_milestones(org_id,id),
    FOREIGN KEY(org_id,cost_id,source_revision_id) REFERENCES cost_revisions(org_id,cost_id,id),
    FOREIGN KEY(org_id,payment_id,source_revision_id) REFERENCES payment_revisions(org_id,payment_id,id),
    FOREIGN KEY(org_id,source_event_id) REFERENCES contract_finance_events(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    CHECK((kind='cost' AND cost_id IS NOT NULL AND payment_id IS NULL) OR
          (kind IN ('cash','refund') AND cost_id IS NULL AND payment_id IS NOT NULL) OR
          (kind IN ('release','review') AND cost_id IS NULL AND payment_id IS NULL)),
    CHECK((kind IN ('refund','release'))=(source_event_id IS NOT NULL)),
    CHECK((kind='review' AND amount IS NULL AND source_version IS NULL AND source_revision_id IS NULL AND source_content_hash IS NULL
           AND review_snapshot IS NOT NULL AND review_fingerprint IS NOT NULL AND jsonb_typeof(review_snapshot)='object' AND review_fingerprint~'^[0-9a-f]{32}$'
           AND conditions_satisfied IS NOT NULL AND acknowledge_difference IS NOT NULL AND acknowledge_unattributed_refunds IS NOT NULL)
       OR (kind<>'review' AND amount IS NOT NULL AND amount>0 AND review_snapshot IS NULL AND review_fingerprint IS NULL
           AND conditions_satisfied IS NULL AND acknowledge_difference IS NULL AND acknowledge_unattributed_refunds IS NULL)),
    CHECK((kind IN ('cost','cash','refund') AND source_version IS NOT NULL AND source_version>0 AND source_revision_id IS NOT NULL AND source_content_hash IS NOT NULL AND source_content_hash~'^[0-9a-f]{64}$')
       OR (kind IN ('release','review') AND source_version IS NULL AND source_revision_id IS NULL AND source_content_hash IS NULL))
);
ALTER TABLE contract_finance_heads ADD FOREIGN KEY(org_id,contract_id,current_event_id) REFERENCES contract_finance_events(org_id,contract_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE contract_finance_event_files (
    org_id uuid NOT NULL,event_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,event_id,version_id),
    FOREIGN KEY(org_id,event_id) REFERENCES contract_finance_events(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE INDEX contract_finance_node ON contract_finance_events(org_id,contract_id,milestone_id,seq DESC);
CREATE INDEX contract_finance_cost ON contract_finance_events(org_id,cost_id) WHERE cost_id IS NOT NULL;
CREATE INDEX contract_finance_payment ON contract_finance_events(org_id,payment_id) WHERE payment_id IS NOT NULL;
CREATE INDEX contract_finance_source ON contract_finance_events(org_id,source_event_id) WHERE source_event_id IS NOT NULL;

CREATE FUNCTION contract_finance_remaining(oid uuid,eid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT e.amount-COALESCE((SELECT sum(r.amount) FROM contract_finance_events r WHERE r.org_id=e.org_id AND r.source_event_id=e.id AND r.kind='release'),0)
    FROM contract_finance_events e WHERE e.org_id=oid AND e.id=eid AND e.kind IN ('cost','cash','refund')
$$;
CREATE FUNCTION contract_finance_effective(oid uuid,eid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN e.kind='cost' AND c.state='confirmed' THEN contract_finance_remaining(oid,eid)
        WHEN e.kind='cash' AND p.state='recorded' THEN contract_finance_remaining(oid,eid)
        WHEN e.kind='refund' AND p.state='recorded' AND original.state='recorded' THEN -contract_finance_remaining(oid,eid) ELSE 0 END
    FROM contract_finance_events e LEFT JOIN actual_cost_entries c ON c.org_id=e.org_id AND c.id=e.cost_id
    LEFT JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id
    LEFT JOIN contract_finance_events src ON src.org_id=e.org_id AND src.id=e.source_event_id
    LEFT JOIN payment_records original ON original.org_id=src.org_id AND original.id=src.payment_id WHERE e.org_id=oid AND e.id=eid
$$;
CREATE FUNCTION contract_cash_return_reserved(oid uuid,pid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(contract_finance_remaining(oid,e.id)),0) FROM contract_finance_events e
    JOIN contract_finance_events src ON src.org_id=e.org_id AND src.id=e.source_event_id
    JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id
    JOIN payment_records original ON original.org_id=src.org_id AND original.id=src.payment_id
    JOIN payment_revisions r ON r.org_id=original.org_id AND r.id=original.current_revision_id
    WHERE e.org_id=oid AND e.payment_id=pid AND e.kind='refund' AND p.state='recorded' AND original.state='recorded' AND r.direction='incoming'
$$;
-- Used for dated cash reports, including the original and its explicit reversal.
CREATE FUNCTION contract_cash_return_share(oid uuid,pid uuid,scope uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(contract_finance_remaining(oid,e.id)),0) FROM contract_finance_events e
    JOIN contract_finance_events src ON src.org_id=e.org_id AND src.id=e.source_event_id
    JOIN payment_records original ON original.org_id=src.org_id AND original.id=src.payment_id
    JOIN payment_revisions r ON r.org_id=original.org_id AND r.id=original.current_revision_id
    JOIN contracts c ON c.org_id=e.org_id AND c.id=e.contract_id
    WHERE e.org_id=oid AND e.payment_id=pid AND e.kind='refund' AND r.direction='incoming' AND c.project_id IS NOT DISTINCT FROM scope
$$;
CREATE FUNCTION contract_unattributed_refunds(oid uuid,pid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(v.amount-COALESCE((SELECT sum(contract_finance_remaining(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id AND e.kind='refund'),0)),0)
    FROM payment_records p JOIN payment_revisions v ON v.org_id=p.org_id AND v.id=p.current_revision_id
    WHERE p.org_id=oid AND v.refund_of_id=pid AND p.record_kind='payment' AND p.state='recorded'
$$;
CREATE FUNCTION contract_refund_family_fingerprint(oid uuid,pid uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT md5(COALESCE(string_agg(p.id::text||':'||p.version::text||':'||p.current_revision_id::text||':'||p.state||':'||v.amount::text,',' ORDER BY p.id),''))
    FROM payment_records p JOIN payment_revisions v ON v.org_id=p.org_id AND v.id=p.current_revision_id
    WHERE p.org_id=oid AND v.refund_of_id=pid AND p.record_kind='payment' AND p.state IN ('recorded','reversed')
$$;
CREATE FUNCTION contract_finance_snapshot(oid uuid,mid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('milestone_id',m.id,'contract_revision_id',m.revision_id,'sources',COALESCE((
        SELECT jsonb_agg(jsonb_build_object('event_id',e.id,'kind',e.kind,'amount',e.amount::text,'remaining_amount',contract_finance_remaining(oid,e.id)::text,
            'source_event_id',e.source_event_id,'cost_id',e.cost_id,'payment_id',e.payment_id,'pinned_revision_id',e.source_revision_id,
            'current_version',COALESCE(c.version,p.version),'current_revision_id',COALESCE(c.current_revision_id,p.current_revision_id),
            'content_hash',COALESCE(cr.content_hash,pr.content_hash),'state',COALESCE(c.state,p.state),'source_amount',COALESCE(cr.amount,pr.amount)::text,
            'currency',COALESCE(cr.currency,pr.currency),'direction',pr.direction,'effective_amount',contract_finance_effective(oid,e.id)::text,
            'unattributed_refund_amount',CASE WHEN e.kind='cash' THEN contract_unattributed_refunds(oid,e.payment_id)::text ELSE '0' END,
            'refund_family_fingerprint',CASE WHEN e.kind='cash' THEN contract_refund_family_fingerprint(oid,e.payment_id) ELSE NULL END) ORDER BY e.seq)
        FROM contract_finance_events e LEFT JOIN actual_cost_entries c ON c.org_id=e.org_id AND c.id=e.cost_id
        LEFT JOIN cost_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
        LEFT JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id LEFT JOIN payment_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id
        WHERE e.org_id=oid AND e.milestone_id=mid AND e.kind IN ('cost','cash','refund')),'[]'::jsonb))
    FROM contract_milestones m WHERE m.org_id=oid AND m.id=mid
$$;

-- Preserve the entire existing cash/cost ACL; add historical contract disclosure.
ALTER FUNCTION can_read_payment(uuid,uuid,uuid,text,boolean) RENAME TO can_read_payment_0037_base;
CREATE FUNCTION can_read_payment(p_org uuid,p_user uuid,p_id uuid,p_channel text,p_ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT can_read_payment_0037_base(p_org,p_user,p_id,p_channel,p_ai) AND NOT EXISTS(
        SELECT 1 FROM contract_finance_events e JOIN contracts c ON c.org_id=e.org_id AND c.id=e.contract_id
        WHERE e.org_id=p_org AND e.payment_id=p_id AND NOT can_access(c.org_id,p_user,'contract.read','contract',c.id,c.project_id,c.creator_id,c.classification,p_channel,p_ai))
$$;
ALTER FUNCTION validate_payment_balance(uuid,uuid) RENAME TO validate_payment_balance_0037_base;
CREATE FUNCTION validate_payment_balance(oid uuid,pid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p payment_records;r payment_revisions;allocated numeric;refunds numeric;reserved numeric;BEGIN
    PERFORM validate_payment_balance_0037_base(oid,pid);
    SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    IF p.record_kind<>'payment' OR r.direction IS DISTINCT FROM 'outgoing' THEN RETURN;END IF;
    reserved:=contract_cash_return_reserved(oid,pid);
    SELECT COALESCE(sum(x.effect*x.amount),0) INTO allocated FROM payment_allocations x JOIN payment_records q ON q.org_id=x.org_id AND q.id=x.payment_id
        JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id WHERE x.org_id=oid AND (q.id=pid OR q.reverses_id=pid OR v.refund_of_id=pid);
    SELECT COALESCE(sum(v.amount),0) INTO refunds FROM payment_records q JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id
        WHERE q.org_id=oid AND v.refund_of_id=pid AND q.state='recorded' AND q.record_kind='payment';
    IF allocated+reserved>COALESCE(r.amount,0)-refunds THEN RAISE EXCEPTION 'receipt return reserves cash already used for expenses' USING ERRCODE='23514';END IF;
END; $$;
CREATE FUNCTION contract_finance_cash_guard(oid uuid,pid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p payment_records;r payment_revisions;used numeric;BEGIN
    SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    SELECT COALESCE(sum(contract_finance_remaining(oid,e.id)),0) INTO used FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=pid AND e.kind IN ('cash','refund');
    IF used>COALESCE(r.amount,0) THEN RAISE EXCEPTION 'cash attribution exceeds the source amount' USING ERRCODE='23514';END IF;
    IF p.state='reversed' AND EXISTS(SELECT 1 FROM contract_finance_events src JOIN contract_finance_events e ON e.org_id=src.org_id AND e.source_event_id=src.id AND e.kind='refund'
        JOIN payment_records q ON q.org_id=e.org_id AND q.id=e.payment_id WHERE src.org_id=oid AND src.payment_id=pid AND src.kind='cash' AND q.state='recorded' AND contract_finance_remaining(oid,e.id)>0) THEN
        RAISE EXCEPTION 'reverse or detach actual returns before reversing the original receipt' USING ERRCODE='23514';END IF;
    PERFORM validate_payment_balance(oid,pid);
END; $$;
CREATE FUNCTION check_contract_finance_cash() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM contract_finance_cash_guard(NEW.org_id,NEW.id);RETURN NULL;END; $$;
CREATE CONSTRAINT TRIGGER contract_cash_change AFTER INSERT OR UPDATE ON payment_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_contract_finance_cash();

CREATE FUNCTION prepare_contract_finance_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts;m contract_milestones;r contract_revisions;s sessions;u users;src contract_finance_events;pc actual_cost_entries;cr cost_revisions;p payment_records;pr payment_revisions;original payment_records;op payment_revisions;exp integer;next_seq bigint;BEGIN
    SELECT * INTO STRICT c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.contract_id FOR UPDATE;
    SELECT * INTO STRICT m FROM contract_milestones WHERE org_id=NEW.org_id AND id=NEW.milestone_id;
    SELECT * INTO STRICT r FROM contract_revisions WHERE org_id=NEW.org_id AND id=c.active_revision_id;
    IF c.lifecycle<>'effective' OR c.deleted_at IS NOT NULL OR m.contract_id<>c.id OR m.revision_id<>c.active_revision_id OR NEW.contract_revision_id<>m.revision_id THEN
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
CREATE TRIGGER contract_finance_prepare BEFORE INSERT ON contract_finance_events FOR EACH ROW EXECUTE FUNCTION prepare_contract_finance_event();
CREATE FUNCTION validate_contract_finance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE oid uuid:=NEW.org_id;cid uuid;h contract_finance_heads;e contract_finance_events;x contract_finance_events;used numeric;rnum numeric;source_amount numeric;cash_amount numeric;BEGIN
    IF TG_TABLE_NAME='contract_finance_heads' THEN cid:=NEW.contract_id;
    ELSIF TG_TABLE_NAME='contract_finance_event_files' THEN SELECT contract_id INTO STRICT cid FROM contract_finance_events WHERE org_id=oid AND id=NEW.event_id;
    ELSE cid:=NEW.contract_id;END IF;
    SELECT * INTO STRICT h FROM contract_finance_heads WHERE org_id=oid AND contract_id=cid;
    SELECT * INTO STRICT e FROM contract_finance_events WHERE org_id=oid AND id=h.current_event_id;
    IF e.contract_id<>cid OR e.seq<>h.version OR h.version<>(SELECT count(*) FROM contract_finance_events WHERE org_id=oid AND contract_id=cid)
        OR e.seq<>(SELECT max(seq) FROM contract_finance_events WHERE org_id=oid AND contract_id=cid) THEN
        RAISE EXCEPTION 'contract finance head is detached or incomplete' USING ERRCODE='23514';END IF;
    FOR x IN SELECT * FROM contract_finance_events WHERE org_id=oid AND contract_id=cid AND kind IN ('cost','cash','refund') LOOP
        rnum:=contract_finance_remaining(oid,x.id);
        IF rnum<0 THEN RAISE EXCEPTION 'release exceeds the original attribution' USING ERRCODE='23514';END IF;
        IF x.kind='cash' THEN
            SELECT COALESCE(sum(contract_finance_remaining(oid,f.id)),0) INTO used FROM contract_finance_events f JOIN payment_records p ON p.org_id=f.org_id AND p.id=f.payment_id
                WHERE f.org_id=oid AND f.source_event_id=x.id AND f.kind='refund' AND p.state='recorded';
            IF used>rnum THEN RAISE EXCEPTION 'returns exceed remaining original attribution' USING ERRCODE='23514';END IF;
        END IF;
        IF x.cost_id IS NOT NULL THEN
            SELECT r.amount INTO source_amount FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.id=x.cost_id;
            SELECT COALESCE(sum(contract_finance_remaining(oid,f.id)),0) INTO used FROM contract_finance_events f WHERE f.org_id=oid AND f.cost_id=x.cost_id AND f.kind='cost';
            IF used>source_amount THEN RAISE EXCEPTION 'cost attributed twice across contract milestones' USING ERRCODE='23514';END IF;
        ELSE PERFORM contract_finance_cash_guard(oid,x.payment_id);END IF;
    END LOOP;
    FOR x IN SELECT * FROM contract_finance_events WHERE org_id=oid AND contract_id=cid AND kind='review' LOOP
        IF NOT EXISTS(SELECT 1 FROM contract_finance_event_files f WHERE f.org_id=oid AND f.event_id=x.id) THEN RAISE EXCEPTION 'review requires fixed evidence' USING ERRCODE='23514';END IF;
    END LOOP;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER contract_finance_head_complete AFTER INSERT OR UPDATE ON contract_finance_heads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_contract_finance();
CREATE CONSTRAINT TRIGGER contract_finance_event_complete AFTER INSERT ON contract_finance_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_contract_finance();
CREATE CONSTRAINT TRIGGER contract_finance_files_complete AFTER INSERT ON contract_finance_event_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_contract_finance();
CREATE FUNCTION preserve_contract_finance_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.org_id<>OLD.org_id OR NEW.contract_id<>OLD.contract_id OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'contract finance head must advance once' USING ERRCODE='23514';END IF;RETURN NEW;
END; $$;
CREATE TRIGGER contract_finance_head_identity BEFORE UPDATE ON contract_finance_heads FOR EACH ROW EXECUTE FUNCTION preserve_contract_finance_head();
CREATE TRIGGER contract_finance_head_no_delete BEFORE DELETE ON contract_finance_heads FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER contract_finance_events_immutable BEFORE UPDATE OR DELETE ON contract_finance_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER contract_finance_files_immutable BEFORE UPDATE OR DELETE ON contract_finance_event_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','deliverable','recovery_set'));
CREATE FUNCTION register_contract_finance_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM contract_finance_events WHERE org_id=NEW.org_id AND id=NEW.event_id AND kind='review') THEN RAISE EXCEPTION 'fixed review files require the same review event' USING ERRCODE='23514';END IF;
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) SELECT gen_random_uuid(),NEW.org_id,file_id,id,'contract_finance_event',NEW.event_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_finance_file_reference AFTER INSERT ON contract_finance_event_files FOR EACH ROW EXECUTE FUNCTION register_contract_finance_file();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'contract_reconciler','合同对账人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('contract.read'),('contract.reconcile')) a(action) WHERE key='contract_reconciler' ON CONFLICT DO NOTHING;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT,UPDATE ON contract_finance_heads TO copypaste_app;
    GRANT SELECT,INSERT ON contract_finance_events,contract_finance_event_files TO copypaste_app;
    REVOKE DELETE ON contract_finance_heads FROM copypaste_app;
    REVOKE UPDATE,DELETE ON contract_finance_events,contract_finance_event_files FROM copypaste_app;
END IF;END; $$;
