package project

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
)

// Only an exact stored human submission intent may use its original session.
// Keep the AI actor/run on business permissions, receipts, events and audit.
// This grants neither a general interactive identity nor acceptance authority.
func deliverableSubmissionSession(ctx context.Context, db platform.DBTX, c core.Command, id string) error {
	a := c.Actor
	if a.RunID == "" {
		return identity.RequireInteractiveSession(ctx, db, a, false)
	}
	if c.Route != "/internal/agent-mutations/submit_deliverable" || c.Key != "agent-mutation:"+a.RunID {
		return core.Forbidden()
	}
	var session, originalChannel string
	err := db.QueryRow(ctx, `SELECT m.session_id,agent_original_channel(r.org_id,r.id,r.channel) FROM agent_run_mutations m
 JOIN agent_runs r ON r.org_id=m.org_id AND r.id=m.run_id
 JOIN conversations c ON c.org_id=r.org_id AND c.id=r.conversation_id
 JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
 JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
 WHERE m.org_id=$1 AND m.run_id=$2 AND m.actor_id=$3 AND m.resource_id=$4
 AND m.tool='submit_deliverable' AND m.resource_type='deliverable' AND m.session_id=$5
 AND m.arguments=$6::jsonb AND r.actor_id=m.actor_id AND r.conversation_id=m.conversation_id
 AND r.actor_auth_version=$7 AND r.organization_auth_version=$8 AND r.channel=$9
 AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp()
 AND cr.state='active' AND c.project_id=m.project_id AND c.current_revision_id=r.conversation_revision_id
 AND cr.context_security_version=r.context_security_version AND p.skill_id='project-update' AND p.state='active'
 AND (r.channel<>'telegram' OR EXISTS(SELECT 1 FROM telegram_write_grants g
 JOIN telegram_write_activations wa ON wa.org_id=g.org_id AND wa.grant_id=g.id
 JOIN telegram_channels ch ON ch.org_id=g.org_id AND ch.id=g.channel_id
 JOIN telegram_bindings b ON b.org_id=g.org_id AND b.id=g.binding_id
 WHERE g.org_id=r.org_id AND g.run_id=r.id AND g.actor_id=r.actor_id AND g.session_id=m.session_id
 AND g.expires_at>clock_timestamp() AND ch.state='active' AND ch.version=g.channel_version
 AND b.state='active' AND b.version=g.binding_version AND b.user_id=g.actor_id
 AND b.bot_id=ch.bot_id AND p.telegram_write_configuration_sha256=g.configuration_sha256))`,
		a.OrgID, a.RunID, a.UserID, id, a.SessionID, c.Body, a.AuthVersion, a.OrgAuthVersion, a.Channel).Scan(&session, &originalChannel)
	if err != nil {
		return core.Forbidden()
	}
	telegram := a.Channel == "telegram"
	a.RunID, a.SessionID, a.Channel = "", session, originalChannel
	return identity.RequireInteractiveSession(ctx, db, a, telegram)
}
