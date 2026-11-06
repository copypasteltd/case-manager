-- Ordinary human references connect exact contract nodes to actual project work.
-- They never create expense, cash, approval or task-completion facts.
ALTER TABLE contract_milestones ADD CONSTRAINT contract_milestone_task_identity
 UNIQUE(org_id,contract_id,revision_id,id);
CREATE TABLE contract_milestone_task_events (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,contract_id uuid NOT NULL,milestone_id uuid NOT NULL,
 contract_revision_id uuid NOT NULL,contract_version bigint NOT NULL CHECK(contract_version BETWEEN 1 AND 9007199254740991),
 seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991),
 action text NOT NULL CHECK(action IN ('link','revoke')),source_link_id uuid,
 task_id uuid NOT NULL,task_version bigint NOT NULL CHECK(task_version BETWEEN 1 AND 9007199254740991),
 task_project_id uuid NOT NULL,task_title text NOT NULL,task_state text NOT NULL,
 cross_scope boolean NOT NULL,scope_difference_acknowledged boolean NOT NULL,
 actor_id uuid NOT NULL,session_id uuid NOT NULL,mfa_verified_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(char_length(btrim(reason)) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id),UNIQUE(org_id,contract_id,seq),
 FOREIGN KEY(org_id,contract_id,contract_revision_id,milestone_id) REFERENCES contract_milestones(org_id,contract_id,revision_id,id),
 FOREIGN KEY(org_id,task_project_id,task_id) REFERENCES tasks(org_id,project_id,id),
 FOREIGN KEY(org_id,source_link_id) REFERENCES contract_milestone_task_events(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
 CHECK((action='link' AND source_link_id IS NULL) OR (action='revoke' AND source_link_id IS NOT NULL)),
 CHECK(NOT cross_scope OR scope_difference_acknowledged)
);
CREATE UNIQUE INDEX contract_task_revoke_once ON contract_milestone_task_events(org_id,source_link_id) WHERE action='revoke';
CREATE INDEX contract_task_node_events ON contract_milestone_task_events(org_id,contract_id,milestone_id,seq DESC);
CREATE INDEX contract_task_pair ON contract_milestone_task_events(org_id,milestone_id,task_id,seq DESC);
CREATE FUNCTION validate_contract_milestone_task_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c contracts;t tasks;root contract_milestone_task_events;expected_seq bigint;
BEGIN
 SELECT * INTO c FROM contracts WHERE org_id=NEW.org_id AND id=NEW.contract_id FOR UPDATE;
 IF NOT FOUND OR c.deleted_at IS NOT NULL OR c.active_revision_id IS DISTINCT FROM NEW.contract_revision_id
    OR c.version<>NEW.contract_version OR c.lifecycle NOT IN ('effective','completed','terminated')
    OR (NEW.action='link' AND c.lifecycle<>'effective') THEN
  RAISE EXCEPTION 'contract task reference requires the reviewed current effective basis' USING ERRCODE='23514'; END IF;
 SELECT * INTO t FROM tasks WHERE org_id=NEW.org_id AND id=NEW.task_id FOR SHARE;
 IF NOT FOUND OR (t.version,t.project_id,t.title,t.state) IS DISTINCT FROM (NEW.task_version,NEW.task_project_id,NEW.task_title,NEW.task_state)
    OR NEW.cross_scope IS DISTINCT FROM (c.project_id IS DISTINCT FROM t.project_id) THEN
  RAISE EXCEPTION 'contract task reference source changed' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
   WHERE s.org_id=NEW.org_id AND s.id=NEW.session_id AND s.user_id=NEW.actor_id AND u.status='active'
   AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.auth_version=u.auth_version
   AND s.device_type IN ('web','android','wechat','douyin') AND s.mfa_verified_at=NEW.mfa_verified_at
   AND s.mfa_verified_at>=clock_timestamp()-interval '10 minutes' AND s.mfa_verified_at<=clock_timestamp()+interval '1 minute') THEN
  RAISE EXCEPTION 'contract task reference requires original human session evidence' USING ERRCODE='23514'; END IF;
 SELECT coalesce(max(seq),0)+1 INTO expected_seq FROM contract_milestone_task_events WHERE org_id=NEW.org_id AND contract_id=NEW.contract_id;
 IF NEW.seq<>expected_seq THEN RAISE EXCEPTION 'contract task reference sequence changed' USING ERRCODE='23514'; END IF;
 IF NEW.action='link' THEN
  IF EXISTS(SELECT 1 FROM contract_milestone_task_events e WHERE e.org_id=NEW.org_id AND e.milestone_id=NEW.milestone_id AND e.task_id=NEW.task_id
      AND e.action='link' AND NOT EXISTS(SELECT 1 FROM contract_milestone_task_events r WHERE r.org_id=e.org_id AND r.source_link_id=e.id AND r.action='revoke')) THEN
   RAISE EXCEPTION 'contract task reference is already active' USING ERRCODE='23514'; END IF;
 ELSE
  SELECT * INTO root FROM contract_milestone_task_events WHERE org_id=NEW.org_id AND id=NEW.source_link_id;
  IF NOT FOUND OR root.action<>'link' OR (root.contract_id,root.milestone_id,root.contract_revision_id,root.task_id)
       IS DISTINCT FROM (NEW.contract_id,NEW.milestone_id,NEW.contract_revision_id,NEW.task_id) THEN
   RAISE EXCEPTION 'contract task revocation must name its exact original link' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER contract_task_validate BEFORE INSERT ON contract_milestone_task_events
 FOR EACH ROW EXECUTE FUNCTION validate_contract_milestone_task_event();
CREATE TRIGGER contract_task_immutable BEFORE UPDATE OR DELETE ON contract_milestone_task_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON contract_milestone_task_events TO copypaste_app;
 REVOKE UPDATE,DELETE ON contract_milestone_task_events FROM copypaste_app;
END IF; END $$;
