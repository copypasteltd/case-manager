-- Legacy rows retain protocol 0; never invent their creator or acceptance facts.
ALTER TABLE deliverables ADD COLUMN creator_id uuid;
ALTER TABLE deliverables ADD COLUMN protocol_version integer NOT NULL DEFAULT 0 CHECK(protocol_version IN(0,1));
ALTER TABLE deliverables ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE deliverables ADD CHECK(protocol_version=0 OR creator_id IS NOT NULL);
CREATE INDEX deliverables_list ON deliverables(org_id,project_id,updated_at DESC,id DESC);
ALTER TABLE deliverable_revisions ADD COLUMN protocol_version integer NOT NULL DEFAULT 0 CHECK(protocol_version IN(0,1));
ALTER TABLE deliverable_revisions ADD COLUMN name text;
ALTER TABLE deliverable_revisions ADD COLUMN acceptance_criteria text;
ALTER TABLE deliverable_revisions ADD COLUMN owner_id uuid;
ALTER TABLE deliverable_revisions ADD COLUMN milestone_id uuid;
ALTER TABLE deliverable_revisions ADD COLUMN classification text;
ALTER TABLE deliverable_revisions ADD COLUMN project_version bigint;
ALTER TABLE deliverable_revisions ADD COLUMN head_version bigint;
ALTER TABLE deliverable_revisions ADD COLUMN content_hash text;
ALTER TABLE deliverable_revisions ADD COLUMN session_id uuid;
ALTER TABLE deliverable_revisions ADD COLUMN reason text;
ALTER TABLE deliverable_revisions ADD FOREIGN KEY(org_id,owner_id) REFERENCES users(org_id,id);
ALTER TABLE deliverable_revisions ADD FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id);
ALTER TABLE deliverable_revisions ADD CHECK(protocol_version=0 OR
    (name IS NOT NULL AND length(btrim(name)) BETWEEN 1 AND 200 AND acceptance_criteria IS NOT NULL AND length(btrim(acceptance_criteria)) BETWEEN 1 AND 20000
    AND owner_id IS NOT NULL AND classification IS NOT NULL AND project_version>0 AND project_version IS NOT NULL AND head_version>0 AND head_version IS NOT NULL
    AND content_hash IS NOT NULL AND content_hash ~ '^[a-f0-9]{64}$' AND session_id IS NOT NULL AND reason IS NOT NULL AND length(btrim(reason)) BETWEEN 1 AND 200
    AND cardinality(file_version_ids) BETWEEN 1 AND 20 AND array_position(file_version_ids,NULL) IS NULL AND length(btrim(description)) BETWEEN 1 AND 20000));
CREATE TABLE deliverable_revision_files (
    org_id uuid NOT NULL, revision_id uuid NOT NULL, version_id uuid NOT NULL, ordinal integer NOT NULL CHECK(ordinal BETWEEN 1 AND 20),
    PRIMARY KEY(org_id,revision_id,version_id), UNIQUE(org_id,revision_id,ordinal),
    FOREIGN KEY(org_id,revision_id) REFERENCES deliverable_revisions(org_id,id),
    FOREIGN KEY(org_id,version_id) REFERENCES file_versions(org_id,id)
);
CREATE TRIGGER deliverable_files_immutable BEFORE UPDATE OR DELETE ON deliverable_revision_files FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE acceptance_decisions ADD COLUMN protocol_version integer NOT NULL DEFAULT 0 CHECK(protocol_version IN(0,1));
ALTER TABLE acceptance_decisions ADD COLUMN session_id uuid;
ALTER TABLE acceptance_decisions ADD COLUMN mfa_verified_at timestamptz;
ALTER TABLE acceptance_decisions ADD COLUMN revision_hash text;
ALTER TABLE acceptance_decisions ADD COLUMN head_version bigint;
ALTER TABLE acceptance_decisions ADD COLUMN files_verified boolean NOT NULL DEFAULT false;
ALTER TABLE acceptance_decisions ADD COLUMN criteria_verified boolean NOT NULL DEFAULT false;
ALTER TABLE acceptance_decisions ADD FOREIGN KEY(org_id,session_id) REFERENCES sessions(org_id,id);
ALTER TABLE acceptance_decisions ADD CHECK(protocol_version=0 OR (session_id IS NOT NULL AND mfa_verified_at IS NOT NULL
    AND revision_hash IS NOT NULL AND revision_hash ~ '^[a-f0-9]{64}$' AND head_version IS NOT NULL AND head_version>0
    AND length(btrim(reason)) BETWEEN 1 AND 20000 AND (decision='return' OR (files_verified AND criteria_verified))));

