-- Explicit retirement of exact, known originals. Historical metadata, source
-- evidence and business facts are never deleted. Unknown objects are not inputs.
ALTER TABLE file_versions DROP CONSTRAINT file_versions_availability_check;
ALTER TABLE file_versions ADD CONSTRAINT file_versions_availability_check
    CHECK(availability IN ('available','quarantined','missing','retiring','purged'));
ALTER TABLE upload_sessions ADD COLUMN committed_at timestamptz;
-- Existing terminal rows retain their recorded commit/update timestamp. This is
-- an ordinary schema upgrade, not a historical-content migration.
UPDATE upload_sessions SET committed_at=updated_at WHERE state='committed';
CREATE FUNCTION seal_upload_commit_time() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.committed_at IS NOT NULL AND NEW.committed_at IS DISTINCT FROM OLD.committed_at THEN
        RAISE EXCEPTION 'commit timestamp is immutable' USING ERRCODE='23514';
    END IF;
    IF OLD.state<>'committed' AND NEW.state='committed' THEN NEW.committed_at:=clock_timestamp(); END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER upload_commit_time BEFORE UPDATE ON upload_sessions FOR EACH ROW EXECUTE FUNCTION seal_upload_commit_time();

CREATE TABLE object_retirement_plans (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, file_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('history','committed_draft')), target_id uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    state text NOT NULL DEFAULT 'observing' CHECK(state IN ('observing','approved','cancelled','completed')),
    snapshot jsonb NOT NULL, snapshot_sha256 text NOT NULL CHECK(snapshot_sha256~'^[a-f0-9]{64}$'),
    objects jsonb NOT NULL CHECK(jsonb_typeof(objects)='array' AND jsonb_array_length(objects) BETWEEN 1 AND 2),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(), ready_after timestamptz NOT NULL,
    expires_at timestamptz NOT NULL, approved_at timestamptz, approval_reason text,
    cancelled_at timestamptz, cancellation_reason text,
    approved_by uuid, approved_session_id uuid, approved_channel text,
    approved_auth_version bigint, approved_org_version bigint,
    UNIQUE(org_id,id), FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
    FOREIGN KEY(org_id,approved_by) REFERENCES users(org_id,id),
    CHECK(ready_after>=created_at+interval '24 hours' AND expires_at>ready_after),
    CHECK(state NOT IN ('approved','completed') OR
        (approved_at IS NOT NULL AND approved_at>=ready_after AND approved_at<expires_at AND
         approved_by IS NOT NULL AND approved_session_id IS NOT NULL AND
         approved_channel IN ('web','android','wechat','douyin') AND approved_auth_version IS NOT NULL AND approved_org_version IS NOT NULL))
);
CREATE TRIGGER retirement_plan_input_immutable BEFORE UPDATE OF org_id,file_id,kind,target_id,snapshot,snapshot_sha256,objects,reason,created_at,ready_after,expires_at
    OR DELETE ON object_retirement_plans FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX retirement_plan_file ON object_retirement_plans(org_id,file_id,created_at,id);
CREATE FUNCTION protect_retirement_plan_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.version<>OLD.version+1 OR OLD.state IN ('cancelled','completed') OR
       (OLD.state='observing' AND NEW.state NOT IN ('approved','cancelled')) OR
       (OLD.state='approved' AND NEW.state NOT IN ('completed','cancelled')) THEN
        RAISE EXCEPTION 'invalid retirement plan transition' USING ERRCODE='23514';
    END IF;
    IF OLD.approved_at IS NOT NULL AND ROW(NEW.approved_at,NEW.approved_by,NEW.approved_session_id,NEW.approved_channel,NEW.approved_auth_version,NEW.approved_org_version,NEW.approval_reason)
        IS DISTINCT FROM ROW(OLD.approved_at,OLD.approved_by,OLD.approved_session_id,OLD.approved_channel,OLD.approved_auth_version,OLD.approved_org_version,OLD.approval_reason) THEN
        RAISE EXCEPTION 'retirement approval is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retirement_plan_state BEFORE UPDATE ON object_retirement_plans FOR EACH ROW EXECUTE FUNCTION protect_retirement_plan_state();

