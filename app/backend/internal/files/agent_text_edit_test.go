package files

import (
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestAgentTextEditsPreserveExactBytesAndComputeDiff(t *testing.T) {
	base := "\ufeff香港🙂\r\nold\r\n结尾\r\n"
	text, diff, err := ApplyAgentTextEdits(base, []AgentTextEdit{{Start: 6, End: 9, Text: "新🙂"}})
	want := "\ufeff香港🙂\r\n新🙂\r\n结尾\r\n"
	if err != nil || text != want || diff.Start != 6 || diff.Before != "old" || diff.After != "新🙂" || diff.RemovedChars != 3 || diff.AddedChars != 2 || diff.LineEnding != "crlf" || !diff.UTF8BOM || diff.AddedSHA256 != core.Digest([]byte("新🙂")) {
		t.Fatalf("exact Unicode/CRLF edit lost evidence: %v %+v", err, diff)
	}
	large := strings.Repeat("a", int(AgentTextMaxBytes)-1) + "b"
	text, diff, err = ApplyAgentTextEdits(large, []AgentTextEdit{{Start: len(large) - 1, End: len(large), Text: "c"}})
	if err != nil || len(text) != int(AgentTextMaxBytes) || text[len(text)-1] != 'c' || diff.Before != "b" || diff.After != "c" {
		t.Fatal("bounded full-size file cannot be changed by a small exact patch")
	}
}

func TestAgentTextEditsRejectAmbiguousOrDestructiveFormats(t *testing.T) {
	cases := []struct {
		name, base string
		edits      []AgentTextEdit
	}{
		{"overlap", "abcdef", []AgentTextEdit{{1, 4, "x"}, {3, 5, "y"}}},
		{"same-position", "abc", []AgentTextEdit{{1, 1, "x"}, {1, 1, "y"}}},
		{"negative", "abc", []AgentTextEdit{{-1, 1, "x"}}},
		{"past-end", "🙂", []AgentTextEdit{{1, 2, "x"}}},
		{"no-change", "abc", []AgentTextEdit{{1, 2, "b"}}},
		{"empty-result", "abc", []AgentTextEdit{{0, 3, ""}}},
		{"nul", "abc", []AgentTextEdit{{1, 2, "\x00"}}},
		{"invalid-utf8", "abc", []AgentTextEdit{{1, 2, string([]byte{0xff})}}},
		{"drop-bom", "\ufeffabc", []AgentTextEdit{{0, 1, ""}}},
		{"add-bom", "abc", []AgentTextEdit{{0, 0, "\ufeff"}}},
		{"mixed-base", "a\r\nb\n", []AgentTextEdit{{0, 1, "c"}}},
		{"change-crlf", "a\r\nb", []AgentTextEdit{{0, 1, "c\n"}}},
		{"too-large", "a", []AgentTextEdit{{1, 1, strings.Repeat("b", int(AgentTextMaxBytes))}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ApplyAgentTextEdits(c.base, c.edits); err == nil {
				t.Fatal("accepted ambiguous text edits")
			}
		})
	}
}
