-- Explicit organization metadata authority is separate from ordinary file access.
CREATE FUNCTION valid_storage_replica_snapshot(jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE
 SET search_path=pg_catalog,public AS $$
 SELECT coalesce(jsonb_typeof($1)='object'
 AND $1 ?& ARRAY['id','version','provider','state','file_id','file_version_id','file_revision','file_state','availability','is_current',
  'storage_epoch','primary_provider','source_available','error_code','job','created_at','updated_at']
 AND ($1-ARRAY['id','version','provider','state','file_id','file_version_id','file_revision','file_state','availability','is_current',
  'storage_epoch','primary_provider','source_available','error_code','job','created_at','updated_at'])='{}'::jsonb
 AND $1->>'id' ~ '^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$'
 AND $1->>'file_id' ~ '^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$'
 AND $1->>'file_version_id' ~ '^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$'
 AND jsonb_typeof($1->'version')='number' AND jsonb_typeof($1->'file_revision')='number' AND jsonb_typeof($1->'storage_epoch')='number'
 AND $1->>'provider' IN ('oss','r2') AND $1->>'primary_provider' IN ('oss','r2')
 AND $1->>'state' IN ('queued','copying','verifying','verified','retry_wait','failed','missing')
 AND $1->>'file_state' IN ('active','trashed')
 AND jsonb_typeof($1->'is_current')='boolean' AND jsonb_typeof($1->'source_available')='boolean'
 AND $1->>'version' ~ '^[1-9][0-9]*$' AND $1->>'file_revision' ~ '^[1-9][0-9]*$' AND $1->>'storage_epoch' ~ '^[1-9][0-9]*$'
 AND length($1->>'availability') BETWEEN 1 AND 64 AND length($1->>'created_at') BETWEEN 20 AND 40 AND length($1->>'updated_at') BETWEEN 20 AND 40
 AND ($1->'error_code'='null'::jsonb OR $1->>'error_code' IN ('REPLICA_HASH_MISMATCH','REPLICA_SIZE_MISMATCH','REPLICA_SOURCE_UNAVAILABLE',
  'REPLICA_SOURCE_MISMATCH','STORAGE_EPOCH_CHANGED','STORAGE_CONFIGURATION_CHANGED','STORAGE_TARGETS_NOT_CONFIGURED','STORAGE_UNAVAILABLE',
  'OBJECT_NOT_FOUND','LEASE_EXHAUSTED','ACCESS_DENIED','RETRY_EXHAUSTED','PROVIDER_UNAVAILABLE','OTHER_STORAGE_FAILURE'))
 AND ($1->'job'='null'::jsonb OR (jsonb_typeof($1->'job')='object'
  AND ($1->'job') ?& ARRAY['id','state','attempt','max_attempts','updated_at']
  AND (($1->'job')-ARRAY['id','state','attempt','max_attempts','updated_at'])='{}'::jsonb
  AND $1->'job'->>'state' IN ('queued','running','succeeded','retry_wait','dead','cancelled')
  AND $1->'job'->>'id' ~ '^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$'
  AND jsonb_typeof($1->'job'->'attempt')='number' AND jsonb_typeof($1->'job'->'max_attempts')='number'
  AND length($1->'job'->>'updated_at') BETWEEN 20 AND 40
  AND $1->'job'->>'attempt' ~ '^[0-9]+$' AND $1->'job'->>'max_attempts' ~ '^[1-9][0-9]*$')),false);
$$;

CREATE TABLE storage_replica_retries (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, replica_id uuid NOT NULL, file_id uuid NOT NULL, file_version_id uuid NOT NULL,
 actor_id uuid NOT NULL, session_id uuid NOT NULL, business_operation_id uuid NOT NULL UNIQUE REFERENCES idempotency_records(id),
 expected_version bigint NOT NULL CHECK(expected_version>0), result_version bigint NOT NULL CHECK(result_version=expected_version+1),
 basis_sha256 text NOT NULL CHECK(basis_sha256 ~ '^[a-f0-9]{64}$'),
 before_text text NOT NULL CHECK(octet_length(before_text)<=16384 AND valid_storage_replica_snapshot(before_text::jsonb)),
 after_data jsonb NOT NULL CHECK(octet_length(after_data::text)<=16384 AND valid_storage_replica_snapshot(after_data)),
 normal_before_sha256 text NOT NULL CHECK(normal_before_sha256 ~ '^[a-f0-9]{64}$'),
 normal_after_sha256 text NOT NULL CHECK(normal_after_sha256 ~ '^[a-f0-9]{64}$'),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(org_id,replica_id) REFERENCES file_replicas(org_id,id),
 FOREIGN KEY(org_id,file_id,file_version_id) REFERENCES file_versions(org_id,file_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id),
 UNIQUE(org_id,replica_id,result_version),
 CHECK(basis_sha256=encode(sha256(convert_to(before_text,'UTF8')),'hex')),
 CHECK((before_text::jsonb)->>'id'=replica_id::text AND after_data->>'id'=replica_id::text
  AND (before_text::jsonb)->>'file_id'=file_id::text AND after_data->>'file_id'=file_id::text
  AND (before_text::jsonb)->>'file_version_id'=file_version_id::text AND after_data->>'file_version_id'=file_version_id::text
  AND ((before_text::jsonb)->>'version')::bigint=expected_version AND (after_data->>'version')::bigint=result_version
  AND (before_text::jsonb)->>'state'<>'verified' AND (before_text::jsonb)->>'availability' NOT IN ('retiring','purged')
  AND (before_text::jsonb)->>'provider'=after_data->>'provider'
  AND after_data->>'state'='queued' AND after_data->'job'='null'::jsonb AND after_data->'error_code'='null'::jsonb)
);
CREATE TRIGGER storage_replica_retry_immutable BEFORE UPDATE OR DELETE ON storage_replica_retries
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION bind_storage_replica_retry() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
 SET search_path=pg_catalog,public AS $$
DECLARE r public.file_replicas; v public.file_versions; f public.files; d public.folders;
BEGIN
 SELECT * INTO r FROM public.file_replicas WHERE org_id=NEW.org_id AND id=NEW.replica_id FOR UPDATE;
 SELECT * INTO v FROM public.file_versions WHERE org_id=NEW.org_id AND id=r.version_id;
 SELECT * INTO f FROM public.files WHERE org_id=NEW.org_id AND id=v.file_id;
 SELECT * INTO d FROM public.folders WHERE org_id=NEW.org_id AND id=f.folder_id;
 IF r.id IS NULL OR v.id<>NEW.file_version_id OR f.id<>NEW.file_id OR r.version<>NEW.result_version OR r.state<>'queued'
  OR r.active_job_id IS NOT NULL OR r.last_error IS NOT NULL OR r.actual_sha256 IS NOT NULL OR r.source_id IS NOT NULL OR r.verified_at IS NOT NULL
  OR NEW.after_data->>'provider'<>r.provider OR (NEW.after_data->>'updated_at')::timestamptz<>r.updated_at
  OR (NEW.after_data->>'file_revision')::bigint<>f.version OR NEW.after_data->>'availability'<>v.availability
  OR NOT EXISTS(SELECT 1 FROM public.idempotency_records o WHERE o.org_id=NEW.org_id AND o.id=NEW.business_operation_id
   AND o.actor_id=NEW.actor_id AND o.state='succeeded' AND o.route_key='/internal/storage-replica-retry/'||r.id::text
   AND o.idempotency_key ~ '^storage-retry:[a-f0-9]{64}$'
   AND o.response_data->>'id'=r.id::text AND (o.response_data->>'version')::bigint=r.version
   AND o.response_data->>'provider'=r.provider AND o.response_data->>'state'='queued')
  OR NOT EXISTS(SELECT 1 FROM public.audit_events a WHERE a.org_id=NEW.org_id AND a.actor_id=NEW.actor_id AND a.session_id=NEW.session_id
   AND a.action='replica.retry.requested' AND a.resource_id=r.id AND a.resource_version=r.version
   AND a.before_digest=NEW.normal_before_sha256 AND a.after_digest=NEW.normal_after_sha256)
  OR NOT EXISTS(SELECT 1 FROM public.sessions s JOIN public.users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id AND u.status='active' AND s.auth_version=u.auth_version
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.device_type IN ('web','wechat','douyin','android')
   AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
   AND public.can_access(NEW.org_id,NEW.actor_id,'storage.operations.read','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)
   AND public.can_access(NEW.org_id,NEW.actor_id,'storage.operations.retry','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)
   AND public.can_access(NEW.org_id,NEW.actor_id,'file.update','file',f.id,d.project_id,f.creator_id,f.classification,s.device_type,false)) THEN
  RAISE EXCEPTION 'storage retry requires the exact ordinary command, audit and authorized human session' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION bind_storage_replica_retry() FROM PUBLIC;
CREATE TRIGGER storage_replica_retry_binding BEFORE INSERT ON storage_replica_retries
 FOR EACH ROW EXECUTE FUNCTION bind_storage_replica_retry();
CREATE INDEX storage_replica_retry_page ON storage_replica_retries(org_id,replica_id,created_at DESC,id DESC);
CREATE INDEX storage_replica_operations_page ON file_replicas(org_id,created_at DESC,id DESC);
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'storage_operations_reader','文件复制运维查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'storage_operations_operator','文件复制运维操作人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'storage.operations.read' FROM roles WHERE key IN ('storage_operations_reader','storage_operations_operator','system_admin');
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'storage.operations.retry' FROM roles WHERE key IN ('storage_operations_operator','system_admin');
UPDATE organizations SET authorization_version=authorization_version+1;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON storage_replica_retries TO copypaste_app;
END IF; END $$;
