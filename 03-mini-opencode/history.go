package main

import (
	"fmt"

	"github.com/wislist/llmg"
)

type History struct {
	systemPrompt string
	messages     []llmg.Message
	maxMessages  int

	sessionID        string
	store            *SessionStore
	messageIDs       []int64
	persistedSummary string
}

func NewHistory(systemPrompt string, max int) *History {
	return &History{
		systemPrompt: systemPrompt,
		maxMessages:  max,
	}
}

func (h *History) BindSession(sessionID string, store *SessionStore) {
	h.sessionID = sessionID
	h.store = store
}

func (h *History) SessionID() string { return h.sessionID }

func (h *History) Add(msg llmg.Message) {
	h.messages = append(h.messages, msg)
	h.messageIDs = append(h.messageIDs, 0)
	if h.store != nil && h.sessionID != "" {
		id, err := h.store.AppendMessage(h.sessionID, msg)
		if err != nil {
			fmt.Printf("\n[warn] persist message: %v\n", err)
		} else if len(h.messageIDs) > 0 {
			h.messageIDs[len(h.messageIDs)-1] = id
		}
	}
	h.truncate()
}

// safeCutForward 返回 >= minCut 的最近 user 消息索引。
func (h *History) safeCutForward(minCut int) int {
	for i := minCut; i < len(h.messages); i++ {
		if h.messages[i].Role == llmg.RoleUser {
			return i
		}
	}
	return minCut
}

// safeCutBackward 返回 <= maxCut 的最近 user 消息索引。
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
	cut := h.safeCutForward(trim)
	h.messages = h.messages[cut:]
	if len(h.messageIDs) >= cut {
		h.messageIDs = h.messageIDs[cut:]
	}
}

func (h *History) LoadSession(sessionID string) error {
	if h.store == nil {
		return fmt.Errorf("no session store bound")
	}
	h.sessionID = sessionID
	return h.LoadContext()
}

// LoadContext 从数据库加载摘要 + 摘要之后的新消息。
func (h *History) LoadContext() error {
	if h.store == nil {
		return fmt.Errorf("no session store bound")
	}
	summary, msgs, ids, err := h.store.LoadContext(h.sessionID)
	if err != nil {
		return err
	}
	h.messages = msgs
	h.messageIDs = ids
	h.persistedSummary = summary
	h.truncate()
	return nil
}

func (h *History) Messages() []llmg.Message {
	out := make([]llmg.Message, 0, len(h.messages)+2)
	out = append(out, llmg.Message{Role: llmg.RoleSystem, Content: h.systemPrompt})
	if h.persistedSummary != "" {
		out = append(out, llmg.Message{
			Role:    llmg.RoleAssistant,
			Content: "[会话摘要] " + h.persistedSummary,
		})
	}
	out = append(out, h.messages...)
	return out
}

func (h *History) Clear() {
	h.messages = h.messages[:0]
	h.messageIDs = h.messageIDs[:0]
	h.persistedSummary = ""
}

// EstimatedTokens 返回当前历史（不含 system prompt）的估算 token 数。
func (h *History) EstimatedTokens() int {
	return estimateMessagesTokens(h.messages)
}

// NeedsSummary 返回是否需要压缩历史。
// 触发条件：消息数达到硬上限的 80%，或 token 估算达到预算。
func (h *History) NeedsSummary(tokenBudget int) bool {
	threshold := int(float64(h.maxMessages) * 0.8)
	if len(h.messages) >= threshold {
		return true
	}
	if len(h.messages) < 6 {
		return false
	}
	return estimateMessagesTokens(h.messages) >= tokenBudget
}

// Summarizable 返回可被压缩的旧消息段。
// 切点必须在 user 消息上，保证 kept 部分以 user 开头。
func (h *History) Summarizable() []llmg.Message {
	half := len(h.messages) / 2
	if half < 2 {
		return nil
	}
	cut := h.safeCutBackward(half)
	if cut <= 0 {
		return nil
	}
	return h.messages[:cut]
}

// SummarizableIDs 返回与 Summarizable 对应的消息数据库 ID。
func (h *History) SummarizableIDs() []int64 {
	half := len(h.messages) / 2
	if half < 2 {
		return nil
	}
	cut := h.safeCutBackward(half)
	if cut <= 0 {
		return nil
	}
	if cut > len(h.messageIDs) {
		cut = len(h.messageIDs)
	}
	return h.messageIDs[:cut]
}

// ReplaceWithSummary 用摘要消息替换掉旧的头部消息段。
// 返回被覆盖的最后一条消息的数据库 ID（用于持久化摘要检查点）。
func (h *History) ReplaceWithSummary(summary string, oldCount int) int64 {
	if oldCount > len(h.messages) {
		oldCount = len(h.messages)
	}
	var coveredID int64
	if oldCount > 0 && oldCount <= len(h.messageIDs) {
		coveredID = h.messageIDs[oldCount-1]
	}
	summaryMsg := llmg.Message{
		Role:    llmg.RoleAssistant,
		Content: "[会话摘要] " + summary,
	}
	h.persistedSummary = summary
	if oldCount <= len(h.messageIDs) {
		h.messageIDs = h.messageIDs[oldCount:]
	}
	h.messages = append([]llmg.Message{summaryMsg}, h.messages[oldCount:]...)
	return coveredID
}