CREATE FUNCTION deliverable_revision_hash(r deliverable_revisions) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT encode(sha256(convert_to(jsonb_build_object('id',r.id,'deliverable_id',r.deliverable_id,'seq',r.seq,
        'name',r.name,'acceptance_criteria',r.acceptance_criteria,'owner_id',r.owner_id,'milestone_id',r.milestone_id,
        'classification',r.classification,'project_version',r.project_version,'head_version',r.head_version,
        'file_version_ids',r.file_version_ids,'description',r.description,'submitted_by',r.submitted_by)::text,'UTF8')),'hex')
$$;
CREATE FUNCTION deliverable_files_available(o uuid,p uuid,c text,ids uuid[]) RETURNS boolean LANGUAGE sql STABLE AS $$
    WITH RECURSIVE active_tree AS (
        SELECT id,project_id FROM folders WHERE org_id=o AND parent_id IS NULL AND state='active'
        UNION ALL SELECT f.id,f.project_id FROM folders f JOIN active_tree a ON a.id=f.parent_id WHERE f.org_id=o AND f.state='active'
    ) SELECT cardinality(ids)=(SELECT count(*) FROM file_versions v JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id
        JOIN active_tree t ON t.id=f.folder_id WHERE v.org_id=o AND v.id=ANY(ids) AND f.state='active'
        AND v.availability='available' AND v.scan_state IN('clean','waived') AND classification_rank(f.classification)<=classification_rank(c)
        AND (t.project_id IS NULL OR t.project_id=p))
$$;
CREATE FUNCTION guard_deliverable_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p projects; BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'deliverable history cannot be deleted' USING ERRCODE='23514'; END IF;
    IF TG_OP='UPDATE' AND (NEW.id,NEW.org_id,NEW.project_id,NEW.creator_id,NEW.protocol_version) IS DISTINCT FROM (OLD.id,OLD.org_id,OLD.project_id,OLD.creator_id,OLD.protocol_version) THEN
        RAISE EXCEPTION 'deliverable identity is immutable' USING ERRCODE='23514'; END IF;
    IF NEW.protocol_version<>1 THEN RAISE EXCEPTION 'legacy deliverable requires separate evidence migration' USING ERRCODE='23514'; END IF;
    SELECT * INTO p FROM projects WHERE org_id=NEW.org_id AND id=NEW.project_id FOR UPDATE;
    IF p.id IS NULL OR p.state IN('closing','archived','cancelled') THEN RAISE EXCEPTION 'project is read only' USING ERRCODE='23514'; END IF;
    IF NOT EXISTS(SELECT 1 FROM project_members m JOIN users u ON u.org_id=m.org_id AND u.id=m.user_id WHERE m.org_id=NEW.org_id AND m.project_id=NEW.project_id AND m.user_id=NEW.owner_id AND u.status='active') THEN
        RAISE EXCEPTION 'deliverable owner must be an active project member' USING ERRCODE='23514'; END IF;
    IF NEW.milestone_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM milestones WHERE org_id=NEW.org_id AND project_id=NEW.project_id AND id=NEW.milestone_id AND state IN('open','missed')) THEN
        RAISE EXCEPTION 'linked milestone must be reopened' USING ERRCODE='23514'; END IF;
    IF TG_OP='UPDATE' THEN
        IF OLD.milestone_id IS NOT NULL AND EXISTS(SELECT 1 FROM milestones WHERE org_id=OLD.org_id AND id=OLD.milestone_id AND state IN('achieved','cancelled')) THEN
            RAISE EXCEPTION 'previous linked milestone must be reopened' USING ERRCODE='23514'; END IF;
        IF NEW.version<>OLD.version+1 OR ((OLD.state IN('submitted','accepted')) AND (NEW.name,NEW.acceptance_criteria,NEW.owner_id,NEW.milestone_id) IS DISTINCT FROM (OLD.name,OLD.acceptance_criteria,OLD.owner_id,OLD.milestone_id))
            OR (NEW.current_revision_id IS DISTINCT FROM OLD.current_revision_id AND NEW.state<>'submitted') THEN
            RAISE EXCEPTION 'deliverable requires the exact next fixed revision or decision' USING ERRCODE='23514'; END IF;
    ELSIF NEW.version<>1 OR NEW.state<>'draft' OR NEW.current_revision_id IS NOT NULL THEN
        RAISE EXCEPTION 'deliverable must start as an empty draft' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER deliverable_head_guard BEFORE INSERT OR UPDATE OR DELETE ON deliverables FOR EACH ROW EXECUTE FUNCTION guard_deliverable_head();

