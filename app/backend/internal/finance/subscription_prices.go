package finance

import (
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
)

type SubscriptionPriceInput struct {
	EffectiveFrom string  `json:"effective_from"`
	EffectiveTo   *string `json:"effective_to"`
	Currency      string  `json:"currency"`
	Amount        string  `json:"amount"`
	Plan          string  `json:"plan"`
}

// Each configuration revision owns its complete price timeline. Splitting an
// interval creates a new revision; previously generated periods pin old rows.
func subscriptionPrices(prices []SubscriptionPriceInput, currencies map[string]int) ([]SubscriptionPriceInput, error) {
	if len(prices) > 1000 {
		return nil, core.Invalid("prices", "一个订阅修订最多包含1000个价格区间")
	}
	out := append([]SubscriptionPriceInput{}, prices...)
	slices.SortFunc(out, func(a, b SubscriptionPriceInput) int { return strings.Compare(a.EffectiveFrom, b.EffectiveFrom) })
	for i := range out {
		p := &out[i]
		if _, err := civilDate(p.EffectiveFrom); err != nil {
			return nil, core.Invalid("effective_from", "请填写价格生效日期")
		}
		if p.EffectiveTo != nil {
			if _, err := civilDate(*p.EffectiveTo); err != nil || *p.EffectiveTo <= p.EffectiveFrom {
				return nil, core.Invalid("effective_to", "价格结束日不包含在区间内，必须晚于生效日")
			}
		}
		if i > 0 && (out[i-1].EffectiveTo == nil || *out[i-1].EffectiveTo > p.EffectiveFrom) {
			return nil, core.Invalid("prices", "同一价格修订中的生效区间不得重叠")
		}
		amount, err := core.CheckCurrencyAmount(p.Currency, p.Amount, currencies)
		if err != nil {
			return nil, err
		}
		p.Amount = amount.String()
		p.Plan = strings.TrimSpace(p.Plan)
		if len([]rune(p.Plan)) > 200 {
			return nil, core.Invalid("plan", "套餐说明最多200字符")
		}
	}
	return out, nil
}

func subscriptionPriceAt(prices []SubscriptionPriceInput, date string) *SubscriptionPriceInput {
	for _, p := range prices {
		if p.EffectiveFrom <= date && (p.EffectiveTo == nil || date < *p.EffectiveTo) {
			copy := p
			return &copy
		}
	}
	return nil
}

func subscriptionAppendPrice(prices []SubscriptionPriceInput, next SubscriptionPriceInput, currencies map[string]int) ([]SubscriptionPriceInput, error) {
	old, err := subscriptionPrices(prices, currencies)
	if err != nil {
		return nil, err
	}
	if len(old) > 0 {
		last := &old[len(old)-1]
		if next.EffectiveFrom <= last.EffectiveFrom {
			return nil, core.Invalid("effective_from", "新价格须晚于最后一个已登记价格的起点，历史更正另存新订阅修订")
		}
		if last.EffectiveTo == nil || *last.EffectiveTo > next.EffectiveFrom {
			date := next.EffectiveFrom
			last.EffectiveTo = &date
		}
	}
	return subscriptionPrices(append(old, next), currencies)
}
