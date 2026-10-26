package spend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
)

func purchaseRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Record, error) {
	d, err := metadata(ctx, db, a.OrgID, "purchase", id)
	if err != nil {
		return d, err
	}
	return d, authz.Require(ctx, db, a, "purchase.read", resource(d))
}
func executionFiles(ctx context.Context, db platform.DBTX, a core.Actor, d Record, ids []string, strict bool) ([]fileref.Evidence, error) {
	items := []fileref.Evidence{}
	for _, id := range ids {
		var v fileref.Evidence
		var err error
		if strict {
			v, err = fileref.ForApproval(ctx, db, a, id, d.ProjectID, d.Classification)
		} else {
			v, err = fileref.Resolve(ctx, db, a, id, d.ProjectID, d.Classification)
		}
		if err != nil {
			if !strict && hidden(err) {
				continue
			}
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}
func orderMetadata(ctx context.Context, db platform.DBTX, org, purchase, id string) (Order, error) {
	o := Order{Lines: []OrderLine{}, Attachments: []fileref.Evidence{}}
	if !core.ValidID(id) {
		return o, core.Missing()
	}
	err := db.QueryRow(ctx, `SELECT o.id,o.purchase_id,o.revision_id,o.purchase_version,o.request_revision_id,o.execution_plan_revision_id,o.supplier_id,o.supplier_revision_id,
        o.external_ref,o.ordered_on::text,o.expected_delivery_on::text,o.currency,o.amount::text,o.evidence_version_ids,o.notes,o.created_at,
        e.actor_id,e.reason,CASE WHEN EXISTS(SELECT 1 FROM purchase_execution_events x WHERE x.org_id=o.org_id AND (x.order_id=o.id AND x.kind='order_cancelled' OR x.purchase_id=o.purchase_id AND x.kind='purchase_cancelled')) THEN 'cancelled'
            WHEN EXISTS(SELECT 1 FROM purchase_order_lines l WHERE l.org_id=o.org_id AND l.order_id=o.id AND purchase_net_accepted(o.org_id,l.id)<l.quantity) THEN 'open' ELSE 'fulfilled' END
        FROM purchase_orders o JOIN purchase_execution_events e ON e.org_id=o.org_id AND e.purchase_id=o.purchase_id AND e.purchase_version=o.purchase_version
        WHERE o.org_id=$1 AND o.purchase_id=$2 AND o.id=$3`, org, purchase, id).Scan(&o.ID, &o.PurchaseID, &o.RevisionID, &o.PurchaseVersion, &o.RequestRevisionID, &o.ExecutionPlanRevisionID, &o.Supplier.PartyID, &o.Supplier.RevisionID,
		&o.ExternalRef, &o.OrderedOn, &o.ExpectedDeliveryOn, &o.Currency, &o.Amount, &o.EvidenceVersionIDs, &o.Notes, &o.CreatedAt, &o.ActorID, &o.Reason, &o.State)
	if err != nil {
		return o, platform.DatabaseError(err)
	}
	rows, err := db.Query(ctx, `SELECT id,purchase_line_id,description,unit,quantity::text,unit_price::text,tax_amount::text,discount_amount::text,amount::text,
        purchase_net_accepted(org_id,id)::numeric(20,6)::text,(quantity-purchase_net_accepted(org_id,id))::numeric(20,6)::text
        FROM purchase_order_lines WHERE org_id=$1 AND order_id=$2 ORDER BY position`, org, id)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var l OrderLine
		if err = rows.Scan(&l.ID, &l.PurchaseLineID, &l.Description, &l.Unit, &l.Quantity, &l.UnitPrice, &l.TaxAmount, &l.DiscountAmount, &l.Amount, &l.NetAcceptedQuantity, &l.OutstandingQuantity); err != nil {
			return o, err
		}
		o.Lines = append(o.Lines, l)
	}
	return o, rows.Err()
}
func (s *Service) Order(ctx context.Context, a core.Actor, purchase, id string) (Order, error) {
	d, err := purchaseRead(ctx, s.DB, a, purchase)
	if err != nil {
		return Order{}, err
	}
	o, err := orderMetadata(ctx, s.DB, a.OrgID, purchase, id)
	if err != nil {
		return o, err
	}
	suppliers, _, err := References(ctx, s.DB, a, d, Content{SupplierCandidates: []Supplier{o.Supplier}}, false, false)
	if err != nil {
		return Order{}, err
	}
	if len(suppliers) == 1 {
		o.SupplierEvidence = &suppliers[0]
	}
	o.Attachments, err = executionFiles(ctx, s.DB, a, d, o.EvidenceVersionIDs, false)
	return o, err
}
func receiptMetadata(ctx context.Context, db platform.DBTX, org, purchase, id string) (Receipt, error) {
	v := Receipt{Lines: []ReceiptLine{}, Attachments: []fileref.Evidence{}}
	if !core.ValidID(id) {
		return v, core.Missing()
	}
	err := db.QueryRow(ctx, `SELECT r.id,r.purchase_id,r.order_id,r.purchase_version,r.received_on::text,r.evidence_version_ids,r.notes,r.created_at,e.actor_id,e.reason
        FROM purchase_receipts r JOIN purchase_execution_events e ON e.org_id=r.org_id AND e.purchase_id=r.purchase_id AND e.purchase_version=r.purchase_version
        WHERE r.org_id=$1 AND r.purchase_id=$2 AND r.id=$3`, org, purchase, id).Scan(&v.ID, &v.PurchaseID, &v.OrderID, &v.PurchaseVersion, &v.ReceivedOn, &v.EvidenceVersionIDs, &v.Notes, &v.CreatedAt, &v.ActorID, &v.Reason)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	rows, err := db.Query(ctx, `SELECT id,order_line_id,quantity::text,result,reverses_line_id,reason,
        (CASE WHEN result='accepted' THEN quantity-COALESCE((SELECT sum(quantity) FROM purchase_receipt_lines x WHERE x.org_id=l.org_id AND x.reverses_line_id=l.id),0) ELSE 0 END)::numeric(20,6)::text
        FROM purchase_receipt_lines l WHERE org_id=$1 AND receipt_id=$2 ORDER BY position`, org, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var l ReceiptLine
		if err = rows.Scan(&l.ID, &l.OrderLineID, &l.Quantity, &l.Result, &l.ReversesLineID, &l.Reason, &l.ReturnableQuantity); err != nil {
			return v, err
		}
		v.Lines = append(v.Lines, l)
	}
	return v, rows.Err()
}
func (s *Service) Receipt(ctx context.Context, a core.Actor, purchase, id string) (Receipt, error) {
	d, err := purchaseRead(ctx, s.DB, a, purchase)
	if err != nil {
		return Receipt{}, err
	}
	v, err := receiptMetadata(ctx, s.DB, a.OrgID, purchase, id)
	if err != nil {
		return v, err
	}
	v.Attachments, err = executionFiles(ctx, s.DB, a, d, v.EvidenceVersionIDs, false)
	return v, err
}

// Fetch only a bounded page of immutable identifiers before resolving today's
// independently authorized supplier/file evidence. No rows remain open then.
func executionPage[T any](ctx context.Context, s *Service, a core.Actor, purchase, kind, order, cursor string, limit int, load func(string) (T, error)) (core.Page[T], error) {
	out := core.Page[T]{Items: []T{}}
	if _, err := purchaseRead(ctx, s.DB, a, purchase); err != nil {
		return out, err
	}
	if order != "" && !core.ValidID(order) {
		return out, core.Invalid("order_id", "订单筛选无效")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := binding(a, fmt.Sprint("execution/", purchase, "/", kind, "/", order))
	var at *time.Time
	var before *string
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	table := "purchase_orders"
	if kind == "receipts" {
		table = "purchase_receipts"
	} else if kind == "events" {
		table = "purchase_execution_events"
	}
	query := "SELECT id,created_at FROM " + table + ` WHERE org_id=$1 AND purchase_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid))`
	args := []any{a.OrgID, purchase, at, before, limit + 1}
	if order != "" {
		query += " AND order_id=$6"
		args = append(args, order)
	}
	rows, err := s.DB.Query(ctx, query+" ORDER BY created_at DESC,id DESC LIMIT $5", args...)
	if err != nil {
		return out, err
	}
	type key struct {
		id string
		at time.Time
	}
	keys := []key{}
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.id, &k.at); err != nil {
			rows.Close()
			return out, err
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(keys) > limit {
		keys = keys[:limit]
		k := keys[len(keys)-1]
		next := s.Pager.Encode(platform.Cursor{ID: k.id, UpdatedAt: k.at, Binding: bound})
		out.NextCursor = &next
	}
	for _, k := range keys {
		v, e := load(k.id)
		if e != nil {
			return core.Page[T]{Items: []T{}}, e
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (s *Service) Orders(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Order], error) {
	return executionPage(ctx, s, a, id, "orders", "", cursor, limit, func(key string) (Order, error) { return s.Order(ctx, a, id, key) })
}
func (s *Service) Receipts(ctx context.Context, a core.Actor, id, order, cursor string, limit int) (core.Page[Receipt], error) {
	return executionPage(ctx, s, a, id, "receipts", order, cursor, limit, func(key string) (Receipt, error) { return s.Receipt(ctx, a, id, key) })
}
func (s *Service) ExecutionHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[ExecutionEvent], error) {
	return executionPage(ctx, s, a, id, "events", "", cursor, limit, func(key string) (ExecutionEvent, error) {
		var e ExecutionEvent
		err := s.DB.QueryRow(ctx, `SELECT id,seq,purchase_version,kind,order_id,receipt_id,variance_revision_id,lifecycle,actor_id,reason,created_at FROM purchase_execution_events WHERE org_id=$1 AND purchase_id=$2 AND id=$3`, a.OrgID, id, key).Scan(&e.ID, &e.Seq, &e.PurchaseVersion, &e.Kind, &e.OrderID, &e.ReceiptID, &e.VarianceRevisionID, &e.Lifecycle, &e.ActorID, &e.Reason, &e.CreatedAt)
		return e, err
	})
}
func (s *Service) Fulfillment(ctx context.Context, a core.Actor, id string) (Fulfillment, error) {
	d, err := purchaseRead(ctx, s.DB, a, id)
	if err != nil {
		return Fulfillment{}, err
	}
	out := Fulfillment{PurchaseID: id, Version: d.Version, Lifecycle: d.Lifecycle, Currency: d.Currency, Lines: []FulfillmentLine{}}
	if d.ApprovalState != "approved" {
		return out, core.Fail(409, "PURCHASE_NOT_APPROVED", "批准后才可查看采购履行额度")
	}
	err = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM purchase_orders WHERE org_id=$1 AND purchase_id=$2),(SELECT count(*) FROM purchase_receipts WHERE org_id=$1 AND purchase_id=$2),
        (SELECT count(*) FROM purchase_execution_events WHERE org_id=$1 AND purchase_id=$2 AND kind='order_cancelled')`, a.OrgID, id).Scan(&out.OrderCount, &out.ReceiptCount, &out.CancelledOrderCount)
	if err != nil {
		return out, err
	}
	var raw []byte
	if err = s.DB.QueryRow(ctx, "SELECT purchase_fulfillment_lines($1,$2)", a.OrgID, id).Scan(&raw); err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out.Lines)
	return out, err
}
