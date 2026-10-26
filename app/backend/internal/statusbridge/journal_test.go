package statusbridge

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

func pendingFixture(j *journal, at time.Time) checkpoint {
	seq := j.current.Sequence + 1
	latency := int64(90)
	s := sample{at, true, latency}
	body, _ := json.Marshal(servicestatus.SignedObservation{RevisionID: j.target.RevisionID, Sequence: strconv.FormatInt(seq, 10), ObservedAt: at, Healthy: true, LatencyMS: &latency})
	return checkpoint{Binding: j.binding, Sequence: seq, ObservedAt: at, SampleSHA: s.digest(), Body: body,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(j.key, servicestatus.SigningMessage(j.target.SourceID, body)))}
}

func acknowledgeFixture(t *testing.T, j *journal) {
	t.Helper()
	next := j.current
	next.Ack = &servicestatus.ObservationAck{ID: core.NewID(), SourceID: j.target.SourceID, RevisionID: j.target.RevisionID, Sequence: strconv.FormatInt(next.Sequence, 10), PayloadSHA256: core.Digest(next.Body), Applied: true}
	if err := j.save(next); err != nil {
		t.Fatal(SafeCode(err))
	}
}

func TestStatusBridgeJournalRecovery(t *testing.T) {
	f := newBridgeFixture(t)
	b := openFixture(t, f)
	j := b.stores[0]
	if err := j.save(pendingFixture(j, f.at)); err != nil {
		t.Fatal(SafeCode(err))
	}
	if _, err := Open(f.c); SafeCode(err) != "STATE_ALREADY_OWNED" {
		t.Fatal("same revision acquired twice")
	}
	path := j.path
	b.Close()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte{0, 0, 1})
	_ = file.Sync()
	_ = file.Close()
	b = openFixture(t, f)
	j = b.stores[0]
	if j.current.Sequence != 1 || j.current.Ack != nil {
		t.Fatal("partial tail lost pending operation")
	}
	stat, _ := j.file.Stat()
	if stat.Size() != int64(len(frame(j.current))) {
		t.Fatal("incomplete append tail not recovered")
	}
	acknowledgeFixture(t, j)
	if err = j.save(pendingFixture(j, f.at.Add(time.Millisecond))); err != nil {
		t.Fatal(SafeCode(err))
	}
	if err = j.compact(); err != nil {
		t.Fatal(SafeCode(err))
	}
	b.Close()
	b = openFixture(t, f)
	j = b.stores[0]
	if j.current.Sequence != 2 || j.current.Ack != nil {
		t.Fatal("compaction lost new pending operation")
	}
	acknowledgeFixture(t, j)
	b.Close()
	b = openFixture(t, f)
	if b.stores[0].current.Sequence != 2 || b.stores[0].current.Ack == nil {
		t.Fatal("ack did not survive restart")
	}
}

func TestStatusBridgeJournalCorruptionAndRebinding(t *testing.T) {
	t.Run("completed_checksum_corruption_fails_closed", func(t *testing.T) {
		f := newBridgeFixture(t)
		b := openFixture(t, f)
		j := b.stores[0]
		_ = j.save(pendingFixture(j, f.at))
		path := j.path
		b.Close()
		raw, _ := os.ReadFile(path)
		raw[len(raw)-1] ^= 1
		_ = os.WriteFile(path, raw, 0600)
		if _, err := Open(f.c); SafeCode(err) != "STATE_CORRUPT_OR_REBOUND" {
			t.Fatal("corrupt checkpoint silently reset")
		}
	})
	t.Run("same_revision_different_target_refused", func(t *testing.T) {
		f := newBridgeFixture(t)
		b := openFixture(t, f)
		_ = b.stores[0].save(pendingFixture(b.stores[0], f.at))
		b.Close()
		f.c.Targets[0].Instance = "https://changed.invalid"
		if _, err := Open(f.c); SafeCode(err) != "STATE_CORRUPT_OR_REBOUND" {
			t.Fatal("revision rebound to another target")
		}
	})
	t.Run("new_published_revision_starts_own_sequence", func(t *testing.T) {
		f := newBridgeFixture(t)
		b := openFixture(t, f)
		_ = b.stores[0].save(pendingFixture(b.stores[0], f.at))
		b.Close()
		f.c.Targets[0].RevisionID = core.NewID()
		b = openFixture(t, f)
		if b.stores[0].current.Sequence != 0 {
			t.Fatal("new revision inherited old sequence")
		}
	})
	t.Run("pending_cannot_be_overwritten_by_new_sample", func(t *testing.T) {
		f := newBridgeFixture(t)
		b := openFixture(t, f)
		j := b.stores[0]
		_ = j.save(pendingFixture(j, f.at))
		if err := j.save(pendingFixture(j, f.at.Add(time.Millisecond))); SafeCode(err) != "STATE_TRANSITION_INVALID" {
			t.Fatal("pending replaced before acknowledgment")
		}
	})
}
