package main

import (
	"fmt"

	"github.com/wislist/llmg"
)

type History struct {
	systemPrompt string
	messages     []llmg.Message
	maxMessages  int

	sessionID string
	store     *SessionStore
}

func NewHistory(systemPrompt string, max int) *History {
	return &History{systemPrompt: systemPrompt, maxMessages: max}
}

func (h *History) BindSession(sessionID string, store *SessionStore) {
	h.sessionID = sessionID
	h.store = store
}

func (h *History) SessionID() string { return h.sessionID }

func (h *History) Add(msg llmg.Message) {
	h.messages = append(h.messages, msg)
	if h.store != nil && h.sessionID != "" {
		seq := len(h.messages)
		if err := h.store.AppendMessage(h.sessionID, seq, msg); err != nil {
			fmt.Printf("\n[warn] persist message: %v\n", err)
		}
	}
	h.truncate()
}

// safeCutForward 返回 >= minCut 的最近 user 消息索引，
// 保证 messages[返回值:] 以 user 开头（回合边界，无孤儿 tool）。
// 找不到 user 时退化为 minCut（最后兜底，至少不会比原来更糟）。
func (h *History) safeCutForward(minCut int) int {
	for i := minCut; i < len(h.messages); i++ {
		if h.messages[i].Role == llmg.RoleUser {
			return i
		}
	}
	return minCut
}

// safeCutBackward 返回 <= maxCut 的最近 user 消息索引，
// 用于 Summarizable：保证 kept 部分 messages[返回值:] 以 user 开头。
// 找不到返回 -1。
func (h *History) safeCutBackward(maxCut int) int {
	for i := maxCut; i > 0; i-- {
		if h.messages[i].Role == llmg.RoleUser {
			return i
		}
	}
	return -1
}

func (h *History) truncate() {
	if len(h.messages) <= h.maxMessages {
		return
	}
	trim := len(h.messages) - h.maxMessages
	// 在 user 边界切，保证 kept 部分不以孤儿 tool 开头
	cut := h.safeCutForward(trim)
	h.messages = h.messages[cut:]
}

func (h *History) LoadSession(sessionID string) error {
	if h.store == nil {
		return fmt.Errorf("no session store bound")
	}
	msgs, err := h.store.LoadMessages(sessionID)
	if err != nil {
		return err
	}
	h.sessionID = sessionID
	h.messages = msgs
	h.truncate()
	return nil
}

func (h *History) Messages() []llmg.Message {
	out := make([]llmg.Message, 0, len(h.messages)+1)
	out = append(out, llmg.Message{Role: llmg.RoleSystem, Content: h.systemPrompt})
	out = append(out, h.messages...)
	return out
}

func (h *History) Clear() { h.messages = h.messages[:0] }

// NeedsSummary 返回是否需要压缩历史（消息数超过 80% 上限）。
func (h *History) NeedsSummary() bool {
	threshold := int(float64(h.maxMessages) * 0.8)
	return len(h.messages) >= threshold
}

// Summarizable 返回可被压缩的旧消息段。
// 切点必须在 user 消息上：保证 kept 部分以 user 开头，
// 摘要部分以完整回合结尾，不会产生孤儿 tool 消息。
func (h *History) Summarizable() []llmg.Message {
	half := len(h.messages) / 2
	if half < 2 {
		return nil
	}
	cut := h.safeCutBackward(half)
	if cut <= 0 {
		return nil // 找不到安全边界，不压缩
	}
	return h.messages[:cut]
}

// ReplaceWithSummary 用摘要消息替换掉旧的头部消息段。
func (h *History) ReplaceWithSummary(summary string, oldCount int) {
	if oldCount > len(h.messages) {
		oldCount = len(h.messages)
	}
	summaryMsg := llmg.Message{
		Role:    llmg.RoleAssistant,
		Content: "[会话摘要] " + summary,
	}
	h.messages = append([]llmg.Message{summaryMsg}, h.messages[oldCount:]...)
}
