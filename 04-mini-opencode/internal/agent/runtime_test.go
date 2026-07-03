package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type scriptedProvider struct {
	responses []AssistantResponse
	calls     int
}

func (p *scriptedProvider) Complete(ctx context.Context, req Request) (AssistantResponse, error) {
	if p.calls >= len(p.responses) {
		return AssistantResponse{Content: "done"}, nil
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

type uppercaseTool struct{}

func (t uppercaseTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "uppercase",
		Description: "Uppercase input text.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
		Behavior: ToolBehavior{ReadOnly: true},
	}
}

func (t uppercaseTool) Run(ctx context.Context, toolInput ToolInput) (ToolOutput, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(toolInput.Arguments, &args); err != nil {
		return ToolOutput{}, err
	}
	return ToolOutput{Content: strings.ToUpper(args.Text)}, nil
}

func TestRuntimeExecutesToolAndContinues(t *testing.T) {
	provider := &scriptedProvider{
		responses: []AssistantResponse{
			{
				Content: "using tool",
				ToolCalls: []ToolCall{
					{
						ID:        "call-1",
						Name:      "uppercase",
						Arguments: json.RawMessage(`{"text":"hello"}`),
					},
				},
			},
			{Content: "finished"},
		},
	}
	runtime := NewRuntime(provider, WithTool(uppercaseTool{}))

	var events []EventType
	err := runtime.Run(context.Background(), "start", func(event Event) {
		events = append(events, event.Type)
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	messages := runtime.Messages()
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	if messages[2].Role != RoleTool || messages[2].Content != "HELLO" {
		t.Fatalf("tool message = %#v", messages[2])
	}
	if events[len(events)-1] != EventRunFinished {
		t.Fatalf("last event = %s, want %s", events[len(events)-1], EventRunFinished)
	}
}
