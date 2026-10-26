package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

//go:embed *.sql
var Files embed.FS

func Apply(ctx context.Context, connection string) error {
	conn, err := pgx.Connect(ctx, connection)
	if err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	defer conn.Close(ctx)
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(731924801)"); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(731924801)")
	_, err = conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY,sha256 text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())")
	if err != nil {
		return err
	}
	names, err := fs.Glob(Files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := Files.ReadFile(name)
		if err != nil {
			return err
		}
		var stored string
		err = conn.QueryRow(ctx, "SELECT sha256 FROM schema_migrations WHERE name=$1", name).Scan(&stored)
		if err == nil {
			if stored != core.Digest(body) {
				return fmt.Errorf("applied migration changed: %s", name)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, string(body))
		if err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO schema_migrations(name,sha256) VALUES($1,$2)", name, core.Digest(body))
		}
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
func Names() ([]string, error) {
	names, err := fs.Glob(Files, "*.sql")
	sort.Strings(names)
	return names, err
}
func IsSQL(name string) bool { return strings.HasSuffix(name, ".sql") }
