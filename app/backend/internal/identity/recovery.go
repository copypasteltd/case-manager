package identity

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"crypto/subtle"
	"time"
)

// RecoverMFA consumes a single recovery code under a password-authenticated,
// short-lived challenge. It does not disable the configured authenticator.
func (s *Service) RecoverMFA(ctx context.Context, input MFAInput, requestID string) (LoginResult, error) {
	var out LoginResult
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = challengeOrganization(ctx, tx, core.Digest([]byte(input.Challenge))); err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	var challengeID, org, user, device string
	var version int64
	err = tx.QueryRow(ctx, `SELECT c.id,c.org_id,c.user_id,c.device_type,u.auth_version FROM auth_challenges c
        JOIN users u ON u.org_id=c.org_id AND u.id=c.user_id WHERE c.token_hash=$1 AND c.purpose='mfa'
        AND c.auth_version=u.auth_version AND u.status='active' AND c.expires_at>now() AND c.consumed_at IS NULL AND c.attempts<5
        FOR UPDATE OF c,u`, core.Digest([]byte(input.Challenge))).Scan(&challengeID, &org, &user, &device, &version)
	if err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	var hashes []string
	if err = tx.QueryRow(ctx, "SELECT recovery_hashes FROM mfa_credentials WHERE org_id=$1 AND user_id=$2 FOR UPDATE", org, user).Scan(&hashes); err != nil {
		return out, err
	}
	submitted := core.Digest([]byte(input.Code))
	matched := -1
	for index, hash := range hashes {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(submitted)) == 1 {
			matched = index
		}
	}
	if matched < 0 || len(input.Code) != 20 {
		if _, err = tx.Exec(ctx, "UPDATE auth_challenges SET attempts=attempts+1 WHERE id=$1", challengeID); err != nil {
			return out, err
		}
		if err = authAudit(ctx, tx, org, user, requestID, "auth.mfa_recovery", "denied", device); err != nil {
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		return out, core.Fail(401, "MFA_INVALID", "恢复码无效或已使用")
	}
	remaining := append(hashes[:matched:matched], hashes[matched+1:]...)
	if _, err = tx.Exec(ctx, "UPDATE mfa_credentials SET recovery_hashes=$3 WHERE org_id=$1 AND user_id=$2", org, user, remaining); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "UPDATE auth_challenges SET consumed_at=now() WHERE id=$1", challengeID); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND user_id=$2", org, user); err != nil {
		return out, err
	}
	at := time.Now().UTC()
	out, err = s.issue(ctx, tx, org, user, device, core.NewID(), version, &at)
	if err != nil {
		return out, err
	}
	if err = authAudit(ctx, tx, org, user, requestID, "auth.mfa_recovery", "succeeded", device); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}
