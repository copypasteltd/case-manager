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

const AgentOfficeEditAdapter = "agent-office-edit-v1"

type AgentOfficeEditInput struct {
	WorkspaceSHA256 string         `json:"workspace_sha256"`
	Changes         []OfficeChange `json:"changes"`
}

type agentOfficeEditRecipe struct {
	Adapter      string               `json:"adapter"`
	Kind         string               `json:"kind"`
	SourceSHA256 string               `json:"source_sha256"`
	Edit         AgentOfficeEditInput `json:"edit"`
}

func officeProjection(workspace OfficeWorkspace) string {
	var out strings.Builder
	for _, u := range workspace.Units {
		out.WriteString("[" + u.Group + " · " + u.Label + "] " + u.Value + "\n")
	}
	return out.String()
}

// Only existing writable units can change. The ordinary Office editor keeps
// untouched compressed parts byte-for-byte, rejects macros and external data,
// and never accepts caller-provided archive bytes or part paths.
func AgentEditOffice(ctx context.Context, media string, source []byte, edit AgentOfficeEditInput) ([]byte, AgentTextDiff, string, error) {
	var diff AgentTextDiff
	kind := AgentOfficeKind(media)
	encoded, err := json.Marshal(edit)
	if kind == "" || len(source) < 1 || int64(len(source)) > AgentTextMaxBytes || err != nil || len(encoded) > AgentCreateMaxBytes || len(edit.Changes) < 1 || len(edit.Changes) > 64 {
		return nil, diff, "", core.Invalid("office", "请选择准确Office工作区及1至64项编辑；结构化内容不得超过48KiB")
	}
	before, err := OfficeDocumentWorkspace(ctx, kind, source)
	if err != nil {
		return nil, diff, "", err
	}
	body, after, err := EditOfficeDocument(ctx, kind, source, edit.WorkspaceSHA256, edit.Changes)
	if err != nil {
		return nil, diff, "", err
	}
	if int64(len(body)) > AgentTextMaxBytes {
		return nil, diff, "", core.Invalid("office", "秘书Office编辑候选不得超过1MiB")
	}
	oldText, newText := officeProjection(before), officeProjection(after)
	oldRunes, newRunes := []rune(oldText), []rune(newText)
	sourceSHA := core.Digest(source)
	diff = AgentTextDiff{RemovedChars: utf8.RuneCountInString(oldText), AddedChars: utf8.RuneCountInString(newText),
		RemovedSHA256: core.Digest([]byte(oldText)), AddedSHA256: core.Digest([]byte(newText)),
		Before: string(oldRunes[:min(len(oldRunes), 1000)]), After: string(newRunes[:min(len(newRunes), 1000)]),
		BeforeComplete: len(oldRunes) <= 1000, AfterComplete: len(newRunes) <= 1000, LineEnding: "lf",
		Office: &AgentOfficePreview{AgentOfficeEditAdapter, kind, sourceSHA, after.SHA256, len(after.Units), append([]string{}, after.Warnings...)}}
	recipe, err := json.Marshal(agentOfficeEditRecipe{AgentOfficeEditAdapter, kind, sourceSHA, edit})
	return body, diff, string(recipe), err
}

func AgentOfficeRecoveryEdit(saved, media, sourceSHA string) (AgentOfficeEditInput, error) {
	var recipe agentOfficeEditRecipe
	d := json.NewDecoder(strings.NewReader(saved))
	d.DisallowUnknownFields()
	if d.Decode(&recipe) != nil || d.Decode(new(any)) != io.EOF {
		return AgentOfficeEditInput{}, core.Fail(409, "FILE_RECOVERY_RECIPE_CHANGED", "Office编辑恢复依据无效")
	}
	canonical, err := json.Marshal(recipe)
	if err != nil || recipe.Adapter != AgentOfficeEditAdapter || recipe.Kind != AgentOfficeKind(media) || recipe.SourceSHA256 != sourceSHA || !bytes.Equal(canonical, []byte(saved)) {
		return AgentOfficeEditInput{}, core.Fail(409, "FILE_RECOVERY_RECIPE_CHANGED", "Office编辑原件、适配器或准确编辑依据已经变化")
	}
	return recipe.Edit, nil
}
