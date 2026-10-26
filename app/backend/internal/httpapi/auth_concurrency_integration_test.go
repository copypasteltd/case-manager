package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

// Permission mutations lock the organization before their target user. A
// refresh must wait at that same organization lock without holding the user.
func TestRefreshPermissionMutationLockOrder(t *testing.T) {
	f := setup(t)
	login := f.login("alice", "android")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	permission, err := f.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer permission.Rollback(context.Background())
	if _, err = permission.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", f.org); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result identity.LoginResult
		err    error
	}
	finished := make(chan outcome, 1)
	requestID := core.NewID()
	go func() {
		result, err := f.api.Identity.Refresh(ctx, login.RefreshToken, "", false, requestID)
		finished <- outcome{result, err}
	}()
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	waiting := false
	for !waiting {
		if err = f.owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock'
			AND query LIKE 'SELECT id FROM organizations WHERE id=%FOR KEY SHARE')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case result := <-finished:
			t.Fatalf("refresh bypassed the permission lock: %v", result.err)
		case <-deadline.C:
			t.Fatal("refresh did not reach the organization lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err = permission.Exec(ctx, "SELECT id FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE NOWAIT", f.org, f.alice); err != nil {
		t.Fatal("refresh locked a user before its organization", err)
	}
	if err = permission.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	result := <-finished
	if result.err != nil || result.result.AccessToken == "" {
		t.Fatal("refresh did not finish after the permission mutation", result.err)
	}
	var audits int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE request_id=$1 AND action='auth.refresh' AND result='succeeded'", requestID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("refresh and its audit were not committed together", err, audits)
	}
}
