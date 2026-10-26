package spend

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func lockVariancePurchase(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Record, error) {
	p, err := purchaseRead(ctx, db, a, id)
	if err != nil {
		return p, err
	}
	if err = projectWritable(ctx, db, a, p, true); err != nil {
		return p, err
	}
	if _, err = db.Exec(ctx, "SELECT id FROM spend_documents WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
		return p, err
	}
	p, err = purchaseRead(ctx, db, a, id)
	if err != nil {
		return p, err
	}
	if p.ApprovalState != "approved" || p.Lifecycle == "cancelled" || p.Lifecycle == "completed" || p.DeletedAt != nil {
		return p, core.Fail(409, "PURCHASE_VARIANCE_UNAVAILABLE", "仅已批准且未取消／完成的采购可调整执行基线")
	}
	return p, nil
}

func normalizeVariance(ctx context.Context, db platform.DBTX, a core.Actor, d Record, p Record, c Content) (Content, []LineValue, *string, error) {
	normalizer := d
	normalizer.Kind = "purchase"
	c, values, amount, _, err := normalize(ctx, db, a, normalizer, c, &p.Content, false)
	if err != nil {
		return c, nil, nil, err
	}
	if c.Currency == nil || p.Currency == nil || *c.Currency != *p.Currency || c.PricingBasis != p.PricingBasis || c.RoundingMode != p.RoundingMode || len(c.Lines) != len(p.Lines) || amount == nil {
		return c, nil, nil, core.Invalid("content", "差异提案须保留原币、计价与舍入规则及全部准确行，并完整填写新执行额度")
	}
	original := map[string]Line{}
	for _, l := range p.Lines {
		original[l.ID] = l
	}
	for _, l := range c.Lines {
		old, ok := original[l.ID]
		if !ok || l.Description != old.Description || l.Unit != old.Unit {
			return c, nil, nil, core.Invalid("lines", "差异提案须保留准确行身份、描述和单位；新增物品应另建采购")
		}
	}
	return c, values, amount, nil
}
func samePlan(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func varianceBasisMatches(b VarianceBasis, version int64, plan *string) error {
	if b.PurchaseVersion != version || !samePlan(b.ExecutionPlanRevisionID, plan) {
		return core.Fail(409, "PURCHASE_VARIANCE_BASIS_CHANGED", "采购执行依据已变化，请保留本次输入并核对当前基线")
	}
	return nil
}
func insertVarianceRevision(ctx context.Context, tx pgx.Tx, a core.Actor, v Variance, revision string, seq int64, base *string, c Content, values []LineValue, amount *string, reason string) error {
	raw, err := json.Marshal(map[string]any{"schema": 1, "kind": "purchase_variance", "document_id": v.ID, "project_id": v.ProjectID, "classification": v.Classification, "content": c, "basis": v.Basis, "line_values": values, "amount": amount})
	if err != nil {
		return err
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return err
	}
	basis, err := json.Marshal(v.Basis)
	if err != nil {
		return err
	}
	lineValues, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO purchase_variance_revisions(id,org_id,document_id,purchase_id,seq,base_revision_id,payload,basis,line_values,content_hash,amount,author_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, revision, a.OrgID, v.ID, v.Basis.PurchaseID, seq, base, payload, basis, lineValues, core.Digest(canonical), amount, a.UserID, reason)
	if err != nil {
		return err
	}
	ids := FileVersionIDs(c)
	for _, id := range FileVersionIDs(v.Basis.Content) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, "INSERT INTO purchase_variance_files(org_id,revision_id,version_id) VALUES($1,$2,$3)", a.OrgID, revision, id); err != nil {
			return err
		}
	}
	return nil
}
func varianceResult(v Variance, event string, status int, reason string) core.Result {
	out := result(v.Record, event, status, reason)
	out.Data = v
	out.Event.Payload = map[string]any{"revision_id": v.CurrentRevisionID, "purchase_id": v.Basis.PurchaseID, "purchase_version": v.Basis.PurchaseVersion, "reason": reason}
	return out
}
func (s *Service) replayVariance(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var v Variance
	if e = json.Unmarshal(raw, &v); e != nil {
		return core.Result{}, e
	}
	current, e := s.Variance(ctx, a, v.ID)
	if e != nil {
		return core.Result{}, e
	}
	v, e = presentVariance(ctx, s.DB, a, v)
	if e != nil {
		return core.Result{}, e
	}
	if v.Version != current.Version {
		v.CanEdit = false
		v.CanPrepareRequest = false
		v.CanActivate = false
	}
	out.Data = v
	return out, nil
}
func (s *Service) CreateVariance(ctx context.Context, c core.Command, purchase string, in VarianceCreate) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replayVariance(ctx, c.Actor, out, commandErr) }()
	if core.Name(in.Reason) != nil || core.RequireVersion(in.ExpectedPurchaseVersion) != nil {
		return out, core.Invalid("reason", "请固定当前采购版本并填写差异依据")
	}
	if in.ExecutionPlanRevisionID != nil && !core.ValidID(*in.ExecutionPlanRevisionID) {
		return out, core.Invalid("execution_plan_revision_id", "执行基线标识无效")
	}
	authorize := func(tx pgx.Tx) error {
		p, err := purchaseRead(ctx, tx, c.Actor, purchase)
		if err != nil {
			return err
		}
		d := p
		d.ID = ""
		d.Kind = "purchase_variance"
		d.CreatorID = c.Actor.UserID
		for _, action := range []string{"purchase_variance.read", "purchase_variance.create"} {
			if err = authz.Require(ctx, tx, c.Actor, action, resource(d)); err != nil {
				return err
			}
		}
		return nil
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		p, err := lockVariancePurchase(ctx, tx, c.Actor, purchase)
		if err != nil {
			return core.Result{}, err
		}
		basis, err := currentVarianceBasis(ctx, tx, c.Actor.OrgID, purchase)
		if err != nil {
			return core.Result{}, err
		}
		if err = varianceBasisMatches(basis, in.ExpectedPurchaseVersion, in.ExecutionPlanRevisionID); err != nil {
			return core.Result{}, err
		}
		v := Variance{Record: Record{ID: core.NewID(), Kind: "purchase_variance", CreatorID: c.Actor.UserID, ProjectID: p.ProjectID, Classification: p.Classification, CurrentRevisionID: core.NewID()}, Basis: basis}
		content, values, amount, err := normalizeVariance(ctx, tx, c.Actor, v.Record, p, in.Content)
		if err != nil {
			return core.Result{}, err
		}
		if err = tx.QueryRow(ctx, "SELECT 'CP-PV-'||nextval('spend_number_seq')::text").Scan(&v.DocumentNo); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO purchase_variances(id,org_id,purchase_id,document_no,project_id,creator_id,classification,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, c.Actor.OrgID, purchase, v.DocumentNo, v.ProjectID, v.CreatorID, v.Classification, v.CurrentRevisionID); err != nil {
			return core.Result{}, err
		}
		if err = insertVarianceRevision(ctx, tx, c.Actor, v, v.CurrentRevisionID, 1, nil, content, values, amount, in.Reason); err != nil {
			return core.Result{}, err
		}
		v, err = varianceMetadata(ctx, tx, c.Actor.OrgID, v.ID)
		if err != nil {
			return core.Result{}, err
		}
		v, err = presentVariance(ctx, tx, c.Actor, v)
		return varianceResult(v, "purchase_variance.created", 201, in.Reason), err
	})
}
func (s *Service) mutateVariance(ctx context.Context, c core.Command, id, action string, version int64, fn func(pgx.Tx, Variance, Record) (core.Result, error)) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replayVariance(ctx, c.Actor, out, commandErr) }()
	if core.RequireVersion(version) != nil {
		return out, core.Conflict()
	}
	check := func(tx pgx.Tx) (Variance, error) {
		v, err := varianceMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return v, err
		}
		if _, err = purchaseRead(ctx, tx, c.Actor, v.Basis.PurchaseID); err != nil {
			return v, err
		}
		for _, a := range []string{"purchase_variance.read", action} {
			if err = authz.Require(ctx, tx, c.Actor, a, resource(v.Record)); err != nil {
				return v, err
			}
		}
		if action == "purchase_variance.activate" {
			if err = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
				return v, err
			}
		}
		return v, nil
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, err := check(tx); return err }, func(tx pgx.Tx) (core.Result, error) {
		v, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		p, err := lockVariancePurchase(ctx, tx, c.Actor, v.Basis.PurchaseID)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM purchase_variances WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		v, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if v.Version != version {
			return core.Result{}, core.Conflict()
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return core.Result{}, err
		}
		out, err := fn(tx, v, p)
		out.Event.BeforeDigest = core.Digest(raw)
		return out, err
	})
}
func (s *Service) UpdateVariance(ctx context.Context, c core.Command, id string, in VariancePatch) (core.Result, error) {
	if !core.ValidID(in.BaseRevisionID) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("base_revision_id", "请固定差异修订并填写修改依据")
	}
	return s.mutateVariance(ctx, c, id, "purchase_variance.update", in.ExpectedVersion, func(tx pgx.Tx, v Variance, p Record) (core.Result, error) {
		if !editable(v.Record) || v.CurrentRevisionID != strings.ToLower(in.BaseRevisionID) {
			return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_READ_ONLY", "当前差异提案不能修改，请核对阶段和版本")
		}
		basis, err := currentVarianceBasis(ctx, tx, c.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		if err = varianceBasisMatches(basis, in.ExpectedPurchaseVersion, in.ExecutionPlanRevisionID); err != nil {
			return core.Result{}, err
		}
		v.Basis = basis
		content, values, amount, err := normalizeVariance(ctx, tx, c.Actor, v.Record, p, in.Content)
		if err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM purchase_variance_revisions WHERE org_id=$1 AND id=$2", c.Actor.OrgID, v.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if err = insertVarianceRevision(ctx, tx, c.Actor, v, revision, seq+1, &v.CurrentRevisionID, content, values, amount, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE purchase_variances SET current_revision_id=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision); err != nil {
			return core.Result{}, err
		}
		v, err = varianceMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		v, err = presentVariance(ctx, tx, c.Actor, v)
		return varianceResult(v, "purchase_variance.revised", 200, in.Reason), err
	})
}

