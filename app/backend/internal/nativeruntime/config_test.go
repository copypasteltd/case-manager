package nativeruntime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
)

func fixtureConfig() Config {
	digest := strings.Repeat("a", 64)
	return Config{Socket: "/run/copypaste-agent-native/control.sock", WorkerUID: 1001, WorkerGID: 1001, StateDirectory: "/run/copypaste-agent-native", Release: "/opt/copypaste/runtime-release", ReleaseSHA256: digest,
		Node: FilePin{"/usr/bin/node", digest}, Codex: FilePin{"/opt/copypaste/bin/codex", digest}, Broker: FilePin{"/opt/copypaste/bin/agent-native", digest}, Entry: "/opt/copypaste/runtime-release/app/agent-runtime/dist/worker.js", WorkDirectory: "/var/lib/copypaste/agent-work",
		BridgeAddress: "127.0.0.1:18081", ModelAddress: "127.0.0.1:18082", ModelOrigin: "https://api.openai.com/v1", ModelKeyFile: "/etc/copypaste/private/model.key", SigningKeyFile: "/etc/copypaste/private/proof.key",
		LibraryMounts: []string{"/usr/lib", "/lib", "/lib64", "/etc/ld.so.cache"}, MaxConcurrency: 3, Limits: Limits{MemoryBytes: 1 << 30, CPUPercent: 100, Tasks: 64, RuntimeSeconds: 600, WorkBytes: 64 << 20, TempBytes: 16 << 20},
		Profiles: []Profile{{ID: "12345678-1234-4234-8234-123456789abc", Model: "company-approved-model", SkillSHA256: digest, ConfigurationSHA256: strings.Repeat("b", 64), Tools: []string{"company/read_resource", "company/inspect_operation"}}}}
}
func fixtureInput(c Config, now time.Time) WorkerInput {
	p := c.Profiles[0]
	return WorkerInput{Repository: c.Release, Settings: agentworker.NativeSettings{Enabled: true, Model: p.Model, WorkingDirectory: c.WorkDirectory, ProfileID: p.ID, MCPEndpoint: "http://" + c.BridgeAddress + agentworker.MCPPath, APIKey: GatewayKey, APIBaseURL: "http://" + c.ModelAddress + "/v1", ExecutablePath: c.Codex.Path},
		Run: agent.Execution{RunID: "abcdef12-1234-4234-8234-123456789abc", Channel: "web", AuthorityStamp: "current-native-authority-stamp", Credential: strings.Repeat("t", 40), SkillSHA256: p.SkillSHA256, ExpiresAt: now.Add(90 * time.Second), Input: map[string]any{}, PreparedContext: "Synthetic authorized material.", MaxToolCalls: 10, MaxInputTokens: 1000, MaxOutputTokens: 500,
			Profile: agent.RuntimeProfile{ID: p.ID, SkillID: "project-briefing", SkillVersion: "1.0.0", SkillSHA256: p.SkillSHA256, RuntimeVersion: agentworker.RuntimeVersion, ConfigurationSHA256: p.ConfigurationSHA256}}}
}
func clone[T any](value T) T {
	raw, _ := json.Marshal(value)
	var result T
	_ = json.Unmarshal(raw, &result)
	return result
}
func TestNativeRuntimePolicyRefusesUnsafeBoundaries(t *testing.T) {
	c := fixtureConfig()
	if c.Validate() != nil {
		t.Fatal("valid explicit policy refused")
	}
	cases := map[string]func(*Config){
		"secret inside OS mount": func(c *Config) { c.ModelKeyFile = "/usr/lib/provider.key" }, "executable inside tmp": func(c *Config) { c.Node.Path = "/tmp/node" }, "library overlap with work": func(c *Config) { c.WorkDirectory = "/usr/lib/work" },
		"privileged worker": func(c *Config) { c.WorkerUID = 0 }, "unit expansion in path": func(c *Config) { c.Release = "/opt/release%u" }, "traversal": func(c *Config) { c.WorkDirectory = "/var/../tmp/work" },
		"model redirect origin": func(c *Config) { c.ModelOrigin = "https://api.openai.com.evil.example/v1" }, "arbitrary network listener": func(c *Config) { c.ModelAddress = "0.0.0.0:18082" }, "colliding local services": func(c *Config) { c.ModelAddress = c.BridgeAddress },
		"secret mounted in release": func(c *Config) { c.ModelKeyFile = c.Release + "/key" }, "broad filesystem mount": func(c *Config) { c.LibraryMounts = []string{"/etc"} }, "model writes worker state": func(c *Config) { c.WorkDirectory = c.StateDirectory + "/work" },
		"unbounded memory": func(c *Config) { c.Limits.MemoryBytes = 0 }, "unbounded tasks": func(c *Config) { c.Limits.Tasks = 0 }, "unbounded time": func(c *Config) { c.Limits.RuntimeSeconds = 100000 }, "uncapped tmpfs": func(c *Config) { c.Limits.WorkBytes = 2 << 30 },
		"duplicate profile": func(c *Config) { c.Profiles = append(c.Profiles, c.Profiles[0]) }, "duplicate tool": func(c *Config) { c.Profiles[0].Tools = append(c.Profiles[0].Tools, c.Profiles[0].Tools[0]) }, "unknown tool namespace": func(c *Config) { c.Profiles[0].Tools = []string{"shell/execute"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := clone(c)
			mutate(&v)
			if v.Validate() == nil {
				t.Fatal("unsafe policy admitted")
			}
		})
	}
}
func TestNativeRuntimeRunBindsExactReleaseAndCredentials(t *testing.T) {
	c := fixtureConfig()
	now := time.Now()
	in := fixtureInput(c, now)
	if _, e := c.ValidateRun(in, c.Node.Path, c.Entry, now); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*WorkerInput){
		"wrong repository": func(i *WorkerInput) { i.Repository = "/srv/whole-company" }, "real provider credential in child": func(i *WorkerInput) { i.Settings.APIKey = "synthetic-host-only-provider-secret" }, "wrong gateway": func(i *WorkerInput) { i.Settings.APIBaseURL = "https://api.openai.com/v1" },
		"wrong executable": func(i *WorkerInput) { i.Settings.ExecutablePath = "/bin/sh" }, "profile changed": func(i *WorkerInput) { i.Run.Profile.ConfigurationSHA256 = strings.Repeat("c", 64) }, "expired run": func(i *WorkerInput) { i.Run.ExpiresAt = now.Add(-time.Second) }, "lifetime exceeds policy": func(i *WorkerInput) { i.Run.ExpiresAt = now.Add(time.Hour) },
		"wrong authority-free task": func(i *WorkerInput) { i.Run.AuthorityStamp = "" }, "missing task credential": func(i *WorkerInput) { i.Run.Credential = "" }, "model override": func(i *WorkerInput) { i.Settings.Model = "unapproved-model" }, "working directory override": func(i *WorkerInput) { i.Settings.WorkingDirectory = "/etc" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := clone(in)
			mutate(&v)
			if _, e := c.ValidateRun(v, c.Node.Path, c.Entry, now); e == nil {
				t.Fatal("changed run admitted")
			}
		})
	}
	if _, e := c.ValidateRun(in, "/bin/sh", c.Entry, now); e == nil {
		t.Fatal("launcher executable override admitted")
	}
	if _, e := c.ValidateRun(in, c.Node.Path, "/arbitrary/module.js", now); e == nil {
		t.Fatal("launcher module override admitted")
	}
}
func TestNativeRuntimePlanUsesAnOwnedUnitAndBoundedTmpfs(t *testing.T) {
	c := fixtureConfig()
	now := time.Now()
	p, e := c.Plan(fixtureInput(c, now), now)
	if e != nil {
		t.Fatal(e)
	}
	args := p.Arguments()
	raw := strings.Join(args, "\n")
	for _, v := range []string{"--expand-environment=no", "--property=DynamicUser=yes", "--property=PrivateNetwork=yes", "--property=MemorySwapMax=0", "--property=KillMode=control-group", "--property=NoNewPrivileges=yes", "size=67108864", "size=16777216", "--property=NoExecPaths=/", "--property=SystemCallFilter=~", "--property=RootDirectory=" + p.Root} {
		if !strings.Contains(raw, v) {
			t.Fatal("required boundary missing", v)
		}
	}
	if strings.Contains(raw, "-c\n") || strings.Contains(raw, "synthetic-host-only-provider-secret") || strings.Contains(raw, "current-native-authority-stamp") {
		t.Fatal("shell or credential leaked into command arguments")
	}
}
func TestNativeRuntimeFramesRejectAmbiguityAndOversizedInput(t *testing.T) {
	var out bytes.Buffer
	value := Request{Action: "prove"}
	if WriteFrame(&out, value) != nil {
		t.Fatal("encode failed")
	}
	var got Request
	if ReadFrame(&out, &got) != nil || got.Action != "prove" {
		t.Fatal("frame roundtrip failed")
	}
	for _, raw := range [][]byte{{0, 32, 0, 1}, {0, 0, 0, 2, '{'}, {0, 0, 0, 2, '[', ']'}} {
		if ReadFrame(bytes.NewReader(raw), &got) == nil {
			t.Fatal("invalid frame admitted")
		}
	}
	for _, raw := range []string{`{"action":"prove","unknown":true}`, `{"action":"prove"}{}`, `[]`} {
		if Decode(strings.NewReader(raw), &got, 100) == nil {
			t.Fatal("ambiguous request admitted")
		}
	}
}
