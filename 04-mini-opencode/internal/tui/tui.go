package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wislist/mini-opencode/internal/agent"
	"github.com/wislist/mini-opencode/internal/config"
)

type appState int

const (
	stateIdle appState = iota
	stateRunning
	statePermission
	stateKeyPrompt
	stateQuitting
)

// Messages bridging the synchronous runtime goroutine into Bubble Tea.
type runtimeEventMsg struct{ event agent.Event }
type runtimeDoneMsg struct{ err error }
type permissionRequestMsg struct {
	call   agent.ToolCall
	result *agent.ToolResult
	resp   chan bool
}

// Callbacks the TUI needs from app.go.
type KeySaver       func(key string) (config.Config, error)
type RuntimeFactory func(cfg config.Config) (*agent.Runtime, error)

type Model struct {
	viewport   viewport.Model
	input      textinput.Model
	spinner    spinner.Model
	keyInput   textinput.Model

	state      appState
	runtime    *agent.Runtime
	program    *tea.Program
	cfg        *config.Config
	workingDir string
	version    string

	blocks     []string
	width      int
	height     int

	pendingPerm    *permissionRequestMsg
	keySaver       KeySaver
	runtimeFactory RuntimeFactory

	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg *config.Config, workingDir, ver string) *Model {
	vp := viewport.New(80, 20)

	ti := textinput.New()
	ti.Placeholder = "ask anything...  (/help for commands)"
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	ki := textinput.New()
	ki.Prompt = ""
	ki.EchoMode = textinput.EchoPassword
	ki.CharLimit = 0

	return &Model{
		viewport:   vp,
		input:      ti,
		spinner:    sp,
		keyInput:   ki,
		state:      stateIdle,
		cfg:        cfg,
		workingDir: workingDir,
		version:    ver,
	}
}

func (m *Model) SetRuntime(rt *agent.Runtime)          { m.runtime = rt }
func (m *Model) SetProgram(p *tea.Program)             { m.program = p }
func (m *Model) SetKeySaver(ks KeySaver)               { m.keySaver = ks }
func (m *Model) SetRuntimeFactory(rf RuntimeFactory)   { m.runtimeFactory = rf }
func (m *Model) MakeConfirmer() agent.PermissionConfirmer {
	return func(ctx context.Context, call agent.ToolCall, result agent.ToolResult) bool {
		resp := make(chan bool, 1)
		m.program.Send(permissionRequestMsg{call: call, result: &result, resp: resp})
		select {
		case <-ctx.Done():
			return false
		case ok := <-resp:
			return ok
		}
	}
}

func (m *Model) Init() tea.Cmd {
	m.addBlock(dimStyle.Render("welcome to mini-opencode") + "\n" +
		dimStyle.Render("type /help for commands, or just start typing."))
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = max(1, msg.Height-7)
		m.refreshViewport()
		return m, nil

	case spinner.TickMsg:
		if m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case runtimeEventMsg:
		m.handleRuntimeEvent(msg.event)

	case runtimeDoneMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.addBlock(errorStyle.Render("✗ " + msg.err.Error()))
		}
		m.refreshViewport()
		m.input.Focus()
		cmds = append(cmds, textinput.Blink)

	case permissionRequestMsg:
		m.pendingPerm = &msg
		m.state = statePermission
		m.refreshViewport()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	if m.state == stateIdle || m.state == stateRunning {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.state == stateKeyPrompt {
		m.keyInput, cmd = m.keyInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateIdle:
		return m.handleIdleKey(msg)
	case stateRunning:
		return m.handleRunningKey(msg)
	case statePermission:
		return m.handlePermissionKey(msg)
	case stateKeyPrompt:
		return m.handleKeyPromptKey(msg)
	case stateQuitting:
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleIdleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		m.state = stateQuitting
		return m, tea.Quit
	case tea.KeyEnter:
		input := strings.TrimSpace(m.input.Value())
		if input == "" {
			return m, nil
		}
		m.input.Reset()
		return m.handleInput(input)
	case tea.KeyUp:
		m.viewport.LineUp(1)
		return m, nil
	case tea.KeyDown:
		m.viewport.LineDown(1)
		return m, nil
	case tea.KeyPgUp:
		m.viewport.HalfViewUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleRunningKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.cancel != nil {
			m.cancel()
		}
		m.state = stateIdle
		m.addBlock(errorStyle.Render("✗ interrupted"))
		m.refreshViewport()
		return m, textinput.Blink
	case tea.KeyUp:
		m.viewport.LineUp(1)
	case tea.KeyDown:
		m.viewport.LineDown(1)
	}
	return m, nil
}

func (m *Model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.pendingPerm != nil {
			m.pendingPerm.resp <- true
		}
		m.pendingPerm = nil
		m.state = stateRunning
		m.refreshViewport()
	case "n", "N", "esc":
		if m.pendingPerm != nil {
			m.pendingPerm.resp <- false
		}
		m.pendingPerm = nil
		m.state = stateRunning
		m.refreshViewport()
	}
	return m, nil
}

