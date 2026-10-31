-- Conserve three independent units. Existing journal payloads, amount/cost
-- columns and project shares remain unchanged; NULL cash is the legacy format.
ALTER TABLE payment_allocations ADD COLUMN refund_cash_amount numeric(20,6);
ALTER TABLE payment_allocations ADD COLUMN refund_fx_acknowledged boolean NOT NULL DEFAULT false;
ALTER TABLE payment_allocations ADD CHECK((refund_cash_amount IS NULL AND NOT refund_fx_acknowledged)
 OR (refund_cash_amount IS NOT NULL AND refund_cash_amount>=0 AND refund_fx_acknowledged AND kind IN ('refund','reversal')));
ALTER TABLE payment_allocation_shares ADD COLUMN refund_cash_amount numeric(20,6) CHECK(refund_cash_amount>=0);

CREATE FUNCTION payment_allocation_currency(oid uuid,rid uuid,pid uuid) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT r.currency FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
 WHERE p.org_id=oid AND p.id=COALESCE((SELECT payment_id FROM payment_allocations WHERE org_id=oid AND id=rid),pid)
$$;

CREATE OR REPLACE FUNCTION prepare_payment_fx_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pc text; cc text; ce int; pe int; src payment_allocations; available numeric; cost_available numeric; expected numeric; numerator numeric; denominator numeric;
BEGIN
 SELECT payment_allocation_currency(NEW.org_id,NEW.root_id,NEW.payment_id) INTO STRICT pc FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
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

CREATE OR REPLACE FUNCTION validate_payment_fx() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a payment_allocations; src payment_allocations; pc text; cc text; pe int; ce int; day date; net numeric;
BEGIN
 IF TG_TABLE_NAME='payment_allocations' THEN a:=NEW;
 ELSE SELECT * INTO STRICT a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id; END IF;
 SELECT payment_allocation_currency(a.org_id,a.root_id,a.payment_id),r.paid_on INTO STRICT pc,day FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
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
        IF original.record_kind<>'payment' OR original.state NOT IN ('recorded','reversed') OR oldr.direction<>'outgoing' OR r.paid_at<oldr.paid_at
            OR ROW(r.counterparty_id,r.counterparty_revision_id,r.employee_id,p.project_id,p.classification) IS DISTINCT FROM ROW(oldr.counterparty_id,oldr.counterparty_revision_id,oldr.employee_id,original.project_id,original.classification) THEN
            RAISE EXCEPTION 'refund must pin the original outgoing cash identity and scope' USING ERRCODE='23514';
        END IF;
    END IF;
    PERFORM validate_refund_cash(oid,pid);
    PERFORM validate_payment_balance(oid,pid);
    IF r.refund_of_id IS NOT NULL THEN PERFORM validate_payment_balance(oid,r.refund_of_id);END IF;
    IF p.reverses_id IS NOT NULL THEN PERFORM validate_payment_balance(oid,p.reverses_id);END IF;
    SELECT COALESCE(sum(effect*amount),0) INTO allocated FROM payment_allocations WHERE org_id=oid AND payment_id=pid;
    IF (r.direction='refund' AND (allocated>0 OR -allocated>r.refund_original_amount) AND p.record_kind='payment') OR (r.direction NOT IN ('outgoing','refund') AND allocated<>0) THEN RAISE EXCEPTION 'allocation direction differs from actual cash fact' USING ERRCODE='23514';END IF;
    FOR a IN SELECT * FROM payment_allocations WHERE org_id=oid AND payment_id=pid LOOP
        PERFORM validate_cost_settlement(oid,a.cost_id);
        SELECT exponent INTO exp FROM currency_definitions WHERE code=payment_allocation_currency(oid,a.root_id,pid);
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

