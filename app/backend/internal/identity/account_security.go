package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// A device is a refresh family, not an individual rotating access credential.
// No raw credentials, user-agent claims, or inferred locations are returned.
type DeviceSession struct {
	ID               string     `json:"id"`
	Device           string     `json:"device_type"`
	StartedAt        time.Time  `json:"started_at"`
	RotatedAt        time.Time  `json:"rotated_at"`
	AccessExpiresAt  time.Time  `json:"access_expires_at"`
	RefreshExpiresAt time.Time  `json:"refresh_expires_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	Status           string     `json:"status"`
	Current          bool       `json:"is_current"`
}
type SessionRevocation struct {
	ID        string    `json:"id"`
	RevokedAt time.Time `json:"revoked_at"`
}
type SessionRevokeInput struct {
	Reason string `json:"reason"`
}

func (s *Service) CurrentFamily(ctx context.Context, a core.Actor) (string, error) {
	var id string
	err := s.DB.QueryRow(ctx, `SELECT family_id FROM sessions WHERE org_id=$1 AND user_id=$2 AND id=$3`, a.OrgID, a.UserID, a.SessionID).Scan(&id)
	return id, platform.DatabaseError(err)
}

func (s *Service) Sessions(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[DeviceSession], error) {
	out := core.Page[DeviceSession]{Items: []DeviceSession{}}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, a.OrgID); err != nil {
		return out, err
	}
	if err = RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	var family string
	if err = tx.QueryRow(ctx, `SELECT family_id FROM sessions WHERE org_id=$1 AND user_id=$2 AND id=$3`, a.OrgID, a.UserID, a.SessionID).Scan(&family); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := core.Digest([]byte(fmt.Sprintf("%s/%s/%d/sessions", a.OrgID, a.UserID, a.AuthVersion)))
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, beforeID = &c.UpdatedAt, &c.ID
	}
	rows, err := tx.Query(ctx, `WITH families AS (
        SELECT DISTINCT ON(family_id) family_id,device_type,
            min(created_at) OVER(PARTITION BY family_id) AS started_at,created_at AS rotated_at,
            expires_at,refresh_expires_at,revoked_at,auth_version
        FROM sessions WHERE org_id=$1 AND user_id=$2
        ORDER BY family_id,created_at DESC,id DESC
    ) SELECT family_id,device_type,started_at,rotated_at,expires_at,refresh_expires_at,revoked_at,
        CASE WHEN revoked_at IS NOT NULL THEN 'revoked'
             WHEN refresh_expires_at<=clock_timestamp() OR auth_version<>$6 THEN 'expired' ELSE 'active' END,
        family_id=$3::uuid
        FROM families WHERE ($4::timestamptz IS NULL OR (started_at,family_id)<($4,$5::uuid))
        ORDER BY started_at DESC,family_id DESC LIMIT $7`, a.OrgID, a.UserID, family, before, beforeID, a.AuthVersion, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v DeviceSession
		if err = rows.Scan(&v.ID, &v.Device, &v.StartedAt, &v.RotatedAt, &v.AccessExpiresAt, &v.RefreshExpiresAt, &v.RevokedAt, &v.Status, &v.Current); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.StartedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, tx.Commit(ctx)
}

func (s *Service) RevokeSession(ctx context.Context, c core.Command, id string, in SessionRevokeInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if err := core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	// Refresh and credential mutations take the organization lock before users
	// and sessions. This makes revocation atomic against a racing rotation.
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return RequireInteractiveSession(ctx, tx, c.Actor, false) }, func(tx pgx.Tx) (core.Result, error) {
		var at *time.Time
		err := tx.QueryRow(ctx, `SELECT revoked_at FROM sessions WHERE org_id=$1 AND user_id=$2 AND family_id=$3 ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, c.Actor.OrgID, c.Actor.UserID, id).Scan(&at)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if at == nil {
			now := time.Now().UTC()
			at = &now
		}
		if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=coalesce(revoked_at,$4) WHERE org_id=$1 AND user_id=$2 AND family_id=$3`, c.Actor.OrgID, c.Actor.UserID, id, *at); err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: SessionRevocation{ID: id, RevokedAt: *at}, Event: core.Event{Type: "auth.session_revoked", ResourceType: "session_family", ResourceID: id, Version: 1, Payload: map[string]any{"reason": in.Reason}}}, nil
	})
}

type PasswordChangeInput struct {
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
}
type PasswordChanged struct {
	Changed bool `json:"changed"`
}

func (s *Service) ChangePassword(ctx context.Context, c core.Command, in PasswordChangeInput) (core.Result, error) {
	if err := s.rate(ctx, "password-change:"+c.Actor.OrgID+":"+c.Actor.UserID, 10); err != nil {
		return core.Result{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return core.Result{}, core.Invalid("password", "密码需为12个字符以上，且不超过1024字节")
	}
	// The generic command fingerprint is deliberately fast. Credential-bearing
	// inputs need a keyed domain first, so a database-only copy of the receipt
	// cannot become a fast offline verifier for guesses of the password pair.
	credentialBody, err := json.Marshal(in)
	if err != nil {
		return core.Result{}, err
	}
	intent := hmac.New(sha256.New, s.Config.CursorKey)
	intent.Write([]byte("copypaste/password-change/intent/v1\x00"))
	intent.Write(credentialBody)
	c.Body, err = json.Marshal(map[string]string{"credential_intent": base64.RawURLEncoding.EncodeToString(intent.Sum(nil))})
	if err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		var enrolled bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mfa_credentials WHERE org_id=$1 AND user_id=$2)`, c.Actor.OrgID, c.Actor.UserID).Scan(&enrolled); err != nil {
			return err
		}
		return RequireInteractiveSession(ctx, tx, c.Actor, enrolled)
	}, func(tx pgx.Tx) (core.Result, error) {
		var current string
		if err := tx.QueryRow(ctx, `SELECT credential_hash FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE`, c.Actor.OrgID, c.Actor.UserID).Scan(&current); err != nil {
			return core.Result{}, err
		}
		if !CheckPassword(current, in.CurrentPassword) {
			return core.Result{}, core.Fail(403, "CURRENT_PASSWORD_INVALID", "当前密码不正确，请核对后重试")
		}
		if CheckPassword(current, in.Password) {
			return core.Result{}, core.Invalid("password", "新密码不能与当前密码相同")
		}
		version, err := replacePassword(ctx, tx, c.Actor.OrgID, c.Actor.UserID, hash, "")
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: PasswordChanged{Changed: true}, Event: core.Event{Type: "auth.password_changed", ResourceType: "user", ResourceID: c.Actor.UserID, Version: version, Payload: map[string]any{"sessions_revoked": true}}}, nil
	})
}

type PasswordResetInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type PasswordResetReceipt struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"reset_token,omitempty"`
}

func (s *Service) resetToken(org, id, user string, version int64, expires time.Time) string {
	domain := hmac.New(sha256.New, s.Config.CursorKey)
	domain.Write([]byte("copypaste/password-reset/key/v1"))
	mac := hmac.New(sha256.New, domain.Sum(nil))
	fmt.Fprintf(mac, "%s\n%s\n%s\n%d\n%d", org, id, user, version, expires.Unix())
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) IssuePasswordReset(ctx context.Context, c core.Command, id string, in PasswordResetInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if err := core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "user.password_reset")
	}, func(tx pgx.Tx) (core.Result, error) {
		var version, authVersion int64
		var status string
		err := tx.QueryRow(ctx, `SELECT version,auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE`, c.Actor.OrgID, id).Scan(&version, &authVersion, &status)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "成员资料已更新，请刷新后重试")
		}
		if status != "active" {
			return core.Result{}, core.Fail(409, "ACCOUNT_NOT_ACTIVE", "仅可为有效账号签发密码重置凭证")
		}
		if _, err = tx.Exec(ctx, `UPDATE password_resets SET revoked_at=now() WHERE org_id=$1 AND user_id=$2 AND consumed_at IS NULL AND revoked_at IS NULL`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		receipt := PasswordResetReceipt{ID: core.NewID(), UserID: id, ExpiresAt: time.Now().UTC().Truncate(time.Second).Add(15 * time.Minute)}
		token := s.resetToken(c.Actor.OrgID, receipt.ID, id, authVersion, receipt.ExpiresAt)
		_, err = tx.Exec(ctx, `INSERT INTO password_resets(id,org_id,user_id,issuer_id,token_hash,auth_version,reason,expires_at,issuer_auth_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, receipt.ID, c.Actor.OrgID, id, c.Actor.UserID, core.Digest([]byte(token)), authVersion, in.Reason, receipt.ExpiresAt, c.Actor.AuthVersion)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: receipt, Event: core.Event{Type: "auth.password_reset_issued", ResourceType: "user", ResourceID: id, Version: version, Payload: map[string]any{"reset_id": receipt.ID, "reason": in.Reason}}}, nil
	})
}

