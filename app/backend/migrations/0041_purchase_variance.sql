-- A reviewed commercial variance is a separate approval, activated explicitly.
-- The original purchase approval and every existing order remain immutable.
ALTER TABLE workflow_definitions DROP CONSTRAINT workflow_definitions_kind_check;
ALTER TABLE workflow_definitions ADD CHECK(kind IN ('contract','purchase','claim','change','purchase_variance'));
ALTER TABLE requests DROP CONSTRAINT requests_kind_check;
ALTER TABLE requests ADD CHECK(kind IN ('contract','purchase','claim','change','purchase_variance'));

CREATE TABLE purchase_variances (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL,
    kind text NOT NULL DEFAULT 'purchase_variance' CHECK(kind='purchase_variance'), document_no text NOT NULL,
    project_id uuid, creator_id uuid NOT NULL, classification text NOT NULL,
    lifecycle text NOT NULL DEFAULT 'draft' CHECK(lifecycle IN ('draft','requested','effective')),
    approval_state text NOT NULL DEFAULT 'not_submitted' CHECK(approval_state IN ('not_submitted','in_review','approved','returned','rejected','withdrawn')),
    current_revision_id uuid NOT NULL, approved_revision_id uuid, request_id uuid, deleted_at timestamptz CHECK(deleted_at IS NULL),
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,purchase_id,id), UNIQUE(org_id,document_no),
    FOREIGN KEY(org_id,purchase_id) REFERENCES spend_documents(org_id,id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id), FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,request_id,kind,id) REFERENCES requests(org_id,id,kind,business_id) DEFERRABLE INITIALLY DEFERRED,
    CHECK((approval_state='approved')=(approved_revision_id IS NOT NULL)),
    CHECK(approved_revision_id IS NULL OR approved_revision_id=current_revision_id),
    CHECK((approval_state IN ('in_review','approved'))=(lifecycle IN ('requested','effective'))),
    CHECK(lifecycle<>'effective' OR approval_state='approved')
);
CREATE TABLE purchase_variance_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, document_id uuid NOT NULL, purchase_id uuid NOT NULL,
    seq bigint NOT NULL CHECK(seq>0), base_revision_id uuid,
    payload_schema_version integer NOT NULL DEFAULT 1 CHECK(payload_schema_version=1),
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),
    line_values jsonb NOT NULL CHECK(jsonb_typeof(line_values)='array'),
    title text GENERATED ALWAYS AS (payload->>'title') STORED NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
    category text GENERATED ALWAYS AS (payload->>'category') STORED NOT NULL,
    currency text GENERATED ALWAYS AS (payload->>'currency') STORED NOT NULL REFERENCES currency_definitions(code),
    purchase_version bigint GENERATED ALWAYS AS ((basis->>'purchase_version')::bigint) STORED NOT NULL,
    original_revision_id uuid GENERATED ALWAYS AS ((basis->>'original_revision_id')::uuid) STORED NOT NULL,
    execution_plan_revision_id uuid GENERATED ALWAYS AS ((basis->>'execution_plan_revision_id')::uuid) STORED,
    content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'), amount numeric(20,6) NOT NULL CHECK(amount>=0),
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,document_id,id), UNIQUE(org_id,purchase_id,id), UNIQUE(org_id,document_id,seq),
    FOREIGN KEY(org_id,purchase_id,document_id) REFERENCES purchase_variances(org_id,purchase_id,id),
    FOREIGN KEY(org_id,document_id,base_revision_id) REFERENCES purchase_variance_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,purchase_id,original_revision_id) REFERENCES spend_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,purchase_id,execution_plan_revision_id) REFERENCES purchase_variance_revisions(org_id,purchase_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)),
    CHECK(jsonb_typeof(payload->'lines')='array' AND jsonb_array_length(payload->'lines') BETWEEN 1 AND 100)
);
ALTER TABLE purchase_variances ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES purchase_variance_revisions(org_id,document_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE purchase_variances ADD FOREIGN KEY(org_id,id,approved_revision_id) REFERENCES purchase_variance_revisions(org_id,document_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX purchase_variance_list ON purchase_variances(org_id,purchase_id,updated_at DESC,id DESC);
CREATE TABLE purchase_variance_files (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, version_id uuid NOT NULL,
    PRIMARY KEY(org_id,revision_id,version_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES purchase_variance_revisions(org_id,id),
    FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TABLE purchase_plan_activations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, purchase_id uuid NOT NULL, purchase_version bigint NOT NULL,
    variance_id uuid NOT NULL, variance_revision_id uuid NOT NULL, request_revision_id uuid NOT NULL,
    actor_id uuid NOT NULL, session_id uuid NOT NULL, mfa_verified_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,purchase_id,purchase_version), UNIQUE(org_id,variance_id),
    FOREIGN KEY(org_id,purchase_id,variance_id) REFERENCES purchase_variances(org_id,purchase_id,id),
    FOREIGN KEY(org_id,variance_id,variance_revision_id) REFERENCES purchase_variance_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,request_revision_id) REFERENCES request_revisions(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    FOREIGN KEY(org_id,purchase_id,purchase_version) REFERENCES purchase_execution_events(org_id,purchase_id,purchase_version) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE purchase_execution_events ADD COLUMN variance_revision_id uuid;
ALTER TABLE purchase_execution_events ADD FOREIGN KEY(org_id,purchase_id,variance_revision_id) REFERENCES purchase_variance_revisions(org_id,purchase_id,id);
ALTER TABLE purchase_execution_events DROP CONSTRAINT purchase_execution_events_kind_check;
ALTER TABLE purchase_execution_events DROP CONSTRAINT purchase_execution_events_lifecycle_check;
ALTER TABLE purchase_execution_events ADD CHECK(kind IN ('order_registered','receipt_registered','order_cancelled','purchase_cancelled','plan_activated'));
ALTER TABLE purchase_execution_events ADD CHECK(lifecycle IN ('requested','ordering','receiving','cancelled'));
ALTER TABLE purchase_execution_events ADD CHECK((kind='plan_activated')=(variance_revision_id IS NOT NULL));
ALTER TABLE purchase_orders ADD COLUMN execution_plan_revision_id uuid;
ALTER TABLE purchase_orders ADD FOREIGN KEY(org_id,purchase_id,execution_plan_revision_id) REFERENCES purchase_variance_revisions(org_id,purchase_id,id);

CREATE FUNCTION purchase_effective_plan(oid uuid,pid uuid,before_version bigint DEFAULT 9007199254740991)
RETURNS TABLE(payload jsonb,amount numeric,line_values jsonb,variance_id uuid,revision_id uuid,request_id uuid,activated_at timestamptz)
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(v.payload,r.payload),COALESCE(v.amount,r.amount),COALESCE(v.line_values,
        (SELECT COALESCE(jsonb_agg(jsonb_build_object('line_id',l.line_id,'amount',l.amount::text,'duplicate_receipt',false) ORDER BY position),'[]'::jsonb) FROM spend_lines l WHERE l.org_id=r.org_id AND l.revision_id=r.id)),
        a.variance_id,a.variance_revision_id,q.request_id,a.created_at
    FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id
    LEFT JOIN LATERAL(SELECT x.* FROM purchase_plan_activations x WHERE x.org_id=d.org_id AND x.purchase_id=d.id AND x.purchase_version<before_version ORDER BY x.purchase_version DESC LIMIT 1) a ON true
    LEFT JOIN purchase_variance_revisions v ON v.org_id=a.org_id AND v.id=a.variance_revision_id
    LEFT JOIN request_revisions q ON q.org_id=a.org_id AND q.id=a.request_revision_id
    WHERE d.org_id=oid AND d.id=pid AND d.kind='purchase';
$$;
CREATE FUNCTION purchase_fulfillment_lines(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT COALESCE(jsonb_agg(jsonb_build_object('line_id',b.item->>'id','approved_quantity',(b.item->>'quantity')::numeric(20,6)::text,
        'reserved_quantity',COALESCE(c.quantity,0)::numeric(20,6)::text,
        'net_accepted_quantity',COALESCE((SELECT sum(purchase_net_accepted(l.org_id,l.id)) FROM purchase_order_lines l WHERE l.org_id=oid AND l.purchase_id=pid AND l.purchase_line_id=(b.item->>'id')::uuid),0)::numeric(20,6)::text,
        'outstanding_quantity',COALESCE((SELECT sum(l.quantity-purchase_net_accepted(l.org_id,l.id)) FROM purchase_order_lines l WHERE l.org_id=oid AND l.purchase_id=pid AND l.purchase_line_id=(b.item->>'id')::uuid
            AND NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=l.org_id AND (x.order_id=l.order_id AND x.kind='order_cancelled' OR x.purchase_id=l.purchase_id AND x.kind='purchase_cancelled'))),0)::numeric(20,6)::text,
        'approved_amount',(p.line_values->(b.position::integer-1)->>'amount')::numeric(20,6)::text,
        'reserved_amount',COALESCE(c.amount,0)::numeric(20,6)::text,'reserved_tax',COALESCE(c.tax,0)::numeric(20,6)::text,'reserved_discount',COALESCE(c.discount,0)::numeric(20,6)::text) ORDER BY b.position),'[]'::jsonb)
    FROM purchase_effective_plan(oid,pid) p CROSS JOIN LATERAL jsonb_array_elements(p.payload->'lines') WITH ORDINALITY b(item,position)
    LEFT JOIN purchase_commitments(oid,pid) c ON c.line_id=(b.item->>'id')::uuid;
$$;
CREATE FUNCTION purchase_variance_basis(oid uuid,pid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
    SELECT jsonb_build_object('purchase_id',d.id,'purchase_version',d.version,'original_revision_id',r.id,'original_content_hash',r.content_hash,
        'execution_plan_revision_id',p.revision_id,'content',p.payload,'amount',p.amount::text,'line_values',p.line_values,'commitments',purchase_fulfillment_lines(oid,pid))
    FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id CROSS JOIN LATERAL purchase_effective_plan(oid,pid) p
    WHERE d.org_id=oid AND d.id=pid;
$$;
CREATE FUNCTION preserve_purchase_variance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.purchase_id,NEW.kind,NEW.document_no,NEW.project_id,NEW.creator_id,NEW.classification,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.purchase_id,OLD.kind,OLD.document_no,OLD.project_id,OLD.creator_id,OLD.classification,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR OLD.lifecycle='effective'
        OR (OLD.approval_state='approved' AND ROW(NEW.current_revision_id,NEW.approved_revision_id,NEW.request_id,NEW.approval_state) IS DISTINCT FROM ROW(OLD.current_revision_id,OLD.approved_revision_id,OLD.request_id,OLD.approval_state))
        OR (OLD.approval_state='in_review' AND NEW.current_revision_id<>OLD.current_revision_id) THEN
        RAISE EXCEPTION 'variance identity and reviewing or approved content are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER preserve_purchase_variance BEFORE UPDATE ON purchase_variances FOR EACH ROW EXECUTE FUNCTION preserve_purchase_variance();
CREATE FUNCTION purchase_variance_file_ids(proposed jsonb,basis jsonb) RETURNS TABLE(version_id uuid) LANGUAGE sql IMMUTABLE AS $$
    SELECT DISTINCT pinned.id::uuid FROM (SELECT proposed AS value UNION ALL SELECT basis->'content') content
    CROSS JOIN LATERAL(SELECT id FROM jsonb_array_elements_text(content.value->'quote_version_ids') id
        UNION ALL SELECT id FROM jsonb_array_elements(content.value->'lines') line CROSS JOIN LATERAL jsonb_array_elements_text(line->'receipt_version_ids') id) pinned;
$$;
CREATE FUNCTION validate_purchase_variance_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r purchase_variance_revisions; d purchase_variances; p spend_documents; original spend_revisions; item jsonb; v jsonb; computed numeric:=0; n integer; exponent integer; target uuid;
BEGIN
    IF TG_TABLE_NAME='purchase_variances' THEN target:=NEW.current_revision_id;
    ELSIF TG_TABLE_NAME='purchase_variance_files' THEN target:=NEW.revision_id; ELSE target:=NEW.id; END IF;
    SELECT * INTO r FROM purchase_variance_revisions WHERE org_id=NEW.org_id AND id=target;
    SELECT * INTO d FROM purchase_variances WHERE org_id=r.org_id AND id=r.document_id;
    SELECT * INTO p FROM spend_documents WHERE org_id=r.org_id AND id=r.purchase_id FOR UPDATE;
    SELECT * INTO original FROM spend_revisions WHERE org_id=p.org_id AND id=p.current_revision_id;
    IF p.kind<>'purchase' OR p.approval_state<>'approved' OR p.deleted_at IS NOT NULL
        OR ROW(d.project_id,d.classification) IS DISTINCT FROM ROW(p.project_id,p.classification)
        OR r.original_revision_id IS DISTINCT FROM original.id OR r.basis->>'purchase_id' IS DISTINCT FROM p.id::text
        OR r.basis->>'original_content_hash' IS DISTINCT FROM original.content_hash
        OR ROW(r.currency,r.payload->>'pricing_basis',r.payload->>'rounding_mode') IS DISTINCT FROM ROW(original.currency,original.payload->>'pricing_basis',original.payload->>'rounding_mode')
        OR jsonb_array_length(r.payload->'lines')<>jsonb_array_length(original.payload->'lines')
        OR jsonb_array_length(r.line_values)<>jsonb_array_length(r.payload->'lines')
        OR (SELECT count(DISTINCT row_item.value->>'id') FROM jsonb_array_elements(r.payload->'lines') row_item(value))<>jsonb_array_length(r.payload->'lines') THEN
        RAISE EXCEPTION 'variance must preserve original purchase identity currency and scope' USING ERRCODE='23514';
    END IF;
    SELECT c.exponent INTO exponent FROM currency_definitions c WHERE c.code=r.currency;
    FOR item,n IN SELECT value,ordinality::integer FROM jsonb_array_elements(r.payload->'lines') WITH ORDINALITY LOOP
        IF NOT EXISTS(SELECT 1 FROM jsonb_array_elements(original.payload->'lines') l WHERE l->>'id'=item->>'id' AND l->>'description'=item->>'description' AND l->>'unit'=item->>'unit')
            OR item->>'quantity' IS NULL OR item->>'unit_price' IS NULL
            OR (item->>'quantity')::numeric<=0 OR (item->>'unit_price')::numeric<0 THEN
            RAISE EXCEPTION 'variance lines must preserve their exact approved identity and unit' USING ERRCODE='23514'; END IF;
        v:=r.line_values->(n-1);
        IF v->>'line_id' IS DISTINCT FROM item->>'id' OR v->>'duplicate_receipt' IS DISTINCT FROM 'false'
            OR (v->>'amount')::numeric IS DISTINCT FROM spend_expected_line_amount('purchase',r.payload,item,exponent)
            OR v->>'amount' IS NULL THEN
            RAISE EXCEPTION 'variance amounts must be independently computed from exact inputs' USING ERRCODE='23514'; END IF;
        computed:=computed+(v->>'amount')::numeric;
    END LOOP;
    IF computed<>r.amount OR (r.seq>1 AND NOT EXISTS(SELECT 1 FROM purchase_variance_revisions previous WHERE previous.org_id=r.org_id AND previous.id=r.base_revision_id AND previous.document_id=r.document_id AND previous.seq=r.seq-1))
        OR (SELECT count(*) FROM purchase_variance_revisions WHERE org_id=d.org_id AND document_id=d.id)<>(SELECT max(seq) FROM purchase_variance_revisions WHERE org_id=d.org_id AND document_id=d.id)
        OR d.current_revision_id IS DISTINCT FROM (SELECT id FROM purchase_variance_revisions WHERE org_id=d.org_id AND document_id=d.id ORDER BY seq DESC LIMIT 1) THEN
        RAISE EXCEPTION 'variance requires a complete immutable revision sequence' USING ERRCODE='23514'; END IF;
    IF EXISTS((SELECT version_id FROM purchase_variance_file_ids(r.payload,r.basis))
        EXCEPT SELECT version_id FROM purchase_variance_files WHERE org_id=r.org_id AND revision_id=r.id)
        OR EXISTS(SELECT version_id FROM purchase_variance_files WHERE org_id=r.org_id AND revision_id=r.id
        EXCEPT SELECT version_id FROM purchase_variance_file_ids(r.payload,r.basis)) THEN
        RAISE EXCEPTION 'variance file pins must exactly cover proposed and prior content' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER purchase_variance_revision_valid AFTER INSERT ON purchase_variance_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_revision();
CREATE CONSTRAINT TRIGGER purchase_variance_head_valid AFTER INSERT OR UPDATE ON purchase_variances DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_revision();
CREATE CONSTRAINT TRIGGER purchase_variance_files_valid AFTER INSERT ON purchase_variance_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_revision();
CREATE FUNCTION guard_purchase_variance_basis() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM id FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.purchase_id FOR UPDATE;
    IF NEW.basis IS DISTINCT FROM purchase_variance_basis(NEW.org_id,NEW.purchase_id) THEN
        RAISE EXCEPTION 'new variance revision requires the exact current purchase basis' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER guard_purchase_variance_basis BEFORE INSERT ON purchase_variance_revisions FOR EACH ROW EXECUTE FUNCTION guard_purchase_variance_basis();

ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_order','purchase_receipt','claim_revision','cost_revision','payment_revision','subscription_revision','subscription_period_event','contract_finance_event','contract_closure','purchase_variance_revision','deliverable','recovery_set'));
CREATE FUNCTION pin_purchase_variance_file() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,file_id,id,'purchase_variance_revision',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER pin_purchase_variance_file AFTER INSERT ON purchase_variance_files FOR EACH ROW EXECUTE FUNCTION pin_purchase_variance_file();
CREATE FUNCTION validate_purchase_variance_request() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d purchase_variances; r requests; target uuid;
BEGIN
    IF TG_TABLE_NAME='requests' THEN
        IF NEW.kind<>'purchase_variance' THEN RETURN NULL; END IF;
        target:=NEW.business_id;
    ELSE target:=NEW.id; END IF;
    SELECT * INTO d FROM purchase_variances WHERE org_id=NEW.org_id AND id=target;
    IF NOT FOUND THEN RAISE EXCEPTION 'variance request needs its exact business' USING ERRCODE='23514'; END IF;
    IF d.request_id IS NULL THEN
        IF d.approval_state<>'not_submitted' OR d.approved_revision_id IS NOT NULL THEN RAISE EXCEPTION 'variance requires its request' USING ERRCODE='23514'; END IF;
    ELSE
        SELECT * INTO r FROM requests WHERE org_id=d.org_id AND id=d.request_id;
        IF r.kind<>'purchase_variance' OR r.business_id<>d.id OR ROW(r.project_id,r.classification) IS DISTINCT FROM ROW(d.project_id,d.classification)
            OR d.approval_state IS DISTINCT FROM (CASE WHEN r.state='draft' THEN 'not_submitted' ELSE r.state END) THEN
            RAISE EXCEPTION 'variance and request state or scope differ' USING ERRCODE='23514'; END IF;
    END IF;
    IF (d.lifecycle='effective')<>EXISTS(SELECT 1 FROM purchase_plan_activations WHERE org_id=d.org_id AND variance_id=d.id) THEN
        RAISE EXCEPTION 'effective variance requires its immutable activation' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER purchase_variance_request_valid AFTER INSERT OR UPDATE ON purchase_variances DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_request();
CREATE CONSTRAINT TRIGGER request_variance_valid AFTER INSERT OR UPDATE ON requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_request();
CREATE FUNCTION validate_purchase_variance_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d purchase_variances; r purchase_variance_revisions;
BEGIN
    IF NOT EXISTS(SELECT 1 FROM requests WHERE org_id=NEW.org_id AND id=NEW.request_id AND kind='purchase_variance') THEN RETURN NEW; END IF;
    SELECT v.* INTO r FROM purchase_variance_revisions v JOIN requests q ON q.org_id=v.org_id AND q.business_id=v.document_id WHERE q.org_id=NEW.org_id AND q.id=NEW.request_id AND v.id=NEW.business_revision_id;
    SELECT * INTO d FROM purchase_variances WHERE org_id=r.org_id AND id=r.document_id;
    IF r.id IS NULL OR NEW.business_content_hash<>r.content_hash OR NEW.payload->>'kind'<>'purchase_variance'
        OR NEW.payload->'content' IS DISTINCT FROM r.payload OR NEW.payload->'purchase_variance' IS DISTINCT FROM r.basis
        OR NEW.payload->'line_values' IS DISTINCT FROM r.line_values OR (NEW.payload->>'amount')::numeric IS DISTINCT FROM r.amount
        OR NEW.payload->>'document_no'<>d.document_no OR NEW.payload->>'classification'<>d.classification
        OR (NEW.payload->>'project_id')::uuid IS DISTINCT FROM d.project_id
        OR EXISTS(SELECT version_id FROM purchase_variance_files WHERE org_id=r.org_id AND revision_id=r.id EXCEPT SELECT (x->>'version_id')::uuid FROM jsonb_array_elements(NEW.payload->'attachments') x)
        OR EXISTS(SELECT (x->>'version_id')::uuid FROM jsonb_array_elements(NEW.payload->'attachments') x EXCEPT SELECT version_id FROM purchase_variance_files WHERE org_id=r.org_id AND revision_id=r.id) THEN
        RAISE EXCEPTION 'variance approval snapshot must preserve all fixed facts and evidence' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER purchase_variance_snapshot BEFORE INSERT ON request_revisions FOR EACH ROW EXECUTE FUNCTION validate_purchase_variance_snapshot();

CREATE FUNCTION guard_purchase_plan_activation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p spend_documents; d purchase_variances; r purchase_variance_revisions; q requests; snap request_revisions; s sessions; current_basis jsonb; spent numeric;
BEGIN
    SELECT * INTO p FROM spend_documents WHERE org_id=NEW.org_id AND id=NEW.purchase_id FOR UPDATE;
    SELECT * INTO d FROM purchase_variances WHERE org_id=NEW.org_id AND id=NEW.variance_id FOR UPDATE;
    SELECT * INTO r FROM purchase_variance_revisions WHERE org_id=d.org_id AND id=d.current_revision_id;
    SELECT * INTO q FROM requests WHERE org_id=d.org_id AND id=d.request_id;
    SELECT * INTO snap FROM request_revisions WHERE org_id=q.org_id AND id=q.current_revision_id;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    IF s.id IS NULL OR s.user_id<>NEW.actor_id OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp()
        OR s.device_type NOT IN ('web','android','wechat','douyin') OR NEW.mfa_verified_at IS DISTINCT FROM s.mfa_verified_at
        OR s.mfa_verified_at IS NULL OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp()
        OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.actor_id AND status='active' AND auth_version=s.auth_version) THEN
        RAISE EXCEPTION 'activation requires the actual current human session and recent MFA' USING ERRCODE='23514'; END IF;
    current_basis:=purchase_variance_basis(NEW.org_id,NEW.purchase_id);
    IF p.kind<>'purchase' OR p.approval_state<>'approved' OR p.lifecycle='cancelled' OR p.version+1<>NEW.purchase_version
        OR d.purchase_id<>p.id OR d.approval_state<>'approved' OR d.lifecycle<>'requested' OR r.id<>NEW.variance_revision_id
        OR r.basis IS DISTINCT FROM current_basis OR q.state<>'approved' OR q.business_id<>d.id OR q.kind<>'purchase_variance'
        OR q.current_revision_id<>NEW.request_revision_id OR snap.business_revision_id<>r.id OR snap.business_content_hash<>r.content_hash
        OR NOT EXISTS(SELECT 1 FROM approval_instances WHERE org_id=q.org_id AND id=q.active_instance_id AND state='approved') THEN
        RAISE EXCEPTION 'activation requires the exact approved proposal and unchanged purchase basis' USING ERRCODE='23514'; END IF;
    PERFORM c.id FROM actual_cost_entries c JOIN cost_links l ON l.org_id=c.org_id AND l.cost_id=c.id WHERE l.org_id=p.org_id AND l.source_type='purchase' AND l.source_id=p.id ORDER BY c.id FOR UPDATE OF c;
    SELECT COALESCE(sum(v.amount),0) INTO spent FROM actual_cost_entries c JOIN cost_revisions v ON v.org_id=c.org_id AND v.id=c.current_revision_id
        JOIN cost_links l ON l.org_id=c.org_id AND l.cost_id=c.id WHERE l.org_id=p.org_id AND l.source_type='purchase' AND l.source_id=p.id AND c.state='confirmed' AND c.entry_kind='expense';
    IF spent>r.amount OR EXISTS(SELECT 1 FROM purchase_commitments(p.org_id,p.id) c LEFT JOIN LATERAL(SELECT value FROM jsonb_array_elements(r.payload->'lines') WHERE value->>'id'=c.line_id::text) b ON true
        WHERE b.value IS NULL OR c.quantity>(b.value->>'quantity')::numeric OR c.tax>COALESCE((b.value->>'tax_amount')::numeric,0)
            OR c.discount>COALESCE((b.value->>'discount_amount')::numeric,0) OR c.amount>(SELECT (v->>'amount')::numeric FROM jsonb_array_elements(r.line_values) v WHERE v->>'line_id'=c.line_id::text)) THEN
        RAISE EXCEPTION 'execution plan cannot fall below current commitments or actual costs' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER guard_purchase_plan_activation BEFORE INSERT ON purchase_plan_activations FOR EACH ROW EXECUTE FUNCTION guard_purchase_plan_activation();
DO $$ DECLARE name text; BEGIN
    FOREACH name IN ARRAY ARRAY['purchase_variance_revisions','purchase_variance_files','purchase_plan_activations'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION reject_immutable_change()','immutable_'||name,name);
    END LOOP;
END; $$;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),o.id,r.key,r.name,r.mfa FROM organizations o CROSS JOIN (VALUES
        ('purchase_variance_manager','采购差异提案管理人',false),('purchase_variance_reader','采购差异查阅人',false),('purchase_plan_activator','采购执行基线启用人',true)) r(key,name,mfa);
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN (VALUES
    ('purchase_variance_manager','purchase.read'),('purchase_variance_manager','purchase_variance.read'),('purchase_variance_manager','purchase_variance.create'),('purchase_variance_manager','purchase_variance.update'),
    ('purchase_variance_reader','purchase_variance.read'),('purchase_plan_activator','purchase.read'),('purchase_plan_activator','purchase_variance.read'),('purchase_plan_activator','purchase_variance.activate')) a(key,action) WHERE r.key=a.key;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON purchase_variances TO copypaste_app;
        GRANT SELECT,INSERT ON purchase_variance_revisions,purchase_variance_files,purchase_plan_activations TO copypaste_app;
        REVOKE DELETE ON purchase_variances FROM copypaste_app;
        REVOKE UPDATE,DELETE ON purchase_variance_revisions,purchase_variance_files,purchase_plan_activations FROM copypaste_app;
    END IF;
END; $$;

-- Keep chronology, exact deliveries, source returns and cancellation guards.
CREATE OR REPLACE FUNCTION validate_purchase_execution() RETURNS trigger LANGUAGE plpgsql AS $$
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
        WHEN EXISTS(SELECT 1 FROM purchase_receipts WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'receiving' WHEN EXISTS(SELECT 1 FROM purchase_orders WHERE org_id=d.org_id AND purchase_id=d.id) THEN 'ordering' ELSE 'requested' END;
    IF d.lifecycle<>expected THEN RAISE EXCEPTION 'purchase lifecycle must reflect its execution facts' USING ERRCODE='23514'; END IF;
    IF EXISTS(SELECT 1 FROM purchase_execution_events x LEFT JOIN purchase_execution_events prev ON prev.org_id=x.org_id AND prev.purchase_id=x.purchase_id AND prev.seq=x.seq-1
        WHERE x.org_id=d.org_id AND x.purchase_id=d.id AND (
            (x.seq>1 AND (prev.id IS NULL OR x.purchase_version<>prev.purchase_version+1))
            OR (x.kind='plan_activated' AND NOT EXISTS(SELECT 1 FROM purchase_plan_activations a WHERE a.org_id=x.org_id AND a.purchase_id=x.purchase_id AND a.purchase_version=x.purchase_version AND a.variance_revision_id=x.variance_revision_id))
            OR (x.kind='order_registered' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version=x.purchase_version))
            OR (x.kind='receipt_registered' AND NOT EXISTS(SELECT 1 FROM purchase_receipts a WHERE a.org_id=x.org_id AND a.id=x.receipt_id AND a.order_id=x.order_id AND a.purchase_version=x.purchase_version))
            OR (x.kind='order_cancelled' AND NOT EXISTS(SELECT 1 FROM purchase_orders o WHERE o.org_id=x.org_id AND o.id=x.order_id AND o.purchase_version<x.purchase_version))
            OR x.lifecycle<>CASE WHEN EXISTS(SELECT 1 FROM purchase_execution_events cancelled WHERE cancelled.org_id=x.org_id AND cancelled.purchase_id=x.purchase_id AND cancelled.kind='purchase_cancelled' AND cancelled.purchase_version<=x.purchase_version) THEN 'cancelled'
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
    RETURN NULL;
END; $$;

-- Preserve subscription and historical cost invariants while extending purchase caps.
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
            SELECT v.currency,CASE WHEN d.kind='purchase' THEN (SELECT amount FROM purchase_effective_plan(d.org_id,d.id)) ELSE v.amount END,d.classification,d.project_id INTO source_currency,source_amount,source_class,source_project FROM spend_documents d JOIN spend_revisions v ON v.org_id=d.org_id AND v.id=link.revision_id AND v.document_id=d.id
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
