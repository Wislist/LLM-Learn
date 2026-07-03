package agent

import (
	"context"
	"fmt"
)

type Runtime struct {
	systemPrompt string
	provider     Provider
	tools        *ToolRegistry
	messages     []Message
	maxTurns     int
}

type RuntimeOption func(*Runtime)

func NewRuntime(provider Provider, opts ...RuntimeOption) *Runtime {
	r := &Runtime{
		systemPrompt: defaultSystemPrompt,
		provider:     provider,
		tools:        NewToolRegistry(),
		maxTurns:     8,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func WithSystemPrompt(prompt string) RuntimeOption {
	return func(r *Runtime) {
		if prompt != "" {
			r.systemPrompt = prompt
		}
	}
}

func WithMaxTurns(maxTurns int) RuntimeOption {
	return func(r *Runtime) {
		if maxTurns > 0 {
			r.maxTurns = maxTurns
		}
	}
}

func WithTool(tool Tool) RuntimeOption {
	return func(r *Runtime) {
		_ = r.tools.Register(tool)
	}
}

func (r *Runtime) Messages() []Message {
	out := make([]Message, len(r.messages))
	copy(out, r.messages)
	return out
}

func (r *Runtime) Run(ctx context.Context, input string, emit func(Event)) error {
	if emit == nil {
		emit = func(Event) {}
	}

	r.messages = append(r.messages, Message{Role: RoleUser, Content: input})
	emit(Event{Type: EventRunStarted})

	for turn := 1; turn <= r.maxTurns; turn++ {
		emit(Event{Type: EventTurnStarted, Turn: turn})

		resp, err := r.provider.Complete(ctx, Request{
			SystemPrompt: r.systemPrompt,
			Messages:     r.Messages(),
			Tools:        r.tools.Specs(),
		})
		if err != nil {
			emit(Event{Type: EventRunFailed, Turn: turn, Error: err})
			return err
		}

		msg := Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		r.messages = append(r.messages, msg)
		emit(Event{Type: EventAssistantResponse, Turn: turn, Message: &msg})

		if len(resp.ToolCalls) == 0 {
			emit(Event{Type: EventRunFinished, Turn: turn})
			return nil
		}

		for _, call := range resp.ToolCalls {
			call := call
			emit(Event{Type: EventToolCallStarted, Turn: turn, ToolCall: &call})

			result := r.tools.Run(ctx, call)
			r.messages = append(r.messages, Message{
				Role:       RoleTool,
				ToolCallID: result.ToolCallID,
				Content:    resultMessageContent(result),
			})
			emit(Event{Type: EventToolCallFinished, Turn: turn, ToolCall: &call, ToolResult: &result})
		}
	}

	err := fmt.Errorf("max turns reached: %d", r.maxTurns)
	emit(Event{Type: EventRunFailed, Turn: r.maxTurns, Error: err})
	return err
}

func resultMessageContent(result ToolResult) string {
	if result.Error != "" {
		return `{"error": "` + result.Error + `"}`
	}
	return result.Content
}

const defaultSystemPrompt = `You are mini-opencode, a local coding agent terminal.
Work step by step. Use tools when needed. Keep final answers concise.`
