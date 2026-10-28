package nativeruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func gatewayFixture(t *testing.T) (*gateway, *broker, *fixtureHost, context.CancelFunc, <-chan Reply) {
	t.Helper()
	b, h, in := fixtureBroker(t)
	_, cancel, done := startFixture(t, b, h, in)
	b.mu.Lock()
	s := b.runs[in.Run.RunID]
	b.mu.Unlock()
	return &gateway{b, s}, b, h, cancel, done
}
func gatewayRequest(g *gateway, path, body string) *http.Request {
	r := httptest.NewRequest("POST", "http://local"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	key := GatewayKey
	if strings.HasPrefix(path, "/company/") {
		key = g.run.input.Run.Credential
	}
	r.Header.Set("Authorization", "Bearer "+key)
	return r.WithContext(context.WithValue(r.Context(), peerContextKey{}, Peer{4200, 62001, 62001}))
}
func finishGateway(t *testing.T, cancel context.CancelFunc, done <-chan Reply) {
	t.Helper()
	cancel()
	if !receive(t, done).Stopped {
		t.Fatal("fixture not stopped")
	}
}
func modelBody(g *gateway) string {
	return `{"model":"` + g.run.profile.Model + `","input":[{"role":"user","content":[{"type":"input_text","text":"Synthetic permitted context"}]}],"max_output_tokens":999999,"store":true,"tools":[{"type":"function","name":"mcp__company__read_resource","parameters":{"type":"object"}}]}`
}

func TestNativeGatewayKeepsProviderCredentialOutsideChildAndPinsTechnicalBounds(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	// Real loopback HTTP exchange checks framing and forwarded data; the injected
	// transport only redirects the fixed production hostname to this owned fixture.
	var observed map[string]json.RawMessage
	var hostAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/responses" {
			t.Error("unexpected provider route")
		}
		if json.NewDecoder(r.Body).Decode(&observed) != nil {
			t.Error("bad forwarded JSON")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer server.Close()
	transport := secureTransport()
	defer transport.CloseIdleConnections()
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Extra-Secret") != "" {
			t.Error("upstream target or headers escaped policy")
		}
		copy := r.Clone(r.Context())
		copy.URL.Scheme = "http"
		copy.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return transport.RoundTrip(copy)
	})
	req := gatewayRequest(g, "/model/v1/responses", modelBody(g))
	req.Header.Set("Cookie", "should-not-forward")
	req.Header.Set("X-Extra-Secret", "synthetic-private")
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "response.completed") {
		t.Fatal("actual stream not forwarded")
	}
	if hostAuth != "Bearer "+b.modelKey || string(observed["max_output_tokens"]) != "500" || string(observed["store"]) != "false" {
		t.Fatal("provider credential or technical bounds not enforced")
	}
	if strings.Contains(rec.Body.String(), b.modelKey) || strings.Contains(modelBody(g), b.modelKey) {
		t.Fatal("host provider key exposed")
	}
}

func TestNativeGatewayRejectsPeerPathAuthorityAndRemoteProviderTools(t *testing.T) {
	g, b, h, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	calls := 0
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	cases := map[string]func(*http.Request){
		"missing peer": func(r *http.Request) { *r = *r.WithContext(context.Background()) }, "host peer": func(r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), peerContextKey{}, Peer{10, 0, 0}))
		}, "wrong bearer": func(r *http.Request) { r.Header.Set("Authorization", "Bearer another-run") }, "browser origin": func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }, "query smuggling": func(r *http.Request) { r.URL.RawQuery = "target=https://other.example" }, "arbitrary provider route": func(r *http.Request) { r.URL.Path = "/model/v1/files" }, "encoded slash": func(r *http.Request) { r.URL.RawPath = "/model/v1%2fresponses" }, "unsupported compression": func(r *http.Request) { r.Header.Set("Content-Encoding", "zstd") }, "oversized body": func(r *http.Request) { r.ContentLength = MaxInput + 1 }, "wrong content type": func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, "wrong method": func(r *http.Request) { r.Method = "GET" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := gatewayRequest(g, "/model/v1/responses", modelBody(g))
			mutate(r)
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code != 403 && w.Code != 413 {
				t.Fatal("disallowed request reached provider")
			}
		})
	}
	for _, body := range []string{
		`{"model":"different","input":"x"}`, `{"model":"` + g.run.profile.Model + `","input":"x","tools":[{"type":"web_search"}]}`,
		`{"model":"` + g.run.profile.Model + `","input":"x","tools":[{"type":"mcp","server_url":"https://external.example"}]}`,
		`{"model":"` + g.run.profile.Model + `","input":"x","tools":[{"type":"code_interpreter"}]}`,
		`{"model":"` + g.run.profile.Model + `","input":"x","previous_response_id":"old-response"}`,
		`{"model":"` + g.run.profile.Model + `","input":"x","conversation":"old-conversation"}`,
		`{"model":"` + g.run.profile.Model + `","input":"x","background":true}`,
		`{"model":"` + g.run.profile.Model + `","input":[{"role":"user","content":[{"type":"input_file","file_url":"https://external.example"}]}]}`,
		`{"model":"` + g.run.profile.Model + `","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://external.example"}]}]}`,
		`{"model":"` + g.run.profile.Model + `","input":[{"type":"function_call_output","call_id":"current","output":[{"type":"input_image","image_url":"https://external.example"}]}]}`,
		`{"model":"` + g.run.profile.Model + `","input":[{"type":"custom_tool_call_output","call_id":"current","output":[{"type":"input_file","file_url":"https://external.example"}]}]}`,
	} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, gatewayRequest(g, "/model/v1/responses", body))
		if w.Code != 403 {
			t.Fatal("provider-side work or prior context admitted")
		}
	}
	h.peerDenied = true
	w := httptest.NewRecorder()
	g.ServeHTTP(w, gatewayRequest(g, "/model/v1/responses", modelBody(g)))
	if w.Code != 403 || calls != 0 {
		t.Fatal("failed cgroup identity reached provider")
	}
}

