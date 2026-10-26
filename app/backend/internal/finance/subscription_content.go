package finance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func subscriptionCalendar(c SubscriptionContent) (SubscriptionCalendar, error) {
	if c.Cycle == nil || c.FirstChargeDate == nil || c.Timezone == "" {
		return SubscriptionCalendar{}, core.Invalid("calendar", "请补齐周期、账期时区与首次计费日期")
	}
	calendar := SubscriptionCalendar{Cycle: *c.Cycle, Timezone: c.Timezone, FirstChargeDate: *c.FirstChargeDate}
	if c.AnchorDay != nil {
		calendar.AnchorDay = *c.AnchorDay
	}
	if c.CustomIntervalDays != nil {
		calendar.CustomIntervalDays = *c.CustomIntervalDays
	}
	_, err := calendar.validate()
	return calendar, err
}

func normalizeSubscription(ctx context.Context, db platform.DBTX, c SubscriptionContent, complete bool) (SubscriptionContent, error) {
	c.Product = strings.TrimSpace(c.Product)
	c.Category = strings.TrimSpace(c.Category)
	c.PaymentMethodLabel = strings.TrimSpace(c.PaymentMethodLabel)
	if core.Name(c.Product) != nil || len([]rune(c.Category)) > 50 || len([]rune(c.PaymentMethodLabel)) > 100 || len([]rune(c.Notes)) > 20000 {
		return c, core.Invalid("content", "请核对订阅产品、类别、付款方式标签及备注长度")
	}
	if c.Supplier != nil {
		c.Supplier = &PartyRef{PartyID: strings.ToLower(c.Supplier.PartyID), RevisionID: strings.ToLower(c.Supplier.RevisionID)}
		if !core.ValidID(c.Supplier.PartyID) || !core.ValidID(c.Supplier.RevisionID) {
			return c, core.Invalid("supplier", "请固定准确供应方资料修订")
		}
	}
	if c.OwnerID != nil && !core.ValidID(*c.OwnerID) {
		return c, core.Invalid("owner_id", "请指定有效的公司负责人")
	}
	if c.OwnerID != nil {
		id := strings.ToLower(*c.OwnerID)
		c.OwnerID = &id
	}
	for _, date := range []*string{c.StartDate, c.TrialUntil, c.FirstChargeDate} {
		if date != nil {
			if _, err := civilDate(*date); err != nil {
				return c, err
			}
		}
	}
	if c.StartDate != nil && c.FirstChargeDate != nil && *c.StartDate > *c.FirstChargeDate {
		return c, core.Invalid("first_charge_date", "首次计费日期不能早于订阅开始日")
	}
	if c.TrialUntil != nil && ((c.StartDate != nil && *c.TrialUntil < *c.StartDate) || (c.FirstChargeDate != nil && *c.TrialUntil > *c.FirstChargeDate)) {
		return c, core.Invalid("trial_until", "试用截止须在订阅开始与首次计费日期之间")
	}
	if c.Timezone != "" {
		if _, err := SubscriptionToday(time.Now(), c.Timezone); err != nil {
			return c, err
		}
	}
	if c.Cycle != nil && !slices.Contains([]string{"month", "year", "custom"}, *c.Cycle) {
		return c, core.Invalid("cycle", "请选择月、年或自定义自然日周期")
	}
	if c.AnchorDay != nil && (*c.AnchorDay < 1 || *c.AnchorDay > 31) {
		return c, core.Invalid("anchor_day", "原始月锚点须为1至31")
	}
	if c.CustomIntervalDays != nil && (*c.CustomIntervalDays < 1 || *c.CustomIntervalDays > 3660) {
		return c, core.Invalid("custom_interval_days", "自定义周期须为1至3660个自然日")
	}
	if c.Cycle != nil && ((*c.Cycle == "custom" && c.AnchorDay != nil) || (*c.Cycle != "custom" && c.CustomIntervalDays != nil)) {
		return c, core.Invalid("calendar", "月/年周期使用原始锚点，自定义周期使用自然日数")
	}
	calendarPresent := c.Cycle != nil && c.FirstChargeDate != nil && c.Timezone != "" &&
		((*c.Cycle == "custom" && c.CustomIntervalDays != nil) || (*c.Cycle != "custom" && c.AnchorDay != nil))
	if calendarPresent {
		if _, err := subscriptionCalendar(c); err != nil {
			return c, err
		}
	}
	if !slices.Contains([]string{"", "automatic", "manual"}, c.RenewalMode) {
		return c, core.Invalid("renewal_mode", "续费方式须为自动、人工或尚未核对")
	}
	c.ReminderDays = append([]int{}, c.ReminderDays...)
	slices.Sort(c.ReminderDays)
	if len(c.ReminderDays) > 10 {
		return c, core.Invalid("reminder_days", "最多设置10个提前提醒日期")
	}
	for i, days := range c.ReminderDays {
		if days < 0 || days > 90 || (i > 0 && c.ReminderDays[i-1] == days) {
			return c, core.Invalid("reminder_days", "提前提醒为0至90个自然日且不得重复")
		}
	}
	if c.VendorState == "" {
		c.VendorState = "unknown"
	}
	if !slices.Contains([]string{"unknown", "active", "paused", "cancelled"}, c.VendorState) {
		return c, core.Invalid("vendor_state", "请区分未核对与供应方实际状态")
	}
	c.VendorEvidenceVersionIDs = append([]string{}, c.VendorEvidenceVersionIDs...)
	for i := range c.VendorEvidenceVersionIDs {
		c.VendorEvidenceVersionIDs[i] = strings.ToLower(c.VendorEvidenceVersionIDs[i])
	}
	slices.Sort(c.VendorEvidenceVersionIDs)
	if len(c.VendorEvidenceVersionIDs) > 20 {
		return c, core.Invalid("vendor_evidence_version_ids", "供应方状态证据最多20个固定文件版本")
	}
	for i, id := range c.VendorEvidenceVersionIDs {
		if !core.ValidID(id) || (i > 0 && c.VendorEvidenceVersionIDs[i-1] == id) {
			return c, core.Invalid("vendor_evidence_version_ids", "供应方状态证据须为不同的有效固定版本")
		}
	}
	if c.VendorCheckedAt != nil {
		checked, err := time.Parse(time.RFC3339Nano, *c.VendorCheckedAt)
		if err != nil {
			return c, core.Invalid("vendor_checked_at", "请提供含时区的供应方实际核对时间")
		}
		var now time.Time
		if err = db.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return c, err
		}
		if checked.After(now) || len(c.VendorEvidenceVersionIDs) == 0 {
			return c, core.Invalid("vendor_checked_at", "实际核对时间不得在未来，且须有固定文件证据")
		}
		value := checked.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
		c.VendorCheckedAt = &value
	}
	if c.VendorState != "unknown" && (c.VendorCheckedAt == nil || len(c.VendorEvidenceVersionIDs) == 0) {
		return c, core.Invalid("vendor_state", "供应方实际状态须有核对时间与固定证据，内部停订不等于供应方已取消")
	}
	if len(c.VendorEvidenceVersionIDs) > 0 && c.VendorCheckedAt == nil {
		return c, core.Invalid("vendor_checked_at", "请同时记录供应方状态证据的实际核对时间")
	}
	definitions := map[string]int{}
	for _, price := range c.Prices {
		if _, found := definitions[price.Currency]; !found {
			var exponent int
			if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", price.Currency).Scan(&exponent); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return c, core.Invalid("currency", "订阅价格所用币种尚未配置")
				}
				return c, err
			}
			definitions[price.Currency] = exponent
		}
	}
	var err error
	c.Prices, err = subscriptionPrices(c.Prices, definitions)
	if err != nil {
		return c, err
	}
	if complete && (c.Supplier == nil || c.OwnerID == nil || c.Category == "" || c.StartDate == nil || !calendarPresent || c.RenewalMode == "" || subscriptionPriceAt(c.Prices, *c.FirstChargeDate) == nil) {
		return c, core.Invalid("content", "启用前须补齐供应方、负责人、类别、开始日、完整账期、续费方式及首次账期价格")
	}
	return c, nil
}

