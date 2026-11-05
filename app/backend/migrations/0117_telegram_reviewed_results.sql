CREATE TABLE telegram_handoff_reviews (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, handoff_id uuid NOT NULL, run_id uuid NOT NULL,
 run_version bigint NOT NULL CHECK(run_version>0), body_sha256 text NOT NULL CHECK(body_sha256 ~ '^[a-f0-9]{64}$'),
 start_char integer NOT NULL CHECK(start_char>=0), max_chars integer NOT NULL CHECK(max_chars BETWEEN 1 AND 2000),
 actor_id uuid NOT NULL, session_id uuid NOT NULL, actor_auth_version bigint NOT NULL, organization_auth_version bigint NOT NULL,
 reviewed boolean NOT NULL CHECK(reviewed), disclosed_to_group boolean NOT NULL CHECK(disclosed_to_group),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(org_id,id), UNIQUE(org_id,handoff_id),
 FOREIGN KEY(org_id,handoff_id) REFERENCES telegram_human_handoffs(org_id,id), FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id), FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE TRIGGER telegram_handoff_review_immutable BEFORE UPDATE OR DELETE ON telegram_handoff_reviews FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION telegram_reviewed_delivery_authorized(w_org uuid,w_inbox uuid) RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,public AS $$
 SELECT EXISTS(SELECT 1 FROM public.telegram_human_handoffs h JOIN public.telegram_handoff_reviews rv ON rv.org_id=h.org_id AND rv.handoff_id=h.id
 JOIN public.telegram_inbox i ON i.org_id=h.org_id AND i.id=h.inbox_id JOIN public.telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
 JOIN public.telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id JOIN public.users u ON u.org_id=h.org_id AND u.id=h.actor_id
 JOIN public.organizations o ON o.id=h.org_id JOIN public.sessions s ON s.org_id=rv.org_id AND s.id=rv.session_id
 JOIN public.agent_runs r ON r.org_id=h.org_id AND r.id=h.run_id JOIN public.agent_run_results result ON result.org_id=r.org_id AND result.run_id=r.id
 WHERE h.org_id=w_org AND h.inbox_id=w_inbox AND rv.run_id=h.run_id AND rv.actor_id=h.actor_id
 AND i.state='withdrawn' AND i.reason_code='human_authorized_handoff' AND i.received_at>clock_timestamp()-interval '24 hours'
 AND c.state='active' AND c.delivery_enabled AND c.version=h.channel_version AND c.version=i.channel_version AND c.bot_id=i.bot_id
 AND b.state='active' AND b.version=h.binding_version AND b.user_id=h.actor_id AND b.bot_id=i.bot_id AND b.telegram_user_id=i.telegram_user_id
 AND r.state='succeeded' AND r.outcome='completed' AND r.version=rv.run_version AND r.actor_id=h.actor_id
 AND rv.body_sha256=encode(sha256(convert_to(result.body,'UTF8')),'hex') AND rv.start_char+rv.max_chars<=length(result.body)
 AND u.status='active' AND u.auth_version=rv.actor_auth_version AND u.auth_version=h.actor_auth_version
 AND o.authorization_version=rv.organization_auth_version AND o.authorization_version=h.organization_auth_version
 AND s.user_id=h.actor_id AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version
 AND public.can_access(h.org_id,h.actor_id,'telegram.handoff.disclose','organization',h.org_id,NULL,NULL,'internal',s.device_type,false));
$$;
REVOKE ALL ON FUNCTION telegram_reviewed_delivery_authorized(uuid,uuid) FROM PUBLIC;
CREATE FUNCTION guard_telegram_handoff_review() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.telegram_human_handoffs h JOIN public.agent_runs r ON r.org_id=h.org_id AND r.id=h.run_id
 JOIN public.agent_run_results result ON result.org_id=r.org_id AND result.run_id=r.id JOIN public.sessions s ON s.org_id=h.org_id AND s.id=NEW.session_id
 JOIN public.users u ON u.org_id=h.org_id AND u.id=h.actor_id JOIN public.organizations o ON o.id=h.org_id
 WHERE h.org_id=NEW.org_id AND h.id=NEW.handoff_id AND h.run_id=NEW.run_id AND h.actor_id=NEW.actor_id
 AND r.state='succeeded' AND r.outcome='completed' AND r.version=NEW.run_version
 AND NEW.body_sha256=encode(sha256(convert_to(result.body,'UTF8')),'hex') AND NEW.start_char+NEW.max_chars<=length(result.body)
 AND s.user_id=NEW.actor_id AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version AND u.status='active'
 AND u.auth_version=NEW.actor_auth_version AND o.authorization_version=NEW.organization_auth_version AND h.organization_auth_version=o.authorization_version
 AND s.device_type IN ('web','wechat','douyin','android') AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<clock_timestamp()+interval '1 minute'
 AND public.can_access(NEW.org_id,NEW.actor_id,'telegram.handoff.disclose','organization',NEW.org_id,NULL,NULL,'internal',s.device_type,false)) THEN
 RAISE EXCEPTION 'exact current result and fresh human disclosure approval required' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION guard_telegram_handoff_review() FROM PUBLIC;
