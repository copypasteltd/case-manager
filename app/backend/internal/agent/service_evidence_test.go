package agent

import (
	"copypaste.hk/platform/internal/core"
	"strings"
	"testing"
)

func TestServiceEvidenceVersionsDoNotCollide(t *testing.T) {
	id, v1, v2 := core.NewID(), core.NewID(), core.NewID()
	metadata := SourceSnapshot{SourceReference: SourceReference{Type: "service", ID: id}, ResourceVersion: 1, SHA256: strings.Repeat("a", 64)}
	one := SourceSnapshot{SourceReference: SourceReference{Type: "service", ID: id, VersionID: &v1}, ResourceVersion: 1, SHA256: strings.Repeat("b", 64)}
	two := SourceSnapshot{SourceReference: SourceReference{Type: "service", ID: id, VersionID: &v2}, ResourceVersion: 1, SHA256: strings.Repeat("c", 64)}
	all, e := mergeExecutionSources([]SourceSnapshot{metadata, one}, []SourceSnapshot{two, one})
	if e != nil || len(all) != 3 {
		t.Fatal("distinct captured reports collided with metadata", e)
	}
	changed := one
	changed.SHA256 = strings.Repeat("d", 64)
	if _, e = mergeExecutionSources([]SourceSnapshot{one, changed}); e == nil {
		t.Fatal("same report identity accepted changed evidence")
	}
}
