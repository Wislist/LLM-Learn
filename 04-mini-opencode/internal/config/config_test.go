package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigUsesEchoDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Provider.Name != "echo" {
		t.Fatalf("provider = %q", cfg.Provider.Name)
	}
}

func TestProviderConfigResolvesAPIKeyFromEnv(t *testing.T) {
	t.Setenv("TEST_DEEPSEEK_KEY", "secret")
	cfg := ProviderConfig{APIKeyEnv: "TEST_DEEPSEEK_KEY"}
	if got := cfg.ResolvedAPIKey(); got != "secret" {
		t.Fatalf("ResolvedAPIKey() = %q", got)
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":{"name":"deepseek","model":"deepseek-chat"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Provider.Name != "deepseek" || cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("config = %#v", cfg.Provider)
	}
}
