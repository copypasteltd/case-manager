package notification

import "testing"

func TestContractNotificationRuleRequiresAnExplicitCompanyPolicy(t *testing.T) {
	if err := (ContractRule{}).Validate(); err != nil {
		t.Fatal("disabled rule should not invent a policy", err)
	}
	org := "00000000-0000-4000-8000-000000000001"
	for _, rule := range []ContractRule{{OrgID: org}, {Timezone: "Asia/Hong_Kong", DaysBefore: []int{7}},
		{OrgID: org, Timezone: "Local", DaysBefore: []int{0}}, {OrgID: org, Timezone: "invalid/timezone", DaysBefore: []int{0}},
		{OrgID: org, Timezone: "Asia/Hong_Kong", DaysBefore: []int{7, 7}}, {OrgID: org, Timezone: "UTC", DaysBefore: []int{-1}},
		{OrgID: org, Timezone: "UTC", DaysBefore: []int{366}}} {
		if rule.Validate() == nil {
			t.Fatal("incomplete or ambiguous reminder policy accepted", rule)
		}
	}
	if err := (ContractRule{OrgID: org, Timezone: "Asia/Hong_Kong", DaysBefore: []int{30, 7, 0}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
