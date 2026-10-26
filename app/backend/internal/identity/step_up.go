package identity

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

type StepUpInput struct {
	Code string `json:"code"`
}
type StepUpResult struct {
	MFAVerifiedAt time.Time `json:"mfa_verified_at"`
}

// Reverify only the current live session. The device family, credentials and
// any pending business form stay intact; no MFA secret enters an audit/event.
func (s *Service) StepUp(ctx context.Context, a core.Actor, in StepUpInput, requestID string) (StepUpResult, error) {
	var out StepUpResult
	if a.RunID != "" || !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) || a.SessionID == "" {
		return out, core.Forbidden()
	}
	if err := s.rate(ctx, "mfa-step-up/"+a.OrgID+"/"+a.UserID, 8); err != nil {
		return out, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, a.OrgID); err != nil {
		return out, err
	}
	var live bool
	err = tx.QueryRow(ctx, `SELECT s.revoked_at IS NULL AND s.expires_at>now() AND s.device_type=$4
		AND s.auth_version=u.auth_version AND u.auth_version=$5 AND u.status='active'
		FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id
		WHERE s.org_id=$1 AND s.user_id=$2 AND s.id=$3 FOR UPDATE OF s,u`,
		a.OrgID, a.UserID, a.SessionID, a.Channel, a.AuthVersion).Scan(&live)
	if err != nil || !live {
		return out, core.Fail(409, "SESSION_CHANGED", "会话已更新，请确认登录状态后重试身份验证")
	}
	var cipher []byte
	var lastStep int64
	err = tx.QueryRow(ctx, "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2 FOR UPDATE", a.OrgID, a.UserID).Scan(&cipher, &lastStep)
	if err == pgx.ErrNoRows {
		return out, core.Fail(409, "MFA_ENROLLMENT_REQUIRED", "此账号尚未设置验证器，请联系管理员确认角色并完成登录验证设置")
	}
	if err != nil {
		return out, err
	}
	secret, err := Open(s.Config.MFAKey, cipher, a.OrgID+":"+a.UserID)
	if err != nil {
		return out, err
	}
	step, valid := MatchTOTP(secret, in.Code, time.Now())
	if !valid || step <= lastStep {
		if err = authAudit(ctx, tx, a.OrgID, a.UserID, requestID, "auth.mfa_step_up", "denied", a.Channel); err != nil {
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		return out, core.Fail(422, "MFA_INVALID", "验证码无效或已使用，请等待验证器更新后重试")
	}
	out.MFAVerifiedAt = time.Now().UTC()
	if _, err = tx.Exec(ctx, "UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", a.OrgID, a.UserID, step); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", a.SessionID, out.MFAVerifiedAt); err != nil {
		return out, err
	}
	if err = authAudit(ctx, tx, a.OrgID, a.UserID, requestID, "auth.mfa_step_up", "succeeded", a.Channel); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
