package finance

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

func validContractFinanceBase(b ContractFinanceBase) error {
	if b.ExpectedVersion < 0 || b.ExpectedVersion >= 9007199254740991 || core.RequireVersion(b.ContractVersion) != nil || core.Name(strings.TrimSpace(b.Reason)) != nil {
		return core.Invalid("basis", "请固定合同与对账版本，并填写操作依据")
	}
	return nil
}
func contractFinanceSource(ctx context.Context, db platform.DBTX, org, id string) (ContractFinanceEvent, error) {
	if !core.ValidID(id) {
		return ContractFinanceEvent{}, core.Missing()
	}
	return scanContractFinanceEvent(db.QueryRow(ctx, `SELECT `+contractFinanceEventColumns+` FROM contract_finance_events e JOIN users u ON u.org_id=e.org_id AND u.id=e.actor_id WHERE e.org_id=$1 AND e.id=$2`, org, id))
}

// All supported writers share project -> contract -> cost -> sorted cash ordering.
func lockContractFinanceSources(ctx context.Context, tx pgx.Tx, a core.Actor, c contractFinanceMeta, extraCost, extraPayment *string) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT id::text FROM projects WHERE org_id=$1 AND (id=$2::uuid OR id IN(
        SELECT x.project_id FROM cost_allocations x JOIN actual_cost_entries d ON d.org_id=x.org_id AND d.current_revision_id=x.revision_id
        WHERE x.org_id=$1 AND (d.id=$4::uuid OR d.id IN(SELECT cost_id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$3)))) ORDER BY id::text`, a.OrgID, c.ProjectID, c.ID, extraCost)
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
		var state, class, creator string
		if err = tx.QueryRow(ctx, "SELECT state,classification,creator_id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id).Scan(&state, &class, &creator); err != nil {
			return err
		}
		if err = authz.Require(ctx, tx, a, "project.read", authz.Resource{Type: "project", ID: id, ProjectID: &id, OwnerID: &creator, Classification: class}); err != nil {
			return err
		}
		if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
			return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许改变合同财务归属")
		}
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, c.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM actual_cost_entries WHERE org_id=$1 AND (id=$3::uuid OR id IN(SELECT cost_id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$2)) ORDER BY id FOR UPDATE`, a.OrgID, c.ID, extraCost); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT p.id FROM payment_records p WHERE p.org_id=$1 AND (p.id=$3::uuid OR p.id IN(SELECT payment_id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$2)
        OR p.id IN(SELECT r.payment_id FROM payment_revisions r WHERE r.org_id=$1 AND r.refund_of_id IN(SELECT payment_id FROM contract_finance_events WHERE org_id=$1 AND contract_id=$2))) ORDER BY p.id FOR UPDATE`, a.OrgID, c.ID, extraPayment)
	return err
}
func (s *Service) mutateContractFinance(ctx context.Context, cmd core.Command, id string, b ContractFinanceBase, extraCost, extraPayment *string, fn func(pgx.Tx, contractFinanceMeta, int64) (ContractFinanceEvent, []string, error)) (core.Result, error) {
	if err := validContractFinanceBase(b); err != nil {
		return core.Result{}, err
	}
	check := func(tx pgx.Tx) (contractFinanceMeta, error) {
		c, e := contractFinanceMetadata(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return c, e
		}
		if e = authz.Require(ctx, tx, cmd.Actor, "contract.read", contractFinanceResource(c)); e != nil {
			return c, e
		}
		if e = authz.Require(ctx, tx, cmd.Actor, "contract.reconcile", contractFinanceResource(c)); e != nil {
			return c, e
		}
		if e = identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); e != nil {
			return c, e
		}
		for _, axis := range []string{"cost", "payment"} {
			ok, x := contractFinanceAxisReadable(ctx, tx, cmd.Actor, c, axis)
			if x != nil {
				return c, x
			}
			if !ok {
				return c, core.Missing()
			}
		}
		if extraCost != nil {
			x, e := metadata(ctx, tx, cmd.Actor.OrgID, *extraCost)
			if e != nil {
				return c, e
			}
			if e = readable(ctx, tx, cmd.Actor, x, x.CurrentRevisionID); e != nil {
				return c, e
			}
		}
		if extraPayment != nil {
			x, e := paymentMetadata(ctx, tx, cmd.Actor.OrgID, *extraPayment)
			if e != nil {
				return c, e
			}
			if e = paymentReadable(ctx, tx, cmd.Actor, x); e != nil {
				return c, e
			}
		}
		return c, nil
	}
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		c, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if err = lockContractFinanceSources(ctx, tx, cmd.Actor, c, extraCost, extraPayment); err != nil {
			return core.Result{}, err
		}
		c, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if c.Version != b.ContractVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "合同已变化，已保留输入，请核对最新合同")
		}
		if !financialContractStage(c.Lifecycle) || c.ActiveRevisionID == nil {
			return core.Result{}, core.Fail(409, "CONTRACT_NOT_EFFECTIVE", "仅已生效合同及其结束后的财务更正允许登记节点归属")
		}
		var version int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM contract_finance_heads WHERE org_id=$1 AND contract_id=$2),0)", cmd.Actor.OrgID, id).Scan(&version); err != nil {
			return core.Result{}, err
		}
		if version != b.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "合同对账已变化，已保留输入，请核对最新归属")
		}
		e, files, err := fn(tx, c, version)
		if err != nil {
			return core.Result{}, err
		}
		e.ID = core.NewID()
		e.ContractID = c.ID
		e.ContractRevisionID = *c.ActiveRevisionID
		e.Seq = version + 1
		e.Reason = strings.TrimSpace(b.Reason)
		tag, err := tx.Exec(ctx, `INSERT INTO contract_finance_events(id,org_id,contract_id,milestone_id,contract_revision_id,seq,kind,cost_id,payment_id,source_version,source_revision_id,source_content_hash,source_event_id,amount,
            review_snapshot,review_fingerprint,conditions_satisfied,acknowledge_difference,acknowledge_unattributed_refunds,actor_id,session_id,mfa_verified_at,reason,refund_fx_acknowledged)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,s.id,s.mfa_verified_at,$21,$23 FROM sessions s WHERE s.org_id=$2 AND s.id=$22 AND s.user_id=$20`,
			e.ID, cmd.Actor.OrgID, c.ID, e.MilestoneID, e.ContractRevisionID, e.Seq, e.Kind, e.CostID, e.PaymentID, e.SourceVersion, e.SourceRevisionID, e.SourceContentHash, e.SourceEventID, e.Amount,
			e.ReviewSnapshot, e.ReviewFingerprint, e.ConditionsSatisfied, e.AcknowledgeDifference, e.AcknowledgeUnattributedRefunds, cmd.Actor.UserID, e.Reason, cmd.Actor.SessionID, e.RefundFXAcknowledged)
		if err != nil {
			return core.Result{}, err
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Forbidden()
		}
		for _, file := range files {
			if _, err = tx.Exec(ctx, "INSERT INTO contract_finance_event_files(org_id,event_id,version_id) VALUES($1,$2,$3)", cmd.Actor.OrgID, e.ID, file); err != nil {
				return core.Result{}, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO contract_finance_heads(org_id,contract_id,version,current_event_id) VALUES($1,$2,$3,$4)
            ON CONFLICT(org_id,contract_id) DO UPDATE SET version=EXCLUDED.version,current_event_id=EXCLUDED.current_event_id,updated_at=clock_timestamp()`, cmd.Actor.OrgID, id, e.Seq, e.ID); err != nil {
			return core.Result{}, err
		}
		receipt := ContractFinanceReceipt{ContractID: id, MilestoneID: e.MilestoneID, Version: e.Seq, EventID: e.ID}
		return core.Result{Data: receipt, Event: core.Event{Type: "contract.finance." + e.Kind, ResourceType: "contract", ResourceID: id, Version: e.Seq,
			Payload: map[string]any{"reason": e.Reason, "milestone_id": e.MilestoneID, "finance_event_id": e.ID, "contract_revision_id": e.ContractRevisionID}}}, nil
	})
}
func contractFinanceNodeBasis(ctx context.Context, db platform.DBTX, org, id, mid string) (ContractFinanceNode, error) {
	var n ContractFinanceNode
	if !core.ValidID(mid) {
		return n, core.Missing()
	}
	err := db.QueryRow(ctx, "SELECT id,direction,currency,amount::text FROM contract_milestones WHERE org_id=$1 AND contract_id=$2 AND id=$3", org, id, mid).Scan(&n.ID, &n.Direction, &n.Currency, &n.Amount)
	return n, platform.DatabaseError(err)
}
func (s *Service) LinkContractFinance(ctx context.Context, cmd core.Command, id string, in ContractFinanceLink) (core.Result, error) {
	if !core.ValidID(in.MilestoneID) || !core.ValidID(in.SourceID) || !core.ValidID(in.SourceRevisionID) || core.RequireVersion(in.SourceVersion) != nil || len(in.SourceContentHash) != 64 || strings.Trim(in.SourceContentHash, "0123456789abcdef") != "" || !slices.Contains([]string{"cost", "cash", "refund"}, in.Kind) || ((in.Kind == "refund") != (in.SourceEventID != nil)) || (in.SourceEventID != nil && !core.ValidID(*in.SourceEventID)) {
		return core.Result{}, core.Invalid("source", "请固定准确节点、来源版本、修订、摘要和归属类型")
	}
	if in.Kind != "refund" && in.RefundFXAcknowledged {
		return core.Result{}, core.Invalid("refund_fx_acknowledged", "仅异币退款归属接受本金核实声明")
	}
	var costID, paymentID *string
	if in.Kind == "cost" {
		costID = &in.SourceID
	} else {
		paymentID = &in.SourceID
	}
	return s.mutateContractFinance(ctx, cmd, id, in.ContractFinanceBase, costID, paymentID, func(tx pgx.Tx, c contractFinanceMeta, version int64) (ContractFinanceEvent, []string, error) {
		e := ContractFinanceEvent{MilestoneID: in.MilestoneID, Kind: in.Kind, CostID: costID, PaymentID: paymentID, SourceVersion: &in.SourceVersion, SourceRevisionID: &in.SourceRevisionID, SourceContentHash: &in.SourceContentHash, SourceEventID: in.SourceEventID}
		n, err := contractFinanceNodeBasis(ctx, tx, cmd.Actor.OrgID, id, in.MilestoneID)
		if err != nil {
			return e, nil, err
		}
		amount, err := paymentAmount(ctx, tx, n.Currency, in.Amount)
		if err != nil {
			return e, nil, err
		}
		text := amount.String()
		e.Amount = &text
		var total, used string
		if in.Kind == "cost" {
			source, err := metadata(ctx, tx, cmd.Actor.OrgID, in.SourceID)
			if err != nil {
				return e, nil, err
			}
			if err = authz.Require(ctx, tx, cmd.Actor, "cost.record", resource(source)); err != nil {
				return e, nil, err
			}
			if source.Version != in.SourceVersion || source.CurrentRevisionID != in.SourceRevisionID || source.ContentHash != in.SourceContentHash {
				return e, nil, core.Conflict()
			}
			if source.State != "confirmed" || source.EntryKind != "expense" || n.Direction != "payable" {
				return e, nil, core.Fail(409, "CONTRACT_COST_NOT_ELIGIBLE", "应付节点须选择已确认的原费用")
			}
			total = value(source.Amount)
			err = tx.QueryRow(ctx, "SELECT COALESCE(sum(contract_finance_remaining($1,id)),0)::text FROM contract_finance_events WHERE org_id=$1 AND cost_id=$2 AND kind='cost'", cmd.Actor.OrgID, in.SourceID).Scan(&used)
			if err != nil {
				return e, nil, err
			}
		} else {
			source, err := paymentMetadata(ctx, tx, cmd.Actor.OrgID, in.SourceID)
			if err != nil {
				return e, nil, err
			}
			if err = authz.Require(ctx, tx, cmd.Actor, "payment.record", paymentResource(source)); err != nil {
				return e, nil, err
			}
			if source.Version != in.SourceVersion || source.CurrentRevisionID != in.SourceRevisionID || source.ContentHash != in.SourceContentHash {
				return e, nil, core.Conflict()
			}
			if source.State != "recorded" || source.RecordKind != "payment" {
				return e, nil, core.Fail(409, "CONTRACT_CASH_NOT_ELIGIBLE", "须选择已实际登记且未冲销的原资金")
			}
			if in.Kind == "refund" && source.RefundFX != nil {
				if n.Direction != "payable" || source.RefundFX.OriginalCurrency != n.Currency || !in.RefundFXAcknowledged {
					return e, nil, core.Invalid("refund_fx_acknowledged", "异币退款归属须按合同币种核实退回原本金，不能将实收币直接作为合同金额")
				}
				e.RefundFXAcknowledged = true
				total = source.RefundFX.OriginalAmount
			} else {
				if in.RefundFXAcknowledged {
					return e, nil, core.Invalid("refund_fx_acknowledged", "同币归属不接受异币退款声明")
				}
				total = value(source.Amount)
			}
			err = tx.QueryRow(ctx, "SELECT COALESCE(sum(contract_finance_remaining($1,id)),0)::text FROM contract_finance_events WHERE org_id=$1 AND payment_id=$2 AND kind IN ('cash','refund')", cmd.Actor.OrgID, in.SourceID).Scan(&used)
			if err != nil {
				return e, nil, err
			}
			if in.Kind == "refund" {
				original, err := contractFinanceSource(ctx, tx, cmd.Actor.OrgID, value(in.SourceEventID))
				if err != nil {
					return e, nil, err
				}
				if original.ContractID != id || original.MilestoneID != n.ID || original.Kind != "cash" || original.RemainingAmount == nil {
					return e, nil, core.Invalid("source_event_id", "退款须固定该节点原正向资金归属")
				}
				var returned string
				if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(contract_finance_remaining($1,e.id)),0)::text FROM contract_finance_events e JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id WHERE e.org_id=$1 AND e.source_event_id=$2 AND e.kind='refund' AND p.state='recorded'`, cmd.Actor.OrgID, original.ID).Scan(&returned); err != nil {
					return e, nil, err
				}
				remaining, _ := core.ParseDecimal(*original.RemainingAmount)
				back, _ := core.ParseDecimal(returned)
				if back.Add(amount).Cmp(remaining) > 0 {
					return e, nil, core.Fail(409, "CONTRACT_RETURN_EXCEEDED", "退回金额超过原归属当前可退金额")
				}
				if n.Direction == "receivable" {
					if err = paymentBalances(ctx, tx, cmd.Actor.OrgID, &source); err != nil {
						return e, nil, err
					}
					available, _ := core.ParseDecimal(value(source.UnallocatedAmount))
					if amount.Cmp(available) > 0 {
						return e, nil, core.Fail(409, "PAYMENT_BALANCE_EXCEEDED", "退回款占用的金额已用于费用或其他收款退回")
					}
				}
			}
		}
		gross, _ := core.ParseDecimal(total)
		assigned, _ := core.ParseDecimal(used)
		if assigned.Add(amount).Cmp(gross) > 0 {
			return e, nil, core.Fail(409, "CONTRACT_SOURCE_EXCEEDED", "该来源已被其他节点归属，请核对剩余金额")
		}
		return e, nil, nil
	})
}
func (s *Service) ReleaseContractFinance(ctx context.Context, cmd core.Command, id, eventID string, in ContractFinanceRelease) (core.Result, error) {
	if !core.ValidID(eventID) {
		return core.Result{}, core.Missing()
	}
	return s.mutateContractFinance(ctx, cmd, id, in.ContractFinanceBase, nil, nil, func(tx pgx.Tx, c contractFinanceMeta, version int64) (ContractFinanceEvent, []string, error) {
		e := ContractFinanceEvent{Kind: "release", SourceEventID: &eventID}
		original, err := contractFinanceSource(ctx, tx, cmd.Actor.OrgID, eventID)
		if err != nil {
			return e, nil, err
		}
		if original.ContractID != id || !slices.Contains([]string{"cost", "cash", "refund"}, original.Kind) {
			return e, nil, core.Missing()
		}
		e.MilestoneID = original.MilestoneID
		n, err := contractFinanceNodeBasis(ctx, tx, cmd.Actor.OrgID, id, e.MilestoneID)
		if err != nil {
			return e, nil, err
		}
		amount, err := paymentAmount(ctx, tx, n.Currency, in.Amount)
		if err != nil {
			return e, nil, err
		}
		remaining, _ := core.ParseDecimal(value(original.RemainingAmount))
		if amount.Cmp(remaining) > 0 {
			return e, nil, core.Fail(409, "CONTRACT_RELEASE_EXCEEDED", "解除金额超过该归属尚未解除部分")
		}
		if original.Kind == "cash" {
			var returned string
			err = tx.QueryRow(ctx, `SELECT COALESCE(sum(contract_finance_remaining($1,e.id)),0)::text FROM contract_finance_events e JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id WHERE e.org_id=$1 AND e.source_event_id=$2 AND e.kind='refund' AND p.state='recorded'`, cmd.Actor.OrgID, eventID).Scan(&returned)
			if err != nil {
				return e, nil, err
			}
			back, _ := core.ParseDecimal(returned)
			if remaining.Sub(amount).Cmp(back) < 0 {
				return e, nil, core.Fail(409, "CONTRACT_RETURN_STILL_LINKED", "请先核对或解除已退回部分，不能使原归属小于实际退回")
			}
		}
		text := amount.String()
		e.Amount = &text
		return e, nil, nil
	})
}
func (s *Service) ReviewContractFinance(ctx context.Context, cmd core.Command, id, mid string, in ContractFinanceReview) (core.Result, error) {
	if !core.ValidID(mid) || len(in.ExpectedFingerprint) != 32 || strings.Trim(in.ExpectedFingerprint, "0123456789abcdef") != "" || len(in.EvidenceVersionIDs) < 1 || len(in.EvidenceVersionIDs) > 20 {
		return core.Result{}, core.Invalid("review", "请固定当前来源指纹及1至20项准确凭证")
	}
	files := append([]string{}, in.EvidenceVersionIDs...)
	slices.Sort(files)
	for i, f := range files {
		if !core.ValidID(f) || i > 0 && f == files[i-1] {
			return core.Result{}, core.Invalid("evidence_version_ids", "核对凭证无效或重复")
		}
	}
	return s.mutateContractFinance(ctx, cmd, id, in.ContractFinanceBase, nil, nil, func(tx pgx.Tx, c contractFinanceMeta, version int64) (ContractFinanceEvent, []string, error) {
		e := ContractFinanceEvent{Kind: "review", MilestoneID: mid, ReviewFingerprint: &in.ExpectedFingerprint, ConditionsSatisfied: &in.ConditionsSatisfied, AcknowledgeDifference: &in.AcknowledgeDifference, AcknowledgeUnattributedRefunds: &in.AcknowledgeUnattributedRefunds}
		var snapshot json.RawMessage
		var fingerprint string
		if _, err := contractFinanceNodeBasis(ctx, tx, cmd.Actor.OrgID, id, mid); err != nil {
			return e, nil, err
		}
		if err := tx.QueryRow(ctx, "SELECT contract_finance_snapshot($1,$2),md5(contract_finance_snapshot($1,$2)::text)", cmd.Actor.OrgID, mid).Scan(&snapshot, &fingerprint); err != nil {
			return e, nil, err
		}
		if fingerprint != in.ExpectedFingerprint {
			return e, nil, core.Fail(409, "CONTRACT_FINANCE_BASIS_CHANGED", "实际费用、资金或退款已变化，请保留输入并核对最新依据")
		}
		view, err := contractFinanceView(ctx, tx, cmd.Actor, c)
		if err != nil {
			return e, nil, err
		}
		for _, n := range view.Nodes {
			if n.ID == mid {
				actual, _ := core.ParseDecimal(value(n.CashAmount))
				planned, _ := core.ParseDecimal(n.Amount)
				if actual.Cmp(planned) != 0 && !in.AcknowledgeDifference {
					return e, nil, core.Invalid("acknowledge_difference", "请明确核对净收付款与计划金额的差额")
				}
				if n.HasUnattributedRefunds != nil && *n.HasUnattributedRefunds && !in.AcknowledgeUnattributedRefunds {
					return e, nil, core.Invalid("acknowledge_unattributed_refunds", "请核实尚未归属的退款对本节点的影响")
				}
			}
		}
		for _, file := range files {
			if _, err = fileref.ForApproval(ctx, tx, cmd.Actor, file, c.ProjectID, c.Classification); err != nil {
				return e, nil, err
			}
		}
		e.ReviewSnapshot = snapshot
		return e, files, nil
	})
}
