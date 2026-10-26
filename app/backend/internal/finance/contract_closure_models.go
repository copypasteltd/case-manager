package finance

import (
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/fileref"
)

type ContractClosureNode struct {
	ID                     string          `json:"id"`
	Title                  string          `json:"title"`
	Direction              string          `json:"direction"`
	Currency               string          `json:"currency"`
	Amount                 string          `json:"amount"`
	CashAmount             string          `json:"cash_amount"`
	HasUnattributedRefunds bool            `json:"has_unattributed_refunds"`
	ReviewID               *string         `json:"review_id"`
	ReviewCurrent          bool            `json:"review_current"`
	ConditionsSatisfied    *bool           `json:"conditions_satisfied"`
	Sources                json.RawMessage `json:"sources"`
}
type ContractClosureCost struct {
	ID                    string `json:"id"`
	Version               int64  `json:"version"`
	RevisionID            string `json:"revision_id"`
	ContentHash           string `json:"content_hash"`
	State                 string `json:"state"`
	Currency              string `json:"currency"`
	Amount                string `json:"amount"`
	PaidAmount            string `json:"paid_amount"`
	AllocationFingerprint string `json:"allocation_fingerprint"`
}
type ContractClosureChild struct {
	ID        string `json:"id"`
	Version   int64  `json:"version"`
	Lifecycle string `json:"lifecycle"`
}
type ContractClosureSnapshot struct {
	RevisionID     string                 `json:"revision_id"`
	FinanceVersion int64                  `json:"finance_version"`
	ScheduleMode   *string                `json:"schedule_mode"`
	Nodes          []ContractClosureNode  `json:"nodes"`
	Costs          []ContractClosureCost  `json:"costs"`
	Children       []ContractClosureChild `json:"children"`
}
type ContractClosurePreview struct {
	ContractID      string                   `json:"contract_id"`
	ContractVersion int64                    `json:"contract_version"`
	Visibility      string                   `json:"visibility"`
	Basis           *ContractClosureSnapshot `json:"basis"`
	Fingerprint     *string                  `json:"fingerprint"`
	Blockers        []string                 `json:"blockers"`
	CanComplete     bool                     `json:"can_complete"`
	CanTerminate    bool                     `json:"can_terminate"`
}
type ContractClosureInput struct {
	ExpectedVersion        int64    `json:"expected_version"`
	RevisionID             string   `json:"revision_id"`
	ExpectedFinanceVersion int64    `json:"expected_finance_version"`
	ExpectedFingerprint    string   `json:"expected_fingerprint"`
	Action                 string   `json:"action"`
	OccurredOn             string   `json:"occurred_on"`
	TermsVerified          bool     `json:"terms_verified"`
	Attestation            string   `json:"attestation"`
	AcknowledgeUnsettled   bool     `json:"acknowledge_unsettled"`
	UnsettledDisposition   string   `json:"unsettled_disposition"`
	EvidenceVersionIDs     []string `json:"evidence_version_ids"`
	Reason                 string   `json:"reason"`
}
type ContractClosureReceipt struct {
	ContractID string `json:"contract_id"`
	Version    int64  `json:"version"`
	ClosureID  string `json:"closure_id"`
	Action     string `json:"action"`
}
type ContractClosure struct {
	ID                   string                  `json:"id"`
	ContractID           string                  `json:"contract_id"`
	ContractVersion      int64                   `json:"contract_version"`
	Action               string                  `json:"action"`
	OccurredOn           string                  `json:"occurred_on"`
	Basis                ContractClosureSnapshot `json:"basis"`
	Fingerprint          string                  `json:"fingerprint"`
	BasisState           string                  `json:"basis_state"`
	TermsVerified        bool                    `json:"terms_verified"`
	Attestation          string                  `json:"attestation"`
	AcknowledgeUnsettled bool                    `json:"acknowledge_unsettled"`
	UnsettledDisposition string                  `json:"unsettled_disposition"`
	ActorID              string                  `json:"actor_id"`
	ActorName            string                  `json:"actor_name"`
	Reason               string                  `json:"reason"`
	CreatedAt            time.Time               `json:"created_at"`
	Attachments          []fileref.Evidence      `json:"attachments"`
}
