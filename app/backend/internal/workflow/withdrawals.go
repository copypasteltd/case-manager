package workflow

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) WithdrawRequest(ctx context.Context, c core.Command, id string, in WithdrawInput) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.requestReplay(ctx, c.Actor, out, commandErr) }()
	in.RevisionID = strings.ToLower(in.RevisionID)
	if core.RequireVersion(in.ExpectedVersion) != nil || !core.ValidID(in.RevisionID) || !digestPattern.MatchString(in.ContentHash) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("revision_id", "请核对当前申请版本并填写撤回原因")
	}
	authorize := func(tx pgx.Tx) error {
		r, e := requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return e
		}
		if r.ApplicantID != c.Actor.UserID {
			return core.Missing()
		}
		if e = requireRequest(ctx, tx, c.Actor, r, "request.withdraw"); e != nil {
			return e
		}
		return identity.RequireInteractiveSession(ctx, tx, c.Actor, false)
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, d, err := lockRequest(ctx, tx, c.Actor, id, "request.withdraw")
		if err != nil {
			return core.Result{}, err
		}
		if err = requestWritable(ctx, tx, c.Actor, r); err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion || r.CurrentRevisionID != in.RevisionID || r.ContentHash != in.ContentHash {
			return core.Result{}, core.Conflict()
		}
		if r.State != "in_review" || r.Workflow == nil || !r.Workflow.WithdrawalAllowed || r.ActiveInstanceID == nil || d.RequestID == nil || *d.RequestID != r.ID {
			return core.Result{}, core.Fail(409, "WITHDRAWAL_NOT_ALLOWED", "当前阶段或固定制度不允许撤回")
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM approval_instances WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, *r.ActiveInstanceID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE approval_steps SET state='cancelled',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND instance_id=$2 AND state IN ('pending','active')", c.Actor.OrgID, *r.ActiveInstanceID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE approval_instances SET state='withdrawn',updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, *r.ActiveInstanceID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO approval_withdrawals(id,org_id,request_id,revision_id,instance_id,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7)", core.NewID(), c.Actor.OrgID, id, r.CurrentRevisionID, *r.ActiveInstanceID, c.Actor.UserID, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE requests SET state='withdrawn',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		if err = setBusinessState(ctx, tx, c.Actor, d, r.ID, "withdrawn"); err != nil {
			return core.Result{}, err
		}
		r, err = requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		r, err = presentRequest(ctx, tx, c.Actor, r)
		return requestResult(r, "request.withdrawn", 200, in.Reason), err
	})
}
