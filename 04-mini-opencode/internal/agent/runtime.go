package agent

import (
	"context"
	"fmt"
)

type Runtime struct {
	systemPrompt string
	provider     Provider
	tools        *ToolRegistry
	toolService  ToolService
	messages     []Message
	maxTurns     int
}

type RuntimeOption func(*Runtime)

func NewRuntime(provider Provider, opts ...RuntimeOption) *Runtime {
	tools := NewToolRegistry()
	r := &Runtime{
		systemPrompt: defaultSystemPrompt,
		provider:     provider,
		tools:        tools,
		toolService:  NewRegistryToolService(tools),
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

func WithToolService(service ToolService) RuntimeOption {
	return func(r *Runtime) {
		if service != nil {
			r.toolService = service
		}
	}
}

func WithPermissionPolicy(policy PermissionPolicy) RuntimeOption {
	return func(r *Runtime) {
		r.tools.SetPermissionPolicy(policy)
	}
}

func (r *Runtime) Messages() []Message {
	out := make([]Message, len(r.messages))
	copy(out, r.messages)
	return out
}

func (r *Runtime) Tools() []ToolView {
	return r.toolService.ListTools()
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
			if err := r.runTool(ctx, turn, call, emit); err != nil {
				emit(Event{Type: EventRunFailed, Turn: turn, ToolCall: &call, Error: err})
				return err
			}
		}
	}

	err := fmt.Errorf("max turns reached: %d", r.maxTurns)
	emit(Event{Type: EventRunFailed, Turn: r.maxTurns, Error: err})
	return err
}

func (r *Runtime) runTool(ctx context.Context, turn int, call ToolCall, emit func(Event)) error {
	events, err := r.toolService.RunTool(ctx, ToolRunRequest{Call: call})
	if err != nil {
		return err
	}
	for toolEvent := range events {
		switch toolEvent.Type {
		case ToolEventStarted:
			emit(Event{Type: EventToolCallStarted, Turn: turn, ToolCall: &call})
		case ToolEventFinished:
			r.recordToolResult(toolEvent.Result)
			emit(Event{Type: EventToolCallFinished, Turn: turn, ToolCall: &call, ToolResult: toolEvent.Result})
		case ToolEventFailed:
			r.recordToolResult(toolEvent.Result)
			emit(Event{Type: EventToolCallFailed, Turn: turn, ToolCall: &call, ToolResult: toolEvent.Result, Error: toolEvent.Error})
		case ToolEventPermissionRequired:
			r.recordToolResult(toolEvent.Result)
			emit(Event{Type: EventToolPermissionRequired, Turn: turn, ToolCall: &call, ToolResult: toolEvent.Result, Error: toolEvent.Error})
		case ToolEventPermissionDenied:
			r.recordToolResult(toolEvent.Result)
			emit(Event{Type: EventToolPermissionDenied, Turn: turn, ToolCall: &call, ToolResult: toolEvent.Result, Error: toolEvent.Error})
		}
	}
	return nil
}

func (r *Runtime) recordToolResult(result *ToolResult) {
	if result == nil {
		return
	}
	r.messages = append(r.messages, Message{
		Role:       RoleTool,
		ToolCallID: result.ToolCallID,
		Content:    resultMessageContent(*result),
	})
}

func resultMessageContent(result ToolResult) string {
	if result.Error != "" {
		return `{"error": "` + result.Error + `"}`
	}
	return result.Content
}

const defaultSystemPrompt = `You are mini-opencode, a local coding agent terminal.
Work step by step. Use tools when needed. Keep final answers concise.`
