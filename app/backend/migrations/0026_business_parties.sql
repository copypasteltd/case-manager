CREATE TABLE parties (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    project_id uuid, creator_id uuid NOT NULL,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    state text NOT NULL DEFAULT 'active' CHECK(state IN ('active','archived')),
    current_revision_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE TABLE party_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, party_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq>0),
    base_revision_id uuid, name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    kind text NOT NULL CHECK(kind IN ('organization','person')),
    relationships text[] NOT NULL CHECK(cardinality(relationships) BETWEEN 1 AND 3 AND relationships<@ARRAY['supplier','customer','other']),
    contact_name text NOT NULL CHECK(length(contact_name)<=200),
    email text NOT NULL CHECK(length(email)<=254), phone text NOT NULL CHECK(length(phone)<=100),
    notes text NOT NULL CHECK(length(notes)<=10000),
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,party_id,id), UNIQUE(party_id,seq),
    FOREIGN KEY(org_id,party_id) REFERENCES parties(org_id,id),
    FOREIGN KEY(org_id,party_id,base_revision_id) REFERENCES party_revisions(org_id,party_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL))
);
ALTER TABLE parties ADD FOREIGN KEY(org_id,id,current_revision_id)
    REFERENCES party_revisions(org_id,party_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX parties_list ON parties(org_id,state,updated_at DESC,id DESC);
CREATE TRIGGER party_revision_immutable BEFORE UPDATE OR DELETE ON party_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_party_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR NEW.creator_id<>OLD.creator_id
        OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.classification<>OLD.classification
        OR NEW.created_at<>OLD.created_at THEN
        RAISE EXCEPTION 'party identity and security scope require a separate migration' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER party_scope_immutable BEFORE UPDATE ON parties FOR EACH ROW EXECUTE FUNCTION preserve_party_scope();
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'party_manager','往来资料管理人',false FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
        (VALUES('party.read'),('party.create'),('party.update'),('party.archive')) a(action)
    WHERE r.key='party_manager' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON parties TO copypaste_app;
        GRANT SELECT,INSERT ON party_revisions TO copypaste_app;
        REVOKE UPDATE,DELETE ON party_revisions FROM copypaste_app;
        REVOKE DELETE ON parties FROM copypaste_app;
    END IF;
END;
$$;
