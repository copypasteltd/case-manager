package fileimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/files"
)

type Client struct {
	base          string
	token         string
	uploadOrigins map[string]bool
	http          *http.Client
	allowLoopback bool
}

type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string { return fmt.Sprintf("%s (HTTP %d)", e.Code, e.Status) }

var safeErrorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,99}$`)

func allowedURL(raw string, local bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, fmt.Errorf("INVALID_ENDPOINT")
	}
	if u.Scheme != "https" {
		a, err := netip.ParseAddr(u.Hostname())
		if !local || u.Scheme != "http" || err != nil || !a.IsLoopback() {
			return nil, fmt.Errorf("HTTPS_REQUIRED")
		}
	}
	return u, nil
}

func NewClient(base, token string, uploadOrigins []string, local bool) (*Client, error) {
	u, err := allowedURL(strings.TrimRight(base, "/"), local)
	if err != nil {
		return nil, err
	}
	if u.Path != "/v1" || u.RawQuery != "" || u.RawPath != "" {
		return nil, fmt.Errorf("API_BASE_MUST_END_IN_V1")
	}
	if len(token) < 16 || len(token) > 8192 || strings.ContainsAny(token, "\r\n\t ") {
		return nil, fmt.Errorf("API_SESSION_TOKEN_REQUIRED")
	}
	c := &Client{base: u.String(), token: token, uploadOrigins: map[string]bool{}, allowLoopback: local}
	for _, origin := range uploadOrigins {
		v, e := allowedURL(origin, local)
		if e != nil || v.Path != "" || v.RawQuery != "" || v.RawPath != "" {
			return nil, fmt.Errorf("EXPLICIT_UPLOAD_ORIGIN_REQUIRED")
		}
		c.uploadOrigins[v.Scheme+"://"+v.Host] = true
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 2
	c.http = &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections(); c.token = "" }

func (c *Client) Request(ctx context.Context, method, path, key string, in, out any) error {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("INVALID_API_PATH")
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("API_REQUEST_INVALID")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("API_OUTCOME_UNCERTAIN_REREAD_BEFORE_RETRY")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return fmt.Errorf("API_RECEIPT_INCOMPLETE_REREAD_BEFORE_RETRY")
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("API_RECEIPT_INCOMPLETE_REREAD_BEFORE_RETRY")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := "API_REQUEST_REJECTED"
		if envelope.Error != nil && safeErrorCode.MatchString(envelope.Error.Code) {
			code = envelope.Error.Code
		}
		return &APIError{Status: resp.StatusCode, Code: code}
	}
	if envelope.Error != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("API_RECEIPT_INCOMPLETE_REREAD_BEFORE_RETRY")
	}
	if out != nil {
		if err = json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("API_RECEIPT_INVALID_REREAD_BEFORE_RETRY")
		}
	}
	return nil
}

func (c *Client) Put(ctx context.Context, part files.SignedPart, reader io.Reader, size int64) (string, error) {
	u, err := allowedURL(part.URL, c.allowLoopback)
	if err != nil || !c.uploadOrigins[u.Scheme+"://"+u.Host] || part.Method != "PUT" || !part.ExpiresAt.After(time.Now()) || size < 1 {
		return "", fmt.Errorf("UNAPPROVED_OR_EXPIRED_UPLOAD_TARGET")
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", u.String(), reader)
	if err != nil {
		return "", fmt.Errorf("UPLOAD_REQUEST_INVALID")
	}
	req.ContentLength = size
	if len(part.Headers) > 32 {
		return "", fmt.Errorf("UPLOAD_HEADERS_INVALID")
	}
	seen := map[string]bool{}
	for _, h := range part.Headers {
		name := http.CanonicalHeaderKey(h.Name)
		if name == "" || seen[name] || len(h.Value) > 4096 || strings.ContainsAny(h.Value, "\r\n") {
			return "", fmt.Errorf("UPLOAD_HEADERS_INVALID")
		}
		seen[name] = true
		if name == "Content-Length" {
			if h.Value != strconv.FormatInt(size, 10) {
				return "", fmt.Errorf("UPLOAD_SIZE_HEADER_INVALID")
			}
			continue
		}
		if name != "Content-Type" && !strings.HasPrefix(name, "X-Amz-") && !strings.HasPrefix(name, "X-Oss-") {
			return "", fmt.Errorf("UPLOAD_HEADER_NOT_ALLOWED")
		}
		req.Header.Set(name, h.Value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("PART_OUTCOME_UNCERTAIN_RECONCILE_BEFORE_RETRY")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("PART_UPLOAD_REJECTED_RECONCILE_BEFORE_RETRY")
	}
	etag := resp.Header.Get("ETag")
	if len(etag) < 1 || len(etag) > 200 || strings.ContainsAny(etag, "\r\n") {
		return "", fmt.Errorf("PART_RECEIPT_INCOMPLETE_RECONCILE_BEFORE_RETRY")
	}
	return etag, nil
}
