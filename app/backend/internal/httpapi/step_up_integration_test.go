package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func stepUpFixture(t *testing.T) (*fixture, string, string) {
	f := setup(t)
	token := f.administratorToken()
	var cipher []byte
	if err := f.owner.QueryRow(context.Background(), "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.admin).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	secret, err := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(context.Background(), "UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.admin, time.Now().Unix()/30-1); err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(context.Background(), "UPDATE sessions SET mfa_verified_at=now()-interval '1 hour' WHERE org_id=$1 AND user_id=$2", f.org, f.admin); err != nil {
		t.Fatal(err)
	}
	return f, token, secret
}

func TestSessionMFAStepUp(t *testing.T) {
	ctx := context.Background()
	t.Run("proof updates only the current session and rejects consumed codes without logging out", func(t *testing.T) {
		f, token, secret := stepUpFixture(t)
		before, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		input := map[string]string{"code": authenticatorCode(secret, time.Now())}
		rec, result := f.request("POST", "/v1/auth/mfa/step-up", token, "", input, nil)
		expect(t, rec, 200, result)
		proof := data[identity.StepUpResult](t, result)
		after, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil || after.SessionID != before.SessionID || after.MFAVerifiedAt == nil || time.Since(*after.MFAVerifiedAt) > time.Minute || proof.MFAVerifiedAt.IsZero() {
			t.Fatal("step-up replaced or failed to verify the current device", err)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Fatal("step-up unexpectedly rotated session credentials")
		}
		rec, result = f.request("POST", "/v1/auth/mfa/step-up", token, "", input, nil)
		expect(t, rec, 422, result)
		if _, err = f.api.Identity.Authenticate(ctx, token); err != nil {
			t.Fatal("invalid MFA terminated existing login", err)
		}
		rec, result = f.request("GET", "/v1/me", token, "", nil, nil)
		expect(t, rec, 200, result)
	})
	t.Run("audit failure rolls back proof and code consumption", func(t *testing.T) {
		f, token, secret := stepUpFixture(t)
		if _, err := f.owner.Exec(ctx, `CREATE FUNCTION reject_step_up_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action='auth.mfa_step_up' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END $$;
			CREATE TRIGGER reject_step_up_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_step_up_fixture()`); err != nil {
			t.Fatal(err)
		}
		input := map[string]string{"code": authenticatorCode(secret, time.Now())}
		rec, result := f.request("POST", "/v1/auth/mfa/step-up", token, "", input, nil)
		expect(t, rec, 500, result)
		a, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil || a.MFAVerifiedAt == nil || time.Since(*a.MFAVerifiedAt) < 50*time.Minute {
			t.Fatal("failed audit committed MFA proof", err)
		}
		if _, err = f.owner.Exec(ctx, "DROP TRIGGER reject_step_up_fixture ON audit_events"); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/auth/mfa/step-up", token, "", input, nil)
		expect(t, rec, 200, result)
	})
	t.Run("missing enrollment stale session and rate limit fail closed", func(t *testing.T) {
		f, token, _ := stepUpFixture(t)
		rec, result := f.request("POST", "/v1/auth/mfa/step-up", f.aliceToken, "", map[string]string{"code": "123456"}, nil)
		expect(t, rec, 409, result)
		a, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.owner.Exec(ctx, "UPDATE sessions SET revoked_at=now() WHERE id=$1", a.SessionID); err != nil {
			t.Fatal(err)
		}
		if _, err = f.api.Identity.StepUp(ctx, a, identity.StepUpInput{Code: "123456"}, core.NewID()); core.PublicError(err).Code != "SESSION_CHANGED" {
			t.Fatal("revoked device could reverify", err)
		}
		rec, result = f.request("POST", "/v1/auth/mfa/step-up", token, "", map[string]string{"code": "123456"}, nil)
		expect(t, rec, 401, result)
		for range 7 {
			rec, result = f.request("POST", "/v1/auth/mfa/step-up", f.aliceToken, "", map[string]string{"code": "123456"}, nil)
			expect(t, rec, 409, result)
		}
		rec, result = f.request("POST", "/v1/auth/mfa/step-up", f.aliceToken, "", map[string]string{"code": "123456"}, nil)
		expect(t, rec, 429, result)
		a.RunID = core.NewID()
		if _, err = f.api.Identity.StepUp(ctx, a, identity.StepUpInput{Code: "123456"}, core.NewID()); core.PublicError(err).Status != 403 {
			t.Fatal("agent could step up", err)
		}
	})
}
