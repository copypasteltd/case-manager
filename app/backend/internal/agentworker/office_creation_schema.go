package agentworker

func officeEditInputSchema() map[string]any {
	change := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "kind", "value"}, "properties": map[string]any{
		"id":    map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
		"kind":  map[string]any{"type": "string", "enum": []string{"text", "number", "boolean", "formula"}},
		"value": map[string]any{"type": "string", "maxLength": 8192}}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"workspace_sha256", "changes"}, "properties": map[string]any{
		"workspace_sha256": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		"changes":          map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": change}}}
}

func fileEditInputSchema(input map[string]any) map[string]any {
	input["oneOf"] = []any{
		map[string]any{"required": []string{"edits"}, "not": map[string]any{"required": []string{"office"}}},
		map[string]any{"required": []string{"office"}, "not": map[string]any{"required": []string{"edits"}}},
	}
	return input
}

func fileCreationInputSchema(input map[string]any) map[string]any {
	input["oneOf"] = []any{
		map[string]any{"required": []string{"content"}, "not": map[string]any{"required": []string{"document"}}},
		map[string]any{"required": []string{"document"}, "not": map[string]any{"required": []string{"content"}}},
	}
	return input
}

// The model supplies structured content, never OOXML bytes or a remote URL.
// Go performs the stronger format/formula/content validation before rendering.
func officeCreationDocumentSchema() map[string]any {
	text := func(limit int) map[string]any { return map[string]any{"type": "string", "maxLength": limit} }
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	array := func(items any, min, max int) map[string]any {
		return map[string]any{"type": "array", "items": items, "minItems": min, "maxItems": max}
	}
	kind := func(value string) map[string]any { return map[string]any{"type": "string", "const": value} }
	cell := object(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"text", "number", "boolean", "formula"}}, "value": text(8192)}, "kind", "value")
	sheet := object(map[string]any{"name": text(31), "rows": array(array(cell, 1, 64), 1, 500)}, "name", "rows")
	slide := object(map[string]any{"title": text(256), "lines": array(text(8192), 0, 128)}, "title", "lines")
	return map[string]any{"oneOf": []any{
		object(map[string]any{"kind": kind("docx"), "title": text(256), "paragraphs": array(text(8192), 0, 500), "table": array(array(text(8192), 1, 32), 0, 200)}, "kind"),
		object(map[string]any{"kind": kind("xlsx"), "title": text(256), "sheets": array(sheet, 1, 16)}, "kind", "sheets"),
		object(map[string]any{"kind": kind("pptx"), "title": text(256), "slides": array(slide, 1, 64)}, "kind", "slides"),
	}}
}
