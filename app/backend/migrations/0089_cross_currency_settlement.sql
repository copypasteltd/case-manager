-- Preserve every original cash/cost fact and add the second, explicit unit to
-- the immutable settlement ledger. Legacy same-currency rows keep exact values.
ALTER TABLE payment_allocations ADD COLUMN cost_amount numeric(20,6);
ALTER TABLE payment_allocations ADD COLUMN fx_rate numeric(24,10);
ALTER TABLE payment_allocations ADD COLUMN fx_source text;
ALTER TABLE payment_allocations ADD COLUMN fx_quoted_on date;
ALTER TABLE payment_allocation_shares ADD COLUMN cost_amount numeric(20,6);
ALTER TABLE payment_allocations DISABLE TRIGGER immutable_payment_allocations;
ALTER TABLE payment_allocation_shares DISABLE TRIGGER immutable_payment_allocation_shares;
UPDATE payment_allocations SET cost_amount=amount;
UPDATE payment_allocation_shares SET cost_amount=amount;
ALTER TABLE payment_allocations ENABLE TRIGGER immutable_payment_allocations;
ALTER TABLE payment_allocation_shares ENABLE TRIGGER immutable_payment_allocation_shares;
ALTER TABLE payment_allocations ALTER COLUMN cost_amount SET NOT NULL;
ALTER TABLE payment_allocations ADD CHECK(cost_amount>=0 AND (kind<>'allocation' OR cost_amount>0));
ALTER TABLE payment_allocations ADD CHECK((fx_rate IS NULL AND fx_source IS NULL AND fx_quoted_on IS NULL)
 OR (fx_rate IS NOT NULL AND fx_source IS NOT NULL AND fx_quoted_on IS NOT NULL
   AND fx_rate>0 AND length(btrim(fx_source)) BETWEEN 1 AND 200));
ALTER TABLE payment_allocation_shares ALTER COLUMN cost_amount SET NOT NULL;
ALTER TABLE payment_allocation_shares DROP CONSTRAINT payment_allocation_shares_amount_check;
ALTER TABLE payment_allocation_shares ADD CHECK(amount>=0 AND cost_amount>=0 AND (amount>0 OR cost_amount>0));

CREATE FUNCTION cost_has_effective_settlement(oid uuid,cid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM payment_allocations WHERE org_id=oid AND cost_id=cid GROUP BY root_id
   HAVING sum(effect*amount)<>0 OR sum(effect*cost_amount)<>0)
$$;

CREATE FUNCTION prepare_payment_fx_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pc text; cc text; ce int; pe int; src payment_allocations; available numeric; cost_available numeric; expected numeric; numerator numeric; denominator numeric;
BEGIN
 SELECT r.currency INTO STRICT pc FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
   WHERE p.org_id=NEW.org_id AND p.id=NEW.payment_id;
 SELECT currency INTO STRICT cc FROM cost_revisions WHERE org_id=NEW.org_id AND id=NEW.cost_revision_id;
 SELECT exponent INTO STRICT ce FROM currency_definitions WHERE code=cc;
 SELECT exponent INTO STRICT pe FROM currency_definitions WHERE code=pc;
 IF NEW.cost_amount IS NULL THEN
   IF pc<>cc THEN RAISE EXCEPTION 'cross currency requires explicit cost units' USING ERRCODE='23514'; END IF;
   NEW.cost_amount:=NEW.amount;
 END IF;
 IF NEW.kind IN ('release','refund') THEN
   SELECT * INTO STRICT src FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.source_allocation_id;
   SELECT COALESCE(sum(effect*amount),0),COALESCE(sum(effect*cost_amount),0) INTO available,cost_available
     FROM payment_allocations WHERE org_id=NEW.org_id AND root_id=src.root_id;
   IF available<=0 OR NEW.amount>available THEN
     RAISE EXCEPTION 'inverse cash balance exhausted' USING ERRCODE='23514';
   END IF;
   numerator:=NEW.amount*cost_available*power(10::numeric,pe+ce);
   denominator:=available*power(10::numeric,pe);
   expected:=CASE WHEN pc=cc THEN NEW.amount ELSE
     (div(numerator,denominator)+CASE WHEN mod(numerator,denominator)*2>=denominator THEN 1 ELSE 0 END)/power(10::numeric,ce) END;
   IF NEW.cost_amount<>expected THEN RAISE EXCEPTION 'inverse must use exact remaining dual units' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
