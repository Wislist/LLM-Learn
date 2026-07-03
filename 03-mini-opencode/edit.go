package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wislist/llmg"
)

// ---------- edit (str_replace) ----------

type editTool struct{}

func (t *editTool) Name() string { return "edit" }
func (t *editTool) Description() string {
	return "通过精确查找替换编辑文件。old_string 必须与文件内容完全匹配（含空白和缩进）。默认要求唯一匹配；多处匹配时设置 replace_all=true。先 read_file 再 edit。"
}

func (t *editTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "edit",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path":   map[string]any{"type": "string", "description": "要修改的文件路径"},
					"old_string":  map[string]any{"type": "string", "description": "要替换的文本（必须精确匹配，含空白和缩进）"},
					"new_string":  map[string]any{"type": "string", "description": "替换后的文本"},
					"replace_all": map[string]any{"type": "boolean", "description": "替换所有匹配项（默认 false，仅替换唯一匹配）"},
				},
				"required": []string{"file_path", "old_string", "new_string"},
			},
		},
	}
}

func (t *editTool) Execute(args string) (string, error) {
	var params struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("edit: invalid args: %w", err)
	}

	if params.FilePath == "" {
		return "", fmt.Errorf("edit: file_path is required")
	}
	if params.OldString == params.NewString {
		return "", fmt.Errorf("edit: old_string 和 new_string 相同，无需替换")
	}

	data, err := os.ReadFile(params.FilePath)
	if err != nil {
		return "", fmt.Errorf("edit: %w", err)
	}
	original := string(data)

	count := strings.Count(original, params.OldString)
	if count == 0 {
		return "", fmt.Errorf("edit: old_string 未在文件中找到。%s", hintSnippet(original, params.OldString))
	}
	if count > 1 && !params.ReplaceAll {
		return "", fmt.Errorf("edit: old_string 在文件中出现 %d 次。请提供更多上下文使其唯一匹配，或设置 replace_all=true", count)
	}

	var updated string
	if params.ReplaceAll {
		updated = strings.ReplaceAll(original, params.OldString, params.NewString)
	} else {
		updated = strings.Replace(original, params.OldString, params.NewString, 1)
	}

	if err := os.WriteFile(params.FilePath, []byte(updated), 0644); err != nil {
		return "", fmt.Errorf("edit: %w", err)
	}

	add, rem := diffLineCounts(original, updated)
	out, _ := json.Marshal(map[string]any{
		"success":   true,
		"replaced":  count,
		"additions": add,
		"removals":  rem,
	})
	return string(out), nil
}

// hintSnippet 在 old_string 找不到时，返回文件中最相似的片段，帮 LLM 自纠正。
func hintSnippet(content, old string) string {
	lines := strings.Split(content, "\n")
	firstLine := strings.SplitN(old, "\n", 2)[0]
	for i, line := range lines {
		if strings.Contains(line, strings.TrimSpace(firstLine)) || strings.Contains(firstLine, strings.TrimSpace(line)) {
			start := i - 1
			if start < 0 {
				start = 0
			}
			end := i + 2
			if end > len(lines) {
				end = len(lines)
			}
			return fmt.Sprintf("（文件第 %d 行附近最相似的内容：\n%s）", i+1, strings.Join(lines[start:end], "\n"))
		}
	}
	return "（未找到相似内容，请用 read_file 确认文件当前内容）"
}

// diffLineCounts 粗略统计增删行数。
func diffLineCounts(old, new string) (add, rem int) {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")
	if len(newLines) > len(oldLines) {
		return len(newLines) - len(oldLines), 0
	}
	if len(oldLines) > len(newLines) {
		return 0, len(oldLines) - len(newLines)
	}
	return 0, 0
}
