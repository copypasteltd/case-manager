package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/url"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/miniapp"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	MiniappExchange miniapp.Exchanger
	DB              *pgxpool.Pool
	Config          platform.Config
	DummyHash       string
}
type LoginInput struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	Device   string `json:"device_type"`
}
type MFAInput struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}
type LoginResult struct {
	Step          string    `json:"step"`
	Challenge     string    `json:"challenge,omitempty"`
	AccessToken   string    `json:"access_token,omitempty"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	CSRFToken     string    `json:"csrf_token,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitzero"`
	MFASecret     string    `json:"mfa_secret,omitempty"`
	MFAURI        string    `json:"mfa_uri,omitempty"`
	RecoveryCodes []string  `json:"recovery_codes,omitempty"`
	Device        string    `json:"-"`
}

func NewService(db *pgxpool.Pool, c platform.Config) (*Service, error) {
	dummy, err := HashPassword(Token())
	if err != nil {
		return nil, err
	}
	return &Service{DB: db, Config: c, DummyHash: dummy}, nil
}
func (s *Service) Login(ctx context.Context, input LoginInput, ip, requestID string) (LoginResult, error) {
	var out LoginResult
	if input.Device == "" {
		input.Device = "web"
	}
	if input.Device != "web" && input.Device != "android" && input.Device != "wechat" && input.Device != "douyin" {
		return out, core.Invalid("device_type", "客户端类型无效")
	}
	if len(input.Login) > 200 || len(input.Password) > 1024 {
		return out, core.Fail(401, "INVALID_CREDENTIALS", "登录资料无效")
	}
	if err := s.rate(ctx, "login-ip:"+ip, 30); err != nil {
		return out, err
	}
	if err := s.rate(ctx, "login-key:"+strings.ToLower(strings.TrimSpace(input.Login)), 10); err != nil {
		return out, err
	}
	var org, id, hash, status string
	var version int64
	var needsMFA, hasMFA bool
	err := s.DB.QueryRow(ctx, `SELECT u.org_id,u.id,u.credential_hash,u.status,u.auth_version,
		EXISTS(SELECT 1 FROM role_bindings rb JOIN roles r ON r.org_id=rb.org_id AND r.id=rb.role_id
			WHERE rb.org_id=u.org_id AND rb.user_id=u.id AND r.requires_mfa AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())),
		EXISTS(SELECT 1 FROM mfa_credentials m WHERE m.org_id=u.org_id AND m.user_id=u.id)
		FROM users u WHERE u.login_key=$1`, strings.ToLower(strings.TrimSpace(input.Login))).Scan(&org, &id, &hash, &status, &version, &needsMFA, &hasMFA)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err != nil {
		hash = s.DummyHash
	}
	valid := CheckPassword(hash, input.Password)
	if !valid || err != nil || status != "active" {
		if org == "" {
			if auditErr := s.DB.QueryRow(ctx, "SELECT id FROM organizations").Scan(&org); auditErr != nil {
				return out, auditErr
			}
		}
		if auditErr := authAudit(ctx, s.DB, org, id, requestID, "auth.login", "denied", input.Device); auditErr != nil {
			return out, auditErr
		}
		return out, core.Fail(401, "INVALID_CREDENTIALS", "登录资料无效")
	}
	if needsMFA || hasMFA {
		challenge := Token()
		purpose := "mfa"
		if !hasMFA {
			purpose = "mfa_enroll"
		}
		tx, txErr := s.DB.Begin(ctx)
		if txErr != nil {
			return out, txErr
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO auth_challenges(id,org_id,user_id,token_hash,purpose,device_type,expires_at,auth_version)
			VALUES($1,$2,$3,$4,$5,$6,now()+interval '5 minutes',$7)`, core.NewID(), org, id, core.Digest([]byte(challenge)), purpose, input.Device, version)
		if err != nil {
			return out, err
		}
		if err = authAudit(ctx, tx, org, id, requestID, "auth.password_verified", "succeeded", input.Device); err != nil {
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		return LoginResult{Step: purpose, Challenge: challenge, Device: input.Device}, nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, org); err != nil {
		return out, err
	}
	var activeVersion int64
	if err = tx.QueryRow(ctx, "SELECT auth_version FROM users WHERE org_id=$1 AND id=$2 AND status='active' FOR SHARE", org, id).Scan(&activeVersion); err != nil || activeVersion != version {
		return out, core.Fail(401, "INVALID_CREDENTIALS", "登录资料已变更，请重新登录")
	}
	out, err = s.issue(ctx, tx, org, id, input.Device, core.NewID(), version, nil)
	if err != nil {
		return out, err
	}
	if err = authAudit(ctx, tx, org, id, requestID, "auth.login", "succeeded", input.Device); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) Enroll(ctx context.Context, challenge string) (LoginResult, error) {
	var out LoginResult
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = challengeOrganization(ctx, tx, core.Digest([]byte(challenge))); err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	var id, user, org, login string
	var cipher []byte
	err = tx.QueryRow(ctx, `SELECT c.id,c.user_id,c.org_id,u.login_key,c.secret_cipher FROM auth_challenges c JOIN users u ON u.org_id=c.org_id AND u.id=c.user_id
		WHERE c.token_hash=$1 AND c.purpose='mfa_enroll' AND c.expires_at>now() AND c.consumed_at IS NULL AND c.attempts<5 AND u.status='active' AND c.auth_version=u.auth_version FOR UPDATE OF c,u`,
		core.Digest([]byte(challenge))).Scan(&id, &user, &org, &login, &cipher)
	if err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	var challengeDevice string
	if err = tx.QueryRow(ctx, "SELECT device_type FROM auth_challenges WHERE id=$1", id).Scan(&challengeDevice); err != nil {
		return out, err
	}
	if _, err = s.miniappChallengeSource(ctx, tx, id, org, user, challengeDevice); err != nil {
		return out, err
	}
	if len(cipher) > 0 {
		return out, core.Fail(409, "ENROLLMENT_ALREADY_STARTED", "设置资料已展示，请完成验证或重新登录")
	}
	secret := NewTOTPSecret()
	cipher, err = Seal(s.Config.MFAKey, secret, org+":"+user)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, "UPDATE auth_challenges SET secret_cipher=$2 WHERE id=$1", id, cipher)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return LoginResult{Step: "mfa_enroll", MFASecret: secret, MFAURI: "otpauth://totp/" + url.PathEscape("CopyPaste:"+login) + "?secret=" + secret + "&issuer=CopyPaste&algorithm=SHA1&digits=6&period=30"}, nil
}
func (s *Service) VerifyMFA(ctx context.Context, input MFAInput, requestID string) (LoginResult, error) {
	var out LoginResult
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = challengeOrganization(ctx, tx, core.Digest([]byte(input.Challenge))); err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	var id, org, user, purpose, device string
	var cipher []byte
	var version int64
	err = tx.QueryRow(ctx, `SELECT c.id,c.org_id,c.user_id,c.purpose,c.device_type,c.secret_cipher,u.auth_version
		FROM auth_challenges c JOIN users u ON u.org_id=c.org_id AND u.id=c.user_id WHERE c.token_hash=$1 AND c.expires_at>now()
		AND c.consumed_at IS NULL AND c.attempts<5 AND u.status='active' AND c.auth_version=u.auth_version AND c.purpose IN ('mfa','mfa_enroll') FOR UPDATE OF c,u`,
		core.Digest([]byte(input.Challenge))).Scan(&id, &org, &user, &purpose, &device, &cipher, &version)
	if err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "验证已过期，请重新登录")
	}
	bindingID, err := s.miniappChallengeSource(ctx, tx, id, org, user, device)
	if err != nil {
		return out, err
	}
	var lastStep int64
	if purpose == "mfa" {
		err = tx.QueryRow(ctx, "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2 FOR UPDATE", org, user).Scan(&cipher, &lastStep)
		if err != nil {
			return out, err
		}
	}
	secret, err := Open(s.Config.MFAKey, cipher, org+":"+user)
	if err != nil {
		return out, core.Fail(401, "CHALLENGE_INVALID", "请重新开始验证")
	}
	step, valid := MatchTOTP(secret, input.Code, time.Now())
	if !valid || step <= lastStep {
		if _, err = tx.Exec(ctx, "UPDATE auth_challenges SET attempts=attempts+1 WHERE id=$1", id); err != nil {
			return out, err
		}
		if err = authAudit(ctx, tx, org, user, requestID, "auth.mfa", "denied", device); err != nil {
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		return out, core.Fail(401, "MFA_INVALID", "验证码无效或已使用")
	}
	recovery := []string{}
	if purpose == "mfa_enroll" {
		hashes := []string{}
		for range 8 {
			code := Token()[:20]
			recovery = append(recovery, code)
			hashes = append(hashes, core.Digest([]byte(code)))
		}
		_, err = tx.Exec(ctx, "INSERT INTO mfa_credentials(id,org_id,user_id,secret_cipher,recovery_hashes,last_step) VALUES($1,$2,$3,$4,$5,$6)", core.NewID(), org, user, cipher, hashes, step)
	} else {
		_, err = tx.Exec(ctx, "UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", org, user, step)
	}
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, "UPDATE auth_challenges SET consumed_at=now() WHERE id=$1", id)
	if err != nil {
		return out, err
	}
	at := time.Now().UTC()
	out, err = s.issue(ctx, tx, org, user, device, core.NewID(), version, &at)
	if err != nil {
		return out, err
	}
	if err = linkMiniappSession(ctx, tx, out.AccessToken, bindingID); err != nil {
		return LoginResult{}, err
	}
	out.RecoveryCodes = recovery
	if err = authAudit(ctx, tx, org, user, requestID, "auth.mfa", "succeeded", device); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) issue(ctx context.Context, tx pgx.Tx, org, user, device, family string, version int64, mfa *time.Time) (LoginResult, error) {
	out := LoginResult{Step: "authenticated", AccessToken: Token(), RefreshToken: Token(), CSRFToken: Token(), Device: device, ExpiresAt: time.Now().UTC().Add(s.Config.AccessTTL)}
	_, err := tx.Exec(ctx, `INSERT INTO sessions(id,org_id,user_id,family_id,access_hash,refresh_hash,csrf_hash,device_type,expires_at,refresh_expires_at,auth_version,mfa_verified_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, core.NewID(), org, user, family, core.Digest([]byte(out.AccessToken)), core.Digest([]byte(out.RefreshToken)), core.Digest([]byte(out.CSRFToken)),
		device, out.ExpiresAt, time.Now().UTC().Add(s.Config.RefreshTTL), version, mfa)
	return out, err
}
func (s *Service) Authenticate(ctx context.Context, token string) (core.Actor, error) {
	var a core.Actor
	if len(token) < 40 || len(token) > 100 {
		return a, core.Fail(401, "UNAUTHENTICATED", "请先登录")
	}
	err := s.DB.QueryRow(ctx, `SELECT u.org_id,u.id,u.display_name,u.auth_version,o.authorization_version,s.id,s.device_type,s.mfa_verified_at
		FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=u.org_id
		WHERE s.access_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.status='active' AND s.auth_version=u.auth_version
		AND (s.miniapp_binding_id IS NULL OR s.mfa_verified_at IS NOT NULL OR NOT EXISTS(SELECT 1 FROM mfa_credentials mc WHERE mc.org_id=s.org_id AND mc.user_id=s.user_id))
  AND (s.miniapp_binding_id IS NULL OR EXISTS(SELECT 1 FROM miniapp_bindings b WHERE b.id=s.miniapp_binding_id AND b.org_id=s.org_id AND b.user_id=s.user_id AND b.revoked_at IS NULL AND b.platform=s.device_type AND ((b.platform='wechat' AND b.app_id=$2) OR (b.platform='douyin' AND b.app_id=$3))))
  AND (s.mfa_verified_at IS NOT NULL OR NOT EXISTS(SELECT 1 FROM role_bindings rb JOIN roles r ON r.org_id=rb.org_id AND r.id=rb.role_id
			WHERE rb.org_id=u.org_id AND rb.user_id=u.id AND r.requires_mfa AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())))`,
		core.Digest([]byte(token)), s.configuredMiniappID("wechat"), s.configuredMiniappID("douyin")).Scan(&a.OrgID, &a.UserID, &a.DisplayName, &a.AuthVersion, &a.OrgAuthVersion, &a.SessionID, &a.Channel, &a.MFAVerifiedAt)
	if err != nil {
		return a, core.Fail(401, "UNAUTHENTICATED", "会话已过期或权限已变更，请重新登录")
	}
	return a, nil
}
func (s *Service) CSRF(ctx context.Context, sessionID, token string) error {
	if token == "" {
		return core.Forbidden()
	}
	var expected string
	err := s.DB.QueryRow(ctx, "SELECT csrf_hash FROM sessions WHERE id=$1 AND revoked_at IS NULL", sessionID).Scan(&expected)
	if err != nil || subtle.ConstantTimeCompare([]byte(expected), []byte(core.Digest([]byte(token)))) != 1 {
		return core.Forbidden()
	}
	return nil
}
func (s *Service) Refresh(ctx context.Context, refresh, csrf string, cookie bool, requestID string) (LoginResult, error) {
	var out LoginResult
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var id, org, user, family, device, csrfHash, status string
	var revoked, refreshed, mfa *time.Time
	var miniappBinding *string
	var storedVersion, currentVersion int64
	var expires time.Time
	var sessionOrg string
	if err = tx.QueryRow(ctx, "SELECT org_id FROM sessions WHERE refresh_hash=$1", core.Digest([]byte(refresh))).Scan(&sessionOrg); err != nil {
		return out, core.Fail(401, "UNAUTHENTICATED", "请重新登录")
	}
	if err = lockOrganization(ctx, tx, sessionOrg); err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT s.id,s.org_id,s.user_id,s.family_id,s.device_type,s.csrf_hash,s.revoked_at,s.refreshed_at,s.refresh_expires_at,s.auth_version,u.auth_version,u.status,s.mfa_verified_at,s.miniapp_binding_id
		FROM sessions s JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id WHERE refresh_hash=$1 FOR UPDATE OF s,u`,
		core.Digest([]byte(refresh))).Scan(&id, &org, &user, &family, &device, &csrfHash, &revoked, &refreshed, &expires, &storedVersion, &currentVersion, &status, &mfa, &miniappBinding)
	if err != nil {
		return out, core.Fail(401, "UNAUTHENTICATED", "请重新登录")
	}
	if (cookie && device != "web") || (!cookie && device == "web") {
		return out, core.Forbidden()
	}
	if cookie && subtle.ConstantTimeCompare([]byte(csrfHash), []byte(core.Digest([]byte(csrf)))) != 1 {
		return out, core.Forbidden()
	}
	if refreshed != nil {
		if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND family_id=$2", org, family); err != nil {
			return out, err
		}
		if err = authAudit(ctx, tx, org, user, requestID, "auth.refresh_reuse", "denied", device); err != nil {
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		return out, core.Fail(401, "SESSION_REVOKED", "刷新凭证重复使用，会话已撤销")
	}
	if revoked != nil || !time.Now().Before(expires) || status != "active" || storedVersion != currentVersion {
		return out, core.Fail(401, "SESSION_REVOKED", "会话已失效，请重新登录")
	}
	if miniappBinding != nil {
		if mfa == nil {
			var enrolled bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mfa_credentials WHERE org_id=$1 AND user_id=$2)`, org, user).Scan(&enrolled); err != nil {
				return out, err
			}
			if enrolled {
				return out, core.Fail(401, "MINIAPP_MFA_REQUIRED", "请重新通过小程序登录并完成两步验证")
			}
		}
		if err = s.checkMiniappBinding(ctx, tx, *miniappBinding, org, user, device); err != nil {
			return out, err
		}
	}
	_, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=now(),refreshed_at=now() WHERE id=$1", id)
	if err != nil {
		return out, err
	}
	out, err = s.issue(ctx, tx, org, user, device, family, currentVersion, mfa)
	if err != nil {
		return out, err
	}
	if miniappBinding != nil {
		if err = linkMiniappSession(ctx, tx, out.AccessToken, *miniappBinding); err != nil {
			return LoginResult{}, err
		}
	}
	if err = authAudit(ctx, tx, org, user, requestID, "auth.refresh", "succeeded", device); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) Logout(ctx context.Context, a core.Actor, requestID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, a.OrgID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND user_id=$2 AND id=$3", a.OrgID, a.UserID, a.SessionID); err != nil {
		return err
	}
	if err = authAudit(ctx, tx, a.OrgID, a.UserID, requestID, "auth.logout", "succeeded", a.Channel); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) rate(ctx context.Context, key string, limit int) error {
	var attempts int
	err := s.DB.QueryRow(ctx, `INSERT INTO rate_limits(bucket,window_start,attempts) VALUES($1,date_bin('15 minutes',now(),'2000-01-01'::timestamptz),1)
		ON CONFLICT(bucket,window_start) DO UPDATE SET attempts=rate_limits.attempts+1 RETURNING attempts`, core.Digest([]byte(key))).Scan(&attempts)
	if err != nil {
		return err
	}
	if attempts > limit {
		return core.Fail(429, "RATE_LIMITED", "尝试次数过多，请稍后再试")
	}
	return nil
}
func authAudit(ctx context.Context, db platform.DBTX, org, user, requestID, action, result, channel string) error {
	_, err := db.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,result)
		VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6,'user',NULLIF($3,'')::uuid,$7)`, core.NewID(), org, user, channel, requestID, action, result)
	return err
}
func (s *Service) Me(ctx context.Context, a core.Actor) (map[string]any, error) {
	var version int64
	var created, updated, sessionExpires time.Time
	err := s.DB.QueryRow(ctx, `SELECT u.version,u.created_at,u.updated_at,s.expires_at FROM users u
        JOIN sessions s ON s.org_id=u.org_id AND s.user_id=u.id WHERE u.org_id=$1 AND u.id=$2 AND s.id=$3`,
		a.OrgID, a.UserID, a.SessionID).Scan(&version, &created, &updated, &sessionExpires)
	if err != nil {
		return nil, err
	}
	caps, err := s.Capabilities(ctx, a)
	if err != nil {
		return nil, err
	}
	actions := []string{}
	seen := map[string]bool{}
	for _, capability := range caps {
		action := capability["action"].(string)
		if !seen[action] {
			actions = append(actions, action)
			seen[action] = true
		}
	}
	return map[string]any{"user": map[string]any{"id": a.UserID, "version": version, "created_at": created, "updated_at": updated, "display_name": a.DisplayName, "status": "active"}, "actions": actions, "authorization_version": a.OrgAuthVersion, "session_expires_at": sessionExpires, "mfa_verified_at": a.MFAVerifiedAt}, nil
}
func (s *Service) Capabilities(ctx context.Context, a core.Actor) ([]map[string]any, error) {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ra.action,rb.scope_type,rb.scope_id FROM role_bindings rb JOIN role_actions ra ON ra.org_id=rb.org_id AND ra.role_id=rb.role_id
		WHERE rb.org_id=$1 AND rb.user_id=$2 AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now()) ORDER BY ra.action,rb.scope_type,rb.scope_id`, a.OrgID, a.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var action, scope string
		var id *string
		if err = rows.Scan(&action, &scope, &id); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"action": action, "scope_type": scope, "scope_id": id})
	}
	return items, rows.Err()
}
