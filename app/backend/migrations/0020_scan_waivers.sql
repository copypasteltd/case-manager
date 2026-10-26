CREATE TABLE scan_waiver_policies (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted')),
    version bigint NOT NULL CHECK(version>0), enabled boolean NOT NULL,
    media_types text[] NOT NULL CHECK(cardinality(media_types) BETWEEN 1 AND 20),
    allowed_codes text[] NOT NULL CHECK(cardinality(allowed_codes) BETWEEN 1 AND 4),
    max_bytes bigint NOT NULL CHECK(max_bytes BETWEEN 1 AND 120000000000),
    reviewer_id uuid NOT NULL, reason text NOT NULL, published_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,classification,version),
    FOREIGN KEY(org_id,reviewer_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER scan_waiver_policy_immutable BEFORE UPDATE OR DELETE ON scan_waiver_policies
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE scan_waivers (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, session_id uuid NOT NULL, report_id uuid NOT NULL,
    source_digest text NOT NULL CHECK(source_digest~'^[0-9a-f]{64}$'), sha256 text NOT NULL CHECK(sha256~'^[0-9a-f]{64}$'),
    policy_id uuid NOT NULL, policy_version bigint NOT NULL, upload_version bigint NOT NULL,
    owner_id uuid NOT NULL, reviewer_id uuid NOT NULL CHECK(reviewer_id<>owner_id), classification text NOT NULL,
    name text NOT NULL, media_type text NOT NULL, size_bytes bigint NOT NULL, scan_code text NOT NULL, reason text NOT NULL,
    acknowledged_risk boolean NOT NULL CHECK(acknowledged_risk), expires_at timestamptz NOT NULL,
    state text NOT NULL CHECK(state IN ('pending','approved','rejected','cancelled')),
    decision_reason text, decided_at timestamptz, version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,session_id,id),
    FOREIGN KEY(org_id,session_id) REFERENCES upload_sessions(org_id,id),
    FOREIGN KEY(org_id,report_id) REFERENCES upload_scan_reports(org_id,id),
    FOREIGN KEY(org_id,policy_id) REFERENCES scan_waiver_policies(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,reviewer_id) REFERENCES users(org_id,id),
    CHECK(state='pending' OR decided_at IS NOT NULL)
);
CREATE UNIQUE INDEX one_pending_scan_waiver ON scan_waivers(org_id,session_id,report_id) WHERE state='pending';
CREATE TRIGGER scan_waiver_inputs_immutable BEFORE UPDATE OF org_id,session_id,report_id,source_digest,sha256,policy_id,policy_version,upload_version,
    owner_id,reviewer_id,classification,name,media_type,size_bytes,scan_code,reason,acknowledged_risk,expires_at ON scan_waivers
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE scan_waiver_decisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, waiver_id uuid NOT NULL, actor_id uuid NOT NULL,
    decision text NOT NULL CHECK(decision IN ('approve','reject','cancel')), reason text NOT NULL,
    acknowledged_risk boolean NOT NULL DEFAULT false, mfa_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(waiver_id),
    FOREIGN KEY(org_id,waiver_id) REFERENCES scan_waivers(org_id,id), FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
    CHECK(decision<>'approve' OR (acknowledged_risk AND mfa_verified_at IS NOT NULL))
);
CREATE TRIGGER scan_waiver_decision_immutable BEFORE UPDATE OR DELETE ON scan_waiver_decisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE upload_sessions ADD COLUMN scan_waiver_ref uuid,
    ADD FOREIGN KEY(org_id,id,scan_waiver_ref) REFERENCES scan_waivers(org_id,session_id,id);
ALTER TABLE file_versions ADD FOREIGN KEY(org_id,scan_waiver_ref) REFERENCES scan_waivers(org_id,id);
-- Existing organizations receive an unbound role; no person is auto-authorized.
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'file_scan_reviewer','文件扫描例外审核人',true FROM organizations
    ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
        (VALUES('file.scan.waive'),('folder.read'),('file.read')) a(action)
    WHERE r.key='file_scan_reviewer' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON scan_waiver_policies,scan_waiver_decisions TO copypaste_app;
        GRANT SELECT,INSERT,UPDATE ON scan_waivers TO copypaste_app;
    END IF;
END;
$$;
