package files

import (
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestAgentCreateText(t *testing.T) {
	text := "\ufeff# 香港🙂\r\n明确的新资料\r\n"
	diff, err := AgentCreateText(text)
	if err != nil || diff.RemovedChars != 0 || diff.Before != "" || !diff.BeforeComplete || diff.After != text || !diff.AfterComplete || diff.AddedChars != len([]rune(text)) || diff.AddedSHA256 != core.Digest([]byte(text)) || diff.RemovedSHA256 != core.Digest(nil) || diff.LineEnding != "crlf" || !diff.UTF8BOM {
		t.Fatal("exact new UTF-8 text evidence lost", err)
	}
	for _, text := range []string{"", "abc\x00def", "x\ry", "x\r\ny\nz", string([]byte{0xff}), strings.Repeat("a", AgentCreateMaxBytes+1)} {
		if _, err := AgentCreateText(text); err == nil {
			t.Fatal("invalid new content accepted")
		}
	}
	if d, err := AgentCreateText(strings.Repeat("文", AgentCreateMaxBytes/3)); err != nil || d.AfterComplete || len([]rune(d.After)) != 1000 || d.AddedChars != AgentCreateMaxBytes/3 {
		t.Fatal("byte boundary or truncated preview misrepresented", err)
	}
}
