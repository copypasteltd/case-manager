package contracts

import (
	"time"

	"copypaste.hk/platform/internal/fileref"

	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Content struct {
	PaymentSchedule        *PaymentSchedule `json:"payment_schedule"`
	Title                  string           `json:"title"`
	Direction              *string          `json:"direction"`
	Type                   *string          `json:"type"`
	Category               string           `json:"category"`
	CounterpartyID         *string          `json:"counterparty_id"`
	CounterpartyRevisionID *string          `json:"counterparty_revision_id"`
	OwnerID                *string          `json:"owner_id"`
	Currency               *string          `json:"currency"`
	Amount                 *string          `json:"amount"`
	TaxAmount              *string          `json:"tax_amount"`
	StartDate              *string          `json:"start_date"`
	EndDate                *string          `json:"end_date"`
	RenewalTerms           string           `json:"renewal_terms"`
	TerminationTerms       string           `json:"termination_terms"`
	Notes                  string           `json:"notes"`
	DraftFileVersionID     *string          `json:"draft_file_version_id"`
}
type Create struct {
	Content
	ProjectID        *string `json:"project_id"`
	ParentContractID *string `json:"parent_contract_id"`
	Classification   string  `json:"classification"`
	Reason           string  `json:"reason"`
}
type Patch struct {
	Content
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	Reason          string `json:"reason"`
}
type Signature struct {
	ID                  string            `json:"id"`
	RevisionID          string            `json:"revision_id"`
	RequestID           string            `json:"request_id"`
	RequestRevisionID   string            `json:"request_revision_id"`
	InstanceID          string            `json:"instance_id"`
	SignedOn            string            `json:"signed_on"`
	EffectiveOn         string            `json:"effective_on"`
	SignedFileVersionID string            `json:"signed_file_version_id"`
	SignedFile          *fileref.Evidence `json:"signed_file"`
	Attestation         string            `json:"attestation"`
	Reason              string            `json:"reason"`
	ActorID             string            `json:"actor_id"`
	CreatedAt           time.Time         `json:"created_at"`
}
type SignatureInput struct {
	ExpectedVersion     int64  `json:"expected_version"`
	RevisionID          string `json:"revision_id"`
	ContentHash         string `json:"content_hash"`
	SignedOn            string `json:"signed_on"`
	SignedFileVersionID string `json:"signed_file_version_id"`
	Attestation         string `json:"attestation"`
	Reason              string `json:"reason"`
}
type ActivationInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	SignatureID     string `json:"signature_id"`
	Reason          string `json:"reason"`
}
type Contract struct {
	Content
	PayloadSchemaVersion int               `json:"payload_schema_version"`
	Signature            *Signature        `json:"signature"`
	CanRegisterSignature bool              `json:"can_register_signature"`
	CanActivate          bool              `json:"can_activate"`
	Counterparty         *PartyReference   `json:"counterparty"`
	DraftFile            *fileref.Evidence `json:"draft_file"`
	ID                   string            `json:"id"`
	Version              int64             `json:"version"`
	ContractNo           string            `json:"contract_no"`
	ProjectID            *string           `json:"project_id"`
	CreatorID            string            `json:"creator_id"`
	ParentContractID     *string           `json:"parent_contract_id"`
	Classification       string            `json:"classification"`
	Lifecycle            string            `json:"lifecycle"`
	ApprovalState        string            `json:"approval_state"`
	CurrentRevisionID    string            `json:"current_revision_id"`
	ApprovedRevisionID   *string           `json:"approved_revision_id"`
	ActiveRevisionID     *string           `json:"active_revision_id"`
	RequestID            *string           `json:"request_id"`
	ContentHash          string            `json:"content_hash"`
	DeletedAt            *time.Time        `json:"deleted_at"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	CanEdit              bool              `json:"can_edit"`
	CanPrepareRequest    bool              `json:"can_prepare_request"`
	CanReadHistory       bool              `json:"can_read_history"`
	CanTrash             bool              `json:"can_trash"`
}

type PaymentSchedule struct {
	Mode   string            `json:"mode"`
	Reason string            `json:"reason"`
	Items  []PaymentPlanItem `json:"items"`
}
type PaymentPlanItem struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Amount     *string `json:"amount"`
	DueDate    *string `json:"due_date"`
	Conditions string  `json:"conditions"`
}
type ScheduleInput struct {
	ExpectedVersion int64            `json:"expected_version"`
	BaseRevisionID  string           `json:"base_revision_id"`
	PaymentSchedule *PaymentSchedule `json:"payment_schedule"`
	Reason          string           `json:"reason"`
}
type Milestone struct {
	ID         string    `json:"id"`
	ContractID string    `json:"contract_id"`
	RevisionID string    `json:"revision_id"`
	PlanItemID string    `json:"plan_item_id"`
	Position   int       `json:"position"`
	Title      string    `json:"title"`
	Direction  string    `json:"direction"`
	Currency   string    `json:"currency"`
	Amount     string    `json:"amount"`
	DueDate    string    `json:"due_date"`
	Conditions string    `json:"conditions"`
	CreatedAt  time.Time `json:"created_at"`
}
type Revision struct {
	Content
	Counterparty         *PartyReference   `json:"counterparty"`
	DraftFile            *fileref.Evidence `json:"draft_file"`
	ID                   string            `json:"id"`
	ContractID           string            `json:"contract_id"`
	Seq                  int64             `json:"seq"`
	BaseRevisionID       *string           `json:"base_revision_id"`
	PayloadSchemaVersion int               `json:"payload_schema_version"`
	ContentHash          string            `json:"content_hash"`
	AuthorID             string            `json:"author_id"`
	Reason               string            `json:"reason"`
	CreatedAt            time.Time         `json:"created_at"`
}
type PartyReference struct {
	ID         string `json:"id"`
	RevisionID string `json:"revision_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
}
type Filter struct {
	Query, Lifecycle, Cursor string
	ProjectID                *string
	Limit                    int
	Trashed                  bool
}
type TrashInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
