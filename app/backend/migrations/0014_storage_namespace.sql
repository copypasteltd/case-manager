-- Old unknown creators remain NULL; new writes must identify the real actor.
ALTER TABLE folders ADD COLUMN creator_id uuid;
UPDATE folders f SET creator_id=p.creator_id
FROM projects p WHERE p.org_id=f.org_id AND p.id=f.project_id
AND f.created_at=p.created_at;
ALTER TABLE folders ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE files ADD COLUMN creator_id uuid;
ALTER TABLE files ADD FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id);
ALTER TABLE file_versions ADD COLUMN original_name text CHECK(original_name IS NULL OR length(original_name) BETWEEN 1 AND 200);
CREATE TRIGGER file_version_original_name_immutable BEFORE UPDATE OF original_name
ON file_versions FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

-- One namespace shared by files and folders prevents concurrent cross-type
-- collisions, including writers using different endpoints.
CREATE TABLE storage_entries (
    org_id uuid NOT NULL REFERENCES organizations(id),
    namespace_key text NOT NULL, name_key text NOT NULL,
    resource_type text NOT NULL CHECK(resource_type IN ('folder','file')), resource_id uuid NOT NULL,
    PRIMARY KEY(org_id,resource_type,resource_id),
    UNIQUE(org_id,namespace_key,name_key)
);
INSERT INTO storage_entries(org_id,namespace_key,name_key,resource_type,resource_id)
SELECT org_id,CASE WHEN parent_id IS NULL THEN 'roots/'||root_scope ELSE 'folder/'||parent_id::text END,name_key,'folder',id
FROM folders WHERE state='active';
INSERT INTO storage_entries(org_id,namespace_key,name_key,resource_type,resource_id)
SELECT org_id,'folder/'||folder_id::text,name_key,'file',id FROM files WHERE state='active';

CREATE FUNCTION maintain_storage_namespace() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE kind text; namespace text;
BEGIN
    kind:=CASE WHEN TG_TABLE_NAME='folders' THEN 'folder' ELSE 'file' END;
    IF TG_OP='DELETE' THEN
        DELETE FROM public.storage_entries WHERE org_id=OLD.org_id AND resource_type=kind AND resource_id=OLD.id;
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' AND NEW.creator_id IS NULL THEN
        RAISE EXCEPTION 'new storage resources require a creator' USING ERRCODE='23514';
    END IF;
    IF TG_OP='UPDATE' AND NEW.creator_id IS DISTINCT FROM OLD.creator_id THEN
        RAISE EXCEPTION 'storage creator is immutable' USING ERRCODE='23514';
    END IF;
    DELETE FROM public.storage_entries WHERE org_id=NEW.org_id AND resource_type=kind AND resource_id=NEW.id;
    IF NEW.state='active' THEN
        IF kind='folder' THEN
            namespace:=CASE WHEN NEW.parent_id IS NULL THEN 'roots/'||NEW.root_scope ELSE 'folder/'||NEW.parent_id::text END;
        ELSE
            namespace:='folder/'||NEW.folder_id::text;
        END IF;
        INSERT INTO public.storage_entries(org_id,namespace_key,name_key,resource_type,resource_id)
        VALUES(NEW.org_id,namespace,NEW.name_key,kind,NEW.id);
    END IF;
    RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION maintain_storage_namespace() FROM PUBLIC;
CREATE TRIGGER folder_namespace AFTER INSERT OR UPDATE OF name_key,parent_id,root_scope,state,creator_id OR DELETE
ON folders FOR EACH ROW EXECUTE FUNCTION maintain_storage_namespace();
CREATE TRIGGER file_namespace AFTER INSERT OR UPDATE OF name_key,folder_id,state,creator_id OR DELETE
ON files FOR EACH ROW EXECUTE FUNCTION maintain_storage_namespace();
CREATE INDEX folders_parent_lookup ON folders(org_id,parent_id,state);
CREATE INDEX files_folder_lookup ON files(org_id,folder_id,state);
ALTER TABLE file_versions ADD CONSTRAINT file_scan_waiver_evidence
CHECK(scan_state<>'waived' OR scan_waiver_ref IS NOT NULL);
ALTER TABLE file_replicas ADD CONSTRAINT replica_verified_evidence
CHECK(state<>'verified' OR (actual_sha256 IS NOT NULL AND actual_sha256=expected_sha256 AND verified_at IS NOT NULL));
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT ON storage_entries TO copypaste_app;
    END IF;
END;
$$;
