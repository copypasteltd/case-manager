package httpapi

import (
	"context"
	"os"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func TestRound167InvitationBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_ROUND167") != "1" {
		t.Skip("explicit owned browser fixture only")
	}
	f := setup(t)
	ctx := context.Background()
	token := f.administratorToken()
	rec, result := f.request("POST", "/v1/users", token, core.NewID(), map[string]string{"login_key": "expired.supplier.coordinator", "display_name": "新项目协作成员"}, nil)
	expect(t, rec, 201, result)
	invitation := data[identity.InvitationReceipt](t, result)
	if _, err := f.owner.Exec(ctx, "UPDATE invitations SET expires_at=clock_timestamp()-interval '1 minute' WHERE id=$1", invitation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
        SELECT gen_random_uuid(),$1,$2,id,'org','confidential' FROM roles WHERE org_id=$1 AND key='system_admin'`, f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch167-invitation-reissue", "invited_user_id": invitation.User.ID, "invited_user_name": invitation.User.DisplayName, "original_invitation_id": invitation.ID})
}
