-- Request metadata is separate from private inbound prose. Existing requests
-- start at an explicitly observed baseline; this does not invent old events.
CREATE TABLE telegram_request_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), org_id uuid NOT NULL,
 inbox_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 state text NOT NULL CHECK(state IN ('awaiting_executor','dispatched','withdrawn')),
 reason_code text NOT NULL, dispatch_after timestamptz NOT NULL,
 source text NOT NULL CHECK(source IN ('accepted','transition','retained_baseline')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,inbox_id,version), FOREIGN KEY(org_id,inbox_id) REFERENCES telegram_inbox(org_id,id)
);
CREATE INDEX telegram_request_history_page ON telegram_request_events(org_id,inbox_id,created_at DESC,id DESC);
INSERT INTO telegram_request_events(org_id,inbox_id,version,state,reason_code,dispatch_after,source)
 SELECT org_id,id,1,state,reason_code,dispatch_after,'retained_baseline' FROM telegram_inbox WHERE kind='request';
CREATE TRIGGER telegram_request_event_immutable BEFORE UPDATE OR DELETE ON telegram_request_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

-- The application cannot forge journal inserts. Every actual inbox write is
-- serialized by its parent row and observed in the same transaction, including
-- automatic dispatch, expiry, channel changes and binding revocations.
CREATE FUNCTION observe_telegram_request() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER
 SET search_path=pg_catalog,public AS $$
BEGIN
 IF NEW.kind='request' AND (TG_OP='INSERT' OR
   (NEW.state,NEW.reason_code,NEW.dispatch_after) IS DISTINCT FROM (OLD.state,OLD.reason_code,OLD.dispatch_after)) THEN
  INSERT INTO public.telegram_request_events(org_id,inbox_id,version,state,reason_code,dispatch_after,source)
   SELECT NEW.org_id,NEW.id,coalesce(max(version),0)+1,NEW.state,NEW.reason_code,NEW.dispatch_after,
    CASE WHEN TG_OP='INSERT' THEN 'accepted' ELSE 'transition' END
   FROM public.telegram_request_events WHERE org_id=NEW.org_id AND inbox_id=NEW.id;
 END IF;
 RETURN NEW;
END; $$;
REVOKE ALL ON FUNCTION observe_telegram_request() FROM PUBLIC;
CREATE TRIGGER telegram_request_observer AFTER INSERT OR UPDATE ON telegram_inbox
 FOR EACH ROW EXECUTE FUNCTION observe_telegram_request();

CREATE TABLE telegram_request_withdrawals (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, inbox_id uuid NOT NULL,
 expected_version bigint NOT NULL CHECK(expected_version>0), event_version bigint NOT NULL,
 actor_id uuid NOT NULL, session_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(org_id,inbox_id), CHECK(event_version=expected_version+1),
 FOREIGN KEY(org_id,inbox_id,event_version) REFERENCES telegram_request_events(org_id,inbox_id,version),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE TRIGGER telegram_request_withdrawal_immutable BEFORE UPDATE OR DELETE ON telegram_request_withdrawals
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_telegram_request_withdrawal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM telegram_inbox i
  JOIN telegram_request_events e ON e.org_id=i.org_id AND e.inbox_id=i.id AND e.version=NEW.event_version
  JOIN telegram_request_events p ON p.org_id=i.org_id AND p.inbox_id=i.id AND p.version=NEW.expected_version
  JOIN sessions s ON s.org_id=i.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id
  JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
  WHERE i.org_id=NEW.org_id AND i.id=NEW.inbox_id AND i.kind='request' AND i.state='withdrawn'
   AND i.reason_code='manual_queue_withdrawn' AND e.state=i.state AND e.reason_code=i.reason_code
   AND e.source='transition' AND p.state='awaiting_executor'
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version
   AND u.status='active' AND s.device_type IN ('web','wechat','douyin','android')
   AND s.mfa_verified_at>clock_timestamp()-interval '10 minutes'
   AND NOT EXISTS(SELECT 1 FROM telegram_run_links l WHERE l.org_id=i.org_id AND l.inbox_id=i.id)) THEN
  RAISE EXCEPTION 'queue withdrawal needs its exact unstarted request, journal and current human session' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER telegram_request_withdrawal_binding BEFORE INSERT ON telegram_request_withdrawals
 FOR EACH ROW EXECUTE FUNCTION guard_telegram_request_withdrawal();
CREATE FUNCTION validate_telegram_manual_withdrawal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.kind='request' AND NEW.reason_code='manual_queue_withdrawn' AND NOT EXISTS(
  SELECT 1 FROM telegram_request_withdrawals w WHERE w.org_id=NEW.org_id AND w.inbox_id=NEW.id) THEN
  RAISE EXCEPTION 'manual queue withdrawal requires its immutable human receipt' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
CREATE CONSTRAINT TRIGGER telegram_manual_withdrawal_receipt AFTER INSERT OR UPDATE ON telegram_inbox
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_telegram_manual_withdrawal();

CREATE INDEX telegram_inbox_operations_page ON telegram_inbox(org_id,received_at DESC,id DESC) WHERE kind='request';
CREATE INDEX telegram_inbox_operations_state_page ON telegram_inbox(org_id,state,received_at DESC,id DESC) WHERE kind='request';
CREATE INDEX telegram_inbox_operations_channel_page ON telegram_inbox(org_id,channel_id,received_at DESC,id DESC) WHERE kind='request';
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'telegram_inbox_reader','TG入站台账查阅人',false FROM organizations;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
 SELECT gen_random_uuid(),id,'telegram_inbox_manager','TG入站排队经办人',true FROM organizations;
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'telegram.inbox.read' FROM roles WHERE key IN ('telegram_inbox_reader','telegram_inbox_manager','system_admin');
INSERT INTO role_actions(org_id,role_id,action)
 SELECT org_id,id,'telegram.inbox.withdraw' FROM roles WHERE key IN ('telegram_inbox_manager','system_admin');
UPDATE organizations SET authorization_version=authorization_version+1;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT ON telegram_request_events TO copypaste_app;
 REVOKE INSERT,UPDATE,DELETE ON telegram_request_events FROM copypaste_app;
 GRANT SELECT,INSERT ON telegram_request_withdrawals TO copypaste_app;
 REVOKE UPDATE,DELETE ON telegram_request_withdrawals FROM copypaste_app;
END IF; END $$;
