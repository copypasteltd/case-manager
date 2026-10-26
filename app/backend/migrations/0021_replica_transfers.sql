-- Promotion and replication share the resumable transfer journal, but every
-- uncertain initialization gets an independent physical object key.
ALTER TABLE object_transfers DROP CONSTRAINT object_transfers_org_id_version_id_key,
    ALTER COLUMN session_id DROP NOT NULL,
    ADD COLUMN kind text NOT NULL DEFAULT 'promotion' CHECK(kind IN ('promotion','replica')),
    ADD COLUMN replica_id uuid,
    ADD COLUMN target_provider text,
    ADD COLUMN target_bucket text,
    ADD FOREIGN KEY(org_id,replica_id) REFERENCES file_replicas(org_id,id),
    ADD CONSTRAINT transfer_subject CHECK(
        (kind='promotion' AND session_id IS NOT NULL AND replica_id IS NULL)
        OR (kind='replica' AND session_id IS NULL AND replica_id IS NOT NULL));
UPDATE object_transfers t SET target_provider=u.provider,target_bucket=u.bucket
    FROM upload_sessions u WHERE t.org_id=u.org_id AND t.session_id=u.id;
ALTER TABLE object_transfers ALTER COLUMN target_provider SET NOT NULL,
    ALTER COLUMN target_bucket SET NOT NULL,
    ADD CHECK(target_provider IN ('oss','r2')),
    ADD CHECK(length(target_bucket) BETWEEN 3 AND 100);
CREATE TRIGGER transfer_destination_immutable BEFORE UPDATE OF kind,replica_id,target_provider,target_bucket
    ON object_transfers FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX transfer_replica_lookup ON object_transfers(org_id,replica_id,created_at DESC,id DESC)
    WHERE replica_id IS NOT NULL;
ALTER TABLE file_replicas ADD COLUMN active_job_id uuid,
    ADD FOREIGN KEY(org_id,active_job_id) REFERENCES jobs(org_id,id);
CREATE TRIGGER replica_identity_immutable BEFORE UPDATE OF org_id,version_id,provider,bucket,expected_sha256
    ON file_replicas FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX replica_active_job_lookup ON file_replicas(org_id,active_job_id) WHERE active_job_id IS NOT NULL;
