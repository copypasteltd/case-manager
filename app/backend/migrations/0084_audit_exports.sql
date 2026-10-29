-- Organization audit metadata only. Exact immutable event references avoid
-- storing exported files or business content on the API host.
ALTER TABLE audit_events ADD CONSTRAINT audit_org_id_unique UNIQUE(org_id,id);
CREATE TABLE audit_exports (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES organizations(id),
 requester_id uuid NOT NULL,
 channel text NOT NULL CHECK(channel IN ('web','android','wechat','douyin')),
 auth_version bigint NOT NULL CHECK(auth_version>0),
 authorization_version bigint NOT NULL CHECK(authorization_version>0),
 filters jsonb NOT NULL CHECK(jsonb_typeof(filters)='object'),
 disclosure text NOT NULL CHECK(disclosure='organization_metadata_v1'),
 reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
 row_count integer NOT NULL CHECK(row_count BETWEEN 0 AND 10000),
 csv_bytes integer NOT NULL CHECK(csv_bytes BETWEEN 1 AND 8388608),
 csv_sha256 text NOT NULL CHECK(csv_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
 UNIQUE(org_id,id),
 FOREIGN KEY(org_id,requester_id) REFERENCES users(org_id,id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '24 hours 1 second')
);
CREATE INDEX audit_exports_requester ON audit_exports(org_id,requester_id,created_at DESC,id DESC);
CREATE TABLE audit_export_events (
 org_id uuid NOT NULL,
 export_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 1 AND 10000),
 event_id uuid NOT NULL,
 PRIMARY KEY(org_id,export_id,position),
 UNIQUE(org_id,export_id,event_id),
 FOREIGN KEY(org_id,export_id) REFERENCES audit_exports(org_id,id),
 FOREIGN KEY(org_id,event_id) REFERENCES audit_events(org_id,id)
);
CREATE TRIGGER audit_exports_immutable BEFORE UPDATE OR DELETE ON audit_exports FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER audit_export_events_immutable BEFORE UPDATE OR DELETE ON audit_export_events FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_audit_export_count() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n integer; last_position integer;
BEGIN
 SELECT count(*),COALESCE(max(position),0) INTO n,last_position FROM audit_export_events WHERE org_id=NEW.org_id AND export_id=NEW.id;
 IF n<>NEW.row_count OR last_position<>NEW.row_count THEN
  RAISE EXCEPTION 'audit export must commit its complete exact event set' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER audit_export_complete AFTER INSERT ON audit_exports DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_audit_export_count();
CREATE FUNCTION guard_audit_export_position() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM audit_exports WHERE org_id=NEW.org_id AND id=NEW.export_id AND NEW.position<=row_count) THEN
  RAISE EXCEPTION 'audit export event set is sealed' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_export_position BEFORE INSERT ON audit_export_events FOR EACH ROW EXECUTE FUNCTION guard_audit_export_position();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT ON audit_exports,audit_export_events TO copypaste_app;
  REVOKE UPDATE,DELETE ON audit_exports,audit_export_events FROM copypaste_app;
 END IF;
END $$;
