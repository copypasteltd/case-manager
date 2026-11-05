-- Append-only human assessment/publication decisions for exact installed code.
-- Installation and process teardown remain deployment responsibilities.
CREATE TABLE agent_skill_releases (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, profile_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0), stage text NOT NULL CHECK(stage IN ('pilot','published','disabled')),
 profile_version bigint NOT NULL, skill_sha256 text NOT NULL CHECK(skill_sha256 ~ '^[a-f0-9]{64}$'),
 active_basis_sha256 text NOT NULL CHECK(active_basis_sha256 ~ '^[a-f0-9]{64}$'),
 assessment_run_ids uuid[] NOT NULL DEFAULT '{}', rollback_release_id uuid,
 reviewed boolean NOT NULL CHECK(reviewed), reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 actor_id uuid NOT NULL, session_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,profile_id,version),
 FOREIGN KEY(org_id,profile_id) REFERENCES agent_runtime_profiles(org_id,id),
 FOREIGN KEY(org_id,rollback_release_id) REFERENCES agent_skill_releases(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id),
 CHECK(cardinality(assessment_run_ids)<=20), CHECK(stage<>'published' OR cardinality(assessment_run_ids)>0)
);
CREATE TRIGGER agent_skill_release_immutable BEFORE UPDATE OR DELETE ON agent_skill_releases
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION agent_profile_release_enabled(uuid,uuid) RETURNS boolean LANGUAGE sql STABLE
 SET search_path=pg_catalog,public AS $$
 SELECT coalesce((SELECT stage<>'disabled' FROM public.agent_skill_releases WHERE org_id=$1 AND profile_id=$2 ORDER BY version DESC LIMIT 1),true);
$$;
REVOKE ALL ON FUNCTION agent_profile_release_enabled(uuid,uuid) FROM PUBLIC;
CREATE FUNCTION guard_agent_skill_release() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
 SET search_path=pg_catalog,public AS $$
DECLARE p public.agent_runtime_profiles; old_release public.agent_skill_releases;
BEGIN
 PERFORM id FROM public.organizations WHERE id=NEW.org_id FOR UPDATE;
 SELECT * INTO p FROM public.agent_runtime_profiles WHERE org_id=NEW.org_id AND id=NEW.profile_id;
 IF NOT FOUND OR p.state<>'active' OR p.skill_sha256<>NEW.skill_sha256
  OR NEW.profile_version<>(SELECT max(version) FROM public.agent_profile_events WHERE org_id=p.org_id AND profile_id=p.id)
  OR NEW.version<>coalesce((SELECT max(version)+1 FROM public.agent_skill_releases WHERE org_id=p.org_id AND profile_id=p.id),1)
  OR NEW.active_basis_sha256<>public.agent_profile_active_basis(p.org_id,p.id)
  OR NOT EXISTS(SELECT 1 FROM public.sessions s JOIN public.users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id
    AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version AND u.status='active'
    AND s.device_type IN ('web','wechat','douyin','android')
    AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
    AND public.can_access(NEW.org_id,NEW.actor_id,'agent.skill.read','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)
    AND public.can_access(NEW.org_id,NEW.actor_id,'agent.skill.manage','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)) THEN
  RAISE EXCEPTION 'exact skill profile, version, impact and current authorized human session required' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM unnest(NEW.assessment_run_ids) x(id) WHERE NOT EXISTS(
  SELECT 1 FROM public.agent_runs r WHERE r.org_id=NEW.org_id AND r.id=x.id AND r.profile_id=p.id AND r.actor_id=NEW.actor_id AND r.state='succeeded' AND r.outcome='completed')) THEN
  RAISE EXCEPTION 'assessments must be own successful exact-profile runs' USING ERRCODE='23514';
 END IF;
 IF NEW.rollback_release_id IS NOT NULL THEN
  SELECT * INTO old_release FROM public.agent_skill_releases WHERE org_id=NEW.org_id AND id=NEW.rollback_release_id;
  IF NOT FOUND OR old_release.stage<>'published' OR old_release.skill_sha256<>p.skill_sha256
   OR NOT EXISTS(SELECT 1 FROM public.agent_runtime_profiles q WHERE q.org_id=p.org_id AND q.id=old_release.profile_id AND q.skill_id=p.skill_id AND q.runtime_version=p.runtime_version) THEN
   RAISE EXCEPTION 'rollback must reference a published matching installed package' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION guard_agent_skill_release() FROM PUBLIC;
CREATE TRIGGER agent_skill_release_binding BEFORE INSERT ON agent_skill_releases
 FOR EACH ROW EXECUTE FUNCTION guard_agent_skill_release();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'agent_skill_reader','秘书Skill发布查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'agent_skill_manager','秘书Skill发布管理人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'agent.skill.read' FROM roles WHERE key IN ('agent_skill_reader','agent_skill_manager','system_admin');
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'agent.skill.manage' FROM roles WHERE key IN ('agent_skill_manager','system_admin');
UPDATE organizations SET authorization_version=authorization_version+1;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON agent_skill_releases TO copypaste_app;
 REVOKE UPDATE,DELETE ON agent_skill_releases FROM copypaste_app;
 GRANT EXECUTE ON FUNCTION agent_profile_release_enabled(uuid,uuid) TO copypaste_app;
END IF; END $$;

-- Exact Office reads use ordinary immutable tool receipts and observations.
ALTER TABLE agent_tool_receipts DROP CONSTRAINT agent_tool_receipts_tool_check;
ALTER TABLE agent_tool_receipts ADD CONSTRAINT agent_tool_receipts_tool_check CHECK(tool IN ('read_office_workspace','submit_deliverable','read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
ALTER TABLE agent_execution_observations DROP CONSTRAINT agent_execution_observations_tool_check;
ALTER TABLE agent_execution_observations ADD CONSTRAINT agent_execution_observations_tool_check CHECK(tool IN ('read_office_workspace','submit_deliverable','read_subscription_portfolio','list_project_catalog','recover_file_draft','create_task','create_risk','update_risk','create_issue','update_issue','create_deliverable','update_deliverable','create_milestone','list_subscriptions','read_subscription_review','read_finance_record','read_finance_summary','read_service_summary','read_resource','search_resources','inspect_operation','propose_project','create_project','update_project','update_task','update_task_dependency','transition_task','create_file_draft','propose_file_update','commit_file_update','list_directory','propose_organization','commit_organization','stage_file_create','propose_file_create','commit_file_create','draft_request','submit_request','draft_business_text','draft_post','publish_post','transition_project','update_milestone','transition_milestone'));
