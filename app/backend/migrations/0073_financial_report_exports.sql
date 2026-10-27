-- Immutable business report snapshots, not durable CSV files. CSV is rendered
-- on authenticated download; no original is written to the server filesystem.
CREATE TABLE finance_report_exports (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES organizations(id),
    requester_id uuid NOT NULL,
    channel text NOT NULL CHECK(channel IN ('web','android','wechat','douyin')),
    auth_version bigint NOT NULL CHECK(auth_version>0),
    authorization_version bigint NOT NULL CHECK(authorization_version>0),
    filters jsonb NOT NULL CHECK(jsonb_typeof(filters)='object'),
    job_id uuid NOT NULL UNIQUE REFERENCES jobs(id),
    reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
    UNIQUE(org_id,id),
    FOREIGN KEY(org_id,requester_id) REFERENCES users(org_id,id),
    CHECK(expires_at>created_at AND expires_at<=created_at+interval '24 hours 1 second')
);
CREATE INDEX finance_report_export_requester ON finance_report_exports(org_id,requester_id,created_at DESC,id DESC);
CREATE TABLE finance_report_snapshots (
    org_id uuid NOT NULL,
    export_id uuid NOT NULL,
    job_fencing_token bigint NOT NULL CHECK(job_fencing_token>0),
    payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
    snapshot_sha256 text NOT NULL CHECK(snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    csv_sha256 text NOT NULL CHECK(csv_sha256 ~ '^[0-9a-f]{64}$'),
    csv_bytes bigint NOT NULL CHECK(csv_bytes BETWEEN 1 AND 16777216),
    row_count integer NOT NULL CHECK(row_count BETWEEN 0 AND 50000),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,export_id),
    FOREIGN KEY(org_id,export_id) REFERENCES finance_report_exports(org_id,id),
    CHECK(octet_length(payload::text)<=33554432)
);
CREATE FUNCTION guard_finance_report_export() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j jobs; e finance_report_exports;
BEGIN
    IF TG_TABLE_NAME='finance_report_exports' THEN
        SELECT * INTO j FROM jobs WHERE org_id=NEW.org_id AND id=NEW.job_id;
        IF NOT FOUND OR j.kind<>'finance.report_export.v1' OR j.lane<>'reports' OR j.payload_version<>1
           OR j.dedup_key<>'finance-report-export/'||NEW.id::text OR j.payload<>jsonb_build_object('export_id',NEW.id) THEN
            RAISE EXCEPTION 'report export requires its exact versioned job' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    SELECT * INTO e FROM finance_report_exports WHERE org_id=COALESCE(NEW.org_id,OLD.org_id) AND id=COALESCE(NEW.export_id,OLD.export_id);
    IF TG_OP='DELETE' THEN
        IF NOT FOUND OR e.expires_at>clock_timestamp() THEN
            RAISE EXCEPTION 'unexpired report snapshot cannot be removed' USING ERRCODE='23514';
        END IF;
        RETURN OLD;
    END IF;
    SELECT * INTO j FROM jobs WHERE org_id=e.org_id AND id=e.job_id;
    IF e.expires_at<=clock_timestamp() OR j.state<>'running' OR j.lease_until<=clock_timestamp() OR j.fencing_token<>NEW.job_fencing_token
       OR NEW.payload->'report'->'filters' IS DISTINCT FROM e.filters
       OR (NEW.payload->>'protocol') IS DISTINCT FROM '1' OR (NEW.payload->'report'->>'scope') IS DISTINCT FROM 'authorized_records_only'
       OR jsonb_typeof(NEW.payload->'report'->'items') IS DISTINCT FROM 'array' OR jsonb_array_length(NEW.payload->'report'->'items')<>NEW.row_count THEN
        RAISE EXCEPTION 'report snapshot requires a live exact job and complete fixed scope' USING ERRCODE='23514';
    END IF;
    NEW.snapshot_sha256:=encode(sha256(convert_to(NEW.payload::text,'UTF8')),'hex');
    RETURN NEW;
END; $$;
CREATE TRIGGER guard_finance_report_export BEFORE INSERT ON finance_report_exports FOR EACH ROW EXECUTE FUNCTION guard_finance_report_export();
CREATE TRIGGER immutable_finance_report_export BEFORE UPDATE OR DELETE ON finance_report_exports FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER guard_finance_report_snapshot BEFORE INSERT OR DELETE ON finance_report_snapshots FOR EACH ROW EXECUTE FUNCTION guard_finance_report_export();
CREATE TRIGGER immutable_finance_report_snapshot BEFORE UPDATE ON finance_report_snapshots FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'finance_exporter','财务报表导出人',false FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('report.read'),('report.export')) a(action) WHERE key='finance_exporter' ON CONFLICT DO NOTHING;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT ON finance_report_exports TO copypaste_app;
        REVOKE UPDATE,DELETE ON finance_report_exports FROM copypaste_app;
        GRANT SELECT,INSERT,DELETE ON finance_report_snapshots TO copypaste_app;
        REVOKE UPDATE ON finance_report_snapshots FROM copypaste_app;
    END IF;
END; $$;
