package files

import (
	"archive/zip"
	"bytes"
	"context"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

const sheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const presNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
const drawNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
const relNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

func officeZip(t *testing.T, parts map[string]string, mode fs.FileMode) []byte {
	t.Helper()
	var body bytes.Buffer
	z := zip.NewWriter(&body)
	keys := []string{}
	for name := range parts {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		if mode != 0 {
			h.SetMode(mode)
		}
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(parts[name])); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return body.Bytes()
}

func packageParts(kind string) map[string]string {
	part, content := "xl/workbook.xml", "spreadsheetml.sheet"
	if kind == "pptx" {
		part, content = "ppt/presentation.xml", "presentationml.presentation"
	}
	return map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/` + part + `" ContentType="application/vnd.openxmlformats-officedocument.` + content + `.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="` + relNS + `/officeDocument" Target="` + part + `"/></Relationships>`,
	}
}

func workbookParts() map[string]string {
	p := packageParts("xlsx")
	p["xl/workbook.xml"] = `<workbook xmlns="` + sheetNS + `" xmlns:r="` + relNS + `"><workbookPr date1904="true"/><sheets><sheet name="预算🙂" sheetId="7" r:id="second"/><sheet name="内部" sheetId="2" state="veryHidden" r:id="first"/></sheets></workbook>`
	p["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="first" Type="` + relNS + `/worksheet" Target="worksheets/a.xml"/><Relationship Id="second" Type="` + relNS + `/worksheet" Target="worksheets/z.xml"/><Relationship Id="strings" Type="` + relNS + `/sharedStrings" Target="sharedStrings.xml"/><Relationship Id="link" Type="` + relNS + `/externalLink" Target="https://example.invalid/private" TargetMode="External"/></Relationships>`
	p["xl/sharedStrings.xml"] = `<sst xmlns="` + sheetNS + `"><si><r><t>香港</t></r><r><t>预算🙂</t></r><rPh><t>DO_NOT_INDEX_PHONETIC</t></rPh></si></sst>`
	p["xl/worksheets/z.xml"] = `<worksheet xmlns="` + sheetNS + `"><cols><col min="2" max="3" hidden="1"/></cols><sheetData><row r="3" hidden="1"><c r="A3" t="s"><v>0</v></c><c r="C3"><f>SUM(A1:A2)</f><v>12.50</v></c><c r="D3"><f t="shared" si="0"/></c><c r="E3" t="inlineStr"><is><t>_x005F_x0041_ _xD83D__xDE42_</t></is></c></row><row><c t="b"><v>1</v></c><c r="D4" t="e"><v>#DIV/0!</v></c><c r="E4" t="d"><v>2026-10-30T00:00:00Z</v></c></row></sheetData><mergeCells><mergeCell ref="A3:B3"/></mergeCells></worksheet>`
	p["xl/worksheets/a.xml"] = `<worksheet xmlns="` + sheetNS + `"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>隐藏说明</t></is></c></row></sheetData></worksheet>`
	p["xl/worksheets/orphan.xml"] = `<worksheet xmlns="` + sheetNS + `"><sheetData><row><c t="inlineStr"><is><t>ORPHAN_MUST_NOT_APPEAR</t></is></c></row></sheetData></worksheet>`
	return p
}

func presentationParts() map[string]string {
	p := packageParts("pptx")
	p["ppt/presentation.xml"] = `<p:presentation xmlns:p="` + presNS + `" xmlns:r="` + relNS + `"><p:sldIdLst><p:sldId id="258" r:id="second"/><p:sldId id="256" r:id="first"/></p:sldIdLst></p:presentation>`
	p["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="first" Type="` + relNS + `/slide" Target="slides/a.xml"/><Relationship Id="second" Type="` + relNS + `/slide" Target="slides/z.xml"/></Relationships>`
	p["ppt/slides/z.xml"] = `<p:sld xmlns:p="` + presNS + `" xmlns:a="` + drawNS + `" show="0"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>季度</a:t></a:r><a:r><a:t>交付🙂</a:t></a:r><a:br/><a:fld><a:t>缓存字段</a:t></a:fld></a:p></p:txBody></p:sp><p:graphicFrame><a:graphic><a:graphicData><a:tbl><a:tr><a:tc><a:txBody><a:p><a:r><a:t>表格资料</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame></p:spTree></p:cSld></p:sld>`
	p["ppt/slides/a.xml"] = `<p:sld xmlns:p="` + presNS + `" xmlns:a="` + drawNS + `"><p:cSld><p:spTree><p:grpSp><p:sp><p:txBody><a:p><a:r><a:t>第二页</a:t></a:r></a:p></p:txBody></p:sp></p:grpSp></p:spTree></p:cSld></p:sld>`
	p["ppt/slides/_rels/z.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="notes" Type="` + relNS + `/notesSlide" Target="../notesSlides/n.xml"/><Relationship Id="hyperlink" Type="` + relNS + `/hyperlink" Target="https://example.invalid/private" TargetMode="External"/></Relationships>`
	p["ppt/notesSlides/n.xml"] = `<p:notes xmlns:p="` + presNS + `" xmlns:a="` + drawNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>讲者核对说明</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:nvPr><p:ph type="sldNum"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>DO_NOT_INDEX_SLIDE_NUMBER</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`
	return p
}

func TestOfficeTextExtraction(t *testing.T) {
	ctx := context.Background()
	for _, strict := range []bool{false, true} {
		for _, kind := range []string{"xlsx", "pptx"} {
			t.Run(kind+map[bool]string{false: " transitional", true: " strict"}[strict], func(t *testing.T) {
				parts, typ := workbookParts(), xlsxMediaType
				want := []string{"1904", "A3；隐藏行：文本=\"香港预算🙂\"", "C3；隐藏行；隐藏列：公式（未计算）=\"SUM(A1:A2)\"；文件缓存（可能过期）：数值原值=12.50", "无缓存值", "_x0041_ 🙂", "A4：布尔=1", "合并范围 A3:B3", "状态=veryHidden", "隐藏说明"}
				if kind == "pptx" {
					parts, typ = presentationParts(), pptxMediaType
					want = []string{"幻灯片 1；状态=hidden", "季度交付🙂\n缓存字段", "表格资料", "讲者核对说明", "幻灯片 2；状态=visible", "第二页"}
				}
				if strict {
					for name, body := range parts {
						body = strings.ReplaceAll(body, sheetNS, "http://purl.oclc.org/ooxml/spreadsheetml/main")
						body = strings.ReplaceAll(body, presNS, "http://purl.oclc.org/ooxml/presentationml/main")
						body = strings.ReplaceAll(body, drawNS, "http://purl.oclc.org/ooxml/drawingml/main")
						parts[name] = strings.ReplaceAll(body, relNS, "http://purl.oclc.org/ooxml/officeDocument/relationships")
					}
				}
				got, err := ExtractText(ctx, typ, officeZip(t, parts, 0))
				if err != nil {
					t.Fatal(err)
				}
				for _, value := range want {
					if !strings.Contains(got, value) {
						t.Fatalf("missing %q in %q", value, got)
					}
				}
				if strings.Contains(got, "DO_NOT_INDEX") || strings.Contains(got, "ORPHAN_MUST") {
					t.Fatal("unreferenced/boilerplate text indexed")
				}
				if kind == "xlsx" && strings.Index(got, "香港预算") > strings.Index(got, "隐藏说明") || kind == "pptx" && strings.Index(got, "季度交付") > strings.Index(got, "第二页") {
					t.Fatal("sorted filenames instead of document relationships")
				}
			})
		}
	}
	for _, test := range []struct {
		name, kind string
		mutate     func(map[string]string)
	}{
		{"missing target", "xlsx", func(p map[string]string) { delete(p, "xl/worksheets/z.xml") }},
		{"external sheet", "xlsx", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.Replace(p["xl/_rels/workbook.xml.rels"], `Target="worksheets/z.xml"`, `Target="https://example.invalid" TargetMode="External"`, 1)
		}},
		{"missing shared string", "xlsx", func(p map[string]string) { p["xl/sharedStrings.xml"] = `<sst xmlns="` + sheetNS + `"/>` }},
		{"bad cell index", "xlsx", func(p map[string]string) {
			p["xl/worksheets/z.xml"] = strings.ReplaceAll(p["xl/worksheets/z.xml"], "C3", "A3")
		}},
		{"duplicate row", "xlsx", func(p map[string]string) {
			p["xl/worksheets/z.xml"] = strings.Replace(p["xl/worksheets/z.xml"], "<row>", `<row r="3">`, 1)
		}},
		{"NUL escape", "xlsx", func(p map[string]string) {
			p["xl/sharedStrings.xml"] = strings.ReplaceAll(p["xl/sharedStrings.xml"], "香港", "_x0000_")
		}},
		{"unpaired surrogate", "xlsx", func(p map[string]string) {
			p["xl/sharedStrings.xml"] = strings.ReplaceAll(p["xl/sharedStrings.xml"], "香港", "_xD800_")
		}},
		{"bad cell type", "xlsx", func(p map[string]string) {
			p["xl/worksheets/z.xml"] = strings.Replace(p["xl/worksheets/z.xml"], `t="s"`, `t="unknown"`, 1)
		}},
		{"macro entry", "xlsx", func(p map[string]string) { p["xl/vbaProject.bin"] = "unsafe" }},
		{"macro content type", "pptx", func(p map[string]string) {
			p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], "presentationml.presentation.main+xml", "presentationml.presentation.macroEnabled.main+xml")
		}},
		{"escape", "pptx", func(p map[string]string) {
			p["ppt/_rels/presentation.xml.rels"] = strings.ReplaceAll(p["ppt/_rels/presentation.xml.rels"], "slides/z.xml", "../../../secret")
		}},
		{"encoded escape", "pptx", func(p map[string]string) {
			p["ppt/_rels/presentation.xml.rels"] = strings.ReplaceAll(p["ppt/_rels/presentation.xml.rels"], "slides/z.xml", "%2e%2e/%2e%2e/secret")
		}},
		{"duplicate relationship", "pptx", func(p map[string]string) {
			p["ppt/_rels/presentation.xml.rels"] = strings.ReplaceAll(p["ppt/_rels/presentation.xml.rels"], `Id="first"`, `Id="second"`)
		}},
		{"malformed notes", "pptx", func(p map[string]string) { p["ppt/notesSlides/n.xml"] = "<broken>" }},
		{"DTD", "pptx", func(p map[string]string) { p["ppt/slides/z.xml"] = `<!DOCTYPE p:sld>` + p["ppt/slides/z.xml"] }},
		{"duplicate root", "xlsx", func(p map[string]string) { p["xl/worksheets/z.xml"] += p["xl/worksheets/z.xml"] }},
		{"conditional content", "pptx", func(p map[string]string) {
			p["ppt/slides/z.xml"] = strings.Replace(p["ppt/slides/z.xml"], "<p:spTree>", `<p:spTree><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"/>`, 1)
		}},
		{"ZIP path", "xlsx", func(p map[string]string) { p["../escape"] = "unsafe" }},
		{"depth", "xlsx", func(p map[string]string) {
			p["xl/sharedStrings.xml"] = `<sst xmlns="` + sheetNS + `">` + strings.Repeat("<si>", 65) + strings.Repeat("</si>", 65) + "</sst>"
		}},
		{"output limit", "pptx", func(p map[string]string) {
			p["ppt/slides/z.xml"] = strings.Replace(p["ppt/slides/z.xml"], "季度", strings.Repeat("x", TextOutputMaxBytes+1), 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, typ := workbookParts(), xlsxMediaType
			if test.kind == "pptx" {
				p, typ = presentationParts(), pptxMediaType
			}
			test.mutate(p)
			got, err := ExtractText(ctx, typ, officeZip(t, p, 0))
			if err == nil || got != "" {
				t.Fatal("partial or unsafe content indexed", got, err)
			}
		})
	}
	t.Run("special ZIP mode", func(t *testing.T) {
		if _, err := ExtractText(ctx, xlsxMediaType, officeZip(t, workbookParts(), fs.ModeSymlink|0o777)); err == nil {
			t.Fatal("symlink accepted")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := ExtractText(c, pptxMediaType, officeZip(t, presentationParts(), 0)); err == nil {
			t.Fatal("cancel ignored")
		}
	})
	t.Run("extractor separation", func(t *testing.T) {
		for _, typ := range []string{xlsxMediaType, pptxMediaType, strings.ToUpper(xlsxMediaType) + " ; charset=UTF-8"} {
			if TextExtractorFor(typ) != OfficeTextExtractor {
				t.Fatal(typ)
			}
		}
		if TextExtractorFor("text/plain") != TextExtractor {
			t.Fatal("legacy extractor changed")
		}
	})
}