CREATE FUNCTION prepare_deliverable_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d deliverables; p projects; s sessions; BEGIN
    SELECT * INTO d FROM deliverables WHERE org_id=NEW.org_id AND id=NEW.deliverable_id;
    SELECT * INTO p FROM projects WHERE org_id=d.org_id AND id=d.project_id FOR UPDATE;
    SELECT * INTO d FROM deliverables WHERE org_id=NEW.org_id AND id=NEW.deliverable_id FOR UPDATE;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    PERFORM f.id FROM files f WHERE f.org_id=NEW.org_id AND f.id IN(SELECT file_id FROM file_versions WHERE org_id=NEW.org_id AND id=ANY(NEW.file_version_ids)) ORDER BY f.id FOR SHARE;
    IF NEW.protocol_version<>1 OR d.protocol_version<>1 OR d.state NOT IN('draft','changes_requested','accepted') OR p.state IN('closing','archived','cancelled')
        OR NEW.head_version<>d.version+1 OR NEW.project_version<>p.version OR NEW.classification<>p.classification
        OR (NEW.name,NEW.acceptance_criteria,NEW.owner_id,NEW.milestone_id) IS DISTINCT FROM (d.name,d.acceptance_criteria,d.owner_id,d.milestone_id)
        OR NEW.seq<>(SELECT COALESCE(max(seq),0)+1 FROM deliverable_revisions WHERE org_id=NEW.org_id AND deliverable_id=NEW.deliverable_id)
        OR cardinality(NEW.file_version_ids)<>(SELECT count(DISTINCT id) FROM unnest(NEW.file_version_ids)id)
        OR s.id IS NULL OR s.user_id<>NEW.submitted_by OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp()
        OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.submitted_by AND status='active' AND auth_version=s.auth_version)
        OR NOT deliverable_files_available(NEW.org_id,p.id,p.classification,NEW.file_version_ids) THEN
        RAISE EXCEPTION 'submission requires the actual fixed draft and current human session' USING ERRCODE='23514'; END IF;
    NEW.submitted_at:=clock_timestamp();NEW.content_hash:=deliverable_revision_hash(NEW);RETURN NEW;
END; $$;
CREATE TRIGGER deliverable_revision_guard BEFORE INSERT ON deliverable_revisions FOR EACH ROW EXECUTE FUNCTION prepare_deliverable_revision();
CREATE FUNCTION prepare_acceptance_decision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE d deliverables; r deliverable_revisions; p projects; s sessions; BEGIN
    SELECT * INTO r FROM deliverable_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id;
    SELECT * INTO d FROM deliverables WHERE org_id=r.org_id AND id=r.deliverable_id;
    SELECT * INTO p FROM projects WHERE org_id=d.org_id AND id=d.project_id FOR UPDATE;
    SELECT * INTO d FROM deliverables WHERE org_id=r.org_id AND id=r.deliverable_id FOR UPDATE;
    SELECT * INTO s FROM sessions WHERE org_id=NEW.org_id AND id=NEW.session_id FOR SHARE;
    PERFORM f.id FROM files f WHERE f.org_id=NEW.org_id AND f.id IN(SELECT file_id FROM file_versions WHERE org_id=NEW.org_id AND id=ANY(r.file_version_ids)) ORDER BY f.id FOR SHARE;
    IF NEW.protocol_version<>1 OR r.protocol_version<>1 OR d.state<>'submitted' OR d.current_revision_id IS DISTINCT FROM r.id OR NEW.head_version<>d.version+1
        OR NEW.revision_hash IS DISTINCT FROM r.content_hash OR p.state IN('closing','archived','cancelled')
        OR (NEW.decision='accept' AND (NEW.decided_by=r.submitted_by OR NOT NEW.files_verified OR NOT NEW.criteria_verified))
        OR s.id IS NULL OR s.user_id<>NEW.decided_by OR s.revoked_at IS NOT NULL OR s.expires_at<=clock_timestamp()
        OR s.mfa_verified_at IS NULL OR s.mfa_verified_at IS DISTINCT FROM NEW.mfa_verified_at OR s.mfa_verified_at<clock_timestamp()-interval '10 minutes' OR s.mfa_verified_at>clock_timestamp()
        OR NOT EXISTS(SELECT 1 FROM users WHERE org_id=NEW.org_id AND id=NEW.decided_by AND status='active' AND auth_version=s.auth_version)
        OR NOT deliverable_files_available(NEW.org_id,p.id,p.classification,r.file_version_ids) THEN
        RAISE EXCEPTION 'acceptance requires the exact submission and current independent human MFA session' USING ERRCODE='23514'; END IF;
    NEW.decided_at:=clock_timestamp();RETURN NEW;
END; $$;
CREATE TRIGGER acceptance_decision_guard BEFORE INSERT ON acceptance_decisions FOR EACH ROW EXECUTE FUNCTION prepare_acceptance_decision();

