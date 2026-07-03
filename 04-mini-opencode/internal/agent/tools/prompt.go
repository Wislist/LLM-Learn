package tools

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed *.md *.md.tpl
var promptFiles embed.FS

type PromptData struct {
	BannedCommands  string
	MaxOutputLength int
	MaxResults      int
	RgAvailable     bool
}

var promptFileByTool = map[string]string{
	BashToolName:      "bash.md.tpl",
	ReadToolName:      "read.md",
	WriteToolName:     "write.md",
	EditToolName:      "edit.md",
	LSToolName:        "ls.md",
	GlobToolName:      "glob.md.tpl",
	GrepToolName:      "grep.md.tpl",
	JobOutputToolName: "job_output.md",
	JobKillToolName:   "job_kill.md",
}

func DefaultPromptData() PromptData {
	_, rgErr := exec.LookPath("rg")
	return PromptData{
		BannedCommands:  strings.Join(DefaultBannedCommands, ", "),
		MaxOutputLength: DefaultMaxOutputLength,
		MaxResults:      200,
		RgAvailable:     rgErr == nil,
	}
}

func RenderToolPrompt(name string, data PromptData) (string, error) {
	path, ok := promptFileByTool[name]
	if !ok {
		return "", fmt.Errorf("unknown tool prompt: %s", name)
	}

	raw, err := promptFiles.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read tool prompt %s: %w", path, err)
	}
	if !strings.HasSuffix(path, ".tpl") {
		return strings.TrimSpace(string(raw)), nil
	}

	tpl, err := template.New(filepath.Base(path)).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse tool prompt %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute tool prompt %s: %w", path, err)
	}
	return strings.TrimSpace(buf.String()), nil
}
