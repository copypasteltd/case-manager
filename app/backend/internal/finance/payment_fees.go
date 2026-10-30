package finance

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Fees classify already recorded facts. They never add another expense, cash
// movement or allocation, and never infer an exchange gain/loss from a delta.
type PaymentFeeFact struct {
	ID             string  `json:"id"`
	Version        int64   `json:"version"`
	RevisionID     string  `json:"revision_id"`
	ContentHash    string  `json:"content_hash"`
	Title          string  `json:"title"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Currency       string  `json:"currency"`
	Amount         string  `json:"amount"`
}
type PaymentFeeBasis struct {
	Payment    PaymentFeeFact    `json:"payment"`
	FeePayment PaymentFeeFact    `json:"fee_payment"`
	Cost       PaymentFeeFact    `json:"cost"`
	Allocation PaymentAllocation `json:"allocation"`
}
type PaymentFeePreview struct {
	PaymentID   string `json:"payment_id"`
	FeeVersion  int64  `json:"fee_version"`
	Basis       string `json:"basis"`
	BasisSHA256 string `json:"basis_sha256"`
}
type PaymentFeeCreate struct {
	Base
	ExpectedFeeVersion int64  `json:"expected_fee_version"`
	FeePaymentID       string `json:"fee_payment_id"`
	AllocationID       string `json:"allocation_id"`
	BasisSHA256        string `json:"basis_sha256"`
	Reviewed           bool   `json:"reviewed"`
}
type PaymentFeeRevoke struct {
	Base
	ExpectedFeeVersion int64  `json:"expected_fee_version"`
	BasisSHA256        string `json:"basis_sha256"`
	Reviewed           bool   `json:"reviewed"`
}
type PaymentFeeRevocation struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type PaymentFeeLink struct {
	ID          string                `json:"id"`
	PaymentID   string                `json:"payment_id"`
	Version     int64                 `json:"version"`
	Basis       string                `json:"basis"`
	BasisSHA256 string                `json:"basis_sha256"`
	ActorID     string                `json:"actor_id"`
	Reason      string                `json:"reason"`
	CreatedAt   time.Time             `json:"created_at"`
	Revocation  *PaymentFeeRevocation `json:"revocation"`
}
type PaymentFeeItem struct {
	Link    PaymentFeeLink    `json:"link"`
	Current PaymentAllocation `json:"current"`
	State   string            `json:"state"`
}
type PaymentFeeLedger struct {
	PaymentID  string           `json:"payment_id"`
	FeeVersion int64            `json:"fee_version"`
	CanManage  bool             `json:"can_manage"`
	Items      []PaymentFeeItem `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

func paymentFeeVersion(ctx context.Context, tx pgx.Tx, org, id string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, "SELECT payment_fee_version($1,$2)", org, id).Scan(&version)
	return version, err
}
func feePaymentFact(p Payment) PaymentFeeFact {
	return PaymentFeeFact{p.ID, p.Version, p.CurrentRevisionID, p.ContentHash, p.Title, p.ProjectID, p.Classification, value(p.Currency), value(p.Amount)}
}
func feeCostFact(c Cost) PaymentFeeFact {
	return PaymentFeeFact{c.ID, c.Version, c.CurrentRevisionID, c.ContentHash, c.Title, c.ProjectID, c.Classification, value(c.Currency), value(c.Amount)}
}
func feePayment(ctx context.Context, tx pgx.Tx, a core.Actor, id string, manage bool) (Payment, error) {
	p, err := paymentMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return p, err
	}
	if err = paymentReadable(ctx, tx, a, p); err != nil {
		return p, err
	}
	if manage {
		if err = authz.Require(ctx, tx, a, "payment.record", paymentResource(p)); err != nil {
			return p, err
		}
		if err = paymentProjectLocks(ctx, tx, a, p, nil, false); err != nil {
			return p, err
		}
	}
	return p, nil
}
func feeSources(ctx context.Context, tx pgx.Tx, a core.Actor, parent, fee, allocation string, manage, live bool) (PaymentFeeBasis, error) {
	var out PaymentFeeBasis
	p, err := feePayment(ctx, tx, a, parent, manage)
	if err != nil {
		return out, err
	}
	q, err := feePayment(ctx, tx, a, fee, manage)
	if err != nil {
		return out, err
	}
	root, err := allocationMetadata(ctx, tx, a.OrgID, allocation)
	if err != nil {
		return out, err
	}
	if root.PaymentID != q.ID || root.Kind != "allocation" || root.Effect != 1 || root.RootID != root.ID {
		return out, core.Missing()
	}
	c, err := metadata(ctx, tx, a.OrgID, root.CostID)
	if err != nil {
		return out, err
	}
	if err = readable(ctx, tx, a, c, c.CurrentRevisionID); err != nil {
		return out, err
	}
	if manage {
		if err = authz.Require(ctx, tx, a, "cost.record", resource(c)); err != nil {
			return out, err
		}
		if !writable(ctx, tx, a, c) {
			return out, core.Fail(409, "PROJECT_READ_ONLY", "费用关联范围当前不可更改")
		}
	}
	if live {
		cash, _ := core.ParseDecimal(root.RemainingAmount)
		cost, _ := core.ParseDecimal(root.RemainingCostAmount)
		if p.RecordKind != "payment" || p.State != "recorded" || value(p.Direction) != "outgoing" || q.RecordKind != "payment" || q.State != "recorded" || value(q.Direction) != "outgoing" || c.EntryKind != "expense" || c.State != "confirmed" || !cash.Positive() || !cost.Positive() {
			return out, core.Fail(409, "PAYMENT_FEE_SOURCE_INACTIVE", "请选择未冲销付款、已确认费用及仍有效的原分配")
		}
		if value(p.ProjectID) != value(q.ProjectID) || value(p.ProjectID) != value(c.ProjectID) || p.Classification != q.Classification || p.Classification != c.Classification {
			return out, core.Invalid("fee", "原付款、手续费资金与费用须同管理项目、同分类")
		}
	}
	out = PaymentFeeBasis{feePaymentFact(p), feePaymentFact(q), feeCostFact(c), root}
	return out, nil
}
func feeBasisText(v PaymentFeeBasis) (string, string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", "", err
	}
	if len(raw) > 131072 {
		return "", "", core.Invalid("fee", "手续费依据超过可核对范围")
	}
	return string(raw), core.Digest(raw), nil
}
func scanPaymentFee(row pgx.Row) (PaymentFeeLink, error) {
	var v PaymentFeeLink
	var revID, actor, reason *string
	var seq *int64
	var at *time.Time
	err := row.Scan(&v.ID, &v.PaymentID, &v.Version, &v.Basis, &v.BasisSHA256, &v.ActorID, &v.Reason, &v.CreatedAt, &revID, &seq, &actor, &reason, &at)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	if core.Digest([]byte(v.Basis)) != v.BasisSHA256 {
		return v, core.Fail(500, "PAYMENT_FEE_INTEGRITY_FAILED", "手续费依据校验失败")
	}
	if revID != nil {
		v.Revocation = &PaymentFeeRevocation{*revID, *seq, *actor, *reason, *at}
	}
	return v, nil
}

