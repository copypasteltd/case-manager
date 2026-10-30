package statusbridge

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/servicestatus"
)

type FilesystemTarget struct {
	Mountpoint string `json:"mountpoint"`
	Device     string `json:"device"`
	FSType     string `json:"fstype"`
}

func validateResourceTarget(t Target) error {
	if t.Kind != "" && t.Kind != "blackbox" && t.Kind != "host" {
		return fault("CONFIG_INVALID")
	}
	if t.Kind != "host" {
		if len(t.Filesystems) != 0 {
			return fault("CONFIG_INVALID")
		}
		return nil
	}
	if len(t.Filesystems) < 1 || len(t.Filesystems) > 16 {
		return fault("CONFIG_INVALID")
	}
	for _, l := range []string{"cpu", "mode", "mountpoint", "device", "fstype"} {
		if _, ok := t.Labels[l]; ok {
			return fault("CONFIG_INVALID")
		}
	}
	seen := map[string]bool{}
	for _, f := range t.Filesystems {
		if !path.IsAbs(f.Mountpoint) || path.Clean(f.Mountpoint) != f.Mountpoint || len([]rune(f.Mountpoint)) > 200 || seen[f.Mountpoint] || f.Device == "" || f.FSType == "" || len(f.Device) > 500 || len(f.FSType) > 100 || strings.ContainsAny(f.Mountpoint+f.Device+f.FSType, "\x00\r\n\t\\") {
			return fault("CONFIG_INVALID")
		}
		seen[f.Mountpoint] = true
	}
	return nil
}
func resourceSampleDigest(at time.Time, m servicestatus.ResourceMetrics) string {
	raw, _ := json.Marshal(struct {
		ObservedAt time.Time                     `json:"observed_at"`
		Metrics    servicestatus.ResourceMetrics `json:"metrics"`
	}{at, m})
	return core.Digest(raw)
}
func resourceMounts(t Target) []string {
	out := make([]string, 0, len(t.Filesystems))
	for _, f := range t.Filesystems {
		out = append(out, f.Mountpoint)
	}
	return out
}

