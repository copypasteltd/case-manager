package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type personalNoticeRoundTripper func(*http.Request) (*http.Response, error)

func (f personalNoticeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPersonalNoticeTransport(t *testing.T) {
	bot := DeliveryBot{BotID: "123456789", OrgID: "00000000-0000-0000-0000-000000000001", Token: "123456789:" + strings.Repeat("T", 40), PortalOrigin: "https://portal.example.invalid"}
	message := OutboundMessage{ChatID: "987654322", Text: "您有新的公司内部通知，请登录查看。\nhttps://portal.example.invalid/notifications"}
	message.Preview.Disabled = true
	called := false
	transport := BotAPITransport{Transport: personalNoticeRoundTripper(func(r *http.Request) (*http.Response, error) {
		called = true
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["reply_parameters"]; ok {
			t.Fatal("personal notice must not contain a reply parameter")
		}
		if r.URL.Host != "api.telegram.org" || body["chat_id"] != "987654322" {
			t.Fatal("transport destination changed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":9001,"chat":{"id":987654322}}}`)), Header: http.Header{}}, nil
	})}
	outcome := transport.Send(context.Background(), bot, message)
	if !called || outcome.State != "sent" || outcome.MessageID == nil || *outcome.MessageID != 9001 {
		t.Fatal("valid direct-chat receipt rejected", outcome.State)
	}
}
