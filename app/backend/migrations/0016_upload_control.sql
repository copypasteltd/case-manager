CREATE TABLE storage_quotas (
    org_id uuid PRIMARY KEY REFERENCES organizations(id),
    logical_limit_bytes bigint NOT NULL CHECK(logical_limit_bytes>0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- No invented production capacity: an operator must explicitly provision quota.
ALTER TABLE upload_sessions
    ADD COLUMN provider text CHECK(provider IN ('oss','r2')),
    ADD COLUMN bucket text CHECK(length(bucket) BETWEEN 3 AND 200),
    ADD COLUMN classification text CHECK(classification IN ('internal','project_restricted','confidential')),
    ADD COLUMN source_channel text CHECK(source_channel IN ('web','android')),
    ADD COLUMN frozen_etag text,
    ADD COLUMN frozen_native_version text,
    ADD COLUMN finalize_manifest jsonb,
    ADD COLUMN complete_requested_at timestamptz;
ALTER TABLE upload_parts ADD COLUMN cloud_confirmed boolean NOT NULL DEFAULT false;
CREATE INDEX upload_sessions_owner_active ON upload_sessions(org_id,owner_id,state,expires_at);
ALTER TABLE jobs ADD CONSTRAINT jobs_org_identity UNIQUE(org_id,id);
CREATE TABLE upload_attempts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, session_id uuid NOT NULL,
    job_id uuid NOT NULL, fencing_token bigint NOT NULL CHECK(fencing_token>0),
    draft_key text NOT NULL,
    state text NOT NULL CHECK(state IN ('starting','active','uncertain','superseded')),
    provider_upload_id text, error_code text CHECK(error_code IS NULL OR error_code~'^[A-Z][A-Z0-9_]{1,79}$'),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,draft_key), UNIQUE(session_id,job_id,fencing_token),
    FOREIGN KEY(org_id,session_id) REFERENCES upload_sessions(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id)
);
CREATE FUNCTION protect_upload_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.org_id,NEW.owner_id,NEW.target_folder_id,NEW.file_id,NEW.base_version_id,
        NEW.name,NEW.media_type,NEW.size_bytes,NEW.source_fingerprint,NEW.part_size,NEW.part_count,
        NEW.storage_epoch,NEW.provider,NEW.bucket,NEW.classification,NEW.source_channel)
       IS DISTINCT FROM ROW(OLD.org_id,OLD.owner_id,OLD.target_folder_id,OLD.file_id,OLD.base_version_id,
        OLD.name,OLD.media_type,OLD.size_bytes,OLD.source_fingerprint,OLD.part_size,OLD.part_count,
        OLD.storage_epoch,OLD.provider,OLD.bucket,OLD.classification,OLD.source_channel) THEN
        RAISE EXCEPTION 'upload execution input is immutable' USING ERRCODE='23514';
    END IF;
    IF OLD.finalize_manifest IS NOT NULL AND NEW.finalize_manifest IS DISTINCT FROM OLD.finalize_manifest THEN
        RAISE EXCEPTION 'sealed upload manifest is immutable' USING ERRCODE='23514';
    END IF;
    IF OLD.frozen_source_id IS NOT NULL AND ROW(NEW.draft_key,NEW.frozen_source_id,NEW.frozen_etag,NEW.frozen_native_version)
        IS DISTINCT FROM ROW(OLD.draft_key,OLD.frozen_source_id,OLD.frozen_etag,OLD.frozen_native_version) THEN
        RAISE EXCEPTION 'frozen upload source is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER upload_input_immutable BEFORE UPDATE ON upload_sessions
    FOR EACH ROW EXECUTE FUNCTION protect_upload_input();
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT ON storage_quotas TO copypaste_app;
        GRANT SELECT,INSERT,UPDATE ON upload_attempts TO copypaste_app;
    END IF;
END;
$$;
