-- Reviewed destruction of known DB-derived text after its original was purged.
-- Old external output keys have no trusted provider/version identity and remain blocked.
CREATE TABLE file_derived_retirement_plans (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,file_id uuid NOT NULL,version_id uuid NOT NULL,actor_id uuid NOT NULL,session_id uuid NOT NULL,
 basis text NOT NULL CHECK(octet_length(basis) BETWEEN 1 AND 131072 AND jsonb_typeof(basis::jsonb)='object'),
 basis_sha256 text NOT NULL CHECK(basis_sha256=encode(sha256(convert_to(basis,'UTF8')),'hex')),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),ready_after timestamptz NOT NULL,expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,id),
 FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 CHECK(ready_after>=created_at+interval '24 hours' AND expires_at>ready_after AND expires_at<=ready_after+interval '7 days')
);
CREATE TABLE file_derived_retirement_decisions (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,plan_id uuid NOT NULL,actor_id uuid NOT NULL,session_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('execute','cancel')),reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 reviewed boolean NOT NULL CHECK(reviewed),removed_count int NOT NULL CHECK(removed_count BETWEEN 0 AND 16),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,plan_id),FOREIGN KEY(org_id,plan_id) REFERENCES file_derived_retirement_plans(org_id,id),FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 CHECK((action='execute' AND removed_count>0) OR (action='cancel' AND removed_count=0))
);
CREATE TRIGGER derived_retirement_plan_immutable BEFORE UPDATE OR DELETE ON file_derived_retirement_plans FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER derived_retirement_decision_immutable BEFORE UPDATE OR DELETE ON file_derived_retirement_decisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_derived_retirement() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p file_derived_retirement_plans;f files;d folders;v file_versions;uid uuid;sid uuid;b jsonb;is_execute boolean;count_ready int;channel text;
BEGIN
 IF TG_TABLE_NAME='file_derived_retirement_plans' THEN p:=NEW;is_execute:=true;
 ELSE SELECT * INTO p FROM file_derived_retirement_plans WHERE org_id=NEW.org_id AND id=NEW.plan_id;is_execute:=NEW.action='execute';END IF;
 uid:=NEW.actor_id;sid:=NEW.session_id;b:=p.basis::jsonb;
 SELECT * INTO f FROM files WHERE org_id=p.org_id AND id=p.file_id FOR UPDATE;
 SELECT * INTO d FROM folders WHERE org_id=p.org_id AND id=f.folder_id;
 SELECT * INTO v FROM file_versions WHERE org_id=p.org_id AND file_id=p.file_id AND id=p.version_id;
 SELECT s.device_type INTO channel FROM sessions s WHERE s.org_id=p.org_id AND s.id=sid AND s.user_id=uid;
 IF p.id IS NULL OR f.id IS NULL OR d.id IS NULL OR v.id IS NULL OR
 NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id WHERE s.org_id=p.org_id AND s.id=sid AND s.user_id=uid
  AND s.device_type IN ('web','wechat','douyin','android') AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
  AND s.mfa_verified_at>=clock_timestamp()-interval '15 minutes' AND u.status='active' AND s.auth_version=u.auth_version)
 OR NOT can_access(p.org_id,uid,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,channel,false)
 OR NOT can_access(p.org_id,uid,'file.retention.manage','file',f.id,d.project_id,f.creator_id,f.classification,channel,false) THEN
  RAISE EXCEPTION 'current human retention authority and MFA required' USING ERRCODE='23514';END IF;
 IF is_execute THEN
  SELECT count(*) INTO count_ready FROM file_text_derivations WHERE org_id=p.org_id AND version_id=p.version_id AND state='ready';
  IF v.availability<>'purged' OR f.current_version_id=p.version_id OR f.legal_hold OR (b->>'eligible')::boolean IS NOT TRUE
   OR (b->>'file_id'=f.id::text AND (b->>'file_version')::bigint=f.version AND b->>'version_id'=v.id::text AND b->>'source_sha256'=v.sha256) IS NOT TRUE
   OR NOT EXISTS(SELECT 1 FROM object_retirement_plans o WHERE o.org_id=p.org_id AND o.id::text=b->>'original_plan_id' AND o.file_id=f.id AND o.kind='history' AND o.target_id=v.id AND o.state='completed')
   OR NOT EXISTS(SELECT 1 FROM retention_policies rp WHERE rp.org_id=p.org_id AND rp.id::text=b->'policy'->>'id' AND rp.kind='formal_history' AND rp.enabled
    AND rp.version=(SELECT max(version) FROM retention_policies WHERE org_id=p.org_id AND kind='formal_history')
    AND p.ready_after>=p.created_at+make_interval(secs=>greatest(86400,rp.observation_seconds)))
   OR EXISTS(SELECT 1 FROM retention_holds h WHERE h.org_id=p.org_id AND h.file_id=f.id AND (h.valid_until IS NULL OR h.valid_until>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM retention_hold_releases r WHERE r.org_id=h.org_id AND r.hold_id=h.id))
   OR count_ready<1 OR count_ready>16 OR jsonb_array_length(b->'texts')<>(SELECT count(*) FROM file_text_derivations WHERE org_id=p.org_id AND version_id=v.id)
   OR EXISTS(SELECT 1 FROM file_text_derivations t WHERE t.org_id=p.org_id AND t.version_id=v.id AND (t.state='queued' OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements(b->'texts') x
    WHERE x->>'extractor'=t.extractor AND x->>'state'=t.state AND x->>'source_sha256'=t.source_sha256 AND x->>'text_sha256' IS NOT DISTINCT FROM t.text_sha256
    AND (x->>'characters')::integer IS NOT DISTINCT FROM t.character_count AND x->>'engine_sha256' IS NOT DISTINCT FROM t.engine_sha256
    AND (x->>'page_count')::int IS NOT DISTINCT FROM t.page_count AND (x->>'ocr_page_count')::int IS NOT DISTINCT FROM t.ocr_page_count
    AND (x->>'indexed_at')::timestamptz IS NOT DISTINCT FROM t.indexed_at))) THEN
   RAISE EXCEPTION 'derived retirement basis changed or original not purged' USING ERRCODE='23514';END IF;
  IF TG_TABLE_NAME='file_derived_retirement_decisions' THEN
   IF clock_timestamp()<p.ready_after OR clock_timestamp()>=p.expires_at OR NEW.removed_count<>count_ready THEN
    RAISE EXCEPTION 'exact reviewed count and observation required' USING ERRCODE='23514';END IF;
  END IF;
 END IF;RETURN NEW;
