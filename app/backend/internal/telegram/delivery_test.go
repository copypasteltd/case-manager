package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type deliveryRoundTrip func(*http.Request) (*http.Response, error)

func (f deliveryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func isolatedDeliveryBot() DeliveryBot {
	return DeliveryBot{BotID: "123456789", OrgID: "00000000-0000-4000-8000-000000000001", Token: "123456789:" + strings.Repeat("a", 35), PortalOrigin: "https://portal.example.test"}
}
func TestTelegramDeliveryConfiguration(t *testing.T) {
	for _, raw := range []string{"", "disabled"} {
		t.Setenv("CP_TELEGRAM_DELIVERY_BOTS", raw)
		bots, err := LoadDeliveryBots()
		if err != nil || len(bots) != 0 {
			t.Fatal("disabled delivery enabled")
		}
	}
	bot := isolatedDeliveryBot()
	encoded, _ := json.Marshal([]DeliveryBot{bot})
	t.Setenv("CP_TELEGRAM_DELIVERY_BOTS", string(encoded))
	if bots, err := LoadDeliveryBots(); err != nil || len(bots) != 1 {
		t.Fatal("explicit configuration rejected")
	}
	for _, origin := range []string{"http://portal.test", "https://portal.test/", "https://portal.test?", "https://portal.test#", "https://u:p@portal.test", "https://portal.test/x", "https://portal.test?q=1", "https://portal.test/#x", "https://portal.test\\x", "https://portal.test\n"} {
		v := bot
		v.PortalOrigin = origin
		if v.valid() {
			t.Fatal("unsafe portal origin accepted")
		}
	}
	for _, raw := range []string{"[]", "null", string(encoded) + " {}", strings.Replace(string(encoded), "123456789:", "987654321:", 1), strings.Replace(string(encoded), "portal_origin", "webhook_secret", 1), string(encoded[:len(encoded)-1]) + "," + string(encoded[1:]), strings.Repeat(" ", 8193)} {
		t.Setenv("CP_TELEGRAM_DELIVERY_BOTS", raw)
		if _, err := LoadDeliveryBots(); err == nil {
			t.Fatal("invalid delivery configuration accepted")
		}
	}
}
func TestTelegramBotAPIOutcomes(t *testing.T) {
	bot := isolatedDeliveryBot()
	message := portalMessage(bot, "00000000-0000-4000-8000-000000000002", "result", "-1001234567890", 42, 99)
	cases := []struct {
		name                string
		status              int
		body, state, reason string
	}{
		{"confirmed", 200, `{"ok":true,"result":{"message_id":101,"chat":{"id":-1001234567890},"message_thread_id":42,"reply_to_message":{"message_id":99}}}`, "sent", "provider_confirmed"},
		{"wrong chat", 200, `{"ok":true,"result":{"message_id":101,"chat":{"id":-1001},"message_thread_id":42,"reply_to_message":{"message_id":99}}}`, "delivery_unknown", "provider_outcome_unknown"},
		{"wrong topic", 200, `{"ok":true,"result":{"message_id":101,"chat":{"id":-1001234567890},"message_thread_id":41,"reply_to_message":{"message_id":99}}}`, "delivery_unknown", "provider_outcome_unknown"},
		{"missing reply", 200, `{"ok":true,"result":{"message_id":101,"chat":{"id":-1001234567890},"message_thread_id":42}}`, "delivery_unknown", "provider_outcome_unknown"},
		{"malformed", 200, `{"ok":`, "delivery_unknown", "provider_outcome_unknown"},
		{"server error", 500, `{"ok":false,"error_code":500}`, "delivery_unknown", "provider_outcome_unknown"},
		{"redirect", 302, `{}`, "delivery_unknown", "provider_outcome_unknown"},
		{"rate limit", 429, `{"ok":false,"error_code":429,"parameters":{"retry_after":60}}`, "retry_wait", "provider_rate_limited"},
		{"rate no delay", 429, `{"ok":false,"error_code":429}`, "delivery_unknown", "provider_outcome_unknown"},
		{"bad rate delay", 429, `{"ok":false,"error_code":429,"parameters":{"retry_after":90000}}`, "delivery_unknown", "provider_outcome_unknown"},
		{"rejected", 400, `{"ok":false,"error_code":400,"description":"secret echo must never be retained"}`, "failed", "provider_rejected"},
		{"forbidden", 403, `{"ok":false,"error_code":403}`, "failed", "provider_rejected"},
		{"moved chat", 400, `{"ok":false,"error_code":400,"parameters":{"migrate_to_chat_id":-1009876543210}}`, "failed", "chat_migration_requires_review"},
		{"contradictory error", 400, `{"ok":true,"error_code":400}`, "delivery_unknown", "provider_outcome_unknown"},
		{"oversized", 200, strings.Repeat(" ", 65537), "delivery_unknown", "provider_outcome_unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			transport := BotAPITransport{Transport: deliveryRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != "POST" || r.URL.Host != "api.telegram.org" || r.URL.Path != "/bot"+bot.Token+"/sendMessage" || r.URL.RawQuery != "" {
					t.Fatal("outbound target changed")
				}
				var sent OutboundMessage
				if json.NewDecoder(r.Body).Decode(&sent) != nil || sent.ChatID != message.ChatID || sent.TopicID != 42 || sent.Reply.MessageID != 99 || sent.Reply.AllowMissing || !sent.Preview.Disabled || sent.Text != message.Text {
					t.Fatal("request location or disclosure changed")
				}
				return &http.Response{StatusCode: c.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(c.body)), Request: r}, nil
			})}
			out := transport.Send(context.Background(), bot, message)
			if out.State != c.state || out.Reason != c.reason {
				t.Fatalf("unexpected outcome: %s/%s", out.State, out.Reason)
			}
			if c.state == "retry_wait" && out.RetryAfter != time.Minute {
				t.Fatal("retry_after not respected")
			}
		})
	}
	transport := BotAPITransport{Transport: deliveryRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network error containing private data")
	})}
	if out := transport.Send(context.Background(), bot, message); out.State != "delivery_unknown" || strings.Contains(out.Reason, "private") {
		t.Fatal("network uncertainty or redaction failed")
	}
}
