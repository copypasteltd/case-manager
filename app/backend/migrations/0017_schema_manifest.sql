-- Expose only the public compilation manifest, never migration mutation.
CREATE FUNCTION application_schema_manifest() RETURNS TABLE(name text,sha256 text)
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,public
    AS $$ SELECT m.name,m.sha256 FROM public.schema_migrations m ORDER BY m.name $$;
REVOKE ALL ON FUNCTION application_schema_manifest() FROM PUBLIC;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT EXECUTE ON FUNCTION application_schema_manifest() TO copypaste_app;
    END IF;
END;
$$;
