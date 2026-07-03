package llmg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
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
	client           *Client
	executor         ToolExecutor
	messages         []Message
	maxTurns         int
	maxToolResultLen int // cap tool result bytes (0 = default 8000)
}

// AgentConfig configures a new Agent.
type AgentConfig struct {
	Client           *Client
	Executor         ToolExecutor
	System           string
	MaxTurns         int  // max tool-calling turns per user message (default 10)
	MaxToolResultLen int  // cap tool result bytes (default 8000)
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
	a.maxToolResultLen = cfg.MaxToolResultLen
	if a.maxToolResultLen <= 0 {
		a.maxToolResultLen = 8000
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

	var lastCall struct {
		name string
		hash string
	}

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

		// Detect loop: same tool + same args hash = force stop.
		for _, tc := range msg.ToolCalls {
			hash := a.hashCall(tc.Function.Name, tc.Function.Arguments)
			if lastCall.name == tc.Function.Name && lastCall.hash == hash {
				content := msg.Content
				if content == "" {
					content = "(tool loop detected — stopping)"
				}
				a.messages = append(a.messages, Message{
					Role:    RoleAssistant,
					Content: content,
				})
				return content, nil
			}
			lastCall.name = tc.Function.Name
			lastCall.hash = hash
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
			result = a.truncateResult(result)
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

		var lastCall struct {
			name string
			hash string
		}

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

			// Detect loop: same tool + same args hash = force stop.
			for _, tc := range toolCalls {
				hash := a.hashCall(tc.Function.Name, tc.Function.Arguments)
				if lastCall.name == tc.Function.Name && lastCall.hash == hash {
					a.messages = append(a.messages, Message{
						Role:    RoleAssistant,
						Content: fullText,
					})
					eventCh <- AgentEvent{Type: AgentEventDone}
					return
				}
				lastCall.name = tc.Function.Name
				lastCall.hash = hash
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
				result = a.truncateResult(result)
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

// hashCall returns a short hash of tool name + arguments for loop detection.
func (a *Agent) hashCall(name, args string) string {
	h := sha256.Sum256([]byte(name + "\x00" + args))
	return hex.EncodeToString(h[:8])
}

// truncateResult caps the tool result to maxToolResultLen bytes.
func (a *Agent) truncateResult(result string) string {
	if a.maxToolResultLen <= 0 || len(result) <= a.maxToolResultLen {
		return result
	}
	cut := result[:a.maxToolResultLen]
	note := fmt.Sprintf("\n... [truncated at %d bytes, total %d]", a.maxToolResultLen, len(result))
	// Inject truncation note before closing brace if result looks JSON.
	if idx := strings.LastIndex(cut, "}"); idx > 0 {
		return cut[:idx] + note + "}"
	}
	return cut + note
}
