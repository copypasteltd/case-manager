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

// Explicit opt-in only. The enclosing meaningful integration test owns the
// disposable native database and disposes it after the browser releases it.
func holdBrowserBusinessFixture(t *testing.T, f *fixture, values map[string]any) {
	t.Helper()
	if (os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" && os.Getenv("CP_BROWSER_AGENT_FIXTURE") != "1" && os.Getenv("CP_BROWSER_MOBILE_FIXTURE") != "1") || t.Failed() {
		return
	}
	if f.api.Config.Environment != "test" || os.Getenv("CP_TEST_ADMIN_URL") == "" {
		t.Fatal("browser fixture requires explicit native test database")
	}
	if os.Getenv("CP_BROWSER_DELIVERABLE_FIXTURE") == "1" {
		if _, err := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,u.id,r.id,'project',$4,'project_restricted' FROM users u JOIN roles r ON r.org_id=u.org_id AND r.key='deliverable_reviewer' WHERE u.org_id=$1 AND u.id IN($2,$3)`, f.org, f.alice, f.bob, values["project_id"]); err != nil {
			t.Fatal(err)
		}
		_ = memberMFAToken(f, "bob", "android")
	}
	if os.Getenv("CP_BROWSER_PROJECT_CHANGE_FIXTURE") == "1" {
		if _, err := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'project',$3,'project_restricted' FROM roles WHERE org_id=$1 AND key='project_change_implementer'`, f.org, f.alice, values["project_id"]); err != nil {
			t.Fatal(err)
		}
		values["project_change_fixture"] = true
	}
	if os.Getenv("CP_BROWSER_PROJECT_CLOSEOUT_FIXTURE") == "1" {
		if _, err := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT gen_random_uuid(),$1,$2,id,'org','project_restricted' FROM roles WHERE org_id=$1 AND key='project_archiver'`, f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		values["project_closeout_fixture"] = true
	}
	const origin = "http://localhost:5189"
	f.api.Config.PublicOrigin = origin
	f.api.Config.AllowedOrigins = map[string]bool{origin: true}
	f.api.Identity.Config.PublicOrigin = origin
	f.api.Identity.Config.AllowedOrigins = f.api.Config.AllowedOrigins
	f.handler = f.api.Handler()
	var encrypted []byte
	var step int64
	if err := f.owner.QueryRow(context.Background(), "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.alice).Scan(&encrypted, &step); err != nil {
		t.Fatal(err)
	}
	secret, err := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.alice)
	if err != nil {
		t.Fatal("cannot open owned test credential")
	}
	stop := make(chan struct{})
	var once sync.Once
	nonce := identity.Token()
	mux := http.NewServeMux()
	mux.Handle("/", f.handler)
	if values["task_mentions_fixture"] == true {
		mux.HandleFunc("POST /__fixture/task-mentions/drain", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+nonce {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if err := drainTaskMentionFixture(r.Context(), f); err != nil {
				t.Error("owned fixture notification drain", err)
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}

	if values["workbench_fixture"] == true {
		mux.HandleFunc("POST /__fixture/workbench/revoke-approval", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+nonce {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			result, err := f.owner.Exec(r.Context(), "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND user_id=$3 AND revoked_at IS NULL", f.org, values["workbench_review_binding"], f.alice)
			if err != nil || result.RowsAffected() != 1 {
				t.Error("owned workbench revocation failed")
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
	mux.HandleFunc("POST /__fixture/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+nonce {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		if stream, ok := w.(http.Flusher); ok {
			stream.Flush()
		}
		once.Do(func() { close(stop) })
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	values["origin"] = origin
	values["api_origin"] = server.URL
	values["controlled_provider"] = true
	values["isolated_database"] = true
	values["stop_token"] = nonce
	values["alice"] = map[string]any{"login": "alice", "password": "member-test-password-long", "mfa_secret": secret, "last_step": step}
	values["bob"] = map[string]any{"login": "bob", "password": "member-test-password-long"}
	if os.Getenv("CP_BROWSER_DELIVERABLE_FIXTURE") == "1" {
		if err := f.owner.QueryRow(context.Background(), "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.bob).Scan(&encrypted, &step); err != nil {
			t.Fatal(err)
		}
		bobSecret, err := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.bob)
		if err != nil {
			t.Fatal("cannot open owned reviewer credential")
		}
		values["bob"] = map[string]any{"login": "bob", "password": "member-test-password-long", "mfa_secret": bobSecret, "last_step": step}
		values["deliverable_fixture"] = true
	}
	if os.Getenv("CP_BROWSER_AGENT_FIXTURE") == "1" || os.Getenv("CP_BROWSER_MOBILE_FIXTURE") == "1" {
		if err := f.owner.QueryRow(context.Background(), "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.admin).Scan(&encrypted, &step); err != nil {
			t.Fatal(err)
		}
		adminSecret, err := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.admin)
		if err != nil {
			t.Fatal("cannot open owned administrator credential")
		}
		values["administrator"] = map[string]any{"login": "sysadmin", "password": "initial-test-password-very-long", "mfa_secret": adminSecret, "last_step": step}
		values["fixture_kind"] = "agent-context-and-classification"
		if os.Getenv("CP_BROWSER_MOBILE_FIXTURE") == "1" {
			values["fixture_kind"] = "mobile-business"
		}
	}
	if values["workbench_fixture"] == true {
		values["fixture_kind"] = "human-business-workbench"
	}
	if values["scope_transfer_fixture"] == true {
		values["fixture_kind"] = "human-file-scope-transfer"
	}
	folder, err := filepath.Abs(filepath.Join("..", "..", "..", ".local"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(folder, "business-browser-fixture.json")
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("owned browser fixture file already exists or cannot be created")
	}
	defer os.Remove(output)
	err = json.NewEncoder(file).Encode(values)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("cannot save owned browser fixture")
	}
	t.Log("isolated native browser fixture ready")
	select {
	case <-stop:
		t.Log("browser released isolated native fixture")
	case <-time.After(12 * time.Minute):
		t.Fatal("browser fixture release timed out")
	}
}
