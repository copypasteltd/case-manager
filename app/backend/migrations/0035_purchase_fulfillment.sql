-- Real-world order and acceptance facts append to an approved purchase.
ALTER TABLE spend_documents DROP CONSTRAINT spend_documents_lifecycle_check;
DO $$ DECLARE c record; BEGIN
    FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='spend_documents'::regclass AND contype='c'
        AND position('approval_state' IN pg_get_constraintdef(oid))>0 AND position('lifecycle' IN pg_get_constraintdef(oid))>0 LOOP
        EXECUTE format('ALTER TABLE spend_documents DROP CONSTRAINT %I',c.conname);
    END LOOP;
END; $$;
ALTER TABLE spend_documents ADD CONSTRAINT spend_execution_lifecycle CHECK(
    (kind='claim' AND lifecycle IN ('draft','requested') AND ((approval_state IN ('in_review','approved'))=(lifecycle='requested')))
    OR (kind='purchase' AND ((approval_state='approved' AND lifecycle IN ('requested','ordering','receiving','cancelled'))
        OR (approval_state='in_review' AND lifecycle='requested') OR (approval_state NOT IN ('in_review','approved') AND lifecycle='draft'))));
CREATE OR REPLACE FUNCTION preserve_spend_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.kind,NEW.document_no,NEW.project_id,NEW.creator_id,NEW.classification,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.kind,OLD.document_no,OLD.project_id,OLD.creator_id,OLD.classification,OLD.created_at)
        OR NEW.version<>OLD.version+1 THEN
        RAISE EXCEPTION 'spend identity is immutable and every mutation advances its version' USING ERRCODE='23514';
    END IF;
    IF (OLD.approval_state='approved' AND (OLD.kind<>'purchase' OR
        ROW(NEW.current_revision_id,NEW.approved_revision_id,NEW.request_id,NEW.approval_state,NEW.deleted_at)
        IS DISTINCT FROM ROW(OLD.current_revision_id,OLD.approved_revision_id,OLD.request_id,OLD.approval_state,OLD.deleted_at)))
        OR (OLD.approval_state='in_review' AND NEW.current_revision_id<>OLD.current_revision_id) THEN
        RAISE EXCEPTION 'approved or reviewing commercial content cannot be rewritten' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TABLE purchase_execution_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0),
    purchase_version bigint NOT NULL CHECK(purchase_version>1),
    kind text NOT NULL CHECK(kind IN ('order_registered','receipt_registered','order_cancelled','purchase_cancelled')),
    order_id uuid, receipt_id uuid, lifecycle text NOT NULL CHECK(lifecycle IN ('ordering','receiving','cancelled')),
    actor_id uuid NOT NULL, session_id uuid NOT NULL, mfa_verified_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,purchase_id,seq), UNIQUE(org_id,purchase_id,purchase_version),
    FOREIGN KEY(org_id,purchase_id) REFERENCES spend_documents(org_id,id), FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
    CHECK((kind IN ('order_registered','order_cancelled','receipt_registered'))=(order_id IS NOT NULL)),
    CHECK((kind='receipt_registered')=(receipt_id IS NOT NULL)), CHECK((kind='purchase_cancelled')=(lifecycle='cancelled') OR kind='receipt_registered')
);
CREATE UNIQUE INDEX purchase_order_one_cancellation ON purchase_execution_events(org_id,order_id) WHERE kind='order_cancelled';
CREATE UNIQUE INDEX purchase_one_cancellation ON purchase_execution_events(org_id,purchase_id) WHERE kind='purchase_cancelled';
CREATE TABLE purchase_orders (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, revision_id uuid NOT NULL, purchase_version bigint NOT NULL,
    request_revision_id uuid NOT NULL, supplier_id uuid NOT NULL, supplier_revision_id uuid NOT NULL,
    external_ref text NOT NULL CHECK(length(btrim(external_ref)) BETWEEN 1 AND 200), external_ref_key text NOT NULL CHECK(external_ref_key=lower(btrim(external_ref))),
    ordered_on date NOT NULL CHECK(ordered_on<=(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date),
    expected_delivery_on date NOT NULL CHECK(expected_delivery_on>=ordered_on), currency text NOT NULL REFERENCES currency_definitions(code),
    amount numeric(20,6) NOT NULL CHECK(amount>=0), line_count integer NOT NULL CHECK(line_count BETWEEN 1 AND 100),
    evidence_version_ids uuid[] NOT NULL CHECK(cardinality(evidence_version_ids) BETWEEN 1 AND 20),
    notes text NOT NULL CHECK(length(notes)<=4000), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,purchase_id,id), UNIQUE(org_id,supplier_id,external_ref_key),
    FOREIGN KEY(org_id,purchase_id,revision_id) REFERENCES spend_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,request_revision_id) REFERENCES request_revisions(org_id,id),
    FOREIGN KEY(org_id,supplier_id,supplier_revision_id) REFERENCES party_revisions(org_id,party_id,id),
    FOREIGN KEY(org_id,purchase_id,purchase_version) REFERENCES purchase_execution_events(org_id,purchase_id,purchase_version) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE purchase_order_lines (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, order_id uuid NOT NULL, revision_id uuid NOT NULL,
    position integer NOT NULL CHECK(position BETWEEN 1 AND 100), purchase_line_id uuid NOT NULL,
    description text NOT NULL, unit text NOT NULL, quantity numeric(20,6) NOT NULL CHECK(quantity>0),
    unit_price numeric(20,6) NOT NULL CHECK(unit_price>=0), tax_amount numeric(20,6) NOT NULL CHECK(tax_amount>=0),
    discount_amount numeric(20,6) NOT NULL CHECK(discount_amount>=0), amount numeric(20,6) NOT NULL CHECK(amount>=0),
    UNIQUE(org_id,id), UNIQUE(org_id,order_id,id), UNIQUE(org_id,order_id,position), UNIQUE(org_id,order_id,purchase_line_id),
    FOREIGN KEY(org_id,purchase_id,order_id) REFERENCES purchase_orders(org_id,purchase_id,id),
    FOREIGN KEY(org_id,revision_id,purchase_line_id) REFERENCES spend_lines(org_id,revision_id,line_id)
);
CREATE TABLE purchase_receipts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, order_id uuid NOT NULL, purchase_version bigint NOT NULL,
    received_on date NOT NULL CHECK(received_on<=(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date),
    line_count integer NOT NULL CHECK(line_count BETWEEN 1 AND 100), evidence_version_ids uuid[] NOT NULL CHECK(cardinality(evidence_version_ids) BETWEEN 1 AND 20),
    notes text NOT NULL CHECK(length(notes)<=4000), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,purchase_id,id), UNIQUE(org_id,order_id,id),
    FOREIGN KEY(org_id,purchase_id,order_id) REFERENCES purchase_orders(org_id,purchase_id,id),
    FOREIGN KEY(org_id,purchase_id,purchase_version) REFERENCES purchase_execution_events(org_id,purchase_id,purchase_version) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE purchase_receipt_lines (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, order_id uuid NOT NULL, receipt_id uuid NOT NULL,
    position integer NOT NULL CHECK(position BETWEEN 1 AND 100), order_line_id uuid NOT NULL,
    quantity numeric(20,6) NOT NULL CHECK(quantity>0), result text NOT NULL CHECK(result IN ('accepted','rejected','returned')),
    reverses_line_id uuid, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000),
    UNIQUE(org_id,id), UNIQUE(org_id,order_id,id), UNIQUE(org_id,receipt_id,position),
    UNIQUE NULLS NOT DISTINCT(org_id,receipt_id,order_line_id,result,reverses_line_id),
    FOREIGN KEY(org_id,purchase_id,receipt_id) REFERENCES purchase_receipts(org_id,purchase_id,id),
    FOREIGN KEY(org_id,order_id,receipt_id) REFERENCES purchase_receipts(org_id,order_id,id),
    FOREIGN KEY(org_id,order_id,order_line_id) REFERENCES purchase_order_lines(org_id,order_id,id),
    FOREIGN KEY(org_id,order_id,reverses_line_id) REFERENCES purchase_receipt_lines(org_id,order_id,id),
    CHECK((result='returned')=(reverses_line_id IS NOT NULL)), CHECK(reverses_line_id IS NULL OR reverses_line_id<>id)
);
ALTER TABLE purchase_execution_events ADD FOREIGN KEY(org_id,purchase_id,order_id) REFERENCES purchase_orders(org_id,purchase_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE purchase_execution_events ADD FOREIGN KEY(org_id,purchase_id,receipt_id) REFERENCES purchase_receipts(org_id,purchase_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX purchase_order_history ON purchase_orders(org_id,purchase_id,purchase_version DESC);
CREATE INDEX purchase_receipt_history ON purchase_receipts(org_id,purchase_id,purchase_version DESC);
CREATE INDEX purchase_receipt_order_line ON purchase_receipt_lines(org_id,order_line_id);
CREATE INDEX purchase_return_source ON purchase_receipt_lines(org_id,reverses_line_id) WHERE reverses_line_id IS NOT NULL;
CREATE FUNCTION purchase_net_accepted(org uuid, item uuid) RETURNS numeric LANGUAGE sql STABLE AS $$
    SELECT COALESCE(sum(CASE result WHEN 'accepted' THEN quantity WHEN 'returned' THEN -quantity ELSE 0 END),0)
    FROM purchase_receipt_lines WHERE org_id=org AND order_line_id=item;
$$;
CREATE FUNCTION purchase_commitments(org uuid, purchase uuid) RETURNS TABLE(line_id uuid, quantity numeric, amount numeric, tax numeric, discount numeric) LANGUAGE sql STABLE AS $$
    SELECT l.purchase_line_id,sum(CASE WHEN e.id IS NULL THEN l.quantity ELSE purchase_net_accepted(org,l.id) END),
        sum(CASE WHEN e.id IS NULL THEN l.amount ELSE ceil(l.amount*purchase_net_accepted(org,l.id)/l.quantity*power(10::numeric,c.exponent))/power(10::numeric,c.exponent) END),
        sum(CASE WHEN e.id IS NULL THEN l.tax_amount ELSE ceil(l.tax_amount*purchase_net_accepted(org,l.id)/l.quantity*power(10::numeric,c.exponent))/power(10::numeric,c.exponent) END),
        sum(CASE WHEN e.id IS NULL THEN l.discount_amount ELSE ceil(l.discount_amount*purchase_net_accepted(org,l.id)/l.quantity*power(10::numeric,c.exponent))/power(10::numeric,c.exponent) END)
    FROM purchase_order_lines l JOIN purchase_orders o ON o.org_id=l.org_id AND o.id=l.order_id
    JOIN currency_definitions c ON c.code=o.currency LEFT JOIN LATERAL(SELECT x.id FROM purchase_execution_events x WHERE x.org_id=o.org_id
        AND (x.kind='order_cancelled' AND x.order_id=o.id OR x.kind='purchase_cancelled' AND x.purchase_id=o.purchase_id) LIMIT 1) e ON true
    WHERE l.org_id=org AND l.purchase_id=purchase GROUP BY l.purchase_line_id;
$$;
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','deliverable','recovery_set'));
CREATE FUNCTION pin_purchase_execution_files() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; owner text; file_count integer;
BEGIN
    owner:=CASE TG_TABLE_NAME WHEN 'purchase_orders' THEN 'purchase_order' ELSE 'purchase_receipt' END;
    SELECT count(DISTINCT v) INTO file_count FROM unnest(NEW.evidence_version_ids) v;
    IF file_count<>cardinality(NEW.evidence_version_ids) THEN RAISE EXCEPTION 'execution file versions must be distinct' USING ERRCODE='23514'; END IF;
    FOREACH target IN ARRAY NEW.evidence_version_ids LOOP
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
            SELECT gen_random_uuid(),NEW.org_id,file_id,id,owner,NEW.id FROM file_versions WHERE org_id=NEW.org_id AND id=target;
        IF NOT FOUND THEN RAISE EXCEPTION 'execution requires the exact evidence version' USING ERRCODE='23514'; END IF;
    END LOOP;
    RETURN NEW;
END; $$;
CREATE TRIGGER purchase_order_files AFTER INSERT ON purchase_orders FOR EACH ROW EXECUTE FUNCTION pin_purchase_execution_files();
CREATE TRIGGER purchase_receipt_files AFTER INSERT ON purchase_receipts FOR EACH ROW EXECUTE FUNCTION pin_purchase_execution_files();
CREATE FUNCTION validate_purchase_execution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; d spend_documents; r spend_revisions; e purchase_execution_events; expected text;
BEGIN
    IF TG_TABLE_NAME='spend_documents' THEN target:=NEW.id; ELSE target:=NEW.purchase_id; END IF;
    SELECT * INTO d FROM spend_documents WHERE org_id=NEW.org_id AND id=target FOR UPDATE;
    IF d.kind<>'purchase' THEN RETURN NULL; END IF;
    SELECT * INTO r FROM spend_revisions WHERE org_id=d.org_id AND id=d.current_revision_id;
    SELECT * INTO e FROM purchase_execution_events WHERE org_id=d.org_id AND purchase_id=d.id ORDER BY seq DESC LIMIT 1;
    IF NOT FOUND THEN
        IF d.lifecycle IN ('ordering','receiving','cancelled') THEN
            RAISE EXCEPTION 'purchase head requires its execution fact' USING ERRCODE='23514';
        END IF;
        IF TG_TABLE_NAME='spend_documents' THEN
            IF OLD.approval_state='approved' THEN
                RAISE EXCEPTION 'approved purchase mutation requires an execution fact' USING ERRCODE='23514';
            END IF;
        END IF;
        RETURN NULL;
    END IF;
    IF d.approval_state<>'approved' OR d.approved_revision_id<>r.id OR e.purchase_version<>d.version OR e.lifecycle<>d.lifecycle
        OR (SELECT count(*) FROM purchase_execution_events WHERE org_id=d.org_id AND purchase_id=d.id)<>e.seq THEN
        RAISE EXCEPTION 'purchase execution must match its approved head and complete sequence' USING ERRCODE='23514';
    END IF;
    expected:=CASE WHEN EXISTS(SELECT 1 FROM purchase_execution_events WHERE org_id=d.org_id AND purchase_id=d.id AND kind='purchase_cancelled') THEN 'cancelled'
        WHEN EXISTS(SELECT 1 FROM purchase_receipts WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'receiving' ELSE 'ordering' END;
    IF d.lifecycle<>expected THEN RAISE EXCEPTION 'purchase lifecycle must reflect its execution facts' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM purchase_execution_events x LEFT JOIN purchase_execution_events prev ON prev.org_id=x.org_id AND prev.purchase_id=x.purchase_id AND prev.seq=x.seq-1
        WHERE x.org_id=d.org_id AND x.purchase_id=d.id AND (
            (x.seq>1 AND (prev.id IS NULL OR x.purchase_version<>prev.purchase_version+1))
            OR (x.kind='order_registered' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version=x.purchase_version))
            OR (x.kind='receipt_registered' AND NOT EXISTS(SELECT 1 FROM purchase_receipts a WHERE a.org_id=x.org_id AND a.id=x.receipt_id AND a.order_id=x.order_id AND a.purchase_version=x.purchase_version))
            OR (x.kind='order_cancelled' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version<x.purchase_version))
            OR x.lifecycle<>CASE WHEN EXISTS(SELECT 1 FROM purchase_execution_events cancelled WHERE cancelled.org_id=x.org_id AND cancelled.purchase_id=x.purchase_id AND cancelled.kind='purchase_cancelled' AND cancelled.purchase_version<=x.purchase_version) THEN 'cancelled'
                WHEN EXISTS(SELECT 1 FROM purchase_receipts a WHERE a.org_id=x.org_id AND a.purchase_id=x.purchase_id AND a.purchase_version<=x.purchase_version) THEN 'receiving' ELSE 'ordering' END)) THEN
        RAISE EXCEPTION 'execution events require chronological exact subjects' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_orders o JOIN request_revisions q ON q.org_id=o.org_id AND q.id=o.request_revision_id
        JOIN requests request ON request.org_id=q.org_id AND request.id=q.request_id
        WHERE o.org_id=d.org_id AND o.purchase_id=d.id AND (o.revision_id<>r.id OR o.currency<>r.currency OR q.business_revision_id<>r.id OR request.id<>d.request_id
            OR request.state<>'approved' OR q.id<>request.current_revision_id
            OR NOT EXISTS(SELECT 1 FROM spend_revision_suppliers s WHERE s.org_id=o.org_id AND s.revision_id=r.id AND s.party_id=o.supplier_id AND s.party_revision_id=o.supplier_revision_id)
            OR o.line_count<>(SELECT count(*) FROM purchase_order_lines WHERE org_id=o.org_id AND order_id=o.id)
            OR o.amount<>(SELECT sum(amount) FROM purchase_order_lines WHERE org_id=o.org_id AND order_id=o.id)
            OR NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=o.org_id AND x.purchase_id=o.purchase_id AND x.purchase_version=o.purchase_version AND x.kind='order_registered' AND x.order_id=o.id))) THEN
        RAISE EXCEPTION 'order must preserve the exact approval supplier and all lines' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_order_lines l JOIN spend_lines b ON b.org_id=l.org_id AND b.revision_id=l.revision_id AND b.line_id=l.purchase_line_id
        WHERE l.org_id=d.org_id AND l.purchase_id=d.id AND (l.revision_id<>r.id OR l.description<>b.payload->>'description' OR l.unit<>b.payload->>'unit'
            OR l.unit_price<>(b.payload->>'unit_price')::numeric OR l.tax_amount>COALESCE((b.payload->>'tax_amount')::numeric,0)
            OR l.discount_amount>COALESCE((b.payload->>'discount_amount')::numeric,0)
            OR l.amount<>spend_expected_line_amount('purchase',r.payload,jsonb_build_object('quantity',l.quantity::text,'unit_price',l.unit_price::text,'tax_amount',l.tax_amount::text,'discount_amount',l.discount_amount::text),(SELECT exponent FROM currency_definitions WHERE code=r.currency)))) THEN
        RAISE EXCEPTION 'order pricing must follow its exact approved line' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_commitments(d.org_id,d.id) c JOIN spend_lines b ON b.org_id=d.org_id AND b.revision_id=r.id AND b.line_id=c.line_id
        WHERE c.quantity>(b.payload->>'quantity')::numeric OR c.amount>b.amount OR c.tax>COALESCE((b.payload->>'tax_amount')::numeric,0) OR c.discount>COALESCE((b.payload->>'discount_amount')::numeric,0)) THEN
        RAISE EXCEPTION 'cumulative commitments exceed approved purchase scope' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_receipts a JOIN purchase_orders o ON o.org_id=a.org_id AND o.id=a.order_id
        WHERE a.org_id=d.org_id AND a.purchase_id=d.id AND (a.received_on<o.ordered_on OR a.line_count<>(SELECT count(*) FROM purchase_receipt_lines WHERE org_id=a.org_id AND receipt_id=a.id)
            OR NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=a.org_id AND x.purchase_id=a.purchase_id AND x.purchase_version=a.purchase_version AND x.kind='receipt_registered' AND x.receipt_id=a.id AND x.order_id=a.order_id))) THEN
        RAISE EXCEPTION 'receipt must preserve its order and complete lines' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_receipt_lines WHERE org_id=d.org_id AND purchase_id=d.id GROUP BY receipt_id HAVING bool_or(result='returned') AND bool_or(result<>'returned')) THEN
        RAISE EXCEPTION 'returns and deliveries require separate facts' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_order_lines l WHERE l.org_id=d.org_id AND l.purchase_id=d.id AND (purchase_net_accepted(d.org_id,l.id)<0 OR purchase_net_accepted(d.org_id,l.id)>l.quantity)) THEN
        RAISE EXCEPTION 'net accepted quantity must stay within the order' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_receipt_lines line JOIN purchase_receipts batch ON batch.org_id=line.org_id AND batch.id=line.receipt_id
        JOIN purchase_order_lines ordered ON ordered.org_id=line.org_id AND ordered.id=line.order_line_id
        WHERE line.org_id=d.org_id AND line.purchase_id=d.id AND line.result<>'returned'
        GROUP BY batch.org_id,batch.id,batch.purchase_version,ordered.id,ordered.quantity
        HAVING sum(line.quantity)>ordered.quantity-COALESCE((SELECT sum(CASE earlier.result WHEN 'accepted' THEN earlier.quantity WHEN 'returned' THEN -earlier.quantity ELSE 0 END)
            FROM purchase_receipt_lines earlier JOIN purchase_receipts prior ON prior.org_id=earlier.org_id AND prior.id=earlier.receipt_id
            WHERE earlier.org_id=batch.org_id AND earlier.order_line_id=ordered.id AND prior.purchase_version<batch.purchase_version),0)) THEN
        RAISE EXCEPTION 'each delivery batch must fit the outstanding ordered quantity' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_receipt_lines l JOIN purchase_receipts a ON a.org_id=l.org_id AND a.id=l.receipt_id
        JOIN purchase_receipt_lines source ON source.org_id=l.org_id AND source.id=l.reverses_line_id
        JOIN purchase_receipts original ON original.org_id=source.org_id AND original.id=source.receipt_id
        WHERE l.org_id=d.org_id AND l.purchase_id=d.id AND (source.result<>'accepted' OR source.order_line_id<>l.order_line_id
            OR original.purchase_version>=a.purchase_version OR a.received_on<original.received_on
            OR (SELECT sum(quantity) FROM purchase_receipt_lines WHERE org_id=l.org_id AND reverses_line_id=source.id)>source.quantity)) THEN
        RAISE EXCEPTION 'returns must reverse an earlier accepted line without over-returning' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_receipt_lines l JOIN purchase_receipts a ON a.org_id=l.org_id AND a.id=l.receipt_id
        JOIN purchase_execution_events cancel ON cancel.org_id=l.org_id AND (cancel.order_id=l.order_id OR cancel.kind='purchase_cancelled' AND cancel.purchase_id=l.purchase_id)
            AND cancel.kind IN ('order_cancelled','purchase_cancelled')
        WHERE l.org_id=d.org_id AND l.purchase_id=d.id AND l.result<>'returned' AND a.purchase_version>cancel.purchase_version) THEN
        RAISE EXCEPTION 'cancelled commitments allow returns only' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_orders o JOIN purchase_execution_events cancel ON cancel.org_id=o.org_id AND cancel.purchase_id=o.purchase_id AND cancel.kind='purchase_cancelled'
        WHERE o.org_id=d.org_id AND o.purchase_id=d.id AND o.purchase_version>cancel.purchase_version) THEN
        RAISE EXCEPTION 'cancelled purchases cannot acquire new orders' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_execution_events cancelled JOIN purchase_order_lines l ON l.org_id=cancelled.org_id AND l.purchase_id=cancelled.purchase_id
        WHERE cancelled.org_id=d.org_id AND cancelled.purchase_id=d.id AND cancelled.kind='purchase_cancelled'
        AND l.quantity>COALESCE((SELECT sum(CASE item.result WHEN 'accepted' THEN item.quantity WHEN 'returned' THEN -item.quantity ELSE 0 END)
            FROM purchase_receipt_lines item JOIN purchase_receipts receipt ON receipt.org_id=item.org_id AND receipt.id=item.receipt_id
            WHERE item.org_id=l.org_id AND item.order_line_id=l.id AND receipt.purchase_version<cancelled.purchase_version),0)
        AND NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=l.org_id AND x.order_id=l.order_id AND x.kind='order_cancelled' AND x.purchase_version<cancelled.purchase_version)) THEN
        RAISE EXCEPTION 'purchase cancellation requires closing remaining order commitments' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;
DO $$ DECLARE name text; BEGIN
    FOREACH name IN ARRAY ARRAY['purchase_execution_events','purchase_orders','purchase_order_lines','purchase_receipts','purchase_receipt_lines'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||name,name);
        EXECUTE format('CREATE CONSTRAINT TRIGGER %I AFTER INSERT ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_execution()','validate_'||name,name);
    END LOOP;
END; $$;
CREATE CONSTRAINT TRIGGER purchase_head_execution AFTER UPDATE ON spend_documents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='purchase') EXECUTE FUNCTION validate_purchase_execution();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'purchase_operator','采购履行登记人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('purchase.read'),('purchase.execute')) a(action) WHERE key='purchase_operator' ON CONFLICT DO NOTHING;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON purchase_execution_events,purchase_orders,purchase_order_lines,purchase_receipts,purchase_receipt_lines TO copypaste_app;
        REVOKE UPDATE,DELETE ON purchase_execution_events,purchase_orders,purchase_order_lines,purchase_receipts,purchase_receipt_lines FROM copypaste_app;
    END IF;
END; $$;
