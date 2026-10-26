ALTER TABLE upload_sessions DROP CONSTRAINT upload_sessions_state_check;
ALTER TABLE upload_sessions ADD CONSTRAINT upload_sessions_state_check
    CHECK(state IN ('created','uploading','finalizing','verifying','blocked','ready','committed','aborted','expired','failed'));
ALTER TABLE upload_sessions
    ADD COLUMN verified_bytes bigint NOT NULL DEFAULT 0 CHECK(verified_bytes>=0 AND verified_bytes<=size_bytes),
    ADD COLUMN scan_state text NOT NULL DEFAULT 'pending' CHECK(scan_state IN ('pending','clean','quarantined','unscannable','waived')),
    ADD COLUMN scan_result_ref uuid;
CREATE TABLE upload_hash_checkpoints (
    org_id uuid NOT NULL, session_id uuid NOT NULL,
    source_digest text NOT NULL CHECK(source_digest~'^[0-9a-f]{64}$'),
    runtime_version text NOT NULL CHECK(length(runtime_version) BETWEEN 1 AND 100),
    completed_bytes bigint NOT NULL CHECK(completed_bytes BETWEEN 0 AND 120000000000),
    hash_state bytea NOT NULL CHECK(octet_length(hash_state) BETWEEN 1 AND 2048),
    checksum text NOT NULL CHECK(checksum~'^[0-9a-f]{64}$'),
    sha256 text CHECK(sha256 IS NULL OR sha256~'^[0-9a-f]{64}$'),
    job_id uuid NOT NULL, fencing_token bigint NOT NULL CHECK(fencing_token>0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(org_id,session_id),
    FOREIGN KEY(org_id,session_id) REFERENCES upload_sessions(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id)
);
CREATE TABLE upload_scan_reports (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, session_id uuid NOT NULL,
    source_digest text NOT NULL CHECK(source_digest~'^[0-9a-f]{64}$'),
    sha256 text NOT NULL CHECK(sha256~'^[0-9a-f]{64}$'),
    size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 120000000000),
    state text NOT NULL CHECK(state IN ('clean','quarantined','unscannable')),
    code text NOT NULL CHECK(code~'^[A-Z][A-Z0-9_]{1,79}$'),
    engine text, engine_version text, signature_revision text,
    policy_sha256 text CHECK(policy_sha256 IS NULL OR policy_sha256~'^[0-9a-f]{64}$'),
    streamed_bytes bigint NOT NULL CHECK(streamed_bytes>=0 AND streamed_bytes<=size_bytes),
    job_id uuid NOT NULL, fencing_token bigint NOT NULL CHECK(fencing_token>0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,session_id,id),
    FOREIGN KEY(org_id,session_id) REFERENCES upload_sessions(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id),
    CHECK(state<>'clean' OR (engine IS NOT NULL AND engine_version IS NOT NULL AND signature_revision IS NOT NULL
        AND policy_sha256 IS NOT NULL AND streamed_bytes=size_bytes))
);
ALTER TABLE upload_sessions ADD FOREIGN KEY(org_id,id,scan_result_ref)
    REFERENCES upload_scan_reports(org_id,session_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER scan_report_immutable BEFORE UPDATE OR DELETE ON upload_scan_reports
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER upload_digest_immutable BEFORE UPDATE OF final_digest ON upload_sessions
    FOR EACH ROW WHEN (OLD.final_digest IS NOT NULL AND NEW.final_digest IS DISTINCT FROM OLD.final_digest)
    EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER hash_source_immutable BEFORE UPDATE OF org_id,session_id,source_digest ON upload_hash_checkpoints
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON upload_hash_checkpoints TO copypaste_app;
        GRANT SELECT,INSERT ON upload_scan_reports TO copypaste_app;
    END IF;
END;
$$;
