package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/wislist/llmg"
)

func main() {
	config := LoadConfig()
	if config.APIKey == "" {
		fmt.Fprintln(os.Stderr, "请设置 DEEPSEEK_API_KEY 环境变量")
		os.Exit(1)
	}

	workDir, _ := os.Getwd()
	client := llmg.New(llmg.WithDeepSeek(config.APIKey))
	agent := NewAgent(client, config, workDir)

	fmt.Printf("mini-opencode v0.3  (模型: %s)\n", config.Model)
	fmt.Println("命令: /help  /clear  /tools  /sessions  /new  /resume <id>  /session  /yes  exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(">>> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch {
		case input == "exit" || input == "quit":
			return
		case input == "/help":
			fmt.Println("\n命令: /help  /clear  /tools  /sessions  /new  /resume <id>  /session  /yes  exit")
			fmt.Println()
			continue
		case input == "/clear":
			agent.Clear()
			continue
		case input == "/tools":
			fmt.Println("\n可用工具:")
			fmt.Println(agent.tools.ToolPrompt())
			continue
		case input == "/sessions":
			fmt.Println()
			agent.ListSessions()
			fmt.Println()
			continue
		case input == "/new":
			fmt.Println()
			agent.NewSession()
			fmt.Println()
			continue
		case input == "/session":
			fmt.Println()
			agent.CurrentSession()
			fmt.Println()
			continue
		case input == "/yes":
			fmt.Println()
			agent.ToggleAutoApprove()
			fmt.Println()
			continue
		case strings.HasPrefix(input, "/resume "):
			id := strings.TrimSpace(strings.TrimPrefix(input, "/resume "))
			fmt.Println()
			if err := agent.ResumeSession(id); err != nil {
				fmt.Printf("[error] %v\n", err)
			}
			fmt.Println()
			continue
		case input == "/resume":
			fmt.Println("用法: /resume <session-id>（用 /sessions 查看列表）")
			continue
		}

		agent.Run(input)
	}
}
