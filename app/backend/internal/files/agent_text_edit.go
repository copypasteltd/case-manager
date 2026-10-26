package files

import (
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

// Coordinates refer to the unmodified source, in Unicode code points (including
// a UTF-8 BOM when present), exactly as returned by the bounded text reader.
type AgentTextEdit struct {
	Start int    `json:"start_char"`
	End   int    `json:"end_char"`
	Text  string `json:"text"`
}

type AgentTextDiff struct {
	Start          int    `json:"start_char"`
	RemovedChars   int    `json:"removed_chars"`
	AddedChars     int    `json:"added_chars"`
	RemovedSHA256  string `json:"removed_sha256"`
	AddedSHA256    string `json:"added_sha256"`
	Before         string `json:"before"`
	After          string `json:"after"`
	BeforeComplete bool   `json:"before_complete"`
	AfterComplete  bool   `json:"after_complete"`
	LineEnding     string `json:"line_ending"`
	UTF8BOM        bool   `json:"utf8_bom"`
}

// Text edits deliberately preserve bytes outside the selected ranges. A real
// diff is computed from the resulting text, never accepted from a model claim.
func ApplyAgentTextEdits(base string, edits []AgentTextEdit) (string, AgentTextDiff, error) {
	var diff AgentTextDiff
	invalid := func(message string) (string, AgentTextDiff, error) {
		return "", diff, core.Invalid("edits", message)
	}
	if len(base) < 1 || int64(len(base)) > AgentTextMaxBytes || !utf8.ValidString(base) || strings.IndexByte(base, 0) >= 0 {
		return invalid("基础正文不是可支持的有界UTF-8文本")
	}
	ending := agentLineEnding(base)
	if ending == "mixed" {
		return invalid("基础文本含混合或单独CR换行，请先明确格式处理方式")
	}
	if len(edits) < 1 || len(edits) > 64 {
		return invalid("每份草稿需1至64段明确修改")
	}
	runes := []rune(base)
	var out strings.Builder
	cursor, previousStart := 0, -1
	for _, edit := range edits {
		if edit.Start < cursor || edit.Start == previousStart || edit.End < edit.Start || edit.End > len(runes) || !utf8.ValidString(edit.Text) || strings.IndexByte(edit.Text, 0) >= 0 {
			return invalid("修改范围必须按原文字符位置排序、不重叠且包含有效UTF-8文本")
		}
		out.WriteString(string(runes[cursor:edit.Start]))
		out.WriteString(edit.Text)
		if int64(out.Len()) > AgentTextMaxBytes {
			return invalid("修改后的文本不得超过1MiB")
		}
		cursor, previousStart = edit.End, edit.Start
	}
	out.WriteString(string(runes[cursor:]))
	after := out.String()
	if len(after) < 1 || int64(len(after)) > AgentTextMaxBytes {
		return invalid("修改后的文本需为1字节至1MiB")
	}
	if after == base {
		return invalid("修改没有产生实际正文差异")
	}
	bom := strings.HasPrefix(base, "\ufeff")
	if strings.HasPrefix(after, "\ufeff") != bom {
		return invalid("草稿必须保留原UTF-8 BOM状态")
	}
	afterEnding := agentLineEnding(after)
	if afterEnding == "mixed" || afterEnding != "none" && afterEnding != ending && !(ending == "none" && afterEnding == "lf") {
		return invalid("草稿必须保持原换行格式；无换行原文新增行使用LF")
	}
	changed := []rune(after)
	start := 0
	for start < len(runes) && start < len(changed) && runes[start] == changed[start] {
		start++
	}
	endBefore, endAfter := len(runes), len(changed)
	for endBefore > start && endAfter > start && runes[endBefore-1] == changed[endAfter-1] {
		endBefore--
		endAfter--
	}
	removed, added := runes[start:endBefore], changed[start:endAfter]
	diff = AgentTextDiff{Start: start, RemovedChars: len(removed), AddedChars: len(added),
		RemovedSHA256: core.Digest([]byte(string(removed))), AddedSHA256: core.Digest([]byte(string(added))),
		Before: string(removed[:min(len(removed), 1000)]), After: string(added[:min(len(added), 1000)]),
		BeforeComplete: len(removed) <= 1000, AfterComplete: len(added) <= 1000, LineEnding: ending, UTF8BOM: bom}
	return after, diff, nil
}

func agentLineEnding(text string) string {
	pairs := strings.Count(text, "\r\n")
	lf, cr := strings.Count(text, "\n"), strings.Count(text, "\r")
	if cr != pairs || pairs != 0 && lf != pairs {
		return "mixed"
	}
	if pairs != 0 {
		return "crlf"
	}
	if lf != 0 {
		return "lf"
	}
	return "none"
}
