package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wislist/llmg"
)

const defaultSystemPrompt = `你是一个强大的 CLI 编程 Agent，能在终端里读写文件、执行命令、搜索代码。

<critical_rules>
以下规则优先级最高，必须严格遵守：

1. 改文件前先读：用 read_file 读过相关内容后再 edit。edit 的 old_string 必须与文件内容精确匹配（含空白和缩进）。
2. 自主行动：不要反问用户。搜索、读取、思考、决定、执行。把复杂任务拆成多步并用 todos 规划，全部完成。只在遇到硬性阻塞（缺权限/缺凭证/文件不存在）时停下。
3. 改完即测：每次修改后立即用 bash 跑测试或编译验证。
4. 简洁输出：默认 <4 行。简洁指文本量，不是偷工减料——该实现的代码必须完整。
5. 精确匹配：edit 时 old_string 要与文件内容完全一致，包括缩进和换行。
6. 不擅自 commit：除非用户明确说"提交"，否则不 git commit/push。
7. 不加注释：除非用户要求，不写代码注释。
8. 安全第一：只协助防御性安全任务，拒绝创建可能被恶意使用的代码。
9. 不猜 URL：只用用户提供的或本地文件里出现的 URL。
10. 不猜路径：用 glob/ls 确认文件存在再操作，不要瞎猜路径。
11. 大文件只读片段：用 read_file 的 offset/limit 只读需要的部分，不要把整个大文件塞进上下文。
12. 搜索优先：找文件用 glob，搜内容用 grep，不要用 bash 的 find/grep 替代。
13. 避免死循环：如果同一操作连续失败，换方法，不要重复硬试。
14. 记忆管理：用户表达偏好、项目约定、技术决策时用 remember 工具保存。不再需要的记忆用 forget 删除。
</critical_rules>

<communication_style>
- 用与用户提问相同的语言回答。
- 不说废话：无"我来…"、"这是…"、"希望…"等开场/收尾。
- 能一个词回答就一个词。
- 引用代码位置用 file_path:line_number 格式。
- 多步骤任务用 todos 工具规划进度。
</communication_style>

<workflow>
每一步内部遵循（不用说出来）：
- 先搜代码定位相关文件（glob/grep）
- read_file 读当前状态
- edit 精确修改（不要用 write_file 整文件覆盖，除非新建文件）
- bash 跑测试验证
- 失败就分析错误、修代码，直到通过
- 全部完成后再向用户汇报结果
</workflow>`

type Agent struct {
	client      *llmg.Client
	tools       *ToolRegistry
	history     *History
	config      Config
	output      io.Writer
	store       *SessionStore
	memory      *MemoryStore
	skills      *SkillStore
	mcp         *MCPManager
	workDir     string
	hook        *CLIPermissionHook
	toolHistory []string
}

func NewAgent(client *llmg.Client, config Config, workDir string) *Agent {
	tools := NewToolRegistry()
	tools.Register(&readFileTool{})
	tools.Register(&writeFileTool{})
	tools.Register(&editTool{})
	tools.Register(&globTool{})
	tools.Register(&grepTool{})
	tools.Register(&lsTool{})
	tools.Register(newTodosTool())
	tools.Register(&bashTool{workDir: workDir})

	// 加载本地技能。
	skillStore := NewSkillStore(config.SkillsDir)
	// 技能工具始终注册：即使当前无技能，LLM 也能用 install_skill 安装第一个技能。
	tools.Register(&listSkillsTool{store: skillStore})
	tools.Register(&readSkillTool{store: skillStore})
	tools.Register(&installSkillTool{store: skillStore})
	tools.Register(&installSkillFromGitHubTool{store: skillStore})
	tools.Register(&removeSkillTool{store: skillStore})

	// 加载 MCP server。
	mcpMgr := NewMCPManager()
	if len(config.MCP) > 0 {
		mcpMgr.LoadFromConfig(tools, config.MCP)
	}

	store, err := OpenSessionStore(config.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] session store 不可用: %v\n", err)
	}

	var memory *MemoryStore
	if store != nil {
		if err := store.InitMemoryTables(); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] memory tables 初始化失败: %v\n", err)
		} else {
			memory = store.MemoryStore()
			tools.Register(&rememberTool{store: memory})
			tools.Register(&forgetTool{store: memory})
			tools.Register(&listMemoryTool{store: memory})
		}
	}

	a := &Agent{
		client:  client,
		tools:   tools,
		config:  config,
		output:  os.Stdout,
		store:   store,
		memory:  memory,
		skills:  skillStore,
		mcp:     mcpMgr,
		workDir: workDir,
		hook:    NewCLIPermissionHook(config.AutoApprove),
	}

	// 构建 system prompt：基础 prompt + 工具列表 + 技能列表。
	prompt := defaultSystemPrompt + "\n\n可用工具:\n" + tools.ToolPrompt()
	prompt += skillStore.Prompt()
	a.history = NewHistory(prompt, config.MaxHist)

	if store != nil {
		a.resumeOrNew()
	}
	return a
}

