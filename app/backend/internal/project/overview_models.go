package project

import (
	"time"

	"copypaste.hk/platform/internal/finance"
)

// Overview is a read of the caller's current scope, never an archive/closeout
// attestation. Counts exclude inaccessible rows and do not disclose their existence.
type Overview struct {
	ProjectID      string                  `json:"project_id"`
	ProjectVersion int64                   `json:"project_version"`
	AsOf           time.Time               `json:"as_of"`
	Scope          string                  `json:"scope"`
	Groups         []OverviewGroup         `json:"groups"`
	Incurred       finance.ProjectIncurred `json:"incurred"`
	Budgets        []OverviewBudgetTotal   `json:"budgets"`
}

type OverviewItem struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	RevisionID *string   `json:"revision_id"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OverviewState struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

type OverviewGroup struct {
	Kind      string          `json:"kind"`
	Available bool            `json:"available"`
	Count     *int64          `json:"count"`
	States    []OverviewState `json:"states"`
	Items     []OverviewItem  `json:"items"`
	HasMore   bool            `json:"has_more"`
}

type OverviewBudgetTotal struct {
	BudgetID string `json:"budget_id"`
	Revision int64  `json:"revision"`
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}
