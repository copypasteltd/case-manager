package files

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"copypaste.hk/platform/internal/core"
)

type authorBinding struct {
	part string
	node *authorNode
	unit OfficeUnit
}
type authorBindings struct {
	pack   *authorPackage
	units  map[string]authorBinding
	sheets []string
}

func authorRelID(n *authorNode) string {
	for _, a := range n.attrs {
		if a.Name.Local == "id" && officeNS(a.Name.Space, "relationship") {
			return a.Value
		}
	}
	return ""
}
func authorWord(n *authorNode) bool {
	return n.name.Space == wordNS || n.name.Space == "http://purl.oclc.org/ooxml/wordprocessingml/main"
}
func authorParent(n *authorNode, local string) bool {
	for p := n.parent; p != nil; p = p.parent {
		if p.name.Space == n.name.Space && p.name.Local == local {
			return true
		}
	}
	return false
}

// All units are returned or the request fails. A truncated workspace must never
// be mistaken for a complete editable document.
func OfficeDocumentWorkspace(ctx context.Context, kind string, body []byte) (OfficeWorkspace, error) {
	w, _, err := officeAuthorWorkspace(ctx, kind, body)
	return w, err
}
func officeAuthorWorkspace(ctx context.Context, kind string, body []byte) (OfficeWorkspace, *authorBindings, error) {
	w := OfficeWorkspace{Adapter: OfficeAuthorAdapter, Kind: kind, Units: []OfficeUnit{}, Warnings: []string{"按文字片段或单元格修改；不会重新排版。请在原编辑器核对分页、字体、图表及公式重算结果。"}}
	a, err := openAuthorPackage(ctx, kind, body)
	if err != nil {
		return w, nil, err
	}
	b := &authorBindings{pack: a, units: map[string]authorBinding{}}
	bytes := 0
	add := func(part, group, label, k, value string, n *authorNode, readonly bool) error {
		bytes += len(value)
		if len(w.Units) >= OfficeAuthorMaxUnits || bytes > officeAuthorContent || !authorText(value, officeAuthorValue) {
			return officeAuthorError("LIMIT", "文档超过完整编辑工作区上限，请使用原编辑器")
		}
		id := core.Digest([]byte(part + ":" + strconv.Itoa(n.start)))
		u := OfficeUnit{ID: id, Group: group, Label: label, Kind: k, Value: value, ReadOnly: readonly}
		w.Units = append(w.Units, u)
		b.units[id] = authorBinding{part, n, u}
		return nil
	}
	relations, err := a.base.relationships(a.main, true)
	if err != nil {
		return w, nil, err
	}
	parts := []struct{ part, group string }{{a.main, "正文"}}
	switch kind {
	case "docx":
		// Referenced header/footer/note/comment stories remain separate from main
		// content. Never turn deleted/tracked text or field results into plain text.
		targets := map[string]string{}
		for _, n := range a.nodes[a.main] {
			if authorWord(n) && (n.name.Local == "headerReference" || n.name.Local == "footerReference") {
				r, ok := relations[authorRelID(n)]
				if !ok || r.external {
					return w, nil, officeAuthorError("INVALID", "Word页眉页脚引用无效")
				}
				targets[r.target] = "页眉／页脚"
			}
		}
		for _, r := range relations {
			for _, t := range []string{"footnotes", "endnotes", "comments"} {
				if officeRelationshipKind(r.kind, t) && !r.external {
					targets[r.target] = "脚注／尾注／批注"
				}
			}
		}
		names := []string{}
		for part := range targets {
			names = append(names, part)
		}
		sort.Strings(names)
		for _, part := range names {
			parts = append(parts, struct{ part, group string }{part, targets[part]})
		}
		references := map[string]map[string]bool{"footnote": {}, "endnote": {}, "comment": {}}
		for _, part := range parts {
			for _, n := range a.nodes[part.part] {
				if authorWord(n) {
					for kind, ids := range references {
						if n.name.Local == kind+"Reference" || kind == "comment" && n.name.Local == "commentRangeStart" {
							for _, at := range n.attrs {
								if at.Name.Local == "id" {
									ids[at.Value] = true
								}
							}
						}
					}
				}
			}
		}
		for _, part := range parts {
			fieldDepth := 0
			index := 0
			for _, n := range a.nodes[part.part] {
				if !authorWord(n) {
					continue
				}
				if n.name.Local == "fldChar" {
					for _, at := range n.attrs {
						if at.Name.Local == "fldCharType" {
							if at.Value == "begin" {
								fieldDepth++
							}
							if at.Value == "end" && fieldDepth > 0 {
								fieldDepth--
							}
						}
					}
					continue
				}
				if n.name.Local != "t" && n.name.Local != "delText" {
					continue
				}
				orphan := false
				for parent := n.parent; parent != nil; parent = parent.parent {
					if ids, ok := references[parent.name.Local]; ok {
						key := ""
						for _, at := range parent.attrs {
							if at.Name.Local == "id" {
								key = at.Value
							}
						}
						if !ids[key] {
							orphan = true
						}
					}
				}
				if orphan {
					continue
				}
				index++
				ro := fieldDepth > 0 || authorParent(n, "fldSimple") || authorParent(n, "ins") || authorParent(n, "del") || authorParent(n, "moveFrom") || authorParent(n, "moveTo") || authorParent(n, "sdt") || n.name.Local == "delText"
				if err = add(part.part, part.group, fmt.Sprintf("文字 %d", index), "text", n.text.String(), n, ro); err != nil {
					return w, nil, err
				}
			}
		}
		w.Warnings = append(w.Warnings, "修订、字段和内容控件保持只读；文字片段继承原片段样式，跨片段格式须在原编辑器调整。")
	case "xlsx":
		var shared []string
		for _, r := range relations {
			if officeRelationshipKind(r.kind, "sharedStrings") && !r.external {
				root, e := a.base.parse(r.target)
				if e != nil {
					return w, nil, e
				}
				for _, item := range root.children {
					if item.name.Local == "si" {
						s, e := spreadsheetRich(item)
						if e != nil {
							return w, nil, e
						}
						shared = append(shared, s)
					}
				}
			}
		}
		for _, n := range a.nodes[a.main] {
			if n.name.Local == "sheet" && officeNS(n.name.Space, "sheet") {
				b.sheets = append(b.sheets, n.attr("name"))
			}
		}
		for _, n := range a.nodes[a.main] {
			if n.name.Local != "sheet" || !officeNS(n.name.Space, "sheet") {
				continue
			}
			r, ok := relations[authorRelID(n)]
			if !ok || r.external || !officeRelationshipKind(r.kind, "worksheet") {
				return w, nil, officeAuthorError("UNSUPPORTED", "仅支持普通工作表")
			}
			seen := map[string]bool{}
			for _, cell := range a.nodes[r.target] {
				if cell.name.Local != "c" || !officeNS(cell.name.Space, "sheet") {
					continue
				}
				ref := cell.attr("r")
				if _, _, e := cellCoordinate(ref); e != nil || seen[ref] {
					return w, nil, officeAuthorError("INVALID", "工作表单元格引用无效或重复")
				}
				seen[ref] = true
				k, value, ro := "text", "", false
				if f := cell.child("f"); f != nil {
					k, value = "formula", "="+f.text.String()
					ro = f.attr("t") != "" && f.attr("t") != "normal"
					if f.text.Len() > 0 && !authorFormula(value, b.sheets) {
						return w, nil, officeAuthorError("UNSUPPORTED", "工作簿含尚未支持安全改写的公式，请使用原编辑器")
					}
				} else {
					v := cell.child("v")
					if v != nil {
						value = v.text.String()
					}
					switch cell.attr("t") {
					case "s":
						i, e := strconv.Atoi(value)
						if e != nil || i < 0 || i >= len(shared) {
							return w, nil, officeAuthorError("INVALID", "共享字符串引用无效")
						}
						value = shared[i]
					case "inlineStr":
						value = ""
						var collect func(*authorNode) error
						collect = func(child *authorNode) error {
							if child.name.Local == "t" && officeNS(child.name.Space, "sheet") {
								s, e := spreadsheetString(child.text.String())
								if e != nil {
									return e
								}
								value += s
							}
							for _, next := range child.children {
								if e := collect(next); e != nil {
									return e
								}
							}
							return nil
						}
						if e := collect(cell); e != nil {
							return w, nil, e
						}
					case "b":
						k = "boolean"
						if value == "1" {
							value = "true"
						} else if value == "0" {
							value = "false"
						} else {
							return w, nil, officeAuthorError("INVALID", "布尔单元格无效")
						}
					case "n", "":
						if value != "" {
							k = "number"
						}
					case "str":
						var e error
						value, e = spreadsheetString(value)
						if e != nil {
							return w, nil, e
						}
					default:
						ro = true
					}
				}
				if err = add(r.target, n.attr("name"), ref, k, value, cell, ro); err != nil {
					return w, nil, err
				}
			}
		}
		// Non-cell expressions are retained verbatim only if they use the same safe
		// formula subset. Named ranges / dynamic or external expressions need a
		// dedicated adapter instead of being silently copied into an authored file.
		for part, nodes := range a.nodes {
			for _, n := range nodes {
				if officeNS(n.name.Space, "sheet") && (n.name.Local == "definedName" || n.name.Local == "formula" || n.name.Local == "formula1" || n.name.Local == "formula2") {
					if n.text.Len() > 0 && !authorFormula("="+n.text.String(), b.sheets) {
						return w, nil, officeAuthorError("UNSUPPORTED", "工作簿包含需要专用适配的命名或条件表达式")
					}
				}
				_ = part
			}
		}
		w.Warnings = append(w.Warnings, "修改会清除公式缓存并请求原编辑器全量重算；图表缓存可能在重新打开并计算后更新。共享／数组公式保持只读。单元格修改保留单元格样式并替换该格内部富文本。")
	case "pptx":
		parts = nil
		for _, n := range a.nodes[a.main] {
			if n.name.Local == "sldId" && officeNS(n.name.Space, "presentation") {
				r, ok := relations[authorRelID(n)]
				if !ok || r.external || !officeRelationshipKind(r.kind, "slide") {
					return w, nil, officeAuthorError("INVALID", "幻灯片引用无效")
				}
				group := fmt.Sprintf("幻灯片 %d", len(parts)+1)
				parts = append(parts, struct{ part, group string }{r.target, group})
			}
		}
		for _, part := range parts {
			index := 0
			for _, n := range a.nodes[part.part] {
				if n.name.Local == "t" && officeNS(n.name.Space, "drawing") {
					index++
					if err = add(part.part, part.group, fmt.Sprintf("文字 %d", index), "text", n.text.String(), n, authorParent(n, "fld")); err != nil {
						return w, nil, err
					}
				}
			}
		}
		w.Warnings = append(w.Warnings, "仅修改所选幻灯片文字，保留形状、版式、备注和媒体；新增文字的溢出需在原编辑器核对。")
	}
	if len(w.Units) == 0 {
		return w, nil, officeAuthorError("EMPTY", "文档没有支持编辑的文字或单元格")
	}
	sealOfficeWorkspace(&w)
	return w, b, nil
}
func authorParentOf(n, ancestor *authorNode) bool {
	for p := n.parent; p != nil; p = p.parent {
		if p == ancestor {
			return true
		}
	}
	return false
}

