-- Completion records a human review of delivery, differences and actual settlement.
ALTER TABLE spend_documents DROP CONSTRAINT spend_execution_lifecycle;
ALTER TABLE spend_documents ADD CONSTRAINT spend_execution_lifecycle CHECK(
    (kind='claim' AND lifecycle IN ('draft','requested') AND ((approval_state IN ('in_review','approved'))=(lifecycle='requested')))
    OR (kind='purchase' AND ((approval_state='approved' AND lifecycle IN ('requested','ordering','receiving','cancelled','completed'))
        OR (approval_state='in_review' AND lifecycle='requested') OR (approval_state NOT IN ('in_review','approved') AND lifecycle='draft'))));
ALTER TABLE purchase_execution_events DROP CONSTRAINT purchase_execution_events_kind_check;
ALTER TABLE purchase_execution_events DROP CONSTRAINT purchase_execution_events_lifecycle_check;
ALTER TABLE purchase_execution_events ADD CHECK(kind IN ('order_registered','receipt_registered','order_cancelled','purchase_cancelled','plan_activated','purchase_completed'));
ALTER TABLE purchase_execution_events ADD CHECK(lifecycle IN ('requested','ordering','receiving','cancelled','completed'));

CREATE TABLE purchase_completions (
    id uuid PRIMARY KEY,org_id uuid NOT NULL,purchase_id uuid NOT NULL,revision_id uuid NOT NULL,
    purchase_version bigint NOT NULL CHECK(purchase_version>1),occurred_on date NOT NULL,
    basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),fingerprint text NOT NULL CHECK(fingerprint~'^[0-9a-f]{32}$'),
    delivery_verified boolean NOT NULL CHECK(delivery_verified),costs_verified boolean NOT NULL CHECK(costs_verified),
    differences_verified boolean NOT NULL CHECK(differences_verified),
    settlement_mode text NOT NULL CHECK(settlement_mode IN ('recorded_costs','no_payable_obligation')),
    attestation text NOT NULL CHECK(length(btrim(attestation)) BETWEEN 1 AND 20000),
    difference_disposition text NOT NULL CHECK(length(difference_disposition)<=20000),
    evidence_count integer NOT NULL CHECK(evidence_count BETWEEN 1 AND 20),
    actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id),UNIQUE(org_id,purchase_id),UNIQUE(org_id,purchase_id,purchase_version),
    FOREIGN KEY(org_id,purchase_id,revision_id) REFERENCES spend_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TABLE purchase_completion_files (
    org_id uuid NOT NULL,completion_id uuid NOT NULL,version_id uuid NOT NULL,PRIMARY KEY(org_id,completion_id,version_id),
    FOREIGN KEY(org_id,completion_id) REFERENCES purchase_completions(org_id,id),FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE FUNCTION purchase_completion_cost_ids(oid uuid,pid uuid) RETURNS TABLE(id uuid) LANGUAGE sql STABLE AS $$
    WITH linked AS(SELECT cost_id id FROM cost_links WHERE org_id=oid AND source_type='purchase' AND source_id=pid)
    SELECT id FROM linked UNION SELECT d.id FROM actual_cost_entries d WHERE d.org_id=oid AND d.reverses_id IN(SELECT id FROM linked)
$$;
CREATE FUNCTION purchase_completion_cash_ids(oid uuid,pid uuid) RETURNS TABLE(id uuid) LANGUAGE sql STABLE AS $$
    WITH RECURSIVE base AS(SELECT DISTINCT payment_id id FROM payment_allocations WHERE org_id=oid AND cost_id IN(SELECT id FROM purchase_completion_cost_ids(oid,pid))),
    roots AS(SELECT id FROM base UNION SELECT p.reverses_id FROM payment_records p WHERE p.org_id=oid AND p.id IN(SELECT id FROM base)
        UNION SELECT r.refund_of_id FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=oid AND p.id IN(SELECT id FROM base)),
    family(id) AS(SELECT id FROM roots WHERE id IS NOT NULL UNION SELECT p.id FROM payment_records p
        JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id JOIN family f ON p.reverses_id=f.id OR r.refund_of_id=f.id WHERE p.org_id=oid)
    SELECT id FROM family
$$;
CREATE FUNCTION purchase_completion_snapshot(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
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
            'state',d.state,'currency',cr.currency,'amount',cr.amount::text,'paid_amount',COALESCE((SELECT sum(a.effect*a.amount) FROM payment_allocations a WHERE a.org_id=oid AND a.cost_id=d.id),0)::text)
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
CREATE FUNCTION purchase_completion_blockers(b jsonb) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT array_remove(ARRAY[
        CASE WHEN NOT EXISTS(SELECT 1 FROM jsonb_array_elements(b->'orders')o CROSS JOIN LATERAL jsonb_array_elements(o->'lines')l WHERE (l->>'net_accepted_quantity')::numeric>0) THEN 'no_accepted_delivery' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'orders')o CROSS JOIN LATERAL jsonb_array_elements(o->'lines')l WHERE NOT (o->>'cancelled')::boolean AND (l->>'net_accepted_quantity')::numeric<>(l->>'quantity')::numeric) THEN 'order_unfulfilled' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'variances')v WHERE v->>'approval_state'='in_review') THEN 'variance_in_review' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')c WHERE c->>'state'='draft') THEN 'cost_unconfirmed' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')c WHERE c->>'entry_kind'='expense' AND (c->>'paid_amount')::numeric<>(CASE WHEN c->>'state'='confirmed' THEN (c->>'amount')::numeric ELSE 0 END)) THEN 'cost_unsettled' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'payments')p WHERE (p->>'unattributed_refund_amount')::numeric>0) THEN 'unattributed_refund' END,
        CASE WHEN EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')c WHERE c->>'entry_kind'='expense' AND c->>'state'='confirmed' AND c->>'currency' IS DISTINCT FROM b->>'currency') THEN 'currency_mismatch' END
    ],NULL)
