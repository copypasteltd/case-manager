ALTER TABLE comments ADD COLUMN context_revision_id uuid;
ALTER TABLE comments ADD FOREIGN KEY(org_id,context_revision_id) REFERENCES post_revisions(org_id,id);
CREATE INDEX comments_context ON comments(org_id,resource_type,resource_id,context_revision_id,created_at DESC,id DESC);
