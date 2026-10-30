-- Explicit human batches compose existing reversible namespace commands.
-- Exact UTF-8 input and snapshot bytes bind analysis and its one final receipt.
CREATE TABLE file_batch_plans (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 creator_id uuid NOT NULL, origin_session_id uuid NOT NULL,
 origin_channel text NOT NULL CHECK(origin_channel IN ('web','wechat','douyin','android')),
 actor_auth_version bigint NOT NULL CHECK(actor_auth_version>0),
 organization_version bigint NOT NULL CHECK(organization_version>0),
 input text NOT NULL CHECK(jsonb_typeof(input::jsonb)='object'),
 snapshot text NOT NULL CHECK(jsonb_typeof(snapshot::jsonb)='object'),
 plan_hash text NOT NULL CHECK(plan_hash~'^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','executed','cancelled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version IN (1,2)),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 completed_at timestamptz, decision_reason text, decision_session_id uuid, decision_channel text,
 results text CHECK(results IS NULL OR jsonb_typeof(results::jsonb)='array'),
 UNIQUE(org_id,id), FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,origin_session_id) REFERENCES sessions(org_id,id),
 FOREIGN KEY(org_id,decision_session_id) REFERENCES sessions(org_id,id),
 CHECK(octet_length(input)+octet_length(snapshot)<=524288),
 CHECK(encode(sha256(convert_to(input||E'\n'||snapshot,'UTF8')),'hex')=plan_hash),
 CHECK(input::jsonb ?& ARRAY['action','files','target','reason']),
 CHECK(snapshot::jsonb ?& ARRAY['items','target']),
 CHECK(input::jsonb->>'action' IN ('move','trash','restore')),
 CHECK(jsonb_typeof(input::jsonb->'files')='array' AND jsonb_array_length(input::jsonb->'files') BETWEEN 1 AND 50),
 CHECK(jsonb_typeof(snapshot::jsonb->'items')='array' AND jsonb_array_length(snapshot::jsonb->'items')=jsonb_array_length(input::jsonb->'files')),
 CHECK(length(trim(input::jsonb->>'reason')) BETWEEN 1 AND 200),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '30 minutes'),
 CHECK((state='pending' AND version=1 AND completed_at IS NULL AND decision_reason IS NULL
        AND decision_session_id IS NULL AND decision_channel IS NULL AND results IS NULL)
   OR (state IN ('executed','cancelled') AND version=2 AND completed_at IS NOT NULL
       AND length(trim(decision_reason)) BETWEEN 1 AND 200 AND decision_session_id IS NOT NULL
       AND decision_channel IN ('web','wechat','douyin','android')
       AND ((state='executed' AND results IS NOT NULL) OR (state='cancelled' AND results IS NULL))))
);
CREATE INDEX file_batch_plan_owner ON file_batch_plans(org_id,creator_id,created_at DESC,id DESC);

