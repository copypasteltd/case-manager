-- A historical restoration copies fixed bytes into an independent physical key.
-- The current file pointer changes only in a later, authorized commit command.
CREATE TABLE file_history_restores (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, file_id uuid NOT NULL, owner_id uuid NOT NULL,
    owner_auth_version bigint NOT NULL, org_auth_version bigint NOT NULL, source_channel text NOT NULL
        CHECK(source_channel IN ('web','android','wechat','douyin')),
    folder_id uuid NOT NULL, classification text NOT NULL, base_file_version bigint NOT NULL,
    base_version_id uuid NOT NULL, source_version_id uuid NOT NULL, planned_version_id uuid NOT NULL,
    source_replica_id uuid NOT NULL, source_provider text NOT NULL CHECK(source_provider IN ('oss','r2')),
    source_bucket text NOT NULL, fixed_source jsonb NOT NULL, source_digest text NOT NULL CHECK(source_digest~'^[a-f0-9]{64}$'),
    sha256 text NOT NULL CHECK(sha256~'^[a-f0-9]{64}$'), size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 120000000000),
    storage_epoch bigint NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','copying','verifying','retry_wait','ready','committed','cancelled','failed')),
    active_job_id uuid, candidate_transfer_id uuid, result_version_id uuid,
    last_error text, version bigint NOT NULL DEFAULT 1,
    expires_at timestamptz NOT NULL DEFAULT now()+interval '48 hours',
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,planned_version_id),
    FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id),
    FOREIGN KEY(org_id,file_id,base_version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,file_id,source_version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,file_id,result_version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,source_replica_id) REFERENCES file_replicas(org_id,id),
    FOREIGN KEY(org_id,storage_epoch) REFERENCES storage_epochs(org_id,epoch),
    FOREIGN KEY(org_id,active_job_id) REFERENCES jobs(org_id,id),
    CHECK((state='committed')=(result_version_id IS NOT NULL)),
    CHECK(state NOT IN ('ready','committed') OR candidate_transfer_id IS NOT NULL)
);
CREATE INDEX history_restore_file_page ON file_history_restores(org_id,file_id,created_at DESC,id DESC);
CREATE INDEX history_restore_quota ON file_history_restores(org_id,expires_at)
    WHERE state IN ('queued','copying','verifying','retry_wait','ready');
CREATE TRIGGER history_restore_inputs_immutable BEFORE UPDATE OF org_id,file_id,owner_id,owner_auth_version,org_auth_version,
    source_channel,folder_id,classification,base_file_version,base_version_id,source_version_id,planned_version_id,
    source_replica_id,source_provider,source_bucket,fixed_source,source_digest,sha256,size_bytes,storage_epoch,reason,expires_at
    ON file_history_restores FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER history_restore_result_immutable BEFORE UPDATE OF candidate_transfer_id,result_version_id ON file_history_restores
    FOR EACH ROW WHEN((OLD.candidate_transfer_id IS NOT NULL AND NEW.candidate_transfer_id IS DISTINCT FROM OLD.candidate_transfer_id)
        OR (OLD.result_version_id IS NOT NULL AND NEW.result_version_id IS DISTINCT FROM OLD.result_version_id))
    EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE object_transfers DROP CONSTRAINT object_transfers_kind_check, DROP CONSTRAINT transfer_subject,
    ADD COLUMN restore_id uuid,
    ADD CHECK(kind IN ('promotion','replica','restore')),
    ADD FOREIGN KEY(org_id,restore_id) REFERENCES file_history_restores(org_id,id),
    ADD CONSTRAINT transfer_subject CHECK(
        (kind='promotion' AND session_id IS NOT NULL AND replica_id IS NULL AND restore_id IS NULL)
        OR (kind='replica' AND session_id IS NULL AND replica_id IS NOT NULL AND restore_id IS NULL)
        OR (kind='restore' AND session_id IS NULL AND replica_id IS NULL AND restore_id IS NOT NULL));
CREATE TRIGGER transfer_restore_immutable BEFORE UPDATE OF restore_id ON object_transfers
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE file_history_restores ADD FOREIGN KEY(org_id,candidate_transfer_id) REFERENCES object_transfers(org_id,id);
ALTER TABLE file_versions ADD COLUMN restored_from_version_id uuid,
    ADD FOREIGN KEY(org_id,file_id,restored_from_version_id) REFERENCES file_versions(org_id,file_id,id);
CREATE TRIGGER version_restore_origin_immutable BEFORE UPDATE OF restored_from_version_id ON file_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON file_history_restores TO copypaste_app;
    END IF;
END;
$$;
