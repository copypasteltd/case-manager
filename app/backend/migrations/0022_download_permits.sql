-- Download permits contain immutable scope and source evidence, never bearer
-- URLs, cookies, tokens or provider credentials. A permit ID is not authorization.
CREATE TABLE download_permits (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, user_id uuid NOT NULL, session_id uuid NOT NULL, session_family_id uuid NOT NULL,
    file_id uuid NOT NULL, version_id uuid NOT NULL, replica_id uuid NOT NULL,
    provider text NOT NULL CHECK(provider IN ('oss','r2')), bucket text NOT NULL,
    source jsonb NOT NULL CHECK(jsonb_typeof(source)='object'),
    filename text NOT NULL CHECK(length(filename) BETWEEN 1 AND 200),
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    mode text NOT NULL CHECK(mode IN ('signed_url','authenticated_proxy')),
    disposition text NOT NULL CHECK(disposition IN ('inline','attachment')),
    channel text NOT NULL CHECK(channel IN ('web','android','wechat','douyin')),
    auth_version bigint NOT NULL CHECK(auth_version>0), org_auth_version bigint NOT NULL CHECK(org_auth_version>0),
    created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
    UNIQUE(org_id,id), FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),
    FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id),
    FOREIGN KEY(org_id,replica_id) REFERENCES file_replicas(org_id,id),
    CHECK(expires_at>created_at AND expires_at<=created_at+interval '12 hours'),
    CHECK(classification<>'confidential' OR mode='authenticated_proxy')
);
CREATE TRIGGER download_permit_immutable BEFORE UPDATE OR DELETE ON download_permits
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE INDEX download_permit_owner ON download_permits(org_id,user_id,expires_at);
CREATE TABLE download_read_events (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, permit_id uuid NOT NULL, read_id uuid NOT NULL,
    stage text NOT NULL CHECK(stage IN ('started','completed','partial','failed')),
    range_offset bigint NOT NULL CHECK(range_offset>=0),
    range_length bigint NOT NULL CHECK(range_length BETWEEN 0 AND 33554432),
    bytes_written bigint NOT NULL CHECK(bytes_written BETWEEN 0 AND range_length),
    result_code text NOT NULL CHECK(length(result_code) BETWEEN 1 AND 100),
    request_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY(org_id,permit_id) REFERENCES download_permits(org_id,id),
    UNIQUE(org_id,read_id,stage),
    CHECK(stage<>'started' OR bytes_written=0),
    CHECK(stage<>'completed' OR bytes_written=range_length)
);
CREATE UNIQUE INDEX download_read_single_outcome ON download_read_events(org_id,read_id) WHERE stage<>'started';
CREATE TRIGGER download_read_event_immutable BEFORE UPDATE OR DELETE ON download_read_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON download_permits,download_read_events TO copypaste_app;
    END IF;
END;
$$;
