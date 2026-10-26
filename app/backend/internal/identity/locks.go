package identity

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Authentication transactions use the same organization-before-user order as
// permission mutations. Otherwise session FK checks can deadlock with a role
// update that already holds the organization and is waiting for the user.
func lockOrganization(ctx context.Context, tx pgx.Tx, org string) error {
	_, err := tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR KEY SHARE", org)
	return err
}
func challengeOrganization(ctx context.Context, tx pgx.Tx, hash string) error {
	var org string
	if err := tx.QueryRow(ctx, "SELECT org_id FROM auth_challenges WHERE token_hash=$1", hash).Scan(&org); err != nil {
		return err
	}
	return lockOrganization(ctx, tx, org)
}
