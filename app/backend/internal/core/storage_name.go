package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// StorageNameKey preserves display names while making namespace equality
// independent of the client locale, Unicode width, composition and case.
func StorageNameKey(name string) (string, error) {
	if !utf8.ValidString(name) || len([]rune(name)) < 1 || len([]rune(name)) > 200 || strings.TrimSpace(name) != name {
		return "", Invalid("name", "名称需为1至200字符，首尾不能有空白")
	}
	key := norm.NFKC.String(cases.Fold().String(norm.NFKC.String(name)))
	for _, value := range []string{name, key} {
		for _, r := range value {
			if strings.ContainsRune("/\\<>:\"|?*", r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return "", Invalid("name", "名称不能包含路径分隔符、平台禁用字符或隐藏控制字符")
			}
		}
	}
	if key == "." || key == ".." || strings.HasSuffix(key, ".") || strings.HasSuffix(key, " ") {
		return "", Invalid("name", "名称不能是点目录或以点和空格结尾")
	}
	stem := strings.SplitN(key, ".", 2)[0]
	if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" ||
		(len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '1' && stem[3] <= '9') {
		return "", Invalid("name", "名称是平台保留名，请换一个名称")
	}
	return key, nil
}

func ClassificationRank(value string) int {
	switch value {
	case "internal":
		return 1
	case "project_restricted":
		return 2
	case "confidential":
		return 3
	default:
		return 100
	}
}
