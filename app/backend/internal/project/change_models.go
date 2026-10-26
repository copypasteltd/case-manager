package project

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

type BudgetLine struct {
	Category    string `json:"category"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Amount      string `json:"amount"`
}
type ChangeContent struct {
	Title         string  `json:"title"`
	Category      string  `json:"category"`
	Currency      string  `json:"currency"`
	Scope         string  `json:"scope"`
	ScopeDelta    string  `json:"scope_delta"`
	ScheduleDelta string  `json:"schedule_delta"`
	StartDate     *string `json:"start_date"`
	DueDate       *string `json:"due_date"`
	// nil means unchanged; an empty, non-nil collection means clear this currency.
	BudgetLines    []BudgetLine `json:"budget_lines"`
	FileVersionIDs []string     `json:"file_version_ids"`
}
type ChangeBasis struct {
	ProjectID      string       `json:"project_id"`
	ProjectVersion int64        `json:"project_version"`
	Name           string       `json:"name"`
	Scope          string       `json:"scope"`
	StartDate      *string      `json:"start_date"`
	DueDate        *string      `json:"due_date"`
	Classification string       `json:"classification"`
	Currency       string       `json:"currency"`
	BudgetID       *string      `json:"budget_id"`
	BudgetLines    []BudgetLine `json:"budget_lines"`
	BudgetTotal    string       `json:"budget_total"`
}
type ChangeRevision struct {
	ID                   string             `json:"id"`
	ChangeID             string             `json:"change_id"`
	Seq                  int64              `json:"seq"`
	BaseRevisionID       *string            `json:"base_revision_id"`
	PayloadSchemaVersion int                `json:"payload_schema_version"`
	Content              ChangeContent      `json:"content"`
	Basis                ChangeBasis        `json:"basis"`
	ContentHash          string             `json:"content_hash"`
	ApprovalAmount       string             `json:"approval_amount"`
	BudgetDelta          string             `json:"budget_delta"`
	TargetBudgetID       *string            `json:"target_budget_id"`
	AuthorID             string             `json:"author_id"`
	Reason               string             `json:"reason"`
	CreatedAt            time.Time          `json:"created_at"`
	Attachments          []fileref.Evidence `json:"attachments"`
}
type Change struct {
	ID                 string                `json:"id"`
	ProjectID          string                `json:"project_id"`
	CreatorID          string                `json:"creator_id"`
	Classification     string                `json:"classification"`
	State              string                `json:"state"`
	Lifecycle          string                `json:"lifecycle"`
	ApprovalState      string                `json:"approval_state"`
	CurrentRevisionID  string                `json:"current_revision_id"`
	ApprovedRevisionID *string               `json:"approved_revision_id"`
	RequestID          *string               `json:"request_id"`
	Version            int64                 `json:"version"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	Revision           ChangeRevision        `json:"revision"`
	CanEdit            bool                  `json:"can_edit"`
	CanPrepareRequest  bool                  `json:"can_prepare_request"`
	CanImplement       bool                  `json:"can_implement"`
	Implementation     *ChangeImplementation `json:"implementation"`
}
type ChangeImplementation struct {
	ID                   string    `json:"id"`
	ChangeID             string    `json:"change_id"`
	RevisionID           string    `json:"revision_id"`
	RequestRevisionID    string    `json:"request_revision_id"`
	ProjectID            string    `json:"project_id"`
	BeforeProjectVersion int64     `json:"before_project_version"`
	AfterProjectVersion  int64     `json:"after_project_version"`
	ChangeVersion        int64     `json:"change_version"`
	ActorID              string    `json:"actor_id"`
	MFAVerifiedAt        time.Time `json:"mfa_verified_at"`
	ScopeVerified        bool      `json:"scope_verified"`
	BudgetVerified       bool      `json:"budget_verified"`
	Reason               string    `json:"reason"`
	CreatedAt            time.Time `json:"created_at"`
}
type ChangeCreate struct {
	Content            ChangeContent `json:"content"`
	BaseProjectVersion int64         `json:"base_project_version"`
	BaseBudgetID       *string       `json:"base_budget_id"`
	Reason             string        `json:"reason"`
}
type ChangePatch struct {
	ChangeCreate
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
}
type ChangeImplement struct {
	ExpectedVersion    int64  `json:"expected_version"`
	RevisionID         string `json:"revision_id"`
	ContentHash        string `json:"content_hash"`
	RequestRevisionID  string `json:"request_revision_id"`
	BaseProjectVersion int64  `json:"base_project_version"`
	ScopeVerified      bool   `json:"scope_verified"`
	BudgetVerified     bool   `json:"budget_verified"`
	Reason             string `json:"reason"`
}
type ChangeSubmit struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}
type ChangeReceipt struct {
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	RevisionID string `json:"revision_id"`
}
type ProjectBudget struct {
	ID               string       `json:"id"`
	ProjectID        string       `json:"project_id"`
	Revision         int64        `json:"revision"`
	State            string       `json:"state"`
	Currency         string       `json:"currency"`
	CreatorID        string       `json:"creator_id"`
	ChangeID         string       `json:"change_id"`
	ChangeRevisionID string       `json:"change_revision_id"`
	PredecessorID    *string      `json:"predecessor_id"`
	Lines            []BudgetLine `json:"lines"`
	Total            string       `json:"total"`
	CreatedAt        time.Time    `json:"created_at"`
}
