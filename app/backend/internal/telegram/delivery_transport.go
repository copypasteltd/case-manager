package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

type OutboundMessage struct {
	ChatID  string `json:"chat_id"`
	TopicID int64  `json:"message_thread_id,omitempty"`
	Text    string `json:"text"`
	Reply   struct {
		MessageID    int64 `json:"message_id"`
		AllowMissing bool  `json:"allow_sending_without_reply"`
	} `json:"reply_parameters"`
	Preview struct {
		Disabled bool `json:"is_disabled"`
	} `json:"link_preview_options"`
}
type DeliveryOutcome struct {
	State, Reason string
	MessageID     *int64
	RetryAfter    time.Duration
}
type DeliveryTransport interface {
	Send(context.Context, DeliveryBot, OutboundMessage) DeliveryOutcome
}

// A fixed endpoint and disabled redirects keep bot tokens out of arbitrary URLs.
// Transport injection permits local httptest coverage without a real Bot call.
type BotAPITransport struct{ Transport http.RoundTripper }

func (b BotAPITransport) Send(ctx context.Context, bot DeliveryBot, message OutboundMessage) DeliveryOutcome {
	unknown := DeliveryOutcome{State: "delivery_unknown", Reason: "provider_outcome_unknown"}
	if !bot.valid() {
		return DeliveryOutcome{State: "failed", Reason: "delivery_configuration_invalid"}
	}
	body, err := json.Marshal(message)
	if err != nil {
		return DeliveryOutcome{State: "failed", Reason: "payload_invalid"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+bot.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return DeliveryOutcome{State: "failed", Reason: "payload_invalid"}
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: b.Transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return unknown
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return unknown
	}
	var result struct {
		OK        bool `json:"ok"`
		ErrorCode int  `json:"error_code"`
		Result    struct {
			MessageID int64 `json:"message_id"`
			TopicID   int64 `json:"message_thread_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Reply struct {
				MessageID int64 `json:"message_id"`
			} `json:"reply_to_message"`
		} `json:"result"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
			MigrateTo  int64 `json:"migrate_to_chat_id"`
		} `json:"parameters"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return unknown
	}
	if response.StatusCode == 200 && result.OK && result.Result.MessageID > 0 && result.Result.MessageID <= 9007199254740991 &&
		strconv.FormatInt(result.Result.Chat.ID, 10) == message.ChatID && result.Result.TopicID == message.TopicID && result.Result.Reply.MessageID == message.Reply.MessageID {
		return DeliveryOutcome{State: "sent", Reason: "provider_confirmed", MessageID: &result.Result.MessageID}
	}
	if result.OK || response.StatusCode < 400 || response.StatusCode >= 500 || result.ErrorCode != response.StatusCode {
		return unknown
	}
	if result.Parameters.MigrateTo != 0 {
		return DeliveryOutcome{State: "failed", Reason: "chat_migration_requires_review"}
	}
	if response.StatusCode == 429 && result.Parameters.RetryAfter > 0 && result.Parameters.RetryAfter <= 86400 {
		return DeliveryOutcome{State: "retry_wait", Reason: "provider_rate_limited", RetryAfter: time.Duration(result.Parameters.RetryAfter) * time.Second}
	}
	if response.StatusCode == 400 || response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 404 {
		return DeliveryOutcome{State: "failed", Reason: "provider_rejected"}
	}
	return unknown
}
