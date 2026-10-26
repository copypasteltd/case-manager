package spend

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

type CompletionOrderLine struct {
	OrderLineInput
	ID                  string `json:"id"`
	Description         string `json:"description"`
	Unit                string `json:"unit"`
	Amount              string `json:"amount"`
	NetAcceptedQuantity string `json:"net_accepted_quantity"`
}
type CompletionOrder struct {
	ID                      string                `json:"id"`
	PurchaseVersion         int64                 `json:"purchase_version"`
	ExecutionPlanRevisionID *string               `json:"execution_plan_revision_id"`
	ExternalRef             string                `json:"external_ref"`
	SupplierID              string                `json:"supplier_id"`
	SupplierRevisionID      string                `json:"supplier_revision_id"`
	OrderedOn               string                `json:"ordered_on"`
	Amount                  string                `json:"amount"`
	Cancelled               bool                  `json:"cancelled"`
	Lines                   []CompletionOrderLine `json:"lines"`
}
type CompletionReceiptLine struct {
	ReceiptLineInput
	ID string `json:"id"`
}
type CompletionReceipt struct {
	ID              string                  `json:"id"`
	OrderID         string                  `json:"order_id"`
	PurchaseVersion int64                   `json:"purchase_version"`
	ReceivedOn      string                  `json:"received_on"`
	Lines           []CompletionReceiptLine `json:"lines"`
}
type CompletionVariance struct {
	ID                   string `json:"id"`
	Version              int64  `json:"version"`
	RevisionID           string `json:"revision_id"`
	ContentHash          string `json:"content_hash"`
	Title                string `json:"title"`
	ApprovalState        string `json:"approval_state"`
	Lifecycle            string `json:"lifecycle"`
	BasisPurchaseVersion int64  `json:"basis_purchase_version"`
}
type CompletionCost struct {
	ID          string  `json:"id"`
	Version     int64   `json:"version"`
	RevisionID  string  `json:"revision_id"`
	ContentHash string  `json:"content_hash"`
	Title       string  `json:"title"`
	EntryKind   string  `json:"entry_kind"`
	State       string  `json:"state"`
	Currency    *string `json:"currency"`
	Amount      *string `json:"amount"`
	PaidAmount  string  `json:"paid_amount"`
}
type CompletionAllocation struct {
	ID             string `json:"id"`
	CostID         string `json:"cost_id"`
	CostRevisionID string `json:"cost_revision_id"`
	Kind           string `json:"kind"`
	Effect         int    `json:"effect"`
	Amount         string `json:"amount"`
}
type CompletionPayment struct {
	ID                       string                 `json:"id"`
	Version                  int64                  `json:"version"`
	RevisionID               string                 `json:"revision_id"`
	ContentHash              string                 `json:"content_hash"`
	State                    string                 `json:"state"`
	RecordKind               string                 `json:"record_kind"`
	Direction                *string                `json:"direction"`
	Currency                 *string                `json:"currency"`
	Amount                   *string                `json:"amount"`
	RefundOfID               *string                `json:"refund_of_id"`
	UnattributedRefundAmount string                 `json:"unattributed_refund_amount"`
	Allocations              []CompletionAllocation `json:"allocations"`
}
type CompletionSnapshot struct {
	RevisionID              string               `json:"revision_id"`
	ContentHash             string               `json:"content_hash"`
	Currency                string               `json:"currency"`
	ExecutionPlanRevisionID *string              `json:"execution_plan_revision_id"`
	ExecutionAmount         string               `json:"execution_amount"`
	Lines                   []FulfillmentLine    `json:"lines"`
	Orders                  []CompletionOrder    `json:"orders"`
	Receipts                []CompletionReceipt  `json:"receipts"`
	Variances               []CompletionVariance `json:"variances"`
	Costs                   []CompletionCost     `json:"costs"`
	Payments                []CompletionPayment  `json:"payments"`
}
type CompletionPreview struct {
	PurchaseID      string              `json:"purchase_id"`
	PurchaseVersion int64               `json:"purchase_version"`
	Visibility      string              `json:"visibility"`
	Basis           *CompletionSnapshot `json:"basis"`
	Fingerprint     *string             `json:"fingerprint"`
	Blockers        []string            `json:"blockers"`
	HasDifference   bool                `json:"has_difference"`
	SettlementModes []string            `json:"settlement_modes"`
	CanComplete     bool                `json:"can_complete"`
}
type CompletionInput struct {
	ExecutionBase
	ExpectedFingerprint   string   `json:"expected_fingerprint"`
	OccurredOn            string   `json:"occurred_on"`
	DeliveryVerified      bool     `json:"delivery_verified"`
	CostsVerified         bool     `json:"costs_verified"`
	DifferencesVerified   bool     `json:"differences_verified"`
	SettlementMode        string   `json:"settlement_mode"`
	Attestation           string   `json:"attestation"`
	DifferenceDisposition string   `json:"difference_disposition"`
	EvidenceVersionIDs    []string `json:"evidence_version_ids"`
}
type CompletionCommandReceipt struct {
	PurchaseID   string `json:"purchase_id"`
	Version      int64  `json:"version"`
	CompletionID string `json:"completion_id"`
}
type Completion struct {
	ID                    string             `json:"id"`
	PurchaseID            string             `json:"purchase_id"`
	PurchaseVersion       int64              `json:"purchase_version"`
	OccurredOn            string             `json:"occurred_on"`
	Basis                 CompletionSnapshot `json:"basis"`
	Fingerprint           string             `json:"fingerprint"`
	BasisState            string             `json:"basis_state"`
	DeliveryVerified      bool               `json:"delivery_verified"`
	CostsVerified         bool               `json:"costs_verified"`
	DifferencesVerified   bool               `json:"differences_verified"`
	SettlementMode        string             `json:"settlement_mode"`
	Attestation           string             `json:"attestation"`
	DifferenceDisposition string             `json:"difference_disposition"`
	ActorID               string             `json:"actor_id"`
	ActorName             string             `json:"actor_name"`
	Reason                string             `json:"reason"`
	CreatedAt             time.Time          `json:"created_at"`
	Attachments           []fileref.Evidence `json:"attachments"`
}
