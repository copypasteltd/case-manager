package objectstore

import (
	"context"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const DownloadSignTTL = 5 * time.Minute

// DownloadSigner cannot select buckets or receive arbitrary response headers.
// Signatures are transient bearer credentials and must never be persisted.
type DownloadSigner interface {
	SignRead(context.Context, Source, ReadOptions, time.Duration) (Signed, error)
}
type ReadOptions struct{ Filename, Disposition, MediaType string }

func InlineMediaType(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "application/pdf", "text/plain":
		return true
	}
	return false
}
func (o ReadOptions) Headers() (disposition, mediaType string, err error) {
	if !utf8.ValidString(o.Filename) || len([]rune(o.Filename)) < 1 || len([]rune(o.Filename)) > 200 || strings.ContainsAny(o.Filename, "/\\") {
		return "", "", invalid("INVALID_DOWNLOAD_FILENAME")
	}
	for _, r := range o.Filename {
		if unicode.IsControl(r) {
			return "", "", invalid("INVALID_DOWNLOAD_FILENAME")
		}
	}
	if o.Disposition != "attachment" && (o.Disposition != "inline" || !InlineMediaType(o.MediaType)) {
		return "", "", invalid("UNSAFE_DOWNLOAD_DISPOSITION")
	}
	mediaType = "application/octet-stream"
	if o.Disposition == "inline" {
		mediaType = o.MediaType
	}
	return mime.FormatMediaType(o.Disposition, map[string]string{"filename": o.Filename}), mediaType, nil
}
func ValidateDownloadSource(source Source) error {
	if !validKey(source.Key) || !strings.Contains(source.Key, "/files/") || !validETag(source.ETag) ||
		source.Size < 0 || source.Size > MaxObjectBytes || len(source.VersionID) > 2048 || strings.ContainsAny(source.VersionID, "\r\n") {
		return invalid("INVALID_DOWNLOAD_SOURCE")
	}
	return nil
}
func downloadInput(source Source, options ReadOptions, ttl time.Duration) (string, string, error) {
	if err := ValidateDownloadSource(source); err != nil {
		return "", "", err
	}
	if ttl < time.Second || ttl > DownloadSignTTL {
		return "", "", invalid("INVALID_DOWNLOAD_TTL")
	}
	return options.Headers()
}
func (r *OSS) SignRead(ctx context.Context, source Source, options ReadOptions, ttl time.Duration) (Signed, error) {
	disposition, mediaType, err := downloadInput(source, options, ttl)
	if err != nil {
		return Signed{}, err
	}
	in := &oss.GetObjectRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(source.Key), IfMatch: oss.Ptr(source.ETag),
		ResponseContentDisposition: oss.Ptr(disposition), ResponseContentType: oss.Ptr(mediaType), ResponseCacheControl: oss.Ptr("private, no-store")}
	if source.VersionID != "" {
		in.VersionId = oss.Ptr(source.VersionID)
	}
	out, err := r.client.Presign(ctx, in, oss.PresignExpires(ttl))
	if err != nil {
		return Signed{}, failure(err, false)
	}
	headers := http.Header{}
	for name, value := range out.SignedHeaders {
		headers.Set(name, value)
	}
	// Fail closed if a future SDK stops signing the immutable-source condition.
	if headers.Get("If-Match") != source.ETag {
		return Signed{}, invalid("DOWNLOAD_CONDITION_NOT_SIGNED")
	}
	return Signed{Method: out.Method, URL: out.URL, Headers: signedHeaders(headers), ExpiresAt: out.Expiration}, nil
}
func (r *R2) SignRead(ctx context.Context, source Source, options ReadOptions, ttl time.Duration) (Signed, error) {
	disposition, mediaType, err := downloadInput(source, options, ttl)
	if err != nil {
		return Signed{}, err
	}
	if source.VersionID != "" {
		return Signed{}, invalid("R2_NATIVE_VERSION_UNSUPPORTED")
	}
	out, err := r.signer.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(source.Key), IfMatch: aws.String(source.ETag),
		ResponseContentDisposition: aws.String(disposition), ResponseContentType: aws.String(mediaType), ResponseCacheControl: aws.String("private, no-store")},
		func(o *s3.PresignOptions) { o.Expires = ttl })
	if err != nil {
		return Signed{}, failure(err, false)
	}
	if out.SignedHeader.Get("If-Match") != source.ETag {
		return Signed{}, invalid("DOWNLOAD_CONDITION_NOT_SIGNED")
	}
	return Signed{Method: out.Method, URL: out.URL, Headers: signedHeaders(out.SignedHeader), ExpiresAt: time.Now().UTC().Add(ttl)}, nil
}

var _ DownloadSigner = (*OSS)(nil)
var _ DownloadSigner = (*R2)(nil)
