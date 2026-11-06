package identity

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type InvitationReissueInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	InvitationID    string `json:"invitation_id"`
	Reason          string `json:"reason"`
}

// Reissue keeps the original invited user. It never resets an active account,
// grants a role, or extends/reuses an old bearer token.
func (s *Service) ReissueInvitation(ctx context.Context, c core.Command, id string, in InvitationReissueInput) (core.Result, error) {
	if !core.ValidID(id) || !core.ValidID(in.InvitationID) {
		return core.Result{}, core.Missing()
	}
	if err := core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error {
		if err := RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
			return err
		}
		return authz.RequireOrganization(ctx, tx, c.Actor, "user.invite")
	}, func(tx pgx.Tx) (core.Result, error) {
		u, err := user(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if u.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if u.Status != "invited" {
			return core.Result{}, core.UnexpectedState(u.Status, "reissue_invitation")
		}
		var previous string
		err = tx.QueryRow(ctx, `SELECT id FROM invitations i WHERE org_id=$1 AND user_id=$2 AND NOT EXISTS (SELECT 1 FROM invitations n WHERE n.org_id=i.org_id AND n.replaces_id=i.id)
            ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, c.Actor.OrgID, id).Scan(&previous)
		if err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if previous != in.InvitationID {
			return core.Result{}, core.Conflict()
		}
		invitation := core.NewID()
		expires := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
		if _, err = tx.Exec(ctx, `UPDATE invitations SET revoked_at=clock_timestamp(),revoked_by=$3
            WHERE org_id=$1 AND user_id=$2 AND consumed_at IS NULL AND revoked_at IS NULL`, c.Actor.OrgID, id, c.Actor.UserID); err != nil {
			return core.Result{}, err
		}
		token := s.invitationToken(c.Actor.OrgID, invitation, id, expires)
		if _, err = tx.Exec(ctx, `INSERT INTO invitations(id,org_id,user_id,target_login,token_hash,expires_at,inviter_id,replaces_id,reissue_reason)
            SELECT $1,org_id,id,login_key,$4,$5,$6,$7,$8 FROM users WHERE org_id=$2 AND id=$3`,
			invitation, c.Actor.OrgID, id, core.Digest([]byte(token)), expires, c.Actor.UserID, previous, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		u, err = user(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: InvitationReceipt{User: u, ID: invitation, ExpiresAt: expires, ReplacesID: &previous},
			Event: core.Event{Type: "user.invitation_reissued", ResourceType: "user", ResourceID: id, Version: u.Version,
				Payload: map[string]any{"invitation_id": invitation, "replaces_id": previous, "reason": in.Reason}}}, nil
	})
}

// Both the first issue and reissue decorate only after a fresh locked check.
// Secrets never enter command persistence, audit, outbox, or pagination results.
func (s *Service) readInvitation(ctx context.Context, a core.Actor, userID, invitationID string) (InvitationReceipt, error) {
	var out InvitationReceipt
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockOrganization(ctx, tx, a.OrgID); err != nil {
		return out, err
	}
	if err = RequireInteractiveSession(ctx, tx, a, true); err != nil {
		return out, err
	}
	if err = authz.RequireOrganization(ctx, tx, a, "user.invite"); err != nil {
		return out, err
	}
	out.User, err = user(ctx, tx, a.OrgID, userID)
	if err != nil {
		return out, err
	}
	var hash string
	var active bool
	err = tx.QueryRow(ctx, `SELECT i.id,i.expires_at,i.replaces_id,i.revoked_at,i.consumed_at,i.token_hash,
        i.consumed_at IS NULL AND i.revoked_at IS NULL AND i.expires_at>clock_timestamp() AND u.status='invited'
        FROM invitations i JOIN users u ON u.org_id=i.org_id AND u.id=i.user_id
        WHERE i.org_id=$1 AND i.user_id=$2 AND (($3::uuid IS NULL AND NOT EXISTS (SELECT 1 FROM invitations n WHERE n.org_id=i.org_id AND n.replaces_id=i.id)) OR i.id=$3)
        ORDER BY i.created_at DESC,i.id DESC LIMIT 1 FOR SHARE OF i`, a.OrgID, userID, nullableInvitationID(invitationID)).
		Scan(&out.ID, &out.ExpiresAt, &out.ReplacesID, &out.RevokedAt, &out.ConsumedAt, &hash, &active)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if active {
		token := s.invitationToken(a.OrgID, out.ID, userID, out.ExpiresAt)
		if core.Digest([]byte(token)) == hash {
			out.Token = token
		}
	}
	return out, tx.Commit(ctx)
}

func nullableInvitationID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
