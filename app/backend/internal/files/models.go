package files

import (
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Filter struct {
	CompanyOnly                               bool
	ParentID, ProjectID, Query, State, Cursor string
	Limit                                     int
}
type Folder struct {
	ID             string    `json:"id"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ProjectID      *string   `json:"project_id"`
	ParentID       *string   `json:"parent_id"`
	CreatorID      *string   `json:"creator_id"`
	Name           string    `json:"name"`
	Classification string    `json:"classification"`
	State          string    `json:"state"`
	CanCreate      bool      `json:"can_create"`
	CanUpdate      bool      `json:"can_update"`
	CanUpload      bool      `json:"can_upload"`
	RootScope      string    `json:"-"`
}
type FolderCreate struct {
	ParentID       *string `json:"parent_id,omitempty"`
	ProjectID      *string `json:"project_id,omitempty"`
	Name           string  `json:"name"`
	Classification string  `json:"classification"`
}
type Move struct {
	ExpectedVersion int64   `json:"expected_version"`
	TargetFolderID  *string `json:"target_folder_id,omitempty"`
	NewName         *string `json:"new_name,omitempty"`
	Reason          string  `json:"reason"`
	PlanID          *string `json:"plan_id,omitempty"`
	PlanHash        *string `json:"plan_hash,omitempty"`
}
type File struct {
	ID                     string    `json:"id"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	ProjectID              *string   `json:"project_id"`
	FolderID               string    `json:"folder_id"`
	CreatorID              *string   `json:"creator_id"`
	Name                   string    `json:"name"`
	Classification         string    `json:"classification"`
	CurrentVersionID       *string   `json:"current_version_id"`
	State                  string    `json:"state"`
	LegalHold              bool      `json:"legal_hold"`
	CanUpdate              bool      `json:"can_update"`
	CanMove                bool      `json:"can_move"`
	CanTrash               bool      `json:"can_trash"`
	CanRestore             bool      `json:"can_restore"`
	CanRestoreHistory      bool      `json:"can_restore_history"`
	CanViewHistoryRestores bool      `json:"can_view_history_restores"`
}
type Version struct {
	ID                    string    `json:"id"`
	FileID                string    `json:"file_id"`
	Seq                   int64     `json:"seq"`
	BaseVersionID         *string   `json:"base_version_id"`
	RestoredFromVersionID *string   `json:"restored_from_version_id"`
	OriginalName          *string   `json:"original_name"`
	SizeBytes             string    `json:"size_bytes"`
	SHA256                string    `json:"sha256"`
	MediaType             string    `json:"media_type"`
	Source                string    `json:"source"`
	AuthorID              string    `json:"author_id"`
	ExecutorID            string    `json:"executor_id"`
	CreatedAt             time.Time `json:"created_at"`
	ScanState             string    `json:"scan_state"`
	ScanWaiverRef         *string   `json:"scan_waiver_ref"`
	Availability          string    `json:"availability"`
	ReplicaState          *string   `json:"replica_state"`
	Replicas              []Replica `json:"replicas"`
}
type Replica struct {
	ID           string     `json:"id"`
	Version      int64      `json:"version"`
	Provider     string     `json:"provider"`
	State        string     `json:"state"`
	ActualSHA256 *string    `json:"actual_sha256"`
	VerifiedAt   *time.Time `json:"verified_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ErrorCode    *string    `json:"error_code"`
}
type Restore struct {
	ExpectedVersion int64   `json:"expected_version"`
	Mode            string  `json:"mode"`
	SourceVersionID *string `json:"source_version_id,omitempty"`
	Reason          string  `json:"reason"`
}
type Transition struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
