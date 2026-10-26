package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"copypaste.hk/platform/internal/core"
)

// Ingress deliberately has no Bot API token, transport, or outbound capability.
type BotConfig struct {
	ID            string `json:"bot_id"`
	OrgID         string `json:"organization_id"`
	Username      string `json:"username"`
	WebhookSecret string `json:"webhook_secret"`
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)
var secretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,256}$`)

const maxTelegramID int64 = (1 << 52) - 1

func numericID(value string, negative bool) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && strconv.FormatInt(n, 10) == value && n != 0 && n <= maxTelegramID && n >= -maxTelegramID && (n < 0) == negative
}
func ValidateBots(bots []BotConfig) error {
	if len(bots) > 8 {
		return errors.New("at most eight explicit Telegram ingress configurations are supported")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	secrets := map[string]bool{}
	for _, b := range bots {
		name := strings.ToLower(b.Username)
		if !numericID(b.ID, false) || !core.ValidID(b.OrgID) || b.OrgID != strings.ToLower(b.OrgID) || !usernamePattern.MatchString(b.Username) || !strings.HasSuffix(name, "bot") || !secretPattern.MatchString(b.WebhookSecret) || ids[b.ID] || names[name] || secrets[b.WebhookSecret] {
			return errors.New("Telegram ingress requires distinct numeric bots, organizations, usernames and strong webhook secrets")
		}
		ids[b.ID] = true
		names[name] = true
		secrets[b.WebhookSecret] = true
	}
	return nil
}
func LoadBots() ([]BotConfig, error) {
	raw := os.Getenv("CP_TELEGRAM_INGRESS")
	if raw == "" || raw == "disabled" {
		return nil, nil
	}
	if len(raw) > 8192 {
		return nil, errors.New("Telegram ingress configuration exceeds its size limit")
	}
	var bots []BotConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bots) != nil || bots == nil {
		return nil, errors.New("invalid Telegram ingress configuration")
	}
	if err := ValidateBots(bots); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("Telegram ingress configuration must contain one array")
	}
	return bots, nil
}
func (s *Service) bot(org, id string) (BotConfig, bool) {
	for _, b := range s.Bots {
		if b.OrgID == org && b.ID == id {
			return b, true
		}
	}
	return BotConfig{}, false
}
func (s *Service) WebhookBot(id, secret string) (BotConfig, bool) {
	for _, b := range s.Bots {
		if b.ID == id && subtle.ConstantTimeCompare([]byte(b.WebhookSecret), []byte(secret)) == 1 {
			return b, true
		}
	}
	return BotConfig{}, false
}
