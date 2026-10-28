-- Durable import provenance. Local files/checkpoints are never the unique mapping.
CREATE TABLE file_import_batches (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), owner_id uuid NOT NULL,
 source_namespace uuid NOT NULL, inventory_sha256 text NOT NULL CHECK(inventory_sha256~'^[0-9a-f]{64}$'),
 mapping_sha256 text NOT NULL CHECK(mapping_sha256~'^[0-9a-f]{64}$'), plan_sha256 text NOT NULL CHECK(plan_sha256~'^[0-9a-f]{64}$'),
 entry_count integer NOT NULL CHECK(entry_count BETWEEN 1 AND 1000000), total_bytes bigint NOT NULL CHECK(total_bytes>0),
 admitted_count integer NOT NULL DEFAULT 0, admitted_bytes bigint NOT NULL DEFAULT 0,
 admitted_chain text NOT NULL DEFAULT repeat('0',64), reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,owner_id,source_namespace,plan_sha256),
 FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
 CHECK(admitted_count BETWEEN 0 AND entry_count AND admitted_bytes BETWEEN 0 AND total_bytes)
);
CREATE TABLE file_import_entries (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, batch_id uuid NOT NULL, ordinal integer NOT NULL,
 source_namespace uuid NOT NULL, source_key text NOT NULL CHECK(source_key~'^[0-9a-f]{64}$'),
 source_sha256 text NOT NULL CHECK(source_sha256~'^[0-9a-f]{64}$'), size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 120000000000),
 folder_id uuid NOT NULL, folder_version bigint NOT NULL CHECK(folder_version>0), project_id uuid, classification text NOT NULL,
 mapped_owner_id uuid NOT NULL, input jsonb NOT NULL, entry_sha256 text NOT NULL CHECK(entry_sha256~'^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,id), UNIQUE(org_id,batch_id,ordinal),
 UNIQUE(org_id,batch_id,source_key),
 FOREIGN KEY(org_id,batch_id) REFERENCES file_import_batches(org_id,id),
 FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
 FOREIGN KEY(org_id,mapped_owner_id) REFERENCES users(org_id,id), CHECK(octet_length(input::text)<32768)
);
CREATE TABLE file_import_approvals (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, batch_id uuid NOT NULL, owner_id uuid NOT NULL,
 org_auth_version bigint NOT NULL, user_auth_version bigint NOT NULL, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '48 hours',
 UNIQUE(org_id,id), FOREIGN KEY(org_id,batch_id) REFERENCES file_import_batches(org_id,id),
 FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id)
);
CREATE TABLE file_import_objects (
 org_id uuid NOT NULL, source_namespace uuid NOT NULL, source_key text NOT NULL,
 owner_id uuid NOT NULL, file_id uuid, current_upload_id uuid,
 PRIMARY KEY(org_id,source_namespace,source_key),
 FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,file_id) REFERENCES files(org_id,id),
 FOREIGN KEY(org_id,current_upload_id) REFERENCES upload_sessions(org_id,id)
);
CREATE TABLE file_import_attempts (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, entry_id uuid NOT NULL, upload_id uuid NOT NULL,
 reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,id), UNIQUE(org_id,entry_id,upload_id),
 FOREIGN KEY(org_id,entry_id) REFERENCES file_import_entries(org_id,id),
 FOREIGN KEY(org_id,upload_id) REFERENCES upload_sessions(org_id,id)
);
CREATE INDEX file_import_batch_owner ON file_import_batches(org_id,owner_id,created_at DESC,id DESC);
CREATE INDEX file_import_approval_current ON file_import_approvals(org_id,batch_id,created_at DESC,id DESC);
CREATE INDEX file_import_attempt_entry ON file_import_attempts(org_id,entry_id,created_at DESC,id DESC);
CREATE INDEX file_import_attempt_upload ON file_import_attempts(org_id,upload_id);

