package main

import (
	"testing"

	"github.com/wislist/llmg"
)

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

func TestSummarizable_CutsAtUserBoundary(t *testing.T) {
	msgs := multiToolTurn()
	h := &History{messages: msgs, maxMessages: 100}
	old := h.Summarizable()
	if old == nil {
		t.Fatal("expected non-nil summarizable")
	}
	kept := msgs[len(old):]
	if kept[0].Role != llmg.RoleUser {
		t.Errorf("kept should start with user, got %s", kept[0].Role)
	}
	for i, m := range kept {
		if m.Role == llmg.RoleTool {
			if i == 0 {
				t.Error("kept starts with orphan tool")
				break
			}
			prev := kept[i-1]
			if prev.Role != llmg.RoleAssistant || len(prev.ToolCalls) == 0 {
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

func TestTruncate_NoOrphanTool(t *testing.T) {
	msgs := multiToolTurn()
	h := &History{messages: msgs, maxMessages: 5}
	h.truncate()
	if len(h.messages) > 5 {
		t.Errorf("len after truncate: %d, want <= 5", len(h.messages))
	}
	if h.messages[0].Role != llmg.RoleUser {
		t.Errorf("after truncate, first should be user, got %s", h.messages[0].Role)
	}
	for i, m := range h.messages {
		if m.Role == llmg.RoleTool && i == 0 {
			t.Error("truncated history starts with orphan tool")
		}
	}
}

func TestSummarizable_NoUserBoundary(t *testing.T) {
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
	if old != nil {
		t.Errorf("expected nil when no mid user boundary, got len=%d", len(old))
	}
}

// ---------- 新增测试 ----------

func TestEstimateTextTokens(t *testing.T) {
	// 纯 ASCII：约 4 字符/token。
	ascii := estimateTextTokens("hello world")
	if ascii < 4 || ascii > 12 {
		t.Errorf("ASCII token estimate off: got %d for 'hello world'", ascii)
	}

	// 纯中文：每字符约 1 token + 常数。
	chinese := estimateTextTokens("你好世界")
	if chinese < 4 || chinese > 10 {
		t.Errorf("Chinese token estimate off: got %d for '你好世界'", chinese)
	}

	// 空字符串仍应有常数项。
	empty := estimateTextTokens("")
	if empty < 3 || empty > 6 {
		t.Errorf("empty string token estimate off: got %d", empty)
	}
}

func TestNeedsSummary_TokenBudget(t *testing.T) {
	// 少量消息不应触发。
	h := &History{
		messages:    []llmg.Message{{Role: llmg.RoleUser, Content: "hi"}},
		maxMessages: 200,
	}
	if h.NeedsSummary(1000) {
		t.Error("should not need summary with 1 message")
	}

	// 消息数达到硬上限 80% 应触发。
	h.maxMessages = 10
	for i := 0; i < 8; i++ {
		h.messages = append(h.messages, llmg.Message{Role: llmg.RoleUser, Content: "x"})
	}
	if !h.NeedsSummary(999999) {
		t.Error("should trigger on message count threshold")
	}
}

func TestReplaceWithSummary_ReturnsCoveredID(t *testing.T) {
	h := &History{
		messages: []llmg.Message{
			{Role: llmg.RoleUser, Content: "q1"},
			{Role: llmg.RoleAssistant, Content: "a1"},
			{Role: llmg.RoleUser, Content: "q2"},
			{Role: llmg.RoleAssistant, Content: "a2"},
		},
		messageIDs: []int64{10, 20, 30, 40},
		maxMessages: 100,
	}
	coveredID := h.ReplaceWithSummary("summary text", 2)
	if coveredID != 20 {
		t.Errorf("coveredID = %d, want 20", coveredID)
	}
	if h.persistedSummary != "summary text" {
		t.Errorf("persistedSummary = %q, want 'summary text'", h.persistedSummary)
	}
	if len(h.messages) != 3 { // 1 summary + 2 remaining
		t.Errorf("messages len = %d, want 3", len(h.messages))
	}
	if h.messages[0].Content != "[会话摘要] summary text" {
		t.Errorf("first message = %q, want summary", h.messages[0].Content)
	}
	if len(h.messageIDs) != 2 {
		t.Errorf("messageIDs len = %d, want 2", len(h.messageIDs))
	}
}

func TestMessages_IncludesPersistedSummary(t *testing.T) {
	h := &History{
		systemPrompt:      "system",
		messages:          []llmg.Message{{Role: llmg.RoleUser, Content: "hi"}},
		persistedSummary:  "past context",
		maxMessages:       100,
	}
	msgs := h.Messages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (system+summary+user), got %d", len(msgs))
	}
	if msgs[0].Role != llmg.RoleSystem {
		t.Errorf("msg[0] role = %s, want system", msgs[0].Role)
	}
	if msgs[1].Content != "[会话摘要] past context" {
		t.Errorf("msg[1] = %q, want summary", msgs[1].Content)
	}
	if msgs[2].Role != llmg.RoleUser {
		t.Errorf("msg[2] role = %s, want user", msgs[2].Role)
	}
}
