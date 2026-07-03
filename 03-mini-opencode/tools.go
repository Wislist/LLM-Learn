package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wislist/llmg"
)

type Tool interface {
	Name() string
	Description() string
	Schema() llmg.Tool
	Execute(args string) (string, error)
}

type ToolRegistry struct {
	tools map[string]Tool
	order []string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]Tool{}}
}

func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Name()] = t
	r.order = append(r.order, t.Name())
}

func (r *ToolRegistry) ToLLMTools() []llmg.Tool {
	out := make([]llmg.Tool, len(r.order))
	for i, name := range r.order {
		out[i] = r.tools[name].Schema()
	}
	return out
}

func (r *ToolRegistry) Execute(name, args string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(args)
}

func (r *ToolRegistry) ToolPrompt() string {
	var sb strings.Builder
	for _, name := range r.order {
		t := r.tools[name]
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name(), t.Description())
	}
	return sb.String()
}

// ---------- read_file ----------

type readFileTool struct{}

func (t *readFileTool) Name() string { return "read_file" }
func (t *readFileTool) Description() string {
	return "读取文件内容（带行号）。支持 offset/limit 只读片段，避免大文件撑爆上下文。"
}

func (t *readFileTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "read_file",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "文件路径"},
					"offset": map[string]any{"type": "integer", "description": "起始行号（1-based，默认 1）"},
					"limit":  map[string]any{"type": "integer", "description": "最多返回的行数（默认 2000）"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t *readFileTool) Execute(args string) (string, error) {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("read_file: invalid args: %w", err)
	}
	if params.Path == "" {
		return "", fmt.Errorf("read_file: path is required")
	}
	if params.Offset < 1 {
		params.Offset = 1
	}
	if params.Limit <= 0 {
		params.Limit = 2000
	}
	if params.Limit > 2000 {
		params.Limit = 2000
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	total := len(lines)

	start := params.Offset - 1
	if start > total {
		start = total
	}
	end := start + params.Limit
	if end > total {
		end = total
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&sb, "%4d| %s\n", i+1, lines[i])
	}
	out, _ := json.Marshal(map[string]any{
		"content":     sb.String(),
		"lines":       end - start,
		"total_lines": total,
		"offset":      start + 1,
	})
	return string(out), nil
}

// ---------- write_file ----------

type writeFileTool struct{}

func (t *writeFileTool) Name() string { return "write_file" }
func (t *writeFileTool) Description() string {
	return "写入文件内容。目录不存在时自动创建。"
}

func (t *writeFileTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "write_file",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "文件路径"},
					"content": map[string]any{"type": "string", "description": "文件内容"},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func (t *writeFileTool) Execute(args string) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("write_file: invalid args: %w", err)
	}
	if dir := pathDir(params.Path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("write_file: %w", err)
		}
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return `{"success": true}`, nil
}

func pathDir(p string) string {
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[:idx]
	}
	return "."
}
