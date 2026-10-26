CREATE TABLE requests (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    kind text NOT NULL CHECK(kind IN ('contract','purchase','claim','change')), business_id uuid NOT NULL,
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200), applicant_id uuid NOT NULL,
    project_id uuid, classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    state text NOT NULL CHECK(state IN ('draft','in_review','approved','returned','rejected','withdrawn')),
    current_revision_id uuid NOT NULL, active_instance_id uuid, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,id,kind,business_id),
    FOREIGN KEY(org_id,applicant_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE UNIQUE INDEX one_live_business_request ON requests(org_id,kind,business_id) WHERE state<>'rejected';
CREATE INDEX requests_list ON requests(org_id,updated_at DESC,id DESC);
CREATE TABLE request_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, request_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0),
    base_revision_id uuid, business_revision_id uuid NOT NULL, business_content_hash text NOT NULL CHECK(business_content_hash~'^[0-9a-f]{64}$'),
    payload_schema_version integer NOT NULL CHECK(payload_schema_version=1), payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
    workflow_definition_id uuid, workflow_snapshot jsonb, reviewer_evidence jsonb NOT NULL CHECK(jsonb_typeof(reviewer_evidence)='array'), content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'),
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,request_id,id), UNIQUE(org_id,request_id,seq),
    FOREIGN KEY(org_id,request_id) REFERENCES requests(org_id,id),
    FOREIGN KEY(org_id,request_id,base_revision_id) REFERENCES request_revisions(org_id,request_id,id),
    FOREIGN KEY(org_id,workflow_definition_id) REFERENCES workflow_definitions(org_id,id), FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL)), CHECK((workflow_definition_id IS NULL)=(workflow_snapshot IS NULL))
);
ALTER TABLE requests ADD FOREIGN KEY(org_id,id,current_revision_id) REFERENCES request_revisions(org_id,request_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER request_revision_immutable BEFORE UPDATE OR DELETE ON request_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE approval_instances (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, request_id uuid NOT NULL, revision_id uuid NOT NULL, definition_id uuid NOT NULL,
    state text NOT NULL CHECK(state IN ('in_review','approved','returned','rejected','withdrawn')),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,request_id,id), UNIQUE(org_id,request_id,revision_id,id), UNIQUE(org_id,request_id,revision_id),
    FOREIGN KEY(org_id,request_id,revision_id) REFERENCES request_revisions(org_id,request_id,id),
    FOREIGN KEY(org_id,definition_id) REFERENCES workflow_definitions(org_id,id)
);
CREATE UNIQUE INDEX one_active_approval ON approval_instances(org_id,request_id) WHERE state='in_review';
ALTER TABLE requests ADD FOREIGN KEY(org_id,id,active_instance_id) REFERENCES approval_instances(org_id,request_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE approval_steps (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, request_id uuid NOT NULL, revision_id uuid NOT NULL, instance_id uuid NOT NULL,
    step_no integer NOT NULL CHECK(step_no BETWEEN 1 AND 10), title text NOT NULL, assigned_user_id uuid NOT NULL,
    assigned_user_name text NOT NULL, requires_mfa boolean NOT NULL, authority_evidence jsonb NOT NULL CHECK(jsonb_typeof(authority_evidence)='object'),
    state text NOT NULL CHECK(state IN ('pending','active','approved','returned','rejected','cancelled')),
    version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,instance_id,step_no), UNIQUE(org_id,id,request_id,revision_id),
    FOREIGN KEY(org_id,request_id,revision_id,instance_id) REFERENCES approval_instances(org_id,request_id,revision_id,id),
    FOREIGN KEY(org_id,assigned_user_id) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX one_active_approval_step ON approval_steps(org_id,instance_id) WHERE state='active';
CREATE INDEX approval_tasks ON approval_steps(org_id,assigned_user_id,state,updated_at DESC,id DESC);
CREATE TABLE approval_decisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, step_id uuid NOT NULL, request_id uuid NOT NULL, revision_id uuid NOT NULL,
    content_hash text NOT NULL CHECK(content_hash~'^[0-9a-f]{64}$'), actor_id uuid NOT NULL, session_id uuid NOT NULL,
    decision text NOT NULL CHECK(decision IN ('approve','return','reject')), reason text NOT NULL,
    mfa_verified_at timestamptz, authority_evidence jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,step_id), FOREIGN KEY(org_id,step_id,request_id,revision_id) REFERENCES approval_steps(org_id,id,request_id,revision_id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    CHECK(decision='approve' OR length(trim(reason))>0)
);
CREATE TRIGGER approval_decision_immutable BEFORE UPDATE OR DELETE ON approval_decisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE approval_withdrawals (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, request_id uuid NOT NULL, revision_id uuid NOT NULL, instance_id uuid NOT NULL,
    actor_id uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,instance_id),
    FOREIGN KEY(org_id,request_id,revision_id,instance_id) REFERENCES approval_instances(org_id,request_id,revision_id,id), FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TRIGGER approval_withdrawal_immutable BEFORE UPDATE OR DELETE ON approval_withdrawals FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE contracts ADD COLUMN request_kind text GENERATED ALWAYS AS ('contract'::text) STORED;
