CREATE TABLE services (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), project_id uuid,
    creator_id uuid NOT NULL, owner_id uuid NOT NULL,
    classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    name text NOT NULL CHECK(char_length(btrim(name)) BETWEEN 1 AND 200),
    host_label text NOT NULL CHECK(char_length(host_label)<=200),
    endpoint_label text NOT NULL CHECK(char_length(endpoint_label)<=500),
    description text NOT NULL CHECK(char_length(description)<=10000),
    lifecycle text NOT NULL DEFAULT 'active' CHECK(lifecycle IN ('active','archived')),
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE INDEX services_list ON services(org_id,lifecycle,updated_at DESC,id DESC);
CREATE FUNCTION preserve_service_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR NEW.creator_id<>OLD.creator_id
        OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.classification<>OLD.classification OR NEW.created_at<>OLD.created_at THEN
        RAISE EXCEPTION 'service identity and scope are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER service_scope_immutable BEFORE UPDATE ON services FOR EACH ROW EXECUTE FUNCTION preserve_service_scope();

CREATE TABLE service_sources (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, service_id uuid NOT NULL, creator_id uuid NOT NULL,
    current_revision_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,service_id,id),
    FOREIGN KEY(org_id,service_id) REFERENCES services(org_id,id), FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id)
);
CREATE TABLE service_source_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, source_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991), base_revision_id uuid,
    name text NOT NULL CHECK(char_length(btrim(name)) BETWEEN 1 AND 200),
    provider text NOT NULL CHECK(provider IN ('uptime_kuma','prometheus','adapter')),
    external_ref text NOT NULL CHECK(char_length(btrim(external_ref)) BETWEEN 1 AND 200),
    public_key bytea NOT NULL CHECK(octet_length(public_key)=32), enabled boolean NOT NULL,
    failure_count integer NOT NULL CHECK(failure_count BETWEEN 1 AND 10),
    recovery_count integer NOT NULL CHECK(recovery_count BETWEEN 1 AND 10),
    interval_seconds integer NOT NULL CHECK(interval_seconds BETWEEN 15 AND 60),
    stale_after_seconds integer NOT NULL CHECK(stale_after_seconds BETWEEN interval_seconds*3 AND 180),
    latency_limit_ms bigint NOT NULL CHECK(latency_limit_ms BETWEEN 1 AND 120000),
    published_by uuid NOT NULL, reason text NOT NULL CHECK(char_length(btrim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,source_id,id), UNIQUE(source_id,seq),
    FOREIGN KEY(org_id,source_id) REFERENCES service_sources(org_id,id),
    FOREIGN KEY(org_id,source_id,base_revision_id) REFERENCES service_source_revisions(org_id,source_id,id),
    FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id), CHECK((seq=1)=(base_revision_id IS NULL))
);
ALTER TABLE service_sources ADD FOREIGN KEY(org_id,id,current_revision_id)
    REFERENCES service_source_revisions(org_id,source_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER service_source_revision_immutable BEFORE UPDATE OR DELETE ON service_source_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_service_source_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; parent service_sources%ROWTYPE; current_seq bigint; latest bigint; total bigint;
BEGIN
    IF TG_TABLE_NAME='service_sources' THEN target:=NEW.id; ELSE target:=NEW.source_id; END IF;
    SELECT * INTO parent FROM service_sources WHERE org_id=NEW.org_id AND id=target;
    SELECT seq INTO current_seq FROM service_source_revisions WHERE org_id=NEW.org_id AND source_id=target AND id=parent.current_revision_id;
    SELECT max(seq),count(*) INTO latest,total FROM service_source_revisions WHERE org_id=NEW.org_id AND source_id=target;
    IF current_seq IS NULL OR current_seq<>parent.version OR current_seq<>latest OR latest<>total THEN
        RAISE EXCEPTION 'source head must reference its latest revision' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER service_source_head_consistent AFTER INSERT OR UPDATE ON service_sources
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_service_source_head();
CREATE CONSTRAINT TRIGGER service_source_revision_consistent AFTER INSERT ON service_source_revisions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_service_source_head();
CREATE FUNCTION preserve_service_source_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.org_id,NEW.id,NEW.service_id,NEW.creator_id,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.service_id,OLD.creator_id,OLD.created_at) THEN
        RAISE EXCEPTION 'source identity and service are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER service_source_scope_immutable BEFORE UPDATE ON service_sources
    FOR EACH ROW EXECUTE FUNCTION preserve_service_source_scope();
CREATE FUNCTION validate_service_source_base() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE base_seq bigint;
BEGIN
    IF NEW.seq>1 THEN
        SELECT seq INTO base_seq FROM service_source_revisions WHERE org_id=NEW.org_id AND source_id=NEW.source_id AND id=NEW.base_revision_id;
        IF base_seq IS NULL OR base_seq<>NEW.seq-1 THEN
            RAISE EXCEPTION 'source revision must extend its exact predecessor' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER service_source_base_valid BEFORE INSERT ON service_source_revisions
    FOR EACH ROW EXECUTE FUNCTION validate_service_source_base();
