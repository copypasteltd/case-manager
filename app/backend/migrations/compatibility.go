package migrations

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

type manifestReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// CheckCompiled requires an exact migration manifest. This single-host release
// policy rejects stale/mixed binaries instead of discovering drift in a write.
func CheckCompiled(ctx context.Context, db manifestReader) error {
	incompatible := func() error { return core.Fail(503, "SCHEMA_INCOMPATIBLE", "服务版本与数据库结构不一致") }
	names, err := Names()
	if err != nil {
		return incompatible()
	}
	rows, err := db.Query(ctx, "SELECT name,sha256 FROM application_schema_manifest()")
	if err != nil {
		return incompatible()
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var name, sha string
		if err = rows.Scan(&name, &sha); err != nil || index >= len(names) || name != names[index] {
			return incompatible()
		}
		body, err := Files.ReadFile(names[index])
		if err != nil || core.Digest(body) != sha {
			return incompatible()
		}
		index++
	}
	if rows.Err() != nil || index != len(names) {
		return incompatible()
	}
	return nil
}
