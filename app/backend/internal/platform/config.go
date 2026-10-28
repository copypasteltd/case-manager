package platform

import (
	"copypaste.hk/platform/internal/miniapp"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Miniapp              miniapp.Config
	Environment          string
	HTTPAddr             string
	DatabaseURL          string
	MigrationDatabaseURL string
	PublicOrigin         string
	AllowedOrigins       map[string]bool
	CursorKey            []byte
	MFAKey               []byte
	SecureCookies        bool
	AccessTTL            time.Duration
	RefreshTTL           time.Duration
}

func LoadConfig() (Config, error) {
	c := Config{Environment: os.Getenv("CP_ENV"), HTTPAddr: os.Getenv("CP_HTTP_ADDR"), DatabaseURL: os.Getenv("CP_DATABASE_URL"),
		MigrationDatabaseURL: os.Getenv("CP_MIGRATION_DATABASE_URL"), PublicOrigin: os.Getenv("CP_PUBLIC_ORIGIN"),
		CursorKey: []byte(os.Getenv("CP_CURSOR_KEY")), AllowedOrigins: map[string]bool{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = "127.0.0.1:8080"
	}
	if c.DatabaseURL == "" {
		return c, errors.New("CP_DATABASE_URL is required; application never falls back to local files")
	}
	if len(c.CursorKey) < 32 || strings.HasPrefix(string(c.CursorKey), "GENERATE_") {
		return c, errors.New("CP_CURSOR_KEY must be a random secret of at least 32 bytes")
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("CP_MFA_KEY"))
	if err != nil || len(key) != 32 {
		return c, errors.New("CP_MFA_KEY must encode 32 random bytes")
	}
	c.MFAKey = key
	c.Miniapp.WeChat = miniapp.Profile{AppID: os.Getenv("CP_WECHAT_APP_ID"), Secret: os.Getenv("CP_WECHAT_APP_SECRET")}
	c.Miniapp.Douyin = miniapp.Profile{AppID: os.Getenv("CP_DOUYIN_APP_ID"), Secret: os.Getenv("CP_DOUYIN_APP_SECRET")}
	if configured := os.Getenv("CP_MINIAPP_IDENTITY_KEY"); configured != "" {
		c.Miniapp.IdentityKey, err = base64.StdEncoding.DecodeString(configured)
		if err != nil {
			return c, errors.New("CP_MINIAPP_IDENTITY_KEY must encode 32 random bytes")
		}
	}
	if !c.Miniapp.Valid() {
		return c, errors.New("mini-program configuration requires a valid app ID, server secret and dedicated 32-byte identity key")
	}
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return c, errors.New("CP_PUBLIC_ORIGIN must be an absolute HTTP(S) origin")
	}
	c.SecureCookies = c.Environment == "production"
	if c.SecureCookies && os.Getenv("CP_MIGRATION_DATABASE_URL") != "" {
		return c, errors.New("production API must not receive migration credentials")
	}
	if c.SecureCookies && u.Scheme != "https" {
		return c, errors.New("production public origin must use HTTPS")
	}
	for _, origin := range strings.Split(os.Getenv("CP_ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			c.AllowedOrigins[origin] = true
		}
	}
	c.AllowedOrigins[c.PublicOrigin] = true
	if c.SecureCookies && !strings.Contains(c.DatabaseURL, "sslmode=verify-full") {
		return c, errors.New("production PostgreSQL requires sslmode=verify-full")
	}
	return c, nil
}