ALTER TABLE contracts ADD FOREIGN KEY(org_id,request_id,request_kind,id) REFERENCES requests(org_id,id,kind,business_id) DEFERRABLE INITIALLY DEFERRED;
CREATE FUNCTION contract_request_consistent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE contract_id uuid; contract_org uuid; c contracts; r requests;
BEGIN
    IF TG_TABLE_NAME='contracts' THEN contract_id:=NEW.id;
    ELSE
        IF NEW.kind<>'contract' THEN RETURN NULL; END IF;
        contract_id:=NEW.business_id;
    END IF;
    contract_org:=NEW.org_id;
    SELECT * INTO c FROM contracts WHERE org_id=contract_org AND id=contract_id;
    IF c.request_id IS NULL THEN
        IF c.approval_state<>'not_submitted' THEN RAISE EXCEPTION 'contract approval requires its exact request' USING ERRCODE='23514'; END IF;
    ELSE
        SELECT * INTO r FROM requests WHERE org_id=contract_org AND id=c.request_id;
        IF NOT FOUND OR r.business_id<>c.id OR r.kind<>'contract' OR r.project_id IS DISTINCT FROM c.project_id OR r.classification<>c.classification
            OR (CASE WHEN r.state='draft' THEN 'not_submitted' ELSE r.state END)<>c.approval_state THEN
            RAISE EXCEPTION 'contract and current request state or scope disagree' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER contract_request_consistency AFTER INSERT OR UPDATE ON contracts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION contract_request_consistent();
CREATE CONSTRAINT TRIGGER request_contract_consistency AFTER INSERT OR UPDATE ON requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION contract_request_consistent();
CREATE FUNCTION preserve_request_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.kind,NEW.business_id,NEW.applicant_id,NEW.project_id,NEW.classification,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.kind,OLD.business_id,OLD.applicant_id,OLD.project_id,OLD.classification,OLD.created_at) THEN
        RAISE EXCEPTION 'request identity and scope are immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.version<>OLD.version+1 OR NOT (
        (OLD.state='draft' AND NEW.state IN ('draft','in_review')) OR
        (OLD.state IN ('returned','withdrawn') AND NEW.state IN ('draft','in_review')) OR
        (OLD.state='in_review' AND NEW.state IN ('in_review','approved','returned','rejected','withdrawn'))) OR
        (OLD.state='in_review' AND NEW.current_revision_id<>OLD.current_revision_id) THEN
        RAISE EXCEPTION 'request terminal state or snapshot cannot be rewritten' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER request_scope_immutable BEFORE UPDATE ON requests FOR EACH ROW EXECUTE FUNCTION preserve_request_scope();
CREATE FUNCTION preserve_approval_step() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.request_id,NEW.revision_id,NEW.instance_id,NEW.step_no,NEW.title,NEW.assigned_user_id,
        NEW.assigned_user_name,NEW.requires_mfa,NEW.authority_evidence,NEW.created_at) IS DISTINCT FROM
        ROW(OLD.id,OLD.org_id,OLD.request_id,OLD.revision_id,OLD.instance_id,OLD.step_no,OLD.title,OLD.assigned_user_id,
        OLD.assigned_user_name,OLD.requires_mfa,OLD.authority_evidence,OLD.created_at)
        OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='pending' AND NEW.state IN ('active','cancelled'))
            OR (OLD.state='active' AND NEW.state IN ('approved','returned','rejected','cancelled'))) THEN
        RAISE EXCEPTION 'approval step snapshot or transition is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.state='active' AND EXISTS(SELECT 1 FROM approval_steps s WHERE s.org_id=NEW.org_id AND s.instance_id=NEW.instance_id AND s.step_no<NEW.step_no AND s.state<>'approved') THEN
        RAISE EXCEPTION 'approval must follow the fixed step order' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER approval_step_fixed BEFORE UPDATE ON approval_steps FOR EACH ROW EXECUTE FUNCTION preserve_approval_step();
CREATE FUNCTION preserve_approval_instance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.request_id,NEW.revision_id,NEW.definition_id,NEW.created_at) IS DISTINCT FROM
       ROW(OLD.id,OLD.org_id,OLD.request_id,OLD.revision_id,OLD.definition_id,OLD.created_at)
       OR OLD.state<>'in_review' OR NEW.state NOT IN ('approved','returned','rejected','withdrawn') THEN
        RAISE EXCEPTION 'approval instance snapshot or terminal state is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER approval_instance_fixed BEFORE UPDATE ON approval_instances FOR EACH ROW EXECUTE FUNCTION preserve_approval_instance();
