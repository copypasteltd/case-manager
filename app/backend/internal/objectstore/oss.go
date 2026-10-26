package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type OSS struct {
	client *oss.Client
	bucket string
}

func NewOSS(settings Settings) (*OSS, error) { return newOSS(settings, secureHTTP()) }
func newOSS(settings Settings, httpClient *http.Client) (*OSS, error) {
	if err := settingsValid(settings, "oss"); err != nil {
		return nil, err
	}
	cfg := oss.NewConfig().WithEndpoint(settings.Endpoint).WithRegion(settings.Region).WithRetryMaxAttempts(1).WithHttpClient(httpClient).WithAdditionalHeaders([]string{"if-match"}).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(settings.AccessKeyID, settings.AccessKeySecret, settings.SessionToken))
	return &OSS{client: oss.NewClient(cfg), bucket: settings.Bucket}, nil
}
func (r *OSS) Begin(ctx context.Context, key, mediaType string) (Multipart, error) {
	if !validKey(key) {
		return Multipart{}, invalid("INVALID_STORAGE_KEY")
	}
	out, err := r.client.InitiateMultipartUpload(ctx, &oss.InitiateMultipartUploadRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(key), ContentType: oss.Ptr(mediaType), ForbidOverwrite: oss.Ptr("true"), DisableAutoDetectMimeType: true})
	if err != nil {
		return Multipart{}, failure(err, true)
	}
	m := Multipart{Key: key, UploadID: oss.ToString(out.UploadId)}
	if err = validateMultipart(m); err != nil {
		return Multipart{}, err
	}
	return m, nil
}
func (r *OSS) SignPart(ctx context.Context, m Multipart, number int, size int64, ttl time.Duration) (Signed, error) {
	if err := signInput(m, number, size, ttl); err != nil {
		return Signed{}, err
	}
	out, err := r.client.Presign(ctx, &oss.UploadPartRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID), PartNumber: int32(number), ContentLength: oss.Ptr(size)}, oss.PresignExpires(ttl))
	if err != nil {
		return Signed{}, failure(err, false)
	}
	headers := http.Header{}
	for name, value := range out.SignedHeaders {
		headers.Set(name, value)
	}
	return Signed{Method: out.Method, URL: out.URL, Headers: signedHeaders(headers), ExpiresAt: out.Expiration}, nil
}
func (r *OSS) Parts(ctx context.Context, m Multipart, marker string) (PartPage, error) {
	if err := validateMultipart(m); err != nil {
		return PartPage{}, err
	}
	from, err := markerValue(marker)
	if err != nil {
		return PartPage{}, err
	}
	out, err := r.client.ListParts(ctx, &oss.ListPartsRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID), MaxParts: 1000, PartNumberMarker: int32(from)})
	if err != nil {
		return PartPage{}, failure(err, false)
	}
	page := PartPage{Items: []Part{}}
	for _, part := range out.Parts {
		page.Items = append(page.Items, Part{Number: int(part.PartNumber), Size: part.Size, ETag: oss.ToString(part.ETag)})
	}
	if out.IsTruncated {
		page.NextMarker = strconv.Itoa(int(out.NextPartNumberMarker))
		if out.NextPartNumberMarker <= int32(from) || out.NextPartNumberMarker >= MaxParts {
			return PartPage{}, invalid("INVALID_PROVIDER_PAGINATION")
		}
	}
	return page, nil
}
func (r *OSS) Complete(ctx context.Context, m Multipart, parts []Part) (Source, error) {
	if err := validateMultipart(m); err != nil {
		return Source{}, err
	}
	ordered, size, err := completionParts(parts)
	if err != nil {
		return Source{}, err
	}
	completed := make([]oss.UploadPart, len(ordered))
	for i, part := range ordered {
		completed[i] = oss.UploadPart{PartNumber: int32(part.Number), ETag: oss.Ptr(part.ETag)}
	}
	out, err := r.client.CompleteMultipartUpload(ctx, &oss.CompleteMultipartUploadRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID), ForbidOverwrite: oss.Ptr("true"), CompleteMultipartUpload: &oss.CompleteMultipartUpload{Parts: completed}})
	if err != nil {
		return Source{}, failure(err, true)
	}
	if !validETag(oss.ToString(out.ETag)) {
		return Source{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	return Source{Key: m.Key, ETag: oss.ToString(out.ETag), VersionID: oss.ToString(out.VersionId), Size: size}, nil
}
func (r *OSS) Abort(ctx context.Context, m Multipart) error {
	if err := validateMultipart(m); err != nil {
		return err
	}
	_, err := r.client.AbortMultipartUpload(ctx, &oss.AbortMultipartUploadRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID)})
	mapped := failure(err, true)
	if e, ok := mapped.(*Error); ok && e.Code == "OBJECT_NOT_FOUND" {
		return nil
	}
	return mapped
}
func (r *OSS) Head(ctx context.Context, key string) (Source, error) {
	if !validKey(key) {
		return Source{}, invalid("INVALID_STORAGE_KEY")
	}
	out, err := r.client.HeadObject(ctx, &oss.HeadObjectRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(key)})
	if err != nil {
		return Source{}, failure(err, false)
	}
	source := Source{Key: key, ETag: oss.ToString(out.ETag), VersionID: oss.ToString(out.VersionId), Size: out.ContentLength}
	if source.Size < 0 || source.Size > MaxObjectBytes || !validETag(source.ETag) {
		return Source{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	return source, nil
}
func (r *OSS) Read(ctx context.Context, source Source, offset, length int64) (ReadResult, error) {
	if err := validateSource(source, offset, length); err != nil {
		return ReadResult{}, err
	}
	in := &oss.GetObjectRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(source.Key), IfMatch: oss.Ptr(source.ETag),
		Range: oss.Ptr(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)), RangeBehavior: oss.Ptr("standard")}
	if source.VersionID != "" {
		in.VersionId = oss.Ptr(source.VersionID)
	}
	out, err := r.client.GetObject(ctx, in)
	if err != nil {
		return ReadResult{}, failure(err, false)
	}
	version := oss.ToString(out.VersionId)
	if err = checkRead(out.Body, oss.ToString(out.ETag), oss.ToString(out.ContentRange), version, out.ContentLength, source, offset, length); err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Body: out.Body, Size: length, ETag: source.ETag, VersionID: version}, nil
}
func (r *OSS) WritePart(ctx context.Context, m Multipart, number int, body io.Reader, size int64) (Part, error) {
	if err := validatePart(m, number, size); err != nil {
		return Part{}, err
	}
	if body == nil {
		return Part{}, invalid("INVALID_PART_BODY")
	}
	out, err := r.client.UploadPart(ctx, &oss.UploadPartRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID), PartNumber: int32(number), ContentLength: oss.Ptr(size), Body: io.LimitReader(body, size)})
	if err != nil {
		return Part{}, failure(err, true)
	}
	etag := oss.ToString(out.ETag)
	if !validETag(etag) {
		return Part{}, invalid("INVALID_PROVIDER_ETAG")
	}
	return Part{Number: number, Size: size, ETag: etag}, nil
}

var _ Store = (*OSS)(nil)