CREATE TABLE service_source_states (
    org_id uuid NOT NULL, source_id uuid NOT NULL, revision_id uuid NOT NULL,
    last_sequence bigint NOT NULL DEFAULT 0 CHECK(last_sequence BETWEEN 0 AND 9007199254740991),
    observed_at timestamptz, received_at timestamptz,
    state text NOT NULL DEFAULT 'unknown' CHECK(state IN ('operational','degraded','down','unknown')),
    failure_streak integer NOT NULL DEFAULT 0 CHECK(failure_streak BETWEEN 0 AND 10),
    success_streak integer NOT NULL DEFAULT 0 CHECK(success_streak BETWEEN 0 AND 10),
    PRIMARY KEY(org_id,revision_id), FOREIGN KEY(org_id,source_id,revision_id) REFERENCES service_source_revisions(org_id,source_id,id),
    CHECK((last_sequence=0 AND observed_at IS NULL AND received_at IS NULL AND state='unknown' AND failure_streak=0 AND success_streak=0)
        OR (last_sequence>0 AND observed_at IS NOT NULL AND received_at IS NOT NULL))
);
CREATE TABLE service_observations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, source_id uuid NOT NULL, revision_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK(sequence BETWEEN 1 AND 9007199254740991),
    observed_at timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    healthy boolean NOT NULL, latency_ms bigint CHECK(latency_ms BETWEEN 0 AND 3600000),
    signed_payload bytea NOT NULL CHECK(octet_length(signed_payload) BETWEEN 1 AND 4096),
    signature bytea NOT NULL CHECK(octet_length(signature)=64), payload_sha256 text NOT NULL CHECK(payload_sha256~'^[a-f0-9]{64}$'),
    applied boolean NOT NULL, ignored_reason text NOT NULL CHECK(ignored_reason IN ('','LATE_SEQUENCE','LATE_OBSERVATION','STALE_SAMPLE')),
    UNIQUE(org_id,id), UNIQUE(org_id,revision_id,sequence),
    FOREIGN KEY(org_id,source_id,revision_id) REFERENCES service_source_revisions(org_id,source_id,id),
    CHECK(NOT healthy OR latency_ms IS NOT NULL), CHECK(applied=(ignored_reason='')),
    CHECK(observed_at<=received_at+interval '15 seconds')
);
CREATE INDEX service_observations_history ON service_observations(org_id,source_id,received_at DESC,id DESC);
CREATE TRIGGER service_observation_immutable BEFORE UPDATE OR DELETE ON service_observations
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TABLE maintenance_windows (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, service_id uuid NOT NULL,
    start_at timestamptz NOT NULL, end_at timestamptz NOT NULL,
    reason text NOT NULL CHECK(char_length(btrim(reason)) BETWEEN 1 AND 200),
    created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,service_id) REFERENCES services(org_id,id),
    FOREIGN KEY(org_id,created_by) REFERENCES users(org_id,id),
    CHECK(end_at>start_at AND end_at<=start_at+interval '31 days'), CHECK(start_at>=created_at-interval '5 minutes')
);
CREATE TABLE maintenance_cancellations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, window_id uuid NOT NULL,
    reason text NOT NULL CHECK(char_length(btrim(reason)) BETWEEN 1 AND 200),
    cancelled_by uuid NOT NULL, cancelled_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,window_id), FOREIGN KEY(org_id,window_id) REFERENCES maintenance_windows(org_id,id),
    FOREIGN KEY(org_id,cancelled_by) REFERENCES users(org_id,id)
);
CREATE INDEX maintenance_service ON maintenance_windows(org_id,service_id,start_at DESC,id DESC);
CREATE TRIGGER maintenance_window_immutable BEFORE UPDATE OR DELETE ON maintenance_windows
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER maintenance_cancellation_immutable BEFORE UPDATE OR DELETE ON maintenance_cancellations
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE TABLE incidents (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id), creator_id uuid NOT NULL, owner_id uuid NOT NULL,
    title text NOT NULL CHECK(char_length(btrim(title)) BETWEEN 1 AND 200),
    severity text NOT NULL CHECK(severity IN ('low','medium','high','critical')),
    description text NOT NULL CHECK(char_length(description)<=10000),
    service_ids uuid[] NOT NULL CHECK(cardinality(service_ids) BETWEEN 1 AND 20),
    dedup_key text NOT NULL CHECK(char_length(btrim(dedup_key)) BETWEEN 1 AND 200),
    creation_sha256 text NOT NULL CHECK(creation_sha256~'^[a-f0-9]{64}$'),
    state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','investigating','monitoring','resolved')),
    resolution text NOT NULL DEFAULT '' CHECK(char_length(resolution)<=10000),
    postmortem text NOT NULL DEFAULT '' CHECK(char_length(postmortem)<=10000),
    version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991), resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,dedup_key),
    FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id), FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id),
    CHECK((state='resolved')=(resolved_at IS NOT NULL)), CHECK(state<>'resolved' OR char_length(btrim(resolution))>0)
);
CREATE TABLE incident_services (
    org_id uuid NOT NULL, incident_id uuid NOT NULL, service_id uuid NOT NULL, position integer NOT NULL CHECK(position BETWEEN 1 AND 20),
    PRIMARY KEY(org_id,incident_id,service_id), UNIQUE(org_id,incident_id,position),
    FOREIGN KEY(org_id,incident_id) REFERENCES incidents(org_id,id), FOREIGN KEY(org_id,service_id) REFERENCES services(org_id,id)
);
CREATE TABLE incident_timeline (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, incident_id uuid NOT NULL, seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991),
    state text NOT NULL CHECK(state IN ('open','investigating','monitoring','resolved')),
    note text NOT NULL CHECK(char_length(btrim(note)) BETWEEN 1 AND 10000),
    resolution text NOT NULL CHECK(char_length(resolution)<=10000), postmortem text NOT NULL CHECK(char_length(postmortem)<=10000),
    actor_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,incident_id,seq), FOREIGN KEY(org_id,incident_id) REFERENCES incidents(org_id,id),
    FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id)
);
CREATE INDEX incidents_list ON incidents(org_id,state,updated_at DESC,id DESC);
CREATE TRIGGER incident_services_immutable BEFORE UPDATE OR DELETE ON incident_services
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER incident_timeline_immutable BEFORE UPDATE OR DELETE ON incident_timeline
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_incident_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.org_id,NEW.creator_id,NEW.owner_id,NEW.title,NEW.severity,NEW.description,NEW.service_ids,NEW.dedup_key,NEW.creation_sha256,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.creator_id,OLD.owner_id,OLD.title,OLD.severity,OLD.description,OLD.service_ids,OLD.dedup_key,OLD.creation_sha256,OLD.created_at) THEN
        RAISE EXCEPTION 'incident identity and initial impact are immutable' USING ERRCODE='23514';
    END IF;
    IF OLD.state='resolved' AND NEW.state<>'resolved' THEN
        RAISE EXCEPTION 'resolved incident cannot silently reopen' USING ERRCODE='23514';
    END IF;
    IF OLD.resolved_at IS NOT NULL AND NEW.resolved_at IS DISTINCT FROM OLD.resolved_at THEN
        RAISE EXCEPTION 'original incident resolution time is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER incident_identity_immutable BEFORE UPDATE ON incidents FOR EACH ROW EXECUTE FUNCTION preserve_incident_identity();
