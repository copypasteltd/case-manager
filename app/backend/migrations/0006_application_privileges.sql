REVOKE CREATE ON SCHEMA public FROM PUBLIC;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT USAGE ON SCHEMA public TO copypaste_app;
        GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO copypaste_app;
        GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO copypaste_app;
        REVOKE ALL ON schema_migrations FROM copypaste_app;
        REVOKE UPDATE,DELETE ON audit_events,config_revisions,post_revisions,comments,
            deliverable_revisions,acceptance_decisions,file_versions FROM copypaste_app;
    END IF;
END;
$$;
