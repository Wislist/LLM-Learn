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
	workDir     string
	hook        *CLIPermissionHook
	toolHistory []string // tool call 签名序列，用于 loop detection
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

	if len(config.MCP) > 0 {
		LoadMCPFromConfig(tools, config.MCP)
	}

	store, err := OpenSessionStore(config.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] session store 不可用: %v\n", err)
	}

	a := &Agent{
		client:  client,
		tools:   tools,
		config:  config,
		output:  os.Stdout,
		store:   store,
		workDir: workDir,
		hook:    NewCLIPermissionHook(config.AutoApprove),
	}

	prompt := defaultSystemPrompt + "\n\n可用工具:\n" + tools.ToolPrompt()
	a.history = NewHistory(prompt, config.MaxHist)

	if store != nil {
		a.resumeOrNew()
	}
	return a
}

func (a *Agent) resumeOrNew() {
	if latest, err := a.store.LatestSession(); err == nil && latest != nil {
		a.history.BindSession(latest.ID, a.store)
		if err := a.history.LoadSession(latest.ID); err != nil {
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
	if err := a.history.LoadSession(id); err != nil {
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

func (a *Agent) Run(input string) {
	if a.history == nil {
		prompt := defaultSystemPrompt + "\n\n可用工具:\n" + a.tools.ToolPrompt()
		a.history = NewHistory(prompt, a.config.MaxHist)
	}

	a.history.Add(llmg.Message{Role: llmg.RoleUser, Content: input})

	if a.store != nil && a.history.SessionID() != "" {
		if count, err := a.store.MessageCount(a.history.SessionID()); err == nil && count <= 1 {
			title := input
			if len([]rune(title)) > 40 {
				title = string([]rune(title)[:40]) + "..."
			}
			_ = a.store.RenameSession(a.history.SessionID(), title)
		}
	}

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		// auto-summarize：历史过长时压缩旧消息
		if a.history.NeedsSummary() {
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

		// loop detection：把本轮 tool calls 签名追加进历史，检测死循环
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

			// 权限钩子：拦截破坏性 / 越权操作。
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
func (a *Agent) summarizeHistory() error {
	old := a.history.Summarizable()
	if len(old) < 2 {
		return nil
	}
	fmt.Fprintln(a.output, "  [压缩历史中…]")

	req := &llmg.ChatRequest{
		Model: a.config.Model,
		Messages: []llmg.Message{
			{Role: llmg.RoleSystem, Content: summaryPrompt},
			{Role: llmg.RoleUser, Content: messagesToText(old)},
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
	a.history.ReplaceWithSummary(summary, len(old))
	fmt.Fprintf(a.output, "  [历史已压缩: %d 条消息 → 1 条摘要]\n", len(old))
	return nil
}

// messagesToText 把消息序列转成可读文本，喂给摘要 LLM。
func messagesToText(msgs []llmg.Message) string {
	var sb strings.Builder
	for i, m := range msgs {
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
		_ = i
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