CREATE FUNCTION validate_deliverable_consistency() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE did uuid; rid uuid; d deliverables; r deliverable_revisions; a acceptance_decisions; files uuid[]; BEGIN
    IF TG_TABLE_NAME='deliverables' THEN did:=NEW.id;
    ELSIF TG_TABLE_NAME='deliverable_revisions' THEN did:=NEW.deliverable_id;
    ELSE SELECT deliverable_id INTO did FROM deliverable_revisions WHERE org_id=NEW.org_id AND id=NEW.revision_id; END IF;
    SELECT * INTO d FROM deliverables WHERE org_id=NEW.org_id AND id=did;
    IF d.protocol_version<>1 THEN RETURN NULL; END IF;
    IF d.current_revision_id IS NULL THEN
        IF d.state<>'draft' OR EXISTS(SELECT 1 FROM deliverable_revisions WHERE org_id=d.org_id AND deliverable_id=d.id) THEN
            RAISE EXCEPTION 'deliverable requires its submitted revision' USING ERRCODE='23514'; END IF;
        RETURN NULL;
    END IF;
    SELECT * INTO r FROM deliverable_revisions WHERE org_id=d.org_id AND deliverable_id=d.id AND id=d.current_revision_id;
    SELECT * INTO a FROM acceptance_decisions WHERE org_id=d.org_id AND revision_id=r.id;
    IF r.id IS NULL OR r.protocol_version<>1 OR r.content_hash<>deliverable_revision_hash(r) OR r.seq<>(SELECT max(seq) FROM deliverable_revisions WHERE org_id=d.org_id AND deliverable_id=d.id)
        OR (d.state='submitted' AND (a.id IS NOT NULL OR d.version<>r.head_version))
        OR (d.state='accepted' AND (a.id IS NULL OR a.decision<>'accept' OR d.version<>a.head_version))
        OR (d.state='changes_requested' AND (a.id IS NULL OR a.decision<>'return' OR d.version<a.head_version)) OR d.state='draft' THEN
        RAISE EXCEPTION 'deliverable head requires its exact immutable submission and decision' USING ERRCODE='23514'; END IF;
    FOR rid IN SELECT id FROM deliverable_revisions WHERE org_id=d.org_id AND deliverable_id=d.id AND protocol_version=1 LOOP
        SELECT * INTO r FROM deliverable_revisions WHERE org_id=d.org_id AND id=rid;
        SELECT array_agg(version_id ORDER BY ordinal) INTO files FROM deliverable_revision_files WHERE org_id=d.org_id AND revision_id=rid;
        IF files IS DISTINCT FROM r.file_version_ids THEN RAISE EXCEPTION 'submission requires the complete fixed file references' USING ERRCODE='23514'; END IF;
    END LOOP;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER deliverable_head_consistency AFTER INSERT OR UPDATE ON deliverables DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_deliverable_consistency();
CREATE CONSTRAINT TRIGGER deliverable_revision_consistency AFTER INSERT ON deliverable_revisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_deliverable_consistency();
CREATE CONSTRAINT TRIGGER deliverable_decision_consistency AFTER INSERT ON acceptance_decisions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_deliverable_consistency();
CREATE CONSTRAINT TRIGGER deliverable_files_consistency AFTER INSERT ON deliverable_revision_files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_deliverable_consistency();
CREATE FUNCTION pin_deliverable_revision_file() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id)
        SELECT gen_random_uuid(),NEW.org_id,file_id,id,'deliverable',NEW.revision_id FROM file_versions WHERE org_id=NEW.org_id AND id=NEW.version_id;
    RETURN NEW;
END; $$;
CREATE TRIGGER deliverable_reference AFTER INSERT ON deliverable_revision_files FOR EACH ROW EXECUTE FUNCTION pin_deliverable_revision_file();
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
    GRANT SELECT,INSERT ON deliverable_revision_files TO copypaste_app;
    REVOKE UPDATE,DELETE ON deliverable_revision_files,deliverable_revisions,acceptance_decisions FROM copypaste_app;
    REVOKE DELETE ON deliverables FROM copypaste_app;
END IF; END; $$;

-- Narrow optional role also provides a real MFA enrollment path; never bind it automatically.
INSERT INTO roles(id,org_id,key,name,requires_mfa) SELECT gen_random_uuid(),id,'deliverable_reviewer','项目交付验收人',true FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action) SELECT org_id,id,a.action FROM roles CROSS JOIN (VALUES('deliverable.read'),('deliverable.decide'))a(action) WHERE key='deliverable_reviewer' ON CONFLICT DO NOTHING;
