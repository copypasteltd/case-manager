package nativeruntime

import (
	"path"
	"strconv"
	"strings"
	"time"
)

type UnitPlan struct {
	Unit       string    `json:"unit"`
	RunID      string    `json:"run_id"`
	Root       string    `json:"root"`
	Gateway    string    `json:"gateway"`
	ExpiresAt  time.Time `json:"expires_at"`
	Properties []string  `json:"properties"`
	Command    []string  `json:"command"`
}

var criticalDeniedSyscalls = []string{"mount", "umount2", "pivot_root", "chroot", "ptrace", "reboot", "swapon", "swapoff", "init_module", "finit_module", "delete_module", "kexec_load", "clock_settime", "settimeofday", "adjtimex", "perf_event_open"}

func unitName(id string) string { return "cp-agent-" + strings.ReplaceAll(id, "-", "") + ".service" }
func (c Config) Plan(in WorkerInput, now time.Time) (UnitPlan, error) {
	if _, err := c.ValidateRun(in, c.Node.Path, c.Entry, now); err != nil {
		return UnitPlan{}, err
	}
	id := in.Run.RunID
	directory := path.Join(c.StateDirectory, "runs", id)
	p := UnitPlan{Unit: unitName(id), RunID: id, Root: path.Join(directory, "root"), Gateway: path.Join(directory, "gateway.sock"), ExpiresAt: in.Run.ExpiresAt}
	// Use the supplied clock, not a different wall-clock sample, for the plan.
	seconds := int(in.Run.ExpiresAt.Sub(now).Seconds()) + 1
	if seconds > c.Limits.RuntimeSeconds {
		seconds = c.Limits.RuntimeSeconds
	}
	p.Properties = []string{
		"Type=exec", "DynamicUser=yes", "User=cp-agent-" + strings.ReplaceAll(id, "-", "")[:16],
		"RootDirectory=" + p.Root, "WorkingDirectory=" + c.WorkDirectory,
		"NoNewPrivileges=yes", "CapabilityBoundingSet=", "AmbientCapabilities=",
		"PrivateNetwork=yes", "PrivateMounts=yes", "PrivateDevices=yes", "PrivateIPC=yes", "PrivateTmp=no",
		"ProtectSystem=strict", "ProtectHome=yes", "ProtectKernelTunables=yes", "ProtectKernelModules=yes", "ProtectKernelLogs=yes", "ProtectControlGroups=yes", "ProtectClock=yes",
		"ProtectProc=invisible", "ProcSubset=pid", "RestrictNamespaces=yes", "RestrictRealtime=yes", "RestrictSUIDSGID=yes", "LockPersonality=yes",
		"RestrictAddressFamilies=AF_UNIX AF_INET", "SystemCallArchitectures=native",
		"SystemCallFilter=~@mount @reboot @raw-io @swap @privileged @clock @module @debug " + strings.Join(criticalDeniedSyscalls, " "),
		"IPAddressDeny=any", "IPAddressAllow=127.0.0.1/32",
		"MemoryMax=" + strconv.FormatInt(c.Limits.MemoryBytes, 10), "MemorySwapMax=0", "TasksMax=" + strconv.Itoa(c.Limits.Tasks), "CPUQuota=" + strconv.Itoa(c.Limits.CPUPercent) + "%",
		"RuntimeMaxSec=" + strconv.Itoa(seconds), "TimeoutStopSec=2", "KillMode=control-group", "SendSIGKILL=yes", "OOMPolicy=kill", "Restart=no",
		"LimitCORE=0", "LimitNOFILE=256", "UMask=0077", "StandardOutput=null", "StandardError=null",
		"TemporaryFileSystem=" + c.WorkDirectory + ":rw,nosuid,nodev,noexec,mode=1777,size=" + strconv.FormatInt(c.Limits.WorkBytes, 10) + " /tmp:rw,nosuid,nodev,noexec,mode=1777,size=" + strconv.FormatInt(c.Limits.TempBytes, 10),
		"ReadWritePaths=" + c.WorkDirectory + " /tmp",
	}
	binds := []string{c.Release, c.Node.Path, c.Codex.Path, c.Broker.Path}
	binds = append(binds, c.LibraryMounts...)
	for _, b := range binds {
		p.Properties = append(p.Properties, "BindReadOnlyPaths="+b)
	}
	p.Properties = append(p.Properties, "BindReadOnlyPaths="+p.Gateway+":"+SocketTarget)
	p.Properties = append(p.Properties, "NoExecPaths=/", "ExecPaths="+strings.Join(append([]string{c.Node.Path, c.Codex.Path, c.Broker.Path}, executableLibraries(c.LibraryMounts)...), " "))
	p.Command = []string{c.Broker.Path, "child", "--node", c.Node.Path, "--entry", c.Entry, "--bridge", c.BridgeAddress, "--model", c.ModelAddress, "--work", c.WorkDirectory}
	return p, nil
}
func executableLibraries(mounts []string) []string {
	var out []string
	for _, p := range mounts {
		if p == "/lib" || p == "/lib64" || p == "/usr/lib" {
			out = append(out, p)
		}
	}
	return out
}
func (p UnitPlan) Arguments() []string {
	out := []string{"--system", "--quiet", "--wait", "--pipe", "--collect", "--expand-environment=no", "--unit=" + p.Unit}
	for _, property := range p.Properties {
		out = append(out, "--property="+property)
	}
	return append(append(out, "--"), p.Command...)
}
