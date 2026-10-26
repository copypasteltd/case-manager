package servicestatus

import (
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB       *pgxpool.Pool
	Commands platform.Commands
	Pager    platform.Pager
}
type Content struct {
	Name          string `json:"name"`
	OwnerID       string `json:"owner_id"`
	HostLabel     string `json:"host_label"`
	EndpointLabel string `json:"endpoint_label"`
	Description   string `json:"description"`
}
type Create struct {
	Content
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}
type Patch struct {
	Content
	ExpectedVersion int64  `json:"expected_version"`
	Lifecycle       string `json:"lifecycle"`
	Reason          string `json:"reason"`
}
type Record struct {
	Content
	ID             string    `json:"id"`
	ProjectID      *string   `json:"project_id"`
	CreatorID      string    `json:"creator_id"`
	Classification string    `json:"classification"`
	Lifecycle      string    `json:"lifecycle"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Summary
	EvaluatedAt       time.Time `json:"evaluated_at"`
	RecheckAt         time.Time `json:"recheck_at"`
	Sources           []Source  `json:"sources"`
	CanManage         bool      `json:"can_manage"`
	CanUpdateIncident bool      `json:"can_update_incident"`
}
type Filter struct {
	Query, Lifecycle, Cursor string
	ProjectID                *string
	Limit                    int
}
type SourceContent struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	ExternalRef string `json:"external_ref"`
	PublicKey   string `json:"public_key"`
	Enabled     bool   `json:"enabled"`
	Thresholds
}
type SourceInput struct {
	SourceContent
	ExpectedVersion       int64   `json:"expected_version"`
	ExpectedSourceVersion int64   `json:"expected_source_version"`
	BaseRevisionID        *string `json:"base_revision_id"`
	Reason                string  `json:"reason"`
}
type Source struct {
	SourceContent
	ID          string      `json:"id"`
	ServiceID   string      `json:"service_id"`
	RevisionID  string      `json:"revision_id"`
	Version     int64       `json:"version"`
	PublishedBy string      `json:"published_by"`
	Reason      string      `json:"reason"`
	CreatedAt   time.Time   `json:"created_at"`
	State       State       `json:"state"`
	StateReason string      `json:"state_reason"`
	Projection  *Projection `json:"projection"`
}
type SourceRevision struct {
	SourceContent
	ID             string    `json:"id"`
	SourceID       string    `json:"source_id"`
	Seq            int64     `json:"seq"`
	BaseRevisionID *string   `json:"base_revision_id"`
	PublishedBy    string    `json:"published_by"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
	IsCurrent      bool      `json:"is_current"`
}
type Observation struct {
	ID            string    `json:"id"`
	SourceID      string    `json:"source_id"`
	RevisionID    string    `json:"source_revision_id"`
	Sequence      string    `json:"sequence"`
	ObservedAt    time.Time `json:"observed_at"`
	ReceivedAt    time.Time `json:"received_at"`
	Healthy       bool      `json:"healthy"`
	LatencyMS     *int64    `json:"latency_ms"`
	PayloadSHA256 string    `json:"payload_sha256"`
	Applied       bool      `json:"applied"`
	IgnoredReason string    `json:"ignored_reason"`
}
type ObservationAck struct {
	SourceID      string `json:"source_id"`
	RevisionID    string `json:"source_revision_id"`
	Sequence      string `json:"sequence"`
	PayloadSHA256 string `json:"payload_sha256"`
	ID            string `json:"id"`
	Applied       bool   `json:"applied"`
	IgnoredReason string `json:"ignored_reason"`
	Replayed      bool   `json:"replayed"`
}
type MaintenanceInput struct {
	ExpectedVersion int64     `json:"expected_version"`
	Start           time.Time `json:"start_at"`
	End             time.Time `json:"end_at"`
	Reason          string    `json:"reason"`
}
type MaintenanceCancel struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type MaintenanceWindow struct {
	ID                 string     `json:"id"`
	ServiceID          string     `json:"service_id"`
	Start              time.Time  `json:"start_at"`
	End                time.Time  `json:"end_at"`
	Reason             string     `json:"reason"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	CancelledBy        *string    `json:"cancelled_by"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	CancellationReason *string    `json:"cancellation_reason"`
}
type IncidentCreate struct {
	Title        string   `json:"title"`
	Severity     string   `json:"severity"`
	Description  string   `json:"description"`
	ServiceIDs   []string `json:"service_ids"`
	OwnerID      string   `json:"owner_id"`
	DedupKey     string   `json:"dedup_key"`
	TimelineNote string   `json:"timeline_note"`
	Reason       string   `json:"reason"`
}
type IncidentPatch struct {
	ExpectedVersion int64  `json:"expected_version"`
	State           string `json:"state"`
	TimelineNote    string `json:"timeline_note"`
	Resolution      string `json:"resolution"`
	Postmortem      string `json:"postmortem"`
	Reason          string `json:"reason"`
}
type Incident struct {
	ID          string     `json:"id"`
	CreatorID   string     `json:"creator_id"`
	OwnerID     string     `json:"owner_id"`
	Title       string     `json:"title"`
	Severity    string     `json:"severity"`
	Description string     `json:"description"`
	ServiceIDs  []string   `json:"service_ids"`
	DedupKey    string     `json:"dedup_key"`
	State       string     `json:"state"`
	Resolution  string     `json:"resolution"`
	Postmortem  string     `json:"postmortem"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	CanUpdate   bool       `json:"can_update"`
}
type IncidentEntry struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	Seq        int64     `json:"seq"`
	State      string    `json:"state"`
	Note       string    `json:"note"`
	Resolution string    `json:"resolution"`
	Postmortem string    `json:"postmortem"`
	ActorID    string    `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}
