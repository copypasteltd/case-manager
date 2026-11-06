package finance

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// An explicit reviewed realization ledger. Original expense, payment and
// allocation facts never change. Each journal fixes one remaining dual-unit
// allocation and one immutable manual FX book, not the current default book.
type FXJournalBasis struct {
	Payment     PaymentFeeFact    `json:"payment"`
	Cost        PaymentFeeFact    `json:"cost"`
	Allocation  PaymentAllocation `json:"allocation"`
	Book        FXBook            `json:"book"`
	CostDate    string            `json:"cost_date"`
	PaymentDate string            `json:"payment_date"`
	Carrying    SourceValuation   `json:"carrying"`
	Settled     SourceValuation   `json:"settled"`
	Gain        string            `json:"gain"`
	Loss        string            `json:"loss"`
	Method      string            `json:"method"`
}
type FXJournalPreview struct {
	PaymentID      string `json:"payment_id"`
	JournalVersion int64  `json:"journal_version"`
	Basis          string `json:"basis"`
	BasisSHA256    string `json:"basis_sha256"`
}
type FXJournalCreate struct {
	Base
	ExpectedJournalVersion int64  `json:"expected_journal_version"`
	AllocationID           string `json:"allocation_id"`
	FXBookID               string `json:"fx_book_id"`
	BasisSHA256            string `json:"basis_sha256"`
	RecognizedOn           string `json:"recognized_on"`
	Reviewed               bool   `json:"reviewed"`
}
type FXJournalRevoke struct {
	Base
	ExpectedJournalVersion int64  `json:"expected_journal_version"`
	BasisSHA256            string `json:"basis_sha256"`
	Reviewed               bool   `json:"reviewed"`
}
type FXJournal struct {
	ID           string                `json:"id"`
	PaymentID    string                `json:"payment_id"`
	Version      int64                 `json:"version"`
	Basis        string                `json:"basis"`
	BasisSHA256  string                `json:"basis_sha256"`
	RecognizedOn string                `json:"recognized_on"`
	ActorID      string                `json:"actor_id"`
	Reason       string                `json:"reason"`
	CreatedAt    time.Time             `json:"created_at"`
	Revocation   *PaymentFeeRevocation `json:"revocation"`
}
type FXJournalItem struct {
	Journal FXJournal `json:"journal"`
	State   string    `json:"state"`
}
type FXJournalTotal struct {
	Currency string `json:"currency"`
	Gain     string `json:"gain"`
	Loss     string `json:"loss"`
	Count    int    `json:"count"`
}
type FXJournalLedger struct {
	PaymentID      string           `json:"payment_id"`
	JournalVersion int64            `json:"journal_version"`
	CanManage      bool             `json:"can_manage"`
	Items          []FXJournalItem  `json:"items"`
	Totals         []FXJournalTotal `json:"totals"`
	Complete       bool             `json:"complete"`
}

