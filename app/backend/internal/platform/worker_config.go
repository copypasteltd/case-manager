package platform

import (
	"errors"
	"net/url"
	"os"

	"copypaste.hk/platform/internal/core"
)

type WorkerConfig struct{ Environment, DatabaseURL, Owner string }

func LoadWorkerConfig() (WorkerConfig, error) {
	c := WorkerConfig{Environment: os.Getenv("CP_ENV"), DatabaseURL: os.Getenv("CP_DATABASE_URL"), Owner: "upload." + core.NewID()}
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.DatabaseURL == "" {
		return c, errors.New("CP_DATABASE_URL is required for the worker")
	}
	if os.Getenv("CP_MIGRATION_DATABASE_URL") != "" {
		return c, errors.New("worker must not receive migration credentials")
	}
	if c.Environment == "production" {
		parsed, err := url.Parse(c.DatabaseURL)
		if err != nil || parsed.Query().Get("sslmode") != "verify-full" {
			return c, errors.New("production worker PostgreSQL requires sslmode=verify-full")
		}
	}
	// No cursor, MFA, browser-session or model secret is read by this process.
	return c, nil
}
