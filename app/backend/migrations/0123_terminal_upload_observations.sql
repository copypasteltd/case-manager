-- Independent append-only HEAD observations, never upload or transfer success.
CREATE TABLE terminal_upload_observations(
 id uuid PRIMARY KEY,org_id uuid NOT NULL REFERENCES organizations(id),folder_id uuid NOT NULL,upload_id uuid NOT NULL,
 actor_id uuid NOT NULL,session_id uuid NOT NULL,channel text NOT NULL CHECK(channel IN('web','android','wechat','douyin')),
 actor_auth_version bigint NOT NULL,organization_auth_version bigint NOT NULL,folder_version bigint NOT NULL,upload_version bigint NOT NULL,
 basis_sha256 text NOT NULL CHECK(basis_sha256~'^[a-f0-9]{64}$'),snapshot jsonb NOT NULL,mfa_verified_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 200),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,
 UNIQUE(org_id,id),FOREIGN KEY(org_id,folder_id) REFERENCES folders(org_id,id),FOREIGN KEY(org_id,upload_id) REFERENCES upload_sessions(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id),CHECK(expires_at>created_at)
);
CREATE TABLE terminal_upload_observation_results(
 id uuid PRIMARY KEY,org_id uuid NOT NULL,request_id uuid NOT NULL,all_present boolean NOT NULL,
 outcome text NOT NULL CHECK(outcome IN('observed','incomplete','stopped')),error_code text,
 objects jsonb NOT NULL CHECK(jsonb_typeof(objects)='array' AND jsonb_array_length(objects)<=64),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(org_id,id),UNIQUE(org_id,request_id),
 FOREIGN KEY(org_id,request_id) REFERENCES terminal_upload_observations(org_id,id),CHECK(NOT all_present OR outcome='observed')
);
CREATE INDEX terminal_upload_observations_target ON terminal_upload_observations(org_id,upload_id,created_at DESC,id DESC);
CREATE TRIGGER terminal_observation_immutable BEFORE UPDATE OR DELETE ON terminal_upload_observations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER terminal_observation_result_immutable BEFORE UPDATE OR DELETE ON terminal_upload_observation_results FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
 GRANT SELECT,INSERT ON terminal_upload_observations,terminal_upload_observation_results TO copypaste_app;
 REVOKE UPDATE,DELETE ON terminal_upload_observations,terminal_upload_observation_results FROM copypaste_app;
 END IF; END $$;
