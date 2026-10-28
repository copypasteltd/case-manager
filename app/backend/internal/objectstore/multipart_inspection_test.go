package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMultipartAbsenceRequiresExactProviderEvidence(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		for _, code := range []string{"", "NoSuchUpload", "NoSuchKey", "NotFound", "NoSuchBucket", "AccessDenied"} {
			t.Run(provider+"/"+code, func(t *testing.T) {
				calls := 0
				store := providerFixture(t, provider, transportFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Method != "GET" || r.URL.Query().Get("uploadId") != "exact-upload" || r.URL.Query().Get("max-parts") != "1" || !strings.HasSuffix(r.URL.Path, fixtureKey()) {
						t.Fatal("inspection escaped exact multipart", r.Method, r.URL.Path, r.URL.RawQuery)
					}
					status := 200
					body := "<ListPartsResult><IsTruncated>false</IsTruncated></ListPartsResult>"
					if code != "" {
						status = 404
						if code == "AccessDenied" {
							status = 403
						}
						body = fmt.Sprintf("<Error><Code>%s</Code><Message>opaque provider payload</Message></Error>", code)
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				}))
				checker := store.(interface {
					MultipartAbsent(context.Context, Multipart) (bool, error)
				})
				absent, e := checker.MultipartAbsent(context.Background(), Multipart{Key: fixtureKey(), UploadID: "exact-upload"})
				if absent != (code == "NoSuchUpload") || ((code == "" || code == "NoSuchUpload") != (e == nil)) || calls != 1 {
					t.Fatal("unproven absence accepted", absent, e, calls)
				}
			})
		}
	}
}
func TestStorageLocationBindingExcludesCredentialsButBindsAccountAndBucket(t *testing.T) {
	s := Settings{Endpoint: "https://fixture.r2.cloudflarestorage.com", Region: "auto", Bucket: "fixture-bucket"}
	first, e := LocationFingerprint("r2", s)
	if e != nil || len(first) != 64 {
		t.Fatal(e)
	}
	s.AccessKeyID = "rotated"
	s.AccessKeySecret = "rotated-secret"
	second, e := LocationFingerprint("r2", s)
	if e != nil || first != second {
		t.Fatal("credential rotation changed physical binding")
	}
	s.Endpoint = "https://other.r2.cloudflarestorage.com"
	third, e := LocationFingerprint("r2", s)
	if e != nil || first == third {
		t.Fatal("account endpoint not bound")
	}
	s.Bucket = "other-bucket"
	fourth, e := LocationFingerprint("r2", s)
	if e != nil || third == fourth {
		t.Fatal("bucket not bound")
	}
	for _, endpoint := range []string{"http://fixture.r2.cloudflarestorage.com", "https://other.invalid", "https://user:pass@fixture.r2.cloudflarestorage.com", "https://fixture.r2.cloudflarestorage.com/?x=1"} {
		s.Endpoint = endpoint
		if _, e = LocationFingerprint("r2", s); e == nil {
			t.Fatal("invalid location accepted", endpoint)
		}
	}
}
