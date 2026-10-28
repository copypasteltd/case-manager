package httpapi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/miniapp"
)

type syntheticMiniapp struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (p *syntheticMiniapp) Exchange(ctx context.Context, name string, profile miniapp.Profile, code string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return "", errors.New("private provider diagnostic must not escape")
	}
	// Different one-use codes can refer to the same platform subject.
	return strings.SplitN(code, ":", 2)[0], nil
}
func configureMiniapps(f *fixture) *syntheticMiniapp {
	p := &syntheticMiniapp{}
	f.api.Identity.Config.Miniapp = miniapp.Config{WeChat: miniapp.Profile{AppID: "wx0123456789abcdef", Secret: "synthetic-wechat-secret"}, Douyin: miniapp.Profile{AppID: "tt-synthetic-app", Secret: "synthetic-douyin-secret"}, IdentityKey: []byte("0123456789abcdef0123456789abcdef")}
	f.api.Identity.MiniappExchange = p
	return p
}
func (f *fixture) miniappProof(token, name, code string) identity.MiniappProof {
	f.t.Helper()
	profile, _ := f.api.Identity.Config.Miniapp.Profile(name)
	rec, result := f.request("POST", "/v1/me/miniapp-binding-proofs", token, "", identity.MiniappProofInput{Platform: name, AppID: profile.AppID, Code: code, Password: "member-test-password-long"}, nil)
	expect(f.t, rec, 201, result)
	return data[identity.MiniappProof](f.t, result)
}
func (f *fixture) bindMiniapp(token string, proof identity.MiniappProof) (identity.MiniappBinding, string) {
	f.t.Helper()
	key := core.NewID()
	rec, result := f.request("POST", "/v1/me/miniapp-bindings", token, key, identity.MiniappBindInput{ProofID: proof.ID, Confirmed: true, Reason: "本人确认隔离合成平台身份"}, nil)
	expect(f.t, rec, 201, result)
	return data[identity.MiniappBinding](f.t, result), key
}
func (f *fixture) miniappLogin(name, code string) identity.LoginResult {
	f.t.Helper()
	profile, _ := f.api.Identity.Config.Miniapp.Profile(name)
	rec, result := f.request("POST", "/v1/auth/miniapps/login", "", "", identity.MiniappCodeInput{Platform: name, AppID: profile.AppID, Code: code}, nil)
	expect(f.t, rec, 200, result)
	return data[identity.LoginResult](f.t, result)
}
func (f *fixture) revokeMiniapp(token string, b identity.MiniappBinding) identity.MiniappBinding {
	f.t.Helper()
	rec, result := f.request("POST", "/v1/me/miniapp-bindings/"+b.ID+"/revoke", token, core.NewID(), identity.MiniappRevokeInput{ExpectedVersion: b.Version, Reason: "本人确认解除隔离合成绑定"}, nil)
	expect(f.t, rec, 200, result)
	return data[identity.MiniappBinding](f.t, result)
}

