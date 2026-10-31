package notification

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

// Acknowledgement and queue acceptance never establish successful replication.
// Always bind the notice to its exact retained version and current byte proof.
func replicaView(ctx context.Context, tx pgx.Tx, a core.Actor, n thread, v *Notification) error {
	if n.Category != "replica_failure" || n.ContextID == nil {
		return core.Missing()
	}
	f, err := files.NotificationFile(ctx, tx, a, n.ResourceID)
	if err != nil {
		return err
	}
	var version, state, availability string
	var verified bool
	var revision int64
	err = tx.QueryRow(ctx, `SELECT fv.id,r.state,fv.availability,r.version,
        COALESCE(r.actual_sha256=fv.sha256 AND r.expected_sha256=fv.sha256 AND r.verified_at IS NOT NULL,false)
        FROM file_replicas r JOIN file_versions fv ON fv.org_id=r.org_id AND fv.id=r.version_id
        WHERE r.org_id=$1 AND r.id=$2 AND fv.file_id=$3 FOR SHARE OF r,fv`, a.OrgID, *n.ContextID, f.ID).Scan(&version, &state, &availability, &revision, &verified)
	if err != nil {
		return err
	}
	if n.CandidateEventVersion > 0 && (revision != n.CandidateEventVersion ||
		(n.CandidateEventType == "replica.failed" && state != "failed") ||
		(n.CandidateEventType == "replica.verified" && (state != "verified" || !verified))) {
		return core.Missing()
	}
	if availability == "retiring" || availability == "purged" {
		return core.Missing()
	}
	v.ResourceLabel = f.Name
	v.CurrentState = state
	v.AttentionRequired = state != "verified" || !verified
	v.Target.WebPath = "/files/" + f.ID // Older frozen clients retain their safe file entry.
	v.Target.VersionID = &version
	v.Target.ReplicaID = n.ContextID
	switch {
	case !v.AttentionRequired:
		v.Title = "文件副本已恢复并校验"
	case state == "verified":
		v.CurrentState = "replica_unconfirmed"
		v.Title = "文件副本校验证据待核对"
	case state == "queued" || state == "copying" || state == "verifying" || state == "retry_wait":
		v.Title = "文件副本仍待校验完成"
	default:
		v.Title = "文件副本异常"
	}
	return nil
}
