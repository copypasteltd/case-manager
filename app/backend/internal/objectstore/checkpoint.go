package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"runtime"
	"strconv"

	"copypaste.hk/platform/internal/core"
)

// HashCheckpoint is bounded internal state, not a client-authored digest.
// Save must compare the current domain version, source and job fencing token.
type HashCheckpoint struct {
	SourceDigest   string
	RuntimeVersion string
	CompletedBytes int64
	State          []byte
	Checksum       string
	SHA256         string
}

func checkpointRuntime() string { return "go.sha256.state.v1/" + runtime.Version() }
func checkpointDigest(p HashCheckpoint) string {
	return core.Digest([]byte(p.SourceDigest + "\x00" + p.RuntimeVersion + "\x00" + strconv.FormatInt(p.CompletedBytes, 10) + "\x00" + string(p.State) + "\x00" + p.SHA256))
}
func checkpoint(hash hash.Hash, sourceDigest string, completed, total int64) (HashCheckpoint, error) {
	state, err := hash.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return HashCheckpoint{}, invalid("HASH_CHECKPOINT_INVALID")
	}
	p := HashCheckpoint{SourceDigest: sourceDigest, RuntimeVersion: checkpointRuntime(), CompletedBytes: completed, State: state}
	if completed == total {
		p.SHA256 = hex.EncodeToString(hash.Sum(nil))
	}
	p.Checksum = checkpointDigest(p)
	return p, nil
}

// CompletedHash validates persisted authority without rereading the object.
// A completed digest remains usable across Go hash-state format upgrades.
func CompletedHash(source Source, p HashCheckpoint) (Digest, error) {
	identity, _ := json.Marshal(source)
	decoded, err := hex.DecodeString(p.SHA256)
	if err != nil || len(decoded) != sha256.Size || p.SHA256 != hex.EncodeToString(decoded) ||
		p.SourceDigest != core.Digest(identity) || p.CompletedBytes != source.Size ||
		len(p.State) < 1 || len(p.State) > 2048 || len(p.RuntimeVersion) < 1 || len(p.RuntimeVersion) > 100 ||
		p.Checksum != checkpointDigest(p) {
		return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
	}
	if p.RuntimeVersion == checkpointRuntime() {
		h := sha256.New()
		if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(p.State); err != nil || hex.EncodeToString(h.Sum(nil)) != p.SHA256 {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
	}
	return Digest{SHA256: p.SHA256, Size: source.Size}, nil
}

// ResumeHashSource reads exact pinned ranges with a 256 KiB buffer. A durable
// checkpoint after each 32 MiB range permits recovery without rereading earlier
// ranges. Go hash-state format upgrades explicitly restart from zero.
func ResumeHashSource(ctx context.Context, store Store, source Source, prior HashCheckpoint, save func(HashCheckpoint) error) (Digest, error) {
	if store == nil || save == nil || !validKey(source.Key) || !validETag(source.ETag) || source.Size < 0 || source.Size > MaxObjectBytes {
		return Digest{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	identity, _ := json.Marshal(source)
	sourceDigest := core.Digest(identity)
	hash := sha256.New()
	progress := prior
	if prior.SourceDigest == "" {
		if prior.CompletedBytes != 0 || prior.RuntimeVersion != "" || len(prior.State) != 0 || prior.Checksum != "" || prior.SHA256 != "" {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
	} else {
		if prior.SourceDigest != sourceDigest {
			return Digest{}, invalid("SOURCE_CHECKPOINT_MISMATCH")
		}
		if len(prior.State) < 1 || len(prior.State) > 2048 || len(prior.RuntimeVersion) < 1 || len(prior.RuntimeVersion) > 100 || prior.CompletedBytes < 0 || prior.CompletedBytes > source.Size ||
			(prior.CompletedBytes != source.Size && prior.CompletedBytes%DefaultPartBytes != 0) || prior.Checksum != checkpointDigest(prior) {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
	}
	if prior.SourceDigest == "" || prior.RuntimeVersion != checkpointRuntime() {
		var err error
		progress, err = checkpoint(hash, sourceDigest, 0, source.Size)
		if err != nil {
			return Digest{}, err
		}
		if err = save(progress); err != nil {
			return Digest{}, err
		}
	} else {
		if err := hash.(encoding.BinaryUnmarshaler).UnmarshalBinary(prior.State); err != nil {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
		if prior.CompletedBytes == source.Size && prior.SHA256 != hex.EncodeToString(hash.Sum(nil)) {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
		if prior.CompletedBytes != source.Size && prior.SHA256 != "" {
			return Digest{}, invalid("HASH_CHECKPOINT_INVALID")
		}
	}
	buffer := make([]byte, 256<<10)
	for offset := progress.CompletedBytes; offset < source.Size; {
		if ctx.Err() != nil {
			return Digest{}, &Error{Code: "SOURCE_READ_CANCELLED", Retryable: true}
		}
		length := DefaultPartBytes
		if source.Size-offset < length {
			length = source.Size - offset
		}
		read, err := store.Read(ctx, source, offset, length)
		if err != nil {
			return Digest{}, err
		}
		if read.Body == nil {
			return Digest{}, invalid("SOURCE_EVIDENCE_MISMATCH")
		}
		if read.Size != length || read.ETag != source.ETag || read.VersionID != source.VersionID {
			_ = read.Body.Close()
			return Digest{}, invalid("SOURCE_EVIDENCE_MISMATCH")
		}
		count, copyErr := io.CopyBuffer(hash, io.LimitReader(contextReader{ctx, read.Body}, length+1), buffer)
		closeErr := read.Body.Close()
		if copyErr != nil || closeErr != nil {
			return Digest{}, &Error{Code: "SOURCE_READ_FAILED", Retryable: true}
		}
		if count != length {
			return Digest{}, invalid("SOURCE_SIZE_MISMATCH")
		}
		offset += length
		progress, err = checkpoint(hash, sourceDigest, offset, source.Size)
		if err != nil {
			return Digest{}, err
		}
		if err = save(progress); err != nil {
			return Digest{}, err
		}
	}
	if ctx.Err() != nil {
		return Digest{}, &Error{Code: "SOURCE_READ_CANCELLED", Retryable: true}
	}
	return Digest{SHA256: hex.EncodeToString(hash.Sum(nil)), Size: source.Size}, nil
}
