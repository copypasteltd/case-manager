//go:build linux

package nativeruntime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"copypaste.hk/platform/internal/agentworker"
)

func Client(ctx context.Context, socket, action, node, entry string, input io.Reader, output io.Writer) error {
	if !absolute(socket) {
		return invalid()
	}
	request := Request{Action: action}
	if action == "launch" {
		var in WorkerInput
		if Decode(input, &in, MaxInput) != nil {
			return invalid()
		}
		request.Node = node
		request.Entry = entry
		request.Input = &in
	} else if action == "prove" {
		var in agentworker.ProofChallenge
		if Decode(input, &in, 65536) != nil {
			return invalid()
		}
		request.Challenge = &in
	} else {
		return invalid()
	}
	dial := net.Dialer{Timeout: 2 * time.Second}
	conn, e := dial.DialContext(ctx, "unix", socket)
	if e != nil {
		return invalid()
	}
	defer conn.Close()
	peer, e := peerCredential(conn)
	if e != nil || peer.UID != 0 {
		return invalid()
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if WriteFrame(conn, request) != nil {
		return invalid()
	}
	_ = conn.SetWriteDeadline(time.Time{})
	if action == "prove" {
		_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	} else {
		_ = conn.SetReadDeadline(request.Input.Run.ExpiresAt.Add(20 * time.Second))
	}
	var reply Reply
	if ReadFrame(conn, &reply) != nil {
		return invalid()
	}
	if action == "prove" {
		if reply.Code != "NATIVE_PROOF_ISSUED" || reply.Proof == nil {
			return invalid()
		}
		return json.NewEncoder(output).Encode(reply.Proof)
	}
	if reply.Code != "NATIVE_EXECUTION_FINISHED" || !reply.Stopped {
		return invalid()
	}
	return nil
}
func Child(ctx context.Context, node, entry, bridge, model, work string, input io.Reader) error {
	if os.Geteuid() == 0 || !absolute(node) || !absolute(entry) || !absolute(work) || !loopbackAddress(bridge) || !loopbackAddress(model) || bridge == model {
		return invalid()
	}
	var in WorkerInput
	if Decode(input, &in, MaxInput) != nil || in.Settings.APIKey != GatewayKey || in.Settings.APIBaseURL != "http://"+model+"/v1" || in.Settings.MCPEndpoint != "http://"+bridge+agentworker.MCPPath || in.Settings.WorkingDirectory != work {
		return invalid()
	}
	for _, d := range []string{filepath.Join(work, "home"), filepath.Join(work, "tmp")} {
		if os.Mkdir(d, 0700) != nil {
			return invalid()
		}
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", SocketTarget)
	}, MaxIdleConns: 4, IdleConnTimeout: 5 * time.Second, ResponseHeaderTimeout: 45 * time.Second, MaxResponseHeaderBytes: 16384}
	defer transport.CloseIdleConnections()
	makeServer := func(address, prefix string) (*http.Server, error) {
		listener, e := net.Listen("tcp", address)
		if e != nil {
			return nil, invalid()
		}
		server := &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != address || r.Method != "POST" || r.URL.RawQuery != "" || r.URL.EscapedPath() != r.URL.Path || r.Header.Get("Origin") != "" {
				writeSafe(w, 403)
				return
			}
			request, e := http.NewRequestWithContext(r.Context(), "POST", "http://native-gateway"+prefix+r.URL.Path, http.MaxBytesReader(w, r.Body, MaxInput))
			if e != nil {
				writeSafe(w, 403)
				return
			}
			for _, h := range []string{"Authorization", "Content-Type", "Accept", "OpenAI-Beta", "Originator", "Version", "Content-Encoding"} {
				if value := r.Header.Get(h); len(value) <= 8192 {
					request.Header.Set(h, value)
				}
			}
			response, e := transport.RoundTrip(request)
			if e != nil {
				writeSafe(w, 502)
				return
			}
			defer response.Body.Close()
			for _, h := range []string{"Content-Type", "Cache-Control", "X-Content-Type-Options"} {
				if value := response.Header.Get(h); len(value) <= 200 {
					w.Header().Set(h, value)
				}
			}
			w.WriteHeader(response.StatusCode)
			buffer := make([]byte, 16384)
			for {
				n, e := response.Body.Read(buffer)
				if n > 0 {
					if _, err := w.Write(buffer[:n]); err != nil {
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
			}
		})}
		go func() { _ = server.Serve(listener) }()
		return server, nil
	}
	company, e := makeServer(bridge, "/company")
	if e != nil {
		return e
	}
	defer company.Close()
	provider, e := makeServer(model, "/model")
	if e != nil {
		return e
	}
	defer provider.Close()
	raw, _ := json.Marshal(in)
	cmd := exec.Command(node, entry)
	cmd.Dir = work
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = []string{"PATH=" + filepath.Dir(node) + ":" + filepath.Dir(in.Settings.ExecutablePath), "HOME=" + filepath.Join(work, "home"), "TMPDIR=" + filepath.Join(work, "tmp"), "TMP=" + filepath.Join(work, "tmp"), "TEMP=" + filepath.Join(work, "tmp"), "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	if cmd.Start() != nil {
		return invalid()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e := <-done:
		return safeError(e)
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		return invalid()
	}
}
func GenerateKey(name string) error {
	if os.Geteuid() != 0 || !absolute(name) || trusted(filepath.Dir(name), false) != nil {
		return invalid()
	}
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return invalid()
	}
	f, e := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return invalid()
	}
	defer f.Close()
	_, e = io.WriteString(f, base64.StdEncoding.EncodeToString(key)+"\n")
	return safeError(e)
}
