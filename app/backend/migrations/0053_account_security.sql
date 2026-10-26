CREATE TABLE password_resets (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES organizations(id),
    user_id uuid NOT NULL,
    issuer_id uuid NOT NULL,
    token_hash text NOT NULL UNIQUE CHECK(token_hash ~ '^[a-f0-9]{64}$'),
    auth_version bigint NOT NULL CHECK(auth_version>0),
    issuer_auth_version bigint NOT NULL CHECK(issuer_auth_version>0),
    reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,issuer_id) REFERENCES users(org_id,id),
    CHECK(expires_at>created_at),
    CHECK(consumed_at IS NULL OR revoked_at IS NULL)
);
CREATE UNIQUE INDEX password_resets_one_pending ON password_resets(org_id,user_id)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX sessions_family_history ON sessions(org_id,user_id,family_id,created_at DESC,id DESC);
INSERT INTO role_actions(org_id,role_id,action)
    SELECT org_id,id,'user.password_reset' FROM roles WHERE key='system_admin'
    ON CONFLICT DO NOTHING;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON password_resets TO copypaste_app;
    END IF;
END $$;
