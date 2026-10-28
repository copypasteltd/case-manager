// Package miniapp exchanges short-lived platform codes only on fixed official
// endpoints. Platform session keys and raw provider diagnostics never escape it.
package miniapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type Profile struct{ AppID, Secret string }
type Config struct {
	WeChat, Douyin Profile
	IdentityKey    []byte
}
type Exchanger interface {
	Exchange(context.Context, string, Profile, string) (string, error)
}
type Client struct{ Transport http.RoundTripper }

var appID = regexp.MustCompile(`^[A-Za-z0-9_-]{5,128}$`)
var ErrExchange = errors.New("mini-program identity exchange failed; request a new platform code")

func (c Config) Profile(platform string) (Profile, bool) {
	var p Profile
	switch platform {
	case "wechat":
		p = c.WeChat
	case "douyin":
		p = c.Douyin
	default:
		return p, false
	}
	return p, appID.MatchString(p.AppID) && len(p.Secret) >= 16 && len(c.IdentityKey) == 32
}
func (c Config) Valid() bool {
	for _, name := range []string{"wechat", "douyin"} {
		var p Profile
		if name == "wechat" {
			p = c.WeChat
		} else {
			p = c.Douyin
		}
		if p.AppID == "" && p.Secret == "" {
			continue
		}
		if _, ok := c.Profile(name); !ok {
			return false
		}
	}
	return len(c.IdentityKey) == 0 || len(c.IdentityKey) == 32
}
func (c Client) Exchange(ctx context.Context, platform string, p Profile, code string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	switch platform {
	case "wechat":
		query := url.Values{"appid": {p.AppID}, "secret": {p.Secret}, "js_code": {code}, "grant_type": {"authorization_code"}}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, "https://api.weixin.qq.com/sns/jscode2session?"+query.Encode(), nil)
	case "douyin":
		body, _ := json.Marshal(map[string]string{"appid": p.AppID, "secret": p.Secret, "code": code})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://developer.toutiao.com/api/apps/v2/jscode2session", bytes.NewReader(body))
	default:
		return "", ErrExchange
	}
	if err != nil {
		return "", ErrExchange
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: c.Transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", ErrExchange
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ErrExchange
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32769))
	if err != nil || len(body) > 32768 {
		return "", ErrExchange
	}
	var reply struct {
		ErrCode    int    `json:"errcode"`
		ErrNo      int    `json:"err_no"`
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		Data       struct {
			OpenID     string `json:"openid"`
			SessionKey string `json:"session_key"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &reply) != nil || reply.ErrCode != 0 || reply.ErrNo != 0 {
		return "", ErrExchange
	}
	if platform == "douyin" {
		// err_no is mandatory on Douyin; missing status must not mean success.
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || fields["err_no"] == nil {
			return "", ErrExchange
		}
		reply.OpenID, reply.SessionKey = reply.Data.OpenID, reply.Data.SessionKey
	}
	if len(reply.OpenID) < 1 || len(reply.OpenID) > 256 || len(reply.SessionKey) < 1 || len(reply.SessionKey) > 512 {
		return "", ErrExchange
	}
	return reply.OpenID, nil
}
