package project

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

type Deliverable struct {
	ID                 string    `json:"id"`
	Version            int64     `json:"version"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ProjectID          string    `json:"project_id"`
	CreatorID          *string   `json:"creator_id"`
	Name               string    `json:"name"`
	AcceptanceCriteria string    `json:"acceptance_criteria"`
	OwnerID            string    `json:"owner_id"`
	MilestoneID        *string   `json:"milestone_id"`
	CurrentRevisionID  *string   `json:"current_revision_id"`
	State              string    `json:"state"`
	ProtocolVersion    int       `json:"protocol_version"`
	CanEdit            bool      `json:"can_edit"`
	CanSubmit          bool      `json:"can_submit"`
	CanDecide          bool      `json:"can_decide"`
}
type DeliverableCreate struct {
	Name               string  `json:"name"`
	AcceptanceCriteria string  `json:"acceptance_criteria"`
	OwnerID            string  `json:"owner_id"`
	MilestoneID        *string `json:"milestone_id"`
	Reason             string  `json:"reason"`
}
type DeliverablePatch struct {
	DeliverableCreate
	ExpectedVersion int64 `json:"expected_version"`
}
type DeliverableSubmit struct {
	ExpectedVersion    int64    `json:"expected_version"`
	ExpectedRevisionID *string  `json:"expected_revision_id"`
	FileVersionIDs     []string `json:"file_version_ids"`
	Description        string   `json:"description"`
	Reason             string   `json:"reason"`
}
type DeliverableDecisionInput struct {
	ExpectedVersion  int64  `json:"expected_version"`
	RevisionID       string `json:"revision_id"`
	ContentHash      string `json:"content_hash"`
	Decision         string `json:"decision"`
	FilesVerified    bool   `json:"files_verified"`
	CriteriaVerified bool   `json:"criteria_verified"`
	Reason           string `json:"reason"`
}
type DeliverableDecision struct {
	ID               string    `json:"id"`
	Decision         string    `json:"decision"`
	Reason           string    `json:"reason"`
	DecidedBy        string    `json:"decided_by"`
	DecidedAt        time.Time `json:"decided_at"`
	HeadVersion      int64     `json:"head_version"`
	FilesVerified    bool      `json:"files_verified"`
	CriteriaVerified bool      `json:"criteria_verified"`
}
type DeliverableRevision struct {
	ID                 string               `json:"id"`
	DeliverableID      string               `json:"deliverable_id"`
	Seq                int64                `json:"seq"`
	HeadVersion        int64                `json:"head_version"`
	ProjectVersion     int64                `json:"project_version"`
	Name               string               `json:"name"`
	AcceptanceCriteria string               `json:"acceptance_criteria"`
	OwnerID            string               `json:"owner_id"`
	MilestoneID        *string              `json:"milestone_id"`
	Classification     string               `json:"classification"`
	ContentHash        string               `json:"content_hash"`
	Description        string               `json:"description"`
	Reason             string               `json:"reason"`
	SubmittedBy        string               `json:"submitted_by"`
	SubmittedAt        time.Time            `json:"submitted_at"`
	Files              []fileref.Evidence   `json:"files"`
	Decision           *DeliverableDecision `json:"decision"`
}
type DeliverableReceipt struct {
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	RevisionID *string `json:"revision_id"`
}
