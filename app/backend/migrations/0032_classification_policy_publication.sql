-- Existing policy records remain intact. Their author/reason were not recorded
-- and must not be fabricated during upgrade.
ALTER TABLE classification_policies
    ADD COLUMN published_by uuid,
    ADD COLUMN reason text,
    ADD COLUMN base_policy_id uuid,
    ADD CONSTRAINT classification_policy_org_id UNIQUE(org_id,id),
    ADD CONSTRAINT classification_policy_author FOREIGN KEY(org_id,published_by) REFERENCES users(org_id,id),
    ADD CONSTRAINT classification_policy_base FOREIGN KEY(org_id,base_policy_id) REFERENCES classification_policies(org_id,id),
    ADD CONSTRAINT classification_policy_positive_version CHECK(policy_version>0),
    ADD CONSTRAINT classification_policy_publication_reason CHECK(
        (published_by IS NULL AND reason IS NULL) OR
        (published_by IS NOT NULL AND reason IS NOT NULL AND char_length(btrim(reason)) BETWEEN 1 AND 200));

CREATE TRIGGER classification_policy_append_only BEFORE UPDATE OR DELETE ON classification_policies
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();

CREATE FUNCTION validate_classification_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE previous classification_policies%ROWTYPE; normalized text[];
BEGIN
    -- Serialize policy publication with every other authorization mutation.
    PERFORM 1 FROM organizations WHERE id=NEW.org_id FOR UPDATE;
    SELECT * INTO previous FROM classification_policies
        WHERE org_id=NEW.org_id AND level=NEW.level ORDER BY policy_version DESC LIMIT 1;
    IF previous.id IS NULL THEN
        IF NEW.policy_version<>1 OR NEW.base_policy_id IS NOT NULL THEN
            RAISE EXCEPTION 'initial classification policy requires version one' USING ERRCODE='23514';
        END IF;
    ELSE
        IF NEW.policy_version<>previous.policy_version+1 THEN
            RAISE EXCEPTION 'classification policy sequence must be contiguous' USING ERRCODE='23514';
        END IF;
        -- Controlled/legacy callers may omit this field. All new API
        -- publications provide it; a supplied predecessor must always be exact.
        IF NEW.base_policy_id IS NOT NULL AND NEW.base_policy_id<>previous.id THEN
            RAISE EXCEPTION 'classification policy predecessor changed' USING ERRCODE='23514';
        END IF;
    END IF;
    SELECT coalesce(array_agg(DISTINCT item ORDER BY item),ARRAY[]::text[]) INTO normalized
        FROM unnest(NEW.allowed_channels) AS channel_rows(item);
    IF array_position(NEW.allowed_channels,NULL) IS NOT NULL
        OR cardinality(NEW.allowed_channels)<>cardinality(normalized)
        OR NOT NEW.allowed_channels<@ARRAY['android','douyin','telegram','web','wechat']::text[] THEN
        RAISE EXCEPTION 'classification channels must be a unique canonical set' USING ERRCODE='23514';
    END IF;
    NEW.allowed_channels=normalized;
    IF current_user='copypaste_app' AND NEW.policy_version>1
        AND (NEW.published_by IS NULL OR NEW.reason IS NULL OR NEW.base_policy_id IS NULL) THEN
        RAISE EXCEPTION 'application publication requires exact provenance' USING ERRCODE='23514';
    END IF;
    IF NEW.published_by IS NOT NULL AND (
        (NEW.level='internal' AND NOT 'web'=ANY(NEW.allowed_channels))
        OR (NEW.level='confidential' AND NEW.ai_allowed)
        OR 'telegram'=ANY(NEW.allowed_channels)) THEN
        RAISE EXCEPTION 'publication exceeds available classification or channel admission' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER classification_policy_validate BEFORE INSERT ON classification_policies
    FOR EACH ROW EXECUTE FUNCTION validate_classification_policy_revision();

DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        REVOKE UPDATE,DELETE ON classification_policies FROM copypaste_app;
        GRANT SELECT,INSERT ON classification_policies TO copypaste_app;
    END IF;
END;
$$;
