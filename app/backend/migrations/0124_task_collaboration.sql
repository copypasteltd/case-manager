-- Ordinary collaboration metadata; participation never grants project/file access.
ALTER TABLE tasks ADD COLUMN participant_ids uuid[] NOT NULL DEFAULT '{}'::uuid[];
ALTER TABLE tasks ADD CONSTRAINT tasks_participant_count CHECK(cardinality(participant_ids)<=50 AND array_position(participant_ids,NULL) IS NULL);
CREATE FUNCTION guard_task_participants() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' OR NEW.participant_ids IS DISTINCT FROM OLD.participant_ids THEN
  IF cardinality(NEW.participant_ids)<>(SELECT count(DISTINCT v) FROM unnest(NEW.participant_ids) v) OR EXISTS(
   SELECT 1 FROM unnest(NEW.participant_ids) v WHERE NOT EXISTS(SELECT 1 FROM project_members pm JOIN users u ON u.org_id=pm.org_id AND u.id=pm.user_id WHERE pm.org_id=NEW.org_id AND pm.project_id=NEW.project_id AND pm.user_id=v AND u.status='active')
  ) THEN RAISE EXCEPTION 'task participants must be distinct active project members' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER task_participants_guard BEFORE INSERT OR UPDATE OF participant_ids ON tasks FOR EACH ROW EXECUTE FUNCTION guard_task_participants();
