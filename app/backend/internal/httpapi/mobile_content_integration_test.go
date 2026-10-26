package httpapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/objectstore"
)

func allowMobileContent(f *uploadFixture) {
	t := f.f.t
	admin := f.f.administratorToken()
	rec, res := f.f.request("GET", "/v1/classification-policies", admin, "", nil, nil)
	expect(t, rec, 200, res)
	for _, policy := range data[core.Page[identity.ClassificationPolicy]](t, res).Items {
		if policy.Level == "confidential" {
			continue
		}
		channels := map[string]bool{}
		for _, channel := range policy.AllowedChannels {
			channels[channel] = true
		}
		if len(channels) == 4 && channels["web"] && channels["android"] && channels["wechat"] && channels["douyin"] {
			continue
		}
		rec, res = f.f.request("POST", "/v1/classification-policies", admin, core.NewID(), identity.ClassificationPolicyInput{ExpectedVersion: policy.Version, BasePolicyID: policy.ID, Level: policy.Level, AIAllowed: policy.AIAllowed, AllowedChannels: []string{"web", "android", "wechat", "douyin"}, Reason: "Isolated native mobile fixed-content fixture"}, nil)
		expect(t, rec, 201, res)
	}
}

func TestMobileContentChunks(t *testing.T) {
	for _, channel := range []string{"wechat", "douyin"} {
		t.Run(channel+" binds a proxy permit to current access and never signs a cloud URL", func(t *testing.T) {
			f, committed, cloud := downloadFixture(t, mobileChunkBytes+7, false)
			allowMobileContent(f)
			old := f.f.aliceToken
			f.f.aliceToken = f.f.login("alice", channel).AccessToken
			grant := createDownload(t, f, committed.Version.ID)
			if grant.Mode != "authenticated_proxy" || len(grant.RequiredHeaders) != 0 || cloud.signs.Load() != 0 {
				t.Fatal("mobile permit exposed a cloud signature")
			}
			read := func(token, query, condition string) (*downloadChunk, int) {
				rec, res := f.f.request("GET", "/v1/downloads/"+grant.ID+"/chunks?"+query, token, "", nil, map[string]string{"If-Range": condition})
				if rec.Code != 200 {
					return nil, rec.Code
				}
				value := data[downloadChunk](t, res)
				if rec.Header().Get("Cache-Control") != "private, no-store" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
					t.Fatal("content response was cacheable or binary")
				}
				return &value, rec.Code
			}
			for _, span := range [][2]int{{0, mobileChunkBytes}, {mobileChunkBytes, 7}} {
				value, status := read(f.f.aliceToken, fmt.Sprintf("offset=%d&length=%d", span[0], span[1]), `"`+grant.SHA256+`"`)
				if status != 200 || value.DownloadID != grant.ID || value.VersionID != committed.Version.ID || len(value.Data) != span[1] || value.ChunkSHA256 != zeroSHA(int64(span[1])) || value.SHA256 != grant.SHA256 || value.Total != grant.SizeBytes {
					t.Fatal("chunk did not preserve the exact source", status)
				}
			}
			if cloud.reads.Load() != 2 || cloud.closed.Load() != 2 {
				t.Fatal("source was not bounded and closed")
			}
			for _, tc := range []struct {
				token, query, condition string
				status                  int
			}{
				{"", "offset=0&length=1", `"` + grant.SHA256 + `"`, 401},
				{old, "offset=0&length=1", `"` + grant.SHA256 + `"`, 404},
				{f.f.aliceToken, "offset=0&length=1", "", 428},
				{f.f.aliceToken, "offset=0&length=1", `"` + strings.Repeat("a", 64) + `"`, 412},
				{f.f.aliceToken, "offset=00&length=1", `"` + grant.SHA256 + `"`, 416},
				{f.f.aliceToken, "offset=0&length=1048577", `"` + grant.SHA256 + `"`, 416},
				{f.f.aliceToken, "offset=0&length=0", `"` + grant.SHA256 + `"`, 416},
				{f.f.aliceToken, "offset=0&length=1&length=1", `"` + grant.SHA256 + `"`, 400},
				{f.f.aliceToken, "offset=0&length=1&url=https%3A%2F%2Fforbidden.invalid", `"` + grant.SHA256 + `"`, 400},
				{f.f.aliceToken, "offset=9223372036854775807&length=1", `"` + grant.SHA256 + `"`, 416},
			} {
				if _, status := read(tc.token, tc.query, tc.condition); status != tc.status {
					t.Fatalf("range boundary %q: %d, want %d", tc.query, status, tc.status)
				}
			}
			if cloud.reads.Load() != 2 {
				t.Fatal("rejected request touched cloud content")
			}
			if _, err := f.f.owner.Exec(context.Background(), "DELETE FROM project_members WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice); err != nil {
				t.Fatal(err)
			}
			if _, status := read(f.f.aliceToken, "offset=0&length=1", `"`+grant.SHA256+`"`); status != 404 {
				t.Fatal("revoked membership read content", status)
			}
		})
	}
	t.Run("truncated bytes and revocation during buffering never escape as a partial JSON chunk", func(t *testing.T) {
		f, committed, cloud := downloadFixture(t, 70000, true)
		grant := createDownload(t, f, committed.Version.ID)
		cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
			return objectstore.ReadResult{Body: io.NopCloser(io.LimitReader(generatedZeros{ctx}, length-1)), Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
		}
		rec, res := f.f.request("GET", "/v1/downloads/"+grant.ID+"/chunks?offset=0&length=70000", f.f.aliceToken, "", nil, map[string]string{"If-Range": `"` + grant.SHA256 + `"`})
		expect(t, rec, 502, res)
		if len(res.Data) > 0 || strings.Contains(rec.Body.String(), "data_base64") {
			t.Fatal("partial provider bytes escaped")
		}
		cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
			reader := &onContentRead{Reader: io.LimitReader(generatedZeros{ctx}, length), hook: func() {
				if _, err := f.f.owner.Exec(ctx, "DELETE FROM resource_grants WHERE org_id=$1 AND user_id=$2 AND resource_id=$3", f.f.org, f.f.alice, committed.File.ID); err != nil {
					t.Error(err)
				}
				if _, err := f.f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice); err != nil {
					t.Error(err)
				}
			}}
			return objectstore.ReadResult{Body: io.NopCloser(reader), Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
		}
		rec, res = f.f.request("GET", "/v1/downloads/"+grant.ID+"/chunks?offset=0&length=70000", f.f.aliceToken, "", nil, map[string]string{"If-Range": `"` + grant.SHA256 + `"`})
		expect(t, rec, 404, res)
		var leaked bool
		if err := f.f.owner.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM download_read_events WHERE permit_id=$1 AND stage<>'started' AND bytes_written>0)", grant.ID).Scan(&leaked); err != nil || leaked {
			t.Fatal("buffered content was falsely audited as delivered", err)
		}
	})
	t.Run("a signed web permit cannot be turned into an authenticated proxy", func(t *testing.T) {
		f, committed, _ := downloadFixture(t, 15, false)
		grant := createDownload(t, f, committed.Version.ID)
		rec, res := f.f.request("GET", "/v1/downloads/"+grant.ID+"/chunks?offset=0&length=1", f.f.aliceToken, "", nil, map[string]string{"If-Range": `"` + grant.SHA256 + `"`})
		expect(t, rec, 409, res)
	})
}

type onContentRead struct {
	io.Reader
	hook func()
}

func (r *onContentRead) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if r.hook != nil {
		hook := r.hook
		r.hook = nil
		hook()
	}
	return n, err
}

// The digest oracle is the standard library, independent of the browser reader.
func contentSHA(bytes []byte) string { return fmt.Sprintf("%x", sha256.Sum256(bytes)) }
