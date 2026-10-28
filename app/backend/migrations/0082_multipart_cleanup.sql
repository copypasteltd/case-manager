-- Explicitly selected abandoned multipart uploads, never formal object deletion.
CREATE TABLE upload_cleanup_plans (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), folder_id uuid NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), state text NOT NULL DEFAULT 'observing' CHECK(state IN ('observing','approved','cancelled')),
 creator_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 snapshot text NOT NULL, snapshot_sha256 text NOT NULL CHECK(snapshot_sha256 ~ '^[a-f0-9]{64}$'),
 item_count integer NOT NULL CHECK(item_count BETWEEN 1 AND 50),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), ready_after timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 approved_by uuid, approved_session_id uuid, approved_channel text,
 approved_auth_version bigint, approved_org_version bigint, approved_at timestamptz, approval_reason text,
 cancelled_at timestamptz, cancellation_reason text,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id),
 FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,approved_by) REFERENCES users(org_id,id),
 CHECK(encode(sha256(convert_to(snapshot,'UTF8')),'hex')=snapshot_sha256),
 CHECK(ready_after>=created_at+interval '24 hours' AND expires_at>ready_after),
 CHECK((approved_at IS NULL)=(approved_by IS NULL)),
 CHECK(approved_at IS NULL OR (approved_at>=ready_after AND approved_at<expires_at AND approved_session_id IS NOT NULL
  AND approved_channel IN ('web','android','wechat','douyin') AND approved_auth_version>0 AND approved_org_version>0
  AND length(btrim(approval_reason)) BETWEEN 1 AND 200)),
 CHECK((state='cancelled')=(cancelled_at IS NOT NULL))
);
CREATE INDEX upload_cleanup_plans_history ON upload_cleanup_plans(org_id,folder_id,created_at DESC,id DESC);
CREATE FUNCTION guard_upload_cleanup_plan() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.id IS DISTINCT FROM OLD.id OR NEW.org_id IS DISTINCT FROM OLD.org_id
  OR NEW.folder_id IS DISTINCT FROM OLD.folder_id OR NEW.creator_id IS DISTINCT FROM OLD.creator_id
  OR NEW.snapshot IS DISTINCT FROM OLD.snapshot OR NEW.snapshot_sha256 IS DISTINCT FROM OLD.snapshot_sha256
  OR NEW.item_count IS DISTINCT FROM OLD.item_count OR NEW.reason IS DISTINCT FROM OLD.reason
  OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.ready_after IS DISTINCT FROM OLD.ready_after OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
  OR NEW.version<>OLD.version+1 OR NOT ((OLD.state='observing' AND NEW.state IN ('approved','cancelled')) OR (OLD.state='approved' AND NEW.state='cancelled'))
  OR (OLD.approved_at IS NOT NULL AND (NEW.approved_by,NEW.approved_session_id,NEW.approved_channel,NEW.approved_auth_version,
    NEW.approved_org_version,NEW.approved_at,NEW.approval_reason) IS DISTINCT FROM (OLD.approved_by,OLD.approved_session_id,
    OLD.approved_channel,OLD.approved_auth_version,OLD.approved_org_version,OLD.approved_at,OLD.approval_reason)) THEN
  RAISE EXCEPTION 'cleanup plans preserve exact selection, observation and approval' USING ERRCODE='23514';
 END IF; RETURN NEW;
END; $$;
CREATE TRIGGER upload_cleanup_plan_guard BEFORE UPDATE OR DELETE ON upload_cleanup_plans FOR EACH ROW EXECUTE FUNCTION guard_upload_cleanup_plan();
CREATE TABLE upload_cleanup_items (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, plan_id uuid NOT NULL, upload_id uuid NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), generation integer NOT NULL DEFAULT 1 CHECK(generation>0),
 state text NOT NULL DEFAULT 'observing' CHECK(state IN ('observing','queued','running','retry_wait','succeeded','failed','blocked')),
 approved_upload_version bigint, attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 last_error text, confirmed_at timestamptz, updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,plan_id,upload_id),
 FOREIGN KEY(org_id,plan_id) REFERENCES upload_cleanup_plans(org_id,id), FOREIGN KEY(org_id,upload_id) REFERENCES upload_sessions(org_id,id),
 CHECK((state='succeeded')=(confirmed_at IS NOT NULL))
);
CREATE UNIQUE INDEX upload_cleanup_single_reservation ON upload_cleanup_items(org_id,upload_id)
 WHERE state IN ('queued','running','retry_wait','succeeded');
CREATE TABLE upload_cleanup_receipts (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, item_id uuid NOT NULL, job_id uuid NOT NULL,
 generation integer NOT NULL CHECK(generation>0), attempt integer NOT NULL CHECK(attempt>0),
 outcome text NOT NULL CHECK(outcome IN ('started','confirmed_absent','retry_scheduled','failed','blocked')),
 error_code text, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,job_id,attempt,outcome),
 FOREIGN KEY(org_id,item_id) REFERENCES upload_cleanup_items(org_id,id), FOREIGN KEY(org_id,job_id) REFERENCES jobs(org_id,id)
);
CREATE INDEX upload_cleanup_receipts_history ON upload_cleanup_receipts(org_id,item_id,created_at DESC,id DESC);
CREATE UNIQUE INDEX upload_cleanup_one_attempt_result ON upload_cleanup_receipts(org_id,job_id,attempt) WHERE outcome<>'started';
CREATE TRIGGER upload_cleanup_receipt_immutable BEFORE UPDATE OR DELETE ON upload_cleanup_receipts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_upload_cleanup_item() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR NEW.id IS DISTINCT FROM OLD.id OR NEW.org_id IS DISTINCT FROM OLD.org_id
  OR NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.upload_id IS DISTINCT FROM OLD.upload_id
  OR OLD.state='succeeded' OR NEW.version<>OLD.version+1
  OR (OLD.approved_upload_version IS NOT NULL AND NEW.approved_upload_version IS DISTINCT FROM OLD.approved_upload_version)
  OR NOT (NEW.generation=OLD.generation OR (OLD.state='failed' AND NEW.state='queued' AND NEW.generation=OLD.generation+1)) THEN
  RAISE EXCEPTION 'cleanup items preserve source, reservations and confirmed results' USING ERRCODE='23514';
 END IF; RETURN NEW;
END; $$;
CREATE TRIGGER upload_cleanup_item_guard BEFORE UPDATE OR DELETE ON upload_cleanup_items FOR EACH ROW EXECUTE FUNCTION guard_upload_cleanup_item();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT,UPDATE ON upload_cleanup_plans,upload_cleanup_items TO copypaste_app;
 GRANT SELECT,INSERT ON upload_cleanup_receipts TO copypaste_app;
 REVOKE DELETE ON upload_cleanup_plans,upload_cleanup_items FROM copypaste_app;
 REVOKE UPDATE,DELETE ON upload_cleanup_receipts FROM copypaste_app;
END IF; END; $$;
