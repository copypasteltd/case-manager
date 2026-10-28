//go:build linux

package nativeruntime

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func loadConfig(name string) (Config, error) {
	var c Config
	if os.Geteuid() != 0 || trusted(name, true) != nil {
		return c, invalid()
	}
	f, e := os.Open(name)
	if e != nil {
		return c, invalid()
	}
	defer f.Close()
	if Decode(f, &c, 65536) != nil || c.Validate() != nil {
		return c, invalid()
	}
	return c, nil
}
func preflight(c Config) error {
	if os.Geteuid() != 0 {
		return invalid()
	}
	if _, e := os.Stat("/run/systemd/system"); e != nil {
		return invalid()
	}
	controllers, e := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if e != nil {
		return invalid()
	}
	for _, v := range []string{"cpu", "memory", "pids"} {
		if !has(strings.Fields(string(controllers)), v) {
			return invalid()
		}
	}
	// Required properties and --expand-environment are supported on this
	// minimum baseline. Unsupported restrictions remain a startup refusal.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", "--version")
	cmd.Env = nativeEnvironment()
	cmd.Stderr = io.Discard
	raw, e := cmd.Output()
	if e != nil {
		return invalid()
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return invalid()
	}
	version, e := strconv.Atoi(fields[1])
	if e != nil || version < 255 {
		return invalid()
	}
	for _, pin := range []FilePin{c.Node, c.Codex, c.Broker} {
		if checkPin(pin) != nil {
			return invalid()
		}
	}
	executable, e := os.Executable()
	if e != nil || executable != c.Broker.Path {
		return invalid()
	}
	if trusted(c.Entry, false) != nil {
		return invalid()
	}
	digest, e := ReleaseDigest(c.Release)
	if e != nil || digest != c.ReleaseSHA256 {
		return invalid()
	}
	for _, p := range c.LibraryMounts {
		resolved, e := filepath.EvalSymlinks(p)
		if e != nil || trusted(resolved, false) != nil {
			return invalid()
		}
	}
	return nil
}
func Check(config string) error {
	c, e := loadConfig(config)
	if e != nil {
		return e
	}
	return preflight(c)
}
func Serve(ctx context.Context, config string) error {
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()
	c, e := loadConfig(config)
	if e != nil {
		return e
	}
	if preflight(c) != nil {
		return invalid()
	}
	private, e := readPrivate(c.SigningKeyFile)
	if e != nil {
		return e
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(private)))
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return invalid()
	}
	model, e := readPrivate(c.ModelKeyFile)
	if e != nil {
		return e
	}
	modelKey := strings.TrimSpace(string(model))
	if len(modelKey) < 16 || len(modelKey) > 4096 || strings.ContainsAny(modelKey, "\r\n") {
		return invalid()
	}
	if e = os.MkdirAll(c.StateDirectory, 0750); e != nil {
		return invalid()
	}
	if e = os.Chown(c.StateDirectory, 0, int(c.WorkerGID)); e != nil {
		return invalid()
	}
	if trusted(c.StateDirectory, false) != nil {
		return invalid()
	}
	runsDir := filepath.Join(c.StateDirectory, "runs")
	if e = os.MkdirAll(runsDir, 0700); e != nil {
		return invalid()
	}
	lock, e := os.OpenFile(filepath.Join(c.StateDirectory, "broker.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return invalid()
	}
	defer lock.Close()
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return invalid()
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	h := &linuxHost{config: c, servers: map[string]*http.Server{}}
	if e = h.recover(ctx); e != nil {
		return e
	}
	// Only this root-owned socket can be replaced, after proving no service is
	// accepting it. A live broker or an unexpected inode is never overwritten.
	if info, e := os.Lstat(c.Socket); e == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return invalid()
		}
		probe, e := net.DialTimeout("unix", c.Socket, 200*time.Millisecond)
		if e == nil {
			_ = probe.Close()
			return invalid()
		}
		if e = os.Remove(c.Socket); e != nil {
			return invalid()
		}
	} else if !os.IsNotExist(e) {
		return invalid()
	}
	listener, e := net.Listen("unix", c.Socket)
	if e != nil {
		return invalid()
	}
	defer listener.Close()
	defer os.Remove(c.Socket)
	if os.Chown(c.Socket, 0, int(c.WorkerGID)) != nil || os.Chmod(c.Socket, 0660) != nil {
		return invalid()
	}
	transport := secureTransport()
	defer transport.CloseIdleConnections()
	b := &broker{config: c, key: ed25519.PrivateKey(key), modelKey: modelKey, host: h, upstream: transport, clock: time.Now, runs: map[string]*activeRun{}}
	go func() { <-ctx.Done(); _ = listener.Close() }()
	var clients sync.WaitGroup
	defer func() { shutdown(); clients.Wait() }()
	slots := make(chan struct{}, 16)
	for {
		conn, e := listener.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return invalid()
		}
		peer, e := peerCredential(conn)
		if e != nil || peer.UID != c.WorkerUID {
			_ = conn.Close()
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer conn.Close()
			defer func() { <-slots }()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			var request Request
			if ReadFrame(conn, &request) != nil {
				return
			}
			_ = conn.SetReadDeadline(time.Time{})
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() { var extra [1]byte; _, _ = conn.Read(extra[:]); cancel() }()
			if request.Action == "prove" {
				var end context.CancelFunc
				runCtx, end = context.WithTimeout(runCtx, 6*time.Second)
				defer end()
			}
			reply := b.dispatch(runCtx, request)
			_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			_ = WriteFrame(conn, reply)
		}()
	}
}
func (h *linuxHost) recover(ctx context.Context) error {
	root := filepath.Join(h.config.StateDirectory, "runs")
	entries, e := os.ReadDir(root)
	if e != nil {
		return invalid()
	}
	if len(entries) > 10000 {
		return invalid()
	}
	for _, entry := range entries {
		if !entry.IsDir() || !uuidPattern.MatchString(entry.Name()) {
			return invalid()
		}
		directory := filepath.Join(root, entry.Name())
		if trusted(directory, true) != nil {
			return invalid()
		}
		f, e := os.Open(filepath.Join(directory, "plan.json"))
		if e != nil {
			return invalid()
		}
		var plan UnitPlan
		e = Decode(f, &plan, 65536)
		_ = f.Close()
		if e != nil || plan.RunID != entry.Name() || plan.Unit != unitName(entry.Name()) || plan.Root != filepath.Join(directory, "root") || plan.Gateway != filepath.Join(directory, "gateway.sock") {
			return invalid()
		}
		limited, cancel := context.WithTimeout(ctx, 12*time.Second)
		e = h.Stop(limited, plan)
		cancel()
		if e != nil {
			return invalid()
		}
		if e = h.Clean(plan); e != nil {
			return invalid()
		}
		// A completed old tombstone contains no prompt or credential. Active or
		// uncertain units have already made reconciliation fail above.
		if info, e := os.Stat(filepath.Join(directory, "plan.json")); e == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
			if filepath.Dir(directory) != root || !uuidPattern.MatchString(filepath.Base(directory)) {
				return invalid()
			}
			if os.RemoveAll(directory) != nil {
				return invalid()
			}
		}
	}
	return nil
}
func PublicKey(config string) (string, error) {
	c, e := loadConfig(config)
	if e != nil {
		return "", e
	}
	raw, e := readPrivate(c.SigningKeyFile)
	if e != nil {
		return "", e
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return "", invalid()
	}
	return base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey)), nil
}
