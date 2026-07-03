package main

import (
	"testing"

	"github.com/wislist/llmg"
)

// 构造一条带多个 tool_calls 的 assistant 消息 + 多条 tool 结果。
func multiToolTurn() []llmg.Message {
	tc := func(name string) llmg.ToolCall {
		return llmg.ToolCall{ID: name, Type: "function", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: "{}"}}
	}
	assistantWithCalls := llmg.Message{
		Role:      llmg.RoleAssistant,
		Content:   "",
		ToolCalls: []llmg.ToolCall{tc("a"), tc("b")},
	}
	return []llmg.Message{
		{Role: llmg.RoleUser, Content: "do a and b"},
		assistantWithCalls,
		{Role: llmg.RoleTool, ToolCallID: "a", Content: `{"ok":true}`},
		{Role: llmg.RoleTool, ToolCallID: "b", Content: `{"ok":true}`},
		{Role: llmg.RoleUser, Content: "next"},
		assistantWithCalls,
		{Role: llmg.RoleTool, ToolCallID: "a", Content: `{"ok":true}`},
		{Role: llmg.RoleTool, ToolCallID: "b", Content: `{"ok":true}`},
		{Role: llmg.RoleUser, Content: "done"},
	}
}

// 验证 Summarizable 的切点落在 user 消息上，不在两条 tool 结果之间。
func TestSummarizable_CutsAtUserBoundary(t *testing.T) {
	msgs := multiToolTurn()
	// 模拟 9 条消息，half=4，切点应在 index 4（"next" user 消息）
	h := &History{messages: msgs, maxMessages: 100}
	old := h.Summarizable()
	if old == nil {
		t.Fatal("expected non-nil summarizable")
	}
	// kept 部分 = messages[len(old):]，应从 user 开头
	kept := msgs[len(old):]
	if kept[0].Role != llmg.RoleUser {
		t.Errorf("kept should start with user, got %s", kept[0].Role)
	}
	// 验证没有孤儿 tool：每条 tool 前面必有 assistant+tool_calls
	for i, m := range kept {
		if m.Role == llmg.RoleTool {
			if i == 0 {
				t.Error("kept starts with orphan tool")
				break
			}
			prev := kept[i-1]
			if prev.Role != llmg.RoleAssistant || len(prev.ToolCalls) == 0 {
				// 可能是同组前一条 tool 结果，往前找到 assistant
				found := false
				for j := i - 1; j >= 0; j-- {
					if kept[j].Role == llmg.RoleAssistant && len(kept[j].ToolCalls) > 0 {
						found = true
						break
					}
					if kept[j].Role == llmg.RoleUser {
						break
					}
				}
				if !found {
					t.Errorf("orphan tool at kept[%d] with no preceding tool_calls", i)
				}
			}
		}
	}
}

// 验证 truncate 在 user 边界切，不产生孤儿 tool。
func TestTruncate_NoOrphanTool(t *testing.T) {
	msgs := multiToolTurn() // 9 条
	// maxMessages=5，需要截掉 4 条。trim=4，但 msgs[4] 是 user，安全切点=4。
	h := &History{messages: msgs, maxMessages: 5}
	h.truncate()
	if len(h.messages) > 5 {
		t.Errorf("len after truncate: %d, want <= 5", len(h.messages))
	}
	if h.messages[0].Role != llmg.RoleUser {
		t.Errorf("after truncate, first should be user, got %s", h.messages[0].Role)
	}
	// 检查无孤儿 tool
	for i, m := range h.messages {
		if m.Role == llmg.RoleTool && i == 0 {
			t.Error("truncated history starts with orphan tool")
		}
	}
}

// 验证极端场景：单回合无 user 边界时不压缩（返回 nil）。
func TestSummarizable_NoUserBoundary(t *testing.T) {
	// 一长串没有中间 user 的消息
	msgs := []llmg.Message{
		{Role: llmg.RoleUser, Content: "go"},
		{Role: llmg.RoleAssistant, Content: "ok", ToolCalls: []llmg.ToolCall{{}}},
		{Role: llmg.RoleTool, Content: "r1"},
		{Role: llmg.RoleAssistant, Content: "ok2", ToolCalls: []llmg.ToolCall{{}}},
		{Role: llmg.RoleTool, Content: "r2"},
		{Role: llmg.RoleAssistant, Content: "done"},
	}
	h := &History{messages: msgs, maxMessages: 100}
	old := h.Summarizable()
	// half=3，往前找 user：只有 index 0，cut=0 → 返回 nil
	if old != nil {
		t.Errorf("expected nil when no mid user boundary, got len=%d", len(old))
	}
}
