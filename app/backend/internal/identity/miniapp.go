package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/miniapp"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type MiniappProfile struct {
	Platform string `json:"platform"`
	AppID    string `json:"app_id"`
}
type MiniappProfiles struct {
	Items []MiniappProfile `json:"items"`
}
type MiniappCodeInput struct {
	Platform string `json:"platform"`
	AppID    string `json:"app_id"`
	Code     string `json:"code"`
}
type MiniappProofInput struct {
	Platform string `json:"platform"`
	AppID    string `json:"app_id"`
	Code     string `json:"code"`
	Password string `json:"current_password"`
}
type MiniappProof struct {
	ID        string    `json:"id"`
	Platform  string    `json:"platform"`
	AppID     string    `json:"app_id"`
	ExpiresAt time.Time `json:"expires_at"`
}
type MiniappBinding struct {
	ID        string     `json:"id"`
	Platform  string     `json:"platform"`
	AppID     string     `json:"app_id"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	Current   bool       `json:"is_current_session_source"`
}
type MiniappBindInput struct {
	ProofID   string `json:"proof_id"`
	Confirmed bool   `json:"confirmed"`
	Reason    string `json:"reason"`
}
type MiniappRevokeInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

func (s *Service) MiniappProfiles() MiniappProfiles {
	out := MiniappProfiles{Items: []MiniappProfile{}}
	for _, name := range []string{"wechat", "douyin"} {
		if p, ok := s.Config.Miniapp.Profile(name); ok {
			out.Items = append(out.Items, MiniappProfile{Platform: name, AppID: p.AppID})
		}
	}
	return out
}
func (s *Service) miniappProfile(in MiniappCodeInput) (miniapp.Profile, error) {
	p, ok := s.Config.Miniapp.Profile(in.Platform)
	if !ok || in.AppID != p.AppID {
		return p, core.Fail(503, "MINIAPP_UNAVAILABLE", "该小程序登录尚未配置，请使用内部账号登录")
	}
	if len(in.Code) < 1 || len(in.Code) > 512 || strings.ContainsAny(in.Code, "\x00\r\n\t ") {
		return p, core.Invalid("code", "请重新获取平台登录凭证")
	}
	return p, nil
}
func (s *Service) miniappDigest(domain, platform, appID, value string) string {
	mac := hmac.New(sha256.New, s.Config.Miniapp.IdentityKey)
	fmt.Fprintf(mac, "copypaste/miniapp/%s/v1\x00%s\x00%s\x00%s", domain, platform, appID, value)
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) miniappExchange(ctx context.Context, in MiniappCodeInput) (string, error) {
	p, err := s.miniappProfile(in)
	if err != nil {
		return "", err
	}
	// Claim before the network call. A timeout or lost response is not permission
	// to reuse a single-use provider code. A fresh user action obtains a new code.
	tag, err := s.DB.Exec(ctx, `INSERT INTO miniapp_code_claims(code_hash,platform,app_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, s.miniappDigest("code", in.Platform, in.AppID, in.Code), in.Platform, in.AppID)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", core.Fail(409, "MINIAPP_CODE_USED", "平台凭证已使用或结果未知，请重新获取")
	}
	exchanger := s.MiniappExchange
	if exchanger == nil {
		exchanger = miniapp.Client{}
	}
	subject, err := exchanger.Exchange(ctx, in.Platform, p, in.Code)
	if err != nil || subject == "" || len(subject) > 256 {
		return "", core.Fail(503, "MINIAPP_EXCHANGE_FAILED", "平台身份暂未确认，请重新获取凭证后再试")
	}
	return s.miniappDigest("subject", in.Platform, in.AppID, subject), nil
}

