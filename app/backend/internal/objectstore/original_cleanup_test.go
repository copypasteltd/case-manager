package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func originalResponse(r *http.Request, status int, headers http.Header, body string) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: headers, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}
}
func TestOriginalCleanupExactProviderOperations(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		for _, scenario := range []string{"remove", "already_absent", "naked_404", "missing_bucket", "changed_source", "delete_ack_without_absence"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				source := Source{Key: fixtureKey(), ETag: `"fixed-original"`, Size: 15}
				if provider == "oss" {
					source.VersionID = "specific-native-version"
				}
				removed := false
				deletes := 0
				store := providerFixture(t, provider, transportFunc(func(r *http.Request) (*http.Response, error) {
					if !strings.HasSuffix(r.URL.Path, source.Key) {
						t.Fatal("wrong exact object")
					}
					if provider == "oss" && r.URL.Query().Get("versionId") != source.VersionID {
						t.Fatal("native version not pinned")
					}
					if r.Method == "DELETE" {
						deletes++
						if provider == "r2" && r.Header.Get("If-Match") != source.ETag {
							t.Fatal("missing defense-in-depth condition")
						}
						if scenario != "delete_ack_without_absence" {
							removed = true
						}
						return originalResponse(r, 204, nil, ""), nil
					}
					if r.Method != "GET" || r.Header.Get("Range") != "bytes=0-0" || r.Header.Get("If-Match") != source.ETag {
						t.Fatal("inspection must be bounded and pinned")
					}
					code := ""
					switch scenario {
					case "already_absent":
						code = "NoSuchKey"
					case "naked_404":
						return originalResponse(r, 404, nil, ""), nil
					case "missing_bucket":
						code = "NoSuchBucket"
					}
					if removed {
						code = "NoSuchKey"
						if provider == "oss" {
							code = "NoSuchVersion"
						}
					}
					if code != "" {
						return originalResponse(r, 404, http.Header{"Content-Type": {"application/xml"}}, "<Error><Code>"+code+"</Code><Message>synthetic</Message><RequestId>synthetic</RequestId></Error>"), nil
					}
					etag := source.ETag
					if scenario == "changed_source" {
						etag = `"different-original"`
					}
					headers := http.Header{"Content-Length": {"1"}, "Content-Range": {"bytes 0-0/15"}, "Etag": {etag}}
					if provider == "oss" {
						headers.Set("x-oss-version-id", source.VersionID)
					}
					return originalResponse(r, 206, headers, "x"), nil
				}))
				cleaner := store.(interface {
					RemoveFrozen(context.Context, Source) (bool, error)
				})
				absent, e := cleaner.RemoveFrozen(context.Background(), source)
				if scenario == "remove" || scenario == "already_absent" {
					if e != nil || !absent || deletes != map[bool]int{true: 1, false: 0}[scenario == "remove"] {
						t.Fatal(absent, e, deletes)
					}
				} else if scenario == "delete_ack_without_absence" {
					if absent || deletes != 1 {
						t.Fatal("delete ack invented absence")
					}
				} else {
					if e == nil || absent || deletes != 0 {
						t.Fatal("uncertain or changed source removed", absent, e, deletes)
					}
				}
			})
		}
	}
}
func TestOriginalCleanupRejectsUnversionedOSSAndVersionedR2(t *testing.T) {
	for _, provider := range []string{"oss", "r2"} {
		t.Run(provider, func(t *testing.T) {
			store := providerFixture(t, provider, transportFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid source reached provider")
				return nil, nil
			}))
			source := Source{Key: fixtureKey(), ETag: `"fixed"`, Size: 15}
			if provider == "r2" {
				source.VersionID = "unsupported-version"
			}
			_, e := store.(interface {
				RemoveFrozen(context.Context, Source) (bool, error)
			}).RemoveFrozen(context.Background(), source)
			if e == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}
