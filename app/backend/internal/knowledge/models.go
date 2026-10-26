package knowledge

import (
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Filter struct {
	Query, State, ProjectID, Cursor string
	Limit                           int
}
type Source struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	VersionID *string `json:"version_id,omitempty"`
}
type Post struct {
	ID                    string    `json:"id"`
	Version               int64     `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	ProjectID             *string   `json:"project_id"`
	AuthorID              string    `json:"author_id"`
	Type                  string    `json:"type"`
	Title                 string    `json:"title"`
	Body                  string    `json:"body"`
	Summary               string    `json:"summary"`
	Tags                  []string  `json:"tags"`
	Classification        string    `json:"classification"`
	State                 string    `json:"state"`
	CurrentRevisionID     string    `json:"current_revision_id"`
	ContentHash           string    `json:"content_hash"`
	PublishedRevisionID   *string   `json:"published_revision_id"`
	HasUnpublishedChanges bool      `json:"has_unpublished_changes"`
	SourceRefs            []Source  `json:"source_refs"`
	CanEdit               bool      `json:"can_edit"`
	CanReadHistory        bool      `json:"can_read_history"`
	CanPublish            bool      `json:"can_publish"`
	CanComment            bool      `json:"can_comment"`
}
type Create struct {
	ProjectID      *string  `json:"project_id,omitempty"`
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	Summary        string   `json:"summary,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Classification string   `json:"classification"`
	SourceRefs     []Source `json:"source_refs,omitempty"`
}
type Patch struct {
	ExpectedVersion int64                 `json:"expected_version"`
	BaseRevisionID  string                `json:"base_revision_id"`
	ProjectID       core.Nullable[string] `json:"project_id"`
	Type            *string               `json:"type,omitempty"`
	Title           *string               `json:"title,omitempty"`
	Body            *string               `json:"body,omitempty"`
	Summary         *string               `json:"summary,omitempty"`
	Tags            *[]string             `json:"tags,omitempty"`
	Classification  *string               `json:"classification,omitempty"`
	SourceRefs      *[]Source             `json:"source_refs,omitempty"`
}
type Transition struct {
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type Restore struct {
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	RevisionID      string `json:"revision_id"`
	Reason          string `json:"reason"`
}
type Revision struct {
	ID             string    `json:"id"`
	PostID         string    `json:"post_id"`
	Seq            int64     `json:"seq"`
	BaseRevisionID *string   `json:"base_revision_id"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	Summary        string    `json:"summary"`
	Tags           []string  `json:"tags"`
	AuthorID       string    `json:"author_id"`
	CreatedAt      time.Time `json:"created_at"`
	SourceRefs     []Source  `json:"source_refs"`
}
