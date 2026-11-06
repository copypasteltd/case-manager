package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func TestRound168DisableAfterInvitationReissue(t *testing.T) {
	for _, activated := range []bool{false, true} {
		name := "invited"
		if activated {
			name = "activated"
		}
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			token := f.administratorToken()
			rec, result := f.request("POST", "/v1/users", token, core.NewID(), identity.InviteInput{Login: "reissued.disable", DisplayName: "待核对账号"}, nil)
			expect(t, rec, 201, result)
			old := data[identity.InvitationReceipt](t, result)
			rec, result = f.request("POST", "/v1/users/"+old.User.ID+"/invitation-reissues", token, core.NewID(), identity.InvitationReissueInput{ExpectedVersion: old.User.Version, InvitationID: old.ID, Reason: "替换未使用邀请"}, nil)
			expect(t, rec, 201, result)
			current := data[identity.InvitationReceipt](t, result)
			if activated {
				rec, result = f.request("POST", "/v1/auth/activate", "", "", map[string]string{"token": current.Token, "password": "synthetic-activation-for-disable"}, nil)
				expect(t, rec, 200, result)
			}
			var version int64
			if err := f.owner.QueryRow(ctx, "SELECT version FROM users WHERE org_id=$1 AND id=$2", f.org, old.User.ID).Scan(&version); err != nil {
				t.Fatal(err)
			}
			rec, result = f.request("POST", "/v1/users/"+old.User.ID+"/disable", token, core.NewID(), identity.DisableInput{ExpectedVersion: version, Action: "disable", Reason: "人工停用已核对账号"}, nil)
			expect(t, rec, 200, result)
			var correct bool
			if err := f.owner.QueryRow(ctx, `SELECT u.status='disabled' AND prior.revoked_at IS NOT NULL AND prior.consumed_at IS NULL
                AND latest.revoked_at IS NULL AND latest.consumed_at IS NOT NULL
                FROM users u JOIN invitations prior ON prior.org_id=u.org_id AND prior.user_id=u.id AND prior.id=$3
                JOIN invitations latest ON latest.org_id=u.org_id AND latest.id=$4
                WHERE u.org_id=$1 AND u.id=$2`, f.org, old.User.ID, old.ID, current.ID).Scan(&correct); err != nil || !correct {
				t.Fatal("disable did not preserve the historical revocation and invalidate the current invitation", err)
			}
			rec, result = f.request("POST", "/v1/auth/activate", "", "", map[string]string{"token": current.Token, "password": "synthetic-activation-for-disable"}, nil)
			expect(t, rec, 401, result)
		})
	}
}

func TestRound168RoleOriginalSession(t *testing.T) {
	for _, mode := range []string{"logout", "family-revoke"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			token := f.administratorToken()
			var role string
			if err := f.owner.QueryRow(ctx, "SELECT id FROM roles WHERE org_id=$1 AND key='member'", f.org).Scan(&role); err != nil {
				t.Fatal(err)
			}
			input := identity.BindingCreate{UserID: f.bob, RoleID: role, ScopeType: "org", Classification: "internal", Reason: "普通成员授权来源"}
			key := core.NewID()
			rec, result := f.request("POST", "/v1/role-bindings", token, key, input, nil)
			expect(t, rec, 201, result)
			binding := data[identity.RoleBinding](t, result)
			actor, err := f.api.Identity.Authenticate(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM role_bindings WHERE org_id=$1", f.org).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if mode == "logout" {
				rec, result = f.request("POST", "/v1/auth/logout", token, core.NewID(), map[string]any{}, nil)
			} else {
				var family string
				if err = f.owner.QueryRow(ctx, "SELECT family_id FROM sessions WHERE org_id=$1 AND id=$2", f.org, actor.SessionID).Scan(&family); err != nil {
					t.Fatal(err)
				}
				rec, result = f.request("POST", "/v1/me/sessions/"+family+"/revoke", token, core.NewID(), identity.SessionRevokeInput{Reason: "退出当前设备族"}, nil)
			}
			expect(t, rec, 200, result)
			body, _ := json.Marshal(input)
			command := core.Command{Actor: actor, RequestID: core.NewID(), Route: "POST /v1/role-bindings", Key: core.NewID(), Body: body}
			assertRevoked := func(err error) {
				t.Helper()
				var failure *core.Error
				if !errors.As(err, &failure) || failure.Code != "AUTHORIZATION_CHANGED" {
					t.Fatalf("expected original-session rejection, got %v", err)
				}
			}
			_, err = f.api.Identity.CreateBinding(ctx, command, input)
			assertRevoked(err)
			command.Key = key
			_, err = f.api.Identity.CreateBinding(ctx, command, input)
			assertRevoked(err) // An old successful operation is not an authorization bypass.
			revoke := identity.RevokeInput{ExpectedVersion: binding.Version, Reason: "不得用已撤销会话移除授权"}
			command.Body, _ = json.Marshal(revoke)
			command.Key = core.NewID()
			command.Route = "POST /v1/role-bindings/" + binding.ID + "/revoke"
			_, err = f.api.Identity.RevokeBinding(ctx, command, binding.ID, revoke)
			assertRevoked(err)
			var after int
			var unchanged bool
			if err = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM role_bindings WHERE org_id=$1),
                EXISTS(SELECT 1 FROM role_bindings WHERE org_id=$1 AND id=$2 AND revoked_at IS NULL AND version=$3)
                AND EXISTS(SELECT 1 FROM organizations WHERE id=$1 AND authorization_version=$4)`, f.org, binding.ID, binding.Version, actor.OrgAuthVersion).Scan(&after, &unchanged); err != nil || after != count || !unchanged {
				t.Fatal("rejected captured-actor commands changed permissions or organization authority", err)
			}
		})
	}
}
