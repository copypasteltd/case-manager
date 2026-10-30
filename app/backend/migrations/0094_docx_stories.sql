-- Preserve legacy text-v1 DOCX results and queued execution semantics.
ALTER TABLE file_text_derivations DROP CONSTRAINT file_text_derivations_extractor_check;
ALTER TABLE file_text_derivations ADD CONSTRAINT file_text_derivations_extractor_check CHECK(extractor IN ('text-v1','office-v1','docx-v2'));

CREATE OR REPLACE FUNCTION file_text_extractor(media_type text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT CASE lower(btrim(split_part(media_type,';',1)))
        WHEN 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' THEN 'docx-v2'
        WHEN 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' THEN 'office-v1'
        WHEN 'application/vnd.openxmlformats-officedocument.presentationml.presentation' THEN 'office-v1'
        ELSE 'text-v1' END
$$;

CREATE FUNCTION guard_docx_text_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.extractor='docx-v2' AND NOT EXISTS(SELECT 1 FROM file_versions v
        WHERE v.org_id=NEW.org_id AND v.file_id=NEW.file_id AND v.id=NEW.version_id
        AND file_text_extractor(v.media_type)='docx-v2') THEN
        RAISE EXCEPTION 'DOCX derivation requires the exact Word source' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER docx_text_source BEFORE INSERT OR UPDATE ON file_text_derivations
    FOR EACH ROW EXECUTE FUNCTION guard_docx_text_source();
