package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Real native PG/controller commands; the native confinement proof is a fixture,
// so this establishes neither a real model execution nor Linux confinement.
func round161Start(t *testing.T, f *fixture, run string) (*agent.Service, agent.ExecutionLease) {
	t.Helper()
	ctx := context.Background()
	s := &agent.Service{DB: f.api.DB}
	job, e := s.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "round161.fixed", agent.ExecutionLeaseDuration)
	if e != nil || job == nil {
		t.Fatal("no current queued execution", e)
	}
	x, e := s.IssueExecution(ctx, *job)
	if e != nil {
		t.Fatal(e)
	}
	if x.RunID != run {
		t.Fatal("claimed another run")
	}
	l, e := s.LeaseForCredential(ctx, x.Credential)
	if e != nil {
		t.Fatal(e)
	}
	l.Job = *job
	if e = s.RecordVerifiedConfinement(ctx, l, x.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if e = s.StartExecution(ctx, l); e != nil {
		t.Fatal(e)
	}
	return s, l
}

func round161Diagnostics(t *testing.T, f *fixture) {
	var diagnostic bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostic, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostic.String())
		}
	})
}
