package workflow

import (
	"context"
	"encoding/json"
	"slices"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"github.com/jackc/pgx/v5"
)

// AgentBusinessView deliberately excludes actor-dependent editing flags.
type AgentBusinessView struct {
	ID                string   `json:"id"`
	Kind              string   `json:"kind"`
	Version           int64    `json:"version"`
	CreatorID         string   `json:"creator_id"`
	Title             string   `json:"title"`
	ProjectID         *string  `json:"project_id"`
	Classification    string   `json:"classification"`
	Lifecycle         string   `json:"lifecycle"`
	ApprovalState     string   `json:"approval_state"`
	CurrentRevisionID string   `json:"current_revision_id"`
	ContentHash       string   `json:"content_hash"`
	RequestID         *string  `json:"request_id"`
	Payload           Snapshot `json:"payload"`
}

func AgentBusinessRead(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, writable bool) (AgentBusinessView, error) {
	var out AgentBusinessView
	if a.RunID == "" || !slices.Contains([]string{"contract", "purchase", "claim"}, kind) {
		return out, core.Missing()
	}
	d, err := businessRead(ctx, db, a, Request{Kind: kind, BusinessID: id})
	if err != nil {
		return out, err
	}
	if d.DeletedAt != nil {
		return out, core.Missing()
	}
	if writable {
		if _, err = subject(ctx, db, a, kind, id, false, false); err != nil {
			return out, err
		}
	}
	if err = AgentSnapshotReferences(ctx, db, a, d.Payload); err != nil {
		return out, err
	}
	out = AgentBusinessView{d.ID, d.Kind, d.Version, d.CreatorID, d.Title, d.ProjectID, d.Classification, d.Lifecycle, d.ApprovalState, d.CurrentRevisionID, d.ContentHash, d.RequestID, d.Payload}
	return out, nil
}

// References are checked from raw immutable content, including links the normal
// UI may hide. The model never receives an underlying reference it cannot read.
func AgentSnapshotReferences(ctx context.Context, db platform.DBTX, a core.Actor, p Snapshot) error {
	if a.RunID == "" || !slices.Contains([]string{"contract", "purchase", "claim"}, p.Kind) {
		return core.Missing()
	}
	refs, err := snapshotSuppliers(p)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if _, _, err = party.ResolveBusinessReference(ctx, db, a, ref.PartyID, ref.RevisionID, false); err != nil {
			return err
		}
	}
	ids := []string{}
	if p.Kind == "contract" {
		if id := p.ContractContent().DraftFileVersionID; id != nil {
			ids = append(ids, *id)
		}
	} else {
		var content spend.Content
		if json.Unmarshal(p.Content, &content) != nil {
			return core.Missing()
		}
		ids = append(ids, content.QuoteVersionIDs...)
		for _, line := range content.Lines {
			ids = append(ids, line.ReceiptVersionIDs...)
		}
	}
	for _, id := range ids {
		if _, err = fileref.Resolve(ctx, db, a, id, p.ProjectID, p.Classification); err != nil {
			return err
		}
	}
	for _, fixed := range p.Attachments {
		current, e := fileref.Resolve(ctx, db, a, fixed.VersionID, p.ProjectID, p.Classification)
		if e != nil {
			return e
		}
		if current.FileID != fixed.FileID || current.SHA256 != fixed.SHA256 || current.SizeBytes != fixed.SizeBytes {
			return core.Fail(409, "OA_REFERENCE_CHANGED", "固定附件内容已变化，请重新核对")
		}
	}
	return nil
}

func AgentRequestRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Request, error) {
	r, err := requestMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return r, err
	}
	if a.RunID == "" || !slices.Contains([]string{"contract", "purchase", "claim"}, r.Kind) {
		return Request{}, core.Missing()
	}
	if err = requireRequest(ctx, db, a, r, "request.read"); err != nil {
		return Request{}, err
	}
	if err = AgentSnapshotReferences(ctx, db, a, r.Payload); err != nil {
		return Request{}, err
	}
	r.Steps, err = steps(ctx, db, a, r, r.CurrentRevisionID)
	for i := range r.Steps {
		r.Steps[i].CanDecide = false
	}
	return r, err
}

func AgentDraftPermission(ctx context.Context, db platform.DBTX, a core.Actor, business AgentBusinessView, request *Request) error {
	if err := authz.Require(ctx, db, a, business.Kind+".update", authz.Resource{Type: business.Kind, ID: business.ID, ProjectID: business.ProjectID, OwnerID: &business.CreatorID, Classification: business.Classification}); err != nil {
		return err
	}
	if request != nil {
		if request.ApplicantID != a.UserID || !draftState(request.State) {
			return core.Missing()
		}
		return requireRequest(ctx, db, a, *request, "request.update")
	}
	return authz.Require(ctx, db, a, "request.create", requestResource(Request{ApplicantID: a.UserID, ProjectID: business.ProjectID, Classification: business.Classification}))
}

// Only preparation by the actual human uses this entry point. Execution keeps
// RunID set and is authorized by the immutable, exact submission intent below.
func SubmissionPreviewInTx(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (SubmissionPreview, error) {
	r, err := requestMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return SubmissionPreview{}, err
	}
	return submissionPreview(ctx, tx, a, r)
}

func requireSubmissionActor(ctx context.Context, db platform.DBTX, a core.Actor, r Request, executing bool) error {
	if a.RunID == "" {
		return identity.RequireInteractiveSession(ctx, db, a, false)
	}
	var session string
	err := db.QueryRow(ctx, `SELECT i.session_id FROM agent_oa_intents i
 JOIN agent_runs x ON x.org_id=i.org_id AND x.id=i.run_id
 JOIN conversations c ON c.org_id=x.org_id AND c.id=x.conversation_id
	 JOIN conversation_revisions cr ON cr.org_id=c.org_id AND cr.id=c.current_revision_id
 JOIN agent_runtime_profiles p ON p.org_id=x.org_id AND p.id=x.profile_id
 WHERE i.org_id=$1 AND i.run_id=$2 AND i.actor_id=$3 AND i.tool='submit_request' AND i.request_id=$4
 AND x.actor_id=i.actor_id AND x.actor_auth_version=$5 AND x.organization_auth_version=$6 AND x.channel=$7
	 AND x.state IN ('queued','running') AND (NOT $11::boolean OR x.state='running') AND x.cancel_requested_at IS NULL AND x.expires_at>clock_timestamp()
	 AND cr.state='active' AND c.type='private' AND c.creator_id=i.actor_id
	 AND c.current_revision_id=x.conversation_revision_id AND cr.context_security_version=x.context_security_version
 AND p.state='active' AND p.skill_id='oa-assist'
 AND (i.arguments->>'expected_version')::bigint=$8 AND i.arguments->>'revision_id'=$9
	 AND i.arguments->>'content_hash'=$10`, a.OrgID, a.RunID, a.UserID, r.ID, a.AuthVersion, a.OrgAuthVersion, a.Channel, r.Version, r.CurrentRevisionID, r.ContentHash, executing).Scan(&session)
	if err == pgx.ErrNoRows {
		return core.Forbidden()
	}
	if err != nil {
		return err
	}
	human := a
	human.RunID, human.SessionID = "", session
	return identity.RequireInteractiveSession(ctx, db, human, false)
}
