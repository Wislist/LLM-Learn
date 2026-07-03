package tools

import (
	"strings"
	"testing"
)

func TestRenderToolPromptInjectsTemplateData(t *testing.T) {
	prompt, err := RenderToolPrompt(BashToolName, PromptData{
		BannedCommands:  "rm -rf /, git push",
		MaxOutputLength: 1234,
		MaxResults:      50,
		RgAvailable:     true,
	})
	if err != nil {
		t.Fatalf("RenderToolPrompt() error = %v", err)
	}

	for _, want := range []string{
		"rm -rf /, git push",
		"1234",
		"Ripgrep (`rg`) is available",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestRenderToolPromptReadsStaticMarkdown(t *testing.T) {
	prompt, err := RenderToolPrompt(EditToolName, PromptData{})
	if err != nil {
		t.Fatalf("RenderToolPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Edit an existing file") {
		t.Fatalf("unexpected edit prompt: %q", prompt)
	}
}

func TestRenderToolPromptRejectsUnknownTool(t *testing.T) {
	_, err := RenderToolPrompt("missing", PromptData{})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