func (a *Agent) resumeOrNew() {
	if latest, err := a.store.LatestSession(); err == nil && latest != nil {
		a.history.BindSession(latest.ID, a.store)
		if err := a.history.LoadContext(); err != nil {
			fmt.Fprintf(a.output, "[warn] 恢复会话失败: %v\n", err)
			a.newSession()
			return
		}
		fmt.Fprintf(a.output, "[已恢复会话 %s] %s\n", latest.ID, latest.Title)
		return
	}
	a.newSession()
}

func (a *Agent) newSession() {
	if a.store == nil {
		return
	}
	id, err := a.store.CreateSession("新会话", a.config.Model)
	if err != nil {
		fmt.Fprintf(a.output, "[warn] 创建会话失败: %v\n", err)
		return
	}
	a.history.BindSession(id, a.store)
}

func (a *Agent) ResumeSession(id string) error {
	if a.store == nil {
		return fmt.Errorf("session store 不可用")
	}
	sess, err := a.store.GetSession(id)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("会话 %s 不存在", id)
	}
	a.history.BindSession(id, a.store)
	if err := a.history.LoadContext(); err != nil {
		return err
	}
	fmt.Fprintf(a.output, "[已恢复会话 %s] %s\n", sess.ID, sess.Title)
	return nil
}

func (a *Agent) NewSession() {
	a.history.Clear()
	a.newSession()
	fmt.Fprintln(a.output, "[已开启新会话]")
}

func (a *Agent) ListSessions() {
	if a.store == nil {
		fmt.Fprintln(a.output, "[session store 不可用]")
		return
	}
	sessions, err := a.store.ListSessions(10)
	if err != nil {
		fmt.Fprintf(a.output, "[error] %v\n", err)
		return
	}
	if len(sessions) == 0 {
		fmt.Fprintln(a.output, "暂无会话")
		return
	}
	fmt.Fprintln(a.output, "最近会话:")
	for _, s := range sessions {
		marker := "  "
		if s.ID == a.history.SessionID() {
			marker = "→ "
		}
		fmt.Fprintf(a.output, "%s%s  %s  %s\n", marker, s.ID, s.Title, s.UpdatedAt)
	}
}

func (a *Agent) CurrentSession() {
	if a.store == nil || a.history.SessionID() == "" {
		fmt.Fprintln(a.output, "[无活跃会话]")
		return
	}
	sess, err := a.store.GetSession(a.history.SessionID())
	if err != nil || sess == nil {
		fmt.Fprintln(a.output, "[会话信息不可用]")
		return
	}
	fmt.Fprintf(a.output, "会话: %s\n标题: %s\n模型: %s\n更新: %s\nToken: %d in / %d out\n",
		sess.ID, sess.Title, sess.Model, sess.UpdatedAt, sess.PromptTokens, sess.CompletionTokens)
}

// tokenBudget 计算对话历史的可用 token 预算。
// 预算 = 上下文窗口 - system prompt - 工具 schema - 输出预留 - 记忆预留
func (a *Agent) tokenBudget() int {
	systemTokens := estimateTextTokens(a.history.systemPrompt)
	toolTokens := estimateToolsTokens(a.tools.ToLLMTools())
	budget := a.config.ContextWindow - systemTokens - toolTokens - a.config.OutputReserve - a.config.MemoryReserve
	if budget < 4096 {
		budget = 4096
	}
	return int(float64(budget) * a.config.CompressionRatio)
}

