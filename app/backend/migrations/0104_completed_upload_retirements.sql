-- Ordinary schema upgrade. Unknown historical completion times start a new,
-- conservative observation clock; no business or cloud content is migrated.
ALTER TABLE object_retirement_plans ALTER COLUMN file_id DROP NOT NULL,
    ADD COLUMN scope_folder_id uuid,
    ADD FOREIGN KEY(org_id,scope_folder_id) REFERENCES folders(org_id,id),
    DROP CONSTRAINT object_retirement_plans_kind_check,
    DROP CONSTRAINT object_retirement_plans_objects_check,
    ADD CHECK(kind IN ('history','committed_draft','uncommitted_draft')),
    ADD CONSTRAINT retirement_plan_scope CHECK(
      (kind='uncommitted_draft' AND scope_folder_id IS NOT NULL) OR
      (kind IN ('history','committed_draft') AND scope_folder_id IS NULL AND file_id IS NOT NULL)),
    ADD CONSTRAINT retirement_plan_objects CHECK(jsonb_typeof(objects)='array' AND
      ((kind='history' AND jsonb_array_length(objects)=2) OR
       (kind='committed_draft' AND jsonb_array_length(objects)=1) OR
       (kind='uncommitted_draft' AND jsonb_array_length(objects) BETWEEN 1 AND 64)));
CREATE TRIGGER retirement_folder_immutable BEFORE UPDATE OF scope_folder_id ON object_retirement_plans
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX retirement_plan_folder ON object_retirement_plans(org_id,scope_folder_id,created_at,id)
    WHERE scope_folder_id IS NOT NULL;
ALTER TABLE object_retirement_items DROP CONSTRAINT object_retirement_items_ordinal_check,
    ADD CHECK(ordinal BETWEEN 0 AND 63);
ALTER TABLE object_retirement_authorizations DROP CONSTRAINT object_retirement_authorizations_items_check,
    ADD CHECK(jsonb_typeof(items)='array' AND jsonb_array_length(items) BETWEEN 1 AND 64);
ALTER TABLE object_retirement_barriers DROP CONSTRAINT object_retirement_barriers_kind_check,
    ADD CHECK(kind IN ('history','committed_draft','uncommitted_draft'));

ALTER TABLE upload_sessions ADD COLUMN retirement_plan_id uuid,
    ADD COLUMN completed_observed_at timestamptz,
    ADD FOREIGN KEY(org_id,retirement_plan_id) REFERENCES object_retirement_plans(org_id,id);
ALTER TABLE object_transfers ADD COLUMN completed_observed_at timestamptz;
UPDATE upload_sessions SET completed_observed_at=clock_timestamp() WHERE frozen_source_id IS NOT NULL;
UPDATE object_transfers SET completed_observed_at=clock_timestamp() WHERE state='complete';
CREATE FUNCTION seal_upload_completion_observation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND OLD.completed_observed_at IS NOT NULL THEN
        IF NEW.completed_observed_at IS DISTINCT FROM OLD.completed_observed_at THEN
            RAISE EXCEPTION 'completion observation is immutable' USING ERRCODE='23514';
        END IF;
    ELSIF to_jsonb(NEW)->>'frozen_source_id' IS NOT NULL OR
          (TG_TABLE_NAME='object_transfers' AND NEW.state='complete') THEN
        NEW.completed_observed_at:=clock_timestamp();
    ELSE NEW.completed_observed_at:=NULL;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER upload_completion_observation BEFORE INSERT OR UPDATE ON upload_sessions
    FOR EACH ROW EXECUTE FUNCTION seal_upload_completion_observation();
CREATE TRIGGER transfer_completion_observation BEFORE INSERT OR UPDATE ON object_transfers
    FOR EACH ROW EXECUTE FUNCTION seal_upload_completion_observation();

-- Permanent session fencing is independent of plan cancellation. It blocks a
-- late commit, a stale worker and new promotion attempts after human approval.
CREATE FUNCTION protect_retired_upload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.retirement_plan_id IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'retired upload cannot be changed' USING ERRCODE='23514',CONSTRAINT='retired_upload';
    END IF;
    IF OLD.retirement_plan_id IS NULL AND NEW.retirement_plan_id IS NOT NULL THEN
        IF (to_jsonb(NEW)-ARRAY['retirement_plan_id','version','updated_at']) IS DISTINCT FROM
           (to_jsonb(OLD)-ARRAY['retirement_plan_id','version','updated_at']) OR NEW.version<>OLD.version+1 OR
           NOT EXISTS(SELECT 1 FROM object_retirement_plans p WHERE p.org_id=NEW.org_id AND p.id=NEW.retirement_plan_id
             AND p.kind='uncommitted_draft' AND p.target_id=NEW.id AND p.scope_folder_id=NEW.target_folder_id
             AND p.file_id IS NOT DISTINCT FROM NEW.file_id AND p.state='approved'
             AND (p.snapshot->>'upload_version')::bigint=OLD.version
             AND (SELECT count(*) FROM object_retirement_barriers b WHERE b.org_id=p.org_id AND b.plan_id=p.id)=jsonb_array_length(p.objects)) THEN
            RAISE EXCEPTION 'upload retirement requires approved fixed evidence' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER retired_upload_immutable BEFORE UPDATE ON upload_sessions
    FOR EACH ROW EXECUTE FUNCTION protect_retired_upload();
CREATE FUNCTION reject_retired_upload_transfer() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE session uuid;
BEGIN
    IF TG_OP='DELETE' THEN session:=OLD.session_id; ELSE session:=NEW.session_id; END IF;
    IF session IS NOT NULL AND EXISTS(SELECT 1 FROM upload_sessions WHERE id=session AND retirement_plan_id IS NOT NULL) THEN
        RAISE EXCEPTION 'retired upload transfer is sealed' USING ERRCODE='23514',CONSTRAINT='retired_upload';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
CREATE TRIGGER retired_upload_transfer BEFORE INSERT OR UPDATE OR DELETE ON object_transfers
    FOR EACH ROW EXECUTE FUNCTION reject_retired_upload_transfer();
CREATE TRIGGER retired_upload_attempt BEFORE INSERT OR UPDATE OR DELETE ON upload_attempts
    FOR EACH ROW EXECUTE FUNCTION reject_retired_upload_transfer();
