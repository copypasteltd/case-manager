package party

import (
	"time"

	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Content struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Relationships []string `json:"relationships"`
	ContactName   string   `json:"contact_name"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	Notes         string   `json:"notes"`
}
type Create struct {
	Content
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}
type Patch struct {
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	Content
	Reason string `json:"reason"`
}
type Party struct {
	Content
	ID                string    `json:"id"`
	Version           int64     `json:"version"`
	ProjectID         *string   `json:"project_id"`
	CreatorID         string    `json:"creator_id"`
	Classification    string    `json:"classification"`
	State             string    `json:"state"`
	CurrentRevisionID string    `json:"current_revision_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CanEdit           bool      `json:"can_edit"`
	CanArchive        bool      `json:"can_archive"`
	CanReadHistory    bool      `json:"can_read_history"`
}
type Revision struct {
	Content
	ID             string    `json:"id"`
	PartyID        string    `json:"party_id"`
	Seq            int64     `json:"seq"`
	BaseRevisionID *string   `json:"base_revision_id"`
	AuthorID       string    `json:"author_id"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}
type StateInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type Filter struct {
	CompanyOnly                        bool
	Query, State, Relationship, Cursor string
	ProjectID                          *string
	Limit                              int
}
