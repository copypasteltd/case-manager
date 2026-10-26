package telegram

import (
	"copypaste.hk/platform/internal/core"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// The dispatcher receives identities only, never ingress or outbound secrets.
type DispatchBot struct {
	BotID string `json:"bot_id"`
	OrgID string `json:"organization_id"`
}

func LoadDispatchBots() ([]DispatchBot, error) {
	raw := os.Getenv("CP_TELEGRAM_DISPATCH_BOTS")
	if raw == "" || raw == "disabled" {
		return nil, nil
	}
	if len(raw) > 8192 {
		return nil, errors.New("Telegram dispatch configuration exceeds its size limit")
	}
	var bots []DispatchBot
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bots) != nil || len(bots) < 1 || len(bots) > 8 {
		return nil, errors.New("Telegram dispatch requires one to eight explicit bot identities")
	}
	seen := map[string]bool{}
	for _, b := range bots {
		if !numericID(b.BotID, false) || !core.ValidID(b.OrgID) || b.OrgID != strings.ToLower(b.OrgID) || seen[b.BotID] {
			return nil, errors.New("invalid or duplicate Telegram dispatch bot identity")
		}
		seen[b.BotID] = true
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("Telegram dispatch configuration must contain one array")
	}
	return bots, nil
}
