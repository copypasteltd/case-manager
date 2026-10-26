package objectstore

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDownloadSignaturesPinFormalSourceAndResponse(t *testing.T) {
	for _, name := range []string{"oss", "r2"} {
		t.Run(name, func(t *testing.T) {
			store := providerFixture(t, name, transportFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("presigning must not perform network requests")
				return nil, errors.New("unexpected network")
			})).(DownloadSigner)
			key, _ := VersionKey("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
			source := Source{Key: key, ETag: `"fixed-multipart-etag"`, Size: MaxObjectBytes}
			if name == "oss" {
				source.VersionID = "native-version"
			}
			signed, err := store.SignRead(context.Background(), source, ReadOptions{Filename: "合同 原件.pdf", Disposition: "attachment", MediaType: "application/pdf"}, DownloadSignTTL)
			if err != nil {
				t.Fatal(err)
			}
			address, err := url.Parse(signed.URL)
			headers := http.Header{}
			for k, v := range signed.Headers {
				headers.Set(k, v)
			}
			if err != nil || signed.Method != "GET" || address.Scheme != "https" || !strings.HasSuffix(address.Path, key) ||
				headers.Get("If-Match") != source.ETag || address.Query().Get("response-content-type") != "application/octet-stream" ||
				!strings.HasPrefix(address.Query().Get("response-content-disposition"), "attachment; filename*=") ||
				address.Query().Get("response-cache-control") != "private, no-store" {
				t.Fatal("read grant lost source conditions or response constraints")
			}
			if name == "oss" && (address.Query().Get("versionId") != source.VersionID || !strings.Contains(address.Query().Get("x-oss-additional-headers"), "if-match")) {
				t.Fatal("OSS signature did not include immutable native version and condition")
			}
			if name == "r2" && !strings.Contains(address.Query().Get("X-Amz-SignedHeaders"), "if-match") {
				t.Fatal("R2 condition was not covered by signature")
			}
			source.Key = fixtureKey()
			if _, err = store.SignRead(context.Background(), source, ReadOptions{Filename: "draft", Disposition: "attachment"}, DownloadSignTTL); err == nil {
				t.Fatal("draft became downloadable")
			}
			source.Key = key
			for _, ttl := range []time.Duration{0, DownloadSignTTL + time.Second} {
				if _, err = store.SignRead(context.Background(), source, ReadOptions{Filename: "file", Disposition: "attachment"}, ttl); err == nil {
					t.Fatal("unsafe grant lifetime")
				}
			}
		})
	}
}
func TestDownloadResponseOptionsRejectActiveContentAndHeaderInjection(t *testing.T) {
	for _, options := range []ReadOptions{
		{Filename: "a\r\nSet-Cookie: secret", Disposition: "attachment"},
		{Filename: "../file", Disposition: "attachment"},
		{Filename: "active.svg", Disposition: "inline", MediaType: "image/svg+xml"},
		{Filename: "active.html", Disposition: "inline", MediaType: "text/html"},
	} {
		if _, _, err := options.Headers(); err == nil {
			t.Fatal("unsafe response headers accepted")
		}
	}
	if _, media, err := (ReadOptions{Filename: "safe.pdf", Disposition: "inline", MediaType: "application/pdf"}).Headers(); err != nil || media != "application/pdf" {
		t.Fatal("safe inline response rejected")
	}
}
