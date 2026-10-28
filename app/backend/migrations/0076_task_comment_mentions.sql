-- Explicit task mentions are immutable revision data, never parsed from prose.
ALTER TABLE comments ADD COLUMN mentioned_user_ids uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE comments ADD CONSTRAINT comments_task_mentions CHECK (
    cardinality(mentioned_user_ids)<=20 AND (resource_type='task' OR cardinality(mentioned_user_ids)=0)
);

CREATE FUNCTION task_comment_mention_eligible(org uuid, task uuid, author uuid, recipient uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT recipient<>author AND EXISTS (
  SELECT 1 FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
  JOIN project_members m ON m.org_id=t.org_id AND m.project_id=t.project_id AND m.user_id=recipient
  JOIN users u ON u.org_id=m.org_id AND u.id=m.user_id AND u.status='active'
  WHERE t.org_id=org AND t.id=task AND EXISTS (
   SELECT 1 FROM unnest(ARRAY['web','android','wechat','douyin']) channel
   WHERE can_access(org,recipient,'task.read','task',t.id,t.project_id,t.creator_id,p.classification,channel,false)
  )
 )
$$;

CREATE FUNCTION guard_task_comment_mentions() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE prior uuid[] := '{}'; recipient uuid;
BEGIN
 IF cardinality(NEW.mentioned_user_ids)<>(SELECT count(DISTINCT id) FROM unnest(NEW.mentioned_user_ids) id)
 THEN RAISE EXCEPTION 'invalid mention identities' USING ERRCODE='23514'; END IF;
 IF NEW.replaces_id IS NOT NULL THEN
  SELECT mentioned_user_ids INTO prior FROM comments
   WHERE org_id=NEW.org_id AND id=NEW.replaces_id AND resource_type=NEW.resource_type
   AND resource_id=NEW.resource_id AND author_id=NEW.author_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'invalid mention predecessor' USING ERRCODE='23514'; END IF;
 END IF;
 FOREACH recipient IN ARRAY NEW.mentioned_user_ids LOOP
  IF recipient=NEW.author_id OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=recipient)
   OR (NOT recipient=ANY(prior) AND NOT task_comment_mention_eligible(NEW.org_id,NEW.resource_id,NEW.author_id,recipient))
  THEN RAISE EXCEPTION 'ineligible task mention' USING ERRCODE='23514'; END IF;
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER task_comment_mentions_guard BEFORE INSERT ON comments FOR EACH ROW EXECUTE FUNCTION guard_task_comment_mentions();

ALTER TABLE notification_threads DROP CONSTRAINT notification_threads_category_check;
ALTER TABLE notification_threads ADD CONSTRAINT notification_threads_category_check CHECK (
 category IN ('task','mention','approval','request_result','contract_due','subscription_due','replica_failure','incident','secretary')
);
ALTER TABLE notification_threads ADD CONSTRAINT notification_task_mention_context CHECK (
 category<>'mention' OR (resource_type='task' AND context_id IS NOT NULL)
);
