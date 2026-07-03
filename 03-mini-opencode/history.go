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

func (h *History) truncate() {
	if len(h.messages) <= h.maxMessages {
		return
	}
	trim := len(h.messages) - h.maxMessages
	for trim < len(h.messages) {
		m := h.messages[trim]
		if m.Role == llmg.RoleTool && trim > 0 {
			prev := h.messages[trim-1]
			if len(prev.ToolCalls) == 0 {
				break
			}
		}
		if m.Role == llmg.RoleAssistant && len(m.ToolCalls) > 0 && trim+1 < len(h.messages) {
			if h.messages[trim+1].Role != llmg.RoleTool {
				break
			}
		}
		h.messages = h.messages[trim:]
		return
	}
	if trim%2 != 0 {
		trim++
	}
	h.messages = h.messages[trim:]
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
