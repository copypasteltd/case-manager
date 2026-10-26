package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func (f *fixture) deviceSessions(token string) core.Page[identity.DeviceSession] {
	f.t.Helper()
	rec, result := f.request("GET", "/v1/me/sessions", token, "", nil, nil)
	expect(f.t, rec, 200, result)
	return data[core.Page[identity.DeviceSession]](f.t, result)
}
func (f *fixture) issuePasswordReset(admin, user string, version int64) identity.PasswordResetReceipt {
	f.t.Helper()
	rec, result := f.request("POST", "/v1/users/"+user+"/password-resets", admin, core.NewID(), identity.PasswordResetInput{ExpectedVersion: version, Reason: "成员确认身份后申请密码恢复"}, nil)
	expect(f.t, rec, 201, result)
	return data[identity.PasswordResetReceipt](f.t, result)
}

func TestAccountDeviceSessions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	first := f.deviceSessions(f.aliceToken)
	if len(first.Items) != 1 || !first.Items[0].Current || first.Items[0].Status != "active" {
		t.Fatal("current device missing")
	}
	original := first.Items[0]
	mobile := f.login("alice", "wechat")
	rec, result := f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": mobile.RefreshToken}, nil)
	expect(t, rec, 200, result)
	rotated := data[identity.LoginResult](t, result)
	t.Run("rotation keeps one device and stable creation time", func(t *testing.T) {
		f := f.with(t)
		page := f.deviceSessions(rotated.AccessToken)
		if len(page.Items) != 2 {
			t.Fatal("refresh exposed duplicate devices")
		}
		for _, v := range page.Items {
			if v.Current && v.Device != "wechat" {
				t.Fatal("wrong current device")
			}
			if v.ID == original.ID && (!v.StartedAt.Equal(original.StartedAt) || v.Current) {
				t.Fatal("unstable device identity")
			}
		}
	})
	t.Run("pagination binds to the owning employee", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", "/v1/me/sessions?limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[identity.DeviceSession]](t, result)
		if page.NextCursor == nil {
			t.Fatal("missing cursor")
		}
		rec, result = f.request("GET", "/v1/me/sessions?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		next := data[core.Page[identity.DeviceSession]](t, result)
		if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
			t.Fatal("pagination repeated a device")
		}
		rec, result = f.request("GET", "/v1/me/sessions?cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		if rec.Code < 400 || result.Error == nil {
			t.Fatal("foreign cursor accepted")
		}
	})
	t.Run("foreign or unknown device cannot be revoked", func(t *testing.T) {
		f := f.with(t)
		for _, id := range []string{original.ID, core.NewID()} {
			rec, result := f.request("POST", "/v1/me/sessions/"+id+"/revoke", f.bobToken, core.NewID(), identity.SessionRevokeInput{Reason: "移除设备"}, nil)
			expect(t, rec, 404, result)
		}
	})
	var mobileID string
	for _, v := range f.deviceSessions(rotated.AccessToken).Items {
		if v.Current {
			mobileID = v.ID
		}
	}
	t.Run("revoke blocks access and refresh and safely replays", func(t *testing.T) {
		f := f.with(t)
		key := core.NewID()
		path := "/v1/me/sessions/" + mobileID + "/revoke"
		input := identity.SessionRevokeInput{Reason: "旧设备已停用"}
		for i := 0; i < 2; i++ {
			rec, result := f.request("POST", path, f.aliceToken, key, input, nil)
			expect(t, rec, 200, result)
			if data[identity.SessionRevocation](t, result).ID != mobileID {
				t.Fatal("wrong revocation receipt")
			}
		}
		rec, result := f.request("GET", "/v1/me", rotated.AccessToken, "", nil, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": rotated.RefreshToken}, nil)
		expect(t, rec, 401, result)
		for _, v := range f.deviceSessions(f.aliceToken).Items {
			if v.ID == mobileID && v.Status != "revoked" {
				t.Fatal("revoked device shown active")
			}
		}
		var audits, events int
		if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_events WHERE action='auth.session_revoked'),(SELECT count(*) FROM outbox_events WHERE event_type='auth.session_revoked')`).Scan(&audits, &events); err != nil || audits != 1 || events != 1 {
			t.Fatal("revocation audit/outbox replay mismatch")
		}
	})
	t.Run("revocation wins against a racing refresh", func(t *testing.T) {
		f := f.with(t)
		login := f.login("alice", "douyin")
		var family string
		for _, v := range f.deviceSessions(login.AccessToken).Items {
			if v.Current {
				family = v.ID
			}
		}
		var wg sync.WaitGroup
		var refresh identity.LoginResult
		var refreshCode, revokeCode int
		wg.Add(2)
		go func() {
			defer wg.Done()
			rec, result := f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": login.RefreshToken}, nil)
			refreshCode = rec.Code
			if rec.Code == 200 {
				refresh = data[identity.LoginResult](t, result)
			}
		}()
		go func() {
			defer wg.Done()
			rec, _ := f.request("POST", "/v1/me/sessions/"+family+"/revoke", f.aliceToken, core.NewID(), identity.SessionRevokeInput{Reason: "并发撤销验证"}, nil)
			revokeCode = rec.Code
		}()
		wg.Wait()
		if revokeCode != 200 || (refreshCode != 200 && refreshCode != 401) {
			t.Fatalf("unexpected race outcomes %d/%d", revokeCode, refreshCode)
		}
		if refresh.AccessToken != "" {
			rec, result := f.request("GET", "/v1/me", refresh.AccessToken, "", nil, nil)
			expect(t, rec, 401, result)
		}
		var active int
		if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE org_id=$1 AND family_id=$2 AND revoked_at IS NULL`, f.org, family).Scan(&active); err != nil || active != 0 {
			t.Fatal("racing refresh escaped revocation")
		}
	})
	t.Run("current device can revoke itself", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/me/sessions/"+original.ID+"/revoke", f.aliceToken, core.NewID(), identity.SessionRevokeInput{Reason: "退出当前设备"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", "/v1/me", f.aliceToken, "", nil, nil)
		expect(t, rec, 401, result)
	})
}

