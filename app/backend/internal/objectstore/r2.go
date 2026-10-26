package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type R2 struct {
	client *s3.Client
	signer *s3.PresignClient
	bucket string
}

func NewR2(settings Settings) (*R2, error) { return newR2(settings, secureHTTP()) }
func newR2(settings Settings, httpClient *http.Client) (*R2, error) {
	if err := settingsValid(settings, "r2"); err != nil {
		return nil, err
	}
	cfg := aws.Config{Region: "auto", Credentials: credentials.NewStaticCredentialsProvider(settings.AccessKeyID, settings.AccessKeySecret, settings.SessionToken), HTTPClient: httpClient,
		RetryMaxAttempts: 1, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(settings.Endpoint)
		o.UsePathStyle = true
		o.APIOptions = append(o.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
	})
	return &R2{client: client, signer: s3.NewPresignClient(client), bucket: settings.Bucket}, nil
}
func (r *R2) Begin(ctx context.Context, key, mediaType string) (Multipart, error) {
	if !validKey(key) {
		return Multipart{}, invalid("INVALID_STORAGE_KEY")
	}
	out, err := r.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(r.bucket), Key: aws.String(key), ContentType: aws.String(mediaType)})
	if err != nil {
		return Multipart{}, failure(err, true)
	}
	m := Multipart{Key: key, UploadID: aws.ToString(out.UploadId)}
	if err = validateMultipart(m); err != nil {
		return Multipart{}, err
	}
	return m, nil
}
func (r *R2) SignPart(ctx context.Context, m Multipart, number int, size int64, ttl time.Duration) (Signed, error) {
	if err := signInput(m, number, size, ttl); err != nil {
		return Signed{}, err
	}
	out, err := r.signer.PresignUploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID), PartNumber: aws.Int32(int32(number)), ContentLength: aws.Int64(size)}, func(o *s3.PresignOptions) { o.Expires = ttl })
	if err != nil {
		return Signed{}, failure(err, false)
	}
	return Signed{Method: out.Method, URL: out.URL, Headers: signedHeaders(out.SignedHeader), ExpiresAt: time.Now().UTC().Add(ttl)}, nil
}
func (r *R2) Parts(ctx context.Context, m Multipart, marker string) (PartPage, error) {
	if err := validateMultipart(m); err != nil {
		return PartPage{}, err
	}
	if _, err := markerValue(marker); err != nil {
		return PartPage{}, err
	}
	in := &s3.ListPartsInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID), MaxParts: aws.Int32(1000)}
	if marker != "" {
		in.PartNumberMarker = aws.String(marker)
	}
	out, err := r.client.ListParts(ctx, in)
	if err != nil {
		return PartPage{}, failure(err, false)
	}
	page := PartPage{Items: []Part{}}
	for _, part := range out.Parts {
		page.Items = append(page.Items, Part{Number: int(aws.ToInt32(part.PartNumber)), Size: aws.ToInt64(part.Size), ETag: aws.ToString(part.ETag)})
	}
	if aws.ToBool(out.IsTruncated) {
		page.NextMarker = aws.ToString(out.NextPartNumberMarker)
		next, markerErr := markerValue(page.NextMarker)
		previous, _ := markerValue(marker)
		if markerErr != nil || next <= previous {
			return PartPage{}, invalid("INVALID_PROVIDER_PAGINATION")
		}
	}
	return page, nil
}
func (r *R2) Complete(ctx context.Context, m Multipart, parts []Part) (Source, error) {
	if err := validateMultipart(m); err != nil {
		return Source{}, err
	}
	ordered, size, err := completionParts(parts)
	if err != nil {
		return Source{}, err
	}
	completed := make([]types.CompletedPart, len(ordered))
	for i, part := range ordered {
		completed[i] = types.CompletedPart{PartNumber: aws.Int32(int32(part.Number)), ETag: aws.String(part.ETag)}
	}
	out, err := r.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID), MultipartUpload: &types.CompletedMultipartUpload{Parts: completed}})
	if err != nil {
		return Source{}, failure(err, true)
	}
	if !validETag(aws.ToString(out.ETag)) {
		return Source{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	return Source{Key: m.Key, ETag: aws.ToString(out.ETag), VersionID: aws.ToString(out.VersionId), Size: size}, nil
}
func (r *R2) Abort(ctx context.Context, m Multipart) error {
	if err := validateMultipart(m); err != nil {
		return err
	}
	_, err := r.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID)})
	mapped := failure(err, true)
	if e, ok := mapped.(*Error); ok && e.Code == "OBJECT_NOT_FOUND" {
		return nil
	}
	return mapped
}
func (r *R2) Head(ctx context.Context, key string) (Source, error) {
	if !validKey(key) {
		return Source{}, invalid("INVALID_STORAGE_KEY")
	}
	out, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		return Source{}, failure(err, false)
	}
	source := Source{Key: key, ETag: aws.ToString(out.ETag), VersionID: aws.ToString(out.VersionId), Size: aws.ToInt64(out.ContentLength)}
	if source.Size < 0 || source.Size > MaxObjectBytes || !validETag(source.ETag) {
		return Source{}, invalid("INVALID_PROVIDER_SOURCE")
	}
	return source, nil
}
func (r *R2) Read(ctx context.Context, source Source, offset, length int64) (ReadResult, error) {
	if err := validateSource(source, offset, length); err != nil {
		return ReadResult{}, err
	}
	in := &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(source.Key), IfMatch: aws.String(source.ETag), Range: aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))}
	// R2 has no native S3 object versioning. A nonempty native version is rejected.
	if source.VersionID != "" {
		return ReadResult{}, invalid("R2_NATIVE_VERSION_UNSUPPORTED")
	}
	out, err := r.client.GetObject(ctx, in)
	if err != nil {
		return ReadResult{}, failure(err, false)
	}
	if err = checkRead(out.Body, aws.ToString(out.ETag), aws.ToString(out.ContentRange), "", aws.ToInt64(out.ContentLength), source, offset, length); err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Body: out.Body, Size: length, ETag: source.ETag}, nil
}
func (r *R2) WritePart(ctx context.Context, m Multipart, number int, body io.Reader, size int64) (Part, error) {
	if err := validatePart(m, number, size); err != nil {
		return Part{}, err
	}
	if body == nil {
		return Part{}, invalid("INVALID_PART_BODY")
	}
	out, err := r.client.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID), PartNumber: aws.Int32(int32(number)), ContentLength: aws.Int64(size), Body: io.LimitReader(body, size)})
	if err != nil {
		return Part{}, failure(err, true)
	}
	etag := aws.ToString(out.ETag)
	if !validETag(etag) {
		return Part{}, invalid("INVALID_PROVIDER_ETAG")
	}
	return Part{Number: number, Size: size, ETag: etag}, nil
}

var _ Store = (*R2)(nil)