func TestMiniappBindingLifecycle(t *testing.T) {
	for _, name := range []string{"wechat", "douyin"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			provider := configureMiniapps(f)
			passwordSession := f.login("alice", name)
			rec, result := f.request("GET", "/v1/auth/miniapps", "", "", nil, nil)
			expect(t, rec, 200, result)
			if len(data[identity.MiniappProfiles](t, result).Items) != 2 {
				t.Fatal("configured profiles missing")
			}
			profile, _ := f.api.Identity.Config.Miniapp.Profile(name)
			rec, result = f.request("POST", "/v1/me/miniapp-binding-proofs", passwordSession.AccessToken, "", identity.MiniappProofInput{Platform: name, AppID: profile.AppID, Code: "alice:wrong-password", Password: "wrong-password"}, nil)
			expect(t, rec, 403, result)
			if provider.calls != 0 {
				t.Fatal("provider called before employee authentication")
			}
			proof := f.miniappProof(passwordSession.AccessToken, name, "alice:bind")
			b, key := f.bindMiniapp(passwordSession.AccessToken, proof)
			in := identity.MiniappBindInput{ProofID: proof.ID, Confirmed: true, Reason: "本人确认隔离合成平台身份"}
			rec, result = f.request("POST", "/v1/me/miniapp-bindings", passwordSession.AccessToken, key, in, nil)
			expect(t, rec, 201, result)
			if data[identity.MiniappBinding](t, result).ID != b.ID {
				t.Fatal("idempotent binding duplicated")
			}
			rec, result = f.request("POST", "/v1/me/miniapp-bindings", passwordSession.AccessToken, core.NewID(), in, nil)
			expect(t, rec, 409, result)
			login := f.miniappLogin(name, "alice:login")
			if login.Step != "authenticated" || login.AccessToken == "" {
				t.Fatal("existing employee did not authenticate")
			}
			rec, result = f.request("POST", "/v1/auth/miniapps/login", "", "", identity.MiniappCodeInput{Platform: name, AppID: profile.AppID, Code: "alice:login"}, nil)
			expect(t, rec, 409, result)
			rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": login.RefreshToken}, nil)
			expect(t, rec, 200, result)
			rotated := data[identity.LoginResult](t, result)
			rec, result = f.request("GET", "/v1/me/miniapp-bindings", rotated.AccessToken, "", nil, nil)
			expect(t, rec, 200, result)
			page := data[core.Page[identity.MiniappBinding]](t, result)
			if len(page.Items) != 1 || !page.Items[0].Current {
				t.Fatal("rotated session lost binding source")
			}
			rec, result = f.request("GET", "/v1/me/miniapp-bindings", f.bobToken, "", nil, nil)
			expect(t, rec, 200, result)
			if len(data[core.Page[identity.MiniappBinding]](t, result).Items) != 0 {
				t.Fatal("cross-user listing")
			}
			rec, result = f.request("POST", "/v1/me/miniapp-bindings/"+b.ID+"/revoke", f.bobToken, core.NewID(), identity.MiniappRevokeInput{ExpectedVersion: 1, Reason: "cross-user attempt"}, nil)
			expect(t, rec, 404, result)
			revoked := f.revokeMiniapp(passwordSession.AccessToken, b)
			if revoked.RevokedAt == nil || revoked.Version != 2 {
				t.Fatal("missing revocation receipt")
			}
			rec, result = f.request("GET", "/v1/me", rotated.AccessToken, "", nil, nil)
			expect(t, rec, 401, result)
			rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]string{"refresh_token": rotated.RefreshToken}, nil)
			expect(t, rec, 401, result)
			rec, result = f.request("GET", "/v1/me", passwordSession.AccessToken, "", nil, nil)
			expect(t, rec, 200, result)
			rec, result = f.request("POST", "/v1/me/miniapp-bindings", passwordSession.AccessToken, key, in, nil)
			expect(t, rec, 409, result)
			proof = f.miniappProof(passwordSession.AccessToken, name, "alice:rebind")
			next, _ := f.bindMiniapp(passwordSession.AccessToken, proof)
			if next.ID == b.ID {
				t.Fatal("rebind rewrote history")
			}
			rec, result = f.request("GET", "/v1/me/miniapp-bindings?limit=1", passwordSession.AccessToken, "", nil, nil)
			expect(t, rec, 200, result)
			page = data[core.Page[identity.MiniappBinding]](t, result)
			if page.NextCursor == nil {
				t.Fatal("history not paginated")
			}
			rec, result = f.request("GET", "/v1/me/miniapp-bindings?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
			expect(t, rec, 400, result)
			bob := f.login("bob", name)
			bobProof := f.miniappProof(bob.AccessToken, name, "alice:other-employee")
			rec, result = f.request("POST", "/v1/me/miniapp-bindings", bob.AccessToken, core.NewID(), identity.MiniappBindInput{ProofID: bobProof.ID, Confirmed: true, Reason: "conflicting identity"}, nil)
			expect(t, rec, 409, result)
			var protected bool
			err := f.owner.QueryRow(context.Background(), `SELECT NOT has_table_privilege('copypaste_app','miniapp_bindings','DELETE') AND NOT has_column_privilege('copypaste_app','miniapp_bindings','subject_hash','UPDATE')`).Scan(&protected)
			if err != nil || !protected {
				t.Fatal("binding identity/history privileges too broad")
			}
			if strings.Contains(rec.Body.String(), "alice") || strings.Contains(rec.Body.String(), "subject_hash") {
				t.Fatal("identity leaked through collision")
			}
		})
	}
}