const paymentFeeColumns = `l.id,l.payment_id,l.version,l.basis,l.basis_sha256,l.actor_id,l.reason,l.created_at,r.id,r.version,r.actor_id,r.reason,r.created_at`
const paymentFeeJoin = ` FROM payment_fee_links l LEFT JOIN payment_fee_revocations r ON r.org_id=l.org_id AND r.link_id=l.id `

func paymentFeeRead(ctx context.Context, tx pgx.Tx, org, parent, id string) (PaymentFeeLink, error) {
	return scanPaymentFee(tx.QueryRow(ctx, "SELECT "+paymentFeeColumns+paymentFeeJoin+" WHERE l.org_id=$1 AND l.payment_id=$2 AND l.id=$3", org, parent, id))
}
func (s *Service) PaymentFeeBasis(ctx context.Context, a core.Actor, id, fee, allocation string) (PaymentFeePreview, error) {
	var out PaymentFeePreview
	if !core.ValidID(fee) || !core.ValidID(allocation) {
		return out, core.Invalid("fee", "请选择准确资金和原分配")
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	b, err := feeSources(ctx, tx, a, id, fee, allocation, true, true)
	if err != nil {
		return out, err
	}
	out.PaymentID = id
	out.FeeVersion, err = paymentFeeVersion(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.Basis, out.BasisSHA256, err = feeBasisText(b)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) PaymentFees(ctx context.Context, a core.Actor, id, cursor string, limit int) (PaymentFeeLedger, error) {
	out := PaymentFeeLedger{PaymentID: id, Items: []PaymentFeeItem{}}
	if limit < 1 || limit > 100 {
		return out, core.Invalid("limit", "每页需为1至100条手续费关联")
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	p, err := feePayment(ctx, tx, a, id, false)
	if err != nil {
		return out, err
	}
	out.FeeVersion, err = paymentFeeVersion(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	out.CanManage = p.RecordKind == "payment" && value(p.Direction) == "outgoing" && p.State != "draft" && authz.Require(ctx, tx, a, "payment.record", paymentResource(p)) == nil && paymentProjectLocks(ctx, tx, a, p, nil, false) == nil
	bound := core.CursorBinding(a, "payment-fees", id+":"+strconv.FormatInt(out.FeeVersion, 10))
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
	rows, err := tx.Query(ctx, "SELECT "+paymentFeeColumns+paymentFeeJoin+` WHERE l.org_id=$1 AND l.payment_id=$2 AND ($3::timestamptz IS NULL OR (l.created_at,l.id)<($3,$4::uuid)) ORDER BY l.created_at DESC,l.id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	links := []PaymentFeeLink{}
	for rows.Next() {
		v, e := scanPaymentFee(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		links = append(links, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(links) > limit {
		links = links[:limit]
		last := links[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &next
	}
	for _, link := range links {
		var b PaymentFeeBasis
		if json.Unmarshal([]byte(link.Basis), &b) != nil || b.Payment.ID != id {
			return out, core.Missing()
		}
		current, e := feeSources(ctx, tx, a, id, b.FeePayment.ID, b.Allocation.ID, false, false)
		if e != nil {
			return out, e
		}
		state := "active"
		cash, _ := core.ParseDecimal(current.Allocation.RemainingAmount)
		cost, _ := core.ParseDecimal(current.Allocation.RemainingCostAmount)
		if !cash.Positive() || !cost.Positive() {
			state = "exhausted"
		}
		if link.Revocation != nil {
			state = "revoked"
		}
		out.Items = append(out.Items, PaymentFeeItem{link, current.Allocation, state})
	}
	return out, tx.Commit(ctx)
}
func (s *Service) CreatePaymentFee(ctx context.Context, cmd core.Command, id string, in PaymentFeeCreate) (core.Result, error) {
	if validBase(in.Base) != nil || in.ExpectedFeeVersion < 0 || !core.ValidID(in.FeePaymentID) || !core.ValidID(in.AllocationID) || !in.Reviewed || !feeHash(in.BasisSHA256) {
		return core.Result{}, core.Invalid("fee", "请核对准确手续费依据、当前版本并明确确认")
	}
	// Three initial employees: serialize this short metadata command against all
	// ordinary company writes, preserving their established project/claim order.
	cmd.ExclusiveOrganizationLock = true
	authorize := func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); err != nil {
			return err
		}
		_, err := feeSources(ctx, tx, cmd.Actor, id, in.FeePaymentID, in.AllocationID, true, false)
		return err
	}
	return s.Commands.Do(ctx, cmd, authorize, func(tx pgx.Tx) (core.Result, error) {
		b, err := feeSources(ctx, tx, cmd.Actor, id, in.FeePaymentID, in.AllocationID, true, true)
		if err != nil {
			return core.Result{}, err
		}
		text, hash, err := feeBasisText(b)
		if err != nil {
			return core.Result{}, err
		}
		version, err := paymentFeeVersion(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if !feeBaseMatches(b.Payment, in.Base) || version != in.ExpectedFeeVersion || hash != in.BasisSHA256 {
			return core.Result{}, core.Fail(409, "PAYMENT_FEE_BASIS_CHANGED", "付款、费用、分配余额或手续费关联已变化，请重新读取核对")
		}
		var used bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payment_fee_links l WHERE l.org_id=$1 AND l.allocation_id=$2 AND NOT EXISTS(SELECT 1 FROM payment_fee_revocations r WHERE r.org_id=l.org_id AND r.link_id=l.id))`, cmd.Actor.OrgID, in.AllocationID).Scan(&used); err != nil {
			return core.Result{}, err
		}
		if used {
			return core.Result{}, core.Fail(409, "PAYMENT_FEE_ALREADY_LINKED", "此原分配已有有效手续费关联，请先核对或撤销错误关联")
		}
		link := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO payment_fee_links(id,org_id,payment_id,version,fee_payment_id,cost_id,allocation_id,basis,basis_sha256,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, link, cmd.Actor.OrgID, id, version+1, in.FeePaymentID, b.Cost.ID, in.AllocationID, text, hash, cmd.Actor.UserID, cmd.Actor.SessionID, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		saved, err := paymentFeeRead(ctx, tx, cmd.Actor.OrgID, id, link)
		return core.Result{Status: 201, Data: saved, Event: core.Event{Type: "payment.fee.linked", ResourceType: "payment", ResourceID: id, Version: version + 1, Payload: map[string]any{"fee_link_id": link, "fee_payment_id": in.FeePaymentID, "allocation_id": in.AllocationID, "basis_sha256": hash, "reason": in.Reason}}}, err
	})
}
func feeBaseMatches(f PaymentFeeFact, b Base) bool {
	return f.Version == b.ExpectedVersion && f.RevisionID == strings.ToLower(b.RevisionID) && f.ContentHash == b.ContentHash
}
func feeHash(v string) bool { return len(v) == 64 && strings.Trim(v, "0123456789abcdef") == "" }
func (s *Service) RevokePaymentFee(ctx context.Context, cmd core.Command, id, link string, in PaymentFeeRevoke) (core.Result, error) {
	if validBase(in.Base) != nil || in.ExpectedFeeVersion < 1 || !core.ValidID(link) || !in.Reviewed || !feeHash(in.BasisSHA256) {
		return core.Result{}, core.Invalid("fee", "请核对准确关联、当前版本并明确确认撤销")
	}
	cmd.ExclusiveOrganizationLock = true
	var saved PaymentFeeLink
	authorize := func(tx pgx.Tx) error {
		if err := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); err != nil {
			return err
		}
		var err error
		saved, err = paymentFeeRead(ctx, tx, cmd.Actor.OrgID, id, link)
		if err != nil {
			return err
		}
		var basis PaymentFeeBasis
		if json.Unmarshal([]byte(saved.Basis), &basis) != nil {
			return core.Missing()
		}
		_, err = feeSources(ctx, tx, cmd.Actor, id, basis.FeePayment.ID, basis.Allocation.ID, true, false)
		return err
	}
	return s.Commands.Do(ctx, cmd, authorize, func(tx pgx.Tx) (core.Result, error) {
		p, err := feePayment(ctx, tx, cmd.Actor, id, true)
		if err != nil {
			return core.Result{}, err
		}
		version, err := paymentFeeVersion(ctx, tx, cmd.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if !feeBaseMatches(feePaymentFact(p), in.Base) || version != in.ExpectedFeeVersion || saved.BasisSHA256 != in.BasisSHA256 || saved.Revocation != nil {
			return core.Result{}, core.Fail(409, "PAYMENT_FEE_BASIS_CHANGED", "关联或付款已变化，请重新核对当前历史")
		}
		_, err = tx.Exec(ctx, `INSERT INTO payment_fee_revocations(id,org_id,payment_id,version,link_id,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, core.NewID(), cmd.Actor.OrgID, id, version+1, link, cmd.Actor.UserID, cmd.Actor.SessionID, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		saved, err = paymentFeeRead(ctx, tx, cmd.Actor.OrgID, id, link)
		return core.Result{Status: 200, Data: saved, Event: core.Event{Type: "payment.fee.revoked", ResourceType: "payment", ResourceID: id, Version: version + 1, Payload: map[string]any{"fee_link_id": link, "basis_sha256": saved.BasisSHA256, "reason": in.Reason}}}, err
	})
}
