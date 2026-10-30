-- New immutable native derivations do not rewrite old PDF/image unsupported rows.
ALTER TABLE file_text_derivations DROP CONSTRAINT file_text_derivations_extractor_check;
ALTER TABLE file_text_derivations ADD CONSTRAINT file_text_derivations_extractor_check
    CHECK(extractor IN ('text-v1','office-v1','docx-v2','document-ocr-v1'));
ALTER TABLE file_text_derivations ADD COLUMN engine_sha256 text;
ALTER TABLE file_text_derivations ADD COLUMN page_count integer;
ALTER TABLE file_text_derivations ADD COLUMN ocr_page_count integer;
ALTER TABLE file_text_derivations ADD CONSTRAINT document_text_evidence CHECK(
    (extractor='document-ocr-v1' AND engine_sha256 ~ '^[0-9a-f]{64}$' AND engine_sha256 IS NOT NULL
        AND ((state='ready' AND page_count BETWEEN 1 AND 20 AND page_count IS NOT NULL
            AND ocr_page_count=page_count AND ocr_page_count IS NOT NULL)
          OR (state<>'ready' AND page_count IS NULL AND ocr_page_count IS NULL)))
    OR (extractor<>'document-ocr-v1' AND engine_sha256 IS NULL AND page_count IS NULL AND ocr_page_count IS NULL));

CREATE OR REPLACE FUNCTION file_text_extractor(media_type text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT CASE lower(btrim(split_part(media_type,';',1)))
        WHEN 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' THEN 'docx-v2'
        WHEN 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' THEN 'office-v1'
        WHEN 'application/vnd.openxmlformats-officedocument.presentationml.presentation' THEN 'office-v1'
        WHEN 'application/pdf' THEN 'document-ocr-v1'
        WHEN 'image/png' THEN 'document-ocr-v1'
        WHEN 'image/jpeg' THEN 'document-ocr-v1'
        ELSE 'text-v1' END
$$;

CREATE FUNCTION guard_document_text_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND NEW.engine_sha256 IS DISTINCT FROM OLD.engine_sha256 THEN
        RAISE EXCEPTION 'native document engine evidence is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.extractor='document-ocr-v1' AND NOT EXISTS(SELECT 1 FROM file_versions v
        WHERE v.org_id=NEW.org_id AND v.file_id=NEW.file_id AND v.id=NEW.version_id
        AND file_text_extractor(v.media_type)='document-ocr-v1') THEN
        RAISE EXCEPTION 'native document derivation requires the exact PDF/image source' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER document_text_source BEFORE INSERT OR UPDATE ON file_text_derivations
    FOR EACH ROW EXECUTE FUNCTION guard_document_text_source();
