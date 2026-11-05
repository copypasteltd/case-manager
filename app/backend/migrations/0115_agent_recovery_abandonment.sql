-- A human relinquishes only the exact stopped draft's future recovery right.
-- The ordinary known-object retirement protocol still owns deletion and proofs.
CREATE TABLE agent_file_abandonments (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, run_id uuid NOT NULL, draft_id uuid NOT NULL,
 run_version bigint NOT NULL CHECK(run_version>0), upload_version bigint NOT NULL CHECK(upload_version>0),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[a-f0-9]{64}$'),
 actor_id uuid NOT NULL, session_id uuid NOT NULL, reviewed boolean NOT NULL CHECK(reviewed),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,draft_id), FOREIGN KEY(org_id,run_id,draft_id) REFERENCES agent_file_drafts(org_id,run_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE TRIGGER agent_file_abandonment_immutable BEFORE UPDATE OR DELETE ON agent_file_abandonments
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_agent_file_abandonment() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE d public.agent_file_drafts; r public.agent_runs; u public.upload_sessions; f public.folders;
BEGIN
 PERFORM id FROM public.organizations WHERE id=NEW.org_id FOR UPDATE;
 SELECT * INTO d FROM public.agent_file_drafts WHERE org_id=NEW.org_id AND id=NEW.draft_id AND run_id=NEW.run_id;
 IF NOT FOUND OR d.content_sha256<>NEW.content_sha256 THEN RAISE EXCEPTION 'exact draft required' USING ERRCODE='23514'; END IF;
 SELECT * INTO r FROM public.agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id FOR UPDATE;
 SELECT * INTO u FROM public.upload_sessions WHERE org_id=NEW.org_id AND id=NEW.draft_id FOR UPDATE;
 SELECT * INTO f FROM public.folders WHERE org_id=NEW.org_id AND id=u.target_folder_id;
 IF r.actor_id<>NEW.actor_id OR r.version<>NEW.run_version OR r.state NOT IN ('succeeded','failed','cancelled')
  OR u.owner_id<>NEW.actor_id OR u.version<>NEW.upload_version OR u.state='committed' OR u.committed_version_id IS NOT NULL OR u.retirement_plan_id IS NOT NULL
  OR EXISTS(SELECT 1 FROM public.agent_file_commits WHERE org_id=NEW.org_id AND run_id=NEW.run_id)
  OR NOT EXISTS(SELECT 1 FROM public.conversations c WHERE c.org_id=r.org_id AND c.id=r.conversation_id AND c.type='private' AND c.creator_id=NEW.actor_id)
  OR EXISTS(WITH RECURSIVE descendants AS (
   SELECT run_id FROM public.agent_file_recoveries WHERE org_id=NEW.org_id AND source_draft_id=NEW.draft_id
   UNION SELECT child.run_id FROM public.agent_file_recoveries child JOIN descendants p ON p.run_id=child.source_run_id WHERE child.org_id=NEW.org_id)
   SELECT 1 FROM descendants p JOIN public.agent_runs ar ON ar.org_id=NEW.org_id AND ar.id=p.run_id WHERE ar.state IN ('queued','running','awaiting_approval','committing'))
  OR NOT EXISTS(SELECT 1 FROM public.sessions s JOIN public.users person ON person.org_id=s.org_id AND person.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
    AND person.status='active' AND s.auth_version=person.auth_version AND s.device_type IN ('web','wechat','douyin','android')
    AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
    AND public.can_access(NEW.org_id,NEW.actor_id,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,s.device_type,false)
    AND (u.file_id IS NULL AND public.can_access(NEW.org_id,NEW.actor_id,'file.upload','folder',f.id,f.project_id,f.creator_id,f.classification,s.device_type,false)
      OR u.file_id IS NOT NULL AND EXISTS(SELECT 1 FROM public.files src WHERE src.org_id=NEW.org_id AND src.id=u.file_id
       AND public.can_access(NEW.org_id,NEW.actor_id,'file.update','file',src.id,f.project_id,src.creator_id,src.classification,s.device_type,false)))) THEN
  RAISE EXCEPTION 'stopped owned draft, current authority and fresh human review required' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION guard_agent_file_abandonment() FROM PUBLIC;
CREATE TRIGGER agent_file_abandonment_binding BEFORE INSERT ON agent_file_abandonments FOR EACH ROW EXECUTE FUNCTION guard_agent_file_abandonment();
CREATE FUNCTION reject_abandoned_agent_recovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM agent_file_abandonments WHERE org_id=NEW.org_id AND draft_id=NEW.source_draft_id)
  OR EXISTS(SELECT 1 FROM upload_sessions WHERE org_id=NEW.org_id AND id=NEW.source_draft_id AND retirement_plan_id IS NOT NULL) THEN
  RAISE EXCEPTION 'abandoned source cannot authorize new recovery' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER agent_file_recovery_abandoned BEFORE INSERT ON agent_file_recoveries FOR EACH ROW EXECUTE FUNCTION reject_abandoned_agent_recovery();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_file_abandonments TO copypaste_app; REVOKE UPDATE,DELETE ON agent_file_abandonments FROM copypaste_app;
END IF; END $$;
