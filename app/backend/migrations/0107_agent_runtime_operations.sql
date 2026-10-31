-- Deployment still owns installation. The application may only retire an exact
-- installed profile through a current human receipt, never edit or reactivate it.
CREATE TABLE agent_profile_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), org_id uuid NOT NULL, profile_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0), state text NOT NULL CHECK(state IN ('active','retired')),
 source text NOT NULL CHECK(source IN ('installed','retired','retained_baseline')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,profile_id,version),
 FOREIGN KEY(org_id,profile_id) REFERENCES agent_runtime_profiles(org_id,id)
);
INSERT INTO agent_profile_events(org_id,profile_id,version,state,source)
 SELECT org_id,id,1,state,'retained_baseline' FROM agent_runtime_profiles;
CREATE TRIGGER agent_profile_event_immutable BEFORE UPDATE OR DELETE ON agent_profile_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION observe_agent_profile() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
 SET search_path=pg_catalog,public AS $$
BEGIN
 IF TG_OP='INSERT' OR NEW.state IS DISTINCT FROM OLD.state THEN
  INSERT INTO public.agent_profile_events(org_id,profile_id,version,state,source)
   SELECT NEW.org_id,NEW.id,coalesce(max(version),0)+1,NEW.state,
    CASE WHEN TG_OP='INSERT' THEN 'installed' ELSE 'retired' END
   FROM public.agent_profile_events WHERE org_id=NEW.org_id AND profile_id=NEW.id;
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION observe_agent_profile() FROM PUBLIC;
CREATE TRIGGER agent_profile_observer AFTER INSERT OR UPDATE ON agent_runtime_profiles
 FOR EACH ROW EXECUTE FUNCTION observe_agent_profile();

-- Bind review to every currently active run, including version/state/cancel
-- changes. Metadata counts do not grant access to private conversations.
CREATE FUNCTION agent_profile_active_basis(uuid,uuid) RETURNS text LANGUAGE sql STABLE
 SET search_path=pg_catalog,public AS $$
 SELECT encode(sha256(convert_to(coalesce(string_agg(id::text||':'||version::text||':'||state||':'||
  CASE WHEN cancel_requested_at IS NULL THEN '0' ELSE '1' END, E'\n' ORDER BY id),''),'UTF8')),'hex')
 FROM public.agent_runs WHERE org_id=$1 AND profile_id=$2 AND state IN ('queued','running','awaiting_approval','committing');
$$;
REVOKE ALL ON FUNCTION agent_profile_active_basis(uuid,uuid) FROM PUBLIC;
CREATE TABLE agent_profile_retirements (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, profile_id uuid NOT NULL,
 expected_version bigint NOT NULL CHECK(expected_version>0), event_version bigint NOT NULL,
 active_basis_sha256 text NOT NULL CHECK(active_basis_sha256 ~ '^[a-f0-9]{64}$'),
 reviewed_impact boolean NOT NULL CHECK(reviewed_impact),
 actor_id uuid NOT NULL, session_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,profile_id),
 CHECK(event_version=expected_version+1),
 FOREIGN KEY(org_id,profile_id,event_version) REFERENCES agent_profile_events(org_id,profile_id,version),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE TRIGGER agent_profile_retirement_immutable BEFORE UPDATE OR DELETE ON agent_profile_retirements
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION apply_agent_profile_retirement() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
 SET search_path=pg_catalog,public AS $$
DECLARE p public.agent_runtime_profiles;
BEGIN
 PERFORM id FROM public.organizations WHERE id=NEW.org_id FOR UPDATE;
 SELECT * INTO p FROM public.agent_runtime_profiles WHERE org_id=NEW.org_id AND id=NEW.profile_id FOR UPDATE;
 IF NOT FOUND OR p.state<>'active' OR NEW.expected_version<>(SELECT max(version) FROM public.agent_profile_events WHERE org_id=p.org_id AND profile_id=p.id)
  OR NEW.active_basis_sha256<>public.agent_profile_active_basis(p.org_id,p.id)
  OR NOT EXISTS(SELECT 1 FROM public.sessions s JOIN public.users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id
    AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version AND u.status='active'
    AND s.device_type IN ('web','wechat','douyin','android')
    AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
    AND public.can_access(NEW.org_id,NEW.actor_id,'agent.runtime.read','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)
    AND public.can_access(NEW.org_id,NEW.actor_id,'agent.runtime.retire','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)) THEN
  RAISE EXCEPTION 'profile retirement needs the exact current profile, active run basis and authorized human session' USING ERRCODE='23514';
 END IF;
 UPDATE public.agent_runtime_profiles SET state='retired' WHERE org_id=p.org_id AND id=p.id;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION apply_agent_profile_retirement() FROM PUBLIC;
CREATE TRIGGER agent_profile_retirement_binding BEFORE INSERT ON agent_profile_retirements
 FOR EACH ROW EXECUTE FUNCTION apply_agent_profile_retirement();
CREATE INDEX agent_profile_operations_page ON agent_runtime_profiles(org_id,created_at DESC,id DESC);
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'agent_runtime_reader','秘书运行配置查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'agent_runtime_manager','秘书运行配置停用人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'agent.runtime.read' FROM roles WHERE key IN ('agent_runtime_reader','agent_runtime_manager','system_admin');
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'agent.runtime.retire' FROM roles WHERE key IN ('agent_runtime_manager','system_admin');
UPDATE organizations SET authorization_version=authorization_version+1;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT ON agent_profile_events TO copypaste_app;
 REVOKE INSERT,UPDATE,DELETE ON agent_profile_events FROM copypaste_app;
 GRANT SELECT,INSERT ON agent_profile_retirements TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_profile_retirements FROM copypaste_app;
 GRANT EXECUTE ON FUNCTION agent_profile_active_basis(uuid,uuid) TO copypaste_app;
 -- Preserve the original deployment-only installation boundary.
 REVOKE INSERT,UPDATE,DELETE ON agent_runtime_profiles FROM copypaste_app;
END IF; END $$;
