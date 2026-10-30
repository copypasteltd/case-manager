package servicestatus

// Host resources have their own evidence and state. Endpoint availability must
// never be used as a substitute for CPU, memory or filesystem observations.
import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
)

type ResourceThreshold struct {
	Warning  int `json:"warning_bp"`
	Critical int `json:"critical_bp"`
}
type ResourceConfig struct {
	Name        string            `json:"name"`
	PublicKey   string            `json:"public_key"`
	Enabled     bool              `json:"enabled"`
	StaleAfter  int               `json:"stale_after_seconds"`
	CPU         ResourceThreshold `json:"cpu"`
	Memory      ResourceThreshold `json:"memory"`
	Disk        ResourceThreshold `json:"disk"`
	Mountpoints []string          `json:"mountpoints"`
}
type ResourceConfigInput struct {
	ResourceConfig
	ExpectedVersion       int64   `json:"expected_version"`
	ExpectedSourceVersion int64   `json:"expected_source_version"`
	BaseRevisionID        *string `json:"base_revision_id"`
	Reason                string  `json:"reason"`
}
type ResourceSource struct {
	ResourceConfig
	ID          string    `json:"id"`
	RevisionID  string    `json:"revision_id"`
	Version     int64     `json:"version"`
	PublishedBy string    `json:"published_by"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}
type ResourceRevision struct {
	ResourceSource
	BaseRevisionID *string `json:"base_revision_id"`
	IsCurrent      bool    `json:"is_current"`
}
type MemorySample struct {
	Total     string `json:"total_bytes"`
	Available string `json:"available_bytes"`
}
type DiskSample struct {
	Mountpoint string `json:"mountpoint"`
	MemorySample
	ReadOnly bool `json:"read_only"`
}
type ResourceMetrics struct {
	CPUBP       int          `json:"cpu_usage_bp"`
	Memory      MemorySample `json:"memory"`
	Filesystems []DiskSample `json:"filesystems"`
}
type SignedResourceObservation struct {
	RevisionID string    `json:"source_revision_id"`
	Sequence   string    `json:"sequence"`
	ObservedAt time.Time `json:"observed_at"`
	ResourceMetrics
}
type ResourceObservation struct {
	SignedResourceObservation
	ID            string    `json:"id"`
	SourceID      string    `json:"source_id"`
	ReceivedAt    time.Time `json:"received_at"`
	PayloadSHA256 string    `json:"payload_sha256"`
	Applied       bool      `json:"applied"`
	IgnoredReason string    `json:"ignored_reason"`
}
type ResourceProof struct {
	ResourceObservation
	SignedPayload string `json:"signed_payload"`
	Signature     string `json:"signature"`
	PublicKey     string `json:"public_key"`
}
type ResourceView struct {
	ServiceID      string               `json:"service_id"`
	ServiceVersion int64                `json:"service_version"`
	Source         *ResourceSource      `json:"source"`
	State          string               `json:"state"`
	StateReason    string               `json:"state_reason"`
	EvaluatedAt    time.Time            `json:"evaluated_at"`
	RecheckAt      time.Time            `json:"recheck_at"`
	Observation    *ResourceObservation `json:"observation"`
	CanManage      bool                 `json:"can_manage"`
}

func ResourceSigningMessage(source string, body []byte) []byte {
	return append([]byte("CP-HOST-V1\n"+strings.ToLower(source)+"\n"), body...)
}
func validMountpoint(s string) bool {
	return len(s) > 0 && len([]rune(s)) <= 200 && path.IsAbs(s) && path.Clean(s) == s && !strings.ContainsAny(s, "\x00\r\n\t\\")
}
func (c ResourceConfig) validate() error {
	if core.Name(c.Name) != nil || strings.ContainsAny(c.Name, "\x00\r\n") || c.StaleAfter < 45 || c.StaleAfter > 180 || len(c.Mountpoints) < 1 || len(c.Mountpoints) > 16 {
		return core.Invalid("resource_monitor", "请填写名称、45至180秒的过期阈值，以及1至16个完整的受监控挂载点")
	}
	for _, t := range []ResourceThreshold{c.CPU, c.Memory, c.Disk} {
		if t.Warning < 1 || t.Warning >= t.Critical || t.Critical > 10000 {
			return core.Invalid("threshold", "警告与严重阈值须为递增的1至10000基点")
		}
	}
	seen := map[string]bool{}
	for _, m := range c.Mountpoints {
		if !validMountpoint(m) || seen[m] {
			return core.Invalid("mountpoints", "挂载点须为唯一的完整Linux绝对路径")
		}
		seen[m] = true
	}
	return nil
}
func resourceBytes(s string, positive bool) (int64, bool) {
	n, e := strconv.ParseInt(s, 10, 64)
	return n, e == nil && strconv.FormatInt(n, 10) == s && n >= 0 && (!positive || n > 0)
}
func (m MemorySample) valid() bool {
	t, a := resourceBytes(m.Total, true)
	v, b := resourceBytes(m.Available, false)
	return a && b && v <= t
}

// Integer math avoids losing byte precision or overflowing when calculating a
// percentage of a signed 64-bit capacity. Round up so a threshold is not hidden.
func UsageBasisPoints(m MemorySample) int {
	total, _ := resourceBytes(m.Total, true)
	avail, _ := resourceBytes(m.Available, false)
	if total <= 0 || avail < 0 || avail > total {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(total-avail), big.NewInt(10000))
	n.Add(n, big.NewInt(total-1))
	n.Div(n, big.NewInt(total))
	return int(n.Int64())
}
func (m ResourceMetrics) Validate(mounts []string) error {
	if m.CPUBP < 0 || m.CPUBP > 10000 || !m.Memory.valid() || len(m.Filesystems) < 1 || len(m.Filesystems) > 16 {
		return core.Invalid("metrics", "资源指标不完整或超出范围")
	}
	seen := map[string]bool{}
	for _, d := range m.Filesystems {
		if !validMountpoint(d.Mountpoint) || seen[d.Mountpoint] || !d.MemorySample.valid() {
			return core.Invalid("filesystems", "文件系统指标无效")
		}
		seen[d.Mountpoint] = true
	}
	if mounts != nil {
		if len(mounts) != len(seen) {
			return core.Invalid("filesystems", "指标须覆盖当前配置的全部挂载点")
		}
		for _, p := range mounts {
			if !seen[p] {
				return core.Invalid("filesystems", "指标须覆盖当前配置的全部挂载点")
			}
		}
	}
	return nil
}

// Exact field names, duplicate rejection, explicit non-null fields and a bounded
// nesting depth are checked before decoding the signed bytes.
func strictResourceObject(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, e := d.Token()
	if e != nil || start != json.Delim('{') {
		return nil, core.Invalid("observation", "指标对象无效")
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		token, e := d.Token()
		name, ok := token.(string)
		if e != nil || !ok || !slices.Contains(names, name) || out[name] != nil {
			return nil, core.Invalid("observation", "指标存在未知或重复字段")
		}
		var v json.RawMessage
		if e = d.Decode(&v); e != nil || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, core.Invalid("observation", "指标字段不可为空")
		}
		out[name] = v
	}
	if _, e = d.Token(); e != nil || len(out) != len(names) {
		return nil, core.Invalid("observation", "指标必须包含所有字段")
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, core.Invalid("observation", "指标存在多余内容")
	}
	return out, nil
}
func ParseResourceObservation(body []byte) (SignedResourceObservation, error) {
	var out SignedResourceObservation
	if len(body) == 0 || len(body) > 16384 {
		return out, core.Invalid("observation", "资源观测不得超过16384字节")
	}
	top, e := strictResourceObject(body, "source_revision_id", "sequence", "observed_at", "cpu_usage_bp", "memory", "filesystems")
	if e != nil {
		return out, e
	}
	if _, e = strictResourceObject(top["memory"], "total_bytes", "available_bytes"); e != nil {
		return out, e
	}
	var disks []json.RawMessage
	if json.Unmarshal(top["filesystems"], &disks) != nil || len(disks) < 1 || len(disks) > 16 {
		return out, core.Invalid("filesystems", "须提交全部受监控的文件系统")
	}
	for _, d := range disks {
		if _, e = strictResourceObject(d, "mountpoint", "total_bytes", "available_bytes", "read_only"); e != nil {
			return out, e
		}
	}
	if json.Unmarshal(body, &out) != nil || !core.ValidID(out.RevisionID) || out.ObservedAt.IsZero() {
		return out, core.Invalid("observation", "资源观测格式无效")
	}
	n, e := strconv.ParseInt(out.Sequence, 10, 64)
	if e != nil || n < 1 || n > 9007199254740991 || strconv.FormatInt(n, 10) != out.Sequence {
		return out, core.Invalid("sequence", "观测序号必须为准确递增的安全整数")
	}
	return out, out.ResourceMetrics.Validate(nil)
}
func resourceState(c ResourceConfig, m ResourceMetrics) string {
	result := "operational"
	apply := func(value int, t ResourceThreshold) {
		if value >= t.Critical {
			result = "critical"
		} else if value >= t.Warning && result != "critical" {
			result = "degraded"
		}
	}
	apply(m.CPUBP, c.CPU)
	apply(UsageBasisPoints(m.Memory), c.Memory)
	for _, d := range m.Filesystems {
		apply(UsageBasisPoints(d.MemorySample), c.Disk)
		if d.ReadOnly {
			result = "critical"
		}
	}
	return result
}
