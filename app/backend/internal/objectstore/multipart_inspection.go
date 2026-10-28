package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"copypaste.hk/platform/internal/core"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// LocationFingerprint binds non-secret provider/account endpoint and bucket
// metadata. It never constructs a client or grants a process cloud access.
func LocationFingerprint(provider string, s Settings) (string, error) {
	if provider != "oss" && provider != "r2" {
		return "", invalid("INVALID_STORAGE_CONFIGURATION")
	}
	checked := s
	checked.AccessKeyID = "location-validation-only"
	checked.AccessKeySecret = "location-validation-only"
	if e := settingsValid(checked, provider); e != nil {
		return "", e
	}
	raw, _ := json.Marshal([]string{provider, strings.TrimRight(strings.ToLower(s.Endpoint), "/"), s.Region, s.Bucket})
	return core.Digest(raw), nil
}
func noSuchMultipart(err error) bool {
	var sdk smithy.APIError
	var aliyun *oss.ServiceError
	return (errors.As(err, &sdk) && sdk.ErrorCode() == "NoSuchUpload") || (errors.As(err, &aliyun) && aliyun.Code == "NoSuchUpload")
}

// Only NoSuchUpload proves multipart absence. Generic 404/NoSuchKey, missing
// buckets, denial and empty successful listings never establish this result.
func (r *R2) MultipartAbsent(ctx context.Context, m Multipart) (bool, error) {
	if e := validateMultipart(m); e != nil {
		return false, e
	}
	_, e := r.client.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(r.bucket), Key: aws.String(m.Key), UploadId: aws.String(m.UploadID), MaxParts: aws.Int32(1)})
	if noSuchMultipart(e) {
		return true, nil
	}
	return false, failure(e, false)
}
func (r *OSS) MultipartAbsent(ctx context.Context, m Multipart) (bool, error) {
	if e := validateMultipart(m); e != nil {
		return false, e
	}
	_, e := r.client.ListParts(ctx, &oss.ListPartsRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(m.Key), UploadId: oss.Ptr(m.UploadID), MaxParts: 1})
	if noSuchMultipart(e) {
		return true, nil
	}
	return false, failure(e, false)
}
