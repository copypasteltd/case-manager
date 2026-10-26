package finance

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func paymentResource(p Payment) authz.Resource {
	return authz.Resource{Type: "payment", ID: p.ID, ProjectID: p.ProjectID, OwnerID: &p.CreatorID, Classification: p.Classification}
}

func normalizePayment(ctx context.Context, db platform.DBTX, c PaymentContent, complete bool) (PaymentContent, error) {
	c.Title = strings.TrimSpace(c.Title)
	c.Category = strings.TrimSpace(c.Category)
	c.MethodLabel = strings.TrimSpace(c.MethodLabel)
	c.ExternalRef = strings.TrimSpace(c.ExternalRef)
	// Common money, fixed-party and file normalization has one implementation.
	base, err := normalize(ctx, db, Content{Title: c.Title, Currency: c.Currency, Amount: c.Amount, Category: c.Category, Counterparty: c.Counterparty, EvidenceVersionIDs: c.EvidenceVersionIDs, Notes: c.Notes}, false)
	if err != nil {
		return c, err
	}
	c.Currency = base.Currency
	c.Amount = base.Amount
	c.Counterparty = base.Counterparty
	c.EvidenceVersionIDs = base.EvidenceVersionIDs
	if len([]rune(c.MethodLabel)) > 100 || len([]rune(c.ExternalRef)) > 200 || (c.Direction != nil && !slices.Contains([]string{"outgoing", "incoming", "refund", "transfer"}, *c.Direction)) {
		return c, core.Invalid("direction", "请核对资金方向、付款方式与外部流水标识")
	}
	for _, id := range []*string{c.EmployeeID, c.RefundOfID} {
		if id != nil {
			*id = strings.ToLower(*id)
			if !core.ValidID(*id) {
				return c, core.Invalid("recipient", "收款员工或原付款标识无效")
			}
		}
	}
	if c.EmployeeID != nil && c.Counterparty != nil {
		return c, core.Invalid("recipient", "收款方须选择往来方或员工之一")
	}
	if c.PaidAt != nil {
		at, e := time.Parse(time.RFC3339Nano, *c.PaidAt)
		if e != nil {
			return c, core.Invalid("paid_at", "实际付款时间须包含时区")
		}
		text := at.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
		c.PaidAt = &text
	}
	if complete {
		if c.Direction == nil || c.PaidAt == nil || c.Currency == nil || c.Amount == nil || c.Category == "" || c.MethodLabel == "" || len(c.EvidenceVersionIDs) == 0 {
			return c, core.Invalid("content", "登记真实资金须填写方向、实际时间、原币金额、类别、方式和固定凭证")
		}
		amount, _ := core.ParseDecimal(*c.Amount)
		at, _ := time.Parse(time.RFC3339Nano, *c.PaidAt)
		var now time.Time
		if err = db.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return c, err
		}
		if !amount.Positive() || at.After(now) {
			return c, core.Invalid("paid_at", "资金须已实际发生且金额为正数")
		}
		if (*c.Direction == "refund") != (c.RefundOfID != nil) || (*c.Direction == "transfer" && (c.EmployeeID != nil || c.Counterparty != nil)) || (*c.Direction != "transfer" && c.EmployeeID == nil && c.Counterparty == nil) || (*c.Direction == "incoming" && c.EmployeeID != nil) {
			return c, core.Invalid("recipient", "请核对资金方向、实际收款人和退款原付款")
		}
	}
	return c, nil
}

func paymentReferences(ctx context.Context, db platform.DBTX, a core.Actor, p Payment, v PaymentContent, strict, complete bool) (*PartyEvidence, error) {
	party, _, err := references(ctx, db, a, Cost{ProjectID: p.ProjectID, Classification: p.Classification}, Content{Counterparty: v.Counterparty, EvidenceVersionIDs: v.EvidenceVersionIDs}, strict, complete)
	return party, err
}

func insertPaymentRevision(ctx context.Context, tx pgx.Tx, a core.Actor, p Payment, id string, seq int64, base *string, v PaymentContent, reason string) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO payment_revisions(id,org_id,payment_id,seq,base_revision_id,payload,content_hash,paid_at,paid_on,author_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8::timestamptz,($8::timestamptz AT TIME ZONE 'Asia/Hong_Kong')::date,$9,$10)`, id, a.OrgID, p.ID, seq, base, payload, core.Digest(payload), v.PaidAt, a.UserID, reason)
	if err != nil {
		return err
	}
	for _, file := range v.EvidenceVersionIDs {
		if _, err = tx.Exec(ctx, "INSERT INTO payment_revision_files(org_id,revision_id,version_id) VALUES($1,$2,$3)", a.OrgID, id, file); err != nil {
			return err
		}
	}
	return nil
}

func samePaymentIdentity(p, original Payment) bool {
	return value(p.ProjectID) == value(original.ProjectID) && p.Classification == original.Classification && value(p.Currency) == value(original.Currency) && value(p.EmployeeID) == value(original.EmployeeID) &&
		((p.Counterparty == nil && original.Counterparty == nil) || (p.Counterparty != nil && original.Counterparty != nil && *p.Counterparty == *original.Counterparty))
}

func paymentAmount(ctx context.Context, db platform.DBTX, currency, amount string) (core.Decimal, error) {
	var exponent int
	if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", currency).Scan(&exponent); err != nil {
		return core.Decimal{}, core.Invalid("currency", "币种未配置")
	}
	d, err := core.CheckCurrencyAmount(currency, amount, map[string]int{currency: exponent})
	if err != nil {
		return d, err
	}
	if !d.Positive() {
		return d, core.Invalid("amount", "金额须大于零")
	}
	return d, nil
}
