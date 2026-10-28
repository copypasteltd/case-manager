package nativeruntime

import "testing"

func TestNativeMountInfoPreservesKernelReadOnlyAndTmpfsFacts(t *testing.T) {
	raw := "31 20 0:30 / /private/work rw,nosuid,nodev,noexec,relatime shared:2 - tmpfs tmpfs rw,size=65536k,inode64\n32 20 8:1 /release /opt/runtime\\040release ro,relatime - ext4 /dev/sda1 rw\n"
	mounts, e := parseMounts(raw)
	if e != nil || len(mounts) != 2 || mounts[1].Target != "/opt/runtime release" || !has(mounts[1].Options, "ro") || mountSize(mounts[0].SuperOptions) != 64<<20 {
		t.Fatal("mountinfo parser lost actual mount facts")
	}
	for _, raw := range []string{"", "malformed", "31 20 0:30 / /work rw - tmpfs"} {
		if _, e := parseMounts(raw); e == nil {
			t.Fatal("incomplete kernel record accepted")
		}
	}
	for _, option := range []string{"size=max", "size=-1", "size=0", "size=999999999999999999999", "size=1g"} {
		if mountSize([]string{option}) != -1 {
			t.Fatal("unbounded or unsupported mount size accepted")
		}
	}
}