var authorExcelLiteral = regexp.MustCompile(`_x[0-9a-fA-F]{4}_`)

func authorSheetText(value string) string {
	return authorExcelLiteral.ReplaceAllStringFunc(value, func(s string) string { return "_x005F_" + s[1:] })
}

func EditOfficeDocument(ctx context.Context, kind string, source []byte, workspaceSHA string, changes []OfficeChange) ([]byte, OfficeWorkspace, error) {
	w, b, err := officeAuthorWorkspace(ctx, kind, source)
	if err != nil {
		return nil, w, err
	}
	if workspaceSHA != w.SHA256 {
		return nil, w, core.Fail(409, "OFFICE_WORKSPACE_STALE", "编辑依据已改变，请重新读取")
	}
	if len(changes) < 1 || len(changes) > OfficeAuthorMaxUnits {
		return nil, w, officeAuthorError("INPUT", "请选择需要修改的内容")
	}
	patches := map[string][]authorReplacement{}
	seen := map[string]bool{}
	modified := 0
	for _, change := range changes {
		bind, ok := b.units[change.ID]
		if !ok || seen[change.ID] || bind.unit.ReadOnly || !authorText(change.Value, officeAuthorValue) {
			return nil, w, officeAuthorError("INPUT", "修改包含未知、重复、只读或无效文字片段")
		}
		seen[change.ID] = true
		if change.Kind == bind.unit.Kind && change.Value == bind.unit.Value {
			continue
		}
		modified++
		n := bind.node
		var value string
		if kind == "xlsx" {
			cell := OfficeCell{Kind: change.Kind, Value: change.Value}
			if !authorCell(cell, b.sheets) {
				return nil, w, officeAuthorError("INPUT", "单元格内容或公式无效")
			}
			typ, content := authorCellBody(cell, n.name.Space)
			// Attribute values are decoded and re-escaped, preserving style and
			// metadata attributes. No client-supplied XML is accepted.
			var attrs strings.Builder
			for _, at := range n.attrs {
				if at.Name.Space != "" || at.Name.Local == "xmlns" {
					if at.Name.Space != "" {
						return nil, w, officeAuthorError("UNSUPPORTED", "单元格扩展属性需要专用适配")
					}
					continue
				}
				if at.Name.Local != "t" {
					attrs.WriteString(" " + at.Name.Local + `="` + authorEscape(at.Value) + `"`)
				}
			}
			extra := ""
			for _, child := range n.children {
				if child.name.Local != "f" && child.name.Local != "v" && child.name.Local != "is" {
					extra += string(b.pack.data[bind.part][child.start:child.end])
				}
			}
			value = `<c xmlns="` + n.name.Space + `"` + attrs.String() + ` t="` + typ + `">` + content + extra + `</c>`
		} else if change.Kind != "text" {
			return nil, w, officeAuthorError("INPUT", "文字片段不支持切换类型")
		} else if kind == "docx" {
			value = authorWordText(change.Value, n.name.Space)
		} else {
			if strings.ContainsAny(change.Value, "\r\n\t") {
				return nil, w, officeAuthorError("INPUT", "幻灯片文字片段请使用单行文字，段落调整请使用原编辑器")
			}
			value = `<a:t xmlns:a="` + n.name.Space + `" xml:space="preserve">` + authorEscape(change.Value) + `</a:t>`
		}
		patches[bind.part] = append(patches[bind.part], authorReplacement{n.start, n.end, []byte(value)})
	}
	if modified == 0 {
		return nil, w, officeAuthorError("NO_CHANGE", "内容没有变化")
	}
	removed := map[string]bool{}
	if kind == "xlsx" {
		// Every formula cache is invalidated, including formulas in unchanged cells.
		for part, nodes := range b.pack.nodes {
			for _, n := range nodes {
				if n.name.Local == "v" && officeNS(n.name.Space, "sheet") && n.parent != nil && n.parent.child("f") != nil {
					skip := false
					for _, p := range patches[part] {
						if p.start <= n.start && p.end >= n.end {
							skip = true
						}
					}
					if !skip {
						patches[part] = append(patches[part], authorReplacement{n.start, n.end, nil})
					}
				}
			}
		}
		hasCalc := false
		for _, n := range b.pack.nodes[b.pack.main] {
			if n.name.Local == "calcPr" && officeNS(n.name.Space, "sheet") {
				hasCalc = true
				patches[b.pack.main] = append(patches[b.pack.main], authorReplacement{n.start, n.end, []byte(`<calcPr xmlns="` + n.name.Space + `" calcMode="auto" fullCalcOnLoad="1" forceFullCalc="1"/>`)})
			}
		}
		if !hasCalc {
			root := b.pack.nodes[b.pack.main][0]
			patches[b.pack.main] = append(patches[b.pack.main], authorReplacement{root.closeStart, root.closeStart, []byte(`<calcPr xmlns="` + root.name.Space + `" calcMode="auto" fullCalcOnLoad="1" forceFullCalc="1"/>`)})
		}
		rels, _ := b.pack.base.relationships(b.pack.main, true)
		for _, r := range rels {
			if officeRelationshipKind(r.kind, "calcChain") {
				removed[r.target] = true
			}
		}
		for part, nodes := range b.pack.nodes {
			if strings.HasSuffix(part, ".rels") || part == "[Content_Types].xml" {
				for _, n := range nodes {
					if n.name.Local == "Relationship" && officeRelationshipKind(n.attr("Type"), "calcChain") || n.name.Local == "Override" && removed[strings.TrimPrefix(n.attr("PartName"), "/")] {
						patches[part] = append(patches[part], authorReplacement{n.start, n.end, nil})
					}
				}
			}
		}
	}
	changed := map[string][]byte{}
	for part, ps := range patches {
		changed[part], err = authorReplace(b.pack.data[part], ps)
		if err != nil {
			return nil, w, err
		}
	}
	out, err := authorRepack(ctx, b.pack.base, changed, removed)
	if err == nil {
		w, _, err = officeAuthorWorkspace(ctx, kind, out)
	}
	return out, w, err
}
