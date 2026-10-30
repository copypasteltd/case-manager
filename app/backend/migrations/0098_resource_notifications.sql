-- Live work is computed from original observations; the delivery head is not
-- trusted as a substitute for current resource evidence.
CREATE FUNCTION resource_capacity_bp(v jsonb) RETURNS numeric LANGUAGE sql IMMUTABLE AS $$
 SELECT ceil(((v->>'total_bytes')::numeric-(v->>'available_bytes')::numeric)*10000/(v->>'total_bytes')::numeric)
$$;
CREATE FUNCTION service_resource_current(p_org uuid,p_source uuid,p_at timestamptz)
RETURNS TABLE(revision_id uuid,state text,state_reason text,observation_id uuid,ready_after timestamptz,observed_at timestamptz,received_at timestamptz)
LANGUAGE sql STABLE AS $$
 WITH basis AS (
 SELECT r.id revision_id,r.config,r.enabled,r.created_at,s.lifecycle,o.id observation_id,o.observed_at,o.received_at,o.sample,
 r.created_at+make_interval(secs=>(r.config->>'stale_after_seconds')::int) ready_after,
 make_interval(secs=>(r.config->>'stale_after_seconds')::int) ttl
 FROM service_resource_sources h JOIN services s ON s.org_id=h.org_id AND s.id=h.service_id
 JOIN service_resource_revisions r ON r.org_id=h.org_id AND r.source_id=h.id AND r.id=h.current_revision_id
 LEFT JOIN LATERAL (SELECT x.* FROM service_resource_observations x WHERE x.org_id=h.org_id AND x.revision_id=r.id AND x.applied ORDER BY x.sequence DESC LIMIT 1) o ON true
 WHERE h.org_id=p_org AND h.id=p_source
 ), evaluated AS (
 SELECT *,CASE WHEN lifecycle<>'active' THEN 'SERVICE_ARCHIVED' WHEN NOT enabled THEN 'SOURCE_DISABLED'
 WHEN observation_id IS NULL THEN 'NO_OBSERVATION' WHEN observed_at>p_at OR received_at>p_at THEN 'CLOCK_SKEW'
 WHEN observed_at+ttl<=p_at OR received_at+ttl<=p_at THEN 'OBSERVATION_STALE' ELSE '' END reason FROM basis
 )
 SELECT revision_id,CASE WHEN reason<>'' THEN 'unknown'
 WHEN (sample->>'cpu_usage_bp')::int >= (config#>>'{cpu,critical_bp}')::int
 OR resource_capacity_bp(sample->'memory') >= (config#>>'{memory,critical_bp}')::int
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(sample->'filesystems') d WHERE (d->>'read_only')::bool OR resource_capacity_bp(d)>=(config#>>'{disk,critical_bp}')::int) THEN 'critical'
 WHEN (sample->>'cpu_usage_bp')::int >= (config#>>'{cpu,warning_bp}')::int
 OR resource_capacity_bp(sample->'memory') >= (config#>>'{memory,warning_bp}')::int
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(sample->'filesystems') d WHERE resource_capacity_bp(d)>=(config#>>'{disk,warning_bp}')::int) THEN 'degraded'
 ELSE 'operational' END,reason,observation_id,ready_after,observed_at,received_at FROM evaluated
$$;
CREATE FUNCTION resource_needs_attention(p_state text,p_reason text,p_ready timestamptz,p_at timestamptz)
RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT p_state IN ('critical','degraded') OR p_state='unknown' AND
 (p_reason IN ('CLOCK_SKEW','OBSERVATION_STALE') OR p_reason='NO_OBSERVATION' AND p_at>=p_ready)
$$;

CREATE TABLE service_resource_health (
 source_id uuid PRIMARY KEY,org_id uuid NOT NULL,revision_id uuid NOT NULL,recipient_id uuid NOT NULL,
 seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991),event_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('unknown','operational','degraded','critical')),state_reason text NOT NULL,
 attention boolean NOT NULL,evaluated_at timestamptz NOT NULL,
 UNIQUE(org_id,source_id),
 FOREIGN KEY(org_id,source_id) REFERENCES service_resource_sources(org_id,id),
 FOREIGN KEY(org_id,source_id,revision_id) REFERENCES service_resource_revisions(org_id,source_id,id),
 FOREIGN KEY(org_id,recipient_id) REFERENCES users(org_id,id)
);
CREATE TABLE service_resource_health_events (
 id uuid PRIMARY KEY,org_id uuid NOT NULL,source_id uuid NOT NULL,revision_id uuid NOT NULL,recipient_id uuid NOT NULL,
 seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991),base_event_id uuid,
 previous_state text NOT NULL CHECK(previous_state IN ('unconfigured','unknown','operational','degraded','critical')),
 state text NOT NULL CHECK(state IN ('unknown','operational','degraded','critical')),state_reason text NOT NULL,
 attention boolean NOT NULL,notify boolean NOT NULL,
 change_kind text NOT NULL CHECK(change_kind IN ('initial','configuration','responsibility','state')),
 observation_id uuid,evaluated_at timestamptz NOT NULL,
 UNIQUE(org_id,id),UNIQUE(org_id,source_id,id),UNIQUE(source_id,seq),
 FOREIGN KEY(org_id,source_id) REFERENCES service_resource_sources(org_id,id),
 FOREIGN KEY(org_id,source_id,revision_id) REFERENCES service_resource_revisions(org_id,source_id,id),
 FOREIGN KEY(org_id,recipient_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,source_id,base_event_id) REFERENCES service_resource_health_events(org_id,source_id,id),
 FOREIGN KEY(org_id,observation_id) REFERENCES service_resource_observations(org_id,id),
 FOREIGN KEY(org_id,id) REFERENCES outbox_events(org_id,id),CHECK((seq=1)=(base_event_id IS NULL))
);
ALTER TABLE service_resource_health ADD FOREIGN KEY(org_id,source_id,event_id)
 REFERENCES service_resource_health_events(org_id,source_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX resource_health_event_history ON service_resource_health_events(org_id,source_id,evaluated_at DESC,id DESC);
CREATE TRIGGER resource_health_event_immutable BEFORE UPDATE OR DELETE ON service_resource_health_events
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION validate_resource_health_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_basis record; prior service_resource_health_events%ROWTYPE; owner uuid;
BEGIN
 SELECT * INTO current_basis FROM service_resource_current(NEW.org_id,NEW.source_id,NEW.evaluated_at);
 SELECT s.owner_id INTO owner FROM services s JOIN service_resource_sources h ON h.org_id=s.org_id AND h.service_id=s.id WHERE h.org_id=NEW.org_id AND h.id=NEW.source_id;
 SELECT * INTO prior FROM service_resource_health_events WHERE org_id=NEW.org_id AND source_id=NEW.source_id AND id=NEW.base_event_id;
 IF current_basis.revision_id IS NULL OR
 (NEW.revision_id,NEW.recipient_id,NEW.state,NEW.state_reason,NEW.observation_id,NEW.attention) IS DISTINCT FROM
 (current_basis.revision_id,owner,current_basis.state,current_basis.state_reason,current_basis.observation_id,
 resource_needs_attention(current_basis.state,current_basis.state_reason,current_basis.ready_after,NEW.evaluated_at)) OR
 NEW.change_kind<>(CASE WHEN NEW.seq=1 THEN 'initial' WHEN prior.revision_id<>NEW.revision_id THEN 'configuration' WHEN prior.recipient_id<>NEW.recipient_id THEN 'responsibility' ELSE 'state' END) OR
 NEW.notify IS DISTINCT FROM (NEW.attention OR COALESCE(prior.revision_id=NEW.revision_id AND prior.recipient_id=NEW.recipient_id AND prior.attention AND NEW.state='operational',false)) OR
 NOT EXISTS(SELECT 1 FROM outbox_events WHERE org_id=NEW.org_id AND id=NEW.id AND event_type='service.resource_health.changed' AND aggregate_type='service_resource_source' AND aggregate_id=NEW.source_id AND aggregate_version=NEW.seq AND schema_version=1) THEN
  RAISE EXCEPTION 'resource state event must match its original source and outbox' USING ERRCODE='23514';
 END IF;
 IF NEW.seq>1 AND NOT EXISTS(SELECT 1 FROM service_resource_health_events WHERE org_id=NEW.org_id AND source_id=NEW.source_id AND id=NEW.base_event_id AND seq=NEW.seq-1 AND state=NEW.previous_state) THEN
  RAISE EXCEPTION 'resource state event must extend exact predecessor' USING ERRCODE='23514';
 END IF;
 IF NEW.seq=1 AND NEW.previous_state<>'unconfigured' OR NEW.observation_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM service_resource_observations WHERE org_id=NEW.org_id AND id=NEW.observation_id AND source_id=NEW.source_id AND revision_id=NEW.revision_id AND applied) THEN
  RAISE EXCEPTION 'resource state event basis is invalid' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_health_event_basis BEFORE INSERT ON service_resource_health_events FOR EACH ROW EXECUTE FUNCTION validate_resource_health_event();
CREATE FUNCTION protect_resource_health_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.org_id,NEW.source_id) IS DISTINCT FROM (OLD.org_id,OLD.source_id) THEN
  RAISE EXCEPTION 'resource state head identity is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_health_identity BEFORE UPDATE ON service_resource_health FOR EACH ROW EXECUTE FUNCTION protect_resource_health_identity();
CREATE FUNCTION validate_resource_health_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; h service_resource_health%ROWTYPE; e service_resource_health_events%ROWTYPE; latest bigint; total bigint;
BEGIN
 target:=NEW.source_id;
 SELECT * INTO h FROM service_resource_health WHERE org_id=NEW.org_id AND source_id=target;
 SELECT * INTO e FROM service_resource_health_events WHERE org_id=NEW.org_id AND source_id=target AND id=h.event_id;
 SELECT max(seq),count(*) INTO latest,total FROM service_resource_health_events WHERE org_id=NEW.org_id AND source_id=target;
 IF e.id IS NULL OR h.seq<>e.seq OR h.seq<>latest OR latest<>total OR
 (h.revision_id,h.recipient_id,h.state,h.state_reason,h.attention,h.evaluated_at) IS DISTINCT FROM (e.revision_id,e.recipient_id,e.state,e.state_reason,e.attention,e.evaluated_at) THEN
  RAISE EXCEPTION 'resource state head must reference latest exact event' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER resource_health_head_consistent AFTER INSERT OR UPDATE ON service_resource_health
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_resource_health_head();
CREATE CONSTRAINT TRIGGER resource_health_event_consistent AFTER INSERT ON service_resource_health_events
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_resource_health_head();

ALTER TABLE notification_threads DROP CONSTRAINT notification_threads_category_check;
ALTER TABLE notification_threads ADD CHECK(category IN ('mention','task','approval','request_result','contract_due','subscription_due','replica_failure','incident','secretary','service_resource'));
ALTER TABLE notification_threads DROP CONSTRAINT notification_threads_resource_type_check;
ALTER TABLE notification_threads ADD CHECK(resource_type IN ('task','request','contract','subscription','file','incident','agent_run','service'));
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT,UPDATE ON service_resource_health TO copypaste_app;
  GRANT SELECT,INSERT ON service_resource_health_events TO copypaste_app;
 END IF;
END $$;
