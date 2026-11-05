package files

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestRound157OfficeEdits(t *testing.T) {
	for _, document := range []OfficeDocument{
		{Kind: "docx", Title: "原件🙂", Paragraphs: []string{"保持版式的原段落", "第二段"}},
		{Kind: "xlsx", Sheets: []OfficeSheet{{Name: "费用", Rows: [][]OfficeCell{{{Kind: "text", Value: "原件"}, {Kind: "number", Value: "120.50"}}}}}},
		{Kind: "pptx", Slides: []OfficeSlide{{Title: "原件", Lines: []string{"第一段", "第二段"}}}},
	} {
		t.Run(document.Kind, func(t *testing.T) {
			ctx := context.Background()
			media := officeAuthorMedia(document.Kind)
			source, workspace, err := CreateOfficeDocument(ctx, document)
			if err != nil {
				t.Fatal(err)
			}
			var unit OfficeUnit
			for _, u := range workspace.Units {
				if !u.ReadOnly && u.Kind == "text" {
					unit = u
					break
				}
			}
			if unit.ID == "" {
				t.Fatal("missing writable unit")
			}
			edit := AgentOfficeEditInput{workspace.SHA256, []OfficeChange{{ID: unit.ID, Kind: unit.Kind, Value: "人工限定更新🙂"}}}
			body, diff, recipe, err := AgentEditOffice(ctx, media, source, edit)
			if err != nil || bytes.Equal(body, source) || diff.Office == nil || diff.Office.Adapter != AgentOfficeEditAdapter || diff.Office.TemplateSHA256 != core.Digest(source) {
				t.Fatal("original-bound real Office update missing", err)
			}
			recovered, err := AgentOfficeRecoveryEdit(recipe, media, core.Digest(source))
			if err != nil {
				t.Fatal(err)
			}
			again, _, _, err := AgentEditOffice(ctx, media, source, recovered)
			if err != nil || !bytes.Equal(again, body) {
				t.Fatal("exact binary recovery changed", err)
			}
			if _, err = AgentOfficeRecoveryEdit(recipe, media, core.Digest(body)); err == nil {
				t.Fatal("recovery accepted changed source")
			}
			stale := edit
			stale.WorkspaceSHA256 = core.Digest(nil)
			if _, _, _, err = AgentEditOffice(ctx, media, source, stale); err == nil {
				t.Fatal("stale workspace accepted")
			}
			invalid := edit
			invalid.Changes = []OfficeChange{{ID: "arbitrary/part", Kind: "text", Value: "unsafe"}}
			if _, _, _, err = AgentEditOffice(ctx, media, source, invalid); err == nil {
				t.Fatal("invented unit accepted")
			}
			oldZip, _ := zip.NewReader(bytes.NewReader(source), int64(len(source)))
			newZip, _ := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			old := map[string][]byte{}
			for _, p := range oldZip.File {
				r, _ := p.Open()
				old[p.Name], _ = io.ReadAll(r)
				r.Close()
			}
			unchanged := 0
			for _, p := range newZip.File {
				r, _ := p.Open()
				value, _ := io.ReadAll(r)
				r.Close()
				if bytes.Equal(old[p.Name], value) {
					var previous *zip.File
					for _, o := range oldZip.File {
						if o.Name == p.Name {
							previous = o
							break
						}
					}
					a, _ := previous.OpenRaw()
					b, _ := p.OpenRaw()
					ar, _ := io.ReadAll(a)
					br, _ := io.ReadAll(b)
					if !bytes.Equal(ar, br) {
						t.Fatal("untouched compressed part changed", p.Name)
					}
					unchanged++
				}
			}
			if unchanged < 2 {
				t.Fatal("untouched archive parts not preserved")
			}
		})
	}
}