CREATE FUNCTION guard_file_import_entry() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b file_import_batches; next_chain text;
BEGIN
 SELECT * INTO b FROM file_import_batches WHERE org_id=NEW.org_id AND id=NEW.batch_id FOR UPDATE;
 IF NOT FOUND OR NEW.source_namespace<>b.source_namespace OR NEW.ordinal<>b.admitted_count+1 OR b.admitted_count>=b.entry_count
  OR b.admitted_bytes+NEW.size_bytes>b.total_bytes OR EXISTS(SELECT 1 FROM file_import_approvals WHERE org_id=b.org_id AND batch_id=b.id) THEN
  RAISE EXCEPTION 'invalid import manifest sequence' USING ERRCODE='23514'; END IF;
 IF NEW.input->>'source_key' IS DISTINCT FROM NEW.source_key OR NEW.input->>'source_sha256' IS DISTINCT FROM NEW.source_sha256
  OR (NEW.input->>'ordinal')::integer IS DISTINCT FROM NEW.ordinal OR (NEW.input->>'size_bytes')::bigint IS DISTINCT FROM NEW.size_bytes
  OR NEW.input->>'folder_id' IS DISTINCT FROM NEW.folder_id::text OR (NEW.input->>'folder_version')::bigint IS DISTINCT FROM NEW.folder_version
  OR NEW.input->>'project_id' IS DISTINCT FROM NEW.project_id::text OR NEW.input->>'classification' IS DISTINCT FROM NEW.classification
  OR NEW.input->>'mapped_owner_id' IS DISTINCT FROM NEW.mapped_owner_id::text THEN
  RAISE EXCEPTION 'import manifest fields differ' USING ERRCODE='23514'; END IF;
 next_chain:=encode(sha256(decode(b.admitted_chain,'hex')||decode(NEW.entry_sha256,'hex')),'hex');
 IF NEW.ordinal=b.entry_count AND (next_chain<>b.plan_sha256 OR b.admitted_bytes+NEW.size_bytes<>b.total_bytes) THEN
  RAISE EXCEPTION 'import manifest digest differs' USING ERRCODE='23514'; END IF;
 UPDATE file_import_batches SET admitted_count=NEW.ordinal,admitted_bytes=admitted_bytes+NEW.size_bytes,admitted_chain=next_chain
  WHERE org_id=b.org_id AND id=b.id;
 RETURN NEW;
END; $$;
CREATE FUNCTION guard_file_import_batch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR pg_trigger_depth()<2 OR (to_jsonb(NEW)-ARRAY['admitted_count','admitted_bytes','admitted_chain']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['admitted_count','admitted_bytes','admitted_chain']) THEN
  RAISE EXCEPTION 'import batch provenance is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE FUNCTION guard_file_import_approval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b file_import_batches;
BEGIN
 SELECT * INTO b FROM file_import_batches WHERE org_id=NEW.org_id AND id=NEW.batch_id FOR UPDATE;
 IF NOT FOUND OR NEW.owner_id<>b.owner_id OR b.admitted_count<>b.entry_count OR b.admitted_bytes<>b.total_bytes OR b.admitted_chain<>b.plan_sha256
  OR NEW.expires_at>clock_timestamp()+interval '48 hours 1 minute' OR NEW.expires_at<=NEW.created_at THEN
  RAISE EXCEPTION 'approval requires complete exact import manifest' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
