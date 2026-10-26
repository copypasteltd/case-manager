-- References and legal holds are independent from object deletion. No cloud
-- deletion permission or executor is enabled by this migration.
ALTER TABLE retention_holds ADD CONSTRAINT retention_hold_org_id UNIQUE(org_id,id);
CREATE TABLE retention_hold_releases (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, hold_id uuid NOT NULL,
    released_by uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,hold_id),
    FOREIGN KEY(org_id,hold_id) REFERENCES retention_holds(org_id,id),
    FOREIGN KEY(org_id,released_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER retention_hold_immutable BEFORE UPDATE OR DELETE ON retention_holds
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER retention_release_immutable BEFORE UPDATE OR DELETE ON retention_hold_releases
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE file_version_references (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, file_id uuid NOT NULL, version_id uuid NOT NULL,
    owner_type text NOT NULL CHECK(owner_type IN
        ('knowledge_revision','comment','approval_snapshot','contract_revision','purchase_receipt','claim_revision','deliverable','recovery_set')),
    owner_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,owner_type,owner_id,version_id),
    FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id)
);
CREATE INDEX file_reference_version ON file_version_references(org_id,version_id);
CREATE TRIGGER file_reference_immutable BEFORE UPDATE OR DELETE ON file_version_references
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
-- Business modules may release only their own reference through their audited
-- lifecycle. There is deliberately no generic user-facing unpin endpoint.
CREATE TABLE file_reference_releases (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, reference_id uuid NOT NULL, released_by uuid NOT NULL,
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,reference_id),
    FOREIGN KEY(org_id,reference_id) REFERENCES file_version_references(org_id,id),
    FOREIGN KEY(org_id,released_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER file_reference_release_immutable BEFORE UPDATE OR DELETE ON file_reference_releases
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE retention_policies (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), version bigint NOT NULL CHECK(version>0),
    kind text NOT NULL CHECK(kind IN ('formal_history','trash','draft','multipart')),
    enabled boolean NOT NULL, retain_seconds bigint NOT NULL CHECK(retain_seconds BETWEEN 86400 AND 315360000),
    observation_seconds bigint NOT NULL CHECK(observation_seconds BETWEEN 86400 AND 31536000),
    published_by uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,id), UNIQUE(org_id,kind,version),
    FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER retention_policy_immutable BEFORE UPDATE OR DELETE ON retention_policies
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE retention_assessments (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, file_id uuid NOT NULL, version_id uuid NOT NULL,
    requester_id uuid NOT NULL, file_version bigint NOT NULL, snapshot jsonb NOT NULL,
    snapshot_sha256 text NOT NULL CHECK(snapshot_sha256~'^[a-f0-9]{64}$'),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(org_id,id),
    FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,requester_id) REFERENCES users(org_id,id)
);
CREATE TRIGGER retention_assessment_immutable BEFORE UPDATE OR DELETE ON retention_assessments
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
-- Fixed references are captured by the same transaction as every immutable
-- revision, including non-HTTP writers. Follow-current links do not pin an old
-- version; the actual current pointer is independently protected.
CREATE FUNCTION register_knowledge_file_references() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE item jsonb;
BEGIN
    FOR item IN SELECT value FROM jsonb_array_elements(NEW.source_refs) LOOP
        IF item->>'type'='file' AND item->>'version_id' IS NOT NULL THEN
            INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
                VALUES(gen_random_uuid(),NEW.org_id,(item->>'id')::uuid,(item->>'version_id')::uuid,'knowledge_revision',NEW.id)
                ON CONFLICT(org_id,owner_type,owner_id,version_id) DO NOTHING;
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$;
CREATE TRIGGER knowledge_file_references AFTER INSERT ON post_revisions
    FOR EACH ROW EXECUTE FUNCTION register_knowledge_file_references();
INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
    SELECT gen_random_uuid(),r.org_id,(s.value->>'id')::uuid,(s.value->>'version_id')::uuid,'knowledge_revision',r.id
    FROM post_revisions r CROSS JOIN LATERAL jsonb_array_elements(r.source_refs) s(value)
    WHERE s.value->>'type'='file' AND s.value->>'version_id' IS NOT NULL
    ON CONFLICT(org_id,owner_type,owner_id,version_id) DO NOTHING;
CREATE FUNCTION register_comment_file_references() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,v.file_id,v.id,'comment',NEW.id
        FROM unnest(NEW.attachment_versions) a(id) JOIN file_versions v ON v.org_id=NEW.org_id AND v.id=a.id
        ON CONFLICT(org_id,owner_type,owner_id,version_id) DO NOTHING;
    RETURN NEW;
END;
$$;
CREATE TRIGGER comment_file_references AFTER INSERT ON comments
    FOR EACH ROW EXECUTE FUNCTION register_comment_file_references();
INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
    SELECT gen_random_uuid(),c.org_id,v.file_id,v.id,'comment',c.id FROM comments c
    CROSS JOIN LATERAL unnest(c.attachment_versions) a(id)
    JOIN file_versions v ON v.org_id=c.org_id AND v.id=a.id
    ON CONFLICT(org_id,owner_type,owner_id,version_id) DO NOTHING;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'file_retention_manager','文件保留管理人',true FROM organizations
    ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
        (VALUES('file.retention.manage'),('file.read'),('folder.read')) a(action)
    WHERE r.key='file_retention_manager' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON retention_holds,retention_hold_releases,file_version_references,file_reference_releases,
            retention_policies,retention_assessments TO copypaste_app;
        REVOKE UPDATE,DELETE ON retention_holds FROM copypaste_app;
    END IF;
END;
$$;
