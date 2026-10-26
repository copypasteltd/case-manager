ALTER TABLE post_revisions ADD COLUMN title text NOT NULL DEFAULT '';
ALTER TABLE post_revisions ADD COLUMN tags text[] NOT NULL DEFAULT '{}';
-- Existing deployments may have historical bodies without historical titles.
-- Preserve that boundary rather than fabricate an earlier title.
ALTER TABLE post_revisions ADD CONSTRAINT revision_title_length CHECK(length(title)<=200);
ALTER TABLE audit_events ADD COLUMN session_id uuid;
ALTER TABLE audit_events ADD FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id);
CREATE INDEX posts_list ON posts(org_id,project_id,state,updated_at DESC,id DESC);
CREATE INDEX post_revisions_history ON post_revisions(org_id,post_id,seq DESC);
CREATE UNIQUE INDEX comments_single_successor ON comments(org_id,replaces_id) WHERE replaces_id IS NOT NULL;
