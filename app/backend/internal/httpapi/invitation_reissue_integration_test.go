package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func TestInvitationReissue(t *testing.T) {
	f := setup(t)
	token := f.administratorToken()
	ctx := context.Background()
	rec, result := f.request("POST", "/v1/users", token, core.NewID(), map[string]string{"login_key": "same.invited.person", "display_name": "待激活成员"}, nil)
	expect(t, rec, 201, result)
	initial := data[identity.InvitationReceipt](t, result)
	if _, err := f.owner.Exec(ctx, "UPDATE invitations SET expires_at=now()-interval '1 minute' WHERE id=$1", initial.ID); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("GET", "/v1/users/"+initial.User.ID+"/invitation", token, "", nil, nil)
	expect(t, rec, 200, result)
	current := data[identity.InvitationReceipt](t, result)
	if current.Token != "" || current.ID != initial.ID {
		t.Fatal("expired invitation should be reviewable without a token")
	}
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			f := f.with(t)
			// Synthetic native fixture uses each real middleware channel. No device.
			if _, err := f.owner.Exec(ctx, "UPDATE sessions SET device_type=$2 WHERE access_hash=$1", core.Digest([]byte(token)), channel); err != nil {
				t.Fatal(err)
			}
			requestToken := token
			var headers map[string]string
			if channel == "web" {
				csrf := strings.Repeat("f", 43)
				if _, err := f.owner.Exec(ctx, "UPDATE sessions SET csrf_hash=$2 WHERE access_hash=$1", core.Digest([]byte(token)), core.Digest([]byte(csrf))); err != nil {
					t.Fatal(err)
				}
				headers = map[string]string{"Cookie": "cp_session=" + token, "Origin": "http://localhost:5178", "X-CSRF-Token": csrf}
				requestToken = ""
			}
			body := map[string]any{"expected_version": current.User.Version, "invitation_id": current.ID, "reason": "重新交付过期邀请"}
			key := core.NewID()
			rec, result := f.request("POST", "/v1/users/"+current.User.ID+"/invitation-reissues", requestToken, key, body, headers)
			expect(t, rec, 201, result)
			next := data[identity.InvitationReceipt](t, result)
			if next.User.ID != initial.User.ID || next.User.Version != current.User.Version+1 || next.ID == current.ID || next.ReplacesID == nil || *next.ReplacesID != current.ID || len(next.Token) != 43 {
				t.Fatal("reissue did not preserve user and replace exactly the reviewed invitation")
			}
			rec, result = f.request("POST", "/v1/users/"+current.User.ID+"/invitation-reissues", requestToken, key, body, headers)
			expect(t, rec, 201, result)
			if replay := data[identity.InvitationReceipt](t, result); replay.ID != next.ID || replay.Token != next.Token {
				t.Fatal("reissue replay created or changed credentials")
			}
			rec, result = f.request("POST", "/v1/users/"+current.User.ID+"/invitation-reissues", requestToken, core.NewID(), body, headers)
			expect(t, rec, 409, result)
			oldToken := current.Token
			if oldToken == "" {
				oldToken = initial.Token
			}
			rec, result = f.request("POST", "/v1/auth/activate", "", "", map[string]string{"token": oldToken, "password": "unused-original-secret-long"}, nil)
			expect(t, rec, 401, result)
			var stored string
			if err := f.owner.QueryRow(ctx, "SELECT response_data::text FROM idempotency_records WHERE idempotency_key=$1", key).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stored, next.Token) || strings.Contains(stored, "activation_token") {
				t.Fatal("plaintext leaked into permanent receipt")
			}
			current = next
		})
	}
	rec, result = f.request("POST", "/v1/auth/activate", "", "", map[string]string{"token": current.Token, "password": "accepted-invitation-secret-long"}, nil)
	expect(t, rec, 200, result)
	rec, result = f.request("POST", "/v1/users/"+current.User.ID+"/invitation-reissues", token, core.NewID(), map[string]any{"expected_version": current.User.Version, "invitation_id": current.ID, "reason": "拒绝重置有效账号"}, nil)
	if rec.Code == 201 {
		t.Fatal("active account was reset by reissue")
	}
	var users, invitations int
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM users WHERE org_id=$1 AND login_key='same.invited.person'", f.org).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE org_id=$1 AND user_id=$2", f.org, initial.User.ID).Scan(&invitations); err != nil {
		t.Fatal(err)
	}
	if users != 1 || invitations != 5 {
		t.Fatal("identity or invitation history duplicated")
	}
}

func TestInvitationOriginalSession(t *testing.T) {
	f := setup(t)
	token := f.administratorToken()
	ctx := context.Background()
	rec, result := f.request("POST", "/v1/users", token, core.NewID(), map[string]string{"login_key": "session.bound.invite", "display_name": "会话复核"}, nil)
	expect(t, rec, 201, result)
	receipt := data[identity.InvitationReceipt](t, result)
	actor, err := f.api.Identity.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("POST", "/v1/auth/logout", token, core.NewID(), map[string]any{}, nil)
	expect(t, rec, 200, result)
	input := identity.InviteInput{Login: "must.not.exist", DisplayName: "已撤销会话"}
	body, _ := json.Marshal(input)
	command := core.Command{Actor: actor, RequestID: core.NewID(), Route: "POST /v1/users", Key: core.NewID(), Body: body}
	if _, err = f.api.Identity.Invite(ctx, command, input); err == nil {
		t.Fatal("revoked original session created an account")
	}
	if _, err = f.api.Identity.DecorateInvitation(ctx, actor, receipt); err == nil {
		t.Fatal("revoked original session recovered plaintext")
	}
	reissue := identity.InvitationReissueInput{ExpectedVersion: receipt.User.Version, InvitationID: receipt.ID, Reason: "stopped"}
	command.Body, _ = json.Marshal(reissue)
	command.Key = core.NewID()
	command.Route = "POST /v1/users/" + receipt.User.ID + "/invitation-reissues"
	if _, err = f.api.Identity.ReissueInvitation(ctx, command, receipt.User.ID, reissue); err == nil {
		t.Fatal("revoked original session reissued invitation")
	}
	var count int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE org_id=$1 AND user_id=$2", f.org, receipt.User.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected commands changed invitation history")
	}
}
