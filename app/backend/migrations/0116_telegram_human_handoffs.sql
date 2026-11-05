ALTER TABLE telegram_channels DROP CONSTRAINT telegram_channels_allowed_skill_ids_check;
ALTER TABLE telegram_channels ADD CONSTRAINT telegram_channels_allowed_skill_ids_check CHECK(cardinality(allowed_skill_ids) BETWEEN 1 AND 8 AND allowed_skill_ids <@ ARRAY['project-create','project-update','file-update','information-organize','oa-assist','subscription-review','project-briefing','incident-summary']::text[]);
ALTER TABLE telegram_inbox DROP CONSTRAINT telegram_inbox_requested_skill_id_check;
ALTER TABLE telegram_inbox ADD CONSTRAINT telegram_inbox_requested_skill_id_check CHECK(requested_skill_id IN ('project-create','project-update','file-update','information-organize','oa-assist','subscription-review','project-briefing','incident-summary'));
CREATE TABLE telegram_human_handoffs (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, inbox_id uuid NOT NULL, run_id uuid NOT NULL,
 request_version bigint NOT NULL CHECK(request_version>0), request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 channel_version bigint NOT NULL, binding_version bigint NOT NULL,
 actor_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL, session_id uuid NOT NULL,
 reviewed boolean NOT NULL CHECK(reviewed), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,id), UNIQUE(org_id,inbox_id), UNIQUE(org_id,run_id),
 FOREIGN KEY(org_id,inbox_id) REFERENCES telegram_inbox(org_id,id), FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE TRIGGER telegram_human_handoff_immutable BEFORE UPDATE OR DELETE ON telegram_human_handoffs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_telegram_human_handoff() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 PERFORM id FROM public.organizations WHERE id=NEW.org_id FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM public.telegram_inbox i JOIN public.telegram_channels ch ON ch.org_id=i.org_id AND ch.id=i.channel_id
  JOIN public.telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id JOIN public.agent_runs r ON r.org_id=NEW.org_id AND r.id=NEW.run_id
  JOIN public.agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id JOIN public.conversations c ON c.org_id=r.org_id AND c.id=r.conversation_id
  JOIN public.organizations o ON o.id=i.org_id JOIN public.users u ON u.org_id=i.org_id AND u.id=i.requester_id
  JOIN public.sessions s ON s.org_id=i.org_id AND s.id=NEW.session_id AND s.user_id=i.requester_id
  WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.kind='request' AND i.state='awaiting_executor'
   AND i.received_at>clock_timestamp()-interval '30 minutes' AND i.requester_id=NEW.actor_id AND i.actor_auth_version=NEW.actor_auth_version
   AND NEW.request_sha256=encode(sha256(convert_to(i.request_text,'UTF8')),'hex')
   AND NEW.request_version=(SELECT max(version) FROM public.telegram_request_events WHERE org_id=i.org_id AND inbox_id=i.id)
   AND ch.state='active' AND ch.version=i.channel_version AND ch.version=NEW.channel_version
   AND ch.bot_id=i.bot_id AND ch.chat_id=i.chat_id AND ch.topic_id=i.topic_id AND i.requested_skill_id=ANY(ch.allowed_skill_ids)
   AND b.state='active' AND b.version=NEW.binding_version AND b.bot_id=i.bot_id AND b.telegram_user_id=i.telegram_user_id AND b.user_id=NEW.actor_id
   AND r.actor_id=NEW.actor_id AND r.state='queued' AND r.channel=s.device_type AND r.channel IN ('web','wechat','douyin','android')
   AND r.actor_auth_version=NEW.actor_auth_version AND r.organization_auth_version=NEW.organization_auth_version
   AND p.skill_id=i.requested_skill_id AND p.state='active' AND public.agent_profile_release_enabled(p.org_id,p.id)
   AND c.type='private' AND c.creator_id=NEW.actor_id AND c.project_id IS NOT DISTINCT FROM ch.project_id
   AND o.authorization_version=NEW.organization_auth_version AND u.status='active' AND u.auth_version=NEW.actor_auth_version
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version
   AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
   AND public.can_access(NEW.org_id,NEW.actor_id,'agent.run','conversation',c.id,c.project_id,c.creator_id,c.classification,s.device_type,false)
   AND NOT EXISTS(SELECT 1 FROM public.telegram_run_links l WHERE l.org_id=i.org_id AND l.inbox_id=i.id)) THEN
  RAISE EXCEPTION 'handoff requires exact current request, private scope and fresh human run authorization' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION guard_telegram_human_handoff() FROM PUBLIC;
CREATE TRIGGER telegram_human_handoff_binding BEFORE INSERT ON telegram_human_handoffs FOR EACH ROW EXECUTE FUNCTION guard_telegram_human_handoff();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON telegram_human_handoffs TO copypaste_app; REVOKE UPDATE,DELETE ON telegram_human_handoffs FROM copypaste_app;
END IF; END $$;