func fxJournalVersion(ctx context.Context, tx pgx.Tx, org, id string) (int64, error) {
	var v int64
	err := tx.QueryRow(ctx, "SELECT payment_fx_journal_version($1,$2)", org, id).Scan(&v)
	return v, err
}
func fxJournalSources(ctx context.Context, tx pgx.Tx, a core.Actor, payment, allocation, bookID string, manage, live bool) (FXJournalBasis, error) {
	var b FXJournalBasis
	p, err := feePayment(ctx, tx, a, payment, manage)
	if err != nil {
		return b, err
	}
	root, err := allocationMetadata(ctx, tx, a.OrgID, allocation)
	if err != nil {
		return b, err
	}
	if root.PaymentID != payment || root.Kind != "allocation" || root.RootID != root.ID || root.Effect != 1 || root.CostCurrency == root.PaymentCurrency {
		return b, core.Missing()
	}
	c, err := metadata(ctx, tx, a.OrgID, root.CostID)
	if err != nil {
		return b, err
	}
	if err = readable(ctx, tx, a, c, c.CurrentRevisionID); err != nil {
		return b, err
	}
	if err = fxReadAuthority(ctx, tx, a, p.ProjectID); err != nil {
		return b, err
	}
	if manage {
		if err = authz.Require(ctx, tx, a, "cost.record", resource(c)); err != nil {
			return b, err
		}
		if !writable(ctx, tx, a, c) {
			return b, core.Fail(409, "PROJECT_READ_ONLY", "费用关联范围当前不可更改")
		}
	}
	paid, e := core.ParseDecimal(root.RemainingAmount)
	cost, ce := core.ParseDecimal(root.RemainingCostAmount)
	if live && (p.RecordKind != "payment" || p.State != "recorded" || value(p.Direction) != "outgoing" || c.EntryKind != "expense" || c.State != "confirmed" || e != nil || ce != nil || !paid.Positive() || !cost.Positive() || value(p.ProjectID) != value(c.ProjectID) || p.Classification != c.Classification) {
		return b, core.Fail(409, "FX_JOURNAL_SOURCE_INACTIVE", "请选择同范围、同分类且仍有双币余额的有效付款与费用原分配")
	}
	book, err := readFXBook(ctx, tx, a.OrgID, bookID, true)
	if err != nil {
		return b, err
	}
	var exponent int
	if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", book.BaseCurrency).Scan(&exponent); err != nil {
		return b, err
	}
	carrying, err := valueSource(ReportSource{State: c.State, Currency: c.Currency, ContributionAmount: &root.RemainingCostAmount}, *book.Content, exponent, value(c.OccurredOn))
	if err != nil {
		return b, err
	}
	settled, err := valueSource(ReportSource{State: p.State, Currency: p.Currency, ContributionAmount: &root.RemainingAmount}, *book.Content, exponent, value(p.PaidOn))
	if err != nil {
		return b, err
	}
	if carrying.BaseAmount == nil || settled.BaseAmount == nil {
		return b, core.Fail(409, "FX_JOURNAL_RATE_REQUIRED", "所选汇率版本必须覆盖费用发生日和实际付款日的两个币种")
	}
	ca, err := core.ParseDecimal(*carrying.BaseAmount)
	if err != nil {
		return b, err
	}
	sa, err := core.ParseDecimal(*settled.BaseAmount)
	if err != nil {
		return b, err
	}
	gain, loss := "0.000000", "0.000000"
	if ca.Cmp(sa) >= 0 {
		gain = ca.Sub(sa).String()
	} else {
		loss = sa.Sub(ca).String()
	}
	book.Content = nil
	b = FXJournalBasis{feePaymentFact(p), feeCostFact(c), root, book, value(c.OccurredOn), value(p.PaidOn), carrying, settled, gain, loss, "reviewed_remaining_allocation_v1"}
	return b, nil
}
func fxJournalText(b FXJournalBasis) (string, string, error) {
	raw, e := json.Marshal(b)
	if e != nil {
		return "", "", e
	}
	if len(raw) > 131072 {
		return "", "", core.Invalid("basis", "汇兑依据超过核对范围")
	}
	return string(raw), core.Digest(raw), nil
}
func (s *Service) FXJournalBasis(ctx context.Context, a core.Actor, id, allocation, book string) (FXJournalPreview, error) {
	var out FXJournalPreview
	if !core.ValidID(allocation) || !core.ValidID(book) {
		return out, core.Invalid("basis", "请选择准确原分配与固定汇率版本")
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return out, e
	}
	b, e := fxJournalSources(ctx, tx, a, id, allocation, book, true, true)
	if e != nil {
		return out, e
	}
	out.PaymentID = id
	out.JournalVersion, e = fxJournalVersion(ctx, tx, a.OrgID, id)
	if e != nil {
		return out, e
	}
	out.Basis, out.BasisSHA256, e = fxJournalText(b)
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}

const fxJournalColumns = `l.id,l.payment_id,l.version,l.basis,l.basis_sha256,l.recognized_on::text,l.actor_id,l.reason,l.created_at,r.id,r.version,r.actor_id,r.reason,r.created_at`
const fxJournalJoin = ` FROM payment_fx_journals l LEFT JOIN payment_fx_journal_revocations r ON r.org_id=l.org_id AND r.journal_id=l.id `

