package finance

import (
	"context"
	"encoding/json"
	"slices"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
)

func contractClosureReadable(ctx context.Context, db platform.DBTX, a core.Actor, c contractFinanceMeta) (bool, error) {
	if err := authz.Require(ctx, db, a, "contract.read", contractFinanceResource(c)); err != nil {
		return false, err
	}
	var visible bool
	err := db.QueryRow(ctx, "SELECT COALESCE(can_read_contract_closure($1,$2,$3,$4,$5),false)", a.OrgID, a.UserID, c.ID, a.Channel, a.RunID != "").Scan(&visible)
	return visible, err
}
func contractClosureBasis(ctx context.Context, db platform.DBTX, org, id string) (ContractClosureSnapshot, []byte, string, error) {
	var b ContractClosureSnapshot
	var raw []byte
	var fingerprint string
	err := db.QueryRow(ctx, "SELECT contract_closure_snapshot($1,$2),md5(contract_closure_snapshot($1,$2)::text)", org, id).Scan(&raw, &fingerprint)
	if err == nil {
		err = json.Unmarshal(raw, &b)
	}
	if err == nil && (len(b.Costs) > 1000 || len(b.Children) > 1000) {
		err = core.Fail(409, "CONTRACT_CLOSURE_SCOPE_LIMIT", "关闭依据超过单次核对范围，请先完成范围评审")
	}
	return b, raw, fingerprint, err
}
func contractClosureBlockers(b ContractClosureSnapshot) []string {
	out := []string{}
	add := func(v string) {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	if b.ScheduleMode == nil {
		add("plan_unknown")
	}
	for _, n := range b.Nodes {
		planned, e := core.ParseDecimal(n.Amount)
		cash, x := core.ParseDecimal(n.CashAmount)
		if e != nil || x != nil || planned.Cmp(cash) != 0 {
			add("node_amount_difference")
		}
		if n.HasUnattributedRefunds {
			add("unattributed_refunds")
		}
		if !n.ReviewCurrent {
			add("node_review_required")
		}
		if n.ConditionsSatisfied == nil || !*n.ConditionsSatisfied {
			add("conditions_unconfirmed")
		}
	}
	for _, c := range b.Costs {
		if c.State != "confirmed" {
			continue
		}
		amount, e := core.ParseDecimal(c.Amount)
		paid, x := core.ParseDecimal(c.PaidAmount)
		if e != nil || x != nil || amount.Cmp(paid) != 0 {
			add("cost_unsettled")
		}
	}
	for _, c := range b.Children {
		if !slices.Contains([]string{"completed", "terminated"}, c.Lifecycle) {
			add("supplement_unresolved")
		}
		if c.Lifecycle == "effective" {
			add("supplement_effective")
		}
	}
	return out
}
func (s *Service) ContractClosurePreview(ctx context.Context, a core.Actor, id string) (ContractClosurePreview, error) {
	out := ContractClosurePreview{ContractID: id, Visibility: "restricted", Blockers: []string{}}
	tx, err := s.contractFinanceSnapshotTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := contractFinanceMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, tx, a, "contract.read", contractFinanceResource(c)); err != nil {
		return out, err
	}
	out.ContractVersion = c.Version
	if c.ActiveRevisionID == nil {
		out.Visibility = "not_active"
		out.Blockers = []string{"contract_not_effective"}
		return out, tx.Commit(ctx)
	}
	visible, err := contractClosureReadable(ctx, tx, a, c)
	if err != nil {
		return out, err
	}
	if !visible {
		out.Blockers = []string{"restricted"}
		return out, tx.Commit(ctx)
	}
	basis, _, fingerprint, err := contractClosureBasis(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.Visibility = "complete"
	out.Basis = &basis
	out.Fingerprint = &fingerprint
	out.Blockers = contractClosureBlockers(basis)
	allowed := a.RunID == "" && c.Lifecycle == "effective" && authz.Require(ctx, tx, a, "contract.close", contractFinanceResource(c)) == nil && writable(ctx, tx, a, Cost{ProjectID: c.ProjectID, Classification: c.Classification})
	out.CanComplete = allowed && len(out.Blockers) == 0
	out.CanTerminate = allowed && !slices.Contains(out.Blockers, "supplement_effective")
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ContractClosure(ctx context.Context, a core.Actor, id string) (ContractClosure, error) {
	var out ContractClosure
	tx, err := s.contractFinanceSnapshotTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := contractFinanceMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	visible, err := contractClosureReadable(ctx, tx, a, c)
	if err != nil {
		return out, err
	}
	if !visible {
		return out, core.Missing()
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT z.id,z.contract_id,z.contract_version,z.action,z.occurred_on::text,z.basis,z.fingerprint,z.terms_verified,z.attestation,
        z.acknowledge_unsettled,z.unsettled_disposition,z.actor_id,u.display_name,z.reason,z.created_at
        FROM contract_closures z JOIN users u ON u.org_id=z.org_id AND u.id=z.actor_id WHERE z.org_id=$1 AND z.contract_id=$2`, a.OrgID, id).Scan(
		&out.ID, &out.ContractID, &out.ContractVersion, &out.Action, &out.OccurredOn, &raw, &out.Fingerprint, &out.TermsVerified, &out.Attestation, &out.AcknowledgeUnsettled, &out.UnsettledDisposition, &out.ActorID, &out.ActorName, &out.Reason, &out.CreatedAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(raw, &out.Basis); err != nil {
		return out, err
	}
	_, _, now, err := contractClosureBasis(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.BasisState = "changed"
	if now == out.Fingerprint {
		out.BasisState = "current"
	}
	rows, err := tx.Query(ctx, "SELECT version_id FROM contract_closure_files WHERE org_id=$1 AND closure_id=$2 ORDER BY version_id", a.OrgID, out.ID)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.Attachments = []fileref.Evidence{}
	for _, v := range ids {
		file, e := fileref.Resolve(ctx, tx, a, v, c.ProjectID, c.Classification)
		if e != nil {
			return out, e
		}
		out.Attachments = append(out.Attachments, file)
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
