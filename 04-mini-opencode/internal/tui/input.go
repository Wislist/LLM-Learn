package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wislist/mini-opencode/internal/agent"
)

func (m *Model) handleInput(input string) (tea.Model, tea.Cmd) {
	switch {
	case input == "/quit" || input == "/exit" || input == "quit" || input == "exit":
		m.state = stateQuitting
		return m, tea.Quit
	case input == "/help":
		m.addBlock(m.renderHelp())
		m.refreshViewport()
		return m, nil
	case input == "/version":
		m.addBlock(fmt.Sprintf("mini-opencode %s", m.version))
		m.refreshViewport()
		return m, nil
	case input == "/tools":
		m.addBlock(m.renderTools())
		m.refreshViewport()
		return m, nil
	case input == "/status":
		m.gitStatus = collectGitStatus(m.workingDir)
		m.addBlock(m.renderStatus())
		m.refreshViewport()
		return m, nil
	case input == "/key":
		m.state = stateKeyPrompt
		m.keyInput.Reset()
		m.keyInput.Focus()
		return m, textinput.Blink
	case strings.HasPrefix(input, "/key "):
		return m.saveKey(strings.TrimSpace(strings.TrimPrefix(input, "/key ")))
	case input == "/compact":
		return m.startCompact()
	case strings.HasPrefix(input, "/"):
		m.addBlock(errorStyle.Render("unknown command: " + input))
		m.refreshViewport()
		return m, nil
	}

	if m.runtime == nil {
		m.addBlock(errorStyle.Render("no runtime available. use /key to configure."))
		m.refreshViewport()
		return m, nil
	}

	m.addBlock(m.renderUserMessage(input))
	m.state = stateRunning
	m.input.Blur()

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel

	go func() {
		err := m.runtime.Run(ctx, input, func(event agent.Event) {
			m.program.Send(runtimeEventMsg{event: event})
		})
		m.program.Send(runtimeDoneMsg{err: err})
	}()

	return m, spinner.Tick
}

func (m *Model) saveKey(key string) (tea.Model, tea.Cmd) {
	if m.keySaver != nil {
		newCfg, err := m.keySaver(key)
		if err != nil {
			m.addBlock(errorStyle.Render("✗ " + err.Error()))
		} else {
			*m.cfg = newCfg
			m.addBlock(toolArrow.Render("[deepseek key saved]"))
			if m.runtimeFactory != nil {
				rt, rerr := m.runtimeFactory(newCfg)
				if rerr != nil {
					m.addBlock(errorStyle.Render("✗ " + rerr.Error()))
				} else {
					m.SetRuntime(rt)
				}
			}
		}
	}
	m.state = stateIdle
	m.refreshViewport()
	return m, textinput.Blink
}

func (m *Model) startCompact() (tea.Model, tea.Cmd) {
	if m.runtime == nil {
		m.addBlock(errorStyle.Render("no runtime available. use /key to configure."))
		m.refreshViewport()
		return m, nil
	}
	if len(m.runtime.Messages()) == 0 {
		m.addBlock(dimStyle.Render("nothing to compact yet"))
		m.refreshViewport()
		return m, nil
	}
	if m.compactor == nil {
		m.addBlock(errorStyle.Render("compactor not configured"))
		m.refreshViewport()
		return m, nil
	}
	m.state = stateCompacting
	m.input.Blur()
	m.addBlock(dimStyle.Render("compacting context..."))
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel

	go func() {
		summary, err := m.compactor(ctx)
		m.program.Send(compactDoneMsg{summary: summary, err: err})
	}()

	return m, spinner.Tick
}

func (m *Model) handleRuntimeEvent(event agent.Event) {
	switch event.Type {
	case agent.EventAssistantResponse:
		if event.Message != nil && event.Message.Content != "" {
			m.addBlock(m.renderAssistantMessage(event.Message.Content))
		}
	case agent.EventToolCallStarted:
		if event.ToolCall != nil {
			m.addBlock(m.renderToolCall(event.ToolCall))
		}
	case agent.EventToolCallFinished:
		if event.ToolResult != nil {
			if event.ToolResult.Error != "" {
				m.addBlock(toolError.Render("✗ " + event.ToolResult.Error))
			} else {
				content := event.ToolResult.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				m.addBlock(toolArrow.Render("→ " + content))
			}
		}
	case agent.EventToolCallFailed, agent.EventToolPermissionDenied:
		if event.ToolResult != nil && event.ToolResult.Error != "" {
			m.addBlock(toolError.Render("✗ " + event.ToolResult.Error))
		}
		if event.Error != nil && event.Error.Error() != "" {
			m.addBlock(toolError.Render("✗ " + event.Error.Error()))
		}
	}
	m.refreshViewport()
}
