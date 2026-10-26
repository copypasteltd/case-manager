// Package spend owns purchase and reimbursement documents and their immutable
// commercial revisions. Approval, ordering, actual costs and cash are distinct.
package spend

import (
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Supplier struct {
	PartyID    string `json:"party_id"`
	RevisionID string `json:"revision_id"`
}
type SupplierEvidence struct {
	Supplier
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type Line struct {
	ID                     string    `json:"id"`
	Description            string    `json:"description"`
	Quantity               *string   `json:"quantity"`
	Unit                   string    `json:"unit"`
	UnitPrice              *string   `json:"unit_price"`
	DeclaredAmount         *string   `json:"declared_amount"`
	TaxAmount              *string   `json:"tax_amount"`
	DiscountAmount         *string   `json:"discount_amount"`
	ExpenseDate            *string   `json:"expense_date"`
	Category               string    `json:"category"`
	Supplier               *Supplier `json:"supplier"`
	ReceiptVersionIDs      []string  `json:"receipt_version_ids"`
	MissingReceiptReason   string    `json:"missing_receipt_reason"`
	DuplicateReceiptReason string    `json:"duplicate_receipt_reason"`
	Funding                string    `json:"funding"`
}
type Content struct {
	Title                string     `json:"title"`
	Category             string     `json:"category"`
	Purpose              string     `json:"purpose"`
	OwnerID              *string    `json:"owner_id"`
	Currency             *string    `json:"currency"`
	PricingBasis         string     `json:"pricing_basis"`
	RoundingMode         string     `json:"rounding_mode"`
	ExpectedDeliveryDate *string    `json:"expected_delivery_date"`
	PeriodStart          *string    `json:"period_start"`
	PeriodEnd            *string    `json:"period_end"`
	BudgetReference      string     `json:"budget_reference"`
	SupplierCandidates   []Supplier `json:"supplier_candidates"`
	QuoteVersionIDs      []string   `json:"quote_version_ids"`
	Lines                []Line     `json:"lines"`
	Notes                string     `json:"notes"`
}
type Create struct {
	Content
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}
type Patch struct {
	Content
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	Reason          string `json:"reason"`
}
type ArchiveInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type LineValue struct {
	LineID           string  `json:"line_id"`
	Amount           *string `json:"amount"`
	DuplicateReceipt bool    `json:"duplicate_receipt"`
}
type Record struct {
	Content
	ID                          string             `json:"id"`
	Kind                        string             `json:"kind"`
	DocumentNo                  string             `json:"document_no"`
	ProjectID                   *string            `json:"project_id"`
	CreatorID                   string             `json:"creator_id"`
	Classification              string             `json:"classification"`
	Lifecycle                   string             `json:"lifecycle"`
	ApprovalState               string             `json:"approval_state"`
	CurrentRevisionID           string             `json:"current_revision_id"`
	ApprovedRevisionID          *string            `json:"approved_revision_id"`
	RequestID                   *string            `json:"request_id"`
	Version                     int64              `json:"version"`
	PayloadSchemaVersion        int                `json:"payload_schema_version"`
	ContentHash                 string             `json:"content_hash"`
	Amount                      *string            `json:"amount"`
	EmployeeReimbursementAmount *string            `json:"employee_reimbursement_amount"`
	LineValues                  []LineValue        `json:"line_values"`
	Suppliers                   []SupplierEvidence `json:"suppliers"`
	Attachments                 []fileref.Evidence `json:"attachments"`
	CanEdit                     bool               `json:"can_edit"`
	CanArchive                  bool               `json:"can_archive"`
	CanPrepareRequest           bool               `json:"can_prepare_request"`
	CanReadHistory              bool               `json:"can_read_history"`
	CanExecute                  bool               `json:"can_execute"`
	CanCreateVariance           bool               `json:"can_create_variance"`
	DeletedAt                   *time.Time         `json:"deleted_at"`
	CreatedAt                   time.Time          `json:"created_at"`
	UpdatedAt                   time.Time          `json:"updated_at"`
}
type Revision struct {
	ID             string  `json:"id"`
	DocumentID     string  `json:"document_id"`
	Seq            int64   `json:"seq"`
	BaseRevisionID *string `json:"base_revision_id"`
	Content
	PayloadSchemaVersion        int                `json:"payload_schema_version"`
	ContentHash                 string             `json:"content_hash"`
	Amount                      *string            `json:"amount"`
	EmployeeReimbursementAmount *string            `json:"employee_reimbursement_amount"`
	LineValues                  []LineValue        `json:"line_values"`
	Suppliers                   []SupplierEvidence `json:"suppliers"`
	Attachments                 []fileref.Evidence `json:"attachments"`
	AuthorID                    string             `json:"author_id"`
	Reason                      string             `json:"reason"`
	CreatedAt                   time.Time          `json:"created_at"`
}
type Filter struct {
	Query, ApprovalState, Lifecycle, Cursor string
	ProjectID                               *string
	Limit                                   int
}

func ValidKind(kind string) bool { return kind == "purchase" || kind == "claim" }
