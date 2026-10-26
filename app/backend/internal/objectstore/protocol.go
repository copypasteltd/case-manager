package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"copypaste.hk/platform/internal/core"
)

type Layout struct {
	Size, PartBytes int64
	Count           int
}

func Plan(size, partBytes int64) (Layout, error) {
	if size < 1 || size > MaxObjectBytes {
		return Layout{}, invalid("INVALID_OBJECT_SIZE")
	}
	if partBytes == 0 {
		partBytes = DefaultPartBytes
	}
	if partBytes < MinPartBytes || partBytes > MaxPartBytes {
		return Layout{}, invalid("INVALID_PART_SIZE")
	}
	count := (size + partBytes - 1) / partBytes
	if count > MaxParts {
		return Layout{}, invalid("TOO_MANY_PARTS")
	}
	return Layout{Size: size, PartBytes: partBytes, Count: int(count)}, nil
}
func (l Layout) PartSize(number int) (int64, error) {
	checked, err := Plan(l.Size, l.PartBytes)
	if err != nil || checked.Count != l.Count || number < 1 || number > l.Count {
		return 0, invalid("INVALID_PART_NUMBER")
	}
	if number == l.Count {
		return l.Size - int64(number-1)*l.PartBytes, nil
	}
	return l.PartBytes, nil
}
func (l Layout) Reconcile(parts []Part) ([]Part, error) {
	if len(parts) != l.Count {
		return nil, invalid("INCOMPLETE_PARTS")
	}
	ordered := append([]Part(nil), parts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Number < ordered[j].Number })
	for i, p := range ordered {
		size, err := l.PartSize(i + 1)
		if err != nil || p.Number != i+1 || p.Size != size || !validETag(p.ETag) {
			return nil, invalid("PARTS_MISMATCH")
		}
	}
	return ordered, nil
}
func validETag(value string) bool {
	if value == "" || len(value) > 200 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

var keyPattern = regexp.MustCompile(`^org/[0-9a-f-]{36}/(drafts/[0-9a-f-]{36}/source|candidates/[0-9a-f-]{36}/original|files/[0-9a-f-]{36}/versions/[0-9a-f-]{36}/(replicas/[0-9a-f-]{36}/)?original)$`)

func validKey(key string) bool {
	if !keyPattern.MatchString(key) {
		return false
	}
	for _, segment := range strings.Split(key, "/") {
		if len(segment) == 36 && !core.ValidID(segment) {
			return false
		}
	}
	return true
}
func DraftKey(org, session string) (string, error) {
	if !core.ValidID(org) || !core.ValidID(session) {
		return "", invalid("INVALID_STORAGE_ID")
	}
	return "org/" + strings.ToLower(org) + "/drafts/" + strings.ToLower(session) + "/source", nil
}
func CandidateKey(org, candidate string) (string, error) {
	if !core.ValidID(org) || !core.ValidID(candidate) {
		return "", invalid("INVALID_STORAGE_ID")
	}
	return "org/" + strings.ToLower(org) + "/candidates/" + strings.ToLower(candidate) + "/original", nil
}
func VersionKey(org, file, version string) (string, error) {
	if !core.ValidID(org) || !core.ValidID(file) || !core.ValidID(version) {
		return "", invalid("INVALID_STORAGE_ID")
	}
	return "org/" + strings.ToLower(org) + "/files/" + strings.ToLower(file) + "/versions/" + strings.ToLower(version) + "/original", nil
}
func ReplicaKey(org, file, version, attempt string) (string, error) {
	key, err := VersionKey(org, file, version)
	if err != nil || !core.ValidID(attempt) {
		return "", invalid("INVALID_STORAGE_ID")
	}
	return strings.TrimSuffix(key, "original") + "replicas/" + strings.ToLower(attempt) + "/original", nil
}
func validateMultipart(m Multipart) error {
	if !validKey(m.Key) || m.UploadID == "" || len(m.UploadID) > 2048 || strings.ContainsAny(m.UploadID, "\r\n") {
		return invalid("INVALID_MULTIPART")
	}
	return nil
}
func validatePart(m Multipart, number int, size int64) error {
	if err := validateMultipart(m); err != nil {
		return err
	}
	if number < 1 || number > MaxParts || size < 1 || size > MaxPartBytes {
		return invalid("INVALID_PART")
	}
	return nil
}
func validateSource(source Source, offset, length int64) error {
	if !validKey(source.Key) || !validETag(source.ETag) || source.Size < 0 || source.Size > MaxObjectBytes || offset < 0 || length < 1 || offset > source.Size || length > source.Size-offset {
		return invalid("INVALID_SOURCE_RANGE")
	}
	if strings.ContainsAny(source.VersionID, "\r\n") {
		return invalid("INVALID_SOURCE")
	}
	return nil
}

type Digest struct {
	SHA256 string
	Size   int64
}
type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(b)
}

// HashExact allocates one 256 KiB buffer and reads at most expected+1 bytes.
// Callers close the provider body and bind reads to a frozen source.
func HashExact(ctx context.Context, source io.Reader, expected int64) (Digest, error) {
	if source == nil || expected < 0 || expected > MaxObjectBytes {
		return Digest{}, invalid("INVALID_OBJECT_SIZE")
	}
	hash := sha256.New()
	n, err := io.CopyBuffer(hash, io.LimitReader(contextReader{ctx, source}, expected+1), make([]byte, 256<<10))
	if err != nil {
		return Digest{}, &Error{Code: "SOURCE_READ_FAILED", Retryable: ctx.Err() == nil}
	}
	if n != expected {
		return Digest{}, invalid("SOURCE_SIZE_MISMATCH")
	}
	return Digest{SHA256: hex.EncodeToString(hash.Sum(nil)), Size: n}, nil
}
