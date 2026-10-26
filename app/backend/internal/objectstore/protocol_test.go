package objectstore

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestMultipartReconcileUsesCloudSizesAndRejectsGaps(t *testing.T) {
	plan, err := Plan(MaxObjectBytes, 0)
	if err != nil || plan.Count != 3577 {
		t.Fatal(plan, err)
	}
	small, err := Plan(MinPartBytes+1, MinPartBytes)
	if err != nil {
		t.Fatal(err)
	}
	parts := []Part{{Number: 2, Size: 1, ETag: "second"}, {Number: 1, Size: MinPartBytes, ETag: "first"}}
	ordered, err := small.Reconcile(parts)
	if err != nil || ordered[0].Number != 1 || parts[0].Number != 2 {
		t.Fatal("reconciliation lost ordering or mutated evidence", err)
	}
	bad := [][]Part{parts[:1], {{1, MinPartBytes, "a"}, {1, 1, "b"}}, {{1, MinPartBytes - 1, "a"}, {2, 2, "b"}}, {{1, MinPartBytes, "a\r\nb"}, {2, 1, "b"}}}
	for _, value := range bad {
		if _, err = small.Reconcile(value); err == nil {
			t.Fatal("untrusted part manifest accepted", value)
		}
	}
	if _, err = Plan(MaxObjectBytes+1, 0); err == nil {
		t.Fatal("oversized object accepted")
	}
	if _, err = Plan(MaxObjectBytes, MinPartBytes); err == nil {
		t.Fatal("provider part count limit ignored")
	}
}

type boundedReader struct {
	remaining int64
	largest   int
}

func (r *boundedReader) Read(b []byte) (int, error) {
	if len(b) > r.largest {
		r.largest = len(b)
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(b)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	clear(b[:n])
	r.remaining -= int64(n)
	return n, nil
}
func TestExactHashUsesBoundedMemoryAndRefusesLengthOrCancellation(t *testing.T) {
	reader := &boundedReader{remaining: 64 << 20}
	result, err := HashExact(context.Background(), reader, reader.remaining)
	if err != nil || result.Size != 64<<20 || reader.largest > 256<<10 {
		t.Fatal(result, reader.largest, err)
	}
	small, err := HashExact(context.Background(), strings.NewReader("abc"), 3)
	if err != nil || small.SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal(small, err)
	}
	for _, size := range []int64{2, 4} {
		if _, err = HashExact(context.Background(), bytes.NewReader([]byte("abc")), size); err == nil {
			t.Fatal("length mismatch accepted", size)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = HashExact(ctx, strings.NewReader("abc"), 3); err == nil {
		t.Fatal("cancelled hash completed")
	}
}
func TestObjectKeysNeverIncludeDisplayNames(t *testing.T) {
	org := "11111111-1111-4111-8111-111111111111"
	id := "22222222-2222-4222-8222-222222222222"
	for _, makeKey := range []func() (string, error){
		func() (string, error) { return DraftKey(org, id) },
		func() (string, error) { return CandidateKey(org, id) },
		func() (string, error) { return VersionKey(org, id, id) },
		func() (string, error) { return ReplicaKey(org, id, id, id) },
	} {
		key, err := makeKey()
		if err != nil || !validKey(key) {
			t.Fatal(key, err)
		}
	}
	for _, key := range []string{"../../etc/passwd", "org/" + org + "/drafts/" + id + "/../original", "org/" + org + "/drafts/" + id + "/source?x=1"} {
		if validKey(key) {
			t.Fatal("unsafe key accepted")
		}
	}
}