// A binding requires the employee's password, an interactive session and a
// recent MFA step whenever an authenticator is enrolled. AI runs cannot bind.
func miniappSelfAuthority(ctx context.Context, tx pgx.Tx, a core.Actor) error {
	var enrolled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mfa_credentials WHERE org_id=$1 AND user_id=$2)`, a.OrgID, a.UserID).Scan(&enrolled); err != nil {
		return err
	}
	return RequireInteractiveSession(ctx, tx, a, enrolled)
}

// Serialize these short credential checks with refresh and revocation before
// taking user/session locks. Both transactions end before any network exchange.
func (s *Service) miniappActorTX(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var orgVersion, version int64
	var status string
	err = tx.QueryRow(ctx, `SELECT authorization_version FROM organizations WHERE id=$1 FOR UPDATE`, a.OrgID).Scan(&orgVersion)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE`, a.OrgID, a.UserID).Scan(&version, &status)
	}
	if err == nil && (orgVersion != a.OrgAuthVersion || version != a.AuthVersion || status != "active") {
		err = core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新登录")
	}
	if err == nil {
		err = miniappSelfAuthority(ctx, tx, a)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func (s *Service) CreateMiniappProof(ctx context.Context, a core.Actor, in MiniappProofInput, ip, requestID string) (MiniappProof, error) {
	var out MiniappProof
	code := MiniappCodeInput{Platform: in.Platform, AppID: in.AppID, Code: in.Code}
	if _, err := s.miniappProfile(code); err != nil {
		return out, err
	}
	if a.Channel != in.Platform || a.RunID != "" {
		return out, core.Forbidden()
	}
	if len(in.Password) < 1 || len(in.Password) > 1024 {
		return out, core.Invalid("current_password", "请输入当前内部账号密码")
	}
	if err := s.rate(ctx, "miniapp-bind:"+a.OrgID+":"+a.UserID+":"+ip, 10); err != nil {
		return out, err
	}
	tx, err := s.miniappActorTX(ctx, a)
	if err != nil {
		return out, err
	}
	var hash string
	err = tx.QueryRow(ctx, `SELECT credential_hash FROM users WHERE org_id=$1 AND id=$2`, a.OrgID, a.UserID).Scan(&hash)
	if err != nil || !CheckPassword(hash, in.Password) {
		tx.Rollback(ctx)
		return out, core.Fail(403, "CURRENT_PASSWORD_INVALID", "当前密码不正确，请核对后重试")
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	// Never hold row locks while calling either remote provider.
	subject, err := s.miniappExchange(ctx, code)
	if err != nil {
		return out, err
	}
	tx, err = s.miniappActorTX(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out = MiniappProof{ID: core.NewID(), Platform: in.Platform, AppID: in.AppID, ExpiresAt: time.Now().UTC().Add(2 * time.Minute)}
	_, err = tx.Exec(ctx, `INSERT INTO miniapp_binding_proofs(id,org_id,user_id,session_id,platform,app_id,subject_hash,auth_version,org_auth_version,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, out.ID, a.OrgID, a.UserID, a.SessionID, in.Platform, in.AppID, subject, a.AuthVersion, a.OrgAuthVersion, out.ExpiresAt)
	if err != nil {
		return MiniappProof{}, err
	}
	if err = authAudit(ctx, tx, a.OrgID, a.UserID, requestID, "auth.miniapp_proof", "succeeded", a.Channel); err != nil {
		return MiniappProof{}, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) BindMiniapp(ctx context.Context, c core.Command, in MiniappBindInput) (core.Result, error) {
	if !core.ValidID(in.ProofID) || !in.Confirmed {
		return core.Result{}, core.Invalid("proof_id", "请本人确认小程序绑定资料")
	}
	if err := core.Name(strings.TrimSpace(in.Reason)); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := miniappSelfAuthority(ctx, tx, c.Actor); err != nil {
			return err
		}
		var name, appID string
		var valid bool
		err := tx.QueryRow(ctx, `SELECT p.platform,p.app_id,((p.consumed_at IS NULL AND p.expires_at>now()) OR (p.consumed_at IS NOT NULL AND EXISTS(SELECT 1 FROM miniapp_bindings b WHERE b.id=p.binding_id AND b.revoked_at IS NULL))) FROM miniapp_binding_proofs p WHERE p.id=$1 AND p.org_id=$2 AND p.user_id=$3 AND p.session_id=$4 AND p.auth_version=$5 AND p.org_auth_version=$6 FOR SHARE`, in.ProofID, c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID, c.Actor.AuthVersion, c.Actor.OrgAuthVersion).Scan(&name, &appID, &valid)
		profile, ok := s.Config.Miniapp.Profile(name)
		if err != nil || !valid || !ok || profile.AppID != appID || c.Actor.Channel != name {
			return core.Fail(409, "MINIAPP_PROOF_INVALID", "绑定确认已失效，请重新获取平台凭证")
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		var name, appID, subject string
		err := tx.QueryRow(ctx, `SELECT platform,app_id,subject_hash FROM miniapp_binding_proofs WHERE id=$1 AND org_id=$2 AND user_id=$3 AND session_id=$4 AND auth_version=$5 AND org_auth_version=$6 AND expires_at>now() AND consumed_at IS NULL FOR UPDATE`, in.ProofID, c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID, c.Actor.AuthVersion, c.Actor.OrgAuthVersion).Scan(&name, &appID, &subject)
		if err != nil {
			return core.Result{}, core.Fail(409, "MINIAPP_PROOF_INVALID", "绑定确认已失效，请重新获取平台凭证")
		}
		p, ok := s.Config.Miniapp.Profile(name)
		if !ok || p.AppID != appID || c.Actor.Channel != name {
			return core.Result{}, core.Forbidden()
		}
		out := MiniappBinding{ID: core.NewID(), Platform: name, AppID: appID, Version: 1}
		err = tx.QueryRow(ctx, `INSERT INTO miniapp_bindings(id,org_id,user_id,platform,app_id,subject_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, out.ID, c.Actor.OrgID, c.Actor.UserID, name, appID, subject).Scan(&out.CreatedAt)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if _, err = tx.Exec(ctx, `UPDATE miniapp_binding_proofs SET consumed_at=now(),binding_id=$2 WHERE id=$1`, in.ProofID, out.ID); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: out, Event: core.Event{Type: "auth.miniapp_bound", ResourceType: "miniapp_binding", ResourceID: out.ID, Version: 1, Payload: map[string]any{"platform": name, "app_id": appID, "reason": in.Reason}}}, nil
	})
}
func (s *Service) MiniappBindings(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[MiniappBinding], error) {
	out := core.Page[MiniappBinding]{Items: []MiniappBinding{}}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := "miniapp-bindings:" + a.OrgID + ":" + a.UserID + fmt.Sprint(":", a.AuthVersion, ":", a.OrgAuthVersion)
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	// Listing is ordinary self-service and does not require a fresh MFA step.
	rows, err := s.DB.Query(ctx, `SELECT b.id,b.platform,b.app_id,b.version,b.created_at,b.revoked_at,EXISTS(SELECT 1 FROM sessions s WHERE s.id=$3 AND s.org_id=b.org_id AND s.user_id=b.user_id AND s.miniapp_binding_id=b.id) FROM miniapp_bindings b WHERE b.org_id=$1 AND b.user_id=$2 AND ($4::timestamptz IS NULL OR (b.created_at,b.id)<($4,$5::uuid)) ORDER BY b.created_at DESC,b.id DESC LIMIT $6`, a.OrgID, a.UserID, a.SessionID, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v MiniappBinding
		if err = rows.Scan(&v.ID, &v.Platform, &v.AppID, &v.Version, &v.CreatedAt, &v.RevokedAt, &v.Current); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		next := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}
func (s *Service) RevokeMiniapp(ctx context.Context, c core.Command, id string, in MiniappRevokeInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if err := core.Name(strings.TrimSpace(in.Reason)); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return miniappSelfAuthority(ctx, tx, c.Actor) }, func(tx pgx.Tx) (core.Result, error) {
		var out MiniappBinding
		err := tx.QueryRow(ctx, `SELECT id,platform,app_id,version,created_at,revoked_at FROM miniapp_bindings WHERE id=$1 AND org_id=$2 AND user_id=$3 FOR UPDATE`, id, c.Actor.OrgID, c.Actor.UserID).Scan(&out.ID, &out.Platform, &out.AppID, &out.Version, &out.CreatedAt, &out.RevokedAt)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if out.Version != in.ExpectedVersion || out.RevokedAt != nil {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "绑定资料已变更，请刷新列表")
		}
		err = tx.QueryRow(ctx, `UPDATE miniapp_bindings SET revoked_at=now(),version=version+1 WHERE id=$1 RETURNING version,revoked_at`, id).Scan(&out.Version, &out.RevokedAt)
		if err != nil {
			return core.Result{}, err
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND miniapp_binding_id=$2)`, c.Actor.SessionID, id).Scan(&out.Current); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND miniapp_binding_id=$2`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=coalesce(consumed_at,now()) WHERE org_id=$1 AND miniapp_binding_id=$2`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: out, Event: core.Event{Type: "auth.miniapp_revoked", ResourceType: "miniapp_binding", ResourceID: id, Version: out.Version, Payload: map[string]any{"platform": out.Platform, "reason": in.Reason}}}, nil
	})
}

func (s *Service) MiniappLogin(ctx context.Context, in MiniappCodeInput, ip, requestID string) (LoginResult, error) {
	var out LoginResult
	if _, err := s.miniappProfile(in); err != nil {
		return out, err
	}
	if err := s.rate(ctx, "miniapp-login:"+in.Platform+":"+ip, 30); err != nil {
		return out, err
	}
	subject, err := s.miniappExchange(ctx, in)
	if err != nil {
		return out, err
	}
	var id, org, user string
	if err = s.DB.QueryRow(ctx, `SELECT id,org_id,user_id FROM miniapp_bindings WHERE platform=$1 AND app_id=$2 AND subject_hash=$3 AND revoked_at IS NULL`, in.Platform, in.AppID, subject).Scan(&id, &org, &user); err != nil {
		return out, miniappLoginDenied()
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, org); err != nil {
		return out, err
	}
	var version int64
	var status string
	if err = tx.QueryRow(ctx, `SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, user).Scan(&version, &status); err != nil || status != "active" {
		return out, miniappLoginDenied()
	}
	if err = s.checkMiniappBinding(ctx, tx, id, org, user, in.Platform); err != nil {
		return out, err
	}
	var requiresMFA, enrolled bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM role_bindings rb JOIN roles r ON r.org_id=rb.org_id AND r.id=rb.role_id WHERE rb.org_id=$1 AND rb.user_id=$2 AND r.requires_mfa AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())),EXISTS(SELECT 1 FROM mfa_credentials WHERE org_id=$1 AND user_id=$2)`, org, user).Scan(&requiresMFA, &enrolled)
	if err != nil {
		return out, err
	}
	// Once enrolled, platform login always requires MFA, even if a role later
	// stops requiring it. Platform identity never downgrades an existing factor.
	if requiresMFA || enrolled {
		purpose := "mfa"
		if !enrolled {
			purpose = "mfa_enroll"
		}
		out = LoginResult{Step: purpose, Challenge: Token(), Device: in.Platform}
		_, err = tx.Exec(ctx, `INSERT INTO auth_challenges(id,org_id,user_id,token_hash,purpose,device_type,expires_at,auth_version,miniapp_binding_id) VALUES($1,$2,$3,$4,$5,$6,now()+interval '5 minutes',$7,$8)`, core.NewID(), org, user, core.Digest([]byte(out.Challenge)), purpose, in.Platform, version, id)
	} else {
		out, err = s.issue(ctx, tx, org, user, in.Platform, core.NewID(), version, nil)
		if err == nil {
			err = linkMiniappSession(ctx, tx, out.AccessToken, id)
		}
	}
	if err != nil {
		return LoginResult{}, err
	}
	if err = authAudit(ctx, tx, org, user, requestID, "auth.miniapp_login", "succeeded", in.Platform); err != nil {
		return LoginResult{}, err
	}
	return out, tx.Commit(ctx)
}
func miniappLoginDenied() error {
	return core.Fail(401, "MINIAPP_LOGIN_DENIED", "无法使用此平台身份登录，请使用内部账号登录并检查本人绑定")
}
func (s *Service) checkMiniappBinding(ctx context.Context, db platform.DBTX, id, org, user, device string) error {
	var name, appID string
	if err := db.QueryRow(ctx, `SELECT platform,app_id FROM miniapp_bindings WHERE id=$1 AND org_id=$2 AND user_id=$3 AND revoked_at IS NULL`, id, org, user).Scan(&name, &appID); err != nil {
		return miniappLoginDenied()
	}
	p, ok := s.Config.Miniapp.Profile(name)
	if !ok || p.AppID != appID || device != name {
		return miniappLoginDenied()
	}
	return nil
}
func (s *Service) miniappChallengeSource(ctx context.Context, tx pgx.Tx, challenge, org, user, device string) (string, error) {
	var id *string
	if err := tx.QueryRow(ctx, `SELECT miniapp_binding_id FROM auth_challenges WHERE id=$1`, challenge).Scan(&id); err != nil {
		return "", err
	}
	if id == nil {
		return "", nil
	}
	return *id, s.checkMiniappBinding(ctx, tx, *id, org, user, device)
}
func linkMiniappSession(ctx context.Context, tx pgx.Tx, access, id string) error {
	if id == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE sessions SET miniapp_binding_id=$2 WHERE access_hash=$1`, core.Digest([]byte(access)), id)
	return err
}

func (s *Service) configuredMiniappID(name string) string {
	if p, ok := s.Config.Miniapp.Profile(name); ok {
		return p.AppID
	}
	return ""
}
