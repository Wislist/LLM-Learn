package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wislist/llmg"
)

// ---------- ls ----------

type lsTool struct{}

func (t *lsTool) Name() string { return "ls" }
func (t *lsTool) Description() string {
	return "列出目录内容。返回文件/目录名、类型、大小。支持递归深度。"
}

func (t *lsTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "ls",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "目录路径（默认当前目录）"},
					"depth": map[string]any{"type": "integer", "description": "递归深度（默认 1，只列当前层）"},
				},
			},
		},
	}
}

type lsEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`  // "file" | "dir"
	Size  int64  `json:"size"`  // 文件字节数
	Depth int    `json:"depth"` // 相对深度
}

func (t *lsTool) Execute(args string) (string, error) {
	var params struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("ls: invalid args: %w", err)
	}
	if params.Path == "" {
		params.Path = "."
	}
	if params.Depth <= 0 {
		params.Depth = 1
	}
	if params.Depth > 10 {
		params.Depth = 10
	}

	info, err := os.Stat(params.Path)
	if err != nil {
		return "", fmt.Errorf("ls: %w", err)
	}
	if !info.IsDir() {
		// 单个文件，直接返回
		out, _ := json.Marshal(map[string]any{
			"entries": []lsEntry{{Name: params.Path, Type: "file", Size: info.Size(), Depth: 0}},
			"count":   1,
		})
		return string(out), nil
	}

	var entries []lsEntry
	baseDepth := strings.Count(strings.TrimSuffix(filepath.ToSlash(params.Path), "/"), "/")

	walkErr := filepath.Walk(params.Path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if p == params.Path {
			return nil
		}
		relDepth := strings.Count(filepath.ToSlash(p), "/") - baseDepth
		if relDepth >= params.Depth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		// 跳过隐藏文件和常见噪音目录
		if strings.HasPrefix(name, ".") && name != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entryType := "file"
		if info.IsDir() {
			entryType = "dir"
		}
		entries = append(entries, lsEntry{
			Name:  p,
			Type:  entryType,
			Size:  info.Size(),
			Depth: relDepth,
		})
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("ls: %w", walkErr)
	}

	// 目录优先，再按名字排序
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "dir"
		}
		return entries[i].Name < entries[j].Name
	})

	if len(entries) > 500 {
		entries = entries[:500]
	}
	out, _ := json.Marshal(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
	return string(out), nil
}
