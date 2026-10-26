package servicestatus

import (
	"testing"
	"time"
)

func TestMonitoringFailureAndRecoveryHysteresis(t *testing.T) {
	policy := Thresholds{FailureCount: 3, RecoveryCount: 2, Interval: 60, StaleAfter: 180, LatencyMS: 500}
	base := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	p := Projection{State: Unknown}
	for index, step := range []struct {
		healthy bool
		latency int64
		want    State
	}{{true, 100, Unknown}, {true, 100, Operational}, {false, 0, Degraded}, {false, 0, Degraded}, {false, 0, Down}, {true, 100, Down}, {true, 100, Operational}, {true, 501, Degraded}, {true, 100, Degraded}, {true, 100, Operational}} {
		at := base.Add(time.Duration(index) * time.Minute)
		next, ignored, err := Advance(policy, p, Sample{Sequence: int64(index + 1), ObservedAt: at, ReceivedAt: at.Add(time.Second), Healthy: step.healthy, LatencyMS: &step.latency})
		if err != nil || ignored != "" || next.State != step.want {
			t.Fatalf("step %d: %s %s %v, wanted %s", index, next.State, ignored, err, step.want)
		}
		p = next
	}
}

func TestMonitoringLateSamplesNeverReplaceNewerEvidence(t *testing.T) {
	policy := Thresholds{3, 2, 60, 180, 500}
	now := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	prior := Projection{Sequence: 5, State: Down, Failures: 3, ObservedAt: now.Add(-time.Minute), ReceivedAt: now.Add(-59 * time.Second)}
	latency := int64(10)
	for _, sample := range []Sample{
		{4, now, now.Add(time.Second), true, &latency},
		{6, now.Add(-2 * time.Minute), now.Add(time.Second), true, &latency},
		{6, now.Add(time.Second), now.Add(4 * time.Minute), true, &latency},
	} {
		next, reason, err := Advance(policy, prior, sample)
		if err != nil || reason == "" || next != prior {
			t.Fatal("late evidence changed current state", next, reason, err)
		}
	}
}

func TestMonitoringMissingSequenceAndOutageResetConsecutiveCounts(t *testing.T) {
	policy := Thresholds{3, 2, 60, 180, 500}
	now := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	for _, gap := range []struct {
		seq int64
		age time.Duration
	}{{4, time.Minute}, {3, 4 * time.Minute}} {
		prior := Projection{Sequence: 2, State: Degraded, Failures: 2, ObservedAt: now.Add(-gap.age), ReceivedAt: now.Add(-gap.age)}
		next, reason, err := Advance(policy, prior, Sample{Sequence: gap.seq, ObservedAt: now, ReceivedAt: now, Healthy: false})
		if err != nil || reason != "" || next.State != Degraded || next.Failures != 1 {
			t.Fatal("missing evidence was treated as consecutive failures", next, reason, err)
		}
	}
}

func TestMonitoringStalenessAndMaintenanceNeverInventHealthyEvidence(t *testing.T) {
	policy := Thresholds{3, 2, 60, 180, 500}
	now := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	p := Projection{Sequence: 9, State: Operational, ObservedAt: now.Add(-180 * time.Second), ReceivedAt: now.Add(-179 * time.Second)}
	state, reason := VisibleSource(policy, true, p, now)
	if state != Unknown || reason != "OBSERVATION_STALE" {
		t.Fatal(state, reason)
	}
	windows := []Window{{ID: "owned-window", Start: now.Add(-time.Minute), End: now.Add(time.Minute)}}
	value := Summarize([]State{Down, Unknown}, windows, now)
	if value.State != Maintenance || value.ObservedState != Down || value.UnknownSources != 1 {
		t.Fatal("maintenance hid underlying evidence", value)
	}
	value = Summarize([]State{state}, windows, now.Add(time.Minute))
	if value.State != Unknown || value.MaintenanceID != nil {
		t.Fatal("ending maintenance restored stale healthy status", value)
	}
	if Summarize(nil, nil, now).State != Unknown || Summarize([]State{Operational, Unknown}, nil, now).State != Unknown {
		t.Fatal("missing observations reported operational")
	}
}

func TestMonitoringClockAndDisabledSourceCannotExtendFreshness(t *testing.T) {
	policy := Thresholds{3, 2, 60, 180, 500}
	now := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	p := Projection{Sequence: 2, State: Operational, ObservedAt: now.Add(10 * time.Second), ReceivedAt: now}
	for _, at := range []time.Time{now.Add(-time.Second), now.Add(180 * time.Second)} {
		if value, _ := VisibleSource(policy, true, p, at); value != Unknown {
			t.Fatal("server clock or future sample extended freshness", value)
		}
	}
	if value, reason := VisibleSource(policy, false, p, now); value != Unknown || reason != "SOURCE_DISABLED" {
		t.Fatal(value, reason)
	}
	latency := int64(10)
	if _, _, err := Advance(policy, Projection{}, Sample{Sequence: 1, ObservedAt: now.Add(16 * time.Second), ReceivedAt: now, Healthy: true, LatencyMS: &latency}); err == nil {
		t.Fatal("unbounded future timestamp accepted")
	}
	if _, _, err := Advance(policy, Projection{}, Sample{Sequence: 1, ObservedAt: now, ReceivedAt: now, Healthy: true}); err == nil {
		t.Fatal("healthy response without measured latency accepted")
	}
}