CREATE TABLE object_retirement_items (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL, ordinal int NOT NULL CHECK(ordinal BETWEEN 0 AND 1),
    provider text NOT NULL CHECK(provider IN ('oss','r2')), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    generation int NOT NULL DEFAULT 1 CHECK(generation>0),
    state text NOT NULL DEFAULT 'observing' CHECK(state IN ('observing','queued','running','retry_wait','blocked','failed','succeeded')),
    attempts int NOT NULL DEFAULT 0 CHECK(attempts>=0), last_error text CHECK(last_error IS NULL OR last_error~'^[A-Z][A-Z0-9_]{1,79}$'),
    confirmed_at timestamptz, updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,id), UNIQUE(org_id,plan_id,ordinal),
    FOREIGN KEY(org_id,plan_id) REFERENCES object_retirement_plans(org_id,id),
    CHECK((state='succeeded')=(confirmed_at IS NOT NULL))
);
CREATE TRIGGER retirement_item_input_immutable BEFORE UPDATE OF org_id,plan_id,ordinal,provider OR DELETE ON object_retirement_items
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION protect_retirement_item_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.version<>OLD.version+1 OR OLD.state='succeeded' OR NEW.attempts<OLD.attempts
       OR NEW.generation NOT IN (OLD.generation,OLD.generation+1)
       OR (NEW.generation=OLD.generation+1 AND NOT (OLD.state IN ('failed','blocked') AND NEW.state='queued'))
       OR (OLD.state='observing' AND NEW.state NOT IN ('queued','blocked')) THEN
        RAISE EXCEPTION 'invalid retirement item transition' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retirement_item_state BEFORE UPDATE ON object_retirement_items FOR EACH ROW EXECUTE FUNCTION protect_retirement_item_state();

-- Permanent tombstones fence platform writers and references before any cloud
-- operation. Cancellation stops remaining work; it never resurrects an original
-- that might already have been removed by an interrupted attempt.
CREATE TABLE object_retirement_barriers (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL, kind text NOT NULL,
    target_id uuid NOT NULL, provider text NOT NULL, bucket text NOT NULL, object_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(org_id,provider,bucket,object_key),
    FOREIGN KEY(org_id,plan_id) REFERENCES object_retirement_plans(org_id,id),
    CHECK(kind IN ('history','committed_draft'))
);
CREATE INDEX retirement_target_barrier ON object_retirement_barriers(org_id,kind,target_id);
CREATE TRIGGER retirement_barrier_immutable BEFORE UPDATE OR DELETE ON object_retirement_barriers
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION protect_retired_availability() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.availability IN ('retiring','purged') AND NEW.availability IS DISTINCT FROM OLD.availability
        AND NOT (OLD.availability='retiring' AND NEW.availability='purged') THEN
        RAISE EXCEPTION 'retired original cannot be resurrected' USING ERRCODE='23514';
    END IF;
    IF NEW.availability IN ('retiring','purged') AND NEW.availability IS DISTINCT FROM OLD.availability AND
       NOT EXISTS(SELECT 1 FROM object_retirement_barriers b JOIN object_retirement_plans p ON p.org_id=b.org_id AND p.id=b.plan_id
           WHERE b.org_id=NEW.org_id AND b.kind='history' AND b.target_id=NEW.id AND p.state='approved'
           AND (NEW.availability='retiring' OR NOT EXISTS(SELECT 1 FROM object_retirement_items i WHERE i.org_id=p.org_id AND i.plan_id=p.id AND i.state<>'succeeded'))) THEN
        RAISE EXCEPTION 'retirement availability requires exact approved evidence' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retired_availability_immutable BEFORE UPDATE OF availability ON file_versions FOR EACH ROW EXECUTE FUNCTION protect_retired_availability();