func TestMiniappProofBoundaries(t *testing.T) {
	f := setup(t)
	provider := configureMiniapps(f)
	profile, _ := f.api.Identity.Config.Miniapp.Profile("wechat")
	input := identity.MiniappProofInput{Platform: "wechat", AppID: profile.AppID, Code: "alice:android", Password: "member-test-password-long"}
	rec, result := f.request("POST", "/v1/me/miniapp-binding-proofs", f.aliceToken, "", input, nil)
	expect(t, rec, 403, result)
	if provider.calls != 0 {
		t.Fatal("wrong device contacted provider")
	}
	session := f.login("alice", "wechat")
	proof := f.miniappProof(session.AccessToken, "wechat", "alice:proof")
	other := f.login("alice", "wechat")
	in := identity.MiniappBindInput{ProofID: proof.ID, Confirmed: true, Reason: "same employee different session"}
	rec, result = f.request("POST", "/v1/me/miniapp-bindings", other.AccessToken, core.NewID(), in, nil)
	expect(t, rec, 409, result)
	if _, err := f.owner.Exec(context.Background(), `UPDATE miniapp_binding_proofs SET expires_at=now()-interval '1 second' WHERE id=$1`, proof.ID); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("POST", "/v1/me/miniapp-bindings", session.AccessToken, core.NewID(), in, nil)
	expect(t, rec, 409, result)
	provider.fail = true
	input.Code = "alice:timeout"
	rec, result = f.request("POST", "/v1/me/miniapp-binding-proofs", session.AccessToken, "", input, nil)
	expect(t, rec, 503, result)
	if strings.Contains(rec.Body.String(), "private provider") {
		t.Fatal("provider diagnostic leaked")
	}
	provider.fail = false
	rec, result = f.request("POST", "/v1/me/miniapp-binding-proofs", session.AccessToken, "", input, nil)
	expect(t, rec, 409, result)
	input.AppID = "wrong-app-id"
	input.Code = "alice:wrong-app"
	rec, result = f.request("POST", "/v1/me/miniapp-binding-proofs", session.AccessToken, "", input, nil)
	expect(t, rec, 503, result)
	input.AppID = profile.AppID
	input.Code = "alice:authority"
	proof = f.miniappProof(session.AccessToken, "wechat", input.Code)
	if _, err := f.owner.Exec(context.Background(), `UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2`, f.org, f.alice); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("POST", "/v1/me/miniapp-bindings", session.AccessToken, core.NewID(), identity.MiniappBindInput{ProofID: proof.ID, Confirmed: true, Reason: "stale employee authority"}, nil)
	expect(t, rec, 401, result)
	var users int
	if err := f.owner.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("POST", "/v1/auth/miniapps/login", "", "", identity.MiniappCodeInput{Platform: "wechat", AppID: profile.AppID, Code: "unbound:login"}, nil)
	expect(t, rec, 401, result)
	var after int
	if err := f.owner.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&after); err != nil || users != after {
		t.Fatal("platform login provisioned an employee")
	}
}

