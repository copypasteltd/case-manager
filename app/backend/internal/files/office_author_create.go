package files

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:embed office_templates/empty.docx office_templates/empty.xlsx office_templates/empty.pptx
var officeAuthorTemplates embed.FS

const authorSheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const authorPresentationNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
const authorDrawingNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
const authorRelNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const authorOfficeRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
const authorContentNS = "http://schemas.openxmlformats.org/package/2006/content-types"

func authorDocumentValid(in OfficeDocument) error {
	bytes, units := 0, 0
	text := func(s string) bool {
		bytes += len(s)
		units++
		return authorText(s, officeAuthorValue) && bytes <= officeAuthorContent && units <= OfficeAuthorMaxUnits
	}
	if !text(in.Title) || len([]rune(in.Title)) > 256 {
		return officeAuthorError("INPUT", "文档标题无效或过长")
	}
	switch in.Kind {
	case "docx":
		if len(in.Paragraphs)+len(in.Table) < 1 || len(in.Paragraphs) > 500 || len(in.Table) > 200 || len(in.Sheets)+len(in.Slides) != 0 {
			return officeAuthorError("INPUT", "Word需要正文或表格，最多500段及200行")
		}
		for _, s := range in.Paragraphs {
			if !text(s) {
				return officeAuthorError("LIMIT", "Word正文超过编辑上限")
			}
		}
		width := 0
		for _, row := range in.Table {
			if width == 0 {
				width = len(row)
			}
			if width < 1 || width > 32 || len(row) != width {
				return officeAuthorError("INPUT", "Word表格需为1至32列的完整矩形")
			}
			for _, s := range row {
				if !text(s) {
					return officeAuthorError("LIMIT", "Word表格超过编辑上限")
				}
			}
		}
	case "xlsx":
		if len(in.Sheets) < 1 || len(in.Sheets) > 16 || len(in.Paragraphs)+len(in.Table)+len(in.Slides) != 0 {
			return officeAuthorError("INPUT", "Excel需要1至16个工作表")
		}
		names, seen := []string{}, map[string]bool{}
		for _, sheet := range in.Sheets {
			name := strings.TrimSpace(sheet.Name)
			if name == "" || name != sheet.Name || utf8.RuneCountInString(name) > 31 || strings.ContainsAny(name, "[]:*?/\\\r\n\t") || strings.HasPrefix(name, "'") || strings.HasSuffix(name, "'") || seen[strings.ToLower(name)] || len(sheet.Rows) < 1 || len(sheet.Rows) > 500 {
				return officeAuthorError("INPUT", "工作表名称需唯一有效，每表最多500行")
			}
			seen[strings.ToLower(name)] = true
			names = append(names, name)
		}
		for _, sheet := range in.Sheets {
			for _, row := range sheet.Rows {
				if len(row) < 1 || len(row) > 64 {
					return officeAuthorError("INPUT", "每行需要1至64个单元格")
				}
				for _, cell := range row {
					if !text(cell.Value) || !authorCell(cell, names) {
						return officeAuthorError("INPUT", "单元格值、数值或安全公式无效；不执行公式或读取外链")
					}
				}
			}
		}
	case "pptx":
		if len(in.Slides) < 1 || len(in.Slides) > 64 || len(in.Paragraphs)+len(in.Table)+len(in.Sheets) != 0 {
			return officeAuthorError("INPUT", "PPT需要1至64张幻灯片")
		}
		for _, slide := range in.Slides {
			if !text(slide.Title) || len(slide.Lines) > 128 {
				return officeAuthorError("LIMIT", "幻灯片内容超过编辑上限")
			}
			for _, s := range slide.Lines {
				if !text(s) {
					return officeAuthorError("LIMIT", "幻灯片文字超过编辑上限")
				}
			}
		}
	default:
		return officeAuthorError("FORMAT", "请选择DOCX、XLSX或PPTX")
	}
	return nil
}

