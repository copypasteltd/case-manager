package spend

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func Suppliers(c Content) []Supplier {
	out := []Supplier{}
	for _, s := range c.SupplierCandidates {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	for _, l := range c.Lines {
		if l.Supplier != nil && !slices.Contains(out, *l.Supplier) {
			out = append(out, *l.Supplier)
		}
	}
	return out
}
func FileVersionIDs(c Content) []string {
	out := []string{}
	for _, id := range c.QuoteVersionIDs {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, l := range c.Lines {
		for _, id := range l.ReceiptVersionIDs {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
func hidden(err error) bool {
	return slices.Contains([]int{403, 404, 409, 422}, core.PublicError(err).Status)
}

// Evidence is resolved under current access. No object key or storage credential
// is projected into business content or approval snapshots.
func References(ctx context.Context, db platform.DBTX, a core.Actor, d Record, c Content, approval, strict bool) ([]SupplierEvidence, []fileref.Evidence, error) {
	suppliers, files := []SupplierEvidence{}, []fileref.Evidence{}
	for _, s := range Suppliers(c) {
		p, v, err := party.ResolveBusinessReference(ctx, db, a, s.PartyID, s.RevisionID, approval)
		if err != nil {
			if !strict && hidden(err) {
				continue
			}
			return nil, nil, err
		}
		var adequate bool
		if err = db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", p.Classification, d.Classification).Scan(&adequate); err != nil {
			return nil, nil, err
		}
		if !adequate || (p.ProjectID != nil && (d.ProjectID == nil || *p.ProjectID != *d.ProjectID)) {
			if !strict {
				continue
			}
			return nil, nil, core.Invalid("supplier", "供应商资料须在相同范围，分类不能高于申请")
		}
		suppliers = append(suppliers, SupplierEvidence{Supplier: s, Name: v.Name, Kind: v.Kind})
	}
	for _, id := range FileVersionIDs(c) {
		var v fileref.Evidence
		var err error
		if approval {
			v, err = fileref.ForApproval(ctx, db, a, id, d.ProjectID, d.Classification)
		} else {
			v, err = fileref.Resolve(ctx, db, a, id, d.ProjectID, d.Classification)
		}
		if err != nil {
			if !strict && hidden(err) {
				continue
			}
			return nil, nil, err
		}
		files = append(files, v)
	}
	return suppliers, files, nil
}

func normalize(ctx context.Context, db platform.DBTX, a core.Actor, d Record, c Content, previous *Content, complete bool) (Content, []LineValue, *string, *string, error) {
	fail := func(field, message string) (Content, []LineValue, *string, *string, error) {
		return c, nil, nil, nil, core.Invalid(field, message)
	}
	c.Title = strings.TrimSpace(c.Title)
	c.Category = strings.TrimSpace(c.Category)
	c.Purpose = strings.TrimSpace(c.Purpose)
	if core.Name(c.Title) != nil || len([]rune(c.Category)) > 50 || len([]rune(c.Purpose)) > 4000 || len([]rune(c.Notes)) > 20000 || len([]rune(c.BudgetReference)) > 500 {
		return fail("content", "请核对标题、类别、用途和备注长度")
	}
	if !slices.Contains([]string{"", "net_plus_tax", "gross_includes_tax"}, c.PricingBasis) || !slices.Contains([]string{"", "half_up", "half_even"}, c.RoundingMode) {
		return fail("pricing_basis", "请明确计价含税方式和舍入规则")
	}
	if d.Kind == "claim" && c.PricingBasis != "" && c.PricingBasis != "gross_includes_tax" {
		return fail("pricing_basis", "报销填写实际含税费用，税额仅作说明")
	}
	if c.OwnerID != nil {
		v := strings.ToLower(*c.OwnerID)
		c.OwnerID = &v
		if !core.ValidID(v) {
			return fail("owner_id", "负责人标识无效")
		}
		var active bool
		if err := db.QueryRow(ctx, "SELECT status='active' FROM users WHERE org_id=$1 AND id=$2", a.OrgID, v).Scan(&active); err != nil {
			return fail("owner_id", "负责人不属于当前组织")
		}
		if complete && !active {
			return fail("owner_id", "负责人账号已停用")
		}
	}
	exponent := 0
	if c.Currency != nil {
		if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", *c.Currency).Scan(&exponent); err != nil {
			if err == pgx.ErrNoRows {
				return fail("currency", "币种不在支持的精度目录中")
			}
			return c, nil, nil, nil, err
		}
	}
	for _, date := range []*string{c.ExpectedDeliveryDate, c.PeriodStart, c.PeriodEnd} {
		if date != nil && core.Date(*date) != nil {
			return fail("date", "请填写有效日期或留空")
		}
	}
	if c.PeriodStart != nil && c.PeriodEnd != nil && *c.PeriodStart > *c.PeriodEnd {
		return fail("period_end", "费用期间结束不能早于开始")
	}
	if c.SupplierCandidates == nil {
		c.SupplierCandidates = []Supplier{}
	}
	if c.QuoteVersionIDs == nil {
		c.QuoteVersionIDs = []string{}
	}
	if c.Lines == nil {
		c.Lines = []Line{}
	}
	if len(c.Lines) > 100 || len(c.SupplierCandidates) > 20 || len(c.QuoteVersionIDs) > 20 {
		return fail("lines", "明细最多100行，候选供应商和报价各最多20项")
	}
	normalizeSupplier := func(s *Supplier) error {
		s.PartyID = strings.ToLower(s.PartyID)
		s.RevisionID = strings.ToLower(s.RevisionID)
		if !core.ValidID(s.PartyID) || !core.ValidID(s.RevisionID) {
			return core.Invalid("supplier", "须指定供应商及其准确资料版本")
		}
		return nil
	}
	for i := range c.SupplierCandidates {
		if err := normalizeSupplier(&c.SupplierCandidates[i]); err != nil {
			return c, nil, nil, nil, err
		}
		if slices.Contains(c.SupplierCandidates[:i], c.SupplierCandidates[i]) {
			return fail("supplier_candidates", "候选供应商版本不能重复")
		}
	}
	for i, id := range c.QuoteVersionIDs {
		id = strings.ToLower(id)
		c.QuoteVersionIDs[i] = id
		if !core.ValidID(id) || slices.Contains(c.QuoteVersionIDs[:i], id) {
			return fail("quote_version_ids", "报价须固定有效且不重复的文件版本")
		}
	}
	ids := map[string]bool{}
	for i := range c.Lines {
		l := &c.Lines[i]
		l.ID = strings.ToLower(l.ID)
		l.Description = strings.TrimSpace(l.Description)
		l.Category = strings.TrimSpace(l.Category)
		if !core.ValidID(l.ID) || ids[l.ID] || len([]rune(l.Description)) > 1000 || len([]rune(l.Unit)) > 50 || len([]rune(l.Category)) > 50 || len([]rune(l.MissingReceiptReason)) > 2000 || len([]rune(l.DuplicateReceiptReason)) > 2000 {
			return fail("lines", "明细须使用不重复标识，描述和说明不可超长")
		}
		ids[l.ID] = true
		for _, field := range []**string{&l.Quantity, &l.UnitPrice} {
			if *field != nil {
				v, err := core.ParseDecimal(**field)
				if err != nil {
					return c, nil, nil, nil, err
				}
				text := v.String()
				*field = &text
				if field == &l.Quantity && !v.Positive() {
					return fail("quantity", "数量须大于零")
				}
			}
		}
		for _, field := range []**string{&l.DeclaredAmount, &l.TaxAmount, &l.DiscountAmount} {
			if *field != nil {
				if c.Currency == nil {
					return fail("currency", "填写金额前须明确币种")
				}
				v, err := core.CheckCurrencyAmount(*c.Currency, **field, map[string]int{*c.Currency: exponent})
				if err != nil {
					return c, nil, nil, nil, err
				}
				text := v.String()
				*field = &text
			}
		}
		if l.UnitPrice != nil && c.Currency == nil {
			return fail("currency", "填写单价前须明确币种")
		}
		if l.Supplier != nil {
			if err := normalizeSupplier(l.Supplier); err != nil {
				return c, nil, nil, nil, err
			}
		}
		if l.ExpenseDate != nil && core.Date(*l.ExpenseDate) != nil {
			return fail("expense_date", "费用日期无效")
		}
		if l.ExpenseDate != nil && ((c.PeriodStart != nil && *l.ExpenseDate < *c.PeriodStart) || (c.PeriodEnd != nil && *l.ExpenseDate > *c.PeriodEnd)) {
			return fail("expense_date", "明细日期须在费用期间内")
		}
		if l.ReceiptVersionIDs == nil {
			l.ReceiptVersionIDs = []string{}
		}
		if len(l.ReceiptVersionIDs) > 20 {
			return fail("receipt_version_ids", "每行最多20个凭证")
		}
		for j, id := range l.ReceiptVersionIDs {
			id = strings.ToLower(id)
			l.ReceiptVersionIDs[j] = id
			if !core.ValidID(id) || slices.Contains(l.ReceiptVersionIDs[:j], id) {
				return fail("receipt_version_ids", "凭证须固定有效且不重复的文件版本")
			}
		}
		if !slices.Contains([]string{"", "employee_advance", "company_paid"}, l.Funding) {
			return fail("funding", "请选择员工垫付或公司已付")
		}
		if d.Kind == "purchase" && (l.DeclaredAmount != nil || l.ExpenseDate != nil || l.Funding != "" || len(l.ReceiptVersionIDs) > 0 || l.MissingReceiptReason != "" || l.DuplicateReceiptReason != "") {
			return fail("lines", "采购使用数量单价计算，实际费用凭证在执行阶段登记")
		}
		if d.Kind == "claim" && l.DiscountAmount != nil {
			return fail("discount_amount", "报销金额填写实际折后含税费用，无需再次扣折扣")
		}
		if complete && (l.Description == "" || (d.Kind == "purchase" && (l.Quantity == nil || l.UnitPrice == nil || strings.TrimSpace(l.Unit) == "")) ||
			(d.Kind == "claim" && (l.DeclaredAmount == nil || l.ExpenseDate == nil || l.Category == "" || l.Supplier == nil || l.Funding == "" || (len(l.ReceiptVersionIDs) == 0 && strings.TrimSpace(l.MissingReceiptReason) == "")))) {
			return fail("lines", "送审前须补全明细描述、采购数量单价，或报销日期、类别、供应商、资金来源及凭证说明")
		}
	}
	if len(FileVersionIDs(c)) > 200 {
		return fail("attachments", "单份申请最多引用200个文件版本")
	}
	if d.Kind == "purchase" && (c.PeriodStart != nil || c.PeriodEnd != nil) {
		return fail("period_start", "采购使用预期交付日期")
	}
	if d.Kind == "claim" && (c.ExpectedDeliveryDate != nil || len(c.SupplierCandidates) > 0 || len(c.QuoteVersionIDs) > 0 || c.BudgetReference != "") {
		return fail("content", "报销使用明细供应商和费用凭证")
	}
	previousSuppliers := []Supplier{}
	if previous != nil {
		previousSuppliers = Suppliers(*previous)
	}
	for _, ref := range Suppliers(c) {
		p, _, err := party.ResolveBusinessReference(ctx, db, a, ref.PartyID, ref.RevisionID, complete || !slices.Contains(previousSuppliers, ref))
		if err != nil {
			return c, nil, nil, nil, err
		}
		var adequate bool
		if err = db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", p.Classification, d.Classification).Scan(&adequate); err != nil {
			return c, nil, nil, nil, err
		}
		if !adequate || (p.ProjectID != nil && (d.ProjectID == nil || *p.ProjectID != *d.ProjectID)) {
			return fail("supplier", "供应商范围和资料分类须匹配申请")
		}
	}
	if _, _, err := References(ctx, db, a, d, c, complete, true); err != nil {
		return c, nil, nil, nil, err
	}
	values, amount, employee, err := calculate(d.Kind, c, exponent)
	if err != nil {
		return c, nil, nil, nil, err
	}
	if complete && (c.Category == "" || c.Purpose == "" || c.OwnerID == nil || c.Currency == nil || c.PricingBasis == "" || c.RoundingMode == "" || len(c.Lines) == 0 || amount == nil ||
		(d.Kind == "purchase" && (len(c.SupplierCandidates) == 0 || c.ExpectedDeliveryDate == nil)) || (d.Kind == "claim" && (c.PeriodStart == nil || c.PeriodEnd == nil))) {
		return fail("content", "送审前须补全类别、用途、负责人、币种、计价规则、明细及供应商交付信息或费用期间")
	}
	return c, values, amount, employee, nil
}

// The exact quantity-price product is rounded once to the selected currency.
// Claim tax is included in the declared amount, never added a second time.
func calculate(kind string, c Content, exponent int) ([]LineValue, *string, *string, error) {
	values := []LineValue{}
	total, employee := core.Decimal{}, core.Decimal{}
	known := len(c.Lines) > 0
	fundingKnown := true
	for _, l := range c.Lines {
		value := LineValue{LineID: l.ID}
		var amount *core.Decimal
		if kind == "purchase" && l.Quantity != nil && l.UnitPrice != nil && c.Currency != nil && c.RoundingMode != "" && c.PricingBasis != "" {
			v, err := core.QuantityPrice(*l.Quantity, *l.UnitPrice, exponent, c.RoundingMode)
			if err != nil {
				return nil, nil, nil, err
			}
			tax, discount := core.Decimal{}, core.Decimal{}
			if l.TaxAmount != nil {
				tax, _ = core.ParseDecimal(*l.TaxAmount)
			}
			if l.DiscountAmount != nil {
				discount, _ = core.ParseDecimal(*l.DiscountAmount)
			}
			if c.PricingBasis == "net_plus_tax" {
				v = v.Add(tax)
			} else if tax.Cmp(v) > 0 {
				return nil, nil, nil, core.Invalid("tax_amount", "含税税额不能超过行总额")
			}
			if discount.Cmp(v) > 0 {
				return nil, nil, nil, core.Invalid("discount_amount", "折扣不能超过行总额")
			}
			v = v.Sub(discount)
			if tax.Cmp(v) > 0 {
				return nil, nil, nil, core.Invalid("tax_amount", "列示税额不能超过折扣后的行总额")
			}
			amount = &v
		} else if kind == "claim" && l.DeclaredAmount != nil {
			v, err := core.ParseDecimal(*l.DeclaredAmount)
			if err != nil {
				return nil, nil, nil, err
			}
			amount = &v
			if l.TaxAmount != nil {
				tax, _ := core.ParseDecimal(*l.TaxAmount)
				if tax.Cmp(v) > 0 {
					return nil, nil, nil, core.Invalid("tax_amount", "报销税额不能超过含税费用")
				}
			}
			if (l.Quantity == nil) != (l.UnitPrice == nil) {
				return nil, nil, nil, core.Invalid("unit_price", "报销的说明性数量与单价须同时填写或同时留空")
			}
			if l.Quantity != nil && c.RoundingMode != "" {
				product, e := core.QuantityPrice(*l.Quantity, *l.UnitPrice, exponent, c.RoundingMode)
				if e != nil {
					return nil, nil, nil, e
				}
				if product.Cmp(v) != 0 {
					return nil, nil, nil, core.Invalid("declared_amount", "数量单价与报销含税费用不一致")
				}
			}
		}
		if amount != nil {
			text := amount.String()
			if _, err := core.ParseDecimal(text); err != nil {
				return nil, nil, nil, err
			}
			value.Amount = &text
			total = total.Add(*amount)
			if l.Funding == "employee_advance" {
				employee = employee.Add(*amount)
			}
		} else {
			known = false
		}
		if l.Funding == "" {
			fundingKnown = false
		}
		values = append(values, value)
	}
	var amountText, employeeText *string
	if known {
		text := total.String()
		if _, err := core.ParseDecimal(text); err != nil {
			return nil, nil, nil, core.Invalid("amount", "明细合计超过金额范围")
		}
		amountText = &text
		if kind == "claim" && fundingKnown {
			text := employee.String()
			employeeText = &text
		}
	}
	return values, amountText, employeeText, nil
}
func contentHash(d Record, c Content, values []LineValue, amount, employee *string) (string, error) {
	raw, err := json.Marshal(map[string]any{"schema": 1, "kind": d.Kind, "document_id": d.ID, "project_id": d.ProjectID, "classification": d.Classification, "content": c, "line_values": values, "amount": amount, "employee_reimbursement_amount": employee})
	if err != nil {
		return "", err
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	return core.Digest(canonical), nil
}