// recallMemories 检索与用户输入相关的长期记忆，返回注入用的文本。
func (a *Agent) recallMemories(input string) string {
	if a.memory == nil || strings.TrimSpace(input) == "" {
		return ""
	}
	memories, err := a.memory.Search(input, 5)
	if err != nil || len(memories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n<relevant_memories>\n")
	for _, m := range memories {
		fmt.Fprintf(&sb, "- [%s] %s\n", m.Kind, m.Content)
	}
	sb.WriteString("</relevant_memories>")
	return sb.String()
}

func (a *Agent) Run(input string) {
	if a.history == nil {
		prompt := defaultSystemPrompt + "\n\n可用工具:\n" + a.tools.ToolPrompt()
		a.history = NewHistory(prompt, a.config.MaxHist)
	}

	// 召回长期记忆，注入到本轮 user 消息前。
	memoryContext := a.recallMemories(input)
	userContent := input
	if memoryContext != "" {
		userContent = memoryContext + "\n\n" + input
	}

	a.history.Add(llmg.Message{Role: llmg.RoleUser, Content: userContent})

	if a.store != nil && a.history.SessionID() != "" {
		if count, err := a.store.MessageCount(a.history.SessionID()); err == nil && count <= 1 {
			title := input
			if len([]rune(title)) > 40 {
				title = string([]rune(title)[:40]) + "..."
			}
			_ = a.store.RenameSession(a.history.SessionID(), title)
		}
	}

	budget := a.tokenBudget()

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		if a.history.NeedsSummary(budget) {
			if err := a.summarizeHistory(); err != nil {
				fmt.Fprintf(os.Stderr, "\n[warn] 摘要失败: %v\n", err)
			}
		}

		events, err := a.client.ChatStream(context.Background(), &llmg.ChatRequest{
			Model:    a.config.Model,
			Messages: a.history.Messages(),
			Tools:    a.tools.ToLLMTools(),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[error] %v\n", err)
			return
		}

		responseText := ""
		var pendingToolCalls []llmg.ToolCall

		for evt := range events {
			switch evt.Type {
			case llmg.EventText:
				fmt.Fprint(a.output, evt.Content)
				responseText += evt.Content
			case llmg.EventToolCall:
				if evt.ToolCall != nil {
					pendingToolCalls = append(pendingToolCalls, *evt.ToolCall)
				}
			case llmg.EventDone:
				if evt.Usage != nil && a.store != nil && a.history.SessionID() != "" {
					_ = a.store.AddUsage(a.history.SessionID(), evt.Usage.PromptTokens, evt.Usage.CompletionTokens)
				}
			case llmg.EventError:
				fmt.Fprintf(os.Stderr, "\n[error] %v\n", evt.Err)
			}
		}

		if len(pendingToolCalls) == 0 {
			a.history.Add(llmg.Message{Role: llmg.RoleAssistant, Content: responseText})
			fmt.Fprintln(a.output)
			return
		}

		fmt.Fprintln(a.output)
		a.history.Add(llmg.Message{
			Role:      llmg.RoleAssistant,
			Content:   responseText,
			ToolCalls: pendingToolCalls,
		})

		// loop detection
		for _, tc := range pendingToolCalls {
			a.toolHistory = append(a.toolHistory, toolSignature(tc))
		}
		if looped, reason := detectLoop(a.toolHistory); looped {
			fmt.Fprintf(a.output, "  ⚠ %s\n", reason)
			a.history.Add(llmg.Message{
				Role:    llmg.RoleAssistant,
				Content: "（已中止：检测到死循环，请换一种方法）",
			})
			return
		}

		for _, tc := range pendingToolCalls {
			short := strings.ReplaceAll(tc.Function.Arguments, "\n", " ")
			if len(short) > 80 {
				short = short[:80] + "..."
			}
			fmt.Fprintf(a.output, "  [%s] %s(%s)\n", tc.Function.Name, tc.Function.Name, short)

			decision, reason := a.hook.Check(tc.Function.Name, tc.Function.Arguments)
			var result string
			var err error
			switch decision {
			case DecisionDeny:
				err = fmt.Errorf("权限拒绝: %s", reason)
				fmt.Fprintf(a.output, "  ✗ %s\n", err)
			case DecisionConfirm:
				if !a.hook.Confirm(tc.Function.Name, tc.Function.Arguments) {
					err = fmt.Errorf("用户拒绝执行 %s", tc.Function.Name)
					fmt.Fprintf(a.output, "  ✗ %s\n", err)
				}
			}
			if err == nil {
				result, err = a.tools.Execute(tc.Function.Name, tc.Function.Arguments)
			}
			if err != nil {
				result = fmt.Sprintf(`{"error": "%s"}`, err.Error())
			}
			a.history.Add(llmg.Message{
				Role:       llmg.RoleTool,
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	fmt.Fprintln(a.output, "\n[达到最大轮数]")
}

const summaryPrompt = `你正在压缩一段对话历史，供后续继续工作使用。这份摘要是唯一的上下文，请务必详尽。

如果已有旧摘要，请将其与新对话合并，不要丢弃旧信息。

要求包含：
## 当前状态
- 正在做什么任务（用户原始请求）
- 已完成什么、正在做什么、还剩什么

## 文件改动
- 改过哪些文件（简述改动）
- 读过哪些文件、为什么

## 技术上下文
- 关键架构决定、用到的库/命令
- 踩过的坑、假设、风险

## 下一步
- 具体下一步要做什么（不要写"实现认证"，要写"在 auth.go 里加 JWT 校验函数"）
`

// summarizeHistory 把旧的头部消息段调 LLM 压缩成一条摘要消息。
// 如果已有旧摘要，合并后生成新摘要。结果持久化到 session_summaries。
func (a *Agent) summarizeHistory() error {
	old := a.history.Summarizable()
	if len(old) < 2 {
		return nil
	}
	fmt.Fprintln(a.output, "  [压缩历史中…]")

	// 合并旧摘要
	priorSummary := a.history.persistedSummary
	userContent := messagesToText(old)
	if priorSummary != "" {
		userContent = "## 旧摘要\n" + priorSummary + "\n\n## 新对话\n" + userContent
	}

	req := &llmg.ChatRequest{
		Model: a.config.Model,
		Messages: []llmg.Message{
			{Role: llmg.RoleSystem, Content: summaryPrompt},
			{Role: llmg.RoleUser, Content: userContent},
		},
	}
	resp, err := a.client.Chat(context.Background(), req)
	if err != nil {
		return err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return fmt.Errorf("摘要响应为空")
	}
	summary := resp.Choices[0].Message.Content
	coveredID := a.history.ReplaceWithSummary(summary, len(old))

	// 持久化摘要检查点
	if a.store != nil && a.history.SessionID() != "" && coveredID > 0 {
		if err := a.store.SaveSummary(a.history.SessionID(), summary, coveredID); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] 保存摘要失败: %v\n", err)
		}
	}

	fmt.Fprintf(a.output, "  [历史已压缩: %d 条消息 → 1 条摘要]\n", len(old))
	return nil
}

// messagesToText 把消息序列转成可读文本，喂给摘要 LLM。
func messagesToText(msgs []llmg.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case llmg.RoleUser:
			fmt.Fprintf(&sb, "[用户] %s\n", m.Content)
		case llmg.RoleAssistant:
			if m.Content != "" {
				fmt.Fprintf(&sb, "[助手] %s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&sb, "[工具调用] %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
			}
		case llmg.RoleTool:
			fmt.Fprintf(&sb, "[工具结果] %s\n", m.Content)
		}
	}
	return sb.String()
}

func (a *Agent) Clear() {
	a.history.Clear()
	fmt.Fprintln(a.output, "[历史已清除]")
}

func (a *Agent) ToggleAutoApprove() {
	v := !a.hook.AutoApprove()
	a.hook.SetAutoApprove(v)
	state := "关闭"
	if v {
		state = "开启"
	}
	fmt.Fprintf(a.output, "[自动批准已%s]  破坏性操作将%s确认\n", state, map[bool]string{true: "不再", false: "需要"}[v])
}

// ---------- skills & MCP 命令 ----------

func (a *Agent) ListSkills() {
	if a.skills == nil || len(a.skills.All()) == 0 {
		fmt.Fprintln(a.output, "[无可用技能] 让 LLM 用 install_skill 创建，或 /install-skill 安装")
		return
	}
	fmt.Fprintln(a.output, "可用技能:")
	for _, s := range a.skills.All() {
		fmt.Fprintf(a.output, "  %-20s %s\n", s.Name, s.Description)
	}
}

// InstallSkill 创建一个本地技能。由 /install-skill 命令调用。
func (a *Agent) InstallSkill(name, content string) {
	if a.skills == nil {
		fmt.Fprintln(a.output, "[错误] 技能存储不可用")
		return
	}
	path, err := a.skills.Install(name, content)
	if err != nil {
		fmt.Fprintf(a.output, "[错误] %v\n", err)
		return
	}
	fmt.Fprintf(a.output, "[已安装技能] %s -> %s\n", name, path)
}

// RemoveSkill 删除一个本地技能。由 /remove-skill 命令调用。
func (a *Agent) RemoveSkill(name string) {
	if a.skills == nil {
		fmt.Fprintln(a.output, "[错误] 技能存储不可用")
		return
	}
	if err := a.skills.Remove(name); err != nil {
		fmt.Fprintf(a.output, "[错误] %v\n", err)
		return
	}
	fmt.Fprintf(a.output, "[已删除技能] %s\n", name)
}

func (a *Agent) ListMCP() {
	if a.mcp == nil || len(a.mcp.clients) == 0 {
		fmt.Fprintln(a.output, "[无 MCP server] 在 config.yaml 的 mcp 段配置")
		return
	}
	fmt.Fprintln(a.output, "MCP server:")
	for name, client := range a.mcp.clients {
		tools, err := client.ListTools(context.Background())
		if err != nil {
			fmt.Fprintf(a.output, "  %s: (工具列表获取失败: %v)\n", name, err)
			continue
		}
		fmt.Fprintf(a.output, "  %s: %d 个工具\n", name, len(tools))
		for _, t := range tools {
			fmt.Fprintf(a.output, "    - %s: %s\n", t.Name, t.Description)
		}
		// 尝试列出 resources
		resources, err := client.ListResources(context.Background())
		if err == nil && len(resources) > 0 {
			fmt.Fprintf(a.output, "  %s: %d 个资源\n", name, len(resources))
			for _, r := range resources {
				fmt.Fprintf(a.output, "    - %s: %s\n", r.URI, r.Name)
			}
		}
		// 尝试列出 prompts
		prompts, err := client.ListPrompts(context.Background())
		if err == nil && len(prompts) > 0 {
			fmt.Fprintf(a.output, "  %s: %d 个 prompt\n", name, len(prompts))
			for _, p := range prompts {
				fmt.Fprintf(a.output, "    - %s: %s\n", p.Name, p.Description)
			}
		}
	}
}

func (a *Agent) GetPrompt(input string) {
	if a.mcp == nil || len(a.mcp.clients) == 0 {
		fmt.Fprintln(a.output, "[无 MCP server]")
		return
	}
	// 解析 prompt 名和参数: /prompt name key=val key2=val2
	parts := strings.Fields(input)
	if len(parts) == 0 {
		fmt.Fprintln(a.output, "用法: /prompt <name> [key=val ...]")
		return
	}
	name := parts[0]
	args := map[string]string{}
	for _, p := range parts[1:] {
		if idx := strings.Index(p, "="); idx > 0 {
			args[p[:idx]] = p[idx+1:]
		}
	}
	text, err := a.mcp.GetPrompt(name, args)
	if err != nil {
		fmt.Fprintf(a.output, "[error] %v\n", err)
		return
	}
	fmt.Fprintln(a.output, text)
}
