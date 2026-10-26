package identity

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// RequireInteractiveSession is called inside an existing command transaction,
// after its organization/user locks. The row lock prevents a concurrent logout
// or rotation from revoking this session between the check and commit.
func RequireInteractiveSession(ctx context.Context, db platform.DBTX, a core.Actor, mfa bool) error {
	if a.RunID != "" || !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) {
		return core.Forbidden()
	}
	var verified *time.Time
	var id string
	err := db.QueryRow(ctx, `SELECT s.id,s.mfa_verified_at FROM sessions s
        JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
        WHERE s.org_id=$1 AND s.id=$2 AND s.user_id=$3 AND s.device_type=$4
        AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
        AND s.auth_version=u.auth_version AND u.auth_version=$5 AND o.authorization_version=$6 FOR SHARE OF s`,
		a.OrgID, a.SessionID, a.UserID, a.Channel, a.AuthVersion, a.OrgAuthVersion).Scan(&id, &verified)
	if err == pgx.ErrNoRows {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "权限或会话已变更，请重新获取当前授权")
	}
	if err != nil {
		return err
	}
	if mfa && (verified == nil || time.Since(*verified) > 10*time.Minute || verified.After(time.Now().Add(time.Minute))) {
		return core.Fail(403, "REAUTHENTICATION_REQUIRED", "此操作需要最近10分钟内完成两步验证")
	}
	return nil
}
