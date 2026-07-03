package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wislist/llmg"
)

// ---------- grep ----------

type grepTool struct{}

func (t *grepTool) Name() string { return "grep" }
func (t *grepTool) Description() string {
	return "按正则搜索文件内容。返回 file:line:匹配行 列表。默认递归当前目录。"
}

func (t *grepTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "grep",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":     map[string]any{"type": "string", "description": "正则表达式"},
					"path":        map[string]any{"type": "string", "description": "搜索根目录（默认当前目录）"},
					"include":     map[string]any{"type": "string", "description": "文件名过滤，如 *.go"},
					"max_results": map[string]any{"type": "integer", "description": "最多返回结果数（默认 50）"},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

type grepHit struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func (t *grepTool) Execute(args string) (string, error) {
	var params struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Include    string `json:"include"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("grep: invalid args: %w", err)
	}
	if params.Pattern == "" {
		return "", fmt.Errorf("grep: pattern is required")
	}
	if params.Path == "" {
		params.Path = "."
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 50
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid regex: %w", err)
	}

	var hits []grepHit
	truncated := false

	walkErr := filepath.Walk(params.Path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// 跳过 .git 等
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".mini-opencode" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(hits) >= params.MaxResults {
			truncated = true
			return filepath.SkipAll
		}
		// 文件名过滤
		if params.Include != "" {
			ok, _ := filepath.Match(params.Include, info.Name())
			if !ok {
				return nil
			}
		}
		// 跳过二进制/超大文件
		if info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if len(hits) >= params.MaxResults {
				truncated = true
				break
			}
			if re.MatchString(line) {
				trimmed := line
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				hits = append(hits, grepHit{File: p, Line: i + 1, Content: trimmed})
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("grep: %w", walkErr)
	}

	out, _ := json.Marshal(map[string]any{
		"hits":      hits,
		"count":     len(hits),
		"truncated": truncated,
	})
	return string(out), nil
}
