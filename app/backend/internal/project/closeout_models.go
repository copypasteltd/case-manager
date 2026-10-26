package project

import (
	"copypaste.hk/platform/internal/fileref"
	"time"
)

// Facts are kept in the immutable database record. Clients receive only the
// authorized checklist, never the raw nested financial/party/file payloads.
type CloseoutItem struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Version  int64  `json:"version"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Blocking bool   `json:"blocking"`
}
type CloseoutGroup struct {
	Kind       string         `json:"kind"`
	Visibility string         `json:"visibility"`
	Count      *int           `json:"count"`
	OpenCount  *int           `json:"open_count"`
	Items      []CloseoutItem `json:"items"`
}
type CloseoutPreview struct {
	Project     Project         `json:"project"`
	AsOf        time.Time       `json:"as_of"`
	Visibility  string          `json:"visibility"`
	Fingerprint *string         `json:"fingerprint"`
	Groups      []CloseoutGroup `json:"groups"`
	CanClose    bool            `json:"can_close"`
	CanArchive  bool            `json:"can_archive"`
}
type ArchiveInput struct {
	ExpectedVersion     int64    `json:"expected_version"`
	ExpectedFingerprint string   `json:"expected_fingerprint"`
	Summary             string   `json:"summary"`
	LessonsLearned      string   `json:"lessons_learned"`
	FollowUp            string   `json:"follow_up"`
	BusinessVerified    bool     `json:"business_verified"`
	FinancialVerified   bool     `json:"financial_verified"`
	FileVersionIDs      []string `json:"file_version_ids"`
	Reason              string   `json:"reason"`
}
type ProjectArchive struct {
	ID                string             `json:"id"`
	ProjectID         string             `json:"project_id"`
	ProjectVersion    int64              `json:"project_version"`
	ProjectName       string             `json:"project_name"`
	Fingerprint       string             `json:"fingerprint"`
	Summary           string             `json:"summary"`
	LessonsLearned    string             `json:"lessons_learned"`
	FollowUp          string             `json:"follow_up"`
	BusinessVerified  bool               `json:"business_verified"`
	FinancialVerified bool               `json:"financial_verified"`
	FileVersionIDs    []string           `json:"file_version_ids"`
	Attachments       []fileref.Evidence `json:"attachments"`
	Groups            []CloseoutGroup    `json:"groups"`
	ActorID           string             `json:"actor_id"`
	ActorName         string             `json:"actor_name"`
	CreatedAt         time.Time          `json:"created_at"`
	Reason            string             `json:"reason"`
	CurrentBasisState string             `json:"current_basis_state"`
}
type ArchiveReceipt struct {
	Project Project        `json:"project"`
	Archive ProjectArchive `json:"archive"`
}
type ArchiveList struct {
	Visibility string           `json:"visibility"`
	Items      []ProjectArchive `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}
