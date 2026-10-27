package files

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func docxFixture(t *testing.T, document string, extra map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	z := zip.NewWriter(&body)
	entries := map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": document}
	for k, v := range extra {
		entries[k] = v
	}
	for k, v := range entries {
		w, err := z.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func TestFileTextExtraction(t *testing.T) {
	ctx := context.Background()
	mime := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	document := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>香港🙂 &amp; CopyPaste</w:t><w:tab/><w:t>正文</w:t><w:br/><w:t>第二行</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>表格内容</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`
	t.Run("docx paragraphs tables and Unicode", func(t *testing.T) {
		got, err := ExtractText(ctx, mime, docxFixture(t, document, nil))
		if err != nil || got != "香港🙂 & CopyPaste\t正文\n第二行\n表格内容" {
			t.Fatalf("extraction %q %v", got, err)
		}
	})
	for name, input := range map[string]string{"UTF8": "\ufeff香港🙂\r\n第二行\r结尾", "JSON": "{\"说明\":\"香港🙂\"}"} {
		t.Run(name, func(t *testing.T) {
			typ := "text/plain; charset=UTF-8"
			want := "香港🙂\n第二行\n结尾"
			if name == "JSON" {
				typ = "application/json"
				want = input
			}
			got, e := ExtractText(ctx, typ, []byte(input))
			if e != nil || got != want {
				t.Fatalf("%q %v", got, e)
			}
		})
	}
	cases := []struct {
		name, typ string
		body      []byte
	}{
		{"unsupported PDF", "application/pdf", []byte("%PDF")},
		{"invalid UTF8", "text/plain", []byte{0xff}}, {"NUL", "text/plain", []byte{'a', 0}},
		{"wrong charset", "text/plain; charset=gb18030", []byte("test")},
		{"invalid JSON", "application/json", []byte("{truncated")},
		{"text bound", "text/plain", bytes.Repeat([]byte{'x'}, int(AgentTextMaxBytes)+1)},
		{"source bound", mime, bytes.Repeat([]byte{'x'}, int(TextSourceMaxBytes)+1)},
		{"malformed ZIP", mime, []byte("not a zip")},
		{"malformed XML", mime, docxFixture(t, strings.TrimSuffix(document, "</w:document>"), nil)},
		{"DTD", mime, docxFixture(t, `<!DOCTYPE x [<!ENTITY ext SYSTEM "https://example.invalid/private">]>`+document, nil)},
		{"multiple roots", mime, docxFixture(t, document+document, nil)},
		{"wrong root", mime, docxFixture(t, `<document><t>hidden</t></document>`, nil)},
		{"path traversal", mime, docxFixture(t, document, map[string]string{"../secret": "x"})},
		{"macro", mime, docxFixture(t, document, map[string]string{"word/vbaProject.bin": "x"})},
		{"expansion", mime, docxFixture(t, document, map[string]string{"word/payload": strings.Repeat("x", 2<<20)})},
		{"depth", mime, docxFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`+strings.Repeat("<w:p>", 65)+strings.Repeat("</w:p>", 65)+"</w:document>", nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, e := ExtractText(ctx, c.typ, c.body)
			if e == nil || text != "" {
				t.Fatal("unsafe or incomplete content indexed")
			}
		})
	}
	t.Run("cancellation", func(t *testing.T) {
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := ExtractText(c, "text/plain", []byte("x")); e == nil {
			t.Fatal("cancelled extraction ran")
		}
	})
}
