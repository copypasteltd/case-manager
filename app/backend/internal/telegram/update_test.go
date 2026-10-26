package telegram

import (
	"encoding/json"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestTelegramUpdateProtocol(t *testing.T) {
	bot := BotConfig{ID: "123456789", OrgID: core.NewID(), Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("s", 32)}
	t.Run("numeric identities never round or accept alternate spellings", func(t *testing.T) {
		for _, s := range []string{"0", "01", "+1", "1.0", "1e2", "4503599627370496", "-1"} {
			if numericID(s, false) {
				t.Fatalf("accepted noncanonical identity %q", s)
			}
		}
		if !numericID("4503599627370495", false) || !numericID("-1009876543210", true) {
			t.Fatal("valid 52-bit identities rejected")
		}
	})
	t.Run("ambiguous and multiply rooted updates are refused", func(t *testing.T) {
		for _, raw := range []string{`{"update_id":null}`, `{"update_id":1,"update_id":2}`, `{"update_id":1} {}`, `{"update_id":1,"message":{"chat":{"id":1,"id":2}}}`, `{"update_id":1,"message":{},"edited_message":{}}`, `{"update_id":1.5}`, `{"update_id":"1"}`, `{"update_id":-1}`} {
			if _, _, err := parseUpdate([]byte(raw)); err == nil {
				t.Fatal("ambiguous update accepted")
			}
		}
		if _, _, err := parseUpdate([]byte(`{"update_id":9007199254740991,"unknown_future_update":{"a":true}}`)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("own UTF16 mention and explicit reply trigger, arbitrary substrings do not", func(t *testing.T) {
		m := message{Text: "😀 @CopyPasteTestBot 请核对", Entities: []entity{{Type: "mention", Offset: 3, Length: 17}}}
		if !triggered(m, bot) {
			t.Fatal("valid mention after surrogate pair rejected")
		}
		m.Entities[0].Offset = 2
		if triggered(m, bot) {
			t.Fatal("misaligned mention accepted")
		}
		m = message{Text: "请访问example/@CopyPasteTestBot"}
		if triggered(m, bot) {
			t.Fatal("untyped substring triggered")
		}
		m = message{Text: "/ask@OtherBot 查阅", Entities: []entity{{Type: "bot_command", Offset: 0, Length: 13}}}
		if triggered(m, bot) {
			t.Fatal("another bot command triggered")
		}
		m = message{Text: "/brief", Entities: []entity{{Type: "bot_command", Offset: 0, Length: 6}}}
		if !triggered(m, bot) {
			t.Fatal("explicit command rejected")
		}
		raw := `{"reply_to_message":{"from":{"id":123456789,"is_bot":true}}}`
		if err := json.Unmarshal([]byte(raw), &m); err != nil || !triggered(m, bot) {
			t.Fatal("actual bot reply not recognized")
		}
	})
	t.Run("binding proof requires an actual start command entity", func(t *testing.T) {
		m := message{Text: "/start secret", Entities: []entity{{Type: "bot_command", Offset: 0, Length: 6}}}
		if code, ok := startCode(m, bot); !ok || code != "secret" {
			t.Fatal("explicit start rejected")
		}
		m.Entities = nil
		if _, ok := startCode(m, bot); ok {
			t.Fatal("untyped copied code accepted")
		}
	})
}
func TestTelegramIngressConfiguration(t *testing.T) {
	t.Setenv("CP_TELEGRAM_INGRESS", "")
	if v, e := LoadBots(); e != nil || len(v) != 0 {
		t.Fatal("ingress enabled implicitly")
	}
	good := BotConfig{ID: "123456789", OrgID: core.NewID(), Username: "CopyPasteTestBot", WebhookSecret: strings.Repeat("s", 32)}
	if err := ValidateBots([]BotConfig{good}); err != nil {
		t.Fatal(err)
	}
	if ValidateBots([]BotConfig{good, good}) == nil {
		t.Fatal("ambiguous bot identity accepted")
	}
	for _, change := range []func(*BotConfig){func(b *BotConfig) { b.ID = "01" }, func(b *BotConfig) { b.Username = "not_a_bot_name" }, func(b *BotConfig) { b.WebhookSecret = "short" }, func(b *BotConfig) { b.OrgID = "unknown" }} {
		bad := good
		change(&bad)
		if ValidateBots([]BotConfig{bad}) == nil {
			t.Fatal("invalid ingress configuration accepted")
		}
	}
	raw, _ := json.Marshal([]BotConfig{good})
	t.Setenv("CP_TELEGRAM_INGRESS", string(raw)+" trailing")
	if _, e := LoadBots(); e == nil {
		t.Fatal("trailing configuration accepted")
	}
}
