// Package objectstore implements bounded provider operations. Authorization,
// lifecycle, fencing, scanning and database commit remain domain responsibilities.
package objectstore

import (
	"context"
	"io"
	"time"
)

const (
	MaxObjectBytes   int64 = 120000000000
	DefaultPartBytes int64 = 32 << 20
	MinPartBytes     int64 = 5 << 20
	MaxPartBytes     int64 = 512 << 20
	MaxParts               = 10000
	MaxSignBatch           = 20
	MaxSignTTL             = 15 * time.Minute
)

type Part struct {
	Number int
	Size   int64
	ETag   string
}
type Multipart struct{ Key, UploadID string }

// Source has an opaque ETag condition and optional native version. ETag is never
// interpreted as a whole-object digest, including for multipart objects.
type Source struct {
	Key, ETag, VersionID string
	Size                 int64
}
type Signed struct {
	Method, URL string
	Headers     map[string]string
	ExpiresAt   time.Time
}
type PartPage struct {
	Items      []Part
	NextMarker string
}
type ReadResult struct {
	Body            io.ReadCloser
	Size            int64
	ETag, VersionID string
}

type Store interface {
	Begin(context.Context, string, string) (Multipart, error)
	SignPart(context.Context, Multipart, int, int64, time.Duration) (Signed, error)
	Parts(context.Context, Multipart, string) (PartPage, error)
	Complete(context.Context, Multipart, []Part) (Source, error)
	Abort(context.Context, Multipart) error
	Head(context.Context, string) (Source, error)
	Read(context.Context, Source, int64, int64) (ReadResult, error)
	WritePart(context.Context, Multipart, int, io.Reader, int64) (Part, error)
}

// Error is safe to log or map to a public error. Provider payloads and signed
// URLs are deliberately not retained in the error or an unwrap chain.
type Error struct {
	Code                 string
	Retryable, Uncertain bool
}

func (e *Error) Error() string  { return e.Code }
func invalid(code string) error { return &Error{Code: code} }

type Settings struct {
	Endpoint, Region, Bucket                   string
	AccessKeyID, AccessKeySecret, SessionToken string `json:"-"`
}

func (s Settings) String() string   { return "objectstore.Settings{credentials:redacted}" }
func (s Settings) GoString() string { return s.String() }
