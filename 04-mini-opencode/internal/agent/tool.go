package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

type Tool interface {
	Spec() ToolSpec
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ToolRegistry struct {
	tools map[string]Tool
	order []string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]Tool{}}
}

func (r *ToolRegistry) Register(tool Tool) error {
	spec := tool.Spec()
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
		specs = append(specs, r.tools[name].Spec())
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

	out, err := tool.Run(ctx, call.Arguments)
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
		Content:    out,
	}
}
