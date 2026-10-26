package spend

import (
	"context"
	"encoding/json"
	"slices"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (s *Service) completionSnapshotTx(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	var orgVersion, userVersion int64
	var status string
	err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if err == nil {
		err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, platform.DatabaseError(err)
	}
	if status != "active" || orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion {
		tx.Rollback(ctx)
		return nil, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变化，请重新获取授权")
	}
	return tx, nil
}
func completionBasis(ctx context.Context, db platform.DBTX, org, id string) (CompletionSnapshot, []byte, string, []string, bool, error) {
	var b CompletionSnapshot
	var raw []byte
	var fingerprint string
	var blockers []string
	var difference bool
	err := db.QueryRow(ctx, `SELECT b,md5(b::text),purchase_completion_blockers(b),purchase_completion_has_difference(b) FROM (SELECT purchase_completion_snapshot($1,$2)b)x`, org, id).Scan(&raw, &fingerprint, &blockers, &difference)
	if err == nil {
		err = json.Unmarshal(raw, &b)
	}
	if err == nil && (len(b.Orders) > 1000 || len(b.Receipts) > 5000 || len(b.Costs) > 1000 || len(b.Payments) > 1000 || len(b.Variances) > 1000) {
		err = core.Fail(409, "PURCHASE_COMPLETION_SCOPE_LIMIT", "核对集合超过单次处理范围，请先完成范围评审")
	}
	return b, raw, fingerprint, blockers, difference, err
}
func completionEvidence(ctx context.Context, db platform.DBTX, a core.Actor, d Record, approval bool) error {
	if _, _, err := References(ctx, db, a, d, d.Content, approval, true); err != nil {
		return err
	}
	rows, err := db.Query(ctx, `SELECT DISTINCT supplier_id::text,supplier_revision_id::text FROM purchase_orders WHERE org_id=$1 AND purchase_id=$2`, a.OrgID, d.ID)
	if err != nil {
		return err
	}
	suppliers := []Supplier{}
	for rows.Next() {
		var v Supplier
		if err = rows.Scan(&v.PartyID, &v.RevisionID); err != nil {
			rows.Close()
			return err
		}
		suppliers = append(suppliers, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, _, err = References(ctx, db, a, d, Content{SupplierCandidates: suppliers}, approval, true); err != nil {
		return err
	}
	rows, err = db.Query(ctx, "SELECT id::text FROM purchase_variances WHERE org_id=$1 AND purchase_id=$2 ORDER BY id", a.OrgID, d.ID)
	if err != nil {
		return err
	}
	proposals := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		proposals = append(proposals, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range proposals {
		v, e := varianceMetadata(ctx, db, a.OrgID, id)
		if e != nil {
			return e
		}
		for _, content := range []Content{v.Content, v.Basis.Content} {
			if _, _, e = References(ctx, db, a, v.Record, content, approval, true); e != nil {
				return e
			}
		}
	}
	rows, err = db.Query(ctx, `SELECT version_id::text FROM spend_revision_files WHERE org_id=$1 AND revision_id=$3
        UNION SELECT unnest(evidence_version_ids)::text FROM purchase_orders WHERE org_id=$1 AND purchase_id=$2
        UNION SELECT unnest(evidence_version_ids)::text FROM purchase_receipts WHERE org_id=$1 AND purchase_id=$2
        UNION SELECT f.version_id::text FROM cost_revision_files f JOIN actual_cost_entries c ON c.org_id=f.org_id AND c.current_revision_id=f.revision_id WHERE f.org_id=$1 AND c.id IN(SELECT id FROM purchase_completion_cost_ids($1,$2))
        UNION SELECT f.version_id::text FROM payment_revision_files f JOIN payment_records p ON p.org_id=f.org_id AND p.current_revision_id=f.revision_id WHERE f.org_id=$1 AND p.id IN(SELECT id FROM purchase_completion_cash_ids($1,$2))
        UNION SELECT f.version_id::text FROM purchase_completion_files f JOIN purchase_completions z ON z.org_id=f.org_id AND z.id=f.completion_id WHERE z.org_id=$1 AND z.purchase_id=$2
        ORDER BY 1`, a.OrgID, d.ID, d.CurrentRevisionID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if approval {
			_, err = fileref.ForApproval(ctx, db, a, id, d.ProjectID, d.Classification)
		} else {
			_, err = fileref.Resolve(ctx, db, a, id, d.ProjectID, d.Classification)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func completionReadable(ctx context.Context, db platform.DBTX, a core.Actor, d Record, old *CompletionSnapshot) (bool, error) {
	if err := authz.Require(ctx, db, a, "purchase.read", resource(d)); err != nil {
		return false, err
	}
	var visible bool
	if err := db.QueryRow(ctx, "SELECT COALESCE(can_read_purchase_completion($1,$2,$3,$4,$5),false)", a.OrgID, a.UserID, d.ID, a.Channel, a.RunID != "").Scan(&visible); err != nil || !visible {
		return false, err
	}
	if old != nil {
		for _, c := range old.Costs {
			if err := db.QueryRow(ctx, "SELECT COALESCE(can_read_cost($1,$2,$3,$4,$5,$6),false)", a.OrgID, a.UserID, c.ID, c.RevisionID, a.Channel, a.RunID != "").Scan(&visible); err != nil || !visible {
				return false, err
			}
		}
	}
	if err := completionEvidence(ctx, db, a, d, false); err != nil {
		if core.PublicError(err).Status < 500 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
func completionModes(b CompletionSnapshot) []string {
	for _, c := range b.Costs {
		if c.EntryKind == "expense" && c.State == "confirmed" {
			return []string{"recorded_costs"}
		}
	}
	return []string{"no_payable_obligation"}
}
func (s *Service) CompletionPreview(ctx context.Context, a core.Actor, id string) (CompletionPreview, error) {
	out := CompletionPreview{PurchaseID: id, Visibility: "restricted", Blockers: []string{"restricted"}, SettlementModes: []string{}}
	tx, err := s.completionSnapshotTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	d, err := purchaseRead(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	out.PurchaseVersion = d.Version
	if d.ApprovalState != "approved" {
		out.Visibility = "not_approved"
		out.Blockers = []string{"not_approved"}
		return out, platform.DatabaseError(tx.Commit(ctx))
	}
	visible, err := completionReadable(ctx, tx, a, d, nil)
	if err != nil {
		return out, err
	}
	if !visible {
		return out, platform.DatabaseError(tx.Commit(ctx))
	}
	b, _, fingerprint, blockers, difference, err := completionBasis(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.Visibility = "complete"
	out.Basis = &b
	out.Fingerprint = &fingerprint
	out.Blockers = blockers
	out.HasDifference = difference
	out.SettlementModes = completionModes(b)
	out.CanComplete = a.RunID == "" && slices.Contains([]string{"ordering", "receiving"}, d.Lifecycle) && len(blockers) == 0 && authz.Require(ctx, tx, a, "purchase.complete", resource(d)) == nil && projectWritable(ctx, tx, a, d, false) == nil
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) Completion(ctx context.Context, a core.Actor, id string) (Completion, error) {
	var out Completion
	tx, err := s.completionSnapshotTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	d, err := purchaseRead(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	visible, err := completionReadable(ctx, tx, a, d, nil)
	if err != nil {
		return out, err
	}
	if !visible {
		return out, core.Missing()
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT z.id,z.purchase_id,z.purchase_version,z.occurred_on::text,z.basis,z.fingerprint,z.delivery_verified,z.costs_verified,z.differences_verified,z.settlement_mode,z.attestation,z.difference_disposition,z.actor_id,u.display_name,z.reason,z.created_at
        FROM purchase_completions z JOIN users u ON u.org_id=z.org_id AND u.id=z.actor_id WHERE z.org_id=$1 AND z.purchase_id=$2`, a.OrgID, id).Scan(&out.ID, &out.PurchaseID, &out.PurchaseVersion, &out.OccurredOn, &raw, &out.Fingerprint, &out.DeliveryVerified, &out.CostsVerified, &out.DifferencesVerified, &out.SettlementMode, &out.Attestation, &out.DifferenceDisposition, &out.ActorID, &out.ActorName, &out.Reason, &out.CreatedAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(raw, &out.Basis); err != nil {
		return out, err
	}
	visible, err = completionReadable(ctx, tx, a, d, &out.Basis)
	if err != nil {
		return out, err
	}
	if !visible {
		return Completion{}, core.Missing()
	}
	_, _, now, _, _, err := completionBasis(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.BasisState = "changed"
	if now == out.Fingerprint {
		out.BasisState = "current"
	}
	rows, err := tx.Query(ctx, "SELECT version_id::text FROM purchase_completion_files WHERE org_id=$1 AND completion_id=$2 ORDER BY version_id", a.OrgID, out.ID)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.Attachments = []fileref.Evidence{}
	for _, id := range ids {
		file, e := fileref.Resolve(ctx, tx, a, id, d.ProjectID, d.Classification)
		if e != nil {
			return out, e
		}
		out.Attachments = append(out.Attachments, file)
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
