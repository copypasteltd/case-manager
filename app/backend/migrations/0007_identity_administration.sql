ALTER TABLE role_bindings ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE role_bindings ADD COLUMN revoked_at timestamptz;
ALTER TABLE role_bindings ADD COLUMN grantor_id uuid;
ALTER TABLE role_bindings ADD COLUMN reason text NOT NULL DEFAULT 'initial bootstrap';
ALTER TABLE role_bindings ADD FOREIGN KEY(org_id,grantor_id) REFERENCES users(org_id,id);
-- A revoked binding leaves the active uniqueness slot and cannot authorize again.
DROP INDEX role_bindings_scope;
CREATE UNIQUE INDEX role_bindings_scope ON role_bindings(org_id,user_id,role_id,scope_type,coalesce(scope_id,'00000000-0000-0000-0000-000000000000')) WHERE revoked_at IS NULL;
CREATE INDEX invitations_expiry ON invitations(expires_at) WHERE consumed_at IS NULL;
