package spend

import (
	"time"

	"copypaste.hk/platform/internal/fileref"
)

// VarianceBasis pins the original approval and the execution scope actually
// reviewed. Approval of a proposal does not itself replace the execution plan.
type VarianceBasis struct {
	PurchaseID              string            `json:"purchase_id"`
	PurchaseVersion         int64             `json:"purchase_version"`
	OriginalRevisionID      string            `json:"original_revision_id"`
	OriginalContentHash     string            `json:"original_content_hash"`
	ExecutionPlanRevisionID *string           `json:"execution_plan_revision_id"`
	Content                 Content           `json:"content"`
	Amount                  *string           `json:"amount"`
	LineValues              []LineValue       `json:"line_values"`
	Commitments             []FulfillmentLine `json:"commitments"`
}

type Variance struct {
	Record
	Basis       VarianceBasis `json:"basis"`
	CanActivate bool          `json:"can_activate"`
}

type VarianceRevision struct {
	Revision
	Basis VarianceBasis `json:"basis"`
}

type VarianceCreate struct {
	Content
	ExpectedPurchaseVersion int64   `json:"expected_purchase_version"`
	ExecutionPlanRevisionID *string `json:"execution_plan_revision_id"`
	Reason                  string  `json:"reason"`
}

type VariancePatch struct {
	VarianceCreate
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
}

type VarianceActivation struct {
	ExecutionBase
	ExpectedPurchaseVersion int64 `json:"expected_purchase_version"`
}

type ExecutionPlan struct {
	PurchaseID         string             `json:"purchase_id"`
	PurchaseVersion    int64              `json:"purchase_version"`
	VarianceID         *string            `json:"variance_id"`
	VarianceRevisionID *string            `json:"variance_revision_id"`
	VarianceRequestID  *string            `json:"variance_request_id"`
	Content            Content            `json:"content"`
	Amount             *string            `json:"amount"`
	LineValues         []LineValue        `json:"line_values"`
	ActivatedAt        *time.Time         `json:"activated_at"`
	Suppliers          []SupplierEvidence `json:"suppliers"`
	Attachments        []fileref.Evidence `json:"attachments"`
}
