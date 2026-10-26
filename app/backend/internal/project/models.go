package project

import (
	"copypaste.hk/platform/internal/core"
	"time"
)

type Project struct {
	CreatorID           string    `json:"creator_id"`
	ID                  string    `json:"id"`
	Version             int64     `json:"version"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Name                string    `json:"name"`
	Scope               string    `json:"scope"`
	Type                string    `json:"type"`
	OwnerID             string    `json:"owner_id"`
	MemberIDs           []string  `json:"member_ids"`
	StartDate           *string   `json:"start_date,omitempty"`
	DueDate             *string   `json:"due_date,omitempty"`
	Classification      string    `json:"classification"`
	TemplateID          *string   `json:"template_id,omitempty"`
	TemplateVersion     *int64    `json:"template_version,omitempty"`
	ProjectNo           string    `json:"project_no"`
	State               string    `json:"state"`
	InitializationState string    `json:"initialization_state"`
}
type Create struct {
	Name           string   `json:"name"`
	Scope          string   `json:"scope"`
	Type           string   `json:"type"`
	OwnerID        string   `json:"owner_id"`
	MemberIDs      []string `json:"member_ids,omitempty"`
	StartDate      string   `json:"start_date,omitempty"`
	DueDate        string   `json:"due_date,omitempty"`
	Classification string   `json:"classification"`
	TemplateID     *string  `json:"template_id,omitempty"`
}
type Patch struct {
	ExpectedVersion int64                 `json:"expected_version"`
	Name            *string               `json:"name,omitempty"`
	Scope           *string               `json:"scope,omitempty"`
	Type            *string               `json:"type,omitempty"`
	OwnerID         *string               `json:"owner_id,omitempty"`
	StartDate       core.Nullable[string] `json:"start_date"`
	DueDate         core.Nullable[string] `json:"due_date"`
	Classification  *string               `json:"classification,omitempty"`
}
type Transition struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type Task struct {
	CreatorID      string     `json:"creator_id"`
	ID             string     `json:"id"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ProjectID      string     `json:"project_id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	AssigneeID     *string    `json:"assignee_id"`
	ParentID       *string    `json:"parent_id"`
	MilestoneID    *string    `json:"milestone_id"`
	StartAt        *time.Time `json:"start_at,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	Priority       string     `json:"priority"`
	State          string     `json:"state"`
	ReviewRequired bool       `json:"review_required"`
	CompletedAt    *time.Time `json:"-"`
}
type TaskCreate struct {
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	AssigneeID     *string    `json:"assignee_id,omitempty"`
	ParentID       *string    `json:"parent_id,omitempty"`
	MilestoneID    *string    `json:"milestone_id,omitempty"`
	StartAt        *time.Time `json:"start_at,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	Priority       string     `json:"priority"`
	ReviewRequired bool       `json:"review_required"`
}
type TaskPatch struct {
	ExpectedVersion int64                    `json:"expected_version"`
	Title           *string                  `json:"title,omitempty"`
	Body            *string                  `json:"body,omitempty"`
	AssigneeID      core.Nullable[string]    `json:"assignee_id"`
	ParentID        core.Nullable[string]    `json:"parent_id"`
	MilestoneID     core.Nullable[string]    `json:"milestone_id"`
	StartAt         core.Nullable[time.Time] `json:"start_at"`
	DueAt           core.Nullable[time.Time] `json:"due_at"`
	Priority        *string                  `json:"priority,omitempty"`
	ReviewRequired  *bool                    `json:"review_required,omitempty"`
}
type Dependency struct {
	ExpectedVersion int64  `json:"expected_version"`
	PredecessorID   string `json:"predecessor_id"`
	Action          string `json:"action"`
}
type Filter struct {
	State  string
	Query  string
	Cursor string
	Limit  int
}