// Workflow prepares a fixed proposal, while the actual purchase remains usable
// under its previous approved plan until a separate activation succeeds.
func VarianceApprovalDraft(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock, complete bool) (Variance, error) {
	v, err := varianceMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return v, err
	}
	for _, action := range []string{"purchase_variance.read", "purchase_variance.update"} {
		if err = authz.Require(ctx, db, a, action, resource(v.Record)); err != nil {
			return Variance{}, err
		}
	}
	var p Record
	if lock {
		p, err = lockVariancePurchase(ctx, db, a, v.Basis.PurchaseID)
		if err != nil {
			return v, err
		}
		if _, err = db.Exec(ctx, "SELECT id FROM purchase_variances WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
			return v, err
		}
		v, err = varianceMetadata(ctx, db, a.OrgID, id)
	} else {
		p, err = purchaseRead(ctx, db, a, v.Basis.PurchaseID)
	}
	if err != nil {
		return v, err
	}
	if p.Lifecycle == "completed" || p.Lifecycle == "cancelled" || v.Lifecycle != "draft" || !slices.Contains([]string{"not_submitted", "returned", "withdrawn", "rejected"}, v.ApprovalState) {
		return Variance{}, core.Fail(409, "PURCHASE_VARIANCE_READ_ONLY", "当前差异阶段不能准备送审内容")
	}
	if complete {
		if p.Version != v.Basis.PurchaseVersion {
			return v, core.Fail(409, "PURCHASE_VARIANCE_BASIS_CHANGED", "采购执行已变化，请更新差异提案并重新固定审批依据")
		}
		n := v.Record
		n.Kind = "purchase"
		if _, _, _, _, err = normalize(ctx, db, a, n, v.Content, &p.Content, true); err != nil {
			return v, err
		}
	}
	v, err = presentVariance(ctx, db, a, v)
	if err != nil {
		return v, err
	}
	if complete {
		v.Suppliers, v.Attachments, err = varianceEvidence(ctx, db, a, v, true)
	}
	return v, err
}

