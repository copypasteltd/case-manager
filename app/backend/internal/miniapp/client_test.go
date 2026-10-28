package miniapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfficialMiniappExchange(t *testing.T) {
	for _, platform := range []string{"wechat", "douyin"} {
		t.Run(platform, func(t *testing.T) {
			seen := 0
			payload := `{"openid":"synthetic-subject","session_key":"private-session-key"}`
			if platform == "douyin" {
				payload = `{"err_no":0,"data":{"openid":"synthetic-subject","session_key":"private-session-key"}}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen++
				if platform == "wechat" {
					if r.Method != "GET" || r.URL.Path != "/sns/jscode2session" || r.URL.Query().Get("js_code") != "single-use-code" || r.URL.Query().Get("grant_type") != "authorization_code" || r.URL.Query().Get("secret") != "server-only-secret" {
						t.Error("wrong WeChat exchange")
					}
				} else {
					var body map[string]string
					if json.NewDecoder(r.Body).Decode(&body) != nil || r.Method != "POST" || r.URL.Path != "/api/apps/v2/jscode2session" || body["appid"] != "tt-synthetic-app" || body["code"] != "single-use-code" || body["secret"] != "server-only-secret" || r.URL.RawQuery != "" {
						t.Error("wrong Douyin exchange")
					}
				}
				io.WriteString(w, payload)
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			client := Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				expected := "api.weixin.qq.com"
				if platform == "douyin" {
					expected = "developer.toutiao.com"
				}
				if r.URL.Scheme != "https" || r.URL.Host != expected {
					t.Fatal("endpoint is not fixed official HTTPS")
				}
				clone := r.Clone(r.Context())
				u := *r.URL
				u.Scheme, u.Host = target.Scheme, target.Host
				clone.URL = &u
				return http.DefaultTransport.RoundTrip(clone)
			})}
			id, err := client.Exchange(context.Background(), platform, Profile{AppID: "tt-synthetic-app", Secret: "server-only-secret"}, "single-use-code")
			if err != nil || id != "synthetic-subject" || seen != 1 {
				t.Fatal("native synthetic HTTP exchange failed")
			}
		})
	}
}
func TestMiniappProviderFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, platform, body string
		status               int
	}{
		{"provider-error", "wechat", `{"errcode":40029,"errmsg":"sensitive raw provider details"}`, 200},
		{"anonymous-only", "douyin", `{"err_no":0,"data":{"anonymous_openid":"anonymous","session_key":"private"}}`, 200},
		{"missing-status", "douyin", `{"data":{"openid":"subject","session_key":"private"}}`, 200},
		{"missing-session-key", "wechat", `{"openid":"subject"}`, 200},
		{"invalid-json", "wechat", `{`, 200},
		{"oversized", "wechat", strings.Repeat("x", 32769), 200},
		{"server-error", "wechat", `{}`, 503},
		{"redirect", "wechat", `{}`, 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://untrusted.example/steal"}}, Request: r}, nil
			})}
			id, err := client.Exchange(context.Background(), tc.platform, Profile{}, "code")
			if id != "" || err != ErrExchange || calls != 1 {
				t.Fatal("unsafe provider failure handling")
			}
		})
	}
}
