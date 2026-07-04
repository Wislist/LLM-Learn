package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersionThenQuit(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("/version\n/quit\n")

	if err := Run(context.Background(), in, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "mini-opencode 0.1.0") {
		t.Fatalf("output missing version: %q", got)
	}
}

func TestRunKeyCommandSavesLocalDeepSeekConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var out bytes.Buffer
	in := strings.NewReader("/key test-key\n/version\n/quit\n")

	if err := Run(context.Background(), in, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "[deepseek key saved]") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("config.json not saved: %v", err)
	}
	secretPath := filepath.Join(dir, ".mini-opencode", "secrets.json")
	data, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatalf("secrets not saved: %v", err)
	}
	if !strings.Contains(string(data), "test-key") {
		t.Fatalf("secret store missing key: %q", data)
	}
}
