package finance

import (
	"context"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func closureSourcePlan(ctx context.Context, db platform.DBTX, org, id string) ([]string, []string, []string, error) {
	var costs, projects, payments []string
	err := db.QueryRow(ctx, `WITH target_costs AS(SELECT cost_id id FROM cost_links WHERE org_id=$1 AND source_type='contract' AND source_id=$2
        UNION SELECT cost_id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$2 AND cost_id IS NOT NULL),
        cash AS(SELECT payment_id id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$2 AND payment_id IS NOT NULL
            UNION SELECT payment_id FROM payment_allocations WHERE org_id=$1 AND cost_id IN(SELECT id FROM target_costs)),
        all_cash AS(SELECT id FROM cash UNION SELECT p.id FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
            WHERE p.org_id=$1 AND (r.refund_of_id IN(SELECT id FROM cash) OR p.id IN(SELECT r.refund_of_id FROM payment_revisions r WHERE r.org_id=$1 AND r.payment_id IN(SELECT id FROM cash))))
        SELECT ARRAY(SELECT id::text FROM target_costs ORDER BY id),
            ARRAY(SELECT id::text FROM projects WHERE org_id=$1 AND (id=(SELECT project_id FROM contracts WHERE org_id=$1 AND id=$2)
                OR id IN(SELECT a.project_id FROM cost_allocations a JOIN actual_cost_entries d ON d.org_id=a.org_id AND d.current_revision_id=a.revision_id WHERE a.org_id=$1 AND d.id IN(SELECT id FROM target_costs))) ORDER BY id),
            ARRAY(SELECT id::text FROM all_cash ORDER BY id)`, org, id).Scan(&costs, &projects, &payments)
	return costs, projects, payments, err
}
func lockClosureSources(ctx context.Context, tx pgx.Tx, a core.Actor, c contractFinanceMeta) error {
	costs, projects, _, err := closureSourcePlan(ctx, tx, a.OrgID, c.ID)
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
			return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许登记合同完成或终止")
		}
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, c.ID); err != nil {
		return err
	}
	nextCosts, nextProjects, _, err := closureSourcePlan(ctx, tx, a.OrgID, c.ID)
	if err != nil {
		return err
	}
	if !slices.Equal(costs, nextCosts) || !slices.Equal(projects, nextProjects) {
		return core.Fail(409, "CONTRACT_CLOSURE_BASIS_CHANGED", "关联范围已变化，请保留输入并核对最新依据")
	}
	for _, id := range costs {
		if _, err = tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return err
		}
	}
	_, _, payments, err := closureSourcePlan(ctx, tx, a.OrgID, c.ID)
	if err != nil {
		return err
	}
	for _, id := range payments {
		if _, err = tx.Exec(ctx, "SELECT id FROM payment_records WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) CloseContract(ctx context.Context, cmd core.Command, id string, in ContractClosureInput) (core.Result, error) {
	in.Attestation = strings.TrimSpace(in.Attestation)
	in.UnsettledDisposition = strings.TrimSpace(in.UnsettledDisposition)
	in.Reason = strings.TrimSpace(in.Reason)
	if core.RequireVersion(in.ExpectedVersion) != nil || !core.ValidID(in.RevisionID) || in.ExpectedFinanceVersion < 0 || in.ExpectedFinanceVersion >= 9007199254740991 ||
		len(in.ExpectedFingerprint) != 32 || strings.Trim(in.ExpectedFingerprint, "0123456789abcdef") != "" || !slices.Contains([]string{"completed", "terminated"}, in.Action) ||
		core.Date(in.OccurredOn) != nil || !in.TermsVerified || len([]rune(in.Attestation)) < 1 || len([]rune(in.Attestation)) > 20000 || len([]rune(in.UnsettledDisposition)) > 20000 || core.Name(in.Reason) != nil || len(in.EvidenceVersionIDs) < 1 || len(in.EvidenceVersionIDs) > 20 {
		return core.Result{}, core.Invalid("closure", "请固定准确合同与当前依据、真实日期、条款核实说明和1至20项凭证")
	}
	files := append([]string{}, in.EvidenceVersionIDs...)
	slices.Sort(files)
	for i, id := range files {
		if !core.ValidID(id) || i > 0 && id == files[i-1] {
			return core.Result{}, core.Invalid("evidence_version_ids", "凭证版本无效或重复")
		}
	}
	check := func(tx pgx.Tx) (contractFinanceMeta, error) {
		c, err := contractFinanceMetadata(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return c, err
		}
		if err = authz.Require(ctx, tx, cmd.Actor, "contract.close", contractFinanceResource(c)); err != nil {
			return c, err
		}
		if err = identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); err != nil {
			return c, err
		}
		visible, err := contractClosureReadable(ctx, tx, cmd.Actor, c)
		if err != nil {
			return c, err
		}
		if !visible {
			return c, core.Missing()
		}
		return c, nil
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		// Shared with signature/activation before any project or contract lock.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('contract-lifecycle:'||$1::text,0))", cmd.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		c, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if err = lockClosureSources(ctx, tx, cmd.Actor, c); err != nil {
			return core.Result{}, err
		}
		c, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if c.Version != in.ExpectedVersion || value(c.ActiveRevisionID) != in.RevisionID {
			return core.Result{}, core.Conflict()
		}
		if c.Lifecycle != "effective" {
			return core.Result{}, core.Fail(409, "CONTRACT_NOT_EFFECTIVE", "仅当前生效合同可以登记完成或终止")
		}
		basis, raw, fingerprint, err := contractClosureBasis(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if basis.FinanceVersion != in.ExpectedFinanceVersion || fingerprint != in.ExpectedFingerprint {
			return core.Result{}, core.Fail(409, "CONTRACT_CLOSURE_BASIS_CHANGED", "财务、结算或补充协议已变化，请保留输入并核对最新依据")
		}
		blockers := contractClosureBlockers(basis)
		if slices.Contains(blockers, "supplement_effective") {
			return core.Result{}, core.Fail(409, "CONTRACT_EFFECTIVE_SUPPLEMENTS", "请先处理仍生效的补充协议")
		}
		if in.Action == "completed" && len(blockers) > 0 {
			return core.Result{}, core.Fail(409, "CONTRACT_UNSETTLED_OBLIGATIONS", "仍有未结节点、条件、费用或补充协议，不能登记完成")
		}
		if in.Action == "terminated" && len(blockers) > 0 && (!in.AcknowledgeUnsettled || in.UnsettledDisposition == "") {
			return core.Result{}, core.Invalid("unsettled_disposition", "请明确确认未结清单并说明各项后续处理")
		}
		var dateOK bool
		if err = tx.QueryRow(ctx, "SELECT $3::date BETWEEN start_date AND (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date FROM contract_revisions WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, in.RevisionID, in.OccurredOn).Scan(&dateOK); err != nil {
			return core.Result{}, err
		}
		if !dateOK {
			return core.Result{}, core.Invalid("occurred_on", "发生日期须介于合同生效日和香港当前日期之间")
		}
		for _, id := range files {
			if _, err = fileref.ForApproval(ctx, tx, cmd.Actor, id, c.ProjectID, c.Classification); err != nil {
				return core.Result{}, err
			}
		}
		closureID := core.NewID()
		tag, err := tx.Exec(ctx, `INSERT INTO contract_closures(id,org_id,contract_id,revision_id,contract_version,finance_version,action,occurred_on,basis,fingerprint,
            terms_verified,attestation,acknowledge_unsettled,unsettled_disposition,evidence_count,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,s.id,s.mfa_verified_at,$17 FROM sessions s WHERE s.org_id=$2 AND s.id=$18 AND s.user_id=$16`,
			closureID, cmd.Actor.OrgID, id, in.RevisionID, c.Version+1, basis.FinanceVersion, in.Action, in.OccurredOn, raw, fingerprint, in.TermsVerified, in.Attestation, in.AcknowledgeUnsettled, in.UnsettledDisposition, len(files), cmd.Actor.UserID, in.Reason, cmd.Actor.SessionID)
		if err != nil {
			return core.Result{}, err
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Forbidden()
		}
		for _, file := range files {
			if _, err = tx.Exec(ctx, "INSERT INTO contract_closure_files(org_id,closure_id,version_id) VALUES($1,$2,$3)", cmd.Actor.OrgID, closureID, file); err != nil {
				return core.Result{}, err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE contracts SET lifecycle=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", cmd.Actor.OrgID, id, in.Action); err != nil {
			return core.Result{}, err
		}
		receipt := ContractClosureReceipt{ContractID: id, Version: c.Version + 1, ClosureID: closureID, Action: in.Action}
		return core.Result{Status: 201, Data: receipt, Event: core.Event{Type: "contract." + in.Action, ResourceType: "contract", ResourceID: id, Version: c.Version + 1, Payload: map[string]any{"reason": in.Reason, "closure_id": closureID, "revision_id": in.RevisionID, "basis_fingerprint": fingerprint}}}, nil
	})
}