// Derive only for the still-authorized caller and a live receipt. Command and
// audit persistence never contain the reset bearer secret, including on replay.
func (s *Service) DecoratePasswordReset(ctx context.Context, a core.Actor, result core.Result) (core.Result, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, a.OrgID); err != nil {
		return result, err
	}
	if err = RequireInteractiveSession(ctx, tx, a, true); err != nil {
		return result, err
	}
	if err = authz.RequireOrganization(ctx, tx, a, "user.password_reset"); err != nil {
		return result, err
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return result, err
	}
	var receipt PasswordResetReceipt
	if err = json.Unmarshal(raw, &receipt); err != nil {
		return result, err
	}
	var hash string
	var version int64
	err = tx.QueryRow(ctx, `SELECT p.token_hash,p.auth_version FROM password_resets p JOIN users u ON u.org_id=p.org_id AND u.id=p.user_id
        WHERE p.org_id=$1 AND p.id=$2 AND p.user_id=$3 AND p.issuer_id=$4 AND p.consumed_at IS NULL AND p.revoked_at IS NULL
        AND p.expires_at>clock_timestamp() AND u.status='active' AND u.auth_version=p.auth_version FOR SHARE OF p,u`, a.OrgID, receipt.ID, receipt.UserID, a.UserID).Scan(&hash, &version)
	if err != nil && err != pgx.ErrNoRows {
		return result, err
	}
	if err == nil {
		token := s.resetToken(a.OrgID, receipt.ID, receipt.UserID, version, receipt.ExpiresAt)
		if core.Digest([]byte(token)) == hash {
			receipt.Token = token
		}
	}
	result.Data = receipt
	return result, tx.Commit(ctx)
}

type CompletePasswordResetInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Service) CompletePasswordReset(ctx context.Context, in CompletePasswordResetInput, ip, requestID string) error {
	if err := s.rate(ctx, "password-reset-ip:"+ip, 10); err != nil {
		return err
	}
	if err := s.rate(ctx, "password-reset-token:"+core.Digest([]byte(in.Token)), 5); err != nil {
		return err
	}
	invalid := core.Fail(401, "PASSWORD_RESET_INVALID", "重置凭证无效或已过期，请联系管理员重新签发")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, `SELECT org_id FROM password_resets WHERE token_hash=$1`, core.Digest([]byte(in.Token))).Scan(&org); err != nil {
		if err != pgx.ErrNoRows {
			return err
		}
		if err = passwordResetDenied(ctx, s.DB, requestID); err != nil {
			return err
		}
		return invalid
	}
	// Exclusive organization lock shares the command lock order and prevents
	// issuing, disabling, refreshing, or consuming this credential concurrently.
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org); err != nil {
		return err
	}
	var id, user string
	err = tx.QueryRow(ctx, `SELECT p.id,p.user_id FROM password_resets p JOIN users u ON u.org_id=p.org_id AND u.id=p.user_id
        JOIN users issuer ON issuer.org_id=p.org_id AND issuer.id=p.issuer_id
        WHERE p.org_id=$1 AND p.token_hash=$2 AND p.consumed_at IS NULL AND p.revoked_at IS NULL
		AND p.expires_at>clock_timestamp() AND p.auth_version=u.auth_version AND u.status='active'
        AND issuer.status='active' AND issuer.auth_version=p.issuer_auth_version
        AND EXISTS(SELECT 1 FROM role_bindings b JOIN role_actions ra ON ra.org_id=b.org_id AND ra.role_id=b.role_id
            WHERE b.org_id=p.org_id AND b.user_id=p.issuer_id AND b.scope_type='org' AND b.revoked_at IS NULL
            AND b.valid_from<=clock_timestamp() AND (b.valid_until IS NULL OR b.valid_until>clock_timestamp()) AND ra.action='user.password_reset')
        FOR UPDATE OF p,u`, org, core.Digest([]byte(in.Token))).Scan(&id, &user)
	if err != nil {
		if err != pgx.ErrNoRows {
			return err
		}
		// Roll back first: an independent denial audit must not wait for the
		// organization lock held by this failed credential transaction.
		tx.Rollback(ctx)
		if err = passwordResetDenied(ctx, s.DB, requestID); err != nil {
			return err
		}
		return invalid
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return core.Invalid("password", "密码需为12个字符以上，且不超过1024字节")
	}
	var oldHash string
	if err = tx.QueryRow(ctx, `SELECT credential_hash FROM users WHERE org_id=$1 AND id=$2`, org, user).Scan(&oldHash); err != nil {
		return err
	}
	if CheckPassword(oldHash, in.Password) {
		return core.Invalid("password", "新密码不能与当前密码相同")
	}
	version, err := replacePassword(ctx, tx, org, user, hash, id)
	if err != nil {
		return err
	}
	// Possession of a recovery token is not an authenticated employee session.
	// Preserve that distinction in audit instead of attributing this to issuer.
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,resource_id,resource_version,result,reason)
        VALUES($1,$2,'password_reset',$3,'auth.password_reset_completed','user',$4,$5,'succeeded','single-use recovery token')`, core.NewID(), org, requestID, user, version)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
        VALUES($1,$2,'auth.password_reset_completed','user',$3,$4,jsonb_build_object('reset_id',$5::text,'sessions_revoked',true),$6)`, core.NewID(), org, user, version, id, requestID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func passwordResetDenied(ctx context.Context, db platform.DBTX, requestID string) error {
	var org string
	if err := db.QueryRow(ctx, `SELECT id FROM organizations`).Scan(&org); err != nil {
		return err
	}
	return authAudit(ctx, db, org, "", requestID, "auth.password_reset", "denied", "password_reset")
}
func replacePassword(ctx context.Context, tx pgx.Tx, org, user, hash, consumedReset string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `UPDATE users SET credential_hash=$3,auth_version=auth_version+1,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 RETURNING version`, org, user, hash).Scan(&version)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND user_id=$2`, org, user); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=coalesce(consumed_at,now()) WHERE org_id=$1 AND user_id=$2`, org, user); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE password_resets SET consumed_at=CASE WHEN id=NULLIF($3,'')::uuid THEN now() ELSE consumed_at END,
        revoked_at=CASE WHEN id=NULLIF($3,'')::uuid THEN NULL ELSE now() END
        WHERE org_id=$1 AND user_id=$2 AND consumed_at IS NULL AND revoked_at IS NULL`, org, user, consumedReset); err != nil {
		return 0, err
	}
	return version, nil
}
