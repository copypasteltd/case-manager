package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/spend"
	"github.com/jackc/pgx/v5"
)

type Snapshot struct {
	ProjectChangeBudgetDelta     *string                   `json:"project_change_budget_delta,omitempty"`
	ProjectChangeBasis           *project.ChangeBasis      `json:"project_change_basis,omitempty"`
	PurchaseVariance             *spend.VarianceBasis      `json:"purchase_variance,omitempty"`
	BusinessPayloadSchemaVersion int                       `json:"business_payload_schema_version,omitempty"`
	Kind                         string                    `json:"kind"`
	BusinessID                   string                    `json:"business_id"`
	BusinessRevisionID           string                    `json:"business_revision_id"`
	BusinessContentHash          string                    `json:"business_content_hash"`
	ContractNo                   string                    `json:"contract_no"`
	ProjectID                    *string                   `json:"project_id"`
	Classification               string                    `json:"classification"`
	Content                      json.RawMessage           `json:"content"`
	Counterparty                 *contracts.PartyReference `json:"counterparty"`
	Attachments                  []fileref.Evidence        `json:"attachments"`
	DocumentNo                   string                    `json:"document_no,omitempty"`
	Amount                       *string                   `json:"amount,omitempty"`
	EmployeeReimbursementAmount  *string                   `json:"employee_reimbursement_amount,omitempty"`
	Suppliers                    []spend.SupplierEvidence  `json:"suppliers,omitempty"`
	LineValues                   []spend.LineValue         `json:"line_values,omitempty"`
}

func (p Snapshot) ContractContent() contracts.Content {
	var c contracts.Content
	_ = json.Unmarshal(p.Content, &c)
	return c
}

type Request struct {
	ID                string         `json:"id"`
	Version           int64          `json:"version"`
	Title             string         `json:"title"`
	Kind              string         `json:"kind"`
	BusinessID        string         `json:"business_id"`
	ApplicantID       string         `json:"applicant_id"`
	ProjectID         *string        `json:"project_id"`
	Classification    string         `json:"classification"`
	State             string         `json:"state"`
	CurrentRevisionID string         `json:"current_revision_id"`
	ActiveInstanceID  *string        `json:"active_instance_id"`
	ContentHash       string         `json:"content_hash"`
	Payload           Snapshot       `json:"payload"`
	Workflow          *Definition    `json:"workflow"`
	Steps             []ApprovalStep `json:"steps"`
	CanRevise         bool           `json:"can_revise"`
	CanSubmit         bool           `json:"can_submit"`
	CanWithdraw       bool           `json:"can_withdraw"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
type ApprovalStep struct {
	ID                 string     `json:"id"`
	Version            int64      `json:"version"`
	RequestID          string     `json:"request_id"`
	RequestVersion     int64      `json:"request_version"`
	RequestTitle       string     `json:"request_title"`
	RevisionID         string     `json:"revision_id"`
	InstanceID         string     `json:"instance_id"`
	Seq                int        `json:"seq"`
	Title              string     `json:"title"`
	AssignedUserID     string     `json:"assigned_user_id"`
	AssignedUserName   string     `json:"assigned_user_name"`
	AssignedUserActive bool       `json:"assigned_user_active"`
	RequiresMFA        bool       `json:"requires_mfa"`
	State              string     `json:"state"`
	Decision           *string    `json:"decision"`
	DecisionReason     *string    `json:"decision_reason"`
	DecidedAt          *time.Time `json:"decided_at"`
	CanDecide          bool       `json:"can_decide"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
type Revision struct {
	ID                   string         `json:"id"`
	RequestID            string         `json:"request_id"`
	Seq                  int64          `json:"seq"`
	BaseRevisionID       *string        `json:"base_revision_id"`
	PayloadSchemaVersion int            `json:"payload_schema_version"`
	Payload              Snapshot       `json:"payload"`
	Workflow             *Definition    `json:"workflow"`
	ContentHash          string         `json:"content_hash"`
	Steps                []ApprovalStep `json:"steps"`
	AuthorID             string         `json:"author_id"`
	Reason               string         `json:"reason"`
	CreatedAt            time.Time      `json:"created_at"`
}
type RequestCreate struct {
	Kind                    string `json:"kind"`
	BusinessID              string `json:"business_id"`
	ExpectedBusinessVersion int64  `json:"expected_business_version"`
	BusinessRevisionID      string `json:"business_revision_id"`
	BusinessContentHash     string `json:"business_content_hash"`
	Reason                  string `json:"reason"`
}
type RequestRevise struct {
	ExpectedVersion         int64  `json:"expected_version"`
	ExpectedBusinessVersion int64  `json:"expected_business_version"`
	BusinessRevisionID      string `json:"business_revision_id"`
	BusinessContentHash     string `json:"business_content_hash"`
	Reason                  string `json:"reason"`
}
type SubmitInput struct {
	ExpectedVersion      int64  `json:"expected_version"`
	RevisionID           string `json:"revision_id"`
	ContentHash          string `json:"content_hash"`
	WorkflowDefinitionID string `json:"workflow_definition_id"`
	PreviewHash          string `json:"preview_hash"`
	Reason               string `json:"reason"`
}
type DecisionInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	RequestVersion  int64  `json:"request_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}
type WithdrawInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}
type RequestFilter struct {
	State, Kind, Query, Cursor string
	Limit                      int
}

const requestColumns = `r.id,r.version,r.title,r.kind,r.business_id,r.applicant_id,r.project_id,r.classification,r.state,r.current_revision_id,r.active_instance_id,
    v.content_hash,v.payload,v.workflow_snapshot,r.created_at,r.updated_at`
const requestJoin = ` FROM requests r JOIN request_revisions v ON v.org_id=r.org_id AND v.request_id=r.id AND v.id=r.current_revision_id `

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	var payload, flow []byte
	err := row.Scan(&r.ID, &r.Version, &r.Title, &r.Kind, &r.BusinessID, &r.ApplicantID, &r.ProjectID, &r.Classification, &r.State, &r.CurrentRevisionID, &r.ActiveInstanceID, &r.ContentHash, &payload, &flow, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &r.Payload); err != nil {
		return r, err
	}
	if len(flow) > 0 {
		err = json.Unmarshal(flow, &r.Workflow)
	}
	r.Steps = []ApprovalStep{}
	return r, err
}
func requestResource(r Request) authz.Resource {
	return authz.Resource{Type: "request", ID: r.ID, ProjectID: r.ProjectID, OwnerID: &r.ApplicantID, Classification: r.Classification}
}
func requestMetadata(ctx context.Context, db platform.DBTX, org, id string) (Request, error) {
	if !core.ValidID(id) {
		return Request{}, core.Missing()
	}
	return scanRequest(db.QueryRow(ctx, "SELECT "+requestColumns+requestJoin+"WHERE r.org_id=$1 AND r.id=$2", org, id))
}
func draftState(state string) bool {
	return slices.Contains([]string{"draft", "returned", "withdrawn"}, state)
}
func requestBinding(a core.Actor, scope string) string {
	return core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/requests/", scope)))
}
