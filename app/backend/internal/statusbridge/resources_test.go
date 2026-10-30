package statusbridge

import "testing"

func TestResourceMetricPrecision(t *testing.T) {
	for _, tc := range []struct{ in, out string }{{"9007199254740993", "9007199254740993"}, {"9.007199254740993e15", "9007199254740993"}, {"0", "0"}, {"9223372036854775807", "9223372036854775807"}} {
		out, e := exactMetricBytes(tc.in)
		if e != nil || out != tc.out {
			t.Fatal("exact metric bytes", tc.in)
		}
	}
	for _, bad := range []string{"NaN", "Inf", "-1", "1.25", "9223372036854775808"} {
		if _, e := exactMetricBytes(bad); e == nil {
			t.Fatal("invalid metric accepted", bad)
		}
	}
}
func TestResourceTargetInventory(t *testing.T) {
	base := Target{Kind: "host", Job: "node", Instance: "fixture:9100", Filesystems: []FilesystemTarget{{Mountpoint: "/", Device: "/dev/root", FSType: "ext4"}}}
	if validateResourceTarget(base) != nil {
		t.Fatal("valid inventory")
	}
	for _, bad := range []Target{{Kind: "host"}, {Kind: "other"}, {Kind: "blackbox", Filesystems: base.Filesystems}, {Kind: "host", Filesystems: append(append([]FilesystemTarget{}, base.Filesystems...), base.Filesystems...)}, {Kind: "host", Labels: map[string]string{"cpu": "0"}, Filesystems: base.Filesystems}} {
		if validateResourceTarget(bad) == nil {
			t.Fatal("unbound or ambiguous inventory")
		}
	}
	q, at := resourceCPUQueries(base)
	if q != `100*(1-avg by(instance,job)(rate(node_cpu_seconds_total{instance="fixture:9100",job="node",mode="idle"}[5m])))` || at != `min by(instance,job)(timestamp(node_cpu_seconds_total{instance="fixture:9100",job="node",mode="idle"}))` {
		t.Fatal("fixed CPU query grammar changed")
	}
}
