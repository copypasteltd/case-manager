package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// A separate identity owns known-object removal. API processes receive location
// metadata only. R2 removal also requires an explicitly exclusive managed prefix.
type OriginalCleaner interface {
	RemoveFrozen(context.Context, objectstore.Source) (bool, error)
}
type OriginalCleanupBackend struct {
	Provider, Bucket, BindingSHA256 string
	ExclusiveNamespace              bool
	Cleaner                         OriginalCleaner
}
type ObjectRetirementService struct {
	Files    *Service
	Backends map[string]*OriginalCleanupBackend
}
type RetirementCandidate struct {
	Kind            string         `json:"kind"`
	TargetID        string         `json:"target_id"`
	FileID          string         `json:"file_id"`
	FileVersion     int64          `json:"file_version"`
	Name            string         `json:"name"`
	SizeBytes       string         `json:"size_bytes"`
	Seq             int64          `json:"seq"`
	Policy          *RetentionRule `json:"policy"`
	TrashPolicy     *RetentionRule `json:"trash_policy"`
	RetainUntil     *time.Time     `json:"retain_until"`
	SourceSHA256    string         `json:"source_sha256"`
	Eligible        bool           `json:"eligible"`
	BlockingReasons []string       `json:"blocking_reasons"`
}
type RetirementCreate struct {
	ExpectedFileVersion int64  `json:"expected_file_version"`
	Kind                string `json:"kind"`
	TargetID            string `json:"target_id"`
	Reason              string `json:"reason"`
}
type RetirementObject struct {
	Provider           string
	Bucket             string
	BindingSHA256      string
	ExclusiveNamespace bool
	ReplicaID          string
	Source             objectstore.Source
}
type RetirementItem struct {
	ID          string     `json:"id"`
	Provider    string     `json:"provider"`
	Ordinal     int        `json:"ordinal"`
	Version     int64      `json:"version"`
	Generation  int        `json:"generation"`
	State       string     `json:"state"`
	Attempts    int        `json:"attempts"`
	LastError   *string    `json:"last_error"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type RetirementPlan struct {
	ID                                           string              `json:"id"`
	Version                                      int64               `json:"version"`
	State                                        string              `json:"state"`
	Snapshot                                     RetirementCandidate `json:"snapshot"`
	SnapshotSHA256                               string              `json:"snapshot_sha256"`
	Reason                                       string              `json:"reason"`
	CreatedAt                                    time.Time           `json:"created_at"`
	ReadyAfter                                   time.Time           `json:"ready_after"`
	ExpiresAt                                    time.Time           `json:"expires_at"`
	ApprovedAt                                   *time.Time          `json:"approved_at"`
	ApprovalReason                               *string             `json:"approval_reason"`
	CancelledAt                                  *time.Time          `json:"cancelled_at"`
	CancellationReason                           *string             `json:"cancellation_reason"`
	Items                                        []RetirementItem    `json:"items"`
	Objects                                      []RetirementObject  `json:"-"`
	ApprovedBy, ApprovedSession, ApprovedChannel *string             `json:"-"`
	ApprovedAuthVersion, ApprovedOrgVersion      *int64              `json:"-"`
}

const retirementColumns = `id,version,state,snapshot,snapshot_sha256,reason,created_at,ready_after,expires_at,approved_at,approval_reason,cancelled_at,cancellation_reason,objects,approved_by,approved_session_id,approved_channel,approved_auth_version,approved_org_version`
const retirementItemColumns = `id,provider,ordinal,version,generation,state,attempts,last_error,confirmed_at,updated_at`

func retirementItem(row pgx.Row) (RetirementItem, error) {
	var out RetirementItem
	e := row.Scan(&out.ID, &out.Provider, &out.Ordinal, &out.Version, &out.Generation, &out.State, &out.Attempts, &out.LastError, &out.ConfirmedAt, &out.UpdatedAt)
	return out, platform.DatabaseError(e)
}
func retirementPlan(ctx context.Context, db platform.DBTX, org, id string, lock bool) (RetirementPlan, error) {
	out := RetirementPlan{Items: []RetirementItem{}}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	q := "SELECT " + retirementColumns + " FROM object_retirement_plans WHERE org_id=$1 AND id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	var snapshot, objects []byte
	e := db.QueryRow(ctx, q, org, id).Scan(&out.ID, &out.Version, &out.State, &snapshot, &out.SnapshotSHA256, &out.Reason, &out.CreatedAt, &out.ReadyAfter, &out.ExpiresAt, &out.ApprovedAt, &out.ApprovalReason, &out.CancelledAt, &out.CancellationReason, &objects, &out.ApprovedBy, &out.ApprovedSession, &out.ApprovedChannel, &out.ApprovedAuthVersion, &out.ApprovedOrgVersion)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	if e = json.Unmarshal(snapshot, &out.Snapshot); e != nil {
		return out, e
	}
	if e = json.Unmarshal(objects, &out.Objects); e != nil {
		return out, e
	}
	rows, e := db.Query(ctx, "SELECT "+retirementItemColumns+" FROM object_retirement_items WHERE org_id=$1 AND plan_id=$2 ORDER BY ordinal", org, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := retirementItem(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s *ObjectRetirementService) actor(ctx context.Context, db platform.DBTX, a core.Actor, fileID string, mfa bool) (File, error) {
	if e := cleanupChannel(a); e != nil {
		return File{}, e
	}
	if e := currentDownloadActor(ctx, db, a); e != nil {
		return File{}, e
	}
	if mfa {
		if e := interactiveMFA(a); e != nil {
			return File{}, e
		}
	}
	return retentionAccess(ctx, db, a, fileID)
}
func (s *ObjectRetirementService) readPlan(ctx context.Context, db platform.DBTX, a core.Actor, id string, mfa, lock bool) (RetirementPlan, error) {
	p, e := retirementPlan(ctx, db, a.OrgID, id, lock)
	if e != nil {
		return p, e
	}
	_, e = s.actor(ctx, db, a, p.Snapshot.FileID, mfa)
	return p, e
}
func (s *ObjectRetirementService) Plan(ctx context.Context, a core.Actor, id string) (RetirementPlan, error) {
	return s.readPlan(ctx, s.Files.DB, a, id, false, false)
}
func retirementEvent(kind string, p RetirementPlan, reason string) core.Event {
	return core.Event{Type: kind, ResourceType: "object_retirement_plan", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"file_id": p.Snapshot.FileID, "target_id": p.Snapshot.TargetID, "kind": p.Snapshot.Kind, "snapshot_sha256": p.SnapshotSHA256, "reason": reason}}
}