func subscriptionFixedCalendar(c SubscriptionContent) []byte {
	data, _ := json.Marshal(struct {
		Start, First *string
		Cycle        *string
		Timezone     string
		Anchor, Days *int
	}{c.StartDate, c.FirstChargeDate, c.Cycle, c.Timezone, c.AnchorDay, c.CustomIntervalDays})
	return data
}

func subscriptionReferences(ctx context.Context, db platform.DBTX, a core.Actor, s Subscription, c SubscriptionContent, strict bool) (*PartyEvidence, []fileref.Evidence, *string, bool, error) {
	supplier, files, err := references(ctx, db, a, Cost{ProjectID: s.ProjectID, Classification: s.Classification}, Content{Counterparty: c.Supplier, EvidenceVersionIDs: c.VendorEvidenceVersionIDs}, strict, strict && len(c.VendorEvidenceVersionIDs) > 0)
	if err != nil || c.OwnerID == nil {
		return supplier, files, nil, false, err
	}
	var name, state string
	err = db.QueryRow(ctx, "SELECT display_name,status FROM users WHERE org_id=$1 AND id=$2", a.OrgID, *c.OwnerID).Scan(&name, &state)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil, false, err
		}
		if strict {
			return nil, nil, nil, false, core.Invalid("owner_id", "该公司负责人不存在或不在当前组织")
		}
		return supplier, files, nil, false, nil
	}
	if strict && state != "active" {
		return nil, nil, nil, false, core.Invalid("owner_id", "请选择当前有效的负责人")
	}
	return supplier, files, &name, state == "active", nil
}