func authorWordText(value, ns string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	for i, line := range strings.Split(value, "\n") {
		if i > 0 {
			out.WriteString(`<w:br xmlns:w="` + ns + `"/>`)
		}
		for j, fragment := range strings.Split(line, "\t") {
			if j > 0 {
				out.WriteString(`<w:tab xmlns:w="` + ns + `"/>`)
			}
			out.WriteString(`<w:t xmlns:w="` + ns + `" xml:space="preserve">` + authorEscape(fragment) + `</w:t>`)
		}
	}
	return out.String()
}
func authorParagraph(text, style string) string {
	props := ""
	if style != "" {
		props = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + props + `<w:r>` + authorWordText(text, wordNS) + `</w:r></w:p>`
}
func authorCellBody(cell OfficeCell, ns string) (string, string) {
	switch cell.Kind {
	case "number":
		return "n", `<v xmlns="` + ns + `">` + authorEscape(cell.Value) + `</v>`
	case "boolean":
		v := "0"
		if cell.Value == "true" {
			v = "1"
		}
		return "b", `<v xmlns="` + ns + `">` + v + `</v>`
	case "formula":
		return "n", `<f xmlns="` + ns + `">` + authorEscape(strings.TrimPrefix(cell.Value, "=")) + `</f>`
	}
	return "inlineStr", `<is xmlns="` + ns + `"><t xml:space="preserve">` + authorEscape(authorSheetText(cell.Value)) + `</t></is>`
}
func authorAppendXML(a *authorPackage, name, value string) ([]byte, error) {
	nodes := a.nodes[name]
	if len(nodes) == 0 {
		return nil, officeAuthorError("INVALID", "Office模板缺少必要部件")
	}
	root := nodes[0]
	if root.closeStart == root.openEnd && strings.HasSuffix(string(a.data[name][root.start:root.openEnd]), "/>") {
		return nil, officeAuthorError("INVALID", "Office模板根节点不允许省略结束标记")
	}
	return authorReplace(a.data[name], []authorReplacement{{root.closeStart, root.closeStart, []byte(value)}})
}

func CreateOfficeDocument(ctx context.Context, in OfficeDocument) ([]byte, OfficeWorkspace, error) {
	var workspace OfficeWorkspace
	if err := authorDocumentValid(in); err != nil {
		return nil, workspace, err
	}
	template, err := officeAuthorTemplates.ReadFile("office_templates/empty." + in.Kind)
	if err != nil {
		return nil, workspace, err
	}
	a, err := openAuthorPackage(ctx, in.Kind, template)
	if err != nil {
		return nil, workspace, err
	}
	changed := map[string][]byte{}
	switch in.Kind {
	case "docx":
		var body *authorNode
		for _, n := range a.nodes[a.main] {
			if n.name.Local == "body" && n.name.Space == wordNS {
				body = n
				break
			}
		}
		if body == nil {
			return nil, workspace, officeAuthorError("INVALID", "Word模板缺少正文")
		}
		var content strings.Builder
		if in.Title != "" {
			content.WriteString(authorParagraph(in.Title, "Title"))
		}
		for _, text := range in.Paragraphs {
			content.WriteString(authorParagraph(text, "Normal"))
		}
		if len(in.Table) > 0 {
			content.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>`)
			for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
				content.WriteString(`<w:` + edge + ` w:val="single" w:sz="4" w:color="888888"/>`)
			}
			content.WriteString(`</w:tblBorders></w:tblPr><w:tblGrid>`)
			for range in.Table[0] {
				content.WriteString(`<w:gridCol w:w="` + strconv.Itoa(9000/len(in.Table[0])) + `"/>`)
			}
			content.WriteString(`</w:tblGrid>`)
			for _, row := range in.Table {
				content.WriteString(`<w:tr>`)
				for _, text := range row {
					content.WriteString(`<w:tc><w:tcPr><w:tcW w:w="` + strconv.Itoa(9000/len(row)) + `" w:type="dxa"/></w:tcPr>` + authorParagraph(text, "Normal") + `</w:tc>`)
				}
				content.WriteString(`</w:tr>`)
			}
			content.WriteString(`</w:tbl>`)
		}
		for _, n := range body.children {
			if n.name.Local == "sectPr" {
				content.Write(a.data[a.main][n.start:n.end])
			}
		}
		changed[a.main], err = authorReplace(a.data[a.main], []authorReplacement{{body.openEnd, body.closeStart, []byte(content.String())}})
	case "xlsx":
		var sheets, relationships, types strings.Builder
		for i, sheet := range in.Sheets {
			n := strconv.Itoa(i + 1)
			sheets.WriteString(`<sheet name="` + authorEscape(sheet.Name) + `" sheetId="` + n + `" r:id="sheet` + n + `"/>`)
			relationships.WriteString(`<Relationship Id="sheet` + n + `" Type="` + authorOfficeRel + `worksheet" Target="worksheets/sheet` + n + `.xml"/>`)
			if i > 0 {
				types.WriteString(`<Override xmlns="` + authorContentNS + `" PartName="/xl/worksheets/sheet` + n + `.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
			}
			var data strings.Builder
			data.WriteString(`<worksheet xmlns="` + authorSheetNS + `"><sheetData>`)
			for r, row := range sheet.Rows {
				rn := strconv.Itoa(r + 1)
				data.WriteString(`<row r="` + rn + `">`)
				for c, cell := range row {
					typ, value := authorCellBody(cell, authorSheetNS)
					data.WriteString(`<c r="` + cellColumn(c+1) + rn + `" t="` + typ + `">` + value + `</c>`)
				}
				data.WriteString(`</row>`)
			}
			data.WriteString(`</sheetData></worksheet>`)
			changed["xl/worksheets/sheet"+n+".xml"] = []byte(data.String())
		}
		changed[a.main] = []byte(`<workbook xmlns="` + authorSheetNS + `" xmlns:r="` + strings.TrimSuffix(authorOfficeRel, "/") + `"><bookViews><workbookView/></bookViews><sheets>` + sheets.String() + `</sheets><calcPr calcId="0" fullCalcOnLoad="1" forceFullCalc="1"/></workbook>`)
		changed["xl/_rels/workbook.xml.rels"] = []byte(`<Relationships xmlns="` + authorRelNS + `">` + relationships.String() + `<Relationship Id="styles" Type="` + authorOfficeRel + `styles" Target="styles.xml"/><Relationship Id="theme" Type="` + authorOfficeRel + `theme" Target="theme/theme1.xml"/></Relationships>`)
		changed["[Content_Types].xml"], err = authorAppendXML(a, "[Content_Types].xml", types.String())
	case "pptx":
		var slides, rels, types strings.Builder
		for i, slide := range in.Slides {
			n := strconv.Itoa(i + 1)
			slides.WriteString(`<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="authored` + n + `"/>`)
			rels.WriteString(`<Relationship xmlns="` + authorRelNS + `" Id="authored` + n + `" Type="` + authorOfficeRel + `slide" Target="slides/slide` + n + `.xml"/>`)
			types.WriteString(`<Override xmlns="` + authorContentNS + `" PartName="/ppt/slides/slide` + n + `.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`)
			changed["ppt/slides/slide"+n+".xml"] = []byte(authorSlideXML(slide))
			changed["ppt/slides/_rels/slide"+n+".xml.rels"] = []byte(`<Relationships xmlns="` + authorRelNS + `"><Relationship Id="layout" Type="` + authorOfficeRel + `slideLayout" Target="../slideLayouts/slideLayout7.xml"/></Relationships>`)
		}
		insert := -1
		for _, n := range a.nodes[a.main] {
			if n.name.Local == "sldSz" {
				insert = n.start
				break
			}
		}
		if insert < 0 {
			return nil, workspace, officeAuthorError("INVALID", "PPT模板缺少页面尺寸")
		}
		changed[a.main], err = authorReplace(a.data[a.main], []authorReplacement{{insert, insert, []byte(`<p:sldIdLst xmlns:p="` + authorPresentationNS + `" xmlns:r="` + strings.TrimSuffix(authorOfficeRel, "/") + `">` + slides.String() + `</p:sldIdLst>`)}})
		if err == nil {
			changed["ppt/_rels/presentation.xml.rels"], err = authorAppendXML(a, "ppt/_rels/presentation.xml.rels", rels.String())
		}
		if err == nil {
			changed["[Content_Types].xml"], err = authorAppendXML(a, "[Content_Types].xml", types.String())
		}
	}
	if err != nil {
		return nil, workspace, err
	}
	body, err := authorRepack(ctx, a.base, changed, nil)
	if err == nil {
		workspace, _, err = officeAuthorWorkspace(ctx, in.Kind, body)
	}
	return body, workspace, err
}

func authorSlideXML(slide OfficeSlide) string {
	shape := func(id int, name string, y, h, size int, lines []string) string {
		var text strings.Builder
		for _, line := range lines {
			for _, value := range strings.Split(strings.ReplaceAll(line, "\r\n", "\n"), "\n") {
				text.WriteString(`<a:p><a:r><a:rPr lang="zh-HK" sz="` + strconv.Itoa(size) + `"/><a:t>` + authorEscape(value) + `</a:t></a:r><a:endParaRPr lang="zh-HK"/></a:p>`)
			}
		}
		if text.Len() == 0 {
			text.WriteString(`<a:p/>`)
		}
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="457200" y="%d"/><a:ext cx="8229600" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square"/><a:lstStyle/>%s</p:txBody></p:sp>`, id, name, y, h, text.String())
	}
	return `<p:sld xmlns:p="` + authorPresentationNS + `" xmlns:a="` + authorDrawingNS + `"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` + shape(2, "Title", 274320, 914400, 3200, []string{slide.Title}) + shape(3, "Body", 1371600, 4800600, 2200, slide.Lines) + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
}
