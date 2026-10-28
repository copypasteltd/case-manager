package files

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// The cleaner receives only exact multipart control, never object deletion.
type MultipartCleaner interface {
	Abort(context.Context, objectstore.Multipart) error
	MultipartAbsent(context.Context, objectstore.Multipart) (bool, error)
}
type CleanupBackend struct {
	Provider, Bucket, BindingSHA256 string
	Cleaner                         MultipartCleaner
}
type MultipartCleanupService struct {
	Files    *Service
	Backends map[string]*CleanupBackend
}
type CleanupCandidate struct {
	UploadID        string         `json:"upload_id"`
	Version         int64          `json:"version"`
	Name            string         `json:"name"`
	SizeBytes       string         `json:"size_bytes"`
	State           string         `json:"state"`
	CreatedAt       time.Time      `json:"created_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
	RetainUntil     *time.Time     `json:"retain_until"`
	Policy          *RetentionRule `json:"policy"`
	SourceSHA256    string         `json:"source_sha256"`
	BackendSHA256   string         `json:"backend_sha256"`
	Eligible        bool           `json:"eligible"`
	BlockingReasons []string       `json:"blocking_reasons"`
}
type CleanupSelection struct {
	UploadID        string `json:"upload_id"`
	ExpectedVersion int64  `json:"expected_version"`
}
type CleanupCreate struct {
	Items  []CleanupSelection `json:"items"`
	Reason string             `json:"reason"`
}
type CleanupDecision struct {
	ExpectedVersion int64  `json:"expected_version"`
	SnapshotSHA256  string `json:"snapshot_sha256"`
	Reason          string `json:"reason"`
}
type CleanupSnapshot struct {
	FolderID      string             `json:"folder_id"`
	FolderVersion int64              `json:"folder_version"`
	Items         []CleanupCandidate `json:"items"`
}
type CleanupItem struct {
	ID                    string     `json:"id"`
	UploadID              string     `json:"upload_id"`
	Version               int64      `json:"version"`
	Generation            int        `json:"generation"`
	State                 string     `json:"state"`
	Attempts              int        `json:"attempts"`
	LastError             *string    `json:"last_error"`
	ConfirmedAt           *time.Time `json:"confirmed_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ApprovedUploadVersion *int64     `json:"-"`
}
type CleanupPlan struct {
	ID                                           string          `json:"id"`
	Version                                      int64           `json:"version"`
	State                                        string          `json:"state"`
	Snapshot                                     CleanupSnapshot `json:"snapshot"`
	SnapshotSHA256                               string          `json:"snapshot_sha256"`
	Reason                                       string          `json:"reason"`
	CreatedAt                                    time.Time       `json:"created_at"`
	ReadyAfter                                   time.Time       `json:"ready_after"`
	ExpiresAt                                    time.Time       `json:"expires_at"`
	ApprovedAt                                   *time.Time      `json:"approved_at"`
	ApprovalReason                               *string         `json:"approval_reason"`
	CancelledAt                                  *time.Time      `json:"cancelled_at"`
	CancellationReason                           *string         `json:"cancellation_reason"`
	Items                                        []CleanupItem   `json:"items"`
	ApprovedBy, ApprovedSession, ApprovedChannel *string         `json:"-"`
	ApprovedAuthVersion, ApprovedOrgVersion      *int64          `json:"-"`
}
type CleanupReceipt struct {
	ID         string    `json:"id"`
	Generation int       `json:"generation"`
	Attempt    int       `json:"attempt"`
	Outcome    string    `json:"outcome"`
	ErrorCode  *string   `json:"error_code"`
	CreatedAt  time.Time `json:"created_at"`
}

const cleanupPlanColumns = `id,version,state,snapshot,snapshot_sha256,reason,created_at,ready_after,expires_at,
 approved_at,approval_reason,cancelled_at,cancellation_reason,approved_by,approved_session_id,approved_channel,approved_auth_version,approved_org_version`
const cleanupItemColumns = `id,upload_id,version,generation,state,attempts,last_error,confirmed_at,updated_at,approved_upload_version`

