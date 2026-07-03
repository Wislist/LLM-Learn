package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeClient struct {
	result CallToolResult
}

func (c fakeClient) Initialize(ctx context.Context) error { return nil }
func (c fakeClient) ListTools(ctx context.Context) ([]ToolDef, error) {
	return nil, nil
}
func (c fakeClient) CallTool(ctx context.Context, name string, args json.RawMessage) (CallToolResult, error) {
	return c.result, nil
}
func (c fakeClient) Close() error { return nil }

func TestToolAdapterReturnsTextContent(t *testing.T) {
	adapter := NewToolAdapter(fakeClient{
		result: CallToolResult{
			Content: []ToolContent{
				{Type: "text", Text: "hello"},
				{Type: "text", Text: "world"},
			},
		},
	}, ToolDef{Name: "go_doc", Description: "Go docs"})

	got, err := adapter.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "hello\nworld" {
		t.Fatalf("Run() = %q", got)
	}
}
