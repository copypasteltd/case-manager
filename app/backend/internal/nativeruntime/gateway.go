package nativeruntime

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type gateway struct {
	broker *broker
	run    *activeRun
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, s := g.broker, g.run
	peer, ok := peerFrom(r)
	if !ok || !b.active(s) || b.host.Peer(s.plan, peer) != nil || r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" || r.URL.EscapedPath() != r.URL.Path || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" || r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity" {
		writeSafe(w, 403)
		return
	}
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		writeSafe(w, 503)
		return
	}
	var target *url.URL
	isModel := false
	if r.URL.Path == "/model/v1/responses" {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+GatewayKey)) != 1 {
			writeSafe(w, 403)
			return
		}
		target, _ = url.Parse(b.config.ModelOrigin + "/responses")
		isModel = true
		select {
		case s.models <- struct{}{}:
			defer func() { <-s.models }()
		default:
			writeSafe(w, 503)
			return
		}
	} else if strings.HasPrefix(r.URL.Path, "/company/") {
		endpoint := strings.TrimPrefix(r.URL.Path, "/company")
		allowed := endpoint == "/internal/v1/agent-mcp"
		for _, v := range []string{"current", "confinement", "receipts", "events"} {
			allowed = allowed || endpoint == "/internal/v1/agent-control/"+v
		}
		if !allowed || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.input.Run.Credential)) != 1 {
			writeSafe(w, 403)
			return
		}
		target, _ = url.Parse("http://" + b.config.BridgeAddress + endpoint)
	} else {
		writeSafe(w, 403)
		return
	}
	if r.ContentLength > MaxInput {
		writeSafe(w, 413)
		return
	}
	var body io.Reader = http.MaxBytesReader(w, r.Body, MaxInput)
	if isModel {
		raw, e := io.ReadAll(body)
		if e != nil {
			writeSafe(w, 413)
			return
		}
		var value map[string]json.RawMessage
		if json.Unmarshal(raw, &value) != nil || len(value) == 0 {
			writeSafe(w, 403)
			return
		}
		var model string
		if json.Unmarshal(value["model"], &model) != nil || model != s.profile.Model {
			writeSafe(w, 403)
			return
		}
		if !textOnlyRequest(value) {
			writeSafe(w, 403)
			return
		}
		// Output length remains a technical bound. This does not introduce fee,
		// token-purchase, monthly quota, reservation or billing controls.
		value["max_output_tokens"], _ = json.Marshal(s.input.Run.MaxOutputTokens)
		value["store"] = json.RawMessage(`false`)
		raw, _ = json.Marshal(value)
		body = bytes.NewReader(raw)
	}
	// Bind the upstream stream to the request AND the owning run. Cancelling the
	// launcher closes provider streams even if a child ignores termination.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if deadline, ok := s.ctx.Deadline(); ok {
		var end context.CancelFunc
		ctx, end = context.WithDeadline(ctx, deadline)
		defer end()
	}
	request, err := http.NewRequestWithContext(ctx, "POST", target.String(), body)
	if err != nil {
		writeSafe(w, 502)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", r.Header.Get("Accept"))
	if isModel {
		request.Header.Set("Authorization", "Bearer "+b.modelKey)
		for _, name := range []string{"OpenAI-Beta", "Originator", "Version"} {
			if v := r.Header.Get(name); len(v) <= 200 && !strings.ContainsAny(v, "\r\n") {
				request.Header.Set(name, v)
			}
		}
	} else {
		request.Header.Set("Authorization", "Bearer "+s.input.Run.Credential)
	}
	response, err := b.upstream.RoundTrip(request)
	if err != nil {
		writeSafe(w, 502)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		writeSafe(w, 502)
		return
	}
	// Provider error bodies can echo parts of the host's API credential.
	if isModel && response.StatusCode >= 400 {
		writeSafe(w, 502)
		return
	}
	if !b.active(s) {
		writeSafe(w, 403)
		return
	}
	if v := response.Header.Get("Content-Type"); len(v) <= 200 {
		w.Header().Set("Content-Type", v)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	maximum := int64(8 << 20)
	if !isModel {
		maximum = 1 << 20
	}
	// Stream actual chunks. No response body is buffered into a host file or log.
	buffer := make([]byte, 16<<10)
	written := int64(0)
	for {
		n, e := response.Body.Read(buffer)
		if n > 0 {
			written += int64(n)
			if written > maximum {
				cancel()
				panic(http.ErrAbortHandler)
			}
			if _, err = w.Write(buffer[:n]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if e == io.EOF {
			return
		}
		if e != nil {
			panic(http.ErrAbortHandler)
		}
		if !b.active(s) {
			cancel()
			panic(http.ErrAbortHandler)
		}
	}
}
func secureTransport() *http.Transport {
	return &http.Transport{Proxy: nil, MaxIdleConns: 8, MaxIdleConnsPerHost: 2, IdleConnTimeout: 10 * time.Second, ResponseHeaderTimeout: 45 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ForceAttemptHTTP2: true, MaxResponseHeaderBytes: 16384}
}

// Provider-hosted tools and URL/file inputs would execute outside the unit's
// network namespace. Only local tool calls and text context are admitted.
func textOnlyRequest(value map[string]json.RawMessage) bool {
	for _, key := range []string{"conversation", "previous_response_id"} {
		if v, ok := value[key]; ok && string(v) != "null" {
			return false
		}
	}
	if v, ok := value["background"]; ok && string(v) != "false" && string(v) != "null" {
		return false
	}
	if v, ok := value["tools"]; ok {
		var tools []map[string]json.RawMessage
		if json.Unmarshal(v, &tools) != nil {
			return false
		}
		for _, tool := range tools {
			var kind string
			if json.Unmarshal(tool["type"], &kind) != nil || kind != "function" && kind != "custom" {
				return false
			}
		}
	}
	v, ok := value["input"]
	if !ok {
		return false
	}
	var text string
	if json.Unmarshal(v, &text) == nil {
		return true
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(v, &items) != nil {
		return false
	}
	for _, item := range items {
		var kind string
		if v, ok := item["type"]; ok && json.Unmarshal(v, &kind) != nil {
			return false
		}
		switch kind {
		case "", "message":
			var role string
			if json.Unmarshal(item["role"], &role) != nil || !has([]string{"user", "assistant", "system", "developer"}, role) {
				return false
			}
			if json.Unmarshal(item["content"], &text) == nil {
				continue
			}
			var content []map[string]json.RawMessage
			if json.Unmarshal(item["content"], &content) != nil {
				return false
			}
			for _, part := range content {
				if json.Unmarshal(part["type"], &kind) != nil || !has([]string{"input_text", "output_text", "summary_text", "refusal"}, kind) {
					return false
				}
			}
		case "function_call_output", "custom_tool_call_output":
			// A content-array tool result can carry provider-fetched images/files.
			// Company MCP returns text; keep that narrower protocol at this edge.
			if json.Unmarshal(item["output"], &text) != nil {
				return false
			}
		case "reasoning", "function_call", "custom_tool_call":
		default:
			return false
		}
	}
	return true
}
