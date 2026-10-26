package spend

import (
	"context"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func executionBase(in *ExecutionBase) error {
	in.RevisionID = strings.ToLower(in.RevisionID)
	in.Reason = strings.TrimSpace(in.Reason)
	if !core.ValidID(in.RevisionID) || len(in.ContentHash) != 64 || strings.Trim(in.ContentHash, "0123456789abcdef") != "" || core.Name(in.Reason) != nil {
		return core.Invalid("revision_id", "请固定准确批准修订与摘要，并说明登记依据")
	}
	return core.RequireVersion(in.ExpectedVersion)
}
func approvedPurchase(d Record, in ExecutionBase, allowCancelled bool) error {
	if d.DeletedAt != nil || d.ApprovalState != "approved" || d.ApprovedRevisionID == nil || d.RequestID == nil || d.Currency == nil || d.Amount == nil ||
		*d.ApprovedRevisionID != in.RevisionID || d.CurrentRevisionID != in.RevisionID || d.ContentHash != in.ContentHash {
		return core.Fail(409, "PURCHASE_APPROVED_REVISION_REQUIRED", "请核对已批准采购的准确修订")
	}
	if d.Lifecycle == "cancelled" && !allowCancelled {
		return core.Fail(409, "PURCHASE_CANCELLED", "已取消采购只允许登记既有验收的退货")
	}
	if d.Lifecycle == "completed" && !allowCancelled {
		return core.Fail(409, "PURCHASE_COMPLETED", "已完成采购只允许登记既有验收的来源退货")
	}
	return nil
}
func evidenceIDs(ids []string) ([]string, error) {
	if len(ids) < 1 || len(ids) > 20 {
		return nil, core.Invalid("evidence_version_ids", "须选择1至20个准确依据文件版本")
	}
	result := append([]string{}, ids...)
	for i, v := range result {
		result[i] = strings.ToLower(v)
		if !core.ValidID(result[i]) {
			return nil, core.Invalid("evidence_version_ids", "文件版本标识无效")
		}
	}
	slices.Sort(result)
	for i := 1; i < len(result); i++ {
		if result[i] == result[i-1] {
			return nil, core.Invalid("evidence_version_ids", "文件版本不能重复")
		}
	}
	return result, nil
}
func todayHK(ctx context.Context, db platform.DBTX) (string, error) {
	var v string
	err := db.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&v)
	return v, err
}
func decimal(v *string) core.Decimal {
	if v == nil {
		return core.Decimal{}
	}
	d, _ := core.ParseDecimal(*v)
	return d
}

