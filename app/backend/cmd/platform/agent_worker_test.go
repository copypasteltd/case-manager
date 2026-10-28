package main

import (
	"copypaste.hk/platform/internal/agentworker"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentWorkerNativeModelGatewayExcludesHostProviderKey(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "codex")
	if os.WriteFile(executable, []byte("synthetic executable path fixture"), 0700) != nil {
		t.Fatal("fixture unavailable")
	}
	t.Setenv("CP_AGENT_MODEL", "approved-model")
	t.Setenv("CP_AGENT_MODEL_GATEWAY", "native")
	t.Setenv("CP_AGENT_MODEL_BASE_URL", "http://127.0.0.1:18082/v1")
	t.Setenv("CP_AGENT_CODEX_EXECUTABLE", executable)
	t.Setenv("CP_AGENT_OPENAI_API_KEY", "")
	var settings agentworker.NativeSettings
	if configureAgentModel(&settings) != nil || settings.APIKey != "native-broker-managed" || settings.APIBaseURL != "http://127.0.0.1:18082/v1" || settings.ExecutablePath != executable {
		t.Fatal("native relay settings unavailable")
	}
	t.Run("host key never belongs in worker", func(t *testing.T) {
		t.Setenv("CP_AGENT_OPENAI_API_KEY", "synthetic-host-key")
		if configureAgentModel(&agentworker.NativeSettings{}) == nil {
			t.Fatal("host model key accepted by native worker")
		}
	})
	for _, endpoint := range []string{"https://api.openai.com/v1", "http://0.0.0.0:18082/v1", "http://127.0.0.1/v1", "http://user@127.0.0.1:18082/v1", "http://127.0.0.1:18082/v1?url=x", "http://127.0.0.1:18082/v1#x"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("CP_AGENT_MODEL_BASE_URL", endpoint)
			if configureAgentModel(&agentworker.NativeSettings{}) == nil {
				t.Fatal("unapproved relay accepted")
			}
		})
	}
	t.Run("missing executable denied", func(t *testing.T) {
		t.Setenv("CP_AGENT_CODEX_EXECUTABLE", executable+".missing")
		if configureAgentModel(&agentworker.NativeSettings{}) == nil {
			t.Fatal("missing executable accepted")
		}
	})
	t.Run("unknown mode denied", func(t *testing.T) {
		t.Setenv("CP_AGENT_MODEL_GATEWAY", "fallback")
		if configureAgentModel(&agentworker.NativeSettings{}) == nil {
			t.Fatal("unknown mode accepted")
		}
	})
	t.Run("existing signed custom launcher path retained", func(t *testing.T) {
		t.Setenv("CP_AGENT_MODEL_GATEWAY", "")
		t.Setenv("CP_AGENT_OPENAI_API_KEY", "synthetic-custom-provider-key")
		var s agentworker.NativeSettings
		if configureAgentModel(&s) != nil || s.APIKey != "synthetic-custom-provider-key" || s.APIBaseURL != "" {
			t.Fatal("custom signed runtime configuration changed")
		}
	})
}
