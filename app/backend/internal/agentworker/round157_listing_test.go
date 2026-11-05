package agentworker

import (
	"encoding/json"
	"testing"
)

func TestRound157ToolListing(t *testing.T) {
	tools := toolDefinitions("file-update")
	encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "list", "result": map[string]any{"tools": tools}})
	if err != nil {
		t.Fatal("file-update tool listing cannot serialize:", err)
	}
	if len(tools) != 10 || len(encoded) > 120000 {
		t.Fatalf("unexpected listing bounds: %d tools, %d bytes", len(tools), len(encoded))
	}
	var received struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if json.Unmarshal(encoded, &received) != nil || len(received.Result.Tools) != 10 {
		t.Fatal("complete client-decodable tool listing required")
	}
}
