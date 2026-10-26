-- "Own" scope means created by the actor, rather than whoever is currently
-- assigned to the project or task. Ownership changes must not widen that scope.
ALTER TABLE projects ADD COLUMN creator_id uuid;
UPDATE projects p SET creator_id=coalesce((SELECT a.actor_id FROM audit_events a
    WHERE a.org_id=p.org_id AND a.resource_id=p.id AND a.action='project.created' ORDER BY a.occurred_at LIMIT 1),p.owner_id);
ALTER TABLE projects ALTER COLUMN creator_id SET NOT NULL;
ALTER TABLE projects ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE tasks ADD COLUMN creator_id uuid;
UPDATE tasks t SET creator_id=coalesce((SELECT a.actor_id FROM audit_events a
    WHERE a.org_id=t.org_id AND a.resource_id=t.id AND a.action='task.created' ORDER BY a.occurred_at LIMIT 1),
    (SELECT p.creator_id FROM projects p WHERE p.org_id=t.org_id AND p.id=t.project_id));
ALTER TABLE tasks ALTER COLUMN creator_id SET NOT NULL;
ALTER TABLE tasks ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
