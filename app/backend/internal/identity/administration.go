package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type User struct {
	ID          string    `json:"id"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
}

func user(ctx context.Context, db platform.DBTX, org, id string) (User, error) {
	var value User
	err := db.QueryRow(ctx, "SELECT id,version,created_at,updated_at,display_name,status FROM users WHERE org_id=$1 AND id=$2", org, id).Scan(&value.ID, &value.Version, &value.CreatedAt, &value.UpdatedAt, &value.DisplayName, &value.Status)
	return value, platform.DatabaseError(err)
}

type InvitationReceipt struct {
	User      User      `json:"user"`
	ID        string    `json:"invitation_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"activation_token,omitempty"`
}
type InviteInput struct {
	Login       string `json:"login_key"`
	DisplayName string `json:"display_name"`
}

func recentMFA(a core.Actor) error {
	if a.MFAVerifiedAt == nil || time.Since(*a.MFAVerifiedAt) > 10*time.Minute {
		return core.Fail(403, "REAUTHENTICATION_REQUIRED", "此操作需要最近10分钟内完成两步验证，请重新登录")
	}
	return nil
}
func (s *Service) Users(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[User], error) {
	out := core.Page[User]{Items: []User{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "user.read"); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.Digest([]byte(a.OrgID + a.UserID + fmt.Sprint(a.OrgAuthVersion) + "/users"))
	pager := platform.Pager{Key: s.Config.CursorKey}
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		c, err := pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before, beforeID = &c.UpdatedAt, &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,version,created_at,updated_at,display_name,status FROM users WHERE org_id=$1
        AND ($2::timestamptz IS NULL OR (updated_at,id)<($2,$3::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $4`, a.OrgID, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var value User
		if err = rows.Scan(&value.ID, &value.Version, &value.CreatedAt, &value.UpdatedAt, &value.DisplayName, &value.Status); err != nil {
			return out, err
		}
		out.Items = append(out.Items, value)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		c := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}
func (s *Service) invitationToken(org, id, user string, expires time.Time) string {
	domain := hmac.New(sha256.New, s.Config.CursorKey)
	domain.Write([]byte("copypaste/invitation/key/v1"))
	mac := hmac.New(sha256.New, domain.Sum(nil))
	fmt.Fprintf(mac, "%s\n%s\n%s\n%d", org, id, user, expires.Unix())
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) Invite(ctx context.Context, c core.Command, input InviteInput) (core.Result, error) {
	input.Login = strings.ToLower(strings.TrimSpace(input.Login))
	if err := core.Name(input.Login); err != nil {
		return core.Result{}, err
	}
	if err := core.Name(input.DisplayName); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := recentMFA(c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "user.invite")
	}, func(tx pgx.Tx) (core.Result, error) {
		id, invitation := core.NewID(), core.NewID()
		expires := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
		// Invited accounts cannot authenticate. The placeholder is a modern
		// hash and activation always replaces it with a user-chosen credential.
		if _, err := tx.Exec(ctx, "INSERT INTO users(id,org_id,login_key,display_name,status,credential_hash) VALUES($1,$2,$3,$4,'invited',$5)", id, c.Actor.OrgID, input.Login, input.DisplayName, s.DummyHash); err != nil {
			return core.Result{}, err
		}
		token := s.invitationToken(c.Actor.OrgID, invitation, id, expires)
		if _, err := tx.Exec(ctx, "INSERT INTO invitations(id,org_id,user_id,target_login,token_hash,expires_at,inviter_id) VALUES($1,$2,$3,$4,$5,$6,$7)", invitation, c.Actor.OrgID, id, input.Login, core.Digest([]byte(token)), expires, c.Actor.UserID); err != nil {
			return core.Result{}, err
		}
		u, err := user(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		// The command receipt never stores the plaintext activation token.
		return core.Result{Status: 201, Data: InvitationReceipt{User: u, ID: invitation, ExpiresAt: expires}, Event: core.Event{Type: "user.invited", ResourceType: "user", ResourceID: id, Version: u.Version, Payload: map[string]any{"invitation_id": invitation}}}, nil
	})
}
func (s *Service) DecorateInvitation(ctx context.Context, a core.Actor, receipt InvitationReceipt) (InvitationReceipt, error) {
	if err := recentMFA(a); err != nil {
		return receipt, err
	}
	if err := authz.RequireOrganization(ctx, s.DB, a, "user.invite"); err != nil {
		return receipt, err
	}
	var hash string
	var active bool
	err := s.DB.QueryRow(ctx, `SELECT i.token_hash,i.consumed_at IS NULL AND i.expires_at>now() AND u.status='invited'
        FROM invitations i JOIN users u ON u.org_id=i.org_id AND u.id=i.user_id WHERE i.org_id=$1 AND i.id=$2 AND i.user_id=$3`, a.OrgID, receipt.ID, receipt.User.ID).Scan(&hash, &active)
	if err != nil {
		return receipt, platform.DatabaseError(err)
	}
	if active {
		token := s.invitationToken(a.OrgID, receipt.ID, receipt.User.ID, receipt.ExpiresAt)
		if core.Digest([]byte(token)) == hash {
			receipt.Token = token
		}
	}
	return receipt, nil
}

type ActivateInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Service) Invitation(ctx context.Context, a core.Actor, id string) (InvitationReceipt, error) {
	var receipt InvitationReceipt
	if !core.ValidID(id) {
		return receipt, core.Missing()
	}
	if err := recentMFA(a); err != nil {
		return receipt, err
	}
	if err := authz.RequireOrganization(ctx, s.DB, a, "user.invite"); err != nil {
		return receipt, err
	}
	u, err := user(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return receipt, err
	}
	receipt.User = u
	err = s.DB.QueryRow(ctx, `SELECT id,expires_at FROM invitations WHERE org_id=$1 AND user_id=$2
        AND consumed_at IS NULL AND expires_at>now() ORDER BY created_at DESC,id DESC LIMIT 1`, a.OrgID, id).Scan(&receipt.ID, &receipt.ExpiresAt)
	if err != nil {
		return receipt, platform.DatabaseError(err)
	}
	return s.DecorateInvitation(ctx, a, receipt)
}

func (s *Service) Activate(ctx context.Context, input ActivateInput, ip, requestID string) error {
	if err := s.rate(ctx, "activate-ip:"+ip, 10); err != nil {
		return err
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return core.Invalid("password", "密码需为12个字符以上，且不超过1024字节")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	invalid := core.Fail(401, "INVITATION_INVALID", "邀请无效或已过期，请联系管理员")
	var org string
	if err = tx.QueryRow(ctx, "SELECT org_id FROM invitations WHERE token_hash=$1", core.Digest([]byte(input.Token))).Scan(&org); err != nil {
		return invalid
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); err != nil {
		return err
	}
	var invitation, id string
	err = tx.QueryRow(ctx, `SELECT i.id,i.user_id FROM invitations i JOIN users u ON u.org_id=i.org_id AND u.id=i.user_id
        WHERE i.org_id=$1 AND i.token_hash=$2 AND i.consumed_at IS NULL AND i.expires_at>now() AND u.status='invited' FOR UPDATE OF i,u`, org, core.Digest([]byte(input.Token))).Scan(&invitation, &id)
	if err != nil {
		return invalid
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET status='active',credential_hash=$3,auth_version=auth_version+1,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", org, id, hash); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE invitations SET consumed_at=now() WHERE id=$1", invitation); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", org); err != nil {
		return err
	}
	if err = authAudit(ctx, tx, org, id, requestID, "user.activated", "succeeded", "web"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
        SELECT $1,org_id,'user.activated','user',id,version,'{}',$2 FROM users WHERE org_id=$3 AND id=$4`, core.NewID(), requestID, org, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type DisableInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}

func (s *Service) Disable(ctx context.Context, c core.Command, id string, input DisableInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if input.Action != "disable" {
		return core.Result{}, core.Invalid("action", "用户停用操作无效")
	}
	if id == c.Actor.UserID {
		return core.Result{}, core.Fail(422, "SELF_DISABLE_FORBIDDEN", "不能在此操作中停用当前账号")
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := recentMFA(c.Actor); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "user.disable")
	}, func(tx pgx.Tx) (core.Result, error) {
		if _, err := tx.Exec(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		u, err := user(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if u.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if u.Status == "disabled" {
			return core.Result{}, core.UnexpectedState(u.Status, input.Action)
		}
		var remaining int
		if err = tx.QueryRow(ctx, `SELECT count(DISTINCT u.id) FROM users u JOIN role_bindings rb ON rb.org_id=u.org_id AND rb.user_id=u.id
            JOIN roles r ON r.org_id=rb.org_id AND r.id=rb.role_id WHERE u.org_id=$1 AND u.id<>$2 AND u.status='active'
            AND r.key='system_admin' AND rb.revoked_at IS NULL AND rb.revoked_at IS NULL AND rb.valid_from<=now() AND (rb.valid_until IS NULL OR rb.valid_until>now())`, c.Actor.OrgID, id).Scan(&remaining); err != nil {
			return core.Result{}, err
		}
		if remaining == 0 {
			return core.Result{}, core.Fail(422, "LAST_ADMIN_REQUIRED", "至少需要保留一位有效的系统管理员")
		}
		if _, err = tx.Exec(ctx, "UPDATE users SET status='disabled',disabled_at=now(),auth_version=auth_version+1,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE org_id=$1 AND user_id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE invitations SET consumed_at=coalesce(consumed_at,now()) WHERE org_id=$1 AND user_id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		u, err = user(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: u, Event: core.Event{Type: "user.disabled", ResourceType: "user", ResourceID: id, Version: u.Version, Payload: map[string]any{"reason": input.Reason}}}, nil
	})
}