func finishExecution(ctx context.Context, tx pgx.Tx, a core.Actor, d Record, kind string, order, receipt *string, reason string, status int) (core.Result, error) {
	lifecycle := "ordering"
	if d.Lifecycle == "receiving" || kind == "receipt_registered" {
		lifecycle = "receiving"
	}
	if d.Lifecycle == "cancelled" || kind == "purchase_cancelled" {
		lifecycle = "cancelled"
	}
	if d.Lifecycle == "completed" {
		lifecycle = "completed"
	}
	var seq int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(max(seq),0)+1 FROM purchase_execution_events WHERE org_id=$1 AND purchase_id=$2", a.OrgID, d.ID).Scan(&seq); err != nil {
		return core.Result{}, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO purchase_execution_events(id,org_id,purchase_id,seq,purchase_version,kind,order_id,receipt_id,lifecycle,actor_id,session_id,mfa_verified_at,reason)
        SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,s.id,s.mfa_verified_at,$11 FROM sessions s WHERE s.org_id=$2 AND s.id=$12`, core.NewID(), a.OrgID, d.ID, seq, d.Version+1, kind, order, receipt, lifecycle, a.UserID, reason, a.SessionID)
	if err != nil {
		return core.Result{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE spend_documents SET lifecycle=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", a.OrgID, d.ID, lifecycle); err != nil {
		return core.Result{}, err
	}
	d, err = metadata(ctx, tx, a.OrgID, "purchase", d.ID)
	if err != nil {
		return core.Result{}, err
	}
	d, err = present(ctx, tx, a, d)
	if err != nil {
		return core.Result{}, err
	}
	out := result(d, "purchase."+strings.ReplaceAll(kind, "_", "."), status, reason)
	out.Event.Payload = map[string]any{"revision_id": d.CurrentRevisionID, "order_id": order, "receipt_id": receipt, "reason": reason}
	return out, nil
}

func (s *Service) RegisterOrder(ctx context.Context, c core.Command, id string, in OrderInput) (core.Result, error) {
	if err := executionBase(&in.ExecutionBase); err != nil {
		return core.Result{}, err
	}
	in.Supplier.PartyID = strings.ToLower(in.Supplier.PartyID)
	in.Supplier.RevisionID = strings.ToLower(in.Supplier.RevisionID)
	in.ExternalRef = strings.TrimSpace(in.ExternalRef)
	if !core.ValidID(in.Supplier.PartyID) || !core.ValidID(in.Supplier.RevisionID) || core.Name(in.ExternalRef) != nil || core.Date(in.OrderedOn) != nil || core.Date(in.ExpectedDeliveryOn) != nil || in.ExpectedDeliveryOn < in.OrderedOn || len([]rune(in.Notes)) > 4000 || len(in.Lines) < 1 || len(in.Lines) > 100 {
		return core.Result{}, core.Invalid("order", "请填写真实订单编号、批准供应商、日期及1至100行订购明细")
	}
	ids, err := evidenceIDs(in.EvidenceVersionIDs)
	if err != nil {
		return core.Result{}, err
	}
	in.EvidenceVersionIDs = ids
	return s.mutate(ctx, c, "purchase", id, "purchase.execute", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if err := approvedPurchase(d, in.ExecutionBase, false); err != nil {
			return core.Result{}, err
		}
		plan, err := effectivePlan(ctx, tx, c.Actor.OrgID, d)
		if err != nil {
			return core.Result{}, err
		}
		if !samePlan(plan.VarianceRevisionID, in.ExecutionPlanRevisionID) {
			return core.Result{}, core.Fail(409, "PURCHASE_EXECUTION_PLAN_CHANGED", "当前执行基线已变化，请保留订单输入并核对")
		}
		if !slices.Contains(plan.Content.SupplierCandidates, in.Supplier) {
			return core.Result{}, core.Fail(409, "PURCHASE_SUPPLIER_OUTSIDE_APPROVAL", "供应商及其资料修订须属于准确批准候选")
		}
		today, err := todayHK(ctx, tx)
		if err != nil {
			return core.Result{}, err
		}
		if in.OrderedOn > today {
			return core.Result{}, core.Invalid("ordered_on", "订购日期不能晚于香港当前日期")
		}
		if _, _, err = References(ctx, tx, c.Actor, d, Content{SupplierCandidates: []Supplier{in.Supplier}, QuoteVersionIDs: plan.Content.QuoteVersionIDs}, true, true); err != nil {
			return core.Result{}, err
		}
		if _, err = executionFiles(ctx, tx, c.Actor, d, in.EvidenceVersionIDs, true); err != nil {
			return core.Result{}, err
		}
		var requestRevision string
		if err = tx.QueryRow(ctx, `SELECT q.id FROM requests request JOIN request_revisions q ON q.org_id=request.org_id AND q.id=request.current_revision_id
            JOIN approval_instances i ON i.org_id=request.org_id AND i.id=request.active_instance_id
            WHERE request.org_id=$1 AND request.id=$2 AND request.kind='purchase' AND request.business_id=$3 AND request.state='approved' AND i.state='approved'
            AND q.business_revision_id=$4 AND q.business_content_hash=$5`, c.Actor.OrgID, *d.RequestID, d.ID, in.RevisionID, in.ContentHash).Scan(&requestRevision); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		var exponent int
		if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", *d.Currency).Scan(&exponent); err != nil {
			return core.Result{}, err
		}
		approved := map[string]Line{}
		for _, l := range plan.Content.Lines {
			approved[l.ID] = l
		}
		used := map[string]bool{}
		lines := []OrderLine{}
		content := plan.Content
		content.Lines = []Line{}
		for _, l := range in.Lines {
			l.PurchaseLineID = strings.ToLower(l.PurchaseLineID)
			b, ok := approved[l.PurchaseLineID]
			if !ok || used[l.PurchaseLineID] {
				return core.Result{}, core.Invalid("purchase_line_id", "订购明细须逐行引用批准采购，不能重复")
			}
			used[l.PurchaseLineID] = true
			q, e := core.ParseDecimal(l.Quantity)
			if e != nil || !q.Positive() {
				return core.Result{}, core.Invalid("quantity", "订购数量须为正数，最多6位小数")
			}
			p, e := core.ParseDecimal(l.UnitPrice)
			if e != nil {
				return core.Result{}, e
			}
			if p.Cmp(decimal(b.UnitPrice)) != 0 {
				return core.Result{}, core.Fail(409, "PURCHASE_PRICE_VARIANCE_REQUIRES_APPROVAL", "实际单价与批准依据不同，需要独立差异审批")
			}
			tax, e := core.CheckCurrencyAmount(*d.Currency, l.TaxAmount, map[string]int{*d.Currency: exponent})
			if e != nil {
				return core.Result{}, e
			}
			discount, e := core.CheckCurrencyAmount(*d.Currency, l.DiscountAmount, map[string]int{*d.Currency: exponent})
			if e != nil {
				return core.Result{}, e
			}
			l.Quantity = q.String()
			l.UnitPrice = p.String()
			l.TaxAmount = tax.String()
			l.DiscountAmount = discount.String()
			b.Quantity = &l.Quantity
			b.UnitPrice = &l.UnitPrice
			b.TaxAmount = &l.TaxAmount
			b.DiscountAmount = &l.DiscountAmount
			content.Lines = append(content.Lines, b)
			lines = append(lines, OrderLine{ID: core.NewID(), OrderLineInput: l, Description: b.Description, Unit: b.Unit})
		}
		values, total, _, err := calculate("purchase", content, exponent)
		if err != nil {
			return core.Result{}, err
		}
		for i := range lines {
			lines[i].Amount = *values[i].Amount
			b := approved[lines[i].PurchaseLineID]
			var quantity, amount, tax, discount string
			err = tx.QueryRow(ctx, `SELECT COALESCE(max(quantity),0)::numeric(20,6)::text,COALESCE(max(amount),0)::numeric(20,6)::text,COALESCE(max(tax),0)::numeric(20,6)::text,COALESCE(max(discount),0)::numeric(20,6)::text
                FROM purchase_commitments($1,$2) WHERE line_id=$3`, c.Actor.OrgID, d.ID, b.ID).Scan(&quantity, &amount, &tax, &discount)
			if err != nil {
				return core.Result{}, err
			}
			var approvedAmount string
			for _, v := range plan.LineValues {
				if v.LineID == b.ID && v.Amount != nil {
					approvedAmount = *v.Amount
				}
			}
			for _, check := range [][3]string{{quantity, lines[i].Quantity, *b.Quantity}, {amount, lines[i].Amount, approvedAmount}, {tax, lines[i].TaxAmount, valueOrZero(b.TaxAmount)}, {discount, lines[i].DiscountAmount, valueOrZero(b.DiscountAmount)}} {
				a, _ := core.ParseDecimal(check[0])
				v, _ := core.ParseDecimal(check[1])
				cap, _ := core.ParseDecimal(check[2])
				if a.Add(v).Cmp(cap) > 0 {
					return core.Result{}, core.Fail(409, "PURCHASE_APPROVED_SCOPE_EXCEEDED", "累计订购数量、金额、税额或折扣超过批准依据，须先处理差异审批")
				}
			}
		}
		order := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO purchase_orders(id,org_id,purchase_id,revision_id,purchase_version,request_revision_id,supplier_id,supplier_revision_id,external_ref,external_ref_key,ordered_on,expected_delivery_on,currency,amount,line_count,evidence_version_ids,notes,execution_plan_revision_id)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,lower($9),$10,$11,$12,$13,$14,$15,$16,$17)`, order, c.Actor.OrgID, d.ID, d.CurrentRevisionID, d.Version+1, requestRevision, in.Supplier.PartyID, in.Supplier.RevisionID, in.ExternalRef, in.OrderedOn, in.ExpectedDeliveryOn, *d.Currency, *total, len(lines), in.EvidenceVersionIDs, in.Notes, plan.VarianceRevisionID)
		if err != nil {
			return core.Result{}, err
		}
		for i, l := range lines {
			_, err = tx.Exec(ctx, `INSERT INTO purchase_order_lines(id,org_id,purchase_id,order_id,revision_id,position,purchase_line_id,description,unit,quantity,unit_price,tax_amount,discount_amount,amount)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, l.ID, c.Actor.OrgID, d.ID, order, d.CurrentRevisionID, i+1, l.PurchaseLineID, l.Description, l.Unit, l.Quantity, l.UnitPrice, l.TaxAmount, l.DiscountAmount, l.Amount)
			if err != nil {
				return core.Result{}, err
			}
		}
		return finishExecution(ctx, tx, c.Actor, d, "order_registered", &order, nil, in.Reason, 201)
	})
}
func valueOrZero(v *string) string {
	if v == nil {
		return "0"
	}
	return *v
}

func (s *Service) RegisterReceipt(ctx context.Context, c core.Command, id string, in ReceiptInput) (core.Result, error) {
	if err := executionBase(&in.ExecutionBase); err != nil {
		return core.Result{}, err
	}
	in.OrderID = strings.ToLower(in.OrderID)
	if !core.ValidID(in.OrderID) || core.Date(in.ReceivedOn) != nil || len([]rune(in.Notes)) > 4000 || len(in.Lines) < 1 || len(in.Lines) > 100 {
		return core.Result{}, core.Invalid("receipt", "请填写准确订单、实际日期及1至100行验收或退货明细")
	}
	ids, err := evidenceIDs(in.EvidenceVersionIDs)
	if err != nil {
		return core.Result{}, err
	}
	in.EvidenceVersionIDs = ids
	return s.mutate(ctx, c, "purchase", id, "purchase.execute", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if err := approvedPurchase(d, in.ExecutionBase, true); err != nil {
			return core.Result{}, err
		}
		o, err := orderMetadata(ctx, tx, c.Actor.OrgID, d.ID, in.OrderID)
		if err != nil {
			return core.Result{}, err
		}
		today, err := todayHK(ctx, tx)
		if err != nil {
			return core.Result{}, err
		}
		if in.ReceivedOn > today || in.ReceivedOn < o.OrderedOn {
			return core.Result{}, core.Invalid("received_on", "实际日期须在订单日期与香港当前日期之间")
		}
		if _, err = executionFiles(ctx, tx, c.Actor, d, in.EvidenceVersionIDs, true); err != nil {
			return core.Result{}, err
		}
		lines := map[string]OrderLine{}
		for _, l := range o.Lines {
			lines[l.ID] = l
		}
		used := map[string]bool{}
		arrivals := map[string]core.Decimal{}
		hasArrival, hasReturn := false, false
		for i := range in.Lines {
			l := &in.Lines[i]
			l.OrderLineID = strings.ToLower(l.OrderLineID)
			l.Reason = strings.TrimSpace(l.Reason)
			base, ok := lines[l.OrderLineID]
			if !ok {
				return core.Result{}, core.Invalid("order_line_id", "验收明细须属于当前订单")
			}
			q, e := core.ParseDecimal(l.Quantity)
			if e != nil || !q.Positive() {
				return core.Result{}, core.Invalid("quantity", "验收或退货数量须为正数，最多6位小数")
			}
			l.Quantity = q.String()
			if !slices.Contains([]string{"accepted", "rejected", "returned"}, l.Result) || len([]rune(l.Reason)) < 1 || len([]rune(l.Reason)) > 2000 {
				return core.Result{}, core.Invalid("result", "请明确每行验收结果和依据")
			}
			key := l.OrderLineID + "/" + l.Result
			if l.Result == "returned" {
				hasReturn = true
				if l.ReversesLineID == nil || !core.ValidID(*l.ReversesLineID) {
					return core.Result{}, core.Invalid("reverses_line_id", "退货必须引用一条既有通过验收明细")
				}
				v := strings.ToLower(*l.ReversesLineID)
				l.ReversesLineID = &v
				key += "/" + v
				var remaining, acceptedOn string
				err = tx.QueryRow(ctx, `SELECT (l.quantity-COALESCE((SELECT sum(quantity) FROM purchase_receipt_lines x WHERE x.org_id=l.org_id AND x.reverses_line_id=l.id),0))::numeric(20,6)::text,r.received_on::text
                    FROM purchase_receipt_lines l JOIN purchase_receipts r ON r.org_id=l.org_id AND r.id=l.receipt_id
                    WHERE l.org_id=$1 AND l.purchase_id=$2 AND l.order_id=$3 AND l.order_line_id=$4 AND l.id=$5 AND l.result='accepted'`, c.Actor.OrgID, d.ID, o.ID, l.OrderLineID, v).Scan(&remaining, &acceptedOn)
				if err != nil {
					return core.Result{}, platform.DatabaseError(err)
				}
				cap, _ := core.ParseDecimal(remaining)
				if q.Cmp(cap) > 0 {
					return core.Result{}, core.Fail(409, "PURCHASE_RETURN_EXCEEDS_SOURCE", "退货超过所引用验收明细的当前可退数量")
				}
				if in.ReceivedOn < acceptedOn {
					return core.Result{}, core.Invalid("received_on", "退货日期不能早于所引用的通过验收日期")
				}
			} else {
				hasArrival = true
				if l.ReversesLineID != nil {
					return core.Result{}, core.Invalid("reverses_line_id", "仅退货明细可以引用原验收")
				}
				if o.State == "cancelled" || d.Lifecycle == "cancelled" || d.Lifecycle == "completed" {
					return core.Result{}, core.Fail(409, "PURCHASE_ORDER_CANCELLED", "已取消或完成的承诺只允许登记既有验收的来源退货")
				}
				arrivals[l.OrderLineID] = arrivals[l.OrderLineID].Add(q)
				cap, _ := core.ParseDecimal(base.OutstandingQuantity)
				if arrivals[l.OrderLineID].Cmp(cap) > 0 {
					return core.Result{}, core.Fail(409, "PURCHASE_RECEIPT_EXCEEDS_ORDER", "本批通过与拒收数量合计超过订单待收数量")
				}
			}
			if used[key] {
				return core.Result{}, core.Invalid("lines", "同批相同订单行、结果与退货来源不能重复")
			}
			used[key] = true
		}
		if hasArrival && hasReturn {
			return core.Result{}, core.Invalid("lines", "退货与新的收货验收须分别登记，保留各自依据")
		}
		receipt := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO purchase_receipts(id,org_id,purchase_id,order_id,purchase_version,received_on,line_count,evidence_version_ids,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, receipt, c.Actor.OrgID, d.ID, o.ID, d.Version+1, in.ReceivedOn, len(in.Lines), in.EvidenceVersionIDs, in.Notes)
		if err != nil {
			return core.Result{}, err
		}
		for i, l := range in.Lines {
			_, err = tx.Exec(ctx, `INSERT INTO purchase_receipt_lines(id,org_id,purchase_id,order_id,receipt_id,position,order_line_id,quantity,result,reverses_line_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, core.NewID(), c.Actor.OrgID, d.ID, o.ID, receipt, i+1, l.OrderLineID, l.Quantity, l.Result, l.ReversesLineID, l.Reason)
			if err != nil {
				return core.Result{}, err
			}
		}
		return finishExecution(ctx, tx, c.Actor, d, "receipt_registered", &o.ID, &receipt, in.Reason, 201)
	})
}