CREATE FUNCTION validate_approval_step_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS(SELECT 1 FROM approval_instances i JOIN request_revisions r ON r.org_id=i.org_id AND r.request_id=i.request_id AND r.id=i.revision_id
        JOIN workflow_definition_steps d ON d.org_id=i.org_id AND d.definition_id=i.definition_id AND d.step_no=NEW.step_no
        WHERE i.org_id=NEW.org_id AND i.id=NEW.instance_id AND i.state='in_review' AND r.workflow_definition_id=i.definition_id
        AND d.assigned_user_id=NEW.assigned_user_id AND d.title=NEW.title AND d.requires_mfa=NEW.requires_mfa) THEN
        RAISE EXCEPTION 'approval step must match the fixed policy and submission' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER approval_step_source BEFORE INSERT ON approval_steps FOR EACH ROW EXECUTE FUNCTION validate_approval_step_source();
CREATE FUNCTION approval_instance_complete() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_state text;
BEGIN
    IF (SELECT count(*) FROM approval_steps WHERE org_id=NEW.org_id AND instance_id=NEW.id)<>
       (SELECT count(*) FROM workflow_definition_steps WHERE org_id=NEW.org_id AND definition_id=NEW.definition_id) THEN
        RAISE EXCEPTION 'approval instance requires all fixed steps' USING ERRCODE='23514';
    END IF;
    SELECT state INTO current_state FROM approval_instances WHERE org_id=NEW.org_id AND id=NEW.id;
    IF (current_state='approved' AND EXISTS(SELECT 1 FROM approval_steps WHERE org_id=NEW.org_id AND instance_id=NEW.id AND state<>'approved'))
        OR (current_state='in_review' AND (SELECT count(*) FROM approval_steps WHERE org_id=NEW.org_id AND instance_id=NEW.id AND state='active')<>1)
        OR (current_state<>'in_review' AND EXISTS(SELECT 1 FROM approval_steps WHERE org_id=NEW.org_id AND instance_id=NEW.id AND state IN ('pending','active'))) THEN
        RAISE EXCEPTION 'approval instance and normalized step states disagree' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER approval_instance_complete_at_commit AFTER INSERT ON approval_instances DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION approval_instance_complete();
CREATE CONSTRAINT TRIGGER approval_instance_terminal_at_commit AFTER UPDATE ON approval_instances DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION approval_instance_complete();
CREATE FUNCTION register_request_snapshot_files() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE attachment jsonb; request_kind text; business uuid;
BEGIN
    SELECT kind,business_id INTO request_kind,business FROM requests WHERE org_id=NEW.org_id AND id=NEW.request_id;
    IF request_kind='contract' AND NOT EXISTS(SELECT 1 FROM contract_revisions WHERE org_id=NEW.org_id AND contract_id=business
        AND id=NEW.business_revision_id AND content_hash=NEW.business_content_hash) THEN
        RAISE EXCEPTION 'request revision must fix the exact business revision' USING ERRCODE='23514';
    END IF;
    FOR attachment IN SELECT value FROM jsonb_array_elements(NEW.payload->'attachments') LOOP
        INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        VALUES(gen_random_uuid(),NEW.org_id,(attachment->>'file_id')::uuid,(attachment->>'version_id')::uuid,'approval_snapshot',NEW.id);
    END LOOP;
    RETURN NEW;
END;
$$;
CREATE TRIGGER request_snapshot_file_references AFTER INSERT ON request_revisions FOR EACH ROW EXECUTE FUNCTION register_request_snapshot_files();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'request_manager','申请管理人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'approval_reviewer','审批处理人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'contract_reader','合同查阅人',false FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
    (VALUES('request.read'),('request.create'),('request.update'),('request.submit'),('request.withdraw')) a(action) WHERE r.key='request_manager';
INSERT INTO role_actions(org_id,role_id,action) SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
    (VALUES('request.read'),('approval.decide')) a(action) WHERE r.key='approval_reviewer';
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'contract.read' FROM roles WHERE key='contract_reader';
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON requests,approval_instances,approval_steps TO copypaste_app;
        GRANT SELECT,INSERT ON request_revisions,approval_decisions,approval_withdrawals TO copypaste_app;
        REVOKE DELETE ON requests,approval_instances,approval_steps FROM copypaste_app;
        REVOKE UPDATE,DELETE ON request_revisions,approval_decisions,approval_withdrawals FROM copypaste_app;
    END IF;
END; $$;
