package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"strings"
)

func PrepareAgentSubscriptionCreate(ctx context.Context, db platform.DBTX, a core.Actor, in SubscriptionCreate) (SubscriptionCreate, error) {
	in.SourceKey = strings.TrimSpace(in.SourceKey)
	if a.RunID == "" || !sourcePattern.MatchString(in.SourceKey) || core.Name(in.Reason) != nil || !core.Classification(in.Classification) || in.Classification == "confidential" || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) {
		return in, core.Invalid("business_draft", "请明确订阅永久来源、范围、分类和依据")
	}
	v := Subscription{ID: core.NewID(), Version: 1, SourceKey: in.SourceKey, ProjectID: in.ProjectID, CreatorID: a.UserID, Classification: in.Classification, State: "draft"}
	for _, action := range []string{"subscription.read", "subscription.create"} {
		if e := authz.Require(ctx, db, a, action, subscriptionResource(v)); e != nil {
			return in, e
		}
	}
	if e := subscriptionProjectLocks(ctx, db, a, v, nil, false, false); e != nil {
		return in, e
	}
	c, e := normalizeSubscription(ctx, db, in.SubscriptionContent, false)
	if e != nil {
		return in, e
	}
	if _, _, _, _, e = subscriptionReferences(ctx, db, a, v, c, true); e != nil {
		return in, e
	}
	in.SubscriptionContent = c
	return in, nil
}

func AgentSubscriptionDraftRead(ctx context.Context, db platform.DBTX, a core.Actor, id string, args json.RawMessage) error {
	if a.RunID == "" {
		return core.Missing()
	}
	v, e := subscriptionMetadata(ctx, db, a.OrgID, id)
	if e != nil {
		return e
	}
	if e = subscriptionReadable(ctx, db, a, v); e != nil {
		return e
	}
	var in SubscriptionCreate
	if json.Unmarshal(args, &in) != nil || value(in.ProjectID) != value(v.ProjectID) || in.Classification != v.Classification {
		return core.Missing()
	}
	_, _, _, _, e = subscriptionReferences(ctx, db, a, v, in.SubscriptionContent, false)
	return e
}
func agentSubscriptionCommand(ctx context.Context, db platform.DBTX, cmd core.Command) error {
	var session string
	e := db.QueryRow(ctx, `SELECT i.session_id FROM agent_business_draft_intents i JOIN agent_runs r ON r.org_id=i.org_id AND r.id=i.run_id JOIN agent_runtime_profiles p ON p.org_id=r.org_id AND p.id=r.profile_id
 WHERE i.org_id=$1 AND i.run_id=$2 AND i.actor_id=$3 AND i.kind='subscription' AND i.arguments=$4::jsonb AND r.state='running' AND r.cancel_requested_at IS NULL AND p.skill_id='oa-assist' AND p.state='active' AND r.expires_at>clock_timestamp()`, cmd.Actor.OrgID, cmd.Actor.RunID, cmd.Actor.UserID, cmd.Body).Scan(&session)
	if e != nil {
		return core.Missing()
	}
	human := cmd.Actor
	human.RunID = ""
	human.SessionID = session
	return identity.RequireInteractiveSession(ctx, db, human, false)
}
