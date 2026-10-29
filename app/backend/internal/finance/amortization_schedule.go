package finance

import (
	"math/big"
	"time"

	"copypaste.hk/platform/internal/core"
)

const AmortizationMaxPeriods = 120

// Every source allocation retains its own exact total. Cumulative half-up
// rounding gives deterministic month shares without negative last-month tails.
func amortizationSchedule(c Cost, exponent int, in AmortizationInput) (AmortizationContent, error) {
	out := AmortizationContent{CostID: c.ID, CostVersion: c.Version, CostRevisionID: c.CurrentRevisionID, CostSHA256: c.ContentHash,
		Currency: value(c.Currency), Exponent: exponent, Method: in.Method, Rounding: "allocation_cumulative_half_up",
		ServiceFrom: in.ServiceFrom, ServiceTo: in.ServiceTo, Periods: []AmortizationPeriod{}}
	start, e := time.Parse(time.DateOnly, in.ServiceFrom)
	end, last := time.Parse(time.DateOnly, in.ServiceTo)
	if e != nil || last != nil || start.Year() < 1900 || end.Year() > 2200 || !end.After(start) || end.Sub(start) > 3660*24*time.Hour ||
		(in.Method != "monthly_equal" && in.Method != "daily_weighted") || exponent < 0 || exponent > 6 {
		return out, core.Invalid("service_from", "明确选择均分月份或按服务天数，服务区间为半开日期且不超过120个月")
	}
	total, err := core.CheckCurrencyAmount(out.Currency, value(c.Amount), map[string]int{out.Currency: exponent})
	if err != nil || !total.Positive() || len(c.Allocations) == 0 || len(c.Allocations) > 100 {
		return out, core.Invalid("amount", "摊销须引用有准确原币金额与分摊的已确认费用")
	}
	out.Amount = total.String()
	weights := []int64{}
	var weight int64
	for day := start; day.Before(end); {
		next := time.Date(day.Year(), day.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		if next.After(end) {
			next = end
		}
		w := int64(1)
		if in.Method == "daily_weighted" {
			w = int64(next.Sub(day) / (24 * time.Hour))
		}
		weight += w
		weights = append(weights, w)
		out.Periods = append(out.Periods, AmortizationPeriod{Sequence: len(out.Periods) + 1, ServiceFrom: day.Format(time.DateOnly),
			ServiceTo: next.Format(time.DateOnly), RecognizedOn: next.AddDate(0, 0, -1).Format(time.DateOnly), Amount: "0.000000", Shares: []Allocation{}})
		if len(out.Periods) > AmortizationMaxPeriods {
			return out, core.Invalid("service_to", "最多包含120个服务月份")
		}
		day = next
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	unit := func(text string) (*big.Int, error) {
		d, err := core.CheckCurrencyAmount(out.Currency, text, map[string]int{out.Currency: exponent})
		if err != nil {
			return nil, err
		}
		r, ok := new(big.Rat).SetString(d.String())
		if !ok {
			return nil, core.Invalid("amount", "金额无效")
		}
		r.Mul(r, new(big.Rat).SetInt(factor))
		if !r.IsInt() {
			return nil, core.Invalid("amount", "金额精度无效")
		}
		return new(big.Int).Set(r.Num()), nil
	}
	text := func(n *big.Int) string { return new(big.Rat).SetFrac(n, factor).FloatString(6) }
	sum := core.Decimal{}
	for _, share := range c.Allocations {
		d, err := core.ParseDecimal(share.Amount)
		if err != nil {
			return out, err
		}
		sum = sum.Add(d)
		units, err := unit(share.Amount)
		if err != nil {
			return out, err
		}
		used := new(big.Int)
		var cumulative int64
		for i, w := range weights {
			cumulative += w
			numerator := new(big.Int).Mul(units, big.NewInt(cumulative))
			rounded, rest := new(big.Int), new(big.Int)
			rounded.QuoRem(numerator, big.NewInt(weight), rest)
			if new(big.Int).Lsh(rest, 1).Cmp(big.NewInt(weight)) >= 0 {
				rounded.Add(rounded, big.NewInt(1))
			}
			amount := new(big.Int).Sub(rounded, used)
			used.Set(rounded)
			if amount.Sign() > 0 {
				out.Periods[i].Shares = append(out.Periods[i].Shares, Allocation{ProjectID: share.ProjectID, Amount: text(amount)})
				current, _ := core.ParseDecimal(out.Periods[i].Amount)
				add, _ := core.ParseDecimal(text(amount))
				out.Periods[i].Amount = current.Add(add).String()
			}
		}
	}
	if sum.Cmp(total) != 0 {
		return out, core.Invalid("allocations", "费用分摊合计必须与原额完全一致")
	}
	return out, nil
}
