package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"os"
	"testing"
)

func TestMiniappBrowserFixture(t *testing.T) {
	f := setup(t)
	configureMiniapps(f)
	ids := map[string]string{}
	for _, name := range []string{"wechat", "douyin"} {
		password := f.login("alice", name)
		proof := f.miniappProof(password.AccessToken, name, "owned-synthetic-employee:bind-"+name)
		binding, _ := f.bindMiniapp(password.AccessToken, proof)
		ids[name] = binding.ID
	}
	var count int
	if err := f.owner.QueryRow(context.Background(), `SELECT count(*) FROM miniapp_bindings WHERE org_id=$1 AND user_id=$2 AND revoked_at IS NULL`, f.org, f.alice).Scan(&count); err != nil || count != 2 {
		t.Fatal("two-platform fixture not admitted")
	}
	if os.Getenv("CP_BROWSER_MINIAPP") != "1" {
		return
	}
	if _, err := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='contract_operator'`, core.NewID(), f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	_ = f.administratorToken()
	holdBrowserBusinessFixture(t, f, map[string]any{"miniapp_identity_fixture": true, "binding_ids": ids, "platform_login_on_real_device": false})
}
