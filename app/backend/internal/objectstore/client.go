package objectstore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aws/smithy-go"
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func settingsValid(s Settings, provider string) error {
	endpoint, err := url.Parse(s.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.Trim(endpoint.Path, "/") != "" || !bucketPattern.MatchString(s.Bucket) || s.AccessKeyID == "" || s.AccessKeySecret == "" {
		return invalid("INVALID_STORAGE_CONFIGURATION")
	}
	host := strings.ToLower(endpoint.Hostname())
	if provider == "r2" && !strings.HasSuffix(host, ".r2.cloudflarestorage.com") {
		return invalid("INVALID_R2_ENDPOINT")
	}
	if provider == "oss" && (s.Region == "" || !strings.HasSuffix(host, ".aliyuncs.com")) {
		return invalid("INVALID_OSS_ENDPOINT")
	}
	return nil
}
func secureHTTP() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
			MaxConnsPerHost: 8, MaxIdleConns: 16, IdleConnTimeout: 90 * time.Second, ForceAttemptHTTP2: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
func failure(err error, mutation bool) error {
	if err == nil {
		return nil
	}
	var own *Error
	if errors.As(err, &own) {
		return own
	}
	code := "PROVIDER_FAILURE"
	retryable := false
	var sdk smithy.APIError
	var aliyun *oss.ServiceError
	var timeout net.Error
	providerCode := ""
	if errors.As(err, &sdk) {
		providerCode = sdk.ErrorCode()
	}
	if errors.As(err, &aliyun) {
		providerCode = aliyun.Code
	}
	switch providerCode {
	case "NoSuchKey", "NoSuchUpload", "NotFound":
		code = "OBJECT_NOT_FOUND"
	case "PreconditionFailed", "ObjectAlreadyExists", "FileAlreadyExists":
		code = "SOURCE_CHANGED"
	case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidSecurityToken", "ExpiredToken":
		code = "PROVIDER_ACCESS_DENIED"
	case "SlowDown", "Throttling", "ServiceUnavailable", "InternalError", "RequestTimeout":
		code = "PROVIDER_RETRYABLE"
		retryable = true
	case "InvalidPart", "InvalidPartOrder", "EntityTooSmall", "InvalidArgument":
		code = "PROVIDER_PARTS_REJECTED"
	default:
		retryable = true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		code = "PROVIDER_TIMEOUT"
		retryable = true
	}
	return &Error{Code: code, Retryable: retryable, Uncertain: mutation && retryable}
}
func markerValue(marker string) (int, error) {
	if marker == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(marker)
	if err != nil || n < 0 || n >= MaxParts {
		return 0, invalid("INVALID_PART_MARKER")
	}
	return n, nil
}
func signedHeaders(headers http.Header) map[string]string {
	out := map[string]string{}
	for name, values := range headers {
		if strings.EqualFold(name, "host") || strings.EqualFold(name, "content-length") {
			continue
		}
		out[name] = strings.Join(values, ",")
	}
	return out
}
func signInput(m Multipart, number int, size int64, ttl time.Duration) error {
	if err := validatePart(m, number, size); err != nil {
		return err
	}
	if !strings.Contains(m.Key, "/drafts/") {
		return invalid("CLIENT_MAY_ONLY_SIGN_DRAFT_PARTS")
	}
	if ttl < time.Second || ttl > MaxSignTTL {
		return invalid("INVALID_SIGNATURE_EXPIRY")
	}
	return nil
}
func completionParts(parts []Part) ([]Part, int64, error) {
	if len(parts) < 1 || len(parts) > MaxParts {
		return nil, 0, invalid("INVALID_PARTS")
	}
	var size int64
	for _, part := range parts {
		if part.Size < 1 || part.Size > MaxPartBytes {
			return nil, 0, invalid("INVALID_PART_SIZE")
		}
		size += part.Size
	}
	sorted := append([]Part(nil), parts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })
	bytes := sorted[0].Size
	if len(parts) == 1 && bytes < MinPartBytes {
		bytes = MinPartBytes
	}
	plan, err := Plan(size, bytes)
	if err != nil {
		return nil, 0, err
	}
	ordered, err := plan.Reconcile(parts)
	return ordered, size, err
}
func checkRead(body ioCloser, etag, contentRange, version string, size int64, source Source, offset, length int64) error {
	expected := fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, source.Size)
	if size != length || etag != source.ETag || (source.VersionID != "" && version != source.VersionID) || contentRange != expected {
		_ = body.Close()
		return invalid("SOURCE_RANGE_MISMATCH")
	}
	return nil
}

type ioCloser interface{ Close() error }
