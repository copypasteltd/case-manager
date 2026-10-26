package objectstore

import (
	"context"
	"errors"
	"testing"
)

func TestFrozenHashResumesOnlyDurablyAcknowledgedRanges(t *testing.T) {
	source := Source{Key: fixtureKey(), ETag: "frozen-source", Size: 2*DefaultPartBytes + 91}
	var saved HashCheckpoint
	var offsets []int64
	var bodies []*trackedBody
	failSecond := true
	store := streamStore{read: func(ctx context.Context, s Source, offset, length int64) (ReadResult, error) {
		offsets = append(offsets, offset)
		if failSecond && offset == DefaultPartBytes {
			return ReadResult{}, &Error{Code: "PROVIDER_TIMEOUT", Retryable: true}
		}
		body := &trackedBody{Reader: &boundedReader{remaining: length}}
		bodies = append(bodies, body)
		return ReadResult{Body: body, Size: length, ETag: source.ETag}, nil
	}}
	save := func(p HashCheckpoint) error { saved = p; saved.State = append([]byte(nil), p.State...); return nil }
	if _, err := ResumeHashSource(context.Background(), store, source, HashCheckpoint{}, save); err == nil || saved.CompletedBytes != DefaultPartBytes {
		t.Fatal("interrupted hash did not preserve durable first range", saved.CompletedBytes, err)
	}
	failSecond = false
	offsets = nil
	digest, err := ResumeHashSource(context.Background(), store, source, saved, save)
	expected, e := HashExact(context.Background(), &boundedReader{remaining: source.Size}, source.Size)
	if err != nil || e != nil || digest != expected || len(offsets) != 2 || offsets[0] != DefaultPartBytes || offsets[1] != 2*DefaultPartBytes || saved.SHA256 != expected.SHA256 {
		t.Fatal("resumption reread prior ranges or changed full hash", digest, offsets, err)
	}
	for _, body := range bodies {
		if !body.closed {
			t.Fatal("hash range leaked")
		}
	}
	offsets = nil
	if _, err = ResumeHashSource(context.Background(), store, source, saved, save); err != nil || len(offsets) != 0 {
		t.Fatal("completed hash was reread", err)
	}
	if d, err := CompletedHash(source, saved); err != nil || d != expected {
		t.Fatal("completed checkpoint rejected", err)
	}
	corrupted := saved
	corrupted.Checksum = "changed"
	if _, err := CompletedHash(source, corrupted); err == nil {
		t.Fatal("corrupt completed authority accepted")
	}
}
func TestHashCheckpointsBindSourceAndDetectCorruptionAndRuntimeChanges(t *testing.T) {
	source := Source{Key: fixtureKey(), ETag: "frozen-source", Size: DefaultPartBytes + 7}
	var saved HashCheckpoint
	var offsets []int64
	stop := errors.New("controlled checkpoint persistence failure")
	store := streamStore{read: func(ctx context.Context, s Source, offset, length int64) (ReadResult, error) {
		offsets = append(offsets, offset)
		return ReadResult{Body: &trackedBody{Reader: &boundedReader{remaining: length}}, Size: length, ETag: s.ETag, VersionID: s.VersionID}, nil
	}}
	_, err := ResumeHashSource(context.Background(), store, source, HashCheckpoint{}, func(p HashCheckpoint) error {
		if p.CompletedBytes > DefaultPartBytes {
			return stop
		}
		saved = p
		return nil
	})
	if !errors.Is(err, stop) || saved.CompletedBytes != DefaultPartBytes {
		t.Fatal("unacknowledged checkpoint was accepted", err)
	}
	corrupted := saved
	corrupted.State = append([]byte(nil), saved.State...)
	corrupted.State[4] ^= 1
	if _, err = ResumeHashSource(context.Background(), store, source, corrupted, func(HashCheckpoint) error { return nil }); err == nil {
		t.Fatal("corrupt state was accepted")
	}
	changed := source
	changed.ETag = "different-source"
	if _, err = ResumeHashSource(context.Background(), store, changed, saved, func(HashCheckpoint) error { return nil }); err == nil {
		t.Fatal("checkpoint followed another source")
	}
	saved.RuntimeVersion = "go.sha256.state.v1/fixture-older-runtime"
	saved.Checksum = checkpointDigest(saved)
	offsets = nil
	var restarted bool
	_, err = ResumeHashSource(context.Background(), store, source, saved, func(p HashCheckpoint) error {
		if p.CompletedBytes == 0 {
			restarted = true
		}
		return nil
	})
	if err != nil || !restarted || len(offsets) != 2 || offsets[0] != 0 {
		t.Fatal("runtime format change did not explicitly restart", offsets, err)
	}
}
