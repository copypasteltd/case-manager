package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const paymentJoin = ` FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.payment_id=p.id AND r.id=p.current_revision_id `
const paymentColumns = `p.id,p.version,p.source_key,p.project_id,p.creator_id,p.classification,p.record_kind,p.state,p.reverses_id,p.current_revision_id,r.content_hash,r.payload,r.paid_on::text,p.created_at,p.updated_at,(SELECT x.id FROM payment_records x WHERE x.org_id=p.org_id AND x.reverses_id=p.id)`

func scanPayment(row pgx.Row) (Payment, error) {
	p := Payment{}
	var payload []byte
	err := row.Scan(&p.ID, &p.Version, &p.SourceKey, &p.ProjectID, &p.CreatorID, &p.Classification, &p.RecordKind, &p.State, &p.ReversesID, &p.CurrentRevisionID, &p.ContentHash, &payload, &p.PaidOn, &p.CreatedAt, &p.UpdatedAt, &p.ReversalID)
	if err != nil {
		return p, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &p.PaymentContent)
	return p, err
}
func paymentMetadata(ctx context.Context, db platform.DBTX, org, id string) (Payment, error) {
	if !core.ValidID(id) {
		return Payment{}, core.Missing()
	}
	return scanPayment(db.QueryRow(ctx, "SELECT "+paymentColumns+paymentJoin+"WHERE p.org_id=$1 AND p.id=$2", org, id))
}
func paymentReadable(ctx context.Context, db platform.DBTX, a core.Actor, p Payment) error {
	var ok bool
	if err := db.QueryRow(ctx, "SELECT can_read_payment($1,$2,$3,$4,$5)", a.OrgID, a.UserID, p.ID, a.Channel, a.RunID != "").Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return core.Missing()
	}
	return nil
}