CREATE TRIGGER telegram_handoff_review_binding BEFORE INSERT ON telegram_handoff_reviews FOR EACH ROW EXECUTE FUNCTION guard_telegram_handoff_review();
ALTER TABLE telegram_deliveries DROP CONSTRAINT telegram_deliveries_phase_check;
ALTER TABLE telegram_deliveries ADD CONSTRAINT telegram_deliveries_phase_check CHECK(phase IN ('accepted','progress','result','reviewed_result'));
ALTER TABLE telegram_deliveries DROP CONSTRAINT telegram_deliveries_org_id_run_id_fkey;
ALTER TABLE telegram_deliveries ADD FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id);
CREATE FUNCTION guard_telegram_delivery_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.phase='reviewed_result' THEN
  IF NOT EXISTS(SELECT 1 FROM telegram_human_handoffs h JOIN telegram_handoff_reviews rv ON rv.org_id=h.org_id AND rv.handoff_id=h.id
   JOIN telegram_inbox i ON i.org_id=h.org_id AND i.id=h.inbox_id WHERE h.org_id=NEW.org_id AND h.inbox_id=NEW.inbox_id AND h.run_id=NEW.run_id AND i.channel_id=NEW.channel_id AND i.bot_id=NEW.bot_id) THEN
   RAISE EXCEPTION 'reviewed result requires exact human handoff and disclosure' USING ERRCODE='23514'; END IF;
 ELSIF NEW.run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM telegram_run_links WHERE org_id=NEW.org_id AND run_id=NEW.run_id AND inbox_id=NEW.inbox_id) THEN
  RAISE EXCEPTION 'automatic notices require original Telegram run' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_delivery_origin BEFORE INSERT ON telegram_deliveries FOR EACH ROW EXECUTE FUNCTION guard_telegram_delivery_origin();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'telegram_handoff_discloser','TG结果发布审核人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,'telegram.handoff.disclose' FROM roles WHERE key IN ('telegram_handoff_discloser','system_admin');
UPDATE organizations SET authorization_version=authorization_version+1;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON telegram_handoff_reviews TO copypaste_app; REVOKE UPDATE,DELETE ON telegram_handoff_reviews FROM copypaste_app;
 GRANT EXECUTE ON FUNCTION telegram_reviewed_delivery_authorized(uuid,uuid) TO copypaste_app;
END IF; END $$;

-- Keep ordinary transition/fencing invariants for independently reviewed results.
CREATE OR REPLACE FUNCTION guard_telegram_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'delivery history cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.phase='reviewed_result' THEN
   IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempts<>0 OR NOT EXISTS(
    SELECT 1 FROM telegram_inbox i JOIN telegram_human_handoffs h ON h.org_id=i.org_id AND h.inbox_id=i.id
    WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.channel_id=NEW.channel_id AND i.bot_id=NEW.bot_id
     AND h.run_id=NEW.run_id AND telegram_reviewed_delivery_authorized(i.org_id,i.id)) THEN
    RAISE EXCEPTION 'reviewed delivery requires its exact authorized original request' USING ERRCODE='23514'; END IF;
  ELSE
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempts<>0 OR NOT EXISTS(
   SELECT 1 FROM telegram_inbox i LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
   LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id
   WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.channel_id=NEW.channel_id AND i.bot_id=NEW.bot_id
    AND NEW.run_id IS NOT DISTINCT FROM l.run_id AND telegram_delivery_authorized(i.org_id,i.id)
    AND NEW.phase=CASE WHEN r.state IS NULL OR r.state='queued' THEN 'accepted' WHEN r.state IN ('running','awaiting_approval','committing') THEN 'progress' ELSE 'result' END) THEN
   RAISE EXCEPTION 'delivery must originate in its current authorized request' USING ERRCODE='23514'; END IF;
  END IF;
 ELSE
  IF (NEW.id,NEW.org_id,NEW.inbox_id,NEW.channel_id,NEW.bot_id,NEW.run_id,NEW.phase,NEW.created_at)
    IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.inbox_id,OLD.channel_id,OLD.bot_id,OLD.run_id,OLD.phase,OLD.created_at)
    OR NEW.version<>OLD.version+1 THEN
   RAISE EXCEPTION 'delivery identity is immutable and revisions are sequential' USING ERRCODE='23514'; END IF;
  IF NOT ((OLD.state IN ('pending','retry_wait') AND NEW.state IN ('sending','superseded','suppressed'))
    OR (OLD.state='sending' AND NEW.state IN ('sent','delivery_unknown','retry_wait','failed','suppressed'))
    OR (OLD.state='delivery_unknown' AND NEW.state IN ('confirmed_sent','retry_wait','abandoned','suppressed'))) THEN
   RAISE EXCEPTION 'invalid delivery transition' USING ERRCODE='23514'; END IF;
  IF NEW.attempts<>OLD.attempts+(CASE WHEN NEW.state='sending' THEN 1 ELSE 0 END) THEN
   RAISE EXCEPTION 'every outbound attempt must be prepared exactly once' USING ERRCODE='23514'; END IF;
  IF NEW.state='sending' AND NOT (CASE WHEN NEW.phase='reviewed_result' THEN telegram_reviewed_delivery_authorized(NEW.org_id,NEW.inbox_id) ELSE telegram_delivery_authorized(NEW.org_id,NEW.inbox_id) END) THEN
   RAISE EXCEPTION 'outbound request authorization changed' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
