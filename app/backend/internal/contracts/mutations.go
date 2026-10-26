package contracts

import (
	"context"
	"encoding/json"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

func (s *Service) mutate(ctx context.Context, c core.Command, id string, version int64, action string, fn func(pgx.Tx, Contract) (core.Result, error)) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replay(ctx, c.Actor, out, commandErr) }()
	if err := core.RequireVersion(version); err != nil {
		return core.Result{}, err
	}
	check := func(tx pgx.Tx) (Contract, error) {
		d, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return Contract{}, err
		}
		if err = authz.Require(ctx, tx, c.Actor, "contract.read", resource(d)); err != nil {
			return Contract{}, err
		}
		if err = authz.Require(ctx, tx, c.Actor, action, resource(d)); err != nil {
			return Contract{}, err
		}
		if action == "contract.execute" {
			if err = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
				return Contract{}, err
			}
		}
		return d, nil
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, err := check(tx); return err }, func(tx pgx.Tx) (core.Result, error) {
		d, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if action == "contract.execute" {
			if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('contract-lifecycle:'||$1::text,0))", c.Actor.OrgID); err != nil {
				return core.Result{}, err
			}
		}
		if err = projectWritable(ctx, tx, c.Actor.OrgID, d.ProjectID, d.Classification, true); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		d, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if d.Version != version {
			return core.Result{}, core.Conflict()
		}
		before, err := json.Marshal(d)
		if err != nil {
			return core.Result{}, err
		}
		out, err := fn(tx, d)
		out.Event.BeforeDigest = core.Digest(before)
		return out, err
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, id string, in Patch) (core.Result, error) {
	return s.update(ctx, c, id, in.ExpectedVersion, in.BaseRevisionID, in.Reason, func(Contract) Content { return in.Content })
}
func (s *Service) update(ctx context.Context, c core.Command, id string, version int64, base, reason string, content func(Contract) Content) (core.Result, error) {
	base = strings.ToLower(base)
	if !core.ValidID(base) || core.Name(reason) != nil {
		return core.Result{}, core.Invalid("base_revision_id", "请固定当前合同修订并说明修改依据")
	}
	return s.mutate(ctx, c, id, version, "contract.update", func(tx pgx.Tx, d Contract) (core.Result, error) {
		if !editable(d) {
			return core.Result{}, core.Fail(409, "CONTRACT_READ_ONLY", "合同当前业务或审批阶段不允许普通草稿编辑")
		}
		if d.CurrentRevisionID != base {
			return core.Result{}, core.Conflict()
		}
		value, err := normalize(ctx, tx, c.Actor, d, content(d), &d.Content)
		if err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM contract_revisions WHERE org_id=$1 AND contract_id=$2 AND id=$3", c.Actor.OrgID, id, d.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if err = insertRevision(ctx, tx, c.Actor, d, revision, seq+1, &d.CurrentRevisionID, value, reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE contracts SET current_revision_id=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision); err != nil {
			return core.Result{}, err
		}
		if d.RequestID != nil {
			tag, e := tx.Exec(ctx, "UPDATE requests SET state='draft',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 AND business_id=$3 AND kind='contract' AND state IN ('draft','returned','withdrawn')", c.Actor.OrgID, *d.RequestID, id)
			if e != nil {
				return core.Result{}, e
			}
			if tag.RowsAffected() != 1 {
				return core.Result{}, core.Fail(409, "REQUEST_READ_ONLY", "对应申请当前不能同步为草稿")
			}
			if _, e = tx.Exec(ctx, "UPDATE contracts SET approval_state='not_submitted' WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); e != nil {
				return core.Result{}, e
			}
		}
		d, err = metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, "contract.revised", 200, reason), err
	})
}
func (s *Service) Trash(ctx context.Context, c core.Command, id string, in TrashInput) (core.Result, error) {
	if (in.Action != "trash" && in.Action != "restore") || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("action", "请明确草稿回收或还原及其依据")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, "contract.archive", func(tx pgx.Tx, d Contract) (core.Result, error) {
		if d.Lifecycle != "draft" || d.ApprovedRevisionID != nil || d.ActiveRevisionID != nil || d.RequestID != nil || d.ApprovalState != "not_submitted" {
			return core.Result{}, core.Fail(409, "CONTRACT_RECORD_PROTECTED", "已有审批或履约依据的合同不能通过普通草稿回收操作处理")
		}
		if (in.Action == "trash") == (d.DeletedAt != nil) {
			return core.Result{}, core.Fail(409, "CONTRACT_STATE_CONFLICT", "合同回收状态已变化，请重新核对")
		}
		if in.Action == "trash" {
			var children bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM contracts WHERE org_id=$1 AND parent_contract_id=$2 AND deleted_at IS NULL)", c.Actor.OrgID, id).Scan(&children); err != nil {
				return core.Result{}, err
			}
			if children {
				return core.Result{}, core.Fail(409, "CONTRACT_HAS_SUPPLEMENTS", "须先处理关联的补充协议草稿，再回收主合同")
			}
		} else if err := validateParent(ctx, tx, c.Actor, d); err != nil {
			return core.Result{}, err
		}
		event := "contract.trashed"
		query := "UPDATE contracts SET deleted_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2"
		if in.Action == "restore" {
			event = "contract.restored"
			query = "UPDATE contracts SET deleted_at=NULL,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2"
		}
		if _, err := tx.Exec(ctx, query, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		d, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, event, 200, in.Reason), err
	})
}
