// Package servicestatus aggregates durable results from external monitoring.
// It neither probes arbitrary targets nor grants access to a server shell.
package servicestatus

import (
	"errors"
	"time"
)

type State string

const (
	Operational State = "operational"
	Degraded    State = "degraded"
	Down        State = "down"
	Maintenance State = "maintenance"
	Unknown     State = "unknown"
)

// Thresholds are fixed in each published source revision. The caller must
// supply them explicitly; an unconfigured service never acquires a green state.
type Thresholds struct {
	FailureCount  int   `json:"failure_count"`
	RecoveryCount int   `json:"recovery_count"`
	Interval      int   `json:"interval_seconds"`
	StaleAfter    int   `json:"stale_after_seconds"`
	LatencyMS     int64 `json:"latency_limit_ms"`
}

func (t Thresholds) Validate() error {
	if t.FailureCount < 1 || t.FailureCount > 10 || t.RecoveryCount < 1 || t.RecoveryCount > 10 || t.Interval < 15 || t.Interval > 60 || t.StaleAfter < 3*t.Interval || t.StaleAfter > 180 || t.LatencyMS < 1 || t.LatencyMS > 120000 {
		return errors.New("invalid monitoring thresholds")
	}
	return nil
}

type Projection struct {
	Sequence   int64     `json:"sequence"`
	ObservedAt time.Time `json:"observed_at"`
	ReceivedAt time.Time `json:"received_at"`
	State      State     `json:"state"`
	Failures   int       `json:"failure_streak"`
	Successes  int       `json:"success_streak"`
}

type Sample struct {
	Sequence   int64
	ObservedAt time.Time
	ReceivedAt time.Time
	Healthy    bool
	LatencyMS  *int64
}

// Advance returns an unchanged projection and a reason for valid but late
// samples. The persistence layer must still retain their original facts.
func Advance(t Thresholds, before Projection, sample Sample) (Projection, string, error) {
	if err := t.Validate(); err != nil {
		return before, "", err
	}
	if sample.Sequence < 1 || sample.Sequence > 9007199254740991 || sample.ObservedAt.IsZero() || sample.ReceivedAt.IsZero() || sample.ObservedAt.After(sample.ReceivedAt.Add(15*time.Second)) || (sample.Healthy && sample.LatencyMS == nil) || (sample.LatencyMS != nil && (*sample.LatencyMS < 0 || *sample.LatencyMS > 3600000)) {
		return before, "", errors.New("invalid monitoring sample")
	}
	if sample.Sequence <= before.Sequence {
		return before, "LATE_SEQUENCE", nil
	}
	if !before.ObservedAt.IsZero() && !sample.ObservedAt.After(before.ObservedAt) {
		return before, "LATE_OBSERVATION", nil
	}
	stale := time.Duration(t.StaleAfter) * time.Second
	if sample.ReceivedAt.Sub(sample.ObservedAt) >= stale {
		return before, "STALE_SAMPLE", nil
	}
	if before.Sequence == 0 || sample.Sequence != before.Sequence+1 || sample.ObservedAt.Sub(before.ObservedAt) >= stale || sample.ReceivedAt.Sub(before.ReceivedAt) >= stale {
		before.State, before.Failures, before.Successes = Unknown, 0, 0
	}
	after := before
	after.Sequence, after.ObservedAt, after.ReceivedAt = sample.Sequence, sample.ObservedAt.UTC(), sample.ReceivedAt.UTC()
	switch {
	case !sample.Healthy:
		after.Successes = 0
		after.Failures = min(t.FailureCount, before.Failures+1)
		if before.State == Down || after.Failures >= t.FailureCount {
			after.State = Down
		} else {
			after.State = Degraded
		}
	case *sample.LatencyMS > t.LatencyMS:
		after.Failures, after.Successes = 0, 0
		if before.State != Down {
			after.State = Degraded
		}
	default:
		after.Failures = 0
		after.Successes = min(t.RecoveryCount, before.Successes+1)
		if before.State == Operational || after.Successes >= t.RecoveryCount {
			after.State = Operational
		}
	}
	return after, "", nil
}

func VisibleSource(t Thresholds, enabled bool, p Projection, now time.Time) (State, string) {
	if !enabled {
		return Unknown, "SOURCE_DISABLED"
	}
	if t.Validate() != nil || p.Sequence == 0 || p.ObservedAt.IsZero() || p.ReceivedAt.IsZero() {
		return Unknown, "NO_OBSERVATION"
	}
	limit := time.Duration(t.StaleAfter) * time.Second
	// A slightly future client timestamp never extends freshness beyond receipt.
	if now.Before(p.ReceivedAt) || now.Sub(p.ObservedAt) >= limit || now.Sub(p.ReceivedAt) >= limit {
		return Unknown, "OBSERVATION_STALE"
	}
	if p.State == Unknown {
		return Unknown, "WARMING_UP"
	}
	return p.State, ""
}

type Window struct {
	ID          string
	Start, End  time.Time
	CancelledAt *time.Time
}

type Summary struct {
	State          State   `json:"state"`
	ObservedState  State   `json:"observed_state"`
	UnknownSources int     `json:"unknown_sources"`
	MaintenanceID  *string `json:"maintenance_id"`
}

func Summarize(sources []State, windows []Window, now time.Time) Summary {
	out := Summary{State: Unknown, ObservedState: Unknown}
	if len(sources) > 0 {
		out.ObservedState = Operational
	}
	for _, state := range sources {
		if state != Operational && state != Degraded && state != Down {
			state = Unknown
		}
		if state == Unknown {
			out.UnknownSources++
		}
		if state == Down {
			out.ObservedState = Down
		} else if out.ObservedState != Down && state == Unknown {
			out.ObservedState = Unknown
		} else if out.ObservedState == Operational && state == Degraded {
			out.ObservedState = Degraded
		}
	}
	out.State = out.ObservedState
	for _, w := range windows {
		if w.CancelledAt == nil && !now.Before(w.Start) && now.Before(w.End) {
			id := w.ID
			out.State, out.MaintenanceID = Maintenance, &id
			break
		}
	}
	return out
}
