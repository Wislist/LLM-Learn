package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type PermissionAction string

const (
	PermissionAllow   PermissionAction = "allow"
	PermissionDeny    PermissionAction = "deny"
	PermissionConfirm PermissionAction = "confirm"
)

type PermissionDecision struct {
	Action PermissionAction
	Reason string
}

type PermissionPolicy interface {
	Check(ctx context.Context, call ToolCall, definition ToolDefinition) PermissionDecision
}

type DefaultPermissionPolicy struct {
	WorkDir        string
	BannedCommands []string
}

func NewDefaultPermissionPolicy(workDir string) DefaultPermissionPolicy {
	return DefaultPermissionPolicy{
		WorkDir: workDir,
		BannedCommands: []string{
			"rm -rf",
			"sudo",
			"chmod -R",
			"chown -R",
			"git push",
			"git reset --hard",
		},
	}
}

func (p DefaultPermissionPolicy) Check(ctx context.Context, call ToolCall, definition ToolDefinition) PermissionDecision {
	select {
	case <-ctx.Done():
		return PermissionDecision{Action: PermissionDeny, Reason: ctx.Err().Error()}
	default:
	}

	if reason := p.checkWorkspacePaths(call.Arguments); reason != "" {
		return PermissionDecision{Action: PermissionDeny, Reason: reason}
	}
	if reason := p.checkBannedCommand(call.Arguments); reason != "" {
		return PermissionDecision{Action: PermissionDeny, Reason: reason}
	}
	if definition.Behavior.RequiresConfirmation || definition.Behavior.Dangerous {
		return PermissionDecision{Action: PermissionConfirm, Reason: "tool requires confirmation"}
	}
	return PermissionDecision{Action: PermissionAllow}
}

func (p DefaultPermissionPolicy) checkBannedCommand(args json.RawMessage) string {
	var values map[string]any
	if err := json.Unmarshal(args, &values); err != nil {
		return ""
	}
	command, ok := values["command"].(string)
	if !ok || command == "" {
		return ""
	}
	lower := strings.ToLower(command)
	for _, banned := range p.BannedCommands {
		if strings.Contains(lower, strings.ToLower(banned)) {
			return fmt.Sprintf("banned command matched %q", banned)
		}
	}
	return ""
}

func (p DefaultPermissionPolicy) checkWorkspacePaths(args json.RawMessage) string {
	if p.WorkDir == "" {
		return ""
	}
	var values map[string]any
	if err := json.Unmarshal(args, &values); err != nil {
		return ""
	}
	for _, key := range []string{"path", "working_dir"} {
		value, ok := values[key].(string)
		if !ok || value == "" {
			continue
		}
		if !pathWithinWorkspace(p.WorkDir, value) {
			return fmt.Sprintf("%s escapes workspace: %s", key, value)
		}
	}
	return ""
}

func pathWithinWorkspace(workDir string, path string) bool {
	base, err := filepath.Abs(workDir)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
