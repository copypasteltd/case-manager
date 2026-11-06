package agent

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

// File writes are delegated by one exact live human login. A later login is
// allowed to read the receipt or explicitly recover its bytes in a NEW run,
// but cannot revive the old delegation. Do not change the executing actor's
// channel or RunID: the temporary human identity is used only for this gate.
func requireFileIntentSession(ctx context.Context, tx pgx.Tx, a core.Actor, i *fileIntentRecord) error {
	if i.SessionID == nil || i.OriginChannel == nil || !core.ValidID(*i.SessionID) || *i.OriginChannel != a.Channel || i.OrgID != a.OrgID || i.ActorID != a.UserID {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "文件操作的原始登录授权已变化或未绑定，请明确新建恢复请求")
	}
	human := a
	human.RunID = ""
	human.SessionID = *i.SessionID
	human.Channel = *i.OriginChannel
	return identity.RequireInteractiveSession(ctx, tx, human, false)
}

// A read-only status query deliberately does not require the original session
// to remain live. All ordinary current conversation/file reading checks still
// run before this metadata is shown. Session identifiers stay private.
func fileIntentAuthorizationState(ctx context.Context, tx pgx.Tx, i *fileIntentRecord) (string, error) {
	var state string
	err := tx.QueryRow(ctx, `SELECT CASE WHEN i.session_id IS NULL OR i.origin_channel IS NULL THEN 'legacy_unbound'
 WHEN EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
 JOIN organizations o ON o.id=s.org_id JOIN agent_runs r ON r.org_id=i.org_id AND r.id=i.run_id
 WHERE s.org_id=i.org_id AND s.id=i.session_id AND s.user_id=i.actor_id AND r.actor_id=i.actor_id
 AND s.device_type=i.origin_channel AND r.channel=i.origin_channel AND s.revoked_at IS NULL
 AND s.expires_at>clock_timestamp() AND u.status='active' AND s.auth_version=u.auth_version
 AND u.auth_version=r.actor_auth_version AND o.authorization_version=r.organization_auth_version)
 THEN 'current' ELSE 'changed' END FROM agent_file_intents i WHERE i.org_id=$1 AND i.run_id=$2`, i.OrgID, i.RunID).Scan(&state)
	return state, err
}
