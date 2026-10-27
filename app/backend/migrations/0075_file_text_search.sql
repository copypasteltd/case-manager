-- Derived plaintext is tied to an immutable formal version, never a mutable file head.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE file_text_derivations (
    org_id uuid NOT NULL,
    version_id uuid NOT NULL,
    file_id uuid NOT NULL,
    extractor text NOT NULL CHECK(extractor='text-v1'),
    source_sha256 text NOT NULL CHECK(source_sha256 ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','ready','unsupported','blocked','failed')),
    body text,
    text_sha256 text,
    character_count integer,
    error_code text,
    indexed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(org_id,version_id,extractor),
    FOREIGN KEY(org_id,file_id,version_id) REFERENCES file_versions(org_id,file_id,id),
    CHECK(octet_length(body)<=2097152),
    CHECK((state='ready' AND body IS NOT NULL AND text_sha256 IS NOT NULL AND character_count IS NOT NULL
          AND text_sha256=encode(sha256(convert_to(body,'UTF8')),'hex')
          AND character_count=char_length(body) AND error_code IS NULL AND indexed_at IS NOT NULL)
       OR (state<>'ready' AND body IS NULL AND text_sha256 IS NULL AND character_count IS NULL)),
    CHECK((state='queued' AND indexed_at IS NULL AND error_code IS NULL)
       OR (state<>'queued' AND indexed_at IS NOT NULL)),
    CHECK(error_code IS NULL OR error_code ~ '^FILE_TEXT_[A-Z_]{1,64}$')
);
CREATE INDEX file_text_content_trgm ON file_text_derivations USING gin(body gin_trgm_ops) WHERE state='ready';
CREATE INDEX file_search_name_trgm ON files USING gin(name gin_trgm_ops) WHERE state='active';
CREATE INDEX post_search_body_trgm ON post_revisions USING gin(body gin_trgm_ops);
CREATE FUNCTION guard_file_text_derivation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND
       (OLD.state<>'queued' OR ROW(NEW.org_id,NEW.version_id,NEW.file_id,NEW.extractor,NEW.source_sha256,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.org_id,OLD.version_id,OLD.file_id,OLD.extractor,OLD.source_sha256,OLD.created_at))) THEN
        RAISE EXCEPTION 'file text evidence is immutable' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM file_versions v WHERE v.org_id=NEW.org_id AND v.file_id=NEW.file_id
                  AND v.id=NEW.version_id AND v.sha256=NEW.source_sha256) THEN
        RAISE EXCEPTION 'file text source must match the formal version' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER immutable_file_text BEFORE INSERT OR UPDATE OR DELETE ON file_text_derivations
    FOR EACH ROW EXECUTE FUNCTION guard_file_text_derivation();
-- Lock-only capability: the application still cannot UPDATE formal versions.
CREATE FUNCTION lock_file_text_version(p_org uuid,p_file uuid,p_version uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$ BEGIN
    PERFORM 1 FROM public.file_versions WHERE org_id=p_org AND file_id=p_file AND id=p_version FOR SHARE;
END $$;
REVOKE ALL ON FUNCTION lock_file_text_version(uuid,uuid,uuid) FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON file_text_derivations TO copypaste_app;
        GRANT EXECUTE ON FUNCTION lock_file_text_version(uuid,uuid,uuid) TO copypaste_app;
    END IF;
END $$;
