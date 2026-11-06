package agentworker

import (
	"copypaste.hk/platform/internal/agent"
	"testing"
)

func TestRound162TelegramProjectCatalog(t *testing.T) {
	defs := scopedToolDefinitions("project-update", "telegram-authorized")
	if len(defs) != 18 {
		t.Fatal("complete controlled project catalog required", len(defs))
	}
	seen := map[string]bool{}
	for _, raw := range defs {
		item := raw.(map[string]any)
		name := item["name"].(string)
		if seen[name] || !agent.TelegramProjectTool(name) {
			t.Fatal("unexpected or duplicate tool", name)
		}
		seen[name] = true
	}
	for _, skill := range []string{"file-update", "oa-assist", "information-organize", "project-create", "project-briefing", "subscription-review", "incident-summary"} {
		if len(scopedToolDefinitions(skill, "telegram-authorized")) != 0 {
			t.Fatal("another Skill inherited a project grant", skill)
		}
	}
	if agent.TelegramProjectTool("accept_deliverable") || agent.TelegramProjectTool("send_notification") {
		t.Fatal("catalog widened acceptance or recipient authority")
	}
}
