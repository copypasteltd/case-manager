CREATE TABLE organizations (
    id uuid PRIMARY KEY,
    singleton boolean NOT NULL DEFAULT true UNIQUE CHECK(singleton),
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
    default_timezone text NOT NULL DEFAULT 'Asia/Hong_Kong',
    base_currency text,
    authorization_version bigint NOT NULL DEFAULT 1 CHECK(authorization_version>0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE users (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    login_key text NOT NULL, display_name text NOT NULL,
    status text NOT NULL CHECK(status IN ('invited','active','disabled')),
    credential_hash text NOT NULL,
    auth_version bigint NOT NULL DEFAULT 1 CHECK(auth_version>0),
    disabled_at timestamptz, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,login_key),
    CHECK(login_key=lower(trim(login_key)))
);
CREATE TABLE invitations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    user_id uuid NOT NULL, target_login text NOT NULL, token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL, consumed_at timestamptz,
    inviter_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,inviter_id) REFERENCES users(org_id,id)
);
CREATE TABLE sessions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    user_id uuid NOT NULL, family_id uuid NOT NULL,
    access_hash text NOT NULL UNIQUE, refresh_hash text NOT NULL UNIQUE, csrf_hash text NOT NULL,
    device_type text NOT NULL CHECK(device_type IN ('web','wechat','douyin','android')),
    expires_at timestamptz NOT NULL, refresh_expires_at timestamptz NOT NULL,
    revoked_at timestamptz, refreshed_at timestamptz, auth_version bigint NOT NULL,
    mfa_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE INDEX sessions_user ON sessions(org_id,user_id,revoked_at);
