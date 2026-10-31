package finance

import (
	"copypaste.hk/platform/internal/fileref"
	"encoding/json"
	"time"
)

type ContractFinanceBase struct {
	ExpectedVersion int64  `json:"expected_version"`
	ContractVersion int64  `json:"contract_version"`
	Reason          string `json:"reason"`
}
type ContractFinanceLink struct {
	ContractFinanceBase
	MilestoneID          string  `json:"milestone_id"`
	Kind                 string  `json:"kind"`
	SourceID             string  `json:"source_id"`
	SourceVersion        int64   `json:"source_version"`
	SourceRevisionID     string  `json:"source_revision_id"`
	SourceContentHash    string  `json:"source_content_hash"`
	SourceEventID        *string `json:"source_event_id"`
	Amount               string  `json:"amount"`
	RefundFXAcknowledged bool    `json:"refund_fx_acknowledged,omitempty"`
}
type ContractFinanceRelease struct {
	ContractFinanceBase
	Amount string `json:"amount"`
}
type ContractFinanceReview struct {
	ContractFinanceBase
	ExpectedFingerprint            string   `json:"expected_fingerprint"`
	ConditionsSatisfied            bool     `json:"conditions_satisfied"`
	AcknowledgeDifference          bool     `json:"acknowledge_difference"`
	AcknowledgeUnattributedRefunds bool     `json:"acknowledge_unattributed_refunds"`
	EvidenceVersionIDs             []string `json:"evidence_version_ids"`
}
type ContractFinanceReceipt struct {
	ContractID  string `json:"contract_id"`
	MilestoneID string `json:"milestone_id"`
	Version     int64  `json:"version"`
	EventID     string `json:"event_id"`
}
type ContractFinanceNode struct {
	ID                     string  `json:"id"`
	Position               int     `json:"position"`
	Title                  string  `json:"title"`
	Direction              string  `json:"direction"`
	Currency               string  `json:"currency"`
	Amount                 string  `json:"amount"`
	DueDate                string  `json:"due_date"`
	Conditions             string  `json:"conditions"`
	CostAmount             *string `json:"cost_amount"`
	CostVisibility         string  `json:"cost_visibility"`
	CashAmount             *string `json:"cash_amount"`
	CashVisibility         string  `json:"cash_visibility"`
	OutstandingAmount      *string `json:"outstanding_amount"`
	ExcessAmount           *string `json:"excess_amount"`
	HasUnattributedRefunds *bool   `json:"has_unattributed_refunds"`
	Fingerprint            *string `json:"fingerprint"`
	ReviewState            string  `json:"review_state"`
	ReviewID               *string `json:"review_id"`
	ConditionsSatisfied    *bool   `json:"conditions_satisfied"`
	CanReview              bool    `json:"can_review"`
}
type ContractFinance struct {
	ContractID        string                `json:"contract_id"`
	ContractVersion   int64                 `json:"contract_version"`
	ActiveRevisionID  *string               `json:"active_revision_id"`
	ScheduleMode      *string               `json:"schedule_mode"`
	Version           int64                 `json:"version"`
	CanReconcile      bool                  `json:"can_reconcile"`
	HistoryVisibility string                `json:"history_visibility"`
	Nodes             []ContractFinanceNode `json:"nodes"`
}

// RefundCash describes the entire immutable refund cash fact, not a converted
// portion. Event Amount and RemainingAmount always use the milestone currency.
type ContractRefundCash struct {
	Currency string          `json:"currency"`
	Amount   string          `json:"amount"`
	PaidOn   string          `json:"paid_on"`
	RefundFX PaymentRefundFX `json:"refund_fx"`
}

type ContractFinanceEvent struct {
	RefundFXAcknowledged           bool                `json:"refund_fx_acknowledged,omitempty"`
	RefundCash                     *ContractRefundCash `json:"refund_cash,omitempty"`
	ID                             string              `json:"id"`
	ContractID                     string              `json:"contract_id"`
	MilestoneID                    string              `json:"milestone_id"`
	ContractRevisionID             string              `json:"contract_revision_id"`
	Seq                            int64               `json:"seq"`
	Kind                           string              `json:"kind"`
	CostID                         *string             `json:"cost_id"`
	PaymentID                      *string             `json:"payment_id"`
	SourceVersion                  *int64              `json:"source_version"`
	SourceRevisionID               *string             `json:"source_revision_id"`
	SourceContentHash              *string             `json:"source_content_hash"`
	SourceEventID                  *string             `json:"source_event_id"`
	Amount                         *string             `json:"amount"`
	RemainingAmount                *string             `json:"remaining_amount"`
	EffectiveAmount                string              `json:"effective_amount"`
	ReviewSnapshot                 json.RawMessage     `json:"review_snapshot"`
	ReviewFingerprint              *string             `json:"review_fingerprint"`
	ConditionsSatisfied            *bool               `json:"conditions_satisfied"`
	AcknowledgeDifference          *bool               `json:"acknowledge_difference"`
	AcknowledgeUnattributedRefunds *bool               `json:"acknowledge_unattributed_refunds"`
	ActorID                        string              `json:"actor_id"`
	ActorName                      string              `json:"actor_name"`
	Reason                         string              `json:"reason"`
	CreatedAt                      time.Time           `json:"created_at"`
	Attachments                    []fileref.Evidence  `json:"attachments"`
	CanRelease                     bool                `json:"can_release"`
}