CREATE FUNCTION guard_file_batch_plan() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE basis jsonb; source jsonb; selection jsonb; action text; answer jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'file batch evidence is immutable' USING ERRCODE='23514'; END IF;
 basis:=NEW.snapshot::jsonb; action:=NEW.input::jsonb->>'action';
 IF TG_OP='INSERT' THEN
   IF NEW.state<>'pending' OR NEW.version<>1 OR NOT EXISTS(
     SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
     JOIN organizations o ON o.id=s.org_id
     WHERE s.org_id=NEW.org_id AND s.id=NEW.origin_session_id AND s.user_id=NEW.creator_id
       AND s.device_type=NEW.origin_channel AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
       AND s.auth_version=u.auth_version AND u.status='active' AND u.auth_version=NEW.actor_auth_version
       AND o.authorization_version=NEW.organization_version)
     OR (action='move') IS DISTINCT FROM (NEW.input::jsonb->'target'<>'null'::jsonb)
     OR (action='move') IS DISTINCT FROM (basis->'target'<>'null'::jsonb)
     OR (SELECT count(DISTINCT i->>'id') FROM jsonb_array_elements(NEW.input::jsonb->'files') i)
        <>jsonb_array_length(NEW.input::jsonb->'files') THEN
     RAISE EXCEPTION 'file batch requires a real human and an exact unique inventory' USING ERRCODE='23514';
   END IF;
   FOR source,selection IN SELECT i,s FROM jsonb_array_elements(basis->'items') WITH ORDINALITY b(i,n)
       JOIN jsonb_array_elements(NEW.input::jsonb->'files') WITH ORDINALITY a(s,n) USING(n) LOOP
     IF NOT (source->'file' ?& ARRAY['id','version','name','folder_id','project_id','creator_id','classification','current_version_id','state'])
       OR jsonb_typeof(source->'path') IS DISTINCT FROM 'array'
       OR jsonb_array_length(source->'path') NOT BETWEEN 1 AND 32
       OR jsonb_typeof(source->'blocking_reasons') IS DISTINCT FROM 'array'
       OR jsonb_typeof(source->'retention_held') IS DISTINCT FROM 'boolean'
       OR source->'file'->>'id' IS DISTINCT FROM selection->>'id'
       OR source->'file'->'version' IS DISTINCT FROM selection->'expected_version'
       OR source->'file'->'current_version_id' IS DISTINCT FROM selection->'current_version_id'
       OR NOT EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
          WHERE f.org_id=NEW.org_id AND f.id=(source->'file'->>'id')::uuid
          AND jsonb_build_object('id',f.id,'version',f.version,'name',f.name,'folder_id',f.folder_id,
            'project_id',d.project_id,'creator_id',f.creator_id,'classification',f.classification,
            'current_version_id',f.current_version_id,'state',f.state)=source->'file') THEN
       RAISE EXCEPTION 'file batch source must match its exact current metadata' USING ERRCODE='23514';
     END IF;
   END LOOP;
   RETURN NEW;
 END IF;
 IF (to_jsonb(NEW)-ARRAY['version','state','completed_at','decision_reason','decision_session_id','decision_channel','results'])
     IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','state','completed_at','decision_reason','decision_session_id','decision_channel','results'])
   OR OLD.state<>'pending' OR NEW.state NOT IN ('executed','cancelled') OR NEW.version<>OLD.version+1 THEN
   RAISE EXCEPTION 'a fixed file batch permits only one final decision' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.decision_session_id AND s.user_id=NEW.creator_id
     AND s.device_type=NEW.decision_channel AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
     AND s.auth_version=u.auth_version AND u.status='active' AND (NEW.state<>'executed' OR
       (u.auth_version=NEW.actor_auth_version AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes'
        AND s.mfa_verified_at<=clock_timestamp()+interval '1 minute'))) THEN
   RAISE EXCEPTION 'file batch decision requires its actual current human session' USING ERRCODE='23514';
 END IF;
 IF NEW.state='executed' THEN
   IF NEW.expires_at<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM organizations
       WHERE id=NEW.org_id AND authorization_version=NEW.organization_version)
     OR jsonb_array_length(NEW.results::jsonb)<>jsonb_array_length(basis->'items') THEN
     RAISE EXCEPTION 'file batch execution requires current unexpired authority and complete results' USING ERRCODE='23514';
   END IF;
   FOR source,answer IN SELECT i,r FROM jsonb_array_elements(basis->'items') WITH ORDINALITY b(i,n)
       JOIN jsonb_array_elements(NEW.results::jsonb) WITH ORDINALITY a(r,n) USING(n) LOOP
     IF jsonb_array_length(source->'blocking_reasons')<>0 OR NOT EXISTS(
       SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
       WHERE f.org_id=NEW.org_id AND f.id=(source->'file'->>'id')::uuid
         AND f.version=(source->'file'->>'version')::bigint+1
         AND f.name=source->'file'->>'name' AND f.classification=source->'file'->>'classification'
         AND d.project_id IS NOT DISTINCT FROM (source->'file'->>'project_id')::uuid
         AND f.creator_id IS NOT DISTINCT FROM (source->'file'->>'creator_id')::uuid
         AND f.current_version_id IS NOT DISTINCT FROM (source->'file'->>'current_version_id')::uuid
         AND f.state=CASE WHEN action='trash' THEN 'trashed' ELSE 'active' END
         AND f.folder_id=CASE WHEN action='move' THEN (basis->'target'->'folder'->>'id')::uuid
                             ELSE (source->'file'->>'folder_id')::uuid END
         AND jsonb_build_object('id',f.id,'version',f.version,'name',f.name,'folder_id',f.folder_id,
           'project_id',d.project_id,'creator_id',f.creator_id,'classification',f.classification,
           'current_version_id',f.current_version_id,'state',f.state)=answer) THEN
       RAISE EXCEPTION 'file batch receipt requires every exact ordinary file result' USING ERRCODE='23514';
     END IF;
   END LOOP;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER file_batch_plan_guard BEFORE INSERT OR UPDATE OR DELETE ON file_batch_plans
 FOR EACH ROW EXECUTE FUNCTION guard_file_batch_plan();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
   GRANT SELECT,INSERT,UPDATE ON file_batch_plans TO copypaste_app;
   REVOKE DELETE ON file_batch_plans FROM copypaste_app;
 END IF;
END; $$;
