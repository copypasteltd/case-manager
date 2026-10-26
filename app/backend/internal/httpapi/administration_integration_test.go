package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/project"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func (f *fixture) administratorToken() string {
	f.t.Helper()
	challenge := f.login("sysadmin", "android")
	rec, result := f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
	expect(f.t, rec, 200, result)
	enrollment := data[identity.LoginResult](f.t, result)
	rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(enrollment.MFASecret, time.Now())}, nil)
	expect(f.t, rec, 200, result)
	return data[identity.LoginResult](f.t, result).AccessToken
}
func TestIdentityAdministrationIntegration(t *testing.T) {
	f := setup(t)
	adminToken := f.administratorToken()
	rec, result := f.request("GET", "/v1/roles", adminToken, "", nil, nil)
	expect(t, rec, 200, result)
	roles := data[core.Page[identity.Role]](t, result)
	var memberRole string
	for _, role := range roles.Items {
		if role.Key == "member" {
			memberRole = role.ID
		}
	}
	if memberRole == "" {
		t.Fatal("member role missing")
	}
	t.Run("invitation is single use and secrets are excluded from idempotency persistence", func(t *testing.T) {
		f := f.with(t)
		key := core.NewID()
		input := map[string]string{"login_key": "invited.user", "display_name": "受邀成员"}
		rec, result := f.request("POST", "/v1/users", adminToken, key, input, nil)
		expect(t, rec, 201, result)
		receipt := data[identity.InvitationReceipt](t, result)
		if len(receipt.Token) != 43 {
			t.Fatal("invitation token missing")
		}
		var stored string
		if err := f.owner.QueryRow(context.Background(), "SELECT response_data::text FROM idempotency_records WHERE idempotency_key=$1", key).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored, receipt.Token) || strings.Contains(stored, "activation_token") {
			t.Fatal("activation secret persisted in command receipt")
		}
		rec, result = f.request("POST", "/v1/users", adminToken, key, input, nil)
		expect(t, rec, 201, result)
		if data[identity.InvitationReceipt](t, result).Token != receipt.Token {
			t.Fatal("authorized invitation receipt was not repeatable")
		}
		activate := map[string]string{"token": receipt.Token, "password": "new-member-password-long"}
		rec, result = f.request("POST", "/v1/auth/activate", "", "", activate, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/auth/activate", "", "", activate, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: input["login_key"], Password: activate["password"], Device: "android"}, nil)
		expect(t, rec, 200, result)
		login := data[identity.LoginResult](t, result)
		rec, result = f.request("GET", "/v1/projects", login.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[project.Project]](t, result).Items) != 0 {
			t.Fatal("activation automatically granted content")
		}
		rec, result = f.request("POST", "/v1/role-bindings", adminToken, core.NewID(), map[string]any{"user_id": receipt.User.ID, "role_id": memberRole, "scope_type": "org", "classification_ceiling": "project_restricted", "reason": "加入项目协作角色"}, nil)
		expect(t, rec, 201, result)
		binding := data[identity.RoleBinding](t, result)
		rec, result = f.request("GET", "/v1/me", login.AccessToken, "", nil, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: input["login_key"], Password: activate["password"], Device: "android"}, nil)
		expect(t, rec, 200, result)
		login = data[identity.LoginResult](t, result)
		rec, result = f.request("POST", "/v1/role-bindings/"+binding.ID+"/revoke", adminToken, core.NewID(), map[string]any{"expected_version": binding.Version, "reason": "撤销项目协作角色"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", "/v1/me", login.AccessToken, "", nil, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/users/"+receipt.User.ID+"/disable", adminToken, core.NewID(), map[string]any{"expected_version": 2, "action": "disable", "reason": "账号停用验证"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: input["login_key"], Password: activate["password"], Device: "android"}, nil)
		expect(t, rec, 401, result)
	})
	t.Run("expired invitation and stale MFA cannot administer accounts", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/users", adminToken, core.NewID(), map[string]string{"login_key": "expired.invitation", "display_name": "过期邀请"}, nil)
		expect(t, rec, 201, result)
		receipt := data[identity.InvitationReceipt](t, result)
		if _, err := f.owner.Exec(context.Background(), "UPDATE invitations SET expires_at=now()-interval '1 minute' WHERE id=$1", receipt.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/auth/activate", "", "", map[string]string{"token": receipt.Token, "password": "new-member-password-long"}, nil)
		expect(t, rec, 401, result)
		if _, err := f.owner.Exec(context.Background(), "UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE user_id=$1", f.admin); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/users", adminToken, core.NewID(), map[string]string{"login_key": "stale.mfa", "display_name": "拒绝验证"}, nil)
		expect(t, rec, 403, result)
		if result.Error.Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("stale MFA did not require reauthentication")
		}
	})
}
func TestResourceScopesUseCreatorAndExactTask(t *testing.T) {
	f := setup(t)
	p := f.createProject("Own scope semantics", []string{f.bob})
	rec, result := f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "owner_id": f.bob}, nil)
	expect(t, rec, 200, result)
	p = data[project.Project](t, result)
	a, b := f.createTask(p, "Exact readable task", false), f.createTask(p, "Hidden predecessor", false)
	rec, result = f.request("POST", "/v1/tasks/"+a.ID+"/dependencies", f.aliceToken, core.NewID(), map[string]any{"expected_version": a.Version, "predecessor_id": b.ID, "action": "add"}, nil)
	expect(t, rec, 200, result)
	ctx := context.Background()
	ownRole := core.NewID()
	if _, err := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'own_reader','本人创建资料读取')", ownRole, f.org); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"project.read", "task.read"} {
		if _, err := f.owner.Exec(ctx, "INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, ownRole, action); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'own')", core.NewID(), f.org, f.bob, ownRole); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("GET", "/v1/projects/"+p.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 404, result)
	rec, result = f.request("GET", "/v1/tasks/"+a.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 404, result)
	if _, err := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id) VALUES($1,$2,$3,$4,'resource',$5)", core.NewID(), f.org, f.bob, ownRole, a.ID); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("GET", "/v1/tasks/"+a.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 200, result)
	rec, result = f.request("GET", "/v1/tasks/"+b.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 404, result)
	rec, result = f.request("GET", "/v1/tasks/"+a.ID+"/dependencies", f.bobToken, "", nil, nil)
	expect(t, rec, 200, result)
	var edges struct {
		Items []project.Predecessor `json:"items"`
	}
	if err := json.Unmarshal(result.Data, &edges); err != nil {
		t.Fatal(err)
	}
	if len(edges.Items) != 0 {
		t.Fatal("dependency titles bypassed exact resource scope")
	}
}
