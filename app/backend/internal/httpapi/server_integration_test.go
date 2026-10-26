package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contract"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t                                 *testing.T
	owner                             *pgxpool.Pool
	api                               *Server
	handler                           http.Handler
	org, admin, alice, bob, member    string
	aliceToken, bobToken, memberToken string
}
type response struct {
	Data  json.RawMessage `json:"data"`
	Error *core.Error     `json:"error"`
	Meta  meta            `json:"meta"`
}

func setup(t *testing.T) *fixture {
	t.Helper()
	connection := os.Getenv("CP_TEST_ADMIN_URL")
	if connection == "" {
		t.Skip("isolated PostgreSQL not configured; run python app/scripts/test_backend.py")
	}
	if os.Getenv("CP_ENV") != "test" {
		t.Fatal("database integration tests require CP_ENV=test")
	}
	parsed, err := url.Parse(connection)
	if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Path != "/postgres" {
		t.Fatal("integration database must use the explicit local fixture admin")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, connection)
	if err != nil {
		t.Fatal("cannot connect test cluster")
	}
	defer admin.Close(ctx)
	databaseName := "cp_test_" + strings.ReplaceAll(core.NewID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// DROP can wait for a native Windows checkpoint to sync fixture files.
		// Keep durability enabled and bound cleanup independently of assertions.
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		connection, err := pgx.Connect(cleanup, connection)
		if err != nil {
			t.Error("cannot connect for fixture cleanup")
			return
		}
		defer connection.Close(cleanup)
		if !strings.HasPrefix(databaseName, "cp_test_") || len(databaseName) != 40 {
			t.Error("refusing unsafe fixture cleanup")
			return
		}
		if _, err = connection.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	parsed.Path = "/" + databaseName
	if err = migrations.Apply(ctx, parsed.String()); err != nil {
		t.Fatal(err)
	}
	// Reapplying verifies checksums and confirms the migration runner is repeatable.
	if err = migrations.Apply(ctx, parsed.String()); err != nil {
		t.Fatal(err)
	}
	owner, err := platform.OpenDatabase(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	application, err := url.Parse(os.Getenv("CP_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	application.Path = "/" + databaseName
	db, err := platform.OpenDatabase(ctx, application.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	c := platform.Config{Environment: "test", DatabaseURL: application.String(), PublicOrigin: "http://localhost:5178", AllowedOrigins: map[string]bool{"http://localhost:5178": true}, CursorKey: []byte("test-cursor-secret-01234567890123456789"), MFAKey: []byte("01234567890123456789012345678901"), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour}
	ids, err := identity.NewService(db, c)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contract.New()
	if err != nil {
		t.Fatal(err)
	}
	api := &Server{DB: db, Config: c, Identity: ids, Projects: &project.Service{DB: db, Commands: platform.Commands{DB: db}, Pager: platform.Pager{Key: c.CursorKey}}, Contract: validator, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	f := &fixture{t: t, owner: owner, api: api, handler: api.Handler()}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	boot, err := identity.Bootstrap(ctx, tx, identity.BootstrapInput{Organization: "Test CopyPaste", Login: "sysadmin", DisplayName: "系统管理员", Password: "initial-test-password-very-long"})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.org, f.admin = boot.OrganizationID, boot.UserID
	hash, err := identity.HashPassword("member-test-password-long")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		login, role string
		id          *string
	}{{"alice", "project_manager", &f.alice}, {"bob", "project_manager", &f.bob}, {"member", "member", &f.member}} {
		*entry.id = core.NewID()
		if _, err = owner.Exec(ctx, "INSERT INTO users(id,org_id,login_key,display_name,status,credential_hash) VALUES($1,$2,$3,$3,'active',$4)", *entry.id, f.org, entry.login, hash); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type)
            SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, *entry.id, entry.role); err != nil {
			t.Fatal(err)
		}
	}
	f.aliceToken = f.login("alice", "android").AccessToken
	f.bobToken = f.login("bob", "android").AccessToken
	f.memberToken = f.login("member", "android").AccessToken
	return f
}
func (f *fixture) request(method, path, token, key string, body any, headers map[string]string) (*httptest.ResponseRecorder, response) {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	var result response
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		f.t.Fatalf("non-JSON response: %s", rec.Body.String())
	}
	return rec, result
}
func data[T any](t *testing.T, result response) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(result.Data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func expect(t *testing.T, rec *httptest.ResponseRecorder, want int, result response) {
	t.Helper()
	if rec.Code != want {
		code := ""
		if result.Error != nil {
			code = result.Error.Code
		}
		details := ""
		if result.Error != nil {
			details = fmt.Sprint(result.Error.Details)
		}
		t.Fatalf("HTTP %d, want %d (%s), %s", rec.Code, want, code, details)
	}
}
func (f *fixture) login(login, device string) identity.LoginResult {
	f.t.Helper()
	password := "member-test-password-long"
	if login == "sysadmin" {
		password = "initial-test-password-very-long"
	}
	rec, result := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: login, Password: password, Device: device}, map[string]string{"Origin": "http://localhost:5178"})
	expect(f.t, rec, 200, result)
	return data[identity.LoginResult](f.t, result)
}
func (f *fixture) createProject(name string, members []string) project.Project {
	f.t.Helper()
	if members == nil {
		members = []string{}
	}
	rec, result := f.request("POST", "/v1/projects", f.aliceToken, core.NewID(), map[string]any{"name": name, "scope": "交付一套可验收的项目成果", "owner_id": f.alice, "member_ids": members, "classification": "project_restricted"}, nil)
	expect(f.t, rec, 201, result)
	return data[project.Project](f.t, result)
}
func (f *fixture) with(t *testing.T) *fixture { copy := *f; copy.t = t; return &copy }
func (f *fixture) createTask(p project.Project, title string, review bool) project.Task {
	f.t.Helper()
	rec, result := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": title, "priority": "normal", "review_required": review}, nil)
	expect(f.t, rec, 201, result)
	return data[project.Task](f.t, result)
}
func (f *fixture) transition(task project.Task, action, token string) (*httptest.ResponseRecorder, response) {
	return f.request("POST", "/v1/tasks/"+task.ID+"/transition", token, core.NewID(), map[string]any{"expected_version": task.Version, "action": action, "reason": "integration verification"}, nil)
}
func TestPlatformIntegration(t *testing.T) {
	f := setup(t)
	t.Run("application identity has no superuser or audit mutation privileges", func(t *testing.T) {
		f := f.with(t)
		var super, bypass, canDelete bool
		err := f.api.DB.QueryRow(context.Background(), `SELECT rolsuper,rolbypassrls,has_table_privilege(current_user,'audit_events','DELETE') FROM pg_roles WHERE rolname=current_user`).Scan(&super, &bypass, &canDelete)
		if err != nil {
			t.Fatal(err)
		}
		if super || bypass || canDelete {
			t.Fatal("application identity has elevated authority")
		}
	})
	t.Run("resource ACL filters lists and rejects foreign projects before pagination", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Alice restricted project", nil)
		rec, result := f.request("GET", "/v1/projects/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("GET", "/v1/projects?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[project.Project]](t, result)
		if len(page.Items) != 0 || page.NextCursor != nil {
			t.Fatal("foreign resource leaked through listing")
		}
		rec, result = f.request("GET", "/v1/dashboard", f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		dashboard := data[project.Dashboard](t, result)
		if dashboard.ActiveProjects != 0 || dashboard.MyOpenTasks != 0 || len(dashboard.Tasks) != 0 {
			t.Fatal("dashboard leaked foreign work")
		}
		rec, result = f.request("GET", "/v1/projects/not-a-uuid", f.aliceToken, "", nil, nil)
		expect(t, rec, 404, result)
	})
	t.Run("idempotency commits once, detects changed content and reauthorizes replays", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Replay target", []string{f.member})
		key := core.NewID()
		payload := map[string]any{"expected_version": p.Version, "name": "Replay changed"}
		rec, result := f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, key, payload, nil)
		expect(t, rec, 200, result)
		operation := rec.Header().Get("X-Operation-ID")
		if operation == "" {
			t.Fatal("operation receipt missing")
		}
		rec, result = f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, key, payload, nil)
		expect(t, rec, 200, result)
		if rec.Header().Get("X-Idempotent-Replay") != "true" || rec.Header().Get("X-Operation-ID") != operation {
			t.Fatal("replay was not stable")
		}
		payload["name"] = "Different content"
		rec, result = f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, key, payload, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "IDEMPOTENCY_MISMATCH" {
			t.Fatal("wrong conflict")
		}
		payload["name"] = "Replay changed"
		if _, err := f.owner.Exec(context.Background(), "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, p.ID, f.alice); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, key, payload, nil)
		expect(t, rec, 404, result)
		var audits, outbox int
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM audit_events WHERE resource_id=$1 AND action='project.updated'", p.ID).Scan(&audits); err != nil {
			t.Fatal(err)
		}
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='project.updated'", p.ID).Scan(&outbox); err != nil {
			t.Fatal(err)
		}
		if audits != 1 || outbox != 1 {
			t.Fatal("replay duplicated business audit or outbox")
		}
	})
	t.Run("optimistic conflict and failed mutation leave no outbox or idempotency record", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Rollback target", nil)
		key := core.NewID()
		rec, result := f.request("PATCH", "/v1/projects/"+p.ID, f.aliceToken, key, map[string]any{"expected_version": 99, "name": "must not persist"}, nil)
		expect(t, rec, 409, result)
		var count int
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1", key).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("failed command persisted an idempotency result")
		}
		rec, result = f.request("GET", "/v1/projects/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if data[project.Project](t, result).Name != p.Name {
			t.Fatal("failed mutation persisted")
		}
	})
	t.Run("dependency blocking, review and parent cycles enforce a real task lifecycle", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Dependency graph", []string{f.member})
		a, b := f.createTask(p, "Predecessor", false), f.createTask(p, "Reviewable successor", true)
		rec, result := f.request("POST", "/v1/tasks/"+b.ID+"/dependencies", f.aliceToken, core.NewID(), map[string]any{"expected_version": b.Version, "predecessor_id": a.ID, "action": "add"}, nil)
		expect(t, rec, 200, result)
		b = data[project.Task](t, result)
		rec, result = f.transition(b, "start", f.aliceToken)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/tasks/"+a.ID+"/dependencies", f.aliceToken, core.NewID(), map[string]any{"expected_version": a.Version, "predecessor_id": b.ID, "action": "add"}, nil)
		expect(t, rec, 409, result)
		rec, result = f.transition(a, "start", f.aliceToken)
		expect(t, rec, 200, result)
		a = data[project.Task](t, result)
		rec, result = f.transition(a, "complete", f.aliceToken)
		expect(t, rec, 200, result)
		rec, result = f.transition(b, "start", f.aliceToken)
		expect(t, rec, 200, result)
		b = data[project.Task](t, result)
		rec, result = f.transition(b, "complete", f.aliceToken)
		expect(t, rec, 409, result)
		rec, result = f.transition(b, "review", f.aliceToken)
		expect(t, rec, 200, result)
		b = data[project.Task](t, result)
		rec, result = f.transition(b, "complete", f.memberToken)
		expect(t, rec, 404, result)
		rec, result = f.transition(b, "complete", f.aliceToken)
		expect(t, rec, 200, result)
		if data[project.Task](t, result).State != "done" {
			t.Fatal("review did not complete")
		}
		parent, child := f.createTask(p, "Parent", false), f.createTask(p, "Child", false)
		rec, result = f.request("PATCH", "/v1/tasks/"+child.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": child.Version, "parent_id": parent.ID}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("PATCH", "/v1/tasks/"+parent.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": parent.Version, "parent_id": child.ID}, nil)
		expect(t, rec, 409, result)
	})
	t.Run("concurrent opposing edges cannot form a dependency cycle", func(t *testing.T) {
		f := f.with(t)
		p := f.createProject("Concurrent graph", nil)
		a, b := f.createTask(p, "A", false), f.createTask(p, "B", false)
		start := make(chan struct{})
		statuses := make(chan int, 2)
		var group sync.WaitGroup
		for _, pair := range [][2]project.Task{{a, b}, {b, a}} {
			group.Add(1)
			go func(pair [2]project.Task) {
				defer group.Done()
				<-start
				rec, _ := f.request("POST", "/v1/tasks/"+pair[0].ID+"/dependencies", f.aliceToken, core.NewID(), map[string]any{"expected_version": 1, "predecessor_id": pair[1].ID, "action": "add"}, nil)
				statuses <- rec.Code
			}(pair)
		}
		close(start)
		group.Wait()
		close(statuses)
		success, conflict := 0, 0
		for code := range statuses {
			if code == 200 {
				success++
			}
			if code == 409 {
				conflict++
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("concurrent graph outcomes: %d succeeded, %d conflicts", success, conflict)
		}
		var edges int
		if err := f.owner.QueryRow(context.Background(), "SELECT count(*) FROM task_dependencies WHERE project_id=$1", p.ID).Scan(&edges); err != nil {
			t.Fatal(err)
		}
		if edges != 1 {
			t.Fatal("cycle committed")
		}
	})
	t.Run("archived projects remain read only and cannot close with open tasks", func(t *testing.T) {
		// Archive uses real administrator MFA; keep its enrollment independent
		// from the later authentication enrollment and recovery scenarios.
		f := setup(t)
		p := f.createProject("Closure target", nil)
		rec, result := f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "activate", "reason": "ready"}, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		task := f.createTask(p, "Open blocker", false)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "close", "reason": "finish"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.transition(task, "cancel", f.aliceToken)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "close", "reason": "closure verification"}, nil)
		expect(t, rec, 200, result)
		p = data[project.Project](t, result)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "archive", "reason": "legacy direct archive must not bypass review"}, nil)
		expect(t, rec, 422, result)
		if result.Error.Code != "PROJECT_ARCHIVE_REVIEW_REQUIRED" {
			t.Fatal(result.Error)
		}
		archiveProjectFixture(t, f, p.ID)
		rec, result = f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": "forbidden", "priority": "normal", "review_required": false}, nil)
		expect(t, rec, 409, result)
	})
	t.Run("web cookies require CSRF and refresh replay revokes the family", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "bob", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
		expect(t, rec, 200, result)
		login := data[identity.LoginResult](t, result)
		if login.AccessToken != "" || login.RefreshToken != "" {
			t.Fatal("web tokens leaked into JSON")
		}
		cookies := map[string]*http.Cookie{}
		for _, cookie := range rec.Result().Cookies() {
			cookies[cookie.Name] = cookie
		}
		if !cookies["cp_session"].HttpOnly || !cookies["cp_refresh"].HttpOnly {
			t.Fatal("session cookie is not HttpOnly")
		}
		headers := map[string]string{"Origin": "http://localhost:5178", "Cookie": "cp_session=" + cookies["cp_session"].Value}
		rec, result = f.request("POST", "/v1/projects", "", "", map[string]any{"name": "CSRF", "scope": "scope", "owner_id": f.bob, "classification": "internal"}, headers)
		expect(t, rec, 403, result)
		refreshHeaders := map[string]string{"Origin": "http://localhost:5178", "Cookie": "cp_refresh=" + cookies["cp_refresh"].Value, "X-CSRF-Token": login.CSRFToken}
		rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]any{}, refreshHeaders)
		expect(t, rec, 200, result)
		var newAccess string
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == "cp_session" {
				newAccess = cookie.Value
			}
		}
		rec, result = f.request("POST", "/v1/auth/refresh", "", "", map[string]any{}, refreshHeaders)
		expect(t, rec, 401, result)
		rec, result = f.request("GET", "/v1/me", "", "", nil, map[string]string{"Cookie": "cp_session=" + newAccess})
		expect(t, rec, 401, result)
	})
	t.Run("administrator MFA does not grant content and credentials reject old challenges", func(t *testing.T) {
		f := f.with(t)
		challenge := f.login("sysadmin", "android")
		if challenge.Step != "mfa_enroll" {
			t.Fatal("administrator did not require MFA")
		}
		rec, result := f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
		expect(t, rec, 200, result)
		enrollment := data[identity.LoginResult](t, result)
		code := authenticatorCode(enrollment.MFASecret, time.Now())
		rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": code}, nil)
		expect(t, rec, 200, result)
		authenticated := data[identity.LoginResult](t, result)
		if len(authenticated.RecoveryCodes) != 8 {
			t.Fatal("recovery codes missing")
		}
		rec, result = f.request("GET", "/v1/projects", authenticated.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[project.Project]](t, result).Items) != 0 {
			t.Fatal("system administrator received content authority")
		}
		next := f.login("sysadmin", "android")
		rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": next.Challenge, "code": code}, nil)
		expect(t, rec, 401, result)
		recovery := authenticated.RecoveryCodes[0]
		rec, result = f.request("POST", "/v1/auth/mfa/recover", "", "", map[string]string{"challenge": next.Challenge, "code": recovery}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", "/v1/me", authenticated.AccessToken, "", nil, nil)
		expect(t, rec, 401, result)
		next = f.login("sysadmin", "android")
		rec, result = f.request("POST", "/v1/auth/mfa/recover", "", "", map[string]string{"challenge": next.Challenge, "code": recovery}, nil)
		expect(t, rec, 401, result)
		if _, err := f.owner.Exec(context.Background(), "UPDATE users SET auth_version=auth_version+1 WHERE id=$1", f.admin); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": next.Challenge, "code": authenticatorCode(enrollment.MFASecret, time.Now().Add(30*time.Second))}, nil)
		expect(t, rec, 401, result)
	})
	t.Run("schema prevents actor injection, invalid dates and missing required fields", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/projects", f.aliceToken, core.NewID(), map[string]any{"name": "Injected", "scope": "", "owner_id": f.alice, "classification": "internal", "actor_id": f.bob}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/projects", f.aliceToken, core.NewID(), map[string]any{"name": "bad date", "scope": "", "owner_id": f.alice, "classification": "internal", "due_date": "2026-02-30"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/projects", f.aliceToken, core.NewID(), map[string]any{"name": "missing fields"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("GET", "/v1/me", f.aliceToken, "", nil, map[string]string{"Origin": "https://untrusted.example"})
		expect(t, rec, 403, result)
	})
}
func authenticatorCode(secret string, at time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(hash[offset:offset+4])&0x7fffffff)%1000000)
}
