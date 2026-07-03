package agent

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResult struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type AssistantResponse struct {
	Content   string
	ToolCalls []ToolCall
}

type EventType string

const (
	EventRunStarted        EventType = "run_started"
	EventTurnStarted       EventType = "turn_started"
	EventAssistantResponse EventType = "assistant_response"
	EventToolCallStarted   EventType = "tool_call_started"
	EventToolCallFinished  EventType = "tool_call_finished"
	EventRunFinished       EventType = "run_finished"
	EventRunFailed         EventType = "run_failed"
)

type Event struct {
	Type       EventType
	Turn       int
	Message    *Message
	ToolCall   *ToolCall
	ToolResult *ToolResult
	Error      error
}
