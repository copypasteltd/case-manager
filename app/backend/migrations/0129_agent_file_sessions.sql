-- New file delegations bind one original human login. Existing intent text,
-- hashes and receipts are retained byte-for-byte: absence of historical
-- provenance is explicit and never backfilled from a later login.
ALTER TABLE agent_file_intents
 ADD COLUMN session_id uuid,
 ADD COLUMN origin_channel text,
 ADD CONSTRAINT agent_file_intent_session_pair CHECK(
  (session_id IS NULL AND origin_channel IS NULL)
  OR (session_id IS NOT NULL AND origin_channel IS NOT NULL AND origin_channel IN ('web','android','wechat','douyin'))),
 ADD CONSTRAINT agent_file_intent_session_fk FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id);

-- Keep the previous exact target/upload/scan/ordinary-command/atomic-receipt
-- guards intact. This additional gate applies only to NEW authority/evidence;
-- it does not prevent human reads of historical rows or their explicit recovery
-- into a new intent with a NEW live authorizing session.
CREATE FUNCTION validate_agent_file_session() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r agent_runs; i agent_file_intents; original_session uuid; original_channel text;
BEGIN
 SELECT * INTO r FROM agent_runs WHERE org_id=NEW.org_id AND id=NEW.run_id;
 IF TG_TABLE_NAME='agent_file_intents' THEN
  original_session:=NEW.session_id;
  original_channel:=NEW.origin_channel;
  IF ((NEW.intent::jsonb)->>'session_id'=original_session::text
   AND (NEW.intent::jsonb)->>'origin_channel'=original_channel) IS NOT TRUE THEN
   RAISE EXCEPTION 'file intent requires immutable original session provenance' USING ERRCODE='23514';
  END IF;
 ELSE
  SELECT * INTO i FROM agent_file_intents WHERE org_id=NEW.org_id AND run_id=NEW.run_id;
  original_session:=i.session_id;
  original_channel:=i.origin_channel;
  IF i.run_id IS NULL OR i.actor_id IS DISTINCT FROM r.actor_id THEN
   RAISE EXCEPTION 'file session does not match its original actor' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='agent_file_recoveries' THEN
   IF (NEW.recovery_body::jsonb)->>'session_id' IS DISTINCT FROM original_session::text THEN
    RAISE EXCEPTION 'recovery must bind the new file intent session' USING ERRCODE='23514';
   END IF;
  ELSIF TG_TABLE_NAME='agent_contract_body_intents' THEN
   IF NEW.session_id IS DISTINCT FROM original_session THEN
    RAISE EXCEPTION 'contract body must bind the same file intent session' USING ERRCODE='23514';
   END IF;
  END IF;
 END IF;
 IF r.id IS NULL OR original_session IS NULL OR original_channel IS NULL
  OR original_channel NOT IN ('web','android','wechat','douyin') OR original_channel<>r.channel THEN
  RAISE EXCEPTION 'file execution requires its original interactive session' USING ERRCODE='23514';
 END IF;
 -- Match RequireInteractiveSession(false). The SHARE lock serializes with
 -- logout/rotation/revocation until this atomic ordinary transaction commits.
 -- L1 file work deliberately does not introduce a new MFA requirement.
 PERFORM 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
  JOIN organizations o ON o.id=s.org_id
  WHERE s.org_id=r.org_id AND s.id=original_session AND s.user_id=r.actor_id
  AND s.device_type=original_channel AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
  AND u.status='active' AND s.auth_version=u.auth_version AND u.auth_version=r.actor_auth_version
  AND o.authorization_version=r.organization_auth_version FOR SHARE OF s;
 IF NOT FOUND THEN
  RAISE EXCEPTION 'file original session is expired, revoked or changed' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;

DO $$
DECLARE target text;
BEGIN
 FOREACH target IN ARRAY ARRAY['agent_file_intents','agent_file_drafts','agent_file_stages',
  'agent_file_proposals','agent_file_commits','agent_file_recoveries','agent_contract_body_intents'] LOOP
  EXECUTE format('CREATE TRIGGER agent_file_session_guard BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION validate_agent_file_session()',target);
 END LOOP;
END; $$;
