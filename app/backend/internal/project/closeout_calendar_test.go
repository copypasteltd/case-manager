package project

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloseoutCalendarProjection(t *testing.T) {
	var item CloseoutItem
	if err := json.Unmarshal([]byte(`{"kind":"subscriptions","id":"fixture","title":"Fixture","facts":{"calendar":{"timezone":"Pacific/Kiritimati","business_date":"2026-11-06","cancel_effective_on":"2026-11-06","cancellation_effective":true},"private_finance":{"account":"NEVER_EXPOSE"}}}`), &item); err != nil {
		t.Fatal(err)
	}
	if item.SubscriptionCalendar == nil || item.SubscriptionCalendar.Timezone != "Pacific/Kiritimati" || !item.SubscriptionCalendar.CancellationEffective {
		t.Fatal("exact snapshot calendar missing")
	}
	raw, err := json.Marshal(item)
	if err != nil || strings.Contains(string(raw), "NEVER_EXPOSE") || strings.Contains(string(raw), "facts") {
		t.Fatal("raw private facts exposed")
	}
	if err := json.Unmarshal([]byte(`{"kind":"subscriptions","facts":{"legacy":true}}`), &item); err != nil || item.SubscriptionCalendar != nil {
		t.Fatal("legacy snapshot was recomputed or reused")
	}
	if err := json.Unmarshal([]byte(`{"kind":"expenses","facts":{"calendar":{"timezone":"UTC"}}}`), &item); err != nil || item.SubscriptionCalendar != nil {
		t.Fatal("calendar disclosed for another group")
	}
}