CREATE FUNCTION guard_file_import_object() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE u upload_sessions;
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-ARRAY['file_id','current_upload_id']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['file_id','current_upload_id'])
  OR (OLD.file_id IS NOT NULL AND NEW.file_id IS DISTINCT FROM OLD.file_id) THEN
  RAISE EXCEPTION 'import source identity is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.current_upload_id IS NOT NULL THEN
  SELECT * INTO u FROM upload_sessions WHERE org_id=NEW.org_id AND id=NEW.current_upload_id;
  IF NOT FOUND OR u.owner_id<>NEW.owner_id OR NOT EXISTS(SELECT 1 FROM file_import_attempts a JOIN file_import_entries e ON e.org_id=a.org_id AND e.id=a.entry_id
   WHERE a.org_id=NEW.org_id AND a.upload_id=u.id AND e.source_namespace=NEW.source_namespace AND e.source_key=NEW.source_key)
   OR (NEW.file_id IS NOT NULL AND NEW.file_id IS DISTINCT FROM COALESCE(u.file_id,(SELECT file_id FROM file_versions WHERE org_id=u.org_id AND id=u.committed_version_id))) THEN
   RAISE EXCEPTION 'import source requires its exact upload' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE FUNCTION guard_file_import_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE e file_import_entries; o file_import_objects; v file_versions;
BEGIN
 IF NEW.state='committed' AND OLD.state<>'committed' THEN
  SELECT entry.* INTO e FROM file_import_entries entry JOIN file_import_attempts a ON a.org_id=entry.org_id AND a.entry_id=entry.id
   WHERE a.org_id=NEW.org_id AND a.upload_id=NEW.id ORDER BY a.created_at LIMIT 1;
  IF FOUND THEN
   SELECT * INTO o FROM file_import_objects WHERE org_id=e.org_id AND source_namespace=e.source_namespace AND source_key=e.source_key FOR UPDATE;
   SELECT * INTO v FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.committed_version_id;
   IF o.current_upload_id IS DISTINCT FROM NEW.id OR v.id IS NULL OR v.sha256<>e.source_sha256 OR v.size_bytes<>e.size_bytes
    OR (o.file_id IS NOT NULL AND o.file_id<>v.file_id) THEN
    RAISE EXCEPTION 'stale or mismatching import upload cannot commit' USING ERRCODE='23514'; END IF;
   UPDATE file_import_objects SET file_id=v.file_id WHERE org_id=o.org_id AND source_namespace=o.source_namespace AND source_key=o.source_key;
  END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER file_import_entry_guard BEFORE INSERT ON file_import_entries FOR EACH ROW EXECUTE FUNCTION guard_file_import_entry();
CREATE TRIGGER file_import_entry_immutable BEFORE UPDATE OR DELETE ON file_import_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER file_import_batch_guard BEFORE UPDATE OR DELETE ON file_import_batches FOR EACH ROW EXECUTE FUNCTION guard_file_import_batch();
CREATE TRIGGER file_import_approval_guard BEFORE INSERT ON file_import_approvals FOR EACH ROW EXECUTE FUNCTION guard_file_import_approval();
CREATE TRIGGER file_import_approval_immutable BEFORE UPDATE OR DELETE ON file_import_approvals FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER file_import_attempt_immutable BEFORE UPDATE OR DELETE ON file_import_attempts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER file_import_object_guard BEFORE UPDATE OR DELETE ON file_import_objects FOR EACH ROW EXECUTE FUNCTION guard_file_import_object();
CREATE TRIGGER file_import_commit_guard AFTER UPDATE OF state ON upload_sessions FOR EACH ROW EXECUTE FUNCTION guard_file_import_commit();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'file_importer','文件迁移操作人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('file.import'),('folder.read'),('file.read'),('file.upload'),('file.update')) a(action) WHERE key='file_importer' ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'file.import' FROM roles WHERE key='admin' ON CONFLICT DO NOTHING;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON file_import_batches,file_import_entries,file_import_approvals,file_import_objects,file_import_attempts TO copypaste_app;
  GRANT UPDATE(admitted_count,admitted_bytes,admitted_chain) ON file_import_batches TO copypaste_app;
  GRANT UPDATE(file_id,current_upload_id) ON file_import_objects TO copypaste_app;
  REVOKE DELETE ON file_import_batches,file_import_entries,file_import_approvals,file_import_objects,file_import_attempts FROM copypaste_app;
 END IF;
END; $$;
