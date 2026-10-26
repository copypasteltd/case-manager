package finance

import (
	"reflect"
	"testing"
	"time"
)

func TestSubscriptionCalendar(t *testing.T) {
	t.Run("January 31 recovers its anchor after both February lengths", func(t *testing.T) {
		for _, year := range []string{"2024", "2025"} {
			c := SubscriptionCalendar{Cycle: "month", Timezone: "Asia/Hong_Kong", FirstChargeDate: year + "-01-31", AnchorDay: 31}
			got, err := c.Windows(year+"-01-01", year+"-05-01")
			february := year + "-02-28"
			if year == "2024" {
				february = year + "-02-29"
			}
			want := []SubscriptionWindow{{year + "-01-31", february}, {february, year + "-03-31"}, {year + "-03-31", year + "-04-30"}, {year + "-04-30", year + "-05-31"}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal(got, err)
			}
		}
	})
	t.Run("annual February 29 returns after three capped years", func(t *testing.T) {
		c := SubscriptionCalendar{Cycle: "year", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2024-02-29", AnchorDay: 29}
		for index, want := range []string{"2024-02-29", "2025-02-28", "2026-02-28", "2027-02-28", "2028-02-29"} {
			got, err := c.Window(index)
			if err != nil || got.StartDate != want {
				t.Fatal(got, err)
			}
		}
	})
	t.Run("half open queries and catch up never shift the original anchor", func(t *testing.T) {
		c := SubscriptionCalendar{Cycle: "month", Timezone: "America/New_York", FirstChargeDate: "2000-01-31", AnchorDay: 31}
		got, err := c.Windows("2025-03-31", "2025-05-31")
		if err != nil || !reflect.DeepEqual(got, []SubscriptionWindow{{"2025-03-31", "2025-04-30"}, {"2025-04-30", "2025-05-31"}}) {
			t.Fatal(got, err)
		}
		before, err := c.Windows("1999-01-01", "1999-02-01")
		if err != nil || len(before) != 0 {
			t.Fatal(before, err)
		}
	})
	t.Run("custom intervals use natural days across daylight saving and long histories", func(t *testing.T) {
		c := SubscriptionCalendar{Cycle: "custom", Timezone: "America/New_York", FirstChargeDate: "0001-01-01", CustomIntervalDays: 1}
		got, err := c.Windows("2025-03-08", "2025-03-11")
		want := []SubscriptionWindow{{"2025-03-08", "2025-03-09"}, {"2025-03-09", "2025-03-10"}, {"2025-03-10", "2025-03-11"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, err)
		}
	})
	t.Run("invalid anchors cycles timezones and unbounded queries are rejected", func(t *testing.T) {
		valid := SubscriptionCalendar{Cycle: "month", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2025-01-31", AnchorDay: 31}
		for _, change := range []func(*SubscriptionCalendar){
			func(c *SubscriptionCalendar) { c.Timezone = "" }, func(c *SubscriptionCalendar) { c.Timezone = "Local" },
			func(c *SubscriptionCalendar) { c.Timezone = "Not/AZone" }, func(c *SubscriptionCalendar) { c.AnchorDay = 30 },
			func(c *SubscriptionCalendar) { c.Cycle = "week" }, func(c *SubscriptionCalendar) { c.CustomIntervalDays = 1 },
			func(c *SubscriptionCalendar) { c.Cycle = "custom"; c.AnchorDay = 0; c.CustomIntervalDays = 0 },
			func(c *SubscriptionCalendar) { c.FirstChargeDate = "2025-02-30" },
		} {
			c := valid
			change(&c)
			if _, err := c.Window(0); err == nil {
				t.Fatal("accepted invalid calendar", c)
			}
		}
		for _, dates := range [][2]string{{"2025-01-01", "2025-01-01"}, {"2025-02-01", "2025-01-01"}, {"2025-01-01", "2027-01-01"}} {
			if _, err := valid.Windows(dates[0], dates[1]); err == nil {
				t.Fatal("accepted invalid range", dates)
			}
		}
		if _, err := valid.Window(-1); err == nil {
			t.Fatal("accepted negative period")
		}
	})
	t.Run("today uses the declared billing timezone instead of the server timezone", func(t *testing.T) {
		now := time.Date(2025, 3, 1, 1, 30, 0, 0, time.UTC)
		for zone, want := range map[string]string{"Asia/Hong_Kong": "2025-03-01", "America/Los_Angeles": "2025-02-28"} {
			got, err := SubscriptionToday(now, zone)
			if err != nil || got != want {
				t.Fatal(zone, got, err)
			}
		}
	})
	t.Run("next charge supports long custom cycles and the original month anchor", func(t *testing.T) {
		for _, tc := range []struct {
			calendar   SubscriptionCalendar
			from, want string
		}{
			{SubscriptionCalendar{Cycle: "month", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2025-01-31", AnchorDay: 31}, "2025-03-01", "2025-03-31"},
			{SubscriptionCalendar{Cycle: "year", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2024-02-29", AnchorDay: 29}, "2025-03-01", "2026-02-28"},
			{SubscriptionCalendar{Cycle: "custom", Timezone: "Asia/Hong_Kong", FirstChargeDate: "2025-01-31", CustomIntervalDays: 3660}, "2025-02-01", "2035-02-08"},
		} {
			got, e := tc.calendar.Next(tc.from)
			if e != nil || got.StartDate != tc.want {
				t.Fatal(tc, got, e)
			}
		}
	})
}