CREATE FUNCTION validate_incident_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; parent incidents%ROWTYPE; actual uuid[]; latest incident_timeline%ROWTYPE; total bigint;
BEGIN
    IF TG_TABLE_NAME='incidents' THEN target:=NEW.id; ELSE target:=NEW.incident_id; END IF;
    SELECT * INTO parent FROM incidents WHERE org_id=NEW.org_id AND id=target;
    SELECT array_agg(service_id ORDER BY position) INTO actual FROM incident_services WHERE org_id=NEW.org_id AND incident_id=target;
    SELECT * INTO latest FROM incident_timeline WHERE org_id=NEW.org_id AND incident_id=target ORDER BY seq DESC LIMIT 1;
    SELECT count(*) INTO total FROM incident_timeline WHERE org_id=NEW.org_id AND incident_id=target;
    IF actual IS DISTINCT FROM parent.service_ids OR latest.id IS NULL OR latest.seq<>parent.version OR total<>latest.seq
        OR ROW(latest.state,latest.resolution,latest.postmortem) IS DISTINCT FROM ROW(parent.state,parent.resolution,parent.postmortem) THEN
        RAISE EXCEPTION 'incident head requires its exact impact and timeline' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER incident_head_consistent AFTER INSERT OR UPDATE ON incidents
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_incident_head();
CREATE CONSTRAINT TRIGGER incident_services_consistent AFTER INSERT ON incident_services
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_incident_head();
CREATE CONSTRAINT TRIGGER incident_timeline_consistent AFTER INSERT ON incident_timeline
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_incident_head();

INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'status_reader','服务状态查阅人',false FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'status_operator','服务状态维护人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT org_id,id,'service.read' FROM roles WHERE key='status_reader' ON CONFLICT DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN (VALUES('service.read'),('service.manage'),('incident.update')) a(action)
    WHERE r.key='status_operator' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON services,service_sources,service_source_states,incidents TO copypaste_app;
        GRANT SELECT,INSERT ON service_source_revisions,service_observations,maintenance_windows,maintenance_cancellations,incident_services,incident_timeline TO copypaste_app;
        REVOKE DELETE ON services,service_sources,service_source_states,incidents FROM copypaste_app;
        REVOKE UPDATE,DELETE ON service_source_revisions,service_observations,maintenance_windows,maintenance_cancellations,incident_services,incident_timeline FROM copypaste_app;
    END IF;
END;
$$;
