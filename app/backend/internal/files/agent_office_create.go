package files

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

const AgentOfficeCreateAdapter = "agent-office-create-v1"

// Office preview coordinates and hashes describe an extracted unit projection.
// ContentSHA256 on the enclosing draft is always the complete OOXML byte hash.
type AgentOfficePreview struct {
	Adapter         string   `json:"adapter"`
	Kind            string   `json:"kind"`
	TemplateSHA256  string   `json:"template_sha256"`
	WorkspaceSHA256 string   `json:"workspace_sha256"`
	UnitCount       int      `json:"unit_count"`
	Warnings        []string `json:"warnings"`
}

type agentOfficeRecipe struct {
	Adapter        string         `json:"adapter"`
	TemplateSHA256 string         `json:"template_sha256"`
	Document       OfficeDocument `json:"document"`
}

func AgentOfficeKind(media string) string { return officeAuthorKind(media) }

func agentOfficeTemplate(kind string) (string, error) {
	if officeAuthorMedia(kind) == "" {
		return "", core.Invalid("document", "请选择受支持的Office格式")
	}
	body, err := officeAuthorTemplates.ReadFile("office_templates/empty." + kind)
	if err != nil {
		return "", err
	}
	return core.Digest(body), nil
}

// Structured intent is bounded separately from the generated binary artifact.
// No caller-supplied ZIP, macro, URL, embedded object or binary data is accepted.
func AgentCreateOffice(ctx context.Context, document OfficeDocument, media string) ([]byte, AgentTextDiff, string, error) {
	var diff AgentTextDiff
	kind := AgentOfficeKind(media)
	if kind == "" || kind != document.Kind {
		return nil, diff, "", core.Invalid("document", "Office文档类型必须匹配员工选定的文件格式")
	}
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) > AgentCreateMaxBytes {
		return nil, diff, "", core.Invalid("document", "Office结构化内容不得超过48KiB")
	}
	body, workspace, err := CreateOfficeDocument(ctx, document)
	if err != nil {
		return nil, diff, "", err
	}
	if len(body) < 1 || int64(len(body)) > AgentTextMaxBytes {
		return nil, diff, "", core.Invalid("document", "秘书新建Office候选不得超过1MiB")
	}
	templateSHA, err := agentOfficeTemplate(kind)
	if err != nil {
		return nil, diff, "", err
	}
	var projection strings.Builder
	for _, unit := range workspace.Units {
		projection.WriteString("[" + unit.Group + " · " + unit.Label + "] ")
		projection.WriteString(unit.Value)
		projection.WriteByte('\n')
	}
	text := projection.String()
	runes := []rune(text)
	warnings := append([]string{}, workspace.Warnings...)
	diff = AgentTextDiff{AddedChars: utf8.RuneCountInString(text), RemovedSHA256: core.Digest(nil),
		AddedSHA256: core.Digest([]byte(text)), After: string(runes[:min(len(runes), 1000)]),
		BeforeComplete: true, AfterComplete: len(runes) <= 1000, LineEnding: "lf",
		Office: &AgentOfficePreview{AgentOfficeCreateAdapter, kind, templateSHA, workspace.SHA256, len(workspace.Units), warnings}}
	recipe, err := json.Marshal(agentOfficeRecipe{AgentOfficeCreateAdapter, templateSHA, document})
	if err != nil {
		return nil, AgentTextDiff{}, "", err
	}
	return body, diff, string(recipe), nil
}

// Recovery uses the immutable original structured recipe, never a model rewrite.
// It pins the template as well as the existing outer complete artifact hash.
func AgentOfficeRecoveryDocument(saved, media string) (OfficeDocument, error) {
	var recipe agentOfficeRecipe
	decoder := json.NewDecoder(strings.NewReader(saved))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&recipe) != nil || decoder.Decode(new(any)) != io.EOF {
		return OfficeDocument{}, core.Fail(409, "FILE_RECOVERY_RECIPE_CHANGED", "Office恢复依据无效")
	}
	templateSHA, err := agentOfficeTemplate(recipe.Document.Kind)
	canonical, encodeErr := json.Marshal(recipe)
	if err != nil || encodeErr != nil || recipe.Adapter != AgentOfficeCreateAdapter ||
		recipe.Document.Kind != AgentOfficeKind(media) || recipe.TemplateSHA256 != templateSHA || !bytes.Equal(canonical, []byte(saved)) {
		return OfficeDocument{}, core.Fail(409, "FILE_RECOVERY_RECIPE_CHANGED", "Office恢复适配器、模板或准确内容已变化")
	}
	return recipe.Document, nil
}
