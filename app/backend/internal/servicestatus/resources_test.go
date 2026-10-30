package servicestatus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func resourceFixture() SignedResourceObservation {
	return SignedResourceObservation{RevisionID: "10000000-0000-4000-8000-000000000001", Sequence: "1", ObservedAt: time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC), ResourceMetrics: ResourceMetrics{CPUBP: 5000, Memory: MemorySample{Total: "9223372036854775807", Available: "1"}, Filesystems: []DiskSample{{Mountpoint: "/", MemorySample: MemorySample{Total: "10000", Available: "2000"}}}}}
}
func TestResourceObservationFormat(t *testing.T) {
	raw, _ := json.Marshal(resourceFixture())
	if _, e := ParseResourceObservation(raw); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"duplicate", "\"cpu_usage_bp\":5000", "\"cpu_usage_bp\":5000,\"cpu_usage_bp\":0"},
		{"case alias", "cpu_usage_bp", "CPU_USAGE_BP"}, {"nested duplicate", "\"read_only\":false", "\"read_only\":false,\"read_only\":true"},
		{"nested unknown", "\"read_only\":false", "\"read_only\":false,\"unreviewed\":true"},
		{"explicit boolean", "\"read_only\":false", "\"read_only\":null"},
		{"missing boolean", ",\"read_only\":false", ""}, {"cpu null", "\"cpu_usage_bp\":5000", "\"cpu_usage_bp\":null"},
		{"decimal byte", "9223372036854775807", "1.2"}, {"overflow byte", "9223372036854775807", "9223372036854775808"},
		{"unsafe sequence", "\"sequence\":\"1\"", "\"sequence\":\"9007199254740992\""},
		{"negative cpu", "\"cpu_usage_bp\":5000", "\"cpu_usage_bp\":-1"}, {"cpu overflow", "\"cpu_usage_bp\":5000", "\"cpu_usage_bp\":10001"},
		{"zero total", "\"total_bytes\":\"10000\"", "\"total_bytes\":\"0\""},
		{"over capacity", "\"available_bytes\":\"2000\"", "\"available_bytes\":\"10001\""},
		{"noncanonical mount", "\"mountpoint\":\"/\"", "\"mountpoint\":\"/a/../b\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(string(raw), tc.from, tc.to, 1)
			if changed == string(raw) {
				t.Fatal("fixture mutation missed")
			}
			if _, e := ParseResourceObservation([]byte(changed)); e == nil {
				t.Fatal("ambiguous or invalid signed observation accepted")
			}
		})
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), raw...), make([]byte, 16385)} {
		if _, e := ParseResourceObservation(bad); e == nil {
			t.Fatal("invalid size or trailing payload")
		}
	}
}
func TestResourceStateAndCapacity(t *testing.T) {
	cfg := ResourceConfig{Name: "host", Enabled: true, StaleAfter: 90, CPU: ResourceThreshold{8000, 9500}, Memory: ResourceThreshold{8000, 9500}, Disk: ResourceThreshold{8000, 9500}, Mountpoints: []string{"/", "/data"}}
	if cfg.validate() != nil {
		t.Fatal("valid config")
	}
	m := ResourceMetrics{CPUBP: 100, Memory: MemorySample{"9223372036854775807", "9223372036854775806"}, Filesystems: []DiskSample{{"/", MemorySample{"100", "99"}, false}, {"/data", MemorySample{"100", "99"}, false}}}
	if UsageBasisPoints(m.Memory) != 1 || resourceState(cfg, m) != "operational" || m.Validate(cfg.Mountpoints) != nil {
		t.Fatal("exact large capacity calculation")
	}
	m.Filesystems[1].Available = "20"
	if resourceState(cfg, m) != "degraded" {
		t.Fatal("disk warning boundary")
	}
	m.Filesystems[1].Available = "5"
	if resourceState(cfg, m) != "critical" {
		t.Fatal("disk critical boundary")
	}
	m.Filesystems[1].Available = "99"
	m.Filesystems[0].ReadOnly = true
	if resourceState(cfg, m) != "critical" {
		t.Fatal("read-only filesystem")
	}
	m.Filesystems = m.Filesystems[:1]
	if m.Validate(cfg.Mountpoints) == nil {
		t.Fatal("missing configured filesystem accepted")
	}
	cfg.Mountpoints = []string{"/", "/"}
	if cfg.validate() == nil {
		t.Fatal("duplicate inventory accepted")
	}
}