func TestAccountRecoveryIssuerChange(t *testing.T) {
	f := setup(t)
	admin := f.administratorToken()
	reset := f.issuePasswordReset(admin, f.bob, 1)
	rec, result := f.request("POST", "/v1/me/password", admin, core.NewID(), identity.PasswordChangeInput{CurrentPassword: "initial-test-password-very-long", Password: "admin-replaced-password-long"}, nil)
	expect(t, rec, 200, result)
	rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: reset.Token, Password: "another-bob-password-long"}, nil)
	expect(t, rec, 401, result)
	rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: strings.Repeat("z", 43), Password: "another-bob-password-long"}, nil)
	expect(t, rec, 401, result)
	var denied int
	if err := f.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE action='auth.password_reset' AND result='denied' AND actor_id IS NULL`).Scan(&denied); err != nil || denied != 2 {
		t.Fatal("failed recovery attempts lost audit evidence")
	}
}

func TestAccountSecurityBrowserFixture(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='system_admin'`, core.NewID(), f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	var androidID, bobID string
	for _, v := range f.deviceSessions(f.aliceToken).Items {
		if v.Current {
			androidID = v.ID
		}
	}
	for _, v := range f.deviceSessions(f.bobToken).Items {
		if v.Current {
			bobID = v.ID
		}
	}
	reset := f.issuePasswordReset(f.aliceToken, f.bob, 1)
	actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"web", "telegram"} {
		a := actor
		a.Channel = channel
		a.RunID = core.NewID()
		if _, err = f.api.Identity.Sessions(ctx, a, "", 20); err == nil {
			t.Fatal("agent accessed employee device sessions")
		}
	}
	if os.Getenv("CP_BROWSER_ACCOUNT_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"account_security_fixture": true, "alice_id": f.alice, "bob_id": f.bob, "alice_android_id": androidID, "bob_device_id": bobID, "mobile_reset_token": reset.Token})
	}
}

