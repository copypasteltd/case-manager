package telegram

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"copypaste.hk/platform/internal/core"
)

// DeliveryBot belongs only to worker-telegram-delivery. Never log this value.
type DeliveryBot struct {
	BotID        string `json:"bot_id"`
	OrgID        string `json:"organization_id"`
	Token        string `json:"token"`
	PortalOrigin string `json:"portal_origin"`
}

var deliveryTokenPattern = regexp.MustCompile(`^[1-9][0-9]{0,15}:[A-Za-z0-9_-]{30,128}$`)

// Valid lets the independent business-notice worker enforce the same fixed
// Bot identity and HTTPS portal boundary without persisting provider secrets.
func (b DeliveryBot) Valid() bool { return b.valid() }

func (b DeliveryBot) valid() bool {
	u, err := url.Parse(b.PortalOrigin)
	return numericID(b.BotID, false) && core.ValidID(b.OrgID) && b.OrgID == strings.ToLower(b.OrgID) &&
		deliveryTokenPattern.MatchString(b.Token) && strings.SplitN(b.Token, ":", 2)[0] == b.BotID &&
		err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		u.Path == "" && u.Opaque == "" && u.RawPath == "" && b.PortalOrigin == "https://"+u.Host && !strings.ContainsAny(b.PortalOrigin, "\r\n\\")
}
func LoadDeliveryBots() ([]DeliveryBot, error) {
	raw := os.Getenv("CP_TELEGRAM_DELIVERY_BOTS")
	if raw == "" || raw == "disabled" {
		return nil, nil
	}
	failure := errors.New("Telegram delivery requires one to eight distinct explicit bots and HTTPS portal origins")
	if len(raw) > 8192 {
		return nil, failure
	}
	var bots []DeliveryBot
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bots) != nil || len(bots) < 1 || len(bots) > 8 {
		return nil, failure
	}
	seen := map[string]bool{}
	for _, b := range bots {
		if !b.valid() || seen[b.BotID] {
			return nil, failure
		}
		seen[b.BotID] = true
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, failure
	}
	return bots, nil
}
