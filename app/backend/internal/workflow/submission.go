package workflow

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
)

type SubmissionPreview struct {
	RequestID      string              `json:"request_id"`
	RequestVersion int64               `json:"request_version"`
	RevisionID     string              `json:"revision_id"`
	ContentHash    string              `json:"content_hash"`
	PreviewHash    string              `json:"preview_hash"`
	Workflow       Definition          `json:"workflow"`
	Payload        Snapshot            `json:"payload"`
	Proofs         []AuthorityEvidence `json:"-"`
}

func submissionPreview(ctx context.Context, tx pgx.Tx, a core.Actor, r Request) (SubmissionPreview, error) {
	var out SubmissionPreview
	if r.ApplicantID != a.UserID {
		return out, core.Missing()
	}
	if err := requireRequest(ctx, tx, a, r, "request.submit"); err != nil {
		return out, err
	}
	if err := requireSubmissionActor(ctx, tx, a, r, false); err != nil {
		return out, err
	}
	if !draftState(r.State) {
		return out, core.Fail(409, "REQUEST_READ_ONLY", "当前申请不能再次送审")
	}
	d, err := subject(ctx, tx, a, r.Kind, r.BusinessID, false, true)
	if err != nil {
		return out, err
	}
	if d.RequestID == nil || *d.RequestID != r.ID || d.CurrentRevisionID != r.Payload.BusinessRevisionID || d.ContentHash != r.Payload.BusinessContentHash {
		return out, core.Fail(409, "REQUEST_DRAFT_STALE", "业务内容已修订，请先更新申请草稿并核对")
	}
	policy, err := SelectDefinition(ctx, tx, a.OrgID, r.Kind, d.Category, *d.Currency, *d.Amount)
	if err != nil {
		return out, err
	}
	out = SubmissionPreview{RequestID: r.ID, RequestVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Payload: snapshot(d), Workflow: policy, Proofs: []AuthorityEvidence{}}
	for _, step := range policy.Steps {
		if step.AssignedUserID == r.ApplicantID && policy.SelfApprovalPolicy != "allow" {
			return out, core.Fail(409, "SELF_APPROVAL_DENIED", "此制度禁止申请人审核自己的申请")
		}
		evidence, e := reviewerAuthority(ctx, tx, a, r, out.Payload, step.AssignedUserID)
		if e != nil {
			return out, e
		}
		out.Proofs = append(out.Proofs, evidence)
	}
	content, err := revisionHash(out.Payload, &out.Workflow, out.Proofs)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(map[string]any{"request_id": r.ID, "request_version": r.Version, "revision_id": r.CurrentRevisionID, "draft_content_hash": r.ContentHash,
		"submission_content_hash": content, "actor": a.UserID, "user_auth_version": a.AuthVersion, "org_auth_version": a.OrgAuthVersion, "channel": a.Channel})
	if err != nil {
		return out, err
	}
	raw, err = core.CanonicalJSON(raw)
	if err != nil {
		return out, err
	}
	out.PreviewHash = core.Digest(raw)
	return out, nil
}
func (s *Service) SubmissionPreview(ctx context.Context, a core.Actor, id string) (SubmissionPreview, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return SubmissionPreview{}, err
	}
	defer tx.Rollback(ctx)
	r, err := requestMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return SubmissionPreview{}, err
	}
	preview, err := submissionPreview(ctx, tx, a, r)
	if err != nil {
		return preview, err
	}
	return preview, tx.Commit(ctx)
}
func (s *Service) SubmitRequest(ctx context.Context, c core.Command, id string, in SubmitInput) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.requestReplay(ctx, c.Actor, out, commandErr) }()
	in.RevisionID = strings.ToLower(in.RevisionID)
	in.WorkflowDefinitionID = strings.ToLower(in.WorkflowDefinitionID)
	if core.RequireVersion(in.ExpectedVersion) != nil || !core.ValidID(in.RevisionID) || !core.ValidID(in.WorkflowDefinitionID) || !digestPattern.MatchString(in.ContentHash) || !digestPattern.MatchString(in.PreviewHash) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("revision_id", "请确认准确申请、审批制度与送审预览")
	}
	authorize := func(tx pgx.Tx) error {
		r, e := requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return e
		}
		if r.ApplicantID != c.Actor.UserID {
			return core.Missing()
		}
		if e = requireRequest(ctx, tx, c.Actor, r, "request.submit"); e != nil {
			return e
		}
		d, e := businessRead(ctx, tx, c.Actor, r)
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, tx, c.Actor, d.Kind+".update", businessResource(d)); e != nil {
			return e
		}
		return requireSubmissionActor(ctx, tx, c.Actor, r, true)
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, business, err := lockRequest(ctx, tx, c.Actor, id, "request.submit")
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion || r.CurrentRevisionID != in.RevisionID || r.ContentHash != in.ContentHash {
			return core.Result{}, core.Conflict()
		}
		preview, err := submissionPreview(ctx, tx, c.Actor, r)
		if err != nil {
			return core.Result{}, err
		}
		if preview.Workflow.ID != in.WorkflowDefinitionID || preview.PreviewHash != in.PreviewHash {
			return core.Result{}, core.Fail(409, "SUBMISSION_PREVIEW_CHANGED", "送审内容、制度或审批权限已变化，请重新核对预览")
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM request_revisions WHERE org_id=$1 AND request_id=$2 AND id=$3", c.Actor.OrgID, id, r.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision, instance := core.NewID(), core.NewID()
		if _, err = insertRequestRevision(ctx, tx, c.Actor, r, revision, seq+1, &r.CurrentRevisionID, preview.Payload, &preview.Workflow, preview.Proofs, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO approval_instances(id,org_id,request_id,revision_id,definition_id,state) VALUES($1,$2,$3,$4,$5,'in_review')", instance, c.Actor.OrgID, id, revision, preview.Workflow.ID); err != nil {
			return core.Result{}, err
		}
		for i, step := range preview.Workflow.Steps {
			evidence, e := json.Marshal(preview.Proofs[i])
			if e != nil {
				return core.Result{}, e
			}
			state := "pending"
			if i == 0 {
				state = "active"
			}
			if _, err = tx.Exec(ctx, `INSERT INTO approval_steps(id,org_id,request_id,revision_id,instance_id,step_no,title,assigned_user_id,assigned_user_name,requires_mfa,authority_evidence,state)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, core.NewID(), c.Actor.OrgID, id, revision, instance, i+1, step.Title, step.AssignedUserID, step.AssignedUserName, step.RequiresMFA, evidence, state); err != nil {
				return core.Result{}, err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE requests SET current_revision_id=$3,active_instance_id=$4,state='in_review',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision, instance); err != nil {
			return core.Result{}, err
		}
		if err = setBusinessState(ctx, tx, c.Actor, business, id, "in_review"); err != nil {
			return core.Result{}, err
		}
		r, err = requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		r, err = presentRequest(ctx, tx, c.Actor, r)
		return requestResult(r, "request.submitted", 200, in.Reason), err
	})
}
