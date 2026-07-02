package llmg

import (
	"context"
	"fmt"
)

// ToolExecutor defines a set of tools and their execution logic.
// An Agent uses this interface to both advertise tools to the LLM
// and invoke them when the LLM requests a tool call.
type ToolExecutor interface {
	Tools() []Tool
	Execute(toolCall ToolCall) (string, error)
}

// MultiExecutor chains multiple ToolExecutors, merging their tool definitions
// and dispatching execution by tool name across the chain.
type MultiExecutor struct {
	executors []ToolExecutor
}

func NewMultiExecutor(execs ...ToolExecutor) *MultiExecutor {
	return &MultiExecutor{executors: execs}
}

func (m *MultiExecutor) Tools() []Tool {
	var tools []Tool
	for _, e := range m.executors {
		tools = append(tools, e.Tools()...)
	}
	return tools
}

func (m *MultiExecutor) Execute(tc ToolCall) (string, error) {
	for _, e := range m.executors {
		for _, t := range e.Tools() {
			if t.Function.Name == tc.Function.Name {
				return e.Execute(tc)
			}
		}
	}
	return "", fmt.Errorf("unknown tool: %q", tc.Function.Name)
}

// Agent wraps a Client with a persistent message history and automatic
// tool-calling loop. Each call to Send or SendStream appends the user
// input, then iterates: chat → tool calls → tool results → chat again
// until the LLM stops requesting tools or maxTurns is exhausted.
type Agent struct {
	client   *Client
	executor ToolExecutor
	messages []Message
	maxTurns int
}

// AgentConfig configures a new Agent.
type AgentConfig struct {
	Client   *Client
	Executor  ToolExecutor
	System   string
	MaxTurns int
}

func NewAgent(cfg AgentConfig) *Agent {
	a := &Agent{
		client:   cfg.Client,
		executor: cfg.Executor,
		maxTurns: cfg.MaxTurns,
	}
	if a.maxTurns <= 0 {
		a.maxTurns = 10
	}
	if cfg.System != "" {
		a.messages = append(a.messages, Message{Role: RoleSystem, Content: cfg.System})
	}
	return a
}

func (a *Agent) History() []Message {
	out := make([]Message, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *Agent) Reset(system string) {
	a.messages = a.messages[:0]
	if system != "" {
		a.messages = append(a.messages, Message{Role: RoleSystem, Content: system})
	}
}

func (a *Agent) Send(ctx context.Context, input string) (string, error) {
	a.messages = append(a.messages, Message{Role: RoleUser, Content: input})
	for turn := 0; turn < a.maxTurns; turn++ {
		resp, err := a.client.Chat(ctx, &ChatRequest{
			Messages: a.messages,
			Tools:    a.executor.Tools(),
		})
		if err != nil {
			return "", fmt.Errorf("chat: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		choice := resp.Choices[0]
		msg := choice.Message
		if len(msg.ToolCalls) == 0 {
			a.messages = append(a.messages, Message{
				Role:    RoleAssistant,
				Content: msg.Content,
			})
			return msg.Content, nil
		}
		a.messages = append(a.messages, Message{
			Role:      RoleAssistant,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})
		for _, tc := range msg.ToolCalls {
			result, execErr := a.executor.Execute(tc)
			if execErr != nil {
				result = fmt.Sprintf(`{"error": "%s"}`, execErr.Error())
			}
			a.messages = append(a.messages, Message{
				Role:       RoleTool,
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	return "", fmt.Errorf("exceeded max tool-calling turns (%d)", a.maxTurns)
}

// ---------- Agent stream types ----------

type AgentEventType int

const (
	AgentEventText      AgentEventType = iota
	AgentEventToolResult
	AgentEventDone
	AgentEventError
)

type AgentEvent struct {
	Type     AgentEventType
	Content  string
	ToolName string
	Err      error
}

func (a *Agent) SendStream(ctx context.Context, input string) (<-chan AgentEvent, error) {
	a.messages = append(a.messages, Message{Role: RoleUser, Content: input})
	eventCh := make(chan AgentEvent, 64)
	go func() {
		defer close(eventCh)
		for turn := 0; turn < a.maxTurns; turn++ {
			streamCh, err := a.client.ChatStream(ctx, &ChatRequest{
				Messages: a.messages,
				Tools:    a.executor.Tools(),
			})
			if err != nil {
				eventCh <- AgentEvent{Type: AgentEventError, Err: err}
				return
			}
			var fullText string
			var toolCalls []ToolCall
			for evt := range streamCh {
				switch evt.Type {
				case EventText:
					fullText += evt.Content
					eventCh <- AgentEvent{Type: AgentEventText, Content: evt.Content}
				case EventToolCall:
					if evt.ToolCall != nil {
						toolCalls = append(toolCalls, *evt.ToolCall)
					}
				case EventError:
					eventCh <- AgentEvent{Type: AgentEventError, Err: evt.Err}
					return
				}
			}
			if len(toolCalls) == 0 {
				a.messages = append(a.messages, Message{
					Role:    RoleAssistant,
					Content: fullText,
				})
				eventCh <- AgentEvent{Type: AgentEventDone}
				return
			}
			a.messages = append(a.messages, Message{
				Role:      RoleAssistant,
				Content:   fullText,
				ToolCalls: toolCalls,
			})
			for _, tc := range toolCalls {
				result, execErr := a.executor.Execute(tc)
				if execErr != nil {
					result = fmt.Sprintf(`{"error": "%s"}`, execErr.Error())
				}
				eventCh <- AgentEvent{
					Type:     AgentEventToolResult,
					ToolName: tc.Function.Name,
					Content:  result,
				}
				a.messages = append(a.messages, Message{
					Role:       RoleTool,
					ToolCallID: tc.ID,
					Content:    result,
				})
			}
		}
		eventCh <- AgentEvent{Type: AgentEventError, Err: fmt.Errorf("exceeded max turns")}
	}()
	return eventCh, nil
}
