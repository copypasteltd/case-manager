package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/identity"
)

// Only the explicitly opted-in, disposable native test exports this capability.
// Its synthetic identity secrets stay in an ignored 0600 file, outside media.
func holdFollowupBrowser(t *testing.T, f *fixture, ids map[string]string) {
	if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" || os.Getenv("CP_BROWSER_FOLLOWUPS_FIXTURE") != "1" || f.api.Config.Environment != "test" || os.Getenv("CP_TEST_ADMIN_URL") == "" {
		t.Fatal("explicit owned native fixture required")
	}
	const origin = "http://localhost:5189"
	f.api.Config.PublicOrigin = origin
	f.api.Config.AllowedOrigins = map[string]bool{origin: true}
	f.api.Identity.Config.PublicOrigin = origin
	f.api.Identity.Config.AllowedOrigins = f.api.Config.AllowedOrigins
	var encrypted []byte
	var step int64
	if err := f.owner.QueryRow(context.Background(), `SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2`, f.org, f.bob).Scan(&encrypted, &step); err != nil {
		t.Fatal(err)
	}
	secret, err := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.bob)
	if err != nil {
		t.Fatal("cannot open owned synthetic credential")
	}
	nonce := identity.Token()
	stop := make(chan struct{})
	var once sync.Once
	mux := http.NewServeMux()
	mux.Handle("/", f.api.Handler())
	mux.HandleFunc("POST /__fixture/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+nonce {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		once.Do(func() { close(stop) })
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	folder, err := filepath.Abs("../../../.local")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(folder, "business-browser-fixture.json")
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("owned fixture file already exists")
	}
	defer os.Remove(output)
	values := map[string]any{"fixture_kind": "business-followups", "origin": origin, "api_origin": server.URL, "controlled_provider": true, "isolated_database": true, "stop_token": nonce, "followup_ids": ids, "alice": map[string]any{"login": "bob", "password": "member-test-password-long", "mfa_secret": secret, "last_step": step}}
	err = json.NewEncoder(file).Encode(values)
	closed := file.Close()
	if err != nil || closed != nil {
		t.Fatal("cannot save synthetic fixture")
	}
	t.Log("isolated native business followup fixture ready")
	select {
	case <-stop:
		t.Log("browser released native business followup fixture")
	case <-time.After(12 * time.Minute):
		t.Fatal("owned fixture release timed out")
	}
}
