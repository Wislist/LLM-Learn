package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

type Tool interface {
	Definition() ToolDefinition
	Run(ctx context.Context, input ToolInput) (ToolOutput, error)
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Prompt      string         `json:"prompt,omitempty"`
	Behavior    ToolBehavior   `json:"behavior"`
}

type ToolSpec = ToolDefinition

type ToolBehavior struct {
	Dangerous            bool `json:"dangerous"`
	RequiresConfirmation bool `json:"requires_confirmation"`
	SupportsBackground   bool `json:"supports_background"`
	ReadOnly             bool `json:"read_only"`
}

type ToolInput struct {
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolOutput struct {
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ToolRegistry struct {
	tools map[string]Tool
	order []string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]Tool{}}
}

func (r *ToolRegistry) Register(tool Tool) error {
	spec := tool.Definition()
	if spec.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, ok := r.tools[spec.Name]; ok {
		return fmt.Errorf("tool already registered: %s", spec.Name)
	}
	r.tools[spec.Name] = tool
	r.order = append(r.order, spec.Name)
	sort.Strings(r.order)
	return nil
}

func (r *ToolRegistry) Specs() []ToolSpec {
	specs := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		specs = append(specs, r.tools[name].Definition())
	}
	return specs
}

func (r *ToolRegistry) Run(ctx context.Context, call ToolCall) ToolResult {
	tool, ok := r.tools[call.Name]
	if !ok {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Error:      "unknown tool",
		}
	}

	out, err := tool.Run(ctx, ToolInput{
		CallID:    call.ID,
		Name:      call.Name,
		Arguments: call.Arguments,
	})
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Error:      err.Error(),
		}
	}
	return ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    out.Content,
		Metadata:   out.Metadata,
	}
}