END;$$;
CREATE TRIGGER derived_retirement_plan_binding BEFORE INSERT ON file_derived_retirement_plans FOR EACH ROW EXECUTE FUNCTION validate_derived_retirement();
CREATE TRIGGER derived_retirement_decision_binding BEFORE INSERT ON file_derived_retirement_decisions FOR EACH ROW EXECUTE FUNCTION validate_derived_retirement();
CREATE OR REPLACE FUNCTION guard_file_text_derivation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND OLD.state='ready' AND NEW.state='blocked' AND NEW.error_code='FILE_TEXT_RETIRED'
  AND NEW.body IS NULL AND NEW.text_sha256 IS NULL AND NEW.character_count IS NULL AND NEW.page_count IS NULL AND NEW.ocr_page_count IS NULL
  AND ROW(NEW.org_id,NEW.version_id,NEW.file_id,NEW.extractor,NEW.source_sha256,NEW.engine_sha256,NEW.indexed_at,NEW.created_at)
   IS NOT DISTINCT FROM ROW(OLD.org_id,OLD.version_id,OLD.file_id,OLD.extractor,OLD.source_sha256,OLD.engine_sha256,OLD.indexed_at,OLD.created_at)
  AND EXISTS(SELECT 1 FROM file_derived_retirement_decisions d JOIN file_derived_retirement_plans p ON p.org_id=d.org_id AND p.id=d.plan_id
   WHERE p.org_id=OLD.org_id AND p.file_id=OLD.file_id AND p.version_id=OLD.version_id AND d.action='execute'
   AND EXISTS(SELECT 1 FROM jsonb_array_elements(p.basis::jsonb->'texts') x WHERE x->>'extractor'=OLD.extractor AND x->>'state'='ready' AND x->>'text_sha256'=OLD.text_sha256)) THEN RETURN NEW;END IF;
 IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND (OLD.state<>'queued' OR ROW(NEW.org_id,NEW.version_id,NEW.file_id,NEW.extractor,NEW.source_sha256,NEW.created_at)
  IS DISTINCT FROM ROW(OLD.org_id,OLD.version_id,OLD.file_id,OLD.extractor,OLD.source_sha256,OLD.created_at))) THEN
  RAISE EXCEPTION 'file text evidence immutable except exact reviewed retirement' USING ERRCODE='23514';END IF;
 IF TG_OP='INSERT' AND EXISTS(SELECT 1 FROM file_derived_retirement_decisions d JOIN file_derived_retirement_plans p ON p.org_id=d.org_id AND p.id=d.plan_id WHERE p.org_id=NEW.org_id AND p.version_id=NEW.version_id AND d.action='execute') THEN
  RAISE EXCEPTION 'retired version cannot recreate text derivations' USING ERRCODE='23514';END IF;
 IF NOT EXISTS(SELECT 1 FROM file_versions v WHERE v.org_id=NEW.org_id AND v.file_id=NEW.file_id AND v.id=NEW.version_id AND v.sha256=NEW.source_sha256) THEN
  RAISE EXCEPTION 'file text source must match formal version' USING ERRCODE='23514';END IF;RETURN NEW;
END;$$;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON file_derived_retirement_plans,file_derived_retirement_decisions TO copypaste_app;
 REVOKE UPDATE,DELETE ON file_derived_retirement_plans,file_derived_retirement_decisions FROM copypaste_app;
END IF;END;$$;
