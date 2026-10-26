CREATE TABLE conversations (
    id uuid PRIMARY KEY, org_id uuid NOT NULL REFERENCES organizations(id),
    creator_id uuid NOT NULL, type text NOT NULL CHECK(type IN ('private','project')),
    project_id uuid, classification text NOT NULL CHECK(classification IN ('internal','project_restricted','confidential')),
    current_revision_id uuid NOT NULL, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), FOREIGN KEY(org_id,creator_id) REFERENCES users(org_id,id),
    FOREIGN KEY(org_id,project_id) REFERENCES projects(org_id,id),
    CHECK(type<>'project' OR project_id IS NOT NULL),
    CHECK(classification<>'project_restricted' OR project_id IS NOT NULL)
);
CREATE TABLE conversation_revisions (
    id uuid PRIMARY KEY, org_id uuid NOT NULL, conversation_id uuid NOT NULL,
    seq bigint NOT NULL CHECK(seq>0), base_revision_id uuid,
    title text NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    state text NOT NULL CHECK(state IN ('active','archived')),
    member_ids uuid[] NOT NULL CHECK(cardinality(member_ids) BETWEEN 1 AND 100 AND array_position(member_ids,NULL) IS NULL),
    context_security_version bigint NOT NULL CHECK(context_security_version>0),
    author_id uuid NOT NULL, reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id,id), UNIQUE(org_id,conversation_id,id), UNIQUE(conversation_id,seq),
    UNIQUE(conversation_id,base_revision_id),
    FOREIGN KEY(org_id,conversation_id) REFERENCES conversations(org_id,id),
    FOREIGN KEY(org_id,conversation_id,base_revision_id) REFERENCES conversation_revisions(org_id,conversation_id,id),
    FOREIGN KEY(org_id,author_id) REFERENCES users(org_id,id),
    CHECK((seq=1)=(base_revision_id IS NULL))
);
CREATE TABLE conversation_members (
    org_id uuid NOT NULL, conversation_id uuid NOT NULL, revision_id uuid NOT NULL, user_id uuid NOT NULL,
    PRIMARY KEY(revision_id,user_id),
    FOREIGN KEY(org_id,conversation_id,revision_id) REFERENCES conversation_revisions(org_id,conversation_id,id),
    FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
ALTER TABLE conversations ADD FOREIGN KEY(org_id,id,current_revision_id)
    REFERENCES conversation_revisions(org_id,conversation_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX conversations_list ON conversations(org_id,updated_at DESC,id DESC);
CREATE INDEX conversation_members_access ON conversation_members(org_id,user_id,conversation_id,revision_id);
CREATE TRIGGER conversation_revision_immutable BEFORE UPDATE OR DELETE ON conversation_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER conversation_members_immutable BEFORE UPDATE OR DELETE ON conversation_members
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION preserve_conversation_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR NEW.creator_id<>OLD.creator_id OR NEW.type<>OLD.type
        OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.classification<>OLD.classification
        OR NEW.created_at<>OLD.created_at THEN
        RAISE EXCEPTION 'conversation identity and security scope are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER conversation_scope_immutable BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION preserve_conversation_scope();
CREATE FUNCTION validate_conversation_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_conversation uuid;
    target_revision uuid;
    c conversations;
    r conversation_revisions;
    previous conversation_revisions;
    member_count integer;
    changed boolean;
    declared_members uuid[];
    actual_members uuid[];
BEGIN
    IF TG_TABLE_NAME='conversations' THEN
        target_conversation:=NEW.id;
        target_revision:=NEW.current_revision_id;
    ELSE
        target_conversation:=NEW.conversation_id;
        IF TG_TABLE_NAME='conversation_revisions' THEN target_revision:=NEW.id;
        ELSE target_revision:=NEW.revision_id; END IF;
    END IF;
    SELECT * INTO STRICT c FROM conversations WHERE id=target_conversation;
    SELECT * INTO STRICT r FROM conversation_revisions WHERE id=target_revision AND conversation_id=c.id;
    SELECT count(*) INTO member_count FROM conversation_members WHERE revision_id=r.id;
    SELECT array_agg(user_id ORDER BY user_id) INTO actual_members FROM conversation_members WHERE revision_id=r.id;
    SELECT array_agg(id ORDER BY id) INTO declared_members FROM unnest(r.member_ids) AS members(id);
    IF actual_members IS DISTINCT FROM declared_members THEN
        RAISE EXCEPTION 'conversation membership must exactly match the immutable declared snapshot' USING ERRCODE='23514';
    END IF;
    IF member_count NOT BETWEEN 1 AND 100 OR NOT EXISTS(
        SELECT 1 FROM conversation_members WHERE revision_id=r.id AND user_id=c.creator_id)
        OR (c.type='private' AND member_count<>1) THEN
        RAISE EXCEPTION 'conversation snapshot requires creator and a bounded private or shared membership' USING ERRCODE='23514';
    END IF;
    IF r.seq=1 THEN
        IF r.context_security_version<>1 THEN RAISE EXCEPTION 'initial conversation context epoch must be one' USING ERRCODE='23514'; END IF;
    ELSE
        SELECT * INTO STRICT previous FROM conversation_revisions WHERE id=r.base_revision_id AND conversation_id=c.id;
        IF previous.seq+1<>r.seq THEN RAISE EXCEPTION 'conversation revision sequence is not contiguous' USING ERRCODE='23514'; END IF;
        SELECT EXISTS(
            (SELECT user_id FROM conversation_members WHERE revision_id=r.id EXCEPT SELECT user_id FROM conversation_members WHERE revision_id=previous.id)
            UNION ALL
            (SELECT user_id FROM conversation_members WHERE revision_id=previous.id EXCEPT SELECT user_id FROM conversation_members WHERE revision_id=r.id)
        ) INTO changed;
        IF r.context_security_version<>previous.context_security_version+(CASE WHEN changed THEN 1 ELSE 0 END) THEN
            RAISE EXCEPTION 'membership changes require an exact new conversation context epoch' USING ERRCODE='23514';
        END IF;
    END IF;
    IF (TG_TABLE_NAME='conversations' AND (c.current_revision_id<>r.id OR c.version<>r.seq))
        OR c.version<>(SELECT max(seq) FROM conversation_revisions WHERE conversation_id=c.id) THEN
        RAISE EXCEPTION 'conversation head must reference the latest complete snapshot' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER conversation_head_consistent AFTER INSERT OR UPDATE ON conversations
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_conversation_snapshot();
CREATE CONSTRAINT TRIGGER conversation_revision_consistent AFTER INSERT ON conversation_revisions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_conversation_snapshot();
CREATE CONSTRAINT TRIGGER conversation_members_consistent AFTER INSERT ON conversation_members
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_conversation_snapshot();
INSERT INTO roles(id,org_id,key,name,requires_mfa)
    SELECT gen_random_uuid(),id,'agent_user','公司秘书使用人',false FROM organizations ON CONFLICT(org_id,key) DO NOTHING;
INSERT INTO role_actions(org_id,role_id,action)
    SELECT r.org_id,r.id,a.action FROM roles r CROSS JOIN
        (VALUES('conversation.read'),('conversation.create'),('conversation.update'),('conversation.manage_members'),('agent.run')) a(action)
    WHERE r.key='agent_user' ON CONFLICT DO NOTHING;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') THEN
        GRANT SELECT,INSERT,UPDATE ON conversations TO copypaste_app;
        GRANT SELECT,INSERT ON conversation_revisions,conversation_members TO copypaste_app;
        REVOKE DELETE ON conversations FROM copypaste_app;
        REVOKE UPDATE,DELETE ON conversation_revisions,conversation_members FROM copypaste_app;
    END IF;
END;
$$;