func (s *Service) ActivateVariance(ctx context.Context, c core.Command, id string, in VarianceActivation) (core.Result, error) {
	if err := executionBase(&in.ExecutionBase); err != nil {
		return core.Result{}, err
	}
	return s.mutateVariance(ctx, c, id, "purchase_variance.activate", in.ExpectedVersion, func(tx pgx.Tx, v Variance, p Record) (core.Result, error) {
		if v.ApprovalState != "approved" || v.Lifecycle != "requested" || v.CurrentRevisionID != in.RevisionID || v.ContentHash != in.ContentHash {
			return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_NOT_APPROVED", "请固定准确已批准且尚未启用的差异修订")
		}
		basis, err := currentVarianceBasis(ctx, tx, c.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		if in.ExpectedPurchaseVersion != p.Version || v.Basis.PurchaseVersion != p.Version {
			return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_BASIS_CHANGED", "采购执行已变化，请保留输入并重新核对、送审差异")
		}
		before, _ := json.Marshal(v.Basis)
		current, _ := json.Marshal(basis)
		if string(before) != string(current) {
			return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_BASIS_CHANGED", "采购履行集合已变化，请重新固定差异审批")
		}
		if _, _, err = varianceEvidence(ctx, tx, c.Actor, v, true); err != nil {
			return core.Result{}, err
		}
		rows, err := tx.Query(ctx, `SELECT d.id FROM actual_cost_entries d JOIN cost_links l ON l.org_id=d.org_id AND l.cost_id=d.id WHERE l.org_id=$1 AND l.source_type='purchase' AND l.source_id=$2 ORDER BY d.id FOR UPDATE OF d`, c.Actor.OrgID, p.ID)
		if err != nil {
			return core.Result{}, err
		}
		costIDs := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return core.Result{}, err
			}
			costIDs = append(costIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return core.Result{}, err
		}
		for _, id := range costIDs {
			var readable bool
			if err = tx.QueryRow(ctx, `SELECT can_read_cost(org_id,$2,id,current_revision_id,$3,$4) FROM actual_cost_entries WHERE org_id=$1 AND id=$5`, c.Actor.OrgID, c.Actor.UserID, c.Actor.Channel, c.Actor.RunID != "", id).Scan(&readable); err != nil {
				return core.Result{}, err
			}
			if !readable {
				return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_COST_SCOPE_UNAVAILABLE", "当前无法完整核对采购已关联费用，请由具有相应权限的人员处理")
			}
		}
		var spent string
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(r.amount),0)::text FROM actual_cost_entries d JOIN cost_revisions r ON r.org_id=d.org_id AND r.id=d.current_revision_id JOIN cost_links l ON l.org_id=d.org_id AND l.cost_id=d.id WHERE l.org_id=$1 AND l.source_type='purchase' AND l.source_id=$2 AND d.state='confirmed' AND d.entry_kind='expense'`, c.Actor.OrgID, p.ID).Scan(&spent); err != nil {
			return core.Result{}, err
		}
		used, _ := core.ParseDecimal(spent)
		if used.Cmp(decimal(v.Amount)) > 0 {
			return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_BELOW_ACTUAL_COSTS", "拟执行额度低于现有有效实际费用，请重新核对差异")
		}
		lineMap := map[string]Line{}
		amountMap := map[string]*string{}
		for _, l := range v.Lines {
			lineMap[l.ID] = l
		}
		for _, l := range v.LineValues {
			amountMap[l.LineID] = l.Amount
		}
		for _, l := range basis.Commitments {
			proposed := lineMap[l.LineID]
			q, _ := core.ParseDecimal(l.ReservedQuantity)
			amount, _ := core.ParseDecimal(l.ReservedAmount)
			tax, _ := core.ParseDecimal(l.ReservedTax)
			discount, _ := core.ParseDecimal(l.ReservedDiscount)
			if q.Cmp(decimal(proposed.Quantity)) > 0 || amount.Cmp(decimal(amountMap[l.LineID])) > 0 || tax.Cmp(decimal(proposed.TaxAmount)) > 0 || discount.Cmp(decimal(proposed.DiscountAmount)) > 0 {
				return core.Result{}, core.Fail(409, "PURCHASE_VARIANCE_BELOW_COMMITMENTS", "拟执行基线低于现有有效订单承诺，请重新核对数量、金额、税额和折扣")
			}
		}
		var requestRevision string
		if err = tx.QueryRow(ctx, "SELECT current_revision_id FROM requests WHERE org_id=$1 AND id=$2 AND state='approved'", c.Actor.OrgID, v.RequestID).Scan(&requestRevision); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO purchase_plan_activations(id,org_id,purchase_id,purchase_version,variance_id,variance_revision_id,request_revision_id,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,$6,$7,$8,s.id,s.mfa_verified_at,$9 FROM sessions s WHERE s.org_id=$2 AND s.id=$10`, core.NewID(), c.Actor.OrgID, p.ID, p.Version+1, v.ID, v.CurrentRevisionID, requestRevision, c.Actor.UserID, in.Reason, c.Actor.SessionID); err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE(max(seq),0)+1 FROM purchase_execution_events WHERE org_id=$1 AND purchase_id=$2", c.Actor.OrgID, p.ID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO purchase_execution_events(id,org_id,purchase_id,seq,purchase_version,kind,variance_revision_id,lifecycle,actor_id,session_id,mfa_verified_at,reason)
            SELECT $1,$2,$3,$4,$5,'plan_activated',$6,$7,$8,s.id,s.mfa_verified_at,$9 FROM sessions s WHERE s.org_id=$2 AND s.id=$10`, core.NewID(), c.Actor.OrgID, p.ID, seq, p.Version+1, v.CurrentRevisionID, p.Lifecycle, c.Actor.UserID, in.Reason, c.Actor.SessionID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE spend_documents SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, p.ID); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE purchase_variances SET lifecycle='effective',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		v, err = varianceMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		v, err = presentVariance(ctx, tx, c.Actor, v)
		return varianceResult(v, "purchase_variance.activated", 200, in.Reason), err
	})
}