// These selectors are constructed from a fixed grammar and exact label values.
// There is no configuration field for arbitrary PromQL or a target HTTP URL.
func resourceCPUQueries(t Target) (string, string) {
	keys := []string{"job", "instance"}
	for k := range t.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	l := map[string]string{}
	for k, v := range t.Labels {
		l[k] = v
	}
	l["mode"] = "idle"
	t.Labels = l
	q := selector("node_cpu_seconds_total", t)
	group := strings.Join(keys, ",")
	return "100*(1-avg by(" + group + ")(rate(" + q + "[5m])))", "min by(" + group + ")(timestamp(" + q + "))"
}
func exactMetricBytes(raw string) (string, error) {
	if len(raw) > 64 {
		return "", fault("SAMPLE_VALUE_INVALID")
	}
	r, ok := new(big.Rat).SetString(raw)
	if !ok || !r.IsInt() || !r.Num().IsInt64() || r.Sign() < 0 {
		return "", fault("SAMPLE_VALUE_INVALID")
	}
	return r.Num().String(), nil
}
func fetchResourceSample(ctx context.Context, client *http.Client, base *url.URL, bearer string, t Target, now time.Time, maxAge int) (servicestatus.SignedResourceObservation, error) {
	out := servicestatus.SignedResourceObservation{ResourceMetrics: servicestatus.ResourceMetrics{Filesystems: []servicestatus.DiskSample{}}}
	now = now.UTC().Truncate(time.Millisecond)
	checkTime := func(raw string) error {
		at, e := millisecondTime(raw)
		if e != nil || at.After(now) || now.Sub(at) >= time.Duration(maxAge)*time.Second || !out.ObservedAt.IsZero() && !out.ObservedAt.Equal(at) {
			return fault("SAMPLE_TIMESTAMP_INVALID")
		}
		out.ObservedAt = at
		return nil
	}
	metric := func(name string, target Target) (string, error) {
		q := selector(name, target)
		v, e := query(ctx, client, base, bearer, q, name, target, now)
		if e != nil {
			return "", e
		}
		at, e := query(ctx, client, base, bearer, "timestamp("+q+")", "", target, now)
		if e != nil {
			return "", e
		}
		if e = checkTime(at); e != nil {
			return "", e
		}
		return v, nil
	}
	up, e := metric("up", t)
	if e != nil {
		return out, e
	}
	if up != "1" {
		return out, fault("SCRAPE_UNAVAILABLE")
	}
	q, stamp := resourceCPUQueries(t)
	cpu, e := query(ctx, client, base, bearer, q, "", t, now)
	if e != nil {
		return out, e
	}
	at, e := query(ctx, client, base, bearer, stamp, "", t, now)
	if e != nil {
		return out, e
	}
	if e = checkTime(at); e != nil {
		return out, e
	}
	n, e := strconv.ParseFloat(cpu, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100 {
		return out, fault("SAMPLE_VALUE_INVALID")
	}
	out.CPUBP = int(math.Ceil(n * 100))
	readBytes := func(name string, target Target) (string, error) {
		raw, e := metric(name, target)
		if e != nil {
			return "", e
		}
		return exactMetricBytes(raw)
	}
	if out.Memory.Total, e = readBytes("node_memory_MemTotal_bytes", t); e != nil {
		return out, e
	}
	if out.Memory.Available, e = readBytes("node_memory_MemAvailable_bytes", t); e != nil {
		return out, e
	}
	for _, f := range t.Filesystems {
		diskTarget := t
		diskTarget.Labels = map[string]string{}
		for k, v := range t.Labels {
			diskTarget.Labels[k] = v
		}
		diskTarget.Labels["mountpoint"] = f.Mountpoint
		diskTarget.Labels["device"] = f.Device
		diskTarget.Labels["fstype"] = f.FSType
		disk := servicestatus.DiskSample{Mountpoint: f.Mountpoint}
		if disk.Total, e = readBytes("node_filesystem_size_bytes", diskTarget); e != nil {
			return out, e
		}
		if disk.Available, e = readBytes("node_filesystem_avail_bytes", diskTarget); e != nil {
			return out, e
		}
		ro, e := metric("node_filesystem_readonly", diskTarget)
		if e != nil {
			return out, e
		}
		if ro != "0" && ro != "1" {
			return out, fault("SAMPLE_VALUE_INVALID")
		}
		disk.ReadOnly = ro == "1"
		out.Filesystems = append(out.Filesystems, disk)
	}
	if out.ResourceMetrics.Validate(resourceMounts(t)) != nil {
		return out, fault("SAMPLE_VALUE_INVALID")
	}
	return out, nil
}
func (b *Bridge) targetSample(ctx context.Context, j *journal, seq int64) (time.Time, string, []byte, []byte, error) {
	if j.target.Kind == "host" {
		in, e := fetchResourceSample(ctx, b.prom, b.promURL, b.bearer, j.target, time.Now(), b.config.MaxSampleAge)
		if e != nil {
			return time.Time{}, "", nil, nil, e
		}
		in.RevisionID = j.target.RevisionID
		in.Sequence = strconv.FormatInt(seq, 10)
		raw, _ := json.Marshal(in)
		return in.ObservedAt, resourceSampleDigest(in.ObservedAt, in.ResourceMetrics), raw, servicestatus.ResourceSigningMessage(j.target.SourceID, raw), nil
	}
	in, e := fetchSample(ctx, b.prom, b.promURL, b.bearer, j.target, time.Now(), b.config.MaxSampleAge)
	if e != nil {
		return time.Time{}, "", nil, nil, e
	}
	raw, _ := json.Marshal(servicestatus.SignedObservation{RevisionID: j.target.RevisionID, Sequence: strconv.FormatInt(seq, 10), ObservedAt: in.ObservedAt, Healthy: in.Healthy, LatencyMS: &in.LatencyMS})
	return in.ObservedAt, in.digest(), raw, servicestatus.SigningMessage(j.target.SourceID, raw), nil
}
