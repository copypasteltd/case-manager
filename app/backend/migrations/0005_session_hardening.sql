ALTER TABLE auth_challenges ADD COLUMN auth_version bigint NOT NULL DEFAULT 1 CHECK(auth_version>0);
CREATE INDEX auth_challenges_expiry ON auth_challenges(expires_at) WHERE consumed_at IS NULL;
CREATE INDEX rate_limit_expiry ON rate_limits(window_start);
