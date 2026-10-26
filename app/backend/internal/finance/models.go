// Package finance owns canonical incurred costs; approval and cash are separate.
package finance

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
type PartyRef struct {
	PartyID    string `json:"party_id"`
	RevisionID string `json:"revision_id"`
}
type PartyEvidence struct {
	PartyRef
	Name string `json:"name"`
}
type Allocation struct {
	ProjectID *string `json:"project_id"`
	Amount    string  `json:"amount"`
}
type Content struct {
	Title              string       `json:"title"`
	OccurredOn         *string      `json:"occurred_on"`
	Currency           *string      `json:"currency"`
	Amount             *string      `json:"amount"`
	Category           string       `json:"category"`
	Counterparty       *PartyRef    `json:"counterparty"`
	Allocations        []Allocation `json:"allocations"`
	EvidenceVersionIDs []string     `json:"evidence_version_ids"`
	Notes              string       `json:"notes"`
}
type Create struct {
	Content
	SourceKey      string  `json:"source_key"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}
type Base struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}
type Patch struct {
	Content
	Base
}
type Reverse struct {
	Base
	EffectiveOn string `json:"effective_on"`
}
type LinkInput struct {
	Base
	SourceType       string `json:"source_type"`
	SourceID         string `json:"source_id"`
	SourceRevisionID string `json:"source_revision_id"`
	Relation         string `json:"relation"`
}
type Link struct {
	ID               string    `json:"id"`
	SourceType       string    `json:"source_type"`
	SourceID         string    `json:"source_id"`
	SourceRevisionID string    `json:"source_revision_id"`
	Relation         string    `json:"relation"`
	Title            *string   `json:"title"`
	Reason           string    `json:"reason"`
	ActorID          string    `json:"actor_id"`
	CreatedAt        time.Time `json:"created_at"`
}
type Cost struct {
	Content
	ID                   string             `json:"id"`
	Version              int64              `json:"version"`
	SourceKey            string             `json:"source_key"`
	ProjectID            *string            `json:"project_id"`
	CreatorID            string             `json:"creator_id"`
	Classification       string             `json:"classification"`
	EntryKind            string             `json:"entry_kind"`
	State                string             `json:"state"`
	ReversesID           *string            `json:"reverses_id"`
	ReversalID           *string            `json:"reversal_id"`
	CurrentRevisionID    string             `json:"current_revision_id"`
	ContentHash          string             `json:"content_hash"`
	BaseCurrency         *string            `json:"base_currency"`
	BaseAmount           *string            `json:"base_amount"`
	CounterpartyEvidence *PartyEvidence     `json:"counterparty_evidence"`
	Attachments          []fileref.Evidence `json:"attachments"`
	Links                []Link             `json:"links"`
	PaidAmount           *string            `json:"paid_amount"`
	OutstandingAmount    *string            `json:"outstanding_amount"`
	SettlementState      *string            `json:"settlement_state"`
	SettlementVisibility string             `json:"settlement_visibility"`
	CanEdit              bool               `json:"can_edit"`
	CanConfirm           bool               `json:"can_confirm"`
	CanReverse           bool               `json:"can_reverse"`
	CanLink              bool               `json:"can_link"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}
type Revision struct {
	Content
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
type Event struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	RevisionID string    `json:"revision_id"`
	Kind       string    `json:"kind"`
	State      string    `json:"state"`
	ActorID    string    `json:"actor_id"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
type Filter struct {
	Query, State, Currency, Category, From, To, Cursor string
	ProjectID                                          *string
	CompanyOnly                                        bool
	Limit                                              int
}
type Total struct {
	Currency       string `json:"currency"`
	ExpenseAmount  string `json:"expense_amount"`
	ReversalAmount string `json:"reversal_amount"`
	NetAmount      string `json:"net_amount"`
	EntryCount     int64  `json:"entry_count"`
}
type Report struct {
	Kind             string    `json:"kind"`
	From             string    `json:"from"`
	To               string    `json:"to"`
	Timezone         string    `json:"timezone"`
	AsOf             time.Time `json:"as_of"`
	BaseCurrency     *string   `json:"base_currency"`
	FXCoverage       string    `json:"fx_coverage"`
	Totals           []Total   `json:"totals"`
	UnconfirmedCount int64     `json:"unconfirmed_count"`
}