CREATE TABLE mfa_credentials (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL,
    secret_cipher bytea NOT NULL, recovery_hashes text[] NOT NULL,
    last_step bigint NOT NULL DEFAULT 0, enabled_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,user_id), FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE TABLE auth_challenges (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL,
    token_hash text NOT NULL UNIQUE, purpose text NOT NULL CHECK(purpose IN ('mfa','mfa_enroll','reset','external_bind')),
    device_type text NOT NULL, secret_cipher bytea, provider text, expires_at timestamptz NOT NULL,
    consumed_at timestamptz, attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE TABLE roles (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    key text NOT NULL, name text NOT NULL, requires_mfa boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,key)
);
CREATE TABLE role_actions (
    org_id uuid NOT NULL, role_id uuid NOT NULL, action text NOT NULL CHECK(action<>'*'),
    PRIMARY KEY(role_id,action), FOREIGN KEY(org_id,role_id) REFERENCES roles(org_id,id)
);
CREATE TABLE role_bindings (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL, role_id uuid NOT NULL,
    scope_type text NOT NULL CHECK(scope_type IN ('org','project','resource','own')),
    scope_id uuid, classification_ceiling text NOT NULL DEFAULT 'project_restricted'
        CHECK(classification_ceiling IN ('internal','project_restricted','confidential')),
    valid_from timestamptz NOT NULL DEFAULT now(), valid_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,role_id) REFERENCES roles(org_id,id),
    CHECK((scope_type IN ('org','own') AND scope_id IS NULL) OR (scope_type IN ('project','resource') AND scope_id IS NOT NULL)),
    CHECK(valid_until IS NULL OR valid_until>valid_from)
);
CREATE UNIQUE INDEX role_bindings_scope ON role_bindings(org_id,user_id,role_id,scope_type,coalesce(scope_id,'00000000-0000-0000-0000-000000000000'));
CREATE TABLE resource_grants (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL,
    resource_type text NOT NULL, resource_id uuid NOT NULL, action text NOT NULL,
    classification_ceiling text NOT NULL CHECK(classification_ceiling IN ('internal','project_restricted','confidential')),
    expires_at timestamptz, grantor_id uuid NOT NULL, reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,grantor_id) REFERENCES users(org_id,id)
);
CREATE INDEX grants_lookup ON resource_grants(org_id,user_id,resource_type,resource_id,action);
CREATE TABLE classification_policies (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    level text NOT NULL CHECK(level IN ('internal','project_restricted','confidential')),
    ai_allowed boolean NOT NULL DEFAULT false,
    allowed_channels text[] NOT NULL DEFAULT ARRAY['web','wechat','douyin','android'],
    policy_version bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,level,policy_version)
);
CREATE TABLE external_identities (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL,
    provider text NOT NULL CHECK(provider IN ('wechat','douyin','telegram')),
    subject text NOT NULL, verified_at timestamptz NOT NULL, revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX identities_active_subject ON external_identities(org_id,provider,subject) WHERE revoked_at IS NULL;
CREATE TABLE rate_limits (
    bucket text NOT NULL, window_start timestamptz NOT NULL, attempts integer NOT NULL,
    PRIMARY KEY(bucket,window_start)
);
CREATE TABLE idempotency_records (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), actor_id uuid NOT NULL,
    route_key text NOT NULL, idempotency_key text NOT NULL,
    request_hash text NOT NULL CHECK(request_hash~'^[0-9a-f]{64}$'),
    state text NOT NULL CHECK(state IN ('in_progress','succeeded')),
    response_status integer NOT NULL DEFAULT 0, response_data jsonb NOT NULL DEFAULT 'null',
    expires_at timestamptz NOT NULL, completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,actor_id,route_key,idempotency_key),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE TABLE audit_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), actor_id uuid, executor_id uuid,
    channel text NOT NULL, request_id text NOT NULL, run_id uuid, job_id uuid,
    action text NOT NULL, resource_type text NOT NULL, resource_id uuid, resource_version bigint,
    before_digest text, after_digest text, result text NOT NULL, reason text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,executor_id) REFERENCES users(org_id,id)
);
CREATE INDEX audit_lookup ON audit_events(org_id,occurred_at DESC,id DESC);
CREATE FUNCTION reject_immutable_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable record' USING ERRCODE='23514'; END;
$$;
CREATE TRIGGER audit_append_only BEFORE UPDATE OR DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE outbox_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    event_type text NOT NULL, schema_version integer NOT NULL DEFAULT 1,
    aggregate_type text NOT NULL, aggregate_id uuid NOT NULL, aggregate_version bigint NOT NULL,
    payload jsonb NOT NULL, request_id text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz
);
CREATE INDEX outbox_pending ON outbox_events(occurred_at,id) WHERE published_at IS NULL;
CREATE TABLE jobs (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    kind text NOT NULL, lane text NOT NULL, dedup_key text NOT NULL, payload_version integer NOT NULL DEFAULT 1,
    payload jsonb NOT NULL, state text NOT NULL CHECK(state IN ('queued','running','succeeded','retry_wait','dead','cancelled')),
    attempt integer NOT NULL DEFAULT 0, max_attempts integer NOT NULL DEFAULT 8,
    available_at timestamptz NOT NULL DEFAULT now(), lease_until timestamptz, lease_owner text,
    fencing_token bigint NOT NULL DEFAULT 0, last_error text, first_failed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,dedup_key)
);
CREATE INDEX jobs_runnable ON jobs(lane,state,available_at,lease_until);
CREATE TABLE processed_events (
    consumer text NOT NULL, event_id uuid NOT NULL REFERENCES outbox_events(id), result_ref uuid,
    processed_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(consumer,event_id)
);
CREATE TABLE config_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), namespace text NOT NULL,
    revision bigint NOT NULL, content jsonb NOT NULL, hash text NOT NULL,
    approved_by uuid, effective_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,namespace,revision), FOREIGN KEY(org_id,approved_by) REFERENCES users(org_id,id)
);
CREATE TRIGGER config_append_only BEFORE UPDATE OR DELETE ON config_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE currency_definitions (
    code text PRIMARY KEY CHECK(code~'^[A-Z]{3}$'), exponent integer NOT NULL CHECK(exponent BETWEEN 0 AND 6)
);
INSERT INTO currency_definitions VALUES('HKD',2),('USD',2),('CNY',2),('EUR',2),('GBP',2),('JPY',0);
CREATE TABLE storage_epochs (
    org_id uuid NOT NULL REFERENCES organizations(id), epoch bigint NOT NULL CHECK(epoch>0),
    active_primary text NOT NULL CHECK(active_primary IN ('oss','r2')),
    changed_by uuid, changed_at timestamptz NOT NULL DEFAULT now(), fence_evidence text NOT NULL,
    PRIMARY KEY(org_id,epoch), FOREIGN KEY(org_id,changed_by) REFERENCES users(org_id,id)
);
