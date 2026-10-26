package telegram

import (
	"strings"
	"testing"
)

func TestTelegramTaskSelection(t *testing.T) {
	bot := BotConfig{ID: "123456789", Username: "CompanyBot"}
	defaults := "incident-summary"
	cases := []struct {
		name, text string
		skills     []string
		defaultID  *string
		want       string
		ok         bool
	}{
		{"explicit briefing", "/brief 项目简报", []string{"project-briefing", "incident-summary"}, &defaults, "project-briefing", true},
		{"explicit incident", "/incident 事件摘要", []string{"project-briefing", "incident-summary"}, nil, "incident-summary", true},
		{"configured ask default", "/ask 核对情况", []string{"project-briefing", "incident-summary"}, &defaults, "incident-summary", true},
		{"multiple tasks never select by order", "/ask 核对情况", []string{"project-briefing", "incident-summary"}, nil, "", false},
		{"sole permitted task", "/ask 核对情况", []string{"project-briefing"}, nil, "project-briefing", true},
		{"explicit task cannot fall back to default", "/incident 核对事件", []string{"project-briefing"}, nil, "incident-summary", false},
		{"command target is case insensitive", "/incident@COMPANYBOT 核对事件", []string{"incident-summary"}, nil, "incident-summary", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			command := strings.Fields(c.text)[0]
			m := message{Text: c.text, Entities: []entity{{Type: "bot_command", Offset: 0, Length: len(command)}}}
			skill, ok := requestSkill(m, bot, Channel{AllowedSkillIDs: c.skills, DefaultSkillID: c.defaultID})
			if !triggered(m, bot) || skill != c.want || ok != c.ok {
				t.Fatal("task selection did not preserve the explicit intent")
			}
		})
	}
}

func TestTelegramDispatchConfiguration(t *testing.T) {
	for _, raw := range []string{"", "disabled"} {
		t.Setenv("CP_TELEGRAM_DISPATCH_BOTS", raw)
		bots, err := LoadDispatchBots()
		if err != nil || len(bots) != 0 {
			t.Fatal("disabled dispatch was enabled")
		}
	}
	identity := `{"bot_id":"123456789","organization_id":"00000000-0000-4000-8000-000000000001"}`
	t.Setenv("CP_TELEGRAM_DISPATCH_BOTS", "["+identity+"]")
	bots, err := LoadDispatchBots()
	if err != nil || len(bots) != 1 {
		t.Fatal("valid identity-only dispatch configuration rejected")
	}
	for _, raw := range []string{"[]", "null", "[" + identity + "," + identity + "]", "[" + identity + "] {}", `[{"bot_id":"0123","organization_id":"00000000-0000-4000-8000-000000000001"}]`, strings.Replace("["+identity+"]", `"bot_id"`, `"webhook_secret":"must-not-be-used","bot_id"`, 1), strings.Repeat(" ", 8193)} {
		t.Setenv("CP_TELEGRAM_DISPATCH_BOTS", raw)
		if _, err = LoadDispatchBots(); err == nil {
			t.Fatal("invalid or secret-bearing dispatch configuration accepted")
		}
	}
}
