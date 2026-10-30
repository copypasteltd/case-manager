package statusbridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

type checkpoint struct {
	Generation uint64                        `json:"generation"`
	Binding    string                        `json:"binding"`
	Sequence   int64                         `json:"sequence"`
	ObservedAt time.Time                     `json:"observed_at"`
	SampleSHA  string                        `json:"sample_sha256"`
	Body       []byte                        `json:"body"`
	Signature  string                        `json:"signature"`
	Ack        *servicestatus.ObservationAck `json:"ack"`
}

type journal struct {
	file, lock    *os.File
	unlock        func() error
	path, binding string
	target        Target
	key           ed25519.PrivateKey
	current       checkpoint
	broken        bool
}

func privateStateFile(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !st.Mode().IsRegular() || runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return fault("STATE_FILE_INVALID")
	}
	return nil
}

func openJournal(c Config, t Target, key ed25519.PrivateKey) (*journal, error) {
	dir, err := os.Lstat(c.StateDirectory)
	if err != nil || !dir.IsDir() || runtime.GOOS != "windows" && dir.Mode().Perm()&0077 != 0 {
		return nil, fault("STATE_DIRECTORY_INVALID")
	}
	name := filepath.Join(c.StateDirectory, t.SourceID+"."+t.RevisionID+".journal")
	for _, path := range []string{name, name + ".lock", name + ".next"} {
		if err = privateStateFile(path); err != nil {
			return nil, err
		}
	}
	l, err := os.OpenFile(name+".lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fault("STATE_LOCK_FAILED")
	}
	unlock, err := lockFile(l)
	if err != nil {
		l.Close()
		return nil, fault("STATE_ALREADY_OWNED")
	}
	j := &journal{lock: l, unlock: unlock, path: name, binding: binding(c, t, key), target: t, key: key}
	good := false
	defer func() {
		if !good {
			j.Close()
		}
	}()
	j.file, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fault("STATE_OPEN_FAILED")
	}
	if j.file.Sync() != nil || syncStateDirectory(c.StateDirectory) != nil {
		return nil, fault("STATE_WRITE_UNCERTAIN")
	}
	if err = j.restore(); err != nil {
		return nil, err
	}
	// A leftover compaction file never contains a unique committed operation:
	// the complete current checkpoint was synced in the primary journal first.
	if err = os.Remove(name + ".next"); err != nil && !os.IsNotExist(err) {
		return nil, fault("STATE_RECOVERY_FAILED")
	}
	good = true
	return j, nil
}

func (j *journal) Close() error {
	if j.file != nil {
		j.file.Close()
		j.file = nil
	}
	if j.unlock != nil {
		j.unlock()
		j.unlock = nil
	}
	if j.lock != nil {
		j.lock.Close()
		j.lock = nil
	}
	return nil
}

