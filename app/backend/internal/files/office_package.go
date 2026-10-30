package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"
)

const OfficeTextExtractor = "office-v1"
const xlsxMediaType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
const pptxMediaType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"

const officeXMLMaxBytes = 8 << 20
const officeMaxItems = 50000

type officeNode struct {
	name     xml.Name
	attrs    map[xml.Name]string
	text     strings.Builder
	children []*officeNode
}

func (n *officeNode) attr(name string) string { return n.attrs[xml.Name{Local: name}] }
func (n *officeNode) child(space, name string) *officeNode {
	for _, child := range n.children {
		if child.name.Local == name && child.name.Space == space {
			return child
		}
	}
	return nil
}

func officeNS(space, family string) bool {
	switch family {
	case "sheet":
		return space == "http://schemas.openxmlformats.org/spreadsheetml/2006/main" || space == "http://purl.oclc.org/ooxml/spreadsheetml/main"
	case "presentation":
		return space == "http://schemas.openxmlformats.org/presentationml/2006/main" || space == "http://purl.oclc.org/ooxml/presentationml/main"
	case "drawing":
		return space == "http://schemas.openxmlformats.org/drawingml/2006/main" || space == "http://purl.oclc.org/ooxml/drawingml/main"
	case "relationship":
		return space == "http://schemas.openxmlformats.org/officeDocument/2006/relationships" || space == "http://purl.oclc.org/ooxml/officeDocument/relationships"
	}
	return false
}

func relationshipID(n *officeNode) (string, error) {
	id := ""
	for name, value := range n.attrs {
		if name.Local == "id" && officeNS(name.Space, "relationship") {
			if id != "" || value == "" {
				return "", textError("DOCUMENT_INVALID")
			}
			id = value
		}
	}
	if id == "" {
		return "", textError("DOCUMENT_INVALID")
	}
	return id, nil
}

type officePackage struct {
	ctx      context.Context
	files    map[string]*zip.File
	parts    map[string]*officeNode
	tokens   int
	xmlBytes int
	output   strings.Builder
	items    int
}

func openOfficePackage(ctx context.Context, body []byte) (*officePackage, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil || len(z.File) > 4096 {
		return nil, textError("DOCUMENT_INVALID")
	}
	p := &officePackage{ctx: ctx, files: map[string]*zip.File{}, parts: map[string]*officeNode{}}
	var total uint64
	for _, file := range z.File {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		name := file.Name
		_, exists := p.files[name]
		if exists || name == "" || strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || file.Flags&1 != 0 || file.Mode().Type() & ^fs.ModeDir != 0 {
			return nil, textError("DOCUMENT_INVALID")
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "vbaproject") || strings.Contains(lower, "/activex/") || strings.HasSuffix(lower, ".xlsm") || strings.HasSuffix(lower, ".pptm") {
			return nil, textError("MACRO_UNSUPPORTED")
		}
		if file.UncompressedSize64 > 64<<20 || total > (64<<20)-file.UncompressedSize64 {
			return nil, textError("EXPANSION_LIMIT")
		}
		total += file.UncompressedSize64
		if file.UncompressedSize64 > 1<<20 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > 100) {
			return nil, textError("EXPANSION_LIMIT")
		}
		p.files[name] = file
	}
	return p, nil
}

func (p *officePackage) parse(name string) (*officeNode, error) {
	if n := p.parts[name]; n != nil {
		return n, nil
	}
	file := p.files[name]
	if file == nil || file.FileInfo().IsDir() || file.UncompressedSize64 > officeXMLMaxBytes {
		return nil, textError("DOCUMENT_INVALID")
	}
	r, err := file.Open()
	if err != nil {
		return nil, textError("DOCUMENT_INVALID")
	}
	body, readErr := io.ReadAll(io.LimitReader(r, officeXMLMaxBytes+1))
	closeErr := r.Close()
	if readErr != nil || closeErr != nil || len(body) > officeXMLMaxBytes || uint64(len(body)) != file.UncompressedSize64 {
		return nil, textError("DOCUMENT_INVALID")
	}
	p.xmlBytes += len(body)
	if p.xmlBytes > 32<<20 {
		return nil, textError("PARSE_LIMIT")
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var root *officeNode
	stack := []*officeNode{}
	for {
		if err = p.ctx.Err(); err != nil {
			return nil, err
		}
		p.tokens++
		if p.tokens > 500000 {
			return nil, textError("PARSE_LIMIT")
		}
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, textError("DOCUMENT_INVALID")
		}
		switch value := token.(type) {
		case xml.Directive:
			return nil, textError("DOCUMENT_INVALID")
		case xml.StartElement:
			if len(stack) >= 64 || value.Name.Local == "AlternateContent" && value.Name.Space == "http://schemas.openxmlformats.org/markup-compatibility/2006" {
				return nil, textError("FEATURE_UNSUPPORTED")
			}
			n := &officeNode{name: value.Name, attrs: map[xml.Name]string{}}
			for _, attr := range value.Attr {
				if _, duplicate := n.attrs[attr.Name]; duplicate {
					return nil, textError("DOCUMENT_INVALID")
				}
				n.attrs[attr.Name] = attr.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, textError("DOCUMENT_INVALID")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, textError("DOCUMENT_INVALID")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(value)) != "" {
					return nil, textError("DOCUMENT_INVALID")
				}
			} else {
				stack[len(stack)-1].text.Write(value)
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, textError("DOCUMENT_INVALID")
	}
	p.parts[name] = root
	return root, nil
}

type officeRelationship struct {
	target, kind string
	external     bool
}

func officeRelationshipKind(value, kind string) bool {
	return value == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"+kind || value == "http://purl.oclc.org/ooxml/officeDocument/relationships/"+kind
}

func officeTarget(source, target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", textError("DOCUMENT_INVALID")
	}
	decoded := u.Path
	if decoded == "" || strings.ContainsAny(decoded, "\\\x00") {
		return "", textError("DOCUMENT_INVALID")
	}
	resolved := path.Clean(path.Join(path.Dir(source), decoded))
	if strings.HasPrefix(decoded, "/") {
		resolved = path.Clean(strings.TrimPrefix(decoded, "/"))
	}
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") || strings.HasPrefix(resolved, "/") {
		return "", textError("DOCUMENT_INVALID")
	}
	return resolved, nil
}

