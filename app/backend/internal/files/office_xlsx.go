package files

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

var officeNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[Ee][+-]?[0-9]+)?$`)

func officeInteger(value string, maximum int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maximum || strconv.Itoa(n) != value {
		return 0, textError("DOCUMENT_INVALID")
	}
	return n, nil
}

// ST_Xstring escapes are decoded once; an escaped underscore must not cause a
// second decoding pass. XML entities were already decoded by encoding/xml.
func spreadsheetString(value string) (string, error) {
	var out strings.Builder
	code := func(s string) (uint64, bool) {
		if len(s) < 7 || s[0] != '_' || s[1] != 'x' && s[1] != 'X' || s[6] != '_' {
			return 0, false
		}
		n, err := strconv.ParseUint(s[2:6], 16, 16)
		return n, err == nil
	}
	for i := 0; i < len(value); {
		n, encoded := code(value[i:])
		if !encoded {
			out.WriteByte(value[i])
			i++
			continue
		}
		i += 7
		r := rune(n)
		if r >= 0xd800 && r <= 0xdbff {
			low, ok := code(value[i:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return "", textError("DOCUMENT_INVALID")
			}
			r = utf16.DecodeRune(r, rune(low))
			i += 7
		} else if r >= 0xdc00 && r <= 0xdfff || r == 0 {
			return "", textError("DOCUMENT_INVALID")
		}
		out.WriteRune(r)
	}
	return out.String(), nil
}

func spreadsheetRich(n *officeNode) (string, error) {
	var out strings.Builder
	var visit func(*officeNode) error
	visit = func(node *officeNode) error {
		if !officeNS(node.name.Space, "sheet") || node.name.Local == "rPh" || node.name.Local == "phoneticPr" {
			return nil
		}
		if node.name.Local == "t" {
			value, err := spreadsheetString(node.text.String())
			if err != nil {
				return err
			}
			out.WriteString(value)
			return nil
		}
		for _, child := range node.children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(n); err != nil {
		return "", err
	}
	return out.String(), nil
}

func cellCoordinate(value string) (int, int, error) {
	col, i := 0, 0
	for i < len(value) && value[i] >= 'A' && value[i] <= 'Z' {
		col = col*26 + int(value[i]-'A'+1)
		i++
		if col > 16384 {
			return 0, 0, textError("DOCUMENT_INVALID")
		}
	}
	row, err := officeInteger(value[i:], 1048576)
	if err != nil || col == 0 {
		return 0, 0, textError("DOCUMENT_INVALID")
	}
	return col, row, nil
}

func cellColumn(col int) string {
	name := ""
	for col > 0 {
		col--
		name = string(rune('A'+col%26)) + name
		col /= 26
	}
	return name
}

func extractXLSX(ctx context.Context, body []byte) (string, error) {
	p, err := openOfficePackage(ctx, body)
	if err != nil {
		return "", err
	}
	main, err := p.mainPart("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml")
	if err != nil {
		return "", err
	}
	book, err := p.parse(main)
	if err != nil {
		return "", err
	}
	if !officeNS(book.name.Space, "sheet") || book.name.Local != "workbook" {
		return "", textError("DOCUMENT_INVALID")
	}
	rels, err := p.relationships(main, false)
	if err != nil {
		return "", err
	}
	shared := []string{}
	sharedPart := ""
	for _, relation := range rels {
		if officeRelationshipKind(relation.kind, "sharedStrings") {
			if relation.external || sharedPart != "" {
				return "", textError("DOCUMENT_INVALID")
			}
			sharedPart = relation.target
		}
	}
	if sharedPart != "" {
		sst, e := p.parse(sharedPart)
		if e != nil {
			return "", e
		}
		if !officeNS(sst.name.Space, "sheet") || sst.name.Local != "sst" {
			return "", textError("DOCUMENT_INVALID")
		}
		for _, item := range sst.children {
			if item.name.Space != sst.name.Space || item.name.Local != "si" || len(shared) >= officeMaxItems {
				return "", textError("PARSE_LIMIT")
			}
			text, e := spreadsheetRich(item)
			if e != nil {
				return "", e
			}
			shared = append(shared, text)
		}
	}
	epoch := "1900"
	if props := book.child(book.name.Space, "workbookPr"); props != nil {
		value, e := officeBool(props.attr("date1904"))
		if e != nil {
			return "", e
		}
		if value {
			epoch = "1904"
		}
	}
	if err = p.write("XLSX 提取正文 / Extracted workbook text\n单元格使用文件原始值；数值未套用日期、货币或其他显示格式。日期系统：" + epoch + "。\n公式仅作文字，文件缓存可能过期；不计算公式、不读取外部链接。包含已标示的隐藏工作表、行与列。图表、图片及批注请查原件。\n"); err != nil {
		return "", err
	}
	sheets := book.child(book.name.Space, "sheets")
	if sheets == nil || len(sheets.children) == 0 || len(sheets.children) > 256 {
		return "", textError("DOCUMENT_INVALID")
	}
	seenIDs, seenNames, seenParts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for index, sheet := range sheets.children {
		if sheet.name.Space != book.name.Space || sheet.name.Local != "sheet" || sheet.attr("name") == "" || len([]rune(sheet.attr("name"))) > 128 || seenIDs[sheet.attr("sheetId")] || seenNames[strings.ToLower(sheet.attr("name"))] {
			return "", textError("DOCUMENT_INVALID")
		}
		if _, e := officeInteger(sheet.attr("sheetId"), 2147483647); e != nil {
			return "", e
		}
		seenIDs[sheet.attr("sheetId")], seenNames[strings.ToLower(sheet.attr("name"))] = true, true
		id, e := relationshipID(sheet)
		if e != nil {
			return "", e
		}
		relation, ok := rels[id]
		if !ok || relation.external || !officeRelationshipKind(relation.kind, "worksheet") {
			return "", textError("FEATURE_UNSUPPORTED")
		}
		if seenParts[relation.target] {
			return "", textError("DOCUMENT_INVALID")
		}
		seenParts[relation.target] = true
		state := sheet.attr("state")
		if state == "" {
			state = "visible"
		}
		if state != "visible" && state != "hidden" && state != "veryHidden" {
			return "", textError("DOCUMENT_INVALID")
		}
		if err = p.write(fmt.Sprintf("\n[工作表 %d：%s；状态=%s]\n", index+1, strconv.Quote(sheet.attr("name")), state)); err != nil {
			return "", err
		}
		if err = p.worksheet(relation.target, shared); err != nil {
			return "", err
		}
	}
	return strings.TrimSuffix(p.output.String(), "\n"), nil
}

func (p *officePackage) worksheet(name string, shared []string) error {
	n, err := p.parse(name)
	if err != nil {
		return err
	}
	if !officeNS(n.name.Space, "sheet") || n.name.Local != "worksheet" {
		return textError("DOCUMENT_INVALID")
	}
	hiddenCols := [16385]bool{}
	if cols := n.child(n.name.Space, "cols"); cols != nil {
		last := 0
		for _, col := range cols.children {
			first, e1 := officeInteger(col.attr("min"), 16384)
			end, e2 := officeInteger(col.attr("max"), 16384)
			hidden, e3 := officeBool(col.attr("hidden"))
			if col.name.Space != n.name.Space || col.name.Local != "col" || e1 != nil || e2 != nil || e3 != nil || first <= last || end < first {
				return textError("DOCUMENT_INVALID")
			}
			last = end
			for c := first; c <= end; c++ {
				hiddenCols[c] = hidden
			}
		}
	}
	data := n.child(n.name.Space, "sheetData")
	if data == nil {
		return textError("DOCUMENT_INVALID")
	}
	lastRow := 0
	for _, row := range data.children {
		if err = p.ctx.Err(); err != nil {
			return err
		}
		if row.name.Space != n.name.Space || row.name.Local != "row" {
			return textError("DOCUMENT_INVALID")
		}
		r := lastRow + 1
		if row.attr("r") != "" {
			r, err = officeInteger(row.attr("r"), 1048576)
		}
		hidden, hiddenErr := officeBool(row.attr("hidden"))
		if err != nil || hiddenErr != nil || r <= lastRow || r > 1048576 {
			return textError("DOCUMENT_INVALID")
		}
		lastCol := 0
		for _, cell := range row.children {
			if cell.name.Space != n.name.Space || cell.name.Local != "c" {
				return textError("DOCUMENT_INVALID")
			}
			p.items++
			if p.items > officeMaxItems {
				return textError("PARSE_LIMIT")
			}
			col, cellRow := lastCol+1, r
			if cell.attr("r") != "" {
				col, cellRow, err = cellCoordinate(cell.attr("r"))
			}
			if err != nil || cellRow != r || col <= lastCol || col > 16384 {
				return textError("DOCUMENT_INVALID")
			}
			lastCol = col
			value, e := spreadsheetCell(cell, shared)
			if e != nil {
				return e
			}
			if value == "" {
				continue
			}
			flags := ""
			if hidden {
				flags += "；隐藏行"
			}
			if hiddenCols[col] {
				flags += "；隐藏列"
			}
			if err = p.write(cellColumn(col) + strconv.Itoa(r) + flags + "：" + value + "\n"); err != nil {
				return err
			}
		}
		lastRow = r
	}
	if merges := n.child(n.name.Space, "mergeCells"); merges != nil {
		if len(merges.children) > officeMaxItems {
			return textError("PARSE_LIMIT")
		}
		seen := map[string]bool{}
		for _, merge := range merges.children {
			parts := strings.Split(merge.attr("ref"), ":")
			if merge.name.Space != n.name.Space || merge.name.Local != "mergeCell" || len(parts) != 2 || seen[merge.attr("ref")] {
				return textError("DOCUMENT_INVALID")
			}
			c1, r1, e1 := cellCoordinate(parts[0])
			c2, r2, e2 := cellCoordinate(parts[1])
			if e1 != nil || e2 != nil || c2 < c1 || r2 < r1 {
				return textError("DOCUMENT_INVALID")
			}
			seen[merge.attr("ref")] = true
			if err = p.write("合并范围 " + merge.attr("ref") + "（保留原单元格，不补写值）\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func spreadsheetCell(cell *officeNode, shared []string) (string, error) {
	switch cell.attr("t") {
	case "", "n", "s", "str", "b", "e", "d", "inlineStr":
	default:
		return "", textError("FEATURE_UNSUPPORTED")
	}
	var value, formula, inline *officeNode
	for _, child := range cell.children {
		if !officeNS(child.name.Space, "sheet") {
			continue
		}
		switch child.name.Local {
		case "v":
			if value != nil {
				return "", textError("DOCUMENT_INVALID")
			}
			value = child
		case "f":
			if formula != nil {
				return "", textError("DOCUMENT_INVALID")
			}
			formula = child
		case "is":
			if inline != nil {
				return "", textError("DOCUMENT_INVALID")
			}
			inline = child
		}
	}
	raw, result := "", ""
	if value != nil {
		if len(value.children) != 0 {
			return "", textError("DOCUMENT_INVALID")
		}
		raw = value.text.String()
	}
	if formula != nil && len(formula.children) != 0 {
		return "", textError("DOCUMENT_INVALID")
	}
	if inline != nil && (value != nil || formula != nil || cell.attr("t") != "inlineStr") {
		return "", textError("DOCUMENT_INVALID")
	}
	if inline != nil {
		text, err := spreadsheetRich(inline)
		if err != nil {
			return "", err
		}
		result = "文本=" + strconv.Quote(text)
	} else if raw != "" {
		switch cell.attr("t") {
		case "s":
			index, err := strconv.Atoi(raw)
			if err != nil || index < 0 || index >= len(shared) || strconv.Itoa(index) != raw {
				return "", textError("DOCUMENT_INVALID")
			}
			result = "文本=" + strconv.Quote(shared[index])
		case "str":
			text, err := spreadsheetString(raw)
			if err != nil {
				return "", err
			}
			result = "文本=" + strconv.Quote(text)
		case "b":
			if raw != "0" && raw != "1" {
				return "", textError("DOCUMENT_INVALID")
			}
			result = "布尔=" + raw
		case "e":
			result = "错误值=" + strconv.Quote(raw)
		case "d":
			result = "日期原值=" + strconv.Quote(raw)
		case "", "n":
			if !officeNumber.MatchString(raw) {
				return "", textError("DOCUMENT_INVALID")
			}
			result = "数值原值=" + raw
		default:
			return "", textError("FEATURE_UNSUPPORTED")
		}
	}
	if formula == nil {
		return result, nil
	}
	if formula.attr("t") != "" && formula.attr("t") != "normal" && formula.attr("t") != "shared" && formula.attr("t") != "array" && formula.attr("t") != "dataTable" {
		return "", textError("FEATURE_UNSUPPORTED")
	}
	f := formula.text.String()
	if f == "" {
		f = "[" + formula.attr("t") + " 公式引用；查阅原件]"
	}
	if result == "" {
		result = "无缓存值"
	} else {
		result = "文件缓存（可能过期）：" + result
	}
	return "公式（未计算）=" + strconv.Quote(f) + "；" + result, nil
}
