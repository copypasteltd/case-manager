package files

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func extractPPTX(ctx context.Context, body []byte) (string, error) {
	p, err := openOfficePackage(ctx, body)
	if err != nil {
		return "", err
	}
	main, err := p.mainPart("application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml")
	if err != nil {
		return "", err
	}
	n, err := p.parse(main)
	if err != nil {
		return "", err
	}
	if !officeNS(n.name.Space, "presentation") || n.name.Local != "presentation" {
		return "", textError("DOCUMENT_INVALID")
	}
	rels, err := p.relationships(main, false)
	if err != nil {
		return "", err
	}
	list := n.child(n.name.Space, "sldIdLst")
	if list == nil || len(list.children) == 0 || len(list.children) > 256 {
		return "", textError("DOCUMENT_INVALID")
	}
	if err = p.write("PPTX 提取正文 / Extracted presentation text\n按演示文稿顺序读取幻灯片形状、表格及讲者备注中的文字；包含隐藏幻灯片与对象文字。字段是文件缓存，不刷新；不执行宏或外部链接。版式、图片、图表、SmartArt、母版及动画请查原件。\n"); err != nil {
		return "", err
	}
	seenIDs, seenParts := map[string]bool{}, map[string]bool{}
	for index, item := range list.children {
		if item.name.Space != n.name.Space || item.name.Local != "sldId" || seenIDs[item.attr("id")] {
			return "", textError("DOCUMENT_INVALID")
		}
		// PresentationML slide IDs are unsigned 32-bit values starting at 256.
		id, e := strconv.ParseUint(item.attr("id"), 10, 32)
		if e != nil || id < 256 || strconv.FormatUint(id, 10) != item.attr("id") {
			return "", textError("DOCUMENT_INVALID")
		}
		seenIDs[item.attr("id")] = true
		rid, e := relationshipID(item)
		if e != nil {
			return "", e
		}
		relation, ok := rels[rid]
		if !ok || relation.external || !officeRelationshipKind(relation.kind, "slide") || seenParts[relation.target] {
			return "", textError("DOCUMENT_INVALID")
		}
		seenParts[relation.target] = true
		if err = p.slide(relation.target, index+1); err != nil {
			return "", err
		}
	}
	return strings.TrimSuffix(p.output.String(), "\n"), nil
}

func (p *officePackage) slide(name string, index int) error {
	n, err := p.parse(name)
	if err != nil {
		return err
	}
	if !officeNS(n.name.Space, "presentation") || n.name.Local != "sld" {
		return textError("DOCUMENT_INVALID")
	}
	visible := true
	if n.attr("show") != "" {
		visible, err = officeBool(n.attr("show"))
		if err != nil {
			return err
		}
	}
	state := "visible"
	if !visible {
		state = "hidden"
	}
	if err = p.write(fmt.Sprintf("\n[幻灯片 %d；状态=%s]\n", index, state)); err != nil {
		return err
	}
	common := n.child(n.name.Space, "cSld")
	if common == nil {
		return textError("DOCUMENT_INVALID")
	}
	tree := common.child(n.name.Space, "spTree")
	if tree == nil {
		return textError("DOCUMENT_INVALID")
	}
	if err = p.drawingParagraphs(tree); err != nil {
		return err
	}
	rels, err := p.relationships(name, true)
	if err != nil {
		return err
	}
	notes := ""
	for _, relation := range rels {
		if officeRelationshipKind(relation.kind, "notesSlide") {
			if relation.external || notes != "" {
				return textError("DOCUMENT_INVALID")
			}
			notes = relation.target
		}
	}
	if notes == "" {
		return nil
	}
	n, err = p.parse(notes)
	if err != nil {
		return err
	}
	if !officeNS(n.name.Space, "presentation") || n.name.Local != "notes" {
		return textError("DOCUMENT_INVALID")
	}
	common = n.child(n.name.Space, "cSld")
	if common == nil {
		return textError("DOCUMENT_INVALID")
	}
	tree = common.child(n.name.Space, "spTree")
	if tree == nil {
		return textError("DOCUMENT_INVALID")
	}
	if err = p.write("[讲者备注 / Speaker notes]\n"); err != nil {
		return err
	}
	for _, shape := range tree.children {
		if !officeNS(shape.name.Space, "presentation") {
			continue
		}
		// Date, footer, header, slide number and image placeholders are not
		// speaker prose. Body and ordinary notes shapes are retained.
		if shape.name.Local == "sp" {
			skip := false
			if nv := shape.child(shape.name.Space, "nvSpPr"); nv != nil {
				if props := nv.child(shape.name.Space, "nvPr"); props != nil {
					if ph := props.child(shape.name.Space, "ph"); ph != nil {
						switch ph.attr("type") {
						case "dt", "ftr", "hdr", "sldNum", "sldImg":
							skip = true
						}
					}
				}
			}
			if skip {
				continue
			}
		}
		if err = p.drawingParagraphs(shape); err != nil {
			return err
		}
	}
	return nil
}

// XML shape order is preserved; it is not a geometric or visual reading order.
// A paragraph is emitted once, including run/field text and explicit breaks.
func (p *officePackage) drawingParagraphs(n *officeNode) error {
	if officeNS(n.name.Space, "drawing") && n.name.Local == "p" {
		p.items++
		if p.items > officeMaxItems {
			return textError("PARSE_LIMIT")
		}
		var out strings.Builder
		var text func(*officeNode)
		text = func(node *officeNode) {
			if !officeNS(node.name.Space, "drawing") {
				return
			}
			switch node.name.Local {
			case "t":
				out.WriteString(node.text.String())
				return
			case "br":
				out.WriteByte('\n')
				return
			case "rPr", "pPr", "endParaRPr":
				return
			}
			for _, child := range node.children {
				text(child)
			}
		}
		text(n)
		return p.write(out.String() + "\n")
	}
	for _, child := range n.children {
		if err := p.drawingParagraphs(child); err != nil {
			return err
		}
	}
	return nil
}
