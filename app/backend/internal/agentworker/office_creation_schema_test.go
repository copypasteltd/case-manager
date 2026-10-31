package agentworker

import "testing"

func TestOfficeCreationMCPBoundary(t *testing.T) {
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		found := false
		for _, raw := range scopedToolDefinitions("file-update", channel) {
			definition := raw.(map[string]any)
			if definition["name"] != "stage_file_create" {
				continue
			}
			found = true
			input := definition["inputSchema"].(map[string]any)
			properties := input["properties"].(map[string]any)
			branches := properties["document"].(map[string]any)["oneOf"].([]any)
			if len(input["oneOf"].([]any)) != 2 || len(branches) != 3 || input["additionalProperties"] != false || properties["content"] == nil || properties["media_type"] != nil || properties["content_base64"] != nil || properties["template_path"] != nil || properties["url"] != nil {
				t.Fatal("structured-only content and exact selected scope required", channel)
			}
			for _, branch := range branches {
				if branch.(map[string]any)["additionalProperties"] != false {
					t.Fatal("unknown Office fields exposed")
				}
			}
		}
		if !found {
			t.Fatal("missing ordinary creation tool", channel)
		}
	}
	for _, raw := range scopedToolDefinitions("file-update", "telegram") {
		definition := raw.(map[string]any)
		if definition["name"] == "stage_file_create" || definition["name"] == "commit_file_create" {
			t.Fatal("TG gained file write authority")
		}
	}
}
