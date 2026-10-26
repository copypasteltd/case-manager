package platform

import (
	"copypaste.hk/platform/internal/core"
	"strings"
	"testing"
	"time"
)

func TestCursorRejectsChangedAuthorityAndTampering(t *testing.T) {
	pager := Pager{Key: []byte("01234567890123456789012345678901")}
	cursor := pager.Encode(Cursor{ID: core.NewID(), UpdatedAt: time.Now().UTC(), Binding: "alice:policy-1:projects"})
	if _, err := pager.Decode(cursor, "alice:policy-1:projects"); err != nil {
		t.Fatal(err)
	}
	if _, err := pager.Decode(cursor, "bob:policy-1:projects"); err == nil {
		t.Fatal("accepted another actor's cursor")
	}
	if _, err := pager.Decode(cursor, "alice:policy-2:projects"); err == nil {
		t.Fatal("accepted stale authority cursor")
	}
	parts := strings.Split(cursor, ".")
	replacement := "A"
	if strings.HasPrefix(parts[1], replacement) {
		replacement = "B"
	}
	if _, err := pager.Decode(parts[0]+"."+replacement+parts[1][1:], "alice:policy-1:projects"); err == nil {
		t.Fatal("accepted modified signature")
	}
}
