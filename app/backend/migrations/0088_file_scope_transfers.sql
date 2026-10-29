-- Human-reviewed namespace transfers. Stored plans and final receipts preserve
-- the exact inventory and access analysis; object versions are never rewritten.
CREATE TABLE file_scope_transfers (
 id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
 creator_id uuid NOT NULL, origin_session_id uuid NOT NULL, origin_channel text NOT NULL
   CHECK(origin_channel IN ('web','wechat','douyin','android')),
 actor_auth_version bigint NOT NULL CHECK(actor_auth_version>0),
 organization_version bigint NOT NULL CHECK(organization_version>0),
 resource_kind text NOT NULL CHECK(resource_kind IN ('file','folder')), resource_id uuid NOT NULL,
 input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 plan_hash text NOT NULL CHECK(plan_hash~'^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','executed','cancelled')),
 version bigint NOT NULL DEFAULT 1 CHECK(version IN (1,2)),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 completed_at timestamptz, decision_reason text, decision_session_id uuid, decision_channel text,
 UNIQUE(org_id,id), FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,origin_session_id) REFERENCES sessions(org_id,id),
 FOREIGN KEY(org_id,decision_session_id) REFERENCES sessions(org_id,id),
 CHECK((input->>'kind')=resource_kind AND (input->>'resource_id')::uuid=resource_id
   AND (snapshot->'source'->>'id')::uuid=resource_id AND jsonb_array_length(snapshot->'items') BETWEEN 1 AND 100),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '30 minutes'),
 CHECK((state='pending' AND version=1 AND completed_at IS NULL AND decision_reason IS NULL AND decision_session_id IS NULL AND decision_channel IS NULL)
   OR (state IN ('executed','cancelled') AND version=2 AND completed_at IS NOT NULL
       AND length(trim(decision_reason)) BETWEEN 1 AND 200 AND decision_session_id IS NOT NULL
       AND decision_channel IN ('web','wechat','douyin','android')))
);
CREATE INDEX file_scope_transfer_owner ON file_scope_transfers(org_id,creator_id,resource_kind,resource_id,created_at DESC,id DESC);
CREATE FUNCTION guard_file_scope_transfer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'scope transfer evidence is immutable' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['version','state','completed_at','decision_reason','decision_session_id','decision_channel'])
      IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['version','state','completed_at','decision_reason','decision_session_id','decision_channel'])
    OR OLD.state<>'pending' OR NEW.state NOT IN ('executed','cancelled') OR NEW.version<>OLD.version+1 THEN
   RAISE EXCEPTION 'only a pending transfer may receive one final decision' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM sessions s WHERE s.org_id=NEW.org_id AND s.id=NEW.decision_session_id
   AND s.user_id=NEW.creator_id AND s.device_type=NEW.decision_channel AND s.revoked_at IS NULL
   AND s.expires_at>clock_timestamp() AND (NEW.state<>'executed' OR
     (s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<=clock_timestamp()+interval '1 minute'))) THEN
   RAISE EXCEPTION 'scope decision requires its actual current human session' USING ERRCODE='23514';
 END IF;
 IF NEW.state='executed' THEN
   IF NEW.expires_at<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM organizations o
      WHERE o.id=NEW.org_id AND o.authorization_version=NEW.organization_version+1)
     OR EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.snapshot->'items') i WHERE
       classification_rank(i->>'new_classification')<classification_rank(i->>'classification')
       OR jsonb_array_length(i->'blocking_reasons')<>0
       OR (i->>'kind'='folder' AND NOT EXISTS(SELECT 1 FROM folders f WHERE f.org_id=NEW.org_id AND f.id=(i->>'id')::uuid
         AND f.version=(i->>'version')::bigint+1 AND f.project_id IS NOT DISTINCT FROM (NEW.snapshot->'target'->>'project_id')::uuid
         AND f.classification=i->>'new_classification' AND f.state=i->>'state'
         AND (f.id<>NEW.resource_id OR (f.parent_id=(NEW.snapshot->'target'->>'id')::uuid AND f.name=NEW.snapshot->>'new_name'))))
       OR (i->>'kind'='file' AND NOT EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
         WHERE f.org_id=NEW.org_id AND f.id=(i->>'id')::uuid AND f.version=(i->>'version')::bigint+1
         AND d.project_id IS NOT DISTINCT FROM (NEW.snapshot->'target'->>'project_id')::uuid
         AND f.classification=i->>'new_classification' AND f.state=i->>'state'
         AND f.current_version_id IS NOT DISTINCT FROM (i->>'current_version_id')::uuid
         AND (f.id<>NEW.resource_id OR (f.folder_id=(NEW.snapshot->'target'->>'id')::uuid AND f.name=NEW.snapshot->>'new_name'))))) THEN
     RAISE EXCEPTION 'scope execution requires its exact completed inventory and authorization invalidation' USING ERRCODE='23514';
   END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER file_scope_transfer_guard BEFORE UPDATE OR DELETE ON file_scope_transfers
 FOR EACH ROW EXECUTE FUNCTION guard_file_scope_transfer();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
   GRANT SELECT,INSERT,UPDATE ON file_scope_transfers TO copypaste_app;
 END IF;
END; $$;
