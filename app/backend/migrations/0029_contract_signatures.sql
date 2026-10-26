ALTER TABLE file_version_references DROP CONSTRAINT file_version_references_owner_type_check;
ALTER TABLE file_version_references ADD CONSTRAINT file_version_references_owner_type_check CHECK(owner_type IN
    ('knowledge_revision','comment','approval_snapshot','contract_revision','contract_signature','purchase_receipt','claim_revision','deliverable','recovery_set'));
CREATE TABLE contract_signatures (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, contract_id uuid NOT NULL, revision_id uuid NOT NULL,
    request_id uuid NOT NULL, request_revision_id uuid NOT NULL, instance_id uuid NOT NULL,
    signed_on date NOT NULL, effective_on date NOT NULL, signed_file_version_id uuid NOT NULL,
    attestation text NOT NULL CHECK(length(trim(attestation)) BETWEEN 1 AND 20000),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    actor_id uuid NOT NULL, session_id uuid NOT NULL, mfa_verified_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,contract_id,revision_id), UNIQUE(org_id,contract_id,revision_id,id),
    FOREIGN KEY(org_id,contract_id,revision_id) REFERENCES contract_revisions(org_id,contract_id,id),
    FOREIGN KEY(org_id,request_id,request_revision_id,instance_id) REFERENCES approval_instances(org_id,request_id,revision_id,id),
    FOREIGN KEY(org_id,signed_file_version_id) REFERENCES file_versions(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TRIGGER contract_signature_immutable BEFORE UPDATE OR DELETE ON contract_signatures FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_contract_signature() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.signed_on>(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date OR NOT EXISTS(
        SELECT 1 FROM contracts c JOIN requests q ON q.org_id=c.org_id AND q.id=c.request_id
        JOIN request_revisions r ON r.org_id=q.org_id AND r.id=q.current_revision_id
        JOIN contract_revisions cr ON cr.org_id=c.org_id AND cr.contract_id=c.id AND cr.id=NEW.revision_id
        JOIN approval_instances i ON i.org_id=q.org_id AND i.id=q.active_instance_id
        WHERE c.org_id=NEW.org_id AND c.id=NEW.contract_id AND c.deleted_at IS NULL
        AND c.lifecycle='pending_signature' AND c.approval_state='approved' AND q.state='approved' AND i.state='approved'
        AND NEW.effective_on=cr.start_date AND c.current_revision_id=NEW.revision_id AND c.approved_revision_id=NEW.revision_id
        AND r.business_revision_id=NEW.revision_id AND q.id=NEW.request_id AND r.id=NEW.request_revision_id AND i.id=NEW.instance_id
    ) THEN RAISE EXCEPTION 'signature requires exact approved contract and actual signature date' USING ERRCODE='23514'; END IF;
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,file_id,id,'contract_signature',NEW.id FROM file_versions
        WHERE org_id=NEW.org_id AND id=NEW.signed_file_version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_signature_validate BEFORE INSERT ON contract_signatures FOR EACH ROW EXECUTE FUNCTION validate_contract_signature();

CREATE TABLE contract_lifecycle_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, contract_id uuid NOT NULL, revision_id uuid NOT NULL, signature_id uuid NOT NULL,
    from_state text NOT NULL CHECK(from_state='pending_signature'), to_state text NOT NULL CHECK(to_state='effective'),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200), actor_id uuid NOT NULL, session_id uuid NOT NULL,
    mfa_verified_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,contract_id,revision_id,to_state),
    FOREIGN KEY(org_id,contract_id,revision_id,signature_id) REFERENCES contract_signatures(org_id,contract_id,revision_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id)
);
CREATE TRIGGER contract_lifecycle_event_immutable BEFORE UPDATE OR DELETE ON contract_lifecycle_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_contract_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.lifecycle<>'draft' AND (NEW.current_revision_id<>OLD.current_revision_id OR NEW.approved_revision_id IS DISTINCT FROM OLD.approved_revision_id
        OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at) THEN
        RAISE EXCEPTION 'approved contract content must use a separate amendment' USING ERRCODE='23514';
    END IF;
    IF OLD.active_revision_id IS NOT NULL AND NEW.active_revision_id IS DISTINCT FROM OLD.active_revision_id THEN
        RAISE EXCEPTION 'active content cannot be rewritten' USING ERRCODE='23514';
    END IF;
    IF NEW.lifecycle<>OLD.lifecycle AND NOT (
        (OLD.lifecycle='draft' AND NEW.lifecycle='pending_signature') OR
        (OLD.lifecycle='pending_signature' AND NEW.lifecycle='effective')
    ) THEN RAISE EXCEPTION 'contract transition is not implemented' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER contract_lifecycle_guard BEFORE UPDATE ON contracts FOR EACH ROW EXECUTE FUNCTION preserve_contract_lifecycle();
CREATE FUNCTION contract_activation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts%ROWTYPE;
BEGIN
    SELECT * INTO c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.id;
    IF c.lifecycle='draft' AND (c.approved_revision_id IS NOT NULL OR c.active_revision_id IS NOT NULL) THEN
        RAISE EXCEPTION 'draft cannot claim approved or active content' USING ERRCODE='23514';
    END IF;
    IF c.lifecycle<>'draft' AND (c.approval_state<>'approved' OR c.approved_revision_id IS DISTINCT FROM c.current_revision_id) THEN
        RAISE EXCEPTION 'approved contract revision is required' USING ERRCODE='23514';
    END IF;
    IF c.lifecycle='pending_signature' AND c.active_revision_id IS NOT NULL THEN
        RAISE EXCEPTION 'pending signature has no active content' USING ERRCODE='23514';
    END IF;
    IF c.lifecycle='effective' AND c.parent_contract_id IS NOT NULL AND NOT EXISTS(
        SELECT 1 FROM contracts p WHERE p.org_id=c.org_id AND p.id=c.parent_contract_id AND p.lifecycle='effective' AND p.deleted_at IS NULL
    ) THEN RAISE EXCEPTION 'supplement requires effective parent' USING ERRCODE='23514'; END IF;
    IF c.lifecycle='effective' AND NOT EXISTS(SELECT 1 FROM contract_signatures s
        JOIN contract_revisions r ON r.org_id=s.org_id AND r.contract_id=s.contract_id AND r.id=s.revision_id
        JOIN contract_lifecycle_events e ON e.org_id=s.org_id AND e.contract_id=s.contract_id AND e.revision_id=s.revision_id AND e.signature_id=s.id AND e.to_state='effective'
        WHERE s.org_id=c.org_id AND s.contract_id=c.id AND s.revision_id=c.current_revision_id
        AND c.active_revision_id=s.revision_id AND r.start_date<=(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date
        AND s.signed_on<=(clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date) THEN
        RAISE EXCEPTION 'activation requires signature event and reached effective date' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER contract_activation_guard AFTER INSERT OR UPDATE ON contracts DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION contract_activation_evidence();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'contract_operator','合同签署与履行登记人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
    (VALUES('contract.read'),('contract.execute')) a(action) WHERE r.key='contract_operator';
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON contract_signatures,contract_lifecycle_events TO copypaste_app;
        REVOKE UPDATE,DELETE ON contract_signatures,contract_lifecycle_events FROM copypaste_app;
    END IF;
END; $$;
