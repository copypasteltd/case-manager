package nativeruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Mount struct {
	Target, Type          string
	Options, SuperOptions []string
}
type Observation struct {
	Unit                                                                       string `json:"unit"`
	PID                                                                        int    `json:"pid"`
	UID, GID                                                                   uint32
	StartTicks                                                                 uint64 `json:"start_ticks"`
	Cgroup                                                                     string `json:"cgroup"`
	ProcessCgroup                                                              string `json:"process_cgroup"`
	MountNamespace, HostMountNamespace, NetworkNamespace, HostNetworkNamespace uint64
	RootDevice, RootInode, ExpectedRootDevice, ExpectedRootInode               uint64
	CapEffective, CapPermitted, CapBounding                                    string
	NoNewPrivileges, Seccomp                                                   int
	MemoryMax, SwapMax, TasksMax, CPUQuota, CPUPeriod                          int64
	Properties                                                                 map[string]string `json:"properties"`
	Mounts                                                                     []Mount           `json:"mounts"`
	ReleaseSHA256                                                              string            `json:"release_sha256"`
	InspectedAt                                                                time.Time         `json:"inspected_at"`
}

func (o Observation) Validate(c Config, p UnitPlan) error {
	if o.Unit != p.Unit || o.PID <= 1 || o.UID == 0 || o.UID == c.WorkerUID || o.GID == 0 || o.StartTicks == 0 || !strings.HasPrefix(o.Cgroup, "/system.slice/"+p.Unit) || o.Cgroup != "/system.slice/"+p.Unit || o.ProcessCgroup != o.Cgroup || o.MountNamespace == 0 || o.NetworkNamespace == 0 || o.MountNamespace == o.HostMountNamespace || o.NetworkNamespace == o.HostNetworkNamespace || o.RootInode == 0 || o.RootInode != o.ExpectedRootInode || o.RootDevice != o.ExpectedRootDevice || o.NoNewPrivileges != 1 || o.Seccomp != 2 || !zeroHex(o.CapEffective) || !zeroHex(o.CapPermitted) || !zeroHex(o.CapBounding) || o.ReleaseSHA256 != c.ReleaseSHA256 {
		return invalid()
	}
	if o.MemoryMax != c.Limits.MemoryBytes || o.SwapMax != 0 || o.TasksMax != int64(c.Limits.Tasks) || o.CPUQuota <= 0 || o.CPUQuota > 2000000 || o.CPUPeriod < 1000 || o.CPUPeriod > 1000000 || o.CPUQuota*100 > o.CPUPeriod*int64(c.Limits.CPUPercent) {
		return invalid()
	}
	for _, key := range []string{"DynamicUser", "NoNewPrivileges", "PrivateNetwork", "PrivateMounts", "PrivateDevices", "PrivateIPC", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs", "ProtectControlGroups", "ProtectClock", "RestrictRealtime", "RestrictSUIDSGID", "LockPersonality", "SendSIGKILL"} {
		if o.Properties[key] != "yes" {
			return invalid()
		}
	}
	for key, value := range map[string]string{"RootDirectory": p.Root, "WorkingDirectory": c.WorkDirectory, "ProtectSystem": "strict", "ProtectHome": "yes", "ProtectProc": "invisible", "ProcSubset": "pid", "KillMode": "control-group", "OOMPolicy": "kill", "Restart": "no", "Delegate": "no", "CapabilityBoundingSet": "", "AmbientCapabilities": "", "ActiveState": "active", "SubState": "running"} {
		if o.Properties[key] != value {
			return invalid()
		}
	}
	if o.Properties["RestrictNamespaces"] != "yes" && o.Properties["RestrictNamespaces"] != "0" {
		return invalid()
	}
	if !sameTools(strings.Fields(o.Properties["RestrictAddressFamilies"]), []string{"AF_UNIX", "AF_INET"}) || o.Properties["NoExecPaths"] != "/" {
		return invalid()
	}
	arch := map[string]string{"amd64": "x86-64", "arm64": "arm64"}[runtime.GOARCH]
	if o.Properties["SystemCallArchitectures"] != "native" && (arch == "" || o.Properties["SystemCallArchitectures"] != arch) {
		return invalid()
	}
	execPaths := append([]string{c.Node.Path, c.Codex.Path, c.Broker.Path}, executableLibraries(c.LibraryMounts)...)
	if !sameTools(strings.Fields(o.Properties["ExecPaths"]), execPaths) {
		return invalid()
	}
	filter := strings.TrimPrefix(o.Properties["SystemCallFilter"], "~")
	// systemd expands groups in D-Bus output. Require the explicit critical
	// denied syscalls in that actual resolved set, not the original group text.
	if !strings.HasPrefix(o.Properties["SystemCallFilter"], "~") {
		return invalid()
	}
	denied := strings.Fields(filter)
	for _, name := range criticalDeniedSyscalls {
		if !has(denied, name) {
			return invalid()
		}
	}
	for _, target := range append([]string{c.Release, c.Node.Path, c.Codex.Path, c.Broker.Path}, c.LibraryMounts...) {
		found := false
		for _, m := range o.Mounts {
			if m.Target == target && has(m.Options, "ro") {
				found = true
			}
		}
		if !found {
			return invalid()
		}
	}
	for target, limit := range map[string]int64{c.WorkDirectory: c.Limits.WorkBytes, "/tmp": c.Limits.TempBytes} {
		found := false
		for _, m := range o.Mounts {
			if m.Target != target || m.Type != "tmpfs" {
				continue
			}
			size := mountSize(m.SuperOptions)
			if has(m.Options, "rw") && has(m.Options, "nosuid") && has(m.Options, "nodev") && has(m.Options, "noexec") && size > 0 && size <= limit {
				found = true
			}
		}
		if !found {
			return invalid()
		}
	}
	return nil
}
func zeroHex(value string) bool {
	n, err := strconv.ParseUint(value, 16, 64)
	return err == nil && value != "" && n == 0
}
func has(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func mountSize(options []string) int64 {
	for _, v := range options {
		if !strings.HasPrefix(v, "size=") {
			continue
		}
		v = strings.TrimPrefix(v, "size=")
		multiplier := int64(1)
		if strings.HasSuffix(v, "k") {
			multiplier = 1024
			v = strings.TrimSuffix(v, "k")
		}
		if strings.HasSuffix(v, "m") {
			multiplier = 1024 * 1024
			v = strings.TrimSuffix(v, "m")
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n > 0 && n <= 1<<40 {
			return n * multiplier
		}
	}
	return -1
}
func configurationDigest(c Config) string {
	raw, _ := json.Marshal(c)
	v := sha256.Sum256(raw)
	return hex.EncodeToString(v[:])
}