-- PostgreSQL orders same-kind triggers by name: the existing cost/payment lock
-- must precede the current-root calculation, including direct SQL insertions.
CREATE TRIGGER payment_allocation_z_fx_basis BEFORE INSERT ON payment_allocations
 FOR EACH ROW EXECUTE FUNCTION prepare_payment_fx_allocation();

CREATE FUNCTION prepare_payment_fx_share() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a payment_allocations;
BEGIN
 IF NEW.cost_amount IS NULL THEN
   SELECT * INTO STRICT a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id;
   IF a.fx_rate IS NOT NULL THEN RAISE EXCEPTION 'cross currency shares require both units' USING ERRCODE='23514'; END IF;
   NEW.cost_amount:=NEW.amount;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER payment_share_fx_basis BEFORE INSERT ON payment_allocation_shares
 FOR EACH ROW EXECUTE FUNCTION prepare_payment_fx_share();

CREATE FUNCTION validate_payment_fx() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a payment_allocations; src payment_allocations; pc text; cc text; pe int; ce int; day date; net numeric;
BEGIN
 IF TG_TABLE_NAME='payment_allocations' THEN a:=NEW;
 ELSE SELECT * INTO STRICT a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id; END IF;
 SELECT r.currency,r.paid_on INTO STRICT pc,day FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
   WHERE p.org_id=a.org_id AND p.id=a.payment_id;
 SELECT currency INTO STRICT cc FROM cost_revisions WHERE org_id=a.org_id AND id=a.cost_revision_id;
 SELECT exponent INTO STRICT pe FROM currency_definitions WHERE code=pc;
 SELECT exponent INTO STRICT ce FROM currency_definitions WHERE code=cc;
 IF a.cost_amount*power(10::numeric,ce)<>trunc(a.cost_amount*power(10::numeric,ce))
   OR (SELECT COALESCE(sum(cost_amount),0) FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id)<>a.cost_amount
   OR EXISTS(SELECT 1 FROM payment_allocation_shares s WHERE s.org_id=a.org_id AND s.allocation_id=a.id
       AND s.cost_amount*power(10::numeric,ce)<>trunc(s.cost_amount*power(10::numeric,ce))) THEN
   RAISE EXCEPTION 'cost unit precision or shares do not balance' USING ERRCODE='23514';
 END IF;
 IF pc=cc THEN
   IF a.fx_rate IS NOT NULL OR a.cost_amount<>a.amount
     OR EXISTS(SELECT 1 FROM payment_allocation_shares s WHERE s.org_id=a.org_id AND s.allocation_id=a.id AND s.amount<>s.cost_amount) THEN
     RAISE EXCEPTION 'same currency keeps identical units and has no FX quote' USING ERRCODE='23514'; END IF;
 ELSE
   IF a.fx_rate IS NULL THEN RAISE EXCEPTION 'cross currency quote is required' USING ERRCODE='23514'; END IF;
   IF a.kind='allocation' AND (a.fx_quoted_on<>day OR round(a.cost_amount*a.fx_rate,pe)<>a.amount) THEN
     RAISE EXCEPTION 'cross currency root must pin actual day and exact rounded quote' USING ERRCODE='23514'; END IF;
 END IF;
 IF a.kind<>'allocation' THEN
   SELECT * INTO STRICT src FROM payment_allocations WHERE org_id=a.org_id AND id=a.source_allocation_id;
   IF ROW(a.fx_rate,a.fx_source,a.fx_quoted_on) IS DISTINCT FROM ROW(src.fx_rate,src.fx_source,src.fx_quoted_on)
     OR (a.kind='reversal' AND (a.cost_amount<>src.cost_amount OR EXISTS(
       SELECT position,project_id,cost_amount FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id
       EXCEPT SELECT position,project_id,cost_amount FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=src.id))) THEN
     RAISE EXCEPTION 'inverse quote and reversal cost shares preserve exact source' USING ERRCODE='23514'; END IF;
 END IF;
 SELECT COALESCE(sum(effect*cost_amount),0) INTO net FROM payment_allocations WHERE org_id=a.org_id AND root_id=a.root_id;
 IF net<0 OR net>(SELECT cost_amount FROM payment_allocations WHERE org_id=a.org_id AND id=a.root_id) THEN
   RAISE EXCEPTION 'root cost balance exhausted' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM (SELECT s.project_id,sum(q.effect*s.cost_amount) remaining
   FROM payment_allocations q JOIN payment_allocation_shares s ON s.org_id=q.org_id AND s.allocation_id=q.id
   WHERE q.org_id=a.org_id AND q.root_id=a.root_id GROUP BY s.project_id) t
   LEFT JOIN payment_allocation_shares o ON o.org_id=a.org_id AND o.allocation_id=a.root_id AND o.project_id IS NOT DISTINCT FROM t.project_id
   WHERE t.remaining<0 OR t.remaining>COALESCE(o.cost_amount,0)) THEN
   RAISE EXCEPTION 'root cost project balance exhausted' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER validate_payment_fx_allocation AFTER INSERT ON payment_allocations
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_payment_fx();
CREATE CONSTRAINT TRIGGER validate_payment_fx_share AFTER INSERT ON payment_allocation_shares
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_payment_fx();

