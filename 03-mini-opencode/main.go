package main

import (
	"bufio"
	"fmt"
	"path/filepath"
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

	fmt.Printf("mini-opencode v0.4  (模型: %s)\n", config.Model)
	fmt.Println("命令: /help  /clear  /tools  /skills  /install-skill <name> <file>  /install-skill-github <repo> <path> [ref]  /remove-skill <name>  /mcp  /prompt <name>  /sessions  /new  /resume <id>  /session  /yes  exit")
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
			fmt.Println("\n命令: /help  /clear  /tools  /skills  /install-skill <name> <file>  /install-skill-github <repo> <path> [ref]  /remove-skill <name>  /mcp  /prompt <name>  /sessions  /new  /resume <id>  /session  /yes  exit")
			fmt.Println()
			continue
		case input == "/clear":
			agent.Clear()
			continue
		case input == "/tools":
			fmt.Println("\n可用工具:")
			fmt.Println(agent.tools.ToolPrompt())
			continue
		case input == "/skills":
			fmt.Println()
			agent.ListSkills()
			fmt.Println()
			continue
		case strings.HasPrefix(input, "/install-skill-github "):
			fields := strings.Fields(strings.TrimPrefix(input, "/install-skill-github "))
			fmt.Println()
			if len(fields) < 2 {
				fmt.Println("用法: /install-skill-github <owner/repo> <skill_path> [ref]")
			} else {
				installSkillFromGitHubCLI(agent, fields[0], fields[1], nthOr(fields, 2, "main"))
			}
			fmt.Println()
			continue
		case strings.HasPrefix(input, "/install-skill "):
			fields := strings.Fields(strings.TrimPrefix(input, "/install-skill "))
			fmt.Println()
			if len(fields) < 2 {
				fmt.Println("用法: /install-skill <name> <file>  (file 内容作为 SKILL.md)")
			} else {
				installSkillCLI(agent, fields[0], fields[1])
			}
			fmt.Println()
			continue
		case input == "/install-skill":
			fmt.Println("用法: /install-skill <name> <file>  (file 内容作为 SKILL.md)")
			fmt.Println("      /install-skill-github <owner/repo> <skill_path> [ref]")
			continue
		case strings.HasPrefix(input, "/remove-skill "):
			name := strings.TrimSpace(strings.TrimPrefix(input, "/remove-skill "))
			fmt.Println()
			agent.RemoveSkill(name)
			fmt.Println()
			continue
		case input == "/remove-skill":
			fmt.Println("用法: /remove-skill <name>")
			continue
		case input == "/mcp":
			fmt.Println()
			agent.ListMCP()
			fmt.Println()
			continue
		case strings.HasPrefix(input, "/prompt "):
			name := strings.TrimSpace(strings.TrimPrefix(input, "/prompt "))
			fmt.Println()
			agent.GetPrompt(name)
			fmt.Println()
			continue
		case input == "/prompt":
			fmt.Println("用法: /prompt <name> [args]（用 /mcp 查看可用 prompt）")
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

// installSkillCLI 从本地文件读取内容并安装为技能。
func installSkillCLI(agent *Agent, name, file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Printf("[错误] 读取文件失败: %v\n", err)
		return
	}
	agent.InstallSkill(name, string(data))
}

// installSkillFromGitHubCLI 从 GitHub 仓库下载并安装技能。
func installSkillFromGitHubCLI(agent *Agent, repo, skillPath, ref string) {
	if agent.skills == nil {
		fmt.Println("[错误] 技能存储不可用")
		return
	}
	path, err := agent.skills.InstallFromGitHub(repo, skillPath, ref)
	if err != nil {
		fmt.Printf("[错误] %v\n", err)
		return
	}
	fmt.Printf("[已安装技能] %s -> %s\n", filepath.Base(skillPath), path)
}

// nthOr 返回 s[i]，越界时返回 fallback。
func nthOr(s []string, i int, fallback string) string {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return fallback
}
