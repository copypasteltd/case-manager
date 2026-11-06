package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateBotConfiguration(t *testing.T) {
	const key = "CP_TELEGRAM_DELIVERY_BOTS"
	// Tests must unset rather than set an empty inline source: two sources are
	// ambiguous even when one of them is empty.
	previous, present := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if present {
			os.Setenv(key, previous)
		} else {
			os.Unsetenv(key)
		}
	})
	path := filepath.Join(t.TempDir(), "bots.json")
	content := `[{"bot_id":"123456","organization_id":"10000000-0000-4000-8000-000000000001","token":"123456:abcdefghijklmnopqrstuvwxyz123456","portal_origin":"https://case.example.invalid"}]`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(key+"_FILE", path)
	bots, err := LoadDeliveryBots()
	if err != nil || len(bots) != 1 || bots[0].BotID != "123456" {
		t.Fatalf("private credential load failed: %v", err)
	}
	t.Run("ambiguous-empty-inline", func(t *testing.T) {
		t.Setenv(key, "")
		_, err := LoadDeliveryBots()
		if err == nil || strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "abcdefghijklmnopqrstuvwxyz") {
			t.Fatal("ambiguous source or diagnostic disclosure")
		}
	})
	for _, value := range []string{"", "bots.json", filepath.Join(path, "missing"), filepath.Dir(path)} {
		t.Run("invalid-file", func(t *testing.T) {
			t.Setenv(key+"_FILE", value)
			if _, err := LoadDeliveryBots(); err == nil {
				t.Fatal("invalid file accepted")
			}
		})
	}
	for _, invalid := range []string{strings.Repeat("x", 8193), "", content + "{}", strings.Replace(content, "bot_id", "unknown", 1)} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDeliveryBots(); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(path), "alias.json")
	if os.Symlink(path, link) == nil {
		t.Setenv(key+"_FILE", link)
		if _, err := LoadDeliveryBots(); err == nil {
			t.Fatal("symlink credential accepted")
		}
	}
}

func TestPrivateIngressAndDispatchConfiguration(t *testing.T) {
	for _, key := range []string{"CP_TELEGRAM_INGRESS", "CP_TELEGRAM_DISPATCH_BOTS"} {
		old, present := os.LookupEnv(key)
		os.Unsetenv(key)
		key, old, present := key, old, present
		t.Cleanup(func() {
			if present {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "ingress.json")
	if err := os.WriteFile(path, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CP_TELEGRAM_INGRESS_FILE", path)
	if bots, err := LoadBots(); err != nil || len(bots) != 0 {
		t.Fatal("explicit disabled ingress rejected")
	}
	if err := os.WriteFile(path, []byte(`[{"bot_id":"123456","organization_id":"10000000-0000-4000-8000-000000000001"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CP_TELEGRAM_DISPATCH_BOTS_FILE", path)
	if bots, err := LoadDispatchBots(); err != nil || len(bots) != 1 {
		t.Fatal("private dispatcher identities rejected")
	}
}