-- Existing payment cash-family rules remain in payment units. Expense, claim
-- and closeout projections below use the separately conserved cost units.

CREATE OR REPLACE FUNCTION validate_cost_settlement(p_org uuid,p_cost uuid) RETURNS void LANGUAGE plpgsql AS $$ DECLARE c actual_cost_entries;r cost_revisions;paid numeric;BEGIN
    SELECT * INTO STRICT c FROM actual_cost_entries WHERE org_id=p_org AND id=p_cost;
    SELECT * INTO STRICT r FROM cost_revisions WHERE org_id=p_org AND id=c.current_revision_id;
    SELECT COALESCE(sum(effect*cost_amount),0) INTO paid FROM payment_allocations WHERE org_id=p_org AND cost_id=p_cost;
    IF paid<0 OR paid>COALESCE(r.amount,0) OR (cost_has_effective_settlement(p_org,p_cost) AND (c.state<>'confirmed' OR c.entry_kind<>'expense')) THEN
        RAISE EXCEPTION 'cost effective settlement cannot exceed its confirmed amount or survive cost reversal' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM (SELECT s.project_id,sum(a.effect*s.cost_amount) paid FROM payment_allocations a JOIN payment_allocation_shares s ON s.org_id=a.org_id AND s.allocation_id=a.id WHERE a.org_id=p_org AND a.cost_id=p_cost GROUP BY s.project_id) totals
        LEFT JOIN cost_allocations x ON x.org_id=p_org AND x.revision_id=r.id AND x.project_id IS NOT DISTINCT FROM totals.project_id WHERE totals.paid<0 OR totals.paid>COALESCE(x.amount,0)) THEN
        RAISE EXCEPTION 'cost project settlement share exceeds its confirmed allocation' USING ERRCODE='23514';
    END IF;
END; $$;

