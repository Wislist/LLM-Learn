package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wislist/llmg"
)

// rememberTool 让 LLM 主动保存长期记忆。
type rememberTool struct {
	store *MemoryStore
}

func (t *rememberTool) Name() string { return "remember" }

func (t *rememberTool) Description() string {
	return "保存一条长期记忆。用于跨会话保留用户偏好、项目事实、技术决策或约束。"
}

func (t *rememberTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "remember",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
						"description": "记忆类型",
						"enum": []string{"preference", "project", "decision", "constraint", "task"},
					},
					"content":   map[string]any{"type": "string", "description": "记忆内容（简洁、可跨会话复用）"},
					"scope":     map[string]any{"type": "string", "description": "记忆范围，默认 global"},
				},
				"required": []string{"kind", "content"},
			},
		},
	}
}

func (t *rememberTool) Execute(args string) (string, error) {
	var p struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("remember: invalid args: %w", err)
	}
	if p.Content == "" {
		return "", fmt.Errorf("remember: content is required")
	}
	if p.Scope == "" {
		p.Scope = "global"
	}
	if t.store == nil {
		return "", fmt.Errorf("remember: memory store unavailable")
	}
	if err := t.store.Save(p.Scope, MemoryKind(p.Kind), p.Content, ""); err != nil {
		return "", fmt.Errorf("remember: %w", err)
	}
	return fmt.Sprintf(`{"saved": true, "kind": "%s", "scope": "%s"}`, p.Kind, p.Scope), nil
}

// forgetTool 删除记忆。
type forgetTool struct {
	store *MemoryStore
}

func (t *forgetTool) Name() string { return "forget" }

func (t *forgetTool) Description() string {
	return "根据 ID 删除一条长期记忆。"
}

func (t *forgetTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "forget",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer", "description": "记忆 ID"},
				},
				"required": []string{"id"},
			},
		},
	}
}

func (t *forgetTool) Execute(args string) (string, error) {
	var p struct{ ID int64 }
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("forget: invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("forget: memory store unavailable")
	}
	if err := t.store.Delete(p.ID); err != nil {
		return "", fmt.Errorf("forget: %w", err)
	}
	return `{"deleted": true}`, nil
}

// listMemoryTool 列出记忆。
type listMemoryTool struct {
	store *MemoryStore
}

func (t *listMemoryTool) Name() string { return "list_memory" }

func (t *listMemoryTool) Description() string {
	return "列出长期记忆条目，可选按类型过滤。"
}

func (t *listMemoryTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "list_memory",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
						"description": "按类型过滤（可选）",
						"enum": []string{"preference", "project", "decision", "constraint", "task"},
					},
					"limit": map[string]any{"type": "integer", "description": "返回条数（默认 20）"},
				},
			},
		},
	}
}

func (t *listMemoryTool) Execute(args string) (string, error) {
	var p struct {
		Kind  string `json:"kind"`
		Limit int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if t.store == nil {
		return "", fmt.Errorf("list_memory: memory store unavailable")
	}
	memories, err := t.store.ListAll(p.Limit)
	if err != nil {
		return "", fmt.Errorf("list_memory: %w", err)
	}
	if p.Kind != "" {
		var filtered []Memory
		for _, m := range memories {
			if string(m.Kind) == p.Kind {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
	}
	if len(memories) == 0 {
		return `{"memories": [], "count": 0}`, nil
	}
	var sb strings.Builder
	sb.WriteString(`{"memories": [`)
	for i, m := range memories {
		if i > 0 {
			sb.WriteString(",")
		}
		entry, _ := json.Marshal(map[string]any{
			"id":      m.ID,
			"kind":    string(m.Kind),
			"scope":   m.Scope,
			"content": m.Content,
		})
		sb.Write(entry)
	}
	sb.WriteString(`], "count": `)
	fmt.Fprintf(&sb, "%d}", len(memories))
	return sb.String(), nil
}
