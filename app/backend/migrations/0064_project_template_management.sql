-- Template IDs identify fixed published revisions, not a moving family head.
ALTER TABLE project_templates ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE project_templates ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE project_templates ADD COLUMN description text NOT NULL DEFAULT '' CHECK(length(description)<=2000);
ALTER TABLE project_templates ADD COLUMN creator_id uuid;
ALTER TABLE project_templates ADD COLUMN based_on_id uuid;
ALTER TABLE project_templates ADD COLUMN published_at timestamptz;
ALTER TABLE project_templates ADD COLUMN published_by uuid;
ALTER TABLE project_templates ADD COLUMN retired_at timestamptz;
ALTER TABLE project_templates ADD COLUMN retired_by uuid;
ALTER TABLE project_templates ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE project_templates ADD FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id);
ALTER TABLE project_templates ADD FOREIGN KEY(org_id,retired_by) REFERENCES users(org_id,id);
ALTER TABLE project_templates ADD FOREIGN KEY(org_id,based_on_id) REFERENCES project_templates(org_id,id);
CREATE INDEX project_templates_list ON project_templates(org_id,state,updated_at DESC,id DESC);
CREATE UNIQUE INDEX project_templates_one_draft ON project_templates(org_id,key) WHERE state='draft';
CREATE TABLE project_template_events (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, template_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0), kind text NOT NULL CHECK(kind IN('created','revised','updated','published','retired')),
 actor_id uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
 content_hash text NOT NULL CHECK(content_hash ~ '^[a-f0-9]{64}$'), snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(org_id,template_id,version),
 FOREIGN KEY(org_id,template_id) REFERENCES project_templates(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TRIGGER project_template_event_immutable BEFORE UPDATE OR DELETE ON project_template_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_project_template_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF current_user='copypaste_app' AND (NEW.creator_id IS NULL OR NEW.state<>'draft' OR NEW.version<>1) THEN
   RAISE EXCEPTION 'application creates attributed drafts only' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'template revisions cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (NEW.id,NEW.org_id,NEW.key,NEW.revision,NEW.creator_id,NEW.created_at,NEW.based_on_id)
  IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.key,OLD.revision,OLD.creator_id,OLD.created_at,OLD.based_on_id)
  OR NEW.version<>OLD.version+1 OR OLD.state='archived' THEN
  RAISE EXCEPTION 'template identity or version cannot be replaced' USING ERRCODE='23514';
 END IF;
 IF OLD.state='published' AND (NEW.state<>'archived' OR
  (NEW.name,NEW.description,NEW.task_and_folder_schema,NEW.published_at,NEW.published_by)
   IS DISTINCT FROM (OLD.name,OLD.description,OLD.task_and_folder_schema,OLD.published_at,OLD.published_by)) THEN
  RAISE EXCEPTION 'published template content is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.state='draft' AND NEW.state NOT IN('draft','published','archived') THEN
  RAISE EXCEPTION 'invalid template state' USING ERRCODE='23514';
 END IF;
 IF NEW.state='published' AND OLD.state='draft' AND (NEW.published_at IS NULL OR NEW.published_by IS NULL) THEN
  RAISE EXCEPTION 'template publication requires attribution' USING ERRCODE='23514';
 END IF;
 IF NEW.state='archived' AND (NEW.retired_at IS NULL OR NEW.retired_by IS NULL) THEN
  RAISE EXCEPTION 'template retirement requires attribution' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;$$;
CREATE TRIGGER project_template_revision_guard BEFORE INSERT OR UPDATE OR DELETE ON project_templates
 FOR EACH ROW EXECUTE FUNCTION guard_project_template_revision();
-- Legacy imported rows retain unknown attribution. New managed revisions
-- require their exact snapshot in append-only history, in the same tx.
CREATE FUNCTION check_project_template_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.creator_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM project_template_events e WHERE e.org_id=NEW.org_id AND e.template_id=NEW.id AND e.version=NEW.version
  AND e.snapshot->>'id'=NEW.id::text AND e.snapshot->>'key'=NEW.key
  AND (e.snapshot->>'revision')::bigint=NEW.revision AND (e.snapshot->>'version')::bigint=NEW.version
  AND e.snapshot->>'content_hash'=e.content_hash AND e.snapshot->>'state'=NEW.state
  AND e.snapshot->>'name'=NEW.name AND e.snapshot->>'description'=NEW.description
  AND e.snapshot->'schema'=NEW.task_and_folder_schema) THEN
  RAISE EXCEPTION 'managed template revision requires its exact history' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END;$$;
CREATE CONSTRAINT TRIGGER project_template_event_consistency AFTER INSERT OR UPDATE ON project_templates
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_template_event();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON project_template_events TO copypaste_app;
 REVOKE UPDATE,DELETE ON project_template_events FROM copypaste_app;
 REVOKE DELETE ON project_templates FROM copypaste_app;
END IF;END;$$;
