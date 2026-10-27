package identity

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BeginInteractiveRead holds the same authorization locks as commands. The
// caller must close this short transaction; external I/O never belongs in it.
func BeginInteractiveRead(ctx context.Context, db *pgxpool.Pool, a core.Actor) (pgx.Tx, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&id); err == nil {
		err = tx.QueryRow(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&id)
	}
	if err == nil {
		err = RequireInteractiveSession(ctx, tx, a, false)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