func TestNativeGatewayCompanyRoutesUseOnlyCurrentTaskCredential(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	paths := []string{}
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.String())
		if r.Header.Get("Authorization") != "Bearer "+g.run.input.Run.Credential || strings.Contains(r.Header.Get("Authorization"), b.modelKey) {
			t.Error("wrong company credential")
		}
		return &http.Response{StatusCode: 409, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":"RUNTIME_AUTHORITY_CHANGED"}`))}, nil
	})
	for _, path := range []string{"/internal/v1/agent-mcp", "/internal/v1/agent-control/current", "/internal/v1/agent-control/confinement", "/internal/v1/agent-control/receipts", "/internal/v1/agent-control/events"} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, gatewayRequest(g, "/company"+path, "{}"))
		if w.Code != 409 || !strings.Contains(w.Body.String(), "RUNTIME_AUTHORITY_CHANGED") {
			t.Fatal("company error not preserved")
		}
	}
	for _, path := range []string{"/company/api/v1/admin", "/company/internal/v1/agent-control/../keys", "/company/internal/v1/agent-mcp/extra"} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, gatewayRequest(g, path, "{}"))
		if w.Code != 403 {
			t.Fatal("broader company route admitted")
		}
	}
	if len(paths) != 5 {
		t.Fatal("wrong route count")
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "http://"+b.config.BridgeAddress+"/internal/v1/") {
			t.Fatal("company target override")
		}
	}
}

func TestNativeGatewayNeverFollowsRedirectOrReturnsProviderErrorSecrets(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	for _, status := range []int{302, 307, 400, 401, 429, 500} {
		b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.example"}}, Body: io.NopCloser(strings.NewReader(b.modelKey))}, nil
		})
		w := httptest.NewRecorder()
		g.ServeHTTP(w, gatewayRequest(g, "/model/v1/responses", modelBody(g)))
		if w.Code != 502 || strings.Contains(w.Body.String(), b.modelKey) || w.Header().Get("Location") != "" {
			t.Fatal("redirect or provider credential leaked")
		}
	}
}

func TestNativeGatewayRunCancellationClosesOwnedProviderStream(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	started := make(chan struct{})
	closed := make(chan struct{})
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(closed)
		return nil, r.Context().Err()
	})
	served := make(chan struct{})
	go func() {
		defer close(served)
		g.ServeHTTP(httptest.NewRecorder(), gatewayRequest(g, "/model/v1/responses", modelBody(g)))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	receive(t, done)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("provider context survived run cancellation")
	}
	<-served
}
func TestNativeGatewayStopsAnOversizedStream(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), (8<<20)+1)))}, nil
	})
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Error("oversized provider stream was not aborted")
		}
	}()
	g.ServeHTTP(httptest.NewRecorder(), gatewayRequest(g, "/model/v1/responses", modelBody(g)))
}

func TestNativeGatewayBoundsConcurrentHostWork(t *testing.T) {
	g, b, _, cancel, done := gatewayFixture(t)
	defer finishGateway(t, cancel, done)
	calls := 0
	b.upstream = roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, invalid() })
	for i := 0; i < cap(g.run.models); i++ {
		g.run.models <- struct{}{}
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, gatewayRequest(g, "/model/v1/responses", modelBody(g)))
	if w.Code != 503 || calls != 0 {
		t.Fatal("excess provider concurrency admitted")
	}
	for len(g.run.models) > 0 {
		<-g.run.models
	}
	for i := 0; i < cap(g.run.requests); i++ {
		g.run.requests <- struct{}{}
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, gatewayRequest(g, "/company/internal/v1/agent-control/current", "{}"))
	if w.Code != 503 || calls != 0 {
		t.Fatal("excess privileged relay concurrency admitted")
	}
	for len(g.run.requests) > 0 {
		<-g.run.requests
	}
}