func TestAccountPasswordRecovery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	admin := f.administratorToken()
	t.Run("authority requires current MFA and explicit action", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/users/"+f.bob+"/password-resets", f.aliceToken, core.NewID(), identity.PasswordResetInput{ExpectedVersion: 1, Reason: "拒绝越权"}, nil)
		expect(t, rec, 403, result)
		if _, err := f.owner.Exec(ctx, `UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE user_id=$1`, f.admin); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/users/"+f.bob+"/password-resets", admin, core.NewID(), identity.PasswordResetInput{ExpectedVersion: 1, Reason: "拒绝陈旧验证"}, nil)
		expect(t, rec, 403, result)
		if _, err := f.owner.Exec(ctx, `UPDATE sessions SET mfa_verified_at=now() WHERE user_id=$1`, f.admin); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("token excluded from persistence and replay stops after reissue", func(t *testing.T) {
		f := f.with(t)
		key := core.NewID()
		body := identity.PasswordResetInput{ExpectedVersion: 1, Reason: "首次签发恢复"}
		path := "/v1/users/" + f.bob + "/password-resets"
		rec, result := f.request("POST", path, admin, key, body, nil)
		expect(t, rec, 201, result)
		first := data[identity.PasswordResetReceipt](t, result)
		if len(first.Token) != 43 {
			t.Fatal("reset token absent")
		}
		rec, result = f.request("POST", path, admin, key, body, nil)
		expect(t, rec, 201, result)
		if data[identity.PasswordResetReceipt](t, result).Token != first.Token {
			t.Fatal("live receipt not replayable")
		}
		var stored string
		if err := f.owner.QueryRow(ctx, `SELECT response_data::text FROM idempotency_records WHERE idempotency_key=$1`, key).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored, first.Token) || strings.Contains(stored, "reset_token") {
			t.Fatal("secret stored in command response")
		}
		second := f.issuePasswordReset(admin, f.bob, 1)
		rec, result = f.request("POST", path, admin, key, body, nil)
		expect(t, rec, 201, result)
		if data[identity.PasswordResetReceipt](t, result).Token != "" {
			t.Fatal("revoked receipt regenerated secret")
		}
		rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: first.Token, Password: "new-bob-password-long"}, nil)
		expect(t, rec, 401, result)
		if _, err := f.owner.Exec(ctx, `UPDATE password_resets SET expires_at=now()-interval '1 second',created_at=now()-interval '20 minutes' WHERE id=$1`, second.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: second.Token, Password: "new-bob-password-long"}, nil)
		expect(t, rec, 401, result)
	})
	t.Run("version and disabled account checks", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/users/"+f.bob+"/password-resets", admin, core.NewID(), identity.PasswordResetInput{ExpectedVersion: 999, Reason: "陈旧成员资料"}, nil)
		expect(t, rec, 409, result)
		reset := f.issuePasswordReset(admin, f.bob, 1)
		if _, err := f.owner.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, f.bob); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: reset.Token, Password: "new-bob-password-long"}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/users/"+f.bob+"/password-resets", admin, core.NewID(), identity.PasswordResetInput{ExpectedVersion: 1, Reason: "停用成员拒绝恢复"}, nil)
		expect(t, rec, 409, result)
		if _, err := f.owner.Exec(ctx, `UPDATE users SET status='active' WHERE id=$1`, f.bob); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("single use reset revokes all sessions without granting permissions", func(t *testing.T) {
		f := f.with(t)
		old := f.login("bob", "wechat")
		reset := f.issuePasswordReset(admin, f.bob, 1)
		var wg sync.WaitGroup
		codes := make([]int, 2)
		wg.Add(2)
		for i := range codes {
			go func(i int) {
				defer wg.Done()
				rec, _ := f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: reset.Token, Password: "new-bob-password-long"}, nil)
				codes[i] = rec.Code
			}(i)
		}
		wg.Wait()
		if !((codes[0] == 200 && codes[1] == 401) || (codes[1] == 200 && codes[0] == 401)) {
			t.Fatalf("single use failed: %v", codes)
		}
		rec, result := f.request("GET", "/v1/me", f.bobToken, "", nil, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": old.RefreshToken}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "bob", Password: "member-test-password-long", Device: "android"}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "bob", Password: "new-bob-password-long", Device: "android"}, nil)
		expect(t, rec, 200, result)
		login := data[identity.LoginResult](t, result)
		if login.Step != "authenticated" {
			t.Fatal("non-MFA member could not log in")
		}
		rec, result = f.request("POST", "/v1/users/"+f.alice+"/password-resets", login.AccessToken, core.NewID(), identity.PasswordResetInput{ExpectedVersion: 1, Reason: "恢复不提升权限"}, nil)
		expect(t, rec, 403, result)
		var count int
		if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='auth.password_reset_completed' AND actor_id IS NULL AND resource_id=$1`, f.bob).Scan(&count); err != nil || count != 1 {
			t.Fatal("reset proof was attributed to an authenticated employee")
		}
	})
	t.Run("reset preserves enrolled MFA and consumes old challenges", func(t *testing.T) {
		f := f.with(t)
		challenge := f.login("sysadmin", "android")
		reset := f.issuePasswordReset(admin, f.admin, 1)
		rec, result := f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: reset.Token, Password: "new-admin-password-long"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/auth/mfa/recover", "", "", identity.MFAInput{Challenge: challenge.Challenge, Code: strings.Repeat("x", 20)}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "sysadmin", Password: "new-admin-password-long", Device: "android"}, nil)
		expect(t, rec, 200, result)
		if data[identity.LoginResult](t, result).Step != "mfa" {
			t.Fatal("password recovery bypassed administrator MFA")
		}
	})
}

func TestAccountPasswordChange(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	old := f.login("alice", "douyin")
	admin := f.administratorToken()
	t.Run("wrong current password and same password leave sessions valid", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/me/password", f.aliceToken, core.NewID(), identity.PasswordChangeInput{CurrentPassword: "incorrect-old-password", Password: "changed-alice-password-long"}, nil)
		expect(t, rec, 403, result)
		rec, result = f.request("POST", "/v1/me/password", f.aliceToken, core.NewID(), identity.PasswordChangeInput{CurrentPassword: "member-test-password-long", Password: "member-test-password-long"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("GET", "/v1/me", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
	})
	t.Run("confirmed change invalidates access refresh and pending recovery", func(t *testing.T) {
		f := f.with(t)
		reset := f.issuePasswordReset(admin, f.alice, 1)
		rec, result := f.request("POST", "/v1/me/password", f.aliceToken, core.NewID(), identity.PasswordChangeInput{CurrentPassword: "member-test-password-long", Password: "changed-alice-password-long"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", "/v1/me", old.AccessToken, "", nil, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": old.RefreshToken}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/password/reset", "", "", identity.CompletePasswordResetInput{Token: reset.Token, Password: "another-alice-password-long"}, nil)
		expect(t, rec, 401, result)
		rec, result = f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "alice", Password: "changed-alice-password-long", Device: "android"}, nil)
		expect(t, rec, 200, result)
		var stored string
		if err := f.owner.QueryRow(ctx, `SELECT string_agg(response_data::text,' ') FROM idempotency_records WHERE route_key='POST /v1/me/password'`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored, "changed-alice-password-long") || strings.Contains(stored, "member-test-password-long") {
			t.Fatal("password persisted outside credential hash")
		}
		var fingerprint string
		if err := f.owner.QueryRow(ctx, `SELECT request_hash FROM idempotency_records WHERE actor_id=$1 AND route_key='POST /v1/me/password'`, f.alice).Scan(&fingerprint); err != nil {
			t.Fatal(err)
		}
		input, _ := json.Marshal(identity.PasswordChangeInput{CurrentPassword: "member-test-password-long", Password: "changed-alice-password-long"})
		canonical, err := core.CanonicalJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		if fingerprint == core.Digest(append([]byte("POST /v1/me/password\n"), canonical...)) {
			t.Fatal("credential receipt exposed an unkeyed password fingerprint")
		}
	})
	t.Run("MFA account requires fresh verification even with correct password", func(t *testing.T) {
		f := f.with(t)
		if _, err := f.owner.Exec(ctx, `UPDATE sessions SET mfa_verified_at=now()-interval '11 minutes' WHERE user_id=$1`, f.admin); err != nil {
			t.Fatal(err)
		}
		rec, result := f.request("POST", "/v1/me/password", admin, core.NewID(), identity.PasswordChangeInput{CurrentPassword: "initial-test-password-very-long", Password: "changed-admin-password-long"}, nil)
		expect(t, rec, 403, result)
	})
}
