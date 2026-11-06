package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func AgentSubscriptionRevisionBasis(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Subscription, error) {
	v, e := subscriptionMetadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return v, e
	}
	if a.RunID == "" || v.State != "draft" {
		return v, core.Forbidden()
	}
	if e = subscriptionReadable(ctx, tx, a, v); e != nil {
		return v, e
	}
	if e = authz.Require(ctx, tx, a, "subscription.update", subscriptionResource(v)); e != nil {
		return v, e
	}
	if _, _, _, _, e = subscriptionReferences(ctx, tx, a, v, v.SubscriptionContent, false); e != nil {
		return v, e
	}
	return presentSubscription(ctx, tx, a, v)
}

func PrepareAgentSubscriptionRevision(ctx context.Context, tx pgx.Tx, a core.Actor, id string, in SubscriptionPatch) (SubscriptionPatch, Subscription, error) {
	v, e := subscriptionMetadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return in, v, e
	}
	if a.RunID == "" || v.State != "draft" {
		return in, v, core.Forbidden()
	}
	if e = subscriptionReadable(ctx, tx, a, v); e != nil {
		return in, v, e
	}
	if e = authz.Require(ctx, tx, a, "subscription.update", subscriptionResource(v)); e != nil {
		return in, v, e
	}
	if validBase(in.Base) != nil || in.ExpectedVersion != v.Version || in.RevisionID != v.CurrentRevisionID || in.ContentHash != v.ContentHash {
		return in, v, core.Conflict()
	}
	if e = subscriptionProjectLocks(ctx, tx, a, v, nil, false, false); e != nil {
		return in, v, e
	}
	in.SubscriptionContent, e = normalizeSubscription(ctx, tx, in.SubscriptionContent, false)
	if e != nil {
		return in, v, e
	}
	if _, _, _, _, e = subscriptionReferences(ctx, tx, a, v, in.SubscriptionContent, true); e != nil {
		return in, v, e
	}
	v, e = presentSubscription(ctx, tx, a, v)
	return in, v, e
}

// This exception is only for the exact immutable human revision intent. It
// never admits activation, cash, a different subscription or a changed payload.
func agentSubscriptionRevisionCommand(ctx context.Context, db platform.DBTX, cmd core.Command, id string) error {
	var session string
	e := db.QueryRow(ctx, `SELECT i.session_id FROM agent_business_revision_intents i JOIN agent_runs r ON r.org_id=i.org_id AND r.id=i.run_id JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
 WHERE i.org_id=$1 AND i.run_id=$2 AND i.actor_id=$3 AND i.kind='subscription' AND i.resource_id=$4 AND i.arguments=$5::jsonb
 AND r.state='running' AND r.cancel_requested_at IS NULL AND r.expires_at>clock_timestamp() AND p.skill_id='oa-assist' AND p.state='active'`, cmd.Actor.OrgID, cmd.Actor.RunID, cmd.Actor.UserID, id, cmd.Body).Scan(&session)
	if e != nil {
		return core.Missing()
	}
	human := cmd.Actor
	human.RunID = ""
	human.SessionID = session
	return identity.RequireInteractiveSession(ctx, db, human, true)
}

// Retained old references remain authority-sensitive after an exact revision.
func AgentSubscriptionRevisionReferences(ctx context.Context, tx pgx.Tx, a core.Actor, id string, content json.RawMessage) error {
	v, e := subscriptionMetadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return e
	}
	if a.RunID == "" {
		return core.Forbidden()
	}
	if e = subscriptionReadable(ctx, tx, a, v); e != nil {
		return e
	}
	var selected SubscriptionContent
	if json.Unmarshal(content, &selected) != nil {
		return core.Missing()
	}
	_, _, _, _, e = subscriptionReferences(ctx, tx, a, v, selected, false)
	return e
}
