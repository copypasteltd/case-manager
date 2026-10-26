package finance

import (
	"time"

	"copypaste.hk/platform/internal/core"
	_ "time/tzdata"
)

// Dates are civil billing dates, not a fabricated time of a bank debit. Calendar
// arithmetic uses UTC dates so daylight saving changes cannot shift an anchor.
type SubscriptionCalendar struct {
	Cycle              string `json:"cycle"`
	Timezone           string `json:"timezone"`
	FirstChargeDate    string `json:"first_charge_date"`
	AnchorDay          int    `json:"anchor_day"`
	CustomIntervalDays int    `json:"custom_interval_days"`
}

type SubscriptionWindow struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func civilDate(value string) (time.Time, error) {
	v, err := time.Parse(time.DateOnly, value)
	if err != nil || v.Year() < 1 || v.Year() > 9999 {
		return time.Time{}, core.Invalid("date", "请填写有效的账期日期")
	}
	return v, nil
}

func calendarDay(year int, month time.Month, anchor int) (time.Time, error) {
	if year < 1 || year > 9999 {
		return time.Time{}, core.Invalid("date", "账期超过支持的日期范围")
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if anchor > last {
		anchor = last
	}
	return time.Date(year, month, anchor, 0, 0, 0, 0, time.UTC), nil
}

func (c SubscriptionCalendar) validate() (time.Time, error) {
	first, err := civilDate(c.FirstChargeDate)
	if err != nil {
		return first, err
	}
	if _, err = time.LoadLocation(c.Timezone); c.Timezone == "" || err != nil || c.Timezone == "Local" {
		return first, core.Invalid("timezone", "请明确选择有效的账期时区")
	}
	if c.Cycle == "custom" {
		if c.AnchorDay != 0 || c.CustomIntervalDays < 1 || c.CustomIntervalDays > 3660 {
			return first, core.Invalid("custom_interval_days", "自定义周期须为1至3660个自然日，不另填月锚点")
		}
		return first, nil
	}
	if (c.Cycle != "month" && c.Cycle != "year") || c.AnchorDay < 1 || c.AnchorDay > 31 || c.CustomIntervalDays != 0 {
		return first, core.Invalid("cycle", "请选择月/年周期及原始锚点，或明确自定义自然日周期")
	}
	expected, err := calendarDay(first.Year(), first.Month(), c.AnchorDay)
	if err != nil || !expected.Equal(first) {
		return first, core.Invalid("first_charge_date", "首次计费日期须符合原始锚点；月底允许截到当月末")
	}
	return first, nil
}

func (c SubscriptionCalendar) anchor(first time.Time, index int) (time.Time, error) {
	if index < 0 || index > 3652059 {
		return time.Time{}, core.Invalid("date", "账期序号超出支持范围")
	}
	switch c.Cycle {
	case "month":
		month := int(first.Month()) - 1 + index
		return calendarDay(first.Year()+month/12, time.Month(month%12+1), c.AnchorDay)
	case "year":
		return calendarDay(first.Year()+index, first.Month(), c.AnchorDay)
	default:
		// Avoid time.Duration's roughly 292-year ceiling. A civil date and its
		// natural-day distance remain exact throughout years 1 through 9999.
		days := int64(index) * int64(c.CustomIntervalDays)
		if days > 3652058 {
			return time.Time{}, core.Invalid("date", "账期超出支持范围")
		}
		next := first.AddDate(0, 0, int(days))
		if next.Year() > 9999 {
			return time.Time{}, core.Invalid("date", "账期超出支持范围")
		}
		return next, nil
	}
}

func (c SubscriptionCalendar) Window(index int) (SubscriptionWindow, error) {
	first, err := c.validate()
	if err != nil {
		return SubscriptionWindow{}, err
	}
	start, err := c.anchor(first, index)
	if err != nil {
		return SubscriptionWindow{}, err
	}
	end, err := c.anchor(first, index+1)
	if err != nil {
		return SubscriptionWindow{}, err
	}
	return SubscriptionWindow{start.Format(time.DateOnly), end.Format(time.DateOnly)}, nil
}

// Next finds the first original calendar anchor on or after a civil date. It
// also supports long custom cycles without inventing a one-year forecast.
func (c SubscriptionCalendar) Next(from string) (SubscriptionWindow, error) {
	first, err := c.validate()
	if err != nil {
		return SubscriptionWindow{}, err
	}
	start, err := civilDate(from)
	if err != nil {
		return SubscriptionWindow{}, err
	}
	index := 0
	if start.After(first) {
		switch c.Cycle {
		case "month":
			index = (start.Year()-first.Year())*12 + int(start.Month()) - int(first.Month())
		case "year":
			index = start.Year() - first.Year()
		default:
			index = int((start.Unix() - first.Unix()) / 86400 / int64(c.CustomIntervalDays))
		}
	}
	at, err := c.anchor(first, index)
	if err != nil {
		return SubscriptionWindow{}, err
	}
	if at.Before(start) {
		index++
	}
	return c.Window(index)
}

// Windows selects charge/start dates in [from,to); no result repeats a natural
// period. Every date is computed from the original anchor, never the prior cap.
func (c SubscriptionCalendar) Windows(from, to string) ([]SubscriptionWindow, error) {
	first, err := c.validate()
	if err != nil {
		return nil, err
	}
	start, err := civilDate(from)
	if err != nil {
		return nil, err
	}
	end, err := civilDate(to)
	if err != nil || !end.After(start) || (end.Unix()-start.Unix())/86400 > 366 {
		return nil, core.Invalid("to", "一次选择1至366个自然日，结束日不包含在期间内")
	}
	index := 0
	if start.After(first) {
		switch c.Cycle {
		case "month":
			index = (start.Year()-first.Year())*12 + int(start.Month()) - int(first.Month())
		case "year":
			index = start.Year() - first.Year()
		default:
			index = int((start.Unix() - first.Unix()) / 86400 / int64(c.CustomIntervalDays))
		}
	}
	out := []SubscriptionWindow{}
	for {
		at, e := c.anchor(first, index)
		if e != nil {
			return nil, e
		}
		if !at.Before(end) {
			return out, nil
		}
		if !at.Before(start) {
			window, e := c.Window(index)
			if e != nil {
				return nil, e
			}
			out = append(out, window)
		}
		index++
	}
}

func SubscriptionToday(now time.Time, timezone string) (string, error) {
	zone, err := time.LoadLocation(timezone)
	if timezone == "" || timezone == "Local" || err != nil {
		return "", core.Invalid("timezone", "请明确选择有效的账期时区")
	}
	return now.In(zone).Format(time.DateOnly), nil
}
