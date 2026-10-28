-- Identity digests have a dedicated deployment key. Neither code, openid,
-- unionid nor platform session_key is persisted or placed in audit/outbox.
CREATE TABLE miniapp_code_claims (
 code_hash text PRIMARY KEY CHECK(length(code_hash)=64),
 platform text NOT NULL CHECK(platform IN ('wechat','douyin')),
 app_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE miniapp_bindings (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL,
 platform text NOT NULL CHECK(platform IN ('wechat','douyin')), app_id text NOT NULL,
 subject_hash text NOT NULL CHECK(length(subject_hash)=64),
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 revoked_at timestamptz, UNIQUE(org_id,id),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX miniapp_active_subject ON miniapp_bindings(platform,app_id,subject_hash) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX miniapp_active_user ON miniapp_bindings(org_id,user_id,platform,app_id) WHERE revoked_at IS NULL;
CREATE INDEX miniapp_own_history ON miniapp_bindings(org_id,user_id,created_at DESC,id DESC);
CREATE TABLE miniapp_binding_proofs (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL, session_id uuid NOT NULL REFERENCES sessions(id),
 platform text NOT NULL CHECK(platform IN ('wechat','douyin')), app_id text NOT NULL,
 subject_hash text NOT NULL CHECK(length(subject_hash)=64),
 auth_version bigint NOT NULL, org_auth_version bigint NOT NULL,
 expires_at timestamptz NOT NULL, consumed_at timestamptz, binding_id uuid REFERENCES miniapp_bindings(id),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
ALTER TABLE sessions ADD COLUMN miniapp_binding_id uuid;
ALTER TABLE sessions ADD CONSTRAINT session_miniapp_binding FOREIGN KEY(org_id,miniapp_binding_id) REFERENCES miniapp_bindings(org_id,id);
CREATE INDEX session_miniapp_source ON sessions(org_id,miniapp_binding_id) WHERE miniapp_binding_id IS NOT NULL;
ALTER TABLE auth_challenges ADD COLUMN miniapp_binding_id uuid;
ALTER TABLE auth_challenges ADD CONSTRAINT challenge_miniapp_binding FOREIGN KEY(org_id,miniapp_binding_id) REFERENCES miniapp_bindings(org_id,id);
CREATE FUNCTION guard_miniapp_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'binding history is immutable' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['version','revoked_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','revoked_at'])
  OR OLD.revoked_at IS NOT NULL OR NEW.revoked_at IS NULL OR NEW.version<>OLD.version+1 THEN
  RAISE EXCEPTION 'binding can only be revoked once' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER miniapp_binding_guard BEFORE UPDATE OR DELETE ON miniapp_bindings FOR EACH ROW EXECUTE FUNCTION guard_miniapp_binding();
CREATE TRIGGER miniapp_claim_immutable BEFORE UPDATE OR DELETE ON miniapp_code_claims FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON miniapp_code_claims,miniapp_bindings,miniapp_binding_proofs TO copypaste_app;
  GRANT UPDATE(version,revoked_at) ON miniapp_bindings TO copypaste_app;
  GRANT UPDATE(consumed_at,binding_id) ON miniapp_binding_proofs TO copypaste_app;
  REVOKE DELETE ON miniapp_code_claims,miniapp_bindings,miniapp_binding_proofs FROM copypaste_app;
 END IF;
END $$;
