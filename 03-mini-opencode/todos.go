package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/wislist/llmg"
)

// ---------- todos ----------

type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

type todosTool struct {
	mu    sync.Mutex
	items []todoItem
}

func newTodosTool() *todosTool { return &todosTool{} }

func (t *todosTool) Name() string { return "todos" }
func (t *todosTool) Description() string {
	return "管理结构化任务列表。多步任务用它规划进度。保持恰好一个 in_progress。简单任务跳过。"
}

func (t *todosTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "todos",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":  map[string]any{"type": "string", "description": "操作: list | add | update | clear"},
					"content": map[string]any{"type": "string", "description": "add 时为任务内容"},
					"index":   map[string]any{"type": "integer", "description": "update 时的任务索引（0-based）"},
					"status":  map[string]any{"type": "string", "description": "update 的新状态: pending | in_progress | completed"},
				},
				"required": []string{"action"},
			},
		},
	}
}

func (t *todosTool) Execute(args string) (string, error) {
	var params struct {
		Action  string `json:"action"`
		Content string `json:"content"`
		Index   int    `json:"index"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("todos: invalid args: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch params.Action {
	case "list":
		return t.list()
	case "add":
		if params.Content == "" {
			return "", fmt.Errorf("todos: add 需要 content")
		}
		// 新增任务默认 pending；若当前没有 in_progress，自动设为 in_progress
		status := "pending"
		hasInProgress := false
		for _, it := range t.items {
			if it.Status == "in_progress" {
				hasInProgress = true
				break
			}
		}
		if !hasInProgress && len(t.items) == 0 {
			status = "in_progress"
		}
		t.items = append(t.items, todoItem{Content: params.Content, Status: status})
		return t.list()
	case "update":
		if params.Index < 0 || params.Index >= len(t.items) {
			return "", fmt.Errorf("todos: index 越界（0-%d）", len(t.items)-1)
		}
		if params.Status != "pending" && params.Status != "in_progress" && params.Status != "completed" {
			return "", fmt.Errorf("todos: 非法 status %q", params.Status)
		}
		// in_progress 互斥：设为 in_progress 时，其它 in_progress 降为 pending
		if params.Status == "in_progress" {
			for i := range t.items {
				if t.items[i].Status == "in_progress" {
					t.items[i].Status = "pending"
				}
			}
		}
		t.items[params.Index].Status = params.Status
		// 当前 in_progress 完成后，自动把下一个 pending 提为 in_progress
		if params.Status == "completed" {
			hasInProgress := false
			for _, it := range t.items {
				if it.Status == "in_progress" {
					hasInProgress = true
					break
				}
			}
			if !hasInProgress {
				for i := range t.items {
					if t.items[i].Status == "pending" {
						t.items[i].Status = "in_progress"
						break
					}
				}
			}
		}
		return t.list()
	case "clear":
		t.items = t.items[:0]
		return `{"cleared": true}`, nil
	default:
		return "", fmt.Errorf("todos: 未知 action %q（支持: list/add/update/clear）", params.Action)
	}
}

func (t *todosTool) list() (string, error) {
	out, _ := json.Marshal(map[string]any{
		"todos": t.items,
		"count": len(t.items),
	})
	return string(out), nil
}
