package files

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestAgentOfficeCreation(t *testing.T) {
	documents := []OfficeDocument{
		{Kind: "docx", Title: "项目说明", Paragraphs: []string{"香港团队🙂", "完整交付正文"}, Table: [][]string{{"事项", "负责人"}, {"方案", "员工"}}},
		{Kind: "xlsx", Sheets: []OfficeSheet{{Name: "月开支", Rows: [][]OfficeCell{{{Kind: "text", Value: "订阅"}, {Kind: "number", Value: "1200.50"}}, {{Kind: "text", Value: "合计"}, {Kind: "formula", Value: "=SUM(B1:B1)"}}}}}},
		{Kind: "pptx", Slides: []OfficeSlide{{Title: "项目周报", Lines: []string{"交付已准备", "待员工核对"}}, {Title: "下一步", Lines: []string{"明确验收依据"}}}},
	}
	for _, document := range documents {
		t.Run(document.Kind, func(t *testing.T) {
			body, diff, recipe, err := AgentCreateOffice(context.Background(), document, officeAuthorMedia(document.Kind))
			if err != nil || !bytes.HasPrefix(body, []byte("PK")) || diff.Office == nil || diff.Office.Kind != document.Kind || diff.Office.UnitCount < 1 || diff.After == "" || diff.AddedSHA256 == core.Digest(body) {
				t.Fatal("real binary and explicit content projection required", err)
			}
			workspace, err := OfficeDocumentWorkspace(context.Background(), document.Kind, body)
			if err != nil || workspace.SHA256 != diff.Office.WorkspaceSHA256 || len(workspace.Units) != diff.Office.UnitCount {
				t.Fatal("preview not bound to actual generated OOXML", err)
			}
			recovered, err := AgentOfficeRecoveryDocument(recipe, officeAuthorMedia(document.Kind))
			if err != nil {
				t.Fatal(err)
			}
			second, again, recipeAgain, err := AgentCreateOffice(context.Background(), recovered, officeAuthorMedia(document.Kind))
			if err != nil || !bytes.Equal(body, second) || !reflect.DeepEqual(diff, again) || recipe != recipeAgain {
				t.Fatal("recovery must reproduce exact bytes and projection", err)
			}
		})
	}
}

func TestAgentOfficeCreationBoundaries(t *testing.T) {
	for _, document := range []OfficeDocument{
		{Kind: "docx", Paragraphs: []string{strings.Repeat("A", 8000), strings.Repeat("B", 8000), strings.Repeat("C", 8000), strings.Repeat("D", 8000), strings.Repeat("E", 8000), strings.Repeat("F", 8000), strings.Repeat("G", 8000)}},
		{Kind: "docx", Paragraphs: []string{"bad\x00content"}},
		{Kind: "xlsx", Sheets: []OfficeSheet{{Name: "费用", Rows: [][]OfficeCell{{{Kind: "formula", Value: "=WEBSERVICE(\"https://invalid.example\")"}}}}}},
		{Kind: "xlsx", Sheets: []OfficeSheet{{Name: "费用", Rows: [][]OfficeCell{{{Kind: "number", Value: "NaN"}}}}}},
		{Kind: "docx", Paragraphs: []string{"不能换格式"}, Sheets: []OfficeSheet{{Name: "混合输入"}}},
	} {
		if _, _, _, err := AgentCreateOffice(context.Background(), document, officeAuthorMedia(document.Kind)); err == nil {
			t.Fatal("unsafe or unsupported structured content accepted", document.Kind)
		}
	}
	if _, _, _, err := AgentCreateOffice(context.Background(), OfficeDocument{Kind: "docx", Paragraphs: []string{"wrong target"}}, xlsxMediaType); err == nil {
		t.Fatal("human-selected media scope was expanded")
	}
	document := OfficeDocument{Kind: "xlsx", Sheets: []OfficeSheet{{Name: "Literal", Rows: [][]OfficeCell{{{Kind: "text", Value: "=WEBSERVICE(\"https://invalid.example\")"}}}}}}
	body, _, _, err := AgentCreateOffice(context.Background(), document, xlsxMediaType)
	if err != nil {
		t.Fatal("explicit literal must not become a formula", err)
	}
	workspace, err := OfficeDocumentWorkspace(context.Background(), "xlsx", body)
	if err != nil || len(workspace.Units) != 1 || workspace.Units[0].Kind != "text" {
		t.Fatal("literal expression was reinterpreted", err)
	}
}

func TestAgentOfficeRecoveryBoundaries(t *testing.T) {
	_, _, recipe, err := AgentCreateOffice(context.Background(), OfficeDocument{Kind: "docx", Paragraphs: []string{"原始批准范围内的正文"}}, docxMediaType)
	if err != nil {
		t.Fatal(err)
	}
	for _, saved := range []string{
		" " + recipe, recipe + "{}", strings.Replace(recipe, AgentOfficeCreateAdapter, "other-adapter", 1),
		strings.Replace(recipe, `"template_sha256":"`, `"template_sha256":"0`, 1),
		strings.Replace(recipe, `"document":`, `"unknown":true,"document":`, 1),
	} {
		if _, err := AgentOfficeRecoveryDocument(saved, docxMediaType); err == nil {
			t.Fatal("changed recovery evidence accepted")
		}
	}
	if _, err := AgentOfficeRecoveryDocument(recipe, pptxMediaType); err == nil {
		t.Fatal("recovery changed target format")
	}
}