func paymentBalances(ctx context.Context, db platform.DBTX, org string, p *Payment) error {
	p.EffectiveAllocatedAmount = "0.000000"
	p.RefundedAmount = "0.000000"
	p.RefundAllocatedPrincipal = "0.000000"
	p.ContractReturnReservedAmount = "0.000000"
	p.UnallocatedAmount = nil
	if p.State == "draft" || p.Amount == nil {
		return nil
	}
	var net string
	if err := db.QueryRow(ctx, `SELECT COALESCE(sum(a.effect*(CASE WHEN $3='refund' THEN COALESCE(a.refund_cash_amount,a.amount) ELSE a.amount END)),0)::text FROM payment_allocations a JOIN payment_records q ON q.org_id=a.org_id AND q.id=a.payment_id JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id
        WHERE a.org_id=$1 AND (q.id=$2 OR q.reverses_id=$2 OR ($3='outgoing' AND v.refund_of_id=$2))`, org, p.ID, value(p.Direction)).Scan(&net); err != nil {
		return err
	}
	if value(p.Direction) == "refund" && len(net) > 0 && net[0] == '-' {
		net = net[1:]
	}
	if d, e := core.ParseDecimal(net); e == nil {
		net = d.String()
	}
	p.EffectiveAllocatedAmount = net
	if p.RefundFX != nil && p.RecordKind == "payment" {
		if err := db.QueryRow(ctx, `SELECT COALESCE(-sum(a.effect*a.amount),0)::text FROM payment_allocations a JOIN payment_records q ON q.org_id=a.org_id AND q.id=a.payment_id
 WHERE a.org_id=$1 AND (q.id=$2 OR q.reverses_id=$2)`, org, p.ID).Scan(&p.RefundAllocatedPrincipal); err != nil {
			return err
		}
	}

	if p.RecordKind != "payment" || !slices.Contains([]string{"outgoing", "refund"}, value(p.Direction)) {
		return nil
	}
	if value(p.Direction) == "outgoing" {
		var reserved string
		if err := db.QueryRow(ctx, "SELECT contract_cash_return_reserved($1,$2)::text", org, p.ID).Scan(&reserved); err != nil {
			return err
		}
		if d, err := core.ParseDecimal(reserved); err == nil {
			p.ContractReturnReservedAmount = d.String()
		}
		if err := db.QueryRow(ctx, `SELECT COALESCE(sum(v.refund_original_amount),0)::text FROM payment_records q JOIN payment_revisions v ON v.org_id=q.org_id AND v.id=q.current_revision_id WHERE q.org_id=$1 AND v.refund_of_id=$2 AND q.state='recorded' AND q.record_kind='payment'`, org, p.ID).Scan(&p.RefundedAmount); err != nil {
			return err
		}
	}
	if d, e := core.ParseDecimal(p.RefundedAmount); e == nil {
		p.RefundedAmount = d.String()
	}
	if p.State == "reversed" {
		zero := "0.000000"
		p.UnallocatedAmount = &zero
		return nil
	}
	amount, _ := core.ParseDecimal(*p.Amount)
	paid, _ := core.ParseDecimal(net)
	refund, _ := core.ParseDecimal(p.RefundedAmount)
	reserved, _ := core.ParseDecimal(p.ContractReturnReservedAmount)
	balance := amount.Sub(paid).Sub(refund).Sub(reserved).String()
	p.UnallocatedAmount = &balance
	return nil
}
func presentPayment(ctx context.Context, db platform.DBTX, a core.Actor, p Payment) (Payment, error) {
	if err := paymentReadable(ctx, db, a, p); err != nil {
		return Payment{}, err
	}
	var err error
	p.CounterpartyEvidence, p.Attachments, err = references(ctx, db, a, Cost{ProjectID: p.ProjectID, Classification: p.Classification}, Content{Counterparty: p.Counterparty, EvidenceVersionIDs: p.EvidenceVersionIDs}, false, false)
	if err != nil {
		return p, err
	}
	if err = paymentBalances(ctx, db, a.OrgID, &p); err != nil {
		return p, err
	}
	write := a.RunID == "" && paymentProjectLocks(ctx, db, a, p, nil, false) == nil
	p.CanEdit = write && p.State == "draft" && authz.Require(ctx, db, a, "payment.record", paymentResource(p)) == nil
	p.CanRecord = p.CanEdit
	p.CanAllocate = write && p.State == "recorded" && p.RecordKind == "payment" && slices.Contains([]string{"outgoing", "refund"}, value(p.Direction)) && authz.Require(ctx, db, a, "payment.record", paymentResource(p)) == nil
	p.CanReverse = write && p.State == "recorded" && p.RecordKind == "payment" && p.RefundedAmount == "0.000000" && authz.Require(ctx, db, a, "payment.reverse", paymentResource(p)) == nil
	if p.CanReverse {
		blocked, err := paymentHasContractReturns(ctx, db, a.OrgID, p.ID)
		if err != nil {
			return p, err
		}
		p.CanReverse = !blocked
	}
	return p, nil
}
func paymentHasContractReturns(ctx context.Context, db platform.DBTX, org, id string) (bool, error) {
	var blocked bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contract_finance_events src JOIN contract_finance_events e ON e.org_id=src.org_id AND e.source_event_id=src.id AND e.kind='refund'
		JOIN payment_records p ON p.org_id=e.org_id AND p.id=e.payment_id WHERE src.org_id=$1 AND src.payment_id=$2 AND src.kind='cash' AND p.state='recorded' AND contract_finance_remaining(e.org_id,e.id)>0)`, org, id).Scan(&blocked)
	return blocked, err
}
func (s *Service) Payment(ctx context.Context, a core.Actor, id string) (Payment, error) {
	p, err := paymentMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return p, err
	}
	return presentPayment(ctx, s.DB, a, p)
}
func validatePaymentFilter(f PaymentFilter) error {
	if len([]rune(f.Query)) > 200 || len([]rune(f.Category)) > 50 || !slices.Contains([]string{"", "draft", "recorded", "reversed", "cash"}, f.State) || !slices.Contains([]string{"", "outgoing", "incoming", "refund", "transfer"}, f.Direction) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) || (f.CompanyOnly && f.ProjectID != nil) || (f.ProjectRecords && f.ProjectID == nil) || core.DateRange(f.From, f.To) != nil {
		return core.Invalid("filter", "资金筛选无效")
	}
	return nil
}
func (s *Service) Payments(ctx context.Context, a core.Actor, f PaymentFilter) (core.Page[Payment], error) {
	out := core.Page[Payment]{Items: []Payment{}}
	if err := validatePaymentFilter(f); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	bound := core.CursorBinding(a, "payments", f.Query, f.State, f.Direction, f.Currency, f.Category, f.From, f.To, value(f.ProjectID), fmt.Sprint(f.CompanyOnly))
	if f.ProjectRecords {
		bound += "/project-records"
	}
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		v, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+paymentColumns+paymentJoin+`WHERE p.org_id=$1 AND can_read_payment(p.org_id,$2,p.id,$3,$4)
        AND ($5='' OR position(lower($5) in lower(r.title))>0 OR p.source_key=$5) AND ($6='' OR p.state=$6 OR ($6='cash' AND p.state<>'draft' AND r.direction IN ('outgoing','refund'))) AND ($7='' OR r.direction=$7) AND ($8='' OR r.currency=$8) AND ($9='' OR r.category=$9)
        AND ($10::date IS NULL OR r.paid_on>=$10) AND ($11::date IS NULL OR r.paid_on<$11)
        AND (($12::uuid IS NULL AND NOT $13) OR ($17 AND p.project_id=$12::uuid) OR EXISTS(SELECT 1 FROM payment_allocations a JOIN payment_allocation_shares sh ON sh.org_id=a.org_id AND sh.allocation_id=a.id WHERE a.org_id=p.org_id AND a.payment_id=p.id AND sh.project_id IS NOT DISTINCT FROM $12::uuid) OR contract_cash_return_share(p.org_id,COALESCE(p.reverses_id,p.id),$12::uuid)>0)
        AND ($14::timestamptz IS NULL OR (p.updated_at,p.id)<($14,$15::uuid)) ORDER BY p.updated_at DESC,p.id DESC LIMIT $16`, a.OrgID, a.UserID, a.Channel, a.RunID != "", f.Query, f.State, f.Direction, f.Currency, f.Category, optional(f.From), optional(f.To), f.ProjectID, f.CompanyOnly, at, before, f.Limit+1, f.ProjectRecords)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanPayment(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &cursor
	}
	for i, p := range out.Items {
		out.Items[i], err = presentPayment(ctx, s.DB, a, p)
		if err != nil {
			return core.Page[Payment]{Items: []Payment{}}, err
		}
	}
	return out, nil
}

const allocationColumns = `a.id,a.payment_id,a.cost_id,a.cost_revision_id,a.kind,a.effect,a.amount::text,a.root_id,a.source_allocation_id,a.claim_id,a.claim_revision_id,a.claim_line_id,a.actor_id,a.reason,a.created_at,(SELECT COALESCE(sum(x.effect*x.amount),0)::text FROM payment_allocations x WHERE x.org_id=a.org_id AND x.root_id=a.root_id),
 a.cost_amount::text,(SELECT currency FROM cost_revisions WHERE org_id=a.org_id AND id=a.cost_revision_id),
 (SELECT r.currency FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=a.org_id AND p.id=(SELECT payment_id FROM payment_allocations WHERE org_id=a.org_id AND id=a.root_id)),
 (SELECT d.exponent FROM cost_revisions r JOIN currency_definitions d ON d.code=r.currency WHERE r.org_id=a.org_id AND r.id=a.cost_revision_id),
 (SELECT d.exponent FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id JOIN currency_definitions d ON d.code=r.currency WHERE p.org_id=a.org_id AND p.id=(SELECT payment_id FROM payment_allocations WHERE org_id=a.org_id AND id=a.root_id)),
 (SELECT COALESCE(sum(x.effect*x.cost_amount),0)::text FROM payment_allocations x WHERE x.org_id=a.org_id AND x.root_id=a.root_id),a.fx_rate::text,a.fx_source,a.fx_quoted_on::text,a.refund_cash_amount::text,(SELECT r.payload FROM payment_records p JOIN payment_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE p.org_id=a.org_id AND p.id=a.payment_id)`

func scanAllocation(row pgx.Row) (PaymentAllocation, error) {
	v := PaymentAllocation{Shares: []Allocation{}, CostShares: []Allocation{}}
	var rate, source, day, cash *string
	var paymentPayload []byte
	err := row.Scan(&v.ID, &v.PaymentID, &v.CostID, &v.CostRevisionID, &v.Kind, &v.Effect, &v.Amount, &v.RootID, &v.SourceAllocationID, &v.ClaimID, &v.ClaimRevisionID, &v.ClaimLineID, &v.ActorID, &v.Reason, &v.CreatedAt, &v.RemainingAmount, &v.CostAmount, &v.CostCurrency, &v.PaymentCurrency, &v.CostExponent, &v.PaymentExponent, &v.RemainingCostAmount, &rate, &source, &day, &cash, &paymentPayload)
	if err == nil && rate != nil && source != nil && day != nil {
		v.FX = &PaymentFXQuote{Rate: *rate, Source: *source, QuotedOn: *day}
	}
	if err == nil && cash != nil {
		var p PaymentContent
		if e := json.Unmarshal(paymentPayload, &p); e != nil {
			return v, e
		}
		if p.RefundFX == nil {
			return v, core.Fail(409, "REFUND_BASIS_INVALID", "实收分配缺少固定退款凭证")
		}
		q := p.RefundFX
		v.RefundCash = &PaymentAllocationRefundCash{Amount: *cash, Currency: value(p.Currency), WholeAmount: value(p.Amount), OriginalAmount: q.OriginalAmount, OriginalCurrency: q.OriginalCurrency, Rate: q.Rate, Source: q.Source, Shares: []Allocation{}}
	}
	return v, platform.DatabaseError(err)
}
func allocationShares(ctx context.Context, db platform.DBTX, org string, a *PaymentAllocation) error {
	rows, err := db.Query(ctx, "SELECT project_id,amount::text,cost_amount::text,refund_cash_amount::text FROM payment_allocation_shares WHERE org_id=$1 AND allocation_id=$2 ORDER BY position", org, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	a.Shares = []Allocation{}
	a.CostShares = []Allocation{}
	for rows.Next() {
		var v Allocation
		var cost string
		var cash *string
		if err = rows.Scan(&v.ProjectID, &v.Amount, &cost, &cash); err != nil {
			return err
		}
		a.Shares = append(a.Shares, v)
		a.CostShares = append(a.CostShares, Allocation{ProjectID: v.ProjectID, Amount: cost})
		if a.RefundCash != nil && cash != nil {
			a.RefundCash.Shares = append(a.RefundCash.Shares, Allocation{ProjectID: v.ProjectID, Amount: *cash})
		}
	}
	return rows.Err()
}
func allocationMetadata(ctx context.Context, db platform.DBTX, org, id string) (PaymentAllocation, error) {
	if !core.ValidID(id) {
		return PaymentAllocation{}, core.Missing()
	}
	v, err := scanAllocation(db.QueryRow(ctx, "SELECT "+allocationColumns+" FROM payment_allocations a WHERE a.org_id=$1 AND a.id=$2", org, id))
	if err != nil {
		return v, err
	}
	err = allocationShares(ctx, db, org, &v)
	return v, err
}
func (s *Service) PaymentAllocations(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[PaymentAllocation], error) {
	out := core.Page[PaymentAllocation]{Items: []PaymentAllocation{}}
	if _, err := s.Payment(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "payment-allocations", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+allocationColumns+` FROM payment_allocations a WHERE a.org_id=$1 AND a.payment_id=$2 AND ($3::timestamptz IS NULL OR (a.created_at,a.id)<($3,$4::uuid)) ORDER BY a.created_at DESC,a.id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanAllocation(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	for i := range out.Items {
		if err = allocationShares(ctx, s.DB, a.OrgID, &out.Items[i]); err != nil {
			return out, err
		}
	}
	return out, nil
}