func scanFXJournal(row pgx.Row) (FXJournal, error) {
	var v FXJournal
	var id, actor, reason *string
	var version *int64
	var at *time.Time
	e := row.Scan(&v.ID, &v.PaymentID, &v.Version, &v.Basis, &v.BasisSHA256, &v.RecognizedOn, &v.ActorID, &v.Reason, &v.CreatedAt, &id, &version, &actor, &reason, &at)
	if e != nil {
		return v, platform.DatabaseError(e)
	}
	if core.Digest([]byte(v.Basis)) != v.BasisSHA256 {
		return v, core.Fail(500, "FX_JOURNAL_INTEGRITY_FAILED", "汇兑台账依据校验失败")
	}
	if id != nil {
		v.Revocation = &PaymentFeeRevocation{*id, *version, *actor, *reason, *at}
	}
	return v, nil
}
func readFXJournal(ctx context.Context, tx pgx.Tx, org, payment, id string) (FXJournal, error) {
	return scanFXJournal(tx.QueryRow(ctx, "SELECT "+fxJournalColumns+fxJournalJoin+" WHERE l.org_id=$1 AND l.payment_id=$2 AND l.id=$3", org, payment, id))
}
func (s *Service) FXJournals(ctx context.Context, a core.Actor, id string) (FXJournalLedger, error) {
	out := FXJournalLedger{PaymentID: id, Items: []FXJournalItem{}, Totals: []FXJournalTotal{}}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return out, e
	}
	p, e := feePayment(ctx, tx, a, id, false)
	if e != nil {
		return out, e
	}
	if e = fxReadAuthority(ctx, tx, a, p.ProjectID); e != nil {
		return out, e
	}
	out.CanManage = p.RecordKind == "payment" && value(p.Direction) == "outgoing" && p.State != "draft" && authz.Require(ctx, tx, a, "payment.record", paymentResource(p)) == nil && paymentProjectLocks(ctx, tx, a, p, nil, false) == nil
	out.JournalVersion, e = fxJournalVersion(ctx, tx, a.OrgID, id)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT "+fxJournalColumns+fxJournalJoin+" WHERE l.org_id=$1 AND l.payment_id=$2 ORDER BY l.version LIMIT 1001", a.OrgID, id)
	if e != nil {
		return out, e
	}
	journals := []FXJournal{}
	for rows.Next() {
		v, err := scanFXJournal(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		journals = append(journals, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(journals) > 1000 {
		return out, core.Fail(409, "FX_JOURNAL_REVIEW_LIMIT", "本笔付款汇兑历史超过完整核对上限，请由财务管理员处理")
	}
	totals := map[string]FXJournalTotal{}
	for _, journal := range journals {
		var b FXJournalBasis
		if json.Unmarshal([]byte(journal.Basis), &b) != nil || b.Payment.ID != id {
			return out, core.Missing()
		}
		current, err := fxJournalSources(ctx, tx, a, id, b.Allocation.ID, b.Book.ID, false, false)
		if err != nil {
			return out, err
		}
		state := "active"
		_, hash, err := fxJournalText(current)
		if err != nil {
			return out, err
		}
		if hash != journal.BasisSHA256 || p.State != "recorded" {
			state = "basis_changed"
		}
		if journal.Revocation != nil {
			state = "revoked"
		}
		out.Items = append(out.Items, FXJournalItem{journal, state})
		if state == "active" {
			v := totals[b.Book.BaseCurrency]
			v.Currency = b.Book.BaseCurrency
			g, _ := core.ParseDecimal(v.Gain)
			l, _ := core.ParseDecimal(v.Loss)
			bg, ge := core.ParseDecimal(b.Gain)
			bl, le := core.ParseDecimal(b.Loss)
			if ge != nil || le != nil {
				return out, core.Missing()
			}
			v.Gain = g.Add(bg).String()
			v.Loss = l.Add(bl).String()
			v.Count++
			totals[v.Currency] = v
		}
	}
	keys := []string{}
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Totals = append(out.Totals, totals[k])
	}
	out.Complete = true
	return out, tx.Commit(ctx)
}
func (s *Service) CreateFXJournal(ctx context.Context, cmd core.Command, id string, in FXJournalCreate) (core.Result, error) {
	if validBase(in.Base) != nil || in.ExpectedJournalVersion < 0 || !core.ValidID(in.AllocationID) || !core.ValidID(in.FXBookID) || !feeHash(in.BasisSHA256) || !in.Reviewed || core.Date(in.RecognizedOn) != nil {
		return core.Result{}, core.Invalid("journal", "请核对汇兑依据、准确版本、确认日期并明确确认")
	}
	cmd.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		if e := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); e != nil {
			return e
		}
		_, e := fxJournalSources(ctx, tx, cmd.Actor, id, in.AllocationID, in.FXBookID, true, false)
		return e
	}, func(tx pgx.Tx) (core.Result, error) {
		b, e := fxJournalSources(ctx, tx, cmd.Actor, id, in.AllocationID, in.FXBookID, true, true)
		if e != nil {
			return core.Result{}, e
		}
		text, hash, e := fxJournalText(b)
		if e != nil {
			return core.Result{}, e
		}
		version, e := fxJournalVersion(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if !feeBaseMatches(b.Payment, in.Base) || version != in.ExpectedJournalVersion || hash != in.BasisSHA256 {
			return core.Result{}, core.Fail(409, "FX_JOURNAL_BASIS_CHANGED", "原分配、费用、付款或台账已变化，请重新核对")
		}
		if in.RecognizedOn < b.PaymentDate {
			return core.Result{}, core.Invalid("recognized_on", "确认日不得早于实际付款日")
		}
		var used bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payment_fx_journals j WHERE j.org_id=$1 AND j.allocation_id=$2 AND NOT EXISTS(SELECT 1 FROM payment_fx_journal_revocations r WHERE r.org_id=j.org_id AND r.journal_id=j.id))`, cmd.Actor.OrgID, in.AllocationID).Scan(&used)
		if e != nil {
			return core.Result{}, e
		}
		if used {
			return core.Result{}, core.Fail(409, "FX_JOURNAL_ALREADY_ACTIVE", "原分配已有未撤销汇兑记录，请先核对并撤销旧记录")
		}
		journal := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO payment_fx_journals(id,org_id,payment_id,version,allocation_id,cost_id,fx_book_id,basis,basis_sha256,recognized_on,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, journal, cmd.Actor.OrgID, id, version+1, in.AllocationID, b.Cost.ID, in.FXBookID, text, hash, in.RecognizedOn, cmd.Actor.UserID, cmd.Actor.SessionID, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		v, e := readFXJournal(ctx, tx, cmd.Actor.OrgID, id, journal)
		return core.Result{Status: 201, Data: v, Event: core.Event{Type: "payment.fx_difference.recorded", ResourceType: "payment", ResourceID: id, Version: version + 1, Payload: map[string]any{"journal_id": journal, "basis_sha256": hash, "allocation_id": in.AllocationID, "fx_book_id": in.FXBookID, "reason": in.Reason}}}, e
	})
}
func (s *Service) RevokeFXJournal(ctx context.Context, cmd core.Command, id, journal string, in FXJournalRevoke) (core.Result, error) {
	if validBase(in.Base) != nil || in.ExpectedJournalVersion < 1 || !core.ValidID(journal) || !feeHash(in.BasisSHA256) || !in.Reviewed {
		return core.Result{}, core.Invalid("journal", "请核对准确记录、当前版本并明确确认撤销")
	}
	cmd.ExclusiveOrganizationLock = true
	var saved FXJournal
	return s.Commands.Do(ctx, cmd, func(tx pgx.Tx) error {
		if e := identity.RequireInteractiveSession(ctx, tx, cmd.Actor, true); e != nil {
			return e
		}
		var e error
		saved, e = readFXJournal(ctx, tx, cmd.Actor.OrgID, id, journal)
		if e != nil {
			return e
		}
		var b FXJournalBasis
		if json.Unmarshal([]byte(saved.Basis), &b) != nil {
			return core.Missing()
		}
		_, e = fxJournalSources(ctx, tx, cmd.Actor, id, b.Allocation.ID, b.Book.ID, true, false)
		return e
	}, func(tx pgx.Tx) (core.Result, error) {
		p, e := feePayment(ctx, tx, cmd.Actor, id, true)
		if e != nil {
			return core.Result{}, e
		}
		version, e := fxJournalVersion(ctx, tx, cmd.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if !feeBaseMatches(feePaymentFact(p), in.Base) || version != in.ExpectedJournalVersion || saved.BasisSHA256 != in.BasisSHA256 || saved.Revocation != nil {
			return core.Result{}, core.Fail(409, "FX_JOURNAL_BASIS_CHANGED", "记录或付款已变化，请重新读取完整历史")
		}
		_, e = tx.Exec(ctx, `INSERT INTO payment_fx_journal_revocations(id,org_id,payment_id,version,journal_id,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, core.NewID(), cmd.Actor.OrgID, id, version+1, journal, cmd.Actor.UserID, cmd.Actor.SessionID, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		v, e := readFXJournal(ctx, tx, cmd.Actor.OrgID, id, journal)
		return core.Result{Status: 200, Data: v, Event: core.Event{Type: "payment.fx_difference.revoked", ResourceType: "payment", ResourceID: id, Version: version + 1, Payload: map[string]any{"journal_id": journal, "basis_sha256": saved.BasisSHA256, "reason": in.Reason}}}, e
	})
}