CREATE TABLE object_retirement_receipts (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, item_id uuid NOT NULL, job_id uuid NOT NULL,
    generation int NOT NULL CHECK(generation>0), attempt int NOT NULL CHECK(attempt>0),
    outcome text NOT NULL CHECK(outcome IN ('started','confirmed_absent','retry_scheduled','blocked','failed')),
    error_code text CHECK(error_code IS NULL OR error_code~'^[A-Z][A-Z0-9_]{1,79}$'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY(org_id,item_id) REFERENCES object_retirement_items(org_id,id),
    FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id),
    UNIQUE(org_id,item_id,generation,attempt,outcome)
);
CREATE TRIGGER retirement_receipt_immutable BEFORE UPDATE OR DELETE ON object_retirement_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION reject_retired_version_use() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; state text;
BEGIN
    target := (to_jsonb(NEW)->>TG_ARGV[0])::uuid;
    IF target IS NULL THEN RETURN NEW; END IF;
    -- The row lock serializes a concurrent new dependency with approval.
    SELECT availability INTO state FROM file_versions WHERE org_id=NEW.org_id AND id=target FOR SHARE;
    IF state IN ('retiring','purged') THEN
        RAISE EXCEPTION 'retired version cannot acquire new dependencies' USING ERRCODE='23514',CONSTRAINT='retired_version_dependency';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retired_reference_barrier BEFORE INSERT ON file_version_references FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('version_id');
CREATE TRIGGER retired_current_barrier BEFORE INSERT OR UPDATE OF current_version_id ON files FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('current_version_id');
CREATE TRIGGER retired_download_barrier BEFORE INSERT ON download_permits FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('version_id');
CREATE TRIGGER retired_restore_barrier BEFORE INSERT ON file_history_restores FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('source_version_id');
CREATE TRIGGER retired_upload_base_barrier BEFORE INSERT ON upload_sessions FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('base_version_id');
CREATE TRIGGER retired_derived_barrier BEFORE INSERT ON derived_artifacts FOR EACH ROW EXECUTE FUNCTION reject_retired_version_use('version_id');

CREATE FUNCTION reject_retired_object_write() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE key text; provider text; bucket text;
BEGIN
    key := to_jsonb(NEW)->>TG_ARGV[0];
    IF key IS NULL THEN RETURN NEW; END IF;
    IF EXISTS(SELECT 1 FROM object_retirement_barriers b WHERE b.org_id=NEW.org_id AND b.object_key=key) THEN
        RAISE EXCEPTION 'retired object key cannot be reused' USING ERRCODE='23514',CONSTRAINT='retired_object_key';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retired_transfer_barrier BEFORE INSERT OR UPDATE OF object_key ON object_transfers FOR EACH ROW EXECUTE FUNCTION reject_retired_object_write('object_key');
CREATE TRIGGER retired_replica_key_barrier BEFORE INSERT OR UPDATE OF object_key ON file_replicas FOR EACH ROW EXECUTE FUNCTION reject_retired_object_write('object_key');
CREATE TRIGGER retired_version_key_barrier BEFORE INSERT ON file_versions FOR EACH ROW EXECUTE FUNCTION reject_retired_object_write('object_key');
CREATE TRIGGER retired_draft_key_barrier BEFORE INSERT OR UPDATE OF draft_key ON upload_attempts FOR EACH ROW EXECUTE FUNCTION reject_retired_object_write('draft_key');

DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT UPDATE(availability) ON file_versions TO copypaste_app;
        GRANT SELECT,INSERT,UPDATE ON object_retirement_plans,object_retirement_items TO copypaste_app;
        GRANT SELECT,INSERT ON object_retirement_barriers,object_retirement_receipts TO copypaste_app;
    END IF;
END $$;
