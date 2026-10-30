package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type authorNode struct {
	name                            xml.Name
	attrs                           []xml.Attr
	start, openEnd, closeStart, end int
	parent                          *authorNode
	children                        []*authorNode
	text                            strings.Builder
}

func (n *authorNode) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (n *authorNode) child(local string) *authorNode {
	for _, c := range n.children {
		if c.name.Space == n.name.Space && c.name.Local == local {
			return c
		}
	}
	return nil
}
func authorNodes(ctx context.Context, body []byte) ([]*authorNode, error) {
	if len(body) > officeXMLMaxBytes {
		return nil, officeAuthorError("LIMIT", "Office部件超过编辑大小限制")
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	nodes, stack := []*authorNode{}, []*authorNode{}
	roots, tokens := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start := int(d.InputOffset())
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, officeAuthorError("INVALID", "Office文档部件无效")
		}
		tokens++
		if tokens > 500000 {
			return nil, officeAuthorError("LIMIT", "Office文档结构过于复杂")
		}
		switch v := token.(type) {
		case xml.Directive:
			return nil, officeAuthorError("UNSUPPORTED", "不支持含DTD或自定义实体的Office文档")
		case xml.ProcInst:
			if v.Target != "xml" || roots != 0 {
				return nil, officeAuthorError("UNSUPPORTED", "不支持此Office处理指令")
			}
		case xml.StartElement:
			if len(stack) >= 64 {
				return nil, officeAuthorError("LIMIT", "Office文档嵌套过深")
			}
			n := &authorNode{name: v.Name, attrs: v.Attr, start: start, openEnd: int(d.InputOffset())}
			seen := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if seen[a.Name] {
					return nil, officeAuthorError("INVALID", "Office文档包含重复属性")
				}
				seen[a.Name] = true
			}
			if len(stack) == 0 {
				roots++
			} else {
				n.parent = stack[len(stack)-1]
				n.parent.children = append(n.parent.children, n)
			}
			nodes, stack = append(nodes, n), append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, officeAuthorError("INVALID", "Office文档结束标记无效")
			}
			n := stack[len(stack)-1]
			n.closeStart, n.end = start, int(d.InputOffset())
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(v)
			} else if strings.TrimSpace(string(v)) != "" {
				return nil, officeAuthorError("INVALID", "Office文档根之外包含正文")
			}
		}
	}
	if roots != 1 || len(stack) != 0 {
		return nil, officeAuthorError("INVALID", "Office文档缺少唯一根部件")
	}
	return nodes, nil
}

type authorPackage struct {
	base  *officePackage
	data  map[string][]byte
	nodes map[string][]*authorNode
	main  string
	kind  string
}

