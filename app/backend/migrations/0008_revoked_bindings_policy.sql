CREATE OR REPLACE FUNCTION can_access(
    p_org uuid, p_user uuid, p_action text, p_type text, p_id uuid,
    p_project uuid, p_owner uuid, p_class text, p_channel text, p_ai boolean
) RETURNS boolean LANGUAGE sql STABLE AS $$
SELECT
    EXISTS(SELECT 1 FROM users u WHERE u.org_id=p_org AND u.id=p_user AND u.status='active')
    AND EXISTS(
        SELECT 1 FROM classification_policies cp
        WHERE cp.org_id=p_org AND cp.level=p_class
        AND cp.policy_version=(SELECT max(cp2.policy_version) FROM classification_policies cp2 WHERE cp2.org_id=p_org AND cp2.level=p_class)
        AND p_channel=ANY(cp.allowed_channels) AND (NOT p_ai OR cp.ai_allowed)
    )
    AND EXISTS(
        SELECT 1 FROM role_bindings rb JOIN role_actions ra ON ra.org_id=rb.org_id AND ra.role_id=rb.role_id
        WHERE rb.org_id=p_org AND rb.user_id=p_user AND ra.action=p_action
        AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())
        AND classification_rank(rb.classification_ceiling)>=classification_rank(p_class)
        AND (
            rb.scope_type='org'
            OR (rb.scope_type='project' AND rb.scope_id=p_project)
            OR (rb.scope_type='resource' AND rb.scope_id=p_id)
            OR (rb.scope_type='own' AND p_owner=p_user)
        )
    )
    AND (
        p_class='internal'
        OR (p_class='project_restricted' AND p_project IS NOT NULL AND EXISTS(
            SELECT 1 FROM project_members pm WHERE pm.org_id=p_org AND pm.project_id=p_project AND pm.user_id=p_user
        ))
        OR EXISTS(
            SELECT 1 FROM resource_grants rg WHERE rg.org_id=p_org AND rg.user_id=p_user
            AND rg.resource_type=p_type AND rg.resource_id=p_id AND rg.action=p_action
            AND classification_rank(rg.classification_ceiling)>=classification_rank(p_class)
            AND (rg.expires_at IS NULL OR rg.expires_at>now())
        )
    );
$$;