CREATE OR REPLACE FUNCTION validate_payment() RETURNS trigger LANGUAGE plpgsql AS $$
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
        IF a.cost_revision_id IS DISTINCT FROM (SELECT current_revision_id FROM actual_cost_entries WHERE org_id=oid AND id=a.cost_id)
            OR p.state='draft' OR a.amount*power(10::numeric,exp)<>trunc(a.amount*power(10::numeric,exp))
            OR EXISTS(SELECT 1 FROM payment_allocation_shares s WHERE s.org_id=oid AND s.allocation_id=a.id AND s.amount*power(10::numeric,exp)<>trunc(s.amount*power(10::numeric,exp)))
            OR classification_rank(p.classification)<(SELECT classification_rank(classification) FROM actual_cost_entries WHERE org_id=oid AND id=a.cost_id)
            OR (SELECT COALESCE(sum(amount),0) FROM payment_allocation_shares WHERE org_id=oid AND allocation_id=a.id)<>a.amount THEN
            RAISE EXCEPTION 'allocation requires fixed cost and balanced payment-currency project shares' USING ERRCODE='23514';END IF;
        IF a.kind='allocation' THEN
            IF p.record_kind<>'payment' OR r.direction<>'outgoing' OR p.state='draft' OR NOT EXISTS(SELECT 1 FROM payment_events x WHERE x.org_id=oid AND x.payment_id=pid AND x.allocation_id=a.id AND x.kind='allocation_added') THEN RAISE EXCEPTION 'allocation requires recorded outgoing and own event' USING ERRCODE='23514';END IF;
            IF r.employee_id IS NOT NULL THEN
                SELECT * INTO claim FROM spend_documents WHERE org_id=oid AND id=a.claim_id AND kind='claim';
                SELECT * INTO claimrev FROM spend_revisions WHERE org_id=oid AND id=a.claim_revision_id;
                SELECT v INTO line FROM jsonb_array_elements(claimrev.payload->'lines') v WHERE v->>'id'=a.claim_line_id::text;
                IF claim.id IS NULL OR claim.creator_id<>r.employee_id OR claim.approved_revision_id IS DISTINCT FROM a.claim_revision_id OR claimrev.currency IS DISTINCT FROM (SELECT currency FROM cost_revisions WHERE org_id=oid AND id=a.cost_revision_id) OR line IS NULL OR line->>'funding'<>'employee_advance'
                    OR NOT EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.cost_id=a.cost_id AND l.source_type='claim' AND l.source_id=a.claim_id AND l.revision_id=a.claim_revision_id)
                    OR (line->'supplier'->>'party_id')::uuid IS DISTINCT FROM (SELECT counterparty_id FROM cost_revisions WHERE org_id=oid AND id=a.cost_revision_id)
                    OR (SELECT COALESCE(sum(effect*cost_amount),0) FROM payment_allocations WHERE org_id=oid AND claim_id=a.claim_id AND claim_revision_id=a.claim_revision_id AND claim_line_id=a.claim_line_id)>(line->>'declared_amount')::numeric THEN
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

CREATE OR REPLACE FUNCTION contract_closure_snapshot(oid uuid,cid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('revision_id',c.active_revision_id,'finance_version',COALESCE(h.version,0),'schedule_mode',r.payment_schedule->>'mode',
        'nodes',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'title',m.title,'direction',m.direction,'currency',m.currency,'amount',m.amount::text,
            'cash_amount',COALESCE((SELECT sum(contract_finance_effective(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind IN ('cash','refund')),0)::text,
            'has_unattributed_refunds',EXISTS(SELECT 1 FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind='cash' AND contract_unattributed_refunds(oid,e.payment_id)>0),
            'review_id',v.id,'review_current',COALESCE(v.review_fingerprint=md5(contract_finance_snapshot(oid,m.id)::text),false),
            'conditions_satisfied',v.conditions_satisfied,'sources',contract_finance_snapshot(oid,m.id)) ORDER BY m.position)
            FROM contract_milestones m LEFT JOIN LATERAL(SELECT id,review_fingerprint,conditions_satisfied FROM contract_finance_events e WHERE e.org_id=oid AND e.milestone_id=m.id AND e.kind='review' ORDER BY seq DESC LIMIT 1)v ON true
            WHERE m.org_id=oid AND m.contract_id=cid),'[]'::jsonb),
        'costs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'version',d.version,'revision_id',cr.id,'content_hash',cr.content_hash,'state',d.state,
            'currency',cr.currency,'amount',cr.amount::text,'paid_amount',COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id),0)::text,
            'allocation_fingerprint',md5(COALESCE((SELECT string_agg(a.id::text||':'||a.effect::text||':'||a.amount::text||':'||p.version::text||':'||p.state,',' ORDER BY a.id)
                FROM payment_allocations a JOIN payment_records p ON p.org_id=a.org_id AND p.id=a.payment_id WHERE a.org_id=oid AND a.cost_id=d.id),''))) ORDER BY d.id)
            FROM actual_cost_entries d JOIN cost_revisions cr ON cr.org_id=d.org_id AND cr.id=d.current_revision_id
            WHERE d.org_id=oid AND EXISTS(SELECT 1 FROM cost_links l WHERE l.org_id=oid AND l.source_type='contract' AND l.source_id=cid AND l.cost_id=d.id)),'[]'::jsonb),
        'children',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',ch.id,'version',ch.version,'lifecycle',ch.lifecycle) ORDER BY ch.id)
            FROM contracts ch WHERE ch.org_id=oid AND ch.parent_contract_id=cid AND ch.deleted_at IS NULL),'[]'::jsonb))
    FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
    LEFT JOIN contract_finance_heads h ON h.org_id=c.org_id AND h.contract_id=c.id WHERE c.org_id=oid AND c.id=cid
