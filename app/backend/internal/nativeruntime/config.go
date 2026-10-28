// Package nativeruntime owns the native Linux boundary outside the model process.
// Its production entry points require root, systemd and cgroup v2; tests never
// substitute a successful confinement result for an unavailable kernel boundary.
package nativeruntime

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
)

const MaxInput = 1 << 20
const GatewayKey = "native-broker-managed"
const SocketTarget = "/run/copypaste-gateway.sock"

var cleanPath = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,100}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var credentialPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
var authorityPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,200}$`)

type Limits struct {
	MemoryBytes    int64 `json:"memory_bytes"`
	CPUPercent     int   `json:"cpu_percent"`
	Tasks          int   `json:"tasks"`
	RuntimeSeconds int   `json:"runtime_seconds"`
	WorkBytes      int64 `json:"work_bytes"`
	TempBytes      int64 `json:"temp_bytes"`
}
type FilePin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Profile struct {
	ID                  string   `json:"id"`
	Model               string   `json:"model"`
	SkillSHA256         string   `json:"skill_sha256"`
	ConfigurationSHA256 string   `json:"configuration_sha256"`
	Tools               []string `json:"tools"`
}
type Config struct {
	Socket         string    `json:"socket"`
	WorkerUID      uint32    `json:"worker_uid"`
	WorkerGID      uint32    `json:"worker_gid"`
	StateDirectory string    `json:"state_directory"`
	Release        string    `json:"release"`
	ReleaseSHA256  string    `json:"release_sha256"`
	Node           FilePin   `json:"node"`
	Codex          FilePin   `json:"codex"`
	Broker         FilePin   `json:"broker"`
	Entry          string    `json:"entry"`
	WorkDirectory  string    `json:"work_directory"`
	BridgeAddress  string    `json:"bridge_address"`
	ModelAddress   string    `json:"model_address"`
	ModelOrigin    string    `json:"model_origin"`
	ModelKeyFile   string    `json:"model_key_file"`
	SigningKeyFile string    `json:"signing_key_file"`
	LibraryMounts  []string  `json:"library_mounts"`
	MaxConcurrency int       `json:"max_concurrency"`
	Limits         Limits    `json:"limits"`
	Profiles       []Profile `json:"profiles"`
}
type WorkerInput struct {
	Repository string                     `json:"repository"`
	Settings   agentworker.NativeSettings `json:"settings"`
	Run        agent.Execution            `json:"run"`
}

func invalid() error { return errors.New("native runtime configuration or binding refused") }
func absolute(value string) bool {
	return cleanPath.MatchString(value) && path.Clean(value) == value && value != "/" && !strings.HasSuffix(value, "/")
}
func descendant(base, value string) bool { return strings.HasPrefix(value, base+"/") }
func loopbackAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	return err == nil && host == "127.0.0.1" && regexp.MustCompile(`^[1-9][0-9]{3,4}$`).MatchString(port) && validPort(port)
}
func validPort(value string) bool {
	var number int
	for _, c := range value {
		number = number*10 + int(c-'0')
	}
	return number >= 1024 && number <= 65535
}
func (c Config) Validate() error {
	for _, p := range []string{c.Socket, c.StateDirectory, c.Release, c.Node.Path, c.Codex.Path, c.Broker.Path, c.Entry, c.WorkDirectory, c.ModelKeyFile, c.SigningKeyFile} {
		if !absolute(p) {
			return invalid()
		}
	}
	if c.WorkerUID == 0 || c.WorkerGID == 0 || c.MaxConcurrency < 1 || c.MaxConcurrency > 3 || !descendant(c.Release, c.Entry) || c.Entry != path.Join(c.Release, "app/agent-runtime/dist/worker.js") {
		return invalid()
	}
	// The read-only release must be a dedicated runtime bundle, never the working
	// checkout containing databases, cloud keys, signing material or caches.
	for _, p := range []string{c.StateDirectory, c.WorkDirectory, c.ModelKeyFile, c.SigningKeyFile, c.Socket} {
		if p == c.Release || descendant(c.Release, p) || descendant(p, c.Release) {
			return invalid()
		}
	}
	if !descendant(c.StateDirectory, c.Socket) || descendant(c.StateDirectory, c.WorkDirectory) || descendant(c.WorkDirectory, c.StateDirectory) || c.WorkDirectory == "/tmp" || descendant("/tmp", c.WorkDirectory) {
		return invalid()
	}
	for _, p := range []string{c.Node.Path, c.Codex.Path, c.Broker.Path} {
		for _, writable := range []string{c.StateDirectory, c.WorkDirectory, "/tmp"} {
			if p == writable || descendant(writable, p) || descendant(p, writable) {
				return invalid()
			}
		}
	}
	if !digestPattern.MatchString(c.ReleaseSHA256) || !digestPattern.MatchString(c.Node.SHA256) || !digestPattern.MatchString(c.Codex.SHA256) || !digestPattern.MatchString(c.Broker.SHA256) {
		return invalid()
	}
	if !loopbackAddress(c.BridgeAddress) || !loopbackAddress(c.ModelAddress) || c.BridgeAddress == c.ModelAddress {
		return invalid()
	}
	u, err := url.Parse(c.ModelOrigin)
	if err != nil || u.Scheme != "https" || u.Host != "api.openai.com" || u.Path != "/v1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return invalid()
	}
	allowed := []string{"/lib", "/lib64", "/usr/lib", "/etc/ld.so.cache", "/etc/ssl/certs"}
	if len(c.LibraryMounts) == 0 || len(c.LibraryMounts) > len(allowed) {
		return invalid()
	}
	seen := map[string]bool{}
	for _, p := range c.LibraryMounts {
		if !slices.Contains(allowed, p) || seen[p] {
			return invalid()
		}
		seen[p] = true
	}
	for _, mount := range append([]string{c.Release, c.Node.Path, c.Codex.Path, c.Broker.Path}, c.LibraryMounts...) {
		for _, secret := range []string{c.ModelKeyFile, c.SigningKeyFile} {
			if mount == secret || descendant(mount, secret) {
				return invalid()
			}
		}
		for _, writable := range []string{c.WorkDirectory, "/tmp", c.StateDirectory} {
			if mount == writable || descendant(mount, writable) || descendant(writable, mount) {
				return invalid()
			}
		}
	}
	l := c.Limits
	if l.MemoryBytes < 256<<20 || l.MemoryBytes > 4<<30 || l.CPUPercent < 10 || l.CPUPercent > 200 || l.Tasks < 16 || l.Tasks > 128 || l.RuntimeSeconds < 30 || l.RuntimeSeconds > 1800 || l.WorkBytes < 16<<20 || l.WorkBytes > 1<<30 || l.TempBytes < 8<<20 || l.TempBytes > 256<<20 || l.WorkBytes+l.TempBytes > l.MemoryBytes/2 {
		return invalid()
	}
	if len(c.Profiles) == 0 || len(c.Profiles) > 32 {
		return invalid()
	}
	seen = map[string]bool{}
	for _, p := range c.Profiles {
		key := p.ID + "/" + p.ConfigurationSHA256
		if !uuidPattern.MatchString(p.ID) || !identifier.MatchString(p.Model) || !digestPattern.MatchString(p.SkillSHA256) || !digestPattern.MatchString(p.ConfigurationSHA256) || seen[key] || len(p.Tools) == 0 || len(p.Tools) > 50 {
			return invalid()
		}
		seen[key] = true
		tools := map[string]bool{}
		for _, tool := range p.Tools {
			if !strings.HasPrefix(tool, "company/") || !regexp.MustCompile(`^company/[a-z_]{3,60}$`).MatchString(tool) || tools[tool] {
				return invalid()
			}
			tools[tool] = true
		}
	}
	return nil
}
func Decode(reader io.Reader, target any, maximum int64) error {
	raw, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(raw)) > maximum || len(raw) == 0 || raw[0] != '{' {
		return invalid()
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return invalid()
	}
	return nil
}
func (c Config) ValidateRun(in WorkerInput, node, entry string, now time.Time) (Profile, error) {
	if node != c.Node.Path || entry != c.Entry || in.Repository != c.Release || !in.Settings.Enabled || in.Settings.WorkingDirectory != c.WorkDirectory || in.Settings.MCPEndpoint != "http://"+c.BridgeAddress+agentworker.MCPPath || in.Settings.APIKey != GatewayKey || in.Settings.APIBaseURL != "http://"+c.ModelAddress+"/v1" || in.Settings.ExecutablePath != c.Codex.Path {
		return Profile{}, invalid()
	}
	r := in.Run
	if !uuidPattern.MatchString(r.RunID) || !authorityPattern.MatchString(r.AuthorityStamp) || len(r.Credential) < 32 || len(r.Credential) > 4096 || !credentialPattern.MatchString(r.Credential) || !r.ExpiresAt.After(now.Add(10*time.Second)) || r.ExpiresAt.After(now.Add(time.Duration(c.Limits.RuntimeSeconds)*time.Second)) || r.MaxToolCalls < 1 || r.MaxInputTokens < 1 || r.MaxOutputTokens < 1 || len(r.PreparedContext) > 131072 || r.Profile.ID != in.Settings.ProfileID || r.Profile.RuntimeVersion != agentworker.RuntimeVersion || r.SkillSHA256 != r.Profile.SkillSHA256 {
		return Profile{}, invalid()
	}
	for _, p := range c.Profiles {
		if p.ID == r.Profile.ID && p.ConfigurationSHA256 == r.Profile.ConfigurationSHA256 && p.SkillSHA256 == r.SkillSHA256 && p.Model == in.Settings.Model {
			return p, nil
		}
	}
	return Profile{}, invalid()
}