func (m *Model) handleKeyPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		key := strings.TrimSpace(m.keyInput.Value())
		m.keyInput.Reset()
		if key == "" {
			m.state = stateIdle
			m.addBlock(errorStyle.Render("key input cancelled"))
			m.refreshViewport()
			return m, textinput.Blink
		}
		return m.saveKey(key)
	case tea.KeyCtrlC, tea.KeyEsc:
		m.keyInput.Reset()
		m.state = stateIdle
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}

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
	case input == "/key":
		m.state = stateKeyPrompt
		m.keyInput.Reset()
		m.keyInput.Focus()
		return m, textinput.Blink
	case strings.HasPrefix(input, "/key "):
		return m.saveKey(strings.TrimSpace(strings.TrimPrefix(input, "/key ")))
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
			m.addBlock(errorStyle.Render("✗ "+err.Error()))
		} else {
			*m.cfg = newCfg
			m.addBlock(toolArrow.Render("[deepseek key saved]"))
			if m.runtimeFactory != nil {
				rt, rerr := m.runtimeFactory(newCfg)
				if rerr != nil {
					m.addBlock(errorStyle.Render("✗ "+rerr.Error()))
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

// ── View ──────────────────────────────────────────────

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.viewport.View())
	if m.state == statePermission && m.pendingPerm != nil {
		sections = append(sections, m.renderPermissionPrompt())
	} else if m.state == stateKeyPrompt {
		sections = append(sections, m.renderKeyPrompt())
	} else {
		sections = append(sections, m.renderInputBar())
	}
	sections = append(sections, m.renderHelpBar())
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *Model) renderHeader() string {
	left := headerStyle.Render("◆ mini-opencode")
	right := dimStyle.Render(fmt.Sprintf("v%s · %s", m.version, m.cfg.Provider.Name))
	space := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return left + strings.Repeat(" ", space) + right
}

func (m *Model) renderInputBar() string {
	return inputBorder.Render(promptStyle.Render("❯") + " " + m.input.View())
}

func (m *Model) renderKeyPrompt() string {
	return permBox.Render(keyLabel.Render("DeepSeek API Key:") + " " + m.keyInput.View())
}

func (m *Model) renderPermissionPrompt() string {
	if m.pendingPerm == nil {
		return ""
	}
	call := m.pendingPerm.call
	content := toolName.Render(call.Name) + "\n" +
		dimStyle.Render(extractToolDetail(call)) + "\n" +
		permAsk.Render("allow? [y/N]")
	return permBox.Render(content)
}

func (m *Model) renderHelpBar() string {
	if m.state == stateRunning {
		return spinnerStyle.Render(m.spinner.View()) + " " + dimStyle.Render("thinking...  ctrl+c to interrupt")
	}
	left := dimStyle.Render("/help /version /tools /key /quit")
	right := dimStyle.Render("↑↓ scroll")
	space := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", space) + right
}

func (m *Model) renderUserMessage(text string) string {
	return userLabel.Render("▸ you") + "\n" + userText.Render(text)
}

func (m *Model) renderAssistantMessage(text string) string {
	return assistantLabel.Render("◂ assistant") + "\n" + assistantText.Render(text)
}

func (m *Model) renderToolCall(call *agent.ToolCall) string {
	return toolBox.Render(toolName.Render(call.Name) + "\n" + dimStyle.Render(extractToolDetail(*call)))
}

func (m *Model) renderTools() string {
	if m.runtime == nil {
		return dimStyle.Render("no runtime")
	}
	var lines []string
	lines = append(lines, toolName.Render("tools:"))
	for _, t := range m.runtime.Tools() {
		lines = append(lines, fmt.Sprintf("  %-12s %s", t.Name, t.Description))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderHelp() string {
	return toolName.Render("mini-opencode") + "\n" +
		dimStyle.Render("  a fresh Go agent terminal\n\n") +
		"  " + lipgloss.NewStyle().Foreground(colorCyan).Render("/help") + "    show this help\n" +
		"  " + lipgloss.NewStyle().Foreground(colorCyan).Render("/version") + " show version\n" +
		"  " + lipgloss.NewStyle().Foreground(colorCyan).Render("/tools") + "   list registered tools\n" +
		"  " + lipgloss.NewStyle().Foreground(colorCyan).Render("/key") + "     set DeepSeek API key\n" +
		"  " + lipgloss.NewStyle().Foreground(colorCyan).Render("/quit") + "    exit"
}

// ── helpers ───────────────────────────────────────────

func (m *Model) addBlock(block string) {
	m.blocks = append(m.blocks, block)
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(strings.Join(m.blocks, "\n\n"))
	m.viewport.GotoBottom()
}

func extractToolDetail(call agent.ToolCall) string {
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return string(call.Arguments)
	}
	for _, key := range []string{"command", "path", "file_path", "pattern", "query", "content"} {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%s: %v", key, v)
		}
	}
	var parts []string
	count := 0
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
		count++
		if count >= 3 {
			break
		}
	}
	return strings.Join(parts, "  ")
}
