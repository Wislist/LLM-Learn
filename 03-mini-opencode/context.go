package main

import (
	"encoding/json"
	"unicode"

	"github.com/wislist/llmg"
)

func estimateTextTokens(text string) int {
	ascii := 0
	nonASCII := 0
	for _, r := range text {
		if r <= unicode.MaxASCII {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII + 4
}

func estimateMessageTokens(msg llmg.Message) int {
	tokens := estimateTextTokens(msg.Content) + 6
	for _, tc := range msg.ToolCalls {
		tokens += estimateTextTokens(tc.Function.Name)
		tokens += estimateTextTokens(tc.Function.Arguments)
		tokens += 8
	}
	if msg.ToolCallID != "" {
		tokens += estimateTextTokens(msg.ToolCallID) + 4
	}
	return tokens
}

func estimateMessagesTokens(messages []llmg.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
}

func estimateToolsTokens(tools []llmg.Tool) int {
	raw, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return estimateTextTokens(string(raw))
}
