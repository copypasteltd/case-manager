package contracts

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"slices"
)

// ApprovalDraft is the business adapter used by the approval module. Mutations
// always lock project then business before the request or approval instance.
func ApprovalDraft(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock, complete bool) (Contract, error) {
	d, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return d, err
	}
	if err = authz.Require(ctx, db, a, "contract.read", resource(d)); err != nil {
		return Contract{}, err
	}
	if err = authz.Require(ctx, db, a, "contract.update", resource(d)); err != nil {
		return Contract{}, err
	}
	if err = projectWritable(ctx, db, a.OrgID, d.ProjectID, d.Classification, lock); err != nil {
		return Contract{}, err
	}
	if lock {
		if _, err = db.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return Contract{}, err
		}
		d, err = metadata(ctx, db, a.OrgID, id)
		if err != nil {
			return d, err
		}
	}
	if d.DeletedAt != nil || d.Lifecycle != "draft" || !slices.Contains([]string{"not_submitted", "returned", "withdrawn", "rejected"}, d.ApprovalState) {
		return Contract{}, core.Fail(409, "CONTRACT_READ_ONLY", "当前合同阶段不能准备新的送审内容")
	}
	if complete {
		if d.Direction == nil || d.Type == nil || d.Category == "" || d.CounterpartyID == nil || d.OwnerID == nil || d.Currency == nil || d.Amount == nil || d.StartDate == nil || d.EndDate == nil || d.DraftFileVersionID == nil {
			return Contract{}, core.Invalid("contract", "送审前须补全收付方向、类型、业务类别、往来方、负责人、币种金额、日期和正文版本")
		}
		if _, err = normalize(ctx, db, a, d, d.Content, nil); err != nil {
			return Contract{}, err
		}
		if _, err = normalizeSchedule(ctx, db, d.Content, true); err != nil {
			return Contract{}, err
		}
		var ownerActive bool
		if err = db.QueryRow(ctx, "SELECT status='active' FROM users WHERE org_id=$1 AND id=$2", a.OrgID, *d.OwnerID).Scan(&ownerActive); err != nil {
			return Contract{}, err
		}
		if !ownerActive {
			return Contract{}, core.Invalid("owner_id", "合同负责人账号已停用")
		}
		if _, err = fileref.ForApproval(ctx, db, a, *d.DraftFileVersionID, d.ProjectID, d.Classification); err != nil {
			return Contract{}, err
		}
	}
	return present(ctx, db, a, d)
}

// ApprovalRead requires current business visibility without granting editing.
func ApprovalRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Contract, error) {
	d, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return d, err
	}
	return present(ctx, db, a, d)
}
