package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wislist/mini-opencode/internal/agent"
)

type ToolAdapter struct {
	client Client
	def    ToolDef
}

func NewToolAdapter(client Client, def ToolDef) *ToolAdapter {
	return &ToolAdapter{client: client, def: def}
}

func (t *ToolAdapter) Spec() agent.ToolSpec {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if len(t.def.InputSchema) > 0 {
		_ = json.Unmarshal(t.def.InputSchema, &schema)
	}

	return agent.ToolSpec{
		Name:        t.def.Name,
		Description: t.def.Description,
		InputSchema: schema,
	}
}

func (t *ToolAdapter) Run(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := t.client.CallTool(ctx, t.def.Name, args)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, item := range result.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		}
	}
	output := strings.Join(parts, "\n")
	if result.IsError {
		if output == "" {
			output = "mcp tool returned an error"
		}
		return "", errors.New(output)
	}
	return output, nil
}
