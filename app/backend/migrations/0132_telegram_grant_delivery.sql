-- Exact effective run origin for generic grant notices. Human reviewed disclosure remains separate.
CREATE OR REPLACE FUNCTION telegram_delivery_authorized(w_org uuid,w_inbox uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM telegram_inbox i
 JOIN telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
 JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id
 JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
 LEFT JOIN telegram_effective_run_links actual ON actual.org_id=i.org_id AND actual.inbox_id=i.id
 LEFT JOIN agent_runs run ON run.org_id=actual.org_id AND run.id=actual.run_id
 LEFT JOIN telegram_conversation_scopes s ON s.org_id=l.org_id AND s.conversation_id=l.conversation_id
 JOIN organizations o ON o.id=i.org_id
 WHERE i.org_id=w_org AND i.id=w_inbox AND i.kind='request' AND i.state IN ('awaiting_executor','dispatched')
 AND c.state='active' AND c.delivery_enabled AND c.disclosure_mode='portal_only' AND c.version=i.channel_version
 AND c.bot_id=i.bot_id AND b.state='active' AND b.user_id=i.requester_id AND b.bot_id=i.bot_id
 AND b.telegram_user_id=i.telegram_user_id AND u.status='active' AND u.auth_version=i.actor_auth_version
 AND (actual.run_id IS NULL OR (run.actor_id=u.id AND run.actor_auth_version=u.auth_version AND run.organization_auth_version=o.authorization_version))
 AND (l.run_id IS NULL OR (s.actor_id=u.id AND s.actor_auth_version=u.auth_version
  AND s.organization_auth_version=o.authorization_version AND s.channel_version=c.version)))
$$;

CREATE OR REPLACE FUNCTION guard_telegram_delivery_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.phase='reviewed_result' THEN
  IF NOT EXISTS(SELECT 1 FROM telegram_human_handoffs h JOIN telegram_handoff_reviews rv ON rv.org_id=h.org_id AND rv.handoff_id=h.id
   JOIN telegram_inbox i ON i.org_id=h.org_id AND i.id=h.inbox_id WHERE h.org_id=NEW.org_id AND h.inbox_id=NEW.inbox_id AND h.run_id=NEW.run_id AND i.channel_id=NEW.channel_id AND i.bot_id=NEW.bot_id) THEN
   RAISE EXCEPTION 'reviewed result requires exact human handoff and disclosure' USING ERRCODE='23514'; END IF;
 ELSIF NEW.run_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM telegram_effective_run_links WHERE org_id=NEW.org_id AND run_id=NEW.run_id AND inbox_id=NEW.inbox_id) THEN
  RAISE EXCEPTION 'automatic notices require original Telegram run' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;

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
   SELECT 1 FROM telegram_inbox i LEFT JOIN telegram_effective_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
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
