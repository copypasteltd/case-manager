CREATE TABLE storage_targets (
    org_id uuid NOT NULL REFERENCES organizations(id), provider text NOT NULL CHECK(provider IN ('oss','r2')),
    bucket text NOT NULL CHECK(length(bucket) BETWEEN 3 AND 100), enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(org_id,provider)
);
CREATE TABLE object_transfers (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, session_id uuid NOT NULL, job_id uuid NOT NULL,
    file_id uuid NOT NULL, version_id uuid NOT NULL,
    source_digest text NOT NULL CHECK(source_digest~'^[0-9a-f]{64}$'),
    object_key text NOT NULL, provider_upload_id text, manifest jsonb, frozen_source jsonb,
    state text NOT NULL CHECK(state IN ('starting','active','sealed','complete','uncertain','superseded')),
    fencing_token bigint NOT NULL CHECK(fencing_token>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,object_key), UNIQUE(org_id,version_id),
    FOREIGN KEY(org_id,session_id) REFERENCES upload_sessions(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id),
    CHECK(state NOT IN ('active','sealed','complete') OR provider_upload_id IS NOT NULL),
    CHECK(state<>'sealed' OR manifest IS NOT NULL),
    CHECK(state<>'complete' OR frozen_source IS NOT NULL)
);
CREATE TABLE object_transfer_parts (
    transfer_id uuid NOT NULL REFERENCES object_transfers(id), part_no integer NOT NULL CHECK(part_no BETWEEN 1 AND 10000),
    size_bytes bigint NOT NULL CHECK(size_bytes>0), etag text NOT NULL,
    fencing_token bigint NOT NULL CHECK(fencing_token>0), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(transfer_id,part_no)
);
CREATE TABLE object_transfer_hashes (
    transfer_id uuid PRIMARY KEY REFERENCES object_transfers(id),
    source_digest text NOT NULL CHECK(source_digest~'^[0-9a-f]{64}$'), runtime_version text NOT NULL,
    completed_bytes bigint NOT NULL CHECK(completed_bytes BETWEEN 0 AND 120000000000),
    hash_state bytea NOT NULL CHECK(octet_length(hash_state) BETWEEN 1 AND 2048),
    checksum text NOT NULL CHECK(checksum~'^[0-9a-f]{64}$'),
    sha256 text CHECK(sha256 IS NULL OR sha256~'^[0-9a-f]{64}$'),
    fencing_token bigint NOT NULL CHECK(fencing_token>0), updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE upload_sessions ADD COLUMN candidate_transfer_id uuid,
    ADD FOREIGN KEY(org_id,candidate_transfer_id) REFERENCES object_transfers(org_id,id);
CREATE TRIGGER transfer_inputs_immutable BEFORE UPDATE OF org_id,session_id,job_id,file_id,version_id,source_digest,object_key
    ON object_transfers FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER transfer_upload_id_immutable BEFORE UPDATE OF provider_upload_id ON object_transfers
    FOR EACH ROW WHEN(OLD.provider_upload_id IS NOT NULL AND NEW.provider_upload_id IS DISTINCT FROM OLD.provider_upload_id)
    EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER transfer_manifest_immutable BEFORE UPDATE OF manifest ON object_transfers
    FOR EACH ROW WHEN(OLD.manifest IS NOT NULL AND NEW.manifest IS DISTINCT FROM OLD.manifest)
    EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER transfer_frozen_immutable BEFORE UPDATE OF frozen_source ON object_transfers
    FOR EACH ROW WHEN(OLD.frozen_source IS NOT NULL AND NEW.frozen_source IS DISTINCT FROM OLD.frozen_source)
    EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER upload_candidate_immutable BEFORE UPDATE OF candidate_key,candidate_source_id,candidate_transfer_id ON upload_sessions
    FOR EACH ROW WHEN(OLD.candidate_transfer_id IS NOT NULL AND
        (NEW.candidate_key IS DISTINCT FROM OLD.candidate_key OR NEW.candidate_source_id IS DISTINCT FROM OLD.candidate_source_id
        OR NEW.candidate_transfer_id IS DISTINCT FROM OLD.candidate_transfer_id))
    EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX transfer_job_lookup ON object_transfers(org_id,job_id,created_at DESC,id DESC);
ALTER TABLE upload_scan_reports ADD UNIQUE(org_id,id);
ALTER TABLE file_versions ADD COLUMN scan_report_ref uuid,
    ADD FOREIGN KEY(org_id,scan_report_ref) REFERENCES upload_scan_reports(org_id,id);
CREATE TRIGGER file_scan_report_immutable BEFORE UPDATE OF scan_report_ref ON file_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT ON storage_targets TO copypaste_app;
        GRANT SELECT,INSERT,UPDATE ON object_transfers,object_transfer_parts,object_transfer_hashes TO copypaste_app;
    END IF;
END;
$$;
