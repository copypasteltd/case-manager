//go:build linux

package nativeruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type linuxHost struct {
	config  Config
	mu      sync.Mutex
	servers map[string]*http.Server
}

func trusted(path string, private bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return invalid()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return invalid()
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
		return invalid()
	}
	if private && info.Mode().Perm()&0077 != 0 {
		return invalid()
	}
	if path != "/" {
		return trusted(filepath.Dir(path), false)
	}
	return nil
}
func readPrivate(path string) ([]byte, error) {
	if trusted(path, true) != nil {
		return nil, invalid()
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return nil, invalid()
	}
	return os.ReadFile(path)
}
func checkPin(pin FilePin) error {
	if trusted(pin.Path, false) != nil {
		return invalid()
	}
	f, e := os.Open(pin.Path)
	if e != nil {
		return invalid()
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return invalid()
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil || hex.EncodeToString(h.Sum(nil)) != pin.SHA256 {
		return invalid()
	}
	return nil
}
func ReleaseDigest(root string) (string, error) {
	if trusted(root, false) != nil {
		return "", invalid()
	}
	h := sha256.New()
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return invalid()
		}
		relative, e := filepath.Rel(root, name)
		if e != nil {
			return invalid()
		}
		if relative == "." {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		for _, part := range parts {
			if part == ".local" || part == ".tools" || part == ".git" || strings.HasPrefix(part, ".env") {
				return invalid()
			}
		}
		if parts[0] != "app" && parts[0] != "docs" {
			return invalid()
		}
		if len(parts) > 1 && parts[0] == "docs" && parts[1] != "skills" {
			return invalid()
		}
		if len(parts) > 1 && parts[0] == "app" && !has([]string{"agent-runtime", "node_modules", "skills", "packages", "package.json", "package-lock.json"}, parts[1]) {
			return invalid()
		}
		info, e := entry.Info()
		if e != nil {
			return invalid()
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0022 != 0 {
			return invalid()
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, e := filepath.EvalSymlinks(name)
			if e != nil || !descendant(root, resolved) {
				return invalid()
			}
			target, e := os.Readlink(name)
			if e != nil {
				return invalid()
			}
			_, _ = h.Write([]byte(filepath.ToSlash(relative) + "\x00link\x00" + target + "\x00"))
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return invalid()
		}
		_, _ = h.Write([]byte(filepath.ToSlash(relative) + "\x00file\x00"))
		f, e := os.Open(name)
		if e != nil {
			return invalid()
		}
		_, e = io.Copy(h, f)
		_ = f.Close()
		if e != nil {
			return invalid()
		}
		_, _ = h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func peerCredential(conn net.Conn) (Peer, error) {
	if wrapped, ok := conn.(*boundedConnection); ok {
		conn = wrapped.Conn
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return Peer{}, invalid()
	}
	raw, e := u.SyscallConn()
	if e != nil {
		return Peer{}, invalid()
	}
	var credential *unix.Ucred
	var inner error
	e = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if e != nil || inner != nil || credential == nil {
		return Peer{}, invalid()
	}
	return Peer{PID: int(credential.Pid), UID: credential.Uid, GID: credential.Gid}, nil
}
func (h *linuxHost) Prepare(ctx context.Context, p UnitPlan, handler http.Handler) error {
	runDir := filepath.Dir(p.Root)
	if !uuidPattern.MatchString(p.RunID) || p.Unit != unitName(p.RunID) || p.Root != filepath.Join(h.config.StateDirectory, "runs", p.RunID, "root") {
		return invalid()
	}
	if err := os.Mkdir(runDir, 0700); err != nil {
		return invalid()
	}
	// The marker precedes unit creation; restart reconciliation only ever acts
	// on this root-owned exact unit identity, never a glob or a caller's unit.
	raw, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(runDir, "plan.json"), raw, 0600); err != nil {
		return invalid()
	}
	if err := os.Mkdir(p.Root, 0755); err != nil {
		return invalid()
	}
	directories := append([]string{h.config.Release, h.config.WorkDirectory, "/tmp", "/proc", "/dev", "/run"}, h.config.LibraryMounts...)
	for _, v := range directories {
		if info, e := os.Stat(v); e == nil && !info.IsDir() {
			v = filepath.Dir(v)
		}
		if err := os.MkdirAll(filepath.Join(p.Root, strings.TrimPrefix(v, "/")), 0755); err != nil {
			return invalid()
		}
	}
	for _, v := range []string{h.config.Node.Path, h.config.Codex.Path, h.config.Broker.Path} {
		if err := os.MkdirAll(filepath.Join(p.Root, strings.TrimPrefix(filepath.Dir(v), "/")), 0755); err != nil {
			return invalid()
		}
	}
	listener, err := net.Listen("unix", p.Gateway)
	if err != nil {
		return invalid()
	}
	if err = os.Chmod(p.Gateway, 0666); err != nil {
		_ = listener.Close()
		return invalid()
	}
	listener = &boundedListener{Listener: listener, slots: make(chan struct{}, 16)}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			p, e := peerCredential(c)
			if e != nil {
				return ctx
			}
			return context.WithValue(ctx, peerContextKey{}, p)
		}}
	h.mu.Lock()
	h.servers[p.RunID] = server
	h.mu.Unlock()
	go func() { _ = server.Serve(listener) }()
	return nil
}
func nativeEnvironment() []string {
	return []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
}
func (h *linuxHost) Run(ctx context.Context, p UnitPlan, input []byte) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", p.Arguments()...)
	cmd.Env = nativeEnvironment()
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 3 * time.Second
	return safeError(cmd.Run())
}
func commandOutput(ctx context.Context, args ...string) ([]byte, error) {
	limited, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(limited, "/usr/bin/systemctl", args...)
	cmd.Env = nativeEnvironment()
	cmd.Stderr = io.Discard
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return out.Bytes(), invalid()
	}
	return out.Bytes(), nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, invalid()
	}
	return b.Buffer.Write(p)
}
func (h *linuxHost) Stop(ctx context.Context, p UnitPlan) error {
	// A transient service outlives a killed systemd-run process. PID1 receives
	// the stop, then the kernel cgroup (not the client exit) proves teardown.
	_, _ = commandOutput(ctx, "stop", p.Unit)
	for ctx.Err() == nil {
		properties, e := unitProperties(ctx, p.Unit)
		if e != nil {
			return invalid()
		}
		if properties["LoadState"] == "not-found" || properties["ActiveState"] == "inactive" || properties["ActiveState"] == "failed" {
			group := properties["ControlGroup"]
			if group != "" && group != "/system.slice/"+p.Unit {
				return invalid()
			}
			group = "/system.slice/" + p.Unit
			raw, e := os.ReadFile(filepath.Join("/sys/fs/cgroup", group, "cgroup.events"))
			if os.IsNotExist(e) || e == nil && strings.Contains(string(raw), "populated 0") {
				return nil
			}
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return invalid()
}
func (h *linuxHost) Clean(p UnitPlan) error {
	h.mu.Lock()
	server := h.servers[p.RunID]
	delete(h.servers, p.RunID)
	h.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	expected := filepath.Join(h.config.StateDirectory, "runs", p.RunID, "root")
	if !uuidPattern.MatchString(p.RunID) || p.Root != expected {
		return invalid()
	}
	if err := os.RemoveAll(p.Root); err != nil {
		return invalid()
	}
	if err := os.Remove(p.Gateway); err != nil && !os.IsNotExist(err) {
		return invalid()
	}
	return os.WriteFile(filepath.Join(filepath.Dir(p.Root), "stopped.json"), []byte(`{"cgroup_empty":true}`), 0600)
}
func (h *linuxHost) SaveEvidence(p UnitPlan, digest string, raw []byte) error {
	if !digestPattern.MatchString(digest) || len(raw) > 65536 {
		return invalid()
	}
	dir := filepath.Join(filepath.Dir(p.Root), "evidence")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return invalid()
	}
	name := filepath.Join(dir, digest+".json")
	f, e := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		return nil
	}
	if e != nil {
		return invalid()
	}
	_, e = f.Write(raw)
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return invalid()
	}
	return nil
}
func (h *linuxHost) Peer(p UnitPlan, peer Peer) error {
	if peer.PID <= 1 || peer.UID == 0 || peer.UID == h.config.WorkerUID {
		return invalid()
	}
	status, e := readStatus(peer.PID)
	if e != nil || statusUID(status) != int64(peer.UID) {
		return invalid()
	}
	group, e := processCgroup(peer.PID)
	if e != nil || group != "/system.slice/"+p.Unit {
		return invalid()
	}
	return nil
}
func unitProperties(ctx context.Context, unit string) (map[string]string, error) {
	keys := []string{"LoadState", "ActiveState", "SubState", "MainPID", "ControlGroup", "Delegate", "DynamicUser", "NoNewPrivileges", "PrivateNetwork", "PrivateMounts", "PrivateDevices", "PrivateIPC", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs", "ProtectControlGroups", "ProtectClock", "RestrictRealtime", "RestrictSUIDSGID", "LockPersonality", "SendSIGKILL", "RootDirectory", "WorkingDirectory", "ProtectSystem", "ProtectHome", "ProtectProc", "ProcSubset", "KillMode", "OOMPolicy", "Restart", "CapabilityBoundingSet", "AmbientCapabilities", "RestrictNamespaces", "RestrictAddressFamilies", "SystemCallFilter", "SystemCallArchitectures", "NoExecPaths", "ExecPaths"}
	args := []string{"show", unit, "--no-pager"}
	for _, key := range keys {
		args = append(args, "--property="+key)
	}
	raw, e := commandOutput(ctx, args...)
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			out[key] = value
		}
	}
	if e != nil && out["LoadState"] != "not-found" {
		return nil, e
	}
	return out, nil
}
func readStatus(pid int) (map[string]string, error) {
	raw, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if e != nil {
		return nil, invalid()
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out, nil
}
func statusUID(status map[string]string) int64 {
	values := strings.Fields(status["Uid"])
	if len(values) != 4 {
		return -1
	}
	first, e := strconv.ParseInt(values[0], 10, 64)
	if e != nil {
		return -1
	}
	for _, v := range values {
		if v != values[0] {
			return -1
		}
	}
	return first
}
func processCgroup(pid int) (string, error) {
	raw, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if e != nil {
		return "", invalid()
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "0::/") {
		return "", invalid()
	}
	return strings.TrimPrefix(lines[0], "0::"), nil
}
func inode(path string) (uint64, uint64, error) {
	info, e := os.Stat(path)
	if e != nil {
		return 0, 0, invalid()
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, invalid()
	}
	return uint64(s.Dev), s.Ino, nil
}
func readNumber(path string) (int64, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return 0, invalid()
	}
	v, e := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if e != nil || v < 0 {
		return 0, invalid()
	}
	return v, nil
}
func (h *linuxHost) Observe(ctx context.Context, p UnitPlan) (Observation, error) {
	o := Observation{Unit: p.Unit, ReleaseSHA256: h.config.ReleaseSHA256, InspectedAt: time.Now().UTC()}
	var e error
	o.Properties, e = unitProperties(ctx, p.Unit)
	if e != nil {
		return o, e
	}
	o.PID, e = strconv.Atoi(o.Properties["MainPID"])
	if e != nil || o.PID <= 1 {
		return o, invalid()
	}
	status, e := readStatus(o.PID)
	if e != nil {
		return o, e
	}
	uid := statusUID(status)
	if uid < 1 {
		return o, invalid()
	}
	o.UID = uint32(uid)
	gids := strings.Fields(status["Gid"])
	if len(gids) != 4 {
		return o, invalid()
	}
	gid, e := strconv.ParseUint(gids[0], 10, 32)
	if e != nil || gid == 0 {
		return o, invalid()
	}
	for _, v := range gids {
		if v != gids[0] {
			return o, invalid()
		}
	}
	o.GID = uint32(gid)
	o.NoNewPrivileges, _ = strconv.Atoi(status["NoNewPrivs"])
	o.Seccomp, _ = strconv.Atoi(status["Seccomp"])
	o.CapEffective = status["CapEff"]
	o.CapPermitted = status["CapPrm"]
	o.CapBounding = status["CapBnd"]
	o.Cgroup = o.Properties["ControlGroup"]
	o.ProcessCgroup, e = processCgroup(o.PID)
	if e != nil {
		return o, e
	}
	proc := filepath.Join("/proc", strconv.Itoa(o.PID))
	_, o.MountNamespace, e = inode(filepath.Join(proc, "ns/mnt"))
	if e != nil {
		return o, e
	}
	_, o.HostMountNamespace, e = inode("/proc/self/ns/mnt")
	if e != nil {
		return o, e
	}
	_, o.NetworkNamespace, e = inode(filepath.Join(proc, "ns/net"))
	if e != nil {
		return o, e
	}
	_, o.HostNetworkNamespace, e = inode("/proc/self/ns/net")
	if e != nil {
		return o, e
	}
	o.RootDevice, o.RootInode, e = inode(filepath.Join(proc, "root"))
	if e != nil {
		return o, e
	}
	o.ExpectedRootDevice, o.ExpectedRootInode, e = inode(p.Root)
	if e != nil {
		return o, e
	}
	raw, e := os.ReadFile(filepath.Join(proc, "stat"))
	if e != nil {
		return o, invalid()
	}
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return o, invalid()
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) < 20 {
		return o, invalid()
	}
	o.StartTicks, e = strconv.ParseUint(fields[19], 10, 64)
	if e != nil {
		return o, invalid()
	}
	if o.Cgroup != "/system.slice/"+p.Unit {
		return o, invalid()
	}
	cg := filepath.Join("/sys/fs/cgroup", o.Cgroup)
	o.MemoryMax, e = readNumber(filepath.Join(cg, "memory.max"))
	if e != nil {
		return o, e
	}
	o.SwapMax, e = readNumber(filepath.Join(cg, "memory.swap.max"))
	if e != nil {
		return o, e
	}
	o.TasksMax, e = readNumber(filepath.Join(cg, "pids.max"))
	if e != nil {
		return o, e
	}
	raw, e = os.ReadFile(filepath.Join(cg, "cpu.max"))
	if e != nil {
		return o, invalid()
	}
	cpu := strings.Fields(string(raw))
	if len(cpu) != 2 {
		return o, invalid()
	}
	o.CPUQuota, e = strconv.ParseInt(cpu[0], 10, 64)
	if e != nil {
		return o, invalid()
	}
	o.CPUPeriod, e = strconv.ParseInt(cpu[1], 10, 64)
	if e != nil {
		return o, invalid()
	}
	raw, e = os.ReadFile(filepath.Join(proc, "mountinfo"))
	if e != nil || len(raw) > 1<<20 {
		return o, invalid()
	}
	o.Mounts, e = parseMounts(string(raw))
	if e != nil {
		return o, e
	}
	return o, o.Validate(h.config, p)
}
