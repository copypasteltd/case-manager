package agentworker

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestIncidentSummaryMCPBoundary(t *testing.T) {
	expected := []string{"read_resource", "search_resources", "inspect_operation", "read_service_summary"}
	for _, channel := range []string{"web", "telegram"} {
		defs := scopedToolDefinitions("incident-summary", channel)
		names := []string{}
		for _, raw := range defs {
			d := raw.(map[string]any)
			names = append(names, d["name"].(string))
			if !d["annotations"].(map[string]any)["readOnlyHint"].(bool) {
				t.Fatal("incident tool is writable")
			}
			schema, _ := json.Marshal(d["inputSchema"])
			if len(schema) == 0 {
				t.Fatal("missing controlled schema")
			}
		}
		if !slices.Equal(names, expected) {
			t.Fatalf("%s exposed %v", channel, names)
		}
	}
	if len(scopedToolDefinitions("file-update", "telegram")) != 0 {
		t.Fatal("Telegram received a write skill")
	}
}

func TestSubscriptionReviewMCPBoundary(t *testing.T) {
	names := []string{}
	for _, raw := range scopedToolDefinitions("subscription-review", "web") {
		tool := raw.(map[string]any)
		names = append(names, tool["name"].(string))
		if !tool["annotations"].(map[string]any)["readOnlyHint"].(bool) {
			t.Fatal("finance review exposes a write")
		}
	}
	if !slices.Equal(names, []string{"read_resource", "inspect_operation", "read_finance_summary", "list_subscriptions", "read_subscription_review", "read_finance_record"}) {
		t.Fatal(names)
	}
	if len(scopedToolDefinitions("subscription-review", "telegram")) != 0 {
		t.Fatal("finance review reached the shareholder group without a channel grant")
	}
}

func TestOAProseMCPBoundary(t *testing.T) {
	names := []string{}
	for _, raw := range scopedToolDefinitions("oa-assist", "web") {
		tool := raw.(map[string]any)
		name := tool["name"].(string)
		names = append(names, name)
		if name == "draft_business_text" && tool["annotations"].(map[string]any)["readOnlyHint"].(bool) {
			t.Fatal("proposal tool falsely declared read only")
		}
	}
	if !slices.Equal(names, []string{"read_resource", "inspect_operation", "draft_business_text", "draft_request", "submit_request"}) {
		t.Fatal(names)
	}
	if len(scopedToolDefinitions("oa-assist", "telegram")) != 0 {
		t.Fatal("OA body write exposed to Telegram without channel authorization")
	}
}
