 package main
 
 import (
 	"context"
 	"encoding/json"
 	"fmt"
 	"os/exec"
 	"strings"
 	"time"
 
 	"github.com/wislist/llmg"
 )
 
 type bashTool struct {
 	workDir string
 }
 
 func (t *bashTool) Name() string        { return "bash" }
 func (t *bashTool) Description() string { return "在当前项目目录执行 shell 命令。超时 30 秒，输出截断到 4KB。" }
 
 func (t *bashTool) Schema() llmg.Tool {
 	return llmg.Tool{
 		Type: "function",
 		Function: llmg.ToolFunction{
 			Name:        "bash",
 			Description: t.Description(),
 			Parameters: map[string]any{
 				"type": "object",
 				"properties": map[string]any{
 					"command": map[string]any{"type": "string", "description": "要执行的命令"},
 				},
 				"required": []string{"command"},
 			},
 		},
 	}
 }
 
 func (t *bashTool) Execute(args string) (string, error) {
 	var params struct{ Command string }
 	json.Unmarshal([]byte(args), &params)
 	if strings.TrimSpace(params.Command) == "" {
 		return "", fmt.Errorf("bash: empty command")
 	}
 
 	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
 	defer cancel()
 
 	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
 	cmd.Dir = t.workDir
 
 	output, err := cmd.CombinedOutput()
 	outStr := string(output)
 	if len(outStr) > 4096 {
 		outStr = outStr[:4096] + "\n... (截断)"
 	}
 
 	exitCode := 0
 	if err != nil {
 		if exitErr, ok := err.(*exec.ExitError); ok {
 			exitCode = exitErr.ExitCode()
 		} else {
 			exitCode = -1
 		}
 	}
 
 	result, _ := json.Marshal(map[string]any{
 		"exit_code": exitCode,
 		"output":    outStr,
 	})
 	return string(result), nil
 }
