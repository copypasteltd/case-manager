package workflow

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

func requestWritable(ctx context.Context, tx pgx.Tx, a core.Actor, r Request) error {
	if r.ProjectID == nil {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, *r.ProjectID).Scan(&state); err != nil {
		return err
	}
	if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许审批写入")
	}
	return nil
}
func (s *Service) Decide(ctx context.Context, c core.Command, stepID string, in DecisionInput) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.requestReplay(ctx, c.Actor, out, commandErr) }()
	in.RevisionID = strings.ToLower(in.RevisionID)
	in.Reason = strings.TrimSpace(in.Reason)
	if !core.ValidID(stepID) || !core.ValidID(in.RevisionID) || !digestPattern.MatchString(in.ContentHash) || core.RequireVersion(in.ExpectedVersion) != nil || core.RequireVersion(in.RequestVersion) != nil || !slices.Contains([]string{"approve", "return", "reject"}, in.Decision) || len([]rune(in.Reason)) > 20000 || (in.Decision != "approve" && in.Reason == "") {
		return core.Result{}, core.Invalid("decision", "请固定申请与步骤版本，退回或拒绝须说明原因")
	}
	var requestID, assigned, revision string
	var mfa bool
	authorize := func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT request_id,assigned_user_id,revision_id,requires_mfa FROM approval_steps WHERE org_id=$1 AND id=$2", c.Actor.OrgID, stepID).Scan(&requestID, &assigned, &revision, &mfa); err != nil {
			return platform.DatabaseError(err)
		}
		if assigned != c.Actor.UserID {
			return core.Missing()
		}
		r, err := requestMetadata(ctx, tx, c.Actor.OrgID, requestID)
		if err != nil {
			return err
		}
		if err = requireRequest(ctx, tx, c.Actor, r, "approval.decide"); err != nil {
			return err
		}
		if err = identity.RequireInteractiveSession(ctx, tx, c.Actor, mfa); err != nil {
			return err
		}
		old, err := revisionMetadata(ctx, tx, c.Actor.OrgID, r.ID, revision)
		if err != nil {
			return err
		}
		_, err = approvalAuthority(ctx, tx, c.Actor, r, old.Payload)
		return err
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, d, err := lockRequest(ctx, tx, c.Actor, requestID, "approval.decide")
		if err != nil {
			return core.Result{}, err
		}
		if err = requestWritable(ctx, tx, c.Actor, r); err != nil {
			return core.Result{}, err
		}
		if r.Version != in.RequestVersion || r.CurrentRevisionID != in.RevisionID || r.ContentHash != in.ContentHash {
			return core.Result{}, core.Conflict()
		}
		if r.State != "in_review" || r.ActiveInstanceID == nil || r.Workflow == nil || d.RequestID == nil || *d.RequestID != r.ID || d.CurrentRevisionID != r.Payload.BusinessRevisionID || d.ContentHash != r.Payload.BusinessContentHash || !businessReviewing(d) {
			return core.Result{}, core.Fail(409, "APPROVAL_NOT_ACTIVE", "申请或业务已不在此固定审批阶段")
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM approval_instances WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, *r.ActiveInstanceID); err != nil {
			return core.Result{}, err
		}
		var version int64
		var state, instance, fixedRevision, user string
		var number int
		if err = tx.QueryRow(ctx, "SELECT version,state,instance_id,revision_id,assigned_user_id,step_no FROM approval_steps WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, stepID).Scan(&version, &state, &instance, &fixedRevision, &user, &number); err != nil {
			return core.Result{}, core.Missing()
		}
		if version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if state != "active" || instance != *r.ActiveInstanceID || fixedRevision != r.CurrentRevisionID || user != c.Actor.UserID {
			return core.Result{}, core.Fail(409, "APPROVAL_NOT_ACTIVE", "仅当前分配人可以决定当前步骤")
		}
		if user == r.ApplicantID && r.Workflow.SelfApprovalPolicy != "allow" {
			return core.Result{}, core.Fail(409, "SELF_APPROVAL_DENIED", "此制度禁止申请人审核自己的申请")
		}
		proof, err := approvalAuthority(ctx, tx, c.Actor, r, r.Payload)
		if err != nil {
			return core.Result{}, err
		}
		evidence, err := json.Marshal(proof)
		if err != nil {
			return core.Result{}, err
		}
		var verified *time.Time
		if err = tx.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE org_id=$1 AND id=$2", c.Actor.OrgID, c.Actor.SessionID).Scan(&verified); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO approval_decisions(id,org_id,step_id,request_id,revision_id,content_hash,actor_id,session_id,decision,reason,mfa_verified_at,authority_evidence)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, core.NewID(), c.Actor.OrgID, stepID, r.ID, r.CurrentRevisionID, r.ContentHash, c.Actor.UserID, c.Actor.SessionID, in.Decision, in.Reason, verified, evidence); err != nil {
			return core.Result{}, err
		}
		final := map[string]string{"approve": "approved", "return": "returned", "reject": "rejected"}[in.Decision]
		if _, err = tx.Exec(ctx, "UPDATE approval_steps SET state=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, stepID, final); err != nil {
			return core.Result{}, err
		}
		requestState := final
		if in.Decision == "approve" {
			var next string
			e := tx.QueryRow(ctx, "SELECT id FROM approval_steps WHERE org_id=$1 AND instance_id=$2 AND step_no>$3 AND state='pending' ORDER BY step_no LIMIT 1", c.Actor.OrgID, instance, number).Scan(&next)
			if e == nil {
				requestState = "in_review"
				if _, err = tx.Exec(ctx, "UPDATE approval_steps SET state='active',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, next); err != nil {
					return core.Result{}, err
				}
			} else if e != pgx.ErrNoRows {
				return core.Result{}, e
			}
		} else {
			if _, err = tx.Exec(ctx, "UPDATE approval_steps SET state='cancelled',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND instance_id=$2 AND state='pending'", c.Actor.OrgID, instance); err != nil {
				return core.Result{}, err
			}
		}
		if requestState != "in_review" {
			if _, err = tx.Exec(ctx, "UPDATE approval_instances SET state=$3,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, instance, requestState); err != nil {
				return core.Result{}, err
			}
			if err = setBusinessState(ctx, tx, c.Actor, d, r.ID, requestState); err != nil {
				return core.Result{}, err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE requests SET state=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, r.ID, requestState); err != nil {
			return core.Result{}, err
		}
		r, err = requestMetadata(ctx, tx, c.Actor.OrgID, r.ID)
		if err != nil {
			return core.Result{}, err
		}
		r, err = presentRequest(ctx, tx, c.Actor, r)
		result := requestResult(r, "approval.step.decided", 200, in.Reason)
		result.Event.Payload["step_id"] = stepID
		result.Event.Payload["decision"] = in.Decision
		return result, err
	})
}
