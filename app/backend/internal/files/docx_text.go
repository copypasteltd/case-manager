package files

import (
	"context"
	"encoding/xml"
	"strconv"
	"strings"
)

// A separate identity preserves both ready and failed text-v1 derivations.
const DOCXTextExtractor = "docx-v2"
const docxMediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

func wordNode(n *officeNode, name string) bool {
	return n.name.Local == name && (n.name.Space == wordNS || n.name.Space == "http://purl.oclc.org/ooxml/wordprocessingml/main")
}

func wordAttr(n *officeNode, name string) string {
	return n.attrs[xml.Name{Space: n.name.Space, Local: name}]
}

type docxReference struct{ kind, id string }
type docxHeader struct {
	kind, variant, target string
	section               int
}
type docxText struct {
	packageFile *officePackage
	relations   map[string]officeRelationship
	references  []docxReference
	seen        map[docxReference]bool
	headers     []docxHeader
	section     int
	fields      []bool
	notes       map[string]map[string]*officeNode
	mainStory   bool
	deleting    int
}

func extractComplexDOCX(ctx context.Context, body []byte) (string, error) {
	p, err := openOfficePackage(ctx, body)
	if err != nil {
		return "", err
	}
	main, err := p.mainPart("application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml")
	if err != nil {
		return "", err
	}
	n, err := p.parse(main)
	if err != nil {
		return "", err
	}
	if !wordNode(n, "document") {
		return "", textError("DOCUMENT_INVALID")
	}
	bodyNode := n.child(n.name.Space, "body")
	if bodyNode == nil {
		return "", textError("DOCUMENT_INVALID")
	}
	count := 0
	for _, child := range n.children {
		if wordNode(child, "body") {
			count++
		}
	}
	if count != 1 {
		return "", textError("DOCUMENT_INVALID")
	}
	rels, err := p.relationships(main, true)
	if err != nil {
		return "", err
	}
	d := &docxText{packageFile: p, relations: rels, seen: map[docxReference]bool{}, notes: map[string]map[string]*officeNode{}, mainStory: true}
	err = d.story(bodyNode)
	d.mainStory = false
	// Only section-referenced headers/footers are included, never ZIP filename guesses.
	seen := map[string]bool{}
	for _, header := range d.headers {
		if err != nil {
			break
		}
		key := header.kind + "/" + header.target
		label, root := "页眉", "hdr"
		if header.kind == "footer" {
			label, root = "页脚", "ftr"
		}
		variant := map[string]string{"default": "默认", "first": "首页", "even": "偶数页"}[header.variant]
		if err = p.write("\n[第 " + strconv.Itoa(header.section) + " 节" + label + " " + variant + "]\n"); err != nil {
			break
		}
		if seen[key] {
			err = p.write("[同一部件已列出；继承与分页请查看原件]\n")
			continue
		}
		seen[key] = true
		var part *officeNode
		part, err = p.parse(header.target)
		if err == nil && !wordNode(part, root) {
			err = textError("DOCUMENT_INVALID")
		}
		if err == nil {
			err = d.story(part)
		}
	}
	// References can occur in main text, headers and notes. Bound this growing
	// queue and render each exact referenced ID once, in first-reference order.
	for i := 0; err == nil && i < len(d.references); i++ {
		err = d.note(d.references[i])
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(p.output.String(), "\n"), nil
}

func (d *docxText) story(n *officeNode) error {
	if len(d.fields) != 0 {
		return textError("DOCUMENT_INVALID")
	}
	if err := d.walk(n); err != nil {
		return err
	}
	if len(d.fields) != 0 {
		return textError("DOCUMENT_INVALID")
	}
	return nil
}

func (d *docxText) reference(kind string, n *officeNode, label string) error {
	id := wordAttr(n, "id")
	number, err := strconv.ParseInt(id, 10, 32)
	if err != nil || number < 0 || strconv.FormatInt(number, 10) != id {
		return textError("DOCUMENT_INVALID")
	}
	ref := docxReference{kind, id}
	if !d.seen[ref] {
		if len(d.references) >= 4096 {
			return textError("PARSE_LIMIT")
		}
		d.seen[ref] = true
		d.references = append(d.references, ref)
	}
	if label == "" {
		label = docxReferenceLabel(kind) + "引用"
	}
	return d.packageFile.write("[" + label + " " + id + "]")
}

func docxReferenceLabel(kind string) string {
	switch kind {
	case "footnotes":
		return "脚注"
	case "endnotes":
		return "尾注"
	default:
		return "批注"
	}
}

func (d *docxText) note(ref docxReference) error {
	if notes, ok := d.notes[ref.kind]; ok {
		return d.renderNote(ref, notes[ref.id])
	}
	target := ""
	for _, r := range d.relations {
		if officeRelationshipKind(r.kind, ref.kind) {
			if r.external || target != "" {
				return textError("DOCUMENT_INVALID")
			}
			target = r.target
		}
	}
	if target == "" {
		return textError("DOCUMENT_INVALID")
	}
	part, err := d.packageFile.parse(target)
	if err != nil {
		return err
	}
	if !wordNode(part, ref.kind) {
		return textError("DOCUMENT_INVALID")
	}
	childName := strings.TrimSuffix(ref.kind, "s")
	ids := map[string]*officeNode{}
	for _, n := range part.children {
		if !wordNode(n, childName) {
			return textError("DOCUMENT_INVALID")
		}
		id := wordAttr(n, "id")
		if id == "" || ids[id] != nil {
			return textError("DOCUMENT_INVALID")
		}
		ids[id] = n
	}
	d.notes[ref.kind] = ids
	return d.renderNote(ref, ids[ref.id])
}

func (d *docxText) renderNote(ref docxReference, match *officeNode) error {
	if match == nil || wordAttr(match, "type") != "" && wordAttr(match, "type") != "normal" {
		return textError("DOCUMENT_INVALID")
	}
	if err := d.packageFile.write("\n[" + docxReferenceLabel(ref.kind) + " " + ref.id + "]\n"); err != nil {
		return err
	}
	return d.story(match)
}

func (d *docxText) sectionReferences(n *officeNode) error {
	if !d.mainStory {
		return textError("DOCUMENT_INVALID")
	}
	d.section++
	if d.section > 256 {
		return textError("PARSE_LIMIT")
	}
	seen := map[string]bool{}
	for _, c := range n.children {
		kind := ""
		if wordNode(c, "headerReference") {
			kind = "header"
		}
		if wordNode(c, "footerReference") {
			kind = "footer"
		}
		if kind == "" {
			continue
		}
		variant := wordAttr(c, "type")
		if variant != "default" && variant != "first" && variant != "even" || seen[kind+variant] {
			return textError("DOCUMENT_INVALID")
		}
		seen[kind+variant] = true
		id, err := relationshipID(c)
		if err != nil {
			return err
		}
		r, ok := d.relations[id]
		if !ok || r.external || !officeRelationshipKind(r.kind, kind) {
			return textError("DOCUMENT_INVALID")
		}
		d.headers = append(d.headers, docxHeader{kind, variant, r.target, d.section})
	}
	return nil
}

func (d *docxText) children(n *officeNode) error {
	for _, child := range n.children {
		if err := d.walk(child); err != nil {
			return err
		}
	}
	return nil
}

func (d *docxText) wrap(n *officeNode, label string) error {
	if err := d.packageFile.write("[" + label + "]"); err != nil {
		return err
	}
	if err := d.children(n); err != nil {
		return err
	}
	return d.packageFile.write("[/" + label + "]")
}

func (d *docxText) walk(n *officeNode) error {
	p := d.packageFile
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.items++
	if p.items > officeMaxItems {
		return textError("PARSE_LIMIT")
	}
	if n.name.Space != wordNS && n.name.Space != "http://purl.oclc.org/ooxml/wordprocessingml/main" {
		return d.children(n)
	}
	switch n.name.Local {
	case "t", "delText":
		if n.name.Local == "delText" && d.deleting == 0 {
			return textError("DOCUMENT_INVALID")
		}
		if len(n.children) != 0 {
			return textError("DOCUMENT_INVALID")
		}
		return p.write(n.text.String())
	case "instrText", "delInstrText":
		if len(n.children) != 0 {
			return textError("DOCUMENT_INVALID")
		}
		return p.write("[字段指令，不执行：" + n.text.String() + "]")
	case "fldSimple":
		if err := p.write("[字段指令，不执行：" + wordAttr(n, "instr") + "][缓存结果，可能过期："); err != nil {
			return err
		}
		if err := d.children(n); err != nil {
			return err
		}
		return p.write("]")
	case "fldChar":
		switch wordAttr(n, "fldCharType") {
		case "begin":
			if len(d.fields) >= 32 {
				return textError("PARSE_LIMIT")
			}
			d.fields = append(d.fields, false)
			return p.write("[字段开始；不计算]")
		case "separate":
			if len(d.fields) == 0 || d.fields[len(d.fields)-1] {
				return textError("DOCUMENT_INVALID")
			}
			d.fields[len(d.fields)-1] = true
			return p.write("[缓存结果，可能过期：")
		case "end":
			if len(d.fields) == 0 {
				return textError("DOCUMENT_INVALID")
			}
			hasCache := d.fields[len(d.fields)-1]
			d.fields = d.fields[:len(d.fields)-1]
			if hasCache {
				return p.write("][字段结束]")
			}
			return p.write("[字段结束；无缓存结果]")
		default:
			return textError("DOCUMENT_INVALID")
		}
	case "tab":
		return p.write("\t")
	case "br", "cr":
		return p.write("\n")
	case "noBreakHyphen":
		return p.write("‑")
	case "softHyphen":
		return p.write("\u00ad")
	case "p":
		if err := d.children(n); err != nil {
			return err
		}
		return p.write("\n")
	case "tbl":
		if err := p.write("\n[表格]\n"); err != nil {
			return err
		}
		if err := d.children(n); err != nil {
			return err
		}
		return p.write("[/表格]\n")
	case "tr":
		if err := d.children(n); err != nil {
			return err
		}
		return p.write("\n")
	case "tc":
		if err := p.write("[单元格]"); err != nil {
			return err
		}
		if err := d.children(n); err != nil {
			return err
		}
		return p.write("[/单元格]\t")
	case "ins":
		return d.wrap(n, "插入")
	case "del":
		d.deleting++
		err := d.wrap(n, "删除")
		d.deleting--
		return err
	case "moveFrom":
		d.deleting++
		err := d.wrap(n, "移出")
		d.deleting--
		return err
	case "moveTo":
		return d.wrap(n, "移入")
	case "txbxContent":
		return d.wrap(n, "文本框")
	case "sdt":
		content := n.child(n.name.Space, "sdtContent")
		if content == nil {
			return textError("DOCUMENT_INVALID")
		}
		return d.children(content)
	case "r":
		if props := n.child(n.name.Space, "rPr"); props != nil {
			for _, property := range props.children {
				if wordNode(property, "vanish") || wordNode(property, "webHidden") {
					value := wordAttr(property, "val")
					if value == "" || value == "on" {
						return d.wrap(n, "直接隐藏文字")
					}
					if value == "off" {
						continue
					}
					on, err := officeBool(value)
					if err != nil {
						return err
					}
					if on {
						return d.wrap(n, "直接隐藏文字")
					}
				}
			}
		}
	case "footnoteReference":
		return d.reference("footnotes", n, "")
	case "endnoteReference":
		return d.reference("endnotes", n, "")
	case "commentReference":
		return d.reference("comments", n, "")
	case "commentRangeStart", "commentRangeEnd":
		label := "批注范围开始"
		if n.name.Local == "commentRangeEnd" {
			label = "批注范围结束"
		}
		return d.reference("comments", n, label)
	case "sectPr":
		return d.sectionReferences(n)
	case "pPr":
		// Ignore old formatting snapshots but retain this paragraph's current section.
		if section := n.child(n.name.Space, "sectPr"); section != nil {
			return d.sectionReferences(section)
		}
		return nil
	case "rPr", "tblPr", "tblGrid", "tcPr", "trPr", "sdtPr", "sdtEndPr", "pPrChange", "rPrChange", "sectPrChange":
		return nil
	case "altChunk", "subDoc":
		return textError("FEATURE_UNSUPPORTED")
	case "object", "pict", "drawing":
		if err := p.write("[图像／绘图／嵌入对象；原件核对]"); err != nil {
			return err
		}
		return d.children(n)
	}
	return d.children(n)
}
