package files

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

const OfficeAuthorAdapter = "office-author-v1"
const OfficeAuthorMaxSource = 8 << 20
const OfficeAuthorMaxOutput = 12 << 20
const OfficeAuthorMaxUnits = 2000
const officeAuthorContent = 512 << 10
const officeAuthorValue = 8192

type OfficeCell struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
type OfficeSheet struct {
	Name string         `json:"name"`
	Rows [][]OfficeCell `json:"rows"`
}
type OfficeSlide struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}
type OfficeDocument struct {
	Kind       string        `json:"kind"`
	Title      string        `json:"title"`
	Paragraphs []string      `json:"paragraphs"`
	Table      [][]string    `json:"table"`
	Sheets     []OfficeSheet `json:"sheets"`
	Slides     []OfficeSlide `json:"slides"`
}
type OfficeUnit struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	ReadOnly bool   `json:"read_only"`
}
type OfficeWorkspace struct {
	Adapter  string       `json:"adapter"`
	Kind     string       `json:"kind"`
	Units    []OfficeUnit `json:"units"`
	Warnings []string     `json:"warnings"`
	SHA256   string       `json:"sha256"`
}
type OfficeChange struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func officeAuthorError(code, message string) error {
	return core.Fail(422, "OFFICE_AUTHOR_"+code, message)
}
func officeAuthorKind(media string) string {
	switch media {
	case docxMediaType:
		return "docx"
	case xlsxMediaType:
		return "xlsx"
	case pptxMediaType:
		return "pptx"
	}
	return ""
}
func officeAuthorMedia(kind string) string {
	switch kind {
	case "docx":
		return docxMediaType
	case "xlsx":
		return xlsxMediaType
	case "pptx":
		return pptxMediaType
	}
	return ""
}
func authorText(value string, limit int) bool {
	if !utf8.ValidString(value) || len(value) > limit {
		return false
	}
	for _, r := range value {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' || r == 0xfffe || r == 0xffff {
			return false
		}
	}
	return true
}

var authorNumber = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,29})(?:\.[0-9]{1,20})?(?:[eE][+-]?[0-9]{1,3})?$`)
var authorCellRef = regexp.MustCompile(`^\$?([A-Za-z]{1,3})\$?([1-9][0-9]{0,6})$`)

// Formula input is never evaluated here. Only finite arithmetic and a closed
// set of pure functions/internal references can be written to an authored file.
// External books, DDE, names, indirect addressing and network functions fail.
func authorFormulaTokens(value string, sheets []string) bool {
	if len(value) < 2 || len(value) > 2048 || value[0] != '=' || !authorText(value, 2048) {
		return false
	}
	allowed := map[string]bool{}
	for _, name := range []string{"SUM", "AVERAGE", "MIN", "MAX", "COUNT", "COUNTA", "ROUND", "ROUNDDOWN", "ROUNDUP", "IF", "IFERROR", "AND", "OR", "NOT", "ABS", "MOD", "INT", "DATE", "YEAR", "MONTH", "DAY", "LEN", "LEFT", "RIGHT", "MID", "TRIM", "CONCATENATE"} {
		allowed[name] = true
	}
	knownSheet := func(name string) bool {
		for _, item := range sheets {
			if strings.EqualFold(item, name) {
				return true
			}
		}
		return false
	}
	depth, tokens := 0, 0
	for i := 1; i < len(value); {
		tokens++
		if tokens > 512 {
			return false
		}
		c := value[i]
		if c == '"' {
			i++
			closed := false
			for i < len(value) {
				if value[i] == '"' {
					i++
					if i < len(value) && value[i] == '"' {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return false
			}
			continue
		}
		if c == '\'' {
			i++
			var name strings.Builder
			closed := false
			for i < len(value) {
				if value[i] == '\'' {
					i++
					if i < len(value) && value[i] == '\'' {
						name.WriteByte('\'')
						i++
						continue
					}
					closed = true
					break
				}
				name.WriteByte(value[i])
				i++
			}
			if !closed || i >= len(value) || value[i] != '!' || !knownSheet(name.String()) {
				return false
			}
			i++
			continue
		}
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '$' || c == '_' {
			start := i
			for i < len(value) && (value[i] >= 'A' && value[i] <= 'Z' || value[i] >= 'a' && value[i] <= 'z' || value[i] >= '0' && value[i] <= '9' || value[i] == '$' || value[i] == '_') {
				i++
			}
			word := value[start:i]
			if i < len(value) && value[i] == '!' {
				if !knownSheet(word) {
					return false
				}
				i++
				continue
			}
			if i < len(value) && value[i] == '(' {
				if !allowed[strings.ToUpper(word)] {
					return false
				}
				continue
			}
			if strings.EqualFold(word, "TRUE") || strings.EqualFold(word, "FALSE") {
				continue
			}
			ref := strings.ReplaceAll(word, "$", "")
			if !authorCellRef.MatchString(word) {
				return false
			}
			if _, _, err := cellCoordinate(strings.ToUpper(ref)); err != nil {
				return false
			}
			continue
		}
		if c == '(' {
			depth++
			if depth > 32 {
				return false
			}
		} else if c == ')' {
			depth--
			if depth < 0 {
				return false
			}
		} else if !strings.ContainsRune("0123456789.+-*/^&,:<>= %\t\r\n", rune(c)) {
			return false
		}
		i++
	}
	return depth == 0
}
func authorCell(cell OfficeCell, sheets []string) bool {
	if !authorText(cell.Value, officeAuthorValue) {
		return false
	}
	switch cell.Kind {
	case "text":
		return true
	case "number":
		if !authorNumber.MatchString(cell.Value) {
			return false
		}
		_, err := strconv.ParseFloat(cell.Value, 64)
		mantissa := strings.Split(strings.ToLower(cell.Value), "e")[0]
		digits := strings.TrimLeft(strings.ReplaceAll(strings.TrimPrefix(mantissa, "-"), ".", ""), "0")
		return err == nil && len(digits) <= 15
	case "boolean":
		return cell.Value == "true" || cell.Value == "false"
	case "formula":
		return authorFormula(cell.Value, sheets)
	}
	return false
}
func sealOfficeWorkspace(w *OfficeWorkspace) {
	w.SHA256 = ""
	body, _ := json.Marshal(w)
	w.SHA256 = core.Digest(body)
}