$$;

CREATE OR REPLACE FUNCTION purchase_completion_snapshot(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('revision_id',r.id,'content_hash',r.content_hash,'currency',r.currency,
        'execution_plan_revision_id',plan.revision_id,'execution_amount',plan.amount::text,'lines',purchase_fulfillment_lines(oid,pid),
        'orders',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',o.id,'purchase_version',o.purchase_version,'execution_plan_revision_id',o.execution_plan_revision_id,
            'external_ref',o.external_ref,'supplier_id',o.supplier_id,'supplier_revision_id',o.supplier_revision_id,'ordered_on',o.ordered_on,'amount',o.amount::text,
            'cancelled',EXISTS(SELECT 1 FROM purchase_execution_events e WHERE e.org_id=oid AND e.order_id=o.id AND e.kind='order_cancelled'),
            'lines',(SELECT jsonb_agg(jsonb_build_object('id',l.id,'purchase_line_id',l.purchase_line_id,'description',l.description,'unit',l.unit,'quantity',l.quantity::text,
                'unit_price',l.unit_price::text,'tax_amount',l.tax_amount::text,'discount_amount',l.discount_amount::text,'amount',l.amount::text,
                'net_accepted_quantity',purchase_net_accepted(oid,l.id)::numeric(20,6)::text) ORDER BY l.position) FROM purchase_order_lines l WHERE l.org_id=oid AND l.order_id=o.id)) ORDER BY o.purchase_version)
            FROM purchase_orders o WHERE o.org_id=oid AND o.purchase_id=pid),'[]'::jsonb),
        'receipts',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',q.id,'order_id',q.order_id,'purchase_version',q.purchase_version,'received_on',q.received_on,
            'lines',(SELECT jsonb_agg(jsonb_build_object('id',l.id,'order_line_id',l.order_line_id,'quantity',l.quantity::text,'result',l.result,'reverses_line_id',l.reverses_line_id,'reason',l.reason) ORDER BY l.position)
                FROM purchase_receipt_lines l WHERE l.org_id=oid AND l.receipt_id=q.id)) ORDER BY q.purchase_version) FROM purchase_receipts q WHERE q.org_id=oid AND q.purchase_id=pid),'[]'::jsonb),
        'variances',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',v.id,'version',v.version,'revision_id',v.current_revision_id,'content_hash',vr.content_hash,
            'title',vr.payload->>'title','approval_state',v.approval_state,'lifecycle',v.lifecycle,'basis_purchase_version',vr.basis->'purchase_version') ORDER BY v.id)
            FROM purchase_variances v JOIN purchase_variance_revisions vr ON vr.org_id=v.org_id AND vr.id=v.current_revision_id WHERE v.org_id=oid AND v.purchase_id=pid),'[]'::jsonb),
        'costs',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'version',d.version,'revision_id',cr.id,'content_hash',cr.content_hash,'title',cr.title,'entry_kind',d.entry_kind,
            'state',d.state,'currency',cr.currency,'amount',cr.amount::text,'paid_amount',COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id),0)::text)
            ORDER BY d.id) FROM actual_cost_entries d JOIN cost_revisions cr ON cr.org_id=d.org_id AND cr.id=d.current_revision_id WHERE d.org_id=oid AND d.id IN(SELECT id FROM purchase_completion_cost_ids(oid,pid))),'[]'::jsonb),
        'payments',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',p.id,'version',p.version,'revision_id',pr.id,'content_hash',pr.content_hash,'state',p.state,'record_kind',p.record_kind,
            'direction',pr.direction,'amount',pr.amount::text,'currency',pr.currency,'refund_of_id',pr.refund_of_id,
            'unattributed_refund_amount',CASE WHEN p.record_kind='payment' AND p.state='recorded' AND pr.direction='refund' THEN greatest(0,pr.amount+
                COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),0)-
                COALESCE((SELECT sum(contract_finance_remaining(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id AND e.kind='refund'),0))::text ELSE '0' END,
            'allocations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',a.id,'cost_id',a.cost_id,'cost_revision_id',a.cost_revision_id,'kind',a.kind,'effect',a.effect,'amount',a.amount::text) ORDER BY a.id)
                FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),'[]'::jsonb)) ORDER BY p.id)
            FROM payment_records p JOIN payment_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM purchase_completion_cash_ids(oid,pid))),'[]'::jsonb))
    FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.approved_revision_id
    CROSS JOIN LATERAL purchase_effective_plan(oid,pid) plan WHERE d.org_id=oid AND d.id=pid AND d.kind='purchase'
