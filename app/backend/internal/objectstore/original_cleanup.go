package objectstore

import (
	"context"
	"errors"
	"io"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

func exactOriginalAbsent(e error) bool {
	var sdk smithy.APIError
	var aliyun *oss.ServiceError
	code := ""
	if errors.As(e, &sdk) {
		code = sdk.ErrorCode()
	}
	if errors.As(e, &aliyun) {
		code = aliyun.Code
	}
	// A naked HTTP404/NotFound may mean a missing bucket or proxy. Only an exact
	// object/version error from a bounded signed GET establishes absence.
	return code == "NoSuchKey" || code == "NoSuchVersion"
}
func originalInput(source Source) error {
	if e := validateSource(source, 0, 1); e != nil {
		return e
	}
	return nil
}
func (r *OSS) inspectOriginal(ctx context.Context, source Source) (bool, error) {
	out, e := r.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(source.Key), VersionId: oss.Ptr(source.VersionID), IfMatch: oss.Ptr(source.ETag), Range: oss.Ptr("bytes=0-0"), RangeBehavior: oss.Ptr("standard")})
	if exactOriginalAbsent(e) {
		return true, nil
	}
	if e != nil {
		return false, failure(e, false)
	}
	if e = checkRead(out.Body, oss.ToString(out.ETag), oss.ToString(out.ContentRange), oss.ToString(out.VersionId), out.ContentLength, source, 0, 1); e != nil {
		return false, e
	}
	defer out.Body.Close()
	n, e := io.CopyN(io.Discard, out.Body, 1)
	if e != nil || n != 1 {
		return false, invalid("INVALID_PROVIDER_SOURCE")
	}
	return false, nil
}
func (r *OSS) RemoveFrozen(ctx context.Context, source Source) (bool, error) {
	if e := originalInput(source); e != nil {
		return false, e
	}
	// Without an actual native version, DeleteObject could merely create a delete
	// marker. Never describe that as removal of the retained data version.
	if source.VersionID == "" || source.VersionID == "null" {
		return false, invalid("NATIVE_OBJECT_VERSION_REQUIRED")
	}
	absent, e := r.inspectOriginal(ctx, source)
	if e != nil || absent {
		return absent, e
	}
	_, e = r.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(r.bucket), Key: oss.Ptr(source.Key), VersionId: oss.Ptr(source.VersionID)})
	if e != nil {
		return false, failure(e, true)
	}
	return r.inspectOriginal(ctx, source)
}
func (r *R2) inspectOriginal(ctx context.Context, source Source) (bool, error) {
	out, e := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(source.Key), IfMatch: aws.String(source.ETag), Range: aws.String("bytes=0-0")})
	if exactOriginalAbsent(e) {
		return true, nil
	}
	if e != nil {
		return false, failure(e, false)
	}
	if e = checkRead(out.Body, aws.ToString(out.ETag), aws.ToString(out.ContentRange), aws.ToString(out.VersionId), aws.ToInt64(out.ContentLength), source, 0, 1); e != nil {
		return false, e
	}
	defer out.Body.Close()
	n, e := io.CopyN(io.Discard, out.Body, 1)
	if e != nil || n != 1 {
		return false, invalid("INVALID_PROVIDER_SOURCE")
	}
	return false, nil
}

// RemoveFrozen on R2 is usable only behind the domain's permanent key barrier,
// after bounded writers have drained and with an operator-attested exclusive
// managed namespace. R2 documents no native versioning / DeleteObject CAS. The
// additional If-Match header is defense in depth, never claimed as proven CAS.
func (r *R2) RemoveFrozen(ctx context.Context, source Source) (bool, error) {
	if e := originalInput(source); e != nil {
		return false, e
	}
	if source.VersionID != "" {
		return false, invalid("R2_NATIVE_VERSION_UNSUPPORTED")
	}
	absent, e := r.inspectOriginal(ctx, source)
	if e != nil || absent {
		return absent, e
	}
	_, e = r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(source.Key), IfMatch: aws.String(source.ETag)})
	if e != nil {
		return false, failure(e, true)
	}
	return r.inspectOriginal(ctx, source)
}
