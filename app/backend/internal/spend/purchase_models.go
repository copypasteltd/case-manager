package spend

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

type ExecutionBase struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}
type OrderLineInput struct {
	PurchaseLineID string `json:"purchase_line_id"`
	Quantity       string `json:"quantity"`
	UnitPrice      string `json:"unit_price"`
	TaxAmount      string `json:"tax_amount"`
	DiscountAmount string `json:"discount_amount"`
}
type OrderInput struct {
	ExecutionBase
	ExecutionPlanRevisionID *string          `json:"execution_plan_revision_id"`
	Supplier                Supplier         `json:"supplier"`
	ExternalRef             string           `json:"external_ref"`
	OrderedOn               string           `json:"ordered_on"`
	ExpectedDeliveryOn      string           `json:"expected_delivery_on"`
	EvidenceVersionIDs      []string         `json:"evidence_version_ids"`
	Lines                   []OrderLineInput `json:"lines"`
	Notes                   string           `json:"notes"`
}
type OrderLine struct {
	OrderLineInput
	ID                  string `json:"id"`
	Description         string `json:"description"`
	Unit                string `json:"unit"`
	Amount              string `json:"amount"`
	NetAcceptedQuantity string `json:"net_accepted_quantity"`
	OutstandingQuantity string `json:"outstanding_quantity"`
}
type Order struct {
	ID                      string             `json:"id"`
	PurchaseID              string             `json:"purchase_id"`
	RevisionID              string             `json:"revision_id"`
	PurchaseVersion         int64              `json:"purchase_version"`
	RequestRevisionID       string             `json:"request_revision_id"`
	ExecutionPlanRevisionID *string            `json:"execution_plan_revision_id"`
	Supplier                Supplier           `json:"supplier"`
	SupplierEvidence        *SupplierEvidence  `json:"supplier_evidence"`
	ExternalRef             string             `json:"external_ref"`
	OrderedOn               string             `json:"ordered_on"`
	ExpectedDeliveryOn      string             `json:"expected_delivery_on"`
	Currency                string             `json:"currency"`
	Amount                  string             `json:"amount"`
	EvidenceVersionIDs      []string           `json:"evidence_version_ids"`
	Attachments             []fileref.Evidence `json:"attachments"`
	Lines                   []OrderLine        `json:"lines"`
	Notes                   string             `json:"notes"`
	State                   string             `json:"state"`
	ActorID                 string             `json:"actor_id"`
	Reason                  string             `json:"reason"`
	CreatedAt               time.Time          `json:"created_at"`
}
type ReceiptLineInput struct {
	OrderLineID    string  `json:"order_line_id"`
	Quantity       string  `json:"quantity"`
	Result         string  `json:"result"`
	ReversesLineID *string `json:"reverses_line_id"`
	Reason         string  `json:"reason"`
}
type ReceiptInput struct {
	ExecutionBase
	OrderID            string             `json:"order_id"`
	ReceivedOn         string             `json:"received_on"`
	EvidenceVersionIDs []string           `json:"evidence_version_ids"`
	Lines              []ReceiptLineInput `json:"lines"`
	Notes              string             `json:"notes"`
}
type ReceiptLine struct {
	ReceiptLineInput
	ID                 string `json:"id"`
	ReturnableQuantity string `json:"returnable_quantity"`
}
type Receipt struct {
	ID                 string             `json:"id"`
	PurchaseID         string             `json:"purchase_id"`
	OrderID            string             `json:"order_id"`
	PurchaseVersion    int64              `json:"purchase_version"`
	ReceivedOn         string             `json:"received_on"`
	EvidenceVersionIDs []string           `json:"evidence_version_ids"`
	Attachments        []fileref.Evidence `json:"attachments"`
	Lines              []ReceiptLine      `json:"lines"`
	Notes              string             `json:"notes"`
	ActorID            string             `json:"actor_id"`
	Reason             string             `json:"reason"`
	CreatedAt          time.Time          `json:"created_at"`
}
type ExecutionEvent struct {
	VarianceRevisionID *string   `json:"variance_revision_id"`
	ID                 string    `json:"id"`
	Seq                int64     `json:"seq"`
	PurchaseVersion    int64     `json:"purchase_version"`
	Kind               string    `json:"kind"`
	OrderID            *string   `json:"order_id"`
	ReceiptID          *string   `json:"receipt_id"`
	Lifecycle          string    `json:"lifecycle"`
	ActorID            string    `json:"actor_id"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}
type FulfillmentLine struct {
	LineID              string `json:"line_id"`
	ApprovedQuantity    string `json:"approved_quantity"`
	ReservedQuantity    string `json:"reserved_quantity"`
	NetAcceptedQuantity string `json:"net_accepted_quantity"`
	OutstandingQuantity string `json:"outstanding_quantity"`
	ApprovedAmount      string `json:"approved_amount"`
	ReservedAmount      string `json:"reserved_amount"`
	ReservedTax         string `json:"reserved_tax"`
	ReservedDiscount    string `json:"reserved_discount"`
}
type Fulfillment struct {
	PurchaseID          string            `json:"purchase_id"`
	Version             int64             `json:"version"`
	Lifecycle           string            `json:"lifecycle"`
	Currency            *string           `json:"currency"`
	OrderCount          int64             `json:"order_count"`
	ReceiptCount        int64             `json:"receipt_count"`
	CancelledOrderCount int64             `json:"cancelled_order_count"`
	Lines               []FulfillmentLine `json:"lines"`
}