func scanPaymentRevision(row pgx.Row) (PaymentRevision, error) {
	var v PaymentRevision
	var payload []byte
	err := row.Scan(&v.ID, &v.Seq, &v.BaseRevisionID, &v.ContentHash, &payload, &v.AuthorID, &v.Reason, &v.CreatedAt)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	err = json.Unmarshal(payload, &v.PaymentContent)
	return v, err
}
func presentPaymentRevision(ctx context.Context, db platform.DBTX, a core.Actor, p Payment, v PaymentRevision) (PaymentRevision, error) {
	var err error
	v.CounterpartyEvidence, v.Attachments, err = references(ctx, db, a, Cost{ProjectID: p.ProjectID, Classification: p.Classification}, Content{Counterparty: v.Counterparty, EvidenceVersionIDs: v.EvidenceVersionIDs}, false, false)
	return v, err
}
func (s *Service) PaymentRevision(ctx context.Context, a core.Actor, id, revision string) (PaymentRevision, error) {
	p, err := s.Payment(ctx, a, id)
	if err != nil {
		return PaymentRevision{}, err
	}
	if !core.ValidID(revision) {
		return PaymentRevision{}, core.Missing()
	}
	v, err := scanPaymentRevision(s.DB.QueryRow(ctx, "SELECT "+revisionColumns+" FROM payment_revisions WHERE org_id=$1 AND payment_id=$2 AND id=$3", a.OrgID, id, revision))
	if err != nil {
		return v, err
	}
	return presentPaymentRevision(ctx, s.DB, a, p, v)
}
func (s *Service) PaymentHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[PaymentRevision], error) {
	out := core.Page[PaymentRevision]{Items: []PaymentRevision{}}
	p, err := s.Payment(ctx, a, id)
	if err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "payment-history", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+` FROM payment_revisions WHERE org_id=$1 AND payment_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanPaymentRevision(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	for i, v := range out.Items {
		out.Items[i], err = presentPaymentRevision(ctx, s.DB, a, p, v)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) PaymentEvents(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[PaymentEvent], error) {
	out := core.Page[PaymentEvent]{Items: []PaymentEvent{}}
	if _, err := s.Payment(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "payment-events", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &v.UpdatedAt
		before = &v.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,payment_version,revision_id,kind,state,actor_id,reason,created_at,allocation_id FROM payment_events WHERE org_id=$1 AND payment_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v PaymentEvent
		if err = rows.Scan(&v.ID, &v.Version, &v.RevisionID, &v.Kind, &v.State, &v.ActorID, &v.Reason, &v.CreatedAt, &v.AllocationID); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	return out, nil
}
