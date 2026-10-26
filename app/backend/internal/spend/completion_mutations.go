package spend

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func completionSourcePlan(ctx context.Context, db platform.DBTX, org, id string) ([]string, []string, []string, error) {
	var costs, projects, payments []string
	err := db.QueryRow(ctx, `WITH costs AS(SELECT id FROM purchase_completion_cost_ids($1,$2)),cash AS(SELECT id FROM purchase_completion_cash_ids($1,$2))
        SELECT ARRAY(SELECT id::text FROM costs ORDER BY id),ARRAY(SELECT p.id::text FROM projects p WHERE p.org_id=$1 AND
            (p.id=(SELECT project_id FROM spend_documents WHERE org_id=$1 AND id=$2)
            OR p.id IN(SELECT project_id FROM actual_cost_entries WHERE org_id=$1 AND id IN(SELECT id FROM costs))
            OR p.id IN(SELECT a.project_id FROM cost_allocations a JOIN actual_cost_entries c ON c.org_id=a.org_id AND c.current_revision_id=a.revision_id WHERE a.org_id=$1 AND c.id IN(SELECT id FROM costs))
            OR p.id IN(SELECT project_id FROM payment_records WHERE org_id=$1 AND id IN(SELECT id FROM cash))) ORDER BY p.id),
            ARRAY(SELECT id::text FROM cash ORDER BY id)`, org, id).Scan(&costs, &projects, &payments)
	return costs, projects, payments, err
}
func lockCompletionSources(ctx context.Context, tx pgx.Tx, a core.Actor, d Record) error {
	costs, projects, _, err := completionSourcePlan(ctx, tx, a.OrgID, d.ID)
	if err != nil {
		return err
	}
	for _, id := range projects {
		var state, class, owner string
		if err = tx.QueryRow(ctx, "SELECT state,classification,creator_id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id).Scan(&state, &class, &owner); err != nil {
			return err
		}
		if err = authz.Require(ctx, tx, a, "project.read", authz.Resource{Type: "project", ID: id, ProjectID: &id, OwnerID: &owner, Classification: class}); err != nil {
			return err
		}
		if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
			return core.Fail(409, "PROJECT_READ_ONLY", "关联项目当前阶段不允许登记采购完成")
		}
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM spend_documents WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, d.ID); err != nil {
		return err
	}
	nextCosts, nextProjects, _, err := completionSourcePlan(ctx, tx, a.OrgID, d.ID)
	if err != nil {
		return err
	}
	conflict := func() error {
		return core.Fail(409, "PURCHASE_COMPLETION_BASIS_CHANGED", "关联范围已变化，请保留输入并核对完整最新依据")
	}
	if !slices.Equal(costs, nextCosts) || !slices.Equal(projects, nextProjects) {
		return conflict()
	}
	for _, id := range costs {
		if _, err = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return err
		}
	}
	nextCosts, nextProjects, payments, err := completionSourcePlan(ctx, tx, a.OrgID, d.ID)
	if err != nil {
		return err
	}
	if !slices.Equal(costs, nextCosts) || !slices.Equal(projects, nextProjects) {
		return conflict()
	}
	for _, id := range payments {
		if _, err = tx.Exec(ctx, "SELECT id FROM payment_records WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return err
		}
	}
	nextCosts, nextProjects, nextPayments, err := completionSourcePlan(ctx, tx, a.OrgID, d.ID)
	if err != nil {
		return err
	}
	if !slices.Equal(costs, nextCosts) || !slices.Equal(projects, nextProjects) || !slices.Equal(payments, nextPayments) {
		return conflict()
	}
	return projectWritable(ctx, tx, a, d, false)
}
func (s *Service) CompletePurchase(ctx context.Context, c core.Command, id string, in CompletionInput) (core.Result, error) {
	if err := executionBase(&in.ExecutionBase); err != nil {
		return core.Result{}, err
	}
	in.Attestation = strings.TrimSpace(in.Attestation)
	in.DifferenceDisposition = strings.TrimSpace(in.DifferenceDisposition)
	if len(in.ExpectedFingerprint) != 32 || strings.Trim(in.ExpectedFingerprint, "0123456789abcdef") != "" || core.Date(in.OccurredOn) != nil ||
		!in.DeliveryVerified || !in.CostsVerified || !in.DifferencesVerified || !slices.Contains([]string{"recorded_costs", "no_payable_obligation"}, in.SettlementMode) ||
		len([]rune(in.Attestation)) < 1 || len([]rune(in.Attestation)) > 20000 || len([]rune(in.DifferenceDisposition)) > 20000 {
		return core.Result{}, core.Invalid("completion", "请固定完整核对指纹、实际日期、明确结算方式及三项人工核实说明")
	}
	files, err := evidenceIDs(in.EvidenceVersionIDs)
	if err != nil {
		return core.Result{}, err
	}
	check := func(tx pgx.Tx) (Record, error) {
		d, e := purchaseRead(ctx, tx, c.Actor, id)
		if e != nil {
			return d, e
		}
		if e = authz.Require(ctx, tx, c.Actor, "purchase.complete", resource(d)); e != nil {
			return d, e
		}
		if e = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); e != nil {
			return d, e
		}
		visible, e := completionReadable(ctx, tx, c.Actor, d, nil)
		if e != nil {
			return d, e
		}
		if !visible {
			return d, core.Missing()
		}
		return d, nil
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		d, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = lockCompletionSources(ctx, tx, c.Actor, d); e != nil {
			return core.Result{}, e
		}
		d, e = check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if d.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if e = approvedPurchase(d, in.ExecutionBase, false); e != nil {
			return core.Result{}, e
		}
		if !slices.Contains([]string{"ordering", "receiving"}, d.Lifecycle) {
			return core.Result{}, core.Fail(409, "PURCHASE_NOT_OPEN", "仅已批准且实际履行中的采购可以登记完成")
		}
		b, raw, fingerprint, blockers, difference, e := completionBasis(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if fingerprint != in.ExpectedFingerprint {
			return core.Result{}, core.Fail(409, "PURCHASE_COMPLETION_BASIS_CHANGED", "订单、验收、差异或实际结算已变化，请保留输入并重新核对")
		}
		if len(blockers) > 0 {
			return core.Result{}, core.Fail(409, "PURCHASE_COMPLETION_UNRESOLVED", "仍有未结交付、差异审批、费用或退款，不能登记完成")
		}
		if !slices.Contains(completionModes(b), in.SettlementMode) {
			return core.Result{}, core.Invalid("settlement_mode", "核实方式与当前实际费用记录不符")
		}
		if difference && in.DifferenceDisposition == "" {
			return core.Result{}, core.Invalid("difference_disposition", "请说明未使用额度、取消余量、未启用提案及费用差异的处理")
		}
		var dateOK bool
		if e = tx.QueryRow(ctx, `SELECT $3::date BETWEEN (SELECT max(received_on) FROM purchase_receipts WHERE org_id=$1 AND purchase_id=$2) AND (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date`, c.Actor.OrgID, id, in.OccurredOn).Scan(&dateOK); e != nil {
			return core.Result{}, e
		}
		if !dateOK {
			return core.Result{}, core.Invalid("occurred_on", "完成日期须在全部既有交付日期与香港当前日期之间")
		}
		if e = completionEvidence(ctx, tx, c.Actor, d, true); e != nil {
			return core.Result{}, e
		}
		for _, file := range files {
			if _, e = fileref.ForApproval(ctx, tx, c.Actor, file, d.ProjectID, d.Classification); e != nil {
				return core.Result{}, e
			}
		}
		completionID := core.NewID()
		tag, e := tx.Exec(ctx, `INSERT INTO purchase_completions(id,org_id,purchase_id,revision_id,purchase_version,occurred_on,basis,fingerprint,delivery_verified,costs_verified,differences_verified,settlement_mode,attestation,difference_disposition,evidence_count,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,s.id,s.mfa_verified_at,$17 FROM sessions s WHERE s.org_id=$2 AND s.id=$18 AND s.user_id=$16`,
			completionID, c.Actor.OrgID, id, in.RevisionID, d.Version+1, in.OccurredOn, raw, fingerprint, in.DeliveryVerified, in.CostsVerified, in.DifferencesVerified, in.SettlementMode, in.Attestation, in.DifferenceDisposition, len(files), c.Actor.UserID, in.Reason, c.Actor.SessionID)
		if e != nil {
			return core.Result{}, e
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Forbidden()
		}
		for _, file := range files {
			if _, e = tx.Exec(ctx, "INSERT INTO purchase_completion_files(org_id,completion_id,version_id) VALUES($1,$2,$3)", c.Actor.OrgID, completionID, file); e != nil {
				return core.Result{}, e
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO purchase_execution_events(id,org_id,purchase_id,seq,purchase_version,kind,lifecycle,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,(SELECT COALESCE(max(seq),0)+1 FROM purchase_execution_events WHERE org_id=$2 AND purchase_id=$3),$4,'purchase_completed','completed',$5,s.id,s.mfa_verified_at,$6 FROM sessions s WHERE s.org_id=$2 AND s.id=$7`, core.NewID(), c.Actor.OrgID, id, d.Version+1, c.Actor.UserID, in.Reason, c.Actor.SessionID)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE spend_documents SET lifecycle='completed',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		before, _ := json.Marshal(d)
		return core.Result{Status: 201, Data: CompletionCommandReceipt{PurchaseID: id, Version: d.Version + 1, CompletionID: completionID}, Event: core.Event{Type: "purchase.completed", ResourceType: "purchase", ResourceID: id, Version: d.Version + 1, BeforeDigest: core.Digest(before), Payload: map[string]any{"reason": in.Reason, "revision_id": in.RevisionID, "completion_id": completionID, "basis_fingerprint": fingerprint}}}, nil
	})
}
