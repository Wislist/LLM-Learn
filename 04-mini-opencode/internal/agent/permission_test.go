package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultPermissionPolicyAllowsReadOnlyTool(t *testing.T) {
	policy := NewDefaultPermissionPolicy(t.TempDir())
	decision := policy.Check(context.Background(), ToolCall{
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"file.txt"}`),
	}, ToolDefinition{Behavior: ToolBehavior{ReadOnly: true}})

	if decision.Action != PermissionAllow {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestDefaultPermissionPolicyDeniesBannedCommand(t *testing.T) {
	policy := NewDefaultPermissionPolicy(t.TempDir())
	decision := policy.Check(context.Background(), ToolCall{
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"git push origin main"}`),
	}, ToolDefinition{Behavior: ToolBehavior{Dangerous: true}})

	if decision.Action != PermissionDeny {
		t.Fatalf("decision = %#v", decision)
	}
	if !strings.Contains(decision.Reason, "git push") {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestDefaultPermissionPolicyDeniesWorkspaceEscape(t *testing.T) {
	policy := NewDefaultPermissionPolicy(t.TempDir())
	decision := policy.Check(context.Background(), ToolCall{
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"../outside.txt"}`),
	}, ToolDefinition{Behavior: ToolBehavior{ReadOnly: true}})

	if decision.Action != PermissionDeny {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestDefaultPermissionPolicyConfirmsDangerousTool(t *testing.T) {
	policy := NewDefaultPermissionPolicy(t.TempDir())
	decision := policy.Check(context.Background(), ToolCall{
		Name:      "write",
		Arguments: json.RawMessage(`{"path":"file.txt"}`),
	}, ToolDefinition{Behavior: ToolBehavior{Dangerous: true, RequiresConfirmation: true}})

	if decision.Action != PermissionConfirm {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestToolRegistryReturnsPermissionMetadata(t *testing.T) {
	registry := NewToolRegistry()
	registry.SetPermissionPolicy(NewDefaultPermissionPolicy(t.TempDir()))
	if err := registry.Register(permissionTestTool{}); err != nil {
		t.Fatal(err)
	}

	result := registry.Run(context.Background(), ToolCall{
		ID:        "call-1",
		Name:      "write_like",
		Arguments: json.RawMessage(`{"path":"file.txt"}`),
	})
	if result.Error == "" {
		t.Fatal("expected permission error")
	}
	if result.Metadata["permission"] != string(PermissionConfirm) {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

type permissionTestTool struct{}

func (permissionTestTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "write_like",
		Description: "test tool",
		InputSchema: map[string]any{"type": "object"},
		Behavior:    ToolBehavior{Dangerous: true, RequiresConfirmation: true},
	}
}

func (permissionTestTool) Run(ctx context.Context, input ToolInput) (ToolOutput, error) {
	return ToolOutput{Content: "ran"}, nil
}
