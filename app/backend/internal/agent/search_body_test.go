package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSearchBodyUnicode(t *testing.T) {
	for _, prefix := range []string{"香港🙂", "K", "Ⱥ"} {
		t.Run(prefix, func(t *testing.T) {
			body := strings.Repeat(prefix, 200) + "预算核对🙂"
			got := searchSnippet(body, "预算核对")
			if !utf8.ValidString(got) || len([]rune(got)) > 240 || !strings.Contains(got, "预算核对🙂") {
				t.Fatal("Unicode snippet lost the actual match")
			}
		})
	}
	if searchSnippet("未匹配正文", "不存在") != "" {
		t.Fatal("invented snippet")
	}
}