func cleanupChannel(a core.Actor) error {
	if a.RunID != "" || !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) {
		return core.Forbidden()
	}
	return nil
}
func cleanupFolder(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Folder, error) {
	f, e := authorizeFolder(ctx, db, a, id, "folder.read")
	if e != nil {
		return f, e
	}
	return f, authz.Require(ctx, db, a, "file.retention.manage", folderResource(f))
}
func (s *MultipartCleanupService) actor(ctx context.Context, db platform.DBTX, a core.Actor, folderID string, mfa bool) (Folder, error) {
	if e := cleanupChannel(a); e != nil {
		return Folder{}, e
	}
	if mfa {
		if e := interactiveMFA(a); e != nil {
			return Folder{}, e
		}
	}
	if e := currentDownloadActor(ctx, db, a); e != nil {
		return Folder{}, e
	}
	return cleanupFolder(ctx, db, a, folderID)
}
func cleanupUploadAccess(ctx context.Context, db platform.DBTX, a core.Actor, f Folder, u Upload) error {
	if u.FolderID != f.ID || u.Classification == nil || *u.Classification != f.Classification || u.SourceChannel == nil || !slices.Contains([]string{"web", "android"}, *u.SourceChannel) {
		return core.Missing()
	}
	if u.FileID != nil {
		target, e := retentionAccess(ctx, db, a, *u.FileID)
		if e != nil {
			return e
		}
		if target.FolderID != f.ID || target.Classification != f.Classification {
			return core.Missing()
		}
	}
	return nil
}
func cleanupSource(u Upload) string {
	raw, _ := json.Marshal([]any{u.ID, u.FolderID, u.FileID, u.BaseVersionID, u.Provider, u.Bucket, u.UploadID, u.DraftKey, u.StorageEpoch, u.SourceChannel, u.Classification, u.SourceFingerprint, u.OwnerID, u.Name, u.SizeBytes, u.MediaType, u.CreatedAt, u.ExpiresAt})
	return core.Digest(raw)
}
func (s *MultipartCleanupService) assess(ctx context.Context, db platform.DBTX, org string, u Upload, ownItem string) (CleanupCandidate, error) {
	out := CleanupCandidate{UploadID: u.ID, Version: u.Version, Name: u.Name, SizeBytes: u.SizeBytes, State: u.State, CreatedAt: u.CreatedAt, ExpiresAt: u.ExpiresAt, SourceSHA256: cleanupSource(u), BlockingReasons: []string{}}
	var now time.Time
	if e := db.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return out, e
	}
	p, e := scanRetentionPolicy(db.QueryRow(ctx, "SELECT "+retentionPolicyColumns+" FROM retention_policies WHERE org_id=$1 AND kind='multipart' ORDER BY version DESC LIMIT 1", org))
	if e != nil {
		if core.PublicError(e).Status != 404 {
			return out, e
		}
		out.BlockingReasons = append(out.BlockingReasons, "POLICY_REQUIRED")
	} else {
		out.Policy = &RetentionRule{ID: p.ID, Version: p.Version, Kind: p.Kind, Enabled: p.Enabled, RetainSeconds: p.RetainSeconds, ObservationSeconds: p.ObservationSeconds}
		until := u.CreatedAt.Add(time.Duration(p.RetainSeconds) * time.Second)
		out.RetainUntil = &until
		if !p.Enabled {
			out.BlockingReasons = append(out.BlockingReasons, "POLICY_DISABLED")
		}
		if now.Before(until) {
			out.BlockingReasons = append(out.BlockingReasons, "RETENTION_PERIOD_ACTIVE")
		}
	}
	if !slices.Contains([]string{"uploading", "aborted", "expired", "failed"}, u.State) {
		out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_STATE_UNSUPPORTED")
	}
	if now.Before(u.ExpiresAt.Add(objectstore.MaxSignTTL)) {
		out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_OR_PERMIT_ACTIVE")
	}
	if u.UploadID == nil || *u.UploadID == "" {
		out.BlockingReasons = append(out.BlockingReasons, "MULTIPART_ID_REQUIRED")
	}
	if u.ResultVersionID != nil || u.FrozenSourceID != nil || len(u.Manifest) > 0 {
		out.BlockingReasons = append(out.BlockingReasons, "FINALIZATION_OR_FORMAL_CONTENT")
	}
	var attemptID string
	e = db.QueryRow(ctx, `SELECT id FROM upload_attempts WHERE org_id=$1 AND session_id=$2
 AND draft_key=$3 AND provider_upload_id=$4 AND state='active'`, org, u.ID, u.DraftKey, u.UploadID).Scan(&attemptID)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	key, keyError := objectstore.DraftKey(org, attemptID)
	if e == pgx.ErrNoRows || keyError != nil || key != u.DraftKey {
		out.BlockingReasons = append(out.BlockingReasons, "SOURCE_CHANGED")
	}
	if u.Provider == nil || u.Bucket == nil {
		out.BlockingReasons = append(out.BlockingReasons, "CLEANUP_BACKEND_UNAVAILABLE")
	} else {
		b := s.Backends[*u.Provider]
		if b == nil || b.Provider != *u.Provider || b.Bucket != *u.Bucket || len(b.BindingSHA256) != 64 {
			out.BlockingReasons = append(out.BlockingReasons, "CLEANUP_BACKEND_UNAVAILABLE")
		} else {
			out.BackendSHA256 = b.BindingSHA256
		}
	}
	var busy, reserved bool
	e = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE org_id=$1 AND payload->>'session_id'=$2 AND kind LIKE 'upload.%' AND state IN ('queued','running','retry_wait')),
 EXISTS(SELECT 1 FROM upload_cleanup_items WHERE org_id=$1 AND upload_id=$2::uuid AND id::text<>$3 AND state IN ('queued','running','retry_wait','succeeded'))`, org, u.ID, ownItem).Scan(&busy, &reserved)
	if e != nil {
		return out, e
	}
	if busy {
		out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_JOB_ACTIVE")
	}
	if reserved {
		out.BlockingReasons = append(out.BlockingReasons, "CLEANUP_RESERVED_OR_CONFIRMED")
	}
	if u.FileID != nil {
		held, e := retentionHeld(ctx, db, org, *u.FileID)
		if e != nil {
			return out, e
		}
		if held {
			out.BlockingReasons = append(out.BlockingReasons, "RETENTION_HOLD")
		}
	}
	out.Eligible = len(out.BlockingReasons) == 0
	return out, nil
}

func scanCleanupItem(row pgx.Row) (CleanupItem, error) {
	var v CleanupItem
	e := row.Scan(&v.ID, &v.UploadID, &v.Version, &v.Generation, &v.State, &v.Attempts, &v.LastError, &v.ConfirmedAt, &v.UpdatedAt, &v.ApprovedUploadVersion)
	return v, platform.DatabaseError(e)
}
func cleanupPlan(ctx context.Context, db platform.DBTX, org, id string, lock bool) (CleanupPlan, error) {
	out := CleanupPlan{Items: []CleanupItem{}}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	q := "SELECT " + cleanupPlanColumns + " FROM upload_cleanup_plans WHERE org_id=$1 AND id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	var raw []byte
	e := db.QueryRow(ctx, q, org, id).Scan(&out.ID, &out.Version, &out.State, &raw, &out.SnapshotSHA256, &out.Reason, &out.CreatedAt, &out.ReadyAfter, &out.ExpiresAt, &out.ApprovedAt, &out.ApprovalReason, &out.CancelledAt, &out.CancellationReason, &out.ApprovedBy, &out.ApprovedSession, &out.ApprovedChannel, &out.ApprovedAuthVersion, &out.ApprovedOrgVersion)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	if e = json.Unmarshal(raw, &out.Snapshot); e != nil {
		return out, e
	}
	rows, e := db.Query(ctx, "SELECT "+cleanupItemColumns+" FROM upload_cleanup_items WHERE org_id=$1 AND plan_id=$2 ORDER BY upload_id", org, id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scanCleanupItem(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
func (s *MultipartCleanupService) readPlan(ctx context.Context, db platform.DBTX, a core.Actor, id string, mfa, lock bool) (CleanupPlan, error) {
	p, e := cleanupPlan(ctx, db, a.OrgID, id, lock)
	if e != nil {
		return p, e
	}
	f, e := s.actor(ctx, db, a, p.Snapshot.FolderID, mfa)
	if e != nil {
		return p, e
	}
	for _, item := range p.Items {
		u, e := upload(ctx, db, a.OrgID, item.UploadID, false)
		if e != nil {
			return p, e
		}
		if e = cleanupUploadAccess(ctx, db, a, f, u); e != nil {
			return p, e
		}
	}
	return p, nil
}
func (s *MultipartCleanupService) Plan(ctx context.Context, a core.Actor, id string) (CleanupPlan, error) {
	return s.readPlan(ctx, s.Files.DB, a, id, false, false)
}
func cleanupEvent(kind string, p CleanupPlan, reason string) core.Event {
	return core.Event{Type: kind, ResourceType: "upload_cleanup_plan", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"folder_id": p.Snapshot.FolderID, "snapshot_sha256": p.SnapshotSHA256, "item_count": len(p.Items), "reason": reason}}
}
func (s *MultipartCleanupService) queue() jobqueue.Queue { return jobqueue.Queue{DB: s.Files.DB} }
