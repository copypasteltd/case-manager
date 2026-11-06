-- Preserve invitation identities and distinguish reissue from activation.
ALTER TABLE invitations ADD COLUMN revoked_at timestamptz;
ALTER TABLE invitations ADD COLUMN revoked_by uuid;
ALTER TABLE invitations ADD COLUMN replaces_id uuid;
ALTER TABLE invitations ADD COLUMN reissue_reason text;
ALTER TABLE invitations ADD CONSTRAINT invitations_org_identity UNIQUE(org_id,id);
ALTER TABLE invitations ADD CONSTRAINT invitations_revoker FOREIGN KEY(org_id,revoked_by) REFERENCES users(org_id,id);
ALTER TABLE invitations ADD CONSTRAINT invitations_replaces FOREIGN KEY(org_id,replaces_id) REFERENCES invitations(org_id,id);
ALTER TABLE invitations ADD CONSTRAINT invitations_revocation_pair CHECK ((revoked_at IS NULL)=(revoked_by IS NULL));
ALTER TABLE invitations ADD CONSTRAINT invitations_not_consumed_and_revoked CHECK (consumed_at IS NULL OR revoked_at IS NULL);
ALTER TABLE invitations ADD CONSTRAINT invitations_reissue_reason CHECK (
    (replaces_id IS NULL AND reissue_reason IS NULL) OR
    (replaces_id IS NOT NULL AND reissue_reason IS NOT NULL AND char_length(reissue_reason) BETWEEN 1 AND 200));
CREATE UNIQUE INDEX invitations_single_successor ON invitations(org_id,replaces_id) WHERE replaces_id IS NOT NULL;