func TestMiniappMFAAndRevocation(t *testing.T) {
	f := setup(t)
	configureMiniapps(f)
	ctx := context.Background()
	session := f.login("alice", "wechat")
	proof := f.miniappProof(session.AccessToken, "wechat", "alice:bind")
	b, _ := f.bindMiniapp(session.AccessToken, proof)
	prior := f.miniappLogin("wechat", "alice:before-enrollment")
	secret := identity.NewTOTPSecret()
	cipher, err := identity.Seal(f.api.Config.MFAKey, secret, f.org+":"+f.alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(ctx, `INSERT INTO mfa_credentials(id,org_id,user_id,secret_cipher,recovery_hashes,last_step) VALUES($1,$2,$3,$4,$5,0)`, core.NewID(), f.org, f.alice, cipher, []string{core.Digest([]byte("01234567890123456789"))}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.api.Identity.Authenticate(ctx, prior.AccessToken); err == nil {
		t.Fatal("new authenticator did not fence old platform access")
	}
	if _, err = f.api.Identity.Refresh(ctx, prior.RefreshToken, "", false, core.NewID()); err == nil {
		t.Fatal("old platform session bypassed newly enrolled MFA through refresh")
	}
	// A credential added after this password session also makes binding changes
	// require fresh MFA; use a real recovery challenge to obtain that factor.
	challenge := f.miniappLogin("wechat", "alice:challenge")
	if challenge.Step != "mfa" || challenge.AccessToken != "" {
		t.Fatal("platform bypassed enrolled MFA")
	}
	rec, result := f.request("POST", "/v1/auth/mfa/recover", "", "", identity.MFAInput{Challenge: challenge.Challenge, Code: "01234567890123456789"}, nil)
	expect(t, rec, 200, result)
	recovered := data[identity.LoginResult](t, result)
	var source string
	if err = f.owner.QueryRow(ctx, `SELECT miniapp_binding_id FROM sessions WHERE access_hash=$1`, core.Digest([]byte(recovered.AccessToken))).Scan(&source); err != nil || source != b.ID {
		t.Fatal("MFA recovery lost binding provenance")
	}
	pending := f.miniappLogin("wechat", "alice:pending")
	revoked := f.revokeMiniapp(recovered.AccessToken, b)
	if !revoked.Current {
		t.Fatal("self unbinding did not identify current source")
	}
	rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", identity.MFAInput{Challenge: pending.Challenge, Code: "000000"}, nil)
	expect(t, rec, 401, result)
	rec, result = f.request("POST", "/v1/auth/mfa/recover", "", "", identity.MFAInput{Challenge: pending.Challenge, Code: "01234567890123456789"}, nil)
	expect(t, rec, 401, result)
	rec, result = f.request("GET", "/v1/me", recovered.AccessToken, "", nil, nil)
	expect(t, rec, 401, result)
	var consumed *time.Time
	if err = f.owner.QueryRow(ctx, `SELECT consumed_at FROM auth_challenges WHERE token_hash=$1`, core.Digest([]byte(pending.Challenge))).Scan(&consumed); err != nil || consumed == nil {
		t.Fatal("pending challenge survived unbinding")
	}
}

func TestMiniappRefreshRevocationRace(t *testing.T) {
	f := setup(t)
	configureMiniapps(f)
	password := f.login("alice", "douyin")
	proof := f.miniappProof(password.AccessToken, "douyin", "alice:bind")
	b, _ := f.bindMiniapp(password.AccessToken, proof)
	login := f.miniappLogin("douyin", "alice:login")
	ctx := context.Background()
	actor, err := f.api.Identity.Authenticate(ctx, password.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	var rotated identity.LoginResult
	var refreshErr, revokeErr error
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rotated, refreshErr = f.api.Identity.Refresh(ctx, login.RefreshToken, "", false, core.NewID())
	}()
	go func() {
		defer wg.Done()
		<-start
		in := identity.MiniappRevokeInput{ExpectedVersion: 1, Reason: "concurrent revocation verification"}
		body := []byte(`{"expected_version":1,"reason":"concurrent revocation verification"}`)
		_, revokeErr = f.api.Identity.RevokeMiniapp(ctx, core.Command{Actor: actor, Key: core.NewID(), Route: "POST /me/miniapp-bindings/" + b.ID + "/revoke", Body: body, RequestID: core.NewID()}, b.ID, in)
	}()
	close(start)
	wg.Wait()
	if revokeErr != nil {
		t.Fatal(revokeErr)
	}
	if refreshErr == nil {
		if _, err = f.api.Identity.Authenticate(ctx, rotated.AccessToken); err == nil {
			t.Fatal("racing refresh survived unbinding")
		}
	}
	if _, err = f.api.Identity.Authenticate(ctx, login.AccessToken); err == nil {
		t.Fatal("original session survived unbinding")
	}
	// The proof password check and a password-session refresh serialize at the
	// organization before touching user/session rows; neither holds locks over IO.
	proofSession := f.login("bob", "douyin")
	profile, _ := f.api.Identity.Config.Miniapp.Profile("douyin")
	proofActor, err := f.api.Identity.Authenticate(ctx, proofSession.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	var proofErr, proofRefreshErr error
	wg.Add(2)
	start = make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		_, proofErr = f.api.Identity.CreateMiniappProof(ctx, proofActor, identity.MiniappProofInput{Platform: "douyin", AppID: profile.AppID, Code: "bob:concurrent-proof", Password: "member-test-password-long"}, "owned-fixture", core.NewID())
	}()
	go func() {
		defer wg.Done()
		<-start
		_, proofRefreshErr = f.api.Identity.Refresh(ctx, proofSession.RefreshToken, "", false, core.NewID())
	}()
	close(start)
	wg.Wait()
	if proofRefreshErr != nil {
		t.Fatal("concurrent proof prevented refresh", proofRefreshErr)
	}
	if proofErr != nil {
		var apiError *core.Error
		if !errors.As(proofErr, &apiError) || (apiError.Code != "AUTHORIZATION_CHANGED" && apiError.Code != "REAUTHENTICATION_REQUIRED") {
			t.Fatal("unexpected proof/refresh lock failure", proofErr)
		}
	}
	// Disabling a configured profile also blocks already issued platform sessions.
	proof = f.miniappProof(password.AccessToken, "douyin", "alice:rebind")
	f.bindMiniapp(password.AccessToken, proof)
	login = f.miniappLogin("douyin", "alice:configured")
	f.api.Identity.Config.Miniapp.Douyin = miniapp.Profile{}
	if _, err = f.api.Identity.Authenticate(ctx, login.AccessToken); err == nil {
		t.Fatal("disabled provider session survived")
	}
	if _, err = f.api.Identity.Refresh(ctx, login.RefreshToken, "", false, core.NewID()); err == nil {
		t.Fatal("disabled provider refreshed")
	}
}

func TestMiniappMFAEnrollmentAndVerify(t *testing.T) {
	f := setup(t)
	configureMiniapps(f)
	password := f.login("alice", "wechat")
	proof := f.miniappProof(password.AccessToken, "wechat", "alice:enroll-bind")
	binding, _ := f.bindMiniapp(password.AccessToken, proof)
	if _, err := f.owner.Exec(context.Background(), `UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='project_manager'`, f.org); err != nil {
		t.Fatal(err)
	}
	challenge := f.miniappLogin("wechat", "alice:enroll-login")
	if challenge.Step != "mfa_enroll" {
		t.Fatal("required enrollment was skipped")
	}
	rec, result := f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
	expect(t, rec, 200, result)
	enrollment := data[identity.LoginResult](t, result)
	if enrollment.MFASecret == "" {
		t.Fatal("enrollment did not expose its one-time setup material")
	}
	rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", identity.MFAInput{Challenge: challenge.Challenge, Code: authenticatorCode(enrollment.MFASecret, time.Now())}, nil)
	expect(t, rec, 200, result)
	verified := data[identity.LoginResult](t, result)
	if verified.Step != "authenticated" || len(verified.RecoveryCodes) != 8 {
		t.Fatal("platform MFA enrollment incomplete")
	}
	var source string
	if err := f.owner.QueryRow(context.Background(), `SELECT miniapp_binding_id FROM sessions WHERE access_hash=$1`, core.Digest([]byte(verified.AccessToken))).Scan(&source); err != nil || source != binding.ID {
		t.Fatal("MFA verifier lost binding source")
	}
	f.revokeMiniapp(verified.AccessToken, binding)
	rec, result = f.request("GET", "/v1/me", verified.AccessToken, "", nil, nil)
	expect(t, rec, 401, result)
}
