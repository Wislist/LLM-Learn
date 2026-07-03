package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wislist/llmg"
)

func main() {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "请设置 DEEPSEEK_API_KEY")
		os.Exit(1)
	}

	client := llmg.New(llmg.WithDeepSeek(key))

	executor := llmg.NewMultiExecutor(
		&demoExecutor{},
		llmg.NewTerminalExecutor(llmg.TerminalConfig{
			AllowExec:      true,
			CommandTimeout: 30 * time.Second,
		}),
	)

	agent := llmg.NewAgent(llmg.AgentConfig{
		Client:   client,
		Executor: executor,
		MaxTurns: 15,
		System: strings.Join([]string{
			"你是终端助手，可以读写文件、执行 shell 命令、搜索代码。",
			"",
			"行为准则：",
			"- 每次调工具前先想清楚：这次调用能获得什么新信息？",
			"- 如果工具返回了你需要的数据，直接给用户回答，不要再调同一个工具。",
			"- 读文件最多读一次；如果内容太长被截断，告知用户并用 search_content 精准搜索。",
			"- 一个任务完成后，给出简洁总结，不要再追加工具调用。",
			"- 回答简洁，代码优先。",
		}, "\n"),
	})

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("LLM-Learn Agent (输入 exit 退出)\n")

	for {
		fmt.Print(">>> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		events, err := agent.SendStream(context.Background(), input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			continue
		}

		for evt := range events {
			switch evt.Type {
			case llmg.AgentEventText:
				fmt.Print(evt.Content)
			case llmg.AgentEventToolResult:
				fmt.Printf("\n  [%s] %s\n", evt.ToolName, truncate(evt.Content, 200))
			case llmg.AgentEventError:
				fmt.Fprintf(os.Stderr, "\nerror: %v\n", evt.Err)
			case llmg.AgentEventDone:
				break
			}
		}
		fmt.Println()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// demoExecutor provides get_weather and get_date as demo tools.
type demoExecutor struct{}

func (d *demoExecutor) Tools() []llmg.Tool {
	return []llmg.Tool{
		{
			Type: "function",
			Function: llmg.ToolFunction{
				Name:        "get_weather",
				Description: "查询指定城市的天气",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string", "description": "城市名"},
					},
					"required": []string{"city"},
				},
			},
		},
		{
			Type: "function",
			Function: llmg.ToolFunction{
				Name:        "get_date",
				Description: "获取今天的日期",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
	}
}

func (d *demoExecutor) Execute(tc llmg.ToolCall) (string, error) {
	switch tc.Function.Name {
	case "get_weather":
		var p struct{ City string }
		json.Unmarshal([]byte(tc.Function.Arguments), &p)
		out, _ := json.Marshal(map[string]any{
			"temperature": 32,
			"condition":   "晴",
			"city":        p.City,
		})
		return string(out), nil
	case "get_date":
		return `{"date": "2026-07-02"}`, nil
	}
	return "", fmt.Errorf("demo: unknown tool %q", tc.Function.Name)
}
