package files

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficeAuthorCreateEditPreservesParts(t *testing.T) {
	docs := []OfficeDocument{
		{Kind: "docx", Title: "交付说明", Paragraphs: []string{"第一段 香港🙂", "第二段"}, Table: [][]string{{"项目", "金额"}, {"案例", "1200"}}},
		{Kind: "xlsx", Title: "费用", Sheets: []OfficeSheet{{Name: "开支", Rows: [][]OfficeCell{{{Kind: "text", Value: "_x0041_ 合计"}, {Kind: "number", Value: "1200"}}, {{Kind: "text", Value: "项目"}, {Kind: "formula", Value: "=SUM(B1:B1)"}}}}, {Name: "备注", Rows: [][]OfficeCell{{{Kind: "text", Value: "待审批"}}}}}},
		{Kind: "pptx", Title: "项目报告", Slides: []OfficeSlide{{Title: "项目概况", Lines: []string{"已完成方案", "待财务审批"}}, {Title: "下一步", Lines: []string{"核对交付"}}}},
	}
	for _, doc := range docs {
		t.Run(doc.Kind, func(t *testing.T) {
			body, w, err := CreateOfficeDocument(context.Background(), doc)
			if err != nil {
				t.Fatal(err)
			}
			if len(w.Units) < 3 {
				t.Fatalf("missing units: %+v", w)
			}
			first := w.Units[0]
			value := "新版内容 香港🙂"
			if first.Kind != "text" {
				t.Fatal(first)
			}
			out, edited, err := EditOfficeDocument(context.Background(), doc.Kind, body, w.SHA256, []OfficeChange{{ID: first.ID, Kind: "text", Value: value}})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, u := range edited.Units {
				if u.Value == value {
					found = true
				}
			}
			if !found || bytes.Equal(out, body) {
				t.Fatal("edit not applied")
			}
			before, _ := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			after, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
			old := map[string]*zip.File{}
			for _, f := range before.File {
				old[f.Name] = f
			}
			preserved := 0
			for _, f := range after.File {
				if old[f.Name] == nil {
					continue
				}
				a, _ := f.OpenRaw()
				b, _ := old[f.Name].OpenRaw()
				var x, y bytes.Buffer
				x.ReadFrom(a)
				y.ReadFrom(b)
				if bytes.Equal(x.Bytes(), y.Bytes()) {
					preserved++
				}
			}
			if preserved < len(before.File)-3 {
				t.Fatalf("unrelated parts rewritten: %d/%d", preserved, len(before.File))
			}
			if _, _, err = EditOfficeDocument(context.Background(), doc.Kind, body, strings.Repeat("a", 64), []OfficeChange{{ID: first.ID, Kind: "text", Value: value}}); err == nil {
				t.Fatal("stale workspace accepted")
			}
			if _, _, err = EditOfficeDocument(context.Background(), doc.Kind, body, w.SHA256, []OfficeChange{{ID: first.ID, Kind: first.Kind, Value: first.Value}}); err == nil {
				t.Fatal("no-op accepted")
			}
			if target := os.Getenv("CP_OFFICE_AUTHOR_FIXTURES"); target != "" {
				if err = os.MkdirAll(target, 0700); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{"created": body, "edited": out} {
					if err = os.WriteFile(filepath.Join(target, name+"."+doc.Kind), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
func TestOfficeAuthorFormulaAndInputBoundaries(t *testing.T) {
	for _, s := range []string{"=SUM(A1:B2)", "=IF(A1>0,ROUND(12.5*2,0),0)", "='开支'!B1+2", "=A1&\" text\"", "=1e2+2"} {
		if !authorFormula(s, []string{"开支"}) {
			t.Errorf("valid formula: %s", s)
		}
	}
	for _, s := range []string{"=1+", "=SUM(A1,,B2)", "=WEBSERVICE(\"https://example.invalid\")", "=[book]Sheet1!A1", "=cmd|' /C calc'!A1", "=INDIRECT(\"A1\")", "=Missing!A1", "=SUM()", "=1 2", "=1.2.3", "=SUM(A1"} {
		if authorFormula(s, nil) {
			t.Errorf("unsafe/invalid formula: %s", s)
		}
	}
	if authorCell(OfficeCell{Kind: "number", Value: "1234567890123456"}, nil) {
		t.Fatal("precision loss allowed")
	}
	for _, doc := range []OfficeDocument{{Kind: "docx"}, {Kind: "xlsx", Sheets: []OfficeSheet{{Name: "bad/name", Rows: [][]OfficeCell{{{Kind: "text", Value: "x"}}}}}}, {Kind: "docx", Paragraphs: []string{"bad\x00text"}}, {Kind: "pptx", Slides: []OfficeSlide{{Title: strings.Repeat("x", 8193)}}}} {
		if _, _, err := CreateOfficeDocument(context.Background(), doc); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestOfficeAuthorUnsafeOriginalsAndReadonlyUnits(t *testing.T) {
	ctx := context.Background()
	body, _, err := CreateOfficeDocument(ctx, OfficeDocument{Kind: "docx", Paragraphs: []string{"可修改正文"}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := openAuthorPackage(ctx, "docx", body)
	if err != nil {
		t.Fatal(err)
	}
	source := string(a.data[a.main])
	for label, fragment := range map[string]string{
		"split external field":  `<w:r><w:instrText>INCLU</w:instrText><w:instrText>DETEXT https://example.invalid</w:instrText></w:r>`,
		"simple external field": `<w:fldSimple w:instr="DDEAUTO hidden"><w:r><w:t>缓存</w:t></w:r></w:fldSimple>`,
		"protected":             `<w:documentProtection w:enforcement="1"/>`,
		"embedded":              `<w:altChunk/>`,
	} {
		t.Run(label, func(t *testing.T) {
			data := strings.Replace(source, "</w:body>", fragment+"</w:body>", 1)
			out, e := authorRepack(ctx, a.base, map[string][]byte{a.main: []byte(data)}, nil)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = OfficeDocumentWorkspace(ctx, "docx", out); e == nil {
				t.Fatal("unsupported original accepted")
			}
		})
	}
	tracked := strings.Replace(source, "</w:body>", `<w:p><w:ins><w:r><w:t>保留修订</w:t></w:r></w:ins><w:fldSimple w:instr="DATE"><w:r><w:t>日期缓存</w:t></w:r></w:fldSimple></w:p></w:body>`, 1)
	out, e := authorRepack(ctx, a.base, map[string][]byte{a.main: []byte(tracked)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	w, e := OfficeDocumentWorkspace(ctx, "docx", out)
	if e != nil {
		t.Fatal(e)
	}
	readonly := 0
	for _, unit := range w.Units {
		if unit.Value == "保留修订" || unit.Value == "日期缓存" {
			readonly++
			if !unit.ReadOnly {
				t.Fatal("tracked/field result editable")
			}
			if _, _, e = EditOfficeDocument(ctx, "docx", out, w.SHA256, []OfficeChange{{ID: unit.ID, Kind: "text", Value: "替换"}}); e == nil {
				t.Fatal("readonly edit accepted")
			}
		}
	}
	if readonly != 2 {
		t.Fatal("missing protected content")
	}
}