$$;
CREATE FUNCTION purchase_completion_has_difference(b jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
    SELECT EXISTS(SELECT 1 FROM jsonb_array_elements(b->'lines')l WHERE (l->>'approved_quantity')::numeric<>(l->>'net_accepted_quantity')::numeric) OR
        EXISTS(SELECT 1 FROM jsonb_array_elements(b->'orders')o WHERE (o->>'cancelled')::boolean) OR
        EXISTS(SELECT 1 FROM jsonb_array_elements(b->'variances')v WHERE v->>'lifecycle'<>'effective') OR
        COALESCE((SELECT sum((l->>'reserved_amount')::numeric) FROM jsonb_array_elements(b->'lines')l),0)<>
        COALESCE((SELECT sum((c->>'amount')::numeric) FROM jsonb_array_elements(b->'costs')c WHERE c->>'entry_kind'='expense' AND c->>'state'='confirmed'),0)
$$;
CREATE FUNCTION can_read_purchase_completion(oid uuid,uid uuid,pid uuid,ch text,ai boolean) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT can_access(oid,uid,'purchase.read','purchase',d.id,d.project_id,d.creator_id,d.classification,ch,ai)
        AND can_access(oid,uid,'cost.read','cost',d.id,d.project_id,d.creator_id,d.classification,ch,ai)
        AND can_access(oid,uid,'payment.read','payment',d.id,d.project_id,d.creator_id,d.classification,ch,ai)
        AND NOT EXISTS(SELECT 1 FROM purchase_completion_cost_ids(oid,pid)x JOIN actual_cost_entries c ON c.org_id=oid AND c.id=x.id WHERE NOT can_read_cost(oid,uid,c.id,c.current_revision_id,ch,ai))
        AND NOT EXISTS(SELECT 1 FROM purchase_completion_cash_ids(oid,pid)p WHERE NOT can_read_payment(oid,uid,p.id,ch,ai))
        AND NOT EXISTS(SELECT 1 FROM purchase_variances v WHERE v.org_id=oid AND v.purchase_id=pid AND NOT can_access(oid,uid,'purchase_variance.read','purchase_variance',v.id,v.project_id,v.creator_id,v.classification,ch,ai))
    FROM spend_documents d WHERE d.org_id=oid AND d.id=pid AND d.kind='purchase'
$$;
CREATE FUNCTION prepare_purchase_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d spend_documents;s sessions;u users;b jsonb;latest date;
BEGIN
    SELECT * INTO STRICT d FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.purchase_id FOR UPDATE;
    IF d.kind<>'purchase' OR d.approval_state<>'approved' OR d.lifecycle NOT IN ('ordering','receiving') OR d.deleted_at IS NOT NULL OR
        NEW.purchase_version<>d.version+1 OR NEW.revision_id IS DISTINCT FROM d.approved_revision_id THEN
        RAISE EXCEPTION 'completion requires the exact approved open purchase' USING ERRCODE='23514'; END IF;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    SELECT * INTO u FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id;
    IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.device_type NOT IN ('web','wechat','douyin','android') OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp() OR
        u.id IS NULL OR u.status<>'active' OR s.auth_version<>u.auth_version OR s.mfa_verified_at IS DISTINCT FROM NEW.mfa_verified_at OR s.mfa_verified_at IS NULL OR
        s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp() THEN
        RAISE EXCEPTION 'purchase completion requires the actual current human MFA session' USING ERRCODE='23514'; END IF;
    PERFORM c.id FROM actual_cost_entries c WHERE c.org_id=NEW.org_id AND c.id IN(SELECT id FROM purchase_completion_cost_ids(NEW.org_id,NEW.purchase_id)) ORDER BY c.id FOR UPDATE;
    PERFORM p.id FROM payment_records p WHERE p.org_id=NEW.org_id AND p.id IN(SELECT id FROM purchase_completion_cash_ids(NEW.org_id,NEW.purchase_id)) ORDER BY p.id FOR UPDATE;
    b:=purchase_completion_snapshot(NEW.org_id,NEW.purchase_id);
    IF jsonb_array_length(b->'orders')>1000 OR jsonb_array_length(b->'receipts')>5000 OR jsonb_array_length(b->'costs')>1000 OR jsonb_array_length(b->'payments')>1000 OR jsonb_array_length(b->'variances')>1000 THEN
        RAISE EXCEPTION 'purchase completion exceeds the bounded single review scope' USING ERRCODE='23514'; END IF;
    IF b IS NULL OR NEW.basis IS DISTINCT FROM b OR NEW.fingerprint<>md5(b::text) THEN
        RAISE EXCEPTION 'purchase completion must pin the actual complete current basis' USING ERRCODE='23514'; END IF;
    IF cardinality(purchase_completion_blockers(b))>0 THEN RAISE EXCEPTION 'purchase completion has unresolved delivery or settlement' USING ERRCODE='23514'; END IF;
    IF purchase_completion_has_difference(b) AND length(btrim(NEW.difference_disposition))<1 THEN
        RAISE EXCEPTION 'purchase completion requires explicit difference disposition' USING ERRCODE='23514'; END IF;
    IF (NEW.settlement_mode='recorded_costs' AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')c WHERE c->>'entry_kind'='expense' AND c->>'state'='confirmed')) OR
        (NEW.settlement_mode='no_payable_obligation' AND EXISTS(SELECT 1 FROM jsonb_array_elements(b->'costs')c WHERE c->>'entry_kind'='expense' AND c->>'state'='confirmed')) THEN
        RAISE EXCEPTION 'purchase completion settlement declaration differs from actual recorded costs' USING ERRCODE='23514'; END IF;
    SELECT max(received_on) INTO latest FROM purchase_receipts WHERE org_id=NEW.org_id AND purchase_id=NEW.purchase_id;
    IF NEW.occurred_on<latest OR NEW.occurred_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date THEN
        RAISE EXCEPTION 'purchase completion date must follow all existing deliveries' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER purchase_completion_prepare BEFORE INSERT ON purchase_completions FOR EACH ROW EXECUTE FUNCTION prepare_purchase_completion();
CREATE FUNCTION validate_purchase_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pid uuid;z purchase_completions;d spend_documents;e purchase_execution_events;
BEGIN
    IF TG_TABLE_NAME='spend_documents' THEN pid:=NEW.id;
    ELSIF TG_TABLE_NAME='purchase_completion_files' THEN SELECT purchase_id INTO pid FROM purchase_completions WHERE org_id=NEW.org_id AND id=NEW.completion_id;
    ELSE pid:=NEW.purchase_id; END IF;
    SELECT * INTO d FROM spend_documents WHERE org_id=NEW.org_id AND id=pid;
    SELECT * INTO z FROM purchase_completions WHERE org_id=NEW.org_id AND purchase_id=pid;
    IF z.id IS NULL THEN
        IF d.lifecycle='completed' OR EXISTS(SELECT 1 FROM purchase_execution_events WHERE org_id=NEW.org_id AND purchase_id=pid AND kind='purchase_completed') THEN
            RAISE EXCEPTION 'completed purchase requires its immutable completion review' USING ERRCODE='23514'; END IF;
        RETURN NULL;
    END IF;
    SELECT * INTO e FROM purchase_execution_events WHERE org_id=NEW.org_id AND purchase_id=pid AND purchase_version=z.purchase_version;
    IF d.lifecycle<>'completed' OR e.id IS NULL OR e.kind<>'purchase_completed' OR e.lifecycle<>'completed' OR e.actor_id<>z.actor_id OR
        e.session_id<>z.session_id OR e.mfa_verified_at<>z.mfa_verified_at OR e.reason<>z.reason OR d.approved_revision_id IS DISTINCT FROM z.revision_id OR
        (SELECT count(*) FROM purchase_completion_files WHERE org_id=z.org_id AND completion_id=z.id)<>z.evidence_count THEN
        RAISE EXCEPTION 'purchase completion requires the exact head event and complete evidence' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER purchase_completion_head AFTER UPDATE ON spend_documents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='purchase') EXECUTE FUNCTION validate_purchase_completion();
CREATE CONSTRAINT TRIGGER purchase_completion_event AFTER INSERT ON purchase_execution_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_completion();
CREATE CONSTRAINT TRIGGER purchase_completion_valid AFTER INSERT ON purchase_completions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_completion();
CREATE CONSTRAINT TRIGGER purchase_completion_files_valid AFTER INSERT ON purchase_completion_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_completion();
ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','contract_closure','purchase_variance_revision','purchase_completion','deliverable','recovery_set'));
CREATE FUNCTION pin_purchase_completion_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,file_id,id,'purchase_completion',NEW.completion_id
        FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;RETURN NEW;
END; $$;
CREATE TRIGGER purchase_completion_file_reference AFTER INSERT ON purchase_completion_files FOR EACH ROW EXECUTE FUNCTION pin_purchase_completion_file();
CREATE TRIGGER immutable_purchase_completions BEFORE UPDATE OR DELETE ON purchase_completions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER immutable_purchase_completion_files BEFORE UPDATE OR DELETE ON purchase_completion_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_completed_purchase_variance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE stage text;BEGIN
    SELECT lifecycle INTO stage FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.purchase_id FOR UPDATE;
    IF stage='completed' THEN RAISE EXCEPTION 'completed purchases cannot acquire or mutate difference proposals' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER completed_purchase_variance BEFORE INSERT OR UPDATE ON purchase_variances FOR EACH ROW EXECUTE FUNCTION guard_completed_purchase_variance();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'purchase_completer','采购完成核对人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a FROM roles r CROSS JOIN unnest(ARRAY['purchase.read','purchase.complete'])a WHERE r.key='purchase_completer';
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT ON purchase_completions,purchase_completion_files TO copypaste_app;
    REVOKE UPDATE,DELETE ON purchase_completions,purchase_completion_files FROM copypaste_app;
END IF; END; $$;

-- Preserve all existing price, quantity, approval, return and chronology checks.
CREATE OR REPLACE FUNCTION validate_purchase_execution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; d spend_documents; r spend_revisions; e purchase_execution_events; expected text;
BEGIN
    IF TG_TABLE_NAME='spend_documents' THEN target:=NEW.id; ELSE target:=NEW.purchase_id; END IF;
    SELECT * INTO d FROM spend_documents WHERE org_id=NEW.org_id AND id=target FOR UPDATE;
    IF d.kind<>'purchase' THEN RETURN NULL; END IF;
    SELECT * INTO r FROM spend_revisions WHERE org_id=d.org_id AND id=d.current_revision_id;
    SELECT * INTO e FROM purchase_execution_events WHERE org_id=d.org_id AND purchase_id=d.id ORDER BY seq DESC LIMIT 1;
    IF NOT FOUND THEN
        IF d.lifecycle IN ('ordering','receiving','cancelled','completed') THEN
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
    expected:=CASE WHEN EXISTS(SELECT 1 FROM purchase_completions WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'completed' WHEN EXISTS(SELECT 1 FROM purchase_execution_events WHERE org_id=d.org_id AND purchase_id=d.id AND kind='purchase_cancelled') THEN 'cancelled'
        WHEN EXISTS(SELECT 1 FROM purchase_receipts WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'receiving' WHEN EXISTS(SELECT 1 FROM purchase_orders WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'ordering' ELSE 'requested' END;
    IF d.lifecycle<>expected THEN RAISE EXCEPTION 'purchase lifecycle must reflect its execution facts' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM purchase_execution_events x LEFT JOIN purchase_execution_events prev ON prev.org_id=x.org_id AND prev.purchase_id=x.purchase_id AND prev.seq=x.seq-1
        WHERE x.org_id=d.org_id AND x.purchase_id=d.id AND (
            (x.seq>1 AND (prev.id IS NULL OR x.purchase_version<>prev.purchase_version+1))
            OR (x.kind='purchase_completed' AND NOT EXISTS(SELECT 1 FROM purchase_completions z WHERE z.org_id=x.org_id AND z.purchase_id=x.purchase_id AND z.purchase_version=x.purchase_version))
            OR (x.kind='plan_activated' AND NOT EXISTS(SELECT 1 FROM purchase_plan_activations a WHERE a.org_id=x.org_id AND a.purchase_id=x.purchase_id AND a.purchase_version=x.purchase_version AND a.variance_revision_id=x.variance_revision_id))
            OR (x.kind='order_registered' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version=x.purchase_version))
            OR (x.kind='receipt_registered' AND NOT EXISTS(SELECT 1 FROM purchase_receipts a WHERE a.org_id=x.org_id AND a.id=x.receipt_id AND a.order_id=x.order_id AND a.purchase_version=x.purchase_version))
            OR (x.kind='order_cancelled' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version<x.purchase_version))
            OR x.lifecycle<>CASE WHEN EXISTS(SELECT 1 FROM purchase_completions z WHERE z.org_id=x.org_id AND z.purchase_id=x.purchase_id AND z.purchase_version<=x.purchase_version) THEN 'completed' WHEN EXISTS(SELECT 1 FROM purchase_execution_events cancelled WHERE cancelled.org_id=x.org_id AND cancelled.purchase_id=x.purchase_id AND cancelled.kind='purchase_cancelled' AND cancelled.purchase_version<=x.purchase_version) THEN 'cancelled'
                WHEN EXISTS(SELECT 1 FROM purchase_receipts a WHERE a.org_id=x.org_id AND a.purchase_id=x.purchase_id AND a.purchase_version<=x.purchase_version) THEN 'receiving' WHEN EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.purchase_id=x.purchase_id AND o.purchase_version<=x.purchase_version) THEN 'ordering' ELSE 'requested' END)) THEN
        RAISE EXCEPTION 'execution events require chronological exact subjects' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_orders o JOIN request_revisions q ON q.org_id=o.org_id AND q.id=o.request_revision_id
        JOIN requests request ON request.org_id=q.org_id AND request.id=q.request_id
        WHERE o.org_id=d.org_id AND o.purchase_id=d.id AND (o.revision_id<>r.id OR o.currency<>r.currency OR q.business_revision_id<>r.id OR request.id<>d.request_id
            OR request.state<>'approved' OR q.id<>request.current_revision_id
            OR o.execution_plan_revision_id IS DISTINCT FROM (SELECT revision_id FROM purchase_effective_plan(o.org_id,o.purchase_id,o.purchase_version))
            OR NOT EXISTS(SELECT 1 FROM purchase_effective_plan(o.org_id,o.purchase_id,o.purchase_version) plan CROSS JOIN LATERAL jsonb_array_elements(plan.payload->'supplier_candidates') s WHERE s->>'party_id'=o.supplier_id::text AND s->>'revision_id'=o.supplier_revision_id::text)
            OR o.line_count<>(SELECT count(*) FROM purchase_order_lines WHERE org_id=o.org_id AND order_id=o.id)
            OR o.amount<>(SELECT sum(amount) FROM purchase_order_lines WHERE org_id=o.org_id AND order_id=o.id)
            OR NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=o.org_id AND x.purchase_id=o.purchase_id AND x.purchase_version=o.purchase_version AND x.kind='order_registered' AND x.order_id=o.id))) THEN
        RAISE EXCEPTION 'order must preserve the exact approval supplier and all lines' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_order_lines l JOIN purchase_orders o ON o.org_id=l.org_id AND o.id=l.order_id
        CROSS JOIN LATERAL purchase_effective_plan(o.org_id,o.purchase_id,o.purchase_version) plan
        LEFT JOIN LATERAL(SELECT value FROM jsonb_array_elements(plan.payload->'lines') WHERE value->>'id'=l.purchase_line_id::text) b ON true
        WHERE l.org_id=d.org_id AND l.purchase_id=d.id AND (b.value IS NULL OR l.revision_id<>r.id OR l.description<>b.value->>'description' OR l.unit<>b.value->>'unit'
            OR l.unit_price<>(b.value->>'unit_price')::numeric OR l.tax_amount>COALESCE((b.value->>'tax_amount')::numeric,0)
            OR l.discount_amount>COALESCE((b.value->>'discount_amount')::numeric,0)
            OR l.amount<>spend_expected_line_amount('purchase',plan.payload,jsonb_build_object('quantity',l.quantity::text,'unit_price',l.unit_price::text,'tax_amount',l.tax_amount::text,'discount_amount',l.discount_amount::text),(SELECT exponent FROM currency_definitions WHERE code=r.currency)))) THEN
        RAISE EXCEPTION 'order pricing must follow its exact enabled execution plan' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM purchase_commitments(d.org_id,d.id) c CROSS JOIN LATERAL purchase_effective_plan(d.org_id,d.id) plan
        LEFT JOIN LATERAL(SELECT value FROM jsonb_array_elements(plan.payload->'lines') WHERE value->>'id'=c.line_id::text) b ON true
        WHERE b.value IS NULL OR c.quantity>(b.value->>'quantity')::numeric OR c.tax>COALESCE((b.value->>'tax_amount')::numeric,0) OR c.discount>COALESCE((b.value->>'discount_amount')::numeric,0)
            OR c.amount>(SELECT (v->>'amount')::numeric FROM jsonb_array_elements(plan.line_values) v WHERE v->>'line_id'=c.line_id::text)) THEN
        RAISE EXCEPTION 'cumulative commitments exceed approved purchase scope: enabled execution plan' USING ERRCODE='23514';
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
    IF EXISTS(SELECT 1 FROM purchase_execution_events x JOIN purchase_completions z ON z.org_id=x.org_id AND z.purchase_id=x.purchase_id
        WHERE x.org_id=d.org_id AND x.purchase_id=d.id AND x.purchase_version>z.purchase_version AND
            (x.kind<>'receipt_registered' OR EXISTS(SELECT 1 FROM purchase_receipt_lines l WHERE l.org_id=x.org_id AND l.receipt_id=x.receipt_id AND l.result<>'returned'))) THEN
        RAISE EXCEPTION 'completed purchases allow only source returns after completion' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;

CREATE TRIGGER completed_purchase_activation BEFORE INSERT ON purchase_plan_activations FOR EACH ROW EXECUTE FUNCTION guard_completed_purchase_variance();
