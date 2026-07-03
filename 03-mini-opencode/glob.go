package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wislist/llmg"
)

// ---------- glob ----------

type globTool struct{}

func (t *globTool) Name() string { return "glob" }
func (t *globTool) Description() string {
	return "按文件名模式搜索文件路径。支持 ** 递归。返回按修改时间倒序的路径列表。"
}

func (t *globTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "glob",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "文件名匹配模式，如 **/*.go"},
					"path":    map[string]any{"type": "string", "description": "搜索根目录（默认当前目录）"},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

func (t *globTool) Execute(args string) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("glob: invalid args: %w", err)
	}
	if params.Pattern == "" {
		return "", fmt.Errorf("glob: pattern is required")
	}
	if params.Path == "" {
		params.Path = "."
	}

	// 把 **/*.go 拆成可走 filepath.Walk 的形式
	// 这里用简化方案：遍历所有文件，用 matchGlob 匹配
	type finfo struct {
		path    string
		modTime int64
	}
	var matches []finfo

	walkErr := filepath.Walk(params.Path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(params.Path, p)
		if matchGlob(params.Pattern, rel) {
			matches = append(matches, finfo{path: p, modTime: info.ModTime().UnixNano()})
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("glob: %w", walkErr)
	}

	// 按修改时间倒序（最新在前）
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].modTime > matches[i].modTime {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// 限制返回数量
	limit := 200
	if len(matches) > limit {
		matches = matches[:limit]
	}
	paths := make([]string, len(matches))
	for i, m := range matches {
		paths[i] = m.path
	}
	out, _ := json.Marshal(map[string]any{
		"matches": paths,
		"count":   len(paths),
	})
	return string(out), nil
}

// matchGlob 支持 ** 的简化 glob 匹配。
// **/*.go  → 任意层级目录下的 .go 文件
// *.go     → 当前层 .go 文件
func matchGlob(pattern, name string) bool {
	// 把分隔符统一成 /
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)

	// 处理 ** 前缀：**/*.go 匹配 a/b/c.go 也匹配 c.go
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		if simpleGlob(suffix, name) {
			return true
		}
		// 匹配任意子目录下的 suffix
		parts := strings.Split(name, "/")
		for i := 0; i < len(parts); i++ {
			if simpleGlob(suffix, strings.Join(parts[i:], "/")) {
				return true
			}
		}
		return false
	}
	// 处理中间的 **：a/**/b.go
	if strings.Contains(pattern, "**") {
		seg := strings.Split(pattern, "**")
		if len(seg) == 2 {
			prefix := strings.TrimSuffix(seg[0], "/")
			suffix := strings.TrimPrefix(seg[1], "/")
			if prefix != "" && !strings.HasPrefix(name, prefix+"/") && name != prefix {
				return false
			}
			rest := name
			if prefix != "" {
				rest = strings.TrimPrefix(name, prefix+"/")
			}
			parts := strings.Split(rest, "/")
			for i := 0; i < len(parts); i++ {
				if simpleGlob(suffix, strings.Join(parts[i:], "/")) {
					return true
				}
			}
			return false
		}
	}
	return simpleGlob(pattern, name)
}

// simpleGlob 是 filepath.Match 的包装，处理 / 分隔。
func simpleGlob(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}
