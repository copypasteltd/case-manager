package files

import "time"

// Scope transfers are explicit human plans. They do not grant permissions,
// move object bytes, or provide an agent with a new business tool.
type ScopeTransferCreate struct {
	Kind                  string  `json:"kind"`
	ResourceID            string  `json:"resource_id"`
	ExpectedVersion       int64   `json:"expected_version"`
	TargetFolderID        string  `json:"target_folder_id"`
	ExpectedTargetVersion int64   `json:"expected_target_version"`
	NewName               *string `json:"new_name"`
	Reason                string  `json:"reason"`
}

type ScopeTransferDecision struct {
	ExpectedVersion int64  `json:"expected_version"`
	PlanHash        string `json:"plan_hash"`
	Reason          string `json:"reason"`
}

type ScopeVisibilityChange struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Channel     string `json:"channel"`
	AI          bool   `json:"ai"`
	Before      bool   `json:"before"`
	After       bool   `json:"after"`
}

type ScopeTransferItem struct {
	Kind              string                  `json:"kind"`
	ID                string                  `json:"id"`
	Version           int64                   `json:"version"`
	Name              string                  `json:"name"`
	State             string                  `json:"state"`
	ParentID          *string                 `json:"parent_id"`
	ProjectID         *string                 `json:"project_id"`
	CreatorID         *string                 `json:"creator_id"`
	Classification    string                  `json:"classification"`
	NewClassification string                  `json:"new_classification"`
	CurrentVersionID  *string                 `json:"current_version_id"`
	ContentVersions   int64                   `json:"content_versions"`
	ContentSHA256     string                  `json:"content_sha256"`
	HumanBefore       int                     `json:"human_before"`
	HumanAfter        int                     `json:"human_after"`
	AIBefore          int                     `json:"ai_before"`
	AIAfter           int                     `json:"ai_after"`
	AudienceSHA256    string                  `json:"audience_sha256"`
	VisibilityChanges []ScopeVisibilityChange `json:"visibility_changes"`
	BlockingReasons   []string                `json:"blocking_reasons"`
}

type ScopeTransferSnapshot struct {
	Source     OrganizationEntry   `json:"source"`
	SourcePath []OrganizationEntry `json:"source_path"`
	Target     OrganizationEntry   `json:"target"`
	TargetPath []OrganizationEntry `json:"target_path"`
	NewName    string              `json:"new_name"`
	Items      []ScopeTransferItem `json:"items"`
}

type ScopeTransfer struct {
	ID                  string                `json:"id"`
	Version             int64                 `json:"version"`
	State               string                `json:"state"`
	CreatorID           string                `json:"creator_id"`
	Input               ScopeTransferCreate   `json:"input"`
	Snapshot            ScopeTransferSnapshot `json:"snapshot"`
	PlanHash            string                `json:"plan_hash"`
	CreatedAt           time.Time             `json:"created_at"`
	ExpiresAt           time.Time             `json:"expires_at"`
	CompletedAt         *time.Time            `json:"completed_at"`
	DecisionReason      *string               `json:"decision_reason"`
	CurrentBasisMatches bool                  `json:"current_basis_matches"`
	CanExecute          bool                  `json:"can_execute"`
	ActorAuthVersion    int64                 `json:"-"`
	OrganizationVersion int64                 `json:"-"`
	OriginalSessionID   string                `json:"-"`
	OriginalChannel     string                `json:"-"`
}
