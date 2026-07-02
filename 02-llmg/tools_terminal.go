package llmg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TerminalConfig controls the behavior of TerminalExecutor.
type TerminalConfig struct {
	// AllowedDirs restricts file I/O to these directories.
	// Empty means no restriction.
	AllowedDirs []string

	// AllowExec enables the exec_command tool. Defaults to false for safety.
	AllowExec bool

	// CommandTimeout limits command execution duration (default 30s).
	CommandTimeout time.Duration
}

// TerminalExecutor implements ToolExecutor with filesystem and shell tools:
//   - read_file / write_file
//   - list_directory
//   - search_content
//   - get_file_info
//   - exec_command (only when AllowExec is true)
type TerminalExecutor struct {
	cfg TerminalConfig
}

func NewTerminalExecutor(cfg TerminalConfig) *TerminalExecutor {
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 30 * time.Second
	}
	return &TerminalExecutor{cfg: cfg}
}

func (t *TerminalExecutor) Tools() []Tool {
	tools := []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "read_file",
				Description: "Read a file and return its contents with line numbers.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path (relative or absolute)."},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "write_file",
				Description: "Create or overwrite a file. Parent directories are created automatically.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "File path."},
						"content": map[string]any{"type": "string", "description": "File content."},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_directory",
				Description: "List entries in a directory, optionally filtered by glob pattern.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "Directory to list (defaults to '.')."},
						"pattern": map[string]any{"type": "string", "description": "Optional glob pattern (e.g. '*.go')."},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_content",
				Description: "Search files for a regex pattern using rg (ripgrep) or grep.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for."},
						"path":    map[string]any{"type": "string", "description": "Directory or file to search (defaults to '.')."},
						"glob":    map[string]any{"type": "string", "description": "File glob filter (e.g. '*.go')."},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_file_info",
				Description: "Return file or directory metadata (size, mode, mod time, type).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Path to inspect."},
					},
					"required": []string{"path"},
				},
			},
		},
	}

	if t.cfg.AllowExec {
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name: "exec_command",
				Description: fmt.Sprintf(
					"Run a shell command and return stdout, stderr, and exit code. Timeout: %v.",
					t.cfg.CommandTimeout,
				),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":     map[string]any{"type": "string", "description": "Shell command to execute."},
						"working_dir": map[string]any{"type": "string", "description": "Working directory for the command."},
					},
					"required": []string{"command"},
				},
			},
		})
	}

	return tools
}

func (t *TerminalExecutor) Execute(tc ToolCall) (string, error) {
	switch tc.Function.Name {
	case "read_file":
		return t.readFile(tc.Function.Arguments)
	case "write_file":
		return t.writeFile(tc.Function.Arguments)
	case "list_directory":
		return t.listDirectory(tc.Function.Arguments)
	case "search_content":
		return t.searchContent(tc.Function.Arguments)
	case "get_file_info":
		return t.getFileInfo(tc.Function.Arguments)
	case "exec_command":
		return t.execCommand(tc.Function.Arguments)
	default:
		return "", fmt.Errorf("terminal: unknown tool %q", tc.Function.Name)
	}
}

// ---------- tool implementations ----------

func (t *TerminalExecutor) readFile(argsJSON string) (string, error) {
	var p struct{ Path string }
	json.Unmarshal([]byte(argsJSON), &p)
	if !t.pathAllowed(p.Path) {
		return "", fmt.Errorf("access denied: %s", p.Path)
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read_file %s: %w", p.Path, err)
	}
	lines := strings.Split(string(data), "\n")
	var sb strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&sb, "%4d| %s\n", i+1, line)
	}
	out, _ := json.Marshal(map[string]any{
		"content": sb.String(),
		"lines":   len(lines),
		"bytes":   len(data),
	})
	return string(out), nil
}

func (t *TerminalExecutor) writeFile(argsJSON string) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.Unmarshal([]byte(argsJSON), &p)
	if !t.pathAllowed(p.Path) {
		return "", fmt.Errorf("access denied: %s", p.Path)
	}
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("write_file %s: %w", p.Path, err)
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
		return "", fmt.Errorf("write_file %s: %w", p.Path, err)
	}
	out, _ := json.Marshal(map[string]any{"ok": true})
	return string(out), nil
}

