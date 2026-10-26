package finance

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

type PaymentContent struct {
	Title              string    `json:"title"`
	Direction          *string   `json:"direction"`
	PaidAt             *string   `json:"paid_at"`
	Currency           *string   `json:"currency"`
	Amount             *string   `json:"amount"`
	Category           string    `json:"category"`
	Counterparty       *PartyRef `json:"counterparty"`
	EmployeeID         *string   `json:"employee_id"`
	RefundOfID         *string   `json:"refund_of_id"`
	MethodLabel        string    `json:"method_label"`
	ExternalRef        string    `json:"external_ref"`
	EvidenceVersionIDs []string  `json:"evidence_version_ids"`
	Notes              string    `json:"notes"`
}
type PaymentCreate struct {
	PaymentContent
	SourceKey      string  `json:"source_key"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}
type PaymentPatch struct {
	PaymentContent
	Base
}
type RefundAllocation struct {
	SourceAllocationID string `json:"source_allocation_id"`
	Amount             string `json:"amount"`
}
type PaymentRecordInput struct {
	Base
	ExpectedSourceVersion *int64             `json:"expected_source_version"`
	RefundAllocations     []RefundAllocation `json:"refund_allocations"`
}
type PaymentReverseInput struct {
	Base
	EffectiveAt string `json:"effective_at"`
}
type PaymentAllocate struct {
	Base
	CostID          string  `json:"cost_id"`
	CostVersion     int64   `json:"cost_version"`
	CostRevisionID  string  `json:"cost_revision_id"`
	CostContentHash string  `json:"cost_content_hash"`
	Amount          string  `json:"amount"`
	ClaimID         *string `json:"claim_id"`
	ClaimRevisionID *string `json:"claim_revision_id"`
	ClaimLineID     *string `json:"claim_line_id"`
}
type PaymentRelease struct {
	Base
	Amount string `json:"amount"`
}
type PaymentRefundAllocate struct {
	Base
	RefundAllocation
}
type PaymentAllocation struct {
	ID                 string       `json:"id"`
	PaymentID          string       `json:"payment_id"`
	CostID             string       `json:"cost_id"`
	CostRevisionID     string       `json:"cost_revision_id"`
	Kind               string       `json:"kind"`
	Effect             int          `json:"effect"`
	Amount             string       `json:"amount"`
	RootID             string       `json:"root_id"`
	SourceAllocationID *string      `json:"source_allocation_id"`
	ClaimID            *string      `json:"claim_id"`
	ClaimRevisionID    *string      `json:"claim_revision_id"`
	ClaimLineID        *string      `json:"claim_line_id"`
	Shares             []Allocation `json:"shares"`
	RemainingAmount    string       `json:"remaining_amount"`
	ActorID            string       `json:"actor_id"`
	Reason             string       `json:"reason"`
	CreatedAt          time.Time    `json:"created_at"`
}
type Payment struct {
	PaymentContent
	ID                           string             `json:"id"`
	Version                      int64              `json:"version"`
	SourceKey                    string             `json:"source_key"`
	ProjectID                    *string            `json:"project_id"`
	CreatorID                    string             `json:"creator_id"`
	Classification               string             `json:"classification"`
	RecordKind                   string             `json:"record_kind"`
	State                        string             `json:"state"`
	ReversesID                   *string            `json:"reverses_id"`
	ReversalID                   *string            `json:"reversal_id"`
	CurrentRevisionID            string             `json:"current_revision_id"`
	ContentHash                  string             `json:"content_hash"`
	PaidOn                       *string            `json:"paid_on"`
	CounterpartyEvidence         *PartyEvidence     `json:"counterparty_evidence"`
	Attachments                  []fileref.Evidence `json:"attachments"`
	EffectiveAllocatedAmount     string             `json:"effective_allocated_amount"`
	RefundedAmount               string             `json:"refunded_amount"`
	ContractReturnReservedAmount string             `json:"contract_return_reserved_amount"`
	UnallocatedAmount            *string            `json:"unallocated_amount"`
	CanEdit                      bool               `json:"can_edit"`
	CanRecord                    bool               `json:"can_record"`
	CanAllocate                  bool               `json:"can_allocate"`
	CanReverse                   bool               `json:"can_reverse"`
	CreatedAt                    time.Time          `json:"created_at"`
	UpdatedAt                    time.Time          `json:"updated_at"`
}
type PaymentRevision struct {
	PaymentContent
	ID                   string             `json:"id"`
	Seq                  int64              `json:"seq"`
	BaseRevisionID       *string            `json:"base_revision_id"`
	ContentHash          string             `json:"content_hash"`
	AuthorID             string             `json:"author_id"`
	Reason               string             `json:"reason"`
	CreatedAt            time.Time          `json:"created_at"`
	CounterpartyEvidence *PartyEvidence     `json:"counterparty_evidence"`
	Attachments          []fileref.Evidence `json:"attachments"`
}
type PaymentEvent struct {
	Event
	AllocationID *string `json:"allocation_id"`
}
type ClaimSettlement struct {
	ClaimID           string  `json:"claim_id"`
	RevisionID        *string `json:"revision_id"`
	Currency          *string `json:"currency"`
	EligibleAmount    *string `json:"eligible_amount"`
	PaidAmount        *string `json:"paid_amount"`
	OutstandingAmount *string `json:"outstanding_amount"`
	State             *string `json:"state"`
	Visibility        string  `json:"visibility"`
}
type PaymentFilter struct {
	Query, State, Direction, Currency, Category, From, To, Cursor string
	ProjectID                                                     *string
	CompanyOnly                                                   bool
	Limit                                                         int
}
type CashTotal struct {
	Currency          string `json:"currency"`
	OutgoingAmount    string `json:"outgoing_amount"`
	RefundAmount      string `json:"refund_amount"`
	CorrectionAmount  string `json:"correction_amount"`
	NetAmount         string `json:"net_amount"`
	UnallocatedAmount string `json:"unallocated_amount"`
	EntryCount        int64  `json:"entry_count"`
}
type CashReport struct {
	Kind             string      `json:"kind"`
	From             string      `json:"from"`
	To               string      `json:"to"`
	Timezone         string      `json:"timezone"`
	AsOf             time.Time   `json:"as_of"`
	BaseCurrency     *string     `json:"base_currency"`
	FXCoverage       string      `json:"fx_coverage"`
	Totals           []CashTotal `json:"totals"`
	UnconfirmedCount int64       `json:"unconfirmed_count"`
}
