package filescan

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type trackedSource struct {
	io.Reader
	closed atomic.Bool
	reads  atomic.Int64
}

func (s *trackedSource) Read(p []byte) (int, error) { s.reads.Add(1); return s.Reader.Read(p) }
func (s *trackedSource) Close() error               { s.closed.Store(true); return nil }

type daemonObservations struct {
	commands atomic.Int64
	maxChunk atomic.Uint32
	bytes    atomic.Int64
	complete atomic.Int64
	mu       sync.Mutex
	digest   string
}

func fixtureDaemon(t *testing.T, scanReply string, changeEngine, stall bool) (string, *daemonObservations) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observed := &daemonObservations{}
	var group sync.WaitGroup
	var probes atomic.Int64
	group.Add(1)
	go func() {
		defer group.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			group.Add(1)
			go func() {
				defer group.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				input := bufio.NewReader(conn)
				command, err := input.ReadString(0)
				if err != nil {
					return
				}
				observed.commands.Add(1)
				switch command {
				case "zVERSIONCOMMANDS\x00":
					if stall {
						_, _ = io.Copy(io.Discard, input)
						return
					}
					signature := "100"
					if probes.Add(1) > 1 && changeEngine {
						signature = "101"
					}
					_, _ = conn.Write([]byte("ClamAV 1.5.4/" + signature + "/Fri Oct 23 2026 | COMMANDS: VERSION INSTREAM PING\x00"))
				case "zINSTREAM\x00":
					hash := sha256.New()
					var header [4]byte
					buffer := make([]byte, 64<<10)
					var count int64
					for {
						if _, err = io.ReadFull(input, header[:]); err != nil {
							return
						}
						size := binary.BigEndian.Uint32(header[:])
						for {
							previous := observed.maxChunk.Load()
							if size <= previous || observed.maxChunk.CompareAndSwap(previous, size) {
								break
							}
						}
						if size == 0 {
							break
						}
						if size > uint32(len(buffer)) {
							return
						}
						if _, err = io.ReadFull(input, buffer[:size]); err != nil {
							return
						}
						_, _ = hash.Write(buffer[:size])
						count += int64(size)
					}
					observed.bytes.Store(count)
					observed.complete.Add(1)
					observed.mu.Lock()
					observed.digest = hex.EncodeToString(hash.Sum(nil))
					observed.mu.Unlock()
					_, _ = conn.Write(append([]byte(scanReply), 0))
				}
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); group.Wait() })
	return listener.Addr().String(), observed
}
func fixtureClient(t *testing.T, address string) *Client {
	t.Helper()
	client, err := New(Config{Network: "tcp", Address: address, Policy: Policy{MaxBytes: 64 << 20, Timeout: time.Second, SHA256: strings.Repeat("a", 64), LimitsAttested: true}})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func TestClamdStreamBindsExactBytesAndPolicy(t *testing.T) {
	address, observed := fixtureDaemon(t, "stream: OK", false, false)
	body := bytes.Repeat([]byte("bounded-byte-stream\n"), 10000)
	source := &trackedSource{Reader: bytes.NewReader(body)}
	result, err := fixtureClient(t, address).Scan(context.Background(), source, int64(len(body)))
	if err != nil || result.State != "clean" || result.EngineVersion != "1.5.4" || result.SignatureRevision != "100" || result.StreamedBytes != int64(len(body)) || result.PolicySHA256 != strings.Repeat("a", 64) {
		t.Fatal("scan evidence incomplete", result, err)
	}
	expected := sha256.Sum256(body)
	observed.mu.Lock()
	actual := observed.digest
	observed.mu.Unlock()
	if actual != hex.EncodeToString(expected[:]) || observed.bytes.Load() != int64(len(body)) || observed.complete.Load() != 1 || observed.commands.Load() != 3 || observed.maxChunk.Load() > 64<<10 || !source.closed.Load() {
		t.Fatal("daemon did not receive exact framed bounded stream")
	}
}
func TestClamdNeverConvertsEngineLimitsIntoClean(t *testing.T) {
	for _, entry := range []struct{ name, reply, state, code string }{
		{"malware", "stream: Fixture.Signature FOUND", "quarantined", "MALWARE_DETECTED"},
		{"archive limit", "stream: Heuristics.Limits.Exceeded.MaxRecursion FOUND", "unscannable", "SCAN_CONTENT_LIMIT"},
		{"encrypted content", "stream: Heuristics.Encrypted.Zip FOUND", "unscannable", "SCAN_CONTENT_LIMIT"},
		{"daemon stream limit", "INSTREAM size limit exceeded. ERROR", "unscannable", "SCAN_ENGINE_SIZE_LIMIT"},
		{"engine error", "stream: private diagnostic ERROR", "unscannable", "SCAN_ENGINE_ERROR"},
		{"disabled command", "COMMAND UNAVAILABLE", "unscannable", "SCAN_COMMAND_UNAVAILABLE"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			address, _ := fixtureDaemon(t, entry.reply, false, false)
			source := &trackedSource{Reader: strings.NewReader("fixture")}
			result, err := fixtureClient(t, address).Scan(context.Background(), source, 7)
			if err != nil || result.State != entry.state || result.Code != entry.code || !source.closed.Load() {
				t.Fatal("unsafe engine result was accepted", result, err)
			}
			if strings.Contains(result.Code, "private") {
				t.Fatal("daemon diagnostics exposed")
			}
		})
	}
	t.Run("definition reload", func(t *testing.T) {
		address, _ := fixtureDaemon(t, "stream: OK", true, false)
		result, err := fixtureClient(t, address).Scan(context.Background(), io.NopCloser(strings.NewReader("fixture")), 7)
		if err != nil || result.State != "unscannable" || result.Code != "SCAN_ENGINE_CHANGED" {
			t.Fatal("definition drift accepted", result, err)
		}
	})
}
func TestClamdRejectsSourceLengthAndAmbiguousReplies(t *testing.T) {
	for _, entry := range []struct {
		name, source, reply, code string
		declared                  int64
	}{
		{"short source", "tiny", "stream: OK", "SCAN_SOURCE_SIZE_MISMATCH", 8},
		{"long source", "too-long", "stream: OK", "SCAN_SOURCE_SIZE_MISMATCH", 3},
		{"multiple records", "fixture", "stream: OK\x00stream: Fixture FOUND", "SCAN_INVALID_REPLY", 7},
		{"oversized diagnostic", "fixture", strings.Repeat("private-diagnostic", 400), "SCAN_INVALID_REPLY", 7},
		{"unknown reply", "fixture", "stream: maybe", "SCAN_INVALID_REPLY", 7},
	} {
		t.Run(entry.name, func(t *testing.T) {
			address, _ := fixtureDaemon(t, entry.reply, false, false)
			source := &trackedSource{Reader: strings.NewReader(entry.source)}
			result, err := fixtureClient(t, address).Scan(context.Background(), source, entry.declared)
			var scanError *Error
			if !errors.As(err, &scanError) || scanError.Code != entry.code || result.State == "clean" || !source.closed.Load() {
				t.Fatal("invalid source or reply was accepted", result, err)
			}
			if strings.Contains(err.Error(), "private-diagnostic") {
				t.Fatal("raw diagnostic exposed")
			}
		})
	}
}
func TestClamdRequiresLocalAttestedPolicyAndClosesSkippedSources(t *testing.T) {
	address, observed := fixtureDaemon(t, "stream: OK", false, false)
	policy := Policy{MaxBytes: 64 << 20, Timeout: time.Second, SHA256: strings.Repeat("a", 64), LimitsAttested: true}
	tooLarge := policy
	tooLarge.MaxBytes = MaxClamAVBytes + 1
	if _, err := New(Config{Network: "tcp", Address: address, Policy: tooLarge}); err == nil {
		t.Fatal("policy exceeded the engine's documented design limit")
	}
	invalid := policy
	invalid.LimitsAttested = false
	if _, err := New(Config{Network: "tcp", Address: address, Policy: invalid}); err == nil {
		t.Fatal("unattested scanner accepted")
	}
	if _, err := New(Config{Network: "tcp", Address: "192.0.2.1:3310", Policy: policy}); err == nil {
		t.Fatal("plaintext remote endpoint accepted")
	}
	source := &trackedSource{Reader: strings.NewReader("unused")}
	result, err := fixtureClient(t, address).Scan(context.Background(), source, 120000000000)
	if err != nil || result.State != "unscannable" || result.Code != "SCAN_SIZE_LIMIT" || !source.closed.Load() || source.reads.Load() != 0 || observed.commands.Load() != 0 {
		t.Fatal("120 GB over-limit source was streamed or marked clean", result, err)
	}
	source = &trackedSource{Reader: strings.NewReader("unused")}
	result, err = (Disabled{}).Scan(context.Background(), source, 6)
	if err != nil || result.State != "unscannable" || !source.closed.Load() || source.reads.Load() != 0 {
		t.Fatal("disabled scanner fabricated clean result", result, err)
	}
}
func TestClamdCancellationClosesSourceAndConnection(t *testing.T) {
	address, _ := fixtureDaemon(t, "stream: OK", false, true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	source := &trackedSource{Reader: strings.NewReader("fixture")}
	result, err := fixtureClient(t, address).Scan(ctx, source, 7)
	var scanError *Error
	if !errors.As(err, &scanError) || !scanError.Retryable || result.State == "clean" || !source.closed.Load() {
		t.Fatal("cancelled scan became successful or leaked source", result, err)
	}
}
