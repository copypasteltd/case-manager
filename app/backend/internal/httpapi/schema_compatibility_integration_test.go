package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"copypaste.hk/platform/migrations"
)

func TestCompiledSchemaCompatibility(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := migrations.CheckCompiled(ctx, f.api.DB); err != nil {
		t.Fatal("application cannot inspect public schema manifest", err)
	}
	if _, err := f.api.DB.Exec(ctx, "SELECT * FROM schema_migrations"); err == nil {
		t.Fatal("application received direct migration ledger access")
	}
	readiness := func(want int) {
		t.Helper()
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
		if rec.Code != want {
			t.Fatal("readiness failed to enforce compiled schema", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "schema_migrations") || strings.Contains(rec.Body.String(), "0017_") {
			t.Fatal("public health leaked schema details")
		}
	}
	readiness(200)
	var name, original string
	if err := f.owner.QueryRow(ctx, "SELECT name,sha256 FROM schema_migrations ORDER BY name LIMIT 1").Scan(&name, &original); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "UPDATE schema_migrations SET sha256=$2 WHERE name=$1", name, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	readiness(503)
	if err := migrations.CheckCompiled(ctx, f.api.DB); err == nil {
		t.Fatal("changed applied migration accepted")
	}
	if _, err := f.owner.Exec(ctx, "UPDATE schema_migrations SET sha256=$2 WHERE name=$1", name, original); err != nil {
		t.Fatal(err)
	}
	readiness(200)
	if _, err := f.owner.Exec(ctx, "INSERT INTO schema_migrations(name,sha256) VALUES('9999_future_fixture.sql',$1)", original); err != nil {
		t.Fatal(err)
	}
	readiness(503)
	if _, err := f.owner.Exec(ctx, "DELETE FROM schema_migrations WHERE name='9999_future_fixture.sql'"); err != nil {
		t.Fatal(err)
	}
	readiness(200)
}
