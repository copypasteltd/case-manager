-- New extraction identities do not rewrite old unsupported/ready evidence.
ALTER TABLE file_text_derivations DROP CONSTRAINT file_text_derivations_extractor_check;
ALTER TABLE file_text_derivations ADD CONSTRAINT file_text_derivations_extractor_check CHECK(extractor IN ('text-v1','office-v1'));

CREATE FUNCTION file_text_extractor(media_type text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT CASE lower(btrim(split_part(media_type,';',1)))
        WHEN 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' THEN 'office-v1'
        WHEN 'application/vnd.openxmlformats-officedocument.presentationml.presentation' THEN 'office-v1'
        ELSE 'text-v1' END
$$;

-- Existing terminal text-v1 Office rows remain immutable; queued legacy jobs
-- still finish with their original unsupported result. No body is backfilled.
CREATE FUNCTION guard_office_text_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.extractor='office-v1' AND NOT EXISTS(SELECT 1 FROM file_versions v
        WHERE v.org_id=NEW.org_id AND v.file_id=NEW.file_id AND v.id=NEW.version_id
        AND file_text_extractor(v.media_type)='office-v1') THEN
        RAISE EXCEPTION 'office derivation requires the exact Office source' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER office_text_source BEFORE INSERT OR UPDATE ON file_text_derivations
    FOR EACH ROW EXECUTE FUNCTION guard_office_text_source();