CREATE OR REPLACE FUNCTION validate_refund_cash(oid uuid,pid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p payment_records; r payment_revisions; original_currency text;
BEGIN
 SELECT * INTO STRICT p FROM payment_records WHERE org_id=oid AND id=pid;
 SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=oid AND id=p.current_revision_id;
 IF p.state='draft' OR r.direction IS DISTINCT FROM 'refund' THEN RETURN; END IF;
 SELECT v.currency INTO STRICT original_currency FROM payment_records x JOIN payment_revisions v ON v.org_id=x.org_id AND v.id=x.current_revision_id WHERE x.org_id=oid AND x.id=r.refund_of_id;
 IF r.currency<>original_currency THEN
  IF r.payload->'refund_fx' IS NULL OR r.payload->'refund_fx'='null'::jsonb
   OR r.payload->'refund_fx'->>'original_currency' IS DISTINCT FROM original_currency
   OR (p.record_kind='payment' AND NOT EXISTS(SELECT 1 FROM payment_events WHERE org_id=oid AND payment_id=pid AND kind='recorded' AND refund_fx_acknowledged))
   THEN RAISE EXCEPTION 'foreign refund requires human reviewed principal and actual cash evidence' USING ERRCODE='23514'; END IF;
 END IF;
END; $$;

-- The ordinary allocation lock executes first; original-root calculation and
-- foreign-cash calculation share that lock, including direct SQL insertions.
CREATE FUNCTION prepare_foreign_refund_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p payment_records;r payment_revisions;src payment_allocations;used numeric;cash_used numeric;pe int;ce int;n numeric;d numeric;expected numeric;
BEGIN
 SELECT * INTO STRICT p FROM payment_records WHERE org_id=NEW.org_id AND id=NEW.payment_id;
 SELECT * INTO STRICT r FROM payment_revisions WHERE org_id=p.org_id AND id=p.current_revision_id;
 IF r.payload->'refund_fx' IS NULL OR r.payload->'refund_fx'='null'::jsonb THEN
  IF NEW.refund_cash_amount IS NOT NULL OR NEW.refund_fx_acknowledged THEN RAISE EXCEPTION 'legacy allocation cannot carry foreign refund cash' USING ERRCODE='23514'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.refund_cash_amount IS NULL OR NOT NEW.refund_fx_acknowledged OR NEW.kind NOT IN ('refund','reversal')
  OR payment_allocation_currency(NEW.org_id,NEW.root_id,NEW.payment_id) IS DISTINCT FROM r.payload->'refund_fx'->>'original_currency' THEN
  RAISE EXCEPTION 'foreign refund attribution requires exact original principal and explicit acknowledgement' USING ERRCODE='23514'; END IF;
 IF NEW.kind='reversal' THEN
  SELECT * INTO STRICT src FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.source_allocation_id;
  IF NEW.refund_cash_amount IS DISTINCT FROM src.refund_cash_amount OR NEW.refund_fx_acknowledged IS DISTINCT FROM src.refund_fx_acknowledged THEN
   RAISE EXCEPTION 'foreign refund reversal preserves exact cash evidence' USING ERRCODE='23514'; END IF;
 ELSE
  SELECT COALESCE(sum(amount),0),COALESCE(sum(refund_cash_amount),0) INTO used,cash_used FROM payment_allocations WHERE org_id=NEW.org_id AND payment_id=NEW.payment_id AND kind='refund';
  IF NEW.amount<=0 OR NEW.amount>r.refund_original_amount-used OR cash_used>r.amount THEN RAISE EXCEPTION 'foreign refund principal exhausted' USING ERRCODE='23514'; END IF;
  SELECT exponent INTO STRICT pe FROM currency_definitions WHERE code=r.payload->'refund_fx'->>'original_currency';
  SELECT exponent INTO STRICT ce FROM currency_definitions WHERE code=r.currency;
  n:=NEW.amount*(r.amount-cash_used)*power(10::numeric,pe+ce);d:=(r.refund_original_amount-used)*power(10::numeric,pe);
  expected:=(div(n,d)+CASE WHEN mod(n,d)*2>=d THEN 1 ELSE 0 END)/power(10::numeric,ce);
  IF NEW.refund_cash_amount<>expected THEN RAISE EXCEPTION 'foreign refund cash must use exact remaining principal ratio' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
DROP TRIGGER payment_foreign_refund_no_attribution ON payment_allocations;
CREATE TRIGGER payment_allocation_zz_refund_cash_basis BEFORE INSERT ON payment_allocations FOR EACH ROW EXECUTE FUNCTION prepare_foreign_refund_allocation();

CREATE FUNCTION validate_foreign_refund_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a payment_allocations;r payment_revisions;ce int;pe int;
BEGIN
 IF TG_TABLE_NAME='payment_allocations' THEN a:=NEW;ELSE SELECT * INTO STRICT a FROM payment_allocations WHERE org_id=NEW.org_id AND id=NEW.allocation_id;END IF;
 IF a.refund_cash_amount IS NULL THEN
  IF EXISTS(SELECT 1 FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id AND refund_cash_amount IS NOT NULL) THEN RAISE EXCEPTION 'legacy shares cannot carry foreign cash' USING ERRCODE='23514'; END IF;
  RETURN NULL;
 END IF;
 SELECT v.* INTO STRICT r FROM payment_records p JOIN payment_revisions v ON v.org_id=p.org_id AND v.id=p.current_revision_id WHERE p.org_id=a.org_id AND p.id=a.payment_id;
 SELECT exponent INTO STRICT ce FROM currency_definitions WHERE code=r.currency;
 SELECT exponent INTO STRICT pe FROM currency_definitions WHERE code=payment_allocation_currency(a.org_id,a.root_id,a.payment_id);
 IF a.refund_cash_amount*power(10::numeric,ce)<>trunc(a.refund_cash_amount*power(10::numeric,ce))
  OR (SELECT COALESCE(sum(refund_cash_amount),0) FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id)<>a.refund_cash_amount
  OR EXISTS(SELECT 1 FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id AND (refund_cash_amount IS NULL OR refund_cash_amount*power(10::numeric,ce)<>trunc(refund_cash_amount*power(10::numeric,ce)) OR (amount=0 AND refund_cash_amount<>0)))
  OR (SELECT COALESCE(sum(refund_cash_amount),0) FROM payment_allocations WHERE org_id=a.org_id AND payment_id=a.payment_id)>r.amount THEN
  RAISE EXCEPTION 'foreign cash precision, cap or shares differ' USING ERRCODE='23514'; END IF;
 -- Actual cash uses largest remainders of these exact principal shares, with
 -- the immutable position as tie breaker. Zero-cash positions stay explicit.
 IF EXISTS(WITH weights AS (
   SELECT position,refund_cash_amount,
    div(a.refund_cash_amount*power(10::numeric,ce)*amount*power(10::numeric,pe),a.amount*power(10::numeric,pe)) base,
    mod(a.refund_cash_amount*power(10::numeric,ce)*amount*power(10::numeric,pe),a.amount*power(10::numeric,pe)) rem
   FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id AND amount>0),
  ranked AS (SELECT *,row_number() OVER(ORDER BY rem DESC,position) pos,a.refund_cash_amount*power(10::numeric,ce)-sum(base) OVER() extra FROM weights)
  SELECT 1 FROM ranked WHERE refund_cash_amount*power(10::numeric,ce)<>base+CASE WHEN pos<=extra THEN 1 ELSE 0 END) THEN
  RAISE EXCEPTION 'foreign cash project shares must follow original principal weights' USING ERRCODE='23514'; END IF;
 IF a.kind='reversal' AND EXISTS(SELECT position,project_id,refund_cash_amount FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.id
  EXCEPT SELECT position,project_id,refund_cash_amount FROM payment_allocation_shares WHERE org_id=a.org_id AND allocation_id=a.source_allocation_id) THEN
  RAISE EXCEPTION 'foreign refund reversal must copy exact cash shares' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER validate_foreign_refund_allocation AFTER INSERT ON payment_allocations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_foreign_refund_allocation();
CREATE CONSTRAINT TRIGGER validate_foreign_refund_share AFTER INSERT ON payment_allocation_shares DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_foreign_refund_allocation();
