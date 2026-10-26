CREATE SEQUENCE contract_number_seq;
CREATE TABLE contracts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), contract_no text NOT NULL,
    project_id uuid, creator_id uuid NOT NULL, parent_contract_id uuid,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    lifecycle text NOT NULL DEFAULT 'draft' CHECK(lifecycle IN ('draft','pending_signature','effective','completed','terminated')),
    approval_state text NOT NULL DEFAULT 'not_submitted' CHECK(approval_state IN ('not_submitted','in_review','approved','returned','rejected','withdrawn')),
    current_revision_id uuid NOT NULL, approved_revision_id uuid, active_revision_id uuid, request_id uuid,
    deleted_at timestamptz, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,contract_no),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,parent_contract_id) REFERENCES contracts(org_id,id),
    CHECK(parent_contract_id IS NULL OR parent_contract_id<>id),
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE TABLE contract_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, contract_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0),
    base_revision_id uuid, payload_schema_version integer NOT NULL CHECK(payload_schema_version=1),
    content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    direction text CHECK(direction IN ('receivable','payable','none')), type text CHECK(length(trim(type)) BETWEEN 1 AND 200),
    category text NOT NULL CHECK(length(category)<=50),
    counterparty_id uuid, counterparty_revision_id uuid, owner_id uuid,
    currency text REFERENCES currency_definitions(code), amount numeric(20,6) CHECK(amount>=0), tax_amount numeric(20,6) CHECK(tax_amount>=0),
    start_date date, end_date date, renewal_terms text NOT NULL CHECK(length(renewal_terms)<=20000),
    termination_terms text NOT NULL CHECK(length(termination_terms)<=20000), notes text NOT NULL CHECK(length(notes)<=20000),
    draft_file_version_id uuid, author_id uuid NOT NULL,
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,contract_id,id), UNIQUE(contract_id,seq),
    FOREIGN KEY(org_id,contract_id) REFERENCES contracts(org_id,id),
    FOREIGN KEY(org_id,contract_id,base_revision_id) REFERENCES contract_revisions(org_id,contract_id,id),
    FOREIGN KEY(org_id,counterparty_id,counterparty_revision_id) REFERENCES party_revisions(org_id,party_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,draft_file_version_id) REFERENCES file_versions(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)), CHECK((counterparty_id IS NULL)=(counterparty_revision_id IS NULL)),
    CHECK((amount IS NULL AND tax_amount IS NULL) OR currency IS NOT NULL),
    CHECK(tax_amount IS NULL OR amount IS NULL OR tax_amount<=amount),
    CHECK(start_date IS NULL OR end_date IS NULL OR start_date<=end_date)
);
ALTER TABLE contracts ADD FOREIGN KEY(org_id,id,current_revision_id)
    REFERENCES contract_revisions(org_id,contract_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE contracts ADD FOREIGN KEY(org_id,id,approved_revision_id)
    REFERENCES contract_revisions(org_id,contract_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE contracts ADD FOREIGN KEY(org_id,id,active_revision_id)
    REFERENCES contract_revisions(org_id,contract_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX contracts_list ON contracts(org_id,updated_at DESC,id DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER contract_revision_immutable BEFORE UPDATE OR DELETE ON contract_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_contract_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id<>OLD.id OR NEW.org_id<>OLD.org_id OR NEW.contract_no<>OLD.contract_no OR NEW.creator_id<>OLD.creator_id
        OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.parent_contract_id IS DISTINCT FROM OLD.parent_contract_id
        OR NEW.classification<>OLD.classification OR NEW.created_at<>OLD.created_at THEN
        RAISE EXCEPTION 'contract identity and scope require a separate process' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER contract_scope_immutable BEFORE UPDATE ON contracts FOR EACH ROW EXECUTE FUNCTION preserve_contract_scope();
CREATE FUNCTION register_contract_file_reference() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.draft_file_version_id IS NOT NULL THEN
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
            SELECT gen_random_uuid(),NEW.org_id,v.file_id,v.id,'contract_revision',NEW.id FROM file_versions v
            WHERE v.org_id=NEW.org_id AND v.id=NEW.draft_file_version_id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER contract_file_reference AFTER INSERT ON contract_revisions FOR EACH ROW EXECUTE FUNCTION register_contract_file_reference();
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'contract_manager','合同管理人',false FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
        (VALUES('contract.read'),('contract.create'),('contract.update'),('contract.archive')) a(action)
    WHERE r.key='contract_manager' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON contracts TO copypaste_app;
        GRANT SELECT,INSERT ON contract_revisions TO copypaste_app;
        GRANT USAGE,SELECT ON contract_number_seq TO copypaste_app;
        REVOKE UPDATE,DELETE ON contract_revisions FROM copypaste_app;
        REVOKE DELETE ON contracts FROM copypaste_app;
    END IF;
END;
$$;
