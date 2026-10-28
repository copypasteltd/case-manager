package importspec

import (
	"copypaste.hk/platform/internal/core"
	"strings"
	"testing"
	"time"
)

func TestImportProtocolBounds(t *testing.T) {
	ns, folder, owner := core.NewID(), core.NewID(), core.NewID()
	e := Entry{Ordinal: 1, SourcePath: "项目/原件.bin", SourceSHA256: strings.Repeat("a", 64), SizeBytes: "120000000000", SourceModifiedAt: time.Now().UTC().Format(time.RFC3339Nano), SourceMode: "-rw-------", SourceOwner: "reviewed source owner", MappedOwnerID: owner, FolderID: folder, FolderVersion: 1, Classification: "internal", Name: "原件.bin", MediaType: "application/octet-stream"}
	e.SourceKey = SourceKey(ns, e.SourcePath)
	if err := ValidateEntry(ns, e); err != nil {
		t.Fatal(err)
	}
	original := EntryDigest(e)
	changed := e
	changed.MappedOwnerID = core.NewID()
	if EntryDigest(changed) == original {
		t.Fatal("owner mapping not bound")
	}
	changed = e
	changed.FolderVersion++
	if EntryDigest(changed) == original {
		t.Fatal("folder version not bound")
	}
	for _, p := range []string{"../outside", "/abs", "a//b", "a/../b", "a\\b", "file:stream", "a/hidden\u202efile", "trailing ", "."} {
		t.Run(p, func(t *testing.T) {
			if RelativePath(p) {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	for _, size := range []string{"0", "01", "-1", "120000000001", "9223372036854775808"} {
		t.Run(size, func(t *testing.T) {
			bad := e
			bad.SizeBytes = size
			if ValidateEntry(ns, bad) == nil {
				t.Fatal("unsupported size accepted")
			}
		})
	}
	update := e
	id := core.NewID()
	update.FileID = &id
	if ValidateEntry(ns, update) == nil {
		t.Fatal("implicit update base allowed")
	}
	b := BatchInput{SourceNamespace: ns, InventorySHA256: original, MappingSHA256: original, PlanSHA256: Chain(ZeroChain, original), EntryCount: 1, TotalBytes: e.SizeBytes, Reason: "reviewed import"}
	if ValidateBatch(b) != nil {
		t.Fatal("bounded batch rejected")
	}
	b.TotalBytes = "120000000001"
	if ValidateBatch(b) == nil {
		t.Fatal("impossible declared total accepted")
	}
}