func openAuthorPackage(ctx context.Context, kind string, body []byte) (*authorPackage, error) {
	if officeAuthorMedia(kind) == "" || len(body) < 1 || len(body) > OfficeAuthorMaxSource {
		return nil, officeAuthorError("LIMIT", "编辑支持8MiB以内的DOCX、XLSX及PPTX")
	}
	p, err := openOfficePackage(ctx, body)
	if err != nil {
		return nil, err
	}
	content := map[string]string{"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"}
	main, err := p.mainPart(content[kind])
	if err != nil {
		return nil, err
	}
	a := &authorPackage{base: p, data: map[string][]byte{}, nodes: map[string][]*authorNode{}, main: main, kind: kind}
	totalXML := 0
	for name, file := range p.files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if file.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "_xmlsignatures/") || strings.Contains(lower, "/embeddings/") || strings.Contains(lower, "/externallinks/") {
			return nil, officeAuthorError("UNSUPPORTED", "签名、嵌入对象及外部工作簿需要专用编辑适配，请保留原件")
		}
		r, e := file.Open()
		if e != nil {
			return nil, officeAuthorError("INVALID", "Office文档压缩部件无效")
		}
		var data []byte
		var count int64
		if strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".rels") {
			data, e = io.ReadAll(io.LimitReader(r, officeXMLMaxBytes+1))
			count = int64(len(data))
		} else {
			count, e = io.Copy(io.Discard, io.LimitReader(r, (64<<20)+1))
		}
		closeErr := r.Close()
		if e != nil || closeErr != nil || uint64(count) != file.UncompressedSize64 {
			return nil, officeAuthorError("INVALID", "Office部件长度或CRC核验失败")
		}
		if data == nil {
			continue
		}
		totalXML += len(data)
		if totalXML > 32<<20 {
			return nil, officeAuthorError("LIMIT", "Office文档解压正文超过编辑上限")
		}
		nodes, e := authorNodes(ctx, data)
		if e != nil {
			return nil, e
		}
		a.data[name], a.nodes[name] = data, nodes
		var instructions strings.Builder
		for _, n := range nodes {
			if authorWord(n) {
				if n.name.Local == "instrText" {
					instructions.WriteString(n.text.String())
				}
				if n.name.Local == "fldSimple" {
					for _, at := range n.attrs {
						if at.Name.Local == "instr" {
							instructions.WriteString(at.Value)
						}
					}
				}
			}
		}
		for _, marker := range []string{"DDE", "INCLUDETEXT", "INCLUDEPICTURE", "LINK", "DATABASE", "IMPORT"} {
			if strings.Contains(strings.ToUpper(instructions.String()), marker) {
				return nil, officeAuthorError("UNSUPPORTED", "外部读取字段不能通过此编辑器改写")
			}
		}
		for _, n := range nodes {
			local := strings.ToLower(n.name.Local)
			if local=="workbookprotection"&&len(n.attrs)==0 { continue }
			if local == "documentprotection" || local == "sheetprotection" || local == "workbookprotection" || local == "encryptedkey" {
				return nil, officeAuthorError("PROTECTED", "受保护文档需要先由授权人员在原编辑器处理保护")
			}
			if local == "altchunk" || local == "oleobject" || local == "control" || local == "alternatecontent" {
				return nil, officeAuthorError("UNSUPPORTED", "此文档包含需要专用适配的嵌入内容")
			}
			if n.name.Local == "instrText" && wordNode(&officeNode{name: n.name}, "instrText") {
				upper := strings.ToUpper(n.text.String())
				for _, marker := range []string{"DDE", "INCLUDETEXT", "INCLUDEPICTURE", "LINK", "DATABASE", "IMPORT"} {
					if strings.Contains(upper, marker) {
						return nil, officeAuthorError("UNSUPPORTED", "含外部读取字段的文档不能通过此编辑器生成新版")
					}
				}
			}
			if strings.HasSuffix(lower, ".rels") && n.name.Local == "Relationship" {
				typ := n.attr("Type")
				if officeRelationshipKind(typ, "package") {
					return nil, officeAuthorError("UNSUPPORTED", "嵌入包需要专用适配")
				}
				for _, marker := range []string{"attachedTemplate", "oleObject", "vbaProject", "activeX", "externalLink"} {
					if strings.Contains(typ, marker) {
						return nil, officeAuthorError("UNSUPPORTED", "文档包含需要专用适配的外部或嵌入关系")
					}
				}
				if n.attr("TargetMode") == "External" {
					u, e := url.Parse(n.attr("Target"))
					if e != nil || !officeRelationshipKind(typ, "hyperlink") || u.User != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto") {
						return nil, officeAuthorError("UNSUPPORTED", "外部内容关系不能自动读取或更新")
					}
				} else {
					source := ""
					if name != "_rels/.rels" {
						source = path.Join(path.Dir(path.Dir(name)), strings.TrimSuffix(path.Base(name), ".rels"))
					}
					target, e := officeTarget(source, n.attr("Target"))
					if e != nil || p.files[target] == nil || p.files[target].FileInfo().IsDir() {
						return nil, officeAuthorError("INVALID", "Office文档内部引用不完整")
					}
				}
			}
		}
	}
	return a, nil
}

type authorReplacement struct {
	start, end int
	value      []byte
}

func authorReplace(body []byte, changes []authorReplacement) ([]byte, error) {
	sort.Slice(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
	var out bytes.Buffer
	previous := 0
	for _, c := range changes {
		if c.start < previous || c.start < 0 || c.end < c.start || c.end > len(body) {
			return nil, officeAuthorError("INVALID", "Office修改片段重叠或超出原件")
		}
		out.Write(body[previous:c.start])
		out.Write(c.value)
		previous = c.end
	}
	out.Write(body[previous:])
	if out.Len() > officeXMLMaxBytes {
		return nil, officeAuthorError("LIMIT", "Office修改结果超过部件大小限制")
	}
	return out.Bytes(), nil
}
func authorEscape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}
func authorRepack(ctx context.Context, base *officePackage, changed map[string][]byte, removed map[string]bool) ([]byte, error) {
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	names := []string{}
	for name := range base.files {
		if !removed[name] {
			names = append(names, name)
		}
	}
	for name := range changed {
		if base.files[name] == nil && !removed[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if data, ok := changed[name]; ok {
			header := zip.FileHeader{Name: name, Method: zip.Deflate}
			header.SetModTime(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
			header.SetMode(0600)
			dest, err := w.CreateHeader(&header)
			if err != nil {
				return nil, err
			}
			if _, err = dest.Write(data); err != nil {
				return nil, err
			}
		} else if err := w.Copy(base.files[name]); err != nil {
			return nil, err
		}
		if out.Len() > OfficeAuthorMaxOutput {
			return nil, officeAuthorError("LIMIT", "Office结果超过12MiB上限")
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if out.Len() > OfficeAuthorMaxOutput {
		return nil, officeAuthorError("LIMIT", "Office结果超过12MiB上限")
	}
	return out.Bytes(), nil
}