func (s *Service) CancelOrder(ctx context.Context, c core.Command, id, order string, in ExecutionBase) (core.Result, error) {
	if err := executionBase(&in); err != nil {
		return core.Result{}, err
	}
	return s.mutate(ctx, c, "purchase", id, "purchase.execute", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if err := approvedPurchase(d, in, false); err != nil {
			return core.Result{}, err
		}
		o, err := orderMetadata(ctx, tx, c.Actor.OrgID, d.ID, order)
		if err != nil {
			return core.Result{}, err
		}
		if o.State != "open" {
			return core.Result{}, core.Fail(409, "PURCHASE_ORDER_NOT_OPEN", "仅可取消仍有待收数量的订单余量")
		}
		return finishExecution(ctx, tx, c.Actor, d, "order_cancelled", &o.ID, nil, in.Reason, 200)
	})
}
func (s *Service) CancelPurchase(ctx context.Context, c core.Command, id string, in ExecutionBase) (core.Result, error) {
	if err := executionBase(&in); err != nil {
		return core.Result{}, err
	}
	return s.mutate(ctx, c, "purchase", id, "purchase.execute", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if err := approvedPurchase(d, in, false); err != nil {
			return core.Result{}, err
		}
		var open bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_order_lines l WHERE l.org_id=$1 AND l.purchase_id=$2 AND l.quantity>purchase_net_accepted(l.org_id,l.id)
            AND NOT EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=l.org_id AND x.order_id=l.order_id AND x.kind='order_cancelled'))`, c.Actor.OrgID, d.ID).Scan(&open)
		if err != nil {
			return core.Result{}, err
		}
		if open {
			return core.Result{}, core.Fail(409, "PURCHASE_OPEN_ORDERS", "请先明确取消各订单未交付的余量")
		}
		return finishExecution(ctx, tx, c.Actor, d, "purchase_cancelled", nil, nil, in.Reason, 200)
	})
}
