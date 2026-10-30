-- Resource evidence is separate from service endpoint availability.
CREATE TABLE service_resource_sources (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, service_id uuid NOT NULL, creator_id uuid NOT NULL,
 current_revision_id uuid NOT NULL, version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(org_id,service_id),
 FOREIGN KEY(org_id,service_id) REFERENCES services(org_id,id),
 FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id)
);
CREATE TABLE service_resource_revisions (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, source_id uuid NOT NULL,
 seq bigint NOT NULL CHECK(seq BETWEEN 1 AND 9007199254740991), base_revision_id uuid,
 config jsonb NOT NULL CHECK(jsonb_typeof(config)='object'),
 public_key bytea NOT NULL CHECK(octet_length(public_key)=32), enabled boolean NOT NULL,
 published_by uuid NOT NULL, reason text NOT NULL CHECK(char_length(btrim(reason)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id), UNIQUE(org_id,source_id,id), UNIQUE(source_id,seq),
 FOREIGN KEY(org_id,source_id) REFERENCES service_resource_sources(org_id,id),
 FOREIGN KEY(org_id,source_id,base_revision_id) REFERENCES service_resource_revisions(org_id,source_id,id),
 FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id), CHECK((seq=1)=(base_revision_id IS NULL))
);
ALTER TABLE service_resource_sources ADD FOREIGN KEY(org_id,id,current_revision_id)
 REFERENCES service_resource_revisions(org_id,source_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER service_resource_revision_immutable BEFORE UPDATE OR DELETE ON service_resource_revisions
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER service_resource_scope_immutable BEFORE UPDATE ON service_resource_sources
 FOR EACH ROW EXECUTE FUNCTION preserve_service_source_scope();
CREATE FUNCTION validate_service_resource_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid; parent service_resource_sources%ROWTYPE; current_seq bigint; latest bigint; total bigint;
BEGIN
 IF TG_TABLE_NAME='service_resource_sources' THEN target:=NEW.id; ELSE target:=NEW.source_id; END IF;
 SELECT * INTO parent FROM service_resource_sources WHERE org_id=NEW.org_id AND id=target;
 SELECT seq INTO current_seq FROM service_resource_revisions WHERE org_id=NEW.org_id AND source_id=target AND id=parent.current_revision_id;
 SELECT max(seq),count(*) INTO latest,total FROM service_resource_revisions WHERE org_id=NEW.org_id AND source_id=target;
 IF current_seq IS NULL OR current_seq<>parent.version OR current_seq<>latest OR latest<>total THEN
  RAISE EXCEPTION 'resource head must reference latest contiguous revision' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER resource_head_consistent AFTER INSERT OR UPDATE ON service_resource_sources
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_service_resource_head();
CREATE CONSTRAINT TRIGGER resource_revision_consistent AFTER INSERT ON service_resource_revisions
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_service_resource_head();
CREATE FUNCTION validate_service_resource_base() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.seq>1 AND NOT EXISTS(SELECT 1 FROM service_resource_revisions WHERE org_id=NEW.org_id
  AND source_id=NEW.source_id AND id=NEW.base_revision_id AND seq=NEW.seq-1) THEN
  RAISE EXCEPTION 'resource revision must extend exact predecessor' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER resource_base_valid BEFORE INSERT ON service_resource_revisions
 FOR EACH ROW EXECUTE FUNCTION validate_service_resource_base();
CREATE TABLE service_resource_observations (
 id uuid PRIMARY KEY, org_id uuid NOT NULL, source_id uuid NOT NULL, revision_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence BETWEEN 1 AND 9007199254740991), observed_at timestamptz NOT NULL,
 received_at timestamptz NOT NULL DEFAULT clock_timestamp(), sample jsonb NOT NULL CHECK(jsonb_typeof(sample)='object'),
 signed_payload bytea NOT NULL CHECK(octet_length(signed_payload) BETWEEN 1 AND 16384),
 signature bytea NOT NULL CHECK(octet_length(signature)=64),
 payload_sha256 text NOT NULL CHECK(payload_sha256 ~ '^[0-9a-f]{64}$'), applied boolean NOT NULL,
 ignored_reason text NOT NULL CHECK(ignored_reason IN ('','LATE_SEQUENCE','LATE_OBSERVATION','STALE_SAMPLE')),
 UNIQUE(org_id,id), UNIQUE(revision_id,sequence),
 FOREIGN KEY(org_id,source_id,revision_id) REFERENCES service_resource_revisions(org_id,source_id,id),
 CHECK(applied=(ignored_reason=''))
);
CREATE INDEX resource_observation_history ON service_resource_observations(org_id,source_id,received_at DESC,id DESC);
CREATE INDEX resource_observation_current ON service_resource_observations(org_id,revision_id,sequence DESC) WHERE applied;
CREATE TRIGGER resource_observation_immutable BEFORE UPDATE OR DELETE ON service_resource_observations
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
  GRANT SELECT,INSERT,UPDATE ON service_resource_sources TO copypaste_app;
  GRANT SELECT,INSERT ON service_resource_revisions,service_resource_observations TO copypaste_app;
 END IF;
END $$;
