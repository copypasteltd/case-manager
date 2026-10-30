package files

import "time"

const FileBatchLimit = 50

type FileBatchSelection struct {
	ID               string  `json:"id"`
	ExpectedVersion  int64   `json:"expected_version"`
	CurrentVersionID *string `json:"current_version_id"`
}

type FileBatchTarget struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type FileBatchCreate struct {
	Action string               `json:"action"`
	Files  []FileBatchSelection `json:"files"`
	Target *FileBatchTarget     `json:"target"`
	Reason string               `json:"reason"`
}

type FileBatchDecision struct {
	ExpectedVersion int64  `json:"expected_version"`
	PlanHash        string `json:"plan_hash"`
	Reason          string `json:"reason"`
	Acknowledged    bool   `json:"acknowledged"`
}

// These are metadata references, not a copy or a new formal content version.
type FileBatchFile struct {
	ID               string  `json:"id"`
	Version          int64   `json:"version"`
	Name             string  `json:"name"`
	FolderID         string  `json:"folder_id"`
	ProjectID        *string `json:"project_id"`
	CreatorID        *string `json:"creator_id"`
	Classification   string  `json:"classification"`
	CurrentVersionID *string `json:"current_version_id"`
	State            string  `json:"state"`
}

type FileBatchItem struct {
	File            FileBatchFile       `json:"file"`
	Path            []OrganizationEntry `json:"path"`
	RetentionHeld   bool                `json:"retention_held"`
	BlockingReasons []string            `json:"blocking_reasons"`
}

type FileBatchDestination struct {
	Folder OrganizationEntry   `json:"folder"`
	Path   []OrganizationEntry `json:"path"`
}

type FileBatchSnapshot struct {
	Items  []FileBatchItem       `json:"items"`
	Target *FileBatchDestination `json:"target"`
}

type FileBatchPlan struct {
	ID                  string            `json:"id"`
	Version             int64             `json:"version"`
	State               string            `json:"state"`
	CreatorID           string            `json:"creator_id"`
	Input               FileBatchCreate   `json:"input"`
	Snapshot            FileBatchSnapshot `json:"snapshot"`
	PlanHash            string            `json:"plan_hash"`
	CreatedAt           time.Time         `json:"created_at"`
	ExpiresAt           time.Time         `json:"expires_at"`
	CompletedAt         *time.Time        `json:"completed_at"`
	DecisionReason      *string           `json:"decision_reason"`
	Results             []FileBatchFile   `json:"results"`
	CurrentBasisMatches bool              `json:"current_basis_matches"`
	CanExecute          bool              `json:"can_execute"`
	ActorAuthVersion    int64             `json:"-"`
	OrganizationVersion int64             `json:"-"`
}

func batchFile(f File) FileBatchFile {
	return FileBatchFile{f.ID, f.Version, f.Name, f.FolderID, f.ProjectID, f.CreatorID, f.Classification, f.CurrentVersionID, f.State}
}
