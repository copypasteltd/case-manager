// Package statusbridge connects explicitly configured Prometheus/Blackbox
// samples to the original signed observation service. It never probes targets.
package statusbridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
)

// Error codes are intentionally safe for unattended logs. Never include URL,
// query text, response bodies, local credential paths or driver diagnostics.
type Fault struct{ Code string }

func (e *Fault) Error() string { return e.Code }
func fault(code string) error  { return &Fault{Code: code} }

type Endpoint struct {
	URL          string   `json:"url"`
	AllowedIPs   []string `json:"allowed_ips"`
	AllowPrivate bool     `json:"allow_private"`
	AllowHTTP    bool     `json:"allow_http"`
}

type Target struct {
	Kind        string             `json:"kind,omitempty"`
	Filesystems []FilesystemTarget `json:"filesystems,omitempty"`
	SourceID    string             `json:"source_id"`
	RevisionID  string             `json:"source_revision_id"`
	KeyFile     string             `json:"key_file"`
	Job         string             `json:"job"`
	Instance    string             `json:"instance"`
	Labels      map[string]string  `json:"labels,omitempty"`
}

type Config struct {
	Version        int      `json:"version"`
	Platform       Endpoint `json:"platform"`
	Prometheus     Endpoint `json:"prometheus"`
	BearerFile     string   `json:"prometheus_bearer_file,omitempty"`
	StateDirectory string   `json:"state_directory"`
	PollSeconds    int      `json:"poll_seconds"`
	MaxSampleAge   int      `json:"max_sample_age_seconds"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Concurrency    int      `json:"concurrency"`
	Targets        []Target `json:"targets"`
}

func LoadConfig(filename string) (Config, error) {
	var out Config
	body, err := boundedFile(filename, 64<<10, false)
	if err != nil || uniqueJSON(body) != nil {
		return out, fault("CONFIG_INVALID")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || out.Validate() != nil {
		return Config{}, fault("CONFIG_INVALID")
	}
	return out, nil
}

func (c Config) Validate() error {
	if c.Version != 1 || !filepath.IsAbs(c.StateDirectory) || len(c.Targets) < 1 || len(c.Targets) > 100 ||
		c.PollSeconds < 15 || c.PollSeconds > 60 || c.MaxSampleAge < 3*c.PollSeconds || c.MaxSampleAge > 180 ||
		c.TimeoutSeconds < 1 || c.TimeoutSeconds > 10 || c.Concurrency < 1 || c.Concurrency > 8 || c.BearerFile != "" && !filepath.IsAbs(c.BearerFile) {
		return fault("CONFIG_INVALID")
	}
	if _, err := endpointURL(c.Platform); err != nil {
		return err
	}
	if _, err := endpointURL(c.Prometheus); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if !core.ValidID(t.SourceID) || t.SourceID != strings.ToLower(t.SourceID) || !core.ValidID(t.RevisionID) ||
			t.RevisionID != strings.ToLower(t.RevisionID) || !filepath.IsAbs(t.KeyFile) || seen[t.SourceID] ||
			strings.TrimSpace(t.Job) != t.Job || strings.TrimSpace(t.Instance) != t.Instance || t.Job == "" || t.Instance == "" ||
			len(t.Job) > 200 || len(t.Instance) > 2048 || strings.ContainsAny(t.Job+t.Instance, "\x00\r\n") {
			return fault("CONFIG_INVALID")
		}
		seen[t.SourceID] = true
		if err := validateResourceTarget(t); err != nil {
			return err
		}
		if len(t.Labels) > 16 {
			return fault("CONFIG_INVALID")
		}
		for name, value := range t.Labels {
			if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(name) || strings.HasPrefix(name, "__") || name == "job" || name == "instance" || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
				return fault("CONFIG_INVALID")
			}
		}
	}
	return nil
}

func boundedFile(filename string, limit int64, secret bool) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || info.Size() == 0 ||
		secret && runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fault("PRIVATE_FILE_INVALID")
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, fault("PRIVATE_FILE_INVALID")
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fault("PRIVATE_FILE_INVALID")
	}
	return body, nil
}

func loadKey(filename string) (ed25519.PrivateKey, error) {
	b, err := boundedFile(filename, 256, true)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fault("SIGNING_KEY_INVALID")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func binding(c Config, t Target, key ed25519.PrivateKey) string {
	// Credentials/paths are excluded. A URL, target, key or timing change within
	// one published revision requires explicit operator reconciliation.
	body, _ := json.Marshal(struct {
		Platform, Prometheus                       Endpoint
		Source, Revision, Job, Instance, PublicKey string
		Labels                                     map[string]string
		Poll, MaxAge, Timeout                      int
	}{c.Platform, c.Prometheus, t.SourceID, t.RevisionID, t.Job, t.Instance,
		base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), t.Labels, c.PollSeconds, c.MaxSampleAge, c.TimeoutSeconds})
	d := sha256.Sum256(body)
	if t.Kind == "host" {
		// Preserve every legacy Blackbox journal binding byte-for-byte. Host
		// journals additionally pin their kind and complete filesystem inventory.
		extra, _ := json.Marshal(struct {
			Kind        string
			Filesystems []FilesystemTarget
		}{t.Kind, t.Filesystems})
		d = sha256.Sum256(append(append(body, '\n'), extra...))
	}
	return hex.EncodeToString(d[:])
}

// Reject ambiguous duplicate keys before ordinary decoding, including nested
// objects. Response additions remain forward compatible; malformed JSON does not.
func uniqueJSON(body []byte) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var read func(int) error
	read = func(depth int) error {
		if depth > 16 {
			return errors.New("depth")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := k.(string)
				if !ok || seen[name] {
					return errors.New("duplicate")
				}
				seen[name] = true
				if err = read(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case json.Delim('['):
			for d.More() {
				if err = read(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := read(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing")
	}
	return nil
}

func timeout(c Config) time.Duration { return time.Duration(c.TimeoutSeconds) * time.Second }