func (j *journal) valid(next checkpoint) bool {
	if j.target.Kind == "host" {
		return j.validResourceCheckpoint(next)
	}
	if next.Generation == 0 || next.Binding != j.binding || next.Sequence < 1 || next.Sequence > 9007199254740991 || len(next.Body) > 4096 || uniqueJSON(next.Body) != nil {
		return false
	}
	var wire servicestatus.SignedObservation
	d := json.NewDecoder(bytes.NewReader(next.Body))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || wire.RevisionID != j.target.RevisionID || wire.Sequence != strconv.FormatInt(next.Sequence, 10) ||
		!wire.ObservedAt.Equal(next.ObservedAt) || wire.LatencyMS == nil || *wire.LatencyMS < 0 || *wire.LatencyMS > 3600000 ||
		(sample{wire.ObservedAt, wire.Healthy, *wire.LatencyMS}).digest() != next.SampleSHA {
		return false
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(next.Signature)
	if err != nil || !ed25519.Verify(j.key.Public().(ed25519.PublicKey), servicestatus.SigningMessage(j.target.SourceID, next.Body), sig) {
		return false
	}
	if next.Ack != nil && !validAck(*next.Ack, j.target, wire.Sequence, core.Digest(next.Body)) {
		return false
	}
	return true
}

func transition(prev, next checkpoint) bool {
	if prev.Generation == 0 {
		return true
	}
	if next.Generation != prev.Generation+1 {
		return false
	}
	if prev.Ack == nil {
		return next.Ack != nil && prev.Sequence == next.Sequence && bytes.Equal(prev.Body, next.Body) && prev.Signature == next.Signature && prev.SampleSHA == next.SampleSHA
	}
	return next.Ack == nil && next.Sequence == prev.Sequence+1 && next.ObservedAt.After(prev.ObservedAt)
}

func frame(c checkpoint) []byte {
	b, _ := json.Marshal(c)
	digest := sha256.Sum256(b)
	out := make([]byte, 4+len(b)+32)
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	copy(out[4+len(b):], digest[:])
	return out
}

func (j *journal) restore() error {
	st, err := j.file.Stat()
	if err != nil || st.Size() > 64<<20 {
		return fault("STATE_CORRUPT")
	}
	b, err := io.ReadAll(j.file)
	if err != nil {
		return fault("STATE_CORRUPT")
	}
	validBytes := 0
	for at := 0; at < len(b); {
		if len(b)-at < 4 {
			break
		}
		n := int(binary.BigEndian.Uint32(b[at:]))
		if n < 1 || n > 64<<10 {
			return fault("STATE_CORRUPT")
		}
		if len(b)-at < 4+n+32 {
			break
		}
		payload := b[at+4 : at+4+n]
		digest := sha256.Sum256(payload)
		var next checkpoint
		if !bytes.Equal(digest[:], b[at+4+n:at+4+n+32]) || uniqueJSON(payload) != nil || json.Unmarshal(payload, &next) != nil || !j.valid(next) || !transition(j.current, next) {
			return fault("STATE_CORRUPT_OR_REBOUND")
		}
		j.current = next
		at += 4 + n + 32
		validBytes = at
	}
	if int64(validBytes) != st.Size() {
		if j.file.Truncate(int64(validBytes)) != nil || j.file.Sync() != nil {
			return fault("STATE_RECOVERY_FAILED")
		}
	}
	return nil
}

func (j *journal) save(next checkpoint) error {
	if j.broken || j.file == nil {
		return fault("STATE_WRITE_UNCERTAIN")
	}
	next.Generation = j.current.Generation + 1
	if !j.valid(next) || !transition(j.current, next) {
		return fault("STATE_TRANSITION_INVALID")
	}
	b := frame(next)
	if _, err := j.file.Seek(0, io.SeekEnd); err != nil {
		j.broken = true
		return fault("STATE_WRITE_UNCERTAIN")
	}
	n, err := j.file.Write(b)
	if err != nil || n != len(b) || j.file.Sync() != nil {
		j.broken = true
		return fault("STATE_WRITE_UNCERTAIN")
	}
	// Freeze owned buffers, not caller aliases, after the durable append.
	encoded, _ := json.Marshal(next)
	_ = json.Unmarshal(encoded, &j.current)
	st, err := j.file.Stat()
	if err != nil {
		j.broken = true
		return fault("STATE_WRITE_UNCERTAIN")
	}
	if st.Size() >= 1<<20 {
		return j.compact()
	}
	return nil
}

func (j *journal) compact() error {
	name := j.path + ".next"
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		j.broken = true
		return fault("STATE_COMPACT_FAILED")
	}
	b := frame(j.current)
	n, writeErr := f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(b) || syncErr != nil || closeErr != nil {
		j.broken = true
		return fault("STATE_COMPACT_FAILED")
	}
	if j.file.Close() != nil {
		j.broken = true
		return fault("STATE_COMPACT_FAILED")
	}
	j.file = nil
	if replaceFile(name, j.path) != nil {
		j.broken = true
		return fault("STATE_COMPACT_FAILED")
	}
	j.file, err = os.OpenFile(j.path, os.O_RDWR, 0600)
	if err != nil {
		j.broken = true
		return fault("STATE_COMPACT_FAILED")
	}
	return nil
}
