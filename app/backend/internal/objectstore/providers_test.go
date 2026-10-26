package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func providerFixture(t *testing.T, name string, transport http.RoundTripper) Store {
	t.Helper()
	settings := Settings{Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou", Bucket: "unit-fixture", AccessKeyID: "unit-key", AccessKeySecret: "unit-secret-do-not-log"}
	client := &http.Client{Transport: transport}
	var store Store
	var err error
	if name == "oss" {
		store, err = newOSS(settings, client)
	} else {
		settings.Endpoint = "https://unit-account.r2.cloudflarestorage.com"
		store, err = newR2(settings, client)
	}
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func fixtureKey() string {
	key, _ := DraftKey("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222")
	return key
}
func TestProviderSigningKeepsClientInsideDraftMultipart(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			network := 0
			store := providerFixture(t, name, transportFunc(func(*http.Request) (*http.Response, error) {
				network++
				return nil, errors.New("network must not be used for signing")
			}))
			m := Multipart{Key: fixtureKey(), UploadID: "owned-upload-id"}
			signed, err := store.SignPart(context.Background(), m, 3, DefaultPartBytes, 5*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			address, err := url.Parse(signed.URL)
			if err != nil || signed.Method != "PUT" || address.Scheme != "https" || address.Query().Get("partNumber") != "3" || address.Query().Get("uploadId") != m.UploadID || !strings.Contains(address.Path, m.Key) || !signed.ExpiresAt.After(time.Now()) {
				t.Fatal("signature lost multipart scope")
			}
			if network != 0 || strings.Contains(signed.URL, "unit-secret-do-not-log") {
				t.Fatal("signing made a request or disclosed secret")
			}
			for header := range signed.Headers {
				if strings.EqualFold(header, "host") || strings.EqualFold(header, "content-length") {
					t.Fatal("browser-forbidden header returned")
				}
			}
			candidate, _ := CandidateKey("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222")
			if _, err = store.SignPart(context.Background(), Multipart{Key: candidate, UploadID: m.UploadID}, 1, 1, time.Minute); err == nil {
				t.Fatal("client obtained a candidate write signature")
			}
			for _, ttl := range []time.Duration{0, MaxSignTTL + time.Second} {
				if _, err = store.SignPart(context.Background(), m, 1, 1, ttl); err == nil {
					t.Fatal("unsafe signature expiry accepted")
				}
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }
func TestProviderReadPinsETagRangeAndRejectsDifferentBytes(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			bad := false
			var last *trackedBody
			store := providerFixture(t, name, transportFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("If-Match") != `"source-etag"` || request.Header.Get("Range") != "bytes=3-5" {
					t.Fatal("source read lost exact version condition")
				}
				last = &trackedBody{Reader: strings.NewReader("abc")}
				etag := `"source-etag"`
				if bad {
					etag = `"replaced"`
				}
				headers := http.Header{"Content-Length": []string{"3"}, "Content-Range": []string{"bytes 3-5/9"}, "Etag": []string{etag}}
				return &http.Response{StatusCode: 206, Status: "206 Partial Content", Header: headers, Body: last, ContentLength: 3, Request: request}, nil
			}))
			source := Source{Key: fixtureKey(), ETag: `"source-etag"`, Size: 9}
			read, err := store.Read(context.Background(), source, 3, 3)
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(read.Body)
			if err != nil || string(content) != "abc" {
				t.Fatal(err)
			}
			read.Body.Close()
			if !last.closed {
				t.Fatal("read body did not close")
			}
			bad = true
			if _, err = store.Read(context.Background(), source, 3, 3); err == nil || !last.closed {
				t.Fatal("replaced source accepted or error leaked a body")
			}
		})
	}
}
func TestProviderErrorsDoNotDiscloseSignedRequests(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			requests := 0
			store := providerFixture(t, name, transportFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				return nil, fmt.Errorf("private-signature=%s Authorization=%s", request.URL.String(), request.Header.Get("Authorization"))
			}))
			_, err := store.Begin(context.Background(), fixtureKey(), "application/octet-stream")
			var safe *Error
			if !errors.As(err, &safe) || !safe.Uncertain || !safe.Retryable || strings.Contains(err.Error(), "private-signature") || strings.Contains(err.Error(), "Authorization") || requests != 1 {
				t.Fatal("provider failure was retried blindly or disclosed credentials")
			}
		})
	}
	settings := Settings{AccessKeyID: "unit-key", AccessKeySecret: "unit-secret-do-not-log", SessionToken: "session-private"}
	encoded, _ := json.Marshal(settings)
	for _, value := range []string{fmt.Sprint(settings), fmt.Sprintf("%#v", settings), string(encoded)} {
		if strings.Contains(value, "unit-secret") || strings.Contains(value, "session-private") || strings.Contains(value, "unit-key") {
			t.Fatal("settings serialization disclosed credentials")
		}
	}
}
func TestProviderPartsStayPagedAndUploadStreamsHaveKnownLength(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			store := providerFixture(t, name, transportFunc(func(request *http.Request) (*http.Response, error) {
				headers := http.Header{"Content-Type": []string{"application/xml"}}
				xml := ""
				switch request.Method {
				case "GET":
					if request.URL.Query().Get("max-parts") != "1000" || request.URL.Query().Get("part-number-marker") != "2" {
						t.Fatal("part listing lost bound or marker")
					}
					xml = `<ListPartsResult><IsTruncated>true</IsTruncated><NextPartNumberMarker>3</NextPartNumberMarker><Part><PartNumber>3</PartNumber><Size>7</Size><ETag>"part"</ETag></Part></ListPartsResult>`
				case "PUT":
					if request.ContentLength != 7 {
						t.Fatal("stream length is not bounded", request.ContentLength)
					}
					bytes, err := io.ReadAll(request.Body)
					if err != nil || string(bytes) != "payload" {
						t.Fatal("stream body wrong", err)
					}
					headers.Set("ETag", `"uploaded"`)
				default:
					t.Fatal("unexpected provider operation", request.Method)
				}
				return &http.Response{StatusCode: 200, Status: "200 OK", Header: headers, Body: io.NopCloser(strings.NewReader(xml)), ContentLength: int64(len(xml)), Request: request}, nil
			}))
			m := Multipart{Key: fixtureKey(), UploadID: "owned-upload-id"}
			page, err := store.Parts(context.Background(), m, "2")
			if err != nil || len(page.Items) != 1 || page.Items[0].Size != 7 || page.NextMarker != "3" {
				t.Fatal("paged part evidence lost", page, err)
			}
			part, err := store.WritePart(context.Background(), m, 1, strings.NewReader("payload"), 7)
			if err != nil || part.Size != 7 || part.ETag != `"uploaded"` {
				t.Fatal(part, err)
			}
		})
	}
}
func TestProviderCompleteOrdersExactManifestAndAbortIsIdempotent(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			store := providerFixture(t, name, transportFunc(func(request *http.Request) (*http.Response, error) {
				status := 200
				headers := http.Header{"Content-Type": []string{"application/xml"}}
				xml := ""
				if request.Method == "POST" {
					content, err := io.ReadAll(request.Body)
					if err != nil {
						t.Fatal(err)
					}
					manifest := string(content)
					if strings.Index(manifest, "<PartNumber>1</PartNumber>") < 0 || strings.Index(manifest, "<PartNumber>1</PartNumber>") > strings.Index(manifest, "<PartNumber>2</PartNumber>") {
						t.Fatal("completion manifest not canonical")
					}
					if name == "oss" && request.Header.Get("x-oss-forbid-overwrite") != "true" {
						t.Fatal("OSS completion permits overwriting a unique key")
					}
					xml = `<CompleteMultipartUploadResult><ETag>"completed"</ETag></CompleteMultipartUploadResult>`
				} else if request.Method == "DELETE" {
					status = 404
					xml = `<Error><Code>NoSuchUpload</Code><Message>private provider diagnostic</Message><RequestId>unit</RequestId></Error>`
				} else {
					t.Fatal("unexpected provider operation", request.Method)
				}
				return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: headers, Body: io.NopCloser(strings.NewReader(xml)), ContentLength: int64(len(xml)), Request: request}, nil
			}))
			m := Multipart{Key: fixtureKey(), UploadID: "owned-upload-id"}
			source, err := store.Complete(context.Background(), m, []Part{{Number: 2, Size: 1, ETag: "last"}, {Number: 1, Size: MinPartBytes, ETag: "first"}})
			if err != nil || source.Size != MinPartBytes+1 || source.Key != m.Key || source.ETag != `"completed"` {
				t.Fatal("completion source lost immutable evidence", source, err)
			}
			if err = store.Abort(context.Background(), m); err != nil {
				t.Fatal("already absent multipart is not idempotent", err)
			}
		})
	}
}
