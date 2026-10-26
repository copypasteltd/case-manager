-- Purchase/claim preparation is independent of ordering, canonical costs and cash.
CREATE SEQUENCE spend_number_seq;
CREATE TABLE spend_documents (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    kind text NOT NULL CHECK(kind IN ('purchase','claim')), document_no text NOT NULL,
    project_id uuid, creator_id uuid NOT NULL,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    lifecycle text NOT NULL DEFAULT 'draft' CHECK(lifecycle IN ('draft','requested')),
    approval_state text NOT NULL DEFAULT 'not_submitted' CHECK(approval_state IN ('not_submitted','in_review','approved','returned','rejected','withdrawn')),
    current_revision_id uuid NOT NULL, approved_revision_id uuid, request_id uuid, deleted_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,document_no),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,request_id,kind,id) REFERENCES requests(org_id,id,kind,business_id) DEFERRABLE INITIALLY DEFERRED,
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL),
    CHECK((approval_state IN ('in_review','approved'))=(lifecycle='requested')),
    CHECK((approval_state='approved')=(approved_revision_id IS NOT NULL)),
    CHECK(approved_revision_id IS NULL OR approved_revision_id=current_revision_id),
    CHECK(deleted_at IS NULL OR (request_id IS NULL AND lifecycle='draft' AND approval_state='not_submitted'))
);
CREATE TABLE spend_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, document_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0), base_revision_id uuid,
    payload_schema_version integer NOT NULL CHECK(payload_schema_version=1), payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
    title text GENERATED ALWAYS AS (payload->>'title') STORED NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
    category text GENERATED ALWAYS AS (payload->>'category') STORED NOT NULL CHECK(length(category)<=50),
    currency text GENERATED ALWAYS AS (payload->>'currency') STORED REFERENCES currency_definitions(code),
    owner_id uuid GENERATED ALWAYS AS ((payload->>'owner_id')::uuid) STORED,
    content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    amount numeric(20,6) CHECK(amount>=0), employee_reimbursement_amount numeric(20,6) CHECK(employee_reimbursement_amount>=0),
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,document_id,id), UNIQUE(org_id,document_id,seq),
    FOREIGN KEY(org_id,document_id) REFERENCES spend_documents(org_id,id),
    FOREIGN KEY(org_id,document_id,base_revision_id) REFERENCES spend_revisions(org_id,document_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)), CHECK(amount IS NULL OR currency IS NOT NULL),
    CHECK(employee_reimbursement_amount IS NULL OR employee_reimbursement_amount<=amount),
    CHECK(jsonb_typeof(payload->'lines')='array' AND jsonb_array_length(payload->'lines')<=100),
    CHECK(jsonb_typeof(payload->'supplier_candidates')='array' AND jsonb_array_length(payload->'supplier_candidates')<=20),
    CHECK(jsonb_typeof(payload->'quote_version_ids')='array' AND jsonb_array_length(payload->'quote_version_ids')<=20)
);
CREATE TABLE spend_lines (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, position integer NOT NULL CHECK(position BETWEEN 1 AND 100), line_id uuid NOT NULL,
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), amount numeric(20,6) CHECK(amount>=0),
    PRIMARY KEY(org_id,revision_id,position), UNIQUE(org_id,revision_id,line_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES spend_revisions(org_id,id), CHECK((payload->>'id')::uuid=line_id)
);
CREATE TABLE spend_revision_suppliers (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, party_id uuid NOT NULL, party_revision_id uuid NOT NULL,
    PRIMARY KEY(org_id,revision_id,party_id,party_revision_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES spend_revisions(org_id,id),
    FOREIGN KEY(org_id,party_id,party_revision_id) REFERENCES party_revisions(org_id,party_id,id)
);
CREATE TABLE spend_revision_files (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, version_id uuid NOT NULL,
    PRIMARY KEY(org_id,revision_id,version_id),
    FOREIGN KEY(org_id,revision_id) REFERENCES spend_revisions(org_id,id), FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
ALTER TABLE spend_documents ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES spend_revisions(org_id,document_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE spend_documents ADD FOREIGN KEY(org_id,id,approved_revision_id) REFERENCES spend_revisions(org_id,document_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX spend_list ON spend_documents(org_id,kind,updated_at DESC,id DESC) WHERE deleted_at IS NULL;
CREATE INDEX spend_receipt_lookup ON spend_revision_files(org_id,version_id,revision_id);
CREATE TRIGGER spend_revision_immutable BEFORE UPDATE OR DELETE ON spend_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER spend_line_immutable BEFORE UPDATE OR DELETE ON spend_lines FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER spend_supplier_immutable BEFORE UPDATE OR DELETE ON spend_revision_suppliers FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER spend_file_immutable BEFORE UPDATE OR DELETE ON spend_revision_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_spend_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.kind,NEW.document_no,NEW.project_id,NEW.creator_id,NEW.classification,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.kind,OLD.document_no,OLD.project_id,OLD.creator_id,OLD.classification,OLD.created_at)
        OR NEW.version<>OLD.version+1 THEN
        RAISE EXCEPTION 'spend identity is immutable and every mutation advances its version' USING ERRCODE='23514';
    END IF;
    IF OLD.approval_state='approved' OR (OLD.approval_state='in_review' AND NEW.current_revision_id<>OLD.current_revision_id) THEN
        RAISE EXCEPTION 'approved or reviewing commercial content cannot be rewritten' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER spend_identity_immutable BEFORE UPDATE ON spend_documents FOR EACH ROW EXECUTE FUNCTION preserve_spend_identity();
-- Independent numeric recomputation catches mistakes in a future caller as well
-- as accidental divergence between JSON content and normalized line values.
CREATE FUNCTION spend_expected_line_amount(kind text,c jsonb,l jsonb,exponent integer) RETURNS numeric LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE value numeric; product numeric; factor numeric:=power(10::numeric,exponent); integral numeric; fraction numeric;
    tax numeric:=COALESCE((l->>'tax_amount')::numeric,0); discount numeric:=COALESCE((l->>'discount_amount')::numeric,0);
BEGIN
    IF kind='purchase' THEN
        IF l->>'quantity' IS NULL OR l->>'unit_price' IS NULL OR c->>'currency' IS NULL OR c->>'pricing_basis'='' OR c->>'rounding_mode'='' THEN RETURN NULL; END IF;
        product:=(l->>'quantity')::numeric*(l->>'unit_price')::numeric;
        integral:=floor(product*factor); fraction:=product*factor-integral;
        IF fraction>0.5 OR (fraction=0.5 AND (c->>'rounding_mode'='half_up' OR mod(integral,2)<>0)) THEN integral:=integral+1; END IF;
        value:=integral/factor;
        IF c->>'pricing_basis'='net_plus_tax' THEN value:=value+tax; END IF;
        value:=value-discount;
    ELSE
        value:=(l->>'declared_amount')::numeric;
    END IF;
    IF value<0 OR tax<0 OR discount<0 OR tax>value OR tax*factor<>trunc(tax*factor) OR discount*factor<>trunc(discount*factor)
        OR value*factor<>trunc(value*factor) THEN
        RAISE EXCEPTION 'commercial line amount or currency precision is invalid' USING ERRCODE='23514';
    END IF;
    RETURN value;
END; $$;
CREATE FUNCTION validate_spend_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; r spend_revisions; d spend_documents; actual jsonb; total bigint; latest uuid; computed numeric; employee numeric; exponent integer;
BEGIN
    IF TG_TABLE_NAME='spend_documents' THEN target:=NEW.current_revision_id;
    ELSIF TG_TABLE_NAME='spend_revisions' THEN target:=NEW.id;
    ELSE target:=NEW.revision_id; END IF;
    SELECT * INTO r FROM spend_revisions WHERE org_id=NEW.org_id AND id=target;
    SELECT * INTO d FROM spend_documents WHERE org_id=r.org_id AND id=r.document_id;
    SELECT cd.exponent INTO exponent FROM currency_definitions cd WHERE cd.code=r.currency;
    IF EXISTS(SELECT 1 FROM spend_lines l WHERE l.org_id=r.org_id AND l.revision_id=r.id
        AND l.amount IS DISTINCT FROM spend_expected_line_amount(d.kind,r.payload,l.payload,COALESCE(exponent,0))) THEN
        RAISE EXCEPTION 'commercial line values must equal their exact numeric inputs' USING ERRCODE='23514';
    END IF;
    SELECT jsonb_agg(payload ORDER BY position),count(*),
        CASE WHEN count(*)>0 AND count(amount)=count(*) THEN sum(amount) END,
        CASE WHEN count(*)>0 AND count(amount)=count(*) AND bool_and(payload->>'funding' IN ('employee_advance','company_paid'))
            THEN sum(CASE WHEN payload->>'funding'='employee_advance' THEN amount ELSE 0 END) END
        INTO actual,total,computed,employee FROM spend_lines WHERE org_id=r.org_id AND revision_id=r.id;
    IF COALESCE(actual,'[]'::jsonb) IS DISTINCT FROM r.payload->'lines' OR total<>jsonb_array_length(r.payload->'lines')
        OR r.amount IS DISTINCT FROM computed OR r.employee_reimbursement_amount IS DISTINCT FROM (CASE WHEN d.kind='claim' THEN employee END) THEN
        RAISE EXCEPTION 'spend revision requires its exact lines and derived totals' USING ERRCODE='23514';
    END IF;
    IF r.seq>1 AND NOT EXISTS(SELECT 1 FROM spend_revisions WHERE org_id=r.org_id AND document_id=r.document_id AND id=r.base_revision_id AND seq=r.seq-1) THEN
        RAISE EXCEPTION 'spend revision base must be the preceding revision' USING ERRCODE='23514';
    END IF;
    SELECT id INTO latest FROM spend_revisions WHERE org_id=d.org_id AND document_id=d.id ORDER BY seq DESC LIMIT 1;
    SELECT count(*) INTO total FROM spend_revisions WHERE org_id=d.org_id AND document_id=d.id;
    IF latest IS DISTINCT FROM d.current_revision_id OR total<>(SELECT seq FROM spend_revisions WHERE id=latest) THEN
        RAISE EXCEPTION 'spend head requires its complete revision chain' USING ERRCODE='23514';
    END IF;
    IF EXISTS(
        (SELECT (s->>'party_id')::uuid,(s->>'revision_id')::uuid FROM jsonb_array_elements(r.payload->'supplier_candidates') s
         UNION SELECT (l->'supplier'->>'party_id')::uuid,(l->'supplier'->>'revision_id')::uuid FROM jsonb_array_elements(r.payload->'lines') l WHERE l->'supplier'<>'null'::jsonb)
        EXCEPT SELECT party_id,party_revision_id FROM spend_revision_suppliers WHERE org_id=r.org_id AND revision_id=r.id)
        OR EXISTS(SELECT party_id,party_revision_id FROM spend_revision_suppliers WHERE org_id=r.org_id AND revision_id=r.id
        EXCEPT (SELECT (s->>'party_id')::uuid,(s->>'revision_id')::uuid FROM jsonb_array_elements(r.payload->'supplier_candidates') s
         UNION SELECT (l->'supplier'->>'party_id')::uuid,(l->'supplier'->>'revision_id')::uuid FROM jsonb_array_elements(r.payload->'lines') l WHERE l->'supplier'<>'null'::jsonb)) THEN
        RAISE EXCEPTION 'spend supplier pins must exactly match content' USING ERRCODE='23514';
    END IF;
    IF EXISTS((SELECT v::uuid FROM jsonb_array_elements_text(r.payload->'quote_version_ids') v
        UNION SELECT v::uuid FROM jsonb_array_elements(r.payload->'lines') l CROSS JOIN LATERAL jsonb_array_elements_text(l->'receipt_version_ids') v)
        EXCEPT SELECT version_id FROM spend_revision_files WHERE org_id=r.org_id AND revision_id=r.id)
        OR EXISTS(SELECT version_id FROM spend_revision_files WHERE org_id=r.org_id AND revision_id=r.id
        EXCEPT (SELECT v::uuid FROM jsonb_array_elements_text(r.payload->'quote_version_ids') v
        UNION SELECT v::uuid FROM jsonb_array_elements(r.payload->'lines') l CROSS JOIN LATERAL jsonb_array_elements_text(l->'receipt_version_ids') v)) THEN
        RAISE EXCEPTION 'spend file pins must exactly match content' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER spend_head_consistent AFTER INSERT OR UPDATE ON spend_documents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_spend_revision();
CREATE CONSTRAINT TRIGGER spend_revision_consistent AFTER INSERT ON spend_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_spend_revision();
CREATE CONSTRAINT TRIGGER spend_lines_consistent AFTER INSERT ON spend_lines DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_spend_revision();
CREATE CONSTRAINT TRIGGER spend_suppliers_consistent AFTER INSERT ON spend_revision_suppliers DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_spend_revision();
CREATE CONSTRAINT TRIGGER spend_files_consistent AFTER INSERT ON spend_revision_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_spend_revision();

ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CHECK(owner_type IN ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_revision','purchase_receipt','claim_revision','deliverable','recovery_set'));
CREATE FUNCTION register_spend_file_reference() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,v.file_id,v.id,d.kind||'_revision',NEW.revision_id FROM file_versions v
        JOIN spend_revisions r ON r.org_id=NEW.org_id AND r.id=NEW.revision_id
        JOIN spend_documents d ON d.org_id=r.org_id AND d.id=r.document_id WHERE v.org_id=NEW.org_id AND v.id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER spend_file_reference AFTER INSERT ON spend_revision_files FOR EACH ROW EXECUTE FUNCTION register_spend_file_reference();
CREATE FUNCTION spend_request_consistent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; d spend_documents; r requests;
BEGIN
    IF TG_TABLE_NAME='spend_documents' THEN target:=NEW.id;
    ELSE IF NEW.kind NOT IN ('purchase','claim') THEN RETURN NULL; END IF; target:=NEW.business_id; END IF;
    SELECT * INTO d FROM spend_documents WHERE org_id=NEW.org_id AND id=target;
    IF NOT FOUND THEN RAISE EXCEPTION 'spend request requires its business' USING ERRCODE='23514'; END IF;
    IF d.request_id IS NULL THEN
        IF d.approval_state<>'not_submitted' THEN RAISE EXCEPTION 'spend approval requires its request' USING ERRCODE='23514'; END IF;
    ELSE
        SELECT * INTO r FROM requests WHERE org_id=d.org_id AND id=d.request_id;
        IF NOT FOUND OR ROW(r.business_id,r.kind,r.project_id,r.classification) IS DISTINCT FROM ROW(d.id,d.kind,d.project_id,d.classification)
            OR (CASE WHEN r.state='draft' THEN 'not_submitted' ELSE r.state END)<>d.approval_state THEN
            RAISE EXCEPTION 'spend request state or scope disagree' USING ERRCODE='23514';
        END IF;
        IF d.approval_state IN ('in_review','approved') AND NOT EXISTS(SELECT 1 FROM request_revisions v
            JOIN spend_revisions b ON b.org_id=v.org_id AND b.document_id=d.id AND b.id=d.current_revision_id
            WHERE v.org_id=r.org_id AND v.id=r.current_revision_id AND v.request_id=r.id
                AND v.business_revision_id=b.id AND v.business_content_hash=b.content_hash) THEN
            RAISE EXCEPTION 'reviewing or approved spend requires the exact current business revision' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER spend_request_consistency AFTER INSERT OR UPDATE ON spend_documents DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION spend_request_consistent();
CREATE CONSTRAINT TRIGGER request_spend_consistency AFTER INSERT OR UPDATE ON requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION spend_request_consistent();
-- Existing contract snapshots and their hashes remain unchanged.
CREATE OR REPLACE FUNCTION register_request_snapshot_files() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE attachment jsonb; request_kind text; business uuid;
BEGIN
    SELECT kind,business_id INTO request_kind,business FROM requests WHERE org_id=NEW.org_id AND id=NEW.request_id;
    IF request_kind='contract' AND NOT EXISTS(SELECT 1 FROM contract_revisions WHERE org_id=NEW.org_id AND contract_id=business AND id=NEW.business_revision_id AND content_hash=NEW.business_content_hash) THEN
        RAISE EXCEPTION 'request revision must fix the exact business revision' USING ERRCODE='23514';
    ELSIF request_kind IN ('purchase','claim') AND NOT EXISTS(SELECT 1 FROM spend_revisions v JOIN spend_documents d ON d.org_id=v.org_id AND d.id=v.document_id
        WHERE v.org_id=NEW.org_id AND v.document_id=business AND d.kind=request_kind AND v.id=NEW.business_revision_id AND v.content_hash=NEW.business_content_hash AND v.payload=NEW.payload->'content'
        AND v.amount IS NOT DISTINCT FROM (NEW.payload->>'amount')::numeric
        AND v.employee_reimbursement_amount IS NOT DISTINCT FROM (NEW.payload->>'employee_reimbursement_amount')::numeric
        AND d.document_no=NEW.payload->>'document_no' AND d.classification=NEW.payload->>'classification'
        AND d.project_id IS NOT DISTINCT FROM (NEW.payload->>'project_id')::uuid) THEN
        RAISE EXCEPTION 'request revision must fix the exact spend content' USING ERRCODE='23514';
    END IF;
    IF request_kind IN ('purchase','claim') AND (
        EXISTS(SELECT version_id FROM spend_revision_files WHERE org_id=NEW.org_id AND revision_id=NEW.business_revision_id
            EXCEPT SELECT (a->>'version_id')::uuid FROM jsonb_array_elements(NEW.payload->'attachments') a)
        OR EXISTS(SELECT (a->>'version_id')::uuid FROM jsonb_array_elements(NEW.payload->'attachments') a
            EXCEPT SELECT version_id FROM spend_revision_files WHERE org_id=NEW.org_id AND revision_id=NEW.business_revision_id)
        OR (SELECT COALESCE(jsonb_agg(jsonb_build_object('line_id',l.line_id,'amount',l.amount::text,'duplicate_receipt',false) ORDER BY position),'[]'::jsonb)
            FROM spend_lines l WHERE org_id=NEW.org_id AND revision_id=NEW.business_revision_id) IS DISTINCT FROM NEW.payload->'line_values') THEN
        RAISE EXCEPTION 'spend snapshot must preserve all exact line values and file versions' USING ERRCODE='23514';
    END IF;
    FOR attachment IN SELECT value FROM jsonb_array_elements(NEW.payload->'attachments') LOOP
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        VALUES(gen_random_uuid(),NEW.org_id,(attachment->>'file_id')::uuid,(attachment->>'version_id')::uuid,'approval_snapshot',NEW.id);
    END LOOP;
    RETURN NEW;
END; $$;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),o.id,r.key,r.name,false FROM organizations o CROSS JOIN
    (VALUES('purchase_manager','采购管理人'),('purchase_reader','采购查阅人'),('claim_submitter','报销填报人'),('claim_reader','报销查阅人')) r(key,name) ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
    (VALUES('purchase_manager','purchase.read'),('purchase_manager','purchase.create'),('purchase_manager','purchase.update'),('purchase_manager','purchase.archive'),('purchase_reader','purchase.read'),
        ('claim_submitter','claim.read'),('claim_submitter','claim.create'),('claim_submitter','claim.update'),('claim_submitter','claim.archive'),('claim_reader','claim.read')) a(key,action)
    WHERE r.key=a.key ON CONFLICT DO NOTHING;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON spend_documents TO copypaste_app;
        GRANT SELECT,INSERT ON spend_revisions,spend_lines,spend_revision_suppliers,spend_revision_files TO copypaste_app;
        GRANT USAGE,SELECT ON spend_number_seq TO copypaste_app;
        REVOKE DELETE ON spend_documents FROM copypaste_app;
        REVOKE UPDATE,DELETE ON spend_revisions,spend_lines,spend_revision_suppliers,spend_revision_files FROM copypaste_app;
    END IF;
END; $$;