$$;

CREATE OR REPLACE FUNCTION project_closeout_items(oid uuid,pid uuid) RETURNS TABLE(kind text,id uuid,version bigint,title text,state text,blocking boolean,facts jsonb) LANGUAGE sql STABLE AS $$
 SELECT 'tasks',t.id,t.version,t.title,t.state,t.state NOT IN('done','cancelled'),to_jsonb(t) FROM tasks t WHERE t.org_id=oid AND t.project_id=pid
 UNION ALL SELECT 'milestones',m.id,m.version,m.title,m.state,m.state NOT IN('achieved','cancelled'),to_jsonb(m) FROM milestones m WHERE m.org_id=oid AND m.project_id=pid
 UNION ALL SELECT 'deliverables',d.id,d.version,d.name,d.state,d.state<>'accepted' OR d.protocol_version<>1,
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'decision',to_jsonb(a)) FROM deliverables d
  LEFT JOIN deliverable_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id LEFT JOIN acceptance_decisions a ON a.org_id=r.org_id AND a.revision_id=r.id WHERE d.org_id=oid AND d.project_id=pid
 UNION ALL SELECT 'risks',r.id,r.version,r.title,r.state,r.state NOT IN('closed','realized'),to_jsonb(r) FROM risks r WHERE r.org_id=oid AND r.project_id=pid
 UNION ALL SELECT 'issues',i.id,i.version,i.title,i.state,i.state<>'closed',to_jsonb(i) FROM issues i WHERE i.org_id=oid AND i.project_id=pid
 UNION ALL SELECT 'changes',c.id,c.version,c.title,c.state,c.state NOT IN('implemented','rejected','withdrawn') OR c.protocol_version<>1,
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r)) FROM change_requests c LEFT JOIN project_change_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.project_id=pid
 UNION ALL SELECT 'budgets',b.id,b.revision,b.currency,b.state,false,jsonb_build_object('head',to_jsonb(b),'lines',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM budget_lines l WHERE l.org_id=oid AND l.budget_id=b.id),'[]'::jsonb)) FROM project_budgets b WHERE b.org_id=oid AND b.project_id=pid
 UNION ALL SELECT 'contracts',c.id,c.version,r.title,c.lifecycle,
  c.lifecycle NOT IN('completed','terminated') OR c.approval_state='in_review' OR c.active_revision_id IS NULL OR COALESCE(contract_closure_has_unsettled(contract_closure_snapshot(oid,c.id)),true),
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r),'financial',contract_closure_snapshot(oid,c.id),'closure',(SELECT to_jsonb(h) FROM contract_closures h WHERE h.org_id=oid AND h.contract_id=c.id))
  FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.project_id=pid AND c.deleted_at IS NULL
 UNION ALL SELECT 'purchases',d.id,d.version,r.title,d.lifecycle,
  d.approval_state NOT IN('approved','rejected','withdrawn') OR d.approval_state='approved' AND (d.lifecycle NOT IN('completed','cancelled') OR COALESCE(cardinality(CASE WHEN d.lifecycle='cancelled' THEN array_remove(purchase_completion_blockers(purchase_completion_snapshot(oid,d.id)),'no_accepted_delivery') ELSE purchase_completion_blockers(purchase_completion_snapshot(oid,d.id)) END)>0,true)),
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'fulfillment',purchase_completion_snapshot(oid,d.id),'completion',(SELECT to_jsonb(h) FROM purchase_completions h WHERE h.org_id=oid AND h.purchase_id=d.id))
  FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id WHERE d.org_id=oid AND d.project_id=pid AND d.kind='purchase' AND d.deleted_at IS NULL
 UNION ALL SELECT 'claims',d.id,d.version,r.title,d.approval_state,
  d.approval_state NOT IN('approved','rejected','withdrawn') OR d.approval_state='approved' AND (
   COALESCE((SELECT sum((l->>'declared_amount')::numeric) FROM jsonb_array_elements(ar.payload->'lines')l WHERE l->>'funding'='employee_advance'),0)
    <>COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id AND a.claim_revision_id=d.approved_revision_id),0)
   OR COALESCE((SELECT sum(cr.amount) FROM cost_links l JOIN actual_cost_entries c ON c.org_id=l.org_id AND c.id=l.cost_id JOIN cost_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
    WHERE l.org_id=oid AND l.source_type='claim' AND l.source_id=d.id AND l.revision_id=d.approved_revision_id AND c.entry_kind='expense' AND c.state='confirmed' AND cr.currency=ar.currency),0) IS DISTINCT FROM ar.amount),
  jsonb_build_object('head',to_jsonb(d),'revision',to_jsonb(r),'approved_revision',to_jsonb(ar),'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.claim_id=d.id),'[]'::jsonb))
  FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id LEFT JOIN spend_revisions ar ON ar.org_id=d.org_id AND ar.id=d.approved_revision_id WHERE d.org_id=oid AND d.project_id=pid AND d.kind='claim' AND d.deleted_at IS NULL
 UNION ALL SELECT 'costs',c.id,c.version,r.title,c.state,c.state='draft' OR c.entry_kind='expense' AND
  COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)<>CASE WHEN c.state='confirmed' THEN r.amount ELSE 0 END,
  jsonb_build_object('head',to_jsonb(c),'revision',to_jsonb(r),'paid_amount',COALESCE((SELECT sum(a.effect*a.cost_amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),0)::text,
   'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=c.id),'[]'::jsonb),
   'links',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM cost_links l WHERE l.org_id=oid AND l.cost_id=c.id),'[]'::jsonb))
  FROM actual_cost_entries c JOIN cost_revisions r ON r.org_id=c.org_id AND r.id=c.current_revision_id WHERE c.org_id=oid AND c.id IN(SELECT id FROM project_closeout_cost_ids(oid,pid))
 UNION ALL SELECT 'payments',p.id,p.version,r.title,p.state,p.state='draft' OR p.state='recorded' AND p.record_kind='payment' AND r.direction='refund' AND
  r.amount+COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),0)>
  COALESCE((SELECT sum(contract_finance_remaining(oid,e.id)) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id AND e.kind='refund'),0),
  jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(r),'allocations',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM payment_allocations a WHERE a.org_id=oid AND a.payment_id=p.id),'[]'::jsonb),
   'contract_events',COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM contract_finance_events e WHERE e.org_id=oid AND e.payment_id=p.id),'[]'::jsonb))
  FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM project_closeout_cash_ids(oid,pid))
 UNION ALL SELECT 'subscriptions',s.id,s.version,r.product,s.state,s.state NOT IN('cancelled','archived') OR s.cancel_effective_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR
  r.payload->>'vendor_state' IS DISTINCT FROM 'cancelled' OR r.payload->>'vendor_checked_at' IS NULL OR COALESCE(jsonb_array_length(r.payload->'vendor_evidence_version_ids'),0)=0 OR
  EXISTS(SELECT 1 FROM subscription_periods p WHERE p.org_id=oid AND p.subscription_id=s.id AND (p.state='forecast' OR p.state='confirmed' AND p.review_fingerprint IS DISTINCT FROM subscription_actual_fingerprint(oid,p.id))),
  jsonb_build_object('head',to_jsonb(s),'revision',to_jsonb(r),'periods',COALESCE((SELECT jsonb_agg(jsonb_build_object('head',to_jsonb(p),'revision',to_jsonb(pr),'actual_fingerprint',subscription_actual_fingerprint(oid,p.id)) ORDER BY p.id)
   FROM subscription_periods p JOIN subscription_period_revisions pr ON pr.org_id=p.org_id AND pr.id=p.current_revision_id WHERE p.org_id=oid AND p.subscription_id=s.id),'[]'::jsonb))
  FROM subscriptions s JOIN subscription_revisions r ON r.org_id=s.org_id AND r.id=s.current_revision_id WHERE s.org_id=oid AND s.project_id=pid
$$;