func (t *TerminalExecutor) listDirectory(argsJSON string) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
	}
	json.Unmarshal([]byte(argsJSON), &p)
	if p.Path == "" {
		p.Path = "."
	}
	if !t.pathAllowed(p.Path) {
		return "", fmt.Errorf("access denied: %s", p.Path)
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return "", fmt.Errorf("list_directory %s: %w", p.Path, err)
	}
	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}
	var result []entry
	for _, e := range entries {
		if p.Pattern != "" {
			if ok, _ := filepath.Match(p.Pattern, e.Name()); !ok {
				continue
			}
		}
		info, err := e.Info()
		sz := int64(0)
		if err == nil {
			sz = info.Size()
		}
		result = append(result, entry{Name: e.Name(), IsDir: e.IsDir(), Size: sz})
	}
	out, _ := json.Marshal(map[string]any{
		"path":    p.Path,
		"entries": result,
		"count":   len(result),
	})
	return string(out), nil
}

func (t *TerminalExecutor) searchContent(argsJSON string) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	json.Unmarshal([]byte(argsJSON), &p)
	if p.Path == "" {
		p.Path = "."
	}
	if !t.pathAllowed(p.Path) {
		return "", fmt.Errorf("access denied: %s", p.Path)
	}
	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), t.cfg.CommandTimeout)
	defer cancel()
	if rg, err := exec.LookPath("rg"); err == nil {
		args := []string{"-n", "--no-heading"}
		if p.Glob != "" {
			args = append(args, "-g", p.Glob)
		}
		args = append(args, p.Pattern, p.Path)
		cmd = exec.CommandContext(ctx, rg, args...)
	} else if grep, err := exec.LookPath("grep"); err == nil {
		args := []string{"-rn"}
		if p.Glob != "" {
			args = append(args, "--include", p.Glob)
		}
		args = append(args, p.Pattern, p.Path)
		cmd = exec.CommandContext(ctx, grep, args...)
	} else {
		return "", fmt.Errorf("search_content: no search tool available (install rg or grep)")
	}
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			out, _ := json.Marshal(map[string]any{"matches": []string{}, "count": 0})
			return string(out), nil
		}
		return "", fmt.Errorf("search_content: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	out, _ := json.Marshal(map[string]any{"matches": lines, "count": len(lines)})
	return string(out), nil
}

func (t *TerminalExecutor) getFileInfo(argsJSON string) (string, error) {
	var p struct{ Path string }
	json.Unmarshal([]byte(argsJSON), &p)
	if !t.pathAllowed(p.Path) {
		return "", fmt.Errorf("access denied: %s", p.Path)
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		return "", fmt.Errorf("get_file_info %s: %w", p.Path, err)
	}
	out, _ := json.Marshal(map[string]any{
		"name":     info.Name(),
		"size":     info.Size(),
		"is_dir":   info.IsDir(),
		"mode":     info.Mode().String(),
		"mod_time": info.ModTime().Format(time.RFC3339),
	})
	return string(out), nil
}

func (t *TerminalExecutor) execCommand(argsJSON string) (string, error) {
	if !t.cfg.AllowExec {
		return "", fmt.Errorf("exec_command is disabled (set AllowExec=true)")
	}
	var p struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir"`
	}
	json.Unmarshal([]byte(argsJSON), &p)
	if p.Command == "" {
		return "", fmt.Errorf("exec_command: command is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.cfg.CommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	if p.WorkingDir != "" {
		cmd.Dir = p.WorkingDir
	}
	output, err := cmd.CombinedOutput()
	result := map[string]any{"stdout": string(output), "exit_code": 0}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result["exit_code"] = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("exec_command: %w", err)
		}
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// pathAllowed returns true when p is within one of the configured AllowedDirs.
// If no dirs are configured, all paths are allowed.
func (t *TerminalExecutor) pathAllowed(p string) bool {
	if len(t.cfg.AllowedDirs) == 0 {
		return true
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	for _, dir := range t.cfg.AllowedDirs {
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, dirAbs) {
			return true
		}
	}
	return false
}
