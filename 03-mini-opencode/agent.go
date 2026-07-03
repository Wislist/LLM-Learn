package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wislist/llmg"
)

const defaultSystemPrompt = `你是一个 CLI Agent，能够读写文件和执行命令。
 
 行为规则：
 - 使用工具完成任务。先读文件再修改。
 - 每次只做一件事，不要一次调用过多工具。
 - 如果 bash 命令失败，分析错误信息后重试。
 - 回答简洁，不要输出多余的解释。`

type Agent struct {
	client  *llmg.Client
	tools   *ToolRegistry
	history *History
	config  Config
	output  io.Writer
	store   *SessionStore
	workDir string
	hook    *CLIPermissionHook
}

func NewAgent(client *llmg.Client, config Config, workDir string) *Agent {
	tools := NewToolRegistry()
	tools.Register(&readFileTool{})
	tools.Register(&writeFileTool{})
	tools.Register(&bashTool{workDir: workDir})

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
	fmt.Fprintf(a.output, "会话: %s\n标题: %s\n模型: %s\n更新: %s\n",
		sess.ID, sess.Title, sess.Model, sess.UpdatedAt)
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
