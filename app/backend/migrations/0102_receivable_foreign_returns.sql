-- Reviewed foreign outgoing returns for receivable contracts. Existing facts and
-- historic same-currency / incoming-refund snapshots remain byte-for-byte valid.
ALTER TABLE contract_finance_events ADD COLUMN return_cash_amount numeric(20,6),
    ADD COLUMN return_fx_rate numeric(24,10), ADD COLUMN return_fx_source text;
ALTER TABLE contract_finance_events ADD CONSTRAINT contract_return_units CHECK(
    (return_cash_amount IS NULL AND return_fx_rate IS NULL AND return_fx_source IS NULL) OR
    (return_cash_amount IS NOT NULL AND return_cash_amount>=0 AND
     ((kind='refund' AND return_cash_amount>0 AND refund_fx_acknowledged AND return_fx_rate IS NOT NULL
       AND return_fx_rate>0 AND return_fx_rate<100000000000000 AND return_fx_source IS NOT NULL
       AND length(btrim(return_fx_source)) BETWEEN 1 AND 200 AND return_fx_source=btrim(return_fx_source)) OR
      (kind='release' AND return_fx_rate IS NULL AND return_fx_source IS NULL))));

CREATE FUNCTION contract_return_remaining_cash(oid uuid,eid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(e.return_cash_amount,e.amount)-COALESCE((SELECT sum(COALESCE(r.return_cash_amount,r.amount)) FROM contract_finance_events r
        WHERE r.org_id=e.org_id AND r.source_event_id=e.id AND r.kind='release'),0)
    FROM contract_finance_events e WHERE e.org_id=oid AND e.id=eid AND e.kind IN ('cash','refund')
$$;
CREATE FUNCTION contract_finance_return_cash(oid uuid,eid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('currency',pr.currency,'amount',e.return_cash_amount::text,'whole_amount',pr.amount::text,'paid_on',pr.paid_on::text,
        'original_amount',root.amount::text,'original_currency',m.currency,'rate',root.return_fx_rate::text,'source',root.return_fx_source,
        'remaining_amount',CASE WHEN e.kind='refund' THEN contract_return_remaining_cash(oid,eid)::text ELSE NULL END)
    FROM contract_finance_events e JOIN contract_finance_events root ON root.org_id=e.org_id AND root.id=CASE WHEN e.kind='release' THEN e.source_event_id ELSE e.id END
    JOIN payment_revisions pr ON pr.org_id=root.org_id AND pr.id=root.source_revision_id
    JOIN contract_milestones m ON m.org_id=e.org_id AND m.id=e.milestone_id
    WHERE e.org_id=oid AND e.id=eid AND e.return_cash_amount IS NOT NULL
$$;

CREATE OR REPLACE FUNCTION prepare_contract_finance_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts;m contract_milestones;r contract_revisions;s sessions;u users;src contract_finance_events;pc actual_cost_entries;cr cost_revisions;p payment_records;pr payment_revisions;original payment_records;op payment_revisions;exp integer;cash_exp integer;next_seq bigint;expected_cash numeric;BEGIN
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
        IF p.state<>'recorded' OR p.record_kind<>'payment' OR p.version<>NEW.source_version OR pr.id<>NEW.source_revision_id OR pr.content_hash<>NEW.source_content_hash OR (pr.currency<>m.currency AND NOT ((NEW.kind='refund' AND m.direction='payable'
                AND jsonb_typeof(pr.payload->'refund_fx')='object' AND pr.payload->'refund_fx'->>'original_currency'=m.currency) OR (NEW.kind='refund' AND m.direction='receivable' AND NEW.return_cash_amount IS NOT NULL)))
            OR p.project_id IS DISTINCT FROM c.project_id OR classification_rank(p.classification)<classification_rank(c.classification) OR pr.counterparty_id IS DISTINCT FROM r.counterparty_id OR pr.employee_id IS NOT NULL THEN
            RAISE EXCEPTION 'cash attribution must pin recorded matching cash' USING ERRCODE='23514';END IF;
        IF NEW.refund_fx_acknowledged IS DISTINCT FROM (NEW.kind='refund' AND pr.currency<>m.currency) THEN
            RAISE EXCEPTION 'foreign refund attribution requires explicit principal acknowledgement' USING ERRCODE='23514';END IF;
        IF NEW.return_cash_amount IS NOT NULL OR (NEW.kind='refund' AND m.direction='receivable' AND pr.currency<>m.currency) THEN
            SELECT exponent INTO STRICT cash_exp FROM currency_definitions WHERE code=pr.currency;
            IF NEW.kind<>'refund' OR m.direction<>'receivable' OR pr.currency=m.currency OR NEW.return_cash_amount IS NULL OR NEW.return_cash_amount<=0
                OR NEW.return_cash_amount<>trunc(NEW.return_cash_amount,cash_exp) OR NEW.return_fx_rate IS NULL OR NEW.return_fx_source IS NULL
                OR round(NEW.amount*NEW.return_fx_rate,cash_exp)<>NEW.return_cash_amount THEN
                RAISE EXCEPTION 'foreign receivable return requires exact reviewed dual units' USING ERRCODE='23514';END IF;
        END IF;
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
        IF src.return_cash_amount IS NOT NULL THEN
            SELECT d.exponent INTO STRICT cash_exp FROM payment_revisions q JOIN currency_definitions d ON d.code=q.currency WHERE q.org_id=src.org_id AND q.id=src.source_revision_id;
            IF contract_finance_remaining(NEW.org_id,src.id)<=0 OR NEW.amount>contract_finance_remaining(NEW.org_id,src.id) THEN
                RAISE EXCEPTION 'release exceeds original dual-unit attribution' USING ERRCODE='23514';END IF;
            expected_cash:=round(NEW.amount*contract_return_remaining_cash(NEW.org_id,src.id)/contract_finance_remaining(NEW.org_id,src.id),cash_exp);
            IF NEW.return_cash_amount IS DISTINCT FROM expected_cash OR NEW.return_fx_rate IS NOT NULL OR NEW.return_fx_source IS NOT NULL THEN
                RAISE EXCEPTION 'release must conserve the exact remaining cash and principal' USING ERRCODE='23514';END IF;
        ELSIF NEW.return_cash_amount IS NOT NULL THEN
            RAISE EXCEPTION 'ordinary release cannot introduce foreign return cash' USING ERRCODE='23514';END IF;
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
    IF NEW.refund_fx_acknowledged AND NEW.kind<>'refund' THEN
        RAISE EXCEPTION 'principal acknowledgement only belongs to a foreign refund attribution' USING ERRCODE='23514';END IF;
    IF NEW.kind NOT IN ('refund','release') AND NEW.return_cash_amount IS NOT NULL THEN
        RAISE EXCEPTION 'foreign return cash only belongs to its refund and releases' USING ERRCODE='23514';END IF;
    RETURN NEW;
END; $$;

CREATE OR REPLACE FUNCTION contract_cash_return_reserved(oid uuid,pid uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(contract_return_remaining_cash(oid,e.id)),0) FROM contract_finance_events e
    JOIN contract_finance_events src ON src.org_id=e.org_id AND src.id=e.source_event_id
    JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id
    JOIN payment_records original ON original.org_id=src.org_id AND original.id=src.payment_id
    JOIN payment_revisions r ON r.org_id=original.org_id AND r.id=original.current_revision_id
    WHERE e.org_id=oid AND e.payment_id=pid AND e.kind='refund' AND p.state='recorded' AND original.state='recorded' AND r.direction='incoming'
$$;
-- Used for dated cash reports, including the original and its explicit reversal.
CREATE OR REPLACE FUNCTION contract_cash_return_share(oid uuid,pid uuid,scope uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(contract_return_remaining_cash(oid,e.id)),0) FROM contract_finance_events e
    JOIN contract_finance_events src ON src.org_id=e.org_id AND src.id=e.source_event_id
    JOIN payment_records original ON original.org_id=src.org_id AND original.id=src.payment_id
    JOIN payment_revisions r ON r.org_id=original.org_id AND r.id=original.current_revision_id
    JOIN contracts c ON c.org_id=e.org_id AND c.id=e.contract_id
    WHERE e.org_id=oid AND e.payment_id=pid AND e.kind='refund' AND r.direction='incoming' AND c.project_id IS NOT DISTINCT FROM scope
$$;
CREATE OR REPLACE FUNCTION contract_finance_cash_guard(oid uuid,pid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p payment_records;r payment_revisions;used numeric;BEGIN
    SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
    SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
    SELECT COALESCE(sum(CASE WHEN e.return_cash_amount IS NOT NULL THEN contract_return_remaining_cash(oid,e.id) ELSE contract_finance_remaining(oid,e.id) END),0) INTO used FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=pid AND e.kind IN ('cash','refund');
    IF used>COALESCE(r.refund_original_amount,0) THEN RAISE EXCEPTION 'cash attribution exceeds the source amount' USING ERRCODE='23514';END IF;
    IF p.state='reversed' AND EXISTS(SELECT 1 FROM contract_finance_events src JOIN contract_finance_events e ON e.org_id=src.org_id AND e.source_event_id=src.id AND e.kind='refund'
        JOIN payment_records q ON q.org_id=e.org_id AND q.id=e.payment_id WHERE src.org_id=oid AND src.payment_id=pid AND src.kind='cash' AND q.state='recorded' AND contract_finance_remaining(oid,e.id)>0) THEN
        RAISE EXCEPTION 'reverse or detach actual returns before reversing the original receipt' USING ERRCODE='23514';END IF;
    PERFORM validate_payment_balance(oid,pid);
END; $$;

ALTER FUNCTION contract_finance_snapshot(uuid,uuid) RENAME TO contract_finance_snapshot_0101_base;
CREATE FUNCTION contract_finance_snapshot(oid uuid,mid uuid) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$
DECLARE b jsonb;amended jsonb;BEGIN
    b:=contract_finance_snapshot_0101_base(oid,mid);
    IF NOT EXISTS(SELECT 1 FROM contract_finance_events WHERE org_id=oid AND milestone_id=mid AND kind='refund' AND return_cash_amount IS NOT NULL) THEN RETURN b;END IF;
    SELECT jsonb_agg(CASE WHEN e.return_cash_amount IS NOT NULL THEN v||jsonb_build_object('attribution_currency',m.currency,'return_cash',contract_finance_return_cash(oid,e.id)) ELSE v END ORDER BY ord)
        INTO amended FROM jsonb_array_elements(b->'sources') WITH ORDINALITY x(v,ord)
        JOIN contract_finance_events e ON e.org_id=oid AND e.id=(v->>'event_id')::uuid
        JOIN contract_milestones m ON m.org_id=e.org_id AND m.id=e.milestone_id;
    RETURN jsonb_set(b,'{sources}',COALESCE(amended,'[]'::jsonb));
END; $$;