func (p *officePackage) relationships(source string, optional bool) (map[string]officeRelationship, error) {
	name := path.Join(path.Dir(source), "_rels", path.Base(source)+".rels")
	if source == "" {
		name = "_rels/.rels"
	}
	if p.files[name] == nil && optional {
		return map[string]officeRelationship{}, nil
	}
	n, err := p.parse(name)
	if err != nil {
		return nil, err
	}
	const ns = "http://schemas.openxmlformats.org/package/2006/relationships"
	if n.name.Space != ns || n.name.Local != "Relationships" {
		return nil, textError("DOCUMENT_INVALID")
	}
	out := map[string]officeRelationship{}
	for _, r := range n.children {
		if r.name.Space != ns || r.name.Local != "Relationship" || r.attr("Id") == "" || r.attr("Type") == "" || r.attr("Target") == "" {
			return nil, textError("DOCUMENT_INVALID")
		}
		if _, exists := out[r.attr("Id")]; exists {
			return nil, textError("DOCUMENT_INVALID")
		}
		mode := r.attr("TargetMode")
		if mode != "" && mode != "Internal" && mode != "External" {
			return nil, textError("DOCUMENT_INVALID")
		}
		v := officeRelationship{kind: r.attr("Type"), external: mode == "External"}
		if !v.external {
			v.target, err = officeTarget(source, r.attr("Target"))
			if err != nil {
				return nil, err
			}
		}
		out[r.attr("Id")] = v
	}
	return out, nil
}

func (p *officePackage) mainPart(contentType string) (string, error) {
	rels, err := p.relationships("", false)
	if err != nil {
		return "", err
	}
	main := ""
	for _, r := range rels {
		if officeRelationshipKind(r.kind, "officeDocument") {
			if r.external || main != "" {
				return "", textError("DOCUMENT_INVALID")
			}
			main = r.target
		}
	}
	if main == "" {
		return "", textError("DOCUMENT_INVALID")
	}
	n, err := p.parse("[Content_Types].xml")
	if err != nil {
		return "", err
	}
	const ns = "http://schemas.openxmlformats.org/package/2006/content-types"
	if n.name.Space != ns || n.name.Local != "Types" {
		return "", textError("DOCUMENT_INVALID")
	}
	matched, names := false, map[string]bool{}
	for _, child := range n.children {
		if child.name.Space != ns {
			return "", textError("DOCUMENT_INVALID")
		}
		if child.name.Local == "Override" {
			part, e := officeTarget("", child.attr("PartName"))
			if e != nil || names[part] {
				return "", textError("DOCUMENT_INVALID")
			}
			names[part] = true
			if part == main {
				matched = child.attr("ContentType") == contentType
			}
		}
		if strings.Contains(strings.ToLower(child.attr("ContentType")), "macroenabled") || strings.Contains(strings.ToLower(child.attr("ContentType")), "vbaproject") {
			return "", textError("MACRO_UNSUPPORTED")
		}
	}
	if !matched {
		return "", textError("DOCUMENT_INVALID")
	}
	return main, nil
}

func (p *officePackage) write(value string) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if p.output.Len()+len(value) > TextOutputMaxBytes || strings.IndexByte(value, 0) >= 0 {
		return textError("OUTPUT_LIMIT")
	}
	p.output.WriteString(value)
	return nil
}

func officeBool(value string) (bool, error) {
	if value == "" || value == "0" || value == "false" {
		return false, nil
	}
	if value == "1" || value == "true" {
		return true, nil
	}
	return false, textError("DOCUMENT_INVALID")
}
